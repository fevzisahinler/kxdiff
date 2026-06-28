// Command kxdiff is a read-only diff tool for two Kubernetes environments.
//
// It runs as a kubectl plugin (kubectl kxdiff) and compares two environments,
// where an environment is a (context, namespace) pair, reporting what differs.
package main

import (
	"os"

	"github.com/fevzisahinler/kxdiff/internal/cli"
)

// Build metadata, injected at link time via -ldflags (see .goreleaser.yaml).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// exitError is the process exit code for an unexpected failure.
//
// The full contract (FEATURES.md §5.3) is: 0 = no diff, 1 = diff found,
// 2 = error. Only the error code is wired up at this skeleton stage.
const exitError = 2

func main() {
	info := cli.BuildInfo{Version: version, Commit: commit, Date: date}
	if err := cli.Execute(info); err != nil {
		// cobra already printed the error to stderr (SilenceErrors=false).
		os.Exit(exitError)
	}
}
