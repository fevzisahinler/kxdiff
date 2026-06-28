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

	"github.com/fevzisahinler/kxdiff/internal/config"
	"github.com/fevzisahinler/kxdiff/internal/discovery"
	"github.com/fevzisahinler/kxdiff/internal/fetch"
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

// runDiff connects to both environments and reports the objects each one holds.
// The diff engine itself is not implemented yet.
func runDiff(ctx context.Context, out io.Writer, kubeconfigPath string, from, to model.Environment, opts fetch.Options) error {
	if err := reportEnvironment(ctx, out, kubeconfigPath, "from", from, opts); err != nil {
		return err
	}
	return reportEnvironment(ctx, out, kubeconfigPath, "to", to, opts)
}

// reportEnvironment connects to one environment's context, discovers its
// resource types, fetches the live objects and prints a short summary.
// Connection and authentication failures are reported clearly, naming the
// context that could not be reached.
func reportEnvironment(ctx context.Context, out io.Writer, kubeconfigPath, label string, env model.Environment, opts fetch.Options) error {
	lw := &lineWriter{w: out}
	lw.printf("%s: context=%q namespace=%q\n", label, env.Context, env.Namespace)

	rc, err := config.RestConfig(kubeconfigPath, env.Context)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}

	disco, err := discovery.ListResourceTypes(rc)
	if err != nil {
		return fmt.Errorf("%s: cannot reach context %q: %w", label, env.Context, err)
	}

	lister, err := fetch.NewListerForConfig(rc)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}

	result, err := fetch.Fetch(ctx, lister, env, disco.Types, opts)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}

	for _, w := range disco.Warnings {
		lw.printf("  warning: %s\n", w)
	}
	for _, w := range result.Warnings {
		lw.printf("  warning: %s\n", w)
	}
	lw.printf("  fetched %d objects\n", len(result.Objects))
	for _, o := range result.Objects {
		lw.printf("    - %s/%s\n", o.GetKind(), o.GetName())
	}
	return lw.err
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
