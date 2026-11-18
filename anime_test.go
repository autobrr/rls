package rls

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestBracketRoles checks the role of every bracket of a name, in the order
// they open, as the structure stage reads it.
func TestBracketRoles(t *testing.T) {
	names := map[uint8]string{roleNone: "none", roleGroup: "group", roleSum: "sum", roleTags: "tags", roleNumber: "number", roleWords: "words", roleYear: "year"}
	for _, test := range []struct {
		s   string
		exp string
	}{
		{"[SubsPlease] Show - 04 (1080p) [ABCD1234].mkv", "group tags sum"},
		{"[YYQ][Youjo Senki S2][03][1080P][MP4]", "group words number tags tags"},
		{"[Group] Show (S2) 06 [1080p HEVC]", "group number tags"},
		{"[Group] Show (01-12) [English Dub] (Batch)", "group number tags words"},
		{"[Judas] Title (Other Name of It) - SP01 [1080p][Multi-Subs] (Weekly)", "group words tags tags words"},
		{"Show - 05 [4K_HDR(2160P)+1080P][]", "tags none"},
		{"(CBB) Title of It - 01 (BD 1080p)", "words tags"},
		{"[Group] Title [abcd1234] [BD]", "group sum tags"},
		{"[Group] Title [Not a Sum] [BD]", "group words tags"},
	} {
		tags, _ := ParseTagsString(test.s)
		_, spans := brackets(tags, nil, []span{})
		var v []string
		for _, s := range spans {
			v = append(v, names[s.role])
		}
		if s := strings.Join(v, " "); s != test.exp {
			t.Errorf("%q: expected the roles %q, got: %q", test.s, test.exp, s)
		}
	}
}

// epsOf writes the episodes of a numbering the way the tests of the stage
// read: 'S2:4' with a season, '1-12' for a range, '2.5' with a half, and the
// kind of an extra behind it ('2 ova').
func epsOf(n Numbering) string {
	var v []string
	for _, e := range n.Episodes {
		s := strconv.Itoa(e.First)
		if e.Last != e.First {
			s += "-" + strconv.Itoa(e.Last)
		}
		if e.Frac != 0 {
			s += "." + strconv.Itoa(int(e.Frac))
		}
		if e.Season != NoSeason {
			s = "S" + strconv.Itoa(e.Season) + ":" + s
		}
		if e.Kind != EpRegular {
			s += " " + e.Kind.String()
		}
		v = append(v, s)
	}
	return strings.Join(v, ", ")
}

// untold is the type of a row whose name does not say what the release is: a
// pack of a show that writes no season and no episode ('[Group] Title [BD
// 1080p]'). The parser has nothing to tell it from a film, so the row does
// not judge the type.
const untold Type = -1

// structureRow is a name written the way a fansub group writes one, with
// what the structure stage reads from it.
type structureRow struct {
	s     string
	typ   Type
	title string
	alt   string
	group string
	// the numbering: seasons, episodes as epsOf writes them, volume,
	// contents and batch
	seasons  []int
	eps      string
	volume   int
	contents string
	batch    bool
}

