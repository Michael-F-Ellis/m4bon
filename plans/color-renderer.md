# Color Renderer — Implementation Plan

**Date:** 2026-06-17
**Status:** Implemented

## Objective

Add a `-render` flag to the m4bon CLI that outputs a human-readable, colorized, one-measure-per-line score format instead of MusicXML. The output mimics the DSL structure but uses color to highlight altered pitches, supports octave subscripts, overlines for chord tones, and measure numbers — designed for both terminal and future web rendering.

## Architecture: Two-Layer Design

```
Event pipeline ([]MeasureResult)
        │
        ▼
┌─────────────────────────────┐
│      Core Renderer          │  ← render/render.go
│  Walk events → produce      │
│  []Cell (IR) per measure    │
└─────────────────────────────┘
        │
        ▼
┌─────────────────────────────┐
│      ANSI Formatter         │  ← render/ansi.go
│  Cells → ANSI-escaped text  │
└─────────────────────────────┘

Future:
┌─────────────────────────────┐
│      HTML Formatter         │  ← render/html.go (not in this plan)
│  Cells → <span>-based HTML  │
└─────────────────────────────┘
```

The core renderer knows nothing about terminals or HTML. It produces an intermediate representation: sequences of `Cell` values, each describing a single display glyph (pitch letter, `-`, `;`, space, punctuation) with associated styles (color class, overline flag, subscript text). Formatters convert this IR to concrete output.

## Types: `render/cell.go`

```go
// StyleClass identifies an ANSI color / CSS class for a glyph.
type StyleClass int

const (
    StyleDefault  StyleClass = iota // no color (natural pitch)
    StyleSharp                      // red    — rgb(209, 34, 34)
    StyleFlat                       // blue   — rgb(152, 140, 254)
    StyleDoubleSharp                // orange — rgb(255, 165, 0)
    StyleDoubleFlat                 // green  — rgb(4, 182, 4)
    StyleSustainRest                // grey   — rgb(160, 160, 160)
)

// Cell describes a single glyph to render.
type Cell struct {
    Content   string      // the character(s) to display (e.g. "c", "-", ";")
    Style     StyleClass  // color/style classification
    Overline  bool        // apply overline (U+0305) to this glyph
    Subscript string      // octave subscript text, empty if none (e.g. "4")
}

// CellSeq is a sequence of cells for one measure.
type CellSeq []Cell
```

## Core Renderer: `render/render.go`

### Entry point

```go
// Render produces the colorized text output for a sequence of measures.
// measures: output of parser.ParseDSL()
// Returns one line per measure, with ANSI escape codes for colors.
func Render(measures []parser.MeasureResult) string
```

### Algorithm (`buildCells`)

For each `MeasureResult`:

1. **Build key signature accidental map** from `Fifths`:
   - `fifths > 0`: first N of `[f, c, g, d, a, e, b]` are sharp
   - `fifths < 0`: first |N| of `[b, e, a, d, g, c, f]` are flat
   - Store as `map[letter]int` (1 for sharp, -1 for flat)

2. **Walk events in order, accumulate tick position**:
   - Skip `EventTupletStart` markers
   - Group consecutive events at the same tick position (same onset)
   - Track whether we've seen the first note/chord in the measure for octave subscript rule

3. **For each group of events (same tick)**:
   - Emit cells for each event in the group
   - Separate event groups with a space cell

4. **Per-event rendering**:

   | Event type | Split? | Render as |
   |---|---|---|
   | `EventNote` | `false` | Pitch letter, colored by effective accidental, subscript if needed |
   | `EventNote` | `true` | `-` in grey (tie continuation) |
   | `EventRest` | — | `;` in grey |
   | `EventChord` | `false` | Each pitch letter with overline, colored, subscript on first pitch if needed |
   | `EventChord` | `true` | `-` in grey (tie continuation) |

5. **Effective accidental determination**:
   ```
   func effectiveAccidental(letter string, explicitAcc int, fifths int) int
   ```
   - If `explicitAcc != 0`: return explicitAcc (overrides key sig)
   - If key sig dictates alteration for this letter: return key sig's accidental
   - Otherwise: return 0 (natural)

   Color mapping:

   | Effective acc | StyleClass |
   |---|---|
   | 1 | `StyleSharp` — red |
   | -1 | `StyleFlat` — blue |
   | 2 | `StyleDoubleSharp` — orange |
   | -2 | `StyleDoubleFlat` — green |
   | 0 | `StyleDefault` — no color |

6. **Octave subscript rule**:
   - Octave number = `event.Midi / 12` (0–10, displayed as 0–8 for human range)
   - **First pitch/chord in measure**: always show subscript
   - **Subsequent pitches**: show subscript only if `event.OctaveShift != 0`
   - **Chord pitches**: subscript on first pitch of chord only, same rules
   - Tuplet continuations don't get subscripts even if `OctaveShift != 0` (they inherit from parent)

7. **Measure prefix**: `N:  ` where N is measure number (1-based, tacking pickup as 0).

