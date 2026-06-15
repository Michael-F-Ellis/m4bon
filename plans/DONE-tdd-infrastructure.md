# TDD Infrastructure Plan

**Date:** 2026-06-13
**Status:** Approved — ready to execute

## Objective

Transform m4bon into a test-driven development environment where:
- DSL input is the only thing you type
- Expected MusicXML output is asserted via golden files or shorthand fragments
- Visual rendering via Verovio is a `make render` away
- No MuseScore, no editor, no manual steps

---

## 1. Create `SHORTHAND.md` — Reference for the assertion language

A document in the repo root defining the notation used to describe expected musical output. Used both for test assertions and for the user to communicate what they see in rendered notation.

### Duration Codes

Single letter + optional dot suffixes for augmentation dots.

| Code | Duration | Example |
|------|----------|---------|
| W    | whole        | `Wc4` |
| H    | half         | `Ha3` |
| Q    | quarter      | `Qc4` |
| E    | eighth       | `Eb3` |
| S    | 16th         | `Sg4` |
| T    | 32nd         | `Tb4` |
| .    | dotted (suffix)  | `Q.c4` = dotted quarter |
| ..   | double-dotted     | `H..a3` = double-dotted half |
| ...  | triple-dotted     | (if needed) |

### Notes

Format: `<duration><pitch>[alteration]<octave>`

- Duration: one of W, H, Q, E, S, T + optional `.` `..` `...`
- Pitch: `a-g` (case-insensitive, stored lowercase)
- Alteration: `#` sharp, `&` flat, `%` natural (optional)
- Octave: number (4 = middle C)

Examples:
- `Qc4` — quarter C4
- `Q.c4` — dotted quarter C4
- `H..a3` — double-dotted half A3
- `E#f4` — eighth F♯4
- `S&b3` — 16th B♭3

### Rests

Format: `<duration>;` (semicolon after duration code)

Examples:
- `Q;` — quarter rest
- `E;` — eighth rest

### Chords

Format: `(<pitch1>,<pitch2>,...) <duration>`

Examples:
- `(C4,E4,G4) Q` — C major triad, quarter duration
- `(D4,F4,A4) E` — D minor triad, eighth

### Tuplets

Format: `M/N<duration><pitch>` — M notes in the time of N of the base unit.

The base unit is determined by the duration code. For a quarter-note triplet in 4/4:
- `2/3Qa3` = 2 notes in the time of 3 quarter notes (quarter-note triplet hemiola)
- `3/2Qa3` = standard triplet (3 notes in time of 2 quarters)

Examples:
- `2abc` in 4/4 → `2/3Qa3 2/3Qb3 2/3Qc4` (quarter-note triplet across 2 beats)
- In 4/4, `a--bcd` → `Ea3 2/3Sb3 2/3Sc4 2/3Sd4` (dotted eighth + triplet 16ths)

### Ties

A `-` separator between tokens indicates a tie from the preceding note to the following note. Where two consecutive tokens have the same pitch and one is followed by `-`, they are tied.

Example:
- `Ha3 - Ea3 Eb3 Qc4` = half A3 tied to eighth A3, eighth B3, quarter C4

### Full example

DSL: `a - -b c`
Short-hand: `Ha3 - Ea3 Eb3 Qc4`

---

## 2. Create `KEY-SIGNATURES.md` — Circle-of-fifths reference

A lookup table mapping DSL key sig directives to accidentals.

The `K` directive accepts the tonic letter + accidentals in any order: `KE&`, `K&e`, `KEb`, `Ke&` all mean E♭ major (3 flats).

| Directive   | Key       | Accidentals                   |
|-------------|-----------|-------------------------------|
| `KC` `K%`   | C major   | (none)                        |
| `KG`        | G major   | F♯                            |
| `KD`        | D major   | F♯, C♯                        |
| `KA`        | A major   | F♯, C♯, G♯                    |
| `KE`        | E major   | F♯, C♯, G♯, D♯               |
| `KB`        | B major   | F♯, C♯, G♯, D♯, A♯           |
| `KF#` `K#f` | F♯ major  | F♯, C♯, G♯, D♯, A♯, E♯       |
| `KC#` `K#c` | C♯ major  | F♯, C♯, G♯, D♯, A♯, E♯, B♯   |
| `KF`        | F major   | B♭                            |
| `K&b` `KB&` | B♭ major  | B♭, E♭                        |
| `K&e` `KE&` | E♭ major  | B♭, E♭, A♭                    |
| `K&a` `KA&` | A♭ major  | B♭, E♭, A♭, D♭               |
| `K&d` `KD&` | D♭ major  | B♭, E♭, A♭, D♭, G♭           |
| `K&g` `KG&` | G♭ major  | B♭, E♭, A♭, D♭, G♭, C♭       |
| `K&c` `KC&` | C♭ major  | B♭, E♭, A♭, D♭, G♭, C♭, F♭   |

