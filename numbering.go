package rls

import (
	"iter"
	"slices"
	"strings"
)

// Numbering is the seasons and the episodes a name gives: 'S01E05' is episode
// 5 of season 1, '[Grp] Show - 01-12' the episodes 1 to 12 with no season, and
// 'Show.S01-S03' three seasons. The zero value is a name that gives neither.
//
// A parse fills it from the name. A Numbering made by hand holds what its
// maker put in the fields, and the methods read those fields and nothing
// else.
type Numbering struct {
	// Scheme is how the name numbers its episodes.
	Scheme Scheme
	// Seasons are the seasons the name writes, each once and in the order it
	// writes them. The seasons inside a range are included, up to
	// maxRangeFill of them ('S01-S03', 'S01-03', 'Seasons 1~7', '(Season 1 -
	// 3)'), and the seasons of a list are those it writes ('S01+S02', 'Season
	// 1+2+3', 'Seasons 1 & 2'). A written season 0 is a season ('S00E05'
	// gives one season, 0), and a name that writes no season gives nil.
	Seasons []int
	// Episodes are the episodes the name writes, in the order it writes
	// them, with a range as one Ep. A hyphen or a tilde inside a tag makes a
	// range ('S01E02-04', '01 ~ 12'), and so does a bare hyphen or tilde
	// between two tags of one season when the end is the higher number
	// ('E02-E04', 'S00E02-S00E04', 'E29~E31'). Episodes written one after
	// the other are one Ep each ('S01E05E06'). The same episode written
	// twice is listed once ('Show.S11E11-GRP/Show.S11E11-GRP.mkv'), and a
	// season with no episode gives none ('S01').
	//
	// An Ep that is not an EpRegular is an extra. It is listed here with its
	// Kind for a caller that wants it, and Episode, Absolute, Shape and All
	// pass over it.
	Episodes []Ep
	// Alt is a second numbering of the same episodes: the 1174 of
	// 'S23E19.Episode.1174' and of '[Grp] Show - 1174 (S23E19)', the 13 of
	// 'S2 - 13 (S02E01)' and the 6 of 'S02E06 [Episode 6]'. It is an episode
	// written with no season in a name that writes the episode of that
	// season too, and its Season is NoSeason. An 'E06' after a season and an
	// episode is another episode and not a second numbering ('S01E05.E06'),
	// and so is the end of a range ('S04E29-31v2').
	Alt []Ep
	// Volume is the volume or the box a name gives in place of episodes. The
	// parser leaves it 0.
	Volume int
	// Contents is what a pack says it holds besides its episodes. The parser
	// leaves it nil.
	Contents []string
	// Batch is whether the name says it is a batch. The parser leaves it
	// false.
	Batch bool
}

// Ep is one episode, or one range of episodes, as a name writes it: 'S01E05'
// is Season 1, First 5 and Last 5, and '01-12' is Season NoSeason, First 1 and
// Last 12.
//
// The zero Ep is episode 0 of season 0, and not an episode with no season. An
// Ep made by hand for a name that writes no season sets Season to NoSeason, as
// every Ep of a parse does.
type Ep struct {
	// Season is the season the episode is in, and NoSeason when the name
	// gives none for it. An episode written with no season is in the season
	// written before it, as the '04' of 'S2 - 04' and of '2nd Season - 04'
	// is in season 2. After a range or a list of seasons it is in none of
	// them ('Season 1-4 (1-76)', 'S1+S2 (01-25)').
	Season int
	// First is the episode, or the first episode of a range. Last is the
	// last episode of a range, and the same as First for one episode. A Last
	// below First, which no parse makes, is one episode too.
	First, Last int
	// Frac is the digits after the point of an episode written with one,
	// read as a number without the zeros that end them: '21.5' is First 21,
	// Last 21 and Frac 5, and so is '21.50'. A zero in front of them is not
	// kept, so '21.05' reads as Frac 5 too. In a range it is the fraction of
	// Last, so '1-11.5' is First 1, Last 11 and Frac 5. A whole episode has
	// Frac 0.
	Frac uint8
	// Kind is what kind of episode it is. Every Ep of a parse is an
	// EpRegular.
	Kind EpKind
}