### Example walkthrough

Input: `M4/4 #f g a b`

Events: `#f` (Midi=66, Acc=1), `g` (Midi=67, Acc=0), `a` (Midi=69, Acc=0), `b` (Midi=71, Acc=0)

- Key sig: C major (fifths=0), no key-altered letters
- Group at tick 0: `#f` — effectiveAcc = 1 → StyleSharp, first pitch → subscript `5` (66/12=5), cell: `f` (red, sub=5)
  Wait — octave = Midi/12. `#f` = F#4? Let me check: F#4 = MIDI 66. 66/12 = 5.5, truncated to 5. But octave 4 would be `midi/12 - 1`. Hmm.

  Actually the user said "Octave is indicated by subscripts, 1-8." Let me use the MusicXML convention: `octave = midi/12 - 1`. MIDI 66 → 66/12 - 1 = 5 - 1 = 4. So `f₄` in subscript.

  Wait, but MIDI 60 = C4. In standard scientific pitch notation, C4 is octave 4. MIDI 60/12 = 5. 5 - 1 = 4. Yes, `midi/12 - 1` gives the correct octave number.

  So octave = `event.Midi / 12 - 1`. For MIDI 66: 66/12 - 1 = 5 - 1 = 4. Show `⁴`.

  But wait, for C4 (MIDI 60): 60/12 - 1 = 5 - 1 = 4. C4 → 4. Correct.
  For C5 (MIDI 72): 72/12 - 1 = 6 - 1 = 5. C5 → 5. Correct.

  So `midi/12 - 1` gives standard octave numbering.

- Group at tick 240: `g` — effectiveAcc = 0 → StyleDefault, not first pitch, OctaveShift=0 → no subscript. Cell: `g` (default color)
- Group at tick 480: `a` — default color, no subscript
- Group at tick 720: `b` — default color, no subscript

Result: `1:  f₄ g a b` (with `f` in red)

Input: `KE& M4/4 e& f g a` (E-flat major: Bb, Eb, Ab)

Events: `e&` (Midi=63, Acc=-1), `f` (Midi=65, Acc=0), `g` (Midi=67, Acc=0), `a` (Midi=69, Acc=0)

- Key sig: fifths=-3 (Eb major), altered letters: {b:-1, e:-1, a:-1}
- Group at tick 0: `e&` — explicit Acc=-1 → StyleFlat, first pitch → subscript `4` (63/12-1=4). Cell: `e` (blue, sub=4)
- Group at tick 240: `f` — Acc=0, key sig doesn't alter F → StyleDefault. No subscript. Cell: `f`
- Group at tick 480: `g` — Acc=0, key sig doesn't alter G → StyleDefault. Cell: `g`
- Group at tick 720: `a` — Acc=0, key sig alters A to flat → StyleFlat. Cell: `a` (blue)

Result: `1:  e₄ f g a` (with `e` and `a` in blue)

Input: `M4/4 a - -b c`

Events after pipeline: a (1/2, Midi=69, Split=f), a (1/8, Midi=69, Split=t), b (1/8, Midi=71, Split=f), c (1/4, Midi=72, Split=f)

- Key sig: C major
- Tick 0: a (Split=f) — StyleDefault, first pitch → sub `4` (69/12-1=4). Cell: `a` (sub=4)
- Tick 960: a (Split=t) → `-` grey. b (Split=f) → StyleDefault, OctaveShift=0 → no sub. Cell: `-` `b`
- Tick 1200: c (Split=f) → StyleDefault, OctaveShift=0 → no sub. Cell: `c`

Result: `1:  a₄ -b c` (with `-` in grey)

Input: `M4/4 (ace)f`

Events: chord (Midis=[57,60,64], Split=f), f (Midi=65, Split=f)

- Key sig: C major
- Tick 0: chord — first pitch gets sub `4` (57/12-1=4 → A3). Wait, 57/12-1 = 4.75-1 = 3. A3? Let me check: A3 = MIDI 57. Yes, A3. So sub `3`.
  - Pitch A (Midi 57): StyleDefault, first in chord, sub `3`
  - Pitch C (Midi 60): StyleDefault, overline
  - Pitch E (Midi 64): StyleDefault, overline
  Cells: `a` (overline, sub=3), `c` (overline), `e` (overline)
- Tick 240: f — StyleDefault, no sub. Cell: `f`

Result: `1:  a₃c̄ē f` (with overline on chord pitches)

Note: overline uses Unicode combining character U+0305. So `a` with overline = `"a\u0305"`.

**Updated to underline (U+0332) instead of overline — better visibility.**

## ANSI Formatter: `render/ansi.go`

```go
// FormatANSI converts a cell sequence per measure into an ANSI-escaped string.
// Each measure becomes one line: "<measure-num>:  <cells>\n"
func FormatANSI(measures [][]Cell) string
```

### ANSI Color Mapping

