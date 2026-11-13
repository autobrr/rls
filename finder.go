package rls

import (
	"regexp/syntax"
	"strings"
	"sync/atomic"

	"github.com/autobrr/rls/taginfo"
)

// A finder finds the row of a tag type that matches a whole string, as
// taginfo.Find does, without the linear scan and, for most rows, without a
// regexp.
//
// taginfo.Find tests every row's compiled regexp in turn -- 123 of them for
// source -- and a tag is looked up twice when it is made, once for its
// captured value and once for its normalized value.
//
// Each row's regexp is fully anchored, so the strings it can match are exactly
// the spellings it expands to. Indexing those by upper cased spelling narrows a
// lookup to a handful of candidate rows. A candidate that is a short list of
// ASCII spellings is confirmed by comparing bytes, each position against the
// set of bytes the row's own regexp allows there, so a row that is case
// sensitive in part ('(?-i:US|USA)') still decides for itself. Any other row,
// and any string with a byte outside ASCII, is confirmed by the row's regexp.
// Rows that could not be expanded stay in an ordered fallback list, and
// candidates are visited in row order, so for ASCII text the row selected is
// the one the scan would have returned.
//
// The index is upper cased byte for byte, which is not how (?i) folds: 'Opuſ'
// (U+017F) matches the OPUS row's regexp and is not found here.
//
// A finder is used by every parse of its parser at once. What it learns as it
// goes, the spellings of a row and the row of a normalized value, is the same
// whichever parse learns it, and is stored with one atomic write.
type finder struct {
	infos []*taginfo.Taginfo
	exact map[string][]int32
	rest  []int32
	// fn is find as a func, which is what a tag holds.
	fn taginfo.FindFunc
	// spelled are the spellings of each row, read off its regexp when the row
	// is first a candidate.
	spelled []atomic.Pointer[rowSpellings]
	// typ is the tag type the rows are of, or noType.
	typ TagType
	// fixed is whether the normalized value of a tag of typ with a row is
	// the same whatever its captured value, and norm the row that value is
	// found as, which is the second lookup of a tag. norm is filled in as the
	// rows are met: nil is a row not yet met, and noInfo one whose normalized
	// value is no row.
	fixed []bool
	norm  []atomic.Pointer[taginfo.Taginfo]
}

// rowSpellings are the strings a row matches, as strings of indexes into
// classes. strs is nil for a row that is confirmed by its regexp.
type rowSpellings struct {
	strs    []string
	classes []byteClass
}

// noType is the type of a finder whose rows are of no one tag type.
const noType TagType = -1

// noInfo stands in finder.norm for a normalized value that no row matches.
var noInfo = new(taginfo.Taginfo)

// noSpellings makes the finders confirm every row with its regexp and look
// every normalized value up, for a test to compare a finder built that way
// with one that compares bytes.
var noSpellings bool

// newFinder returns the finder for the rows of a tag type. typ is noType when
// the rows are of no one type.
func newFinder(typ TagType, infos ...*taginfo.Taginfo) *finder {
	f := &finder{
		infos:   infos,
		exact:   make(map[string][]int32),
		spelled: make([]atomic.Pointer[rowSpellings], len(infos)),
		typ:     typ,
		fixed:   make([]bool, len(infos)),
		norm:    make([]atomic.Pointer[taginfo.Taginfo], len(infos)),
	}
	f.fn = f.find
	for i, info := range infos {
		// a placeholder in the tag is filled from the captured value
		f.fixed[i] = normByRow(typ) && !strings.Contains(info.Tag(), "$") && !noSpellings
		if noSpellings {
			f.spelled[i].Store(new(rowSpellings))
		}
		v, err := expandRE(info.RE(), true)
		if err != nil {
			f.rest = append(f.rest, int32(i))
			continue
		}
		for _, s := range v {
			if s == "" {
				continue
			}
			k := strings.ToUpper(s)
			if c := f.exact[k]; len(c) == 0 || c[len(c)-1] != int32(i) {
				f.exact[k] = append(c, int32(i))
			}
		}
	}
	return f
}

// findFunc is taginfo.Find with the linear scan indexed away, see finder.
func findFunc(infos ...*taginfo.Taginfo) taginfo.FindFunc {
	return newFinder(noType, infos...).fn
}