// structureRows returns the rows of TestStructure. They are in a func of
// their own so that TestNumberingBehindBrackets can read their names.
func structureRows() []structureRow {
	return []structureRow{
		// the roles of the brackets: the group, the title in a bracket of
		// its own, the number alone, and another title
		{"[WBDP] [Vivid] Uchouten Kazoku / The Eccentric Family 2 [BD][1080p-AAC]", untold, "Uchouten Kazoku 2", "The Eccentric Family 2", "WBDP", nil, "", 0, "", false},
		{"Gekkan Shoujo Nozaki-kun [HorribleSubs] (1080p)", untold, "Gekkan Shoujo Nozaki-kun", "", "HorribleSubs", nil, "", 0, "", false},
		// units: an extra with its number is an episode of its kind
		// a range of extras and a list of them: a hyphen makes a range,
		// tight or set off by spaces, and an ampersand a list
		// a spaced hyphen between a marker and its number: every marker that
		// names a kind counts the number, and behind a word that names none
		// the number is the episode
		// an extra with no number says what the release is
		// a volume or a box
		// what a pack holds, and a batch
		// the role of a bare number
		// more than one episode in bare numbers is a pack, and one file with
		// its checksum an episode that holds them
		{"[shincaps] Crayon Shin-chan - 450-459 (CS-EX2 1440x1080 MPEG2 AAC).ts", Series, "Crayon Shin-chan", "", "shincaps", nil, "450-459", 0, "", false},
		{"[Group] Show S01E01-E02 [1080p]", Episode, "Show", "", "Group", []int{1}, "S1:1-2", 0, "", false},
		// two numbers joined by a spaced hyphen are a range
		// 'Batch' outside a bracket is the mark where a hyphen sets it
		// apart, and the last word of a title otherwise. In a bracket of its
		// own it is the mark, at the end of the name as well
		{"[Group] Star Wars - The Bad Batch [1080p]", untold, "Star Wars - The Bad Batch", "", "Group", nil, "", 0, "", false},
		{"[SubsPlease] Star Wars The Bad Batch - 05 (1080p) [ABCD1234].mkv", Episode, "Star Wars The Bad Batch", "", "SubsPlease", nil, "5", 0, "", false},
		{"[Group] The Bad Batch - 05 [1080p]", Episode, "The Bad Batch", "", "Group", nil, "5", 0, "", false},
		{"[Group] Gremlins 2 - The New Batch [1080p]", untold, "Gremlins 2 - The New Batch", "", "Group", nil, "", 0, "", false},
		// a collection is a pack and names no contents, and the number in
		// front of it is a word of the title
		// numbers that are words of the title
		{"[Commie] Chihayafuru 2 [BD 720p AAC]", untold, "Chihayafuru 2", "", "Commie", nil, "", 0, "", false},
		{"[Group] Mob Psycho 100 [1080p]", untold, "Mob Psycho 100", "", "Group", nil, "", 0, "", false},
		{"[UTW-THORA] Evangelion 3.33 You Can (Not) Redo [BD][1080p,x264,flac]", Movie, "Evangelion 3.33 You Can (Not) Redo", "", "UTW-THORA", nil, "", 0, "", false},
		{"[Beatrice-Raws] Evangelion 3.0+1.0 Thrice Upon a Time [BDRip 1920x816 HEVC TrueHD]", Movie, "Evangelion 3.0+1.0 Thrice Upon a Time", "", "Beatrice-Raws", nil, "", 0, "", false},
		// markers of an episode and of a season that no lexer reads
		// the square brackets of CJK text
		// a name that no fansub group wrote is read where it sets a marker
		// apart from its title: behind a hyphen, a plus, the year or the
		// season, in a bracket, or with a word that only an anime release has
		// and a word that only an anime release has in front of the year,
		// in a name that a fansub group wrote
		// a title that ends where a unit starts ends with no joiner
		// another title in a bracket is one in another script or another
		// language than the title
		{"[Judas] Kimetsu no Yaiba (Demon Slayer) - S05E08 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly)", Episode, "Kimetsu no Yaiba", "Demon Slayer", "Judas", []int{5}, "S5:8", 0, "", false},
		{"[Anime Time] Hell's Paradise (Jigokuraku) - S02E09 [1080p][HEVC 10bit x265][AAC][Dual-Audio] [Multi Sub] [Weekly]", Episode, "Hell's Paradise", "Jigokuraku", "Anime Time", []int{2}, "S2:9", 0, "", false},
		{"[Golumpa] Attack on Titan (Shingeki no Kyojin) S2 - 01 [English Dub] [FuniDub 1080p x264 AAC] [MKV]", Episode, "Attack on Titan", "Shingeki no Kyojin", "Golumpa", []int{2}, "S2:1", 0, "", false},
		{"[Judas] Mushoku Tensei (Jobless Reincarnation) - S03E04 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly)", Episode, "Mushoku Tensei", "Jobless Reincarnation", "Judas", []int{3}, "S3:4", 0, "", false},
		{"[Grp] Title of It (進撃の巨人) - 01 [1080p]", Episode, "Title of It", "進撃の巨人", "Grp", nil, "1", 0, "", false},
		// and in the language of the title it is a part of it or a note,
		// which nothing tells apart, so it is left unread
		{"[SubsPlease] Kanteishi (Kari) - 09 (720p) [5C2E5E22].mkv", Episode, "Kanteishi", "", "SubsPlease", nil, "9", 0, "", false},
		{"[Chihiro] Watashi ga Koibito ni Nareru Wake Nai jan, Muri Muri! (Muri ja Nakatta) - 05 [1080p HEVC EAC3][1B252D5B].mkv", Episode, "Watashi ga Koibito ni Nareru Wake Nai jan, Muri Muri!", "", "Chihiro", nil, "5", 0, "", false},
		{"[Erai-raws] Heroine Seijo Iie All Works Maid desu (Hokori) - 05 [1080p CR WEBRip HEVC AAC][MultiSub][535EF1FB]", Episode, "Heroine Seijo Iie All Works Maid desu", "", "Erai-raws", nil, "5", 0, "", false},
		{"[Kametsu] The Legend of Heroes - Trails in the Sky (The Animation) [2011] (BD 1080p x264 10-bit FLAC) [Dual-Audio]", untold, "The Legend of Heroes - Trails in the Sky", "", "Kametsu", nil, "", 0, "", false},
		// a title written with a slash, and the same title twice
		// a bare slash cuts where the script changes, and a list of what
		// the release holds is no other title
		{"[DBD-Raws][人参果/The Monkey King and the Fruit of Immortality][正片+其他动画短片+花絮][1080P][BDRip][HEVC-10bit][中英内封][FLAC][MKV]", untold, "人参果", "The Monkey King and the Fruit of Immortality", "DBD-Raws", nil, "", 0, "", false},
		{"[DBD-Raws][Fate / stay night Unlimited Blade Works/命运之夜：无限剑制][1080P][BDRip][HEVC-10bit][FLAC][MKV]", untold, "Fate / stay night Unlimited Blade Works", "命运之夜：无限剑制", "DBD-Raws", nil, "", 0, "", false},
		{"[NC-Raws] D4DJ All Mix / D4DJ All Mix - 03 (CR 1920x1080 AVC AAC MKV)", Episode, "D4DJ All Mix", "", "NC-Raws", nil, "3", 0, "", false},
		{"[NC-Raws] 世界末日与柴犬同行 / Sekai no Owari ni Shiba Inu to - 2023010362 (Sentai 1920x1080 AVC AAC MKV)", untold, "世界末日与柴犬同行", "Sekai no Owari ni Shiba Inu to", "NC-Raws", nil, "", 0, "", false},
		// more than one title
		{"[Cleo] Saihate no Paladin | The Faraway Paladin [Dual Audio 10bit 1080p][HEVC-x265]", untold, "Saihate no Paladin", "The Faraway Paladin", "Cleo", nil, "", 0, "", false},
		{"[Beatrice-Raws] Tonari no Totoro / My Neighbor Totoro [DCPrip 1998x1080 HEVC TrueHD].mkv", Movie, "Tonari no Totoro", "My Neighbor Totoro", "Beatrice-Raws", nil, "", 0, "", false},
		{"[Void] 幻游猎人 | Huan You Lieren | Fantasy Hunter | Fantasy X Hunter (1080p Multi Sub)", untold, "幻游猎人", "Huan You Lieren|Fantasy Hunter|Fantasy X Hunter", "Void", nil, "", 0, "", false},
		{"[Group] Kyokou Suiri - In/Spectre - 05 [1080p]", Episode, "Kyokou Suiri - In/Spectre", "", "Group", nil, "5", 0, "", false},
		{"[Group] Bullet/Bullet - 03 [1080p]", Episode, "Bullet/Bullet", "", "Group", nil, "3", 0, "", false},
		{"[Cleo] Machikado Mazoku: 2-choume | The Demon Girl Next Door 2 [Dual Audio 10bit 1080p][HEVC-x265]", untold, "Machikado Mazoku: 2-choume", "The Demon Girl Next Door 2", "Cleo", nil, "", 0, "", false},
		{"[LoliHouse] 尼古喵喵 (邪竜解放版) / ヤニねこ / Yani Neko / Chainsmoker Cat - 04 [WebRip 1080p HEVC-10bit AAC][简繁内封字幕]", Episode, "尼古喵喵", "ヤニねこ|Yani Neko|Chainsmoker Cat", "LoliHouse", nil, "4", 0, "", false},
	}
}

