// Command rls parses release names.
//
// The parse command writes each parsed release to standard output as a JSON
// object on its own line (JSON Lines). Release names are taken from the
// command line arguments or, when there are none, read from standard input,
// one per line. Blank lines are skipped.
//
// Usage:
//
//	rls parse [name ...]
//	rls version
//
// Examples:
//
//	rls parse The.Matrix.1999.1080p.BluRay.x264-GROUP
//	rls parse < names.txt
//	Get-Content names.txt | rls.exe parse
package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd returns the rls command with its subcommands.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "rls",
		Short:        "Parse release names",
		SilenceUsage: true,
	}
	cmd.AddCommand(newParseCmd(), newVersionCmd())
	return cmd
}
