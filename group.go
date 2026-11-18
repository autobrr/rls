package rls

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// The group of a release has one owner, which is this file. The group lexer
// guesses that what follows the last hyphen of a name is its group, and reads
// the rows of the group vocabulary. Everything else a name says about its
// group is read here, in three steps, each where what it needs is known:
//
//   - settleGroup, with the zones of the name, before the type is read from
//     the tags and the title from the text: whether the guess of the lexer is
//     the group or a word, and the '-GROUP' in front of brackets that the
//     lexer does not read.
//   - settleGroupType, with the type: a known group of another type, the tag
//     in the opening bracket of an episode, a source after the last hyphen of
//     a video.
//   - group, with the titles read, as the last word left over is only known
//     then: the group of the release from all the candidates, by rank, where
//     it was read, and what becomes of the site.
//
// Nothing else sets Release.Group, and nothing else makes a group tag or
// gives one another type.

// groupOrigin is where the group of a release was read.
type groupOrigin uint8

const (
	// groupNone is a release with no group.
	groupNone groupOrigin = iota
	// groupSuffix is what follows the last hyphen of the name, which the
	// group lexer read ('x264-GRP', 'x264 - YIFY').
	groupSuffix
	// groupKnown is a row of the group vocabulary ('-CODEX').
	groupKnown
	// groupBeforeBrackets is what follows a hyphen in front of the brackets
	// that close the name, which the group lexer does not read ('Some.Title-
	// GROUP (2019) [Windows]', 'H 264-VARYG (Alt Title, Multi-Subs)').
	groupBeforeBrackets
	// groupSource is a source or a streaming service after the last hyphen of
	// a video ('XviD-DVD', 'x264-TBS').
	groupSource
	// groupLeading is the bracket that opens the name ('[SubsPlease] Show -
	// 04', '[DSD] Show - 01').
	groupLeading
	// groupTrailing is the bracket that closes the name ('Show - 04
	// [1080p][Group].mkv').
	groupTrailing
	// groupWord is the last word that nothing else read ('Title PS4 GRP',
	// '(1080p x265 AAC Group)', '- Kyoshi Mura)').
	groupWord
)

// groupTag returns the position of the group tag of tags, or -1. With more
// than one it is the last.
func groupTag(tags []Tag) int {
	for i := len(tags) - 1; i >= 0; i-- {
		if tags[i].typ == TagTypeGroup {
			return i
		}
	}
	return -1
}

// settleGroup decides what the lexers left open about the group tag, with the
// zones of the name: a guess of the group lexer that is a word of the name is
// given back to it, and a '-GROUP' in front of brackets is made the group.
func (b *TagBuilder) settleGroup(r *Release, z zones) {
	b.handBack(r, z)
	b.suffixBeforeBrackets(r)
}

