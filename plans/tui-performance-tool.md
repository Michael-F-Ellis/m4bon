# TUI Performance & Learning Tool — Implementation Plan

**Date:** 2026-06-18
**Status:** Draft — under review

## Objective

Transform m4bon from a CLI compiler into an interactive TUI performance/learning tool. The app displays rendered measure lines (the current `-render` output), sequences a pointer indicator across them in sync with playback, and supports recording user performances over selected measures. All audio and MIDI capabilities are provided by `github.com/Michael-F-Ellis/macaudio`.

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                      cmd/m4bon/tui/                               │
│  ┌─────────────┐   ┌─────────────┐   ┌──────────────────────┐   │
│  │  Model       │   │  View       │   │  Update              │   │
│  │  (state)     │   │  (lipgloss) │   │  (BubbleTea msgs)   │   │
│  └──────┬───────┘   └──────┬──────┘   └──────────┬───────────┘   │
│         │                  │                      │               │
│         ▼                  ▼                      ▼               │
│  ┌──────────────────────────────────────────────────────────────┐ │
│  │                    TUI Application                            │ │
│  │  • Load DSL file / enter DSL string                          │ │
│  │  • Parse → render lines + compute timeline                   │ │
│  │  • Display measure lines + moving indicator                  │ │
│  │  • Transport controls: play, pause, stop, tempo, record      │ │
│  │  • Measure selection for recording                           │ │
│  └──────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
┌─────────────┐   ┌──────────────────┐   ┌─────────────────┐
│  m4bon      │   │  midi/generate   │   │  macaudio       │
│  • Compile  │   │  (NEW)           │   │  • Transport    │
│  • Render   │   │  • Event→MIDI    │   │  • MIDIPlayer   │
│  • ParseDSL │   │  • SMF bytes     │   │  • Player       │
│             │   │  • Timeline      │   │  • Recorder     │
│             │   │  • Metronome     │   │  • Scheduler    │
└─────────────┘   └──────────────────┘   └─────────────────┘
```

The design follows the same two-layer pattern as the renderer: a core computation layer (`midi/` package) produces data (`[]byte` SMF + `[]time.Duration` timeline), and the TUI layer consumes it through macaudio's Transport.

---

## Phase 0 — New Dependency & Module Setup

### 0a — Add macaudio + BubbleTea dependencies

```
go get github.com/Michael-F-Ellis/macaudio
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles
```

These are already tested together in macaudio's `cmd/audiodemo`.

### 0b — Build constraints

The TUI and macaudio are macOS-only. Add `//go:build darwin && cgo` to all TUI files and the `midi/` package (which calls macaudio). Keep existing m4bon packages cross-platform.

---

## Phase 1 — MIDI Generation (`midi/generate.go`)

### 1a — Package purpose

The `midi` package converts parsed `[]parser.MeasureResult` into:
1. **SMF bytes** (Standard MIDI File, format 1) for playback via macaudio's MIDIPlayer
2. **Timeline**: `[]time.Duration` of measure start times, for scheduler callbacks
3. **Metronome events**: embedded in the SMF on channel 10 as GM percussion

### 1b — Public API

```go
package midi

// Timeline maps measure indices to their start time offsets.
type Timeline struct {
    MeasureStarts []time.Duration // index = measure number (0-based)
    TotalDuration time.Duration
    TempoBPM      float64
}

// GenerateSMF produces a Standard MIDI File from parsed measures.
// Returns the SMF bytes, a timeline of measure start times, and any error.
func GenerateSMF(measures []parser.MeasureResult, bpm float64) (smfBytes []byte, timeline Timeline, err error)

// GenerateMetronomeOnly produces an SMF with only metronome clicks.
// Useful for practice without the score notes.
func GenerateMetronomeOnly(measures []parser.MeasureResult, bpm float64) ([]byte, Timeline, error)
```

### 1c — Algorithm: Event → MIDI conversion

**Tick resolution**: 480 ticks per quarter note (DPPQ), matching MusicXML generation.

For each measure, for each event:

1. **Compute event tick position**: accumulate `durationToTicks(ev.Duration)` per voice. Track per-voice tick accumulator (same pattern as `musicxml/xml.go`'s `voiceTick` map).

2. **Compute event tick duration**: `durationToTicks(ev.Duration)`.

3. **Generate MIDI events**:

   | Event type | MIDI messages |
   |---|---|
   | `EventNote` (Split=f) | `NoteOn(channel, ev.Midi, vel)` at tick; `NoteOff(channel, ev.Midi, 0)` at tick+duration |
   | `EventNote` (Split=t) | Skip — tie continuation, no new note |
   | `EventRest` | Skip — no MIDI output |
   | `EventChord` (Split=f) | `NoteOn` for each `ev.Midis[i]`; `NoteOff` for each at tick+duration |
   | `EventChord` (Split=t) | Skip |
   | `EventTupletStart` | Skip |

4. **Voice → MIDI channel mapping**: Voice 0→channel 1, voice 1→channel 1, voice 2→channel 2, voice 3→channel 3. (Voice 0 and 1 both go to channel 1 since voice 0 is the pre-voice-poly default.)

5. **Cross-measure ties**: When `ev.TieNext` is true, emit only `NoteOn` without the corresponding `NoteOff` in this measure. The `NoteOff` appears in the next measure where the tie continues (the `Split=true` event) — actually, cross-measure ties work by having the `NoteOn` in measure N and the `NoteOff` in measure N+1. The parser marks `TieNext=true` on the last event of the sustaining chain in measure N.

   Implementation: when building MIDI events per measure, track "pending note-offs" — notes that started in a previous measure and need to end in this one (first `Split=true` event for that voice/pitch).

   Actually simpler: don't generate `NoteOff` for `TieNext=true` events. In the next measure, the first event of that voice with `Split=false` is the note that ends the tie — generate its `NoteOn` and `NoteOff` normally. Wait, that's wrong. Split=true events are tie continuations — they don't generate new NoteOns.

   Let me re-examine. In the parser, a sustained note like `a---b` in 4/4 produces:
   - Event 0: a, Dur=1/2(q+1/8=5/8? no...), Split=false
   - Event 1: a, Dur=1/8, Split=true (continuation)
   - Event 2: b, Dur=1/4, Split=false

   For MIDI: Event 0 gets NoteOn+NoteOff (the NoteOff at the end of its tick duration). Event 1 is skipped. Event 2 gets NoteOn+NoteOff.

   Cross-measure: `a - | a b`:
   - Measure 1: a (Dur=1/2, Split=f, TieNext=true), a (Dur=1/4, Split=t)
   - Measure 2: a (Dur=1/4, Split=f), b (Dur=1/4, Split=f)

   For MIDI:
   - M1: NoteOn(a) at tick 0. Don't emit NoteOff because TieNext=true on the first event. Skip Split=t event.
   - M2: The `a` with Split=f starts a new note — but it's continuing a tie. In MusicXML this is a `<tie type="stop"/>`. In MIDI, we just emit the NoteOff for the note that started in M1. But we need to know the tick position.

   Actually, looking at the MusicXML generator, cross-measure ties emit `<tie type="start"/>` on the last note before the barline and `<tie type="stop"/>` on the first note after. In MIDI, this means:
   - M1: NoteOn at tick 0, NO NoteOff (extends into M2)
   - M2: NoteOff at the end of the tied note's duration in M2.

   But how do we know the NoteOff tick? It's the accumulated tick at the end of the `Split=f` event in M2. Since we process measure-by-measure, we can track pending NoteOffs across measures.

   **Implementation**: Maintain a `map[voiceKey]pendingNoteOff` across the GenerateSMF loop. A `voiceKey` is `{channel, midiPitch}`. When processing events:
   - On `Split=f` event with no pending NoteOff for this key: emit `NoteOn` now, schedule `NoteOff` at `currentTick + durTicks`
   - On `Split=f` event WITH a pending NoteOff for this key: this is a tie continuation — don't emit new `NoteOn`, update the pending NoteOff to `currentTick + durTicks`
   - On `Split=t` event: skip (purely notational)
   - After processing all events in a measure, any pending NoteOffs that fall within this measure get emitted at their scheduled tick. Those extending beyond become "carry-forward" for the next measure.

   Actually, this is getting complex. Let me simplify: the MusicXML generator already handles all this correctly. The events are already split and marked. For MIDI:

   **Simplified approach**: Process all measures in one pass with a global tick accumulator and pending NoteOff map:
   - `voiceTick[voice]` tracks current tick per voice
   - `pending[voice][pitch]` tracks the tick at which a NoteOff should be emitted
   - For `EventNote` (Split=f): if pending exists, emit NoteOff at `voiceTick`. Emit NoteOn at `voiceTick`. Set pending NoteOff at `voiceTick + durTicks`. Advance `voiceTick += durTicks`.
   - For `EventNote` (Split=t): advance `voiceTick += durTicks`, don't touch pending.
   - For `EventChord` (Split=f): same per-pitch logic.
   - For `EventChord` (Split=t): advance `voiceTick += durTicks`.
   - At end of all measures: emit NoteOff for all remaining pending entries.

   This naturally handles cross-measure ties because pending entries survive across measure boundaries.

5. **Measure boundary tracking**: As we process events, track the global tick at the start of each measure. Convert to time.Duration using the tempo. This populates `Timeline.MeasureStarts`.

### 1d — Tick to time conversion

```
tickDuration = tickCount / DPPQ           // in quarter notes
seconds       = tickDuration * 60 / bpm    // in seconds
```

A measure's start time = sum of all event tick durations in preceding measures, converted to seconds.

### 1e — Metronome track

A separate SMF track on channel 10 (GM percussion).

For each measure, for each beat:
- **Beat 1** (downbeat): NoteOn(note=76, vel=100) — High Wood Block
- **Other beats**: NoteOn(note=77, vel=80) — Low Wood Block

Beat duration in ticks:
- Simple meter (e.g., 4/4): `DPPQ * 4 / timeDen`
- Compound meter (e.g., 6/8): beat = dotted quarter = `DPPQ * 3 / 2` (3 eighths worth of ticks per beat)

The parser's `ResolveBeatDuration(timeNum, timeDen)` already handles this logic. The beat tick duration = `DPPQ * 4 * beatDuration.Num / beatDuration.Den`.

Metronome notes are short: duration of 1 tick is sufficient for percussion (the DLSMusicDevice will play the full sample regardless).

Metronome events are placed on a separate SMF track. The dlsMIDIPlayer flattens all tracks by absolute tick, so ordering is handled automatically.

### 1f — SMF structure

Format 1 (multiple tracks):
- **Track 0**: Tempo map + time signature meta events
  - `smf.MetaTempo{BPM}` at tick 0 (repeated if tempo changes — future)
  - `smf.MetaMeter{Num, Den}` at each time signature change
- **Track 1**: Score notes (channels 1-3, voices 1-3)
- **Track 2**: Metronome (channel 10)

Uses `gitlab.com/gomidi/midi/v2` for SMF construction (already a transitive dependency via macaudio).

### 1g — MIDI note velocity

- Score notes: velocity 90 (mezzo-forte). The Transport's SetVolume scales this further.
- Metronome: velocity 100 (downbeat), 80 (weak beats). Independent of score volume? Not with current macaudio — volume is global to the DLS unit. Acceptable for MVP; future enhancement could adjust relative velocities.

### 1h — Error handling

- Invalid MIDI pitch (< 0 or > 127): skip event, log warning
- Empty measures: no MIDI events, but metronome track still has clicks
- Zero BPM: return error

### 1i — Testing strategy

**Unit tests** (`midi/generate_test.go`):

1. **SMF validity**: Generate SMF from known DSL inputs, parse back with `smf.ReadFrom`, verify:
   - Correct number of tracks
   - Tempo meta event present with correct BPM
   - Time signature meta events match
   - Note count matches expected (excluding rests and Split events)

2. **Timeline correctness**: For a fixed BPM, verify:
   - Measure start times sum correctly
   - Total duration = last measure start + last measure duration
   - Known example: `M4/4 c d e f` at 120 BPM → 4 quarter notes = 2.0 seconds

3. **Metronome events**: Verify:
   - Correct number of metronome events per measure (equals TimeNum)
   - Downbeat uses note 76, others use note 77
   - Compound meter (6/8): 2 beats (dotted quarters), not 6

4. **Cross-measure ties**: DSL `a - | a b` → NoteOn in M1, no NoteOff until M2's tied note ends

5. **Voice-poly chords**: Events on different voices go to correct MIDI channels

6. **Golden file tests**: Same pattern as existing tests — `.dsl` input + `.expected.midi` binary golden files, or JSON-encoded expected event lists

**Test helper**: `parseSMFEvents(t, smfBytes) []midiEvent` — parse SMF back to a testable struct for assertions.

---

## Phase 2 — Timeline computation (within `midi/` package)

### 2a — `MeasureTiming` struct

```go
type MeasureTiming struct {
    Number    int           // 1-based measure number
    StartTime time.Duration // wall-clock start time at current BPM
    Duration  time.Duration // wall-clock duration of this measure
}
```

### 2b — Computation

Walk all events in all measures, accumulate tick positions per measure. Convert ticks→seconds using BPM. Populate `Timeline.MeasureStarts` array.

### 2c — BPM changes

For MVP, single BPM throughout. The `Timeline` struct holds one `TempoBPM`. Future: support mid-score tempo changes via a `T...` DSL directive, producing multiple tempo sections.

---

## Phase 3 — TUI Application (`cmd/m4bon/tui/`)

### 3a — File structure

```
cmd/m4bon/tui/
├── main.go          # Entry point (flag parsing, app initialization)
├── model.go         # BubbleTea Model struct, Init()
├── view.go          # View() — layout, measure display, indicator
├── update.go        # Update() — message routing, key handling
├── transport.go     # macaudio Transport lifecycle + scheduler callbacks
└── app_test.go      # Integration tests (see Phase 5)
```

### 3b — Model state

```go
type model struct {
    // DSL & parsed data
    dslSource  string                // original DSL text or file path
    dslText    string                // DSL content (read from file if needed)
    measures   []parser.MeasureResult
    renderLines []string             // one string per measure (no ANSI)

    // MIDI & timeline
    smfBytes   []byte
    timeline   midi.Timeline
    bpm        float64               // current tempo, default 120

    // Playback state
    transport  *macaudio.Transport
    midiPlayer macaudio.MIDIPlayer
    scheduler  *macaudio.Scheduler
    audioPlayer macaudio.Player      // for backing tracks
    isPlaying  bool
    position   time.Duration

    // Indicator
    currentMeasure int               // 0-based index into measures
    beatInMeasure  int               // which beat within current measure (future)

    // Recording
    recorder    macaudio.Recorder
    isRecording bool
    recordStart int                  // first measure to record (0-based)
    recordEnd   int                  // last measure to record (inclusive)
    recording   *macaudio.Recording

    // UI
    width, height int
    viewportOffset int               // scroll position for long scores
    showHelp     bool
    mode         string              // "normal", "record-select", "tap-tempo"
}
```

### 3c — Key bindings

Following macaudio's audiodemo patterns (type-based key matching, not string):

| Key | Action |
|-----|--------|
| `space` | Play / Pause |
| `s` | Stop |
| `r` | Toggle recording (while playing) |
| `[` / `]` | Tempo -5 / +5 BPM |
| `{` / `}` | Tempo -1 / +1 BPM |
| `↑` / `↓` | Volume up / down |
| `←` / `→` | Seek -1 measure / +1 measure |
| `1`–`9` | Select measure range (e.g., `1-4` selects measures 1–4 for recording) |
| `esc` | Cancel selection / clear recording range |
| `q` | Quit |
| `?` | Toggle help overlay |
| `j` / `k` | Scroll down / up (for long scores) |
| `0` | Reset tempo to 120 BPM |

### 3d — View layout

```
┌─ m4bon ──── M4/4 ──── KE& ──── ♩=120 ──── vol:80% ──────────────────┐
│                                                                       │
│  1:  e♭₄ f g a♭ b♭ c d e♭                                            │
│  2:  f g a♭ b♭ c d e♭ f                                               │
│▶ 3:  g a♭ b♭ c d e♭ f g                                               │
│  4:  a♭ b♭ c d e♭ f g a♭                                             │
│  5:  b♭ c d e♭ f g a♭ b♭                                             │
│                                                                       │
│  ⏵ Playing  │  Measure 3/5  │  00:04.2 / 00:10.0                      │
│  [space] play/pause  [s] stop  [[] BPM-  []] BPM+  [q] quit  [?] help │
└───────────────────────────────────────────────────────────────────────┘
```

**Top bar**: File name, time signature, key signature, tempo, volume
**Main area**: Measure lines (plain text, split from render output with ANSI stripped or rendered via lipgloss)
**Indicator**: `▶` marker (or similar) on the current measure line
**Bottom bar**: Transport status, position, key hints

### 3e — Indicator design

A prominent pointer (e.g., `▶` or `→`) at the start of the current measure line. The pointer moves discretely at measure boundaries.

**BubbleTea integration**: The Scheduler fires callbacks at each `timeline.MeasureStarts[i]`. Each callback sends a `tea.Cmd` that advances `currentMeasure`. The `View()` renders the pointer on the appropriate line.

```go
// In transport.go
func (m *model) scheduleMeasureCallbacks() {
    for i, startTime := range m.timeline.MeasureStarts {
        measureIdx := i
        m.transport.ScheduleAt(startTime, func() {
            // Send message to advance indicator
            program.Send(advanceIndicatorMsg{measure: measureIdx})
        })
    }
    m.transport.Scheduler().Start(50 * time.Millisecond)
}
```

### 3f — Playback lifecycle

1. User loads DSL file or types DSL
2. Parser runs → `measures` + render output
3. MIDI generator runs at current BPM → `smfBytes` + `timeline`
4. SMF bytes written to temp file, loaded into `MIDIPlayer`
5. MIDIPlayer loaded into `Transport`
6. Measure callbacks scheduled
7. User presses space → `transport.Play()` → scheduler starts
8. Callbacks fire → `advanceIndicatorMsg` → View updates
9. User presses space again → `transport.Pause()`
10. User presses s → `transport.Stop()` → indicator resets to measure 0

### 3g — Tempo change during playback

1. User presses `[` or `]`
2. `bpm` is updated
3. Current position is captured via `transport.Position()`
4. Transport is stopped
5. SMF is regenerated at new BPM
6. New SMF is loaded
7. Transport seeks to the captured position
8. Transport resumes playing
9. Scheduler callbacks are re-registered with new timeline

This is a stop-the-world operation but should complete in <50ms for typical scores.

### 3h — Recording flow

1. User enters "record-select" mode (press `r` when stopped)
2. User types measure range: `2-5` (measures 2 through 5 inclusive)
3. Model stores `recordStart=2`, `recordEnd=5`
4. User presses space to begin playback
5. Playback starts from the beginning of the score
6. When `position >= timeline.MeasureStarts[recordStart]`: start recording
7. When `position >= timeline.MeasureStarts[recordEnd+1]`: stop recording
8. Recording is stored as `macaudio.Recording` in memory
9. User can play back the recording (solo) by pressing a dedicated key

Recording start/stop is driven by the same Scheduler callbacks — just check position against the record window boundaries.

**Implementation note**: Since the Transport can only hold one player at a time, and the Recorder is independent, recording runs concurrently with MIDI playback. The audio output (MIDI) and input (recorder) use separate Core Audio units and don't conflict.

**Playback of recording**: The `Recording` type implements the same interface as `Player`, so it can be swapped into the Transport via `transport.SetRecording(rec)`. During recording playback, the score MIDI won't play — this is solo playback. Future: dual-transport mixing.

### 3i — Audio backing track (future)

When a pre-recorded audio file is loaded alongside a DSL score:

1. User loads both a `.dsl` file and a `.mp3`/`.wav` backing track
2. The backing track plays via `macaudio.Player`
3. Measure start times for the backing track come from a `.beatmap` sidecar file (manually tapped — Phase 6)
4. The indicator advances based on the beatmap, not the DSL timeline

Phase 3 MVP does NOT include this — it's MIDI-only playback with metronome.

---

## Phase 4 — Integration: m4bon CLI + TUI

### 4a — CLI entry point

`cmd/m4bon/main.go` gains a `-tui` flag:

```
m4bon -tui                     # launch TUI with empty state
m4bon -tui -f score.dsl        # launch TUI with pre-loaded score
m4bon -tui -f score.dsl -bpm 96
```

When `-tui` is set, the existing CLI logic is bypassed and the TUI app runs instead.

### 4b — Module dependencies

Add to `go.mod`:
```
require (
    github.com/Michael-F-Ellis/macaudio v0.0.0
    github.com/charmbracelet/bubbletea v1.3.10
    github.com/charmbracelet/lipgloss v1.1.0
    github.com/charmbracelet/bubbles v1.0.0
)
```

The `macaudio` dependency can use a `replace` directive pointing to the local path during development:
```
replace github.com/Michael-F-Ellis/macaudio => ../macaudio
```

---

## Phase 5 — Testing Strategy

### 5a — `midi/` package tests (unit)

**Setup**: Standard `go test` — no hardware required. The `midi` package produces bytes, not audio.

**Test cases**:

| Test name | DSL | BPM | Verifies |
|-----------|-----|-----|----------|
| `TestGenerateSMF_BasicNotes` | `c d e f` | 120 | 4 notes, correct pitches, durations |
| `TestGenerateSMF_WithRests` | `c ; d ;` | 120 | Rests produce no MIDI events |
| `TestGenerateSMF_Sustains` | `a - - b` | 120 | Split events skipped, correct durations |
| `TestGenerateSMF_CrossMeasureTie` | `a - \| a b` | 120 | NoteOn in M1, NoteOff in M2 |
| `TestGenerateSMF_Chords` | `(ceg) f` | 120 | Three NoteOns at same tick, one NoteOn later |
| `TestGenerateSMF_VoicePoly` | `(c - e) f` | 120 | Voice 1 on ch1, voice 3 on ch3 |
| `TestGenerateSMF_Metronome` | `M4/4 c d e f` | 120 | 4 clicks per measure, correct notes |
| `TestGenerateSMF_CompoundMeter` | `M6/8 abc def` | 120 | 2 beats (dotted quarters), 2 clicks |
| `TestGenerateSMF_Timeline` | `M4/4 c d e f \| a b c d` | 120 | M1 start=0, M2 start=2.0s |
| `TestGenerateSMF_Tuplets` | `M4/4 abc def` | 120 | Triplet durations, correct ticks |
| `TestGenerateSMF_EmptyMeasures` | `\|` | 120 | No notes, metronome only |
| `TestGenerateSMF_Tempo` | `c d e f` | 60 | Duration doubled vs 120 BPM |
| `TestGenerateSMF_ErrorZeroBPM` | `c d e f` | 0 | Returns error |

**Golden files**: `.dsl` + expected JSON event list (not binary SMF — too opaque). JSON format:

```json
{
  "bpm": 120,
  "events": [
    {"tick": 0, "type": "note_on", "channel": 1, "note": 60, "velocity": 90},
    {"tick": 480, "type": "note_off", "channel": 1, "note": 60},
    {"tick": 0, "type": "note_on", "channel": 10, "note": 76, "velocity": 100}
  ],
  "measureStarts": [0, 2000000000]
}
```

### 5b — SMF round-trip test

Generate SMF bytes → parse back with `smf.ReadFrom` → verify track count, note count, tempo. This validates the gomidi SMF writing code produces valid files.

### 5c — Render integration test

Existing `render_golden_test.go` pattern: `.dsl` → render output. These continue to work unchanged — the TUI reuses the existing render pipeline.

### 5d — TUI integration tests (limited)

BubbleTea provides `tea.NewTestModel` for sending key events and inspecting model state. Testable behaviors:

- Load DSL → model has correct measure count
- Press space → transport state changes to Playing
- Press s → transport state changes to Idle
- Tempo change → BPM updates, SMF regenerated
- Measure selection → recordStart/recordEnd set correctly

Full audio-path testing (actual Core Audio playback) is deferred to manual smoke testing, matching macaudio's approach (hardware tests tagged `//go:build darwin && cgo`).

### 5e — Test helper: `midi/testhelper.go`

```go
// ParseSMFToEvents parses SMF bytes into a testable struct.
func ParseSMFToEvents(t *testing.T, smfBytes []byte) SMFEventList

type SMFEvent struct {
    Tick    int64
    Channel uint8
    Type    string // "note_on", "note_off", "meta_tempo", "meta_meter"
    Note    uint8
    Velocity uint8
    // ... for meta events
    BPM     float64
    MeterNum int
    MeterDen int
}
```

---

## Phase 6 — Future Enhancements (out of scope for this plan)

### 6a — Pre-recorded audio sync (beatmap)

Manual tap-along to create a `.beatmap` sidecar file mapping measure numbers to audio timestamps. Requires a marker editor UI (inspired by Elephant Soup). Loaded audio plays via `macaudio.Player`, indicator advances via beatmap-driven Scheduler callbacks.

### 6b — Dual Transport / mixing

Play MIDI score + audio recording simultaneously. Requires either two Core Audio output units (current macaudio uses one per player type) or offline mixdown to WAV.

### 6c — Beat-level indicator pulsing

Optional pulse animation on each beat within a measure. Would require beat-level callbacks and a smooth animation tick.

### 6d — Tempo ramps / ritardando

Smooth tempo changes over a range of measures. Requires host-driven MIDI clock (bypassing SMF tempo map), as discussed in planning.

### 6e — Web deployment

m4bon as a backend server + WebAudio frontend. The render IR (`[]Cell`) is format-agnostic — an HTML formatter would parallel the existing ANSI formatter.

---

## Changes by File

| File | Change |
|------|--------|
| `midi/generate.go` | **NEW** — SMF generation, timeline computation, metronome track |
| `midi/generate_test.go` | **NEW** — Unit tests for MIDI generation |
| `midi/testhelper.go` | **NEW** — Test utilities for SMF inspection |
| `cmd/m4bon/tui/main.go` | **NEW** — TUI entry point |
| `cmd/m4bon/tui/model.go` | **NEW** — BubbleTea model |
| `cmd/m4bon/tui/view.go` | **NEW** — Lipgloss layout, measure display, indicator |
| `cmd/m4bon/tui/update.go` | **NEW** — Message routing, key handling |
| `cmd/m4bon/tui/transport.go` | **NEW** — macaudio lifecycle, scheduler callbacks, recording |
| `cmd/m4bon/main.go` | Add `-tui` and `-bpm` flags, route to TUI when set |
| `go.mod` | Add macaudio, bubbletea, lipgloss, bubbles dependencies |
| `m4bon.go` | No changes (public API unchanged) |
| `parser/` | No changes |
| `musicxml/` | No changes |
| `render/` | No changes |
| `test/cases/` | Add MIDI golden files (`.dsl` + `.expected.midi.json`) |

---

## Implementation Order

1. **Phase 0**: Add dependencies, set up build constraints
2. **Phase 1**: Implement `midi/generate.go` with full unit tests
3. **Phase 2**: Timeline computation (part of `midi/` package)
4. **Phase 3**: TUI skeleton — model, view, update with dummy state (no audio)
5. **Phase 3b**: Wire up macaudio transport and scheduler callbacks
6. **Phase 3c**: Recording integration
7. **Phase 4**: CLI flag wiring (`-tui`, `-bpm`)
8. **Phase 5**: Integration tests, golden files, smoke testing

Each phase should be fully tested before moving to the next. The `midi/` package can be built and tested independently of the TUI or macaudio hardware — it's pure data transformation.

---

## Risk Assessment

| Risk | Mitigation |
|------|-----------|
| gomidi v2 SMF writing API differs from expected | Prototype a minimal SMF write in Phase 1 first, confirm API before building full generator |
| Cross-measure tie MIDI logic is error-prone | Heavy unit test coverage for tie scenarios, including multi-measure and voice-poly ties |
| Tempo-change regeneration causes audible glitch | Acceptable for MVP; document as known limitation. Mitigation path: host-driven sequencer in future phase |
| Recording + MIDI playback cause audio dropouts | Core Audio handles concurrent units well on modern Macs; test on target hardware early |
| BubbleTea key handling edge cases | Follow macaudio audiodemo patterns exactly (type-based matching, never `msg.String()`) |
| TUI rendering performance with large scores | Viewport scrolling; only render visible lines; measure lines are pre-computed strings |
