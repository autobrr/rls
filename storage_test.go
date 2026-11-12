package rls

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

// storageNames are names that between them reach every lexer. They are
// literals, so the linker puts them in read-only memory.
var storageNames = []string{
	"[SubsPlease] Show Title - 21.5v2 (1080p) [ABCD1234].mkv",
	"Show.Title.S01E02-E04.The.Episode.2160p.UHD.BluRay.REMUX.HDR10+.DV.TrueHD.7.1.Atmos.HEVC-GRP",
	"Movie.Title.2019.Directors.Cut.MULTi5.1080p60.WEB-DL.DDP5.1.H.264.REPACK3-GRP.mkv",
	"Artist_Name-Album_Title-2CD-FLAC-2020-GRP",
	"-={site}=- Some.Game.v1.2.3.Plus.10.Trainer.x64.MacOS-GRP {{pass}} [[k:v]] (REQ)",
	"Event.2024-25.Round.05.Team.A.vs.Team.B.12.10.2024.720p50.HDTV.x264-GRP",
	"Comic Title 001 (2024) (digital) (Scan-Team).cbr",
	"The.Show.1x05.BDRemux.DTS-HD.MA.5.1.25th.Anniversary.Edition.12.inch.vinyl.4.7GB.D01",
	"\t [Group] Title_With_Underscores,and+plus - 01 ~ 12 [BD 1920x1080 x265 10bit FLAC] \n",
}

// TestParseOwnsName checks that a parse keeps nothing of what it was given: the
// bytes can be written over and the string is not the one the tags hold.
func TestParseOwnsName(t *testing.T) {
	for _, name := range storageNames {
		b := []byte(name)
		tags, _ := DefaultParser.Parse(b)
		r := Parse(b)
		h := ParseHint(b, Hint{Type: Movie, DayFirst: true})
		exp := fmt.Sprintf("%q %q %q %q %q", tags, r.Tags(), h.Tags(), r.Title, h.Subtitle)
		for i := range b {
			b[i] = 'x'
		}
		if s := fmt.Sprintf("%q %q %q %q %q", tags, r.Tags(), h.Tags(), r.Title, h.Subtitle); s != exp {
			t.Errorf("%q: expected the tags to keep their values when the bytes change\n%s\ngot:\n%s", name, exp, s)
		}
		// a string made at run time, which a release must not point into
		s := strings.Clone(name)
		lo := uintptr(unsafe.Pointer(unsafe.StringData(s)))
		tags, _ = ParseTagsString(s)
		for _, tags := range [][]Tag{tags, ParseString(s).Tags(), ParseStringHint(s, Hint{}).Tags()} {
			for _, tag := range tags {
				for _, v := range tag.v {
					if p := uintptr(unsafe.Pointer(unsafe.StringData(v))); v != "" && lo <= p && p < lo+uintptr(len(s)) {
						t.Fatalf("%q: expected no value in the string given, got: %q", name, v)
					}
				}
			}
		}
	}
}

// TestParseSharesName checks what the storage is for: the text of every tag
// of a parse is a piece of one copy of the name.
func TestParseSharesName(t *testing.T) {
	p := NewParser()
	for _, name := range storageNames {
		own := []byte(name)
		tags, _ := p.parse(own)
		lo := uintptr(unsafe.Pointer(unsafe.SliceData(own)))
		for _, tag := range tags {
			if v := tag.v[0]; v != "" {
				if p := uintptr(unsafe.Pointer(unsafe.StringData(v))); p < lo || lo+uintptr(len(own)) < p+uintptr(len(v)) {
					t.Errorf("%q: expected the text %q to be a piece of the name", name, v)
				}
			}
		}
	}
}

// TestParseReadOnly parses names held in read-only memory, which a lexer that
// wrote to its input would fault on.
func TestParseReadOnly(t *testing.T) {
	p := NewParser()
	for _, name := range storageNames {
		tags, end := p.parse(unsafe.Slice(unsafe.StringData(name), len(name)))
		r := p.builder.Build(tags, end)
		if s := fmt.Sprintf("%o", r); s != name {
			t.Errorf("expected %q, got: %q", name, s)
		}
		exercise(r)
	}
}

// TestParseAllocs keeps the allocations of a parse from creeping back up.
func TestParseAllocs(t *testing.T) {
	if poolDrops() {
		t.Skip("the regexp keeps its machines in a pool, which drops them under the race detector")
	}
	const name = "Show.Title.S01E02.The.Episode.1080p.WEB-DL.DDP5.1.H.264-GRP"
	b := []byte(name)
	// 37 when this was written: the name, the working copy, the tags, and
	// for each tag its values
	if n := testing.AllocsPerRun(100, func() { DefaultParser.Parse(b) }); n > 40 {
		t.Errorf("expected at most 40 allocations for the tags, got: %v", n)
	}
}
