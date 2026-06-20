# m4bon — Agent Guide

## Project Overview

**Beat-Oriented Note Entry → MusicXML converter.**

Users type rhythmic patterns in a compact DSL where whitespace separates beats and characters within a beat subdivide it equally. The tool produces standard MusicXML that can be opened in MuseScore, Finale, Dorico, or any notation software.

**Tech stack:** Go, MusicXML 4.0, CLI (`flag` package). No external dependencies.

Originally a MuseScore 4 QML plugin — the parser was ported to Go when the MuseScore plugin API proved too constrained for rapid development (see `lessons/session-2026-06-14.md` §11-12).

---

## Directory Structure

```
m4bon/
├── m4bon.go                 # Public API: m4bon.Compile(dsl) → (string, error)
├── go.mod / go.sum          # Go module (github.com/mellis/m4bon)
├── Makefile                 # build, test, check, golden targets
├── cmd/
│   └── m4bon/
│       ├── main.go          # CLI entry point
│       ├── main_test.go     # Integration tests (CLI invocation)
│       ├── golden_test.go   # CLI golden file tests
│       └── tui/             # Interactive TUI (macOS-only, darwin+cgo)
├── parser/
│   ├── parse.go             # Tokenizer + parseGroup state machine
│   ├── pipeline.go          # resolveDurations, split, octaves, ParseDSL
│   ├── parse_test.go        # Unit tests for parser
│   └── sanitize.go          # SanitizeDSL (stripped out of musicxml)
├── musicxml/
│   ├── xml.go               # MusicXML structs + generator
│   ├── xml_test.go          # Unit tests
│   └── golden_test.go       # In-process golden tests (calls ParseDSL+Generate directly)
├── render/
│   ├── cell.go              # Cell IR types (StyleClass, Cell, CellSeq)
│   ├── render.go            # Core renderer: buildCells, NumSlots-based intra-group sustains
│   ├── ansi.go              # ANSI terminal formatter
│   └── render_test.go       # Unit tests for renderer
├── frac/
│   └── frac.go              # Fraction type, GCD, power-of-2 helpers (shared)
├── theory/
│   └── theory.go            # NoteOffsets, FifthsToAccidentalMap, EffectiveAccidental
├── midi/
│   ├── generate.go          # SMF generation, voiceToChannel
│   └── generate_test.go
├── test/
│   └── cases/               # .dsl + .expected.mxml test case files
├── lessons/
│   ├── session-2026-06-14.md
│   ├── session-2026-06-17.md
│   └── session-2026-06-18.md
├── AGENTS.md
├── README.md
├── LICENSE                  # MIT
└── .gitignore
```

---

## DSL Reference

### Core principle

**Whitespace separates beats.** Characters grouped without spaces subdivide that beat equally. You never specify explicit durations — the grouping and the time signature determine everything.

### Symbol table

| Purpose | Symbol |
|---------|--------|
| Sharp | `#` |
| Flat | `&` |
| Natural | `%` |
| Octave up | `^` |
| Octave down | `/` |
| Sustain / tie | `-` |
| Rest | `;` |
| Chord open | `(` |
| Chord close | `)` |
| Barline (ignored) | `|` |

### Time signature → beat resolution

| Denominator | Numerator % 3 == 0? | Beat unit | Examples |
|---|---|---|---|
| 2 | — | half note | 2/2, 3/2 |
| 4 | — | quarter note | 2/4, 3/4, 4/4 |
| 8 | yes (compound) | dotted quarter (3 eighths) | 6/8, 9/8, 12/8 |
| 8 | no | eighth note | 5/8, 7/8, 11/8 |
| 16 | yes (compound) | dotted eighth (3 sixteenths) | 6/16, 9/16, 12/16 |
| 16 | no | sixteenth note | 5/16, 7/16 |

### Beat groups

Each whitespace-separated token spans **1 beat** by default. An optional numeric prefix `N` makes it span **N beats**. Within its span, characters equally divide the total duration.

Examples in 4/4:
```
a b      → quarter + quarter
ab       → 2 eighths
abc      → triplet eighths
a--b     → a dotted eighth + b sixteenth
2abc     → quarter-note triplet (hemiola)
(ace)f   → A-minor chord eighth + F eighth
```

