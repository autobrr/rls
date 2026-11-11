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
