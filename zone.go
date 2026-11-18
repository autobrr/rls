package rls

import (
	"slices"
	"strings"
	"unicode"
)

// The zone stage owns one question: is a word that a lexer claimed for the
// vocabulary a tag, or a word of a title. It reads a name as
//
//	[lead] title anchor [episode title] tags [-group]
//
// where the anchor is the year, the date or the numbering that ends the
// title, and the tags start at the first technical tag (a resolution, a
// codec, ...). settle then decides every claim from where it stands, and
// settleVideo decides the few that need the type of the release.
//
// A scene record has the same zones in another order, with its year at the
// end ('Artist-Title_(Subtitle)-Label-WEB-2019-GRP'). Its anchor is the first
// source or technical tag with a field to itself, or the year when it has
// neither. What stands in front of the anchor is the title there too, and
// three rules keep its words words: a claim with a plain word behind it is
// inside the title, whatever brackets are between ('Bang_(Japanese_Version)-
// JA-DVDRip'); a bracket in front of the anchor belongs to the title, where
// one behind it lists tags, see brackets and word; and a language that
// shares a field between hyphens with a word names the record, see
// fieldWord.

// zones are the zones of a name, as positions in its tags.
type zones struct {
	// start is the first word of the title.
	start int
	// last is the last word in front of the anchor that is no claim, in a
	// bracket or not. The claims in front of it are inside the title.
	last int
	// close is the anchor, or the first bracket after start when that comes
	// first: a title that runs straight to its anchor has close == anchor.
	close int
	// anchor is the tag that ends the title, or end when there is none.
	anchor int
	// tech is the first technical tag at or after the anchor, or end.
	tech int
	// tail is a last word that no hyphen sets off, which is the group of a
	// name written with spaces ('Title PS4 GRP'), or -1.
	tail int
	// dated is the earlier of the last date and the last numbering of the
	// name, or end when it has neither: what stands in front of it stands in
	// front of the year or the numbering, whatever is between. numbered is
	// whether the name has a numbering.
	dated    int
	numbered bool
	// end is the end of the tags the zones cover.
	end int
}

// side is what stands next to a tag, across delimiters.
type side uint8

const (
	// sideNone is the start or the end of the name.
	sideNone side = iota
	// sideWord is a word.
	sideWord
	// sideTag is a tag.
	sideTag
	// sideWall is a bracket.
	sideWall
	// sideTail is the last word of the name, see zones.tail.
	sideTail
	// sideAnchor is the anchor.
	sideAnchor
)

// technical reports whether typ is a type whose words are not words of a
// title: what it claims, it holds.
func technical(typ TagType) bool {
	switch typ {
	case TagTypeResolution, TagTypeFrameRate, TagTypeCodec, TagTypeHDR, TagTypeBitDepth,
		TagTypeSize, TagTypeVersion, TagTypeDisc, TagTypeContainer, TagTypeID:
		return true
	}
	return false
}

// claim strengths. A weak claim is a word wherever the zones call for words:
// inside the title, and between two words anywhere. A firm claim is a word as
// the first word of a name, as the one claim in front of a year or the
// numbering when edgeWord says so, and where a rule of its own in word says
// so ('Dark.Web', 'Extended.Highlights', 'Freestyle.Dual.Moguls').
const (
	strengthNone = iota
	strengthWeak
	strengthFirm
)

// strength returns how well the claim of a tag holds against a title.
func strength(tag Tag) int {
	switch tag.typ {
	case TagTypeLanguage, TagTypeOther, TagTypeCollection, TagTypePlatform, TagTypeArch:
		return strengthWeak
	case TagTypeSource, TagTypeAudio, TagTypeCut, TagTypeEdition, TagTypeGenre, TagTypeRegion:
		if collisionProne(tag) {
			return strengthWeak
		}
		return strengthFirm
	}
	return strengthNone
}

// collisionProne reports whether tag is one of the few spellings of a firm
// type that are ordinary words: 'Web', 'Uncut', 'dc'.
func collisionProne(tag Tag) bool {
	switch t := tag.Text(); tag.typ {
	case TagTypeSource:
		return t == "Web"
	case TagTypeCut:
		return t == "Uncut" || strings.EqualFold(t, "dc")
	}
	return false
}

// keeps reports whether tag is a row that word leaves a tag wherever it
// stands: 'IMAX', a remix, a 'MULTi' ('2024.MULTI.Audio.Sub'), and an F1
// broadcaster ('Japan Qualifying F1TV Live 1080p', 'Race F1TV Live+
// International 2160p'). An 'IMAX' that opens the title of a video is still
// a word, see openingCollection.
func keeps(tag Tag) bool {
	switch tag.typ {
	case TagTypeCollection:
		return tag.Collection() == "IMAX" || service(tag)
	case TagTypeOther:
		return tag.Other() == "REMiX"
	case TagTypeLanguage:
		return strings.HasPrefix(tag.Language(), "MULTi")
	}
	return false
}

// languageName reports whether tag names a language, which can be a word of
// a title ('Going.Dutch', 'The.Italian.Job'). A row that says how a release is
// dubbed or subtitled is none ('Naruto Shippuden Dubbed S01E01', 'Subbed',
// 'Synced', 'HC', 'DL', 'VOSTFR').
func languageName(tag Tag) bool {
	if tag.typ != TagTypeLanguage {
		return false
	}
	switch s := tag.Language(); {
	case strings.Contains(s, "UB"), strings.HasPrefix(s, "MULTi"), strings.HasPrefix(s, "VF"), strings.HasPrefix(s, "VOST"),
		s == "SYNCED", s == "HC", s == "DL", s == "AUDiO.ADDON":
		return false
	}
	return true
}

// capitals reports whether s is written in capitals, as a tag is. The small
// 'i' of the scene is one ('DANiSH', 'iNTERNAL').
func capitals(s string) bool {
	var upper bool
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z' && c != 'i':
			return false
		case c >= 'A' && c <= 'Z':
			upper = true
		}
	}
	return upper
}

// namedLikeWord reports whether tag is one of the few rows of a weak type
// that are a word in front of a year or the numbering, even as the first or
// the last word of the title: 'RED', 'CC', 'Amazon', 'MD', 'Restoration'.
func namedLikeWord(tag Tag) bool {
	switch tag.typ {
	case TagTypeCollection:
		c := tag.Collection()
		return c == "CC" || c == "RED" || c == "AMZN" && strings.EqualFold(tag.Text(), "amazon")
	case TagTypeOther:
		return tag.Other() == "MD" || tag.Other() == "RESTORATiON"
	}
	return false
}

// tagWord reports whether s is a word that no lexer claims and that still
// belongs to the tag in front of it: the 'Dub' of 'English Dub', the 'Subs'
// of 'Eng Subs', the 'Audio' of 'MULTI Audio', the 'Subtitled' of 'English
// Subtitled'. It is no word of a title for the claim in front of it.
func tagWord(s string) bool {
	var words [2]string
	switch len(s) {
	case 3:
		words = [2]string{"dub", "sub"}
	case 4:
		words = [2]string{"dubs", "subs"}
	case 5:
		words = [2]string{"audio"}
	case 6:
		words = [2]string{"audios", "fandub"}
	case 7:
		words = [2]string{"softsub", "hardsub"}
	case 8:
		return strings.EqualFold(s, "subtitle") || strings.EqualFold(s, "softsubs") || strings.EqualFold(s, "hardsubs")
	case 9:
		words = [2]string{"subtitles", "subtitled"}
	default:
		return false
	}
	return strings.EqualFold(s, words[0]) || words[1] != "" && strings.EqualFold(s, words[1])
}

