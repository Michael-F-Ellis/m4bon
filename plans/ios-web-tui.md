# Web-Based m4bon TUI for iOS — Implementation Plan

**Date:** 2026-06-18
**Status:** Complete — implemented on `ios` branch
**Branch:** `ios`

## Objective

Create a web-based version of the `m4bon -tui` experience that runs on iPad (and all modern browsers). The core pipeline (parser, renderer, MusicXML, MIDI) compiles to WebAssembly. The UI and audio layer are implemented in HTML/CSS/JavaScript using browser-native APIs (Web MIDI, Web Audio). The entire app runs as a single static site — no backend, no server.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  web/                                                                │
│  ┌──────────────────────┐  ┌────────────────────────────────────┐   │
│  │  index.html           │  │  wasm_exec.js + m4bon.wasm         │   │
│  │  • Text area for DSL  │  │  • parser/m4bon.CompileDSL()       │   │
│  │  • Measure grid (CSS) │  │  • render/m4bon.RenderHTML()       │   │
│  │  • Transport controls │  │  • musicxml/m4bon.GenerateXML()    │   │
│  │  • Volume, tempo, etc │  │  • midi/m4bon.GenerateSMF()        │   │
│  └──────────┬───────────┘  └──────────────────┬─────────────────┘   │
│             │                                  │                     │
│             ▼                                  ▼                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  JavaScript layer (web/app.js)                                │   │
│  │  • DSL editing + live parsing via WASM                        │   │
│  │  • Playback: Web MIDI API → SMF bytes → schedule NoteOn/Off  │   │
│  │  • Recording: Web Audio API + getUserMedia                    │   │
│  │  • Keyboard shortcuts (space, [, ], etc.)                     │   │
│  │  • File load/save via <input type=file> + Blob download       │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
┌─────────────────┐  ┌──────────────────┐  ┌─────────────────┐
│  Go → WASM       │  │  Web MIDI API    │  │  Web Audio API   │
│  (pure Go pkgs)  │  │  (browser)       │  │  (browser)       │
│  • parser        │  │  • MIDIOutput    │  │  • AudioContext  │
│  • render        │  │  • schedule()    │  │  • MediaStream   │
│  • musicxml      │  │  • MIDIAccess    │  │  • AnalyserNode  │
│  • midi (SMF)    │  │                  │  │                  │
│  • theory, frac  │  │                  │  │                  │
└─────────────────┘  └──────────────────┘  └─────────────────┘
```

**Key design decision**: The Go WASM binary is the *computational core* only. All I/O (display, audio, file access, keyboard) lives in JavaScript. This mirrors how the macOS TUI uses macaudio for I/O and BubbleTea for display — the Go code is the pipeline, not the shell.

---

## Dependency Audit

| Package | Build Constraint | WASM-compatible? | Notes |
|---|---|---|---|
| `parser/` | none | ✅ | Pure Go |
| `render/` | none | ✅ | Pure Go |
| `musicxml/` | none | ✅ | Pure Go |
| `midi/generate.go` | `darwin && cgo` | ⚠️ | Depends on `gitlab.com/gomidi/midi/v2/smf` which is pure Go. Build constraint is vestigial — it was added because the midi package was only consumed by the macOS TUI. **Remove constraint on ios branch.** |
| `theory/` | none | ✅ | Pure Go |
| `frac/` | none | ✅ | Pure Go |
| `cmd/m4bon/tui/` | `darwin && cgo` | ❌ | Stays macOS-only. Not used on web. |

---

## Phase 1 — WASM Build & Render Pipeline

### 1a — Remove build constraint from midi package

Remove `//go:build darwin && cgo` from `midi/generate.go` and `midi/generate_test.go`. The gomidi SMF library is pure Go — verified by checking its source for CGo/build tags.

### 1b — WASM API Contract

All WASM exports follow a uniform pattern: they take a JSON/string argument from JS and return a JSON result object with an `ok` or `err` field. This lets the JS side handle errors cleanly without Go panics leaking into the browser.

#### `m4bonParse(dslJSON string) string`

Input: `JSON.stringify({ dsl: "M4/4 c d e f" })`