// single reports whether the Ep is one episode and no range. A parse makes
// Last the same as First for one episode. In a value made by hand a Last
// below First is no range either.
func (e Ep) single() bool {
	return e.Last <= e.First
}

// NoSeason is the Season of an Ep that the name gives no season for, as in
// '[Grp] Show - 05'. It tells such an episode from one of season 0 ('S00E05').
const NoSeason = -1

// EpKind is what kind of episode an Ep is: one of the run of the show, or an
// extra beside it.
type EpKind uint8

// The kinds of an episode.
const (
	// EpRegular is an episode of the run of the show, and the zero value.
	EpRegular EpKind = iota
	// EpSpecial is a special.
	EpSpecial
	// EpOVA is an original video animation.
	EpOVA
	// EpOP is an opening.
	EpOP
	// EpED is an ending.
	EpED
	// EpPV is a promotional video.
	EpPV
)

// String returns the name of the kind in lower case ('special', 'ova'), and
// the empty string for EpRegular.
func (k EpKind) String() string {
	return nameOf(k, "", "special", "ova", "op", "ed", "pv")
}

// Scheme is how a name numbers its episodes.
type Scheme uint8

// The schemes of a numbering.
const (
	// SchemeNone is a name with no numbering ('Movie.2020.1080p'), and the
	// zero value.
	SchemeNone Scheme = iota
	// SchemeSeasoned is a name that writes a season anywhere: with an
	// episode ('S01E05', '1x05'), in front of one ('S2 - 04', '2nd Season -
	// 08') or with none ('S01').
	SchemeSeasoned
	// SchemeAbsolute is a name that writes episodes and no season ('Show -
	// 1169', 'E1100', '01-12').
	SchemeAbsolute
	// SchemeDaily is a name with a date where an episode would stand
	// ('Show.2024.03.11'): an Episode by its year, month and day that writes
	// no episode. A season written beside the date is still in Seasons
	// ('Show.2024.05.06.Title.Season.1').
	SchemeDaily
)

// String returns the name of the scheme in lower case ('seasoned'), and the
// empty string for SchemeNone.
func (s Scheme) String() string {
	return nameOf(s, "", "seasoned", "absolute", "daily")
}

// Shape is what a numbering covers: one episode, several, one season,
// several, or a day.
type Shape uint8

// The shapes of a numbering.
const (
	// ShapeNone is a numbering with no episode, no season and no date, and
	// the zero value.
	ShapeNone Shape = iota
	// ShapeEpisode is one episode ('S01E05', 'Show - 1169', 'Show - 21.5').
	ShapeEpisode
	// ShapeEpisodes is more than one episode ('S01E05E06', 'S01E02-04',
	// '01-12').
	ShapeEpisodes
	// ShapeSeason is one season and no episode ('S01').
	ShapeSeason
	// ShapeSeasons is more than one season and no episode ('S01-S03',
	// 'S01+S02').
	ShapeSeasons
	// ShapeDaily is a date where an episode would stand ('Show.2024.03.11').
	ShapeDaily
)

// String returns the name of the shape in lower case ('episodes'), and the
// empty string for ShapeNone.
func (s Shape) String() string {
	return nameOf(s, "", "episode", "episodes", "season", "seasons", "daily")
}

// nameOf returns the name of v among names, which are in the order of its
// constants, and the empty string for a value that is none of them.
func nameOf[T ~uint8](v T, names ...string) string {
	if int(v) < len(names) {
		return names[v]
	}
	return ""
}

