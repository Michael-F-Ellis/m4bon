# Efficient Test/Debug Cycle — Investigation & Implementation Plan

**Date:** 2026-06-14  
**Status:** Proposed

## Current workflow (painful)

| Step | Time (est.) |
|------|-------------|
| Edit `m4bon.qml` | seconds |
| Quit MuseScore | 2s |
| Restart MuseScore | 5–8s |
| Open test score | 3s |
| Enable plugin (Home → Plugins) | 2s |
| Invoke plugin (Plugins → m4bon) | 1s |
| Type DSL, press Send | 5s |
| Evaluate output | 2s |
| **Total per cycle** | **~25s** |

## Goal

Reduce the cycle to **<5 seconds** for parser/logic changes, and **<10 seconds** for changes that need actual MuseScore invocation.

---

## Investigation tasks

### 1. Can MuseScore reload plugins without restarting?

- **Check**: Does the Plugins menu update when a `.qml` file is modified?
- **Check**: Is there a "Reload Plugins" button in the Plugin Manager (Home → Plugins)?
- **Check**: Does toggling the plugin off/on in Plugin Manager pick up file changes?
- **Test**: Edit `m4bon.qml`, toggle plugin in Manager, invoke — does it use the new version?
- **Relevant**: MuseScore watches the plugin directory on some platforms. Check if symlink to dev dir works with inotify/FSEvents.

### 2. Can we test the parser in a standalone JS runtime?

- **Candidate**: Node.js, Deno, or Qt's `qml` command
- **Challenge**: The parser depends on MuseScore-specific APIs (`curScore`, `cursor`, `Element` etc.)
- **Opportunity**: Most of the pipeline is pure functions:
  - `normalizePitchInput()` — strings → strings
  - `tokenize()` — string → token array
  - `parseGroup()` — string → structured object
  - `resolveDurations()` — groups + time signature → events
  - `resolveOctaves()` — events → events with MIDI
  - `splitNonStandardDurations()` — events → events
  - `isStandardDuration()`, `gcd()`, `isPowerOf2()`, `lowerPowerOf2()` — math helpers
- **Task**: Extract these functions into a standalone `.js` file, mock the few MuseScore dependencies (beat resolution needs time signature — provide it as test input), run with Node.js.

### 3. Can we mock the MuseScore API for integration tests?

- **Challenge**: `insertNotes()` depends on `curScore`, `cursor.addNote()`, `cursor.setDuration()`, etc.
- **Approach**: Create a thin mock object implementing the cursor/score API used by our plugin.
- **Minimum mock surface**:
  - `cursor.track`, `cursor.tick` (read/write)
  - `cursor.rewind(mode)`, `cursor.next()`
  - `cursor.setDuration(z, n)`, `cursor.addNote(p, add)`, `cursor.addRest()`
  - `cursor.addTuplet()`, `cursor.measure.timesigNumerator/Denominator`
  - `curScore.startCmd()`, `curScore.endCmd()`, `curScore.newCursor()`
- **Benefit**: Test `insertNotes()` output as a list of API calls and positions.

### 4. Can we run the QML in Qt's qml tool?

- **Check**: Is `qml` (QML Runtime) available with Qt 5/6 SDK?
- **Approach**: Create a minimal test harness QML file that imports the parser logic.
- **Challenge**: MuseScore APIs (`import MuseScore 3.0`) won't be available.
- **Workaround**: Extract parser functions into a separate `.js` file (QML can `import` JS), test with pure JS.

### 5. Is there a MuseScore command-line mode for script testing?

- **Check**: `mscore --help` for script/plugin execution flags.
- **Check**: `mscore --script` or `mscore --plugin` options.
- **Check**: Can we pass input to the plugin from command line?

---

## Recommended implementation (phased)

### Phase 1: Fast-reload investigation (<1 hour)

Try these in order:

1. Toggle the plugin off/on in Plugin Manager after editing the `.qml` file — does it pick up changes?
2. If yes: write a helper script that touches the plugin file or a dependency to trigger reload.
3. If no: research whether MuseScore can be started with a `--watch-plugins` flag or similar.

**Success criteria**: Plugin file change picked up without restarting MuseScore.

### Phase 2: Extract pure logic for standalone testing (1–2 hours)

Create `test/parser-tests.js`:

1. Copy all pure functions from `m4bon.qml` into a standalone JS module.
2. Wrap in a CommonJS or ES module structure that exports functions for testing.
3. Add a test harness using Node.js's built-in `assert` (no framework needed).
4. Write tests for each pipeline stage using the examples from AGENTS.md:
   - `parseGroup("a--b", true)` → `{multiplier:1, slots:[note, sustain, sustain, note]}`
   - `parseGroup("(ace)f", true)` → `{multiplier:1, slots:[chord, note]}`
   - `parseGroup("2abc", true)` → `{multiplier:2, slots:[note, note, note]}`
   - `resolveDurations([...], mockTimeSig)` → expected event list
   - `splitNonStandardDurations([{duration:{5,8}}])` → `[{duration:{1,2}},{duration:{1,8}}]`
   - Error cases: `parseGroup("-", false)` → error, `parseGroup("(ace", true)` → error
5. Run with `node test/parser-tests.js`

**Success criteria**: All parser logic tested in <0.5s without MuseScore.

### Phase 3: Mock MuseScore API for integration tests (1–2 hours)

Create `test/mock-musescore.js`:

1. Implement a mock `Cursor` object that tracks ticks, adds notes to an internal list.
2. Implement a mock `Score` object with `startCmd()/endCmd()` support.
3. Wire `insertNotes()` through the mock and verify the output sequence matches expectations.
4. Test the full pipeline end-to-end: `parseDSL("a - -b c")` through mocked `insertNotes()`.

**Success criteria**: Full pipeline (parse → events → insert) tested in JS, verifying note positions and durations match expected MusicXML output.

### Phase 4: QML/JS module separation (30 minutes)

1. Move pure functions into `parser.js` (a standalone JS file in the plugin directory).
2. In `m4bon.qml`, add `import "parser.js" as Parser`.
3. Call parser functions as `Parser.parseGroup(...)`, etc.
4. Both the test suite and the plugin use the same `parser.js` — impossible to have divergent logic.

**Success criteria**: One source of truth for parser logic, tested independently.

---

## Test example structure

```
m4bon/
├── m4bon.qml              # Plugin UI + MuseScore API calls
├── parser.js              # All pure functions (shared)
├── test/
│   ├── parser-tests.js    # Unit tests for parseGroup, resolveDurations, etc.
│   ├── mock-musescore.js  # Mock Score/Cursor for integration tests
│   └── integration.js     # Full pipeline tests
├── plans/
│   └── test-debug-cycle.md
├── issues/
├── lessons/
└── AGENTS.md
```

---

## Script to reduce manual steps

```bash
#!/bin/bash
# test-plugin.sh — runs parser tests and copies plugin to MuseScore
set -e

echo "=== Running parser tests ==="
node test/parser-tests.js

echo "=== Copying to MuseScore plugins ==="
cp m4bon.qml ~/Documents/MuseScore4/Plugins/m4bon/
cp parser.js ~/Documents/MuseScore4/Plugins/m4bon/ 2>/dev/null || true

echo "=== Done. Toggle plugin in MuseScore Plugin Manager ==="
```

---

## Questions to investigate

1. Does MuseScore 4 watch the plugin directory for file changes, or does it require a restart?
2. Is there an `mscore --reload-plugins` or similar CLI option?
3. Can we use Qt's `qml` or `qmlscene` to run the plugin outside MuseScore with mock objects?
4. Does the `qmllint` tool produce useful error messages without a full Qt SDK?
