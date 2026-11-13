# Evaluation corpus

Independent data for measuring parser accuracy.

`labelled/anitomy.json` and `labelled/anitogo.json` are committed, each with the licence file of its
project. **The other data files here are gitignored** (`.gitignore` excludes `*.txt` and `*.json`),
so they live in your working tree and are not committed. `accuracy_test.go` skips the tests that
read them when they are absent, so the package still builds and tests without them.

## Why this exists

`tests.yaml` cannot answer "did this change make the parser better or worse". Its expectations are
written by us, often from this parser's own output when a case is added, so a wrong result can
become the expectation and the parser passes it. That is not hypothetical — it is how `Hi10` came to
be recorded as `HDR10`. (`TESTS=export go test` only sorts the stored expectations; it does not
recompute them.)

Everything here is labelled or collected by someone else, so it can disagree with us.

## Contents

| file | rows | source | labelled |
|---|---|---|---|
| `labelled/anitomy.json` | 182 | [erengy/anitomy](https://github.com/erengy/anitomy) `test/data.json` | yes |
| `labelled/habari.json` | 218 | [5rahim/habari](https://github.com/5rahim/habari) `test/data.json` | yes |
| `labelled/anitogo.json` | 184 | [nssteinbrenner/anitogo](https://github.com/nssteinbrenner/anitogo) `test/data.json` | yes |
| `labelled/habari-known-failures.json` | 11 | habari `test/failing.json` | yes, with reasons |
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

## Using it

```sh
go test -run TestAccuracyLabelled -v .   # per-field accuracy against the labelled sets
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
