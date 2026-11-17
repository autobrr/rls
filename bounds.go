package rls

import (
	"slices"
	"strings"
	"unsafe"
)

// A lexer proves that its token ended by reading what follows it, and a
// regexp without lookahead takes the delimiter it read: 'S01 ' for the season
// of 'Show S01 1080p', 'DTS-HD.MA.' in front of '5.1', '[Group] ' in front of
// a title, '.mkv', ' YIFY' after ' - '. Such a delimiter is no part of the
// tag. join gives it a delimiter tag, so that the tags of a name are its
// values with every delimiter between them a tag of its own, and the text of
// a delimiter tag is the whole run of delimiters at that place, as next reads
// one.

// ellipsis is the run of delimiters that is a tag of its own wherever it
// begins a run ('Title...2019' and 'Title....2019', which is '...' and '.').
const ellipsis = "..."

// join returns the tags of a parse in the order of the name: those read from
// its start, and then those read from its end, which come last first. It also
// returns how many of them begin in the part that was read from the start.
//
// The delimiters at the two ends of a tag that are no part of it are cut off
// it, see edges, and every run of delimiters between two tags becomes the tags
// next gives for it: one tag, after one for each ellipsis the run begins with.
// That joins what the lexers gave as delimiter tags of their own, as the
// hyphen in front of a group is one ('x264 - GRP' has ' - ').
//
// The slice is as long as the tags it holds: measure counts them before they
// are written. A name with nothing to cut or to join costs that count and the
// one copy.
func (p *tagParser) join(src []byte, start, end []Tag) ([]Tag, int) {
	n, made, changed := p.measure(start, end)
	if !changed {
		tags := make([]Tag, len(start)+len(end))
		copy(tags, start)
		for i, k := len(end), len(start); i > 0; i, k = i-1, k+1 {
			tags[k] = end[i-1]
		}
		return tags, len(start)
	}
	j := joiner{p: p, src: src, dst: make([]Tag, 0, n), split: -1}
	if made != 0 {
		j.vs = make([]string, 2*made)
	}
	for i := range start {
		j.tag(&start[i])
	}
	// a run of delimiters that the two parts of the name share is a tag of
	// the part it begins in
	if j.crossed, j.cross = true, len(j.run); j.pieces == 0 {
		j.split = len(j.dst)
	}
	for i := len(end) - 1; i >= 0; i-- {
		j.tag(&end[i])
	}
	j.flush()
	if j.split == -1 {
		j.split = len(j.dst)
	}
	return j.dst, j.split
}

// delimRun counts a run of delimiters as measure reads it: its bytes, the
// dots it begins with, and the delimiter tags and cut off delimiters it is
// made of.
type delimRun struct {
	size, dots, pieces int
	// only is whether the run is dots alone so far.
	only bool
}

// add adds delimiters to the run.
func (r *delimRun) add(s string) {
	if r.pieces == 0 || r.only {
		k := 0
		for k < len(s) && s[k] == '.' {
			k++
		}
		r.dots, r.only = r.dots+k, k == len(s)
	}
	r.size, r.pieces = r.size+len(s), r.pieces+1
}

// tags returns how many tags next gives for the run: one for each ellipsis
// it begins with, and one for the rest.
func (r *delimRun) tags() int {
	n := r.dots / len(ellipsis)
	if r.size > n*len(ellipsis) {
		n++
	}
	return n
}

