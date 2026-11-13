package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// result holds the fields of a parse line that the tests check.
type result struct {
	Input  string `json:"input"`
	Output output `json:"output"`
}

type output struct {
	Type       string `json:"type"`
	Artist     string `json:"artist"`
	Title      string `json:"title"`
	Resolution string `json:"resolution"`
	Year       int    `json:"year"`
	Series     int    `json:"series"`
	Episode    int    `json:"episode"`
	Group      string `json:"group"`
}

var (
	music = result{
		Input: "The_Velvet_Underground-The_Complete_Matrix_Tapes-Reissue_Limited_Edition_Boxset-8LP-2019-NOiR",
		Output: output{
			Type:   "music",
			Artist: "The Velvet Underground",
			Title:  "The Complete Matrix Tapes",
			Year:   2019,
			Group:  "NOiR",
		},
	}
	movie = result{
		Input: "Mr..&.Mrs..Smith-2005(720p)-NOGROUP[ettv]",
		Output: output{
			Type:       "movie",
			Title:      "Mr. & Mrs. Smith",
			Resolution: "720p",
			Year:       2005,
			Group:      "NOGROUP",
		},
	}
	episode = result{
		Input: "Doctor.Who.S02E05.The.Web.Planet.1080p.WEB-DL.AAC2.0.H.264-GROUP",
		Output: output{
			Type:       "episode",
			Title:      "Doctor Who",
			Resolution: "1080p",
			Series:     2,
			Episode:    5,
			Group:      "GROUP",
		},
	}
)

func TestRun(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		exp   []result
	}{
		{
			name: "args",
			args: []string{music.Input, movie.Input},
			exp:  []result{music, movie},
		},
		{
			name:  "args ignore stdin",
			stdin: episode.Input + "\n",
			args:  []string{music.Input},
			exp:   []result{music},
		},
		{
			name:  "stdin",
			stdin: music.Input + "\n" + movie.Input + "\n" + episode.Input + "\n",
			exp:   []result{music, movie, episode},
		},
		{
			name:  "stdin without trailing newline",
			stdin: music.Input + "\n" + movie.Input,
			exp:   []result{music, movie},
		},
		{
			name:  "stdin crlf bom and blank lines",
			stdin: "\ufeff" + music.Input + "\r\n\r\n  " + movie.Input + "\t\r\n \r\n" + episode.Input + "\r\n",
			exp:   []result{music, movie, episode},
		},
		{
			name:  "empty stdin",
			stdin: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := run(&buf, strings.NewReader(tt.stdin), tt.args); err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			out := buf.String()
			if len(tt.exp) == 0 {
				if out != "" {
					t.Fatalf("expected no output, got: %q", out)
				}
				return
			}
			if !strings.HasSuffix(out, "\n") {
				t.Errorf("expected output to end with a newline, got: %q", out)
			}
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) != len(tt.exp) {
				t.Fatalf("expected %d lines, got %d:\n%s", len(tt.exp), len(lines), out)
			}
			var v []result
			for i, line := range lines {
				if !json.Valid([]byte(line)) {
					t.Fatalf("line %d is not valid json: %s", i, line)
				}
				var r result
				if err := json.Unmarshal([]byte(line), &r); err != nil {
					t.Fatalf("line %d: %v", i, err)
				}
				v = append(v, r)
			}
			if !cmp.Equal(v, tt.exp) {
				t.Errorf("expected to be same, got:\n%s", cmp.Diff(tt.exp, v))
			}
		})
	}
}

func TestRunEscape(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, nil, []string{movie.Input}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if s := `"title":"Mr. & Mrs. Smith"`; !strings.Contains(buf.String(), s) {
		t.Errorf("expected output to contain %s, got: %s", s, buf.String())
	}
}

func execute(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got: %v\n%s", err, out.String())
	}
	return out.String()
}

func TestParseCmd(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		exp   string
	}{
		{name: "args", args: []string{"parse", episode.Input}, exp: `"input":"` + episode.Input + `"`},
		{name: "stdin", stdin: episode.Input + "\n", args: []string{"parse"}, exp: `"input":"` + episode.Input + `"`},
		{name: "dash", args: []string{"parse", "--", "-GROUP"}, exp: `"input":"-GROUP"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out := execute(t, tt.stdin, tt.args...); !strings.Contains(out, tt.exp) {
				t.Errorf("expected output to contain %s, got: %s", tt.exp, out)
			}
		})
	}
}

func TestVersionCmd(t *testing.T) {
	out := execute(t, "", "version")
	for _, s := range []string{"rls ", "commit: ", "built at: ", "built by: "} {
		if !strings.Contains(out, s) {
			t.Errorf("expected output to contain %q, got: %s", s, out)
		}
	}
}

func TestRunEmptyLists(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, nil, []string{"The.Matrix.1999-GROUP"}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if strings.Contains(buf.String(), "null") {
		t.Errorf("expected empty lists as [], got: %s", buf.String())
	}
}
