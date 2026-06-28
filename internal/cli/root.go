// Package cli wires up the kxdiff command-line interface (cobra).
//
// It resolves the --from/--to flags against the kubeconfig, connects to each
// cluster, fetches and normalizes objects, then matches and diffs them.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

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

// errDifferencesFound is returned (after the report is printed) when the two
// environments differ, so the process can exit 1 — the CI-gate contract.
var errDifferencesFound = errors.New("differences found")

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
		noColor          bool
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
		SilenceErrors: true,
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
			p := palette{enabled: useColor(noColor)}
			return runDiff(cmd.Context(), cmd.OutOrStdout(), p, kubeconfig, fromEnv, toEnv, fetchOpts, normOpts)
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
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable coloured output")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")

	return cmd
}

// Execute builds and runs the root command and returns the process exit code:
// 0 = no differences, 1 = differences found, 2 = error.
func Execute(info BuildInfo) int {
	cmd := NewRootCmd(info)
	switch err := cmd.Execute(); {
	case err == nil:
		return 0
	case errors.Is(err, errDifferencesFound):
		return 1
	default:
		cmd.PrintErrln("Error:", err)
		return 2
	}
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

// runDiff fetches both environments, matches their objects, diffs matched pairs
// and prints the report. It returns errDifferencesFound when anything differs.
func runDiff(ctx context.Context, out io.Writer, p palette, kubeconfigPath string, from, to model.Environment, fetchOpts fetch.Options, normOpts normalize.Options) error {
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
	if err := printReport(out, p, from, to, fromRes, toRes, m, differing, same); err != nil {
		return err
	}

	if len(m.OnlyFrom) > 0 || len(m.OnlyTo) > 0 || len(differing) > 0 {
		return errDifferencesFound
	}
	return nil
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

// printReport renders the full diff: a summary header, any warnings, the
// only-from / only-to buckets, and the field-level differences of changed pairs.
func printReport(out io.Writer, p palette, from, to model.Environment, fromRes, toRes fetch.Result, m match.Result, differing []resourceChange, same int) error {
	lw := &lineWriter{w: out}
	fromLabel, toLabel := envLabel(from), envLabel(to)

	lw.printf("%s %s  <->  %s\n", p.bold("ENVIRONMENTS:"), fromLabel, toLabel)
	lw.printf("only in %s: %d | only in %s: %d | differs: %d | same: %d\n",
		fromLabel, len(m.OnlyFrom), toLabel, len(m.OnlyTo), len(differing), same)

	for _, w := range fromRes.Warnings {
		lw.printf("  warning (%s): %s\n", fromLabel, w)
	}
	for _, w := range toRes.Warnings {
		lw.printf("  warning (%s): %s\n", toLabel, w)
	}

	printBucket(lw, p, "only in "+fromLabel, refs(m.OnlyFrom), p.red)
	printBucket(lw, p, "only in "+toLabel, refs(m.OnlyTo), p.green)

	lw.printf("\n%s\n", p.bold("differs:"))
	if len(differing) == 0 {
		lw.printf("  (none)\n")
	}
	for _, c := range differing {
		lw.printf("  %s\n", p.yellow(c.ref))
		for _, f := range c.fields {
			lw.printf("      %s  %s → %s\n", f.Path, p.red(f.From), p.green(f.To))
		}
	}
	return lw.err
}

func printBucket(lw *lineWriter, p palette, title string, items []string, color func(string) string) {
	lw.printf("\n%s\n", p.bold(title+":"))
	if len(items) == 0 {
		lw.printf("  (none)\n")
		return
	}
	for _, it := range items {
		lw.printf("  %s\n", color(it))
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

// palette renders ANSI colours, or plain text when disabled.
type palette struct{ enabled bool }

func (p palette) red(s string) string    { return p.wrap("31", s) }
func (p palette) green(s string) string  { return p.wrap("32", s) }
func (p palette) yellow(s string) string { return p.wrap("33", s) }
func (p palette) bold(s string) string   { return p.wrap("1", s) }

func (p palette) wrap(code, s string) string {
	if !p.enabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// useColor decides whether to colour output: off when --no-color or NO_COLOR is
// set, or when stdout is not a terminal (piped / redirected).
func useColor(noColor bool) bool {
	if noColor || os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTerminal(os.Stdout)
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