// measure returns how many tags join gives for the tags of a parse, how many
// of them are delimiter tags that no delimiter tag of the parse can hold, and
// whether they differ at all from what the lexers gave.
//
// It reads the tags as joiner does, in the order of the name, and counts
// where joiner writes.
func (p *tagParser) measure(start, end []Tag) (n, made int, changed bool) {
	var run delimRun
	// settled is whether the run at hand is the tags next gives for it,
	// which are an ellipsis for each it begins with and one for the rest,
	// last whether the delimiter tag before was an ellipsis, and hosted
	// whether a delimiter tag of the parse is in the run
	var settled, last, hosted bool
	for k, total := 0, len(start)+len(end); k <= total; k++ {
		var tag *Tag
		switch {
		case k < len(start):
			tag = &start[k]
		case k < total:
			tag = &end[total-1-k]
		}
		var lead, trail int
		if tag != nil {
			if s := tag.val(0); tag.typ == TagTypeDelim && s != "" {
				whole := len(s) <= len(ellipsis) || !strings.HasPrefix(s, ellipsis)
				settled = whole && (run.pieces == 0 || settled && last)
				hosted, last = hosted || p.pooled && len(tag.v) > 1, s == ellipsis
				run.add(s)
				continue
			}
			if lead, trail, _, _ = p.edges(tag); lead != 0 {
				run.add(tag.v[0][:lead])
				settled = false
			}
		}
		// the run ends at a tag that is no delimiter, and with the name
		if run.pieces != 0 {
			c := run.tags()
			n, made, changed = n+c, made+c, changed || !settled
			if hosted {
				made--
			}
			run, hosted = delimRun{}, false
		}
		if tag == nil {
			break
		}
		n++
		if trail != 0 {
			run.add(tag.v[0][len(tag.v[0])-trail:])
			settled = false
		}
		changed = changed || lead != 0 || trail != 0
	}
	return n, made, changed
}

// joiner writes the tags join gives.
type joiner struct {
	p   *tagParser
	src []byte
	dst []Tag
	// run is the delimiters since the last tag that is none, and pieces how
	// many delimiter tags and cut off delimiters it is made of.
	run    string
	pieces int
	// host is a delimiter tag of the parse in run, whose values hold the
	// first delimiter tag written for it in place of new ones.
	host *Tag
	// vs is room for the values of the delimiter tags that have no host, two
	// each.
	vs []string
	// crossed is whether the tags read from the end were reached, cross how
	// many bytes of run were read before them, and split how many tags begin
	// in the part read from the start, or -1 while that is open.
	crossed bool
	cross   int
	split   int
}

// tag takes the next tag of the name.
func (j *joiner) tag(tag *Tag) {
	if s := tag.val(0); tag.typ == TagTypeDelim && s != "" {
		if j.host == nil && j.p.pooled && len(tag.v) > 1 {
			j.host = tag
		}
		j.run, j.pieces = cat(j.src, j.run, s), j.pieces+1
		return
	}
	lead, trail, vlead, vtrail := j.p.edges(tag)
	if lead == 0 && trail == 0 {
		j.flush()
		j.dst = append(j.dst, *tag)
		return
	}
	s := tag.v[0]
	if lead != 0 {
		j.run, j.pieces = cat(j.src, j.run, s[:lead]), j.pieces+1
	}
	j.flush()
	t := *tag
	// a tag of a lexer that is not this package's may be one it keeps
	if !j.p.pooled {
		t.v = slices.Clone(t.v)
	}
	t.v[0] = s[lead : len(s)-trail]
	if vlead != 0 || vtrail != 0 {
		t.v[1] = t.v[1][vlead : len(t.v[1])-vtrail]
		t = t.resolve()
	}
	j.dst = append(j.dst, t)
	if trail != 0 {
		j.run, j.pieces = s[len(s)-trail:], 1
	}
}

// flush writes the delimiter tags of the run, and ends it.
func (j *joiner) flush() {
	if j.pieces == 0 {
		return
	}
	for run, at := j.run, 0; run != ""; {
		s := run
		if strings.HasPrefix(s, ellipsis) {
			s = ellipsis
		}
		// a tag is in the part of the name where it begins
		if j.crossed && j.split == -1 && at >= j.cross {
			j.split = len(j.dst)
		}
		var v []string
		switch {
		case j.host != nil:
			v, j.host = j.host.v[:2:2], nil
		case len(j.vs) >= 2:
			v, j.vs = j.vs[:2:2], j.vs[2:]
		default:
			v = make([]string, 2)
		}
		v[0], v[1] = s, s
		j.dst = append(j.dst, Tag{typ: TagTypeDelim, v: v})
		run, at = run[len(s):], at+len(s)
	}
	// a run that began in the part read from the start ends that part
	if j.crossed && j.split == -1 {
		j.split = len(j.dst)
	}
	j.run, j.pieces, j.host = "", 0, nil
}

