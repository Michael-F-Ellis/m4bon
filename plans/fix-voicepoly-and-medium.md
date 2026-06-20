# Plan: Fix Voice-Poly Sustain Bug & Medium-Severity Items

## Implementation Order

Dependencies:

```
Bug fix (1) → frac extraction (2a) → remaining items (2b–2g)
```

The voice-poly sustain fix is small and self-contained — do it first since it's a functional bug. The `frac` package extraction touches many files and should come next. The remaining items build on the cleaned-up import graph.

---

## Step 1: Fix Voice-Poly Cross-Measure Sustain Bug

**File:** `parser/pipeline.go`

### The Bug

In `resolveDurationsWithPrior`, two cross-measure sustain paths hard-code `priorEvents[1]`:

```go
// Line 68 — pure sustain group:
if priorEvents != nil {
    pe = priorEvents[1]
}

// Line 152 — mixed group sustain:
if priorEvents != nil {
    pe = priorEvents[1]
}
```

These paths always produce events with `Voice: 1` (lines 83, 165), so `[1]` is correct for the legacy single-voice path. However, when a voice-poly chord follows a traditional chord (e.g. `(c d e) | (- - g)`), the voice-poly entries need per-voice prior events that may only exist on voice 1. Additionally, when a voice has a rest in a prior measure (e.g. `(c ; e)`) and a sustain appears for that voice in a later measure, the voice exists but has no pitch to extend — the sustain should silently pass through.

### The Fix

**For lines 68 and 152:** Add a fallback that checks voices 2–4 if voice 1 has no prior, and also accept the case where the voice existed but produced no note (rest marker):

```go
if priorEvents != nil {
    pe = priorEvents[1]
    if pe == nil {
        for v := 2; v <= 4; v++ {
            if priorEvents[v] != nil {
                pe = priorEvents[v]
                break
            }
        }
    }
}
```

**For cross-measure voice lookup in the voice-poly path** (line 216): Uses `priorEvents[voice]` with the entry's actual voice number. However, when the prior measure's last event is a traditional chord (e.g. `(c d e)` at voice 1), `buildPriorEvents` only creates `priorEvents[1]` — voices 2+ have no entry even though the chord's inner pitches should be available for sustain. The fix for this is in `buildPriorEvents` (see Implementation §1a below).

**For sustain-after-rest:** When `(c ; e)` establishes voice 2 with a rest and `(- ; g)` sustains it later, `buildPriorEvents` only tracks voice 2 if it had a note (EventNote/EventChord). A rest doesn't create a voice entry. The sustain code should treat "voice existed but had no note" as valid — the `-` simply means the voice is silent in this slot. Add a `restedVoices` set to the prior events map: if a voice rested and is sustained, skip it (no event produced).

### Implementation

**1a. In `buildPriorEvents`: expand traditional chords into per-voice entries.**

When a traditional chord like `(c d e)` produces an `EventChord` at voice 1, each pitch should be accessible by index as a per-voice entry for voice-poly sustain resolution. Add after the existing EventNote/EventChord loop:

```go
// Expand traditional chords: each pitch is a virtual voice (1-based).
// This allows voice-poly sustains (e.g. (- - g)) to pick up
// individual pitches from a prior measure's chord (e.g. (c d e)).
for i := len(prevEvents) - 1; i >= 0; i-- {
    ev := &prevEvents[i]
    if ev.Type == EventChord && ev.Voice == 1 && len(ev.Pitches) > 1 {
        for pi := 1; pi < len(ev.Pitches); pi++ {
            v := pi + 1 // voice 2, 3, 4...
            if _, ok := priorEvents[v]; !ok {
                // Build a virtual single-note event from this chord pitch
                p := ev.Pitches[pi]
                virtual := Event{
                    Type:            EventNote,
                    Letter:          p.Letter,
                    Accidental:      p.Accidental,
                    OctaveShift:     p.OctaveShift,
                    ExplicitNatural: p.ExplicitNatural,
                    Voice:           v,
                }
                priorEvents[v] = &virtual
            }
        }
    }
}
```