// TestStructure checks what the structure stage reads from a name written
// the way a fansub group writes one: the title and the other titles, the
// group, and the numbering and the type that the lexers give with them.
// Every name must print as it was written.
func TestStructure(t *testing.T) {
	for _, test := range structureRows() {
		r := ParseString(test.s)
		n := r.Numbering
		if s := fmt.Sprintf("%o", r); s != test.s {
			t.Errorf("%q: expected the name to print as written, got: %q", test.s, s)
		}
		if r.Type != test.typ && test.typ != untold || r.Title != test.title || strings.Join(r.Alt, "|") != test.alt || r.Group != test.group {
			t.Errorf("%q: expected %s %q, other titles %q and group %q, got: %s %q, %q and %q", test.s,
				test.typ, test.title, test.alt, test.group, r.Type, r.Title, strings.Join(r.Alt, "|"), r.Group)
		}
		if !slices.Equal(n.Seasons, test.seasons) || epsOf(n) != test.eps || n.Volume != test.volume || strings.Join(n.Contents, "|") != test.contents || n.Batch != test.batch {
			t.Errorf("%q: expected seasons %v, episodes %q, volume %d, contents %q and batch %t, got: %v, %q, %d, %q and %t", test.s,
				test.seasons, test.eps, test.volume, test.contents, test.batch, n.Seasons, epsOf(n), n.Volume, strings.Join(n.Contents, "|"), n.Batch)
		}
		if _, ok := n.Episode(); ok != slices.ContainsFunc(n.Episodes, func(e Ep) bool { return e.Kind == EpRegular }) {
			t.Errorf("%q: expected Episode to give a regular episode alone, got: %t for %q", test.s, ok, epsOf(n))
		}
	}
}

