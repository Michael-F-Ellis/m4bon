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
│   ├── theory.go            # NoteOffsets, FifthsToAccidentalMap, EffectiveAccidental
│   ├── chords.go            # Chord symbol normalization, ChordRoot extraction
│   └── chords_test.go
├── midi/
│   ├── generate.go          # SMF generation with SMFOptions, chord root track
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
| Chord symbols | `:H` |
| Lyrics | `:L` |

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

### Chord symbols (`:H`)

Optional per-measure directive. Appears after notation, before `|`. One chord symbol per beat; `-` = sustain, `;` = rest.

```
M4/4 c d e f :H C - G7 - |          # four beats: C major, sustain, G7, sustain
M4/4 c d e f :L My heart is sad :H C - G7 - |  # order-independent
```

Input grammar (keyboard-friendly):

| Input | Display | Input | Display |
|---|---|---|---|
| `C` | C | `C7` | C⁷ |
| `Cm`, `C-`, `Cmin` | C⁻ | `Cm7`, `C-7` | C⁻⁷ |
| `Cdim`, `C°` | C° | `Cmaj7`, `CΔ`, `CΔ7` | C∆⁷ |
| `Chdim`, `Cø`, `Cm7b5` | Cø⁷ | `C7♯9`, `C7#9` | C⁷♯⁹ |
| `Caug`, `C+` | C⁺ | `Csus`, `Csus4` | Csus⁴ |
| `C♯`, `C#` | C (red) | `C♭`, `C&` | C (blue) |

Root accidentals: color only (same scheme as notation). Extension accidentals use ♯/♭ glyphs.

### Lyrics (`:L`)

Optional per-measure directive. One syllable per position (including rests and sustains). Lyrics for rests (`;`) and sustains (`-`) render as grey dashes/semicolons in the lyric column. Special tokens:

| Token | Meaning |
|---|---|
| `-` | Syllable extension on sustain position |
| `;` | Rest syllable (lyric continues through silence) |
| `*` | Melisma (note belongs to current syllable) |
| `_` | Multi-syllable within one beat |
| `no_thing` | Two syllables "no" + "thing" on one note |

```
M4/4 ;e fe f e :L My heart is sad and |
M4/4 g - ag fe :L Glo - ** ** |
M4/4 cd ec ^g /c :L no_thing more_than feel ings. |
M4/4 a - - ; :L mance. - - ; |              # dashes/semicolons rendered in lyric column
```

### Render output

Three-column layout in `-render` and TUI: `CHORDS : NOTES : LYRICS`. Columns only appear when at least one measure has that directive.

---

## Pipeline

```
DSL text (newline-separated measures) → sanitize → tokenize (per line)
         → scanMeasureDirectives (K, M, B, :H, :L) → parseGroup
         → resolveDurations → splitAtBarline → splitNonStandardDurations → resolveOctaves → MusicXML
```

Each line of input is one measure. Blank lines are ignored. Comments (`# text`) are stripped.
Lines starting with `!` are comment blocks preserved in render output
(dim italic medium green, toggleable via `showComments`),
but stripped from MusicXML. Consecutive `!` lines form a comment block
rendered as separate output lines.

### Output: MusicXML

- `<score-partwise>` with `<attributes>`, `<note>`, `<tie>`, `<time-modification>`
- 480 DPPQ (divisions per quarter note)
- Measures separated by newlines; end-of-file terminates the final measure
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
  m4bon -render "M4/4 c d e f"                 # Colorized text output
  m4bon -render "M4/4 c d e f :H C - G7 -"     # With chord symbols
  m4bon -render "M4/4 c d e f :L My heart is sad"  # With lyrics
  m4bon -tui                                   # Launch TUI (empty state)
  m4bon -tui -f score.dsl -bpm 96              # TUI with file + custom tempo
```

---

## MIDI Generation

The `midi` package (macOS-only via `//go:build darwin && cgo`) provides:

- `GenerateSMF(measures, bpm)` → `([]byte, Timeline, error)` — backward-compat wrapper (metronome on, no roots)
- `GenerateSMFWithOptions(measures, bpm, opts)` → `([]byte, Timeline, error)` — full control via `SMFOptions`
- `GenerateMetronomeOnly(measures, bpm)` → `([]byte, Timeline, error)` — Metronome-only SMF

`SMFOptions` has `Metronome bool`, `Roots bool`, `Backbeats bool`. Root track uses MIDI channel 8 (Fingered Electric Bass, program 33) in bass range E1–E2 (octave-shifted to 3 if below MIDI 28).

`Timeline` has `MeasureStarts []time.Duration`, `TotalDuration`, `TempoBPM`.

## TUI Application

The TUI (macOS-only) is a self-contained rehearsal tool with transport-based playback, metronome options, chord root playback, and recording. Key bindings:

| Key | Action |
|-----|--------|
| `space` | Play / Pause (disabled during recording) |
| `s` | Stop (also stops recording) |
| `r` | Start / stop recording |
| `m` | Toggle metronome |
| `b` | Toggle backbeats (click on 2 and 4) |
| `R` | Toggle chord roots |
| `[` / `]` | Tempo -5 / +5 BPM |
| `{` / `}` | Tempo -1 / +1 BPM |
| `0` | Reset tempo to 120 |
| `↑` / `↓` | Seek start measure -1 / +1 |
| `⇧↑` / `⇧↓` | Seek end measure -1 / +1 |
| `←` / `→` | Volume down / up |
| `j` / `k` | Scroll down / up |
| `o` | Toggle octave subscripts |
| `u` | Reload from source file |
| `q` | Quit |
| `?` | Toggle help |

