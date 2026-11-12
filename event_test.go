package rls

import (
	"strings"
	"testing"
)

func TestEvent(t *testing.T) {
	const tail = ".1080p.WEB.h264-GRP"
	for _, test := range []struct {
		s   string
		exp Event
	}{
		// a league with a date and the two teams
		{"NFL.2024.10.13.Dallas.Cowboys.vs.Detroit.Lions", Event{Competition: "NFL", Season: "2024"}},
		{"NBA.2024.10.22.Lakers.vs.Timberwolves", Event{Competition: "NBA", Season: "2024"}},
		{"Bundesliga.2024.05.18.Borussia.Dortmund.Vs.SV.Darmstadt", Event{Competition: "Bundesliga", Season: "2024"}},
		{"Champions.League.2024.05.01.Bayern.vs.Real.Madrid", Event{Competition: "UEFA Champions League", Season: "2024"}},
		// the ways names write one competition
		{"Formula1.2024.Round.05.Miami.Grand.Prix.Race", Event{Competition: "Formula 1", Season: "2024", Round: 5, RoundKind: "Round", Session: "Race"}},
		{"Formula.1.2024.Round.05.Miami.Grand.Prix.Race", Event{Competition: "Formula 1", Season: "2024", Round: 5, RoundKind: "Round", Session: "Race"}},
		{"F1.2024.R05.Miami.Grand.Prix.Qualifying", Event{Competition: "Formula 1", Season: "2024", Round: 5, RoundKind: "Round", Session: "Qualifying"}},
		{"Formel.1.2024.Round05.Miami.Rennen", Event{Competition: "Formula 1", Season: "2024", Round: 5, RoundKind: "Round", Session: "Race"}},
		// the year and the round written as a season and an episode
		{"Formula1.2024x05.Miami.Grand.Prix.Race", Event{Competition: "Formula 1", Season: "2024", Round: 5, RoundKind: "Round", Session: "Race"}},
		{"Formula1.S2024E05.Miami.Grand.Prix.Sprint", Event{Competition: "Formula 1", Season: "2024", Round: 5, RoundKind: "Round", Session: "Sprint"}},
		// sessions, with the number in digits and as a word
		{"MotoGP.2026.Germany.Free.Practice.Two", Event{Competition: "MotoGP", Season: "2026", Session: "Practice 2"}},
		{"Formula1.2026.Hungarian.Grand.Prix.FP3", Event{Competition: "Formula 1", Season: "2026", Session: "Practice 3"}},
		{"Formula1.2026.Hungarian.Grand.Prix.Sprint.Shootout", Event{Competition: "Formula 1", Season: "2026", Session: "Sprint Qualifying"}},
		{"MotoGP.2026.Round.09.Italian.GP.Warm.Up", Event{Competition: "MotoGP", Season: "2026", Round: 9, RoundKind: "Round", Session: "Warm Up"}},
		// what a round counts
		{"NFL.2024.Week.06.Dallas.Cowboys.vs.Detroit.Lions", Event{Competition: "NFL", Season: "2024", Round: 6, RoundKind: "Week"}},
		{"EPL.2024.Matchday.08.Arsenal.vs.Liverpool", Event{Competition: "Premier League", Season: "2024", Round: 8, RoundKind: "Matchday"}},
		{"Tour.de.France.2024.Stage.05", Event{Competition: "Cycling", Name: "Tour de France", Season: "2024", Round: 5, RoundKind: "Stage"}},
		{"Tour.De.France.2026.Etappe.11.German", Event{Competition: "Cycling", Name: "Tour de France", Season: "2026", Round: 11, RoundKind: "Stage"}},
		{"WRC.FIA.World.Rally.Championship.2025.Paraguay.SS19", Event{Competition: "WRC", Season: "2025", Round: 19, RoundKind: "Stage"}},
		{"Golf.The.Masters.2019.Day.2", Event{Competition: "Golf", Name: "The Masters", Season: "2019", Round: 2, RoundKind: "Day"}},
		{"AFL.2026.Round.7.St.Kilda.v.West.Coast", Event{Competition: "AFL", Season: "2026", Round: 7, RoundKind: "Round"}},
		{"Wimbledon.2023.Mens.Fourth.Round.Novak.Djokovic.Vs.Hubert.Hurkacz", Event{Competition: "Tennis", Name: "Wimbledon", Season: "2023", Round: 4, RoundKind: "Round"}},
		{"Australian.Open.2024.Mens.3rd.Round.J.Shang.Vs.C.Alcaraz", Event{Competition: "Tennis", Name: "Australian Open", Season: "2024", Round: 3, RoundKind: "Round"}},
		// the round of a playoff series, whose game is not kept
		{"NHL.2024.Stanley.Cup.R3G5.Oilers.vs.Stars", Event{Competition: "NHL", Name: "Stanley Cup", Season: "2024", Round: 3, RoundKind: "Round"}},
		// a season over two years
		{"Premier.League.2025-26.Matchday.32.Arsenal.vs.Chelsea", Event{Competition: "Premier League", Season: "2025-26", Round: 32, RoundKind: "Matchday"}},
		{"NHL.2024-2025.Regular.Season.Oilers.vs.Stars", Event{Competition: "NHL", Season: "2024-25"}},
		// the year of the tournament, played in the December before
		{"Darts.WM.2018.-.2017.12.30.Halbfinale.German", Event{Competition: "Darts", Name: "PDC World Darts Championship", Season: "2018"}},
		// numbered events and the parts of a card
		{"UFC.300.Pereira.vs.Hill.Early.Prelims", Event{Competition: "UFC", Number: 300, Part: "Early Prelims"}},
		{"UFC.300.Pereira.vs.Hill.Prelims", Event{Competition: "UFC", Number: 300, Part: "Prelims"}},
		{"UFC.300.Pereira.vs.Hill.Main.Card", Event{Competition: "UFC", Number: 300, Part: "Main Card"}},
		{"UFC.300.Pereira.vs.Hill", Event{Competition: "UFC", Number: 300}},
		// a numbered card and a Fight Night with the same number are two events
		{"UFC.240.Holloway.vs.Edgar", Event{Competition: "UFC", Number: 240}},
		{"UFC.Fight.Night.240.Allen.vs.Curtis.2", Event{Competition: "UFC", Name: "Fight Night", Number: 240}},
		{"UFC.on.ESPN.64.Moreno.vs.Erceg", Event{Competition: "UFC", Name: "on ESPN", Number: 64}},
		{"Bellator.296.Mousasi.vs.Edwards", Event{Competition: "Bellator", Number: 296}},
		// shows and events inside a promotion
		{"WWE.Raw.2024.10.14", Event{Competition: "WWE", Name: "Raw", Season: "2024"}},
		{"WWE.Monday.Night.Raw.2024.10.14", Event{Competition: "WWE", Name: "Raw", Season: "2024"}},
		{"WWE.Friday.Night.Smackdown.2026.07.17", Event{Competition: "WWE", Name: "SmackDown", Season: "2026"}},
		{"AEW.Dynamite.2023.07.26", Event{Competition: "AEW", Name: "Dynamite", Season: "2023"}},
		{"WWE.Royal.Rumble.2024.Pre-Show", Event{Competition: "WWE", Name: "Royal Rumble", Season: "2024", Part: "Pre-Show"}},
		{"AEW.WrestleDream.2026.PPV.Zero.Hour", Event{Competition: "AEW", Name: "WrestleDream", Season: "2026", Part: "Pre-Show"}},
		// roman numerals, and the night of an event over two
		{"WWE.WrestleMania.XL.Night.Two.2024", Event{Competition: "WWE", Name: "WrestleMania", Number: 40, Season: "2024", Round: 2, RoundKind: "Day"}},
		{"WWE.WrestleMania.38.Tag.1.2022.German", Event{Competition: "WWE", Name: "WrestleMania", Number: 38, Season: "2022", Round: 1, RoundKind: "Day"}},
		{"Super.Bowl.L.Panthers.vs.Broncos.FRENCH", Event{Competition: "NFL", Name: "Super Bowl", Number: 50}},
		{"Super.Bowl.LVIII.2024.Kansas.City.Chiefs.vs.San.Francisco.49ers", Event{Competition: "NFL", Name: "Super Bowl", Number: 58, Season: "2024"}},
		{"NFL.2026.02.08.Super.Bowl.LX.Seattle.Seahawks.Vs.New.England.Patriots", Event{Competition: "NFL", Name: "Super Bowl", Number: 60, Season: "2026"}},
		// a tournament with its sport in front, and alone
		{"Tennis.US.Open.2025.Mens.Final.Jannik.Sinner.vs.Carlos.Alcaraz", Event{Competition: "Tennis", Name: "US Open", Season: "2025"}},
		{"Tennis.Wimbledon.2023.Mens.Final.Carlos.Alcaraz.Vs.Novak.Djokovic", Event{Competition: "Tennis", Name: "Wimbledon", Season: "2023"}},
		{"Wimbledon.2023.Mens.Final.Carlos.Alcaraz.Vs.Novak.Djokovic", Event{Competition: "Tennis", Name: "Wimbledon", Season: "2023"}},
		{"Cycling.Giro.D.Italia.2026.Stage.21.Highlights", Event{Competition: "Cycling", Name: "Giro d'Italia", Season: "2026", Round: 21, RoundKind: "Stage"}},
		{"Giro.Ditalia.2023.Stage.08", Event{Competition: "Cycling", Name: "Giro d'Italia", Season: "2023", Round: 8, RoundKind: "Stage"}},
		{"ATP.French.Open.2021.Second.Round.N.Djokovic.Vs.P.Cuevas", Event{Competition: "Tennis", Name: "French Open", Season: "2021", Round: 2, RoundKind: "Round"}},
		{"Roland.Garros.2021.Second.Round.N.Djokovic.Vs.P.Cuevas", Event{Competition: "Tennis", Name: "French Open", Season: "2021", Round: 2, RoundKind: "Round"}},
		{"Cycling.Tour.de.France.Femmes.2024.Stage.3", Event{Competition: "Cycling", Name: "Tour de France Femmes", Season: "2024", Round: 3, RoundKind: "Stage"}},
		{"Darts.WM.2018.-.2017.12.20.Erste.Runde.Highlights.German", Event{Competition: "Darts", Name: "PDC World Darts Championship", Season: "2018", Round: 1, RoundKind: "Round"}},
		// a league of a team sport is the competition, with or without the
		// word for the sport
		{"Fussball.1.Bundesliga.2011-2012.04.Spieltag.Hannover.96.vs.FSV.Mainz.05.GERMAN", Event{Competition: "Bundesliga", Season: "2011-12", Round: 4, RoundKind: "Matchday"}},
		{"Rugby.Super.League.2024.08.23.Leeds.Rhinos.Vs.Catalans.Dragons", Event{Competition: "Super League", Season: "2024"}},
		{"NBA.Summer.League.2026.07.13.Dallas.Mavericks.vs.Memphis.Grizzlies", Event{Competition: "NBA", Name: "Summer League", Season: "2026"}},
		// an event held once a year needs no more than the year
		{"Paris.Roubaix.2024", Event{Competition: "Cycling", Name: "Paris-Roubaix", Season: "2024"}},
		{"Indianapolis.500.2024", Event{Competition: "IndyCar", Name: "Indianapolis 500", Season: "2024"}},
		// a session is read for motorsport alone: a sprint in cycling and a
		// qualifier in football are other things, and the day of a Test is
		// a round
		{"Tour.de.France.2024.Stage.3.Sprint.Finish.Highlights", Event{Competition: "Cycling", Name: "Tour de France", Season: "2024", Round: 3, RoundKind: "Stage"}},
		{"FIFA.World.Cup.2026.Qualifying.England.vs.Serbia", Event{Competition: "FIFA World Cup", Season: "2026"}},
		{"Cricket.The.Ashes.2023.1st.Test.Day.1.England.vs.Australia", Event{Competition: "The Ashes", Season: "2023", Round: 1, RoundKind: "Day"}},
		// the number of a Fight Night is its number, not a night
		{"Bellator.Fight.Night.5.Smith.vs.Jones", Event{Competition: "Bellator", Name: "Fight Night", Number: 5}},
		{"ONE.Fight.Night.20.Todd.vs.Phetjeeja", Event{Competition: "ONE Championship", Name: "Fight Night", Number: 20}},
		// the number after the word is the round, not the one before it
		{"Serie.A.2024.25.Giornata.10.Inter.vs.Juventus", Event{Competition: "Serie A", Season: "2024", Round: 10, RoundKind: "Matchday"}},
		// the word for a sport
		{"Boxing.2024-10-12.Beterbiev.vs.Bivol", Event{Competition: "Boxing", Season: "2024"}},
		// a league, a year and the event after it
		{"Olympics.Paris.2024.Opening.Ceremony", Event{Competition: "Olympics", Season: "2024"}},
		{"PGA.Tour.2019.John.Deere.Classic.Finalrunde.GERMAN", Event{Competition: "PGA Tour", Season: "2019"}},
	} {
		t.Run(test.s, func(t *testing.T) {
			r := ParseString(test.s + tail)
			ev, ok := r.Event()
			if !ok {
				t.Fatalf("expected an event, got none (type %s, title %q, subtitle %q)", r.Type, r.Title, r.Subtitle)
			}
			if ev != test.exp {
				t.Errorf("expected:\n  %+v\ngot:\n  %+v", test.exp, ev)
			}
		})
	}
}