// TestLeadWord checks which word in the bracket that opens a name is taken
// for who made the release, which makes the name one that a fansub group
// wrote, see shapeOf. A year and a word that the vocabulary reads as a tag
// name no maker. Any other word does, and so does the tag of a tracker,
// which nothing tells from a group ('[TGx]', '[YTS]'), and a word of quality
// that the vocabulary does not hold ('[HD]').
func TestLeadWord(t *testing.T) {
	builder := DefaultParser.(*TagParser).state().builder.(*TagBuilder)
	for _, test := range []struct {
		word  string
		maker bool
	}{
		{"2019", false},
		{"1080p", false},
		{"4K", false},
		{"UHD", false},
		{"BluRay", false},
		{"Remux", false},
		{"NF", false},
		{"Netflix", false},
		{"Criterion", false},
		{"HEVC", false},
		{"x265", false},
		{"Hi10", false},
		{"Uncut", false},
		{"English", false},
		{"PL", false},
		{"RUS", false},
		{"VOSTFR", false},
		{"Multi", false},
		{"MULTi", false},
		{"MULTi5", false},
		{"Subbed", false},
		{"Dubbed", false},
		{"Dual Audio", false},
		{"Rus Eng", false},
		{"ENG SUB", false},
		{"700MB", false},
		{"Comedy", false},
		{"REQ", false},
		// the checksum of a file names no maker, and says in its own way
		// that a fansub group wrote the name
		{"ABCD1234", false},
		{"SubsPlease", true},
		{"Erai-raws", true},
		{"TGx", true},
		{"YTS", true},
		{"RARBG", true},
		{"Torrent9.cz", true},
		{"www.example.org", true},
		{"Sony Music", true},
		{"Request", true},
		{"Repost", true},
		{"Exclusive", true},
		{"New", true},
		{"Mixed", true},
		{"HD", true},
		{"Full HD", true},
	} {
		s := "[" + test.word + "] Kill Bill Vol 2 [1080p]"
		tags, _ := ParseTagsString(s)
		in, spans := brackets(tags, nil, []span{})
		if sh, _ := builder.shapeOf(tags, in, spans); sh.lead != test.maker {
			t.Errorf("%q: expected a maker in the bracket to be %t", s, test.maker)
		}
	}
}

