package rls

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// sourceRows are the kind of the value of each source row of taginfo.csv and
// whether it is written as UHD. A row with a $1 placeholder gives the value it
// is tested as.
var sourceRows = map[string]struct {
	value string
	kind  SourceKind
	uhd   bool
}{
	"3DTV":         {"", SourceOther, false},
	"16mm":         {"", SourceOther, false},
	"35mm":         {"", SourceOther, false},
	"$1INCH.ViNYL": {"12INCH.ViNYL", SourceOther, false},
	"AHDTV":        {"", SourceOther, false},
	"AUDiOBOOK":    {"", SourceOther, false},
	"BDRiP":        {"", SourceOther, false},
	"BDSCR":        {"", SourceOther, false},
	"BRDRip":       {"", SourceOther, false},
	"BluRay":       {"", SourceOther, false},
	"BluRay3D":     {"", SourceOther, false},
	"BluRayRiP":    {"", SourceOther, false},
	"CABLE":        {"", SourceOther, false},
	"CAM":          {"", SourceOther, false},
	"CAMRiP":       {"", SourceOther, false},
	"CD":           {"", SourceOther, false},
	"CDA":          {"", SourceOther, false},
	"CDEP":         {"", SourceOther, false},
	"CDLP":         {"", SourceOther, false},
	"CDM":          {"", SourceOther, false},
	"CDREP":        {"", SourceOther, false},
	"CDRiP":        {"", SourceOther, false},
	"CDS":          {"", SourceOther, false},
	"CDSP":         {"", SourceOther, false},
	"COMiC":        {"", SourceOther, false},
	"CVD":          {"", SourceOther, false},
	"Cassette":     {"", SourceOther, false},
	"CloneCD":      {"", SourceOther, false},
	"CloneDVD":     {"", SourceOther, false},
	"DAT":          {"", SourceOther, false},
	"DCP":          {"", SourceOther, false},
	"DCPRiP":       {"", SourceOther, false},
	"DDC":          {"", SourceOther, false},
	"DDCRiP":       {"", SourceOther, false},
	"DS":           {"", SourceOther, false},
	"DSRiP":        {"", SourceOther, false},
	"DTH":          {"", SourceOther, false},
	"DTHRiP":       {"", SourceOther, false},
	"DTSD":         {"", SourceOther, false},
	"DTV":          {"", SourceOther, false},
	"DTVRiP":       {"", SourceOther, false},
	"DTheater":     {"", SourceOther, false},
	"DVB":          {"", SourceOther, false},
	"DVBC":         {"", SourceOther, false},
	"DVBRiP":       {"", SourceOther, false},
	"DVBS":         {"", SourceOther, false},
	"DVBT":         {"", SourceOther, false},
	"DVD":          {"", SourceOther, false},
	"DVDA":         {"", SourceOther, false},
	"DVDRiP":       {"", SourceOther, false},
	"DVDS":         {"", SourceOther, false},
	"DVDSCR":       {"", SourceOther, false},
	"DVDSCRRiP":    {"", SourceOther, false},
	"DVTV":         {"", SourceOther, false},
	"Digipak":      {"", SourceOther, false},
	"EP":           {"", SourceOther, false},
	"FESTiVAL":     {"", SourceOther, false},
	"FM":           {"", SourceOther, false},
	"HDCAM":        {"", SourceOther, false},
	"HDDVD":        {"", SourceOther, false},
	"HDDVDRiP":     {"", SourceOther, false},
	"HDRiP":        {"", SourceOther, false},
	"HDTC":         {"", SourceOther, false},
	"HDTS":         {"", SourceOther, false},
	"HDTV":         {"", SourceOther, false},
	"HFR":          {"", SourceOther, false},
	"IVTC":         {"", SourceOther, false},
	"LASERDiSC":    {"", SourceOther, false},
	"LP":           {"", SourceOther, false},
	"MAGAZiNE":     {"", SourceOther, false},
	"MBluRay":      {"", SourceOther, false},
	"PDTV":         {"", SourceOther, false},
	"PDVD":         {"", SourceOther, false},
	"PS2CD":        {"", SourceOther, false},
	"PS2DVD":       {"", SourceOther, false},
	"PSXPSP":       {"", SourceOther, false},
	"RADIO":        {"", SourceOther, false},
	"SACD":         {"", SourceOther, false},
	"SAT":          {"", SourceOther, false},
	"SATRiP":       {"", SourceOther, false},
	"SBD":          {"", SourceOther, false},
	"SCAN":         {"", SourceOther, false},
	"SCD":          {"", SourceOther, false},
	"SDTV":         {"", SourceOther, false},
	"SFClone":      {"", SourceOther, false},
	"SFCloneCD":    {"", SourceOther, false},
	"SFCloneDVD":   {"", SourceOther, false},
	"SINGLE":       {"", SourceOther, false},
	"SP":           {"", SourceOther, false},
	"STREAM":       {"", SourceOther, false},
	"SVCD":         {"", SourceOther, false},
	"SVCDRiP":      {"", SourceOther, false},
	"TAPE":         {"", SourceOther, false},
	"TC":           {"", SourceOther, false},
	"TS":           {"", SourceOther, false},
	"TVHSRiP":      {"", SourceOther, false},
	"TVRiP":        {"", SourceOther, false},
	"UHD.BDRiP":    {"", SourceOther, true},
	"UHD.BluRay":   {"", SourceOther, true},
	"UHD.WEB":      {"", SourceWeb, true},
	"UHD.WEB-DL":   {"", SourceWebDL, true},
	"UHD.WEBRiP":   {"", SourceWebRip, true},
	"UHDTV":        {"", SourceOther, true},
	"UMD":          {"", SourceOther, false},
	"UMDMOVIE":     {"", SourceOther, false},
	"UMDRiP":       {"", SourceOther, false},
	"VCD":          {"", SourceOther, false},
	"VCDRiP":       {"", SourceOther, false},
	"VHS":          {"", SourceOther, false},
	"VHSRiP":       {"", SourceOther, false},
	"VLS":          {"", SourceOther, false},
	"VOD":          {"", SourceOther, false},
	"VODRiP":       {"", SourceOther, false},
	"ViNYL":        {"", SourceOther, false},
	"ViNYLRiP":     {"", SourceOther, false},
	"WEB":          {"", SourceWeb, false},
	"WEB-DL":       {"", SourceWebDL, false},
	"WEB-HD":       {"", SourceWeb, false},
	"WEBFLAC":      {"", SourceOther, false},
	"WEBRiP":       {"", SourceWebRip, false},
	"WEBSCR":       {"", SourceOther, false},
	"WORKPRiNT":    {"", SourceOther, false},
	"WebHDRiP":     {"", SourceWebRip, false},
	"WebUHD":       {"", SourceWeb, true},
	"Whitelabel":   {"", SourceOther, false},
	"XBOXDVD":      {"", SourceOther, false},
	"eBook":        {"", SourceOther, false},
}

