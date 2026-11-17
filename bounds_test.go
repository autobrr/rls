package rls

import (
	"fmt"
	"strings"
	"testing"
)

// tagStream writes tags as '<Type:text>' with the text as it is in the name,
// and a '|' in front of the first tag that begins in the part of the name
// read from its end.
func tagStream(tags []Tag, end int) string {
	var sb strings.Builder
	for i, tag := range tags {
		if i == end {
			sb.WriteString("|")
		}
		fmt.Fprintf(&sb, "<%s:%o>", tag.TagType(), tag)
	}
	return sb.String()
}

// TestJoin checks the tags of a name for each kind of tag whose lexer takes a
// delimiter with it: the delimiter is a tag of its own, one for the whole run
// of delimiters at that place, and what is the tag's own stays in it.
func TestJoin(t *testing.T) {
	for _, test := range []struct {
		s, exp string
	}{
		// the delimiter after a season
		{
			"Show Name S01 1080p WEB-DL DDP5.1 H.264-GRP",
			"<Text:Show><Delim: ><Text:Name><Delim: ><Series:S01><Delim: ><Resolution:1080p><Delim: ><Source:WEB-DL><Delim: ><Audio:DDP><Channels:5.1><Delim: ><Codec:H.264>|<Delim:-><Group:GRP>",
		},
		// between an audio tag and its channels
		{
			"Movie.2019.1080p.BluRay.DTS-HD.MA.5.1.x264-GRP",
			"<Text:Movie><Delim:.><Date:2019><Delim:.><Resolution:1080p><Delim:.><Source:BluRay><Delim:.><Audio:DTS-HD.MA><Delim:.><Channels:5.1><Delim:.><Codec:x264>|<Delim:-><Group:GRP>",
		},
		// the spaces around the hyphen of a group, which join the hyphen
		{
			"22 Jump Street (2014) 720p BrRip x264 - YIFY",
			"<Text:22><Delim: ><Text:Jump><Delim: ><Text:Street><Delim: (><Date:2014><Delim:) ><Resolution:720p><Delim: ><Source:BrRip><Delim: ><Codec:x264><Delim: - >|<Group:YIFY>",
		},
		// the hyphen of a group the parser knows
		{
			"Some.Game.v1.2-CODEX",
			"<Text:Some><Delim:.><Text:Game><Delim:.><Version:v1.2>|<Delim:-><Group:CODEX>",
		},
		// the space after a bracket and in front of one, and the dot of a
		// file extension. The brackets are the meta tag's own
		{
			"[Commie] Hataraku Maou-sama! - 01 [1080p] [ ABCD1234 ].mkv",
			"<Meta:[Commie]><Delim: ><Text:Hataraku><Delim: ><Text:Maou><Delim:-><Text:sama!><Delim: - ><Series:01><Delim: [><Resolution:1080p>|<Delim:] ><Meta:[ ABCD1234 ]><Delim:.><Ext:mkv>",
		},
		{
			"x {{p}} [[k:v]] (REQ)",
			"<Text:x>|<Delim: ><Meta:{{p}}><Delim: ><Meta:[[k:v]]><Delim: ><Meta:(REQ)>",
		},
		// the parenthesis that closes a genre
		{
			"(Comedy) Dave Chappelle - The Age of Spin (2017) 1080p WEBRip",
			"<Delim:(><Genre:Comedy><Delim:) ><Text:Dave><Delim: ><Text:Chappelle><Delim: - ><Text:The><Delim: ><Text:Age><Delim: ><Text:of><Delim: ><Text:Spin><Delim: (><Date:2017><Delim:) ><Resolution:1080p><Delim: ><Source:WEBRip>",
		},
		// the parenthesis that closes a music id, and the delimiters in
		// front of what follows a group
		{
			"Airdream_and_Calvin_OCommor_-_Rescue_Me_(Extended_Mix)-(NRG112)-SINGLE-WEB-2022-JUSTiFY_iNT",
			"<Text:Airdream><Delim:_><Text:and><Delim:_><Text:Calvin><Delim:_><Text:OCommor><Delim:_-_><Text:Rescue><Delim:_><Text:Me><Delim:_(><Cut:Extended><Delim:_><Other:Mix><Delim:)-(><ID:NRG112><Delim:)-><Source:SINGLE><Delim:-><Source:WEB><Delim:-><Date:2022>|<Delim:-><Group:JUSTiFY><Delim:_><Other:iNT>",
		},
		{
			"Show.Title.S01E01.720p.HDTV.x264-GRP-Obfuscated",
			"<Text:Show><Delim:.><Text:Title><Delim:.><Series:S01E01><Delim:.><Resolution:720p><Delim:.><Source:HDTV><Delim:.><Codec:x264>|<Delim:-><Group:GRP><Delim:-><Other:Obfuscated>",
		},
		// a delimiter that a row allows in front of a word that is not there
		{
			"The.CD.2020.1080p.WEB.H264-GRP",
			"<Text:The><Delim:.><Source:CD><Delim:.><Date:2020><Delim:.><Resolution:1080p><Delim:.><Source:WEB><Delim:.><Codec:H264>|<Delim:-><Group:GRP>",
		},
		// a plus that is the name of a tag stays in it
		{
			"Movie.2020.2160p.DSN+.WEB-DL.DD+5.1.HDR10+.H.265-GRP",
			"<Text:Movie><Delim:.><Date:2020><Delim:.><Resolution:2160p><Delim:.><Collection:DSN+><Delim:.><Source:WEB-DL><Delim:.><Audio:DD+><Channels:5.1><Delim:.><HDR:HDR10+><Delim:.><Codec:H.265>|<Delim:-><Group:GRP>",
		},
		// what sets the seasons of a range or a list apart. An ampersand is
		// no delimiter, and stays the end of the season in front of it
		{
			"Iruma-Kun S01-S03 1080p",
			"<Text:Iruma><Delim:-><Text:Kun><Delim: ><Series:S01><Delim:-><Series:S03><Delim: ><Resolution:1080p>",
		},
		{
			"Show (Season 1+2+3) [1080p]",
			"<Text:Show><Delim: (><Series:Season 1><Delim:+><Series:2><Delim:+><Series:3><Delim:) [><Resolution:1080p>|<Delim:]>",
		},
		{
			"Show Seasons 1 & 2 1080p",
			"<Text:Show><Delim: ><Series:Seasons 1 &><Delim: ><Series:2><Delim: ><Resolution:1080p>",
		},
		// an ellipsis is a tag of its own where it begins a run, and the
		// rest of the run is one tag
		{
			"Title....2019.x264 - GRP",
			"<Text:Title><Delim:...><Delim:.><Date:2019><Delim:.><Codec:x264><Delim: - >|<Group:GRP>",
		},
		{
			"Title ...2019",
			"<Text:Title><Delim: ...><Date:2019>",
		},
		// the delimiters a lexer gave as tags of its own join the run: the
		// bracket that closes a flag, the one after an episode, the
		// underscore that ends a codec
		{
			"[REPACK] [1080p] Show - 01",
			"<Delim:[><Other:REPACK><Delim:] [><Resolution:1080p><Delim:] ><Text:Show><Delim: - ><Series:01>",
		},
		{
			"Show - 05 ((1080p))",
			"<Text:Show><Delim: - ><Series:05><Delim: ((><Resolution:1080p>|<Delim:))>",
		},
		{
			"Show_480p_x264_-GRP_(CR).mkv",
			"<Text:Show><Delim:_><Resolution:480p><Delim:_><Codec:x264><Delim:_-><Text:GRP><Delim:_(><Collection:CR>|<Delim:).><Ext:mkv>",
		},
		// a run that the two parts of a name share is a tag of the part it
		// begins in
		{
			"Show [1080p].mkv",
			"<Text:Show><Delim: [><Resolution:1080p>|<Delim:].><Ext:mkv>",
		},
		{
			"Movie 2019 Opus 5.1 - GRP.mkv",
			"<Text:Movie><Delim: ><Date:2019><Delim: ><Audio:Opus><Delim: ><Channels:5.1><Delim: - >|<Group:GRP><Delim:.><Ext:mkv>",
		},
	} {
		tags, end := ParseTagsString(test.s)
		if s := tagStream(tags, end); s != test.exp {
			t.Errorf("%q expected:\n  %s\ngot:\n  %s", test.s, test.exp, s)
		}
	}
}

