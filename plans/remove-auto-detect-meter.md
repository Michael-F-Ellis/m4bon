# Remove Meter Auto-Detection and B Directive

**Date:** 2026-07-04
**Status:** Draft

## Objective

Make the parser more robust by:

1. **Removing auto-detection** of time signatures from note content (lines 1072-1081 in `pipeline.go`)
2. **Removing the 'B' directive** for explicit beat duration override (`scanMeasureDirectives`)
3. **Requiring explicit meter** on the first line of multi-measure input (error if missing)
4. **Adding unconditional meter validation** — every measure's total ticks must match its time signature (with pickup exception)

## What Changed Today

A one-line fix was applied to `pipeline.go:1073` changing the auto-detection guard from `!(mi == 0 && hasInitialMeter)` to `mi > 0`. This prevented auto-detection from running on the first line (fixing the pickup bug), but auto-detection on subsequent lines was left intact. This plan supersedes that fix — it removes auto-detection entirely and replaces it with strict validation.

## Why

Auto-detection caused two distinct bugs:

1. **2026-06-18 session:** Auto-detection interacted badly with pickup measures in multi-measure input, producing incorrect time signatures and corrupt MIDI timing.
2. **2026-07-04 (today):** A pickup `^g` (480 ticks) in default 4/4 (1920 expected) triggered auto-detection on the first line, deriving time sig 1/4. This cascaded: subsequent measures got 1/1, 4/1 — making each beat a whole note (1920 ticks instead of 480), producing an 8-second-per-measure tempo slowdown.

The 'B' directive has zero real-world usage (0 examples, 2 test files) and duplicates functionality already covered by `M...` directives.

## Changes Required

### 1. Remove auto-detection block (`pipeline.go`)

**Location:** Lines 1072-1081

**Current code:**
```go
// Auto-detect time sig from content when no explicit directive.
// Skip on the first line (mi==0) to avoid misinterpreting pickups
// as time signature changes — the initial default (4/4) is preserved,
// and subsequent lines auto-detect correctly.
if !md.explicitMeter && !md.hasBeatCode && hasMultipleLines && mi > 0 {
    actualTicks := totalTicks(events)
    expectedTicks := timeSigTicks(effectiveTimeNum, effectiveTimeDen)
    if actualTicks != expectedTicks && actualTicks > 0 {
        g := frac.GCD(actualTicks, frac.TicksPerWholeNote)
        effectiveTimeNum = actualTicks / g
        effectiveTimeDen = frac.TicksPerWholeNote / g
    }
}
```

**Action:** Delete the entire if-block (lines 1072-1081).

### 2. Remove B directive from `scanMeasureDirectives` (`pipeline.go`)

**Location:** Lines 686-738 (approx — the `scanMeasureDirectives` function)

**Changes:**

a. Remove `hasBeatCode` field from `measureDirectives` struct (line 681).

b. In `scanMeasureDirectives`, remove the B-directive parsing. This is the block that checks tokens for `B` prefix and looks up `BeatDurationCodes`. The relevant code is around lines 718-719:
```go
if bd, ok := BeatDurationCodes[bc]; ok {
    md.hasBeatCode = true
```
And the meter derivation from beat code at lines 732-733:
```go
if md.hasBeatCode && !md.explicitMeter {
```

c. Remove the beat-code fallback in the main loop at lines 1042-1044:
```go
// Resolve beat if no B directive
if !md.hasBeatCode {
    md.beat = ResolveBeatDuration(effectiveTimeNum, effectiveTimeDen)
}
```
Change to unconditional:
```go
md.beat = ResolveBeatDuration(effectiveTimeNum, effectiveTimeDen)
```

### 3. Remove `BeatDurationCodes` map (`parse.go`)

**Location:** Lines 223-233

**Action:** Delete the entire `BeatDurationCodes` map and its comment.

### 4. Remove `deriveTimeSig` function (if only used by B directive) (`pipeline.go`)

**Location:** Lines 650-671 (approx)

Check if `deriveTimeSig` is used by anything other than the B directive code. If not, remove it. If it's dead code, the compiler won't complain (Go allows unused functions), but it should be cleaned up.

**Action:** Search for calls to `deriveTimeSig` — if none remain, remove the function.

### 5. Remove `BeatDuration` type (if only used by B directive) (`pipeline.go`)

**Location:** Lines 144-148

`BeatDuration` struct is also used by `ResolveBeatDuration` which IS used by the normal meter path. So KEEP `BeatDuration` and `ResolveBeatDuration`. Only remove `BeatDurationCodes`.

### 6. Enable unconditional meter validation (`pipeline.go`)

**Location:** Lines 850-871 (`validateExplicitMeter`)

**Current code:**
```go
func validateExplicitMeter(events []Event, timeNum, timeDen int, tokens []Token, measureIdx int, hasSecondMeasure, hasExplicitMeter, isFirstMeasure bool) string {
    if !hasExplicitMeter {
        return ""
    }
```

**Change:** Remove the `hasExplicitMeter` parameter and the early return guard. Validation should run for EVERY measure:

```go
func validateExplicitMeter(events []Event, timeNum, timeDen int, tokens []Token, measureIdx int, hasSecondMeasure, isFirstMeasure bool) string {
```

Remove the `hasExplicitMeter` guard (lines 851-853). The rest of the function stays — it already correctly handles pickups (line 860: `isFirstMeasure && hasSecondMeasure && actualTicks < expectedTicks`).