// codecRows are the kind of each codec row of taginfo.csv.
var codecRows = map[string]CodecKind{
	"AV1":      CodecOther,
	"AVC":      CodecAVC,
	"DiVX.SBC": CodecOther,
	"H.261":    CodecOther,
	"H.262":    CodecOther,
	"H.263":    CodecOther,
	"H.264":    CodecAVC,
	"H.265":    CodecHEVC,
	"HEVC":     CodecHEVC,
	"MPEG-2":   CodecOther,
	"VC-1":     CodecOther,
	"VP7":      CodecOther,
	"VP8":      CodecOther,
	"VP9":      CodecOther,
	"XViD":     CodecOther,
	"dxva":     CodecOther,
	"x264":     CodecAVC,
	"x264.HQ":  CodecAVC,
	"x265":     CodecHEVC,
}

// resolutionRows are the height of the value of each resolution row of
// taginfo.csv, 0 for none, and whether it is interlaced. A row with a $1
// placeholder gives the value it is tested as.
var resolutionRows = map[string]struct {
	value      string
	height     int
	interlaced bool
}{
	"$1p":         {"1600p", 1600, false},
	"1080i":       {"", 1080, true},
	"1080p":       {"", 1080, false},
	"1440p":       {"", 1440, false},
	"1800p":       {"", 1800, false},
	"2160p":       {"", 2160, false},
	"2880p":       {"", 2880, false},
	"3240p":       {"", 3240, false},
	"360p":        {"", 360, false},
	"4320p":       {"", 4320, false},
	"480i":        {"", 480, true},
	"480p":        {"", 480, false},
	"540p":        {"", 540, false},
	"576i":        {"", 576, true},
	"576p":        {"", 576, false},
	"720p":        {"", 720, false},
	"900p":        {"", 900, false},
	"DCI2K":       {"", 1080, false},
	"DCI4K":       {"", 2160, false},
	"PN.Selector": {"", 0, false},
}