// computeZones finds the zones of tags[:end].
func computeZones(tags []Tag, end int) zones {
	z := zones{end: end, anchor: end, tech: end, tail: -1}
	// lead: whitespace, delimiters, site and group brackets, and brackets
	// that hold tags
	i := 0
	for {
		for ; i < end && tags[i].Is(TagTypeWhitespace, TagTypeDelim, TagTypeMeta); i++ {
		}
		if i == end || i == 0 || !tags[i-1].Is(TagTypeDelim) || tags[i].Is(TagTypeText) {
			break
		}
		d := tags[i-1].v[0]
		if d == "" || strings.IndexByte("[({", d[len(d)-1]) == -1 {
			break
		}
		// a bracket with nothing but words after its first holds the title
		// ('[Group] [Complete Manual] - 04'), and one with tags in it holds
		// tags ('[BD 1080p] Title')
		var words, n int
		j, depth := i+1, 1
		for ; j < end && depth > 0; j++ {
			switch {
			case tags[j].Is(TagTypeDelim):
				// the bracket may hold brackets ('[4K_HDR(2160P)+1080P]')
				for _, c := range []byte(tags[j].v[0]) {
					switch c {
					case '(', '[', '{':
						depth++
					case ')', ']', '}':
						depth--
					}
					if depth == 0 {
						break
					}
				}
			case tags[j].Is(TagTypeText):
				words++
			case !tags[j].Is(TagTypeWhitespace):
				n++
			}
		}
		if words != 0 && n == 0 || depth > 0 {
			break
		}
		i = j - 1
	}
	z.start = i
	if end > 0 && tags[end-1].Is(TagTypeText) {
		z.tail = end - 1
	}
	// the last date is the one that can end the title, the ones in front of
	// it are numbers in the title ('2001.A.Space.Odyssey.1968')
	date := -1
	for j := end - 1; j > z.start; j-- {
		if tags[j].Is(TagTypeDate) {
			date = j
			break
		}
	}
	// a source ends the title as the numbering does, but for the spellings
	// that are words ('A.Dark.Web.1080p'). A range of numbers that the
	// numbering after it shows to be a word does not ('Tatsuki Fujimoto
	// 17-26 - 01 ~ 08'). Neither does a source inside a field of a scene
	// record, which a hyphen opens and underscores join: behind a word or a
	// language it names the record with them ('Polizei-Dutch_EP_(Rotterdam_
	// Remix)-Vinyl-1997', 'VA-Kale_Plankieren_-_Dutch_Cassette_Rarities-WEB-
	// 2018'), where the source of the release has a field to itself or
	// shares one with a tag ('-Retail_CDA-', '-Promo_CDS-'). It is the
	// anchor only when nothing else is
	inner := -1
	var hyphen, joined, named bool
	for j := z.start + 1; j < end; j++ {
		typ := tags[j].typ
		if typ == TagTypeDelim {
			if joined = strings.Trim(tags[j].v[0], "_") == ""; !joined {
				hyphen = strings.Contains(tags[j].v[0], "-")
			}
			continue
		}
		if typ == TagTypeSeries && titleRange(tags, j, end) {
			continue
		}
		if typ == TagTypeSource && !collisionProne(tags[j]) && hyphen && joined && named {
			if inner == -1 {
				inner = j
			}
			continue
		}
		named = typ == TagTypeText || languageName(tags[j])
		if typ == TagTypeSeries || typ == tagTypeUnit || j == date || technical(typ) || typ == TagTypeSource && !collisionProne(tags[j]) {
			z.anchor = j
			break
		}
	}
	if z.anchor == end && inner != -1 {
		z.anchor = inner
	}
	for j := z.anchor; j < end; j++ {
		if technical(tags[j].typ) {
			z.tech = j
			break
		}
	}
	// the last date and the last numbering, of which the first is where the
	// name is dated
	z.dated = end
	for _, typ := range [...]TagType{TagTypeDate, TagTypeSeries} {
		for j := end - 1; j >= 0; j-- {
			if tags[j].typ == typ {
				z.dated, z.numbered = min(z.dated, j), z.numbered || typ == TagTypeSeries
				break
			}
		}
	}
	z.close = z.anchor
	for j := z.start + 1; j < z.anchor; j++ {
		if tags[j].Is(TagTypeDelim) && strings.ContainsAny(tags[j].v[0], "()[]{}") {
			z.close = j
			break
		}
	}
	z.last = z.start
	for j := z.anchor - 1; j > z.start; j-- {
		if tags[j].Is(TagTypeText) && !tagWord(tags[j].v[0]) || tags[j].Is(TagTypeSeries) {
			z.last = j
			break
		}
	}
	return z
}

// anchorTag returns the tag at the anchor. A name with nothing to anchor it
// has its anchor at end, and no tag stands there when the name ends the tags
// ('Show German]' built with every tag in the name). So the anchor is read
// here alone, and is a tag of no type for such a name.
func (z zones) anchorTag(tags []Tag) Tag {
	if z.anchor >= z.end {
		return Tag{}
	}
	return tags[z.anchor]
}

// titleRange reports whether the tag at i is a range of numbers after a word
// that ends the title, with an episode after it past a spaced hyphen, ie the
// '17-26' of '[Erai-raws] Tatsuki Fujimoto 17-26 - 01 ~ 08', which names the
// show where the '01 ~ 08' are its episodes. A range that starts at 1 is not
// one, as that is how the episodes of a batch are written ('Show 01-12 - 13').
func titleRange(tags []Tag, i, end int) bool {
	if i < 2 || i+2 >= end {
		return false
	}
	tag, delim, next := tags[i], tags[i+1], tags[i+2]
	// the shape the episode range lexer gives, written tight ('17-26')
	return len(tag.v) == 6 && tag.v[1] == "" && tag.v[2] == "" && tag.v[4] == "-" && !strings.ContainsAny(tag.v[0], " ~") &&
		atoi(tag.v[3]) > 1 &&
		tags[i-2].Is(TagTypeText) && tags[i-1].Is(TagTypeDelim) && tags[i-1].v[0] == " " &&
		delim.Is(TagTypeDelim) && delim.v[0] == " - " && next.Is(TagTypeSeries) && next.val(1) == "" && next.wroteEpisode()
}

// side returns what stands next to the tag at i in the direction of inc, its
// position, and whether only dots, underscores and spaces are between. A
// bracket between the two is a wall: what stands past it is no neighbour
// ('Title [English Dub]', 'Title (Multi-Subs)'), the anchor included.
func (z zones) side(tags []Tag, i, inc int) (side, int, bool) {
	plain := true
	var wall bool
	for i += inc; 0 <= i && i < z.end; i += inc {
		switch {
		case tags[i].Is(TagTypeWhitespace):
			continue
		case tags[i].Is(TagTypeDelim):
			d := tags[i].v[0]
			plain = plain && strings.Trim(d, "._ ") == ""
			wall = wall || strings.ContainsAny(d, "()[]{}")
			continue
		case wall:
			return sideWall, i, plain
		case i == z.anchor:
			return sideAnchor, i, plain
		case tags[i].Is(TagTypeText):
			switch {
			case i == z.tail:
				return sideTail, i, plain
			case inc > 0 && tagWord(tags[i].v[0]):
				return sideTag, i, plain
			}
			return sideWord, i, plain
		}
		return sideTag, i, plain
	}
	return sideNone, i, plain
}

