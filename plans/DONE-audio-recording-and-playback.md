# Audio Recording, Metronome, and Chord Roots in the TUI

## Overview

Add live microphone recording, independent metronome/chord-root toggles, and Transport-based playback switching to the m4bon TUI. The TUI becomes a self-contained rehearsal tool: play a score range with optional backing, record yourself playing along, and immediately review the take — all with measure-synced indicators.

---

## Architecture Decisions

| Decision | Rationale |
|---|---|
| Transport as single proxy | Replace all direct `m.midiPlayer.X()` with `m.transport.X()`. After recording, `transport.SetRecording(rec)`. Clean single path for all playback. |
| SMF regeneration for metronome/roots | Simple and fast — SMF files are <10KB, regeneration <1ms. No per-channel volume control needed. |
| Chord roots in bass range E1–E2 (MIDI 28–40) | 4-string bass standard tuning. Octave 2 in m4bon's convention (octave 5 = C4), shift to octave 3 for notes below MIDI 28. |
| Chord root MIDI channel 8 | Below percussion channel 9, above voice channels 0–3. |
| Recording ephemeral, one-at-a-time | New recording discards old. No save (WAV export is a `macaudio` stub anyway). |
| Default mic, no device selection | macOS has adequate system-level I/O management. |

---

## Phase 1: `midi/generate.go` — Options-aware SMF generation

### 1a: Add `SMFOptions` and refactor `GenerateSMF`

**File:** `midi/generate.go`

Add struct and backward-compatible wrapper:

```go
// SMFOptions controls which auxiliary tracks are included in the SMF.
type SMFOptions struct {
    Metronome bool // include metronome track (MIDI channel 9)
    Roots     bool // include chord root track (MIDI channel 8, bass range)
}

// GenerateSMF produces a Standard MIDI File with metronome (backward compat).
func GenerateSMF(measures []parser.MeasureResult, bpm float64) ([]byte, Timeline, error) {
    return GenerateSMFWithOptions(measures, bpm, SMFOptions{Metronome: true, Roots: false})
}

// GenerateSMFWithOptions produces a Standard MIDI File with configurable tracks.
func GenerateSMFWithOptions(measures []parser.MeasureResult, bpm float64, opts SMFOptions) ([]byte, Timeline, error) {
```

Move the full body of `GenerateSMF` into `GenerateSMFWithOptions`, then gate the metronome and root track additions on `opts`.

### 1b: Chord root track generation

Inside the measure loop, after the existing metronome block (lines 203–215), add:

```go
// Chord roots for this measure (if enabled and chords present)
if opts.Roots && len(m.Chords) > 0 {
    for beatIdx := 0; beatIdx < numBeats && beatIdx < len(m.Chords); beatIdx++ {
        raw := m.Chords[beatIdx]
        letter, acc := theory.ChordRoot(raw)
        if letter == "" {
            continue // sustain "-" or rest ";"
        }
        midi := chordRootMIDI(letter, acc)
        beatAbsTick := measureStartTick + int64(beatIdx)*beatTicks
        rootChannel := uint8(8)
        rootEvents = append(rootEvents,
            timedEvent{beatAbsTick, []byte{0x90 | rootChannel, uint8(midi), 90}},
        )
        // NoteOff at beat end
        rootEvents = append(rootEvents,
            timedEvent{beatAbsTick + beatTicks, []byte{0x80 | rootChannel, uint8(midi), 0}},
        )
    }
}
```

Add helper:

```go
const rootChannel uint8 = 8

// chordRootMIDI maps a chord root letter+accidental to a MIDI note in bass range (E1-E2).
// Uses m4bon's octave convention where octave 5 = C4 (MIDI 60).
func chordRootMIDI(letter string, accidental int) int {
    // Start at octave 2 (C2 = MIDI 24, B2 = MIDI 35)
    midi := 2*12 + theory.NoteOffsets[strings.ToLower(letter)] + accidental
    // Shift to octave 3 if below E1 (MIDI 28)
    if midi < 28 {
        midi += 12
    }
    return midi
}
```

Add import for `"github.com/mellis/m4bon/theory"` and `"strings"` if not already imported.

Collect events in a `var rootEvents []timedEvent` slice, build `rootTrack := buildTrackFromEvents(rootEvents)`, add to SMF when `opts.Roots` is true and `len(rootEvents) > 0`.

### 1c: Update tests in `midi/generate_test.go`

All existing tests call `GenerateSMF` — no change needed since the wrapper passes `Metronome:true, Roots:false`.