func TestEventNone(t *testing.T) {
	for _, s := range []string{
		// no competition
		"The.Matrix.1999.1080p.BluRay.x264-GRP",
		"Show.S01E02.1080p.WEB.h264-GRP",
		"[SubsPlease] One Piece - 1100 (1080p) [ABCD1234].mkv",
		"Friday.Night.Lights.S01E01.1080p.WEB.h264-GRP",
		"",
		// a film that starts as a competition does
		"F1.The.Movie.2025.1080p.WEB-DL.DDP5.1.Atmos.H.264-GRP",
		"F1.2025.1080p.BluRay.x264-GRP",
		"Wimbledon.2004.1080p.BluRay.x264-GRP",
		"Le.Mans.66.2019.German.DL.1080p.BluRay.x264-GRP",
		"Boxing.Day.2021.1080p.WEB.h264-GRP",
		"Cricket.On.The.Hearth.1967.1080p.BluRay.x264-OLDTiME",
		"World.Cup.Heist.2020.1080p.AMZN.WEB-DL.DDP2.0.H264-WORM",
		"Rugby.2019.1080p.WEB.h264-GRP",
		"Wimbledon.2004.Extended.Cut.1080p.BluRay.x264-GRP",
		"Tour.de.France.2024.Documentary.1080p.WEB.h264-GRP",
		// a film or a show named with the words of a session or a stage
		"Formula.1.Race.Against.Time.Documentary.2024.1080p.WEB.h264-GRP",
		"Le.Mans.Race.Of.The.Century.2019.1080p.WEB.h264-GRP",
		"Premier.League.Highlights.Of.The.Season.2023.1080p.WEB.h264-GRP",
		// a show of a competition with nothing that dates it
		"WWE.Royal.Rumble.Documentary.1080p.WEB.h264-GRP",
		"WWE.Monday.Night.Raw",
		// a studio show with a date
		"NFL.Gameday.Morning.2024.10.13.1080p.WEB.h264-GRP",
		"Golf.Central.2024.05.01.720p.WEB.h264-GRP",
		// a show numbered by year
		"Serie.A.Highlights.Show.S2024E05.1080p.WEB.h264-GRP",
		// a word that is made of the letters of a roman numeral
		"Super.Bowl.I.Was.There.2019.1080p.WEB.h264-GRP",
		// a show about the sport, numbered by season and episode
		"WWE.Unreal.S01E02.1080p.WEB.h264-EDITH",
		"NASCAR.Full.Speed.S01E03.1080p.WEB.h264-GRP",
		"Formula.1.Drive.to.Survive.S06E01.1080p.WEB.h264-GRP",
		"LALIGA.All.Access.S01E02.iTALiAN.720p.WEB.H264-NEUROSiS",
		// a compilation with a season and nothing else
		"Premier.League.Goals.Of.The.Season.2010-2011.1080p.HDTV.x264-ACES",
		"Super.Bowl.Greatest.Commercials.2020.720p.HDTV.x264-W4F",
		// a game
		"NHL.24-RUNE",
		"F1.23-RUNE",
		"UFC.5.PS5-DUPLEX",
		"WRC.10.FIA.World.Rally.Championship-CODEX",
	} {
		t.Run(s, func(t *testing.T) {
			if ev, ok := ParseString(s).Event(); ok || ev != (Event{}) {
				t.Errorf("expected no event, got: %+v, %t", ev, ok)
			}
		})
	}
	// a release that was not parsed has none
	if ev, ok := (Release{}).Event(); ok || ev != (Event{}) {
		t.Errorf("expected no event for the zero Release, got: %+v, %t", ev, ok)
	}
}