### Implementation Notes

- Normalization: extract the letter and accidentals from `K...`, sort by canonical order to match the table.
- Accidentals in `K` always refer to the tonic. `KE&` = E♭ major (not E major with B♭).
- The key signature always means Ionian (major) mode.
- Key and meter can appear in either order: `KE& M6/8` or `M6/8 KE&`.
- Both directives must appear at the start of the DSL string (beginning of measure 1).
- If no `K` directive, default = C major (no accidentals).
- If no `M` directive, default = 4/4.

---

## 3. Update DSL Grammar — Key & Meter Directives

### Parser changes (`parser/parse.go` and/or `parser/pipeline.go`)

Add a `stripDirectives` or `parsePreamble` function:

1. Scan the beginning of the DSL string for `K...` and `M...` patterns
2. Accept either order
3. `K` directive: extract letter and accidentals (accommodates `KE&`, `K&e`, `KEb`, `K#f`, etc.)
4. `M` directive: parse `M<num>/<den>` (e.g. `M6/8`, `M4/4`)
5. Return the stripped DSL (directives removed → just the beat tokens)
6. Bare `K` with no letter/accedentals => C major (natural)
7. Error if directives not at start of string (or separated only by whitespace)

### `main.go` changes

1. Remove `-time` flag entirely — time sig comes from DSL only
2. Add `-key` flag? No — key sig comes from DSL only too (but keep for potential override)
3. Actually from the discussion: remove both flags, default to 4/4 C major

### `ParseDSL` signature change

Current: `ParseDSL(dsl string, timeNum, timeDen int) DSLResult`
New: `ParseDSL(dsl string) DSLResult` (time sig and key sig parsed from DSL)

Or: `ParseDSL(dsl string, defaultTimeNum, defaultTimeDen int, defaultKey string) DSLResult`

The parser extracts `M...` and `K...` from the DSL string, applies defaults if not found, and
returns the key signature info alongside events. This means `DSLResult` needs a new field:

```go
type DSLResult struct {
    Events    []Event
    Key       KeySignature  // parsed or default
    TimeNum   int           // parsed or default
    TimeDen   int           // parsed or default
    Err       error
}

type KeySignature struct {
    Fifths int  // negative = flats, positive = sharps
}
```

The MusicXML generator then uses `Key.Fifths` to emit `<fifths>` in the `<key>` element.

### Pipeline flow

```
DSL text → stripDirectives (extract K, M) → sanitize → tokenize → parseGroup
         → resolveDurations → splitNonStandard → resolveOctaves → MusicXML
```

---

## 4. MusicXML Schema Validation

### Download XSD

The MusicXML 4.0 schema is available from the W3C or the MusicXML repository.

```bash
curl -o musicxml/musicxml.xsd https://raw.githubusercontent.com/w3c/musicxml/gh-pages/schema/musicxml.xsd
# or use the local mirror documented below
```

Save to `musicxml/musicxml.xsd` (add to `.gitignore` or commit a known-good version).

### Validation in tests

Add a test helper:

```go
func validateMusicXML(t *testing.T, xml string) {
    t.Helper()
    tmpFile := filepath.Join(t.TempDir(), "out.mxl")
    os.WriteFile(tmpFile, []byte(xml), 0644)
    cmd := exec.Command("xmllint", "--schema", "musicxml/musicxml.xsd", "--noout", tmpFile)
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("MusicXML schema validation failed:\n%s", string(out))
    }
}
```

### What to validate

- All generated MusicXML files should pass schema validation
- Add a test case that runs schema validation on the output of every `.dsl` test case

---

## 5. Golden File Test Infrastructure

### Directory structure

```
test/
├── cases/
│   ├── basic-notes.dsl
│   ├── basic-notes.expected.mxml
│   ├── sustain-chain.dsl
│   ├── sustain-chain.expected.mxml
│   ├── chord-group.dsl
│   ├── chord-group.expected.mxml
│   ├── tuplet-triplet.dsl
│   ├── tuplet-triplet.expected.mxml
│   ├── compound-6-8.dsl
│   └── compound-6-8.expected.mxml
└── golden_test.go    # test that runs all .dsl vs .expected.mxml pairs
```

### Expected file format

