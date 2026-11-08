// Package cli wires the cliproxy command tree.
package cli

import (
	"context"
	"os"
	"runtime/debug"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"
)

// Set through -ldflags at release time; see Makefile.
var (
	version = ""
	commit  = ""
)

// Execute runs the root command with fang's styled help, errors, version,
// completion and man page support.
func Execute(ctx context.Context) error {
	return fang.Execute(ctx, newRootCmd(),
		fang.WithVersion(buildVersion()),
		fang.WithCommit(commit),
		fang.WithNotifySignal(os.Interrupt),
	)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cliproxy",
		Short: "A terminal front end for cliproxy",
		Long:  "cliproxy is a terminal front end for cliproxy.\n\nThe real commands are still to come; try the demo to see the UI stack.",
		Example: `  # Play the animated demo
  cliproxy demo

  # Play it at double speed
  cliproxy demo --speed 2`,
		Args: cobra.NoArgs,
	}
	root.AddCommand(newDemoCmd())
	return root
}

// buildVersion prefers the ldflags value and falls back to the module
// version that `go install pkg@vX` records.
func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
