// Package cli wires up the kxdiff command-line interface (cobra).
//
// At this stage it defines the root command and parses its --from/--to flags
// into environments; the diff engine itself is not implemented yet.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

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
		from string
		to   string
	)

	cmd := &cobra.Command{
		Use:   "kxdiff [TYPE[/NAME]...]",
		Short: "Read-only diff between two Kubernetes environments",
		Long: "kxdiff compares two Kubernetes environments (context + namespace) " +
			"and reports what differs.\n\n" +
			"It is strictly read-only: only get/list verbs are used, nothing is " +
			"ever created, changed or deleted.",
		Version: info.Version,
		// Print errors ourselves cleanly; don't dump usage on a runtime error.
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiff(cmd.OutOrStdout(), from, to)
		},
	}

	cmd.SetVersionTemplate(
		fmt.Sprintf("kxdiff version %s (commit %s, built %s)\n",
			info.Version, info.Commit, info.Date),
	)

	cmd.Flags().StringVar(&from, "from", "", "source environment: [context][/namespace]")
	cmd.Flags().StringVar(&to, "to", "", "target environment: [context][/namespace]")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")

	return cmd
}

// runDiff parses the --from/--to environments and reports them. The actual
// diff is not implemented yet; for now it confirms the inputs were understood.
func runDiff(out io.Writer, from, to string) error {
	fromEnv, err := model.ParseEnvironment(from)
	if err != nil {
		return fmt.Errorf("invalid --from: %w", err)
	}
	toEnv, err := model.ParseEnvironment(to)
	if err != nil {
		return fmt.Errorf("invalid --to: %w", err)
	}

	_, err = fmt.Fprintf(out,
		"from: context=%q namespace=%q\nto:   context=%q namespace=%q\n",
		fromEnv.Context, fromEnv.Namespace, toEnv.Context, toEnv.Namespace)
	return err
}

// Execute builds and runs the root command. It is the single entry point used
// by main.
func Execute(info BuildInfo) error {
	return NewRootCmd(info).Execute()
}