// handBack hands a guessed group back to the name when the name opens with
// its group in a bracket. The group lexer takes whatever follows the last
// hyphen, which is a group after a tag ('[TGx] Movie.1080p.x264-GRP'). It is
// the end of the title after a word ('[Commie] Hataraku Maou-sama!'). After a
// hyphen set off by spaces it is a note after an edition ('[Group] Movie -
// 30th Anniversary - Patch Subs'), the title of the episode after a series
// tag ('[Group] Show Episode 00 - Prologue'), and a language when the word
// names one ('[Group] Movie (BD 1080p) - VOSTFR'), which it also is right
// after another language ('VF-VOSTFR').
func (b *TagBuilder) handBack(r *Release, z zones) {
	// the site is the first tag of the lead, which ends where the title
	// starts
	i := 0
	for ; i < z.start && r.tags[i].Is(TagTypeWhitespace, TagTypeDelim); i++ {
	}
	if i >= z.start || !r.tags[i].Is(TagTypeMeta) {
		return
	}
	if k, _ := r.tags[i].Meta(); k != "site" {
		return
	}
	for g := i + 2; g < len(r.tags); g++ {
		if !r.tags[g].Is(TagTypeGroup) || r.tags[g].f != nil || !r.tags[g-1].Is(TagTypeDelim) {
			continue
		}
		// the hyphen is the last one of the delimiters in front of the
		// group, with the spaces after it
		d := r.tags[g-1].v[0]
		h := strings.LastIndexByte(d, '-')
		if h == -1 {
			continue
		}
		p := g - 2
		for ; p > i && r.tags[p].Is(TagTypeWhitespace, TagTypeDelim); p-- {
		}
		tight, spaced := d == "-", h+1 < len(d) && strings.HasSuffix(d[:h], " ")
		s, grp := r.tags[g].v[0], r.tags[g].v[1]
		var tag Tag
		switch {
		case r.tags[p].Is(TagTypeText), spaced && r.tags[p].Is(TagTypeSeries, TagTypeEdition):
			tag = NewTag(TagTypeText, nil, []byte(s), []byte(grp))
		case tight && p == g-2 && r.tags[p].Is(TagTypeOther) && afterBracket(r.tags[p-1]) && isInitials(r.tags[p].v[0], r.tags[i]):
			// a hyphenated word alone after the last bracket is the mark
			// of the group in the leading bracket when it begins with the
			// initials of that group ('[Hakata Ramen] Show (S2) 02 [1080p
			// HEVC] HR-DR'). Its first half only looks like an other tag.
			// Any other word there is the tag it looks like, with the
			// group after it ('[TGx] Movie (2020) [1080p] REPACK-GRP')
			r.tags[p] = r.tags[p].As(TagTypeText, nil)
			tag = NewTag(TagTypeText, nil, []byte(s), []byte(grp))
		case spaced || r.tags[p].Is(TagTypeLanguage):
			// a builder that was not initialized has no language rows
			if b.languagef == nil {
				continue
			}
			// not a row that takes any word with 'sub' or 'dub' in it
			if info := b.languagef.find(grp); info == nil || strings.Contains(info.Tag(), "$") {
				continue
			}
			tag = NewTag(TagTypeLanguage, b.languagef.fn, []byte(s), []byte(grp))
		default:
			continue
		}
		// the word is then read with the tags from the start of the name
		if r.tags[g] = tag; r.end == g-1 || r.end == g {
			r.end = g + 1
		}
	}
}

// suffixBeforeBrackets reads the group of a scene name that a tracker wrote
// out with brackets of tags after it, ie 'Some.Title-GROUP (2019) [Windows /
// Multi-Language / Full ISO]'. The group lexer reads no group in front of a
// bracket, nor in front of the first year, as a hyphen there is part of the
// title ('Spider-Man (2002) [1080p]', 'Artist-Album (2019) [FLAC]'). A
// platform or an arch in the brackets is what sets the scene name apart, as a
// film or a record names neither. A known group gives its type ('-CODEX').
//
// A game can still end its title in a hyphenated word ('Half-Life (1998)
// [Windows]'), so the word is the group only when it is a known group, when
// the name joins its words with dots or underscores as a scene name does, or
// when it is not written as a word is ('GROUP', 'TiNYiSO', not 'Life').
func (b *TagBuilder) suffixBeforeBrackets(r *Release) {
	if groupTag(r.tags) != -1 {
		return
	}
	o := slices.IndexFunc(r.tags, func(tag Tag) bool {
		return tag.Is(TagTypeDelim) && strings.ContainsAny(tag.Delim(), "()[]{}")
	})
	if o < 3 || !r.tags[o-1].Is(TagTypeText) || r.tags[o-1].Retyped() ||
		!r.tags[o-2].Is(TagTypeDelim) || r.tags[o-2].Delim() != "-" || r.tags[o-3].Is(TagTypeDelim, TagTypeWhitespace) {
		return
	}
	if s := strings.TrimLeft(r.tags[o].Delim(), " ._"); s == "" || (s[0] != '(' && s[0] != '[') {
		return
	}
	// only tags in brackets after it, and a file extension, one of which names
	// what it runs on. One in front of the year was read as a word, and may
	// yet be a tag again, see unsettle
	var platform bool
	for i, depth := o, 0; i < len(r.tags); i++ {
		switch {
		case r.tags[i].Is(TagTypeDelim):
			depth = bracketDepth(r.tags[i].Delim(), depth)
		case r.tags[i].Is(TagTypeWhitespace, TagTypeExt):
		case depth == 0:
			return
		default:
			platform = platform || r.tags[i].Is(TagTypePlatform, TagTypeArch) || r.tags[i].Was(TagTypePlatform, TagTypeArch)
		}
	}
	if !platform {
		return
	}
	tag := r.tags[o-1].as(TagTypeGroup, b.groupf)
	dotted := slices.ContainsFunc(r.tags[:o-2], func(tag Tag) bool {
		return tag.Is(TagTypeDelim) && (tag.Delim() == "." || tag.Delim() == "_")
	})
	if tag.Info() == nil && !dotted && plainWord.MatchString(tag.Group()) {
		return
	}
	r.tags[o-1], r.groupFrom = tag, groupBeforeBrackets
}