**1b. In `buildPriorEvents`, track voices that had explicit rests** by adding a nil sentinel:

```go
// After the chord-expansion loop:
for i := len(prevEvents) - 1; i >= 0; i-- {
    ev := &prevEvents[i]
    if ev.Type == EventRest {
        if _, ok := priorEvents[ev.Voice]; !ok {
            priorEvents[ev.Voice] = nil // voice exists, had no pitch
        }
    }
}
```

**2. In `resolveDurationsWithPrior`**, the sustain code at lines 68 and 152 already checks `pe == nil` — extend it to accept a nil sentinel as "voice exists, skip silently" rather than returning an error. When `pe` is non-nil but has `Type == EventChord` (traditional chord → voice-poly transition), note/chord sustain events should be produced normally; the `Pitches`/`Midis` arrays will be copied.

### Test cases (see Step 10)

1. **Error:** `"M4/4 (c - e)"` alone — voice 2 sustain with no antecedent in first measure; must error.
2. **Working:** `"M4/4 (c ; e) | (- ; g)"` — voice 2 rests then is silent; voice 1 sustain extends across measure.
3. **Working:** `"M4/4 (c d e) | (- - g)"` — traditional chord → voice-poly; pitches C and D sustain.

---

## Step 2: Extract `frac` Package (Shared Utilities)

**New files:** `frac/frac.go`

### What moves

| Item | From | To |
|---|---|---|
| `Fraction` type | `parser/parse.go` | `frac/frac.go` |
| `gcd(a, b int) int` | `parser/parse.go` + `musicxml/xml.go` | `frac/frac.go` → `func GCD(a, b int) int` |
| `isPowerOf2(n int) bool` | `parser/parse.go` + `musicxml/xml.go` | `frac/frac.go` → `func IsPowerOf2(n int) bool` |
| `lowerPowerOf2(n int) int` | `parser/parse.go` | `frac/frac.go` → `func LowerPowerOf2(n int) int` |
| `isStandardDuration(z, n int) bool` | `parser/parse.go` | `frac/frac.go` → `func IsStandardDuration(z, n int) bool` |
| `lessThanFraction(a, b Fraction) bool` | `parser/pipeline.go` | `frac/frac.go` → method `(f Fraction) LessThan(other Fraction) bool` |
| `subtractFraction(a, b Fraction) Fraction` | `parser/pipeline.go` | `frac/frac.go` → method `(f Fraction) Sub(other Fraction) Fraction` |

### What stays

- `Fraction` remains a `parser.Fraction` type alias for backward compatibility:
  ```go
  // parser/parse.go
  type Fraction = frac.Fraction
  ```
  This is an alias, not a new type, so all existing code using `parser.Fraction` continues to compile without changes.

### New `frac` package

```go
// Package frac provides rational number types and helpers for the m4bon pipeline.
package frac

// Fraction represents a rational number.
type Fraction struct {
    Num int
    Den int
}

func GCD(a, b int) int { ... }
func IsPowerOf2(n int) bool { ... }
func LowerPowerOf2(n int) int { ... }
func IsStandardDuration(z, n int) bool { ... }

func (f Fraction) LessThan(other Fraction) bool { ... }
func (f Fraction) Sub(other Fraction) Fraction { ... }
func (f Fraction) Add(other Fraction) Fraction { ... }
func (f Fraction) MulInt(num, den int) Fraction { ... }
func (f Fraction) Reduce() Fraction { ... }
```

### Updates needed

1. `parser/parse.go`: Remove `Fraction`, `gcd`, `isPowerOf2`, `lowerPowerOf2`, `isStandardDuration`. Add `type Fraction = frac.Fraction`. Update all callers to use `frac.GCD` etc.
2. `musicxml/xml.go`: Remove `isPowerOf2`, `gcd`. Import `frac` and use `frac.GCD`, `frac.IsPowerOf2`.
3. `parser/pipeline.go`: Remove `lessThanFraction`, `subtractFraction`. Update to use methods: `f.LessThan(g)`, `f.Sub(g)`. Simplify all fraction arithmetic with `Add` / `MulInt` / `Reduce`.
4. `midi/generate.go`: Remove `TicksPerWholeNote` constant — use `frac.TicksPerWholeNote` (export `const TicksPerWholeNote = DPPQ * 4` in the frac package). Import `frac`.
5. `render/render.go`: Same for `TicksPerWholeNote`.
6. All test files: Update imports.

