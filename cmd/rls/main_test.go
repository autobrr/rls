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

// TestRunNumbering checks the keys that give the seasons and the episodes of
// a release: those a line has always had, and the numbering object that
// spells the fields out.
func TestRunNumbering(t *testing.T) {
	tests := []struct {
		name string
		exp  []string
	}{
		{
			"Show.S01E02-E04.1080p.WEB.h264-GRP",
			[]string{
				`"has_season":true,"series":1,"seasons":[1],"has_episode":true,"episode":2,"episodes":[[1,2],[1,3],[1,4]]`,
				`"numbering":{"scheme":"seasoned","shape":"episodes","seasons":[1],"episodes":[{"season":1,"first":2,"last":4,"frac":0,"kind":""}],"alt":[],"volume":0,"contents":[],"batch":false}`,
			},
		},
		// an episode with no season is in season 0 under episodes, and in
		// season -1 in the numbering
		{
			"[Grp] Show - 01-03 [1080p]",
			[]string{
				`"has_season":false,"series":0,"seasons":[],"has_episode":true,"episode":1,"episodes":[[0,1],[0,2],[0,3]]`,
				`"numbering":{"scheme":"absolute","shape":"episodes","seasons":[],"episodes":[{"season":-1,"first":1,"last":3,"frac":0,"kind":""}],"alt":[]`,
			},
		},
		// seasons are in rising order under seasons, and in the order of the
		// name in the numbering
		{
			"[Grp] Show [Season 2 + Season 1 OVA] [BD 1080p]",
			[]string{
				`"has_season":true,"series":2,"seasons":[1,2],"has_episode":false,"episode":0,"episodes":[]`,
				`"numbering":{"scheme":"seasoned","shape":"seasons","seasons":[2,1],"episodes":[],"alt":[]`,
			},
		},
		{
			"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP",
			[]string{
				`"has_season":true,"series":23,"seasons":[23],"has_episode":true,"episode":19,"episodes":[[23,19]]`,
				`"episodes":[{"season":23,"first":19,"last":19,"frac":0,"kind":""}],"alt":[{"season":-1,"first":1174,"last":1174,"frac":0,"kind":""}]`,
			},
		},
		{
			"[Grp] Show - 21.5 [1080p]",
			[]string{`"episodes":[{"season":-1,"first":21,"last":21,"frac":5,"kind":""}]`},
		},
		{
			"Show.2024.03.11.1080p.WEB.h264-GRP",
			[]string{
				`"has_season":false,"series":0,"seasons":[],"has_episode":false,"episode":0,"episodes":[]`,
				`"numbering":{"scheme":"daily","shape":"daily","seasons":[],"episodes":[],"alt":[],"volume":0,"contents":[],"batch":false}`,
			},
		},
		{
			"The.Matrix.1999.1080p.BluRay.x264-GROUP",
			[]string{`"numbering":{"scheme":"","shape":"","seasons":[],"episodes":[],"alt":[],"volume":0,"contents":[],"batch":false}`},
		},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if err := run(&buf, nil, []string{tt.name}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for _, s := range tt.exp {
			if !strings.Contains(buf.String(), s) {
				t.Errorf("%q: expected output to contain %s, got: %s", tt.name, s, buf.String())
			}
		}
	}
}

// TestRunDynamicRange checks that the formats are written in one order,
// whatever order the name gives them in.
func TestRunDynamicRange(t *testing.T) {
	for _, name := range []string{
		"Movie.2020.2160p.WEB-DL.HDR10.DV.HEVC-GRP",
		"Movie.2020.2160p.WEB-DL.DV.HDR10.HEVC-GRP",
	} {
		var buf bytes.Buffer
		if err := run(&buf, nil, []string{name}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if s := `"dynamic_range":["DV","HDR10"]`; !strings.Contains(buf.String(), s) {
			t.Errorf("%q: expected output to contain %s, got: %s", name, s, buf.String())
		}
	}
}