Add new tests:
- `TestGenerateSMF_Roots_Basic`: DSL with `:H` directives, verify root MIDI notes on channel 8 at correct ticks.
- `TestGenerateSMF_Options`: Test `GenerateSMFWithOptions` with various flag combos.
- `TestGenerateSMF_Roots_NoChords`: Measures without `:H` directives produce no root events.

Use the existing `ParseSMFToEvents` helper to validate.

---

## Phase 2: `theory/chords.go` — Export root extraction

### 2a: Add `ChordRoot` function

**File:** `theory/chords.go`

```go
// ChordRoot extracts the root letter and accidental from a chord symbol.
// Returns empty string for sustain "-" and rest ";" markers.
// letter is uppercase (C, D, E, F, G, A, B).
// accidental is 1 for sharp, -1 for flat, 0 for natural.
func ChordRoot(raw string) (letter string, accidental int) {
    if raw == "-" || raw == ";" {
        return "", 0
    }
    return normalizeRoot(raw)
}
```

### 2b: Add tests

**File:** `theory/chords_test.go`

```go
func TestChordRoot(t *testing.T) {
    tests := []struct {
        raw    string
        letter string
        acc    int
    }{
        {"C", "C", 0},
        {"Dm", "D", 0},
        {"E7", "E", 0},
        {"F#m", "F", 1},
        {"Gb", "G", -1},
        {"Ab7", "A", -1},
        {"Bbmaj7", "B", -1},
        {"C#dim", "C", 1},
        {"-", "", 0},
        {";", "", 0},
    }
    for _, tc := range tests {
        letter, acc := ChordRoot(tc.raw)
        if letter != tc.letter || acc != tc.acc {
            t.Errorf("ChordRoot(%q) = (%q, %d), want (%q, %d)",
                tc.raw, letter, acc, tc.letter, tc.acc)
        }
    }
}
```

---

## Phase 3: TUI model — New state fields and Transport refactor

**File:** `cmd/m4bon/tui/model.go`

### 3a: New fields in `model` struct

```go
// Toggle state
metronomeOn bool // toggled by 'm', default true
rootsOn     bool // toggled by 'R', default false
```

Initialize in `initialModel`:
```go
metronomeOn: true,
rootsOn:     false,
```

### 3b: Transport refactor — `loadMIDIPlayer`

After `m.midiPlayer = player`, always set:
```go
m.transport.SetMIDIPlayer(player)
```

Also update `transport.SetMIDIPlayer(nil)` in `loadMIDIPlayer` cleanup section (line 128) to `m.transport.SetMIDIPlayer(nil)`.

### 3c: `regenerateSMF` — use options

```go
func (m *model) regenerateSMF() error {
    opts := midi.SMFOptions{
        Metronome: m.metronomeOn,
        Roots:     m.rootsOn,
    }
    data, tl, err := midi.GenerateSMFWithOptions(m.measures, m.bpm, opts)
    // ... rest unchanged
}
```

### 3d: `reloadMeasures` — use options (same pattern as 3c)

### 3e: `elapsedTick` — poll transport, not midiPlayer

```go
func (m *model) elapsedTick() tea.Cmd {
    return tea.Every(100*time.Millisecond, func(t time.Time) tea.Msg {
        return positionMsg{time.Now()}
    })
}
```

(The `positionMsg` handler already uses `m.midiPlayer.Position()` — see 4e below.)

---

## Phase 4: TUI update — Transport-based handlers + recording

**File:** `cmd/m4bon/tui/update.go`

### 4a: Keybinding dispatch (`handleKeyMsg`)

Add cases:
```go
case "r":
    return m.handleRecordToggle()

case "m":
    return m.handleMetronomeToggle()

case "R":
    return m.handleRootsToggle()
```

### 4b: `handlePlayPause` — go through Transport

**Current:** calls `m.midiPlayer.Pause()`, `m.midiPlayer.Play()`, `m.midiPlayer.Seek()`, `m.midiPlayer.PlaySegment()` directly.

**Change:** Replace all with `m.transport.X()`:
```go
func (m *model) handlePlayPause() (tea.Model, tea.Cmd) {
    if m.isRecording {
        return m, nil  // space ignored during recording
    }
    if m.transport.State() == macaudio.StateIdle && ... {
        return m, nil  // nothing to play
    }
    // ... same logic but using m.transport
}
```

The transport auto-proxies to MIDIPlayer or Recording depending on what's set.