### The `TicksPerWholeNote` constant

Add to `frac/frac.go`:
```go
const DPPQ = 480
const TicksPerWholeNote = DPPQ * 4
```

Remove the duplicate `DPPQ` from `midi/generate.go` and `musicxml/xml.go`. Remove the duplicate `TicksPerWholeNote` from `parser/pipeline.go`, `render/render.go`, and `midi/generate.go`.

---

## Step 3: Extract `theory` Package (Key Signature & Pitch Theory)

**New file:** `theory/theory.go`

### What moves

| Item | From | To |
|---|---|---|
| `noteOffsets` map | `parser/pipeline.go:529` | `theory/theory.go` |
| `fifthsToAccidentalMap` | `parser/pipeline.go:627` | `theory.theory.go` |
| `keySigMap` (render's copy) | `render/render.go:198` | Eliminated — use `theory.FifthsToAccidentalMap` |
| `effectiveAccidental` | `parser/pipeline.go:648` + `render/render.go:183` | `theory/theory.go` |
| `sharpOrder` / `flatOrder` | pipeline.go + render.go | `theory/theory.go` |

The `resolvePitch` and `nextHigherPitch` functions stay in `parser/pipeline.go` since they depend on the pipeline-specific `lastPitch` tracking and MIDI range clamping.

### Updates

1. `parser/pipeline.go`: Import `theory`. Replace `fifthsToAccidentalMap` calls with `theory.FifthsToAccidentalMap`. Replace `effectiveAccidental` calls with `theory.EffectiveAccidental`. Replace `noteOffsets` reference with `theory.NoteOffsets`.
2. `render/render.go`: Import `theory`. Delete `keySigMap` and `effectiveAccidental`. Use `theory.FifthsToAccidentalMap` and `theory.EffectiveAccidental`.

---

## Step 4: Remove Voice 0 from `voiceToChannel`

**File:** `midi/generate.go:41-52`

```go
func voiceToChannel(voice int) uint8 {
    switch voice {
    case 0, 1:  // ← remove case 0
        return 0
    case 2:
        return 1
    case 3:
        return 2
    default:
        return 0
    }
}
```

Change to:
```go
func voiceToChannel(voice int) uint8 {
    switch voice {
    case 1:
        return 0
    case 2:
        return 1
    case 3:
        return 2
    default:
        return 0
    }
}
```

---

## Step 5: Wire Up Constructors or Remove Stubs

### Decision: Wire them up

The constructors (`NewNoteEvent`, `NewChordEvent`, `NewRestEvent`) make event creation self-documenting and prevent field-confusion bugs. Update all event construction sites in `parser/pipeline.go` to use them.

### Sites to update in `resolveDurationsWithPrior`:

| Current code | Replace with |
|---|---|
| `Event{Type: EventNote, Duration: ..., Letter: ..., ...}` (line 195) | `NewNoteEvent(entry.Letter, entry.Accidental, entry.OctaveShift, entry.ExplicitNatural, Fraction{Num: posNum, Den: posDen}, nominalPtr, voice, gi)` |
| `Event{Type: EventRest, ...}` (line 255) | `NewRestEvent(Fraction{Num: posNum, Den: posDen}, nominalPtr, voice, gi)` |
| `Event{Type: pe.Type, Duration: ..., ...}` (line 75, 157, 217, 270) | `NewNoteEvent(...)` or `NewChordEvent(...)` based on `pe.Type` |

Where `nominalPtr` is `&Fraction{Num: nomNum, Den: nomDen}` or `nil` depending on `needsTuplet`.

### Call `Validate()` in tests

Add a helper in `parser/parse_test.go`:

```go
func validateEvents(t *testing.T, events []Event) {
    t.Helper()
    for i, ev := range events {
        if err := ev.Validate(); err != nil {
            t.Errorf("event %d invalid: %v", i, err)
        }
    }
}
```

Call it after `ParseDSL` in each test. This catches field misuse at test time without production overhead.

---

## Step 6: Move `SanitizeDSL` to `parser`

**From:** `musicxml/xml.go:605-622`
**To:** `parser/parse.go` (or `parser/sanitize.go`)

Also update callers:
- `cmd/m4bon/main.go`: change `musicxml.SanitizeDSL(dsl)` to `parser.SanitizeDSL(dsl)`
- `cmd/m4bon/tui/main.go`: same
- `m4bon.go` (`Compile`, `Render`): same

This removes a `musicxml` import from `cmd/m4bon/main.go` (cleaner dependency graph).

---

## Step 7: Fix Chord XML Output to Produce `<chord/>`

**File:** `musicxml/xml.go`

### Current (non-conformant):
```go
type NoteEl struct {
    Chord bool `xml:"chord,omitempty"`
    ...
}
```
Produces: `<chord>true</chord>`

### Fixed (MusicXML-conformant):
```go
type NoteEl struct {
    Chord *struct{} `xml:"chord,omitempty"`
    ...
}
```
Produces: `<chord/>`

### Where `Chord` is set:
In `Generate`, line 474: `Chord: pIdx > 0` → change to:
```go
if pIdx > 0 {
    ne.Chord = &struct{}{}
}
```

And in beam logic (line 513): `xmlNotes[i].Chord` → check `xmlNotes[i].Chord != nil`.

### Test update

Update `musicxml/xml_test.go` `TestGenerateChord` to check for `<chord/>` instead of `<chord>`.

---

## Step 8: Fix `splitAtBarline` Shallow Copy

**File:** `parser/pipeline.go:380`

### Current:
```go
ev2 := ev  // shallow copy — shares Pitches, Midis slices
```

### Fixed:
```go
ev2 := ev
if ev.Pitches != nil {
    ev2.Pitches = make([]Pitch, len(ev.Pitches))
    copy(ev2.Pitches, ev.Pitches)
}
if ev.Midis != nil {
    ev2.Midis = make([]int, len(ev.Midis))
    copy(ev2.Midis, ev.Midis)
}
```

This prevents future code changes from accidentally mutating shared state.

---

## Step 9: Add Library-Level Golden Tests

**New/updated files:** `musicxml/golden_test.go`

### What it does

Calls `parser.ParseDSL` + `musicxml.Generate` on the same `.dsl` files used by the CLI golden tests. Compares against `.expected.mxml`. This provides:

- Coverage for `Generate` from real DSL input (not hand-constructed events)
- Faster execution (no subprocess spawn)
- Debuggability (works with `-run` and breakpoints)
- Cathes regressions across the full pipeline

### Implementation

```go
func TestGoldenFiles(t *testing.T) {
    // Same logic as cmd/m4bon/golden_test.go but calls ParseDSL+Generate directly
}
```

Keep the CLI golden tests as smoke tests (they verify the binary works end-to-end). Add a comment in `cmd/m4bon/golden_test.go` pointing to `musicxml/golden_test.go` for the in-process version.

---

## Step 10: Add Voice-Poly Sustain Test Cases

**New files:** `test/cases/voice-poly-sustain.dsl` + `.expected.mxml`, `test/cases/error-voicepoly-no-antecedent.dsl`

### Error case — voice-poly sustain with no antecedent in first measure

```
M4/4 (c - e) | (- - g)
```

Voice 2's sustain in measure 1 has no prior note. `resolveDurationsWithPrior` catches this: voice 2's sustain entry has no `voiceLastIdx` entry and `priorEvents` is nil (first measure), so the error `"sustain in voice %d with no prior note"` fires. This is existing correct behavior — verify it with a golden error test.

### Working case 1 — voice 2 rests then voice 1 sustains across measure

```
M4/4 (c ; e) | (- ; g)
```

Measure 1: voice 1 = C, voice 2 = rest, voice 3 = E.  
Measure 2: voice 1 = sustain (extends C from m1), voice 2 = rest, voice 3 = G.

Voice 1's `-` in measure 2 should extend the C. The `priorEvents[1]` lookup correctly finds the C note event from measure 1.

### Working case 2 — traditional chord followed by voice-poly sustain

```
M4/4 (c d e) | (- - g)
```

Measure 1: traditional chord C+D+E (all in voice 1).  
Measure 2: voice-poly — voice 1 = sustain (extends C), voice 2 = sustain (extends D), voice 3 = G.

This tests that when a voice-poly chord follows a traditional chord, the individual pitches of the traditional chord are treated as distinct voice entries for sustain resolution. Voice 2's `-` picks up the middle pitch (D) from the prior measure's chord.

---

## Rollout Order & Risk

| Step | Risk | Test Strategy |
|---|---|---|
| 1. Voice-poly bug | Low — add assertion + test case | New test case; golden tests pass |
| 2. `frac` package | Medium — touches all files | Run full suite after each sub-step; type alias ensures binary compatibility |
| 3. `theory` package | Low — pure extraction | Golden tests verify identical output |
| 4. Voice 0 removal | Low — dead code removal | Existing MIDI tests cover `voiceToChannel` |
| 5. Constructor wiring | Medium — many sites | Validate() in tests catches mismatches |
| 6. `SanitizeDSL` move | Low — pure relocation | All existing tests use it; no behavior change |
| 7. Chord XML fix | Low — output format change | Golden test update; existing chord test updated |
| 8. `splitAtBarline` copy | Low — defensive | Chord golden tests validate identical output |
| 9. Library golden tests | None — additive only | New file; doesn't affect existing tests |
| 10. Voice-poly test case | None — additive only | New golden file |

Each step can be committed independently after its tests pass.

---

## Success Criteria

- `go test ./...` passes on target platform
- All golden tests pass with zero diffs (`go test ./cmd/m4bon/ -run Golden`)
- `go vet ./...` produces zero errors
- No duplicated `gcd`, `isPowerOf2`, `effectiveAccidental` across packages (grep confirms one definition each)
- Voice-poly sustain test case passes
- Chord XML output contains `<chord/>` not `<chord>true</chord>`
- `SanitizeDSL` is in `parser` package; `cmd/m4bon` no longer imports `musicxml` directly

---

## Bugs Discovered During Initial Run — Incorporate Into Plan Steps

These were found and fixed during the first execution. Incorporate them into the relevant steps when re-running.

### Bug A: `b` beat-directive prefix conflicts with note B (`scanMeasureDirectives`)

**File:** `parser/pipeline.go`, function `scanMeasureDirectives`

**Symptom:** A group token like `bag` starts with `b` and `len > 1`, so it gets consumed as a beat-code directive with suffix `"ag"`. Since `BeatDurationCodes["AG"]` doesn't exist, the old code fell back to `BeatDuration{1, 4}`, corrupting the time signature for the rest of the piece. Each subsequent measure derived a wrong time sig from the corrupted beat.

**Fix (applied after Step 5 — constructor wiring):**

1. Don't set `md.hasBeatCode = true` until the suffix is confirmed valid:
   ```go
   if strings.HasPrefix(raw, "b") && len(raw) > 1 {
       bc := strings.ToUpper(raw[1:])
       if bd, ok := BeatDurationCodes[bc]; ok {
           md.hasBeatCode = true
           md.beat = bd
           continue
       }
       // Unknown beat suffix — fall through to beatTokens
   }
   ```
2. Add a `foundNotation bool` flag that stops directive scanning after the first non-directive token. This bounds the ambiguity — directives must appear at the start of the measure, before any notation.

### Bug B: Render output omits intra-group sustains

**File:** `render/render.go`, function `buildMeasureCells`

**Symptom:** A group like `c-b` renders as `cb` instead of `c-b`. The sustain `-` is absorbed into the preceding note's duration during event creation and leaves no trace.

**Fix (applied after Step 5 — constructor wiring):**

1. Add a field to `Event`:
   ```go
   NumSlots int  // number of slot positions this event spans (for render)
   ```
2. In `resolveDurationsWithPrior` in `parser/pipeline.go`:
   - Set `NumSlots: len(group.Slots)` on cross-measure sustain events (they span the entire group).
   - Set `NumSlots = 1` on regular note/chord/rest events.
   - After a sustain extends the previous event in the same group: `last.NumSlots++`.
3. In the renderer's `buildMeasureCells`, after rendering each event's cells:
   ```go
   for s := 1; s < ev.NumSlots; s++ {
       cells = append(cells, Cell{Content: "-", Style: StyleSustainRest})
   }
   ```
4. Do NOT increment `NumSlots` in the pure-sustain-group extension path (lines ~92-108 in pipeline.go) — that's a cross-group sustain, not an intra-group absorbed slot.

### Bug C: Uppercase letters accepted as note values

**File:** `parser/parse.go`, function `parseGroup`

**Fix (applied after Step 1 or whenever parseGroup is touched):**

Add a rejection check before the existing pitch letter handler:
```go
if ch >= 'A' && ch <= 'G' {
    return err("uppercase notes not allowed — use lowercase", i)
}
```

Also in `normalizePitchInput`, remove the `strings.ToLower(text)` call — only normalize Unicode accidental glyphs (♯♭♮). This preserves case through the tokenizer, allowing directives `K`, `M`, `B` (uppercase) to be distinguished from note tokens. Update `stripDirectives` regex and `canonicalKey` accordingly.

### Bug D: TUI scheduler goroutine leak (Playing→Stopped state failure)

**Files:** `cmd/m4bon/tui/model.go`, `cmd/m4bon/tui/update.go`

**Symptom:** After playback ends naturally, the state indicator doesn't transition from "▶ Playing" to "■ Stopped". The standalone scheduler created in `handlePlayPause()` (`update.go:181`) is never stored or stopped between play sessions, causing multiple goroutines to poll the player concurrently with stale callback entries.

**Fix (independent of refactoring — can be applied any time):**

1. Add `scheduler *macaudio.Scheduler` field to `model`.
2. In `handlePlayPause()`, stop `m.scheduler` (not `m.transport.Scheduler()`) before creating a new one, and save the new one: `m.scheduler = sched`.
3. In `handleStop()` and `cleanup()`, stop `m.scheduler`.
4. Add a guard in `handlePlaybackEnded()`: `if m.scheduler == nil { return m, nil }` to discard stale `playbackEndedMsg` from previous sessions.
5. Remove `p.playStartUs = 0` from `dlsMIDIPlayer.Stop()` in `/Users/mellis/macaudio/midi_darwin.go` so the scheduler can still read a valid position after the player stops internally.
6. Add a state-based fallback in the `positionMsg` handler: if `m.isPlaying && m.midiPlayer.State() == macaudio.StateIdle`, call `handlePlaybackEnded()`.

### Bug E: `normalizePitchInput` lowercases directives, preventing case-based disambiguation

**File:** `parser/parse.go`, function `normalizePitchInput`

**Fix (part of Bug C — do together):**

Change from:
```go
func normalizePitchInput(text string) string {
    t := strings.ToLower(text)
    for r, s := range accidentalReplacements {
        t = strings.ReplaceAll(t, string(r), s)
    }
    return t
}
```
To:
```go
func normalizePitchInput(text string) string {
    t := strings.ReplaceAll(text, "♯", "#")
    t = strings.ReplaceAll(t, "♭", "&")
    t = strings.ReplaceAll(t, "♮", "%")
    return t
}
```
Then update `stripDirectives` regex from `^(K\S+\s*)?(M\S+\s*)?` to `^([kK]\S+\s*)?([mM]\S+\s*)?` and its `strings.HasPrefix(part, "K")` checks to also accept lowercase. Update `canonicalKey` to handle uppercase letters.