// edgeDelims returns how many bytes at the start and at the end of s are
// delimiters, and 0 for a string of nothing else.
func edgeDelims(s string) (lead, trail int) {
	for lead < len(s) && isAnyDelim(rune(s[lead])) {
		lead++
	}
	if lead == len(s) {
		return 0, 0
	}
	for isAnyDelim(rune(s[len(s)-1-trail])) {
		trail++
	}
	return lead, trail
}

// checkEdges checks the rule the tags of a name follow, lexed or built:
//
//   - A tag begins and ends with no delimiter but one that its value begins
//     or ends with, and that its row needs: the tag reads as another row, or
//     as none, without it ('DD+', 'HDR10+').
//   - A meta tag begins and ends with its brackets.
//   - A delimiter tag holds delimiters only, and the whole run of them at
//     its place, after a tag for each ellipsis the run begins with.
func checkEdges(tb testing.TB, name string, tags []Tag) {
	tb.Helper()
	for i, tag := range tags {
		text := tag.val(0)
		switch {
		case text == "":
		case tag.Is(TagTypeDelim):
			if strings.TrimFunc(text, isAnyDelim) != "" {
				tb.Errorf("%q: tag %d: expected a delimiter tag of delimiters only, got: %q", name, i, text)
			}
			if i != 0 && tags[i-1].Is(TagTypeDelim) && tags[i-1].val(0) != ellipsis || len(text) > len(ellipsis) && strings.HasPrefix(text, ellipsis) {
				tb.Errorf("%q: tag %d: expected one delimiter tag for a run of delimiters, got: %q after %q", name, i, text, tags[max(i-1, 0)].val(0))
			}
		case tag.Is(TagTypeWhitespace):
		case tag.Is(TagTypeMeta):
			if strings.TrimSpace(text) != text {
				tb.Errorf("%q: tag %d: expected a meta tag with no space around its brackets, got: %q", name, i, text)
			}
		default:
			lead, trail := edgeDelims(text)
			if lead == 0 && trail == 0 {
				continue
			}
			value := tag.val(1)
			if vlead, vtrail := edgeDelims(value); lead > vlead || trail > vtrail {
				tb.Errorf("%q: tag %d: expected a %s tag with no delimiter outside its value %q, got: %q", name, i, tag.TagType(), value, text)
				continue
			}
			if tag.f == nil || tag.ninfo == nil {
				continue
			}
			if info := tag.f(value[lead : len(value)-trail]); info == tag.ninfo {
				tb.Errorf("%q: tag %d: expected a %s tag with no delimiter its row %q can do without, got: %q", name, i, tag.TagType(), info.Tag(), text)
			}
		}
	}
}

