package rls

import (
	"go/ast"
	"go/token"
	"slices"
	"testing"
)

// TestGroupOrigin checks the group of a name and where it was read, for each
// of the candidates group ranks.
func TestGroupOrigin(t *testing.T) {
	for _, test := range []struct {
		s, group string
		from     groupOrigin
	}{
		{"Show.S01E02.1080p.WEB.H264-GRP", "GRP", groupSuffix},
		{"22 Jump Street (2014) 720p BrRip x264 - YIFY", "YIFY", groupSuffix},
		{"[TGx] Movie.2019.1080p.BluRay.x264-GRP", "GRP", groupSuffix},
		{"Some.Game.v1.2-CODEX", "CODEX", groupKnown},
		{"Some.Title-GROUP (2019) [Windows / Multi-Language / Full ISO]", "GROUP", groupBeforeBrackets},
		{"Show S01E02 1080p WEB-DL AAC2.0 H 264-VARYG (Alt Title, Multi-Subs)", "VARYG", groupBeforeBrackets},
		{"Movie.1985.DVDRip.XviD-DVD", "DVD", groupSource},
		{"Show.S25.720p.WEB-DL.x264-TBS", "TBS", groupSource},
		{"[SubsPlease] Show - 04 (1080p) [ABCD1234].mkv", "SubsPlease", groupLeading},
		{"[REPACK] [SubsPlease] Show - 04 (1080p)", "SubsPlease", groupLeading},
		{"[DSD] Show - 01 [1080p]", "DSD", groupLeading},
		{"[Commie] Hataraku Maou-sama!", "Commie", groupLeading},
		{"Show - 04 [1080p][Group].mkv", "Group", groupTrailing},
		{"MiniMeters v0 8 4 Beta MacOS BTCR", "BTCR", groupWord},
		{"Show - 05 (1080p x265 AAC Group)", "Group", groupWord},
		{"Show - 05 (BD 1080p - Kyoshi Mura)", "Kyoshi Mura", groupWord},
		// a known group of another type is given back, and is the last word
		{"Movie.2019.1080p.WEB.h264-CODEX", "CODEX", groupWord},
		{"Some.Movie.2019.1080p.WEB.h264", "", groupNone},
		// a word that belongs to the tag in front of it is no group
		{"Ultra Maniac Eng Sub", "", groupNone},
		{"Sentai Daishikkaku - 02 | Go! Go! Loser Ranger! [English Dub][1080p][DSNP]", "", groupNone},
		{"Vivid Strike - OP1 - Future Strike (Yui Ogura) [English Sub][Baaro][393E177A].mkv", "Baaro", groupTrailing},
		{"[Group] Show - 01 [1080p] [ABCD1234]", "Group", groupLeading},
	} {
		if r := ParseString(test.s); r.Group != test.group || r.groupFrom != test.from {
			t.Errorf("%q expected group %q from %d, got: %q from %d", test.s, test.group, test.from, r.Group, r.groupFrom)
		}
	}
}

// TestGroupOwner checks that the group has one owner: Release.Group is set
// and a group tag is made by the functions of group.go alone, besides the
// lexer, which makes its tags with the type in another place.
//
// The group counts as set by an assignment to a field named Group, by taking
// its address, and by a Release written out with one ('*r = Release{Group:
// s}'). See retypes for how a group tag counts as made, and for what a test
// that reads the source cannot see. A Release that is copied whole from
// another keeps the group its owner gave it, and is not counted.
func TestGroupOwner(t *testing.T) {
	sets := funcsWith(t, func(n ast.Node) bool {
		group := func(e ast.Expr) bool {
			sel, ok := e.(*ast.SelectorExpr)
			return ok && sel.Sel.Name == "Group"
		}
		switch n := n.(type) {
		case *ast.AssignStmt:
			return slices.ContainsFunc(n.Lhs, group)
		case *ast.IncDecStmt:
			return group(n.X)
		case *ast.UnaryExpr:
			return n.Op == token.AND && group(n.X)
		case *ast.CompositeLit:
			if typ, ok := n.Type.(*ast.Ident); ok && typ.Name == "Release" {
				return slices.ContainsFunc(n.Elts, func(e ast.Expr) bool {
					kv, ok := e.(*ast.KeyValueExpr)
					if !ok {
						return true
					}
					key, ok := kv.Key.(*ast.Ident)
					return ok && key.Name == "Group"
				})
			}
		}
		return false
	})
	makes := funcsWith(t, func(n ast.Node) bool {
		return retypes(n, "TagTypeGroup")
	})
	for found, owners := range map[*map[string]int][]string{
		&sets:  {"group", "bracketedGroup", "lastWord", "widenGroup"},
		&makes: {"suffixBeforeBrackets", "settleGroupType"},
	} {
		for name := range *found {
			ok := false
			for _, owner := range owners {
				ok = ok || name == owner
			}
			if !ok {
				t.Errorf("%s sets the group or makes a group tag, and is not of the owner in group.go", name)
			}
		}
		for _, owner := range owners {
			if (*found)[owner] == 0 {
				t.Errorf("%s is listed as an owner and does nothing of the kind: take it off the list", owner)
			}
		}
	}
}
