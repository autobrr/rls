package rls

import (
	"fmt"
	"os"
	"strings"
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
		// every type has one today. A row that takes it away costs a regexp
		// run on every token of every name
		if !ok {
			t.Errorf("%s: no prefilter (a row could not be reduced)", typ)
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

// TestPrefilterSameParse checks that the prefilters only save work: a parser
// built without them gives the same tags for every name of tests.yaml, for
// every spelling a row can be expanded to, and for the corpora when they are
// present.
func TestPrefilterSameParse(t *testing.T) {
	noPrefilter = true
	slow := NewParser()
	noPrefilter = false
	fast := NewParser()
	var names []string
	for _, test := range rlsTests(t) {
		names = append(names, test.s)
	}
	for _, info := range taginfo.All() {
		for _, i := range info {
			v, err := expandRE(i.RE(), true)
			if err != nil {
				continue
			}
			for _, s := range v {
				names = append(names, "Title."+s+".1080p-GRP")
			}
		}
	}
	for _, name := range []string{"nyaa.txt", "sports.txt"} {
		if buf, err := os.ReadFile("testdata/corpus/" + name); err == nil && !testing.Short() {
			names = append(names, strings.Split(string(buf), "\n")...)
		}
	}
	for _, name := range names {
		a, an := fast.Parse([]byte(name))
		b, bn := slow.Parse([]byte(name))
		if s, exp := fmt.Sprintf("%q", a), fmt.Sprintf("%q", b); s != exp || an != bn {
			t.Fatalf("%q: expected the tags without prefilters\n%s (%d)\ngot:\n%s (%d)", name, exp, bn, s, an)
		}
	}
	t.Logf("%d names", len(names))
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

// TestFindFuncNonASCII checks that the indexed lookup finds the same row as
// taginfo.Find for a spelling with a non-ASCII letter, which a row's regexp
// can ask for ('ungek[uü]e?rzt') and the index cannot hold.
func TestFindFuncNonASCII(t *testing.T) {
	infos := taginfo.All()["cut"]
	f, find := findFunc(infos...), taginfo.Find(infos...)
	for _, s := range []string{"ungekürzt", "UNGEKÜRZT", "Ungekuerzt", "ungekurzt", "uncut"} {
		exp := find(s)
		if exp == nil {
			t.Fatalf("%q expected a cut tag info", s)
		}
		if info := f(s); info != exp {
			t.Errorf("%q expected %q, got: %v", s, exp.Tag(), info)
		}
	}
}
