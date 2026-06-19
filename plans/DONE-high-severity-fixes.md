# Plan: Fix High-Severity Issues

## Implementation Order

The issues have a dependency chain: **1+2 → 3 → 4 → 5**.

- Issues 1+2 (Event struct cleanup) touch every consumer; do them first.
- Issue 3 (float math) is self-contained but touches code near issue 4's pipeline.
- Issue 4 (ParseDSL decomposition) is blocked on 1+2 because signatures change.
- Issue 5 (musicxml tests) can be written in parallel with 4, but final verification depends on 1-4 being stable.

---

## Issue 1+2: God Event Struct + Tuplet Field Abuse

**Files affected:** `parser/parse.go`, `parser/pipeline.go`, `musicxml/xml.go`, `render/render.go`, `midi/generate.go`, all test files, `cmd/m4bon/main.go` (indirectly)

### Target State

Replace the 19-field `Event` with a flat struct that has properly-typed fields for all event variants, plus typed constructor functions that validate field combinations. **Keep one struct** (not a type hierarchy) because:
- Events live in `[]Event` slices that get sorted, split, and merged — interfaces would add boxing overhead and make operations like `ev.Duration` require method calls.
- Most pipeline stages access common fields (Duration, Voice, Split, GroupIdx) on every event regardless of type.
- The existing code already uses `ev.Type` switches; the refactor just makes the field usage honest.

### Step 1.1: Add proper tuplet fields to Event

```go
type Event struct {
    Type            EventType
    Duration        Fraction
    Nominal         *Fraction // for tuplet notes: the nominal (display) duration
    Letter          string    // EventNote only
    Accidental      int       // EventNote only
    OctaveShift     int       // EventNote only
    ExplicitNatural bool      // EventNote only
    Pitches         []Pitch   // EventChord only
    Midi            int       // EventNote only; resolved MIDI pitch
    Midis           []int     // EventChord only; resolved MIDI pitches
    Split           bool      // continuation from splitNonStandardDurations or barline split
    TieNext         bool      // cross-measure tie to next measure's event
    Voice           int       // 1-based voice number; 0 = legacy default (maps to 1)
    GroupIdx        int       // original beat-group index, for render grouping
    // NEW — tuplet bracket info (EventTupletStart only)
    TupletActualNotes int
    TupletNormalNotes int
}
```

### Step 1.2: Update EventTupletStart code

**In `parser/pipeline.go` (resolveDurationsWithPrior):**

Before:
```go
events[len(events)-1].Midi = ratioNum
events[len(events)-1].OctaveShift = ratioDen
```

After:
```go
events[len(events)-1].TupletActualNotes = ratioNum
events[len(events)-1].TupletNormalNotes = ratioDen
```

**In `musicxml/xml.go` (Generate):**

Before:
```go
tupletRatioNum = ev.Midi
tupletRatioDen = ev.OctaveShift
```

After:
```go
tupletRatioNum = ev.TupletActualNotes
tupletRatioDen = ev.TupletNormalNotes
```

### Step 1.3: Add constructor functions (defensive)

In `parser/parse.go`:

```go
func NewNoteEvent(letter string, accidental, octaveShift int, explicitNatural bool, dur Fraction, nominal *Fraction, voice, groupIdx int) Event { ... }
func NewChordEvent(pitches []Pitch, dur Fraction, nominal *Fraction, voice, groupIdx int) Event { ... }
func NewRestEvent(dur Fraction, nominal *Fraction, voice, groupIdx int) Event { ... }
func NewTupletStartEvent(dur Fraction, actualNotes, normalNotes, groupIdx int) Event { ... }
```

These constructors ensure field consistency. Existing event construction sites in `pipeline.go` are updated to use them (this also makes the pipeline code more self-documenting).

### Step 1.4: Validation

Add a `func (e Event) Validate() error` that checks:
- `EventNote`: `Letter` is non-empty, `Midi` is 0..127, `Pitches` is nil, `Midis` is nil
- `EventChord`: `Pitches` is non-empty, `Letter` is empty, `Midi` is 0
- `EventRest`: `Letter` is empty, `Pitches` is nil, `Midi` is 0
- `EventTupletStart`: `Duration.Num > 0`
- All: `Voice >= 0`, `GroupIdx >= 0`

Call `Validate()` at the end of `ParseDSL` in test builds (use build tags or `testing.Testing()` to avoid production overhead).

### Step 1.5: Eliminate Voice 0

