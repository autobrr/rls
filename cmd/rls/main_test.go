package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type result struct {
	Input      string
	Type       string
	Artist     string
	Title      string
	Resolution string
	Year       int
	Series     int
	Episode    int
	Group      string
}

var (
	music = result{
		Input:  "The_Velvet_Underground-The_Complete_Matrix_Tapes-Reissue_Limited_Edition_Boxset-8LP-2019-NOiR",
		Type:   "music",
		Artist: "The Velvet Underground",
		Title:  "The Complete Matrix Tapes",
		Year:   2019,
		Group:  "NOiR",
	}
	movie = result{
		Input:      "Mr..&.Mrs..Smith-2005(720p)-NOGROUP[ettv]",
		Type:       "movie",
		Title:      "Mr. & Mrs. Smith",
		Resolution: "720p",
		Year:       2005,
		Group:      "NOGROUP",
	}
	episode = result{
		Input:      "Doctor.Who.S02E05.The.Web.Planet.1080p.WEB-DL.AAC2.0.H.264-GROUP",
		Type:       "episode",
		Title:      "Doctor Who",
		Resolution: "1080p",
		Series:     2,
		Episode:    5,
		Group:      "GROUP",
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
	if s := `"Title":"Mr. & Mrs. Smith"`; !strings.Contains(buf.String(), s) {
		t.Errorf("expected output to contain %s, got: %s", s, buf.String())
	}
}
