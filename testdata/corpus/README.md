# Evaluation corpus

Independent data for measuring parser accuracy.

`labelled/anitomy.json` and `labelled/anitogo.json` are committed, each with the licence file of its
project. So are our own fixtures: the two hand-read samples and the two probe sets described under
[Our own fixtures](#our-own-fixtures). **The other data files here are gitignored** (`.gitignore`
excludes `*.txt` and `*.json`), so they live in your working tree and are not committed.
`accuracy_test.go` skips the tests that read them when they are absent, so the package still builds
and tests without them.

## Why this exists

`tests.yaml` cannot answer "did this change make the parser better or worse". Its expectations are
written by us, often from this parser's own output when a case is added, so a wrong result can
become the expectation and the parser passes it. That is not hypothetical — it is how `Hi10` came to
be recorded as `HDR10`. (`TESTS=export go test` only sorts the stored expectations; it does not
recompute them.)

The labelled sets and the corpora are labelled or collected by someone else, so they can disagree
with us. The samples and the probe sets are ours. The probe sets were written by hand. In the
samples a reader judged every value: a value the parser gave stays only where the reader found it
right, and the others were written from the name.

## Contents

| file | rows | source | labelled |
|---|---|---|---|
| `labelled/anitomy.json` | 182 | [erengy/anitomy](https://github.com/erengy/anitomy) `test/data.json` | yes |
| `labelled/habari.json` | 218 | [5rahim/habari](https://github.com/5rahim/habari) `test/data.json` | yes |
| `labelled/anitogo.json` | 184 | [nssteinbrenner/anitogo](https://github.com/nssteinbrenner/anitogo) `test/data.json` | yes |
| `labelled/habari-known-failures.json` | 11 | habari `test/failing.json` | yes, with reasons |
| `labelled/nyaa-400.json` | 400 | a random sample of `nyaa.txt`, read by hand | yes, by us |
| `labelled/nyaa-blind-140.json` | 140 | names of `nyaa.txt` that two prototypes read differently, judged blind | yes, by us |
| `labelled/probes-title.json` | 112 | scene names built by hand, a vocabulary word anywhere in the title | yes, by us |
| `labelled/probes-title-end.json` | 62 | scene names built by hand, a vocabulary word as the last word of the title | yes, by us |
| `labelled/probes-brackets.json` | 368 | names with a bracket that no fansub group wrote, built by hand or real | yes, by us |
| `nyaa.txt` | 13,215 | nyaa.si listing and search pages, categories 1_2 / 1_3 / 1_4 | no |
| `sports.txt` | 2,505 | predb SPORTS and TV sections, plus documented tracker conventions | no |

The three labelled sets are the regression suites of anime-specific parsers, so they are
deliberately adversarial — heavy on old and exotic fansub formats. `nyaa.txt` is representative of
what users actually see today. **Both matter, and they give very different pictures.** Modern
fansub releases parse far better than the labelled sets suggest.

`habari-known-failures.json` documents cases habari itself gets wrong, with reasons. Several are
failure classes this parser shares, so it is useful as a shortlist rather than as a target.

## Committed fixtures

The two committed files are the bytes of the upstream commits below, which `TestLabelledUpstream`
checks by their SHA-256 sums. They are test data. No parser code of either project is in this
repository.

| file | upstream commit | SHA-256 | licence |
|---|---|---|---|
| `labelled/anitomy.json` | [erengy/anitomy `a538eff`](https://github.com/erengy/anitomy/blob/a538eff670cb4666f8395cf66a9c29a535f2c383/test/data.json) | `a47d5336e356b0ffd50ae36128fdb861985c848dc72bca33407a69cba9fbf679` | MPL-2.0, `labelled/anitomy.LICENSE` |
| `labelled/anitogo.json` | [nssteinbrenner/anitogo `260d546`](https://github.com/nssteinbrenner/anitogo/blob/260d546976961245b80ae7bd02f0c0b52a3ecc44/test/data.json) | `2bd2f6f1c09b1e66d5fe31049896c3dff628892cd7ce5b4aafca6c48e08a576c` | MPL-2.0, `labelled/anitogo.LICENSE` |

`labelled/habari.json` is GPL-3.0 and stays out of the repository. Fetch it from
[5rahim/habari `2626d57`](https://github.com/5rahim/habari/blob/2626d57ad205a5ea41a5d060355e9314e0457c3e/test/data.json)
to measure against it locally.

## Our own fixtures

Four files are ours. They use the field names of the labelled sets, so one loader reads them all.

### The samples

The two samples hold 535 names of `nyaa.txt`, which is otherwise not committed.

`labelled/nyaa-400.json` is a random sample of 400 names from `nyaa.txt`. On 2026-11-12 an AI
model read each name beside the parser's output at commit `03ac59b` and marked the fields that were
wrong: title, season and episode, group, type. A field it accepted keeps the value the parser gave.
A field it marked wrong has a value written from the name. The accepted titles were read a second
time, and eleven that still carried a marker word were rewritten, such as the `- OVA` of `Aldnoah
Zero - OVA`. The sample estimates how often the parser is wrong on what nyaa lists today.

`labelled/nyaa-blind-140.json` is 140 names from `nyaa.txt` that the two anime prototypes of
2026-11-12 read differently from master or from each other. A separate AI reader saw each name with
the two or three outputs, shuffled and unnamed, and judged every field of every output as right,
wrong or arguable. A label is the value of an output judged right. Where no output was right, the
label is written from the name, or left out when the name does not settle it. These are hard names
on purpose. Master had 26 of the 140 fully right when the file was made.

No person has checked every row of either file. A label that looks wrong is a bug in the fixture.
Fix it in a PR and say what the name means.

In both files:

| field | meaning |
|---|---|
| `type` | the release type as `Type.String()` gives it: `movie`, `series`, `episode` |
| `anime_title` | the title. A list holds every title that is right, for a name that writes two side by side: `A / B`, `A \| B` |
| `release_group` | the group. A list holds every reading that is right, and may hold the empty string for a name that may have no group |
| `anime_season` | the season, or a list of seasons |
| `episode_number` | one episode, or the two ends of a range. Half an episode keeps its fraction, `18.5` |
| `anime_type` | the kind of an extra, or what the release holds beside it: `OVA`, `Special`, `NCOP`, `NCED`, `ED`, `Movie`, `Extra`. A list holds every kind the name states, all of them and no alternatives: `OVA + Movie` is `["OVA", "Movie"]` |
| `volume_number` | the volume or box, or the two ends of a range of them |

A field that is there and empty says the name has none: `"release_group": ""` is a name with no
group, and a parser that finds one is wrong. A field that is left out was not judged, because the
name does not settle it. The type of `[Commie] Chihayafuru 2 [BD 720p AAC]` is one: nothing in the
name says whether it is a film or a batch of a series. `type` is left out in three cases, the same
in both files:

- a name with no numbering, no year and no word such as `Movie` or `Batch`, even when the work is a
  known film;
- an extra with no regular episode beside it: an OVA, a numbered special, a creditless opening;
- a range written with an episode marker and no word for a batch, `S00E20-E21` or `(E039-E058)`.
  `tests.yaml` types `S01E01-E03` as an episode, the blind reader called such a range a pack, and
  the name does not say which.

A title written after the tags is not in the list of titles. The first title is the title there:
`Show S2 (BD 1080p) | Other Name S2`.

`anime_type` and `volume_number` are scored against `Release.Numbering`, as the fields `kind` and
`volume` of `sampleFloors`, on the rows that carry them:

- `kind` is right when the numbering gives every kind the row names and no other. A numbered extra
  gives its kind as the `Kind` of an `Ep` (`- OVA 02`, `SP01`), and an extra with no number gives it
  in `Contents` (`Title - OVA`, `Title NCOP & NCED`). `NCOP` and `NCED` are an opening and an ending,
  and `OAD` and `ONA` are scored as `OVA`.
- `volume` is right when `Numbering.Volume` is the volume of the row, which is the first one for a
  range (`Vol.1-5` is 1).

A numbered extra carries its number in `episode_number` with its kind beside it, so `- OVA 02` is
episode 2 of kind `OVA`. `Numbering.Episode()` gives regular episodes only, so on a row with an
`anime_type` the `episode` score reads the numbers of the extras when the name gives no regular
episode. A row gives one number, or the two ends of a range, which is every number between them,
and the name has to give exactly those, of every kind it gives: `Specials - 01 - 03` and `SP1&2`
are the rows `["01", "03"]` and `["1", "2"]`, and the extras 1 and 3 are not the row `["1", "3"]`.
A row with no `anime_type` is scored on its regular episodes alone, so the specials a pack lists
beside a season are not its episodes (`Title II + SP01 + SP02`).

`TestAccuracySample` scores both files and compares each field with `sampleFloors` in
`accuracy_test.go`. A floor is the number of names that are right, not a percentage. The test fails
below the floor, and above it too, so that the floor moves in the same diff as the change. Run it
with `-v` to list every name that has a field wrong.

### The probe sets

`labelled/probes-title.json` and `labelled/probes-title-end.json` are scene names built by hand
around one word of the vocabulary that is a word of the title in that name: `The.French.Dispatch.
2021`, `Going.Dutch.S01E01`, `Pardon.My.French.2021`. The corpora are anime and sports names, and a
rule that breaks scene names passes them unnoticed. In the first file the word stands anywhere in
the title: first, inside or last. In the second it is always the last word in front of the year or
the season, which is the place the parser gets wrong most.

| field | meaning |
|---|---|
| `title`, `type` | what the name must give, compared exactly |
| `no_tag` | the value the word would have as a tag, or a list of them. No field may hold it. Each is a value of `taginfo.csv`, which the test checks. It is left out when the name carries the same value as a real tag, as `French.Exit.2020.FRENCH` does |

`TestProbes` runs them. A probe the parser gets wrong today is listed in `probeFailures` in
`accuracy_test.go`, with the reading the parser gives. A probe that is not listed has to pass, and
a listed one has to fail with that reading, so a change that fixes a probe takes it off the list,
and one that reads it in another wrong way writes the new reading.

A row says what the name means, whatever the parser reads. A row whose name does not say what the
release is gives no `type`, and is then not judged on it: `Show Name (2020) OVA [1080p]` is one
film or a set of episodes. A name whose meaning is open in more than that is no probe.

When you add a tag row or a rule that touches title words, add probes for it: movies, episodes,
packs, with the word first, inside and last.

The rows around a bare `UHD`, one that the lexer reads as a source of its own and that a web
source stands behind or in front of, hold the source that `UHD` must not give. A web release with `Web` in
its title or a streaming service between gives no `UHD.BluRay` (`The.Web.S01E02.2160p.UHD.NF.
WEBRip`), and a disc or a `WEB` group gives no web source (`Movie.2020.UHD.2160p.REMUX.HEVC.TrueHD.
Atmos.7.1.AMZN.WEB-DL.DDP5.1.Audio`, `Movie.2020.2160p.UHD.x265-WEB`). Most of them have no
vocabulary word in the title, and their `no_tag` is that source.

`labelled/probes-brackets.json` holds the look-alikes of a fansub name: names with a bracket that
no fansub group wrote. They are films, shows, records, books, games and applications in the shapes
of YTS, QxR, TGx, a Plex library and a tracker listing, with a word or a number that the structure
stage reads in a fansub name: `Kill Bill: Vol. 1 (2003) [BluRay] [1080p] [YTS.AM]`, `The Specials
(2019) [720p] [BluRay] [YTS.LT]`, `Movie 43 (2013) [1080p]`, `[FLAC] Artist - The Complete Series
Box (2015)`. The nyaa corpus holds none of them, so a gate of the stage that is too wide passes it
unnoticed. Beside `title` and `type` a row gives what the name holds, and the release must hold
that and nothing more:

| field | meaning |
|---|---|
| `anime_type` | the contents and the kinds of the extras the name gives. Left out: none |
| `episode_number` | the number of an extra that `anime_type` names |
| `volume_number` | the volume. Left out: none |
| `batch` | `true` when the name says it is a batch |

No probe has another title, so `Alt` must be empty. The two sets of scene names are checked the
same way, and none of their names holds a unit. When you add a rule to the structure stage, or
widen what its gate lets in, add the names of other kinds that look like its target.

## Using it

```sh
go test -run TestAccuracyLabelled -v .   # per-field accuracy against the labelled sets
go test -run TestAccuracySample -v .     # the hand-read samples, with every wrong name
go test -run TestProbes -v .             # the scene probe sets
go test -run TestCorpusStructural -v .   # round trip + structural stats over the unlabelled corpora
go test -short ./...                     # skips the corpus walk
```

`TestAccuracyLabelled` fails when a field drops below the floors recorded in `accuracy_test.go`.
Raise a floor when you improve a field; lower one only deliberately, and say why.

`TestCorpusStructural` asserts the round-trip invariant — `fmt.Sprintf("%o", release)` must
reproduce the input exactly — across all 15,720 unlabelled names, which is a far wider net than the
429 cases in `tests.yaml`.

## Reproducing or extending it

The labelled sets are the `test/data.json` files of the upstream projects; fetch them directly.
`nyaa.txt` was assembled from nyaa.si listing pages and deduplicated. `sports.txt` came from the
predb API's SPORTS and TV sections.

Two caveats if you regenerate:

- Deduplicate. The raw nyaa pull had many near-identical rows across resolutions.
- The labelled fixtures use different field names for the same thing (`anime_title` vs `title`,
  `anime_season` vs `season_number`) and sometimes a list where another uses a string.
  `accuracy_test.go` handles both.

## Not included

A differential harness — parsing the whole corpus with two builds and diffing the tag streams
byte-for-byte — is the strongest check available and was used throughout the optimization work. It
needs two checkouts, so it does not fit a plain `go test`. Build the old and new revisions into
separate binaries that print `fmt.Sprintf("%e", release)` plus every field, run both over
`nyaa.txt` + `sports.txt` + the `tests.yaml` keys, and `cmp` the output. Any behaviour-preserving
change must produce identical output on all 16,154 names.