// TestFlagsInFront checks that a name behind flags in a bracket is read as
// the name alone is, and that the release holds the tags it was read from.
// The builder reads the rest of such a name in a build of its own and puts
// the flags back in front. The structure stage reads some names in a copy of
// their tags, and that copy is what the release has to keep: its numbering
// agrees with its tags, see checkNumbering, and each tag behind the flags has
// the type it has in the name alone.
func TestFlagsInFront(t *testing.T) {
	// typed lists the tags of r from byte from on with their types, less the
	// delimiters, of which the flags and the name share one
	typed := func(r Release, from int) []string {
		var v []string
		at := 0
		for _, tag := range r.tags {
			if at >= from && !tag.Is(TagTypeDelim, TagTypeWhitespace) {
				v = append(v, fmt.Sprintf("%d %s", tag.typ, tag.v[0]))
			}
			at += len(tag.v[0])
		}
		return v
	}
	for _, test := range []struct {
		s     string
		flags int
	}{
		// the input that a fuzz run found. What stands behind its flags
		// opens with words in parentheses, which the stage reads in a copy
		{"[REPACK7](A000)00 000(000000", 9},
		// a group in parentheses, which is a group tag in a copy
		{"[REPACK] (CBB) Aoki Hagane no Arpeggio - The Complete Series (BD 1080p)(HEVC-x265-10bit)", 9},
		{"[PROPER][REPACK] (CBB) Aoki Hagane no Arpeggio - 01 (BD 1080p)", 17},
		// a number with a point in the title, which is text in a copy
		{"[REPACK] [Beatrice-Raws] Evangelion 3.0+1.0 Thrice Upon a Time [BDRip 1920x816 HEVC TrueHD]", 9},
		// a name that is read in place
		{"(REPACK) [SubsPlease] Show - 05 (1080p) [ABCD1234].mkv", 9},
	} {
		r, alone := ParseString(test.s), ParseString(test.s[test.flags:])
		checkNumbering(t, test.s, r)
		if o := fmt.Sprintf("%o", r); o != test.s {
			t.Errorf("%q: expected the name to print as written, got: %q", test.s, o)
		}
		if r.Type != alone.Type || r.Title != alone.Title || r.Group != alone.Group || !slices.Equal(r.Alt, alone.Alt) || !reflect.DeepEqual(r.Numbering, alone.Numbering) {
			t.Errorf("%q: expected what the name alone gives, %s %q, group %q, other titles %q and %+v, got: %s %q, %q, %q and %+v", test.s,
				alone.Type, alone.Title, alone.Group, alone.Alt, alone.Numbering, r.Type, r.Title, r.Group, r.Alt, r.Numbering)
		}
		if got, exp := typed(r, test.flags), typed(alone, 0); !slices.Equal(got, exp) {
			t.Errorf("%q: expected the tags of the name alone behind the flags, %q, got: %q", test.s, exp, got)
		}
	}
}

// TestNumberingBehindBrackets checks checkNumbering, and that the name prints
// as written, on every name of tests.yaml and of TestStructure with each kind
// of bracket in front of it that the builder reads in a way of its own: flags,
// which it cuts off to build the rest alone, a checksum, a site, a group in
// square brackets, in parentheses and in the square brackets of CJK text, and
// a bracket that names no maker. Each of them stands behind flags as well,
// which is where the build of the rest and a copy of the tags meet. Every
// name is also built with all of its tags inside it, see buildWhole.
func TestNumberingBehindBrackets(t *testing.T) {
	names := benchCorpus(t)
	for _, row := range structureRows() {
		names = append(names, row.s)
	}
	flags := []string{"[REPACK] ", "[REPACK]", "[REPACK7]", "[PROPER][REPACK] ", "(REPACK) "}
	brackets := []string{"[ABCD1234] ", "[TGx] ", "[Group] ", "(Group) ", "(Group)", "【Group】", "[4K]"}
	prefixes := slices.Concat(flags, brackets)
	for _, f := range flags[:2] {
		for _, b := range brackets {
			prefixes = append(prefixes, f+b)
		}
	}
	for _, prefix := range prefixes {
		for _, name := range names {
			s := prefix + name
			r := ParseString(s)
			if o := fmt.Sprintf("%o", r); o != s {
				t.Errorf("%q: expected the name to print as written, got: %q", s, o)
			}
			checkNumbering(t, s, r)
			checkNumbering(t, s, buildWhole(s))
		}
	}
}