// TestEventFields checks that Event answers from the fields of a release, so
// that it follows a release a caller built or changed.
func TestEventFields(t *testing.T) {
	r := Release{Type: Episode, Title: "NFL", Subtitle: "Dallas Cowboys vs Detroit Lions", Year: 2024, Month: 10, Day: 13}
	ev, ok := r.Event()
	if exp := (Event{Competition: "NFL", Season: "2024"}); !ok || ev != exp {
		t.Errorf("expected %+v, got: %+v, %t", exp, ev, ok)
	}
	r = Release{Type: Movie, Title: "Formula 1", Subtitle: "Miami Grand Prix Practice One", Year: 2024}
	ev, ok = r.Event()
	if exp := (Event{Competition: "Formula 1", Season: "2024", Session: "Practice 1"}); !ok || ev != exp {
		t.Errorf("expected %+v, got: %+v, %t", exp, ev, ok)
	}
}

// TestEventSeason checks the season for years a parsed name never gives, which
// a caller can set.
func TestEventSeason(t *testing.T) {
	for _, test := range []struct {
		year, end int
		exp       string
	}{
		{2024, 0, "2024"},
		{2024, 2025, "2024-25"},
		{1999, 2000, "1999-00"},
		{1999, 2003, "1999-2003"},
		{2024, 2024, "2024"},
		{2024, 2020, "2024"},
		{0, 0, ""},
		{0, 1, ""},
		{-1, 0, ""},
		{5, 6, "5-6"},
		{99, 100, "99-100"},
		{9999, 10000, "9999-10000"},
	} {
		if got := eventSeason(test.year, test.end); got != test.exp {
			t.Errorf("years %d and %d: expected %q, got: %q", test.year, test.end, test.exp, got)
		}
		r := Release{Type: Episode, Title: "NFL", Subtitle: "A vs B", Year: test.year, YearEnd: test.end}
		if ev, ok := r.Event(); !ok || ev.Season != test.exp {
			t.Errorf("years %d and %d: expected an event of season %q, got: %+v, %t", test.year, test.end, test.exp, ev, ok)
		}
	}
}