// settle decides, for every word the lexers claimed, whether it is a tag or
// a word, and turns the words back into text. The type of the release is not
// known yet, as it is read from the tags that settle leaves: see settleVideo
// for the claims that need it. It returns the zones of the name, which
// settleGroup reads the group in.
func (b *TagBuilder) settle(r *Release) zones {
	b.settleWindows(r)
	b.settleDates(r)
	tags, end := r.tags, r.end
	z := computeZones(tags, end)
	// numbering in front of the anchor is a word of the title, see
	// titleRange, and so is every range of that shape behind it ('Show 17-26
	// - 01 ~ 08 | Other 27-36 - 09 ~ 16')
	for i := z.start + 1; i < end; i++ {
		if tags[i].Is(TagTypeSeries) && (i < z.anchor || titleRange(tags, i, end)) {
			tags[i] = tags[i].As(TagTypeText, nil)
		}
	}
	// the first word of a name is a word of its title, whatever it reads as
	// ('Web.Therapy', 'DOC.Martin'). A publisher is not ('Lynda.com.Title'),
	// and neither is a genre in parentheses ('(Comedy) Title')
	if i := z.start; i < end {
		switch typ := tags[i].typ; {
		case typ.Is(TagTypeResolution, TagTypeCodec, TagTypeHDR, TagTypeBitDepth),
			typ == TagTypeGenre && !parenthesized(tags, i),
			typ != TagTypeGenre && typ != TagTypeCollection && strength(tags[i]) != strengthNone,
			typ == TagTypeCollection && namedLikeWord(tags[i]) && i < z.dated && z.dated != end:
			tags[i] = tags[i].As(TagTypeText, nil)
		}
	}
	// edge is the number of claims between the last word of the title and
	// the anchor, or the first bracket of the title
	var edge int
	for i := z.last + 1; i < z.close; i++ {
		if strength(tags[i]) != strengthNone {
			edge++
		}
	}
	var buf [256]uint8
	in, _ := brackets(tags, buf[:0], nil)
	// from the right, so that each claim sees what its right neighbour
	// became. source is whether a source stands behind the claim
	var source bool
	for i := end - 1; i >= 0; i-- {
		if i != z.start && strength(tags[i]) != strengthNone && word(tags, z, in, i, edge, source) {
			tags[i] = tags[i].As(TagTypeText, nil)
		}
		source = source || tags[i].Is(TagTypeSource)
	}
	// a name has a title. With no word in front of the anchor, the
	// spellings that are ordinary words are it in a bracket too ('(Uncut)
	// 2019', '[CC] 2019 1080p', '(MD) 2019'), and so is the collection that
	// opens the name ('Hulu.2019.1080p.WEB', 'XXX.2002.NORDIC.PAL.DVDR',
	// 'DSNP.S01E01.1080p')
	if !slices.ContainsFunc(tags[:z.anchor], func(t Tag) bool { return t.Is(TagTypeText) }) {
		for i := 0; i < z.dated && z.dated != end; i++ {
			if tag := tags[i]; collisionProne(tag) || namedLikeWord(tag) || z.numbered && tag.typ.Is(TagTypeArch, TagTypePlatform) {
				tags[i] = tag.As(TagTypeText, nil)
			}
		}
		if i := z.start; i < end && tags[i].Is(TagTypeCollection) {
			tags[i] = tags[i].As(TagTypeText, nil)
		}
	}
	return z
}

// The kinds of bracket a tag can stand in, see brackets.
const (
	// bracketNone is no bracket, or one that never closes.
	bracketNone uint8 = iota
	// bracketPhrase is a bracket with a plain word in it: '(Japanese
	// Version)', '(Curated by Lawrence English)'.
	bracketPhrase
	// bracketHead is the words and claims in front of the tags a bracket
	// lists: the 'B-Global Donghua' of '(B-Global Donghua 1920x1080 HEVC)',
	// the title of '(Other Title, Dual-Audio, Multi-Subs)', the 'Japanese
	// Release' of '(Japanese_Release_CDS)'.
	bracketHead
	// bracketLone is a bracket around one claim and nothing else: '(DE)',
	// '[Netflix]', '(Complete)'.
	bracketLone
	// bracketTags is a bracket of claims and the words that belong to them:
	// '[English Dub]', '(English-DUB)', '(EN|FR|DE)'.
	bracketTags
	// bracketList is the part of a bracket where it lists tags: from the
	// first tag in it that only a tag can be, a source, an audio tag or a
	// technical one, with the claims right in front of that tag. It is the
	// 'BD 1080p HEVC English Judas' of '(BD 1080p HEVC English Judas)' and
	// the 'remux-DualAudio' of '[HQR.remux-DualAudio]'. A word in it is a
	// group or a note. What stands in front of it is a bracketHead when it
	// holds a word, and else has the kind it would have alone.
	bracketList
)

// The roles a bracket can have in a name, by what it holds. The structure
// stage reads the title, the numbering and the group of a fansub name from
// them, see structure, and the owner of the group which bracket is the group.
const (
	// roleNone is a bracket that holds nothing.
	roleNone uint8 = iota
	// roleGroup is a group or a site, which the meta lexer read as one
	// ('[SubsPlease]') or the owner of the group made a group tag ('(CBB)').
	roleGroup
	// roleSum is the checksum of the file ('[ABCD1234]').
	roleSum
	// roleTags is a bracket that holds a tag ('[BD 1080p]', '(English Dub)',
	// '[Batch]' once the stage has read the word).
	roleTags
	// roleNumber is numbers and nothing else ('[03]', '(01-12)', '[922]',
	// '(S2)').
	roleNumber
	// roleWords is words and no tag ('[Title of the Show]', '(Other Name)',
	// '(CBB)'). A number or a season may stand among them ('[Title 2]',
	// '[Title S2]').
	roleWords
	// roleYear is a year or a date ('(2004)').
	roleYear
)

// span is one bracket of a name.
type span struct {
	// open is the first tag in the bracket, and end the delimiter that
	// closes it. A meta tag is a bracket of its own, with end == open+1,
	// and meta says that it is one.
	open, end int
	meta      bool
	// by is the byte that opens it: '[', '(' or '{'.
	by byte
	// role is what the bracket holds.
	role uint8
	// words is the number of plain words in it.
	words int
}