// rowTags returns the tags of the rows of typ in the embedded taginfo.csv.
func rowTags(typ string) []string {
	var tags []string
	for _, info := range taginfo.All()[typ] {
		tags = append(tags, info.Tag())
	}
	return tags
}

// checkRows checks that the table of a type has an entry for each row of the
// type and none for a row that is gone, so a new row has to be given one.
func checkRows[V any](t *testing.T, typ string, table map[string]V) {
	t.Helper()
	tags := rowTags(typ)
	for _, tag := range tags {
		if _, ok := table[tag]; !ok {
			t.Errorf("%s row %q is not in the table of its kinds: give it one", typ, tag)
		}
	}
	for _, tag := range slices.Sorted(maps.Keys(table)) {
		if !slices.Contains(tags, tag) {
			t.Errorf("%s table has %q, which is no row", typ, tag)
		}
	}
}

func TestSourceRows(t *testing.T) {
	checkRows(t, "source", sourceRows)
	for tag, exp := range sourceRows {
		s := Source(tag)
		if exp.value != "" {
			s = Source(exp.value)
		}
		if k := s.Kind(); k != exp.kind {
			t.Errorf("%q: expected kind %d, got: %d", s, exp.kind, k)
		}
		if uhd := s.UHD(); uhd != exp.uhd {
			t.Errorf("%q: expected uhd %t, got: %t", s, exp.uhd, uhd)
		}
	}
	if k := Source("").Kind(); k != SourceOther {
		t.Errorf("expected no source to be SourceOther, got: %d", k)
	}
	// in any case, as a caller may build the value
	for _, test := range []struct {
		s    Source
		kind SourceKind
		uhd  bool
	}{
		{"web-dl", SourceWebDL, false},
		{"uhd.web-dl", SourceWebDL, true},
		{"Uhd.Web", SourceWeb, true},
		{"web", SourceWeb, false},
		{"webrip", SourceWebRip, false},
		{"WEBHDRIP", SourceWebRip, false},
		{"webuhd", SourceWeb, true},
		{"web-hd", SourceWeb, false},
		{"uhdtv", SourceOther, true},
		{"bluray", SourceOther, false},
		{"webscr", SourceOther, false},
		{"UHD.", SourceOther, true},
		{"UH", SourceOther, false},
	} {
		if k, uhd := test.s.Kind(), test.s.UHD(); k != test.kind || uhd != test.uhd {
			t.Errorf("%q: expected kind %d and uhd %t, got: %d and %t", test.s, test.kind, test.uhd, k, uhd)
		}
	}
}

func TestCodecRows(t *testing.T) {
	checkRows(t, "codec", codecRows)
	for tag, exp := range codecRows {
		if k := Codec(tag).Kind(); k != exp {
			t.Errorf("%q: expected kind %d, got: %d", tag, exp, k)
		}
	}
	for c, exp := range map[Codec]CodecKind{"X264": CodecAVC, "h.264": CodecAVC, "avc": CodecAVC, "X265": CodecHEVC, "hevc": CodecHEVC, "h.265": CodecHEVC, "xvid": CodecOther, "h264": CodecOther} {
		if k := c.Kind(); k != exp {
			t.Errorf("%q: expected kind %d, got: %d", c, exp, k)
		}
	}
}