Take the opportunity to remove the Voice 0 → Voice 1 mapping scattered across the codebase. All events default to Voice 1. Remove the `if v == 0 { v = 1 }` checks in:
- `resolveDurationsWithPrior` (~4 sites)
- `ParseDSL` octave resolution loop (~2 sites)
- `totalTicks` (~1 site)
- `musicxml/xml.go` Generate (~1 site)

### Testing

All existing tests must pass unchanged. The golden tests verify identical MusicXML output. The render golden tests verify identical text output.

---

## Issue 3: Floating-Point in `splitNonStandardDurations`

**File affected:** `parser/pipeline.go`

### Target State

Replace `float64` comparison and subtraction with exact rational arithmetic using the `Fraction` type.

### Step 3.1: Extract duplicate arithmetic into helper methods

Add to `Fraction` (or nearby in pipeline.go):

```go
// lessThan returns a < b using cross-multiplication (no float).
func lessThan(a, b Fraction) bool {
    return a.Num*b.Den < b.Num*a.Den
}

// subtract returns (a - b) reduced to lowest terms. Assumes a >= b.
func subtract(a, b Fraction) Fraction {
    num := a.Num*b.Den - b.Num*a.Den
    den := a.Den * b.Den
    g := gcd(num, den)
    return Fraction{Num: num / g, Den: den / g}
}
```

### Step 3.2: Rewrite `splitNonStandardDurations`

Current (fragile):
```go
remains := float64(dur.Num) / float64(dur.Den)
for remains > 0.00001 {
    for _, sd := range standardDurations {
        sv := float64(sd.Num) / float64(sd.Den)
        if remains >= sv-0.00001 {
            ne := ev
            ne.Duration = sd
            ...
            remains -= sv
            break
        }
    }
}
```

New:
```go
remains := Fraction{Num: dur.Num, Den: dur.Den}
for remains.Num > 0 {
    for _, sd := range standardDurations {
        if !lessThan(remains, sd) { // remains >= sd
            ne := ev
            ne.Duration = sd
            ...
            remains = subtract(remains, sd)
            break
        }
    }
    // Safety: if no standard duration fits, break to avoid infinite loop
    // (This should never happen since we check isStandardDuration first)
    if remains.Num > 0 {
        // fallback: append remainder as-is
        break
    }
}
```

### Step 3.3: Remove unused float operations

`import "math"` may become removable from `pipeline.go` if `float64` was its only use. Check before removing.

### Testing

Existing tests that exercise non-standard durations (e.g., the sustain chain test `"a - -b c"`) cover this code path.

---

## Issue 4: Decompose `ParseDSL` Monolith

**File affected:** `parser/pipeline.go` (no new files needed — extract functions within the same file)

### Target State

`ParseDSL` becomes a high-level orchestrator (~40 lines) that calls named pipeline stages:

```go
func ParseDSL(text string) DSLResult {
    text, fifths, timeNum, timeDen, hasInitialMeter := stripDirectives(text)
    tokenGroups, hasBarline := splitMeasures(tokenize(text))
    if len(tokenGroups) == 0 { return ... }

    state := &pipelineState{
        currentFifths:   fifths,
        currentTimeNum:  timeNum,
        currentTimeDen:  timeDen,
        hasInitialMeter: hasInitialMeter,
        hasBarline:      hasBarline,
    }

    for _, tg := range tokenGroups {
        result, err := parseOneMeasure(tg, state)
        if err != nil { ... collect error ... }
        state.measures = append(state.measures, result.measure)
    }

    resolveOctaves(state.measures)
    return buildResult(state, fifths, timeNum, timeDen)
}
```

### Step 4.1: Extract `pipelineState` struct

```go
type pipelineState struct {
    currentFifths   int
    currentTimeNum  int
    currentTimeDen  int
    hasInitialMeter bool
    hasBarline      bool
    lastMeasureHadNote bool
    measures        []MeasureResult
}
```

### Step 4.2: Extract `scanMeasureDirectives`

Pull lines 697-751 of the current `ParseDSL` (token scanning for K/M/B directives, beat resolution) into:

```go
type measureDirectives struct {
    fifths        int
    timeNum       int
    timeDen       int
    beat          BeatDuration
    beatTokens    []Token
    explicitMeter bool // true if this measure has its own M directive
}

func scanMeasureDirectives(tokens []Token, state *pipelineState) measureDirectives
```

This is ~50 lines extracted out.

