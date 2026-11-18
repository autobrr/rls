package rls

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"strings"
)

// DynamicRange is a set of dynamic range formats, such as DV and HDR10 in
// "Movie.2020.2160p.WEB-DL.DV.HDR10.HEVC-GRP". The set holds the seven formats
// of the constants; a bit outside them is nothing: Has, Strings, String and
// MarshalJSON pass over it.
type DynamicRange uint8

// The dynamic range formats, in the order Strings gives them. The comment of
// each gives its one spelling, which Strings writes and ParseDynamicRange
// reads in any case.
const (
	// DV is Dolby Vision, spelled "DV". A name may write it "DoVi" or
	// "Dolby.Vision".
	DV DynamicRange = 1 << iota
	// HDR is high dynamic range with no format given, spelled "HDR".
	HDR
	// HDRPlus is HDR+, spelled "HDR+".
	HDRPlus
	// HDR10 is HDR10, spelled "HDR10".
	HDR10
	// HDR10Plus is HDR10+, spelled "HDR10+". A name may write it "HDR10Plus"
	// or "HDR10P".
	HDR10Plus
	// HLG is hybrid log-gamma, spelled "HLG".
	HLG
	// SDR is standard dynamic range, spelled "SDR".
	SDR
)

// allDynamicRange is the set of the seven formats.
const allDynamicRange = DV | HDR | HDRPlus | HDR10 | HDR10Plus | HLG | SDR

// dynamicRangeNames are the spellings of the formats, one per bit from the
// lowest, as the hdr rows of taginfo.csv write them.
var dynamicRangeNames = [...]string{"DV", "HDR", "HDR+", "HDR10", "HDR10+", "HLG", "SDR"}

// ParseDynamicRange returns the format whose spelling s is, in any case
// ("hdr10+" is HDR10Plus), or 0 when s is none of the seven spellings
// ("DoVi" and "HDR10Plus" are none).
func ParseDynamicRange(s string) DynamicRange {
	for i, name := range dynamicRangeNames {
		if strings.EqualFold(s, name) {
			return 1 << i
		}
	}
	return 0
}

// Has reports whether every format of f is in d, and false when f holds no
// format.
func (d DynamicRange) Has(f DynamicRange) bool {
	f &= allDynamicRange
	return f != 0 && d&f == f
}

// Strings returns the spellings of the formats in d, always in the order of
// the constants ("DV", "HDR10"), or nil when d holds no format.
func (d DynamicRange) Strings() []string {
	d &= allDynamicRange
	if d == 0 {
		return nil
	}
	v := make([]string, 0, bits.OnesCount8(uint8(d)))
	for i, name := range dynamicRangeNames {
		if d&(1<<i) != 0 {
			v = append(v, name)
		}
	}
	return v
}

// String returns the spellings of the formats in d joined with a comma
// ("DV,HDR10").
func (d DynamicRange) String() string {
	return strings.Join(d.Strings(), ",")
}

// MarshalJSON writes d as a list of spellings (["DV","HDR10"]), [] when it is
// empty.
func (d DynamicRange) MarshalJSON() ([]byte, error) {
	v := d.Strings()
	if v == nil {
		v = []string{}
	}
	return json.Marshal(v)
}

// UnmarshalJSON reads d from a list of spellings in any case and order, and
// fails on a spelling that is none of the formats. A null leaves d as it is,
// as it does a value of a type of encoding/json.
func (d *DynamicRange) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var v []string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	var set DynamicRange
	for _, s := range v {
		f := ParseDynamicRange(s)
		if f == 0 {
			return fmt.Errorf("rls: unknown dynamic range %q", s)
		}
		set |= f
	}
	*d = set
	return nil
}
