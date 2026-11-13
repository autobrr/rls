package main

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Set by goreleaser through its default ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

// newVersionCmd returns the version command.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v, c, d := buildInfo()
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "rls %s\ncommit: %s\nbuilt at: %s\nbuilt by: %s\n", v, c, d, builtBy)
			return err
		},
	}
}

// buildInfo returns the version, commit and build date. Values set through
// ldflags win; a binary from go install falls back to the module version and
// the VCS stamp from the Go toolchain.
func buildInfo() (v, c, d string) {
	v, c, d = version, commit, date
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, c, d
	}
	if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && c == "none":
			c = s.Value
		case s.Key == "vcs.time" && d == "unknown":
			d = s.Value
		}
	}
	return v, c, d
}
