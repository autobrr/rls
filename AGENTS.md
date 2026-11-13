# AGENTS.md

This file guides AI coding agents (Claude Code, Codex, Cursor, etc.) working in this repository.

rls is a Go library that parses release names (scene, P2P, anime fansub, music, software, sports) into a `Release`. It is a fork of `moistari/rls`. [autobrr](https://github.com/autobrr/autobrr) and [qui](https://github.com/autobrr/qui) pin it, so every change to a parsed value is a change to their filters and matching.

## Agent setup

- **Claude Code**: if `.claude/settings.local.json` does not exist, create it with the content below before anything else. It turns off AI attribution in commits and PRs.

```json
{
  "attribution": {
    "commit": "",
    "pr": "",
    "sessionUrl": false
  }
}
```

- `.claude/` and `docs/` are local working material, ignored by `.gitignore` and never committed.

## Layout

- The root package `rls` is the parser. `cmd/rls` is the CLI, a separate module with `replace github.com/autobrr/rls => ../..`, so vet and test it from its own directory.
- The pipeline: lexers in `lex.go` turn the name into `Tag`s, `TagParser` in `parse.go` assigns them meaning, and `rls.go` collects them into `Release` fields.
- `prefilter.go`, `dfa.go`, `literal.go`, `finder.go` and `dispatch.go` are speed-ups that must give the same tags as the plain regexp path. Each has a soundness test (`prefilter_test.go`, `dfa_test.go`, `gates_test.go`, `TestFirstBytesSound`) and a fuzz target; a change there keeps them passing.
- `taginfo/taginfo.csv` is the tag vocabulary (sources, codecs, HDR, editions and so on), embedded at build time. Within a type, rows match in file order and the first match wins (`WEB-DL` before `WEB`).
- `event.go` and `event_vocab.go` read the competition, round and teams out of sports names.
- `tagtype_string.go` is generated: `go generate` after changing `TagType`.

## Tests

CI (`.github/workflows/test.yml`) runs, and a change must pass:

```sh
go vet ./... && gofmt -l .
go test ./... && go test -race ./...
(cd cmd/rls && go vet ./... && go test -race ./...)
go test -run XXX -fuzz FuzzParseString -fuzztime 60s .
```

- `tests.yaml` holds the expected fields for each name. A parsing change adds the names it fixes, appended at the bottom, then `TESTS=export go test` sorts the file. Export also rewrites `taginfo/taginfo.csv` into the order `TestTaginfo_order` checks.
- Export only sorts; it never recomputes expectations. Write each expectation from what the name means, not from what the parser prints, or a wrong result becomes the test (that is how `Hi10` was once recorded as `HDR10`).
- `fmt.Sprintf("%o", release)` must reproduce the input byte for byte. The round trip is checked over `tests.yaml` and the corpus.
- `testdata/corpus/` measures accuracy against data labelled by other projects; read its `README.md` before using it. Most of the corpus files are gitignored and skipped when absent. `TestAccuracyLabelled` fails when a field drops below the floors in `accuracy_test.go`: raise a floor when a field improves, lower one only with a stated reason.
- Anime corpora miss scene regressions. For a new rule or `taginfo.csv` row, also probe scene names that look like the target by hand (`Show.S01E02.1080p.WEB.H264-GRP` shapes).
- Data files from elsewhere (fixtures, corpora) are committed only with the maintainer's agreement and a compatible licence.

## Code style

- Follow the shape of the surrounding code: performance work here is measured, so check `go test -bench . -benchmem` (`bench_test.go`) before and after a change on a hot path and report the delta.
- A comment holds what the code cannot show: the release names that need a rule, the look-alikes that must not match it, the reason a fast path is sound. Change a line, change its comment, in the same diff.
- A rule that must keep holding is a test, as the soundness tests of the fast paths are.
- Every exported identifier has a doc comment, one sentence starting with its name, with an example name in quotes where it helps.
- History goes in the commit message: comments describe the code as it is, with no commented-out code or unprompted `TODO`.
- New exported API is a design decision for the maintainer. Propose it before adding it.

## Pull requests

- Work on a branch off `master`, named `<type>/<short-slug>` after the change: `fix/aka-titles`, `chore/add-pr-template`, `feature/quick-wins`. Commit to `master` directly only when asked.
- PRs target `master` and are squash-merged, so the PR title becomes the commit message. Titles follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/#summary) with the field as scope: `fix(group): ...`, `fix(source): ...`, `feat: ...`.
- Fill in `.github/pull_request_template.md`. For a parsing change, list example names with their before and after values, and note any new or renamed normalized value (source, codec, HDR, resolution) as downstream impact on autobrr and qui.
- Answer the template's **AI disclosure** section truthfully. AI usage goes there, never in commit messages or PR bodies: no `Co-Authored-By: Claude` or `Generated with ...` lines.