// brackets returns, for each of tags, the kind of bracket it stands in. It
// appends to buf, which lets the caller keep a short name off the heap. With
// spans that is not nil it also appends each bracket of the name to it, in
// the order they open, with the role of each, and returns it.
//
// A bracket is read whole, from where it opens to where the last bracket
// inside it closes ('[4K_HDR(2160P)+1080P]'), and what it holds decides how
// word reads a claim in it. This is the one reading of the brackets of a
// name: the zones, the structure stage and the owner of the group all read
// its result.
func brackets(tags []Tag, buf []uint8, spans []span) ([]uint8, []span) {
	in := append(buf, make([]uint8, len(tags))...)
	var depth, open int
	var by byte
	for i, tag := range tags {
		switch tag.typ {
		case TagTypeMeta:
			if spans != nil && depth == 0 {
				role := uint8(roleTags)
				switch k, _ := tag.Meta(); k {
				case "site":
					role = roleGroup
				case "sum":
					role = roleSum
				}
				spans = append(spans, span{open: i, end: i + 1, meta: true, by: tag.val(0)[0], role: role})
			}
			continue
		case TagTypeDelim:
		default:
			continue
		}
		// one delimiter can close a bracket and open the next ('][')
		for _, c := range []byte(tag.v[0]) {
			switch c {
			case '(', '[', '{':
				if depth++; depth == 1 {
					open, by = i+1, c
				}
			case ')', ']', '}':
				if depth == 0 {
					continue
				}
				if depth--; depth == 0 {
					role, words := bracket(tags, in, open, i)
					if spans != nil {
						spans = append(spans, span{open: open, end: i, by: by, role: role, words: words})
					}
				}
			}
		}
	}
	return in, spans
}

// bracket sets in to the kinds of the bracket that holds tags[open:end], and
// returns its role and the number of plain words in it.
func bracket(tags []Tag, in []uint8, open, end int) (uint8, int) {
	// list is where the bracket starts to list tags
	list, claim := end, -1
	var claims, words, others, numbers, seasons, dates, groups int
	var sum bool
loop:
	for i := open; i < end; i++ {
		switch tag := tags[i]; {
		case tag.Is(TagTypeDelim, TagTypeWhitespace):
			continue
		case technical(tag.typ), tag.Is(TagTypeAudio, TagTypeChannels), tag.typ == TagTypeSource && !collisionProne(tag):
			// claim is the first of the claims right in front of it
			if list = i; claim != -1 {
				for j := claim; j < i; j++ {
					if strength(tags[j]) != strengthNone {
						claims--
					}
				}
				list = claim
			}
			break loop
		case tag.Is(TagTypeText) && !tagWord(tag.v[0]):
			words++
			switch s := tag.v[0]; {
			case len(s) <= 4 && strings.Trim(s, "0123456789") == "":
				numbers++
			case len(s) == 8:
				sum = isChecksum(s)
			}
		case strength(tag) != strengthNone:
			if claims++; claim == -1 {
				claim = i
			}
			continue
		default:
			switch others++; {
			case tag.typ == TagTypeSeries && !tag.wroteEpisode():
				seasons++
			case tag.typ == TagTypeSeries:
				numbers++
			case tag.typ == TagTypeDate:
				dates++
			case tag.typ == TagTypeGroup:
				groups++
			}
		}
		claim = -1
	}
	kind := bracketTags
	switch {
	case words != 0 && list != end:
		kind = bracketHead
	case words != 0:
		kind = bracketPhrase
	case claims == 1 && others == 0:
		kind = bracketLone
	}
	for i := open; i < list; i++ {
		in[i] = kind
	}
	for i := list; i < end; i++ {
		in[i] = bracketList
	}
	role := uint8(roleTags)
	switch total := words + claims + others; {
	case list != end:
	case total == 0:
		role = roleNone
	case numbers+seasons == total:
		role = roleNumber
	case dates == total:
		role = roleYear
	case groups == 1 && total == 1:
		role = roleGroup
	case words == 1 && total == 1 && sum:
		role = roleSum
	// a title may name its season ('[Youjo Senki S2]')
	case words+seasons == total && words != 0:
		role = roleWords
	}
	return role, words
}

// isChecksum reports whether s reads as the checksum of a file: eight hex
// digits, one of them a number ('ABCD1234', not 'DEADBEEF').
func isChecksum(s string) bool {
	if len(s) != 8 {
		return false
	}
	var digit bool
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			digit = true
		case c|0x20 >= 'a' && c|0x20 <= 'f':
		default:
			return false
		}
	}
	return digit
}

// settleVideo decides the claims that settle left to the type of the release,
// which is read from the tags settle leaves. In a film, a series or an episode
// those are a music source among the words of the title and a streaming
// service or a channel that opens it. A word leaves its field, which is then
// read from the tags that are left.
func (b *TagBuilder) settleVideo(r *Release) {
	if !r.Type.Is(Movie, Series, Episode) {
		return
	}
	if musicSourceWords(r.tags) {
		r.Source = sourceOf(r.tags)
	}
	if i := openingCollection(r.tags); i != -1 {
		r.tags[i] = r.tags[i].As(TagTypeText, nil)
		r.Collection = collectionOf(r.tags)
	}
}

// musicSourceWords gives back to the title of a video the music sources
// written among its words, when another source is the video's
// ('The.CD.2020.1080p.WEB.H264-GRP', 'The.Last.CD.Shop.2020.1080p.WEB',
// 'The.DVB.2020.720p.BluRay'), and reports whether there was one. unset gives
// back only a source that is exclusive to music ('The.Cassette.2020.1080p.
// WEB'). A music source alone in the name stays its source, as a broadcast is
// one a video is taken from ('Show.S01E02.DVB.x264-GRP',
// 'Show.Name.DVB.S01E01.720p.x264').
//
// The words start after the brackets that open the name ('[Group] The CD - 01
// [1080p WEB-DL]'), and end at a delimiter other than a separator: a source in
// parentheses after the title is not one of them ('Artist - Live At Wembley
// (CD) 2020 1080p WEB').
func musicSourceWords(tags []Tag) bool {
	first, end, text := -1, len(tags), false
loop:
	for i, tag := range tags {
		switch {
		case tag.Is(TagTypeText):
			text = true
		case !text, tag.Is(TagTypeWhitespace):
		case tag.Is(TagTypeSource) && tag.InfoType() == Music:
			if first == -1 {
				first = i
			}
		case !tag.Is(TagTypeDelim) || strings.Trim(tag.Delim(), " ._") != "":
			end = i
			break loop
		}
	}
	if first == -1 || !slices.ContainsFunc(tags[end:], func(tag Tag) bool {
		return tag.Is(TagTypeSource) && tag.InfoType() != Music
	}) {
		return false
	}
	for i := first; i < end; i++ {
		if tags[i].Is(TagTypeSource) {
			tags[i] = tags[i].As(TagTypeText, nil)
		}
	}
	return true
}

