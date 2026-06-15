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
├── main.go                 # CLI entry point
├── main_test.go            # Integration tests (CLI invocation)
├── go.mod / go.sum         # Go module
├── parser/
│   ├── parse.go            # Tokenizer + parseGroup state machine
│   ├── pipeline.go         # resolveDurations, split, octaves, ParseDSL
│   └── parse_test.go       # Unit tests for parser
├── musicxml/
│   └── xml.go              # MusicXML structs + generator
├── test/
│   ├── cycle.sh            # Deprecated — MuseScore automation experiment
│   ├── deploy.sh           # Deprecated — MuseScore plugin deploy
│   ├── fixtures/           # MuseScore test scores (used by deprecated scripts)
│   └── cases/              # .dsl test case files
├── m4bon.qml               # Original QML plugin (reference only)
├── m4bon-cli.qml           # Deprecated experiment: non-dialog plugin
├── m4bon-runner.qml        # Deprecated experiment: dialog runner
├── test-cli.qml            # Deprecated experiment: minimal non-dialog test
├── plans/
│   └── test-debug-cycle.md
├── issues/
│   └── no-ties.md
├── INSERTING-TIED_NOTES.md
├── TIES-VS-DOTS.md
├── DEBUG-UNDER-MACOS.md
├── MSCORE-CMDLINE-HELP
├── lessons/
│   └── session-2026-06-14.md
├── AGENTS.md
├── README.md
├── LICENSE                 # MIT
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

### Chord grouping `(...)`

Groups pitches into a single chord occupying one position slot. Strictly ascending.

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
         → resolveDurations → splitNonStandardDurations → resolveOctaves → MusicXML
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

Time and key signatures are specified in the DSL via K... and M... directives:
  m4bon "M4/4 c d e f"
  m4bon "KE& M6/8 abc def"

Examples:
  m4bon "c d e f"
  m4bon "M6/8 abc def"
  m4bon -f test/cases/basic-notes.dsl -o out.mxl
```

---

## Essential Commands

```bash
go build -o m4bon .       # Build (0.5s)
go test ./...              # Run all tests (0.4s)
./m4bon "c d e f"         # Quick test
./notify.sh "message"     # Send iMessage notification to maintainer
```

---

## Key Design Decisions

| Decision | Rationale |
|---|---|
| Go over Node.js | Single static binary, zero runtime deps, `encoding/xml`, excellent test facilities |
| MusicXML over MIDI | Human-readable, diff-able, ties/tuplets native, works with all notation apps |
| 480 DPPQ | Standard MIDI convention; cleanly divides all required durations |
| Greedy duration split | Simple, correct for basic cases; engraving-aware splitting deferred |
| No external XML libs | `encoding/xml` covers the subset we need |

---

## Known Limitations

- Single staff (piano), single voice
- All notes placed in measure 1 (no multi-measure layout yet)
- Greedy split doesn't respect engraving rules (cross-bar ties, beat boundaries)
- Key signature supported via `K` directive in DSL (default C major)
- `.qml` files are reference only — active development is in Go