// Season returns the first season the name writes, which is the first of
// Seasons: 1 for 'S01E05', 2 for 'S2 - 04' and 1 for 'S01-S03'. A Numbering
// with no Seasons gives the season of its first episode that has one. ok is
// false when the name gives no season, which tells '[Grp] Show - 05' from a
// season 0 ('S00E05' gives 0 and true).
//
// It is not always the season of the episode that Episode gives: 'S01-S03E05'
// writes season 1 first, and its episode 5 is in season 3. The Season of each
// Ep says which season an episode is in.
func (n Numbering) Season() (season int, ok bool) {
	if len(n.Seasons) != 0 {
		return n.Seasons[0], true
	}
	for i := range n.Episodes {
		if n.Episodes[i].Season != NoSeason {
			return n.Episodes[i].Season, true
		}
	}
	return 0, false
}

// Episode returns the First of the first regular episode: 5 for 'S01E05', and
// 1 for the batch '[Grp] Show - 01-12' as for 'S01E01-E03', so Shape is what
// tells one episode from several. ok is false when the name gives no regular
// episode, which tells a pack ('S01') from a written episode 0 ('S01E00' and
// '[Grp] Show - 00' give 0 and true). An extra is no regular episode: a
// Numbering that holds extras alone gives 0 and false.
func (n Numbering) Episode() (episode int, ok bool) {
	for i := range n.Episodes {
		if n.Episodes[i].Kind == EpRegular {
			return n.Episodes[i].First, true
		}
	}
	return 0, false
}

// Absolute returns the episode when the name numbers its episodes absolutely
// and not within a season, ie '[Group] One Piece - 1169', and Episode gives
// the same number. What Absolute adds is that no season qualifies it, so it is
// not comparable against a season's episode without an external mapping
// (AniDB, anime-relations, TheTVDB). ok is false for a season relative number
// ('Hell Mode S2 - 04') and for a special ('Show.S00E05'), both of which are
// already qualified. A written episode 0 is absolute like any other: '[Group]
// Show - 00' gives 0 and true, and a name with no episode 0 and false.
//
// The name has to give one regular episode and no more. A range has no
// absolute number ('01-12', 'E05-E07'), and neither have two episodes
// ('[Group] Show - 00 - 05', 'Episode 1 - 12'), half an episode ('21.5') or
// an extra.
//
// A name may write the absolute episode beside the season and episode, as a
// second numbering ('S23E19.Episode.1174', '[Group] One Piece - 1174
// (S23E19)'). Absolute then gives it, 1174 and true, and Episode gives the
// season's episode, 19. The season's episode written again is no absolute
// number ('S02E06 [Episode 6]'), and neither is a second numbering beside the
// specials ('S00E05.Episode.12').
func (n Numbering) Absolute() (episode int, ok bool) {
	season, seasoned := n.Season()
	if !seasoned {
		for _, e := range n.Episodes {
			switch {
			case e.Kind != EpRegular:
			case ok || !e.single() || e.Frac != 0:
				return 0, false
			default:
				episode, ok = e.First, true
			}
		}
		return episode, ok
	}
	// beside a season the first second numbering is the absolute episode,
	// unless it writes the season's episode again or the season is the
	// specials
	first, _ := n.Episode()
	for _, e := range n.Alt {
		if e.Kind == EpRegular && e.single() && e.Frac == 0 {
			if season == 0 || e.First == first {
				return 0, false
			}
			return e.First, true
		}
	}
	return 0, false
}

// Shape returns what the numbering covers, by its regular episodes. It is
// ShapeEpisode for one regular Ep that is no range, and ShapeEpisodes for any
// other regular episodes. With none it is ShapeDaily for a SchemeDaily, and
// else ShapeSeason or ShapeSeasons by the number of Seasons. A second
// numbering does not count, so 'S23E19.Episode.1174' is one episode, and
// neither does an extra: a Numbering that holds extras alone has the shape of
// its seasons, or ShapeNone.
func (n Numbering) Shape() Shape {
	// regular is how many regular episodes there are, counted up to two, and
	// one whether the first of them is no range
	var regular int
	var one bool
	for _, e := range n.Episodes {
		if e.Kind != EpRegular {
			continue
		}
		if regular++; regular == 2 {
			break
		}
		one = e.single()
	}
	switch {
	case regular == 1 && one:
		return ShapeEpisode
	case regular != 0:
		return ShapeEpisodes
	case n.Scheme == SchemeDaily:
		return ShapeDaily
	case len(n.Seasons) == 1:
		return ShapeSeason
	case len(n.Seasons) != 0:
		return ShapeSeasons
	}
	return ShapeNone
}