// TestEventVocabulary checks the tables Event reads: every spelling is in the
// form the words of a name are compared in, no spelling is listed twice with
// two meanings, and every name belongs to a competition that exists.
func TestEventVocabulary(t *testing.T) {
	check := func(table, spellings string, seen map[string]string, value string) {
		for spelling := range strings.SplitSeq(spellings, "|") {
			if spelling == "" || spelling != strings.Join(strings.Fields(spelling), " ") {
				t.Errorf("%s: spelling %q is empty or has stray spaces", table, spelling)
			}
			for _, c := range spelling {
				if c != ' ' && c != '#' && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c < 0x80 {
					t.Errorf("%s: spelling %q has %q, which the words of a name never have", table, spelling, c)
				}
			}
			if prev, ok := seen[spelling]; ok && prev != value {
				t.Errorf("%s: spelling %q is listed for %q and for %q", table, spelling, prev, value)
			}
			seen[spelling] = value
		}
	}
	competitions := make(map[string]bool)
	seen := make(map[string]string)
	for _, row := range eventCompetitions {
		competitions[row.competition] = true
		check("eventCompetitions", row.spellings, seen, row.competition+"/"+row.name)
	}
	names := make(map[string]map[string]string)
	for _, row := range eventNames {
		if !competitions[row.competition] {
			t.Errorf("eventNames: %q is in competition %q, which eventCompetitions does not have", row.name, row.competition)
		}
		if names[row.competition] == nil {
			names[row.competition] = make(map[string]string)
		}
		check("eventNames of "+row.competition, row.spellings, names[row.competition], row.name)
	}
	words := make(map[string]string)
	kinds := map[string]bool{"Round": true, "Week": true, "Matchday": true, "Stage": true, "Day": true, "Game": true}
	for _, row := range eventRounds {
		if !kinds[row.value] {
			t.Errorf("eventRounds: %q is not a RoundKind", row.value)
		}
		check("eventRounds", row.spellings, words, "round "+row.value)
		for spelling := range strings.SplitSeq(row.spellings, "|") {
			if !strings.Contains(spelling, "#") {
				t.Errorf("eventRounds: spelling %q has no number", spelling)
			}
		}
	}
	for _, row := range eventSessions {
		check("eventSessions", row.spellings, words, "session "+row.value)
	}
	for _, row := range eventParts {
		check("eventParts", row.spellings, words, "part "+row.value)
	}
	check("eventVersus", eventVersus, words, "versus")
	check("eventStages", eventStages, words, "stage")
}