Complete MusicXML documents (or fragments that can be compared). The test runner:

1. Reads `.dsl` file
2. Strips comments, joins lines
3. Runs full pipeline
4. Formats output with `xml.MarshalIndent`
5. Compares against `.expected.mxml` content
6. Reports diff on mismatch

### Test helper

```go
// In test/golden_test.go
func TestGoldenFiles(t *testing.T) {
    // Walk test/cases/*.dsl
    // For each: read, parse, generate, compare with *.expected.mxml
}
```

### Updating golden files

When intent changes, overwrite the `.expected.mxml` with new output:
```bash
go test ./test -update-golden  # a flag that writes new output
```

---

## 6. Visual Rendering Pipeline

### Prerequisites (user-run)

```bash
brew install verovio          # done
# rsvg-convert already available via librsvg
```

### Pipeline script: `render.sh`

```bash
#!/bin/bash
# render.sh — DSL → MusicXML → SVG → PNG
set -euo pipefail

DSL="${1:-}"
if [ -z "$DSL" ]; then
    echo "Usage: $0 <dsl-string>"
    echo "   or: $0 -f input.dsl"
    exit 1
fi

if [ "$1" = "-f" ]; then
    XML=$(./m4bon "$(cat "$2")")
else
    XML=$(./m4bon "$DSL")
fi

echo "$XML" | verovio --stdin -f xml -o - -s 120 --adjust-page-height \
    | rsvg-convert -o /tmp/m4bon-render.png

echo "Output: /tmp/m4bon-render.png"
open /tmp/m4bon-render.png
```

### Verovio flags explained

| Flag | Purpose |
|------|---------|
| `--stdin` | Read MusicXML from pipe |
| `-f xml` | Input format = MusicXML |
| `-o -` | SVG to stdout |
| `-s 120` | 120% scale for readability |
| `--adjust-page-height` | Trim whitespace around notation |
| `--svg-view-box` | Enable responsive scaling |

### Makefile target (optional)

```makefile
render:
	./render.sh "$(DSL)"

render-file:
	./render.sh -f $(FILE)
```

---

## 7. Implementation Sequence

### Phase A — Documentation (30 min)

1. Create `SHORTHAND.md`
2. Create `KEY-SIGNATURES.md`
3. Update `AGENTS.md` DSL Reference with K/M directives

### Phase B — Parser changes (1–2 hours)

1. Add `stripDirectives()` to `parser/pipeline.go`
   - Regex: `^\s*(K\S+)?\s*(M\d+/\d+)?\s*` (accept either order)
   - Parse key sig letter + accidentals
   - Parse time sig num/den
   - Return stripped DSL + parsed metadata
2. Add `KeySignature` type and update `DSLResult`
3. Wire key sig into pipeline and MusicXML generation
4. Add `<key>` element to MusicXML output (uses `<fifths>`)
5. Update `ParseDSL()` to accept just `dsl string` (no time num/den params)

### Phase C — main.go changes (15 min)

1. Remove `-time` flag
2. `ParseDSL(dsl)` — no time params
3. Pass parsed time/key to Generate

### Phase D — Test infrastructure (1 hour)

1. Download MusicXML 4.0 XSD
2. Add schema validation helper
3. Create golden file test runner
4. Update existing `.dsl` test files with K/M directives
5. Create `.expected.mxml` files from current correct output
6. Add `-update-golden` flag support

### Phase E — Visual pipeline (30 min)

1. Create `render.sh`
2. Test with all existing `.dsl` cases
3. Add optional `Makefile`

### Phase F — Existing test file updates

Update `test/cases/*.dsl` files to include meter (and key where needed):

```
# basic-notes — single beat, quarter notes in 4/4
M4/4
c d e f
```

```
# sustain-chain — a held for 2.5 beats
M4/4
a - -b c
```

```
# compound-6-8 — two dotted-quarter beats
M6/8
abc def
```

---

## 8. Open Questions / Future Work

- **Multiple measures**: The `K` and `M` directives are measure-1 only. Multi-measure input (barline `|`) is deferred.
- **Mid-measure key/meter changes**: Could use `[K...]` or `[M...]` inline syntax. Deferred.
- **Shorthand test assertions inline in Go tests**: e.g. `assertShorthand(t, "Ha3 - Ea3 Eb3 Qc4", result.Events)` — a Go function that parses the shorthand and compares against the event list. This would allow tests to be self-documenting without external golden files for every case.
- **Visual diff of PNG outputs**: Compare renderings pixel-by-pixel or by perceptual hash. Useful but potentially fragile.