// boundNames returns the names the rules of the tag boundaries are checked
// on: those of tests.yaml, and those of the corpora that are there.
func boundNames(t *testing.T) []string {
	var names []string
	for _, test := range rlsTests(t) {
		names = append(names, test.s)
	}
	if testing.Short() {
		return names
	}
	for _, file := range []string{"nyaa.txt", "sports.txt"} {
		if v, err := readCorpus(file); err == nil {
			names = append(names, v...)
		}
	}
	return names
}

// TestTagEdges checks that no tag starts or ends with a delimiter outside its
// value, on the tags of every name as they are lexed and as they are built,
// and that the tags reproduce the name in a slice with no room to spare.
func TestTagEdges(t *testing.T) {
	for _, name := range boundNames(t) {
		checkTags(t, name)
		checkEdges(t, name, ParseString(name).Tags())
	}
}

// TestFormatKeepsDelims checks that %s keeps the delimiters of a name. A
// release prints as its tags, a delimiter tag prints as it is written, and no
// other tag holds a delimiter outside its value, which checkEdges checks. So
// %s is the name with each value written the normal way and every delimiter
// between two values in its place.
func TestFormatKeepsDelims(t *testing.T) {
	for _, test := range []struct {
		s, exp string
	}{
		{"Show Name S01 1080p WEB-DL DDP5.1 H.264-GRP", "Show Name S01 1080p WEB-DL DDP5.1 H.264-GRP"},
		{"Movie.2019.1080p.BluRay.DTS-HD.MA.5.1.x264-GRP", "Movie.2019.1080p.BluRay.DTS-HD.MA.5.1.x264-GRP"},
		{"22 Jump Street (2014) 720p BrRip x264 - YIFY", "22 Jump Street (2014) 720p BDRiP x264 - YIFY"},
		{"[Commie] Hataraku Maou-sama! - 01 [1080p][ABCD1234].mkv", "[Commie] Hataraku Maou-sama! - S00E01 [1080p][ABCD1234].mkv"},
		{"[SubsPlease] Hell Mode S2 - 04 (480p) [20BE14E8].mkv", "[SubsPlease] Hell Mode S02 - S00E04 (480p) [20BE14E8].mkv"},
		{"(Comedy) Dave Chappelle - The Age of Spin (2017) 1080p WEBRip DD5.1 x264-TrollHD.mkv", "(Comedy) Dave Chappelle - The Age of Spin (2017) 1080p WEBRiP DD5.1 x264-TrollHD.mkv"},
		{"The.CD.2020.1080p.WEB.H264-GRP", "The.CD.2020.1080p.WEB.H.264-GRP"},
		{"Show.Title.S01E01.720p.HDTV.x264-GRP-Obfuscated", "Show.Title.S01E01.720p.HDTV.x264-GRP-Obfuscated"},
		{"Welcome To Demon School! Iruma-Kun S01-S03 1080p BDRip AV1 10bit FLAC-Anon", "Welcome To Demon School! Iruma-Kun S01-S03 1080p BDRiP AV1 10BIT FLAC-Anon"},
		{"Movie.2020.2160p.WEB-DL.DD+5.1.HDR10+.H.265-GRP", "Movie.2020.2160p.WEB-DL.DDP5.1.HDR10+.H.265-GRP"},
	} {
		if s := fmt.Sprintf("%s", ParseString(test.s)); s != test.exp {
			t.Errorf("%q expected %%s:\n  %s\ngot:\n  %s", test.s, test.exp, s)
		}
	}
	for _, name := range boundNames(t) {
		r := ParseString(name)
		var sb strings.Builder
		for i, tag := range r.Tags() {
			s := fmt.Sprintf("%s", tag)
			if o := fmt.Sprintf("%o", tag); tag.Is(TagTypeDelim) && s != o {
				t.Errorf("%q: tag %d: expected the delimiter %q to print as written, got: %q", name, i, o, s)
			}
			sb.WriteString(s)
		}
		if s := fmt.Sprintf("%s", r); s != sb.String() {
			t.Errorf("%q: expected %%s to print the tags:\n  %s\ngot:\n  %s", name, sb.String(), s)
		}
	}
}