| StyleClass | ANSI escape | RGB |
|---|---|---|
| `StyleDefault` | none (no escape) | terminal default |
| `StyleSharp` | `\033[38;2;209;34;34m` | red |
| `StyleFlat` | `\033[38;2;152;140;254m` | lavender blue |
| `StyleDoubleSharp` | `\033[38;2;255;165;0m` | orange |
| `StyleDoubleFlat` | `\033[38;2;4;182;4m` | green |
| `StyleSustainRest` | `\033[38;2;160;160;160m` | grey |

Reset after each styled cell: `\033[0m`

### Overline rendering

Apply Unicode combining overline (U+0305) to the cell's Content string. In Go: `content + "\u0305"`.

### Subscript rendering

Unicode subscript digits:

| Digit | Unicode |
|---|---|
| 0 | U+2080 |
| 1 | U+2081 |
| 2 | U+2082 |
| 3 | U+2083 |
| 4 | U+2084 |
| 5 | U+2085 |
| 6 | U+2086 |
| 7 | U+2087 |
| 8 | U+2088 |
| 9 | U+2089 |

Append after the cell content (and after any overline combining char).

## CLI Changes: `cmd/m4bon/main.go`

Add a `-render` flag:

```go
render := flag.Bool("render", false, "Output colorized text format instead of MusicXML")
```

When `-render` is set:
1. Parse DSL as usual via `parser.ParseDSL()`
2. Call `render.Render(result.Measures)` instead of `musicxml.Generate()`
3. Print the rendered text to stdout (respecting `-o` flag for file output)

## Public API: `m4bon.go`

Add a `Render()` function:

```go
// Render parses m4bon DSL text and returns colorized text output
// in the FQS-inspired format: one measure per line with colored
// accidentals, octave subscripts, and chord overlines.
func Render(dsl string) (string, error)
```

## Test Plan

### Unit tests: `render/render_test.go`

| Test | Input | Checks |
|---|---|---|
| `TestBasicNotes` | `M4/4 c d e f` | 4 pitch letters, measure `1:`, octave subscript on `c` |
| `TestAccidentals` | `M4/4 #f &b %c` | `f` sharp color class, `b` flat color class, `c` default |
| `TestKeySignature` | `KE& M4/4 e f g a` | `e` flat color, `a` flat color, `f` default, `g` default |
| `TestSustainChain` | `M4/4 a - -b c` | Grey `-` characters, then `b` and `c` |
| `TestChord` | `M4/4 (ace)f` | 3 overline characters in first group, then `f` |
| `TestOctaveSubscript` | `M4/4 c^c` | Subscript on both `c` letters (second has OctaveShift=1) |
| `TestOctaveSubscriptFirstOnly` | `M4/4 c d e f` | Subscript only on first `c` |
| `TestDoubleAccidentals` | `M4/4 &&d ##c` | Double-flat green, double-sharp orange |
| `TestMultiMeasure` | `M4/4 c d e f \| a b c d` | Two lines, measure numbers 1 and 2 |
| `TestEmpty` | Empty DSL | Error returned |
| `TestNoColor` | `M4/4 c d e f` | No ANSI codes in cell output check (raw cells) |

### Golden file tests: `test/cases/*.render`

For each existing `.dsl` test case (that produces valid output), add a `.expected.render` golden file containing the expected ANSI-rendered output. A new `render_golden_test.go` in `cmd/m4bon/` runs `./m4bon -render -f <dsl>` and compares.

Use `-update-render` flag (analogous to `-update-golden`) to regenerate `.expected.render` files.

Skip error test cases and add minimal render-specific cases:

| File | DSL | Expected output |
|---|---|---|
| `test/cases/render-basic.dsl` | `M4/4 c d e f` | `1:  c₄ d e f\n` |
| `test/cases/render-accidentals.dsl` | `M4/4 #f &b %c` | `1:  f₄ b c\n` (with ANSI codes around `f` and `b`) |

## Implementation Order

1. **`render/cell.go`** — Types: `StyleClass`, `Cell`, `CellSeq`
2. **`render/render.go`** — Core renderer: `Render()`, `buildCells()`, effective accidental helper, key sig map builder
3. **`render/ansi.go`** — ANSI formatter: `FormatANSI()`, color map, subscript mapping
4. **`cmd/m4bon/main.go`** — Add `-render` flag, conditional output path
5. **`m4bon.go`** — Add public `Render()` API
6. **`render/render_test.go`** — Unit tests
7. **`cmd/m4bon/render_golden_test.go`** — Golden file tests
8. **`test/cases/render-*.dsl`** + `.expected.render` — Test fixtures
9. Build & verify all tests pass

## Out of Scope

- HTML formatter (future work after ANSI is validated)
- Beat count display (not needed per user direction)
- Per-beat spacing to match DSL groups exactly (uses tick-position grouping — close enough for v1)
- Multi-voice display (voice-poly chords rendered sequentially, not yet ideal)
- ANSI-free plain text mode (`StyleDefault` only)
- Configurable color palette
