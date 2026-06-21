# Remove MIDI-Based Pitch Resolution

## Principle

Pitch identity (letter + accidental + octave) must be resolved without any
reliance on MIDI numbers or semitone arithmetic. MIDI numbers are derived
from the resolved pitch identity, never the reverse. The DSL grammar
guarantees this is always possible.

## Changes

### 1. `parser/pipeline.go` — New helpers (no MIDI)

```go
// letterIndex maps a-g to 0-6 consecutively.
var letterIndex = map[string]int{"c":0,"d":1,"e":2,"f":3,"g":4,"a":5,"b":6}

// resolveOctave picks the closest octave for targetLetter from a reference
// letter+octave via diatonic step distance, then applies octaveShift.
func resolveOctave(targetLetter, refLetter string, refOctave, octaveShift int) int

// nextHigherOctave returns the octave for targetLetter such that it is
// the next pitch above the reference letter+octave. Chords are always
// ascending in letter; wraps octave when targetIdx <= refIdx.
func nextHigherOctave(refLetter, targetLetter string, refOctave, octaveShift int) int

// midiFromPitch computes MIDI from letter, accidental, octave.
func midiFromPitch(letter string, accidental, octave int) int
```

### 2. `parser/pipeline.go` — Replace `resolvePitch` and `nextHigherPitch`

Delete both. Callers use `resolveOctave` → `midiFromPitch` or
`nextHigherOctave` → `midiFromPitch`.

### 3. `parser/pipeline.go` — Track `lastOctave` instead of `lastPitch`

Current:
```go
lastPitch map[int]int     // voice → MIDI
lastLetter map[int]string // voice → letter
```

Change to:
```go
lastOctave map[int]int    // voice → octave (midi/12 convention for now)
lastLetter map[int]string // voice → letter
```

Initialize `lastOctave[1] = 5` (C4 = MIDI 60 / 12).

In the loop, `ref` becomes `refOctave` and `refLetter` from the maps.

### 4. `parser/pipeline.go` — Chord resolution

Replace:
```go
chordRef := ref               // MIDI
chordRefLetter := refLetter
...
m = resolvePitch(... chordRef, chordRefLetter)
...
m = nextHigherPitch(... chordRef)
...
chordRef = m                  // MIDI
```

With:
```go
chordOct := refOctave
chordLet := refLetter
...
oct := resolveOctave(... chordLet, chordOct)
m = midiFromPitch(... oct)
...
oct = nextHigherOctave(... chordLet, chordOct)
m = midiFromPitch(... oct)
...
chordOct = oct
chordLet = pi.Letter
```

### 5. `musicxml/xml.go` — Use `ev.ResolvedOctave` instead of `ev.Midi`

Add `ResolvedOctave int` field to `parser.Event`. Populate during
`resolveOctavesMeasures` from the computed octave.

Then in MusicXML generation, use `ev.ResolvedOctave` and `ev.Letter`/`ev.EffAccidental`
directly instead of `midi/12` or `midiToStep()`.

For EventChord: `ev.Pitches[p].Letter` + `ev.Pitches[p].Accidental` give step/alter.
Add `ev.ResolvedOctaves []int` parallel to `ev.Midis`.

### 6. Remove `midiToStep` from `musicxml/xml.go`

No longer needed once step/octave/alter come directly from the event fields.

## Migration Strategy

1. Add new helper functions (resolveOctave, nextHigherOctave, midiFromPitch)
2. Add ResolvedOctave field to Event (with Midis → ResolvedOctaves for chords)
3. Rewrite resolveOctavesMeasures to use octave tracking, populating ResolvedOctave
4. Update musicxml to use ResolvedOctave + letter/accidental fields
5. Remove resolvePitch, nextHigherPitch, midiToStep
6. Run full test suite, update golden files
7. Update render package if needed (it uses ev.Midi for octaveSubscript)

## Files Affected

- `parser/pipeline.go` — core resolution rewrite
- `parser/parse.go` — add ResolvedOctave, ResolvedOctaves fields
- `musicxml/xml.go` — use resolved fields, remove midiToStep
- `render/render.go` — may need to use ResolvedOctave instead of Midi/12-1
- Golden test files — expect MusicXML output changes
