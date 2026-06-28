// Package cli wires up the kxdiff command-line interface (cobra).
//
// At this stage it resolves the --from/--to flags against the kubeconfig and
// connects to each cluster to discover its resource types; the diff engine
// itself is not implemented yet.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/fevzisahinler/kxdiff/internal/config"
	"github.com/fevzisahinler/kxdiff/internal/discovery"
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
		from       string
		to         string
		kubeconfig string
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
			return runDiff(cmd.OutOrStdout(), kubeconfig, fromEnv, toEnv)
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

// runDiff connects to both environments and reports the resource types each
// cluster exposes. The diff engine itself is not implemented yet.
func runDiff(out io.Writer, kubeconfigPath string, from, to model.Environment) error {
	if err := reportEnvironment(out, kubeconfigPath, "from", from); err != nil {
		return err
	}
	return reportEnvironment(out, kubeconfigPath, "to", to)
}

// reportEnvironment connects to one environment's context, discovers its
// resource types and prints a short summary. Connection and authentication
// failures are reported clearly, naming the context that could not be reached.
func reportEnvironment(out io.Writer, kubeconfigPath, label string, env model.Environment) error {
	lw := &lineWriter{w: out}
	lw.printf("%s: context=%q namespace=%q\n", label, env.Context, env.Namespace)

	rc, err := config.RestConfig(kubeconfigPath, env.Context)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}

	result, err := discovery.ListResourceTypes(rc)
	if err != nil {
		return fmt.Errorf("%s: cannot reach context %q: %w", label, env.Context, err)
	}

	for _, w := range result.Warnings {
		lw.printf("  warning: %s\n", w)
	}
	lw.printf("  discovered %d resource types\n", len(result.Types))
	for _, rt := range result.Types {
		lw.printf("    - %s\n", rt)
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