Also update the caller at line 1088:
```go
// Before:
if errStr := validateExplicitMeter(events, effectiveTimeNum, effectiveTimeDen, tokens, mi, hasSecondMeasure, hasExplicitMeter, mi == 0); errStr != "" {
// After:
if errStr := validateExplicitMeter(events, effectiveTimeNum, effectiveTimeDen, tokens, mi, hasSecondMeasure, mi == 0); errStr != "" {
```

The `hasExplicitMeter` variable at line 1087 can also be removed if it's no longer used elsewhere (check — it was only used for this call).

### 7. Require explicit meter on first line (`pipeline.go`)

**Location:** Around line 1039 (where effectiveTimeNum/TimeDen get defaults)

After removing auto-detection, when the first line has no `M...` directive, the parser currently defaults to 4/4. Instead, we should produce an error.

**Approach:** After `scanMeasureDirectives`, check if the first measure (mi == 0) has no explicit meter AND there are multiple lines. If so, add an error:

```go
if mi == 0 && !md.explicitMeter && hasMultipleLines {
    errs = append(errs, "Measure 1: no meter signature (M... directive required for multi-measure input)")
    // Continue processing with default 4/4 so other errors can be reported too
    // but the final result will contain this error.
}
```

The validation already requires an explicit meter for `hasExplicitMeter` to work. But after this change, `hasExplicitMeter` will always be derived from explicit directives only (auto-detection is gone, B directive is gone).

### 8. Remove `auto-detect` condition variables (`pipeline.go`)

**Location:** Lines 1017

The `hasInitialMeter` variable was used for:
1. Auto-detection guard (now removed)
2. `hasExplicitMeter` calculation (line 1087)

After the changes above, `hasInitialMeter` is no longer needed. Remove it.

The `hasExplicitMeter` calculation at line 1087 becomes:
```go
hasExplicitMeter := md.explicitMeter
```
(Or just inline `md.explicitMeter` at the call site and remove the variable.)

### 9. Update test files

#### 9a. `test/cases/beat-directive.dsl`

**Current:**
```
# beat-directive — BQ. gives 6/8 time signature
BQ. a b c
```

**Change:** Replace B directive with explicit meter:
```
# beat-directive — M6/8 gives 6/8 time signature
M6/8 a b c
```

Update the corresponding `.expected.mxml` file if the meter affects MusicXML output (key/time sig in `<attributes>`).

#### 9b. `test/cases/beat-change-tie.dsl`

Read this file to understand the full context. It likely has a measure with `BQ.` that needs to become `M6/8`.

#### 9c. Check for any test Go files that reference `BeatDurationCodes`

Search `*_test.go` for `BeatDurationCodes`, `hasBeatCode`, `BQ`, `BQ.`, etc. and update accordingly.

### 10. Update AGENTS.md

In the DSL Reference section, remove documentation for the `B` directive and add a note that `M...` is required on the first line of multi-measure input.

Also update the `scanMeasureDirectives` doc comment if it mentions B directive.

### 11. Update golden test files

After changing the DSL test files, run:
```bash
make golden
```
This regenerates the `.expected.mxml` files.

### 12. Full test run

```bash
make all          # builds binary + WASM + runs tests
make check        # builds + tests + vet
```

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Single-measure input `c d e f` | No meter required. Default 4/4 applies. No validation. Same as today. |
| Multi-line with pickup, explicit meter | `M4/4 ^g` on line 1, then full measures. Validation allows pickup (shorter, first measure, has second measure). |
| Multi-line without meter on line 1 | Error: "Measure 1: no meter signature (M... directive required for multi-measure input)" |
| `BQ. a b c` (old B directive) | Parser won't recognize `BQ.` — treated as unknown token. No error for unknown tokens currently, but the measure will parse wrong. |
| `M3/4 a b` (3 beats in 3/4) | Validation: expected 1440 ticks, actual 1440 ticks. Passes. |
| `M4/4 a b` (2 beats in 4/4) | Validation: expected 1920 ticks, actual 960 ticks. Error: "Measure 1: expected 4/4 (1920 ticks), got 960 ticks" |
| `M4/4 abcdefgh` (8 eighth notes in 4/4) | 8 slots, 480 ticks each? No — 8 slots in one group = 8 subdivision = 480/8 = 60 ticks each. Total = 480. Validation fails. |

## Not Changing

- **`ResolveBeatDuration`** — still needed for computing beat from time sig
- **`BeatDuration` struct** — still used by `ResolveBeatDuration`
- **`detectPickup`** — still needed for pickup flag on render
- **`totalTicks`** — still used by validation
- **`timeSigTicks`** — still used by validation and `detectPickup`
- **Default time sig (4/4)** for single-measure input on the Go side
- **MIDI generation** (`midi/generate.go`) — unchanged, already reads `TimeNum/TimeDen` from `MeasureResult`

## Dependencies

1. Must be done in order: parser changes first, then test updates, then golden regeneration, then WASM rebuild.
2. The `BeatDurationCodes` export (`parse.go:224`) is public (`var BeatDurationCodes`). Check if any external packages reference it (unlikely — single module, but verify with `grep`).

## Verification Checklist

- [ ] `make test` passes
- [ ] `make check` (go vet) passes
- [ ] Multi-line input without `M...` produces clear error
- [ ] Multi-line input with `M4/4 ^g` pickup validates correctly
- [ ] `M4/4 c d e f` (single line) works as before
- [ ] Old `BQ.` test files updated and pass
- [ ] `BeatDurationCodes` removed from codebase
- [ ] `hasInitialMeter` removed from codebase
- [ ] `deriveTimeSig` removed if unused
- [ ] `AGENTS.md` updated
- [ ] WASM rebuilt (`make wasm`)
- [ ] Web app works with test input
