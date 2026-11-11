package rls

import (
	"fmt"
	"sync"
	"testing"
)

func TestReleaseFormatV(t *testing.T) {
	const name = "Show.S01E02.1080p.WEB.h264-GRP"
	r := ParseString(name)
	for _, got := range []string{
		fmt.Sprintf("%v", r),
		fmt.Sprintf("%+v", r),
		fmt.Sprintf("%#v", r),
		fmt.Sprint(r),
		fmt.Sprintf("%o", r),
	} {
		if got != name {
			t.Errorf("formatted release = %q, want %q", got, name)
		}
	}
	if got := fmt.Sprintf("%v", Release{}); got != "" {
		t.Errorf(`Sprintf("%%v", Release{}) = %q, want empty`, got)
	}
	// other verbs that are not implemented keep printing nothing
	if got := fmt.Sprintf("%d", r); got != "" {
		t.Errorf(`Sprintf("%%d", r) = %q, want empty`, got)
	}
}

func TestFindCountClamped(t *testing.T) {
	tags, _ := ParseTagsString("Show.S01E02")
	if len(tags) < 3 {
		t.Fatalf("need at least 3 tags, got %d", len(tags))
	}
	tags = tags[:3]
	for _, count := range []int{10, -5, -1, 3, 1 << 30} {
		// copy path
		got, n := Find(tags, "", count, 'o')
		if len(got) != 3 || n != 3 {
			t.Errorf("copy path, count %d: got %d tags and n %d, want 3 and 3", count, len(got), n)
		}
		// matching path, every tag is a match for the empty pattern of a type
		got, n = Find(tags, "", count, 'o', TagTypeWhitespace, TagTypeDelim, TagTypeText, TagTypeSeries)
		if len(got) != 3 || n != 3 {
			t.Errorf("matching path, count %d: got %d tags and n %d, want 3 and 3", count, len(got), n)
		}
	}
	if got, _ := Find(tags, "", 2, 'o'); len(got) != 2 {
		t.Errorf("count 2 gave %d tags, want 2", len(got))
	}
}

func TestMatchInvalidRE(t *testing.T) {
	tags, _ := ParseTagsString("Show.S01E02.1080p")
	if got, _ := Find(tags, "(", -1, 'r'); len(got) != 0 {
		t.Errorf("invalid pattern found %d tags, want none", len(got))
	}
	if tags[0].Match("(", 'r') {
		t.Error("Match with an invalid pattern = true")
	}
}

func TestMatchValidRE(t *testing.T) {
	tags, _ := ParseTagsString("Show.S01E02.1080p")
	got, _ := Find(tags, `^S\d+E\d+$`, -1, 'r')
	if len(got) != 1 || got[0].Normalize() != "S01E02" {
		t.Errorf("series pattern found %v", got)
	}
	if !tags[0].Match("^Show$", 'r') {
		t.Error("pattern '^Show$' does not match the first tag")
	}
	if tags[0].Match("^Other$", 'r') {
		t.Error("pattern '^Other$' matches the first tag")
	}
}

func TestMatchRECacheLimit(t *testing.T) {
	tag := NewTag(TagTypeText, nil, []byte("abc"), []byte("abc"))
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 2 * reCacheMax {
				// distinct patterns, each one matches 'abc'
				if !tag.Match(fmt.Sprintf("^a(b)c$|x%d_%d", g, i), 'r') {
					t.Errorf("pattern %d did not match", i)
				}
			}
		})
	}
	wg.Wait()
	if n := reCacheLen.Load(); n > reCacheMax {
		t.Errorf("cache counter is %d, want at most %d", n, reCacheMax)
	}
	stored := 0
	reCache.Range(func(_, _ any) bool { stored++; return true })
	// the cache fills: a counter that drifted would stop it short of the limit
	if stored != reCacheMax {
		t.Errorf("cache holds %d patterns, want %d", stored, reCacheMax)
	}
	// a pattern past the limit is compiled for the call and not stored
	const late = "^abc$|^never-stored-before$"
	if !tag.Match(late, 'r') {
		t.Error("pattern past the limit did not match")
	}
	if tag.Match("^xyz$|^never-stored-either$", 'r') {
		t.Error("pattern past the limit matched wrongly")
	}
	if _, ok := reCache.Load(late); ok {
		t.Error("pattern past the limit was stored")
	}
}