// edges returns how many bytes at the start and at the end of the text of tag
// are delimiters that are no part of it, and how many of each its value holds
// as well and is to lose with them.
//
// What a tag's value does not hold is no part of the tag: the dot of
// 'DTS-HD.MA.' in front of '5.1', the space of 'S01 ', the ')' of 'Comedy)',
// the dot of '.mkv', the hyphen of '-CODEX'. A delimiter that the value
// holds is one when the tag reads as the same row without it, as the 'CD.' of
// 'The.CD.2020' does, whose row allows a delimiter in front of a word that is
// not there. It stays when the row needs it, as a '+' is the name of 'DD+',
// 'HDR10+' and 'DSN+', and it stays in a tag that has no rows to ask.
//
// A meta tag is written in brackets, which are its text as the 'S' and the
// 'E' of 'S01E02' are, and only the whitespace outside them is cut ('[Group]
// ' in front of a title, ' [ABCD1234]' at the end of a name).
func (p *tagParser) edges(tag *Tag) (lead, trail, vlead, vtrail int) {
	if tag.typ == TagTypeText || tag.typ == TagTypeWhitespace || len(tag.v) == 0 {
		return 0, 0, 0, 0
	}
	s := tag.v[0]
	if s == "" || !p.isDelim[s[0]] && !p.isDelim[s[len(s)-1]] {
		return 0, 0, 0, 0
	}
	if tag.typ == TagTypeMeta {
		for lead < len(s) && metaSpace(s[lead]) {
			lead++
		}
		for trail < len(s)-lead && metaSpace(s[len(s)-1-trail]) {
			trail++
		}
		if lead == len(s) {
			return 0, 0, 0, 0
		}
		return lead, trail, 0, 0
	}
	lead, trail = p.delims(s)
	if lead == len(s) {
		return 0, 0, 0, 0
	}
	value := tag.val(1)
	vlead, vtrail = p.delims(value)
	if vlead == len(value) {
		vlead, vtrail = 0, 0
	}
	vlead, vtrail = min(vlead, lead), min(vtrail, trail)
	if vlead == 0 && vtrail == 0 {
		return lead, trail, 0, 0
	}
	if tag.f != nil && tag.ninfo != nil {
		t := *tag
		t.v = []string{s, value[vlead : len(value)-vtrail]}
		if t = t.resolve(); t.ninfo == tag.ninfo && t.Normalize() == tag.Normalize() {
			return lead, trail, vlead, vtrail
		}
	}
	return lead - vlead, trail - vtrail, 0, 0
}

// delims returns how many bytes at the start and at the end of s are
// delimiters. For a string of nothing else the first is its length.
func (p *tagParser) delims(s string) (lead, trail int) {
	for lead < len(s) && p.isDelim[s[lead]] {
		lead++
	}
	for trail < len(s)-lead && p.isDelim[s[len(s)-1-trail]] {
		trail++
	}
	return lead, trail
}

// cat returns a followed by b. Two pieces of src that follow one another
// there give that piece of src, and nothing is copied.
func cat(src []byte, a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	case len(src) != 0:
		base := uintptr(unsafe.Pointer(unsafe.SliceData(src)))
		pa, pb := uintptr(unsafe.Pointer(unsafe.StringData(a))), uintptr(unsafe.Pointer(unsafe.StringData(b)))
		if base <= pa && pb == pa+uintptr(len(a)) && pb-base+uintptr(len(b)) <= uintptr(len(src)) {
			return unsafe.String(unsafe.StringData(a), len(a)+len(b))
		}
	}
	return a + b
}