// openingCollection returns the position of a collection that opens the title
// of a video, or -1. A show or a film named after a channel is not taken from
// it ('CNBC.Titans.S01E01', 'NOW.Apocalypse.S01E01', 'ESPN.30.for.30.S01E01',
// 'BBC.The.Hunt.2015'), and the collection is then the next one in the name
// ('CNBC.Titans.S01E01.1080p.AMZN.WEB-DL').
//
// The words start after the bracket that opens the name ('[Group] CNBC Titans
// - 01 [1080p]'), and the code must be followed by a word across a plain
// delimiter: a code set off from the title is a label ('IMAX.-.Journey.to.the.
// South.Pacific.2013'), and one followed by other tags is no title
// ('AMZN.S01E01'). An F1 broadcaster is never a word ('F1TV.Formula1.2024.
// Round19'). A publisher of a tutorial or a book is not a video, and stays
// its collection ('Lynda.com.Setting.Up.MySQL').
func openingCollection(tags []Tag) int {
	// a bracket is a meta tag, or delimiters around tags ('[DSD] Show - 01',
	// '[Group].[1080p].Show.-.01', '[Group] [1080p] [WEB-DL] Show - 01'). A
	// delimiter that closes one bracket can open the next ('] [')
	i, inside := 0, false
loop:
	for ; i < len(tags); i++ {
		switch tag := tags[i]; {
		case tag.Is(TagTypeMeta):
		case tag.Is(TagTypeWhitespace, TagTypeDelim):
			d := tag.Delim()
			if !inside && strings.Trim(d, " ._[]") != "" {
				return -1
			}
			if k := strings.LastIndexAny(d, "[]"); k != -1 {
				inside = d[k] == '['
			}
		case inside:
		default:
			break loop
		}
	}
	if i+2 >= len(tags) || !tags[i].Is(TagTypeCollection) || service(tags[i]) || !tags[i+1].Is(TagTypeDelim) ||
		strings.Trim(tags[i+1].Delim(), " ._") != "" || !tags[i+2].Is(TagTypeText) {
		return -1
	}
	return i
}

// sourceOf returns the source of a release with tags: the first one, or a more
// precise one after a CD or a DVD, see preciser.
func sourceOf(tags []Tag) string {
	var source string
	for _, tag := range tags {
		if tag.Is(TagTypeSource) && preciser(source, tag.Source()) {
			source = tag.Source()
		}
	}
	return source
}

// collectionOf returns the first collection in tags.
func collectionOf(tags []Tag) string {
	for _, tag := range tags {
		if tag.Is(TagTypeCollection) {
			return tag.Collection()
		}
	}
	return ""
}

// unsettle makes the platforms and the architectures that settle read as
// words tags again, and reports whether there was one. It is for a name with
// no other sign of what it is, where the word is the sign ('Some Tool
// MacOS'). A 'Windows' that settleWindows read as a word stays one.
func (b *TagBuilder) unsettle(r *Release) bool {
	var found bool
	for i := len(r.tags); i > 0; i-- {
		if r.tags[i-1].Was(TagTypePlatform, TagTypeArch) {
			r.tags[i-1], found = r.tags[i-1].Revert(), true
			switch r.tags[i-1].TagType() {
			case TagTypePlatform:
				r.Platform = r.tags[i-1].Platform()
			case TagTypeArch:
				r.Arch = r.tags[i-1].Arch()
			}
		}
	}
	return found
}

// settleDates decides which date of a name is the date of the release: the
// last one, which can end the title. The ones in front of it are words
// ('2001.A.Space.Odyssey.1968'), and are kept for the parts of the date they
// hold. A date that opens the name with no other after it is its title
// ('2012.1080p.BluRay.x264-GRP'), unless a parenthesis opens in front of it
// ('(2012) Title').
func (b *TagBuilder) settleDates(r *Release) {
	last := -1
	for i := r.end; i > 0; i-- {
		if r.tags[i-1].Is(TagTypeDate) {
			last = i - 1
			break
		}
	}
	if last == -1 {
		return
	}
	first := 0
	for ; first < r.end && r.tags[first].Is(TagTypeWhitespace, TagTypeDelim); first++ {
	}
	if first == last && !(peek(r.tags, first-1, TagTypeDelim) && strings.HasSuffix(r.tags[first-1].Delim(), "(")) {
		r.tags[first] = r.tags[first].As(TagTypeText, nil)
		return
	}
	r.dates = append(r.dates, last)
	for i := last; i > 0; i-- {
		if r.tags[i-1].Is(TagTypeDate) {
			r.tags[i-1], r.dates = r.tags[i-1].As(TagTypeText, nil), append(r.dates, i-1)
		}
	}
}

// settleWindows reads a bare 'Windows' as a word, unless it is an item of a
// list of tags in the brackets that close the name, which is where a tracker
// names the platform ('Some.Title-GROUP (2019) [Windows / Multi-Language /
// Full ISO]'). Anywhere else it is a word of the title ('Broken.Windows.2020',
// 'Artist - Open Windows (2019) [FLAC]', '(Windows Edition)'). So is it in a
// name with a video or an audio tag, which a film, an episode or a record has
// ('Movie.2019.1080p.WEB.H264-GRP [Windows]'). It goes back to plain text,
// which unsettle does not make a platform again, and it is a word for
// every zone that is read after it.
func (b *TagBuilder) settleWindows(r *Release) {
	media := slices.ContainsFunc(r.tags, func(tag Tag) bool {
		return tag.Is(
			TagTypeSeries,
			TagTypeSource,
			TagTypeResolution,
			TagTypeFrameRate,
			TagTypeCodec,
			TagTypeHDR,
			TagTypeBitDepth,
			TagTypeAudio,
			TagTypeChannels,
		)
	})
	for i, depth := 0, 0; i < len(r.tags); i++ {
		tag := r.tags[i]
		switch {
		case tag.Is(TagTypeDelim):
			depth = bracketDepth(tag.Delim(), depth)
		case tag.Is(TagTypePlatform) && tag.Platform() == "Windows" &&
			(media || depth == 0 || !listItem(r.tags, i) || !closingBrackets(r.tags, i, depth)):
			r.tags[i] = NewTag(TagTypeText, nil, []byte(tag.v[0]), []byte(tag.v[0]))
		}
	}
}

// listItem reports whether the tag at i is a whole item of a list in brackets,
// with a bracket, a '/' or a ',' on either side of it ('[Windows / English]').
func listItem(tags []Tag, i int) bool {
	if !peek(tags, i-1, TagTypeDelim) || !peek(tags, i+1, TagTypeDelim) {
		return false
	}
	prev, next := strings.TrimRight(tags[i-1].Delim(), " "), strings.TrimLeft(tags[i+1].Delim(), " ")
	return prev != "" && strings.IndexByte("[(/,", prev[len(prev)-1]) != -1 &&
		next != "" && strings.IndexByte("])/,", next[0]) != -1
}

// closingBrackets reports whether nothing but brackets follows the tag at i,
// which depth brackets hold.
func closingBrackets(tags []Tag, i, depth int) bool {
	for i++; i < len(tags); i++ {
		switch {
		case tags[i].Is(TagTypeDelim):
			depth = bracketDepth(tags[i].Delim(), depth)
		case tags[i].Is(TagTypeWhitespace, TagTypeExt):
		case depth == 0:
			return false
		}
	}
	return true
}

// langBeforeYear is the cell of the table in edgeWord for a language directly
// in front of a year: false reads it as a tag ('5.Headed.Shark.Attack.German.
// 2017'), which leaves 'Pardon.My.French.2021' with the title 'Pardon My',
// and true reads it as a word.
const langBeforeYear = false