### Step 4.3: Extract `parseBeatGroups`

Pull lines 753-777 (parsing beat groups into `[]ParseResult`, tracking `priorPitch`):

```go
func parseBeatGroups(tokens []Token, priorPitch bool) ([]ParseResult, []error)
```

### Step 4.4: Extract `buildPriorEvents`

Pull lines 784-801 (building per-voice prior events from the previous measure):

```go
func buildPriorEvents(measures []MeasureResult) map[int]*Event
```

### Step 4.5: Extract `markCrossMeasureTies`

Pull lines 814-833 (marking the previous measure's last note for TieNext):

```go
func markCrossMeasureTies(events []Event, measures []MeasureResult)
```

### Step 4.6: Extract `autoDetectTimeSig`

Pull lines 837-845 (auto-detecting time signature from content when no explicit directive):

```go
func autoDetectTimeSig(events []Event, inheritedNum, inheritedDen int, hasBarline, hasInitialMeter, isFirstMeasure bool) (int, int)
```

### Step 4.7: Extract `validateMeasureDuration`

Pull lines 847-874 (validating that measure duration matches the time signature):

```go
func validateMeasureDuration(events []Event, timeNum, timeDen int, tokens []Token, measureIdx int, hasSecondMeasure, hasExplicitMeter bool) []string
```

### Step 4.8: Extract `detectPickup`

Pull lines 882-890:

```go
func detectPickup(events []Event, timeNum, timeDen int, measureIdx int, hasSecondMeasure bool) (isPickup bool)
```

### Step 4.9: Extract `resolveOctaves`

Pull lines 916-981 (octave resolution across all measures) into its own function. It's already a distinct pass at the end of `ParseDSL` — just needs to be lifted out:

```go
func resolveOctaves(measures []MeasureResult)
```

### Step 4.10: The resulting `ParseDSL`

After extraction, `ParseDSL` becomes:

```go
func ParseDSL(text string) DSLResult {
    text, fifths, timeNum, timeDen, hasInitialMeter := stripDirectives(text)
    tokens := tokenize(text)
    tokenGroups, hasBarline := splitMeasures(tokens)
    if len(tokenGroups) == 0 {
        return DSLResult{...error...}
    }

    state := &pipelineState{...}
    var errs []string

    for mi, tg := range tokenGroups {
        dirs := scanMeasureDirectives(tg, state)
        groups, groupErrs := parseBeatGroups(dirs.beatTokens, state.lastMeasureHadNote)
        // ... collect groupErrs ...

        priorEvents := buildPriorEvents(state.measures)
        events, err := resolveDurationsWithPrior(groups, dirs.beat, priorEvents)
        // ... handle err ...

        markCrossMeasureTies(events, state.measures)

        if dirs.timeNum == 0 && dirs.beatCode == "" && hasBarline && !(mi == 0 && hasInitialMeter) {
            dirs.timeNum, dirs.timeDen = autoDetectTimeSig(events, dirs.timeNum, dirs.timeDen, ...)
        }

        // Validate
        hasExplicitMeter := (mi == 0 && hasInitialMeter && hasBarline) || dirs.explicitMeter
        errs = append(errs, validateMeasureDuration(events, dirs.timeNum, dirs.timeDen, tg, mi, len(tokenGroups) > 1, hasExplicitMeter)...)

        // Split
        events = splitAtBarline(events, dirs.timeNum, dirs.timeDen)
        events = splitNonStandardDurations(events)

        isPickup := detectPickup(events, dirs.timeNum, dirs.timeDen, mi, len(tokenGroups) > 1)

        state.measures = append(state.measures, MeasureResult{
            Events: events, TimeNum: dirs.timeNum, TimeDen: dirs.timeDen,
            Fifths: dirs.fifths, IsPickup: isPickup, NumGroups: len(dirs.beatTokens),
        })

        state.lastMeasureHadNote = measureHasNote(events)
        state.currentFifths = dirs.fifths
        state.currentTimeNum = dirs.timeNum
        state.currentTimeDen = dirs.timeDen
    }

    resolveOctaves(state.measures)
    return buildResult(state, fifths, timeNum, timeDen, errs)
}
```

### Testing

No behavioral changes — all existing tests pass unchanged. The extracted functions become independently testable:
- `scanMeasureDirectives` — test with various K/M/B token sequences
- `validateMeasureDuration` — test with over/under-filled measures
- `detectPickup` — test with various first-measure configurations
- `resolveOctaves` — test with single-voice, multi-voice event sequences
- `autoDetectTimeSig` — test with content that doesn't match the inherited meter

---

## Issue 5: `musicxml` Package Unit Tests

**New file:** `musicxml/xml_test.go`

### Step 5.1: Pure-function tests

Test every standalone function:

```go
func TestNoteTypeForDuration(t *testing.T) // ~20 cases: whole, half, quarter, eighth, 16th, dotted, tuplet
func TestDotCount(t *testing.T)            // 0 dots, 1 dot, 2 dots, 3 dots, non-standard
func TestMakeDots(t *testing.T)            // 0→nil, 1→[1], 3→[3]
func TestDurationToTicks(t *testing.T)    // quarter=480, half=960, eighth=240, whole=1920
func TestMidiToStep(t *testing.T)         // MIDI 60=C4, 61=C#4, 59=B3, 0=C0, 127=G10
func TestAccidentalString(t *testing.T)   // 1→sharp, -1→flat, 2→double-sharp, -2→flat-flat, 0→""
func TestSanitizeDSL(t *testing.T)        // comments stripped, blank lines, #c preserved, # comment stripped
```

### Step 5.2: Generator tests

Test `Generate` with small, hand-constructed measure sequences:

```go
func TestGenerateSingleNote(t *testing.T) {
    measures := []parser.MeasureResult{{
        Events:  []parser.Event{parser.NewNoteEvent("c", 0, 0, false, Fraction{1, 4}, nil, 1, 0)},
        TimeNum: 4, TimeDen: 4, Fifths: 0,
    }}
    xml, err := Generate(measures, 0)
    // Assert XML contains: <step>C</step>, <octave>4</octave>, <duration>480</duration>, <type>quarter</type>
}

func TestGenerateChord(t *testing.T) { ... }
func TestGenerateRest(t *testing.T) { ... }
func TestGenerateTuplet(t *testing.T) { ... }
func TestGenerateTie(t *testing.T) { ... }
func TestGenerateKeySignature(t *testing.T) { ... }
func TestGenerateTimeSignatureChange(t *testing.T) { ... }
func TestGenerateMultiVoice(t *testing.T) { ... }
func TestGeneratePickupMeasure(t *testing.T) { ... }
```

### Step 5.3: Roundtrip sanity tests

Parse DSL → Generate MusicXML → verify key structural properties:

```go
func TestRoundtripBasicNotes(t *testing.T) {
    result := parser.ParseDSL("c d e f")
    xml, err := Generate(result.Measures, result.Key.Fifths)
    // Assert XML is non-empty, contains 4 notes, correct durations
}
```

### Step 5.4: Error cases

```go
func TestGenerateEmptyMeasures(t *testing.T) {
    _, err := Generate(nil, 0)
    // Assert no error (empty measures produce valid empty MusicXML)
}
```

---

## Rollout Order & Risk Mitigation

| Step | Risk | Mitigation |
|---|---|---|
| 1.1-1.2: Tuplet fields | Low — field rename only | Grep for `Midi`/`OctaveShift` on TupletStart events; 2 sites |
| 1.3: Constructors | Low — additive | New functions, old construction sites updated one at a time |
| 1.5: Voice 0 elimination | Medium — scattered checks | `git grep 'v == 0\|v = 0\|voice == 0\|voice = 0\|Voice == 0'` first; run full test suite |
| 3: Float math | Low — self-contained | The standard duration set is small (7 items); loop is bounded; test with edge cases |
| 4: Pipeline decomposition | Medium — many extractions | Extract one function at a time, test after each; `ParseDSL` shrink is purely mechanical |
| 5: Tests | None — additive only | New file, no code changes |

Each issue/step can be committed independently after its tests pass. Commit after each step to keep revertible units.

---

## Success Criteria

After all fixes:
- **No `Midi`/`OctaveShift` abuse** — grep for `TupletStart.*Midi` or `TupletStart.*OctaveShift` returns nothing.
- **No `float64` in pipeline.go** — grep for `float64` in `parser/pipeline.go` returns nothing.
- **`ParseDSL` is under 60 lines** — the orchestrator function is short and readable.
- **`musicxml/xml_test.go` exists** — with coverage of all helper functions and the generator.
- **All golden tests pass** — `go test ./cmd/m4bon/ -run Golden` produces zero diffs.
- **`go test ./...` passes** — on the target platform (macOS, since TUI requires it).
