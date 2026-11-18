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
//	[Group] Title | Other Title (Season 1) [tags]
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
//
// The title of such a name is read by titleParts, which knows that it may
// give more than one.
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
// apart with spaces, and that has one of two things more.
//
// A fansub group wrote it, which the name says itself in one of two ways:
//
//   - a bracket opens it that names who made the release: a group or a
//     site ('[Judas]', '(CBB)'), or words that are no tag ('[Golumpa &
//     Aergia]'). A bracket that holds tags or a year opens any kind of name,
//     see nameless ('[FLAC] Artist - Album', '[BluRay] Title', '[2019]
//     Title', '(2001) A Space Odyssey')
//   - it has the checksum of its file ('[ABCD1234]')
//
// The tag of a tracker in front of a spaced title cannot be told from a
// group, and opens the name as one does ('[TGx] Dante 01 1080p WEB-DL
// x264'). No list of such tags is kept.
//
// Or it has a sign of a video: a resolution, a codec or the extension of a
// file. That is the name of a film or a show as YTS, QxR, TGx or a Plex
// library write it, and the sign says what the release is and not who named
// it.
//
// A name with none of this is left as the lexers read it: one with no
// bracket, one that sets its words apart with dots, which is a scene name
// whatever opens it ('[TGx] Movie.Name.2019.1080p.x264-GRP'), and a record or
// a book, which has no sign of a video ('Artist - Album (2019) [FLAC]').
func (b *TagBuilder) shapeOf(tags []Tag, in []uint8, spans []span) (shape, bool) {
	sh := shape{first: -1, head: len(tags)}
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
	return sh, true
}

// nameless reports whether the bracket s, which opens a name, names no one
// who made the release. The meta lexer reads any one word in the bracket that
// opens a name as its site, so the word is looked at here: it is a year
// ('[2019] Title'), or a word that the vocabulary reads as a tag, see tagged
// ('[BluRay]', '[4K]', '[x265]', '[English]', '[PL]', '[Hi10]', '[Uncut]',
// '[Netflix]'), or words set apart by spaces that each are one or belong to
// one ('[Rus Eng]', '[ENG SUB]'). A known group is a group whatever else its
// name spells. Any other word names a maker: one of quality that the
// vocabulary does not hold ('[HD]'), and two tags written as one word
// ('[AAC2.0]').
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
// group from them, and keeps for the title where it starts.
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
	in, spans := brackets(r.tags, buf, sbuf)
	if len(spans) == 0 {
		return in, spans
	}
	// a number with a point in the title, and a group in parentheses that
	// opens the name: both give a tag another type, and the brackets are
	// read again behind them
	if s := spans[0]; decimals(r.tags, false) || !s.meta && s.by == '(' && s.role == roleWords {
		r.tags = slices.Clone(r.tags)
		if found, lead := decimals(r.tags, true), b.leadGroup(r, in, spans); found || lead {
			in, spans = brackets(r.tags, in[:0], spans[:0])
		}
	}
	sh, ok := b.shapeOf(r.tags, in, spans)
	if !ok {
		r.tags, r.end, r.groupFrom = tags, end, from
		return in, spans
	}
	r.shaped, r.first, r.boxed = true, sh.first, sh.boxed
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
	return s
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
