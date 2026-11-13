package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/autobrr/rls"
	"github.com/spf13/cobra"
)

// newParseCmd returns the parse command.
func newParseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "parse [name ...]",
		Short: "Parse release names and write them as JSON Lines",
		Long: `Parses release names and writes each one to standard output as a JSON
object on its own line. Names are read from the arguments or, when there
are none, from standard input, one per line. Use -- before a name that
starts with a dash.`,
		Example: `  rls parse The.Matrix.1999.1080p.BluRay.x264-GROUP
  rls parse < names.txt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.OutOrStdout(), cmd.InOrStdin(), args)
		},
	}
}

// release is the JSON form of a parsed release.
type release struct {
	Input  string        `json:"input"`
	Output releaseOutput `json:"output"`
}

type releaseOutput struct {
	Type string `json:"type"`

	Artist   string   `json:"artist"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Alt      []string `json:"alt"`

	Platform string `json:"platform"`
	Arch     string `json:"arch"`

	Source     string `json:"source"`
	Resolution string `json:"resolution"`
	FrameRate  string `json:"framerate"`
	Collection string `json:"collection"`

	Year    int `json:"year"`
	Month   int `json:"month"`
	Day     int `json:"day"`
	YearEnd int `json:"year_end"`

	HasSeason  bool    `json:"has_season"`
	Series     int     `json:"series"`
	Seasons    []int   `json:"seasons"`
	HasEpisode bool    `json:"has_episode"`
	Episode    int     `json:"episode"`
	Episodes   [][]int `json:"episodes"`
	Version    string  `json:"version"`
	Disc       string  `json:"disc"`

	Codec    []string `json:"codec"`
	HDR      []string `json:"dynamic_range"`
	BitDepth string   `json:"bit_depth"`
	Audio    []string `json:"audio"`
	Channels string   `json:"channels"`
	Other    []string `json:"other"`
	Cut      []string `json:"cut"`
	Edition  []string `json:"edition"`
	Language []string `json:"language"`

	Size      string   `json:"size"`
	Region    string   `json:"region"`
	Container string   `json:"container"`
	Genre     string   `json:"genre"`
	ID        string   `json:"id"`
	Group     string   `json:"group"`
	Meta      []string `json:"meta"`
	Site      string   `json:"site"`
	Sum       string   `json:"sum"`
	Pass      string   `json:"pass"`
	Req       bool     `json:"req"`
	Ext       string   `json:"ext"`
	IsProper  bool     `json:"is_proper"`
	IsRepack  bool     `json:"is_repack"`
}

func rlsToOutput(rls rls.Release) releaseOutput {
	return releaseOutput{
		Artist:     rls.Artist,
		Type:       rls.Type.String(),
		Title:      rls.Title,
		Subtitle:   rls.Subtitle,
		Alt:        orEmpty(rls.Alt),
		Platform:   rls.Platform,
		Arch:       rls.Arch,
		Source:     rls.Source,
		Resolution: rls.Resolution,
		FrameRate:  rls.FrameRate,
		Collection: rls.Collection,
		Year:       rls.Year,
		Month:      rls.Month,
		Day:        rls.Day,
		YearEnd:    rls.YearEnd,
		HasSeason:  rls.HasSeason(),
		Series:     rls.Series,
		Seasons:    orEmpty(rls.Seasons()),
		HasEpisode: rls.HasEpisode(),
		Episode:    rls.Episode,
		Episodes:   orEmpty(rls.SeriesEpisodes()),
		Version:    rls.Version,
		Disc:       rls.Disc,
		Codec:      orEmpty(rls.Codec),
		HDR:        orEmpty(rls.HDR),
		BitDepth:   rls.BitDepth,
		Audio:      orEmpty(rls.Audio),
		Channels:   rls.Channels,
		Other:      orEmpty(rls.Other),
		Cut:        orEmpty(rls.Cut),
		Edition:    orEmpty(rls.Edition),
		Language:   orEmpty(rls.Language),
		Size:       rls.Size,
		Region:     rls.Region,
		Container:  rls.Container,
		Genre:      rls.Genre,
		ID:         rls.ID,
		Group:      rls.Group,
		Meta:       orEmpty(rls.Meta),
		Site:       rls.Site,
		Sum:        rls.Sum,
		Pass:       rls.Pass,
		Req:        rls.Req,
		Ext:        rls.Ext,
		IsProper:   rls.IsProper(),
		IsRepack:   rls.IsRepack(),
	}
}

// orEmpty returns s, or an empty slice when s is nil, so the field is written
// as [] and not null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
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
		return enc.Encode(release{Input: name, Output: rlsToOutput(rel)})
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