// TestJoinCallerLexer checks that the tags of a lexer of a caller's are cut
// like those of this package's, whether it reads the name from the start or
// from the end, and that the tags such a lexer made stay as it made them.
func TestJoinCallerLexer(t *testing.T) {
	var made []Tag
	keep := func(tag Tag) Tag {
		made = append(made, tag)
		return tag
	}
	// '.END' at the end of the name, with the dot in front of it
	once := TagLexer{Once: true, Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
		if !strings.HasSuffix(string(src[i:n]), ".END") {
			return start, end, i, n, false
		}
		return start, append(end, keep(NewTag(TagTypeOther, nil, src[n-4:n], src[n-3:n]))), i, n - 4, true
	}}
	// 'KEY=' and a word, with the delimiters after it
	lexer := TagLexer{Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
		if !strings.HasPrefix(string(src[i:n]), "KEY=") {
			return start, end, i, n, false
		}
		j := i + 4
		for j < n && !isAnyDelim(rune(src[j])) {
			j++
		}
		k := j
		for k < n && isAnyDelim(rune(src[k])) {
			k++
		}
		return append(start, keep(NewTag(TagTypeID, nil, src[i:k], src[i+4:j]))), end, k, n, true
	}}
	p := NewParser(WithLexers(append([]Lexer{once, lexer}, DefaultLexers()...)...))
	const name = "Show.KEY=abc.-.1080p.x264-GRP.END"
	tags, end := p.Parse([]byte(name))
	const exp = "<Text:Show><Delim:.><ID:KEY=abc><Delim:.-.><Resolution:1080p><Delim:.><Codec:x264>|<Delim:-><Group:GRP><Delim:.><Other:END>"
	if s := tagStream(tags, end); s != exp {
		t.Errorf("%q expected:\n  %s\ngot:\n  %s", name, exp, s)
	}
	checkEdges(t, name, tags)
	if len(made) != 2 || made[0].val(0) != ".END" || made[1].val(0) != "KEY=abc.-." {
		t.Errorf("expected the tags of the lexer to keep their text, got: %q", made)
	}
	if s := fmt.Sprintf("%o", p.ParseRelease([]byte(name))); s != name {
		t.Errorf("expected %q, got: %q", name, s)
	}
}

