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
	"github.com/fevzisahinler/kxdiff/internal/discovery"
	"github.com/fevzisahinler/kxdiff/internal/fetch"
	"github.com/fevzisahinler/kxdiff/internal/match"
	"github.com/fevzisahinler/kxdiff/internal/model"
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
			opts := fetch.Options{IncludeGenerated: includeGenerated}
			return runDiff(cmd.Context(), cmd.OutOrStdout(), kubeconfig, fromEnv, toEnv, opts)
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

// runDiff fetches both environments, matches their objects and reports which
// resources are only on one side and which are in both. Content-level diffing
// of the "in both" pairs is the next step.
func runDiff(ctx context.Context, out io.Writer, kubeconfigPath string, from, to model.Environment, opts fetch.Options) error {
	fromRes, err := fetchEnvironment(ctx, kubeconfigPath, from, opts)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	toRes, err := fetchEnvironment(ctx, kubeconfigPath, to, opts)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}

	// Namespace is part of identity only when both sides span all namespaces;
	// otherwise namespace<->namespace comparisons must line up by name alone.
	includeNamespace := from.Namespace == "" && to.Namespace == ""
	result := match.Match(fromRes.Objects, toRes.Objects, includeNamespace)

	return printMatch(out, from, to, fromRes, toRes, result)
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

// printMatch renders the match result: a summary header, any warnings, then the
// three buckets (only-from, only-to, in-both).
func printMatch(out io.Writer, from, to model.Environment, fromRes, toRes fetch.Result, m match.Result) error {
	lw := &lineWriter{w: out}
	fromLabel, toLabel := envLabel(from), envLabel(to)

	lw.printf("ENVIRONMENTS: %s  <->  %s\n", fromLabel, toLabel)
	lw.printf("only in %s: %d | only in %s: %d | in both: %d\n",
		fromLabel, len(m.OnlyFrom), toLabel, len(m.OnlyTo), len(m.Both))

	for _, w := range fromRes.Warnings {
		lw.printf("  warning (%s): %s\n", fromLabel, w)
	}
	for _, w := range toRes.Warnings {
		lw.printf("  warning (%s): %s\n", toLabel, w)
	}

	printBucket(lw, "only in "+fromLabel, refs(m.OnlyFrom))
	printBucket(lw, "only in "+toLabel, refs(m.OnlyTo))
	printBucket(lw, "in both (content diff coming next)", pairRefs(m.Both))
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

func refs(objs []*unstructured.Unstructured) []string {
	out := make([]string, 0, len(objs))
	for _, o := range objs {
		out = append(out, o.GetKind()+"/"+o.GetName())
	}
	return out
}

func pairRefs(pairs []match.Pair) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.From.GetKind()+"/"+p.From.GetName())
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
