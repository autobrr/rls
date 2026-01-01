package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/moistari/rls"
)

var pretty = flag.Bool("pretty", false, "pretty-print JSON output")

func processLine(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	// Just parse the release directly
	r := rls.ParseString(line)

	var b []byte
	var err error

	if *pretty {
		b, err = json.MarshalIndent(r, "", "  ")
	} else {
		b, err = json.Marshal(r)
	}

	if err != nil {
		return err
	}
	fmt.Print(string(b))
	return nil
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		// read lines from stdin
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if err := processLine(scanner.Text()); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "error reading stdin:", err)
			os.Exit(1)
		}
		return
	}
	// treat each arg as a separate release name
	for _, a := range args {
		if err := processLine(a); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}
}