### 4c: `handleStop` — go through Transport

```go
func (m *model) handleStop() (tea.Model, tea.Cmd) {
    if m.isRecording {
        return m.handleRecordToggle()  // stop recording instead
    }
    m.transport.Stop()
    m.transport.Seek(0)
    m.isPlaying = false
    m.isPaused = false
    m.currentMeasure = m.startMeasure
    m.elapsed = 0
    return m, nil
}
```

### 4d: `handleSeekMeasure` — go through Transport

Replace `m.midiPlayer.Seek(seekTime)` with `m.transport.Seek(seekTime)`.

Wait — seeking switches back to MIDI mode. If we're in recording-playback mode, seeking should return to the score. So:

```go
func (m *model) handleSeekMeasure(delta int) (tea.Model, tea.Cmd) {
    // If we're reviewing a recording, switch back to MIDI
    if m.recording != nil && m.transport.Active() == "recording" {
        m.recording.Stop()
        m.recording = nil
        m.transport.SetMIDIPlayer(m.midiPlayer)
    }
    // ... existing seek logic
}
```

### 4e: `positionMsg` handler — use transport state

```go
case positionMsg:
    if m.transport.State() != macaudio.StateIdle || m.isPlaying {
        m.elapsed = m.transport.Position()
        m.currentMeasure = m.measureAtTime(m.elapsed)
        if m.isPlaying && m.transport.State() == macaudio.StateIdle {
            m.isPlaying = false
            m.isPaused = false
            m.elapsed = 0
            m.currentMeasure = m.startMeasure
            return m, nil
        }
    }
    if m.isPlaying || m.isPaused {
        return m, m.elapsedTick()
    }
    return m, nil
```

### 4f: `handleRecordToggle` — new handler

```go
func (m *model) handleRecordToggle() (tea.Model, tea.Cmd) {
    if m.isRecording {
        // Stop recording
        rec, err := m.recorder.Stop()
        if err != nil {
            // Silent fail — no recording produced
            m.isRecording = false
            m.recorder.Close()
            m.recorder = nil
            return m, nil
        }
        m.recording = rec
        m.isRecording = false
        m.recorder.Close()
        m.recorder = nil

        // Stop MIDI playback
        m.midiPlayer.Stop()
        m.midiPlayer.Seek(0)
        m.isPlaying = false
        m.isPaused = false
        m.elapsed = 0

        // Switch transport to recording for review
        m.transport.SetRecording(rec)
        return m, nil
    }

    // Start recording
    if m.midiPlayer == nil {
        return m, nil
    }

    // Create recorder on demand
    if m.recorder == nil {
        r, err := macaudio.NewRecorder()
        if err != nil {
            return m, nil
        }
        m.recorder = r
    }

    // Start mic recording
    if err := m.recorder.Start(""); err != nil {
        return m, nil
    }

    // Start MIDI as backing track
    if len(m.timeline.MeasureStarts) > 0 {
        seekIdx := m.startMeasure
        if seekIdx > m.endMeasure {
            seekIdx = m.endMeasure
        }
        m.elapsed = m.timeline.MeasureStarts[seekIdx]
    }
    m.currentMeasure = m.startMeasure
    m.midiPlayer.Stop()
    m.midiPlayer.Seek(m.elapsed)
    endTime := m.timeline.TotalDuration
    if m.endMeasure+1 < len(m.timeline.MeasureStarts) {
        endTime = m.timeline.MeasureStarts[m.endMeasure+1]
    }
    m.midiPlayer.PlaySegment(m.elapsed, endTime)

    m.isRecording = true
    m.isPlaying = true
    m.isPaused = false
    return m, m.elapsedTick()
}
```

### 4g: `handleMetronomeToggle` — new handler

```go
func (m *model) handleMetronomeToggle() (tea.Model, tea.Cmd) {
    if m.isPlaying || m.isRecording {
        return m, nil  // no toggle during playback
    }
    m.metronomeOn = !m.metronomeOn
    m.regenerateSMF()
    return m, nil
}
```

### 4h: `handleRootsToggle` — new handler

```go
func (m *model) handleRootsToggle() (tea.Model, tea.Cmd) {
    if m.isPlaying || m.isRecording {
        return m, nil
    }
    m.rootsOn = !m.rootsOn
    m.regenerateSMF()
    return m, nil
}
```

### 4i: `handleVolumeDelta` — go through Transport

Replace `m.midiPlayer.SetVolume(m.volume)` with `m.transport.SetVolume(m.volume)`.

