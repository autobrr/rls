package rls

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"strconv"
	"strings"
)

// Flags is a set of the release flags Other holds, such as FlagProper and
// FlagReal in "Movie.2020.REAL.PROPER.1080p.BluRay.x264-GRP". The set holds
// the six flags of the constants; a bit outside them is nothing: Has,
// Strings, String and MarshalJSON pass over it.
type Flags uint16

// The release flags, in the order Strings gives them.
const (
	// FlagProper is a proper, spelled "PROPER", which "REAL.PROPER" in
	// Other gives as well.
	FlagProper Flags = 1 << iota
	// FlagReal is a real release, spelled "REAL", which "REAL.PROPER" in
	// Other gives as well.
	FlagReal
	// FlagRemux is a remux, spelled "REMUX".
	FlagRemux
	// FlagHybrid is a hybrid of several sources, spelled "HYBRiD".
	FlagHybrid
	// FlagInternal is an internal release, spelled "INTERNAL". A name may
	// write it "iNT".
	FlagInternal
	// FlagReRip is a re-rip, spelled "RERiP".
	FlagReRip
)

// allFlags is the set of the six flags.
const allFlags = FlagProper | FlagReal | FlagRemux | FlagHybrid | FlagInternal | FlagReRip

// flagNames are the spellings of the flags, one per bit from the lowest, as
// Other spells them.
var flagNames = [...]string{"PROPER", "REAL", "REMUX", "HYBRiD", "INTERNAL", "RERiP"}

// Has reports whether every flag of g is in f, and false when g holds no
// flag.
func (f Flags) Has(g Flags) bool {
	g &= allFlags
	return g != 0 && f&g == g
}

// Strings returns the spellings of the flags in f, always in the order of the
// constants ("PROPER", "REMUX"), or nil when f holds no flag.
func (f Flags) Strings() []string {
	f &= allFlags
	if f == 0 {
		return nil
	}
	v := make([]string, 0, bits.OnesCount16(uint16(f)))
	for i, name := range flagNames {
		if f&(1<<i) != 0 {
			v = append(v, name)
		}
	}
	return v
}

// String returns the spellings of the flags in f joined with a comma
// ("PROPER,REMUX").
func (f Flags) String() string {
	return strings.Join(f.Strings(), ",")
}

// MarshalJSON writes f as a list of spellings (["PROPER","REMUX"]), [] when it
// is empty.
func (f Flags) MarshalJSON() ([]byte, error) {
	v := f.Strings()
	if v == nil {
		v = []string{}
	}
	return json.Marshal(v)
}

// UnmarshalJSON reads f from a list of spellings in any case and order, and
// fails on a spelling that is none of the flags. A null leaves f as it is, as
// it does a value of a type of encoding/json.
func (f *Flags) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var v []string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	var set Flags
next:
	for _, s := range v {
		for i, name := range flagNames {
			if strings.EqualFold(s, name) {
				set |= 1 << i
				continue next
			}
		}
		return fmt.Errorf("rls: unknown flag %q", s)
	}
	*f = set
	return nil
}

// readOther returns the flags and the repack generation that the values of
// Other give, as the other rows of taginfo.csv spell them and in any case, as
// a caller's row may spell them ('remux'): 'REAL.PROPER' is both FlagReal and
// FlagProper, 'REPACK' generation 1, 'REREPACK' (which 'REPACK2' is read as)
// 2, and 'REPACK3' to 'REPACK99' their number. The repack is the highest
// generation of them. A 'REPACK2', which no built-in row gives, is 2 as well.
func readOther(other []string) (Flags, int) {
	var f Flags
	var repack int
	for _, s := range other {
		for i, name := range flagNames {
			if strings.EqualFold(s, name) {
				f |= 1 << i
			}
		}
		switch {
		case strings.EqualFold(s, "REAL.PROPER"):
			f |= FlagReal | FlagProper
		case strings.EqualFold(s, "REPACK"):
			repack = max(repack, 1)
		case strings.EqualFold(s, "REREPACK"):
			repack = max(repack, 2)
		case 7 <= len(s) && len(s) <= 8 && strings.EqualFold(s[:6], "REPACK"):
			// one digit from 2 on, or two that do not begin with 0
			if n := s[6:]; '1' <= n[0] && n[0] <= '9' {
				if g, err := strconv.Atoi(n); err == nil && 2 <= g {
					repack = max(repack, g)
				}
			}
		}
	}
	return f, repack
}
