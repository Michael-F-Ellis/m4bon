# Leap Indicators — Implementation Plan

## Goal

Make melodic leaps instantly visible in the TUI/render output. Upward leaps
(> perfect 4th) get a circumflex above the letter; downward leaps get a macron
below. Falls back to ANSI overline/underline when requested.

## Leap Detection Rules

- **Interval**: diatonic step count (c→f = 4th always, ignoring accidentals).
- **Threshold**: interval > 4 (i.e. ≥ 5th). A perfect 4th does NOT trigger.
- **Across measures**: the last pitch of measure N carries to the first pitch
  (or first chord bass note) of measure N+1.
- **Within chords**: pitches are ascending. Each successive pitch checks
  against the previous chord tone. The first chord tone checks against the
  prior event's pitch (or prior chord's bass/lowest pitch).
- **Rest/sustain**: do not break the pitch chain — the last sounded pitch is
  used. A rest after a rest has no reference pitch (skip).
- **Voice-poly chords**: per-voice leap tracking. Each voice maintains its
  own last pitch. Voiced sustains (-) and rests (;) preserve the chain per
  voice.
- **OctaveShift**: contributes to leap calculation? No — leaps are purely
  diatonic step count, not semitone distance. `^` and `/` don't affect
  diatonic interval class.

## Data Flow

```
Event stream → buildMeasureCells (compute leaps) → Cell{Leap: Up/Down/None}
             → FormatANSI (apply U+0302/U+0331 or ANSI escapes)
```

## File Changes

### 1. `render/cell.go` — Add leap direction to Cell

```go
type LeapDir int
const (
    LeapNone  LeapDir = iota
    LeapUp            // upward leap > 4th → circumflex
    LeapDown          // downward leap > 4th → macron below
)

type Cell struct {
    Content   string
    Style     StyleClass
    Italic    bool
    Subscript string
    Leap      LeapDir  // NEW
}
```

### 2. `render/render.go` — Compute leaps

New function `computeLeaps(measures []MeasureResult)` annotates events with
leap info before `buildMeasureCells` runs.

Algorithm:
- Track `lastPitch map[int]int` keyed by voice (default voice 1 for
  non-polyphonic). `int` is the diatonic step index (c=0, d=1, ... b=6).
  Special sentinel for "no prior pitch" (-1).
- Walk all events across all measures in order.
- For EventNote: compute step distance from `lastPitch[voice]`. Set leap
  direction on the event (need a field or a side table). Update
  `lastPitch[voice]`.
- For EventChord: first pitch checks against `lastPitch[voice]`; each
  successive pitch checks against the previous pitch in the chord. Update
  `lastPitch[voice]` to the top pitch.
- For EventRest/Sustain: update `lastPitch[voice]`? No — they preserve the
  chain but don't add new pitches.
- Skip Split events (notational ties).
- Use a `map[int]*leapInfo` keyed by event index, or add a transient field.

**Decision**: Add a `Leap` field to `parser.Event`? No — that pollutes the
parser package with render concerns. Instead, `buildMeasureCells` receives a
pre-computed `map[eventRef]*LeapDir`. An `eventRef` could be `(measureIdx,
eventIdx)` or a pointer. Since events are passed by value in the measure
slice, use `(mi, ei)` tuple.

Actually simpler: pre-compute leaps into a `[]LeapDir` parallel to the flat
event list, then `eventToCells` receives the leap direction as a parameter.

### 3. `render/ansi.go` — Apply diacritics or ANSI escapes

New function signature:

```go
func FormatANSI(measures []CellSeq, asciiLeaps bool) string
```

When `asciiLeaps` is false:
- `LeapUp` → append U+0302 (combining circumflex) after Content
- `LeapDown` → append U+0331 (combining macron below) after Content

When `asciiLeaps` is true:
- `LeapUp` → wrap in SGR 53 (overline)
- `LeapDown` → wrap in SGR 4 (underline)

Both combine with existing color/italic escapes. Diacritics are zero-width
combining characters so they don't affect `visibleLen` or layout.

Update `FormatPlain` to accept parameter or strip diacritics. Plain output
should be plain text — strip the combining characters.

### 4. `render/render.go` — Update Render entry point

```go
func Render(measures []parser.MeasureResult, asciiLeaps bool) string {
    cellMeasures := BuildCells(measures)
    return FormatANSI(cellMeasures, asciiLeaps)
}
```

Also need `BuildCells` to accept leap info.

### 5. `BuildCells` — Thread leap info through

Signature change:

```go
func BuildCells(measures []parser.MeasureResult, asciiLeaps bool) []CellSeq
```

Actually `BuildCells` should just produce cells with Leap set, then
`FormatANSI` decides how to render them. So `BuildCells` doesn't need
`asciiLeaps` — only `FormatANSI` does.

So the chain is:

```
computeLeaps(measures) → leapTable (map[(mi,ei)]LeapDir)
BuildCells(measures, leapTable) → []CellSeq (with Leap set)
FormatANSI(cellSeq, asciiLeaps) → string
```

### 6. `cmd/m4bon/main.go` — Add `-ascii-leaps` flag

```go
asciiLeaps := flag.Bool("ascii-leaps", false, "Use ANSI escapes for leap indicators instead of Unicode diacritics")
```

Pass to `render.Render(result.Measures, *asciiLeaps)`.

### 7. `m4bon.go` — Update public API

```go
func Render(dsl string) (string, error)  // default: unicode diacritics
func RenderWithOptions(dsl string, asciiLeaps bool) (string, error)  // NEW
```

Or just add parameter to existing `Render`. Breaking change but minor.
Better: keep `Render(dsl)` for backward compat (uses unicode) and add
`RenderOptions`.

### 8. `cmd/m4bon/tui/model.go` — Pass flag through

Currently calls `render.Render(measures)`. The TUI should also support
`-ascii-leaps`. Add `asciiLeaps` field to `model`, set from `Run()`, pass to
`render.Render`.

### 9. Tests

#### `render/render_test.go`
- Test upward leap: `M4/4 c a` → `a` cell has LeapUp, content includes U+0302
- Test downward leap: `M4/4 a c` → `c` cell has LeapDown
- Test no leap on 4th: `M4/4 c f` → no leap
- Test across measures: `M4/4 c d | a b` → `a` has LeapUp
- Test chord leaps: `(cge)f` → `g` and `e` have LeapUp (5th and 6th from c),
  also check intra-chord leaps (c→g = 5th, g→e = 6th... wait that's
  descending but chords are ascending). Let me reconsider:
  - `(cge)` — c→g is 5th (up), g→e is 6th... but chords are ascending by
    pitch. c4, g4, e5? No, chords are strict ascending: c, e, g. So c→e = 3rd
    (no leap), e→g = 3rd (no leap). Need `(cg)` for a leap within chord.
  - `(c g)` (voice-poly) — voice 1: c, voice 2: g. Per-voice, no internal
    leaps. But each voice's first pitch checks against its prior pitch.
- Test ascii fallback: `BuildCells` returns cells with LeapUp, `FormatANSI`
  with ascii=true produces SGR 53/4 escapes
- Test plain format strips diacritics

---

## Step Order

1. Add `LeapDir` and `Leap` field to `Cell` (`render/cell.go`)
2. Write `computeLeaps` function in `render/render.go`
3. Update `BuildCells` and `eventToCells` to accept and use leap table
4. Update `FormatANSI` and `FormatPlain` for leap rendering
5. Update `Render` entry point
6. Add `-ascii-leaps` flag to CLI
7. Update `m4bon.go` public API
8. Update TUI `model.go` and `main.go`
9. Write tests
10. Run full test suite