// TestStructureLeaves checks the names the structure stage must not read as
// a fansub name: a scene name, a record, a film that opens with its year, a
// line of an announcer. Each has a word or a number the stage would read in a
// fansub name.
func TestStructureLeaves(t *testing.T) {
	for _, test := range []struct {
		s, title string
	}{
		{"Show.OVA.S01E02.1080p.WEB.H264-GRP", "Show OVA"},
		{"Show.Vol.2.2019.1080p.BluRay.x264-GRP", "Show Vol 2"},
		{"[TGx] Movie.Name.Batch.2019.1080p.x264-GRP", "Movie Name Batch"},
		{"[test]50.50.2011.BluRay.1080p.DTS-HD", "50 50"},
		{"(2001) A Space Odyssey (1968).mkv", "2001 A Space Odyssey"},
		{"Artist - Greatest Hits Vol. 2 (2019) [FLAC]", "Greatest Hits Vol. 2"},
		{"(dj_tiesto)-live_at_dutch_dimension_02-02-02__darkone-ipzvid", "dj tiesto"},
		{"-[new TV]2[Some.Show.S05E05.720p.WEB.h264-GROUP]3[000000]", "new TV"},
		{"Show Name Special 01 1080p WEB H264-GRP", "Show Name Special 01"},
		{"[FLAC] Artist - The Complete Series Box (2015)", "The Complete Series Box"},
		{"[MP3] Artist - Live (Batch Release) (2019)", "Live"},
	} {
		r := ParseString(test.s)
		if r.shaped || r.Title != test.title || r.Numbering.Contents != nil || r.Numbering.Volume != 0 || r.Numbering.Batch {
			t.Errorf("%q: expected the stage to leave the name with the title %q, got: shaped %t, title %q and %+v", test.s, test.title, r.shaped, r.Title, r.Numbering)
		}
	}
}

// TestStructureAnnounce checks that the stage reads nothing in a line of an
// announcer, which is no name of a release and has the brackets and the
// resolution of one: the category in a bracket is no contents, and a bracket
// behind the word that names it is no part of a title.
func TestStructureAnnounce(t *testing.T) {
	for _, s := range []string{
		"New in [Movies]: Road House 1989 1080p BluRay DD 5.1 x264-CtrlHD [1080p] [Encode] [11.84 GiB] [FL: No] [INTERNAL: No] [https://znth.cx/torrents/2364] by: [Anonymous]",
		"New in [TV]: Training Day S01 1080p AMZN WEBRip DD+ 5.1 x264-RTN [1080p] [WEBRip] [41.2 GiB] [FL: Yes] [INTERNAL: No] [https://znth.cx/torrents/2348] by: [Bitjoe]",
		"tehFIRE: [TV|XviD] Royal.Institution.Christmas.Lectures.2009.Part2.WS.PDTV.XviD-WATERS :: preGAP: 1m and 9s :: https://pretome.info/details.php?id=127107",
		"Name: [Studio] Example Scene Title - Performer One [2160p] Freeleech [Yes] Size [8.25 GiB] Uploader [Anonymous]",
	} {
		r := ParseString(s)
		if n := r.Numbering; n.Contents != nil || n.Volume != 0 || n.Batch || r.Alt != nil || strings.ContainsAny(r.Title, "[]") {
			t.Errorf("%q: expected no contents, volume, batch or other title, and a title with no bracket, got: %+v, %q and the title %q", s, n, r.Alt, r.Title)
		}
	}
}

