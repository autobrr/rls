// Command rls parses release names and writes each parsed release to standard
// output as a JSON object on its own line (JSON Lines).
//
// Release names are taken from the command line arguments or, when there are
// none, read from standard input, one per line. Blank lines are skipped.
//
// Usage:
//
//	rls [name ...]
//
// Examples:
//
//	rls The.Matrix.1999.1080p.BluRay.x264-GROUP
//	rls < names.txt
//	Get-Content names.txt | rls.exe
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/autobrr/rls"
)

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "usage: rls [name ...]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Parses release names and writes each one to standard output as a JSON")
		fmt.Fprintln(out, "object on its own line. Names are read from the arguments or, when there")
		fmt.Fprintln(out, "are none, from standard input, one per line. Use -- before a name that")
		fmt.Fprintln(out, "starts with a dash.")
	}
	flag.Parse()
	if err := run(os.Stdout, os.Stdin, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "rls:", err)
		os.Exit(1)
	}
}

// release is the JSON form of a parsed release.
type release struct {
	// Input is the release name that was parsed.
	Input string
	// Type is the name of the release type, in place of its numeric value.
	Type string
	rls.Release
}

// run parses each name in args, or each line read from r when args is empty,
// and writes the releases to w as JSON Lines.
func run(w io.Writer, r io.Reader, args []string) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	write := func(name string) error {
		// strip a UTF-8 byte order mark, as written by some Windows tools
		name = strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))
		if name == "" {
			return nil
		}
		rel := rls.ParseString(name)
		return enc.Encode(release{Input: name, Type: rel.Type.String(), Release: rel})
	}
	if len(args) != 0 {
		for _, arg := range args {
			if err := write(arg); err != nil {
				return err
			}
		}
		return nil
	}
	s := bufio.NewScanner(r)
	for s.Scan() {
		if err := write(s.Text()); err != nil {
			return err
		}
	}
	return s.Err()
}
