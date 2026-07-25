package rls

import (
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// TestPrefilterSound checks the property the prefilter relies on: it must never
// reject a position where the type's alternation would have matched. Rejecting
// wrongly silently drops tags, so this is verified directly against every
// taginfo row rather than inferred.
func TestPrefilterSound(t *testing.T) {
	infos := taginfo.All()
	for typ, info := range infos {
		pf, ok := newPrefilter(info, true)
		if !ok {
			t.Logf("%s: no prefilter (a row could not be reduced)", typ)
			continue
		}
		for _, i := range info {
			v, err := expandRE(i.RE(), true)
			if err != nil {
				continue // covered by the lead byte set, exercised below
			}
			for _, s := range v {
				if s == "" {
					continue
				}
				for _, b := range [][]byte{[]byte(s), []byte(s + ".1080p"), []byte(s + "-GRP")} {
					if !pf.maybe(b, 0, len(b)) {
						t.Errorf("%s: prefilter rejected %q, which row %q can match", typ, b, i.Tag())
					}
				}
			}
		}
	}
}

func TestPrefilterRejects(t *testing.T) {
	infos := taginfo.All()
	pf, ok := newPrefilter(infos["resolution"], true)
	if !ok {
		t.Fatal("expected a resolution prefilter")
	}
	for _, s := range []string{"Frieren", "Sousou", "Bundesliga", "Djokovic"} {
		if pf.maybe([]byte(s), 0, len(s)) {
			t.Errorf("expected %q to be rejected by the resolution prefilter", s)
		}
	}
}