// normByRow reports whether the normalized value of a tag of typ is the tag
// of the row its captured value is found as, or what the type makes of that,
// and so the same for every tag with that row.
func normByRow(typ TagType) bool {
	switch typ {
	case TagTypePlatform, TagTypeArch, TagTypeSource, TagTypeResolution, TagTypeCollection,
		TagTypeCodec, TagTypeHDR, TagTypeBitDepth, TagTypeAudio, TagTypeChannels, TagTypeOther,
		TagTypeCut, TagTypeEdition, TagTypeLanguage, TagTypeSize, TagTypeRegion, TagTypeContainer,
		TagTypeGenre:
		return true
	}
	return false
}

// spellings returns the spellings of row i, as the tag info matches the row:
// without regard to case, and whole.
func (f *finder) spellings(i int32) *rowSpellings {
	if sp := f.spelled[i].Load(); sp != nil {
		return sp
	}
	sp := new(rowSpellings)
	// a row that is no regexp by itself ('a)|(?:b') parses inside the group
	// below as something other than what the tag info matches with, so it is
	// left to its regexp
	if _, err := syntax.Parse(f.infos[i].RE(), syntax.Perl); err == nil {
		if tree, err := syntax.Parse(`(?i)(?:`+f.infos[i].RE()+`)`, syntax.Perl); err == nil {
			var cs classSet
			if strs, ok := cs.spell(tree); ok && len(strs) != 0 {
				sp.strs, sp.classes = strs, cs.classes
			}
		}
	}
	f.spelled[i].Store(sp)
	return sp
}

// has reports whether s, which is ASCII, is one of the spellings.
func (sp *rowSpellings) has(s string) bool {
	for _, str := range sp.strs {
		if len(str) != len(s) {
			continue
		}
		l := 0
		for l < len(str) && sp.classes[str[l]].has(s[l]) {
			l++
		}
		if l == len(str) {
			return true
		}
	}
	return false
}

// index returns the first row that matches s, or -1.
func (f *finder) index(s string) int {
	// upper case into a stack buffer: the map lookup below then avoids the
	// allocation strings.ToUpper would make
	var key [48]byte
	var cand []int32
	ascii := len(s) <= len(key)
	if ascii {
		for j := 0; j < len(s); j++ {
			c := s[j]
			if c >= 'a' && c <= 'z' {
				c -= 32
			}
			ascii = ascii && c < 0x80
			key[j] = c
		}
		cand = f.exact[string(key[:len(s)])]
	} else {
		cand = f.exact[strings.ToUpper(s)]
	}
	// walk cand and rest together in row order
	for a, b := 0, 0; a < len(cand) || b < len(f.rest); {
		var i int32
		if b >= len(f.rest) || (a < len(cand) && cand[a] < f.rest[b]) {
			i, a = cand[a], a+1
		} else {
			i, b = f.rest[b], b+1
		}
		if ascii {
			if sp := f.spellings(i); sp.strs != nil {
				if sp.has(s) {
					return int(i)
				}
				continue
			}
		}
		if f.infos[i].Match(s) {
			return int(i)
		}
	}
	return -1
}

// find returns the first row that matches s, or nil.
func (f *finder) find(s string) *taginfo.Taginfo {
	if i := f.index(s); i != -1 {
		return f.infos[i]
	}
	return nil
}

// resolve sets the rows of tag, whose find func is that of f: the row its
// captured value is found as, and the row its normalized value is found as.
// It gives what the two lookups of Tag.resolve give.
//
// The second lookup is made once for a row. The normalized value of a tag
// with a row is what the type makes of the tag of that row, whatever the
// captured value was, unless the row has a placeholder ('MULTi$1'), which is
// filled from the captured value. So the row it is found as is kept with the
// row, for a tag of the type the finder is for.
func (f *finder) resolve(tag *Tag) {
	i := f.index(tag.val(1))
	if i == -1 {
		tag.ninfo, tag.info = nil, f.find(tag.Normalize())
		return
	}
	tag.ninfo = f.infos[i]
	if !f.fixed[i] || tag.typ != f.typ || len(tag.v) < 2 {
		tag.info = f.find(tag.Normalize())
		return
	}
	info := f.norm[i].Load()
	if info == nil {
		if info = f.find(tag.Normalize()); info == nil {
			info = noInfo
		}
		f.norm[i].Store(info)
	}
	if info == noInfo {
		info = nil
	}
	tag.info = info
}
