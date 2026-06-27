# Look-Ahead Scheduler for Web Audio Playback — Implementation Plan

**Date:** 2026-06-26
**Status:** Draft

## Objective

Replace the current "schedule all notes at once" approach in `play()` with a
look-ahead scheduler that polls `audioCtx.currentTime` every ~50ms and only
schedules the next ~200ms of notes. This eliminates accumulated timing drift
that causes Safari's Web Audio to become unreliable during recording (when
`MediaRecorder` competes for audio resources).

**Single code path for all browsers.** Chrome, Firefox, and Safari all benefit
from look-ahead scheduling — it's the standard approach used by professional
Web Audio sequencers.

## Current Architecture

```
play()
  → m4bonGenerateSMF (WASM) → { events, measureStarts, tempoBPM }
  → pre-scan: match noteOn/noteOff pairs → scheduledNotes[]
  → for each scheduledNotes: scheduleNote(ch, pitch, vel, startTime, duration)
      → wafPlayer.queueWaveTable(audioCtx, masterGain, preset, startTime, ...)
  → _startHighlightTimer()
  → setTimeout(onPlaybackEnd, totalSec * 1000)
```

All notes are scheduled at `play()` time with absolute `startTime` values
relative to `audioCtx.currentTime + 0.05`. If the audio clock drifts (Safari
during recording), notes scheduled minutes ahead may land at wrong times.

## New Architecture

```
play()
  → m4bonGenerateSMF (WASM) → { events, measureStarts, tempoBPM }
  → pre-scan: match noteOn/noteOff pairs → scheduledNotes[]
  → _startHighlightTimer()
  → _scheduleLookAhead()        // first tick immediately
      → setTimeout(_scheduleLookAhead, 50)  // recursive, not setInterval
      → now = audioCtx.currentTime
      → windowEnd = now + LOOK_AHEAD (200ms)
      → while nextIdx < scheduledNotes.length:
          → if scheduledNotes[nextIdx].startTime < windowEnd:
              → scheduleNote(...)
              → nextIdx++
          → else: break
      → if nextIdx >= scheduledNotes.length:
          → setTimeout(onPlaybackEnd, lastNoteEndTime + 0.5s)  // final tail
          → return (stop recursing)
  → setTimeout(onPlaybackEnd, totalSec * 1000)   // safety fallback
```

## Constants

```javascript
const SCHEDULER_INTERVAL_MS = 50;   // how often the scheduler runs
const LOOK_AHEAD_SEC = 0.200;       // schedule notes this far ahead
```

## Detailed Changes

### 1. New state fields in constructor

Add to `M4bonApp` constructor:

```javascript
this._scheduledNotes = null;   // pre-scanned note array (from play())
this._schedulerIdx = 0;        // next index to schedule
this._schedulerTimer = null;   // setTimeout handle for _scheduleLookAhead
this._playbackEndTime = 0;     // wall-clock time when last note ends
```

### 2. New method: `_scheduleLookAhead()`

```javascript
_scheduleLookAhead() {
  if (!this.isPlaying) return;
  if (!this.audioCtx) return;

  const now = this.audioCtx.currentTime;
  const windowEnd = now + LOOK_AHEAD_SEC;
  const notes = this._scheduledNotes;
  const len = notes.length;

  let scheduled = false;
  while (this._schedulerIdx < len) {
    const n = notes[this._schedulerIdx];
    // Notes with startTime < now + small margin: schedule immediately
    // with a 10ms grace to avoid glitches from slight clock differences
    if (n.startTime < windowEnd) {
      this.scheduleNote(n.channel, n.pitch, n.velocity, n.startTime, n.duration);
      this._schedulerIdx++;
      scheduled = true;
    } else {
      break;
    }
  }

  // If we scheduled nothing and still have notes, force-schedule the next
  // one (prevents stalls if audio clock jumps forward during recording).
  // Use a minimum startTime of now + 0.01 to avoid scheduling in the past.
  if (!scheduled && this._schedulerIdx < len) {
    const n = notes[this._schedulerIdx];
    const start = Math.max(n.startTime, now + 0.01);
    this.scheduleNote(n.channel, n.pitch, n.velocity, start, n.duration);
    this._schedulerIdx++;
  }

  // All done?
  if (this._schedulerIdx >= len) {
    // Schedule onPlaybackEnd at last note end + 0.5s tail
    const lastNote = notes[len - 1];
    const tail = (lastNote.startTime + lastNote.duration) - now + 0.5;
    this.playbackTimer = setTimeout(() => this.onPlaybackEnd(), Math.max(tail * 1000, 500));
    return;
  }

  // Keep scheduling
  this._schedulerTimer = setTimeout(() => this._scheduleLookAhead(), SCHEDULER_INTERVAL_MS);
}
```

### 3. Changes to `play()` — scheduling section only

**Remove** lines 608–642 (the pre-scan + schedule loop). **Replace** with:

```javascript
// Pre-scan: collect note-on/note-off pairs into a flat schedule array.
// Each entry has absolute wall-clock start/duration for direct scheduling.
const scheduledNotes = [];
const pendingNotes = {}; // key: "ch-pitch" -> [{ tick, velocity }]

for (const ev of events) {
  if (ev.tick < startTick || ev.tick >= endTick) continue;
  if (ev.type === 'metaTempo' || ev.type === 'metaMeter') continue;

  if (ev.type === 'noteOn') {
    const key = ev.channel + '-' + ev.pitch;
    if (!pendingNotes[key]) pendingNotes[key] = [];
    pendingNotes[key].push({ tick: ev.tick, velocity: ev.velocity || this.velocity });
  } else if (ev.type === 'noteOff') {
    const key = ev.channel + '-' + ev.pitch;
    if (pendingNotes[key] && pendingNotes[key].length > 0) {
      const onset = pendingNotes[key].shift();
      let duration = (ev.tick - onset.tick) * tickToSec;
      if (duration <= 0) duration = 0.05;
      scheduledNotes.push({
        channel: ev.channel,
        pitch: ev.pitch,
        velocity: onset.velocity,
        startTime: startWall + (onset.tick - startTick) * tickToSec,
        duration: duration
      });
    }
  }
}

// Flush remaining pending notes
for (const key in pendingNotes) {
  const list = pendingNotes[key];
  const [ch, pitch] = key.split('-').map(Number);
  for (const onset of list) {
    scheduledNotes.push({
      channel: ch,
      pitch: pitch,
      velocity: onset.velocity,
      startTime: startWall + (onset.tick - startTick) * tickToSec,
      duration: 1.0
    });
  }
}

// Sort by startTime (should already be sorted, but ensure it)
scheduledNotes.sort((a, b) => a.startTime - b.startTime);

this._scheduledNotes = scheduledNotes;
this._schedulerIdx = 0;
this._schedulerTimer = null;
```

The `this.isPlaying = true` and UI updates stay in place, followed by:

```javascript
// Start the look-ahead scheduler (replaces the old synchronous schedule loop)
this._scheduleLookAhead();

// Set a safety timeout as fallback (scheduler handles normal completion)
const lastTick = scheduledNotes.length > 0
  ? scheduledNotes[scheduledNotes.length - 1].startTime + scheduledNotes[scheduledNotes.length - 1].duration
  : startWall;
const safetySec = (lastTick - startWall) + 2.0 + countInSec;
this.playbackTimer = setTimeout(() => this.onPlaybackEnd(), safetySec * 1000);
```

The `_startHighlightTimer()`, `_playMeasureStarts`, and recording setup code stay unchanged.

### 4. Changes to `stop()`

Add cleanup of the scheduler timer:

```javascript
if (this._schedulerTimer) {
  clearTimeout(this._schedulerTimer);
  this._schedulerTimer = null;
}
this._scheduledNotes = null;
this._schedulerIdx = 0;
```

(Insert after the existing `clearTimeout(this.playbackTimer)` block.)

### 5. Edge cases handled

| Scenario | Handling |
|----------|----------|
| User presses Stop mid-playback | `stop()` clears scheduler timer + cancels envelopes |
| Audio clock jumps forward (Safari recording) | Force-schedule fallback: if nothing scheduled this tick, force the next note at `max(startTime, now + 0.01)` |
| Very short DSL (few notes) | Scheduler completes quickly, `onPlaybackEnd` fires via tail timeout |
| Very long DSL (many minutes) | Scheduler loops indefinitely in 50ms ticks until done |
| Count-in clicks | Unchanged — `_scheduleClick()` calls remain outside the scheduler |
| `startMeasure`/`endMeasure` range | Unchanged — `startTick`/`endTick` filtering happens before the pre-scan |
| Bass notes (channel 8) | Unchanged — `scheduleNote()` delegates to `_playBassNote()` internally |

### 6. What stays the same

- `scheduleNote()` method — no changes
- `_scheduleClick()` (metronome) — no changes  
- `_startHighlightTimer()` / `_updatePlayHighlight()` — no changes
- `onPlaybackEnd()` — no changes
- `stop()` logic for envelopes, bass, MIDI — no changes (only add scheduler cleanup)
- Recording path (`toggleRecord`, `startRecording`, `_startRecordingHighlight`) — no changes
- SMF generation (WASM) — no changes

## Testing Checklist

1. **Playback on Chrome**: Full-piece playback with metronome. Notes should play without glitches.
2. **Playback on Safari**: Same — verify no regression.
3. **Recording on Safari**: Record a multi-measure piece. Playback should have even timing, no dropped/muted notes, even metronome.
4. **Recording on Chrome**: Verify no regression.
5. **Stop mid-playback**: Press Stop during playback. All audio should stop immediately. Re-play should work.
6. **Range playback**: Set start/end measure range, verify only selected range plays.
7. **Count-in**: Verify count-in clicks play before the first measure.
8. **Bass/roots**: Verify chord root bass notes play on channel 8.
9. **Drum channel**: Verify metronome clicks on channel 9 play correctly.
10. **Empty/error DSL**: Verify error handling still works (no crash).

## Known Risks

- The `queueWaveTable` envelopes tracked in `activeEnvelopes` use a key `channel-pitch-startTime`. With the scheduler, `startTime` might be slightly adjusted by the force-schedule fallback. The key will be different, but this is fine — stop cancellation iterates all keys.
- The force-schedule fallback (`max(n.startTime, now + 0.01)`) could theoretically cause slight timing shifts if Safari's clock is severely out of sync. In practice, 10ms is below human perception threshold for musical timing.