Returns JSON:
```json
{
  "ok": {
    "measures": [
      {
        "timeNum": 4, "timeDen": 4, "fifths": 0,
        "numGroups": 4, "groupSlots": [1,1,1,1], "groupMults": [1,1,1,1],
        "events": [
          {
            "type": "note",
            "letter": "C", "accidental": 0, "effAccidental": 0,
            "midi": 60, "octave": 4, "octaveShift": 0,
            "duration": {"num":1,"den":4}, "split": false,
            "voice": 1, "groupIdx": 0, "numSlots": 1
          }
        ]
      }
    ],
    "keyFifths": 0,
    "timeNum": 4, "timeDen": 4
  }
}
// On error: { "err": "parse error: unexpected ..." }
```

Each event's JSON shape mirrors `parser.Event`:
- `type`: one of `"note"`, `"chord"`, `"rest"`, `"tupletStart"`, `"barline"`
- `letter`: pitch letter (capitalized, e.g. `"C"`, `"F#"`)
- `accidental`: raw accidental from DSL (-1=flat, 0=natural, 1=sharp, -2=dbl-flat, 2=dbl-sharp)
- `effAccidental`: effective accidental after key-signature + measure persistence resolution
- `midi`: MIDI pitch number (note events) or null
- `midis`: array of MIDI pitches (chord events) or null
- `octave`: MIDI/12 - 1 convention (C4=4)
- `octaves`: array of octaves (chord events)
- `duration`: `{num, den}` fraction of whole note
- `split`: continuation flag
- `voice`: 1-based voice index
- `groupIdx`: 0-based beat group index (for render grouping)
- `numSlots`: number of slot positions this event spans

#### `m4bonRenderHTML(dslJSON string) string`

Input: `JSON.stringify({ dsl: "M4/4 c d e f", showSubscripts: true, asciiLeaps: false })`

Returns JSON:
```json
{
  "ok": "<div class=\"m4bon-measures\"><div class=\"m4bon-measure\"><span class=\"m4bon-measure-num\">1.</span><span> c</span><span class=\"subscript\">₄</span> ...</div></div>"
}
```

The HTML mirrors `render.FormatANSI` logic but emits CSS-classed spans:
- `<span class="m4bon-sharp">` (red, rgb(209,34,34))
- `<span class="m4bon-flat">` (blue, rgb(152,140,254))
- `<span class="m4bon-dbl-sharp">` (orange, rgb(255,165,0))
- `<span class="m4bon-dbl-flat">` (green, rgb(4,182,4))
- `<span class="m4bon-sustain-rest">` (grey, rgb(160,160,160))
- `<span class="m4bon-paren">` (med-dark grey, rgb(120,120,120))
- `<span class="m4bon-italic">` for chord pitch letters
- `<sub class="m4bon-octave">` for octave subscripts
- `<span class="m4bon-leap-up">` / `<span class="m4bon-leap-down">` for leap indicators
- `<span class="m4bon-measure-num">` for measure numbers

On error: `{ "err": "..." }`

#### `m4bonGenerateSMF(dslJSON string) string`

Input: `JSON.stringify({ dsl: "M4/4 c d e f", bpm: 120, metronome: true, roots: false, backbeats: false })`

Returns JSON with a flat MIDI event list (already sorted by tick) rather than raw SMF bytes — avoids JS SMF parsing per the user's decision:
```json
{
  "ok": {
    "events": [
      {"tick": 0,    "type": "noteOn",  "channel": 0, "pitch": 60, "velocity": 100},
      {"tick": 480,  "type": "noteOff", "channel": 0, "pitch": 60, "velocity": 0},
      {"tick": 0,    "type": "noteOn",  "channel": 9, "pitch": 76, "velocity": 100},
      {"tick": 0,    "type": "metaTempo", "bpm": 120},
      {"tick": 0,    "type": "metaMeter", "num": 4, "den": 4}
    ],
    "measureStarts": [0, 480],
    "totalDuration": 480,
    "tempoBPM": 120
  }
}
```

Key details:
- `tick` values are cumulative (absolute) ticks at 480 DPPQ
- Channel 9 (0-indexed) = GM percussion/metronome
- `measureStarts` is in **seconds** (float), computed from ticks + BPM
- Events are sorted by tick
- JS side converts `measureStarts[i]` + `performance.now()` to schedule playback

#### `m4bonGenerateXML(dslJSON string) string`

Input: `JSON.stringify({ dsl: "M4/4 c d e f" })`

Returns JSON:
```json
{
  "ok": "<?xml version=\"1.0\"?><!DOCTYPE ...><score-partwise>...</score-partwise>"
}
```