// TestStructureKeepsTags checks that a name the structure stage does not
// read keeps the tags the lexers made, each as it was, and its end and what
// its group was read from: the steps in front of the gate change tags, and
// they work on a copy that is dropped. The names are those of the tests and
// the corpora, and names made so that each of those steps finds something to
// change in a name the gate then turns down.
func TestStructureKeepsTags(t *testing.T) {
	b := DefaultParser.(*TagParser).state().builder.(*TagBuilder)
	names := append(boundNames(t),
		// a number with a point and a group in parentheses, each in a
		// scene name
		"Movie 3.0 Name.Of.It.2019.1080p.x264-GRP [x]",
		"(GRP) Movie.Name.Of.It.2019.x264 [1080p]",
		// and in a record, which has no sign of a video
		"(GRP) Artist - Album 2.0 Remix (2019) [FLAC]",
		"[FLAC] Artist - Album 2.0 Remix (2019)",
	)
	var kept, read int
	for _, name := range names {
		tags, end := ParseTagsString(name)
		r := &Release{tags: tags, end: end}
		b.fixHalfEpisode(r)
		b.fixAudioRange(r)
		before, from := slices.Clone(r.tags), r.groupFrom
		end = r.end
		b.structure(r)
		if r.shaped {
			read++
			continue
		}
		kept++
		same := len(r.tags) == len(before) && r.end == end && r.groupFrom == from
		for i := 0; same && i < len(before); i++ {
			x, y := r.tags[i], before[i]
			same = x.typ == y.typ && x.was == y.was && slices.Equal(x.v, y.v) && fmt.Sprintf("%e", x) == fmt.Sprintf("%e", y)
		}
		if !same {
			t.Errorf("%q: expected the tags of a name the stage does not read as the lexers made them:\n  %e\ngot:\n  %e", name, before, r.tags)
		}
	}
	if kept == 0 || read == 0 {
		t.Errorf("expected names the stage reads and names it does not, got: %d read and %d not", read, kept)
	}
}

// TestStructureLinear checks that the time to read a name grows with its
// length and not with the square of it, for names made of what the structure
// stage looks around for. A name is untrusted input. Each shape is read at
// two lengths, one eight times the other, and the longer may take 24 times
// as long, as in TestSettleLinear. A shape that takes longer is read again,
// up to three times, so that a pause of the machine fails no run.
//
// The shapes of linearShapes are made from the tables of the stage, so that
// what a new rule reads there is timed without anyone adding a shape for it.
// They are timed on the builder alone, which is where the stage runs, with
// tags that repeatTags makes, so that there is time for all of them at a
// length that shows a square. The shapes listed here are those that no table
// gives, and are timed on the whole parse.
func TestStructureLinear(t *testing.T) {
	builder := DefaultParser.(*TagParser).state().builder
	check := func(shape [3]string, n int, whole bool) {
		var names [2]string
		var tags [2][]Tag
		var ends [2]int
		for i, n := range [2]int{n, 8 * n} {
			if names[i] = shape[0] + strings.Repeat(shape[1], n) + shape[2]; !whole {
				tags[i], ends[i] = repeatTags(shape, n)
			}
		}
		var took [2]time.Duration
		for range 3 {
			for i, s := range names {
				// the builder writes to the tags it is given
				own := slices.Clone(tags[i])
				start := time.Now()
				if whole {
					ParseString(s)
				} else {
					builder.Build(own, ends[i])
				}
				took[i] = time.Since(start)
			}
			if took[1] <= 24*took[0] {
				return
			}
		}
		t.Errorf("%q: %d of %q took %s and %d took %s, more than 24 times as long", shape[0]+shape[1]+shape[2], 8*n, shape[1], took[1], n, took[0])
	}
	for _, shape := range [][3]string{
		{"[G] T ", "1.0+", "x [1080p]"},
		{"[G] T", " | A", " [1080p]"},
		{"[G] T", " / A", " - 01 [1080p]"},
		{"[G] T", "/あ", " - 01 [1080p]"},
		{"[G] T ", "(a b) ", "- 01 [1080p]"},
		{"[G] T ", "[01]", "[1080p]"},
		{"[G][T", " a", "][01][1080p]"},
		// a run of merges, each of which join makes: a spaced range of
		// extras, a range in the bracket of a complete series, and a range
		// with the marker on both ends
		// runs of the square brackets of CJK text, which the lexers read as
		// one word
		{"", "】", ""},
		{"[G] T ", "(a) / ", "b [1080p]"},
		{"[G] Ta no ", "(The b) ", "[1080p]"},
		{"(G) T a ", "b ", "[1080p]"},
	} {
		check(shape, 1000, true)
	}
	// the race detector makes a parse some ten times slower, and the run
	// without it times every shape, so one in eight is timed under it. A
	// pool that drops what it is given tells it
	step := 1
	if poolDrops() {
		step = 8
	}
	for i, shape := range linearShapes() {
		if i%step == 0 {
			check(shape, 500, false)
		}
	}
}