// plainWord matches a word written as words are, in lower case after its
// first letter.
var plainWord = regexp.MustCompile(`^[A-Za-z][a-z]*$`)

// isInitials reports whether s is the first letters of the words of the site
// that the meta tag site holds, without regard to case ('HR' of 'Hakata
// Ramen'). A site of one word has no initials to speak of.
func isInitials(s string, site Tag) bool {
	_, v := site.Meta()
	words := strings.FieldsFunc(v, func(r rune) bool {
		return r == ' ' || r == '_' || r == '.' || r == '-'
	})
	if len(words) < 2 || len(words) != len(s) {
		return false
	}
	for i, word := range words {
		if !strings.EqualFold(word[:1], s[i:i+1]) {
			return false
		}
	}
	return true
}

// afterBracket reports whether tag is a delimiter that closes a bracket and
// then a space ('] ', ') ').
func afterBracket(tag Tag) bool {
	if !tag.Is(TagTypeDelim) {
		return false
	}
	d := strings.TrimRight(tag.Delim(), " \t")
	return d != "" && len(d) < len(tag.Delim()) && strings.IndexByte("])}", d[len(d)-1]) != -1
}

// settleGroupType decides what the type of the release says about its group
// tag, once unset has given back the tags of other types.
//
// A known group of another type is no group ('-CODEX' on a film), and its
// word is left to the last step like any other.
//
// The word alone in the bracket that opens an episode or a series is its
// group, when unset gave it back as text ('[DSD] Show - 01 [1080p]', '[VBR]
// Show - 01 [1080p]'). The meta lexer leaves a word that is a short tag to
// its tag, and a tag of a record does not describe a video.
//
// A source or a collection after the last hyphen of a video name is its group
// ('Movie 1954 2160p Remux HEVC DTS-HD MA 5.1-VHS', 'Movie.1985.DVDRip.XviD-
// DVD', 'Show.S25.720p.WEB-DL.x264-TBS'). The group lexer leaves a word that
// is a short tag to its tag, and a source or a streaming service written last
// does not describe the video: a video name writes them in front of the codec
// and the audio. A codec or a resolution in front of the hyphen is what makes
// it a video name. A record ends in its source ('Artist-Title-(ID).2019-
// WEB'), and so does a name with nothing in front of the source
// ('Movie.1985-VHS'). The source and the collection of the release are then
// read from the tags that are left.
func (b *TagBuilder) settleGroupType(r *Release) {
	for i, tag := range r.tags {
		if tag.typ == TagTypeGroup && tag.InfoExcl() && tag.InfoType() != r.Type {
			r.tags[i], r.groupFrom = tag.As(TagTypeText, nil), groupNone
		}
	}
	if groupTag(r.tags) != -1 {
		return
	}
	if r.Type.Is(Series, Episode) && len(r.tags) >= 4 &&
		r.tags[0].Is(TagTypeDelim) && r.tags[0].Delim() == "[" && r.tags[1].Is(TagTypeText) &&
		r.tags[2].Is(TagTypeDelim) && strings.HasPrefix(r.tags[2].Delim(), "]") {
		r.tags[1], r.groupFrom = r.tags[1].as(TagTypeGroup, nil), groupLeading
		return
	}
	if !r.Type.Is(Movie, Series, Episode) {
		return
	}
	i := lastTag(r.tags)
	if i < 2 || !r.tags[i].Is(TagTypeSource, TagTypeCollection) || !r.tags[i-1].Is(TagTypeDelim) || r.tags[i-1].Delim() != "-" ||
		!slices.ContainsFunc(r.tags[:i-1], func(tag Tag) bool { return tag.Is(TagTypeCodec, TagTypeResolution) }) {
		return
	}
	r.tags[i], r.groupFrom = r.tags[i].as(TagTypeGroup, nil), groupSource
	r.Source, r.Collection = sourceOf(r.tags), collectionOf(r.tags)
}