### Sustain (`-`)

Extends the **previous pitch** through its position slot(s). A `-` at the start of a group refers to the last pitch of the previous group (error if none).

### Rest (`;`)

Produces silence at that position slot. Counts as active for subdivision.

### Chord grouping `(...)` and voice-poly chords

Groups pitches into a single chord occupying one position slot. Strictly ascending.

When `-` or `;` appears inside `()`, the chord becomes **voice-polyphonic** — each entry is a separate voice:

| Inside `()` | Meaning |
|---|---|
| `(ceg)` | Traditional chord: C+E+G in voice 1 |
| `(c - e)` | Voice 1: C, Voice 2: sustain, Voice 3: E |
| `(-e)` | Voice 1: sustain, Voice 2: E |
| `(c;e)` | Voice 1: C, Voice 2: rest, Voice 3: E |

Voice indices are 1-based by entry position. Sustains extend the same voice's prior event. Cross-group and cross-measure sustain chains work per-voice. Max 4 voices per chord recommended.

### Pitch entry

- Letters `a-g`, case-insensitive
- Accidentals precede letter: `#f`, `&b`, `%c`
- UTF-8 glyphs ♯♭♮ mapped during normalization
- Initial reference: C4 (MIDI 60)
- Relative octave (Lilypond "closest interval" rule)
- `^` / `/` force octave up/down

---

## Pipeline

```
DSL text → stripDirectives (extract K, M) → sanitize → tokenize → parseGroup
         → resolveDurations → splitAtBarline → splitNonStandardDurations → resolveOctaves → MusicXML
```

### Output: MusicXML

- `<score-partwise>` with `<attributes>`, `<note>`, `<tie>`, `<time-modification>`
- 480 DPPQ (divisions per quarter note)
- MIDI-to-MusicXML: `octave = midi/12 - 1` (MIDI 60 = C4)

---

## CLI Usage

```
Usage: m4bon [options] [dsl]
  -f string    Read DSL from file
  -o string    Write MusicXML to file (default: stdout)
  -render      Colorized text output
  -tui         Launch interactive TUI performance/learning tool
  -bpm float   Tempo in BPM for TUI mode (default 120)

Time and key signatures are specified in the DSL via K... and M... directives:
  m4bon "M4/4 c d e f"
  m4bon "KE& M6/8 abc def"

Examples:
  m4bon "c d e f"
  m4bon "M6/8 abc def"
  m4bon -f test/cases/basic-notes.dsl -o out.mxl
  m4bon -render "M4/4 c d e f"        # Colorized text output
  m4bon -tui                          # Launch TUI (empty state)
  m4bon -tui -f score.dsl -bpm 96     # TUI with file + custom tempo
```

---

## MIDI Generation

The `midi` package (macOS-only via `//go:build darwin && cgo`) provides:

- `GenerateSMF(measures, bpm)` → `([]byte, Timeline, error)` — SMF bytes with score notes (ch1-3), metronome (ch10), and tempo map
- `GenerateMetronomeOnly(measures, bpm)` → `([]byte, Timeline, error)` — Metronome-only SMF

`Timeline` has `MeasureStarts []time.Duration`, `TotalDuration`, `TempoBPM`.

## TUI Application

Key bindings: space=play/pause, s=stop, [/]=tempo±5, {/}=tempo±1, 0=reset 120,
↑/↓=volume, ←/→=seek measure, j/k=scroll, ?=help, q=quit. Lives in `cmd/m4bon/tui/`.

## Essential Commands

```bash
make               # Build binary + run all tests (default)
make build         # Build the m4bon binary only
make test          # Run all tests (without building binary)
make check         # Build + test + vet (full pre-commit check)
make clean         # Remove build artifacts
make golden        # Update golden test files
make notify MSG="done"  # Send iMessage notification after long task

# Individual commands (when make is unavailable):
go build -o m4bon ./cmd/m4bon/  # Build (0.5s) — ALWAYS rebuild after changes
go test ./...                    # Run all tests (0.4s)
./m4bon "c d e f"               # Quick test
./m4bon -tui "c d e f"          # Launch TUI
./notify.sh "message"           # Send iMessage notification
```

## Library Usage

