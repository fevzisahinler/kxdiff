// Package cli wires up the kxdiff command-line interface (cobra).
//
// At this stage it resolves the --from/--to flags against the kubeconfig and
// connects to each cluster to discover its resource types; the diff engine
// itself is not implemented yet.
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/fevzisahinler/kxdiff/internal/config"
	"github.com/fevzisahinler/kxdiff/internal/diff"
	"github.com/fevzisahinler/kxdiff/internal/discovery"
	"github.com/fevzisahinler/kxdiff/internal/fetch"
	"github.com/fevzisahinler/kxdiff/internal/match"
	"github.com/fevzisahinler/kxdiff/internal/model"
	"github.com/fevzisahinler/kxdiff/internal/normalize"
)

// BuildInfo carries link-time build metadata into the CLI for --version.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRootCmd builds the root cobra command. It is a pure constructor (no I/O,
// no globals) so it can be exercised directly in tests.
func NewRootCmd(info BuildInfo) *cobra.Command {
	var (
		from             string
		to               string
		kubeconfig       string
		includeGenerated bool
		revealSecrets    bool
	)

	cmd := &cobra.Command{
		Use:   "kxdiff [TYPE[/NAME]...]",
		Short: "Read-only diff between two Kubernetes environments",
		Long: "kxdiff compares two Kubernetes environments (context + namespace) " +
			"and reports what differs.\n\n" +
			"It is strictly read-only: only get/list verbs are used, nothing is " +
			"ever created, changed or deleted.",
		Version:       info.Version,
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kc, err := config.LoadKubeconfig(kubeconfig)
			if err != nil {
				return err
			}
			fromEnv, toEnv, err := resolveBoth(kc, from, to)
			if err != nil {
				return err
			}
			fetchOpts := fetch.Options{IncludeGenerated: includeGenerated}
			normOpts := normalize.Options{DropNamespace: true, RevealSecrets: revealSecrets}
			return runDiff(cmd.Context(), cmd.OutOrStdout(), kubeconfig, fromEnv, toEnv, fetchOpts, normOpts)
		},
	}

	cmd.SetVersionTemplate(
		fmt.Sprintf("kxdiff version %s (commit %s, built %s)\n",
			info.Version, info.Commit, info.Date),
	)

	cmd.Flags().StringVar(&from, "from", "", "source environment: [context][/namespace]")
	cmd.Flags().StringVar(&to, "to", "", "target environment: [context][/namespace]")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "",
		"path to the kubeconfig file (overrides KUBECONFIG and the default)")
	cmd.Flags().BoolVar(&includeGenerated, "include-generated", false,
		"include controller-managed and system objects (pods, replicasets, events, owned objects, ...)")
	cmd.Flags().BoolVar(&revealSecrets, "reveal-secrets", false,
		"show raw Secret values instead of hashing them (use with care)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")

	return cmd
}

// resolveBoth resolves the --from and --to values against the kubeconfig,
// attributing any error to the right flag.
func resolveBoth(kc config.Kubeconfig, from, to string) (model.Environment, model.Environment, error) {
	fromEnv, err := config.ResolveEnvironment(kc, from)
	if err != nil {
		return model.Environment{}, model.Environment{}, fmt.Errorf("--from: %w", err)
	}
	toEnv, err := config.ResolveEnvironment(kc, to)
	if err != nil {
		return model.Environment{}, model.Environment{}, fmt.Errorf("--to: %w", err)
	}
	return fromEnv, toEnv, nil
}

// runDiff fetches both environments, matches their objects, diffs the content of
// matched pairs and reports the result.
func runDiff(ctx context.Context, out io.Writer, kubeconfigPath string, from, to model.Environment, fetchOpts fetch.Options, normOpts normalize.Options) error {
	fromRes, err := fetchEnvironment(ctx, kubeconfigPath, from, fetchOpts)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	toRes, err := fetchEnvironment(ctx, kubeconfigPath, to, fetchOpts)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}

	// Namespace is part of identity only when both sides span all namespaces;
	// otherwise namespace<->namespace comparisons must line up by name alone.
	includeNamespace := from.Namespace == "" && to.Namespace == ""
	m := match.Match(fromRes.Objects, toRes.Objects, includeNamespace)

	differing, same := diffPairs(m.Both, normOpts)
	return printReport(out, from, to, fromRes, toRes, m, differing, same)
}