// lastTag returns the position of the last tag of the name: the one in front
// of the file extension and its dot, when the name has one, and of the
// whitespace that ends the name. It is -1 for a name of nothing else.
func lastTag(tags []Tag) int {
	i := len(tags) - 1
	for ; i >= 0 && tags[i].Is(TagTypeWhitespace); i-- {
	}
	if i >= 2 && tags[i].Is(TagTypeExt) && tags[i-1].Is(TagTypeDelim) && tags[i-1].Delim() == "." {
		i -= 2
	}
	for ; i >= 0 && tags[i].Is(TagTypeWhitespace); i-- {
	}
	return i
}

// group decides the group of the release, where it was read, and what becomes
// of the site. It takes the first of the candidates there is, in this order:
//
//  1. The group tag: what the lexer read after the last hyphen or as a row of
//     the group vocabulary, and what settleGroup and settleGroupType made one.
//  2. The site in the bracket that opens the name ('[SubsPlease] Show - 04'),
//     past the flags in front of it ('[REPACK] [SubsPlease] Show - 04').
//     Fansub names carry the group there, and it comes before the words left
//     over so that none of those is taken for it.
//  3. A word left over that a hyphen joins to the tags, with a bracket after
//     it ('H 264-VARYG (Alt Title, Multi-Subs)'). The group lexer does not
//     read a name that ends in a bracket, and the last word left over of such
//     a name is inside the bracket.
//  4. The last word left over ('Title PS4 GRP', '(1080p x265 AAC Group)'),
//     with the words in front that belong to it, see widenGroup. Not a word
//     that reads as the checksum of the file, which unused takes, one that a
//     tag follows in its bracket, an item of a list, a word amid the tags, or
//     one that belongs to the tag in front of it ('[English Dub]').
//  5. The site in the bracket that closes the name ('Show - 04
//     [1080p][Group].mkv'). It comes last, so that a group named in the tags
//     is preferred ('(BD 1080p - Encoder)[Uploader]').
//
// A group that came from a word left over takes the word out of the unused
// text. The site stays the site, but for a leading bracket of CJK script in
// front of a scene name, see leadingAlt.
func (b *TagBuilder) group(r *Release) {
	switch i := groupTag(r.tags); {
	case i != -1:
		if r.Group = r.tags[i].Group(); r.groupFrom == groupNone {
			r.groupFrom = groupSuffix
			if r.tags[i].Info() != nil {
				r.groupFrom = groupKnown
			}
		}
	case leadingSite(r):
		r.Group, r.groupFrom = r.Site, groupLeading
	case b.bracketedGroup(r):
		r.groupFrom = groupBeforeBrackets
	case b.lastWord(r):
		r.groupFrom = groupWord
	case trailingSite(r):
		r.Group, r.groupFrom = r.Site, groupTrailing
	}
	leadingAlt(r)
}

// leadingSite reports whether the site of the release is the bracket that
// opens the name, past the flags in front of it.
func leadingSite(r *Release) bool {
	if r.Site == "" {
		return false
	}
	for _, tag := range r.tags {
		switch {
		// only the meta lexer puts an other tag in front of the site, and
		// only a flag
		case tag.Is(TagTypeWhitespace, TagTypeDelim, TagTypeOther):
			continue
		case tag.Is(TagTypeMeta):
			k, v := tag.Meta()
			return k == "site" && v == r.Site
		}
		return false
	}
	return false
}