func TestResolutionRows(t *testing.T) {
	checkRows(t, "resolution", resolutionRows)
	for tag, exp := range resolutionRows {
		r := Resolution(tag)
		if exp.value != "" {
			r = Resolution(exp.value)
		}
		if h := r.Height(); h != exp.height {
			t.Errorf("%q: expected height %d, got: %d", r, exp.height, h)
		}
		if i := r.Interlaced(); i != exp.interlaced {
			t.Errorf("%q: expected interlaced %t, got: %t", r, exp.interlaced, i)
		}
	}
	// in any case, as a caller may build the value
	for _, test := range []struct {
		r          Resolution
		height     int
		interlaced bool
	}{
		{"2160P", 2160, false},
		{"1080I", 1080, true},
		{"dci4k", 2160, false},
		{"Dci2K", 1080, false},
		{"pn.selector", 0, false},
	} {
		if h, i := test.r.Height(), test.r.Interlaced(); h != test.height || i != test.interlaced {
			t.Errorf("%q: expected %d and %t, got: %d and %t", test.r, test.height, test.interlaced, h, i)
		}
	}
	for _, r := range []Resolution{"", "p", "i", "0p", "ip", "1080", "10x0p", "99999999999p", "DCI8K"} {
		if h, i := r.Height(), r.Interlaced(); h != 0 || i {
			t.Errorf("%q: expected no height and not interlaced, got: %d, %t", r, h, i)
		}
	}
}

// rowEntry returns the key of table that v is the value of: v itself, or a
// row with a $1 placeholder whose spelling v has, with what stands for it.
func rowEntry[V any](table map[string]V, v string) (string, string, bool) {
	if _, ok := table[v]; ok {
		return v, "", true
	}
	for key := range table {
		if before, after, ok := strings.Cut(key, "$1"); ok {
			re := regexp.MustCompile(`^` + regexp.QuoteMeta(before) + `(\d+)` + regexp.QuoteMeta(after) + `$`)
			if m := re.FindStringSubmatch(v); m != nil {
				return key, m[1], true
			}
		}
	}
	return "", "", false
}

// TestTypedProduced checks that every source, codec and resolution the parser
// gives for the names of tests.yaml, and of the corpus when it is there, has
// an entry in the tables of the rows, with the kind or height the entry
// gives. A value the parser makes of its own ('UHD.WEB-DL', 'BluRay' for a
// remux, '1080p' for '1920x1080') has to be given one too.
func TestTypedProduced(t *testing.T) {
	var names []string
	for _, test := range rlsTests(t) {
		names = append(names, test.s)
	}
	if !testing.Short() {
		for _, file := range []string{"nyaa.txt", "sports.txt"} {
			v, _ := readCorpus(file)
			names = append(names, v...)
		}
	}
	missing := make(map[string]bool)
	miss := func(typ, v, name string) {
		if k := typ + " " + v; !missing[k] {
			missing[k] = true
			t.Errorf("%s %q (in %q) has no entry in the table of its rows: give it one", typ, v, name)
		}
	}
	for _, name := range names {
		for _, r := range []Release{ParseString(name), ParseStringHint(name, Hint{Type: Movie})} {
			if r.Source != "" {
				if key, _, ok := rowEntry(sourceRows, string(r.Source)); !ok {
					miss("source", string(r.Source), name)
				} else if exp := sourceRows[key]; r.Source.Kind() != exp.kind || r.Source.UHD() != exp.uhd {
					t.Errorf("%q: source %q is kind %d and uhd %t, its entry %q %d and %t", name, r.Source, r.Source.Kind(), r.Source.UHD(), key, exp.kind, exp.uhd)
				}
			}
			for _, c := range r.Codec {
				if exp, ok := codecRows[string(c)]; !ok {
					miss("codec", string(c), name)
				} else if c.Kind() != exp {
					t.Errorf("%q: codec %q is kind %d, its entry %d", name, c, c.Kind(), exp)
				}
			}
			if r.Resolution != "" {
				key, n, ok := rowEntry(resolutionRows, string(r.Resolution))
				if !ok {
					miss("resolution", string(r.Resolution), name)
					continue
				}
				exp := resolutionRows[key]
				height := exp.height
				if n != "" {
					height, _ = strconv.Atoi(n)
				}
				if r.Resolution.Height() != height || r.Resolution.Interlaced() != exp.interlaced {
					t.Errorf("%q: resolution %q is %d and interlaced %t, its entry %q %d and %t", name, r.Resolution, r.Resolution.Height(), r.Resolution.Interlaced(), key, height, exp.interlaced)
				}
			}
		}
	}
	t.Logf("%d names", len(names))
}