// TestLeadingFlagsBracket checks that the name after the flags in brackets
// that open it is read as if it opened the name when a bracket of tags, a
// year in parentheses or a word that is a short tag follows the flags, where
// one run of delimiters closes the flag and opens the bracket ('] [', '] (').
func TestLeadingFlagsBracket(t *testing.T) {
	for _, test := range []struct {
		s, title, group, resolution string
		year                        int
		other                       []string
	}{
		{"[REPACK] [DSD] Show - 01 [1080p]", "Show", "DSD", "1080p", 0, []string{"REPACK"}},
		{"[REPACK][DSD] Show - 01 [1080p]", "Show", "DSD", "1080p", 0, []string{"REPACK"}},
		{"[REPACK] [1080p] Show - 01", "Show", "", "1080p", 0, []string{"REPACK"}},
		{"[REPACK] (2019) Movie Name [1080p]", "Movie Name", "", "1080p", 2019, []string{"REPACK"}},
		{"[PROPER][REPACK] (2019) Movie Name [1080p]", "Movie Name", "", "1080p", 2019, []string{"PROPER", "REPACK"}},
		{"[PROPER] [REPACK] [SubsPlease] Show - 04 (1080p)", "Show", "SubsPlease", "1080p", 0, []string{"PROPER", "REPACK"}},
		{"(REPACK) 2012.1080p.BluRay.x264-GRP", "2012", "GRP", "1080p", 0, []string{"REPACK"}},
	} {
		r := ParseString(test.s)
		if r.Title != test.title || r.Group != test.group || r.Resolution != test.resolution || r.Year != test.year || fmt.Sprint(r.Other) != fmt.Sprint(test.other) {
			t.Errorf("%q expected title %q, group %q, resolution %q, year %d, other %q, got: %q, %q, %q, %d, %q",
				test.s, test.title, test.group, test.resolution, test.year, test.other, r.Title, r.Group, r.Resolution, r.Year, r.Other)
		}
		if s := fmt.Sprintf("%o", r); s != test.s {
			t.Errorf("expected %q, got: %q", test.s, s)
		}
	}
}

// TestJoinReadings checks the readings that rest on a delimiter which a tag
// used to hold: the space after a leading bracket in a name with a slash, the
// dot of a file extension after a title and after a trailing source, the
// parentheses of a genre, the hyphen in front of a word after a leading
// group, and what sets the seasons of a range or a list apart.
func TestJoinReadings(t *testing.T) {
	for _, test := range []struct {
		s, title, group, genre string
		seasons                []int
	}{
		{"[Group] Fate/Zero.S01E01.1080p.WEB.x264-GRP", "Fate/Zero", "GRP", "", []int{1}},
		{"Some Show/Some.Show.S01E01.1080p.WEB.x264-GRP [ABCD1234]", "Some Show", "GRP", "", []int{1}},
		{"Title..mkv", "Title", "", "", nil},
		{"Movie.1985.DVDRip.XviD-DVD.mkv", "Movie", "DVD", "", nil},
		{"Show.S25.720p.WEB-DL.x264-TBS.mkv", "Show", "TBS", "", []int{25}},
		{"(Comedy) Dave Chappelle - The Age of Spin (2017) 1080p WEBRip", "Dave Chappelle - The Age of Spin", "", "Comedy", nil},
		{"( Comedy ) Dave Chappelle - The Age of Spin (2017) 1080p WEBRip", "Dave Chappelle - The Age of Spin", "", "Comedy", nil},
		{"DOC.Martin.S01E01.720p.HDTV.x264-GRP", "DOC Martin", "GRP", "", []int{1}},
		{"[Group] Show Name - Prologue.mkv", "Show Name - Prologue", "Group", "", nil},
		{"[Group] Movie (BD 1080p) - VOSTFR", "Movie", "Group", "", nil},
		{"Show S01-S03 1080p WEB-DL x264-GRP", "Show", "GRP", "", []int{1, 2, 3}},
		{"Show S01-03 1080p WEB-DL x264-GRP", "Show", "GRP", "", []int{1, 2, 3}},
		{"Show (Season 1 - 3) [1080p]", "Show", "", "", []int{1, 2, 3}},
		{"Show Seasons 1~7 [1080p]", "Show", "", "", []int{1, 2, 3, 4, 5, 6, 7}},
		{"Show S01+S02 1080p WEB-DL x264-GRP", "Show", "GRP", "", []int{1, 2}},
		{"Show S01,S02 1080p WEB-DL x264-GRP", "Show", "GRP", "", []int{1, 2}},
		{"Show Seasons 1 & 2 1080p WEB-DL x264-GRP", "Show", "GRP", "", []int{1, 2}},
		{"Show Season 1+2+3 [1080p]", "Show", "", "", []int{1, 2, 3}},
		{"Show.S01.S02.1080p.WEB-DL.x264-GRP", "Show", "GRP", "", []int{1, 2}},
	} {
		r := ParseString(test.s)
		if r.Title != test.title || r.Group != test.group || r.Genre != test.genre || fmt.Sprint(r.Seasons()) != fmt.Sprint(test.seasons) {
			t.Errorf("%q expected title %q, group %q, genre %q, seasons %v, got: %q, %q, %q, %v",
				test.s, test.title, test.group, test.genre, test.seasons, r.Title, r.Group, r.Genre, r.Seasons())
		}
	}
}