// All returns an iterator over the season and the episode of every regular
// Ep in Episodes, in their order, with the episodes inside a range filled in:
// 'S01E02-04' yields 1 and 2, 1 and 3, 1 and 4. The season is NoSeason for an
// episode that has none. A season with no episode yields nothing ('S01'), and
// neither does a second numbering or an extra.
//
// The ranges of a Numbering are filled with at most maxRangeFill episodes
// between them, and a range that does not fit in what is left yields its two
// ends alone ('E1-E99999'), which keeps a name such as
// 'E1-E99999.E1-E99999...' from growing into millions of episodes.
//
// All is safe on a value that no parse made, such as one decoded from a
// stored form. An Ep whose Last is not above its First yields its First
// alone, a number below 0 is yielded as it is, and no value yields more than
// two pairs for each Ep and maxRangeFill pairs beside them.
func (n Numbering) All() iter.Seq2[int, int] {
	return func(yield func(int, int) bool) {
		fill := uint64(maxRangeFill)
		for _, e := range n.Episodes {
			if e.Kind != EpRegular {
				continue
			}
			if !yield(e.Season, e.First) {
				return
			}
			if e.single() {
				continue
			}
			// unsigned, as the distance between two ints may not fit in one
			if between := uint64(e.Last) - uint64(e.First) - 1; between <= fill {
				fill -= between
				for episode := e.First + 1; episode < e.Last; episode++ {
					if !yield(e.Season, episode) {
						return
					}
				}
			}
			if !yield(e.Season, e.Last) {
				return
			}
		}
	}
}

