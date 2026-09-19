## 1. Baseline numbers

Audit date: 2026-09-19. Checkout: `13d03f4ef93d6a5e04bb8f8b6c3950cb25960512`, branch `feature/improvements`. The working tree was clean when measurements started. No parser files were changed. All experiments used scratch copies or Go overlays. There was no unfinished diff to assess.

Environment: Linux/amd64, AMD Ryzen 7 8745HS, 16 logical CPUs, Go 1.27.1. Module target Go 1.25. Shared build and module caches were used. Six benchmark samples were summarized with `golang.org/x/perf/cmd/benchstat` at `22c9c6c9d4da`. The `±` values below are benchstat's 95% confidence ranges, not standard deviations. Measurements include machine noise. The delimiter and scanner experiments have wide ranges.

Input provenance. The user supplied `/home/soup/Downloads/nyaa.txt` (13,215 lines, SHA-256 `6d80c9e940f2d414180b368602c19ecf74e6c28b6cefe2f0ff53272a29f38c78`) and `/home/soup/Downloads/sports.txt` (2,505 lines, `3600618314d1ea957de28dc35d4f84c55ce27b226b676a87a2fbb1475240d1c5`). Their collection dates and selection rules were not supplied. The sports file includes films, magazines and television about sports. It is not a labelled set of 2,505 sporting events.

The requested corpus README, local labels, `accuracy_test.go`, `fuzz_test.go`, and `anime_titles.txt` were absent from this checkout and the local history searched. I fetched replacement labels from these pinned primary sources:

- [anitomy, 182 records](https://github.com/erengy/anitomy/blob/a538eff670cb4666f8395cf66a9c29a535f2c383/test/data.json), SHA-256 `a47d5336e356b0ffd50ae36128fdb861985c848dc72bca33407a69cba9fbf679`.
- [habari, 218 records](https://github.com/5rahim/habari/blob/2626d57ad205a5ea41a5d060355e9314e0457c3e/test/data.json), SHA-256 `7211be0412e918d6256839bbca1c910e468b7522fe571029acd46cc27bd15f9f`.
- [anitogo, 184 records](https://github.com/nssteinbrenner/anitogo/blob/260d546976961245b80ae7bd02f0c0b52a3ecc44/test/data.json), SHA-256 `2bd2f6f1c09b1e66d5fe31049896c3dff628892cd7ce5b4aafca6c48e08a576c`.

These sets share many names and labels. They are not three independent samples. Habari also repeats records. Counts retain records as supplied.

Label agreement. Each cell is correct/nonempty-labelled fields (percent). Case and explicit source/audio aliases are normalized by the scoring code below. Episode and season comparisons use the first labelled number. They do not certify a whole range. List fields require the labelled items to be present, but do not penalize extra items. Resolution converts dimensions to height and preserves the current API's `1080i → 1080p` normalization. Thus these are field agreement scores, not precision, release accuracy, or the missing test's original floors.

| Field | anitomy | habari | anitogo |
| --- | ---: | ---: | ---: |
| Audio | 68/77 (88.3%) | 71/82 (86.6%) | 67/76 (88.2%) |
| BitDepth | 16/17 (94.1%) | 20/22 (90.9%) | 14/15 (93.3%) |
| Channels | 4/7 (57.1%) | 5/10 (50.0%) | 4/6 (66.7%) |
| Codec | 72/75 (96.0%) | 76/79 (96.2%) | 71/74 (95.9%) |
| Episode | 78/139 (56.1%) | 97/165 (58.8%) | 78/138 (56.5%) |
| Ext | 146/146 (100.0%) | 156/156 (100.0%) | 150/150 (100.0%) |
| Group | 132/155 (85.2%) | 154/189 (81.5%) | 135/157 (86.0%) |
| HDR | no label | 1/1 (100.0%) | no label |
| Language | 2/5 (40.0%) | 7/16 (43.8%) | 2/6 (33.3%) |
| Resolution | 88/98 (89.8%) | 107/115 (93.0%) | 89/99 (89.9%) |
| Series | 7/11 (63.6%) | 29/37 (78.4%) | 12/16 (75.0%) |
| Source | 67/70 (95.7%) | 75/80 (93.8%) | 67/70 (95.7%) |
| Subtitle | 11/26 (42.3%) | 15/36 (41.7%) | 13/29 (44.8%) |
| Sum | 82/84 (97.6%) | 81/83 (97.6%) | 82/84 (97.6%) |
| Title | 97/178 (54.5%) | 130/211 (61.6%) | 100/182 (54.9%) |
| Version | 23/26 (88.5%) | 21/32 (65.6%) | 23/26 (88.5%) |
| Year | 12/12 (100.0%) | 12/12 (100.0%) | 12/12 (100.0%) |

Title and episode have the most mismatches: anitomy 81/61, habari 81/68, anitogo 82/60. The most important causes are missing bare-number/range rules, incomplete versioned episode patterns, and title text taken as a suffix group. Group has 23/35/22 mismatches. Some group mismatches are label choices: Habari changes `BakaWolf-m.3.3.w` to `BakaWolf-m 3.3 w`. Anitogo labels `ru_jp` as the group in a name where anitomy selects the trailing group block. These disagreements are not all parser bugs.

Round trip. Equality means exact `%o == input`. Parsing completed without a panic. The probe also records `%e`, `%q`, `%s`, lexical tags, every exported `Release` field, `Unused()` and `SeriesEpisodes()`.

| Corpus | Names | Exact round trips | Panics |
| --- | ---: | ---: | ---: |
| `tests.yaml` keys | 429 | 429 (100%) | 0 |
| Nyaa | 13,215 | 13,215 (100%) | 0 |
| Sports | 2,505 | 2,505 (100%) | 0 |
| `anime_titles.txt` | unavailable | not measured | not measured |

YAML keys with embedded newlines must enter the probe as JSON strings. A plain newline-separated export will split two keys at five embedded newlines and give a false 434-name corpus.

Benchmarks. `Parse`, `ParseTags`, and `Build` operate on one regression name per iteration. `Build` includes copying a pre-lexed tag slice into reusable scratch space because Build mutates tags. These benchmarks use different setup and cannot be subtracted for an exact cost breakdown. `Moistari` and `Cytec` process all 429 names per iteration. Their outputs are not semantically equivalent.

| Benchmark | Time/op (95% range) | Bytes/op | Allocations/op |
| --- | ---: | ---: | ---: |
| Parse, regression | 79.62 µs ±2% | 8.483 KiB ±0% | 128 ±0% |
| ParseTags, regression | 65.67 µs ±3% | 6.488 KiB ±0% | 79 ±0% |
| Build, regression | 10.50 µs ±5% | 1.824 KiB ±0% | 48 ±0% |
| ParseLong | 637.2 µs ±1% | 74.81 KiB ±0% | 843 ±0% |
| Parse, Nyaa | 121.05 µs ±2% | 11.38 KiB ±0% | 166 ±0% |
| NewDefaultParser | 10.92 ms ±6% | 11.44 MiB ±0% | 218.1k ±0% |
| Moistari, whole regression set | 34.01 ms ±2% | 3.555 MiB ±1% | 55.10k ±0% |
| Cytec, whole regression set | 132.3 ms ±7% | 142.1 MiB ±0% | 955.7k ±0% |

`go test -short ./...` passed (root package 0.122 s); `go test -race -short ./...` passed (2.445 s). These are the audit's requested baselines, not new release acceptance tests. The absent `TestAccuracyLabelled` was replaced by the explicit scoring procedure above. Its name alone selects no tests.

Fuzz result. A scratch fuzz target seeded the regression keys, empty text, and 100 opening brackets. It checked parsing and exact round trip.

| Budget | Observed result | Minimal input | Actual | Expected |
| --- | --- | --- | --- | --- |
| 5 minutes requested | Failed after 7.73 s; last progress line: 184,265 executions at 6 s, 177 new interesting inputs | `WEB2000!` | `%o = WEB2000` | `%o = WEB2000!` |

The run stopped at its first failure. It did not pass five minutes. Fuzz artifact: `testdata/fuzz/FuzzParseString/d7559826992ceae8` in the scratch copy. C-01 explains the loss.

Reproduction. Save the named code blocks in the expandable section below to `/tmp/rls-audit-20260919`. Run from this repository at the audited commit. The scripts use the two user-supplied paths above. They make no requests during tests. The separate fixture download obtains only static GitHub JSON. Use the local Go version recorded above for exact regexp instrumentation. Do not set caches to the scratch directory.

<details>
<summary>Commands and complete scratch programs</summary>

The named blocks below are the audit programs. Extract them with Python's standard library:

```sh
python - <<'PYCODE'
import re
from pathlib import Path
p = Path('/tmp/rls-audit-20260919')
p.mkdir(exist_ok=True)
s = Path('docs/DEEP-ANALYSIS.md').read_text()
for name, code in re.findall(r'<!-- audit-file: ([^ ]+) -->\n```[^\n]*\n(.*?)\n```', s, re.S):
    (p / name).write_text(code + '\n')
PYCODE
```

Obtain the two supplied corpora at the recorded paths. Then download the pinned labels and build the baseline probe:

```sh
python /tmp/rls-audit-20260919/prepare.py
sha256sum /home/soup/Downloads/nyaa.txt /home/soup/Downloads/sports.txt /tmp/rls-audit-20260919/anitomy.json /tmp/rls-audit-20260919/habari.json /tmp/rls-audit-20260919/anitogo.json
python /tmp/rls-audit-20260919/accuracy.py
python /tmp/rls-audit-20260919/classify.py
```

The following are the baseline commands. Their logs provide the tables above:

```sh
env -u TESTS go test -short ./...
env -u TESTS go test -race -short ./...
env -u TESTS go test -overlay /tmp/rls-audit-20260919/overlay.json -run '^TestCorpusStructural$' -v .
env -u TESTS go test -run XXX -bench . -benchmem -count 6 . > /tmp/rls-audit-20260919/bench-base.log
env -u TESTS go test -overlay /tmp/rls-audit-20260919/overlay.json -run XXX -bench 'BenchmarkAudit(Nyaa|Startup)$' -benchmem -count 6 . > /tmp/rls-audit-20260919/bench-extra.log
env -u TESTS go test -run XXX -bench '^BenchmarkParse$' -benchtime 5s -cpuprofile /tmp/rls-audit-20260919/cpu.out -memprofile /tmp/rls-audit-20260919/mem.out -o /tmp/rls-audit-20260919/profile.test .
go tool pprof -top -nodecount 10 /tmp/rls-audit-20260919/profile.test /tmp/rls-audit-20260919/cpu.out
go tool pprof -top -alloc_objects -nodecount 10 /tmp/rls-audit-20260919/profile.test /tmp/rls-audit-20260919/mem.out
go tool pprof -top -alloc_space /tmp/rls-audit-20260919/profile.test /tmp/rls-audit-20260919/mem.out
go tool pprof -tree -nodecount 30 /tmp/rls-audit-20260919/profile.test /tmp/rls-audit-20260919/cpu.out
go tool pprof -tree -alloc_objects -nodecount 25 /tmp/rls-audit-20260919/profile.test /tmp/rls-audit-20260919/mem.out
```

Run the failing fuzz target from its scratch repository. It returns nonzero on the reported failure:

```sh
(cd /tmp/rls-audit-20260919/baseline && env -u TESTS go test -run XXX -fuzz FuzzParseString -fuzztime 5m .)
```

Run each performance experiment separately, with no other benchmark running:

```sh
env -u TESTS go test -overlay /tmp/rls-audit-20260919/no-prefilter-overlay.json -run XXX -bench '^(BenchmarkParse|BenchmarkAuditNyaa)$' -benchmem -count 6 . > /tmp/rls-audit-20260919/bench-no-prefilter.log
env -u TESTS go test -overlay /tmp/rls-audit-20260919/byte-delimiter-overlay.json -run XXX -bench '^(BenchmarkParse|BenchmarkAuditNyaa)$' -benchmem -count 6 . > /tmp/rls-audit-20260919/bench-byte-delimiter.log
env -u TESTS go test -overlay /tmp/rls-audit-20260919/meta-suffix-overlay.json -run XXX -bench '^(BenchmarkParse|BenchmarkAuditNyaa)$' -benchmem -count 6 . > /tmp/rls-audit-20260919/bench-meta-suffix.log
cat /tmp/rls-audit-20260919/bench-base.log /tmp/rls-audit-20260919/bench-extra.log > /tmp/rls-audit-20260919/bench-combined.log
go run golang.org/x/perf/cmd/benchstat@v0.0.0-20260908200009-22c9c6c9d4da /tmp/rls-audit-20260919/bench-combined.log /tmp/rls-audit-20260919/bench-no-prefilter.log /tmp/rls-audit-20260919/bench-byte-delimiter.log /tmp/rls-audit-20260919/bench-meta-suffix.log
python /tmp/rls-audit-20260919/differential.py
env -u TESTS go test -overlay /tmp/rls-audit-20260919/sound-overlay.json -run '^TestAuditSound$' -v .
env -u TESTS go test -overlay /tmp/rls-audit-20260919/sound-overlay.json -run XXX -bench '^BenchmarkAuditShape$' -benchmem -benchtime 100ms .
env -u TESTS go test -overlay /tmp/rls-audit-20260919/scanner-overlay.json -run XXX -bench '^BenchmarkAuditScanner$' -benchmem -count 6 .
go run /tmp/rls-audit-20260919/probes.go
go run /tmp/rls-audit-20260919/contracts.go
```

RegExp instrumentation is serial and separate from timing. It modifies the standard library only through an overlay. A different Go version needs its engine entry point identified again:

```sh
python /tmp/rls-audit-20260919/instrument.py
env -u TESTS go test -overlay /tmp/rls-audit-20260919/count-overlay.json -run '^TestAuditCounts$' -v .
python - <<'PYCODE'
import json
from pathlib import Path
p = Path('/tmp/rls-audit-20260919')
v = json.loads((p / 'count-overlay.json').read_text())
v['Replace'][str(Path.cwd() / 'prefilter.go')] = str(p / 'no-prefilter.go')
(p / 'count-no-prefilter-overlay.json').write_text(json.dumps(v))
PYCODE
env -u TESTS AUDIT_COUNT_PREFIX=off- go test -overlay /tmp/rls-audit-20260919/count-no-prefilter-overlay.json -run '^TestAuditCounts$' -v .
```

`counts-*.json` contains totals. Divide each regexp count by `names`. The prefilter arrays are `[attempts, admits]`. Rejects are their difference. To reproduce any listed failure, send its exact string as a JSON line to `trace --json-input`. Check `panic`, `o`, `e`, `release`, `episodes` and `unused`. Timing need not reproduce to the last digit on another run or CPU.


`prepare.py`

<!-- audit-file: prepare.py -->
```python
import ast,json,shutil,subprocess,urllib.request
from pathlib import Path
p=Path(__file__).parent
repo=Path.cwd()
fixtures={
 'anitomy':('erengy/anitomy','a538eff670cb4666f8395cf66a9c29a535f2c383'),
 'habari':('5rahim/habari','2626d57ad205a5ea41a5d060355e9314e0457c3e'),
 'anitogo':('nssteinbrenner/anitogo','260d546976961245b80ae7bd02f0c0b52a3ecc44'),
}
for name,(remote,sha) in fixtures.items():
 url=f'https://raw.githubusercontent.com/{remote}/{sha}/test/data.json'
 (p/(name+'.json')).write_bytes(urllib.request.urlopen(url).read())
 labels=json.loads((p/(name+'.json')).read_text())
 (p/(name+'-input.jsonl')).write_text(''.join(json.dumps(r['file_name'])+'\n' for r in labels))
keys=[ast.literal_eval(s[:-1]) for s in (repo/'tests.yaml').read_text().splitlines() if s.startswith('"') and s.endswith(':')]
assert len(keys)==429
(p/'tests-input.jsonl').write_text(''.join(json.dumps(s)+'\n' for s in keys))
for name in ['nyaa','sports']:
 names=Path('/home/soup/Downloads',name+'.txt').read_text().splitlines()
 (p/(name+'-input.jsonl')).write_text(''.join(json.dumps(s)+'\n' for s in names))
base={str(repo/'z_audit_test.go'):str(p/'audit_test.go')}
(p/'overlay.json').write_text(json.dumps({'Replace':base}))
for kind,file in [('sound','sound_test.go'),('scanner','scanner_only_test.go')]:
 v=base|{str(repo/('z_'+file)):str(p/file)}
 (p/(kind+'-overlay.json')).write_text(json.dumps({'Replace':v}))
variants={
 'byte-delimiter':('parse.go',(repo/'parse.go').read_text().replace('!p.delim.Match(src[j:])','!isAnyDelim(rune(src[j]))')),
 'no-prefilter':('prefilter.go',(repo/'prefilter.go').read_text().replace('func (pf *prefilter) maybe(buf []byte, i, n int) bool {','func (pf *prefilter) maybe(buf []byte, i, n int) bool {\n return true')),
 'meta-suffix':('lex.go',(repo/'lex.go').read_text().replace('if m = suffix[l].FindSubmatch(src[i:n]); m != nil {','if close := strs[l*4+2]; strings.TrimSpace(close) == close && !bytes.HasSuffix(bytes.TrimSpace(src[i:n]), []byte(close)) { continue }\n if m = suffix[l].FindSubmatch(src[i:n]); m != nil {')),
}
for name,(file,source) in variants.items():
 (p/(name+'.go')).write_text(source)
 (p/(name+'-overlay.json')).write_text(json.dumps({'Replace':base|{str(repo/file):str(p/(name+'.go'))}}))
baseline=p/'baseline';baseline.mkdir(exist_ok=True)
for file in list(repo.glob('*.go'))+[repo/'go.mod',repo/'go.sum',repo/'tests.yaml']:
 shutil.copyfile(file,baseline/file.name)
for d in ['taginfo','reutil']:
 shutil.copytree(repo/d,baseline/d,dirs_exist_ok=True)
shutil.copyfile(p/'audit_test.go',baseline/'audit_test.go')
subprocess.run(['go','build','-o',str(p/'trace'),str(p/'trace.go')],check=True)
for name in ['tests','nyaa','sports','anitomy','habari','anitogo','probes','traces']:
 data=(p/(name+'-input.jsonl')).read_bytes()
 result=subprocess.run([str(p/'trace'),'--json-input'],input=data,stdout=subprocess.PIPE,check=True)
 (p/(name+'-parsed.jsonl')).write_bytes(result.stdout)
```

`trace.go`

<!-- audit-file: trace.go -->
```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/autobrr/rls"
)

func parse(s string) (out map[string]any) {
	out = map[string]any{"input": s}
	defer func() {
		if err := recover(); err != nil {
			out["panic"] = fmt.Sprint(err)
		}
	}()
	tags, end := rls.ParseTagsString(s)
	out["lex"], out["end"] = fmt.Sprintf("%v", tags), end
	r := rls.ParseString(s)
	out["release"], out["type"] = r, r.Type.String()
	out["e"], out["o"], out["q"], out["s"] = fmt.Sprintf("%e", r), fmt.Sprintf("%o", r), fmt.Sprintf("%q", r), fmt.Sprintf("%s", r)
	out["unused"], out["episodes"] = fmt.Sprintf("%v", r.Unused()), r.SeriesEpisodes()
	return out
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if len(os.Args) > 1 && os.Args[1] == "--json-input" {
			if err := json.Unmarshal([]byte(line), &line); err != nil { panic(err) }
		}
		if err := enc.Encode(parse(line)); err != nil {
			panic(err)
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}
```

`audit_test.go`

<!-- audit-file: audit_test.go -->
```go
package rls

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"
)

func auditLines(tb testing.TB, file string) []string {
	tb.Helper()
	f, err := os.Open(file)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	var lines []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if err := s.Err(); err != nil {
		tb.Fatal(err)
	}
	return lines
}

func TestCorpusStructural(t *testing.T) {
	corpora := map[string][]string{"tests.yaml": benchCorpus(t)}
	for _, name := range []string{"nyaa", "sports"} {
		corpora[name] = auditLines(t, "/home/soup/Downloads/"+name+".txt")
	}
	for name, lines := range corpora {
		roundtrips, panics := 0, 0
		for _, s := range lines {
			func() {
				defer func() {
					if err := recover(); err != nil {
						panics++
						t.Logf("PANIC %s %q %v", name, s, err)
					}
				}()
				r := ParseString(s)
				if fmt.Sprintf("%o", r) == s {
					roundtrips++
				} else {
					t.Logf("ROUNDTRIP %s %q %q", name, s, fmt.Sprintf("%o", r))
				}
			}()
		}
		t.Logf("TOTAL %s names=%d roundtrips=%d panics=%d", name, len(lines), roundtrips, panics)
	}
	t.Logf("Tag bytes=%d Release bytes=%d", unsafe.Sizeof(Tag{}), unsafe.Sizeof(Release{}))
}

func BenchmarkAuditNyaa(b *testing.B) {
	corpus := auditLines(b, "/home/soup/Downloads/nyaa.txt")
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		ParseString(corpus[i%len(corpus)])
		i++
	}
}

func BenchmarkAuditStartup(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		NewDefaultParser()
	}
}

func FuzzParseString(f *testing.F) {
	for _, s := range benchCorpus(f) {
		f.Add(s)
	}
	f.Add("")
	f.Add(strings.Repeat("[", 100))
	f.Fuzz(func(t *testing.T, s string) {
		r := ParseString(s)
		if actual := fmt.Sprintf("%o", r); actual != s {
			t.Fatalf("round trip: %q != %q", actual, s)
		}
	})
}
```

`accuracy.py`

<!-- audit-file: accuracy.py -->
```python
import collections
import json
import re
from pathlib import Path

root = Path(__file__).parent

def values(row, key):
    value = row.get(key, [])
    return value if isinstance(value, list) else [value]

def expected(row):
    out = {}
    for field, keys in {
        'Title': ['anime_title', 'title'], 'Subtitle': ['episode_title'],
        'Group': ['release_group'], 'Sum': ['file_checksum'], 'Ext': ['file_extension'],
        'Year': ['anime_year', 'year'], 'Series': ['anime_season', 'season_number'],
        'Episode': ['episode_number'], 'Version': ['release_version'],
        'Source': ['source'], 'Resolution': ['video_resolution'],
    }.items():
        vs = next((values(row, k) for k in keys if row.get(k)), [])
        if not vs:
            continue
        v = vs[0]
        if field in ['Year', 'Series', 'Episode']:
            v = str(int(v)) if re.fullmatch(r'\d+', v) else v
        if field == 'Version':
            v = 'v' + v
        if field == 'Source':
            v = {'bd': 'BluRay', 'bluray': 'BluRay', 'blu-ray': 'BluRay', 'bdrip': 'BDRiP', 'dvdrip': 'DVDRiP'}.get(v.lower(), v)
        if field == 'Resolution':
            v = re.sub(r'^\d+[xX×]', '', v).lower()
            if v.isdigit():
                v += 'p'
            # The existing public field combines interlaced and progressive values.
            v = re.sub(r'i$', 'p', v)
        out[field] = v
    codecs = []
    audio = []
    for v in values(row, 'video_term'):
        k = re.sub(r'[ .-]', '', v).lower()
        if k in {'h264', 'x264', 'x265', 'xvid', 'divx5', 'avc', 'av1', 'hevc'}:
            codecs.append({'h264': 'H.264', 'xvid': 'XViD', 'divx5': 'DIVX5'}.get(k, k))
        elif k in {'8bit', '10bit', '10bits', 'hi10', 'hi10p'}:
            out['BitDepth'] = '8BIT' if k == '8bit' else '10BIT'
        elif k == 'hdr':
            out['HDR'] = ['HDR']
    for v in values(row, 'audio_term'):
        k = v.lower().replace('-', '').replace(' ', '')
        channels = re.search(r'[1-7]\.\d', k)
        if channels:
            out['Channels'] = channels[0]
            k = k[:channels.start()]
        k = re.sub(r'x\d+$', '', k)
        if k and k != 'ch':
            audio.append({'ac3': 'DD', 'eac3': 'DDP', 'vorbis': 'OGG', 'dualaudio': 'DUAL.AUDIO', 'dtses': 'DTS-ES'}.get(k, k))
    if codecs:
        out['Codec'] = codecs
    if audio:
        out['Audio'] = audio
    languages = values(row, 'language')
    if languages:
        synonyms = {'jap': 'JAPANESE', 'jpn': 'JAPANESE', 'jp': 'JAPANESE', 'eng': 'ENGLiSH', 'en': 'ENGLiSH', 'english': 'ENGLiSH', 'fr': 'FRENCH', 'ita': 'iTALiAN', 'ru': 'RUSSiAN', 'pt-br': 'BRAZiLiAN', 'por-br': 'BRAZiLiAN', 'ch': 'CHiNESE'}
        out['Language'] = [synonyms.get(v.lower(), v) for v in languages]
    return out

def canon(v):
    if isinstance(v, list):
        return sorted(set(str(x).casefold() for x in v))
    return str(v).casefold()

summary = {}
failures = []
for name in ['anitomy', 'habari', 'anitogo']:
    labels = json.loads((root / (name + '.json')).read_text())
    parsed = [json.loads(s) for s in (root / (name + '-parsed.jsonl')).read_text().splitlines()]
    counts = collections.defaultdict(lambda: [0, 0])
    for label, result in zip(labels, parsed, strict=True):
        assert label['file_name'] == result['input']
        for field, exp in expected(label).items():
            actual = result['release'][field]
            # Check labelled values only. Extra extracted values are a separate precision audit.
            ok = set(canon(exp)) <= set(canon(actual or [])) if isinstance(exp, list) else canon(exp) == canon(actual)
            counts[field][0] += ok
            counts[field][1] += 1
            if not ok:
                failures.append({'set': name, 'input': result['input'], 'field': field, 'expected': exp, 'actual': actual})
    summary[name] = dict(counts)
(root / 'accuracy-summary.json').write_text(json.dumps(summary, indent=2))
(root / 'accuracy-failures.json').write_text(json.dumps(failures, indent=2, ensure_ascii=False))
for field in sorted(set().union(*(v for v in summary.values()))):
    print(field, ' | '.join(f'{name}: {summary[name].get(field)}' for name in summary))
```

`differential.py`

<!-- audit-file: differential.py -->
```python
import json,subprocess
from pathlib import Path
p=Path(__file__).parent
for variant in ['meta-suffix','byte-delimiter','no-prefilter']:
 binary=str(p/('trace-'+variant))
 subprocess.run(['go','build','-overlay',str(p/(variant+'-overlay.json')),'-o',binary,str(p/'trace.go')],check=True)
 for name in ['tests','nyaa','sports','anitomy','habari','anitogo']:
  expected=list(map(json.loads,(p/(name+'-parsed.jsonl')).read_text().splitlines()))
  result=subprocess.run([binary,'--json-input'],input=(p/(name+'-input.jsonl')).read_bytes(),stdout=subprocess.PIPE,check=True)
  actual=list(map(json.loads,result.stdout.splitlines()))
  diffs=[r['input'] for r,a in zip(expected,actual,strict=True) if r!=a]
  print(variant,name,len(expected),'names;',len(diffs),'differences')
  assert not diffs,diffs[:3]
```

`instrument.py`

<!-- audit-file: instrument.py -->
```python
import json
import re
import subprocess
from pathlib import Path

root = Path(__file__).parent
repo = Path.cwd()
goroot = Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip()) / 'src/regexp'
replace = {}

def overlay(src, dst, text):
    dst = root / dst
    dst.write_text(text)
    replace[str(src)] = str(dst)

src = goroot / 'exec.go'
text = src.read_text().replace('func (re *Regexp) find(r io.RuneReader, b []byte, s string, pos int, ncap int, dstCap []int) []int {', 'func (re *Regexp) find(r io.RuneReader, b []byte, s string, pos int, ncap int, dstCap []int) []int {\n if AuditCalls != nil { AuditCalls[AuditStage]++ }')
text += '\nvar AuditStage string\nvar AuditCalls map[string]int\nvar AuditCompiled int\n'
overlay(src, 'regexp-exec.go', text)
src = goroot / 'regexp.go'
overlay(src, 'regexp-regexp.go', src.read_text().replace('func compile(expr string, mode syntax.Flags, longest bool) (*Regexp, error) {', 'func compile(expr string, mode syntax.Flags, longest bool) (*Regexp, error) {\n AuditCompiled++'))
src = repo / 'lex.go'
lines = src.read_text().splitlines(keepends=True)
out = []
name = ''
for line in lines:
    m = re.match(r'func (New\w+)\(', line)
    if m:
        name = m[1]
    out.append(line)
    if 'Lex: func(src, buf []byte' in line:
        stage = '"' + name + '"'
        if name in ['NewRegexpLexer', 'NewRegexpSourceLexer']:
            stage += ' + "/" + typ.String()'
        out.append('\t\t\tregexp.AuditStage = ' + stage + '\n')
text = ''.join(out)
text = text.replace('if !pf.maybe(buf, i, n) {', 'admit := pf.maybe(buf, i, n)\n auditPrefilter(typ, admit)\n if !admit {')
text = text.replace('if !pf.maybe(src, i, n) {', 'admit := pf.maybe(src, i, n)\n auditPrefilter(typ, admit)\n if !admit {')
overlay(src, 'count-lex.go', text)
src = repo / 'parse.go'
text = src.read_text().replace('func (p *TagParser) Parse(src []byte) ([]Tag, int) {', 'func (p *TagParser) Parse(src []byte) ([]Tag, int) {\n regexp.AuditStage = "Parse"')
text = text.replace('// delimiter\n', '// delimiter\n regexp.AuditStage = "Parse"\n', 1)
text = text.replace('// text\n\tj := i', '// text\n regexp.AuditStage = "Parse"\n\tj := i')
text = text.replace('func (b *TagBuilder) Build(tags []Tag, end int) Release {', 'func (b *TagBuilder) Build(tags []Tag, end int) Release {\n regexp.AuditStage = "Build"')
overlay(src, 'count-parse.go', text)
replace[str(repo / 'z_audit_test.go')] = str(root / 'audit_test.go')
replace[str(repo / 'z_count_test.go')] = str(root / 'count_test.go')
(root / 'count-overlay.json').write_text(json.dumps({'Replace': replace}))
```

`count_test.go`

<!-- audit-file: count_test.go -->
```go
package rls

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

var auditPF map[string][2]int

func auditPrefilter(typ TagType, admit bool) {
	if auditPF == nil {
		return
	}
	v := auditPF[typ.String()]
	v[0]++
	if admit {
		v[1]++
	}
	auditPF[typ.String()] = v
}

func TestAuditCounts(t *testing.T) {
	regexp.AuditCompiled = 0
	NewDefaultParser()
	t.Logf("NewDefaultParser compiled=%d", regexp.AuditCompiled)
	corpora := map[string][]string{"tests": benchCorpus(t), "nyaa": auditLines(t, "/home/soup/Downloads/nyaa.txt"), "sports": auditLines(t, "/home/soup/Downloads/sports.txt")}
	for name, lines := range corpora {
		regexp.AuditCalls = map[string]int{}
		auditPF = map[string][2]int{}
		for _, line := range lines {
			ParseString(line)
		}
		v := map[string]any{"names": len(lines), "regexp": regexp.AuditCalls, "prefilter": auditPF}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("/tmp/rls-audit-20260919/counts-"+os.Getenv("AUDIT_COUNT_PREFIX")+name+".json", b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	regexp.AuditCalls, auditPF = nil, nil
}
```

`sound_test.go`

<!-- audit-file: sound_test.go -->
```go
package rls

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/autobrr/rls/reutil"
	"github.com/autobrr/rls/taginfo"
)

func TestAuditSound(t *testing.T) {
	infos := taginfo.All()
	for typ, rows := range infos {
		pf, _ := newPrefilter(rows, true)
		conf := "^ib"
		if typ == "language" {
			conf = "^b"
		}
		re := regexp.MustCompile(reutil.Taginfo(conf, rows...))
		if typ == "codec" || typ == "collection" || typ == "hdr" {
			re = regexp.MustCompile(reutil.Taginfo("^i", rows...)+`(?:\b|[\-\._ ])`)
		}
		indexed, linear := findFunc(rows...), taginfo.Find(rows...)
		for _, row := range rows {
			spellings, _ := expandRE(row.RE(), true)
			spellings = append(spellings, row.Tag())
			if row.Tag() == "Uncut" {
				spellings = append(spellings, "ungekürzt")
			}
			seen := map[string]bool{}
			for _, s := range spellings {
				for _, v := range []string{s, strings.ReplaceAll(strings.ToLower(s), "s", "ſ"), strings.ReplaceAll(strings.ToLower(s), "k", "K")} {
					if seen[v] {
						continue
					}
					seen[v] = true
					if re.MatchString(v) && !pf.maybe([]byte(v), 0, len(v)) {
						t.Logf("PREFILTER %s %q", typ, v)
					}
					if indexed(v) != linear(v) {
						t.Logf("LOOKUP %s %q indexed=%v linear=%v", typ, v, indexed(v), linear(v))
					}
				}
			}
		}
	}
	for _, s := range []string{"Movie.2024.ungekürzt.1080p.BluRay.x264-GRP", "Tool.Pſ5", "Title.S01E01E02.1080p", "Title.S01-04v2.1080p", "Title.S01E01v2.1080p", "Title.Ep04.1080p", "Title.S01E00.1080p", "Event.2023.02.31.1080p", "WEB2000!", "Film.2DVD.1080p", "Film.12Discs.1080p"} {
		t.Logf("CASE %q tags=%e original=%o", s, ParseString(s), ParseString(s))
	}
	t.Logf("zero release %q", fmt.Sprintf("%o|%e|%q|%s|%v", Release{}, Release{}, Release{}, Release{}, Release{}))
}

func BenchmarkAuditShape(b *testing.B) {
	for _, n := range []int{256, 1024, 4096} {
		for _, char := range []string{"(", "a", "."} {
			b.Run(fmt.Sprintf("%x/%d", char, n), func(b *testing.B) {
				s := strings.Repeat(char, n)
				for b.Loop() {
					ParseString(s)
				}
			})
		}
	}
}
```

`scanner_only_test.go`

<!-- audit-file: scanner_only_test.go -->
```go
package rls
import("context";"fmt";"strings";"testing")
func BenchmarkAuditScanner(b *testing.B) {
 text := strings.Join(benchCorpus(b), "\n")
 for _, workers := range []int{1, 16} {
  b.Run(fmt.Sprint(workers), func(b *testing.B) {
   for b.Loop() {
    s := NewScanner(WithWorkers(workers))
    for range s.ScanReader(context.Background(), strings.NewReader(text)) {}
    if err := s.Err(); err != nil { b.Fatal(err) }
   }
  })
 }
}
```

`probes.go`

<!-- audit-file: probes.go -->
```go
package main
import("fmt"; "os"; "runtime/debug"; "context"; "strings"; "time"; "github.com/autobrr/rls"; "github.com/autobrr/rls/taginfo"; "golang.org/x/text/transform")
func safe(s string){defer func(){if e:=recover();e!=nil{fmt.Printf("NORMALIZE %q panic %v\n",s,e)}}();fmt.Printf("NORMALIZE %q -> %q\n",s,rls.MustNormalize(s))}
func main(){
 for _,s:=range []string{"a�b",string([]byte{'a',255,'b'})}{safe(s)}
 c:=rls.NewCollapser(false,true,""," ",nil);dst:=make([]byte,16);n1,_,e1:=c.Transform(dst,[]byte("a "),false);n2,_,e2:=c.Transform(dst[n1:],[]byte(" b"),true);whole,_,e3:=transform.String(c,"a  b");fmt.Printf("COLLAPSE stream=%q whole=%q errors=%v,%v,%v\n",dst[:n1+n2],whole,e1,e2,e3)
 info,err:=taginfo.New("X","","","","INVALID","");fmt.Printf("BADTYPE info=%v err=%v\n",info,err)
 debug.SetGCPercent(-1); taginfo.LoadFile("taginfo/taginfo.csv");before,_:=os.ReadDir("/proc/self/fd");for range 20{_,err=taginfo.LoadFile("taginfo/taginfo.csv");if err!=nil{panic(err)}};after,_:=os.ReadDir("/proc/self/fd");fmt.Printf("LOADFILE descriptors before=%d after=%d\n",len(before),len(after))
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Millisecond);defer cancel();s:=rls.NewScanner(rls.WithWorkers(0));n:=0;for range s.ScanReader(ctx,strings.NewReader("hello\n")){n++};fmt.Printf("ZERO WORKERS n=%d err=%v\n",n,s.Err())
}
```

`contracts.go`

<!-- audit-file: contracts.go -->
```go
package main
import("fmt";"github.com/autobrr/rls";"github.com/autobrr/rls/taginfo")
func main(){
 calls:=0
 p:=rls.NewTagParser(nil,rls.TagLexer{Lex:func(src,buf []byte,start,end []rls.Tag,i,n int)([]rls.Tag,[]rls.Tag,int,int,bool){calls++;return start,end,i+1,n-1,true}})
 p.Parse([]byte("abcd"));fmt.Println("multi lexer calls",calls)
 infos:=taginfo.All()
 for _,s:=range []string{"AAC","HEVC","WEB","BD","Complete","Special","Mixed","PSV","US","10bit","Hi10","HDR10","1080i","Uncut"}{
  fmt.Print(s,":");for typ,rows:=range infos{for _,row:=range rows{if row.Match(s){fmt.Printf(" %s/%s",typ,row.Tag())}}};fmt.Println()
 }
}
```

`classify.py`

<!-- audit-file: classify.py -->
```python
import json,re
from pathlib import Path
p=Path(__file__).parent
out=Path.cwd()/'docs'/'failures';out.mkdir(parents=True,exist_ok=True)
n=list(map(json.loads,(p/'nyaa-parsed.jsonl').read_text().splitlines()))
s=list(map(json.loads,(p/'sports-parsed.jsonl').read_text().splitlines()))
b={}
b['anime-ep-prefix']=[r for r in n if re.search(r'\bEP\d+\s+(?:1080p|2160p|720p|480p)\b',r['input'],re.I) and not r['release']['Episode']]
b['anime-versioned-season']=[r for r in n if re.search(r'S\d{1,2}E\d+v\d+',r['input'],re.I) and not r['release']['Episode']]
b['anime-ordinal-season']=[r for r in n if re.search(r'\b([2-9])(?:nd|rd|th) Season - \d',r['input']) and not r['release']['Series']]
b['anime-bracketed-range']=[r for r in n if (m:=re.search(r'[\[(](\d{1,3})[-~](\d{1,3})[\])]',r['input'])) and int(m[1])<int(m[2]) and not r['episodes']]
b['anime-decimal-episode']=[r for r in n if re.search(r' - \d+\.\d(?:\D|$)',r['input'])]
b['anime-leading-group']=[r for r in n if (m:=re.match(r'^\[([^\]]+)\]',r['input'])) and m[1] not in ('Hakata Ramen','DVDISO') and r['release']['Site']==m[1] and r['release']['Group']!=m[1]]
b['anime-zero-episode']=[r for r in n if re.search(r'(?: - |S\d+E)0{1,3}(?:v\d+)?(?:[ .][(\[]| \(Special| - )',r['input']) and r['type']=='series']
b['sports-region-collision']=[r for r in s if r['release']['Region'] and re.search(r'\.US\.Open\.|\.UK\.Championship\.|(?:\.USA\.(?:vs|gegen)\.|\.(?:vs|gegen)\.USA\.)',r['input'],re.I)]
b['sports-language-collision']=[r for r in s if '.Hungarian.Grand.Prix.' in r['input'] and 'HUNGARiAN' in (r['release']['Language'] or [])]
b['sports-music-collision']=[r for r in s if r['type']=='music']
b['sports-platform-collision']=[r for r in s if r['type']=='game']
b['sports-event-fields']=[r for r in s if re.search(r'\.(?:Round|Week|Stage|Matchday|Day|Session)\.(?:\d+|One|Two|Three)\b',r['input'],re.I)]
for key,rows in b.items():
 names=list(dict.fromkeys(r['input'] for r in rows))
 (out/(key+'.txt')).write_text(''.join(s+'\n' for s in names))
 print(key,'records',len(rows),'unique',len(names),'example',names[0] if names else '')
(p/'taxonomy.json').write_text(json.dumps({k:{'records':len(v),'unique':len(set(r['input'] for r in v))} for k,v in b.items()},indent=2))
```

`probes-input.jsonl`

<!-- audit-file: probes-input.jsonl -->
```jsonl
"Event.2024.10.13.Final.1080p.WEB.x264-GRP"
"Event.2024-10-13.Final.1080p.WEB.x264-GRP"
"Event.13.10.2024.Final.1080p.WEB.x264-GRP"
"Event.10-13-2024.Final.1080p.WEB.x264-GRP"
"Event.24.10.13.Final.1080p.WEB.x264-GRP"
"Event.13.10.24.Final.1080p.WEB.x264-GRP"
"Event.2023.02.31.Final.1080p.WEB.x264-GRP"
"Formula1.2024x05.Miami.Grand.Prix.Race.1080p.WEB.x264-GRP"
"F1.2024.R05.Miami.Qualifying.1080p.WEB.x264-GRP"
"MotoGP.2024.Round.05.1080p.WEB.x264-GRP"
"UFC.300.1080p.WEB.x264-GRP"
"UFC.Fight.Night.240.1080p.WEB.x264-GRP"
"WWE.Raw.2024.10.14.1080p.WEB.x264-GRP"
"NFL.2024.10.13.Team.vs.Team.1080p.WEB.x264-GRP"
"EPL.2024.Matchday.08.1080p.WEB.x264-GRP"
"NBA.2024.10.22.Lakers.vs.Timberwolves.1080p.WEB.x264-GRP"
"Tour.de.France.2024.Stage.05.1080p.WEB.x264-GRP"
"Wimbledon.2024.Mens.Final.1080p.WEB.x264-GRP"
"Boxing.2024-10-12.Fighter.vs.Fighter.1080p.WEB.x264-GRP"
"NHL.2024-25.Team.vs.Team.1080p.WEB.x264-GRP"
"NFL.2024.Week.06.1080p.WEB.x264-GRP"
"Event.2024.Day.1.1080p.WEB.x264-GRP"
"Event.2024.Session.1.1080p.WEB.x264-GRP"
"F1.2024.Practice.1.1080p.WEB.x264-GRP"
"EPL.2024.Extended.Highlights.1080p.WEB.x264-GRP"
"WWE.2024.Pre-Show.PPV.1080p.WEB.x264-GRP"
"Real.Madrid.vs.Sporting.1080p.WEB.x264-GRP"
"Union.vs.Racing.1080p.WEB.x264-GRP"
"Red.Bull.vs.Ajax.1080p.WEB.x264-GRP"
"[SubsPlease] Title - 04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title - 04v2 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title E04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title Ep04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title 04 END (1080p) [ABCD1234].mkv"
"[SubsPlease] Title (01-12) (1080p) [ABCD1234].mkv"
"[SubsPlease] Title 01~12 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title S2 - 04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title 2nd Season - 04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title Season 2 - 04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title Part 2 - 04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title Cour 2 - 04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title - 07.5 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title - 00 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title S01E00 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title - 1234 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title S01E04v2 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title OVA - 01 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title ONA - 01 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title OAD - 01 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title Special - 01 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title Movie 01 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title Recap - 01 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title NCOP 01 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title NCED 01 (1080p) [ABCD1234].mkv"
"[G123-x] Mob Psycho 100 - 04 (1080p).mkv"
"[WEB] Title - 04 (1080p).mkv"
"[deadbeef] Title - 04 (1080p).mkv"
"[G1][G2] Title - 04 (1080p).mkv"
"[\u0413\u0440\u0443\u043f\u043f\u0430] 86 - 04 (1080p).mkv"
"\u3010\u5b57\u5e55\u7ec4\u3011\u300c\u65e5\u672c\u8a9e\u300d - 04 (1080p).mkv"
"[G] Steins;Gate 0 - 04 (1080p).mkv"
"[G] Blue Complete Movie Special Extended - 04 (1080p).mkv"
"[G] WEB of Lies - 04 (1080p).mkv"
"[G] \u65e5\u672c\u8a9e / English Title (2024) - 04 (1080p).mkv"
"[G] Title - 04 (BDRip Hi10 Dual Audio Multi-Subs Uncensored HDR10 1080p).mkv"
""
"[]"
"[Group]"
"2024.10.13"
"[](.foo_-+bar\t\n"
"[[[[[[[[[[[[[[[[[[[[[[[[[[[[[["
"a\ufffdb"
```

`traces-input.jsonl`

<!-- audit-file: traces-input.jsonl -->
```jsonl
"Show.S01E02.1080p.WEB-DL.x264-GROUP"
"The.Matrix.1999.1080p.BluRay.x264-GRP"
"Artist-Album-(ABC123)-2CD-2024-FLAC-GRP"
"Tool.v1.2.3.Win64-GRP"
"[SubsPlease] Title - 04 (1080p) [ABCD1234].mkv"
"[SubsPlease] Title - 04v2 (1080p).mkv"
"[SubsPlease] Title (01-12) (1080p).mkv"
"[SubsPlease] Title 01~12 (1080p).mkv"
"[SubsPlease] Title Season 2 - 04 (1080p).mkv"
"WWE.Raw.2024.10.14.1080p.WEB-DL.x264-GRP"
"NFL.2024.10.13.Team.vs.Team.720p.HDTV.x264-GRP"
"MotoGP.2024.Round.05.1080p.WEB-DL.x264-GRP"
"Show.2024.1080p.x264.extra.words-GRP"
"Film.AKA.Alternate.2024.1080p.BluRay.x264-GRP"
"Show.S01E02.The.Return.1080p.WEB-DL.x264-GRP"
"[\u0413\u0440\u0443\u043f\u043f\u0430] \u846c\u9001\u306e\u30d5\u30ea\u30fc\u30ec\u30f3 - 04 (1080p) [abcd1234].mkv"
"[Anime Time] Title - 04 (1080p).mkv"
"[WEB] Title - 04 (1080p).mkv"
"[deadbeef] Title - 04 (1080p).mkv"
"[SubsPlease][Extra] Title - 04 (1080p).mkv"
"[SubsPlease] Mob Psycho 100 - 04 (1080p).mkv"
"[SubsPlease] 86 - 04 (1080p).mkv"
"[SubsPlease] Steins;Gate 0 - 04 (1080p).mkv"
"[SubsPlease] Title - 00 (1080p).mkv"
"[SubsPlease] Title - 07.5 (1080p).mkv"
"[SubsPlease] Title - 1234 (1080p).mkv"
"[SubsPlease] Title E04 (1080p).mkv"
"[SubsPlease] Title OVA - 01 (1080p).mkv"
"[SubsPlease] Title (1080p Hi10 Dual Audio Multi-Subs BD).mkv"
"Formula1.2024x05.Miami.Grand.Prix.Race.1080p.HDTV.x264-GRP"
"UFC.300.PPV.720p.HDTV.x264-VERUM"
"Boxing.13.10.2024.Fighter.vs.Fighter.1080p.WEB-DL.x264-GRP"
"Boxing.10-13-2024.Fighter.vs.Fighter.1080p.WEB-DL.x264-GRP"
"NHL.2024-25.Team.vs.Team.1080p.HDTV.x264-GRP"
"Artist-Album-WEB2007-FLAC-GRP"
"\u3010\u5b57\u5e55\u7ec4\u3011\u300c\u846c\u9001\u306e\u30d5\u30ea\u30fc\u30ec\u30f3\u300d - 04 (1080p).mkv"
```

</details>

<details>
<summary>Thirty-six manual traces and reconciliation</summary>

I read the pipeline and wrote these predictions before running the probe. T=Text, M=Meta, S=Series, D=Date, R=Resolution, G=Group. The trace-input block above supplies the exact names in row order.

| Row | Predicted semantic lex stream | Predicted result and deciding path |
| --- | --- | --- |
| 1 | T Show, S 1/2, R 1080p, Source WEB-DL, Codec x264, G GROUP | episode, Show, 1/2; collect then inspect |
| 2 | T The Matrix, D 1999, R 1080p, Source BluRay, Codec x264, G GRP | movie, The Matrix, 1999; movieTitles |
| 3 | T Artist Album, ID ABC123, Disc 2x and Source CD, D 2024, Audio FLAC, G GRP | music, Artist/Album, ID ABC123; hyphenated tags select music |
| 4 | T Tool, Version v1.2.3, Platform Win64, G GRP | app, Tool, v1.2.3 |
| 5 | M site SubsPlease, T Title, S 0/4, R 1080p, M sum ABCD1234, Ext mkv | episode, Title, Episode 4, Group and Site SubsPlease |
| 6 | M site, T Title, S 0/4, Version v2, R, Ext | same, Version v2 |
| 7 | M site, T Title, T 01, T 12, R, Ext | movie, Title, no episodes, numeric Unused |
| 8 | M site, T Title 01 12, R, Ext | movie, Title 01~12, no episodes |
| 9 | M site, T Title, S 2/0, S 0/4, R, Ext | episode, Series 2 Episode 4 |
| 10 | T WWE Raw, D 2024/10/14, R, Source, Codec, G | episode, WWE Raw, date 2024-10-14 |
| 11 | T NFL, D 2024/10/13, T Team vs Team, R, Source, Codec, G | episode, Title NFL, Subtitle Team vs Team |
| 12 | T MotoGP, D 2024, T Round 05, R, Source, Codec, G | movie, Title MotoGP, Subtitle Round 05 |
| 13 | T Show, D 2024, R, Codec, T extra words, G | movie, Title Show, Unused extra words |
| 14 | T Film AKA Alternate, D 2024, R, Source, Codec, G | movie, Title Film, Alt Alternate |
| 15 | T Show, S 1/2, T The Return, R, Source, Codec, G | episode, Show, Subtitle The Return |
| 16 | M site Cyrillic, T CJK, S 0/4, R, M sum, Ext | episode, CJK title, Cyrillic group |
| 17 | M spaced site, T Title, S 0/4, R, Ext | episode, Group Anime Time |
| 18 | Source WEB in brackets, T Title, S 0/4, R, Ext | first source resets to text; title WEB and displaced Title |
| 19 | M sum deadbeef, T Title, S 0/4, R, Ext | episode, Sum deadbeef, no group |
| 20 | M site SubsPlease, T Extra, T Title, S 0/4, R, Ext | Title Extra, Title becomes Unused, Group SubsPlease |
| 21 | M site, T Mob Psycho 100, S 0/4, R, Ext | Title Mob Psycho 100, Episode 4 |
| 22 | M site, T 86, S 0/4, R, Ext | Title 86, Episode 4 |
| 23 | M site, T Steins;Gate 0, S 0/4, R, Ext | Title Steins;Gate 0, Episode 4 |
| 24 | M site, T Title, S 0/0, R, Ext | series, Episode 0, SeriesEpisodes empty; zero sentinel |
| 25 | M site, T Title, S 0/7, T 5, R, Ext | episode 7, Subtitle 5 |
| 26 | M site, T Title, S 0/1234, R, Ext | episode 1234 |
| 27 | M site, T Title, S 0/4, R, Ext | episode 4 |
| 28 | M site, T Title OVA, S 0/1, R, Ext | episode 1, Title Title OVA |
| 29 | M site, T Title, R, BitDepth Hi10, Audio Dual Audio, Language Multi-Subs, Source BD, Ext | movie, Title, BitDepth 10BIT, Source BluRay |
| 30 | T Formula1 2024x05 Miami Grand Prix Race, R, Source, Codec, G | movie, complete text title, no Series. 2x lexer limits season to two digits |
| 31 | T UFC 300, Collection PPV, R, Source, Codec, G | episode, Title UFC 300, Episode 0; PPV InfoType |
| 32 | T Boxing, Version 13.10.2024, T Fighter vs Fighter, R, Source, Codec, G | movie, title Boxing; first date regexp accepts shape then rejects month 13 without retry |
| 33 | T Boxing, T 10 and 13, D 2024, T Fighter vs Fighter, R, Source, Codec, G | movie, title Boxing 10-13, Subtitle Fighter vs Fighter; same date validator trap |
| 34 | T NHL, D 2024, T 25 Team vs Team, R, Source, Codec, G | movie, Title NHL, Subtitle 25 Team vs Team |
| 35 | T Artist Album, Source WEB and D 2007, skipped bytes, G | round-trip loss from double advance in NewDiscSourceYearLexer; inspect exact extent |
| 36 | T CJK bracketed name, S 0/4, R, Ext | episode, title includes CJK brackets, Group empty |

All 36 predictions were compared with traces.jsonl. Every record contains the exact lex stream, the post-build stream, and all exported Release fields.

Row 33 was wrong in the prediction. The first day/month-shaped regex names the first number 01 (month), so 10-13-2024 succeeds. The next regex has the same shape with reversed capture names and is unreachable after the first shape matches. 13.10.2024 fails month validation and never tries that next regex. Root: lex.go:129-131,195-225 and NamedCaptureLexer:889.

Rows 12 and 34 correctly predicted Subtitle, but also contain that same text in Unused. movieTitles returns min(start+offset,resolution), which points before the captured subtitle (parse.go:960). unused scans those text tokens again (1232-1235).

Row 18 also assigns Group=Title. fixFirst changes WEB to Text, movieTitles stops at the bracket, episodeTitles collects Title as unused, and unused takes its last nonnumeric token as Group.

Row 35 loses exactly -FLAC from the original. NamedCaptureLexer already returns the advanced i. NewDiscSourceYearLexer adds len(s) again (lex.go:442). The remaining group is a once-lexer end tag and survives.

The other semantic predictions match. Delimiters inside captures explain apparent missing standalone Delim tags: Meta includes trailing spaces, ID includes its closing parenthesis, and Disc can divide 2CD originals across tags. Reproduced originals match on 35 of 36 rows.

Pipeline read in full: parse.go, lex.go, rls.go, prefilter.go, taginfo/taginfo.go, taginfo/taginfo.csv, scan.go, reutil/reutil.go. Phase 1 complete before baseline execution.

The reusable trace program regenerates the complete actual lexical and Release records. In particular, row 33 is a corrected prediction, not an unresolved parser finding.

</details>

## 2. Failure taxonomy

Counts below are reproducible lower bounds for explicit patterns, not an estimate of all bad releases. A name can be in more than one bucket. Files contain unique full names. All selected records happened to be unique. The first seven files use Nyaa only. Sports files use the supplied sports corpus. The selector is included in section 1. Ranges and ordinal seasons are missing information under the stated interpretation of their explicit markers. No external episode database was used.

The tables put broad identity/numbering failures before presentation errors. The ordering uses count × impact qualitatively. It does not assign a false numeric severity to unlike errors.

| Anime bucket / failure file | Count in 13,215 | Example (technical suffix shortened here only) | Actual | Expected | Root cause | Fix bucket |
| --- | ---: | --- | --- | --- | --- | --- |
| [bracketed-range](failures/anime-bracketed-range.txt) | 307 | `Naruto HD [1080p] (001-220) [Complete Series + Movies]` | no `SeriesEpisodes` | retain explicit 1–220 range; no inference about which movie files exist | `lex.go:232`, `lex.go:353`: no bracketed numeric batch rule | v1.5 |
| [ordinal-season](failures/anime-ordinal-season.txt) | 298 | `[shincaps] Seihantai na Kimi to Boku 2nd Season - 03 …` | Series=0 | Series=2; keep title policy separate | `lex.go:72`: season-before-number patterns only | v1.5 |
| [ep-prefix](failures/anime-ep-prefix.txt) | 100 | `[ToonsHub] Detective Conan EP1207 1080p …` | Episode=0; `EP1207` in title | Episode=1207 | `lex.go:87`: delimiter required after `Ep` | v1.5 |
| [leading-group](failures/anime-leading-group.txt) | 46 | `[Commie] Hataraku Maou-sama!` | Group=`sama!`, Title=`Hataraku Maou` | Group=`Commie`, Title=`Hataraku Maou-sama!` | `lex.go:598`, `parse.go:255`: suffix wins before leading group is assigned | v1.5 |
| [versioned-season](failures/anime-versioned-season.txt) | 17 | `[Gecko] Odekake Kozame - S01E69v2 …` | Series=0, Episode=0, version left in title | Series=1, Episode=69, Version=v2 | `lex.go:72`, `lex.go:232`: boundary excludes attached version | v1.5 |
| [decimal-episode](failures/anime-decimal-episode.txt) | 48 | `… Monogatari … - 06.5 [720p]…` | Episode=6, Subtitle=`5` | episode identifier `6.5`, no numeric subtitle | `lex.go:353`, `rls.go:45`: integer-only episode | v2 |
| [zero-episode](failures/anime-zero-episode.txt) | 9 | `[FFF] Saenai Heroine no Sodatekata Flat - 00 [D2861769].mkv` | Type=series, no series/episode pair | retain explicit zero as a numbered item | `parse.go:751`, `rls.go:555`: zero means absent | v2 (presence) |

The group selector excludes all nine `Hakata Ramen … HR-DR` cases, where a suffix attribution is debatable, and `[DVDISO]`, which can name a format. It does not claim all 56 prefix/suffix disagreements are wrong.

| Sports bucket / failure file | Count in 2,505 | Example | Actual | Expected | Root cause | Fix bucket |
| --- | ---: | --- | --- | --- | --- | --- |
| [event-fields](failures/sports-event-fields.txt) | 284 | `ATP.Challenger.Tour.2019.Murray.Trophy.Day.One.…` | round/day/session only in title, subtitle or unused text | structured Day=1 with competition/event retained | no sports fields in `rls.go:25` | v2 |
| [region-collision](failures/sports-region-collision.txt) | 66 | `ATP.Tour.US.Open.2021.09.12.Final.Djokovic.vs.Medvedev.…` | Region=USA, Title=`ATP Tour` | `US Open` in event name; no inferred media region | taginfo/taginfo.csv:583; `parse.go:493` excludes region from reset | v1.5 |
| [language-collision](failures/sports-language-collision.txt) | 13 | `Formula1.2026.Hungarian.Grand.Prix.Practice.One.…` | Language=HUNGARiAN; session omits Hungarian | Hungarian belongs to Grand Prix name | language lexer; `parse.go:946` permits consuming language in subtitle | v1.5 |
| [music-collision](failures/sports-music-collision.txt) | 3 | `Bellator.295.Stots.vs.Mix.1080p.WEB.H264-JUDOCHOP` | Type=music, Other=REMiX | retain competitor Mix; no music inference | taginfo/taginfo.csv:469; `parse.go:709` | v1.5 |
| [platform-collision](failures/sports-platform-collision.txt) | 1 | `Eredivisie.2024.03.10.Ajax.vs.PSV.720p.WEB.h264-ULTRAS` | Type=game, Platform=PSV | retain competitor PSV; no game inference | taginfo/taginfo.csv:534; `parse.go:693` | v1.5 |

The first sports row is an API capability gap, not 284 proved wrong sporting-event classifications. Films such as `Cricket.On.The.Hearth.1967…` and the `World.Cup…HYBRID.MAGAZINE.eBook…` entry were not counted as classification failures merely because they appear in this corpus.

| General bucket | Count / scope | Example | Actual | Expected | Root cause | Fix bucket |
| --- | --- | --- | --- | --- | --- | --- |
| Quadratic edge scan | 3 input lengths × 3 shapes measured | `strings.Repeat("(", 4096)` | 1.431 s, ~9.06 MB | bounded, near-linear delimiter handling | `lex.go:731`, `lex.go:751` | v1.5 |
| Lost bytes | 0 in the three main corpora; fuzz + crafted reproduction | `WEB2000!` | original loses `!` | exact original | `lex.go:442` double advance | v1.5 |
| Date candidate discarded | 1 unambiguous targeted D/M/Y probe | `Event.13.10.2024.Final.1080p.WEB.x264-GRP` | Version=v13.10.2024, no date | 2024-10-13 | `lex.go:129`, `lex.go:195` | v1.5 |
| Invalid calendar date | 1 targeted probe | `Event.2023.02.31.Final.1080p.WEB.x264-GRP` | date=2023-02-31 | no valid date; preserve raw text | `lex.go:205`: separate component validation | v1.5 |
| Unicode regexp mismatch | 2 concrete default-table examples | `Tool.Pſ5`; `Movie.2024.ungekürzt.1080p.BluRay.x264-GRP` | missing PS5; Cut=`ungekürzt` | same result as actual regex: PS5; Cut=Uncut | `prefilter.go:103`, `prefilter.go:539`, `prefilter.go:639` | v1.5 |
| Subtitle also unused | 1 isolated proof; corpus total not scored | `MotoGP.2024.Round.05.1080p.WEB.x264-GRP` | Subtitle=`Round 05`, Unused=`Round 05` | used subtitle tokens absent from Unused | `parse.go:960`, `parse.go:1230` | v1.5 |
| Normalizer truncation | 2 targeted strings | `a�b`, bytes `61 ff 62` | `MustNormalize` returns `a` | preserve valid replacement rune/following b; handle malformed byte explicitly | `rls.go:1257`, `rls.go:1270` | v1.5 |

## 3. Findings

### P-01: Metadata suffix scanning makes short hostile inputs expensive

Evidence. Single 100 ms benchmark samples (not confidence estimates):

| Repeated byte | 256 bytes | 1,024 bytes | 4,096 bytes |
| --- | ---: | ---: | ---: |
| `(` time | 5.59 ms | 86.84 ms | 1.431 s |
| `(` bytes allocated | 39,569 | 584,220 | 9,055,984 |
| `.` time | 5.13 ms | 73.60 ms | 1.304 s |
| `a` time | 80.4 µs | 285.6 µs | 1.311 ms |

The suffix loop runs an unanchored-start, end-anchored regexp across a shrinking prefix at each delimiter (`lex.go:731`). It also prepends one byte to `d` by allocation (`lex.go:751`). Quadrupling delimiter length costs about sixteen times as much. Go's regexp engine does not prevent quadratic work across repeated calls. IRC/RSS strings reach `domain.Release.ParseString` in autobrr. This is a CPU/memory amplification risk, without a demonstrated production outage.

Change, v1.5, 6–10 hours. First skip impossible suffix matches. Then use source indices for pending delimiters instead of repeated prepends. The measured first step adds this inside the suffix pattern loop, before `FindSubmatch`:

```go
if close := strs[l*4+2]; strings.TrimSpace(close) == close &&
    !bytes.HasSuffix(bytes.TrimSpace(src[i:n]), []byte(close)) {
    continue
}
```

The whitespace guard preserves custom delimiter definitions. The original regexp still verifies candidates. This is an experiment, not a proof that all suffix definitions are equivalent. The remaining index change is needed to remove the measured allocation growth. Repeated whitespace also needs a trim-once/index approach. Do not put another whole-prefix scan in the inner loop.

| Experiment | Regression time | Nyaa time | Regression allocs | Nyaa allocs |
| --- | ---: | ---: | ---: | ---: |
| Baseline | 79.62 µs ±2% | 121.05 µs ±2% | 128 | 166 |
| Suffix guard | 64.25 µs ±2%, −19.30% | 87.93 µs ±2%, −27.36% | 128 | 163 |
| Disable prefilter | 135.17 µs ±1%, +69.78% | 199.18 µs ±2%, +64.54% | 128 | 163 |
| Byte delimiter predicate | 83.95 µs ±50%, +5.45% | 125.92 µs ±34%, +4.02% | 128 | 166 |

<details>
<summary>Full benchstat output</summary>

```text
goos: linux
goarch: amd64
pkg: github.com/autobrr/rls
cpu: AMD Ryzen 7 8745HS w/ Radeon 780M Graphics
                │ bench-combined.log │ bench-no-prefilter.log │ bench-byte-delimiter.log │ bench-meta-suffix.log │
                │                   sec/op                   │        sec/op          vs base                 │          sec/op           vs base                │        sec/op         vs base                 │
Moistari-16                                      34.01m ± 2%
Cytec-16                                         132.3m ± 7%
Parse-16                                         79.62µ ± 2%            135.17µ ± 1%  +69.78% (p=0.002 n=6)                 83.95µ ± 50%  +5.45% (p=0.002 n=6)              64.25µ ± 2%  -19.30% (p=0.002 n=6)
ParseTags-16                                     65.67µ ± 3%
Build-16                                         10.50µ ± 5%
ParseLong-16                                     637.2µ ± 1%
AuditNyaa-16                                    121.05µ ± 2%            199.18µ ± 2%  +64.54% (p=0.002 n=6)                125.92µ ± 34%  +4.02% (p=0.004 n=6)              87.93µ ± 2%  -27.36% (p=0.002 n=6)
AuditStartup-16                                  10.92m ± 6%
geomean                                          821.8µ                  164.1µ       +67.14%               ¹               102.8µ        +4.73%               ¹            75.16µ       -23.43%               ¹
¹ benchmark set differs from baseline; geomeans may not be comparable

                │ bench-combined.log │ bench-no-prefilter.log │ bench-byte-delimiter.log │ bench-meta-suffix.log │
                │                    B/op                    │          B/op           vs base                │           B/op            vs base                │         B/op           vs base                │
Moistari-16                                     3.555Mi ± 1%
Cytec-16                                        142.1Mi ± 0%
Parse-16                                        8.483Ki ± 0%             8.399Ki ± 1%  -0.99% (p=0.026 n=6)                 8.502Ki ± 1%       ~ (p=0.180 n=6)              8.471Ki ± 0%       ~ (p=0.264 n=6)
ParseTags-16                                    6.488Ki ± 0%
Build-16                                        1.824Ki ± 0%
ParseLong-16                                    74.81Ki ± 0%
AuditNyaa-16                                    11.38Ki ± 0%             11.07Ki ± 1%  -2.75% (p=0.002 n=6)                 11.39Ki ± 0%       ~ (p=0.970 n=6)              11.13Ki ± 0%  -2.24% (p=0.002 n=6)
AuditStartup-16                                 11.44Mi ± 0%
geomean                                         164.3Ki                  9.643Ki       -1.87%               ¹               9.839Ki       +0.13%               ¹            9.709Ki       -1.19%               ¹
¹ benchmark set differs from baseline; geomeans may not be comparable

                │ bench-combined.log │ bench-no-prefilter.log │ bench-byte-delimiter.log │ bench-meta-suffix.log │
                │                 allocs/op                  │       allocs/op         vs base                │        allocs/op          vs base                │       allocs/op        vs base                │
Moistari-16                                      55.10k ± 0%
Cytec-16                                         955.7k ± 0%
Parse-16                                          128.0 ± 0%               128.0 ± 1%       ~ (p=1.000 n=6)                   128.0 ± 0%       ~ (p=1.000 n=6) ¹              128.0 ± 0%       ~ (p=1.000 n=6) ¹
ParseTags-16                                      79.00 ± 0%
Build-16                                          48.00 ± 0%
ParseLong-16                                      843.0 ± 0%
AuditNyaa-16                                      166.0 ± 0%               163.0 ± 1%  -1.81% (p=0.002 n=6)                   166.0 ± 0%       ~ (p=1.000 n=6) ¹              163.0 ± 0%  -1.81% (p=0.002 n=6)
AuditStartup-16                                  218.1k ± 0%
geomean                                          2.299k                    144.4       -0.91%               ²                 145.8       +0.00%               ²              144.4       -0.91%               ²
¹ all samples are equal
² benchmark set differs from baseline; geomeans may not be comparable
```

</details>

All reported timing comparisons have benchstat p≤0.004, n=6. The byte experiment has wide variance and offers no demonstrated gain. Discard it. The suffix guard used 8.471 KiB per regression parse (no significant change, p=0.264), and 11.13 KiB per Nyaa parse (−2.24%, p=0.002). The prefilter-off run is a cost experiment, not a proposed optimization.

Each of the three overlays produced zero full-output differences on 429 regression keys, 13,215 Nyaa names, 2,505 sports names, and 584 label records. This does not establish equivalence on unseen strings or custom lexers. Regression risk is suffix whitespace, metadata order and original-byte preservation. Check the same differential inputs, custom whitespace delimiters, unmatched brackets, and the size series before accepting the fix. Do not claim the prototype removes the quadratic case. That part has not been implemented or benchmarked.

### C-01: A combined source/year token advances twice and drops input

`WEB2000!` formats back to `WEB2000`. `Artist-Album-WEB2007-FLAC-GRP` loses `-FLAC`. `NamedCaptureLexer` already advances `i` (`lex.go:889`); `NewDiscSourceYearLexer` adds the capture length again (`lex.go:442`). A group extracted by an earlier once lexer can survive beyond the skipped bytes, hiding the loss in normal field assertions.

Change, v1.5, 1–2 hours:

```diff
- if s, v, i, n, ok := lexer(src, buf, i, n); ok {
+ if _, v, i, n, ok := lexer(src, buf, i, n); ok {
@@
- return append(start, tags...), end, i + len(s), n, true
+ return append(start, tags...), end, i, n, true
```

Check exact originals and remaining Audio/Source/Year tags for WEB/CD/VLS forms with and without following tokens. Then rerun the existing fuzz seed and the five-minute budget. Risk is low: the helper's returned index is explicit. No accuracy gain is projected from the main corpora, which all round-trip today.

### A-01: Small gaps in episode rules account for hundreds of explicit misses

The four independent Nyaa selectors find 100 `EP` prefixes, 17 attached versions, 298 ordinal seasons and 307 bracketed ranges. These counts overlap and are not additive release accuracy. Label examples distinguish causes:

- No rule: `[BakaWolf-m.3.3.w] Special A 01 (H.264) [C83164B9].mkv` leaves `01` in Title and Episode=0. The label expects 1. `[Coalgirls]_Toradora_ED2_(704x480_DVD_AAC)_[3B65D1E6].mkv` has no typed ending-number support.
- Wrong rule: `[Thomku] Kill la Kill 01 - 07 Batch [720p][AAC][MP4]` sets Episode=7, although the labelled range starts at 1. The dash-number rule consumes the right endpoint.
- Debatable label/model: Habari labels `One Piece Movie 11 - Film Z …` as Episode=11. A film ordinal is not necessarily a television episode. Do not change generic episode rules to satisfy that label.

Change, v1.5, 10–16 hours. Make `Ep`'s separator optional in the explicit episode-prefix rule. Allow an attached `vN` after an explicit season/episode and emit a separate Version tag, with each original byte captured once. Add ordinal-season matching near an explicit episode marker. Add bracketed numeric ranges after a title/leading group and project them through existing series tags and `SeriesEpisodes()`. Retain the first endpoint in the existing flat field. This reuses the current model and requires no range API.

Do not remove every numeric boundary, or infer every bare title number as an episode. `Mob Psycho 100`, `86` and `Steins;Gate 0` keep their title numbers in the tested ` - 04` forms. `S2 - 04` and `Season 2 - 04` already work. Four-digit ` - 1234` works. `Part 2` and `Cour 2` have no rule. Treating either as Season=2 invents a relationship. Special/OVA/ONA/OAD/Recap remain text while ` - 01` provides the episode; NCOP/NCED plus a bare ordinal are untyped. Those identities belong to M-01.

Risk is high for title numbers and season words. Verify all selected names, preserve correct scene ranges (`S01E01-E03`, `S01E01E03`), and retain negative cases with numbered titles and `Monogatari … Off & Monster Season`. Do not train an unrestricted numeric heuristic on the label sets.

### A-02: Suffix group extraction removes title words before Build can select the leading group

The Nyaa list contains 46 selected failures, including `Maou-sama!`, `Komugi-chan OVA`, alternate-title endings such as `-hen`, and batch endpoints. The anitomy example `[ANBU-umai]_Haiyoru!_Nyaru-Ani_[596DD8E6].mkv` yields Group=`Ani` and Title=`Haiyoru! Nyaru`, despite its explicit leading group. `NewGroupLexer` runs once on the suffix (`lex.go:598`) before `leadingGroup` (`parse.go:255`) fills an empty Group from Site.

Change, v1.5, 8–12 hours. Preserve a leading fansub group candidate before suffix selection. For that context, require the suffix candidate to occur after a recognized technical block, or retain it as text. Do not always prefer any leading bracket: `[1080p HEVC]`, `[WEB]`, and checksums have different meanings. A local group-priority change is smaller than a new parsing engine.

Multiple prefix blocks need a narrow rule. `[G1][G2] Title - 04 …` makes `G2` the Title and puts `Title` in Unused. The parser accepts only one `site`. The second block can be another group, a title, or tags. No universal deletion rule is justified. Similarly, `[deadbeef]` is a checksum under the eight-hex rule even when intended as a group. Preserve the ambiguity in diagnostics (M-02). There is no evidence that every eight-hex group-shaped block is misclassified.

Cyrillic groups and CJK title text work with ASCII brackets in probes. `【字幕组】「日本語」` stays in Title and gives no Group. Full-width bracket support can be added to the bracket lexer without normalizing the source bytes, but its group-versus-title policy needs labelled examples first.

Risk is high for trailing scene groups and short tag names. Check all 46 names and both directions: `[Fansub] … -SceneGroup` and genuine hyphenated title suffixes. Preserve `%o`, technical leading blocks, CRC and arbitrary custom metadata delimiters.

### S-01: General tag words override sporting-event text

Three names become music due to Mix/Mixed. One becomes a game due to PSV; 66 use a tournament or competitor name as a region; 13 use Hungarian from the Grand Prix name as a language. Concrete outputs and CSV lines are in the taxonomy. `Real Madrid`, `Sporting`, `Union`, `Racing` and `Red Bull` were preserved in the constructed `vs` probes. These are not additional proved collisions.

Change, v1.5, 8–14 hours. Reset tags to text in the evidenced phrases. These are competitors adjacent to `vs`/`gegen`, `US Open`, `UK Championship`, and an adjective immediately before `Grand Prix`. Keep these checks in the existing builder's title recovery, before collection/type selection. For `Mixed Team`/`Mixed Final`, do not infer music from REMiX alone in a video event context. This is a limited set of evidenced phrase rules, not a team database or general sports detector.

Deleting PSV, USA or REMiX rows breaks games, regions and music. Removing TypeExclusive alone is insufficient: `inspect` returns Game/Music through direct cases at `parse.go:693` and `709`. The old branch's valid inputs must remain: `Tool.PSV`, `Album.Remix.FLAC`, and ordinary media region/language tags. `The.Office.US.S07E03…` also demonstrates why all region tags cannot become title text.

Risk is high where words have both title and technical roles. Check the 83 collision names (66+13+3+1), the existing region/music/game fixtures, and title/technical pairs. Adding a Sports enum does not fix these consumed words. The type/model work follows separately in M-03.

### C-02: Date validation cannot try the second interpretation and accepts impossible dates

For `Event.<date>.Final.1080p.WEB.x264-GRP`, observed results are:

| Date text | Actual date/type | Interpretation |
| --- | --- | --- |
| `2024.10.13`, `2024-10-13` | 2024-10-13, episode | correct Y/M/D |
| `13.10.2024` | no date; Version=v13.10.2024, movie | wrong; unambiguous D/M/Y |
| `10-13-2024` | 2024-10-13, episode | correct M/D/Y |
| `24.10.13` | 2024-10-13, episode | existing two-digit-year policy |
| `13.10.24` | 2013-10-24, episode | ambiguous; do not declare a global D/M/Y fix |
| `2023.02.31` | 2023-02-31, episode | invalid calendar date |

`lex.go:129` and `131` have the same syntactic shape with exchanged month/day capture names. `NamedCaptureLexer` commits to the first syntactic match before `NewDateLexer` validates. A failed month does not retry the next candidate. Separate month/day parsing accepts February 31. The year/month-only pattern at `lex.go:127` also contains a literal colon where an ordinary boundary was intended. This path is not a demonstrated corpus failure.

Change, v1.5, 4–6 hours. Validate each full candidate before accepting it, try the next candidate after validation failure, and check calendar consistency. Preserve the current first-valid interpretation for ambiguous two-digit dates. Correct the year/month pattern with a focused case only after checking year-versus-episode precedence. Risk: worldwide date ambiguity and music dates. Verify the matrix above, leap days, year-only inputs, and music catalogues. Do not globally swap month/day ordering.

### C-03: The prefilter and indexed lookup do not preserve regexp Unicode semantics

`Tool.Pſ5` is rejected by the prefilter although the case-insensitive PS5 regexp accepts the long s. `Movie.2024.ungekürzt.1080p.BluRay.x264-GRP` produces Cut=`ungekürzt`. The linear tag finder returns canonical `Uncut`. The expander parses `[uü]` by bytes (`prefilter.go:539`), while regexp matches runes. The indexed finder uses ASCII case folding (`prefilter.go:639`). A direct `ſdr` lookup is nil in the index and SDR in the linear finder.

This contradicts the no-false-rejection claim at `prefilter.go:16`. A Go word boundary can occur inside text that participates in Unicode case folding. The ASCII first-run argument is incomplete. `TestPrefilterSound` generates candidates from the same expander being tested and skips unsupported expansions (`prefilter_test.go:22`). Passing it does not prove equivalence to regexp.

Change, v1.5, 3–5 hours. Keep ASCII fast paths. Admit non-ASCII candidate text to the regexp, and use the existing linear finder for non-ASCII lookup inputs/unsupported row expansions. Do not extend the byte regexp parser to another approximation of Unicode. If general regexp analysis is later needed, use Go's `regexp/syntax` tree.

Risk is low to semantics, moderate to performance on CJK names if the fallback scans the whole remaining buffer repeatedly. Check only the candidate region needed for the fast-path decision, and benchmark Nyaa. Verify actual regex acceptance against prefilter admission and canonical lookup for `ſ`, `K`, `ü`, inline case-sensitive rows and ordinary ASCII. No fixture accuracy gain is claimed until measured.

### P-02: Regexp execution dominates; retain the prefilter and optimize measured calls

One `NewDefaultParser()` compiles 907 regexps: 812 CSV-row matchers and 95 other expressions. It shares the 10 already-compiled builder regexps, giving 917 instances used by the default pipeline. `rls.go:840` creates the default parser once. Constructing one per release adds the measured 10.92 ms and 11.44 MiB each time. There is no evidence that autobrr does that.

Instrumentation counted entry to Go 1.27's regexp engine `(*Regexp).find`, including repeat searches inside one regexp API call. Lexer names identify the active stage. Total entries per parse are 180.01/271.71/207.27 for regression/Nyaa/sports. Disabling the prefilter gives 267.47/411.57/316.43. These are engine calls, not 917 attempts at every token.

| Active lexer/stage | Regression calls/parse | Nyaa | Sports |
| --- | ---: | ---: | ---: |
| Build | 55.88 | 71.71 | 54.79 |
| Audio | 3.22 | 4.62 | 3.41 |
| Date | 7.41 | 14.38 | 7.68 |
| Disc | 6.91 | 6.83 | 8.51 |
| DiscSourceYear | 2.21 | 2.22 | 3.12 |
| Episode | 0.22 | 1.16 | 0.34 |
| Ext | 1.00 | 1.00 | 1.00 |
| Genre | 8.66 | 15.96 | 12.46 |
| Group | 3.89 | 3.50 | 3.89 |
| ID | 4.05 | 7.08 | 5.87 |
| Meta | 22.03 | 33.82 | 19.00 |
| Arch | 0.02 | 0.29 | 0.01 |
| BitDepth | 0.06 | 0.56 | 0.01 |
| Channels | 0.07 | 0.42 | 0.12 |
| Container | 0.04 | 0.33 | 0.00 |
| Cut | 0.06 | 0.32 | 0.05 |
| Edition | 0.80 | 1.87 | 0.71 |
| Language | 4.42 | 7.89 | 6.20 |
| Other | 1.49 | 2.86 | 1.28 |
| Platform | 0.10 | 0.29 | 0.07 |
| Region | 0.05 | 0.34 | 0.03 |
| Resolution | 1.38 | 1.85 | 2.04 |
| Size | 1.59 | 2.41 | 2.16 |
| Source | 2.24 | 2.87 | 3.17 |
| Codec | 0.63 | 1.06 | 1.00 |
| Collection | 0.53 | 0.72 | 0.04 |
| HDR | 0.07 | 0.29 | 0.02 |
| Series | 5.61 | 10.25 | 6.83 |
| TrimWhitespace | 2.00 | 2.00 | 2.00 |
| Version | 0.70 | 1.56 | 2.52 |
| Parse | 42.68 | 71.25 | 58.98 |

In the next table, R/A means rejected/admitted prefilter attempts. Execution columns give regexp entries per parse with the prefilter on/off. Combined regexp lexers already use one alternation per type. Combining the CSV again or adding a trie does not address the largest remaining costs.

| Tag type | Regression R/A | Regression calls on/off | Nyaa R/A | Nyaa calls on/off | Sports R/A |
| --- | ---: | ---: | ---: | ---: | ---: |
| Arch | 3733/9 | 0.02/8.72 | 164848/3889 | 0.29/12.77 | 26272/30 |
| BitDepth | 2392/24 | 0.06/5.63 | 121140/7342 | 0.56/9.72 | 16444/22 |
| Channels | 2155/31 | 0.07/5.10 | 110306/5610 | 0.42/8.77 | 16166/292 |
| Codec | 2414/272 | 0.63/6.26 | 124612/14070 | 1.06/10.49 | 16508/2500 |
| Collection | 2977/226 | 0.53/7.47 | 140105/9500 | 0.72/11.32 | 21341/105 |
| Container | 1852/19 | 0.04/4.36 | 101652/4356 | 0.33/8.02 | 15607/0 |
| Cut | 1988/26 | 0.06/4.69 | 110657/4231 | 0.32/8.69 | 16259/124 |
| Edition | 1656/342 | 0.80/4.66 | 90053/24699 | 1.87/8.68 | 14591/1773 |
| HDR | 2409/29 | 0.07/5.68 | 124674/3858 | 0.29/9.73 | 16466/45 |
| Language | 85/1897 | 4.42/4.62 | 10458/104259 | 7.89/8.68 | 843/15521 |
| Other | 1543/638 | 1.49/5.08 | 78099/37749 | 2.86/8.77 | 13260/3198 |
| Platform | 3724/41 | 0.10/8.78 | 164851/3892 | 0.29/12.77 | 26118/185 |
| Region | 1865/22 | 0.05/4.40 | 101890/4447 | 0.34/8.05 | 15607/66 |
| Resolution | 2837/591 | 1.38/7.99 | 137188/24474 | 1.85/12.23 | 18693/5103 |
| Size | 3110/680 | 1.59/8.83 | 136959/31788 | 2.41/12.77 | 20883/5420 |
| Source | 2780/960 | 2.24/8.72 | 130756/37981 | 2.87/12.77 | 18366/7936 |

The CPU profile used an anchored `^BenchmarkParse$` for five seconds, rather than also profiling ParseTags/ParseLong through a substring match. It recorded 6.31 CPU seconds during 5.97 elapsed seconds, including runtime workers.

| Function | Flat CPU time | Flat share |
| --- | ---: | ---: |
| regexp.(*Regexp).tryBacktrack | 970ms | 15.37% |
| regexp.(*machine).add | 970ms | 15.37% |
| regexp.(*machine).step | 500ms | 7.92% |
| regexp.(*bitState).shouldVisit | 430ms | 6.81% |
| regexp/syntax.(*Inst).MatchRunePos | 400ms | 6.34% |
| runtime.memmove | 200ms | 3.17% |
| unicode.SimpleFold | 190ms | 3.01% |
| regexp.(*Regexp).backtrack | 170ms | 2.69% |
| regexp.(*bitState).push | 140ms | 2.22% |
| regexp.(*inputBytes).step | 140ms | 2.22% |

All regexp rows follow `ParseString → ParseRelease → Parse / next or Build/title → regexp API → find`. The backtracking path then goes through `backtrack → tryBacktrack → bitState/inputBytes`. The machine path goes through `machine.match → step/add → MatchRune → MatchRunePos → SimpleFold`. `memmove` is mainly reached from `machine.add` (170 of its 200 ms), also parser slice work. `find` has 72.74% cumulative CPU. Do not add that to its descendants' percentages.

Change, v1.5, 2–4 hours after P-01 and C-03. Keep the prefilter. Measure a first-byte guard on high-attempt non-regexp lexers only after the suffix fix: Genre, Date, Series, ID. Measure whether existing regexp prefix checks already cover the cheap cases. An unconditional lexer reorder is unsafe: date/version, source/group, and music/type examples show that order is semantic. The measured byte-delimiter change is rejected. Risk and acceptance check: byte-identical full differential output and six repeated benchmarks. Retain a guard only if it helps both representative workloads or has a stated workload tradeoff.

### P-03: Tags and title normalization account for most allocation objects

`unsafe.Sizeof(Tag{})` is 56 bytes and `Release{}` is 696 bytes on amd64, before backing arrays and strings. `NewTag` (`rls.go:190`) stores a string slice and converts each byte capture to a string. Reclassification retains the prior type and finder for one `Revert`. It is not a general history stack. Build modifies its input slice. Callers who need the lexical stream must copy it, as BenchmarkBuild does.

The allocation profile sampled 7,767,534 objects, including some startup. It is not an exact allocation count per parse. The benchmark's 128/166 figures serve that purpose.

| Function | Flat objects | Flat share |
| --- | ---: | ---: |
| github.com/autobrr/rls.NewTag | 1902414 | 24.49% |
| regexp.(*Regexp).replaceAll | 1297896 | 16.71% |
| regexp.(*Regexp).FindSubmatch | 941021 | 12.11% |
| internal/bytealg.MakeNoZero | 387756 | 4.99% |
| github.com/autobrr/rls.(*TagParser).next | 323364 | 4.16% |
| github.com/autobrr/rls.(*TagBuilder).collect | 229379 | 2.95% |
| github.com/autobrr/rls.(*TagBuilder).pivots | 200264 | 2.58% |
| github.com/autobrr/rls.(*TagBuilder).title | 189453 | 2.44% |
| regexp.(*Regexp).ReplaceAllStringFunc | 177494 | 2.29% |
| github.com/autobrr/rls.NamedCaptureLexer.func1 | 166575 | 2.14% |

Paths from `ParseString`: NewTag and NamedCaptureLexer come through Parse/next and lexer closures; FindSubmatch comes from those paths and once lexers. replaceAll and ReplaceAllStringFunc are chiefly Build/title, plus the parser's working-buffer replacement. collect/pivots/title are direct Build descendants. MakeNoZero is reached through string construction in Build/title and Tag.Channels. Allocation space differs: `next` 111.11 MiB, Parse 76.10 MiB, NewTag 58 MiB, FindSubmatch 52.50 MiB, regexp lexer 47.55 MiB, Build 44.03 MiB in a 625.99 MiB profile.

Change, v1.5, 4–8 hours of bounded experiments. Replace FindSubmatch with Find where no captures are used. Use FindSubmatchIndex where only capture boundaries are needed. Avoid a working-buffer copy when no `_`, `,` or `+` replacement is needed. Avoid title regex replacement when its trigger cannot occur. These can remove the specific temporary capture/replacement allocations. No measured result here supports claiming zero allocations for Parse or Build. Check mutation/aliasing before reusing source slices. Keep only a change that wins in the differential benchmark.

Build has repeated linear tag scans and repeated title normalization. Its measured 10.50 µs/48 allocations justify a targeted title fast path, not an index of every tag type. There is no sorting in the main Build path; `SeriesEpisodes()` sorts when separately called (`rls.go:153`). Normalizer chains are already pooled (`rls.go:1139`, `1185`), so another pool is redundant. No blanket whole-buffer lowercase allocation or hot-path general logging was found. Tag formatting uses fmt, but benchmark Parse does not format every tag.

v2 alternative, M-02: offsets into one retained immutable input string can remove capture copies and variadic string-slice allocations. The output strings, tag array and release lists still need storage. Retaining the input also has a memory lifetime cost. Do not promise a zero-allocation release object.

### M-01: Integer fields cannot distinguish absent, zero, fractional or separately numbered episodes

There is no Absolute field in this checkout (`rls.go:25`). ` - 04` puts 4 in Episode, `S2 - 04` gives Series=2/Episode=4, and nothing records whether a bare number is absolute or season-relative. ` - 07.5` becomes Episode=7 plus Subtitle=`5`. `S01E00` produces a Series tag but no `SeriesEpisodes()` entry, because `Episodes()` excludes zero (`rls.go:555`). The nine selected Nyaa zero markers include numbered specials and movie-like releases. All being called a series is not an adequate presence model.

Change, v2, 16–24 hours plus consumer migration. Add a present/absent number representation, raw episode identifier, number kind (unspecified/season-relative/absolute/special/opening/ending/movie), and explicit ranges. A string identifier with optional parsed integer is sufficient. A float is unsuitable for identifiers such as `12.10` and `12.1`. Keep cour/part separate from season. Do not invent absolute numbering from filenames without a source hint.

A v1.5 method can expose details from tags. An explicit zero episode token can select Type=episode while the flat field keeps its zero sentinel. The full presence/fraction semantics remain v2. For autobrr, `internal/domain/release.go:727–731` uses `cmp.Or` on numbers. It must use presence when copying Season/Episode/date fields. Filters, stored fields and notifications must then choose first-item legacy projection or structured matching. No database migration was implemented or exhaustively inventoried.

Risk is high. Keep the v1 adapter stable and test both representations on decimal, zero, batch and season-relative inputs. The 48 decimal names define a concrete acceptance set, not a forecast of total anime accuracy.

### M-02: Make lexer consumption explicit and expose diagnostics without replacing the pipeline

`LexFunc` returns two tag slices, two indices and a bool (`lex.go:14`). The once loop adopts both indices, but the multi loop discards the new end index (`parse.go:99`). A custom multi lexer returning `i+1,n-1` is called four times on `abcd`, not twice. The meaning of the same return value depends on placement. There is no progress check for a custom lexer returning success without advancing. This is a caller-extension problem, not an input-triggered hang in the measured default corpus.

Change, v2, 12–20 hours. Replace the positional returns with a named result carrying consumed prefix/suffix spans, tags and matched status. Distinguish edge lexers from position lexers at registration. Validate bounds and forward progress centrally. Keep the existing ordered lexer pipeline. An adapter can host old lexers while callers migrate. For tags, preserve immutable raw spans and separate inferred classification from original classification. Retain a v1 `Tags()` projection instead of an open-ended revert history.

An additive v1.5 `ParseWithDetails` can return Release plus diagnostics and optional hints without changing ParseString. Use concrete hints for date order or known content category. Diagnostics must identify ambiguous number/group, invalid date, unconsumed content and input limit rejection. The normal parser still has deterministic legacy output.

Autobrr's `extraParseSource` reads `Tags()`, `TagType()` and `Prev()` (`internal/domain/release.go:749–764`), so a tag-model migration must replace that suffix recovery or keep the adapter. Most fields are copied from ParseString at `688–744`. Retaining a v1 projection minimizes the initial change. Risk: custom lexers/builders and raw formatting. Verify the bounded `abcd` contract probe and custom lexer bounds, then compare all raw/tag output through the adapter.

Keep Compare as an explicitly named legacy ordering helper during v1.5. It mixes normalized title, number, quality and original-string ordering (`rls.go:865`). No autobrr call site was found. There is no measured reason to remove it now. In v2, use names such as ReleaseKind/TokenKind, keep Cut and Edition distinct, and document Alt as an alternate title. Replace opaque Meta `key:value` strings with key/value records only in the new details model. Keep existing codec/audio/language string vocabularies unless a typed representation solves an actual consumer need. Changing every slice to an enum is not justified by this audit.

### M-03: Sports needs event fields and caller context, not fabricated TV episodes

There is no Sports enum. The tested `UFC.300` and `UFC.Fight.Night.240` keep the numbers in Title and leave Episode=0. `PPV` can select Type=episode through its collection type. `Formula1.2024x05…` is not parsed as Series=2024/Episode=5: the x-style season pattern allows only a short season number. The entire phrase remains title text. Claims to the contrary describe a different checkout.

For `.1080p.WEB.x264-GRP` suffixes, the constructed pattern matrix gives:

| Input prefix | Current useful fields | Missing or misplaced information |
| --- | --- | --- |
| `F1.2024.R05.Miami.Qualifying` | Title=F1, Year=2024, movie | R05/Miami/Qualifying in Subtitle and Unused |
| `MotoGP.2024.Round.05` | Title=MotoGP, Year=2024, movie | Round 05 in Subtitle and Unused |
| `WWE.Raw.2024.10.14` | Title=WWE Raw, full date, episode | event kind |
| `NFL.2024.10.13.Team.vs.Team` | Title=NFL, full date, Subtitle=Team vs Team | two competitors |
| `NBA.2024.10.22.Lakers.vs.Timberwolves` | Title=NBA, full date, Subtitle=Lakers vs Timberwolves | two competitors |
| `EPL.2024.Matchday.08` | Title=EPL, Year=2024, movie | matchday |
| `Tour.de.France.2024.Stage.05` | Title=Tour de France, Year=2024, movie | stage |
| `Wimbledon.2024.Mens.Final` | Title=Wimbledon, Year=2024, movie | event/session |
| `Boxing.2024-10-12.Fighter.vs.Fighter` | Title=Boxing, full date, episode | competitors |
| `NHL.2024-25.Team.vs.Team` | Title=NHL, Year=2024, movie | season range; `25` is subtitle text |
| `NFL.2024.Week.06` | Year=2024; Week 06 as text | week |
| `Event.2024.Day.1` / `Session.1` | Year=2024; marker in text | day/session ordinal |
| `F1.2024.Practice.1` | Year=2024; Practice 1 in text | session kind and ordinal |
| `EPL.2024.Extended.Highlights` | Cut=Extended.Cut; Highlights text | highlight kind; avoid treating Extended as a film cut |
| `WWE.2024.Pre-Show.PPV` | Collection=PPV, episode; Pre-Show text | session kind |

Change, v2, 20–32 hours plus consumer work. Add an optional Event record: competition, event name, season label/range, round kind/ordinal, session, competitors and a date with precision. Use a caller's trusted feed/category hint to enable sports phrase rules. Require more than `vs`, a year, or a known team word when no hint exists. Keep the raw name available. This is one additive record alongside technical metadata, not a sports parser rewrite or a live team lookup.

A v1.5 details API can carry this record experimentally while retaining legacy Type. A breaking Sports kind needs autobrr's source-recovery allowlist (`release.go:750`) updated, plus filter/type serialization and database compatibility review. autobrr already copies technical metadata independently. That path can remain. Risk is false classification of sports-themed films and shows. Include the corpus contaminants as negative cases and do not use all 2,505 lines as automatic sports truth.

### C-04: Used subtitle tokens are returned again by Unused

`MotoGP.2024.Round.05.1080p.WEB.x264-GRP` has Subtitle=`Round 05`, yet both text tokens are in `Unused()`. `movieTitles` returns an index before the subtitle it consumed (`parse.go:960`); `unused` starts from there (`1230`). The same issue occurs in the NHL season-range probe.

Change, v1.5, 2–3 hours. Return the end of all consumed title/subtitle spans, or pass their actual spans to `unused` if they are not contiguous. Do not discard all post-year text. Verify subtitle retention and empty Unused for this probe, plus `Show.2024.1080p.x264.extra.words-GRP`, where the extra words really are unused. Risk is hidden changes to inferred trailing Group, since `unused` can also choose a group.

### C-05: Exported utility paths have concrete validation and resource bugs

- `MustNormalize("a�b")` and `MustNormalize(string([]byte{'a',255,'b'}))` both return `a`. The collapser treats every RuneError as incomplete input and reports bytes consumed past it (`rls.go:1257`, `1270`). A valid U+FFFD is not malformed UTF-8. Direct `Transform` calls on `a ` with atEOF=false and ` b` with atEOF=true produce `ab`. The whole-string result is `a b`. The exported collapser loses the space between chunks.
- After one warmup load with GC disabled, 20 `taginfo.LoadFile` calls grow `/proc/self/fd` from 7 to 27. `taginfo/taginfo.go:123` opens a file without closing it. Eventual finalizers do not provide bounded ownership.
- `taginfo.New("X", "", "", "", "INVALID", "")` returns no error. Its empty-title switch arm skips type validation (`taginfo/taginfo.go:53`). Empty CSV titles are otherwise valid: 126 of 812 rows deliberately fall back to Tag.
- `NewScanner(WithWorkers(0))` consumes no input and exits only when the 20 ms context expires, with `context deadline exceeded`. `scan.go:182` accepts zero workers, leaving the producer with no receiver.

Change, v1.5, 4–7 hours. Add `defer f.Close()` after successful open. Validate tag type independently of title fallback. Clamp invalid worker counts to one (or add a checked constructor while keeping the old constructor usable). For UTF-8, distinguish valid RuneError width 3, incomplete sequences when !atEOF, and malformed bytes at EOF. Never claim unprocessed suffix bytes were consumed. A stateful streaming collapser must retain pending spaces and context across calls, or the exported API must explicitly provide a whole-string path. Do not silently concatenate chunks.

Risk: invalid-input compatibility and transform streaming semantics. The short probes in section 1 reproduce each issue without real network calls. Verify those, split UTF-8 at each byte boundary, and normalizer pool reset. No panic was observed from zero Release `%o/%e/%q/%s/%v`: output was `||""||`. `%v` is documented but not implemented by Release.Format (`rls.go:99`). Fix the documentation or implement the existing documented verb in v1.5 (under one hour). The public Tag constructor can still be misused with too few captures. No default-parser short-input panic was found.

### T-01: Preserve independent truth and make audit inputs reproducible

The source contradicts the audit prompt's statement about test export. `TestExport_tests` copies `test.exp` at `rls_test.go:411` and writes those values at `441`. It does not parse names to regenerate expectations. Its purpose is ordering. This does not prove that the expectations are correct. Section 5 lists concrete problems. Avoid `TESTS=export` during measurement because it writes both YAML and tag CSV.

Change, v1.5, 3–5 hours. Check in a small independent regression subset from the failure lists with explicit field and original assertions. Pin fixture URLs/hashes and document scoring rules. Keep downloaded corpora outside test network paths. Add the minimal fuzz target with the lost-byte seed. The corrected soundness check must use the real regexp as oracle, including Unicode fold variants. Keep label disagreements visible rather than adjusting expected values until scores pass.

Risk: inflated confidence from shared labels, incomplete precision scoring, or a test pattern that matches no test. Validate fixture counts/hashes before scoring and assert that the intended tests execute. Do not claim an original accuracy floor that is not present.

## 4. Roadmap

v1.5, in order:

1. T-01: retain independent reproductions and scoring definitions. This is the acceptance basis for the fixes below.
2. P-01 and C-01: remove quadratic suffix work and double advancement. Confirm full differential output for performance-only changes. Correctness fixes must have a reviewed, explicit expected delta.
3. C-03 and C-02: restore regexp equivalence and valid date selection. Re-measure Nyaa after the Unicode fallback.
4. A-01: explicit EP/version/ordinal/range forms, one syntax family at a time. Keep bare-number inference deferred.
5. A-02, S-01 and C-04: group priority, evidenced sporting phrases, and used-token accounting. These depend on the independent negative fixtures. They have the highest semantic regression risk.
6. C-05: resource ownership, utility validation, zero workers and UTF-8 handling. The file/type fixes are independent and can run earlier.
7. P-02/P-03: retain only further measured wins. Add a details/diagnostics adapter only when M-01/M-03 consumer integration is being built.

The fixed-cost work above is roughly 49–80 engineering hours from the finding estimates, before optional allocation experiments and additive API work. It is a planning estimate, not measured delivery time. No rewrite is needed.

The suffix prototype measured about 64 µs and 128 allocations per regression name. It measured 88 µs and 163 allocations per Nyaa name. The correctness fixes can change those figures. The other fixes need new measurements. The index change must demonstrate near-linear work on hostile input.

The accuracy targets cover 100 EP forms, 17 versioned forms, 298 season markers, 307 ranges and 46 group cases. Each fix needs explicit expected changes. Previously correct cases must keep their results unless review establishes an error. These overlap. They are not a projected percentage improvement on the unlabelled corpus. Re-score the same labelled fields after each family. These data do not support a numeric estimate for total accuracy after v1.5.

v2, in order:

1. M-01: present/absent numbers, raw identifiers, kind and ranges. Port autobrr's `cmp.Or` copying to explicit presence. Keep the v1 adapter.
2. M-03: optional event details using caller context, then a Sports kind after autobrr's type paths and source allowlist are ready. The 284 marker names are an extraction acceptance set, not a sports classifier training truth set.
3. M-02: named lexer results and immutable raw spans if allocation pressure remains material after v1.5. First migrate autobrr's `Tags/Prev` suffix logic. A 12–20 hour lexer/tag contract effort does not include a full production offset representation and every consumer migration. Size that implementation after the adapter proves the contract.

v2 can represent all 48 selected fractional markers and explicit zero markers without truncation. That is a representational guarantee to test, not a forecast of title or group accuracy. There is no measured v2 speedup: offset tags are a hypothesis to benchmark, and the richer model can add allocations. A numeric performance or whole-release accuracy promise has no measured basis.

## 5. Wrong expectations in `tests.yaml`

- `Hold.The.Sunset.S01E00.Christmas.Special.720p.HDTV.X264-MTB`: current `type: series`. Correct `type: episode`, with explicit episode zero retained in the details model. `E00` identifies a numbered special, not a season pack. v1.5 can fix Type. V2 is needed to distinguish zero from absence in the field itself.
- `(2001)A Space Odyssey(1961).mkv`: current Title=`2001`, Group=`Odyssey`, Unused=`A Space`. Correct Title=`2001 A Space Odyssey`, empty inferred Group/Unused. The only visible suffix after the name is a year and extension; `Odyssey` is title text. This claim uses the string's structure and does not replace its supplied 1961 with an externally known film year. v1.5, A-02/title boundary recovery.
- `Rick and Morty 020 (2016) (digital) (d'argh-Empire).cbr`: current Group=`Empire`, Unused=`digital d'argh`. Correct Group=`d'argh-Empire`, Unused=`digital`. The final bracketed attribution is split at its internal hyphen. v1.5, A-02.
- `Rick and Morty Presents - Krombopulos Michael (2018) (digital) (d'argh-Empire).cbr`: the same current and correct group/unused values and cause. v1.5, A-02.

These are field corrections, not a claim that every other expectation is independently validated. `1080i → 1080p`, the Office US region, music alternate-title choices and sport years need an explicit model policy. They are not silently relabelled here to inflate a score.

## 6. What was not verified

- Original corpus provenance, original labelled snapshots and accuracy floors, the missing `accuracy_test.go`/`fuzz_test.go`, and `anime_titles.txt`. The fetched fixtures are identified replacements. The user can supply more inputs later.
- Whole-release anime/sports precision or recall. The corpora are unlabelled. The failure selectors give lower bounds. The scorer omits unlabelled false positives. It also omits fields without an agreed mapping. These include anime type, alternate episode, volume, part, release information and some subtitle-language conventions. Not every raw label mismatch was adjudicated.
- A completed five-minute fuzz run. It stopped at the reported failure. Probes covered nested brackets, delimiters, empty text, group-only/date-only strings, mixed tabs/newlines, replacement characters and long repeated inputs. Other panic or superlinear paths can remain.
- End-to-end autobrr traffic, deployment, bans or outage impact. Consumer inspection used local autobrr commit `247e2794bc7aa94ca6e8896343627466a48d1d14` (its module replace points to rls v0.8.1). No real tracker, IRC server or RSS endpoint was called. Downstream database/filter migration effort is not fully inventoried.
- Exhaustive regexp-language equivalence or overlap enumeration for all possible CSV strings. The CSV loads 812 rows with no duplicate (Type,Tag). Blank titles use a valid fallback. Concrete overlap/precedence problems are reported. Sample canonical probes (`AAC`, `HEVC`, `WEB`, `BD`, `Complete`, `Mixed`, `PSV`, `US`, `10bit`, `Hi10`, `HDR10`, `1080i`, `Uncut`) map as shown by the reproduction tool; `Special` matches no CSV row. No additional unreachable CSV row is asserted without a proof.
- Stream-safe Collapser behavior after a fix, Compare's full ordering laws, concurrent Err calls during a scan, custom lexer safety, and complete Revert misuse behavior. The reported utility/contract probes are narrower. The source contains an unbounded Matchr regexp cache. No observed path lets IRC text supply these patterns. This report does not rank it as an input vulnerability.
- Scanner performance is noisy: a newline-joined YAML-key stream (434 physical records, because two keys contain five newlines) measured 100.3 ms ±29% at one worker, 11.22 ms ±78% at 16; 4.189/4.730 MiB and 56.76k/58.21k allocations per stream. These are not directly comparable to the 429-key whole-corpus parse benchmark. `scan.go:112` emits completion order, with source IDs. It does not restore source order. Panic recovery takes a stack only on panic (`scan.go:132`), not for normal records. Pure channel/recover overhead was not isolated, and the noise does not justify changing worker defaults.
- No production optimization patch exists. Only the suffix-guard, prefilter-off and byte-delimiter overlays were benchmarked and differentially checked. Shape measurements were single samples. No claimed speedup for an unimplemented allocation, dispatch, range, sports or v2 representation change.
