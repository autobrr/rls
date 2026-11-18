package rls

import (
	"regexp/syntax"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// The structure stage reads a name the way a fansub group writes one:
//
//	[Group] Title - 05 [tags][checksum].mkv
//	[Group][Title][05][tags]
//	[Group] Title | Other Title - OVA 02 (Season 1) [tags]
//
// It runs in the builder, in front of the zones, on the tags the lexers
// made. Such a name says with its brackets what a scene name says with its
// order: which part is the group, which the title and which the tags. The
// stage reads, in this order:
//
//   - the role of every bracket, see brackets, and from the roles the group
//     that the meta lexer does not read, see leadGroup
//   - the shape of the name, and whether the stage reads it at all: where
//     the title starts, and where the head ends, which is the title and
//     what numbers it, see shapeOf
//   - what the order of the lexers hides, see repairs
//   - what a pack holds and whether the name is a batch, see contents
//   - the words that say what a release is and the numbers that count them,
//     see units
//   - which bare number of the head is the episode, see numbers
//
// Each step gives the tags it reads their type in place, so that the next
// step, the zones and the titles read what it decided, and the few runs of
// tags that are one number are joined last, see join. A word or a number of
// a unit is a unit tag, see tagTypeUnit, which Release.numbering reads into
// Numbering, and packType reads the type from that. The title of such a name
// is read by titleParts, which knows that it may give more than one.
//
// A name the stage does not read keeps the tags the lexers made, each as it
// was: what the steps in front of shapeOf change, they change in a copy.

// shape is how a name is laid out, as positions in its tags.
type shape struct {
	// first is the first word of the title.
	first int
	// head is where the title and its numbering end: the first bracket of
	// tags, the checksum or a group behind the title, or the first tag
	// outside a bracket that only a tag can be.
	head int
	// year is the single year that ends the title, or -1 ('Title (2003)
	// [1080p]'): every word in front of it is a word of the title.
	year int
	// lead is whether a bracket that names who made the release opens the
	// name, and fansub whether the name says that a fansub group wrote it,
	// which that bracket or a checksum does, see shapeOf.
	lead, fansub bool
	// boxed is whether the title stands in a bracket of its own
	// ('[Group][Title][01][1080p]').
	boxed bool
	// single is whether the name is one file, which a checksum or the
	// extension of a video says: sum and ext are which of the two it has.
	single, sum, ext bool
	// video is whether the name has a sign of a video: a resolution, a
	// video codec, a bit depth or the extension of a video file.
	video bool
}

// hard reports whether typ is a type that ends the head of a name when it
// stands outside a bracket: what it claims is no word of a title there.
func hard(typ TagType) bool {
	switch typ {
	case TagTypeResolution, TagTypeSource, TagTypeCodec, TagTypeHDR, TagTypeBitDepth, TagTypeAudio,
		TagTypeChannels, TagTypeContainer, TagTypeSize, TagTypeFrameRate, TagTypeDisc:
		return true
	}
	return false
}

// videoExt reports whether s is the extension of a video file.
func videoExt(s string) bool {
	switch strings.ToLower(s) {
	case "mkv", "mp4", "avi", "rmvb", "ogm", "wmv", "ts", "m2ts", "webm", "flv", "mov", "mpg", "m4v", "divx":
		return true
	}
	return false
}

// shapeOf reads the shape of tags from its brackets, and reports whether the
// stage reads the name. That is a name with a bracket that sets its words
// apart with spaces, and it is read in one of two ways.
//
// A name that a fansub group wrote is read in full. The name says so itself,
// in one of two ways:
//
//   - a bracket opens it that names who made the release: a group or a
//     site ('[Judas]', '(CBB)'), or words that are no tag ('[Golumpa &
//     Aergia]'). A bracket that holds tags, a year or what the release is
//     opens any kind of name, see nameless ('[FLAC] Artist - Album',
//     '[BluRay] Title', '[2019] Title', '(2001) A Space Odyssey', '[Movies]
//     Title')
//   - it has the checksum of its file ('[ABCD1234]')
//
// The tag of a tracker in front of a spaced title cannot be told from a
// group, and opens the name as one does: '[TGx] Dante 01 1080p WEB-DL x264'
// is the film 'Dante 01', and is read as episode 1 of 'Dante' the way
// '[Group] Title 01 1080p' is. No list of such tags is kept.
//
// Any other name with a sign of a video is the name of a film or a show as
// YTS, QxR, TGx or a Plex library write it: a resolution, a codec or the
// extension of a file says what the release is and not who named it. In such
// a name 'Special', 'Movie', 'Extra' and 'Vol' are words of English, so the
// stage reads there only what the name sets apart from its title, see
// stage.apart, and no volume in the title ('Kill Bill Vol 1 1080p BluRay').
// A bare number needs one more sign to be the episode there, see numbers.
//
// In both, a single year behind the title ends it as it does in the name of
// a film: every word in front of the year is a word of the title, whatever
// it says ('Kill Bill: Vol. 1 (2003) [1080p]', 'The Specials (2019)',
// '[TGx] Movie Name Vol. 2 (2019) 1080p'), and nothing in front of it is
// read as a unit or as the episode. A range of years is the run of a show and
// ends no such title ('Friends The Complete Series (1994-2004)'), and neither
// does a year behind a season or an episode.
//
// A name with none of this is left as the lexers read it: one with no
// bracket, one that sets its words apart with dots, which is a scene name
// whatever opens it ('[TGx] Movie.Name.2019.1080p.x264-GRP'), and a record or
// a book, which has no sign of a video ('Artist - Album (2019) [FLAC]').
func (b *TagBuilder) shapeOf(tags []Tag, in []uint8, spans []span) (shape, bool) {
	sh := shape{first: -1, head: len(tags), year: -1}
	if len(spans) == 0 {
		return sh, false
	}
	for _, tag := range tags {
		switch tag.typ {
		case TagTypeMeta:
			k, _ := tag.Meta()
			sh.sum = sh.sum || k == "sum"
		case TagTypeExt:
			sh.ext = sh.ext || videoExt(tag.val(1))
		case TagTypeResolution, TagTypeCodec, TagTypeBitDepth:
			sh.video = true
		}
	}
	// the brackets that open the name, each right behind the one before. g
	// is the first of them that names who made the release
	k, at, g := 0, 0, 0
	for ; k < len(spans); k++ {
		// the delimiter that opens a bracket stands in front of it, and
		// the one that closes it behind
		open, end := spans[k].open-1, spans[k].end+1
		if spans[k].meta {
			open, end = spans[k].open, spans[k].end
		}
		// a year in parentheses opens a film, and no fansub name ('(2001) A
		// Space Odyssey (1968)'). Neither does a bracket that something
		// else than a space sets off: a hyphen in front of it is a line of
		// an announcer ('-[new TV]2[Some.Show.S05E05]'), and one right
		// behind it joins the artist of a record to its title
		// ('(dj_tiesto)-live_at_dutch_dimension-2002')
		if !gap(tags, at, open) || spans[k].role == roleYear {
			break
		}
		if !spans[k].meta && strings.Trim(tags[open].v[1], " _[({)]}") != "" {
			break
		}
		if end-1 < len(tags) && tags[end-1].Is(TagTypeDelim) && strings.HasPrefix(strings.TrimLeft(tags[end-1].v[1], ")]}"), "-") {
			break
		}
		// who made the release, and in front of that a bracket that names
		// no one ('[4K][Group][Title][2160P]')
		if r := spans[k].role; (r == roleGroup || r == roleWords) && !b.nameless(tags, spans[k]) {
			sh.lead = true
		} else if !sh.lead {
			g = k + 1
		}
		at = end
	}
	for _, s := range spans {
		sh.sum = sh.sum || s.role == roleSum
	}
	sh.single, sh.fansub, sh.video = sh.sum || sh.ext, sh.lead || sh.sum, sh.video || sh.ext
	// the title starts at the first word outside a bracket. With none in
	// front of the tags, it is the bracket of words behind the group
	// ('[Group][Title][01][1080p]'), or the one that opens the name when a
	// number follows it ('[Title][07][1080p]')
	for i := at; i < len(tags) && in[i] == bracketNone; i++ {
		// a number behind the bracket of words that follows the group is
		// the episode of the title in that bracket ('[Group][Title] 02
		// [720p]')
		if tags[i].Is(TagTypeText) &&
			!(k >= g+2 && allDigits(tags[i].v[0], 4) && spans[g].role == roleGroup && spans[g+1].role == roleWords) {
			sh.first = i
			break
		}
		if !tags[i].Is(TagTypeDelim, TagTypeWhitespace, TagTypeText) {
			break
		}
	}
	title := -1
	if sh.first == -1 {
		switch {
		case k >= g+2 && spans[g].role == roleWords && spans[g].by == '[' && spans[g+1].role == roleNumber && spans[g+1].by == '[':
			title = g
		// behind a bracket that names no one, the group is a bracket of
		// words, as the meta lexer read the first bracket as the site
		case k >= g+2 && (spans[g].role == roleGroup || g > 0 && spans[g].role == roleWords) && spans[g+1].role == roleWords && spans[g+1].by == '[':
			title = g + 1
		default:
			return sh, false
		}
		sh.boxed = true
		for i := spans[title].open; i < spans[title].end; i++ {
			if tags[i].Is(TagTypeText) {
				sh.first = i
				break
			}
		}
	}
	if sh.first == -1 {
		return sh, false
	}
	var spaces, dots int
	for i := sh.first; i < len(tags) && (in[i] == bracketNone || sh.boxed) && !hard(tags[i].typ); i++ {
		if !tags[i].Is(TagTypeDelim) {
			continue
		}
		if d := tags[i].v[1]; d == "." {
			dots++
		} else if strings.ContainsAny(d, " _") {
			spaces++
		}
	}
	if dots > spaces || !sh.fansub && !sh.video {
		return sh, false
	}
	// the head ends at the first bracket of tags, checksum or group behind
	// the title, or at the first hard tag outside a bracket
	n := 0
	for n < len(spans) && spans[n].end <= sh.first {
		n++
	}
	if sh.boxed {
		n = title + 1
	}
	for i := sh.first; i < len(tags); i++ {
		for n < len(spans) && spans[n].end <= i {
			n++
		}
		if n < len(spans) && i >= spans[n].open && i < spans[n].end {
			if s := spans[n]; s.open > sh.first && (s.role == roleTags || s.role == roleSum || s.role == roleGroup || s.role == roleNone) {
				sh.head = s.open
				break
			}
			continue
		}
		if hard(tags[i].typ) || tags[i].Is(TagTypeGroup) {
			sh.head = i
			break
		}
	}
	// the year that ends the title. A title in a bracket of its own ends
	// with its bracket
	for i := sh.first + 1; i < sh.head && !sh.boxed && !tags[i].Is(TagTypeSeries); i++ {
		if !tags[i].Is(TagTypeDate) {
			continue
		}
		if _, month, _ := tags[i].Date(); month == 0 && tags[i].yearEnd() == 0 {
			sh.year = i
		}
		break
	}
	return sh, true
}

// nameless reports whether the bracket s, which opens a name, names no one
// who made the release. The meta lexer reads any one word in the bracket that
// opens a name as its site, so the word is looked at here: it is a year
// ('[2019] Title'), a word that says what the release is, see category, or a
// word that the vocabulary reads as a tag, see tagged ('[BluRay]', '[4K]',
// '[x265]', '[English]', '[PL]', '[Hi10]', '[Uncut]', '[Netflix]'), or words
// set apart by spaces that each are one or belong to one ('[Rus Eng]', '[ENG
// SUB]'). A known group is a group whatever else its name spells. Any other
// word names a maker: one of quality that the vocabulary does not hold
// ('[HD]'), and two tags written as one word ('[AAC2.0]').
func (b *TagBuilder) nameless(tags []Tag, s span) bool {
	word := ""
	switch {
	case s.meta:
		_, word = tags[s.open].Meta()
	case s.words == 1:
		for i := s.open; i < s.end; i++ {
			if tags[i].Is(TagTypeText) {
				word = tags[i].v[0]
				break
			}
		}
	}
	switch {
	case word == "":
		return false
	case len(word) == 4 && allDigits(word, 4) && (word[:2] == "19" || word[:2] == "20"):
		return true
	case category(word):
		return true
	}
	// the vocabulary is asked first, as most groups are in neither
	if !b.tagged(word) {
		if !strings.Contains(word, " ") {
			return false
		}
		for w := range strings.FieldsSeq(word) {
			if !b.tagged(w) && !tagWord(w) {
				return false
			}
		}
	}
	return b.groupf == nil || b.groupf.find(word) == nil
}

// tagged reports whether the vocabulary reads s, whole, as a tag of any type
// but a group: a source, a resolution, a codec, an audio format, a language,
// a bit depth, a size and so on ('BluRay', 'x265', 'PL', 'Uncut', '700MB',
// 'MULTi5'). A row with spellings is asked by them, and a row that only its
// regexp confirms by that, unless it takes any run of letters, see lettered.
func (b *TagBuilder) tagged(s string) bool {
	var key [48]byte
	if s == "" || len(s) > len(key) {
		return false
	}
	for j := 0; j < len(s); j++ {
		c := s[j]
		if c >= 0x80 {
			return false
		}
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		key[j] = c
	}
	words := b.words.Load()
	if words == nil {
		words = &tagWords{spelled: make(map[string][]wordRow)}
		for typ, infos := range b.infos {
			if typ == "group" {
				continue
			}
			f := newFinder(noType, infos...)
			for k, rows := range f.exact {
				for _, i := range rows {
					words.spelled[k] = append(words.spelled[k], wordRow{f, i})
				}
			}
			for _, i := range f.rest {
				var lead [256]bool
				if lettered(f.infos[i].RE()) || !firstBytes(f.infos[i].RE(), &lead) {
					continue
				}
				for c, ok := range lead {
					if ok {
						words.rest[c] = append(words.rest[c], f.infos[i])
					}
				}
			}
		}
		b.words.Store(words)
	}
	for _, row := range words.spelled[string(key[:len(s)])] {
		if sp := row.f.spellings(row.i); sp.strs != nil && sp.has(s) {
			return true
		}
	}
	for _, info := range words.rest[s[0]] {
		if info.Match(s) {
			return true
		}
	}
	return false
}

// lettered reports whether the regexp re takes a run of letters of any
// length, or does not parse. Such a row matches words the vocabulary does
// not hold: '([A-Zi]*)([SD])UB([A-Zi]*)' is written for 'VOSTSUB' and
// 'MULTISUBS', and matches the group 'SubsPlease' as well.
func lettered(re string) bool {
	tree, err := syntax.Parse(re, syntax.Perl|syntax.FoldCase)
	if err != nil {
		return true
	}
	var letter, free func(re *syntax.Regexp) bool
	// letter is whether a match of re may hold a letter that no literal
	// fixes
	letter = func(re *syntax.Regexp) bool {
		switch re.Op {
		case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
			return true
		case syntax.OpCharClass:
			for i := 0; i+1 < len(re.Rune); i += 2 {
				if lo, hi := re.Rune[i], re.Rune[i+1]; lo <= 'Z' && hi >= 'A' || lo <= 'z' && hi >= 'a' || hi >= utf8.RuneSelf {
					return true
				}
			}
		}
		return slices.ContainsFunc(re.Sub, letter)
	}
	free = func(re *syntax.Regexp) bool {
		switch {
		case re.Op == syntax.OpStar, re.Op == syntax.OpPlus, re.Op == syntax.OpRepeat && re.Max == -1:
			if letter(re.Sub[0]) {
				return true
			}
		}
		return slices.ContainsFunc(re.Sub, free)
	}
	return free(tree)
}

// category reports whether word says what a release is, and so not who made
// it ('[Movies]', '[TV]', '[OVA]', '[Batch]').
func category(word string) bool {
	m, ok := markerOf(word)
	return ok && m.holds != "" || strings.EqualFold(word, "batch")
}

// gap reports whether nothing but delimiters and whitespace stand in
// tags[from:to].
func gap(tags []Tag, from, to int) bool {
	for i := from; i < to; i++ {
		if !tags[i].Is(TagTypeDelim, TagTypeWhitespace) {
			return false
		}
	}
	return true
}

// decimals reads a channel layout between two words as a number with a
// point, which it is in the title of a name the stage reads ('Evangelion
// 3.0+1.0 Thrice Upon a Time'). The channels of a release stand among its
// tags, so only what is in front of the first resolution, source, codec or
// audio tag is looked at, and the word behind may be the next such number.
// It reports whether there is one, and makes each a text only when told to
// write.
func decimals(tags []Tag, write bool) (found bool) {
	for i := 2; i+2 < len(tags) && !tags[i].Is(TagTypeResolution, TagTypeSource, TagTypeCodec, TagTypeAudio); i++ {
		if !tags[i].Is(TagTypeChannels) || !tags[i-1].Is(TagTypeDelim) || !tags[i+1].Is(TagTypeDelim) {
			continue
		}
		before, after := tags[i-1].v[1], tags[i+1].v[1]
		if (tags[i-2].Is(TagTypeText) && strings.Trim(before, " _") == "" || tags[i-2].Was(TagTypeChannels) && before == "+") &&
			(tags[i+2].Is(TagTypeText) && strings.Trim(after, " _") == "" || tags[i+2].Is(TagTypeChannels) && after == "+") {
			if !write {
				return true
			}
			tags[i], found = tags[i].As(TagTypeText, nil), true
		}
	}
	return found
}

// batchSite reads a bracket that holds 'Batch' and that the meta lexer took
// for the site of the release, as it takes the last bracket of a name with
// no group in front of it ('The Sopranos Complete Series 1080p BluRay x265
// [Batch]'). The word stands in a bracket, which is what sets a mark apart
// from a title. It returns a copy of tags with that tag cut into its brackets
// and the batch mark between them, the way the lexers give any other
// bracket, and the place of the tag. The tags it is given are left as they
// are. The bracket that opens a name is left to nameless.
func batchSite(tags []Tag) ([]Tag, int, bool) {
	at := -1
	for i := 1; i < len(tags) && at == -1; i++ {
		if tags[i].typ != TagTypeMeta {
			continue
		}
		if k, v := tags[i].Meta(); k == "site" && strings.EqualFold(v, "batch") && tags[i].v[0] == "["+v+"]" {
			at = i
		}
	}
	if at == -1 {
		return nil, -1, false
	}
	delim := func(d string) Tag { return Tag{typ: TagTypeDelim, v: []string{d, d}} }
	out := make([]Tag, 0, len(tags)+2)
	// the bracket opens in the delimiter in front of it and closes in the
	// one behind it, as one delimiter stands between two tags
	if out = append(out, tags[:at]...); out[at-1].typ == TagTypeDelim {
		out[at-1] = delim(out[at-1].v[0] + "[")
	} else {
		out = append(out, delim("["))
	}
	word := tags[at].v[0]
	out = append(out, Tag{typ: tagTypeUnit, v: []string{word[1 : len(word)-1], unitBatch}, was: TagTypeMeta ^ wasBit})
	rest := tags[at+1:]
	if len(rest) != 0 && rest[0].typ == TagTypeDelim {
		out, rest = append(out, delim("]"+rest[0].v[0])), rest[1:]
	} else {
		out = append(out, delim("]"))
	}
	return append(out, rest...), at, true
}

// stageRoom is the room the stage reads the brackets of a name into. The
// steps of the stage share it through pointers, so it would be made on the
// heap for every name. A parse takes one from the pool and gives it back.
type stageRoom struct {
	in    []uint8
	spans []span
}

var stageRooms = sync.Pool{New: func() any { return &stageRoom{spans: make([]span, 0, 16)} }}

// structure is the structure stage. A name with no bracket is none the stage
// reads, and pays for one pass over its tags. For any other it reads the
// brackets of the name and its shape, lets the owner of the group read the
// group from them, reads the units and the numbers of the head, and keeps
// for the title where it starts.
func (b *TagBuilder) structure(r *Release) {
	r.first = -1
	var bracketed bool
	for _, tag := range r.tags {
		// a tag made by hand may hold no value, and the stage reads the
		// text and the value of every tag of a name it shapes
		if len(tag.v) < 2 {
			return
		}
		switch tag.typ {
		case TagTypeMeta:
			bracketed = true
		case TagTypeDelim:
			bracketed = bracketed || strings.ContainsAny(tag.v[0], "([{")
		}
	}
	if !bracketed {
		return
	}
	room := stageRooms.Get().(*stageRoom)
	room.in, room.spans = b.shape(r, room.in[:0], room.spans[:0])
	stageRooms.Put(room)
}

// shape is the structure stage for a name with a bracket. buf and sbuf are
// the room for its brackets, which it returns as it grew.
func (b *TagBuilder) shape(r *Release, buf []uint8, sbuf []span) ([]uint8, []span) {
	// what the name was given as, for a name the stage does not read
	tags, end, from := r.tags, r.end, r.groupFrom
	// own is whether the tags of r are a copy that the stage may write to
	own := false
	if cut, at, ok := batchSite(r.tags); ok {
		if at < r.end {
			r.end += len(cut) - len(r.tags)
		}
		r.tags, own = cut, true
	}
	in, spans := brackets(r.tags, buf, sbuf)
	if len(spans) == 0 {
		r.tags, r.end = tags, end
		return in, spans
	}
	// a number with a point in the title, and a group in parentheses that
	// opens the name: both give a tag another type, and the brackets are
	// read again behind them
	if s := spans[0]; decimals(r.tags, false) || !s.meta && s.by == '(' && s.role == roleWords {
		if !own {
			r.tags = slices.Clone(r.tags)
		}
		if found, lead := decimals(r.tags, true), b.leadGroup(r, in, spans); found || lead {
			in, spans = brackets(r.tags, in[:0], spans[:0])
		}
	}
	sh, ok := b.shapeOf(r.tags, in, spans)
	if !ok {
		r.tags, r.end, r.groupFrom = tags, end, from
		return in, spans
	}
	// a word the group lexer took moves the end of the head
	if b.handBackMarker(r, func(s string) bool {
		m, ok := markerOf(s)
		return ok && m.holds != ""
	}) {
		if moved, ok := b.shapeOf(r.tags, in, spans); ok {
			sh = moved
		}
	}
	st := stage{tags: r.tags, in: in, spans: spans, sh: sh, over: len(r.tags)}
	st.repairs()
	for i := sh.first + 1; i < sh.head; i++ {
		if r.tags[i].Is(TagTypeDate, TagTypeSeries) {
			st.over = i
			break
		}
	}
	st.contents()
	st.units()
	st.numbers()
	st.join(r)
	// the name of a season in a bracket of its own says the release is that
	// season, when the name numbers no extra and is no one file ('Title (The
	// Final Season) - Part 1 (BD 1920x1080)')
	pack := st.pack || st.season && !sh.sum && !slices.ContainsFunc(r.tags, func(tag Tag) bool {
		_, extra := epKind(tag.val(1))
		return tag.typ == tagTypeUnit && extra && len(tag.v) > 2
	})
	r.shaped, r.first, r.boxed, r.pack, r.single = true, sh.first, sh.boxed, pack, sh.sum
	return in, spans
}

// firstTitle reads the title that starts at tags[start]. It returns the
// title and the number of tags it covers, as title does, and sets Alt when
// the name gives more than one.
func (b *TagBuilder) firstTitle(r *Release, start int) (string, int) {
	if !r.shaped {
		return b.title(r.tags[start:], r.path, TagTypeText)
	}
	return b.titleParts(r, start)
}

// titleParts reads the titles of a name the structure stage shaped, from the
// text at start. It reads as title does, and goes on past what sets one
// title apart from the next, as a fansub name may give its title in two
// languages or two scripts:
//
//   - a slash or a backslash set off by spaces ('Tonari no Totoro / My
//     Neighbor Totoro'), and a bare slash beside a letter of a CJK script
//     ('火影忍者/Naruto', '黑街/匪徒/Gangsta.'). A bare slash is else a part
//     of the title ('In/Spectre', 'Bullet/Bullet', 'Fate/Zero'), and so is
//     one set off by spaces with a word in small letters behind it ('Fate /
//     stay night')
//   - a bar set off by spaces ('Saihate no Paladin | The Faraway Paladin')
//   - a season that ends one title, with the next after a slash ('虚构推理 第
//     二季 / Kyokou Suiri Season 2', 'Youjo Senki S2')
//   - a bracket of words that ends the title and is written in another
//     script or another language than it, see another ('Koujo Denka no
//     Kateikyoushi (Private Tutor to the Duke's Daughter) - SP01'). With
//     more of the title behind it the bracket is a part of the title ('You
//     Can (Not) Redo', 'In This Corner (and Other Corners) of the World'),
//     and so is one word written onto it or in capitals ('Mezzo(DSA)',
//     'Letter Bee (REVERSE)'). Any other bracket of words is left unread: it
//     is a part of the title or a note on it ('Kanteishi (Kari)', '(The
//     Animation)', '(Subtitles)')
//
// The first title is Title and the others are Alt. A number that ends the
// second of two counts for both ('Uchouten Kazoku / The Eccentric Family 2'
// is 'Uchouten Kazoku 2' and 'The Eccentric Family 2'), unless the first
// writes it too or names its season, which the number then writes again
// ('Machikado Mazoku: 2-choume | The Demon Girl Next Door 2', '怕痛的我 第二季
// / Itai no wa Iya 2').
func (b *TagBuilder) titleParts(r *Release, start int) (string, int) {
	tags := r.tags
	var pbuf [4]string
	parts := pbuf[:0]
	var cur strings.Builder
	text := func(i int) bool { return i < len(tags) && tags[i].typ == TagTypeText }
	// seasoned is whether a season ended one of the titles, and bracketed
	// whether one of them stands in a bracket
	var seasoned, bracketed bool
	i := start
loop:
	for i < len(tags) {
		s, n := b.titleOf(tags[i:], r.path, true, TagTypeText)
		if n == 0 {
			break
		}
		cur.WriteString(s)
		i += n
		if i >= len(tags) {
			break
		}
		// a season ends a title, and a slash behind it starts the next
		if j := i + 1; tags[i].Is(TagTypeSeries) && !tags[i].wroteEpisode() && j < len(tags) && tags[j].Is(TagTypeDelim) &&
			strings.Trim(tags[j].v[1], " _") == "/" && text(j+1) && (!r.path || spaced(tags[j].v[1])) {
			parts, seasoned = append(parts, cur.String()), true
			cur.Reset()
			i = j + 1
			continue
		}
		if !tags[i].Is(TagTypeDelim) || i+1 >= len(tags) {
			break
		}
		d := tags[i].v[1]
		switch sep := strings.Trim(d, " _"); {
		case len(d) > 1 && (sep == "/" && (!r.path || spaced(d)) || sep == `\`) && text(i+1):
			// a word in small letters goes on with the title it follows
			// ('Fate / stay night')
			if c := tags[i+1].v[0][0]; c >= 'a' && c <= 'z' {
				cur.WriteString(d)
				i++
				continue
			}
			parts = append(parts, cur.String())
			cur.Reset()
			i++
		case d == "/" && text(i+1):
			// a bare slash beside a letter of a CJK script, where the title
			// reading stopped at it ('Blade Works/命运之夜')
			before, _ := utf8.DecodeLastRuneInString(cur.String())
			after, _ := utf8.DecodeRuneInString(tags[i+1].v[0])
			if !isCJK(before) && !isCJK(after) {
				break loop
			}
			parts = append(parts, cur.String())
			cur.Reset()
			i++
		case d[len(d)-1] == '(' || d[len(d)-1] == '[':
			// in a row of brackets the next bracket of words is another
			// title ('[Group][Title][Other Title][01]')
			row := r.boxed && d == "]["
			if !row && strings.Trim(d[:len(d)-1], " _") != "" || !strings.ContainsFunc(cur.String(), unicode.IsLetter) {
				break loop
			}
			inner, m := b.titleOf(tags[i+1:], r.path, true, TagTypeText)
			j := i + 1 + m
			if m == 0 || j >= len(tags) || !tags[j].Is(TagTypeDelim) || inner == "" || strings.Trim(inner, "0123456789-") == "" {
				break loop
			}
			e := tags[j].v[1]
			if e[0] != ')' && e[0] != ']' {
				break loop
			}
			after := strings.Trim(e[1:], " _")
			// more of the title follows: a word, and after a hyphen only
			// when the bracket holds one word ('(REVERSE) - Letter Bee').
			// Behind a colon the bracket holds what the word in front of
			// it names, and is no part of a title. The lines of one
			// announcer are written so ('tehFIRE: [TV|XviD]
			// Royal.Institution')
			more := !row && !strings.HasSuffix(cur.String(), ":") && text(j+1) && (after == "" || after == "-" && m == 1) &&
				wordStart(tags[j+1].v[0]) && !afterTitle(tags[j+1].v[0])
			// behind more than one word the title goes on with a small word
			// ('(and Other Corners) of the World'), and a word with a
			// capital starts something else ('(Other Name) Season S1')
			if c := tags[min(j+1, len(tags)-1)].v[0]; more && m > 1 && d[len(d)-1] == '(' && c != "" && c[0] >= 'A' && c[0] <= 'Z' {
				more = false
			}
			// one word in parentheses is a part of the title when it is
			// written onto it or in capitals, and another name for the show
			// otherwise ('(Jigokuraku)')
			one := !row && m == 1 && d[len(d)-1] == '(' && !isChecksum(inner) && (len(d) == 1 || capitals(inner) && strings.ToUpper(inner) == inner)
			// a slash behind the bracket starts the next title ('尼古喵喵 (邪
			// 竜解放版) / Yani Neko')
			next := !row && (!r.path || spaced(e[1:])) && after == "/" && text(j+1)
			if !more && !one {
				// in square brackets a word behind the title is a group or
				// a note ('Title [HorribleSubs] (1080p)')
				if !row && d[len(d)-1] != '(' {
					break loop
				}
				// in the tongue of the title it is a part of the title or a
				// note on it, which nothing tells apart
				other := row || another(cur.String(), inner, m == 1)
				switch first, _, _ := strings.Cut(inner, " "); {
				case afterTitle(first) && !row:
					// a note on what the release holds ('(Collection of 8)')
					break loop
				case row && slices.ContainsFunc(tags[i+1:j], func(tag Tag) bool { return tag.Is(TagTypeDelim) && strings.Contains(tag.v[1], "+") }):
					// a list of what the release holds ('[正片+特典映像]')
					break loop
				case other && ((m > 1 || row || inner[0] >= 0x80) && titleLike(inner) || m == 1 && len(inner) >= 4 && !isChecksum(inner) && wordStart(inner) && strings.ToUpper(inner) != inner):
					parts, bracketed = append(parts, cur.String(), inner), true
					cur.Reset()
				case next:
					// a note on the title, which is no part of it
					parts = append(parts, cur.String())
					cur.Reset()
					for k := i + 1; k < j; k++ {
						if tags[k].Is(TagTypeText) {
							r.unused = append(r.unused, k)
						}
					}
				default:
					break loop
				}
				if i = j; row {
					continue
				}
				if next {
					i = j + 1
					continue
				}
				break loop
			}
			if len(d) > 1 {
				cur.WriteByte(' ')
			}
			cur.WriteByte(d[len(d)-1])
			cur.WriteString(inner)
			cur.WriteByte(e[0])
			if next {
				parts = append(parts, cur.String())
				cur.Reset()
				i = j + 1
				continue
			}
			if !more {
				i = j
				break loop
			}
			if after == "-" {
				cur.WriteString(" - ")
			} else if len(e) > 1 {
				cur.WriteByte(' ')
			}
			i = j + 1
		default:
			break loop
		}
	}
	if cur.Len() != 0 {
		parts = append(parts, cur.String())
	}
	var tbuf [4]string
	titles := tbuf[:0]
	for _, p := range parts {
		for s := range strings.SplitSeq(p, " | ") {
			k := len(titles)
			titles = cutScripts(titles, s)
			for n := k; n < len(titles); n++ {
				titles[n] = trimTitle(titles[n])
			}
			titles = append(titles[:k], slices.DeleteFunc(titles[k:], func(s string) bool { return s == "" || s == "|" })...)
		}
	}
	if len(titles) == 0 {
		return b.title(tags[start:], r.path, TagTypeText)
	}
	// the number of a sequel that ends the last title is the number of them
	// all
	if last := titles[len(titles)-1]; len(titles) == 2 && !seasoned && !bracketed {
		// a number that a word counts is no sequel ('Eighty Six Part 1')
		prev := last[:max(strings.LastIndexByte(last, ' '), 0)]
		prev = prev[strings.LastIndexByte(prev, ' ')+1:]
		if k := strings.LastIndexByte(last, ' '); k != -1 && len(last)-k-1 <= 2 && last[k+1] != '0' && strings.Trim(last[k+1:], "0123456789") == "" && !counts(prev) {
			for n, s := range titles[:len(titles)-1] {
				// not a title that writes the number or its season itself
				// ('Machikado Mazoku: 2-choume | The Demon Girl Next Door 2')
				if !strings.Contains(s, last[k+1:]) && !strings.Contains(s, "第") {
					titles[n] = s + last[k:]
				}
			}
		}
	}
	// a number of five digits or more behind a spaced hyphen that ends a
	// title numbers the release, whatever it counts: the names of one
	// uploader have it ('[NC-Raws] 世界末日与柴犬同行 / Sekai no Owari ni Shiba
	// Inu to - 2023010362')
	for n, t := range titles {
		if k := strings.LastIndex(t, " - "); k > 0 && len(t)-k-3 >= 5 && allDigits(t[k+3:], 12) {
			titles[n] = t[:k]
		}
	}
	if k := i; k < len(tags) {
		// what stands behind the titles
		for k < len(tags)-1 && tags[k].Is(TagTypeDelim) {
			k++
		}
		// a title written twice, of which a lexer took the last word the
		// second time: 'Mix' in '[NC-Raws] D4DJ All Mix / D4DJ All Mix -
		// 03', the one show that has it
		if last := len(titles) - 1; last > 0 && !tags[k].Is(TagTypeDelim) && slices.ContainsFunc(titles[:last], func(t string) bool {
			return strings.EqualFold(t, titles[last]+" "+tags[k].v[0])
		}) {
			titles = titles[:last]
		}
		// a title that ends where a unit starts ends with no joiner ('Title:
		// Vol. 1' is 'Title')
		if last := len(titles) - 1; tags[k].typ == tagTypeUnit {
			// a plus or an ampersand joins the two when it is written onto
			// the unit ('Title+OVA'), and is a part of the title with a
			// space behind it ('Strange+ Complete Series')
			joiners := " :;,~-–—"
			if k > 0 && tags[k-1].Is(TagTypeDelim) && strings.ContainsAny(tags[k-1].v[1][len(tags[k-1].v[1])-1:], "+&/") {
				joiners += "+&/"
			}
			if t := strings.TrimRight(titles[last], joiners); t != "" {
				titles[last] = t
			}
		}
	}
	// each other title once. A name may be all titles, so they are not
	// compared one with each other
	if alt := unique(titles); len(alt) > 1 {
		r.Alt = kept(alt[1:])
	}
	return titles[0], i - start
}

// The scripts the stage tells apart in the words of a title.
const (
	// scriptNone is words with no letter.
	scriptNone uint8 = iota
	scriptLatin
	scriptCJK
	// scriptOther is any other script ('Дара из Рэйвы').
	scriptOther
)

// tongue is what the words of a title say about the language they are in.
type tongue struct {
	script uint8
	// particle is whether one of the words is a particle of Japanese ('no',
	// 'ni', 'wa'), and small whether one is a small word of English ('the',
	// 'of', 'in'). 'to', 'de' and 'made' are words of both and count for
	// neither.
	particle, small bool
	// total is the number of words in Latin letters, and romaji how many of
	// them read as Japanese, see romaji.
	romaji, total int
}

// tongueOf reads the tongue of the words of s. A word is a run of two
// letters or more.
func tongueOf(s string) tongue {
	var t tongue
	for _, c := range s {
		switch {
		case isCJK(c):
			return tongue{script: scriptCJK}
		case c >= 0x370 && unicode.IsLetter(c):
			t.script = scriptOther
		}
	}
	if t.script != scriptNone {
		return t
	}
	var buf [24]byte
	for i := 0; i < len(s); {
		j := i
		for j < len(s) {
			c, n := rune(s[j]), 1
			if c >= 0x80 {
				c, n = utf8.DecodeRuneInString(s[j:])
			}
			if !unicode.IsLetter(c) {
				break
			}
			j += n
		}
		if w := s[i:j]; len(w) >= 2 && len(w) <= len(buf) {
			word := string(asciiLower(buf[:0], w))
			t.total++
			switch word {
			case "no", "ni", "wa", "ga", "wo", "mo", "na", "ja", "desu", "kara", "nai", "suru":
				t.particle = true
			case "the", "of", "and", "is", "are", "my", "your", "you", "with", "for", "from", "this", "that", "in", "on", "at", "as", "by", "an", "it", "be", "we", "our":
				t.small = true
			}
			if romaji(word) {
				t.romaji++
			}
		} else if len(w) > len(buf) {
			t.total++
		}
		// past the byte that ended the word, which starts no rune inside
		// one: the bytes behind it are no letters and are passed one by one
		i = j + 1
	}
	if t.total != 0 {
		t.script = scriptLatin
	}
	return t
}

// romaji reports whether w, a word in small letters, reads as Japanese in
// Latin letters: syllables of a vowel with one consonant or none in front of
// it ('ka', 'shi', 'kyo'), an 'n' that ends a syllable, and a consonant
// written twice ('kk', 'tch'). A letter outside ASCII is a vowel with its
// mark ('ō'). No such word has an 'l', a 'q', a 'v' or an 'x', or ends in a
// consonant other than 'n'. A word of English may read so too ('titan',
// 'game'), which is why another compares shares and does not count one
// word.
func romaji(w string) bool {
	vowel := func(c byte) bool { return c == 'a' || c == 'i' || c == 'u' || c == 'e' || c == 'o' || c >= 0x80 }
	for i := 0; i < len(w); {
		c := w[i]
		if vowel(c) {
			i++
			continue
		}
		next := byte(0)
		if i+1 < len(w) {
			next = w[i+1]
		}
		// an 'n' or an 'm' that ends its syllable ('shinbun', 'shimbun')
		if c == 'n' && !vowel(next) && next != 'y' || c == 'm' && (next == 'b' || next == 'p') {
			i++
			continue
		}
		// a consonant written twice
		if next == c || c == 't' && next == 'c' {
			i++
			c = next
		}
		// the consonant of the syllable, one letter or two
		switch {
		case i+1 < len(w) && (c == 's' && w[i+1] == 'h' || c == 'c' && w[i+1] == 'h' || c == 't' && w[i+1] == 's'):
			i += 2
		case strings.IndexByte("kgsztdnhbpmyrwfj", c) != -1:
			if i++; i < len(w) && w[i] == 'y' && c != 'y' {
				i++
			}
		default:
			return false
		}
		if i >= len(w) || !vowel(w[i]) {
			return false
		}
		i++
	}
	return w != ""
}

// another reports whether inner, the words of a bracket behind title, is
// another title of the release, and no part of the title or note on it. Only
// the name can say so, and it does when the two are written in another
// script ('Title (進撃の巨人)') or in another language. Of those the stage
// tells Japanese in Latin letters from any other, by comparing the two, see
// tongue:
//
//   - one has a particle of Japanese and the other has none, and has a small
//     word of English or a smaller share of Japanese words ('Shingeki no
//     Kyojin (Attack on Titan)', 'Kuro no Shoukanshi (Black Summoner)')
//   - one has a small word of English and no particle, and the other has
//     neither and a larger share of Japanese words ('Mushoku Tensei (The
//     Jobless Reincarnation)')
//   - neither has such a word, and the shares differ by three words in ten
//     ('Senran Kagura (Ninja Flash)')
//
// In the tongue of the title the words are a part of it or a note, which
// nothing tells apart ('Kanteishi (Kari)', 'Muri Muri! (Muri ja Nakatta)',
// 'Trails in the Sky (The Animation)', 'One Piece (26 Half Hours)'), and so
// is a label, see label. one is whether the bracket holds one word, which in
// Latin letters is another title only as the Japanese name of a title in
// another language ('Hell's Paradise (Jigokuraku)'): one word of English is
// as often a note ('Naruto x UT (Uniqlo)', 'Nekomonogatari (Black)').
//
// Two limits are known and stay. A subtitle in English behind a Japanese
// title is read as another title, as nothing in the name tells it from a
// translated one ('Kimetsu no Yaiba (Hashira Training)', 'Haikyuu (To the
// Top)'). And a pair in two other languages that Latin letters write is not
// read: Korean, Spanish, French or German beside English is one title with
// its bracket.
func another(title, inner string, one bool) bool {
	a, b := tongueOf(title), tongueOf(inner)
	if a.script == scriptNone || b.script == scriptNone || label(inner) {
		return false
	}
	if a.script != scriptLatin || b.script != scriptLatin {
		return a.script != b.script
	}
	if one {
		return b.romaji == b.total && !a.particle && a.romaji < a.total
	}
	// less is whether x has a smaller share of Japanese words than y
	less := func(x, y tongue) bool { return x.romaji*y.total < y.romaji*x.total }
	japanese := func(x, y tongue) bool { return x.particle && !y.particle && (y.small || less(y, x)) }
	english := func(x, y tongue) bool { return y.small && !y.particle && !x.small && less(y, x) }
	switch {
	case japanese(a, b), japanese(b, a), english(a, b), english(b, a):
		return true
	case a.particle, a.small, b.particle, b.small:
		return false
	}
	d := a.romaji*b.total - b.romaji*a.total
	return max(d, -d)*10 >= 3*a.total*b.total
}

// label reports whether the words of s name a season, a part or an arc of a
// show, or say only that it is the film ('The Final Season', 'Egghead Arc',
// 'The Movie'): a label on the title, and no other title.
func label(s string) bool {
	only := true
	for w := range strings.FieldsSeq(s) {
		switch {
		case strings.EqualFold(w, "season"), strings.EqualFold(w, "part"), strings.EqualFold(w, "cour"), strings.EqualFold(w, "arc"):
			return true
		case strings.EqualFold(w, "the"), strings.EqualFold(w, "a"), strings.EqualFold(w, "movie"), strings.EqualFold(w, "final"):
		default:
			only = false
		}
	}
	return only
}

// spaced reports whether a space or an underscore stands on both sides of
// what the delimiter d holds, as around the slash between two titles ('A /
// B'). The slash of a path has none ('Show/Show.S01E06.mkv'), so such a slash
// sets two titles apart in the name of a file too.
func spaced(d string) bool {
	return len(d) >= 3 && strings.IndexByte(" _", d[0]) != -1 && strings.IndexByte(" _", d[len(d)-1]) != -1
}

// cutScripts cuts s at every bare slash that sets a title in a CJK script
// apart from the next: one with a letter of such a script right beside it
// ('火影忍者 疾风传/Naruto Shippuuden', '黑街/匪徒/Gangsta.'), or with such a
// letter anywhere on one side of it and none on the other ('魔法少女奈叶剧场版
// 1st/Mahou Shoujo Lyrical Nanoha The Movie 1st'). It appends the parts to
// titles. A slash between two parts in Latin letters is a part of a title
// ('Fate/stay night').
func cutScripts(titles []string, s string) []string {
	// most titles have no slash, and are not read for their script
	if strings.IndexByte(s, '/') == -1 {
		return append(titles, s)
	}
	// from is where the title at hand starts, and wide whether it holds a
	// letter of a CJK script so far
	from, wide := 0, false
	for i := 0; i < len(s); {
		// the part up to the next slash
		j := len(s)
		if k := strings.IndexByte(s[i:], '/'); k != -1 {
			j = i + k
		}
		part := strings.ContainsFunc(s[i:j], isCJK)
		if i > from && i-1 > 0 && j > i {
			before, _ := utf8.DecodeLastRuneInString(s[:i-1])
			after, _ := utf8.DecodeRuneInString(s[i:])
			if isCJK(before) || isCJK(after) || wide != part {
				titles, from, wide = append(titles, s[from:i-1]), i, false
			}
		}
		wide = wide || part
		i = j + 1
	}
	return append(titles, s[from:])
}

// titleLike reports whether what a bracket holds reads as a title and not as
// a row of codes ('LD 640x480 WMV9'): it has a letter outside ASCII, or two
// words with a small letter.
func titleLike(s string) bool {
	if isChecksum(s) || !wordStart(s) {
		return false
	}
	var words int
	for w := range strings.FieldsSeq(s) {
		for _, c := range w {
			if c >= 0x80 && unicode.IsLetter(c) {
				return true
			}
			if c >= 'a' && c <= 'z' {
				words++
				break
			}
		}
	}
	return words >= 2
}

// wordStart reports whether s starts the way a word of a title does, and not
// with the punctuation that sets a second title apart (':', '|', '~').
func wordStart(s string) bool {
	for _, c := range s {
		return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '!' || c == '?'
	}
	return false
}

// afterTitle reports whether s is a word that follows a title and is no part
// of it, which matters behind a bracket of words inside one ('Title (Other
// Name) Season 2').
func afterTitle(s string) bool {
	switch strings.ToLower(s) {
	case "season", "seasons", "part", "cour", "vol", "volume", "episode", "episodes", "ep", "batch", "complete", "trailer", "collection", "includes", "contains":
		return true
	}
	return false
}

// trimTitle trims a title as title does, and keeps a bracket that closes one
// the title opened ('Mezzo(DSA)').
func trimTitle(s string) string {
	s = strings.TrimLeftFunc(s, isTitleTrimDelim)
	for len(s) > 0 {
		c := s[len(s)-1]
		if c >= 0x80 || !isTitleTrimDelim(rune(c)) {
			break
		}
		if c == ')' && strings.Count(s, "(") == strings.Count(s, ")") || c == ']' && strings.Count(s, "[") == strings.Count(s, "]") {
			break
		}
		s = s[:len(s)-1]
	}
	// the plus or the ampersand of a list that the stage read ('Title +
	// OVA', 'Title NCOP & NCED')
	for _, sep := range [...]string{" +", " &"} {
		if t, ok := strings.CutSuffix(s, sep); ok {
			return trimTitle(t)
		}
	}
	return s
}

// unitTitle reads the title of an extra, which follows the word that says
// what the extra is as the title of an episode follows its number: the
// 'Starry Wish' of 'Vivid Strike - ED1 - Starry Wish (Inori Minase)' and the
// 'Valentine Days' of 'School Days OVA - Valentine Days'. pos is where the
// title of the release ends. It sets Subtitle and returns the position
// behind it, or pos when the name has no such title: a bracket behind the
// word holds tags or another name.
func (b *TagBuilder) unitTitle(r *Release, pos int) int {
	if !r.shaped || r.Subtitle != "" {
		return pos
	}
	i, units := pos, false
	for ; i < len(r.tags); i++ {
		switch tag := r.tags[i]; {
		case tag.typ == tagTypeUnit:
			units = true
			continue
		case tag.Is(TagTypeVersion):
			continue
		// the year between the title and the word, in parentheses or not
		// ('Title 2023 Special 1 The Star Beast')
		case tag.Is(TagTypeDate) && !units:
			continue
		case tag.Is(TagTypeDelim) && !units && strings.Trim(tag.v[1], " _()") == "" &&
			(i+1 < len(r.tags) && r.tags[i+1].Is(TagTypeDate) || i > 0 && r.tags[i-1].Is(TagTypeDate)):
			continue
		case tag.Is(TagTypeDelim) && !strings.ContainsAny(tag.v[1], "()[]{}+&|/"):
			continue
		}
		break
	}
	if !units || i >= len(r.tags) || !r.tags[i].Is(TagTypeText) {
		return pos
	}
	s, n := b.titleOf(r.tags[i:], r.path, true, TagTypeText)
	if !strings.ContainsFunc(s, unicode.IsLetter) {
		return pos
	}
	r.Subtitle = s
	return i + n
}

// skipped collects the words in front of the title of a shaped name as
// unused, and returns where the title starts: a bracket of words between the
// group and the title names another group or says something of the release
// ('[Group] [Vivid] Title', '[Group]_[m.3.3.w]_Title').
func (b *TagBuilder) skipped(r *Release, pos int) int {
	if !r.shaped || r.first <= pos || r.first >= len(r.tags) || !r.tags[r.first].Is(TagTypeText) {
		return pos
	}
	for i := pos; i < r.first; i++ {
		if r.tags[i].Is(TagTypeText) {
			r.unused = append(r.unused, i)
		}
	}
	return r.first
}

// allDigits reports whether s is one to max ASCII digits.
func allDigits(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// asciiLower appends s to buf with its ASCII capitals as small letters.
func asciiLower(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c |= 0x20
		}
		buf = append(buf, c)
	}
	return buf
}

// counts reports whether s is a word that counts the number behind it, so
// that the number is no episode ('Part 2', 'Movie 3', 'No. 6', 'Collection
// 1-11').
func counts(s string) bool {
	if len(s) < 2 || len(s) > 10 {
		return false
	}
	var buf [10]byte
	switch string(asciiLower(buf[:0], strings.TrimSuffix(s, "."))) {
	case "movie", "movies", "part", "pt", "season", "no", "chapter", "ch", "act", "phase", "stage", "round", "type", "mk", "ver",
		"version", "top", "cour", "film", "gekijouban", "volume", "vol", "disc", "cd", "the", "of", "and", "x", "vs", "lap", "day", "code", "set",
		"collection":
		return true
	}
	return false
}

// tagTypeUnit is the type of a word that says what a release holds and of
// the number that counts it: the 'OVA' and the '02' of 'Title - OVA 02', the
// 'SP01' of 'Title - SP01', the 'Vol' and the '06' of 'Title - Vol.06', the
// 'Batch' of '(Batch)', the 'Complete Series' of 'Title - Complete Series'.
// Only the structure stage makes one, and Release.numbering reads them into
// Numbering. Its values are the text, the kind (see the unit kinds) and what
// the kind counts or names: the number of an extra or of a volume, with a '-'
// and the end of a range behind it or an '&' and the next number of a list,
// or the name of what a pack holds.
//
// It is declared with no type of its own so that stringer leaves it out: it
// is no part of the exported tag types. Tag.Format prints it as 'Unit', with
// the kind and the value, ie '<Unit:ova 02>', '<Unit:special 01 - 03>',
// '<Unit:volume 06>', '<Unit:contents Complete Series>' and '<Unit:batch>'.
const tagTypeUnit = TagTypeFrameRate + 1

// The kinds of a unit tag.
const (
	// unitSpecial, unitOVA, unitOP, unitED and unitPV are an extra of that
	// kind. With a number the tag is an episode of the kind, and without
	// one it is the marker in front of the number ('OVA 02').
	unitSpecial = "special"
	unitOVA     = "ova"
	unitOP      = "op"
	unitED      = "ed"
	unitPV      = "pv"
	// unitVolume is a volume or a box, with its number or as the marker in
	// front of it.
	unitVolume = "volume"
	// unitHolds is what a pack holds, by its name in Numbering.Contents.
	unitHolds = "contents"
	// unitBatch is a word that says the release is a batch.
	unitBatch = "batch"
	// unitNote is a word that belongs to one of the others and says nothing
	// more: the 'The' of 'The Complete Series', the 'of 24' of '01 of 24'.
	unitNote = "note"
)

// unit returns tag as a unit tag of the kind with the values.
func unit(tag Tag, kind string, vals ...string) Tag {
	v := make([]string, 0, 2+len(vals))
	v = append(append(v, tag.v[0], kind), vals...)
	return Tag{typ: tagTypeUnit, v: v, was: tag.typ ^ wasBit, prevf: tag.f}
}

// episode returns tag as the series tag of one episode written with no
// season.
func episode(tag Tag, num string) Tag {
	return Tag{typ: TagTypeSeries, v: []string{tag.v[0], "", num}, was: tag.typ ^ wasBit, prevf: tag.f}
}

// epKind returns the kind of episode a unit of the kind is, and whether it
// is one.
func epKind(kind string) (EpKind, bool) {
	switch kind {
	case unitSpecial:
		return EpSpecial, true
	case unitOVA:
		return EpOVA, true
	case unitOP:
		return EpOP, true
	case unitED:
		return EpED, true
	case unitPV:
		return EpPV, true
	}
	return EpRegular, false
}

// marker is a word that says what a release holds.
type marker struct {
	// kind is the kind of unit the word marks when a number counts it, or
	// the empty string for a word that no number counts ('Movie', 'TV').
	kind string
	// holds is its name in Numbering.Contents, when it is written with no
	// number.
	holds string
	// apart is whether the number may stand apart from the word ('OVA 02',
	// 'Vol.06'). The others count only written onto it ('SP01', 'ED2').
	apart bool
	// alone is whether the word marks what the release is with no number
	// and nothing to set it apart ('Title OVA [1080p]'). The others do
	// behind a hyphen ('Title - Movie [1080p]') or in a list.
	alone bool
}

// markers are the words that say what a release holds, in small letters.
// The names they give are the spellings of Numbering.Contents: 'TV', 'OVA',
// 'OAD', 'ONA', 'Movie', 'Special', 'OP', 'ED', 'PV', 'Extra' and 'OST',
// beside 'Complete Series', which contents reads. It is a table so that the
// tests of the stage can walk every marker, see TestStructureLinear.
var markers = map[string]marker{
	"ova":  {unitOVA, "OVA", true, true},
	"oav":  {unitOVA, "OVA", true, true},
	"oad":  {unitOVA, "OAD", true, true},
	"ona":  {unitOVA, "ONA", true, true},
	"ovas": {"", "OVA", false, true},
	"oavs": {"", "OVA", false, true},
	"oads": {"", "OAD", false, true},
	"onas": {"", "ONA", false, true},
	"sp":   {unitSpecial, "Special", false, false},
	// an extra episode ('Accel World - EX01')
	"ex":       {unitSpecial, "Special", false, false},
	"special":  {unitSpecial, "Special", true, false},
	"specials": {unitSpecial, "Special", true, false},
	"sps":      {"", "Special", false, false},
	"ncop":     {unitOP, "OP", true, true},
	"nced":     {unitED, "ED", true, true},
	"op":       {unitOP, "OP", false, false},
	"ed":       {unitED, "ED", false, false},
	"pv":       {unitPV, "PV", false, false},
	"pvs":      {"", "PV", false, false},
	"vol":      {unitVolume, "", true, false},
	"volume":   {unitVolume, "", true, false},
	"box":      {unitVolume, "", true, false},
	"movie":    {"", "Movie", false, false},
	"movies":   {"", "Movie", false, false},
	"tv":       {"", "TV", false, false},
	"extra":    {"", "Extra", false, false},
	"extras":   {"", "Extra", false, false},
	"ost":      {"", "OST", false, false},
}

// counted returns the kind of unit that a number behind the marker counts,
// or the empty string: its kind, and for the plural of an extra, which has
// none as no number is written onto it, the kind of the extra ('OVAs').
func (m marker) counted() string {
	if m.kind != "" {
		return m.kind
	}
	switch m.holds {
	case "OVA", "OAD", "ONA":
		return unitOVA
	case "Special":
		return unitSpecial
	case "PV":
		return unitPV
	}
	return ""
}

// markerLead are the bytes a marker begins with, in either case, read off
// markers: most words of a name begin with none of them, and are not looked
// up.
var markerLead = func() (lead [256]bool) {
	for w := range markers {
		lead[w[0]], lead[w[0]&^0x20] = true, true
	}
	return lead
}()

// markerOf returns the marker that s is, and whether it is one, see markers.
func markerOf(s string) (marker, bool) {
	if len(s) < 2 || len(s) > 8 || !markerLead[s[0]] {
		return marker{}, false
	}
	var buf [8]byte
	m, ok := markers[string(asciiLower(buf[:0], s))]
	return m, ok
}

// fused reads a marker written onto its number: 'SP01', 'ED2', 'NCOP2',
// 'OVA01', 'OP4a', 'NCEDv2', 'SP1&3'. It returns the marker, the number, and
// the next number of a list the word gives, which the '3' of 'SP1&3' is.
func fused(s string) (m marker, num, next string, ok bool) {
	// a version behind the number, or behind the marker with no number
	var version bool
	if k := len(s); k > 3 && s[k-2]|0x20 == 'v' && s[k-1] >= '0' && s[k-1] <= '9' {
		s, version = s[:k-2], true
	}
	i := 0
	for i < len(s) && s[i]|0x20 >= 'a' && s[i]|0x20 <= 'z' {
		i++
	}
	if i < 2 || i > 4 || i == len(s) && !version {
		return m, "", "", false
	}
	if m, ok = markerOf(s[:i]); !ok || m.kind == "" {
		return m, "", "", false
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	switch rest := s[j:]; {
	case j-i > 3, j == i && (rest != "" || m.kind == unitVolume):
		return m, "", "", false
	case rest == "":
	case len(rest) == 1 && rest[0]|0x20 >= 'a' && rest[0]|0x20 <= 'd' && !version:
	case len(rest) == 2 && rest[0] == '&' && rest[1] >= '0' && rest[1] <= '9' && s[i:j] < rest[1:] && !version:
		next = rest[1:]
	default:
		return m, "", "", false
	}
	return m, s[i:j], next, true
}

// plain reports whether tag is a delimiter of spaces, underscores and dots
// alone.
func plain(tag Tag) bool {
	return tag.typ == TagTypeDelim && strings.Trim(tag.v[1], " _.") == ""
}

// stage is what the steps of the structure stage share.
type stage struct {
	tags []Tag
	// in is the kind of bracket each tag stands in, and spans the brackets,
	// see brackets.
	in    []uint8
	spans []span
	sh    shape
	// merges are the runs of tags to join into one, which is done last as
	// it moves the tags behind them.
	merges []merge
	// pack is whether the name lists what a pack holds, and season whether
	// a bracket holds the name of a season and nothing else ('(The Final
	// Season)').
	pack, season bool
	// over is where the title is over for sure: at the year, the season or
	// the episode that the head writes first, or the end of the tags.
	over int
}

// merge is a run of tags to join into one tag.
type merge struct {
	from, to int
	typ      TagType
	// vals are the values of the tag behind its text.
	vals []string
}

// prev and next return the tag in front of and behind the one at i that is
// no delimiter, inside [from, to), or -1.
func (st *stage) prev(i, from int) int {
	for i--; i >= from; i-- {
		if st.tags[i].typ != TagTypeDelim && st.tags[i].typ != TagTypeWhitespace {
			return i
		}
	}
	return -1
}

func (st *stage) next(i, to int) int {
	for i++; i < to; i++ {
		if st.tags[i].typ != TagTypeDelim && st.tags[i].typ != TagTypeWhitespace {
			return i
		}
	}
	return -1
}

// between returns the delimiters between the tags at i and j as one string.
func (st *stage) between(i, j int) string {
	switch {
	case j <= i+1:
		return ""
	case j == i+2:
		return st.tags[i+1].val(1)
	}
	var sb strings.Builder
	for k := i + 1; k < j; k++ {
		sb.WriteString(st.tags[k].val(1))
	}
	return sb.String()
}

// word reports whether the tag at i is a word of a title, which a word the
// lexers took for a tag still is at this point ('Bokura Ga Ita 01': 'Ita'
// reads as a language).
func (st *stage) word(i int) bool {
	if i < 0 {
		return false
	}
	switch tag := st.tags[i]; tag.typ {
	case TagTypeText:
		return !allDigits(tag.v[0], 4)
	case TagTypeLanguage, TagTypeOther, TagTypeCut, TagTypeEdition, TagTypeCollection, TagTypeRegion, TagTypeGenre, TagTypePlatform, TagTypeArch:
		return true
	}
	return false
}

// bare reports whether tag is one episode that a lexer read from a bare
// number ('- 06').
func bare(tag Tag) bool {
	return tag.typ == TagTypeSeries && len(tag.v) == 3 && tag.v[1] == "" && tag.v[2] != "" && tag.v[0] != "" && tag.v[0][0] >= '0' && tag.v[0][0] <= '9'
}

// ranged reports whether tag is a range of bare numbers that a lexer read
// ('01-12', '01 ~ 12'), and returns its two ends.
func ranged(tag Tag) (string, string, bool) {
	if tag.typ != TagTypeSeries || len(tag.v) != 6 || tag.v[1] != "" || tag.v[2] != "" || tag.v[4] != "-" {
		return "", "", false
	}
	return tag.v[3], tag.v[5], true
}

// repairs reads what the order of the lexers hides. 'EP' is the format of a
// record to the source lexer, which runs in front of the series lexer, so
// the 'EP 13' of 'Title 2nd EP 13 (BD 720p)' is read here: the word is a
// note and the number the episode, with a higher number behind a hyphen as
// the end of a range ('(EP 1 - 52)'). 'Episodes 1-12' and 'Eps 01-12' are
// read the same way.
func (st *stage) repairs() {
	tags := st.tags
	for i := 1; i < len(tags); i++ {
		tag := tags[i]
		var mark bool
		switch tag.typ {
		case TagTypeSource:
			mark = strings.EqualFold(tag.v[0], "ep")
		case TagTypeText:
			mark = strings.EqualFold(tag.v[0], "episodes") || strings.EqualFold(tag.v[0], "eps")
		}
		if !mark || i+2 >= len(tags) || !plain(tags[i+1]) || !tags[i+2].Is(TagTypeText) || !allDigits(tags[i+2].v[0], 4) {
			continue
		}
		first := tags[i+2].v[0]
		tags[i] = unit(tag, unitNote)
		// the end of a range, which the episode lexer read behind its hyphen
		if k := i + 4; k < len(tags) && tags[i+3].Is(TagTypeDelim) && strings.Trim(tags[i+3].v[1], " _") == "-" {
			last := tags[k].v[0]
			if bare(tags[k]) {
				last = tags[k].v[2]
			}
			if (tags[k].Is(TagTypeText) || bare(tags[k])) && allDigits(last, 4) && atoi(first) < atoi(last) {
				st.merges = append(st.merges, merge{i + 2, k + 1, TagTypeSeries, []string{"", "", first, "-", last}})
				i = k
				continue
			}
		}
		tags[i+2] = episode(tags[i+2], first)
		i += 2
	}
}

// holds reports whether the tag at i is a word that says what a pack holds,
// and returns its name in Numbering.Contents. A word that is a note beside
// one has no name ('Series', 'Collection').
func (st *stage) holds(i int) (string, bool) {
	tag := st.tags[i]
	switch tag.typ {
	case TagTypeText:
		if m, ok := markerOf(tag.v[0]); ok && m.holds != "" {
			return m.holds, true
		}
		switch {
		case strings.EqualFold(tag.v[0], "series"), strings.EqualFold(tag.v[0], "collection"), strings.EqualFold(tag.v[0], "anime"):
			return "", true
		case strings.EqualFold(tag.v[0], "batch"):
			return unitBatch, true
		}
	case TagTypeOther:
		if strings.EqualFold(tag.v[0], "complete") {
			return "Complete Series", true
		}
		return "OST", strings.EqualFold(tag.v[0], "ost")
	case TagTypeSource, TagTypeAudio, TagTypeGenre:
		// a soundtrack in a list is one of the things the pack holds, and
		// not the format of a record. So is 'SP', which the source lexer
		// reads as the format of a single
		switch {
		case strings.EqualFold(tag.v[0], "ost"):
			return "OST", true
		case strings.EqualFold(tag.v[0], "sp"):
			return "Special", true
		}
	case tagTypeUnit:
		return "", tag.v[1] == unitHolds || tag.v[1] == unitBatch
	}
	return "", false
}

// claim makes the tag at i a unit that says the pack holds what name names.
func (st *stage) claim(i int, name string) {
	switch tag := st.tags[i]; {
	case tag.typ == tagTypeUnit:
	case name == unitBatch:
		st.tags[i] = unit(tag, unitBatch)
	case name == "":
		st.tags[i] = unit(tag, unitNote)
	default:
		st.tags[i] = unit(tag, unitHolds, name)
	}
}

// contents reads what a pack says it holds, and whether a name says it is a
// batch.
//
// A list joined by a plus is read from its first plus on, and back from it
// over the words that name contents ('TV+OVA+Movie', '(Complete
// Series+OVAs)', 'Title + OVA', 'S1 01-13 + SP01-03', where the numbered
// extra is left to units). With no plus, 'Complete Series' is read at the end
// of the title and in a bracket ('Title - The Complete Series', '(Complete
// Series)'), and 'Complete Collection' there in a name that a fansub group
// wrote, as a pack with no contents named. 'Batch' is read in a bracket where
// no word follows it ('(Batch)', '[MP4 Batch]', '(unofficial batch)', and not
// '(Batch Release)'), and at the end of the title where the name sets it
// apart ('Title - 01 - 06 - Batch', and not 'The Bad Batch').
//
// Nothing is read in front of the year that ends the title, see shapeOf,
// but what stands in a bracket there ('Title (Movie) (2022)').
func (st *stage) contents() {
	tags, sh := st.tags, st.sh
	// a season in a bracket of words that lists names with a plus is a part
	// of one of the names ('(Tokyo Ghoul+√A+re+re 2nd Season+Jack+Pinto)')
	for _, sp := range st.spans {
		if sp.meta || sp.role != roleWords || sp.open <= sh.first {
			continue
		}
		var plus int
		// named is whether the bracket names a season, and only whether it
		// holds nothing else ('(The Final Season)', and not '(Title: The
		// Final Season Part 3 Special 2)', which is another title)
		named, only := false, true
		for i := sp.open; i < sp.end; i++ {
			switch w := tags[i].v[0]; {
			case tags[i].typ == TagTypeDelim:
				plus += strings.Count(tags[i].v[1], "+")
			case tags[i].typ == TagTypeWhitespace:
			case tags[i].typ != TagTypeText:
				only = false
			case strings.EqualFold(w, "season"):
				named = true
			case !strings.EqualFold(w, "the") && !strings.EqualFold(w, "final"):
				only = false
			}
		}
		st.season = st.season || named && only
		for i := sp.open + 1; plus >= 2 && i < sp.end; i++ {
			if p := i - 2; tags[i].Is(TagTypeSeries) && !tags[i].wroteEpisode() && p >= sp.open && tags[p].Is(TagTypeText) && plain(tags[i-1]) {
				tags[i] = tags[i].As(TagTypeText, nil)
			}
		}
	}
	for i := sh.first + 1; i < len(tags); i++ {
		tag := tags[i]
		// in front of the year that ends the title, what stands in no
		// bracket is the title
		if i < sh.year && st.in[i] == bracketNone {
			continue
		}
		// a season in a list holds the plus behind it ('Season 01+02+OST')
		plus := tag.typ == TagTypeDelim && strings.LastIndexByte(tag.v[1], '+') > strings.LastIndexAny(tag.v[1], "[](){}") ||
			tag.typ == TagTypeSeries && strings.HasSuffix(strings.TrimRight(tag.v[0], " _"), "+")
		if plus {
			j := st.next(i, len(tags))
			if j == -1 {
				break
			}
			name, ok := st.holds(j)
			if _, _, _, isFused := fused(tags[j].val(0)); !ok && !(tags[j].Is(TagTypeText) && isFused) {
				continue
			}
			// what stands in front of the first plus
			if p := st.prev(i, sh.first+1); p != -1 && tags[p].typ != tagTypeUnit {
				if pname, pok := st.holds(p); pok {
					first := p
					for k := st.prev(first, sh.first+1); k != -1 && k+2 == first && plain(tags[k+1]); k = st.prev(first, sh.first+1) {
						kname, kok := st.holds(k)
						if !kok {
							break
						}
						st.claim(k, kname)
						first = k
					}
					st.claim(p, pname)
				}
			}
			if ok {
				st.claim(j, name)
			}
			// a film with its extras is no pack of a series ('Movie + NCED')
			switch name {
			case "Movie", "OP", "ED", "PV", "OST", "Extra":
			default:
				st.pack = true
			}
			// the words behind it that go on with the item
			for k := st.next(j, len(tags)); k != -1 && k == j+2 && plain(tags[j+1]); k = st.next(j, len(tags)) {
				kname, kok := st.holds(k)
				if !kok {
					break
				}
				st.claim(k, kname)
				j = k
			}
			i = j
			continue
		}
		if tag.typ != TagTypeText && tag.typ != TagTypeOther {
			continue
		}
		switch inside := st.in[i] != bracketNone; {
		case tag.typ == TagTypeText && strings.EqualFold(tag.v[0], "batch"):
			p, n := st.prev(i, sh.first), st.next(i, len(tags))
			if !inside && !(i < sh.head && (n == -1 || n >= sh.head || st.in[n] != bracketNone) && p != -1) {
				continue
			}
			// outside a bracket it is the last word of a title unless the
			// name sets it apart, as it does a marker, see apart: a hyphen
			// stands in front of it, or of the 'unofficial' in front of it,
			// or the title is over ('The Bad Batch' and 'Gremlins 2 - The
			// New Batch', against 'Title - 01v2 - 06 - Batch', 'Title -
			// Unofficial Batch' and 'Title 2026 (Episode 897-908) Batch')
			if q, at := p, i; !inside && i < st.over {
				if tags[p].Is(TagTypeText) && strings.EqualFold(tags[p].v[0], "unofficial") {
					q, at = st.prev(p, sh.first), p
				}
				if q == -1 || !strings.HasSuffix(strings.Trim(st.between(q, at), " _"), "-") {
					continue
				}
			}
			// with a word behind it in its bracket it is a word of a phrase.
			// No name of the corpora has one, and the rule keeps a record
			// that would ('[MP3] Artist - Live (Batch Release) (2019)')
			if k := st.nextIn(i); inside && k != -1 && tags[k].Is(TagTypeText) && !allDigits(tags[k].v[0], 4) {
				continue
			}
			tags[i] = unit(tag, unitBatch)
			if p != -1 && tags[p].Is(TagTypeText) && strings.EqualFold(tags[p].v[0], "unofficial") {
				tags[p] = unit(tags[p], unitNote)
			}
		case strings.EqualFold(tag.v[0], "complete"):
			n := st.next(i, len(tags))
			// 'The Complete TV Series'
			if n == i+2 && plain(tags[i+1]) && tags[n].Is(TagTypeText) && strings.EqualFold(tags[n].v[0], "tv") {
				if k := st.next(n, len(tags)); k == n+2 && plain(tags[n+1]) && tags[k].Is(TagTypeText) && strings.EqualFold(tags[k].v[0], "series") {
					tags[n] = unit(tags[n], unitHolds, "TV")
					n = k
				}
			}
			// 'The Complete Batch'
			batch := n != -1 && (tags[n].Is(TagTypeText) || tags[n].typ == tagTypeUnit) && strings.EqualFold(tags[n].v[0], "batch")
			if n == -1 || !plain(tags[n-1]) || st.prev(n, i) != i && tags[st.prev(n, i)].typ != tagTypeUnit || !tags[n].Is(TagTypeText) && !batch ||
				!strings.EqualFold(tags[n].v[0], "series") && !strings.EqualFold(tags[n].v[0], "collection") && !batch {
				continue
			}
			// a title is in front of it, when it stands in no bracket
			p := st.prev(i, sh.first)
			if !inside && (i >= sh.head || p == -1) {
				continue
			}
			// a collection says nothing of what is collected: a box set of
			// films is one too ('The Matrix Complete Collection (1999-2021)',
			// 'Rocky - The Complete Collection'). So it is read in a name
			// that a fansub group wrote alone, where it makes the release a
			// pack and names no contents ('[FacTory] Seitokai Yakuindomo -
			// Complete Collection [Seasons 1-2 + OVAs + Movies]')
			if collection := strings.EqualFold(tags[n].v[0], "collection"); collection && !sh.fansub {
				continue
			} else if collection {
				tags[i], tags[n], st.pack = unit(tag, unitNote), unit(tags[n], unitNote), true
			} else if tags[i], st.pack = unit(tag, unitHolds, "Complete Series"), true; batch {
				tags[n] = unit(tags[n], unitBatch)
			} else {
				tags[n] = unit(tags[n], unitNote)
				// 'Complete Series Collection'
				if k := st.next(n, len(tags)); k == n+2 && plain(tags[n+1]) && tags[k].Is(TagTypeText) && strings.EqualFold(tags[k].v[0], "collection") {
					tags[k], n = unit(tags[k], unitNote), k
				}
			}
			// the episodes it holds, behind it in its bracket ('(Complete
			// Series 001-500)')
			if k := n + 2; inside && k+2 < len(tags) && plain(tags[n+1]) && tags[k].Is(TagTypeText) && allDigits(tags[k].v[0], 4) &&
				tags[k+1].v[1] == "-" && tags[k+2].Is(TagTypeText) && allDigits(tags[k+2].v[0], 4) && atoi(tags[k].v[0]) < atoi(tags[k+2].v[0]) {
				st.merges = append(st.merges, merge{k, k + 3, TagTypeSeries, []string{"", "", tags[k].v[0], "-", tags[k+2].v[0]}})
			}
			if p > sh.first && tags[p].Is(TagTypeText) && strings.EqualFold(tags[p].v[0], "the") && plain(tags[p+1]) {
				tags[p] = unit(tags[p], unitNote)
			}
			i = n
		}
	}
}

// units reads the markers in the head of the name and the numbers that count
// them. A marker counts behind a word of the title, which is more than an
// article, so 'Special A - 01' and 'The Specials' keep their title.
//
//   - written onto its number, the two are one extra or one volume ('SP01',
//     'NCOP2', 'OVA01', 'ED1', and 'BOX7' in a bracket of its own). A hyphen
//     makes a range of it, with the marker written again or not ('SP01-06',
//     'SP01-SP03'), and an ampersand a list ('SP1&3')
//   - apart from its number, the word is the marker and the number the extra
//     or the volume ('OVA 02', 'Special 01', 'NCED_01', 'Vol.06', 'Box 01',
//     'Volume 1'), and so it is with a spaced hyphen between the two, for
//     every marker that names a kind ('- Specials - 02', '- OVA - 03',
//     'OAD - 04', '- ED - 03'). A range counts as one, written tight or
//     with a spaced hyphen between two ends that are written alike
//     ('Specials 01-02', 'OVA 01 - 03', 'Vol.1-5', 'BOX 1-10')
//   - with no number, the word says what the release is, and goes to the
//     contents: a creditless opening or ending anywhere in the head ('Title
//     NCOP [BD]', 'NCOP & NCED'), an OVA at the end of the title ('Title OVA
//     [1080p]', 'Title - OVA (480p)'), and a special, a film or the series
//     behind a hyphen there ('Title - Special [1080p]', 'Title - Movie
//     (V2)', 'Title - TV [BD]')
//
// In a name that no fansub group wrote a marker counts only where the name
// sets it apart, see apart, and nowhere in front of the year that ends the
// title, see shapeOf. A volume is read in a name with a sign of a video
// alone, as a record is numbered the same way ('Artist - Hits Vol. 2').
func (st *stage) units() {
	tags, sh := st.tags, st.sh
	// box is where the bracket that holds the title of a boxed name closes
	var box int
	for _, s := range st.spans {
		if sh.boxed && s.open <= sh.first && sh.first < s.end {
			box = s.end
		}
	}
	var seen bool
	for i := sh.first; i < len(tags); i++ {
		tag := tags[i]
		// behind an episode written with a marker, the words are the title
		// of the episode ('S00E04 Special 4 1080p')
		if tag.typ == TagTypeSeries && tag.wroteEpisode() && !bare(tag) && i < sh.head {
			i = sh.head - 1
			continue
		}
		if tag.typ != TagTypeText {
			continue
		}
		text := tag.v[0]
		// a title is more than its article ('The Specials')
		if !seen {
			seen = !allDigits(text, 4) && !article(text)
			continue
		}
		// in front of the year that ends the title, a word in no bracket is
		// a word of it. One that only an anime release has is still read in
		// a name that a fansub group wrote, which is two signs ('[Group]
		// Title OVA (1999) [DVD]')
		front := i < sh.year && st.in[i] == bracketNone
		// a marker is a word in ASCII letters
		if text == "" || text[0]|0x20 < 'a' || text[0]|0x20 > 'z' {
			continue
		}
		// behind the head a volume alone is read, in a bracket of its own
		// ('S01 1080p BDRip x264 (Vol 1-3)')
		if i >= sh.head {
			// and 'TV Series' says what the release holds wherever it
			// stands, which the discs of one uploader have ('[DVDISO]
			// Doraemon - Disc 7 of 9, TV Series')
			if n := i + 2; n < len(tags) && strings.EqualFold(text, "tv") && plain(tags[i+1]) && tags[n].Is(TagTypeText) && strings.EqualFold(tags[n].v[0], "series") {
				tags[i], tags[n] = unit(tag, unitHolds, "TV"), unit(tags[n], unitNote)
			}
			if m, ok := markerOf(text); !ok || m.kind != unitVolume || !sh.video || st.in[i] == bracketNone || st.prevIn(i) != -1 {
				continue
			}
			if j := st.next(i, len(tags)); j == i+2 && strings.Trim(tags[i+1].v[1], " _.") == "" {
				if first, _, ok := st.counted(j, unitVolume); ok && first != "" {
					tags[i], tags[j] = unit(tag, unitVolume), unit(tags[j], unitVolume, first)
				}
			}
			continue
		}
		// in a bracket, only a marker that opens it counts
		inside := st.in[i] != bracketNone
		if inside && st.prevIn(i) != -1 && !(sh.boxed && i < box) {
			continue
		}
		p := st.prev(i, sh.first)
		if m, num, list, ok := fused(text); ok {
			if !st.apart(i, p, m) || front && !(sh.fansub && m.alone) {
				continue
			}
			// half an extra, which the dot sets apart ('OVA3.5')
			if k := i + 2; k < sh.head && num != "" && list == "" && m.kind != unitVolume && tags[i+1].v[1] == "." && tags[k].Is(TagTypeText) &&
				allDigits(tags[k].v[0], 1) && (st.next(k, sh.head) == -1 || st.in[st.next(k, sh.head)] != bracketNone) {
				st.merges = append(st.merges, merge{i, k + 1, tagTypeUnit, []string{m.kind, num + "." + tags[k].v[0]}})
				i = k
				continue
			}
			// the end of a range behind a hyphen, with its marker again or
			// not ('SP01-06', 'SP01-SP03')
			if k := i + 2; k < sh.head && num != "" && list == "" && m.kind != unitVolume && (tags[i+1].v[1] == "-" || tags[i+1].v[1] == "~") {
				end := tags[k].v[0]
				if bare(tags[k]) {
					end = tags[k].v[2]
				} else if e, n, l, ok := fused(end); ok && e.kind == m.kind && l == "" {
					end = n
				}
				if (tags[k].Is(TagTypeText) || bare(tags[k])) && allDigits(end, 3) && atoi(num) < atoi(end) {
					st.merges = append(st.merges, merge{i, k + 1, tagTypeUnit, []string{m.kind, num, "-", end}})
					i = k
					continue
				}
			}
			switch {
			case m.kind == unitVolume:
				if (inside || strings.EqualFold(text[:3], "vol")) && sh.video && sh.fansub {
					tags[i] = unit(tag, unitVolume, num)
				}
			case num == "":
				tags[i] = unit(tag, unitHolds, m.holds)
			case list != "":
				tags[i] = unit(tag, m.kind, num, "&", list)
			default:
				tags[i] = unit(tag, m.kind, num)
			}
			continue
		}
		m, ok := markerOf(text)
		if !ok {
			// 'END' behind the title says the show ends with this release
			if n := st.next(i, sh.head); text == "END" && p > sh.first && tags[p].Is(TagTypeText) && (n == -1 || st.in[n] != bracketNone && !inside) {
				tags[i] = unit(tag, unitNote)
			}
			continue
		}
		if !st.apart(i, p, m) || front && !(sh.fansub && m.alone) {
			continue
		}
		// the number behind the marker. A volume numbers a film or a record
		// as well ('Kill Bill Vol. 1', 'Hits Vol. 2'), so one is read in the
		// name of a video alone: in one that a fansub group wrote, and in
		// another behind a spaced hyphen with a number of two digits, which
		// no film writes. The boxes of one uploader are named so ('Naruto -
		// Box 01 Remaster [V2] (BDRip 1080p x265 FLAC Multi) - Ryūjin 竜神',
		// 'Dragon Ball Z - Box 14')
		j := st.next(i, sh.head)
		volume := sh.video && (sh.fansub || j == i+2 && p != -1 && strings.HasSuffix(strings.Trim(st.between(p, i), " _"), "-") &&
			len(tags[j].v[0]) >= 2)
		// the number stands right behind a marker that takes one apart
		// ('OVA 02', 'Vol.06'). Or a spaced hyphen sets it off, and then
		// every marker that names a kind counts the number, one rule for
		// all of them: '- Specials - 03' and 'Specials - 03' are special 3,
		// '- Special - 03' is too, '- OVA - 03' and 'OAD - 03' are OVA 3,
		// and '- ED - 03' is ending 3. A word that names no kind says what
		// the release holds, and the number behind it is the episode ('-
		// Movie - 03'). 'SP' with no number on it is a source to the lexers,
		// a tape speed, and no word the stage sees ('- SP - 03')
		kind, d := m.counted(), ""
		if j == i+2 {
			d = tags[i+1].v[1]
		}
		right := m.apart && (strings.Trim(d, " _") == "" || strings.Trim(d, " _") == "." && kind == unitVolume)
		if j == i+2 && (right || kind != "" && hyphened(d)) && (kind != unitVolume || volume) {
			if first, last, ok := st.counted(j, kind); ok {
				m.kind = kind
				tags[i] = unit(tag, m.kind)
				if first != "" {
					tags[j] = unit(tags[j], m.kind, first, "-", last)
					if last == "" {
						tags[j] = unit(tags[j], m.kind, first)
					}
				}
				// a number behind a spaced hyphen ends a range when the two
				// are written as its ends, see ends ('OVA 01 - 03', 'Specials
				// - 01 - 03')
				if k := st.next(j, sh.head); k == j+2 && last == "" && first != "" && hyphened(tags[j+1].v[1]) && m.kind != unitVolume {
					if end, _, ok := st.counted(k, m.kind); ok && end != "" && bare(tags[k]) && st.ends(first, k) {
						st.merges = append(st.merges, merge{j, k + 1, tagTypeUnit, []string{m.kind, first, "-", end}})
						j = k
					}
				}
				i = j
				continue
			}
		}
		if m.holds == "" || p == -1 {
			continue
		}
		// with no number: what the release is
		n := st.next(i, sh.head)
		before := strings.Trim(st.between(p, i), " _")
		ends := n == -1 || st.in[n] != bracketNone && !inside || inside && st.nextIn(i) == -1 || !tags[n].Is(TagTypeText, TagTypeSeries) ||
			strings.ContainsAny(st.between(i, n), "-+&/")
		// more than one of them is a pack ('Title (2015) Specials (BD 1080p)')
		plural := m.kind == "" && m.alone || strings.EqualFold(text, "specials")
		switch {
		case m.kind == unitOP || m.kind == unitED:
			// 'OP' and 'ED' are words too, and count behind a creditless
			// one alone ('NCOP/ED', 'NCOP-ED')
			if m.apart || tags[p].typ == tagTypeUnit && strings.ContainsAny(st.between(p, i), "/-&+") {
				tags[i] = unit(tag, unitHolds, m.holds)
			}
		case m.holds == "TV" && n == i+2 && plain(tags[i+1]) && tags[n].Is(TagTypeText) && strings.EqualFold(tags[n].v[0], "series"):
			// '[DVDISO] Doraemon - Disc 7 of 9, TV Series'
			tags[i], tags[n] = unit(tag, unitHolds, m.holds), unit(tags[n], unitNote)
		case !ends:
		// alone in a bracket it says what the release is behind the title,
		// and names something else with the title behind it, as the lines
		// of one announcer do ('New in [Movies]: Road House 1989')
		case inside && st.prevIn(i) == -1 && st.nextIn(i) == -1:
			if n != -1 && st.in[n] == bracketNone && st.word(n) {
				continue
			}
			tags[i] = unit(tag, unitHolds, m.holds)
		case m.alone, plural, strings.HasSuffix(before, "-"):
			tags[i], st.pack = unit(tag, unitHolds, m.holds), st.pack || plural
		}
	}
}

// apart reports whether the marker m at tag i counts in the name. Any does
// in a name that a fansub group wrote. In another the name has to set it
// apart from the title, see shapeOf: the marker is a word that only an anime
// release has ('OVA', 'NCOP'), or it stands in a bracket, behind a hyphen, a
// plus, a slash or an ampersand ('Title - SP01', 'S01 + Movie'), or behind
// the year or the season, which end the title ('Title (2020) Special 1').
// p is the tag in front of it.
func (st *stage) apart(i, p int, m marker) bool {
	if st.sh.fansub || m.alone || st.in[i] != bracketNone || i > st.over {
		return true
	}
	return p != -1 && strings.ContainsAny(st.between(p, i), "-+/&")
}

// article reports whether s is an article, which is no title alone.
func article(s string) bool {
	return strings.EqualFold(s, "the") || strings.EqualFold(s, "a") || strings.EqualFold(s, "an")
}

// prevIn and nextIn return the tag in front of and behind the one at i
// inside the bracket it stands in, or -1.
func (st *stage) prevIn(i int) int {
	for i--; i >= 0; i-- {
		switch tag := st.tags[i]; {
		case tag.typ == TagTypeDelim && strings.ContainsAny(tag.v[1], "()[]{}"):
			return -1
		case tag.typ != TagTypeDelim && tag.typ != TagTypeWhitespace:
			return i
		}
	}
	return -1
}

func (st *stage) nextIn(i int) int {
	for i++; i < len(st.tags); i++ {
		switch tag := st.tags[i]; {
		case tag.typ == TagTypeDelim && strings.ContainsAny(tag.v[1], "()[]{}"):
			return -1
		case tag.typ != TagTypeDelim && tag.typ != TagTypeWhitespace:
			return i
		}
	}
	return -1
}

// counted reads the number at tag j that a marker of the kind counts: one
// number, or the two ends of a range. A number is at most three digits, and
// a range one the lexers read ('01-02') or two numbers with a hyphen between
// that units joins ('1-5' of 'Vol.1-5'). It reports whether the tag is such
// a number.
func (st *stage) counted(j int, kind string) (first, last string, ok bool) {
	tags := st.tags
	switch tag := tags[j]; {
	case tag.typ == TagTypeText && kind == unitVolume && strings.Contains(tag.v[0], "&"):
		// 'Vol.1&2'
		a, c, _ := strings.Cut(tag.v[0], "&")
		return a, "", allDigits(a, 3) && allDigits(c, 3)
	case tag.typ == TagTypeText && allDigits(tag.v[0], 3):
		first = tag.v[0]
		// 'Vol.1-5', 'BOX 1-10', 'Vol 1-3', and for a volume with spaces
		// beside the hyphen, where the episode lexer reads the end ('Vol 01
		// -02')
		k := j + 2
		if k >= len(tags) || !tags[j+1].Is(TagTypeDelim) {
			return first, "", true
		}
		end := tags[k].v[0]
		if bare(tags[k]) {
			end = tags[k].v[2]
		}
		d := tags[j+1].v[1]
		if (d == "-" || kind == unitVolume && strings.Trim(d, " ") == "-") && (tags[k].Is(TagTypeText) || bare(tags[k])) &&
			allDigits(end, 3) && atoi(first) < atoi(end) {
			if kind == unitVolume {
				tags[k] = unit(tags[k], unitNote)
				return first, "", true
			}
			st.merges = append(st.merges, merge{j, k + 1, tagTypeUnit, []string{kind, first, "-", end}})
			return "", "", true
		}
		return first, "", true
	case bare(tag) && tag.fraction() == "" && len(tag.v[2]) <= 3:
		return tag.v[2], "", true
	}
	if first, last, ok := ranged(tags[j]); ok && len(last) <= 3 {
		return first, last, true
	}
	return "", "", false
}

// hyphened reports whether the delimiter d is a hyphen set off by spaces on
// both sides (' - ').
func hyphened(d string) bool {
	return spaced(d) && strings.Trim(d, " _") == "-"
}

// ends reports whether first and the number of the bare episode at j are the
// two ends of a range that the name writes with a spaced hyphen between
// them. The two ends of a range are written alike: the second is the higher
// and has as many digits, and is written onto no word. So '- 07 - 09' and
// 'OVA 01 - 03' are ranges, and these are not: '- 2 - 05', which is a season
// or a part of the title with its episode, '- 13 - 12', and '- 366 -
// 1000-nen Kessen-hen'.
func (st *stage) ends(first string, j int) bool {
	last := st.tags[j].v[2]
	if len(first) != len(last) || atoi(first) >= atoi(last) {
		return false
	}
	n := st.next(j, st.sh.head)
	return n == -1 || st.between(j, n) != "-" || !st.word(n)
}

// candidate is a bare number of the head that may be the episode.
type candidate struct {
	// at is the tag of the number, and to the last tag of the range it
	// starts, or -1.
	at, to int
	// num is the number, and last the end of the range.
	num, last string
	score     int
	// lexed is whether a lexer read the number as an episode already.
	lexed bool
	// joined is whether the number is written onto a word, or counts the
	// word behind it, which is why it is no candidate.
	joined bool
	// half is whether the tag at to is the half of the episode and no end
	// of a range ('02.5').
	half bool
}

// The score a bare number needs to be the episode: one more in a name that
// no bracket of a group opens, where less says that a fansub group wrote the
// name at all.
const (
	episodeScore     = 3
	episodeScoreBare = 4
)

// numbers decides which bare number of the head is the episode. No one sign
// says so: the episode lexer reads any number behind a hyphen ('- 06'), and
// that is the '100' of 'Fairy Tail - 100 Years Quest - 06' and the '3' of
// 'Gintama - 3-nen Z-gumi Ginpachi-sensei - 09' too, while the '225' of
// '[DB]_Bleach_225_[ABCD1234].avi' has no hyphen. So every bare number of the
// head is scored on how it is written and where it stands, and the best one
// is the episode when it scores enough:
//
//	+3  two digits or more with a zero in front ('06', '068')
//	+1  two digits or more with none ('225'), and -2 for one digit
//	+2  behind a hyphen set off by spaces ('- 06')
//	+2  behind a season, or behind a year or a season in a bracket ('(S2)
//	    06', '(2005) 068')
//	+3  alone in square brackets ('[03]'), +1 alone in parentheses
//	+1  the last word of its run ('Bleach 225 ['), and -2 with a word right
//	    behind it ('Cyborg 009 The Cyborg Soldier')
//	+3  a version behind it ('01v2'), +2 one in the bracket behind it ('870
//	    [v2]')
//	+1  a part letter on it ('111C')
//	+1  the name has a checksum, and +1 it has the extension of a video
//	+2  a higher number behind a hyphen or a tilde makes it a range, and +1
//	    when the two are as wide ('807 - 815')
//
// A number is no candidate when it is a year, the first word of the title,
// in front of the year that ends the title ('Apollo 13 (1995) [1080p]'), a
// part of a decimal or of a number with a comma ('3.33', '80,000'),
// written onto a word with a hyphen ('3-nen', 'R-15'), counted by the word
// in front of it or counting the word behind it ('Part 2', '100 Years'),
// followed by the season ('Gundam 00 Season 2'), a height alone in a bracket
// ('[1080]'), or alone in parentheses with no hyphen in front of them in a
// name that is not one file ('Eighty Six (86)').
//
// A name that writes its episode with a marker is left as it is ('S01E02',
// 'Ep 05', '01-12'). With a number that a lexer read behind a hyphen, no
// other number is made the episode, but for the start of a range in front of
// it ('One Piece 807 - 815'). Such a number is a word of the title when it
// is written onto a word or counts the word behind it ('- 3-nen', '- 100
// Years Quest'), and when it scores 3 less than the best and a word follows
// it.
func (st *stage) numbers() {
	tags, sh := st.tags, st.sh
	// two numbers that a lexer read are a range when they are written as
	// its two ends, see ends, each behind a spaced hyphen ('- 07 - 09', '-
	// 01v2 - 06'): the end carries the range and the start is a note on it,
	// as its version may stand between the two
	for i := sh.first + 1; i < sh.head; i++ {
		if !bare(tags[i]) || tags[i].fraction() != "" {
			continue
		}
		q, p, j := st.prev(i, sh.first), i, st.next(i, sh.head)
		if j == i+1 && tags[j].Is(TagTypeVersion) {
			p, j = j, st.next(j, sh.head)
		}
		// behind the version of the start the hyphen may stand with no
		// space ('- 01v2-03v2')
		if d := st.between(p, j); q == -1 || j == -1 || !bare(tags[j]) || tags[j].fraction() != "" || !st.ends(tags[i].v[2], j) ||
			!hyphened(st.between(q, i)) || !hyphened(d) && !(p != i && d == "-") {
			continue
		}
		tags[j].v = []string{tags[j].v[0], "", "", tags[i].v[2], "-", tags[j].v[2]}
		tags[i] = unit(tags[i], unitNote)
		i = j
	}
	season := -1
	var lexed int
	for i, tag := range tags {
		if tag.typ != TagTypeSeries {
			continue
		}
		switch {
		case bare(tag):
			if i < sh.head && i > sh.first {
				lexed++
			}
		case tag.wroteEpisode():
			return
		case tag.val(1) != "" && i < sh.head:
			season = i
		}
	}
	var cbuf [8]candidate
	cands := cbuf[:0]
	n := 0
	// in front of the year that ends the title a number is a word of it
	for i := max(sh.first, sh.year) + 1; i < sh.head; i++ {
		tag := tags[i]
		if tag.typ != TagTypeText && !bare(tag) {
			continue
		}
		// a number starts with a digit
		if s := tag.v[0]; s == "" || s[0] < '0' || s[0] > '9' {
			continue
		}
		// the next episode of a list, and the end of a range behind the
		// version of its start, are written as the episode in front of them
		// is and are higher ('- 01+02', '- 01v2-04')
		if p := st.prev(i, sh.first); tag.typ == TagTypeText && p != -1 && allDigits(tag.v[0], 4) {
			d, q := st.between(p, i), p
			if tags[p].Is(TagTypeVersion) && p > 0 && bare(tags[p-1]) && d == "-" {
				q = p - 1
			}
			if bare(tags[q]) && (q != p || d == "+" || d == "&") && len(tags[q].v[2]) == len(tag.v[0]) && atoi(tags[q].v[2]) < atoi(tag.v[0]) {
				// the version stands between the two ends of the range, so
				// the end carries the range and the start is a note on it
				if tags[i] = episode(tag, tag.v[0]); q != p {
					tags[i].v = []string{tag.v[0], "", "", tags[q].v[2], "-", tag.v[0]}
					tags[q] = unit(tags[q], unitNote)
				}
				continue
			}
		}
		for n < len(st.spans) && st.spans[n].end <= i {
			n++
		}
		var sp *span
		if n < len(st.spans) && st.spans[n].open <= i {
			if sp = &st.spans[n]; sh.boxed && sp.open <= sh.first {
				continue
			}
		}
		switch c, ok := st.score(i, sp, season); {
		case ok && len(cands) < cap(cands):
			cands = append(cands, c)
		case !ok && c.joined && c.lexed:
			tags[i] = tag.As(TagTypeText, nil)
			lexed--
		}
	}
	if len(cands) == 0 {
		return
	}
	best := 0
	for k, c := range cands {
		if c.score >= cands[best].score {
			best = k
		}
	}
	c := cands[best]
	if lexed != 0 {
		// the start of a range in front of the number a lexer read, which
		// has no hyphen in front of it ('One Piece 807 - 815', 'Great
		// Pretender 01 - 14'). A title may end in zeros, so they start no
		// such range ('Mobile Suit Gundam 00 - 12')
		for _, x := range cands {
			if !x.lexed && x.to != -1 && bare(tags[x.to]) && len(x.num) == len(x.last) && (len(x.num) >= 3 || x.num[0] == '0') && atoi(x.num) != 0 {
				st.merges = append(st.merges, merge{x.at, x.to + 1, TagTypeSeries, []string{"", "", x.num, "-", x.last}})
				return
			}
		}
		for _, x := range cands {
			if nx := st.next(x.at, sh.head); x.lexed && c.lexed && x.score <= c.score-3 && x.at < c.at &&
				nx != -1 && tags[nx].Is(TagTypeText) && !allDigits(tags[nx].v[0], 4) {
				tags[x.at] = tags[x.at].As(TagTypeText, nil)
			}
		}
		return
	}
	need := episodeScoreBare
	if sh.lead {
		need = episodeScore
	}
	switch {
	case c.score < need:
	case c.half:
		// the half is read from the text of the tag, see Tag.fraction
		st.merges = append(st.merges, merge{c.at, c.to + 1, TagTypeSeries, []string{"", c.num}})
	case c.to != -1:
		st.merges = append(st.merges, merge{c.at, c.to + 1, TagTypeSeries, []string{"", "", c.num, "-", c.last}})
	default:
		tags[c.at] = episode(tags[c.at], c.num)
	}
}

// score scores the bare number at tag i as the episode, see numbers, and
// reports whether it is a candidate at all. sp is the bracket it stands in,
// or nil, and season the tag of the season the head writes, or -1.
func (st *stage) score(i int, sp *span, season int) (candidate, bool) {
	tags, sh := st.tags, st.sh
	c := candidate{at: i, to: -1, lexed: tags[i].typ == TagTypeSeries}
	num := tags[i].v[0]
	if c.lexed {
		if num = tags[i].v[2]; tags[i].fraction() != "" {
			return c, false
		}
	}
	var letter bool
	if k := len(num); !c.lexed && k >= 3 && allDigits(num[:k-1], 4) && num[k-1]|0x20 >= 'a' && num[k-1]|0x20 <= 'c' {
		num, letter = num[:k-1], true
	}
	if !allDigits(num, 4) || len(num) == 4 && (num[:2] == "19" || num[:2] == "20") {
		return c, false
	}
	c.num = num
	lo, hi := sh.first, sh.head
	if sp != nil {
		lo, hi = sp.open, sp.end
	}
	p, n := st.prev(i, lo), st.next(i, hi)
	var before, after string
	if p != -1 {
		before = st.between(p, i)
	}
	if n != -1 {
		after = st.between(i, n)
	}
	number := func(k int) bool { return k != -1 && tags[k].Is(TagTypeText) && allDigits(tags[k].v[0], 4) }
	// a range, which the number behind it ends, with half an episode behind
	// that or not ('1-11.5')
	end := i
	if d := strings.Trim(after, " _"); (d == "-" || d == "~") && (!c.lexed || sp != nil) && (atoi(num) != 0 || len(num) >= 2) &&
		(number(n) && atoi(num) < atoi(tags[n].v[0]) || n != -1 && bare(tags[n]) && atoi(num) < atoi(tags[n].v[2])) {
		c.last = tags[n].v[0]
		if bare(tags[n]) {
			c.last = tags[n].v[2]
		}
		c.to, end = n, n
		if k := n + 2; sp != nil && k < hi && tags[n+1].v[1] == "." && number(k) && len(tags[k].v[0]) == 1 && !bare(tags[n]) {
			c.to, end = k, k
		}
		n = st.next(end, hi)
	} else if last := tvEnd(tags, n); sp != nil && d == "-" && last != "" && atoi(num) < atoi(last) && atoi(num) != 0 {
		// the episodes of the series, in a bracket that may list more
		// ('[386-412TV]', '[01-26TV全集+特典映像]')
		c.to, c.last, end, n, p = n, last, n, -1, -1
	}
	// half an episode that ends the title of one file ('Magikarte 02.5
	// [902BB314].mkv'), which the point sets apart from its number
	if k := st.next(n, hi); sp == nil && c.to == -1 && !c.lexed && after == "." && number(n) && len(tags[n].v[0]) == 1 && k == -1 &&
		len(num) >= 2 && num[0] == '0' && sh.single && st.word(p) {
		c.to, c.half, end, n, after = n, true, n, -1, ""
	}
	switch {
	case sp != nil:
		// alone in its bracket, or behind a word that the stage read there
		// ('(Complete Series 001-500)', '(Episodes 1-12)')
		if p != -1 && (tags[p].typ != tagTypeUnit || tags[p].v[1] == unitVolume) || n != -1 {
			return c, false
		}
		switch atoi(num) {
		case 360, 480, 540, 576, 720, 1080, 2160:
			return c, false
		}
		// parentheses hold another name for the show as well ('Eighty Six
		// (86)'), so a number alone in them needs a hyphen in front of them
		// or a name that is one file ('- (84) (960x720)', 'Title (9).mkv')
		if d := tags[sp.open-1].v[1]; sp.by != '[' && p == -1 && c.to == -1 && !sh.single && !strings.Contains(d, "-") {
			return c, false
		}
		if c.score++; sp.by == '[' {
			c.score += 2
		}
	case p == -1:
		return c, false
	// a part of a decimal, or of a number written with a comma ('80,000')
	case before == "." && number(p), after == "." && number(n) && c.to == -1, before == "," && number(p), after == "," && number(n):
		return c, false
	// the season follows the title, and the number is a part of that
	// ('Mobile Suit Gundam 00 Season 2')
	case n != -1 && c.to == -1 && tags[n].Is(TagTypeSeries) && !tags[n].wroteEpisode() && strings.Trim(after, " _") == "":
		return c, false
	// written onto a word, or counting the word behind it
	case before == "-" && st.word(p), after == "-" && st.word(n) && c.to == -1:
		c.joined = true
		return c, false
	// a number written as an episode is, with a zero in front of it,
	// counts nothing ('01 All's well that ends well')
	case n != -1 && c.to == -1 && num[0] != '0' && tags[n].Is(TagTypeText) && strings.Trim(after, " _") == "" &&
		(isCounted([]byte(tags[n].v[0])) || isUnit([]byte(tags[n].v[0]))):
		// behind a hyphen only a word in the plural or a measure is
		// counted: '- 13 OVA' is episode 13, which is an OVA
		if w := tags[n].v[0]; !c.lexed || w[len(w)-1]|0x20 == 's' || isUnit([]byte(w)) {
			c.joined = after != ""
			return c, false
		}
	case after == "" && n != -1 && !tags[n].Is(TagTypeVersion):
		return c, false
	// counted by the word in front of it
	case tags[p].Is(TagTypeText) && counts(tags[p].v[0]) && strings.Trim(before, " _.") == "":
		return c, false
	}
	// in front of what says that the release is complete, a number is a
	// word of the title: a pack is no one episode ('[Group] James Bond 007 -
	// Complete Collection')
	if k := n; k != -1 && c.to == -1 && tags[k].typ == tagTypeUnit {
		if strings.EqualFold(tags[k].v[0], "the") {
			k = st.next(k, hi)
		}
		if k != -1 && tags[k].typ == tagTypeUnit && strings.EqualFold(tags[k].v[0], "complete") {
			return c, false
		}
	}
	// the end of a range says as much as its start ('1-11.5')
	switch wide := len(num) >= 2 || len(c.last) >= 2; {
	case len(num) >= 2 && num[0] == '0':
		c.score += 3
	case wide:
		c.score++
	default:
		c.score -= 2
	}
	if letter {
		c.score++
	}
	if d := strings.Trim(before, " _"); sp == nil && len(before) > 1 && strings.HasSuffix(d, "-") && !strings.ContainsAny(d, "()[]{}") {
		c.score += 2
	}
	// behind the season, or behind a year or a season in a bracket
	if sp == nil && p != -1 && (p == season || tags[p].Is(TagTypeDate, TagTypeSeries) && st.in[p] != bracketNone && !tags[p].wroteEpisode()) {
		c.score += 2
	}
	if c.to != -1 && !c.half {
		if c.score += 2; len(c.last) == len(num) {
			c.score++
		}
	}
	switch {
	case n == -1:
		c.score++
	case tags[n].Is(TagTypeVersion):
		c.score += 3
	case st.in[n] != bracketNone && sp == nil, tags[n].Is(tagTypeUnit):
		c.score++
	// the title goes on behind it ('Cyborg 009 The Cyborg Soldier'), where
	// the title of an episode follows a hyphen ('397 - One Worthy As a
	// Leader')
	case sp == nil && st.word(n) && tags[n].v[0] != "&" && strings.Trim(after, " _") == "":
		c.score -= 2
	}
	// a version in the bracket behind it
	if n == -1 && sp == nil && end+2 < len(tags) && tags[end+2].Is(TagTypeVersion) && st.in[end+2] != bracketNone {
		c.score += 2
	}
	if sh.sum {
		c.score++
	}
	if sh.ext {
		c.score++
	}
	return c, true
}

// tvEnd returns the number at the start of the text at n when 'TV' follows it
// there, ie the '412' of '412TV', and the empty string otherwise.
func tvEnd(tags []Tag, n int) string {
	if n == -1 || !tags[n].Is(TagTypeText) {
		return ""
	}
	s := tags[n].v[0]
	k := 0
	for k < len(s) && k < 4 && s[k] >= '0' && s[k] <= '9' {
		k++
	}
	if k == 0 || !strings.HasPrefix(s[k:], "TV") {
		return ""
	}
	return s[:k]
}

// packType returns the type of a release that the structure stage read, when
// its numbering says more than the tags that typ was read from. A name that
// says it is a batch is a series, whatever episodes it writes ('S01 E01-E13
// (Batch)', '- 01v2 - 06 - Batch').
//
// A name that writes more than one episode with bare numbers, as a range or
// as a list joined by a plus or an ampersand, is a series, which is a pack
// of them ('Title - 01-12', 'Title - 01+02', 'Title - 07 - 09'). When the
// name is one file, the file holds them all and is an episode ('Title -
// 316-317 [DCTP][2411959B].mkv'). The checksum says so, which is that of one
// file. The extension of a video does not: a group may name a folder after
// its files ('[shincaps] Crayon Shin-chan - 450-459 (CS-EX2 1440x1080 MPEG2
// AAC).ts'). The extras follow the same rule: one numbered extra is an
// episode ('Title - OVA 02', 'Title - SP01'), and more than one a series
// unless the name is one file ('Title Specials 01-02'). Episodes written
// with a marker keep the type their tags give ('S01E01-E02').
//
// A film or a name with no sign of a type that gives a volume or what a pack
// holds is a series ('Title - Vol.06', 'Title - Complete Series', 'Title +
// OVA', 'Title - TV'), and so is one that names a season in a bracket of its
// own, see shape ('Title (The Final Season)'). An extra with no number says
// what the release is and not how much of it: 'Title - OVA' and 'Title -
// Movie' keep the type the tags give, and more than one opening or ending is
// a pack of them ('Title NCOP/ED').
func (b *TagBuilder) packType(r *Release, typ Type) Type {
	n := &r.Numbering
	if !r.shaped || !typ.Is(Unknown, Movie, Episode, Series) {
		return typ
	}
	if n.Batch {
		return Series
	}
	// extras is whether the name gives extras alone, and several whether it
	// gives more than one episode: a range, more than one extra, or a list
	// of regular episodes, which a plus or an ampersand joins. Two numbers
	// that each stand behind a hyphen of their own are no list ('- 2 - 05',
	// '- 13 - 12')
	several, extras := false, len(n.Episodes) != 0
	for _, e := range n.Episodes {
		several, extras = several || e.First != e.Last, extras && e.Kind != EpRegular
	}
	several = several || extras && len(n.Episodes) > 1
	// marked is whether an episode is written with a marker ('S01E02', 'E02',
	// 'Episode 2')
	marked := false
	for i, tag := range r.tags {
		if tag.typ != TagTypeSeries || !tag.wroteEpisode() {
			continue
		}
		_, _, plain := ranged(tag)
		marked = marked || !bare(tag) && !plain
		if p := i - 2; p >= 0 && r.tags[p].typ == TagTypeSeries && r.tags[i-1].typ == TagTypeDelim && strings.ContainsAny(r.tags[i-1].v[1], "+&") {
			several = true
		}
	}
	switch {
	case several && !marked && (extras || typ.Is(Episode, Series)):
		if r.single {
			return Episode
		}
		return Series
	case typ.Is(Episode, Series):
		return typ
	case n.Volume != 0, r.pack, slices.Contains(n.Contents, "TV"):
		return Series
	case extras:
		return Episode
	// more than one opening, ending or trailer with no number is a pack of
	// them ('Title NCOP/ED [BD]', 'Title NCOP & NCED')
	case len(n.Contents) > 1 && !slices.ContainsFunc(n.Contents, func(s string) bool { return s != "OP" && s != "ED" && s != "PV" }):
		return Series
	}
	return typ
}

// join joins the runs of tags the steps asked for into one tag each. It is
// done once and last, in one pass over the tags, so that a name of nothing
// but such runs costs no more than its length. Of two runs that overlap, the
// one behind is kept. The tags of the caller are left as they were given.
func (st *stage) join(r *Release) {
	if len(st.merges) == 0 {
		return
	}
	slices.SortFunc(st.merges, func(a, b merge) int { return b.from - a.from })
	// the runs to join, from the end of the name
	keep := st.merges[:0]
	for _, m := range st.merges {
		if len(keep) == 0 || m.to <= keep[len(keep)-1].from {
			keep = append(keep, m)
		}
	}
	tags, end, at := make([]Tag, 0, len(r.tags)), r.end, 0
	for _, m := range slices.Backward(keep) {
		tags = append(tags, r.tags[at:m.from]...)
		var sb strings.Builder
		for _, tag := range r.tags[m.from:m.to] {
			sb.WriteString(tag.v[0])
		}
		v := append([]string{sb.String()}, m.vals...)
		tags = append(tags, Tag{typ: m.typ, v: v, was: TagTypeText ^ wasBit})
		if m.to <= r.end {
			end -= m.to - m.from - 1
		}
		at = m.to
	}
	r.tags, r.end = append(tags, r.tags[at:]...), end
}