// numbering reads the seasons and the episodes of the release from its series
// tags. It also reports whether a tag that is no second numbering writes an
// episode other than 0 in its first place, which Release.numbered keeps.
//
// The tags are walked twice and nothing is looked up across the name for one
// of them, so that the time grows with the length of the name and not with
// its square: a name is untrusted input, and may be all numbering.
//
// An episode written with no season is a second numbering when the name
// writes the episode of its season too, and goes to Alt: the 'Episode.1174'
// of 'S23E19.Episode.1174' and the '1174' of '1174 (S23E19)'. Its season is
// the one written nearest before it, so the '13' of 'S2 - 13 (S02E01)' is
// one, and the 'Episode.2' of 'S01E01.S02.Episode.2' is episode 2 of season
// 2. With no season before it, any season written with an episode makes it
// one. Two kinds of tag are another episode and never a second numbering: an
// 'E06' continuation, and the end of a range ('S04E29-31v2').
//
// A hyphen or a tilde makes a range of two episodes when the end is the
// higher number, inside a tag ('S01E02-04') or bare between two tags of one
// season ('E02-E04'). A second numbering starts no range and ends none: the
// episodes of 'S01E01.Episode.5-E07' are 1 and 7 with 5 in Alt, and those of
// 'S23E19-1174v2' are 19 with 1174 in Alt.
func (r Release) numbering() (n Numbering, numbered bool) {
	// the room most names need, so that only what is kept is allocated
	var wbuf, sbuf [4]int
	var ebuf, abuf [4]Ep
	// with are the seasons written with an episode, in rising order
	with, series := wbuf[:0], false
	for _, tag := range r.tags {
		if tag.typ != TagTypeSeries {
			continue
		}
		if series = true; tag.val(1) != "" && tag.wroteEpisode() {
			with = append(with, atoi(tag.val(1)))
		}
	}
	if !series {
		return n, false
	}
	slices.Sort(with)
	// what is not a second numbering, and what is
	const (
		none = iota
		regular
		second
	)
	seasons, eps, alt := sbuf[:0], ebuf[:0], abuf[:0]
	// season is the season an episode written with none is in, and near the
	// season written nearest before the tag, which a range or a list of
	// seasons leaves as it is
	season, near, fill := NoSeason, NoSeason, maxRangeFill
	// open is which of the two the series tag before wrote its last episode
	// as, which a hyphen may then make a range of
	open := none
	for i, tag := range r.tags {
		if tag.typ != TagTypeSeries {
			continue
		}
		bare := tag.val(1) == ""
		if !bare {
			s, start := atoi(tag.val(1)), r.seasonRange(i)
			if start != -1 {
				if prev := atoi(r.tags[start].val(1)); s-prev-1 <= fill {
					for j := prev + 1; j < s; j++ {
						seasons = append(seasons, j)
					}
					fill -= s - prev - 1
				}
			}
			seasons = append(seasons, s)
			// an episode after a range or a list of seasons is in none of
			// them
			if season, near = s, s; start != -1 || r.seasonList(i) != -1 {
				season = NoSeason
			}
		}
		kind, in := regular, season
		if bare && tag.wroteEpisode() && !tag.continuation() && !r.rangeEnd(i) {
			again := len(with) != 0
			if near != NoSeason {
				_, again = slices.BinarySearch(with, near)
			}
			if again {
				kind, in = second, NoSeason
			}
		}
		numbered = numbered || kind == regular && atoi(tag.val(2)) != 0
		ranged := open == kind && i >= 2 && r.tags[i-2].typ == TagTypeSeries && r.tags[i-1].rangeDelim() &&
			(bare || tag.val(1) == r.tags[i-2].val(1))
		open = none
		vals := tag.rest(2)
		for k, b := range vals {
			switch b {
			case "":
				continue
			case "-":
				ranged = open != none
				continue
			}
			e := Ep{Season: in, First: atoi(b), Last: atoi(b)}
			// the fraction follows the last number of the tag, and '.50'
			// is the half that '.5' is
			if k == len(vals)-1 {
				if f := tag.fraction(); f != "" {
					e.Frac = uint8(atoi(strings.TrimRight(f[1:], "0")))
				}
			}
			if kind == second {
				alt = extend(alt, e, ranged)
			} else {
				eps = extend(eps, e, ranged)
			}
			ranged, open = false, kind
		}
	}
	n = Numbering{Seasons: kept(unique(seasons)), Episodes: kept(unique(eps)), Alt: kept(unique(alt))}
	switch {
	case n.Seasons != nil:
		n.Scheme = SchemeSeasoned
	case n.Episodes != nil:
		n.Scheme = SchemeAbsolute
	}
	return n, numbered
}

// extend adds the episode e to v: as the new end of the last Ep of v when the
// two are the ends of a range and e is the higher one, and else as an Ep of
// its own.
func extend(v []Ep, e Ep, ranged bool) []Ep {
	if last := len(v) - 1; ranged && e.First > v[last].Last {
		v[last].Last, v[last].Frac = e.Last, e.Frac
		return v
	}
	return append(v, e)
}

// unique drops from v every value that one before it equals, and keeps the
// order of the others. It writes them over the start of v.
func unique[T comparable](v []T) []T {
	// one comparison with each value before is the least work for the few
	// values of a name, and a set keeps the time linear in a name that
	// repeats thousands ('E1.E2.E3...')
	const few = 32
	if len(v) > few {
		seen, k := make(map[T]struct{}, len(v)), 0
		for _, x := range v {
			if _, ok := seen[x]; !ok {
				seen[x], v[k], k = struct{}{}, x, k+1
			}
		}
		return v[:k]
	}
	k := min(len(v), 1)
	for i := 1; i < len(v); i++ {
		if !slices.Contains(v[:k], v[i]) {
			v[k], k = v[i], k+1
		}
	}
	return v[:k]
}

// kept returns a copy of v with no room to spare, which is what a release
// holds on to, and nil for no values.
func kept[T any](v []T) []T {
	if len(v) == 0 {
		return nil
	}
	return append(make([]T, 0, len(v)), v...)
}