// edgeWord reports whether the one claim at m, which stands between the last
// word of the title and a year or the numbering, is a word. It is what the
// name alone cannot say, so this is a table of what is likelier:
//
//	             before a year             before the numbering
//	language     tag ('German.2017')       word ('Going.Dutch.S01E01')
//	region       word ('Made.in.USA')      tag ('The.Office.US.S01E01')
//	'complete'   word ('Almost.Complete')  tag ('Complete.Series.1-6')
//	the others   word                      word
//
// For a region and a 'complete' the numbering may stand behind its year
// ('Ghosts.US.2021.S01E01'). A language in front of a year is in the first
// column whatever follows the year ('Stargate.SG1.German.2003.S7.D1'), and
// German in front of a numbering written in German is a tag ('Robocop.German.
// Staffel1', where 'Going.Dutch.Staffel.1' is a word). A language is one that
// is named: a word for how the release is dubbed or subtitled is a tag in
// both columns, see languageName.
//
// The table is for a claim that is written as the words of the title are.
// Three things make it a tag in any cell. It is written in capitals
// ('Night.1.REPACK.2023', 'Hotellet.DANiSH.S01'), which only a region is
// anywhere ('Made.in.USA'). Or no word of the title mixes capitals and small
// letters, so that the spelling says nothing ('THE.MOVIE.PROPER.2017',
// '2.22.3D.2008', 'the.office.german.s01e01'). Or its row spans more than one
// word ('Director's.Cut.2004').
//
// The caller has checked that the claim stands alone: two claims in a row
// are tags ('UNCUT.German.2016'), and so is one set off by a hyphen or a
// bracket ('-Proper-2012-GRP').
func edgeWord(tags []Tag, z zones, m int) bool {
	tag := tags[m]
	s := tag.v[0]
	if len(strings.FieldsFunc(s, isAnyDelim)) != 1 || tag.typ != TagTypeRegion && capitals(s) ||
		!slices.ContainsFunc(tags[z.start:m], func(t Tag) bool { return t.Is(TagTypeText) && mixedCase(t.v[0]) }) {
		return false
	}
	anchor := z.anchorTag(tags)
	numbering := anchor.Is(TagTypeSeries)
	// behind is the numbering, or the numbering right behind the year
	behind := numbering
	if sd, k, plain := z.side(tags, z.anchor, +1); !behind && plain && sd == sideTag {
		behind = tags[k].Is(TagTypeSeries)
	}
	switch tag.typ {
	case TagTypeLanguage:
		// a numbering written in a language says the release is in it
		// ('Robocop.German.Staffel1')
		if numbering && tag.Language() == "GERMAN" && len(anchor.v[0]) >= 7 && strings.EqualFold(anchor.v[0][:7], "staffel") {
			return false
		}
		return languageName(tag) && (numbering || langBeforeYear)
	case TagTypeRegion:
		return !behind
	case TagTypeOther:
		return !behind || tag.Other() != "COMPLETE"
	}
	return true
}

// mixedCase reports whether s has a capital and a small letter, as a word of
// a title written 'Like.This' has.
func mixedCase(s string) bool {
	var upper, lower bool
	for _, c := range s {
		upper, lower = upper || unicode.IsUpper(c), lower || unicode.IsLower(c)
	}
	return upper && lower
}

