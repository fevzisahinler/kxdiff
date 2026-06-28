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

func main() {
	info := cli.BuildInfo{Version: version, Commit: commit, Date: date}
	// Exit code contract (FEATURES.md §5.3): 0 = no diff, 1 = diff, 2 = error.
	os.Exit(cli.Execute(info))
}
