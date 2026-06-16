# Multi-Measure Barlines — Implementation Plan

**Date:** 2026-06-13
**Status:** Approved

## Objective

Replace the current single-measure output with proper barline-aware multi-measure support. Measures are separated by `|` in the DSL. Pickups, per-measure directives (`B`, `M`, `K`), and error reporting are included.

## DSL Syntax

```
# Explicit meter (validation)
M4/4 c d e f | a b c d |

# Beat-duration directive (auto-sizing)
BQ. a b | c d e | BQ f g a |

# Mixed directives per measure
KE& M3/4 e& f g | M6/8 c d e f g a |
```

| Directive | Meaning | Examples |
|-----------|---------|----------|
| `M<N>/<D>` | Explicit meter (validates content) | `M4/4`, `M6/8`, `M3/4` |
| `B<code>` | Beat duration (auto-derives meter from content) | `BQ` (quarter), `BQ.` (dotted quarter), `BE` (eighth), `BE.` (dotted eighth) |
| `K<key>` | Key signature | `KC`, `KE&`, `KF#` |
| `<n>` | Repeat directive (beat multiplier) | `2abc` = 2 beats of triplet eighths |

When neither `M` nor `B` is given, defaults to `BQ` (quarter-note beat = 4/4).

## Phase 1 — Measure splitting with `|`

### 1a — Tokenizer: `|` becomes a measure separator

`ParseDSL` splits the token stream into measure groups at `|` boundaries. Each measure group is a `[]string` of beat-group tokens.

Structure:
```
type MeasureGroup struct {
    Tokens   []string      // beat-group tokens (e.g. "c", "ab", "2def")
    Beat     BeatDuration  // effective beat for this measure
    TimeNum  int           // derived/validated time sig (0 = auto)
    TimeDen  int
    Fifths   int           // key sig
}
```

### 1b — Process each measure independently

For each `MeasureGroup`:
1. Parse tokens via existing `parseGroup()`
2. Run `resolveDurations()` with the measure's beat duration
3. Run `splitAtBarline()` (no-op within a measure — already handled)
4. Run `splitNonStandardDurations()`
5. Collect into per-measure event lists

### 1c — Pickup detection

After resolving durations:

```
if first measure exists AND
   measure has no `M` or `B` directive AND
   its total tick count < measure capacity:
   → mark as pickup (MusicXML: implicit="yes")
   → measure number = 0 (not emitted as "1" in MusicXML)
   → first real measure starts at next `|`
```

Detection is always enabled. A pickup with explicit `M` directive (e.g. `M4/4 c`) means the user deliberately wrote a short first measure — also emit as implicit.

### 1d — Per-measure accidental reset

Move `pitchStates` map from generator-level scope to per-measure scope. Each measure starts with a fresh map.

### 1e — Per-measure beam reset

Move beam tick counter from global to per-measure scope. Beams start fresh at each `|`.

### 1f — Measure attributes

Only emit `<attributes>` (key, time, clef) when they first appear or change. Omit on subsequent measures when unchanged (Verovio/notation software handle line-start convention automatically).

## Phase 2 — `B` directive (beat-duration)

### 2a — Parse `B` syntax

`B` accepts SHORTHAND duration codes:

| Code | Beat |
|------|------|
| `BW` | whole (4/4 implied) |
| `BH` | half (2/2 implied) |
| `BQ` | quarter (default, 4/4) |
| `BQ.` | dotted quarter (compound, 6/8/9/8/12/8) |
| `BE` | eighth |
| `BE.` | dotted eighth |
| `BS` | 16th |
| `BT` | 32nd |

Parse in `stripDirectives()` — check for `B` following same pattern as `K`/`M`.

Beat resolution table:

| Code | Fraction of whole note |
|------|----------------------|
| `BW` | 1/1 |
| `BH` | 1/2 |
| `BQ` | 1/4 |
| `BQ.` | 3/8 |
| `BE` | 1/8 |
| `BE.` | 3/16 |
| `BS` | 1/16 |
| `BT` | 1/32 |

### 2b — Derive time signature from content

For each measure with a `B` directive:
1. Count the number of beat-group tokens (after stripping directives)
2. Compute: `total = numBeats * beat.Num / beat.Den` (fraction of whole note)
3. Simplify to `num/den` — that's the time signature

Examples:

| Directive | Beats | Total | Time sig |
|-----------|-------|-------|----------|
| `BQ.` | 2 | 6/8 | 6/8 |
| `BQ.` | 3 | 9/8 | 9/8 |
| `BQ.` | 4 | 12/8 | 12/8 |
| `BQ` | 4 | 4/4 | 4/4 |
| `BQ` | 3 | 3/4 | 3/4 |
| `BH` | 4 | 4/2 | 4/2 |
| `BE` | 6 | 6/8 | 6/8 |

### 2c — Emit `<time>` only where it changes

Track the current time signature across measures. Emit `<time>` in measure 1 and any measure where `timeNum/timeDen` differs from the previous measure. Same for `<fifths>` and `<clef>` (clef changes deferred but architecture supports it).

## Phase 3 — Error handling

### 3a — Validation in `M` mode

For each measure with an explicit `M` directive:

1. Compute total tick count from resolved events
2. Compute expected tick count = measure capacity
3. If mismatch: append to error accumulator

Error message format:
```
Measure 3: expected 4/4 (1920 ticks), got 5/4 (2400 ticks)
  Input: "M4/4 c d e f g"
  Suggestion: check beat grouping or remove "g"
```

### 3b — Error accumulation

- Collect errors in a slice
- Stop collecting after 10 errors (don't flood)
- Print all collected errors to stderr after parsing completes
- Continue generating MusicXML from whatever we have (lenient)

### 3c — Error test cases

Add `.dsl` files for expected-error cases:
- `test/cases/error-overfill.dsl` — too many beats
- `test/cases/error-underfill.dsl` — too few beats
- `test/cases/error-no-sustain-pitch.dsl` — bare `-` with no prior note

Test runner checks: `ParseDSL()` returns non-nil `Err`, and error message contains measure number.

## Changes by file

| File | Change |
|------|--------|
| `parser/parse.go` | Add `BeatDuration`, `MeasureGroup`, `ParseDirectives` types |
| `parser/pipeline.go` | `ParseDSL`: split tokens at `\|`, process per measure, pickup detection, error accumulation |
| `parser/pipeline.go` | Add `parseBDirective()`, `deriveTimeSig()` helpers |
| `parser/pipeline.go` | `stripDirectives()`: also strip `B` directives |
| `musicxml/xml.go` | `Generate()`: accept per-measure events + metadata, reset state per measure |
| `musicxml/xml.go` | Conditional `<attributes>` emission (only on change) |
| `main.go` | No change needed (already delegates to parser + generator) |
| `test/cases/*.dsl` | Add multi-measure golden test cases + error cases |

## Out of scope

- `<barline>` elements (repeat signs, endings) — just measure separation
- Multiple staves/voices — still single staff
- Mid-measure clef or key changes — whole-measure granularity
- Grace notes, ornaments
- Explicit `<attributes>` after pickup (Verovio handles line-start convention)