```go
import "github.com/mellis/m4bon"

xml, err := m4bon.Compile("M4/4 (c) (-e) (-g) | (-f) (d-) (b-) | (ce) - -")
text, err := m4bon.Render("M4/4 c d e f")  // ANSI-escaped color text

// MIDI generation (darwin+cgo only)
import "github.com/mellis/m4bon/midi"
import "github.com/mellis/m4bon/parser"
measures := parser.ParseDSL("M4/4 c d e f")
smfBytes, timeline, err := midi.GenerateSMF(measures.Measures, 120)
```

---

## Key Design Decisions

| Decision | Rationale |
|---|---|
| Go over Node.js | Single static binary, zero runtime deps, `encoding/xml`, excellent test facilities |
| MusicXML over MIDI | Human-readable, diff-able, ties/tuplets native, works with all notation apps |
| `cmd/m4bon/main.go` layout | Standard Go project layout — root is library package `m4bon`, CLI is `cmd/m4bon/` |
| Slice indices for voice tracking | Pointers into `append`-ed slices become stale on reallocation; use `voiceLastIdx map[int]int` instead |
| 480 DPPQ | Standard MIDI convention; cleanly divides all required durations |
| Events sorted by (tick, voice) in XML | MusicXML requires notes in onset order; multi-voice events interleave by tick then voice |
| Greedy duration split + barline-aware split | Greedy for non-standard durations; fraction-based barline split (no ticks) runs first to respect the invisible-barline rule |
| Two-layer render architecture | Core renderer produces `[]Cell` IR (color class, overline, subscript), formatters (ANSI, future HTML) convert independently |
| No external XML libs | `encoding/xml` covers the subset we need |
| `NumSlots` field on Event | Tracks intra-group sustain slots for render, guarded by `GroupIdx == gi` so cross-group sustains don't attach dashes |
| `go vet` with `make check` | `go vet` catches unkeyed struct fields; run `make check` before committing to catch issues early |
| Makefile with `make build` | `go test ./...` skips `darwin && cgo` TUI code — binary is NOT rebuilt by tests. `make` ensures binary is fresh |
| Scheduler-less TUI cursor | The `macaudio.Scheduler` approach had subtle timing issues because `Position()` returns `playStartUs` when stopped. Elapsed-time polling via `positionMsg` + `measureAtTime()` is more robust |

---

## Render Format

When `-render` is set, each measure is output as one line:

```
1:  c₄ d e f
2:  a₄ b c d
```

- **Colors**: sharp=red, flat=blue, dbl-sharp=orange, dbl-flat=green, sustains/rests=grey
- **RGB values** match FQS: `rgb(209,34,34)`, `rgb(152,140,254)`, `rgb(255,165,0)`, `rgb(4,182,4)`, `rgb(160,160,160)`
- **Octave subscripts**: shown on first pitch per measure, plus any pitch with `^`/`/`
- **Chord parentheses + italic**: chords rendered as `(c₄eg)` with medium-dark grey parens and italic pitch letters
- **Pure sustain groups**: each `-` beat rendered as a separate grey dash
- **Accidentals**: key signature + explicit accidentals determine effective alteration and color; `%` correctly cancels key sig
- **Beat-group grouping**: events from the same DSL beat group rendered without internal spaces

## Known Limitations

- Single staff (piano), maximum 4 voices per chord
- No nested chords inside voice-poly groups
- Voice-poly tuplet combinations not yet supported
- Barline split covers 4/4 midpoint only — odd time sigs may need adjustment
- Beaming may be incomplete for multi-voice measures (same-voice notes at non-adjacent sorted positions)
- Render uses beat-group index grouping (GroupIdx on Event), not tick positions
- Cross-measure sustain for voice-poly is fragile: `priorEvents[1]` hard-coded in legacy sustain path (pipeline.go lines 68, 152). When a voice-poly chord follows a traditional chord (e.g. `(c d e) | (- - g)`), the individual pitches of the traditional chord must be findable through the voice 1 prior event. Sustain-after-rest semantics (e.g. `(c ; e) | (- ; g)`) require nil-sentinel handling — a rest establishes the voice but doesn't provide a pitch to extend.
- `encoding/xml` produces `<chord>true</chord>` instead of spec-conformant `<chord/>`. Most renderers accept this.