// repeatTags returns the tags of the name that shape makes with n of its
// units, and where the head of the name ends, without lexing a name of that
// length: the tags of a name with a few units, with the tags of the last
// unit of its run written again for each unit more. That is what the lexers
// give as long as they read a unit as they read its neighbours, which is
// checked against the tags of a name with one unit more and of one with 16
// more. A shape that does not hold it for is lexed at its length.
func repeatTags(shape [3]string, n int) ([]Tag, int) {
	const few = 8
	lex := func(n int) ([]Tag, int) {
		return ParseTagsString(shape[0] + strings.Repeat(shape[1], n) + shape[2])
	}
	same := func(a, b []Tag) bool {
		return slices.EqualFunc(a, b, func(a, b Tag) bool { return a.typ == b.typ && a.was == b.was && slices.Equal(a.v, b.v) })
	}
	a, aend := lex(few)
	b, bend := lex(few + 1)
	// what the two have in common from the start reaches to the end of the
	// run in the shorter, and the tags in front of that are one unit
	at, per := 0, len(b)-len(a)
	for at < len(a) && same(a[at:at+1], b[at:at+1]) {
		at++
	}
	if n < few || per <= 0 || at < per || bend != aend && bend != aend+per {
		return lex(n)
	}
	grow := func(more int) ([]Tag, int) {
		tags := make([]Tag, 0, len(a)+more*per)
		tags = append(tags, a[:at]...)
		for range more {
			tags = append(tags, a[at-per:at]...)
		}
		end := aend
		if bend != aend {
			end += more * per
		}
		return append(tags, a[at:]...), end
	}
	for _, more := range [2]int{1, 16} {
		got, end := grow(more)
		if exp, expEnd := lex(few + more); !same(got, exp) || end != expEnd {
			return lex(n)
		}
	}
	return grow(n - few)
}

// linearShapes returns the names that TestStructureLinear makes from the
// tables of the stage, as what opens the name, what is repeated and what
// ends it:
//
//   - every joiner, which is any sign of ASCII that is no bracket, written
//     tight and set off by spaces, between numbers, between a word and two
//     numbers, and between words
//   - every kind of bracket the stage reads, around one word and two, a
//     number, a word with a number, a batch, nothing, and left open or never
//     opened
func linearShapes() [][3]string {
	var shapes [][3]string
	add := func(head, unit, tail string) {
		shapes = append(shapes, [3]string{head, unit, tail})
	}
	for c := byte(' '); c < 0x7f; c++ {
		if c >= '0' && c <= '9' || c|0x20 >= 'a' && c|0x20 <= 'z' || strings.IndexByte("()[]{}", c) != -1 {
			continue
		}
		for _, j := range []string{string(c), " " + string(c) + " "} {
			add("[G] T ", "01"+j, "02 [1080p]")
			add("[G] T ", "OVA 01"+j+"03 ", "[1080p]")
			add("[G] T", j+"a", " - 01 [1080p]")
		}
	}
	brackets := [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}}
	for _, b := range brackets {
		for _, inner := range []string{"a", "a b", "01", "OVA", "OVA 01", "Batch", "Complete Series", ""} {
			add("[G] T ", b[0]+inner+b[1]+" ", "[1080p]")
		}
		add("[G] T ", b[0]+" ", "[1080p]")
		add("[G] T ", b[1]+" ", "[1080p]")
		add("", b[0]+"a"+b[1], "[T][01][1080p]")
	}
	return shapes
}
