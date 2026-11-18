package rls

import "strings"

// Resolution is the resolution of a release as the resolution rows of
// taginfo.csv spell it, such as "1080p", "1080i" or "DCI4K".
type Resolution string

// Height returns the lines of the picture r gives, in any case: 1080 for
// "1080p", "1080i" and "1080P", 2160 for "2160p" and "DCI4K", 1080 for
// "DCI2K", and 0 when r gives none ("PN.Selector").
func (r Resolution) Height() int {
	s := string(r)
	switch {
	case strings.EqualFold(s, "DCI4K"):
		return 2160
	case strings.EqualFold(s, "DCI2K"):
		return 1080
	case len(s) < 2 || !strings.ContainsRune("pPiI", rune(s[len(s)-1])):
		return 0
	}
	n := 0
	for _, c := range []byte(s[:len(s)-1]) {
		if c < '0' || '9' < c || n > 1e6 {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// Interlaced reports whether r is an interlaced picture, in any case
// ("1080i", "576I").
func (r Resolution) Interlaced() bool {
	return len(r) > 1 && (r[len(r)-1] == 'i' || r[len(r)-1] == 'I') && r.Height() != 0
}
