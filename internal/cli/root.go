// Package cli wires up the kxdiff command-line interface (cobra).
//
// At this skeleton stage it defines the root command, its flags and version
// reporting only; the diff engine is not implemented yet.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// errNotImplemented is returned by the root command until the diff engine
// is wired up. It keeps the skeleton honest: running the tool fails loudly
// instead of silently doing nothing.
var errNotImplemented = errors.New("not implemented yet")

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
		RunE: func(_ *cobra.Command, _ []string) error {
			return errNotImplemented
		},
	}

	cmd.SetVersionTemplate(
		fmt.Sprintf("kxdiff version %s (commit %s, built %s)\n",
			info.Version, info.Commit, info.Date),
	)

	cmd.Flags().StringVar(&from, "from", "", "source environment: [context][/namespace]")
	cmd.Flags().StringVar(&to, "to", "", "target environment: [context][/namespace]")

	return cmd
}

// Execute builds and runs the root command. It is the single entry point used
// by main.
func Execute(info BuildInfo) error {
	return NewRootCmd(info).Execute()
}