### 4j: `cleanup` — add recorder close (already present at line 210–212)

No change needed — `m.recorder.Close()` is already in cleanup.

---

## Phase 5: TUI view — Status bar and help

**File:** `cmd/m4bon/tui/view.go`

### 5a: Status bar — `transport` label

Replace the transport switch (lines 228–235):

```go
var transport string
switch {
case m.isRecording:
    transport = "● REC"
case m.isPlaying:
    if m.transport.Active() == "recording" {
        transport = "▶ Review"
    } else {
        transport = "▶ Playing"
    }
default:
    transport = "■ Stopped"
}
```

### 5b: Help overlay

Add new keybindings to the help text:

```
  r        Start / stop recording
  m        Toggle metronome (current: on/off)
  R        Toggle chord roots (current: on/off)
```

Insert after `u` (reload) and before `q`.

### 5c: Top bar — add toggle indicators

After the volume display in `topBar()`:

```go
if m.metronomeOn {
    parts = append(parts, "click:on")
} else {
    parts = append(parts, "click:off")
}
if m.rootsOn {
    parts = append(parts, "roots:on")
}
```

---

## Phase 6: TUI main — Initialization

**File:** `cmd/m4bon/tui/main.go`

### 6a: Empty state — use `GenerateSMFWithOptions`

Line 23: Change `midi.GenerateSMF(emptyMeasures, bpm)` to `midi.GenerateSMFWithOptions(emptyMeasures, bpm, midi.SMFOptions{Metronome: true})`

### 6b: Normal state — use `GenerateSMFWithOptions`

Line 50: Change `midi.GenerateSMF(result.Measures, bpm)` to `midi.GenerateSMFWithOptions(result.Measures, bpm, midi.SMFOptions{Metronome: true})`

---

## Phase 7: Tests

### `midi/generate_test.go`

Add:

```go
func TestGenerateSMF_Options_MetronomeOff(t *testing.T) {
    measures := parseDSL(t, "c d e f")
    data, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Metronome: false})
    // verify no events on channel 9
}

func TestGenerateSMF_Roots(t *testing.T) {
    measures := parseDSL(t, ":H C G7 | c d e f")
    data, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Metronome: true, Roots: true})
    // verify events on channel 8 with correct pitches at beat positions
}

func TestGenerateSMF_Roots_NoChords(t *testing.T) {
    measures := parseDSL(t, "c d e f")
    data, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Roots: true})
    // verify no events on channel 8
}
```

### `theory/chords_test.go`

Add `TestChordRoot` as described in Phase 2.

---

## Key bindings (final reference)

| Key | Action |
|-----|--------|
| `space` | Play / Pause (disabled during recording) |
| `s` | Stop (also stops recording) |
| `r` | Start / stop recording |
| `m` | Toggle metronome |
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

---

## Execution order

1. **Phase 2** — `theory/chords.go` + tests (no dependencies)
2. **Phase 1** — `midi/generate.go` + tests (depends on Phase 2)
3. **Phase 3** — TUI model fields + Transport setup
4. **Phase 4** — TUI update handlers (depends on Phase 1, 3)
5. **Phase 5** — TUI view (depends on Phase 3)
6. **Phase 6** — TUI main (depends on Phase 1)
7. **Phase 7** — Run all tests: `go test ./... && make`

---

## Edge cases

| Case | Behavior |
|-------|----------|
| Record with no measures loaded | `r` is ignored (no MIDI player) |
| Record while already recording | `r` stops recording (toggle) |
| Space during recording | Ignored (no pause mid-recording) |
| Seek during recording | Allowed — changes start indicator, MIDI continues playing from current position? NO — seeking seeks the backing track. Design choice: block seek during recording, or allow it. **Block it** for simplicity. |
| Stop during recording | Stops both mic and MIDI, produces a recording. Same as second `r`. |
| Seek after recording review | Switches transport back to MIDI, discards recording reference. |
| New recording replaces old | New `Recorder.Start()` → new `Recording` on stop. Old `Recording.Close()` is called by transport when `SetRecording` replaces it. |
| Empty chord symbols (`:H - -`) | `ChordRoot` returns `"", 0` — no MIDI note emitted. |
| Invalid chord symbols | `ChordRoot` returns `"", 0` — silently skipped. |
| Tempo change during recording | Block tempo changes during recording (regenerates SMF which would desync from mic). |
| Quit during recording | `cleanup()` calls `m.recorder.Close()`. Recording data is lost (ephemeral — by design). |