// trailingSite reports whether the site of the release is the bracket that
// closes the name. A site given by name ('[[site:name]]', '-={name}=-') is
// not a group.
func trailingSite(r *Release) bool {
	if r.Site == "" {
		return false
	}
	for _, tag := range slices.Backward(r.tags) {
		switch {
		case tag.Is(TagTypeWhitespace, TagTypeDelim, TagTypeExt):
			continue
		case tag.Is(TagTypeMeta):
			if k, v := tag.Meta(); k == "site" && v == r.Site {
				s := tag.v[0]
				return strings.HasPrefix(s, "[") && !strings.HasPrefix(s, "[[")
			}
			continue
		}
		return false
	}
	return false
}

// bracketedGroup sets the group from a leftover word that a hyphen joins to
// the tags and a bracket follows, and reports whether there is one.
func (b *TagBuilder) bracketedGroup(r *Release) bool {
	for k, i := range slices.Backward(r.unused) {
		if i < 2 || !r.tags[i-1].Is(TagTypeDelim) {
			continue
		}
		if d := r.tags[i-1].Delim(); !strings.HasSuffix(d, "-") || strings.ContainsAny(d, "()[]{}") || !r.tags[i-2].Is(
			TagTypeSource,
			TagTypeResolution,
			TagTypeFrameRate,
			TagTypeCollection,
			TagTypeCodec,
			TagTypeHDR,
			TagTypeBitDepth,
			TagTypeAudio,
			TagTypeChannels,
			TagTypeOther,
			TagTypeCut,
			TagTypeEdition,
			TagTypeLanguage,
			TagTypeContainer,
		) {
			continue
		}
		// a group may itself hold hyphens ('-Tsundere-Raws')
		last, n := i, k+1
		for ; n < len(r.unused) && r.unused[n] == last+2 && r.tags[last+1].Is(TagTypeDelim) && r.tags[last+1].Delim() == "-"; n++ {
			last += 2
		}
		if !peek(r.tags, last+1, TagTypeDelim) || !strings.ContainsAny(r.tags[last+1].Delim(), "([{") {
			continue
		}
		var sb strings.Builder
		for j := i; j <= last; j++ {
			sb.WriteString(r.tags[j].Text())
		}
		r.Group, r.unused = sb.String(), slices.Delete(r.unused, k, n)
		return true
	}
	return false
}

// lastWord sets the group from the last word left over, and reports whether
// that word can be one.
func (b *TagBuilder) lastWord(r *Release) bool {
	n := len(r.unused)
	if n == 0 {
		return false
	}
	last := r.unused[n-1]
	s := r.tags[last].Text()
	if r.Sum == "" && b.checksum(s) || !strings.ContainsFunc(s, unicode.IsLetter) ||
		beforeTag(r.tags, last) || plusList(r.tags, last) || amidTags(r.tags, last) || afterTag(r.tags, last) {
		return false
	}
	r.Group, r.unused = s, r.unused[:n-1]
	b.widenGroup(r, last)
	return true
}

// hyphenGroup bounds a group widened from leftover words that follow a hyphen,
// as in "(... - Kyoshi Mura)": two words of any length that each contain a
// letter (ie, 'Kyoshi Mura', 'Ky0_Mura'). Leftover words have already failed
// to lex as tags, so it can be looser than the group lexer's bound.
var hyphenGroup = regexp.MustCompile(`(?i)^[a-z0-9]*[a-z][a-z0-9]*[ _][a-z0-9]*[a-z][a-z0-9]*$`)

