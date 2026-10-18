# rls

This is a fork of [moistari/rls](https://github.com/moistari/rls)

rls is a package to parse release names. see [go doc](https://pkg.go.dev/github.com/autobrr/rls).

## Usage example

```go
package main

import (
	"fmt"

	"github.com/autobrr/rls"
)

func main() {
	const title = "The_Velvet_Underground-The_Complete_Matrix_Tapes-Reissue_Limited_Edition_Boxset-8LP-2019-NOiR"
	r := rls.ParseString(title)
	fmt.Printf("%q:\n", r)
	fmt.Printf("  type: %s\n", r.Type)
	fmt.Printf("  artist: %s\n", r.Artist)
	fmt.Printf("  title: %s\n", r.Title)
	fmt.Printf("  year: %d\n", r.Year)
	fmt.Printf("  disc: %s\n", r.Disc)
	fmt.Printf("  source: %s\n", r.Source)
	fmt.Printf("  edition: %q\n", r.Edition)
	fmt.Printf("  other: %q\n", r.Other)
	fmt.Printf("  group: %s\n", r.Group)
	// Output:
	// "The_Velvet_Underground-The_Complete_Matrix_Tapes-Reissue_Limited_Edition_Boxset-8LP-2019-NOiR":
	//   type: music
	//   artist: The Velvet Underground
	//   title: The Complete Matrix Tapes
	//   year: 2019
	//   disc: 8x
	//   source: LP
	//   edition: ["Limited.Edition"]
	//   other: ["REISSUE" "BOXSET"]
	//   group: NOiR
}
```

## Command line

The `rls` command parses release names and writes each result to standard
output as a JSON object on its own line ([JSON Lines](https://jsonlines.org)).
Names are read from the arguments or, when there are none, from standard input,
one per line.

Prebuilt binaries for Linux, macOS, Windows and FreeBSD are attached to each
[release](https://github.com/autobrr/rls/releases). To build from source:

```sh
go install github.com/autobrr/rls/cmd/rls@latest
```

```sh
$ rls The.Matrix.1999.1080p.BluRay.x264-GROUP
$ rls < names.txt
```

```powershell
Get-Content names.txt | rls.exe | ConvertFrom-Json
```

Windows PowerShell 5.1 sends ASCII to native commands by default, which turns
non-ASCII characters into `?`. Run
`$OutputEncoding = [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)`
first, or use PowerShell 7.
