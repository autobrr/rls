package rls

import (
	"fmt"
	"testing"
)

// callAllTagMethods calls every exported method of Tag and formats it with
// every verb, so that a panic on a tag with too few values fails the test.
func callAllTagMethods(t *testing.T, name string, tag Tag) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: panic: %v", name, r)
		}
	}()
	_ = tag.Is(TagTypeText)
	_ = tag.Was(TagTypeText)
	_ = tag.Info()
	_ = tag.SingleEp()
	_ = tag.InfoType()
	_ = tag.InfoExcl()
	_ = tag.TagType()
	_ = tag.Prev()
	_ = tag.InfoTitle()
	_ = tag.Normalize()
	for _, verb := range "sroqev" {
		_ = tag.Match("x", verb)
	}
	_ = tag.Whitespace()
	_ = tag.Delim()
	_ = tag.Text()
	_ = tag.TextReplace("a", "b", -1)
	_ = tag.Platform()
	_ = tag.Arch()
	_ = tag.Source()
	_ = tag.Resolution()
	_ = tag.Collection()
	_, _, _ = tag.Date()
	_, _ = tag.Series()
	_ = tag.Episodes()
	_ = tag.Version()
	_ = tag.Disc()
	_ = tag.Codec()
	_ = tag.HDR()
	_ = tag.BitDepth()
	_ = tag.Audio()
	_ = tag.Channels()
	_ = tag.Other()
	_ = tag.Cut()
	_ = tag.Edition()
	_ = tag.Language()
	_ = tag.Size()
	_ = tag.Region()
	_ = tag.Container()
	_ = tag.Genre()
	_ = tag.ID()
	_ = tag.Group()
	_, _ = tag.Meta()
	_ = tag.Ext()
	_ = tag.FrameRate()
	for _, verb := range []string{"%q", "%o", "%v", "%s", "%r", "%e", "%+v", "%#v", "%10v", "%x"} {
		_ = fmt.Sprintf(verb, tag)
	}
	// the text a tag turns into keeps the type it was, which Text reads
	as := tag.As(TagTypeText, nil)
	_ = as.Text()
	_ = as.TextReplace("a", "b", -1)
	_ = as.Normalize()
	_ = fmt.Sprintf("%s", as)
	_ = tag.Revert()
}

func TestTagTooFewValues(t *testing.T) {
	callAllTagMethods(t, "zero", Tag{})
	for typ := TagTypeWhitespace; typ <= TagTypeFrameRate; typ++ {
		tag := NewTag(typ, nil, []byte("x"), []byte("x"))
		callAllTagMethods(t, typ.String(), tag)
	}
}

func TestZeroTag(t *testing.T) {
	var tag Tag
	if s := tag.Normalize(); s != "" {
		t.Errorf("Tag{}.Normalize() = %q, want empty", s)
	}
	if s := tag.Text(); s != "" {
		t.Errorf("Tag{}.Text() = %q, want empty", s)
	}
	if s := fmt.Sprintf("%o", tag); s != "" {
		t.Errorf(`Sprintf("%%o", Tag{}) = %q, want empty`, s)
	}
	if tag.SingleEp() {
		t.Error("Tag{}.SingleEp() = true")
	}
	if eps := tag.Episodes(); len(eps) != 0 {
		t.Errorf("Tag{}.Episodes() = %v, want none", eps)
	}
}

func TestTagAccessorValues(t *testing.T) {
	// a channels value with no digit has no channels to give
	if s := NewTag(TagTypeChannels, nil, []byte("x"), []byte("x")).Channels(); s != "" {
		t.Errorf("Channels() with no digit = %q, want empty", s)
	}
	// a date holding a year and nothing else has no month and day
	d := NewTag(TagTypeDate, nil, []byte("2009"), []byte("2009"))
	if y, m, day := d.Date(); y != 2009 || m != 0 || day != 0 {
		t.Errorf("Date() = %d, %d, %d, want 2009, 0, 0", y, m, day)
	}
	if s := NewTag(TagTypeFrameRate, nil, []byte("24"), []byte("24")).FrameRate(); s != "24fps" {
		t.Errorf("FrameRate() = %q, want 24fps", s)
	}
	if s := NewTag(TagTypeDisc, nil, []byte("x"), []byte("x")).Disc(); s != "" {
		t.Errorf("Disc() with too few values = %q, want empty", s)
	}
}

func TestNewTagTooFewValuesPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewTag with one value did not panic")
		}
	}()
	NewTag(TagTypeText, nil, []byte("x"))
}

func TestParsedTagsNormalize(t *testing.T) {
	tags, _ := ParseTagsString("Show.S01E02.1080p.WEB.DD5.1.h264-GRP")
	want := map[TagType]string{
		TagTypeSeries:     "S01E02",
		TagTypeResolution: "1080p",
		TagTypeChannels:   "5.1",
		TagTypeCodec:      "H.264",
	}
	got := make(map[TagType]string)
	for _, tag := range tags {
		if _, ok := want[tag.TagType()]; ok {
			got[tag.TagType()] = tag.Normalize()
		}
	}
	for typ, w := range want {
		if got[typ] != w {
			t.Errorf("%s tag normalizes to %q, want %q", typ, got[typ], w)
		}
	}
}

func TestTagRetyped(t *testing.T) {
	fresh := NewTag(TagTypeSource, nil, []byte("WEB"), []byte("WEB"))
	retyped := fresh.As(TagTypeText, nil)

	if (Tag{}).Retyped() {
		t.Error("Tag{}.Retyped() = true, want false")
	}
	if fresh.Retyped() {
		t.Error("NewTag(...).Retyped() = true, want false")
	}
	if !retyped.Retyped() {
		t.Error("As(...).Retyped() = false, want true")
	}
	if !fresh.As(TagTypeSource, nil).Retyped() {
		t.Error("As(same type).Retyped() = false, want true")
	}
	if !retyped.As(TagTypeRegion, nil).Retyped() {
		t.Error("As(As(...)).Retyped() = false, want true")
	}
	if retyped.Revert().Retyped() {
		t.Error("Revert().Retyped() = true, want false")
	}
	if fresh.Revert().Retyped() {
		t.Error("Revert() of a never retyped tag: Retyped() = true, want false")
	}
}

func TestTagPrevOfNeverRetyped(t *testing.T) {
	// autobrr reads a source that was never retyped as Prev() == Whitespace
	fresh := NewTag(TagTypeSource, nil, []byte("WEB"), []byte("WEB"))
	if got := fresh.Prev(); got != TagTypeWhitespace {
		t.Errorf("Prev() = %v, want Whitespace", got)
	}
	if !fresh.Was(TagTypeWhitespace) {
		t.Error("Was(Whitespace) = false for a never retyped tag, want true")
	}
	if got := (Tag{}).Prev(); got != TagTypeWhitespace {
		t.Errorf("Tag{}.Prev() = %v, want Whitespace", got)
	}
	if got := fresh.Revert().TagType(); got != TagTypeWhitespace {
		t.Errorf("Revert() of a never retyped tag has type %v, want Whitespace", got)
	}
}

func TestTagPrevAfterAs(t *testing.T) {
	fresh := NewTag(TagTypeSource, nil, []byte("WEB"), []byte("WEB"))
	retyped := fresh.As(TagTypeText, nil)
	if got := retyped.Prev(); got != TagTypeSource {
		t.Errorf("Prev() = %v, want Source", got)
	}
	if !retyped.Was(TagTypeSource) || retyped.Was(TagTypeWhitespace) {
		t.Error("Was(Source) must be true and Was(Whitespace) false after As from Source")
	}
	if got := retyped.Revert().TagType(); got != TagTypeSource {
		t.Errorf("Revert() has type %v, want Source", got)
	}
	if got := retyped.Revert().Prev(); got != TagTypeWhitespace {
		t.Errorf("Revert().Prev() = %v, want Whitespace", got)
	}
}

func TestTagRetypedFromWhitespace(t *testing.T) {
	// Prev alone cannot tell this tag from one that was never retyped
	tag := NewTag(TagTypeWhitespace, nil, []byte(" "), []byte(" ")).As(TagTypeText, nil)
	if !tag.Retyped() {
		t.Error("Retyped() = false, want true")
	}
	if got := tag.Prev(); got != TagTypeWhitespace {
		t.Errorf("Prev() = %v, want Whitespace", got)
	}
}

