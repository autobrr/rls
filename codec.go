package rls

import "strings"

// Codec is a codec of a release as the codec rows of taginfo.csv spell it,
// such as "x264", "H.265" or "AV1".
type Codec string

// CodecKind is the video format of a codec, which an encoder and the format
// it writes share ("x265", "H.265" and "HEVC").
type CodecKind uint8

// The codec kinds.
const (
	// CodecOther is every codec that is neither AVC nor HEVC, and no codec.
	CodecOther CodecKind = iota
	// CodecAVC is H.264 ("x264", "x264.HQ", "H.264", "AVC").
	CodecAVC
	// CodecHEVC is H.265 ("x265", "H.265", "HEVC").
	CodecHEVC
)

// Kind returns the video format of c, in any case: "x264", "x264.HQ",
// "H.264" and "AVC" are CodecAVC, "x265", "H.265" and "HEVC" are CodecHEVC,
// and every other codec is CodecOther.
func (c Codec) Kind() CodecKind {
	switch v := string(c); {
	case strings.EqualFold(v, "x264"), strings.EqualFold(v, "x264.HQ"), strings.EqualFold(v, "H.264"), strings.EqualFold(v, "AVC"):
		return CodecAVC
	case strings.EqualFold(v, "x265"), strings.EqualFold(v, "H.265"), strings.EqualFold(v, "HEVC"):
		return CodecHEVC
	}
	return CodecOther
}