All playback goes through `macaudio.Transport` — a single proxy that routes to MIDI or recording playback. Recording is ephemeral and one-at-a-time.

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

xml, err := m4bon.Compile("M4/4 (c) (-e) (-g)\n(-f) (d-) (b-)\n(ce) - -")
text, err := m4bon.Render("M4/4 c d e f")  // ANSI-escaped color text
text, err := m4bon.Render("M4/4 c d e f :H C - G7 - :L My heart is sad")  // three-column

// MIDI generation (darwin+cgo only)
import "github.com/mellis/m4bon/midi"
import "github.com/mellis/m4bon/parser"
lines := parser.SanitizeDSL("M4/4 c d e f")
result := parser.ParseDSL(lines)
smfBytes, timeline, err := midi.GenerateSMF(result.Measures, 120)

// With options:
opts := midi.SMFOptions{Metronome: true, Roots: true, Backbeats: true}
smfBytes, timeline, err := midi.GenerateSMFWithOptions(measures.Measures, 120, opts)
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
| `GroupSlots` for cross-group sustain render | `GroupSlots []int` on `MeasureResult` tracks per-group slot count. Render computes `startSustains = slotCount - nonSplitCount - intraGroupSustains` — cross-group sustains that leave no Event are still visible as dashes |
| `EffAccidental` separate from `Accidental` | `EffAccidental` stores the effective accidental (including measure-level persistence). `Accidental` stores the raw DSL value. MusicXML uses `Accidental` for printed accidental symbols and `EffAccidental` for `<alter>`. Render uses `EffAccidental` for note style color |
| Sustain events must not carry `OctaveShift` | Three sustain-creation paths (pure-group, mixed-group, voice-poly) all set `OctaveShift: 0` — a sustain continues the same pitch, not a shifted one |
| Diatonic-only octave resolution | Pitch identity (letter + accidental + octave) resolved via diatonic step distance (`resolveOctave`, `nextHigherOctave`), never MIDI semitones. `midiFromPitch` is a one-way lookup, never used for decisions |
| `ResolvedOctave` / `ResolvedOctaves` on Event | Octave stored alongside `Midi`/`Midis` during resolution. MusicXML and render consume it directly, bypassing `midi/12` reverse-engineering |
| Leap detection from octave marks only | Lilypond rule guarantees no leap > 4th without `^`/`/`. `leapFromShift(OctaveShift)` replaces complex interval computation |
| File watching via `tea.Every` restart loop | BubbleTea's `tea.Every` fires once; file polling requires returning a non-nil message from the callback and restarting the timer from `Update` |
| TUI subscript toggle (`o` key) | Subscripts default off in TUI; `showSubscripts` threaded through `render.Render → BuildCells → octaveSubscript` |
| Transport proxy for all playback | All play/pause/stop/seek/volume calls route through `m.transport` — a single proxy that can switch between MIDIPlayer and Recording without changing callers |
| SMF regeneration for option changes | Metronome/roots/backbeats toggles regenerate the SMF (sub-ms) rather than using per-channel volume — SMF files are <10KB |
| Lyric column from token list, not events | `buildLyricCells` iterates `m.Lyrics` directly (not `m.Events`) because pipeline may merge sustains into fewer events than lyric positions |
| ChordRoot uses normalizeRoot | `theory.ChordRoot(raw)` delegates to the existing `normalizeRoot` (which handles `&`/`#` accidentals), just adds the `-`/`;` guard |

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

- **Safari Web Audio is unreliable for recording.** Safari's `AudioContext` + `MediaRecorder` combination produces timing glitches during live recording capture. The look-ahead scheduler ([plan](plans/look-ahead-web-audio-scheduler.md)) fixes non-recording playback in Safari but cannot overcome low-level contention between audio node scheduling and `MediaRecorder` capture. **Chrome is the recommended browser for the web TUI.**
- Single staff (piano), maximum 4 voices per chord
- No nested chords inside voice-poly groups
- Voice-poly tuplet combinations not yet supported
- Barline (invisible midpoint) split covers 4/4 midpoint only — odd time sigs may need adjustment
- Beaming may be incomplete for multi-voice measures (same-voice notes at non-adjacent sorted positions)
- Render uses beat-group index grouping (GroupIdx on Event), not tick positions
- Cross-measure sustain for voice-poly is fragile: `priorEvents[1]` hard-coded in legacy sustain path (pipeline.go lines 68, 152). When a voice-poly chord follows a traditional chord (e.g. `(c d e) | (- - g)`), the individual pitches of the traditional chord must be findable through the voice 1 prior event. Sustain-after-rest semantics (e.g. `(c ; e) | (- ; g)`) require nil-sentinel handling — a rest establishes the voice but doesn't provide a pitch to extend.
- `encoding/xml` produces `<chord>true</chord>` instead of spec-conformant `<chord/>`. Most renderers accept this.
- Render accidentals follow measure-level persistence (an accidental on a letter affects subsequent same-letter notes), which matches Engraving Rules but differs from some DAW conventions that reset per beat.
- Sustains across measure boundaries use the relative-octave pitch of the source note, not the absolute octave from `^`/`/`. `OctaveShift` is intentionally zeroed on sustain events.
- TUI line truncation (`view.go`) is ANSI-aware and rune-safe; the old byte-based slicing could bisect ANSI sequences and garble the display.
- `buildLyricCells` previously skipped `EventRest` and `Split` events, causing lyric tokens for rests/sustains to be silently dropped. Now iterates `m.Lyrics` directly and renders `;` tokens in grey (matching rest/sustain style in note column).