// TestTypedParse checks the readings of the fields of parsed names, and that
// the values are spelled as their rows are.
func TestTypedParse(t *testing.T) {
	tests := []struct {
		s          string
		source     Source
		kind       SourceKind
		uhd        bool
		codec      []Codec
		codecKind  CodecKind
		resolution Resolution
		height     int
	}{
		{"Show.S01E02.1080p.WEB.H264-GRP", "WEB", SourceWeb, false, []Codec{"H.264"}, CodecAVC, "1080p", 1080},
		{"Movie.2020.2160p.UHD.WEB-DL.DV.HDR10.HEVC-GRP", "UHD.WEB-DL", SourceWebDL, true, []Codec{"HEVC"}, CodecHEVC, "2160p", 2160},
		{"Movie.2022.UHD.2160p.WEB.H265-GRP", "UHD.WEB", SourceWeb, true, []Codec{"H.265"}, CodecHEVC, "2160p", 2160},
		{"Show.S01E02.720p.WEBRip.x264-GRP", "WEBRiP", SourceWebRip, false, []Codec{"x264"}, CodecAVC, "720p", 720},
		{"Movie.2020.2160p.UHD.BluRay.x265-GRP", "UHD.BluRay", SourceOther, true, []Codec{"x265"}, CodecHEVC, "2160p", 2160},
		{"Show.S01.UHD.HDTV.x265-GRP", "HDTV", SourceOther, false, []Codec{"x265"}, CodecHEVC, "2160p", 2160},
		{"Event.2024.1080i.HDTV.MPEG2-GRP", "HDTV", SourceOther, false, []Codec{"MPEG-2"}, CodecOther, "1080i", 1080},
		{"Movie.1999.DVDRip.XviD-GRP", "DVDRiP", SourceOther, false, []Codec{"XViD"}, CodecOther, "", 0},
	}
	for _, test := range tests {
		r := ParseString(test.s)
		if r.Source != test.source || r.Source.Kind() != test.kind || r.Source.UHD() != test.uhd {
			t.Errorf("%q: expected source %q (%d, uhd %t), got: %q (%d, uhd %t)", test.s, test.source, test.kind, test.uhd, r.Source, r.Source.Kind(), r.Source.UHD())
		}
		if !slices.Equal(r.Codec, test.codec) || r.Codec[0].Kind() != test.codecKind {
			t.Errorf("%q: expected codec %q (%d), got: %q (%d)", test.s, test.codec, test.codecKind, r.Codec, r.Codec[0].Kind())
		}
		if r.Resolution != test.resolution || r.Resolution.Height() != test.height {
			t.Errorf("%q: expected resolution %q (%d), got: %q (%d)", test.s, test.resolution, test.height, r.Resolution, r.Resolution.Height())
		}
	}
}

// TestTypedJSON checks that the fields marshal as the strings they were.
func TestTypedJSON(t *testing.T) {
	r := ParseString("Movie.2020.1080p.BluRay.x264-GRP")
	buf, err := json.Marshal(struct {
		Source     Source
		Codec      []Codec
		Resolution Resolution
	}{r.Source, r.Codec, r.Resolution})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if exp := `{"Source":"BluRay","Codec":["x264"],"Resolution":"1080p"}`; string(buf) != exp {
		t.Errorf("expected %s, got: %s", exp, buf)
	}
}