func TestTagTextAfterRetype(t *testing.T) {
	// a date and a series read as they were written, not as their first value
	date := NewTag(TagTypeDate, nil, []byte("2009.01.02"), []byte("2009"), []byte("01"), []byte("02"))
	if got := date.As(TagTypeText, nil).Text(); got != "2009.01.02" {
		t.Errorf("Text() of a retyped date = %q, want %q", got, "2009.01.02")
	}
	series := NewTag(TagTypeSeries, nil, []byte("S01E02"), []byte("01"), []byte("02"))
	if got := series.As(TagTypeText, nil).Text(); got != "S01E02" {
		t.Errorf("Text() of a retyped series = %q, want %q", got, "S01E02")
	}
	// channels read with the dot, however they were written, and the dot is
	// not a separator for TextReplace to replace
	channels := NewTag(TagTypeChannels, nil, []byte("51"), []byte("51")).As(TagTypeText, nil)
	if got := channels.Text(); got != "5.1" {
		t.Errorf("Text() of retyped channels = %q, want %q", got, "5.1")
	}
	if got := channels.TextReplace(".", " ", -1); got != "5.1" {
		t.Errorf("TextReplace() of retyped channels = %q, want %q", got, "5.1")
	}
	// anything else reads as its value
	source := NewTag(TagTypeSource, nil, []byte("web"), []byte("WEB")).As(TagTypeText, nil)
	if got := source.Text(); got != "WEB" {
		t.Errorf("Text() of a retyped source = %q, want %q", got, "WEB")
	}
}

func TestTagRevert(t *testing.T) {
	source := NewTag(TagTypeSource, nil, []byte("WEB"), []byte("WEB"))
	// after two As calls it gives the type between them
	twice := source.As(TagTypeRegion, nil).As(TagTypeText, nil)
	if got := twice.Revert(); got.TagType() != TagTypeRegion || got.Retyped() {
		t.Errorf("Revert() after two As: type %v, retyped %v; want Region, false", got.TagType(), got.Retyped())
	}
	space := NewTag(TagTypeWhitespace, nil, []byte(" "), []byte(" ")).As(TagTypeText, nil)
	if got := space.Revert(); got.TagType() != TagTypeWhitespace || got.Retyped() {
		t.Errorf("Revert() of text that was whitespace: type %v, retyped %v; want Whitespace, false", got.TagType(), got.Retyped())
	}
	// a type outside the constants is kept like any other, also -1, which a
	// caller may use for "none"
	for _, typ := range []TagType{-1, -2, 1000} {
		tag := source.As(typ, nil).As(TagTypeText, nil)
		if !tag.Retyped() || tag.Prev() != typ || !tag.Was(typ) || tag.Revert().TagType() != typ {
			t.Errorf("after As(%d) and As(Text): retyped %v, prev %d, was %v, revert %d", int(typ), tag.Retyped(), int(tag.Prev()), tag.Was(typ), int(tag.Revert().TagType()))
		}
	}
}

func TestParsedTagsRetyped(t *testing.T) {
	find := func(t *testing.T, name, written string) Tag {
		t.Helper()
		for _, tag := range ParseString(name).Tags() {
			if fmt.Sprintf("%o", tag) == written {
				return tag
			}
		}
		t.Fatalf("%s: no tag written %q", name, written)
		return Tag{}
	}

	const tennis = "Tennis.US.Open.2025.Mens.Final.1080p.WEB.h264-GRP"
	us := find(t, tennis, "US")
	if us.TagType() != TagTypeText || !us.Retyped() || us.Prev() != TagTypeRegion {
		t.Errorf("US: type %v, retyped %v, prev %v; want Text, true, Region", us.TagType(), us.Retyped(), us.Prev())
	}
	word := find(t, tennis, "Tennis")
	if word.TagType() != TagTypeText || word.Retyped() {
		t.Errorf("Tennis: type %v, retyped %v; want Text, false", word.TagType(), word.Retyped())
	}

	const show = "Show.S01E02.1080p.WEB.h264-GRP"
	for _, tag := range ParseString(show).Tags() {
		if tag.Retyped() {
			t.Errorf("%s: tag %q is retyped, want none", show, fmt.Sprintf("%o", tag))
		}
	}
}