// widenGroup joins the leftover words in front of a guessed group, so
// "(... - DarQ HONE)" yields the same group as "-DarQ HONE". Words right after
// a hyphen may be separated by single spaces or underscores, within
// hyphenGroup or spacedGroup. Other words join across single spaces within
// spacedGroup only, so trailing alternate titles like "(Meitantei Conan)" are
// not taken as a group. A run outside the bound keeps the single word.
func (b *TagBuilder) widenGroup(r *Release, i int) {
	// a comic names its scan group in the parentheses that close the name,
	// and the hyphen is part of it ('(Scan-Team).cbr')
	if n, j := widenRun(r, i, "-"); r.Type == Comic && n < len(r.unused) && j > 0 &&
		strings.HasSuffix(r.tags[j-1].Text(), "(") && peek(r.tags, i+1, TagTypeDelim) && strings.HasPrefix(r.tags[i+1].Text(), ")") {
		r.Group, r.unused = joinRun(r, n), r.unused[:n]
		return
	}
	if n, j := widenRun(r, i, " _"); n < len(r.unused) && j > 0 && strings.Contains(r.tags[j-1].Text(), "-") {
		if group := joinRun(r, n); hyphenGroup.MatchString(group) || spacedGroup.MatchString(group) {
			r.Group, r.unused = group, r.unused[:n]
			return
		}
	}
	if n, _ := widenRun(r, i, " "); n < len(r.unused) {
		if group := joinRun(r, n); spacedGroup.MatchString(group) {
			r.Group, r.unused = group, r.unused[:n]
		}
	}
}

// widenRun returns where the run of leftover words in front of the guessed
// group at tag i starts, as an index into r.unused and into r.tags, when
// single delimiters from seps separate the words.
func widenRun(r *Release, i int, seps string) (int, int) {
	n := len(r.unused)
	for ; n > 0 && r.unused[n-1] == i-2 && len(r.tags[i-1].Text()) == 1 && strings.Contains(seps, r.tags[i-1].Text()); n-- {
		i = r.unused[n-1]
	}
	return n, i
}

// joinRun joins the leftover words from n in r.unused onto the guessed group,
// keeping their delimiters.
func joinRun(r *Release, n int) string {
	var sb strings.Builder
	for _, j := range r.unused[n:] {
		sb.WriteString(r.tags[j].Text())
		sb.WriteString(r.tags[j+1].Text())
	}
	return sb.String() + r.Group
}

// beforeTag reports whether a tag follows the text at i inside the bracket
// that holds it, ie the 'HEVC2' in '(1280x720 HEVC2 AAC)' or the 'Conan' in
// '(Meitantei Conan, Multi-Subs)'. Such a word is not the group, which a block
// of tags names last ('(1080p x265 AAC Group)').
func beforeTag(tags []Tag, i int) bool {
	var tagged bool
	for i++; i < len(tags); i++ {
		switch {
		case !tags[i].Is(TagTypeDelim):
			tagged = tagged || !tags[i].Is(TagTypeText, TagTypeWhitespace)
		case strings.ContainsAny(tags[i].Delim(), "([{"):
			return false
		case strings.ContainsAny(tags[i].Delim(), ")]}"):
			return tagged
		}
	}
	return false
}

// afterTag reports whether the text at i is a word that belongs to the tag in
// front of it, ie the 'Sub' of 'Ultra Maniac Eng Sub' and the 'Dub' of
// '[English Dub][1080p]', see tagWord. Such a word is not the group.
func afterTag(tags []Tag, i int) bool {
	return tagWord(tags[i].Text()) && i >= 2 && tags[i-1].Is(TagTypeDelim) && strings.Trim(tags[i-1].Delim(), "._ ") == "" &&
		tags[i-2].Is(TagTypeLanguage, TagTypeAudio)
}

// amidTags reports whether the text at i sits inside the run of tags: a tag
// before it, an audio or video tag after it, and a single dot, underscore or
// space on each side ('TrueHD Audio.1080p', '1080p IT BD'). Such a word is a
// tag the vocabulary lacks, not the group, which comes after the tags
// ('H264.GRP', 'H264 GRP') or after a hyphen ('x264 - BTN 4K').
func amidTags(tags []Tag, i int) bool {
	if i < 2 || i+2 >= len(tags) {
		return false
	}
	for _, tag := range []Tag{tags[i-1], tags[i+1]} {
		if !tag.Is(TagTypeDelim, TagTypeWhitespace) || len(tag.Text()) != 1 || !strings.Contains("._ ", tag.Text()) {
			return false
		}
	}
	return !tags[i-2].Is(TagTypeText, TagTypeDelim, TagTypeWhitespace) &&
		tags[i+2].Is(TagTypeSource, TagTypeResolution, TagTypeCodec, TagTypeHDR, TagTypeAudio, TagTypeChannels)
}