// word reports whether the claim at m is a word. in is the kind of bracket
// each tag stands in, edge the number of claims between the last word of the
// title and the anchor, and source whether a source stands behind the claim.
func word(tags []Tag, z zones, in []uint8, m, edge int, source bool) bool {
	tag := tags[m]
	if keeps(tag) {
		return false
	}
	// a record is marked as a bootleg between hyphens or in parentheses
	// ('-Bootleg-', '(Bootleg)'), and the word is a word anywhere else
	// ('The.Bootleg.Series')
	if tag.typ == TagTypeOther && tag.Other() == "BOOTLEG" && !wrapped(tags, m) {
		return true
	}
	// the spellings that are ordinary words are words anywhere in front of
	// the year or the numbering of a name ('House MD (2004)', 'Artist-Uncut-
	// WEB-2019', 'One Piece Season 12 - Amazon Lily [408-421]'), and so is
	// what a programme runs on in front of the numbering ('Halt.and.Catch.
	// Fire.Mac.S01E01'). Not in a bracket, which lists tags ('- 1157
	// [Web][MKV]')
	if m < z.dated && z.dated != z.end && (collisionProne(tag) || namedLikeWord(tag) || z.numbered && tag.typ.Is(TagTypeArch, TagTypePlatform)) &&
		in[m] == bracketNone {
		return true
	}
	weak := strength(tag) == strengthWeak
	left, li, lplain := z.side(tags, m, -1)
	right, ri, rplain := z.side(tags, m, +1)
	if weak && in[m] != bracketNone {
		// name is whether the claim can name something: a language can, and
		// a word for how the release is dubbed or subtitled cannot
		name := tag.typ != TagTypeLanguage || languageName(tag)
		switch {
		// the word behind a claim can belong to it, and the two are a tag
		// wherever the bracket stands ('[English Dub]', '(Eng Subs)', 'Title
		// [Unofficial English Subtitle Scripts][KOTEX]', '{English Subbed,
		// Missing Last Ep}')
		case right != sideWall && rplain && ri < z.end &&
			(tags[ri].Is(TagTypeText) && tagWord(tags[ri].v[0]) || tags[ri].Is(TagTypeLanguage) && !languageName(tags[ri])):
			return false
		// a bracket lists tags from its first source, audio or technical tag
		// on ('(BD 1080p HEVC English Judas)')
		case in[m] == bracketList:
			return false
		// a bracket of tags holds tags ('(English-DUB) + Movie [1080p]',
		// '(EN|FR|DE)'). Only a language with another kind of claim behind
		// it is read where the bracket stands, and that claim with it, as
		// the two may say where an edition is from: words inside the title
		// ('(Japanese_Import)-SRCS300-1997', '(Italian_Bonus_Track)-OST-IT-
		// 2006'), and tags where nothing but tags follow ('(Russian_
		// Reissue)-1999')
		case in[m] == bracketTags:
			if !pairedClaim(tags, z, m, li, ri, lplain, rplain) {
				return false
			}
		// a claim alone in its bracket is a tag ('[Netflix]', '(Complete)',
		// '(CR)'). Only a language is read where the bracket stands, as it
		// may say where an artist is from or what a song is sung in
		// ('German_Martinez_(DE)-Surgery_In_Los_Rios', 'The_Man_Machine_
		// (English)_Track_1'), and then not next to a bracket of tags
		// ('Sub[FRE][ENG][ITA][ARA].1080p')
		case in[m] == bracketLone:
			if !languageName(tag) || !peek(tags[:z.end], li, TagTypeText) || !peek(tags[:z.end], ri, TagTypeText) {
				return false
			}
		// a word behind the claim makes the two a phrase ('(Japanese
		// Version)', '(German Edition)', '(Danish Music Awards 2011)', '(AKA
		// Anime with NO FILLER)', '-2CD-(Dutch_Edition)-1997'). In front of
		// the tags a bracket lists, that holds only in front of the anchor
		// ('(Japanese_Release_CDS)-1997'): behind it the word is a note on
		// the tags ('- 14 (B-Global Donghua 1920x1080 HEVC AAC MKV)')
		case (in[m] == bracketPhrase || m < z.anchor) && name && rplain && ri > m+1 && (right == sideWord || right == sideTail):
			return true
		// in front of the anchor a bracket belongs to the title, and a
		// language behind a word in it is a name ('(Curated by Lawrence
		// English)', '(Hides Her Feelings in Russian)'). Any other claim
		// behind a word is what the word says of the release ('(Korean
		// Import)', '(Chinese Retail)'), and behind the anchor a language is
		// one too ('- 01[1280x720][Vietnamese_English]'). With an edition
		// claim behind it the language says where the edition is from, and
		// is read where the bracket stands ('(Ltd_Russian_Reissue)-2006',
		// '(Limited_Edition_Bonus_Japanese_Import)-JP-2002')
		case m < z.anchor && languageName(tag) && lplain && li < m-1 && left == sideWord && !tagWord(tags[li].v[0]) &&
			!(rplain && right == sideTag && editionClaim(tags[ri])):
			return true
		}
		// a word for how the release is dubbed or subtitled stays one in any
		// bracket
		if !name {
			return false
		}
	}
	// a tag of a scene record has a field to itself, see fieldWord
	if languageName(tag) && m < z.anchor && li == z.last && left == sideWord && fieldWord(tags, m) &&
		!(capitals(tag.v[0]) && slices.ContainsFunc(tags[z.start:m], func(t Tag) bool { return t.Is(TagTypeText) && mixedCase(t.v[0]) })) {
		return true
	}
	numbered := z.anchorTag(tags).Is(TagTypeDate, TagTypeSeries)
	switch {
	// title: a word follows before the anchor, in a bracket or not
	// ('Bang_(Japanese_Version)-JA-DVDRip', 'Buzzcocks-French_(US_Version)-
	// 1995', 'Lebedev_(RU)-English_Accent-WEB-2017'). What stands behind the
	// words of a title, directly in front of a bracket, and names no
	// language is a tag all the same, as it describes what the bracket lists
	// ('Cowboy Bebop Complete (Series + Specials)'). So is a claim with a
	// field to itself in front of the bracket ('Song-Remastered-(Japanese_
	// Release)-2004'). A claim that is all the title behind a hyphen, in
	// front of the bracket, is the title ('Artist-Remastered_(Live)-2003',
	// 'Artist - Remastered (Live) (2011)'), and one that an underscore joins
	// to the bracket is a word of the field of a record they share
	// ('Artist-The_Complete_(Studio_Recordings)-WEB')
	case z.start < m && m < z.last:
		if weak {
			if languageName(tag) || right != sideWall || in[m] != bracketNone {
				return true
			}
			d := tags[m+1].v[0]
			return !strings.HasPrefix(d, "-") && (strings.HasPrefix(d, "_") || strings.Contains(tags[m-1].v[0], "-")) && fieldOpens(tags, m)
		}
	// title edge: the anchor follows
	case z.last < m && m < z.anchor && z.close == z.anchor && numbered:
		if edge == 1 && left == sideWord && right == sideAnchor && lplain && rplain && edgeWord(tags, z, m) {
			return true
		}
	// title edge: the year follows in parentheses, where a language is what
	// it is in front of a year ('Amelie French (2001)'). In front of any
	// other bracket it is a tag ('Movie Title German (DL) 2019',
	// 'Show.Name.German.(S01).COMPLETE')
	case z.last < m && m < z.close && z.close+1 == z.anchor && z.anchorTag(tags).Is(TagTypeDate):
		if languageName(tag) && edge == 1 && left == sideWord && lplain && edgeWord(tags, z, m) {
			return true
		}
	// episode title: right after the anchor with two words behind, ie the
	// 'NO' in 'Show.S01E02.NO.WAY.OUT.1080p', the 'Hungarian' in
	// 'Formula1.2026.Hungarian.Grand.Prix.1080p' and the 'Japanese' in
	// 'Show.S48E02.Japanese.Breakfast-Arlo.Parks.1080p'. One with tags behind
	// it is a tag ('Show.S01E02.GERMAN.1080p'), and so is one with a single
	// word ('S01E02.English.Dub'). No bracket is between any of them, which
	// side reports as a wall
	case z.anchor < m && m < z.tech && left == sideAnchor && numbered && tag.typ.Is(TagTypeLanguage, TagTypeCollection, TagTypeRegion):
		if right == sideWord {
			if r2, _, _ := z.side(tags, ri, +1); r2 == sideWord {
				return true
			}
		}
	}
	between := left == sideWord && right == sideWord
	switch tag.typ {
	case TagTypeRegion, TagTypeLanguage, TagTypePlatform:
		// a claim on either side of a 'vs' is one of the two sides of a
		// match ('Canada.Vs.USA.1080p', 'Ajax.vs.PSV.720p')
		if left == sideWord && lplain && versus(tags, z, li) || right == sideWord && rplain && versus(tags, z, ri) {
			return true
		}
		// a region is a word between two words outside any bracket
		// ('Tennis.US.Open.2025'), where a note in brackets names the
		// region it means ('(Contains the UK dub)'). The words must have
		// been read as words, which keeps the region in 'Videogame EUR NSW'
		if tag.typ == TagTypeRegion {
			return between && lplain && rplain && !tags[li].Retyped() && !tags[ri].Retyped() && in[m] == bracketNone
		}
	case TagTypeSource:
		// a release names its source once amongst its tags: a bare web
		// after a word is a word when a source follows ('A.Dark.Web.1080p.
		// WEBRip')
		return tag.Source() == "WEB" && source && wordBefore(tags, m)
	case TagTypeCut:
		// 'Extended.Highlights' names the programme and not a cut of it
		// ('Arsenal.vs.Liverpool.Extended.Highlights.1080p')
		return strings.EqualFold(tag.v[0], "extended") && rplain && ri < z.end &&
			tags[ri].Is(TagTypeText, TagTypeOther) && strings.EqualFold(tags[ri].v[0], "highlights")
	case TagTypeAudio:
		// a bare 'dual' is the audio tag next to tags or a bracket, and a
		// word between two words ('Freestyle.Dual.Moguls')
		return strings.EqualFold(tag.v[0], "dual") && between && lplain && rplain
	}
	// a weak claim between two words is a word
	return weak && between
}

// pairedClaim reports whether the claim at m, in a bracket of tags, is one of
// a language and another kind of claim right behind it, with a word in front
// of the two: 'Horizons_(Japanese_Import)', 'Girls_2-(Italian_Bonus_Track)',
// 'Germans-(Russian_Reissue)'. Behind a tag the bracket holds tags like any
// other ('Mystical_Realms_EP_(Japanese_Import)'). li and ri are the
// neighbours of the claim and lplain and rplain whether a plain delimiter
// joins them, as side gives them. The second of the two may have been read as
// a word already.
func pairedClaim(tags []Tag, z zones, m, li, ri int, lplain, rplain bool) bool {
	other := func(t Tag) bool {
		typ := t.typ
		if typ == TagTypeText && t.Retyped() {
			typ = t.Prev()
		}
		return typ.Is(TagTypeOther, TagTypeCollection, TagTypePlatform, TagTypeArch, TagTypeSource, TagTypeAudio, TagTypeCut, TagTypeEdition, TagTypeGenre, TagTypeRegion)
	}
	first := m
	switch {
	case languageName(tags[m]):
		if !rplain || ri <= m || ri >= z.end || !other(tags[ri]) {
			return false
		}
	case lplain && li >= 0 && li < m && languageName(tags[li]) && other(tags[m]):
		first = li
	default:
		return false
	}
	_, k, _ := z.side(tags, first, -1)
	return k >= 0 && tags[k].Is(TagTypeText)
}

