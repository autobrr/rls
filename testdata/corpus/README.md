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
with us. The samples and the probe sets are ours. Their labels were written from what each name
says, by reading it, and not copied from what the parser printed.

## Contents

| file | rows | source | labelled |
|---|---|---|---|
| `labelled/anitomy.json` | 182 | [erengy/anitomy](https://github.com/erengy/anitomy) `test/data.json` | yes |
| `labelled/habari.json` | 218 | [5rahim/habari](https://github.com/5rahim/habari) `test/data.json` | yes |
| `labelled/anitogo.json` | 184 | [nssteinbrenner/anitogo](https://github.com/nssteinbrenner/anitogo) `test/data.json` | yes |
| `labelled/habari-known-failures.json` | 11 | habari `test/failing.json` | yes, with reasons |
| `labelled/nyaa-400.json` | 400 | a random sample of `nyaa.txt`, read by hand | yes, by us |
| `labelled/nyaa-blind-140.json` | 140 | names of `nyaa.txt` that two prototypes read differently, judged blind | yes, by us |
| `labelled/probes-title.json` | 103 | scene names built by hand, a vocabulary word inside the title | yes, by us |
| `labelled/probes-title-end.json` | 60 | scene names built by hand, a vocabulary word as the last word of the title | yes, by us |
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

`labelled/nyaa-400.json` is a random sample of 400 names from `nyaa.txt`. On 2026-11-12 an AI
model read each name beside the parser's output at commit `03ac59b` and marked the fields that were
wrong: title, season and episode, group, type. A field it accepted keeps the value the parser gave.
A field it marked wrong has a value written from the name. The accepted titles were read a second
time, and ten that still carried a marker word were rewritten, such as the `- OVA` of `Aldnoah Zero
- OVA`. The sample estimates how often the parser is wrong on what nyaa lists today.

`labelled/nyaa-blind-140.json` is 140 names from `nyaa.txt` that the two anime prototypes of
2026-11-12 read differently from master or from each other. A separate AI reader saw each name with
the two or three outputs, shuffled and unnamed, and judged every field of every output as right,
wrong or arguable. A label is the value of an output judged right. Where no output was right, the
label is written from the name, or left out when the name does not settle it. These are hard names
on purpose. Master had 25 of the 140 fully right when the file was made.

No person has checked every row of either file. A label that looks wrong is a bug in the fixture.
Fix it in a PR and say what the name means.

In both files:

| field | meaning |
|---|---|
| `type` | the release type as `Type.String()` gives it: `movie`, `series`, `episode` |
| `anime_title` | the title. A list holds every title that is right, for a name that gives two |
| `release_group` | the group |
| `anime_season` | the season, or a list of seasons |
| `episode_number` | one episode, or the two ends of a range. Half an episode keeps its fraction, `18.5` |
| `anime_type` | the kind of an extra: `OVA`, `Special`, `NCOP`, `NCED`, `ED`, `Movie` |
| `volume_number` | the volume or box, or the two ends of a range of them |

A field that is there and empty says the name has none: `"release_group": ""` is a name with no
group, and a parser that finds one is wrong. A field that is left out was not judged, because the
name does not settle it. The type of `[Commie] Chihayafuru 2 [BD 720p AAC]` is one: nothing in the
name says whether it is a film or a batch of a series.

`anime_type` and `volume_number` are labels that nothing scores yet, since `Release` has no field
for them. A numbered extra carries its number in `episode_number` with its kind beside it, so
`- OVA 02` is episode 2 of kind `OVA`.

`TestAccuracySample` scores both files and compares each field with `sampleFloors` in
`accuracy_test.go`. A floor is the number of names that are right, not a percentage. The test fails
below the floor, and above it too, so that the floor moves in the same diff as the change. Run it
with `-v` to list every name that has a field wrong.

### The probe sets

`labelled/probes-title.json` and `labelled/probes-title-end.json` are scene names built by hand
around one word of the vocabulary that is a word of the title in that name: `The.French.Dispatch.
2021`, `Going.Dutch.S01E01`, `Pardon.My.French.2021`. The corpora are anime and sports names, and a
rule that breaks scene names passes them unnoticed. In the first file the word stands inside the
title, in the second it is the last word in front of the year or the season.

| field | meaning |
|---|---|
| `title`, `type` | what the name must give, compared exactly |
| `no_tag` | the value the word would have as a tag, or a list of them. No field may hold it. It is left out when the name carries the same value as a real tag, as `French.Exit.2020.FRENCH` does |

`TestProbes` runs them. A probe the parser gets wrong today is listed in `probeFailures` in
`accuracy_test.go`. A probe that is not listed has to pass, and a listed one has to fail, so a
change that fixes a probe takes it off the list.

When you add a tag row or a rule that touches title words, add probes for it: movies, episodes,
packs, with the word first, inside and last.

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