// plusList reports whether the text at i is in a list that a '+' joins on,
// ie the 'Specials' in 'S01 + Specials' and in 'S01 + Movies & Specials', or
// the 'Movie' in '[720p] + Movie'. Such a word is not the group. A '+' with no
// space beside it counts only in a name that separates its words some other
// way, which 'Artist+-+Album+(ripper)' does not.
func plusList(tags []Tag, i int) bool {
	for i--; i >= 0 && tags[i].Is(TagTypeText, TagTypeDelim); i-- {
		if !tags[i].Is(TagTypeDelim) {
			continue
		}
		// see seasonPlus
		if seasonPlus(tags, i) {
			return false
		}
		// what follows the last bracket
		d := tags[i].Delim()
		k := strings.LastIndexAny(d, "()[]{}")
		switch s := d[k+1:]; {
		case strings.TrimSpace(s) != "+":
		case s != "+":
			return true
		default:
			return slices.ContainsFunc(tags, func(tag Tag) bool {
				return tag.Is(TagTypeDelim) && (tag.Delim() == " " || tag.Delim() == ".")
			})
		}
		if k != -1 {
			return false
		}
	}
	return false
}

// leadingAlt reads a leading bracket of CJK script as the title in that
// script when a scene group closes the name ('[港灯].Harbor.Lights.S01-GRP'),
// where the bracket would otherwise be the site. It is left alone when it is
// the group ('[桜都字幕组] Show - 01 [1080p]'), when it names a fansub group
// ('[桜都字幕组] Show.S01E01.1080p.WEB-DL-GRP'), and when another bracket or a
// title in CJK script follows it.
func leadingAlt(r *Release) {
	if len(r.Alt) != 0 || r.Site == "" || r.Group == "" || r.Group == r.Site ||
		strings.ContainsFunc(r.Title, isCJK) || !cjkOnly(r.Site) || fansubMark(r.Site) {
		return
	}
	i := 0
	for ; i < len(r.tags) && r.tags[i].Is(TagTypeWhitespace, TagTypeDelim); i++ {
	}
	if i == len(r.tags) || !r.tags[i].Is(TagTypeMeta) || !strings.HasPrefix(r.tags[i].v[0], "[") {
		return
	}
	if k, v := r.tags[i].Meta(); k != "site" || v != r.Site {
		return
	}
	for i++; i < len(r.tags) && r.tags[i].Is(TagTypeWhitespace, TagTypeDelim); i++ {
		if strings.ContainsAny(r.tags[i].v[0], "[(【（") {
			return
		}
	}
	if i == len(r.tags) || !r.tags[i].Is(TagTypeText) {
		return
	}
	r.Alt, r.Site = []string{r.Site}, ""
}

// isCJK reports whether c is a letter of a CJK script.
func isCJK(c rune) bool {
	return unicode.In(c, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// cjkOnly reports whether s is written in CJK script, with spaces and CJK
// punctuation ('港灯', 'ハーバー・ライツ', '항구의 불빛').
func cjkOnly(s string) bool {
	return strings.ContainsFunc(s, isCJK) && !strings.ContainsFunc(s, func(c rune) bool {
		return !isCJK(c) && c != ' ' && c != '·' && !('　' <= c && c <= '〿') && c != '・' && c != 'ー'
	})
}

// fansubMark reports whether s names a fansub group: subtitles (字幕),
// translation (汉化, 漢化), a studio (工作室), or a group or club (组, 組, 社).
func fansubMark(s string) bool {
	return strings.Contains(s, "字幕") || strings.Contains(s, "汉化") || strings.Contains(s, "漢化") ||
		strings.Contains(s, "工作室") || strings.HasSuffix(s, "组") || strings.HasSuffix(s, "組") || strings.HasSuffix(s, "社")
}