// resourceChange is a matched resource whose content differs.
type resourceChange struct {
	ref    string
	fields []model.FieldDiff
}

// diffPairs normalizes and diffs each matched pair, returning the ones that
// differ and a count of those that are identical after normalization.
func diffPairs(pairs []match.Pair, opts normalize.Options) ([]resourceChange, int) {
	var differing []resourceChange
	same := 0
	for _, p := range pairs {
		fields := diff.Objects(normalize.Normalize(p.From, opts), normalize.Normalize(p.To, opts))
		if len(fields) == 0 {
			same++
			continue
		}
		differing = append(differing, resourceChange{ref: ref(p.From), fields: fields})
	}
	return differing, same
}

// fetchEnvironment connects to one environment, discovers its resource types and
// fetches the live objects. Connection failures name the unreachable context.
func fetchEnvironment(ctx context.Context, kubeconfigPath string, env model.Environment, opts fetch.Options) (fetch.Result, error) {
	rc, err := config.RestConfig(kubeconfigPath, env.Context)
	if err != nil {
		return fetch.Result{}, err
	}

	disco, err := discovery.ListResourceTypes(rc)
	if err != nil {
		return fetch.Result{}, fmt.Errorf("cannot reach context %q: %w", env.Context, err)
	}

	lister, err := fetch.NewListerForConfig(rc)
	if err != nil {
		return fetch.Result{}, err
	}

	res, err := fetch.Fetch(ctx, lister, env, disco.Types, opts)
	if err != nil {
		return fetch.Result{}, err
	}
	res.Warnings = append(disco.Warnings, res.Warnings...)
	return res, nil
}

// printReport renders the full diff: a summary header, any warnings, the
// only-from / only-to buckets, and the field-level differences of changed pairs.
func printReport(out io.Writer, from, to model.Environment, fromRes, toRes fetch.Result, m match.Result, differing []resourceChange, same int) error {
	lw := &lineWriter{w: out}
	fromLabel, toLabel := envLabel(from), envLabel(to)

	lw.printf("ENVIRONMENTS: %s  <->  %s\n", fromLabel, toLabel)
	lw.printf("only in %s: %d | only in %s: %d | differs: %d | same: %d\n",
		fromLabel, len(m.OnlyFrom), toLabel, len(m.OnlyTo), len(differing), same)

	for _, w := range fromRes.Warnings {
		lw.printf("  warning (%s): %s\n", fromLabel, w)
	}
	for _, w := range toRes.Warnings {
		lw.printf("  warning (%s): %s\n", toLabel, w)
	}

	printBucket(lw, "only in "+fromLabel, refs(m.OnlyFrom))
	printBucket(lw, "only in "+toLabel, refs(m.OnlyTo))

	lw.printf("\ndiffers:\n")
	if len(differing) == 0 {
		lw.printf("  (none)\n")
	}
	for _, c := range differing {
		lw.printf("  %s\n", c.ref)
		for _, f := range c.fields {
			lw.printf("      %s  %s → %s\n", f.Path, f.From, f.To)
		}
	}
	return lw.err
}

func printBucket(lw *lineWriter, title string, items []string) {
	lw.printf("\n%s:\n", title)
	if len(items) == 0 {
		lw.printf("  (none)\n")
		return
	}
	for _, it := range items {
		lw.printf("  %s\n", it)
	}
}

func ref(o *unstructured.Unstructured) string {
	return o.GetKind() + "/" + o.GetName()
}

func refs(objs []*unstructured.Unstructured) []string {
	out := make([]string, 0, len(objs))
	for _, o := range objs {
		out = append(out, ref(o))
	}
	return out
}

func envLabel(e model.Environment) string {
	if e.Namespace == "" {
		return e.Context
	}
	return e.Context + "/" + e.Namespace
}

// lineWriter writes formatted lines to w, remembering the first write error so
// callers can check it once at the end.
type lineWriter struct {
	w   io.Writer
	err error
}

func (lw *lineWriter) printf(format string, args ...any) {
	if lw.err == nil {
		_, lw.err = fmt.Fprintf(lw.w, format, args...)
	}
}

// Execute builds and runs the root command. It is the single entry point used
// by main.
func Execute(info BuildInfo) error {
	return NewRootCmd(info).Execute()
}