// recordWord reports whether tag names a record with the language in front
// of it: 'EP' or 'LP' ('The_Russian_EP', 'Excuse_My_English_EP') and a remix
// ('The_Italian_Remixes').
func recordWord(tag Tag) bool {
	switch tag.typ {
	case TagTypeSource:
		return tag.Source() == "EP" || tag.Source() == "LP"
	case TagTypeOther:
		return tag.Other() == "REMiX"
	}
	return false
}

// editionClaim reports whether tag is a claim that a language in front of it
// describes, as it says where an edition is from: 'Japanese_Import',
// 'Russian_Reissue', 'Spanish_Retail', 'French_CDM', 'Japanese_Bonus_Tracks'.
func editionClaim(tag Tag) bool {
	return strength(tag) != strengthNone && tag.typ != TagTypeLanguage && !recordWord(tag)
}

// fieldOpens reports whether the field of a scene record that the tag at m
// stands in opens after a hyphen: only underscores join it to what is in
// front of it, back to a delimiter with a hyphen.
func fieldOpens(tags []Tag, m int) bool {
	for i := m - 1; i >= 0; i-- {
		if d := tags[i].v[0]; tags[i].Is(TagTypeDelim) && strings.Trim(d, "_") != "" {
			return strings.Contains(d, "-")
		}
	}
	return false
}

// fieldWord reports whether the claim at m shares a field of a scene record
// with the word in front of it. Hyphens set off the fields of such a name and
// underscores join the words of one, and a tag has a field to itself ('-DE-',
// '-WEB-', '-Bootleg-'). So a language that an underscore joins to a word
// names the record: the 'English' of 'Fabri_Fibra-Speak_English-x264-IT-
// 2009', of 'Autoflo-Old_English-2004', of 'Hell_Driver-Excuse_My_English_EP-
// WEB-2016' and of 'Losoul-Broken_English_Dubs-(LOG89)-WEB', the 'Russian' of
// 'A2C-White_Russian-(PBR002)-WEB-2010', the 'Dutch' of 'Missy_Elliot-Pass_
// That_Dutch-ivtc-Mvrs-Svcd-2003'.
//
// The field opens after a hyphen and closes at a hyphen or a bracket with
// more than the group behind it, as the title of a record does. The name of a
// game does neither ('Wer_wird_Millionaer_PAL_GERMAN_PLUS4-PARADOX',
// 'EA_Sports_FC_24_In-Game_Commentary_Russian_DLC_NSW-SUXXORS').
//
// Behind the language the field holds at most what names a record with it,
// see recordWord, or a word that belongs to a tag. With any other claim
// behind it the language says where an edition is from, and is a tag
// ('The_Downward_Spiral_Japanese_Import-CD-1994', 'Brazilian_Soul_Japanese_
// Retail-2004', 'Title_French_CDM-2019'). It is one too when it closes the
// field and the tags name the same language again ('Some_Album_German-DE-
// WEB-2019', 'Finding_Home_Swedish-AUDiOBOOK-WEB-SE-2025').
//
// The caller leaves a language in capitals a tag in a name that is not
// written in them ('Acda_en_de_Munnik-Deel_2_Life_is_what_happens_to_you_
// DUTCH-2CD-2002').
func fieldWord(tags []Tag, m int) bool {
	if !peek(tags, m-1, TagTypeDelim) || strings.Trim(tags[m-1].v[0], "_") != "" {
		return false
	}
	// to where the field closes
	i, n := m+1, 0
	for ; i < len(tags); i++ {
		switch tag := tags[i]; {
		case tag.Is(TagTypeDelim) && strings.Trim(tag.v[0], "_") == "":
			continue
		case tag.Is(TagTypeDelim):
		case n < 2 && (tag.Is(TagTypeText) && tagWord(tag.v[0]) || recordWord(tag)):
			n++
			continue
		default:
			return false
		}
		break
	}
	if i+1 >= len(tags) || !strings.ContainsAny(tags[i].v[0], "-([{") || tags[i+1].Is(TagTypeGroup, TagTypeDelim, TagTypeWhitespace) {
		return false
	}
	if lang := tags[m].Language(); n == 0 && slices.ContainsFunc(tags[i+1:], func(t Tag) bool { return t.Is(TagTypeLanguage) && t.Language() == lang }) {
		return false
	}
	return fieldOpens(tags, m)
}

// wrapped reports whether the tag at m stands between two hyphens or in
// parentheses.
func wrapped(tags []Tag, m int) bool {
	if !peek(tags, m-1, TagTypeDelim) || !peek(tags, m+1, TagTypeDelim) {
		return false
	}
	prev, next := tags[m-1].Delim(), tags[m+1].Delim()
	return strings.HasSuffix(prev, "-") && strings.HasPrefix(next, "-") || strings.HasSuffix(prev, "(") && strings.HasPrefix(next, ")")
}

// wordBefore reports whether a word stands in front of the tag at m, across
// delimiters and brackets.
func wordBefore(tags []Tag, m int) bool {
	for m--; m >= 0; m-- {
		if !tags[m].Is(TagTypeDelim, TagTypeWhitespace) {
			return tags[m].Is(TagTypeText)
		}
	}
	return false
}

// versus reports whether the word at v stands between the two sides of a
// match ('Canada.Vs.USA').
func versus(tags []Tag, z zones, v int) bool {
	if !tags[v].Is(TagTypeText) || tags[v].Retyped() || !isVersus(tags[v].v[0]) {
		return false
	}
	for _, inc := range [...]int{-1, +1} {
		sd, k, plain := z.side(tags, v, inc)
		if !plain || sd == sideNone || sd == sideWall || !tags[k].Is(TagTypeText, TagTypeRegion, TagTypeLanguage, TagTypePlatform) {
			return false
		}
	}
	return true
}

// isVersus reports whether s is a word that stands between the two sides of a
// match.
func isVersus(s string) bool {
	switch len(s) {
	case 1:
		return s == "v" || s == "V" || s == "@"
	case 2:
		return strings.EqualFold(s, "vs") || strings.EqualFold(s, "at")
	case 5:
		return strings.EqualFold(s, "gegen")
	case 6:
		return strings.EqualFold(s, "contre")
	}
	return false
}

// service returns true when tag is the collection of an F1 broadcaster, which
// is not a word of a title, so it stays a collection between words ('Japan
// Qualifying F1TV Live 1080p', 'Race F1TV Live+International 2160p'). Other
// services are words when between words ('Show.S01E02.Watch.on.ZEE5.Today').
func service(tag Tag) bool {
	if !tag.Is(TagTypeCollection) {
		return false
	}
	switch tag.Collection() {
	case "F1TV", "SkyF1", "SkyF1UHD":
		return true
	}
	return false
}

// parenthesized reports whether the tag at i stands alone in parentheses, with
// at most spaces beside it ('(Comedy)', '( anime )').
func parenthesized(tags []Tag, i int) bool {
	return peek(tags, i-1, TagTypeDelim) && strings.HasSuffix(strings.TrimRight(tags[i-1].Delim(), "\t\n\f\r "), "(") &&
		peek(tags, i+1, TagTypeDelim) && strings.HasPrefix(strings.TrimLeft(tags[i+1].Delim(), "\t\n\f\r "), ")")
}
