package rls

import (
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Event is what the name of a sports broadcast says about its event.
type Event struct {
	// Competition is the league, series or promotion, in one spelling for all
	// the ways names write it: "Formula 1" for 'Formula1', 'Formula.1' and
	// 'F1'. For a tournament of a sport that names write alone it is the
	// sport, with the tournament in Name: 'Wimbledon.2023' is "Tennis" and
	// "Wimbledon", as 'Tennis.Wimbledon.2023' is.
	Competition string
	// Name is the show, the tournament or the numbered series inside the
	// competition, when the vocabulary knows it: "Fight Night", "Raw",
	// "WrestleMania", "US Open".
	Name string
	// Number is the number of a numbered event: 300 for 'UFC 300', 240 for
	// 'UFC Fight Night 240', 40 for 'WrestleMania XL'. It is 0 when the name
	// gives none.
	Number int
	// Season is the year of the season, or its two years: "2024", "2024-25".
	// A longer span of years is written in full, "1999-2003".
	Season string
	// Round is the number of the round, week, matchday, stage, day or game.
	// It is 0 when the name gives none.
	Round int
	// RoundKind is what Round counts: "Round", "Week", "Matchday", "Stage",
	// "Day" or "Game".
	RoundKind string
	// Session is the session of a race weekend: "Practice 1" to "Practice
	// 3", "Qualifying", "Sprint Qualifying", "Sprint", "Race", "Warm Up" or
	// "Testing". It is read for motorsport alone.
	Session string
	// Part is the part of a fight card or of a show: "Early Prelims",
	// "Prelims", "Main Card", "Pre-Show" or "Post-Show".
	Part string
}

// Event returns the event of a sports name, and false when the name does not
// start with a competition the vocabulary knows.
//
// It reads Title and Subtitle, the date and the season and episode, and it
// changes none of them: Type, Title and Subtitle are what they are without
// it. A competition alone does not make an event, since a film or a game can
// start the same way ('Wimbledon.2004', 'F1.23'). The name also has to give
// one of these: a date or a stage of the tournament ('Final') right after the
// competition, an event number, a round, a part of a card, two sides ('vs'),
// a show or tournament of the competition with a year, or after the year a
// session or, for a league, any text at all. A name with a season and an
// episode that is not a year is a show about the sport ('WWE.Unreal.S01E02')
// and not an event, and neither is a game, an app, music or a book.
//
// A release parsed with Hint.Sports is an event on the caller's word. Event
// then returns true for any name of any type, with an empty Competition when
// it knows none, and with whatever round, session and part the name gives.
func (r Release) Event() (Event, bool) {
	if !r.sports {
		switch r.Type {
		case App, Audiobook, Book, Comic, Education, Game, Magazine, Music:
			return Event{}, false
		}
		if r.Series != 0 && r.Series < 1900 || r.Series == 0 && r.Episode != 0 {
			return Event{}, false
		}
	}
	title, sub := eventWords(r.Title), eventWords(r.Subtitle)
	v := eventVocabulary()
	var ev Event
	// pos is where the competition, its name and its number end in the title
	var pos int
	var flags uint8
	c, _ := v.find("c:", title, 0)
	switch {
	case c != nil:
		ev.Competition, ev.Name, pos, flags = c.competition, c.name, len(c.words), c.flags
	case !r.sports:
		return Event{}, false
	}
	// the name follows the competition, or the year after it
	// ('NHL.2024.Stanley.Cup')
	var named bool
	numbered, roman := flags&evNumbered != 0, flags&evRoman != 0
	if c != nil && ev.Name == "" {
		key := "n:" + ev.Competition + ":"
		if n, _ := v.find(key, title, pos); n != nil {
			ev.Name, pos, named = n.name, pos+len(n.words), true
			numbered, roman = n.flags&evNumbered != 0, n.flags&evRoman != 0
		} else if n, _ := v.find(key, sub, 0); n != nil {
			ev.Name, sub, named = n.name, sub[len(n.words):], true
			// its number follows it there ('NFL.2026.02.08.Super.Bowl.LX')
			if n.flags&evNumbered != 0 && len(sub) != 0 {
				if number := eventNumber(sub[0], n.flags&evRoman != 0); number != 0 {
					ev.Number, sub = number, sub[1:]
				}
			}
			numbered = false
		}
	}
	if numbered && pos < len(title) {
		if n := eventNumber(title[pos], roman); n != 0 {
			ev.Number, pos = n, pos+1
		}
	}
	// the season is the year written with the competition ('Darts.WM.2018',
	// played in December 2017), or else the year of the name
	if pos < len(title) && len(title[pos]) == 4 && (title[pos][:2] == "19" || title[pos][:2] == "20") && atoi(title[pos]) != 0 {
		ev.Season, pos = title[pos], pos+1
	} else {
		ev.Season = eventSeason(r.Year, r.YearEnd)
	}
	// plain is whether the title is the competition and nothing else, as it
	// is when the date or the year of an event follows. More words make it
	// the title of a show ('NFL.Gameday.Morning.2024.10.13')
	plain := pos == len(title)
	// found is whether something besides the competition says this is an
	// event. A session and a stage say so after the year alone, and a stage
	// when the title is plain: in or after a longer title they are as often
	// its words ('Race.Against.Time', 'Song.Contest.2024.Final')
	var found bool
	for k, words := range [][]string{title[pos:], sub} {
		for i := 0; i < len(words); i++ {
			p, n := v.find("w:", words, i)
			if p == nil {
				continue
			}
			switch p.kind {
			case eventKindRound:
				// a number in front of the word is not its number when
				// another follows the word ('2024.25.Giornata.10')
				if next := i + len(p.words); p.words[0][0] == '#' && next < len(words) && atoi(words[next]) != 0 {
					continue
				}
				if ev.Round == 0 && n != 0 {
					ev.Round, ev.RoundKind, found = n, p.name, true
				}
			case eventKindSession:
				// a sprint, a race and a qualifying are other things
				// outside motorsport
				if c != nil && flags&evMotor == 0 {
					continue
				}
				if ev.Session == "" {
					ev.Session, found = p.name, found || k == 1
				}
			case eventKindPart:
				if ev.Part == "" {
					ev.Part, found = p.name, true
				}
			case eventKindVersus:
				found = true
			case eventKindStage:
				found = found || k == 1 && plain
			}
			i += len(p.words) - 1
		}
	}
	// 'Formula1.2024x05' and 'S2024E05' give the year as the season and the
	// round as the episode
	if ev.Round == 0 && plain && r.Series >= 1900 && r.Episode != 0 {
		ev.Round, ev.RoundKind, found = r.Episode, "Round", true
	}
	switch {
	case r.sports, found, ev.Number != 0,
		// a show or a tournament of the competition, and the year it was held
		named && r.Year != 0,
		plain && r.Month != 0 && r.Day != 0,
		flags&evOnce != 0 && r.Year != 0,
		c != nil && flags&evWeak == 0 && r.Year != 0 && len(sub) != 0:
		return ev, true
	}
	return Event{}, false
}

// eventSeason returns the season for a year and the last year of a range of
// years: "2024", "2024-25" for the year after, "1999-2003".
func eventSeason(year, end int) string {
	switch {
	case year <= 0:
		return ""
	case end == year+1 && year >= 1000 && end <= 9999:
		return strconv.Itoa(year) + "-" + strconv.Itoa(end)[2:]
	case end > year:
		return strconv.Itoa(year) + "-" + strconv.Itoa(end)
	}
	return strconv.Itoa(year)
}

// eventNumber returns the number of an event written in up to three digits,
// or in roman numerals when roman is set, and 0 when w is not one. A roman
// numeral is written the one way a number is written, which keeps out the
// words that happen to be made of its letters ('ill', 'mix'), and is not the
// single letter 'I' or 'X', which names use for other things ('I.Quit',
// 'X-Seven').
func eventNumber(w string, roman bool) int {
	if len(w) <= 3 {
		if n := atoi(w); n != 0 {
			return n
		}
	}
	if roman && w != "i" && w != "x" {
		if n, ok := parseRoman(w); ok && 0 < n && n < 100 && eventRoman(n) == w {
			return n
		}
	}
	return 0
}

// eventRoman returns n from 1 to 99 in lower case roman numerals.
func eventRoman(n int) string {
	tens := [...]string{"", "x", "xx", "xxx", "xl", "l", "lx", "lxx", "lxxx", "xc"}
	ones := [...]string{"", "i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix"}
	return tens[n/10%10] + ones[n%10]
}

// eventWords returns the words of a title in lower case and without marks,
// split at everything that is not a letter or a digit, with apostrophes
// dropped ('Giro d'Italia' is giro, ditalia).
func eventWords(s string) []string {
	if s == "" {
		return nil
	}
	return strings.FieldsFunc(MustNormalize(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// The kinds of pattern in the vocabulary.
const (
	eventKindCompetition = iota
	eventKindName
	eventKindRound
	eventKindSession
	eventKindPart
	eventKindVersus
	eventKindStage
)

// eventPattern is one spelling of the vocabulary.
type eventPattern struct {
	// words are the words of the spelling. A word with a "#" stands for a
	// number.
	words []string
	kind  int
	// competition and name are the values a competition or a name gives.
	// For a round, a session or a part name is its value.
	competition string
	name        string
	flags       uint8
}

// eventVocab holds the spellings by the kind of thing they spell and their
// first word: "c:ufc", "n:UFC:fight", "w:stage".
type eventVocab map[string][]eventPattern

// eventVocabulary returns the vocabulary, which is built when Event is first
// called. Parsing does not pay for it.
var eventVocabulary = sync.OnceValue(func() eventVocab {
	v := make(eventVocab)
	add := func(key, spellings string, p eventPattern) {
		for spelling := range strings.SplitSeq(spellings, "|") {
			p.words = strings.Fields(spelling)
			v[key+p.words[0]] = append(v[key+p.words[0]], p)
		}
	}
	for _, row := range eventCompetitions {
		add("c:", row.spellings, eventPattern{kind: eventKindCompetition, competition: row.competition, name: row.name, flags: row.flags})
	}
	for _, row := range eventNames {
		add("n:"+row.competition+":", row.spellings, eventPattern{kind: eventKindName, name: row.name, flags: row.flags})
	}
	for _, row := range eventRounds {
		add("w:", row.spellings, eventPattern{kind: eventKindRound, name: row.value})
	}
	for _, row := range eventSessions {
		add("w:", row.spellings, eventPattern{kind: eventKindSession, name: row.value})
	}
	for _, row := range eventParts {
		add("w:", row.spellings, eventPattern{kind: eventKindPart, name: row.value})
	}
	add("w:", eventVersus, eventPattern{kind: eventKindVersus})
	add("w:", eventStages, eventPattern{kind: eventKindStage})
	return v
})

// find returns the longest pattern under key that words[i:] starts with, and
// the number its first "#" stands for.
func (v eventVocab) find(key string, words []string, i int) (*eventPattern, int) {
	if i >= len(words) {
		return nil, 0
	}
	var best *eventPattern
	var num int
	try := func(first string) {
		patterns := v[key+first]
		for k := range patterns {
			p := &patterns[k]
			if best != nil && len(p.words) <= len(best.words) || len(p.words) > len(words)-i {
				continue
			}
			n, ok := 0, true
			for j, pw := range p.words {
				m, matched := eventWord(pw, words[i+j])
				if ok = matched; !ok {
					break
				}
				if n == 0 {
					n = m
				}
			}
			if ok {
				best, num = p, n
			}
		}
	}
	try(words[i])
	if shape := eventShape(words[i]); shape != words[i] {
		try(shape)
	}
	if eventNumberWords[words[i]] != 0 {
		try("#")
	}
	return best, num
}

// eventWord reports whether the word w of a name is the word pw of a
// spelling, and the number w gives when pw has a "#".
func eventWord(pw, w string) (int, bool) {
	switch {
	case pw == w:
		return 0, true
	case !strings.Contains(pw, "#"):
		return 0, false
	case pw == "#" && eventNumberWords[w] != 0:
		return eventNumberWords[w], true
	case eventShape(w) != pw:
		return 0, false
	}
	// the first run of digits is the number
	i := strings.IndexAny(w, "0123456789")
	j := i
	for j < len(w) && w[j] >= '0' && w[j] <= '9' {
		j++
	}
	return atoi(w[i:j]), true
}

// eventShape returns w with every run of digits replaced by "#", which is how
// the vocabulary writes a word with a number in it ('r09' is "r#", '3rd' is
// "#rd", 'r3g5' is "r#g#").
func eventShape(w string) string {
	if !strings.ContainsAny(w, "0123456789") {
		return w
	}
	b := make([]byte, 0, len(w))
	for i := 0; i < len(w); i++ {
		switch digit := w[i] >= '0' && w[i] <= '9'; {
		case !digit:
			b = append(b, w[i])
		case len(b) == 0 || b[len(b)-1] != '#':
			b = append(b, '#')
		}
	}
	return string(b)
}

// eventNumberWords are the numbers names write as words ('Practice.One',
// 'Fourth.Round', 'Zweite.Runde').
var eventNumberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7,
	"erste": 1, "zweite": 2, "dritte": 3, "vierte": 4,
}