// TestEventHint checks the one thing Hint.Sports changes: a name is an event
// on the caller's word, with what Event can read from it.
func TestEventHint(t *testing.T) {
	sports := Hint{Sports: true}
	for _, test := range []struct {
		s   string
		exp Event
	}{
		// a competition the vocabulary does not know
		{"Curling.2026.02.10.Sweden.vs.Canada.1080p.WEB.h264-GRP", Event{Season: "2026"}},
		{"Speedway.GP.2026.Round.03.Warsaw.1080p.WEB.h264-GRP", Event{Season: "2026", Round: 3, RoundKind: "Round"}},
		{"Stock.Car.Pro.Series.2026.Etapa.05.Qualifying.1080p.WEB.h264-GRP", Event{Season: "2026", Round: 5, RoundKind: "Stage", Session: "Qualifying"}},
		// one it knows reads as it does without the hint
		{"UFC.300.Pereira.vs.Hill.Prelims.1080p.WEB.h264-GRP", Event{Competition: "UFC", Number: 300, Part: "Prelims"}},
		// a name that gives nothing is still an event when the caller says so,
		// also a show with a season and a game
		{"Wimbledon.2004.1080p.BluRay.x264-GRP", Event{Competition: "Tennis", Name: "Wimbledon", Season: "2004"}},
		{"WWE.Unreal.S01E02.1080p.WEB.h264-EDITH", Event{Competition: "WWE"}},
		{"NHL.24-RUNE", Event{Competition: "NHL"}},
	} {
		t.Run(test.s, func(t *testing.T) {
			plain, hinted := ParseString(test.s), ParseStringHint(test.s, sports)
			if ev, ok := hinted.Event(); !ok || ev != test.exp {
				t.Errorf("expected:\n  %+v\ngot:\n  %+v, %t", test.exp, ev, ok)
			}
			// the release itself is the one the parser gives without the hint
			hinted.sports = false
			if !sameRelease(hinted, plain) {
				t.Errorf("the hint changed the release:%s", diffFields(hinted, plain))
			}
		})
	}
	for _, s := range []string{
		"Curling.2026.02.10.Sweden.vs.Canada.1080p.WEB.h264-GRP",
		"Speedway.GP.2026.Round.03.Warsaw.1080p.WEB.h264-GRP",
	} {
		if ev, ok := ParseString(s).Event(); ok {
			t.Errorf("%q: expected no event without the hint, got: %+v", s, ev)
		}
	}
}
