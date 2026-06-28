// Package cli wires up the kxdiff command-line interface (cobra).
//
// It resolves the --from/--to flags against the kubeconfig, connects to each
// cluster, fetches and normalizes objects, then matches and diffs them and
// renders the result as text, JSON or markdown.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

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
		output           string
		includeGenerated bool
		revealSecrets    bool
		noColor          bool
		allNamespaces    bool
		quiet            bool
		onlyFrom         bool
		onlyTo           bool
		onlyDiff         bool
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(output); err != nil {
				return err
			}
			kc, err := config.LoadKubeconfig(kubeconfig)
			if err != nil {
				return err
			}
			fromEnv, toEnv, err := resolveBoth(kc, from, to)
			if err != nil {
				return err
			}
			fetchOpts := fetch.Options{
				IncludeGenerated:        includeGenerated,
				Selectors:               parseSelectors(args),
				IncludeSystemNamespaces: allNamespaces,
			}
			normOpts := normalize.Options{DropNamespace: true, RevealSecrets: revealSecrets}
			view := viewOptions{quiet: quiet, onlyFrom: onlyFrom, onlyTo: onlyTo, onlyDiff: onlyDiff}
			p := palette{enabled: useColor(noColor)}
			return runDiff(cmd.Context(), cmd.OutOrStdout(), p, view, output, kubeconfig, fromEnv, toEnv, fetchOpts, normOpts)
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
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text, json or markdown")
	cmd.Flags().BoolVarP(&allNamespaces, "all-namespaces", "A", false,
		"in whole-context mode, also compare system namespaces (kube-system, ...)")
	cmd.Flags().BoolVar(&includeGenerated, "include-generated", false,
		"include controller-managed and system objects (pods, replicasets, events, owned objects, ...)")
	cmd.Flags().BoolVar(&revealSecrets, "reveal-secrets", false,
		"show raw Secret values instead of hashing them (use with care)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable coloured output")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false,
		"print nothing; rely on the exit code (0 = no diff, 1 = diff, 2 = error)")
	cmd.Flags().BoolVar(&onlyFrom, "only-from", false, "show only resources present only in --from")
	cmd.Flags().BoolVar(&onlyTo, "only-to", false, "show only resources present only in --to")
	cmd.Flags().BoolVar(&onlyDiff, "only-diff", false, "show only resources that differ")
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

const (
	outputText     = "text"
	outputJSON     = "json"
	outputMarkdown = "markdown"
)

func validateOutput(output string) error {
	switch output {
	case "", outputText, outputJSON, outputMarkdown:
		return nil
	default:
		return fmt.Errorf("invalid --output %q (want %s, %s or %s)",
			output, outputText, outputJSON, outputMarkdown)
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

// parseSelectors turns positional TYPE[/NAME] arguments into discovery selectors.
func parseSelectors(args []string) []discovery.Selector {
	out := make([]discovery.Selector, 0, len(args))
	for _, a := range args {
		typ, name, _ := strings.Cut(a, "/")
		out = append(out, discovery.Selector{Type: typ, Name: name})
	}
	return out
}

// runDiff fetches both environments, builds the diff report and renders it.
// It returns errDifferencesFound when anything differs.
func runDiff(ctx context.Context, out io.Writer, p palette, view viewOptions, output, kubeconfigPath string, from, to model.Environment, fetchOpts fetch.Options, normOpts normalize.Options) error {
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

	report := model.DiffReport{
		From:     envLabel(from),
		To:       envLabel(to),
		OnlyFrom: toRefs(m.OnlyFrom),
		OnlyTo:   toRefs(m.OnlyTo),
		Differs:  differing,
		Same:     same,
		Warnings: combineWarnings(envLabel(from), fromRes.Warnings, envLabel(to), toRes.Warnings),
	}

	if !view.quiet {
		if err := renderReport(out, p, view, output, report); err != nil {
			return err
		}
	}

	if len(report.OnlyFrom) > 0 || len(report.OnlyTo) > 0 || len(report.Differs) > 0 {
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

// diffPairs normalizes and diffs each matched pair, returning the ones that
// differ and a count of those that are identical after normalization.
func diffPairs(pairs []match.Pair, opts normalize.Options) ([]model.ResourceDiff, int) {
	var differing []model.ResourceDiff
	same := 0
	for _, p := range pairs {
		fields := diff.Objects(normalize.Normalize(p.From, opts), normalize.Normalize(p.To, opts))
		if len(fields) == 0 {
			same++
			continue
		}
		differing = append(differing, model.ResourceDiff{
			Kind:   p.From.GetKind(),
			Name:   p.From.GetName(),
			Fields: fields,
		})
	}
	return differing, same
}

func combineWarnings(fromLabel string, fromW []string, toLabel string, toW []string) []string {
	out := make([]string, 0, len(fromW)+len(toW))
	for _, w := range fromW {
		out = append(out, fmt.Sprintf("(%s) %s", fromLabel, w))
	}
	for _, w := range toW {
		out = append(out, fmt.Sprintf("(%s) %s", toLabel, w))
	}
	return out
}

func toRefs(objs []*unstructured.Unstructured) []model.ResourceRef {
	out := make([]model.ResourceRef, 0, len(objs))
	for _, o := range objs {
		out = append(out, model.ResourceRef{Kind: o.GetKind(), Name: o.GetName()})
	}
	return out
}

func envLabel(e model.Environment) string {
	if e.Namespace == "" {
		return e.Context
	}
	return e.Context + "/" + e.Namespace
}