### 1c — WASM wrapper implementation (`wasm/main.go`)

```go
//go:build js && wasm

package main

import (
    "encoding/json"
    "syscall/js"
    "github.com/mellis/m4bon/parser"
    "github.com/mellis/m4bon/render"
    "github.com/mellis/m4bon/musicxml"
    "github.com/mellis/m4bon/midi"
)

// Each handler:
// 1. js.ValueOf(this).Get("value").String() to get the single string argument
// 2. json.Unmarshal into a typed request struct
// 3. Call the existing pipeline (parser.ParseDSL, render.Render, etc.)
// 4. json.Marshal the response struct with either "ok" or "err"
// 5. Return JSON string

func main() {
    c := make(chan struct{})
    js.Global().Set("m4bonParse", js.FuncOf(parseWrapper))
    js.Global().Set("m4bonRenderHTML", js.FuncOf(renderHTMLWrapper))
    js.Global().Set("m4bonGenerateSMF", js.FuncOf(smfWrapper))
    js.Global().Set("m4bonGenerateXML", js.FuncOf(xmlWrapper))
    <-c
}
```

The `parseWrapper` is the most complex — it must convert `parser.MeasureResult` + `parser.Event` into the JSON shape defined above. All other wrappers are thin adapters around existing Go functions.

**Important**: `syscall/js` has no `context.Context` and limited goroutine support. Keep WASM handlers synchronous and simple — no goroutines, no channels beyond the main blocking one.

### 1d — Build script

```bash
# Build WASM (from repo root)
GOOS=js GOARCH=wasm go build -o web/m4bon.wasm ./wasm/

# Copy Go's WASM JS glue (needed by the browser to run .wasm)
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/

# Wasm must be served with Content-Type: application/wasm
# Python's http.server handles this correctly.
```

### 1e — HTML render formatter (NEW: `render/html.go`)

Following the existing two-layer render architecture (IR → formatter):
- `render/html.go` produces `<span>` elements with CSS classes instead of ANSI escapes
- Follows the exact same pattern as `render/ansi.go`'s `FormatANSI` function
- Signature: `func FormatHTML(measures []CellSeq, asciiLeaps bool) string`
- For three-column layout (chord/lyric directives): `func FormatHTMLRows(rows []MeasureRow, maxChordW, maxNoteW, maxLyricW int, asciiLeaps bool) string`
- Calls `render.Render()` which internally calls `BuildRows` + `FormatANSIRows` — the HTML path mirrors this with `BuildRows` + `FormatHTMLRows`

### 1f — Test on macOS

Development loop:
```bash
# Terminal 1: build + serve
GOOS=js GOARCH=wasm go build -o web/m4bon.wasm ./wasm/ && python3 -m http.server -d web 8080

# Terminal 2: open in Chrome
open -a "Google Chrome" http://localhost:8080
```

Chrome supports Web MIDI API on macOS — full testing possible locally. Safari does NOT support Web MIDI; testing on iPad requires Chrome for iPad (user confirmed acceptable).

---

## Phase 2 — Basic Web UI

### 2a — Static HTML shell (`web/index.html`)

- CSS grid layout: top bar (time/key/tempo), measure area (scrollable), status bar
- Text area for DSL input (live-parses on keystroke with debounce)
- Measure display area: each measure rendered as a horizontal row of styled `<span>` elements
- Keyboard shortcuts handled via `keydown` events

### 2b — JavaScript app (`web/app.js`)

```javascript
class M4bonApp {
    constructor() {
        this.dsl = '';
        this.bpm = 120;
        this.startMeasure = 0;
        this.endMeasure = 0;
        this.currentMeasure = 0;
        this.showSubscripts = true;
        this.metronomeOn = true;
        this.isPlaying = false;
        this.parsedData = null; // last successful parse result
    }

    parseAndRender(dsl) {
        const result = JSON.parse(m4bonParse(JSON.stringify({ dsl })));
        if (result.err) {
            this.showError(result.err);
            return;
        }
        this.parsedData = result.ok;
        this.updateTopBar();   // time sig, key sig from parsedData
        this.updateMeasures(); // calls m4bonRenderHTML for HTML output
        this.updateStatus();   // measure count, error-free
    }

    updateMeasures() {
        const html = JSON.parse(m4bonRenderHTML(JSON.stringify({
            dsl: this.dsl,
            showSubscripts: this.showSubscripts,
            asciiLeaps: false
        })));
        if (html.ok) {
            this.measuresEl.innerHTML = html.ok;
        }
    }

    updateTopBar() {
        const d = this.parsedData;
        this.topBarEl.textContent = `M${d.timeNum}/${d.timeDen} | K${keyName(d.keyFifths)} | ♩=${this.bpm}`;
    }

    // Playback via Web MIDI API (Phase 3)
    // Recording via Web Audio API (Phase 4)
}
```

