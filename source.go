package rls

import "strings"

// Source is the source of a release as the source rows of taginfo.csv spell
// it, such as "WEB-DL", "UHD.BluRay" or "HDTV".
type Source string

// SourceKind is the kind of a web source, which a source written in other
// ways ("WEB-DL", "UHD.WEB-DL") shares.
type SourceKind uint8

// The source kinds.
const (
	// SourceOther is every source that is not one of the web kinds, and no
	// source.
	SourceOther SourceKind = iota
	// SourceWeb is a web source that says nothing of download or rip ("WEB",
	// "WEB-HD", "WebUHD", "UHD.WEB").
	SourceWeb
	// SourceWebDL is a web download ("WEB-DL", "UHD.WEB-DL").
	SourceWebDL
	// SourceWebRip is a web rip ("WEBRiP", "WebHDRiP", "UHD.WEBRiP").
	SourceWebRip
)

// Kind returns the kind of s, in any case ("web-dl" is SourceWebDL):
// SourceWeb for "WEB", "WEB-HD" and "WebUHD", SourceWebDL for "WEB-DL",
// SourceWebRip for "WEBRiP" and "WebHDRiP", and SourceOther for every other
// source, a web screener ("WEBSCR") and the music of a web store ("WEBFLAC")
// among them. A UHD. prefix does not change it: "UHD.WEB-DL" is SourceWebDL as
// "WEB-DL" is.
func (s Source) Kind() SourceKind {
	v := string(s)
	if len(v) > 4 && strings.EqualFold(v[:4], "UHD.") {
		v = v[4:]
	}
	switch {
	case strings.EqualFold(v, "WEB"), strings.EqualFold(v, "WEB-HD"), strings.EqualFold(v, "WebUHD"):
		return SourceWeb
	case strings.EqualFold(v, "WEB-DL"):
		return SourceWebDL
	case strings.EqualFold(v, "WEBRiP"), strings.EqualFold(v, "WebHDRiP"):
		return SourceWebRip
	}
	return SourceOther
}

// UHD reports whether s is written as UHD, in any case ("UHD.BluRay",
// "UHD.WEB-DL", "UHDTV", "WebUHD", "uhd.bluray").
func (s Source) UHD() bool {
	for i := 0; i+3 <= len(s); i++ {
		if strings.EqualFold(string(s[i:i+3]), "UHD") {
			return true
		}
	}
	return false
}