### 2c — Measure indicators

- Green bold bracket for start measure
- Grey bracket for end measure
- Animated cursor for current playback position
- All rendered in CSS (no Canvas needed — text-based like the TUI)

### 2d — Styling

Match the TUI's lipgloss aesthetics in CSS:
- Top bar: purple (#6366f1) background, white text
- Muted text: `#6b7280`
- Font: monospace (`SF Mono`, `Menlo`, `Consolas`)
- Dark background: `#1e1e2e` (Catppuccin Mocha base)

---

## Phase 3 — MIDI Playback

### 3a — Web MIDI API integration

The WASM side returns a flat JSON event list (see Phase 1b contract). JS converts tick positions to wall-clock offsets using BPM, then schedules them via `output.send()`:

```javascript
async play() {
    const access = await navigator.requestMIDIAccess();
    const output = access.outputs.values().next().value;
    if (!output) throw new Error("No MIDI output available");
    
    const result = m4bonGenerateSMF(JSON.stringify({
        dsl: this.dsl, bpm: this.bpm,
        metronome: this.metronomeOn,
        roots: this.rootsOn,
        backbeats: this.backbeatsOn
    }));
    const data = JSON.parse(result);
    if (data.err) throw new Error(data.err);
    
    const { events, measureStarts, tempoBPM } = data.ok;
    const startTime = performance.now();
    
    // Tick-to-seconds: seconds = ticks * 60 / (480 * bpm)
    const tickToSec = 60.0 / (480.0 * tempoBPM);
    
    for (const ev of events) {
        const delay = ev.tick * tickToSec * 1000; // ms
        const midiBytes = midiEventToBytes(ev);
        output.send(midiBytes, startTime + delay);
    }
    
    // Schedule playback-end callback
    const totalMs = measureStarts[measureStarts.length - 1] * 1000 + 
        (events[events.length-1].tick * tickToSec * 1000);
    this.playbackTimer = setTimeout(() => this.onPlaybackEnd(), totalMs);
}
```

### 3b — MIDI event encoding (JS)

A small helper converts the JSON event to raw MIDI bytes:

```javascript
function midiEventToBytes(ev) {
    switch (ev.type) {
        case 'noteOn':
            return [0x90 | ev.channel, ev.pitch, ev.velocity];
        case 'noteOff':
            return [0x80 | ev.channel, ev.pitch, ev.velocity];
        // Meta events (tempo, meter) are informational — no bytes sent
        default:
            return [];
    }
}
```

No SMF parsing needed — the WASM side produces the event list directly.

### 3c — Transport controls

- Space: play/pause (cancel/reschedule pending events)
- s: stop
- [ / ]: tempo ±5 BPM
- { / }: tempo ±1 BPM
- 0: reset tempo to 120
- up/down: move start measure
- shift+up/down: move end measure
- left/right: volume ±5%

### 3d — Metronome

Schedule metronome click events alongside note events, using the same channel 10 / GM percussion mapping as the macOS TUI. Toggle with 'm' key.

---

## Phase 4 — Audio Recording

### 4a — Web Audio API recording

```javascript
async startRecording() {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    this.mediaRecorder = new MediaRecorder(stream);
    this.audioChunks = [];
    
    this.mediaRecorder.ondataavailable = (e) => this.audioChunks.push(e.data);
    this.mediaRecorder.onstop = () => this.processRecording();
    
    this.mediaRecorder.start();
}
```

### 4b — Recording flow

1. User selects start/end measures (same as TUI)
2. Press 'r' to arm recording
3. Metronome count-in plays
4. Recording captures audio from start to end measure
5. Recording stored as WAV/Opus blob — can be downloaded or replayed

### 4c — Limitations vs. macOS TUI

- No pitch analysis (the macOS TUI doesn't do this either — it records raw audio)
- No overdubbing (macOS TUI uses macaudio.MultiTrack, browser equivalent would need Web Audio offline rendering)
- Recording quality depends on device mic

---

## Phase 5 — Polish & Feature Parity

### 5a — File handling

- **Load .dsl**: `<input type="file">` reader
- **Save .mxl**: Blob download of MusicXML
- **No file watching**: browsers have no general filesystem access. Instead: auto-save to `localStorage` on each edit.

### 5b — Subscript toggle

- 'o' key toggles octave subscripts on/off (same as TUI)
- Renders measure display with/without subscript `<sub>` elements

### 5c — Voice-poly chord display

Match the TUI's chord rendering: parentheses in medium-dark grey, italic pitch letters, ANSI color codes for accidentals — all via CSS classes.

### 5d — Key signature display

Show in top bar (e.g., "K +2 (D major)") — parse the key name from the fifths count, same as TUI.

### 5e — Touch support

For iPad:
- Tap to place start/end measure indicators
- Swipe to scroll measure view
- On-screen transport buttons (in addition to keyboard shortcuts)
- Larger touch targets (44pt minimum)

---

## File Layout (ios branch, additive only)

```
m4bon/
├── web/                          # NEW — all web assets
│   ├── index.html                # Main page
│   ├── app.js                    # JavaScript application
│   ├── app.css                   # Stylesheet
│   ├── wasm_exec.js              # Go WASM JS glue (copied from GOROOT)
│   └── m4bon.wasm                # Compiled WASM binary (gitignored, built)
├── wasm/                         # NEW — WASM entry point
│   └── main.go                   # syscall/js wrappers
├── render/
│   ├── cell.go                   # (unchanged) Cell IR types
│   ├── render.go                 # (unchanged) Core renderer
│   ├── ansi.go                   # (unchanged) ANSI formatter
│   ├── html.go                   # NEW — HTML formatter
│   └── html_test.go               # NEW — HTML formatter tests
├── midi/
│   └── generate.go               # MODIFIED — remove build constraint
├── plans/
│   └── ios-web-tui.md            # This file
└── .gitignore                    # MODIFIED — add web/m4bon.wasm
```

**No existing files are modified** except `midi/generate.go` (remove build constraint) and `.gitignore`.

---

## Testing Strategy on macOS

| Layer | Test method | Command |
|---|---|---|
| Go WASM build | `go build` with GOOS=js | `GOOS=js GOARCH=wasm go build ./wasm/` |
| Go unit tests | Standard `go test` | `go test ./parser/ ./render/ ./musicxml/ ./midi/` |
| HTML render formatter | Unit tests | New `render/html_test.go` |
| Web UI + WASM | Serve + open in Chrome | `python3 -m http.server -d web 8080` |
| Web MIDI playback | Chrome's Web MIDI API | Connect a MIDI synth or use macOS built-in DLS synth |
| Recording | Chrome's Web Audio API | Microphone access prompt |
| Touch/iPad | Xcode iOS Simulator + Safari | `xcrun simctl openurl booted http://localhost:8080` |

**Rapid iteration**: The Go → WASM build takes ~1s. Set up a file watcher (or just re-run the build command). The Python HTTP server auto-reloads on file change. Edit Go → rebuild → refresh browser.

---

## Open Questions

1. **Web MIDI API on iPad Safari**: As of iOS 17, Safari does NOT support Web MIDI API. Options:
   - Tell users to use Chrome for iPad (supports Web MIDI)
   - Use a polyfill / Web Bluetooth MIDI bridge
   - Accept MIDI playback as desktop-only, make recording the primary iPad audio feature
   - *USER SAYS* Chrome limitation is perfectly acceptable.
   
2. **SMF parsing in JS vs JSON**: Should the WASM side return parsed events as JSON (avoiding JS SMF parsing) or raw SMF bytes (letting JS parse)? JSON is simpler and avoids a dependency. Raw SMF is more "correct" but requires a JS parser.
    - *USER SAYS:*  JSON, please.

3. **PWA packaging**: Should the web app be a PWA (installable, offline-capable)? This would require a service worker and manifest. Probably Phase 6.
    - *USER AGREES* Defer for now.

4. **Native wrapper**: For App Store distribution, wrap the static site in a WKWebView Swift app. This adds code signing and provisioning complexity — defer until the web version is feature-complete.
    - *USER AGREES* Defer for now.
---

## What We're NOT Doing

- **No backend server**: Everything runs client-side
- **No database**: DSL files stored in `localStorage` or loaded from file picker
- **No user accounts**: No auth, no cloud sync
- **No React/framework**: Vanilla JS keeps the payload tiny (~200KB .wasm + ~50KB JS/CSS)
- **No rewrite of existing Go code**: The parser, renderer, MusicXML, and MIDI generation stay exactly as-is

---

## Lessons Learned (2026-06-18 session)

### WASM Bootstrap
- `wasm_exec.js` only provides the `Go` class; you must explicitly `fetch()` + `WebAssembly.instantiate()` + `go.run()` the .wasm binary.
- `go.run()` is async; after the first `_resume()` yield, all `js.Global().Set(...)` calls from `main()` are visible. Use `requestAnimationFrame` polling or a readiness flag (`_m4bonReady`).
- The MIME type for .wasm must be `application/wasm` for `WebAssembly.instantiateStreaming`, but using `fetch()` + `arrayBuffer()` + `WebAssembly.instantiate(bytes, ...)` avoids this requirement.

### MIDI Event JSON Serialization
- Never use `json:"omitempty"` on integral fields that can be zero — MIDI channel 0 and pitch 0 are valid values. They were silently dropped from JSON, causing all `midiEventToBytes()` calls to return empty arrays.

### Web Audio API Pre-Scheduling vs SetTimeout
- `BufferSourceNode.start(futureTime)` + `GainNode.setValueAtTime(...)` creates pre-scheduled nodes that CANNOT be cancelled by gain changes or `disconnect()` — the internal AudioContext schedule is immutable.
- For cancellable playback, use `setTimeout` to fire note events at wall-clock times. Store timer IDs and clear on stop.
- This matches the hellogoth pattern where MIDIPlayer uses its own scheduling loop.
- The `WebAudioFontPlayer.queueWaveTable()` calls from the piano path are pre-scheduled and thus NOT cancellable — but they're short-duration notes that aren't noticeable if a few ring after stop.

### Soundfont Quality — The CDN Gap
- The WebAudioFont CDN has complete GM drums (JCLive), but instrument programs 24-39 (guitars and basses) are all 404 across GeneralUserGS, JCLive, and Ntonyx soundfonts.
- FluidR3_GM exists on the CDN for a subset of programs (piano works), but bass programs 32-39 are also missing.
- The full FluidR3_GM.sf2 (142MB) is available from archive.org. Using it requires a SoundFont2 WASM renderer (TinySoundFont), which would add ~100KB compiled + the .sf2 download.
- **Interim solution**: Multi-sampled electric bass from tonejs-instruments (nbrosowsky/tonejs-instruments on GitHub). 4 ogg samples (E1, G1, A♯1, C♯2, ~1.6MB total) played via `AudioBufferSourceNode` with `playbackRate` pitch shifting. ≤3 semitone gaps produce acceptable quality.
- The TUI uses Apple's built-in DLS Synth via CoreMIDI — a high-quality system sampler with complete GM. The web cannot match this without a full SoundFont2 renderer.

### Measure Highlight Timing
- `measureStarts` from the Go `GenerateEventList` is in seconds (unlike the tick-based events array). Don't divide seconds by `tickToSec` — that converts to ticks, making highlight comparisons off by ~480×.
- Apply range offset (`measureStarts[i] - measureStarts[startMeasure]`) to align with wall-clock elapsed time during playback.

### HTML Render Layout
- Inline `<span>` elements with manual space padding don't align columns across separate block-level `<div>` elements. Use CSS table layout (`display: table`, `table-row`, `table-cell`) for cross-measure column alignment.
- A hidden header row with empty cells is needed to establish column widths when using table layout.
- Measure numbers should be wrapped in `<span class="m4bon-measure-num">` for CSS styling (subtle grey).
- Playing measure highlight should use `rgba(0,0,0,0.3)` — a subtle darken overlay that doesn't compete with sharp/flat color coding.

### WebAudioFontPlayer Instrument Loading
- `startLoad()` queues instrument scripts; `waitLoad()` resolves once all pending scripts have loaded.
- Calling `waitLoad()` twice (once per instrument) causes the second call to hang forever if there are no pending loads. Queue all instruments first, then call `waitLoad()` once.
- Instruments loaded via `startLoad` appear in `window['_tone_XXXX_SoundFontName']`; drums appear in `window['_drum_KEY']` with a different URL pattern.
- The loader uses script tag injection; `onerror` is not handled, so missing instruments silently fail.
