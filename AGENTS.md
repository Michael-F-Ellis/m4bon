# m4bon — Agent Guide

## Project Overview

MuseScore 4 plugin for **beat-oriented note entry**. Users type rhythmic patterns in a compact DSL where whitespace separates beats and characters within a beat subdivide it equally. Press **Send** to insert the resulting notes at the current cursor position.

**Tech stack:** QML, MuseScore 3.0 Plugin API (backward-compatible in MS4), QtQuick 2.9, QtQuick.Controls 2.2, QtQuick.Layouts 1.3

---

## Directory Structure

```
m4bon/
├── m4bon.qml          # Main plugin — symbol constants, DSL parser, UI, score insertion
├── README.md
├── AGENTS.md
├── LICENSE            # MIT
└── .gitignore         # Qt/C++ build artifacts (template leftovers)
```

All logic lives in `m4bon.qml` — a single `MuseScore` QML component.

---

## DSL Reference

### Core principle

**Whitespace separates beats.** Characters grouped without spaces subdivide that beat equally. You never specify explicit durations — the grouping and the score's time signature determine everything.

### Symbol table

All symbols defined as QML `property string` constants at the top of `MuseScore`. Override to fork with a different character set.

| Purpose             | Symbol | Property         |
|---------------------|--------|------------------|
| Sharp               | `#`    | `symSharp`       |
| Flat                | `&`    | `symFlat`        |
| Natural             | `%`    | `symNatural`     |
| Octave up           | `^`    | `symUpOctave`    |
| Octave down         | `/`    | `symDownOctave`  |
| Sustain / tie       | `-`    | `symSustain`     |
| Rest                | `;`    | `symRest`        |
| Chord open          | `(`    | `symChordOpen`   |
| Chord close         | `)`    | `symChordClose`  |
| Barline (syntactic sugar, ignored) | `|`    | `symBarline`     |

There is **no partial-beat symbol**. The tuple prefix (see below) handles all asymmetric groupings.

---

### Time signature → beat resolution

The parser reads `curScore`'s time signature to determine the **beat duration**:

| Denominator | Numerator % 3 == 0? | Beat unit                    | Example meters |
|-------------|----------------------|------------------------------|----------------|
| 2           | —                    | half note                    | 2/2, 3/2 |
| 4           | —                    | quarter note                 | 2/4, 3/4, 4/4 |
| 8           | yes (compound)       | dotted quarter (3 eighths)   | 6/8, 9/8, 12/8 |
| 8           | no                   | eighth note                  | 5/8, 7/8, 11/8 |
| 16          | yes (compound)       | dotted eighth (3 sixteenths) | 6/16, 9/16, 12/16 |
| 16          | no                   | sixteenth note               | 5/16, 7/16 |

---

### Beat groups

Each whitespace-separated token is a **beat group**. By default, a group spans **1 beat**. An optional numeric prefix `N` makes it span **N beats**.

```
group       →  spans 1 beat
Ngroup      →  spans N beats
```

Within its span, the group's characters equally divide the total duration.

#### Examples in 4/4 (beat = quarter)

```
a b         →  a spans 1 quarter, b spans 1 quarter
                →  quarter + quarter
ab          →  ab spans 1 quarter, 2 chars
                →  2 eighths
abc         →  abc spans 1 quarter, 3 chars
                →  triplet eighths
a bcd       →  a = quarter, bcd spans 1 quarter ÷ 3
                →  quarter + triplet eighths
a--b        →  a--b spans 1 quarter, 4 char positions
                a occupies positions 0-2 = 3/4 beat = dotted eighth
                b occupies position 3    = 1/4 beat = sixteenth
2abc        →  2abc spans 2 quarters, 3 chars
                →  quarter-note triplet (hemiola)
a -;        →  a = quarter (beat 1)
                -; spans 1 quarter, 2 positions:
                  - extends a for 1/2 beat (eighth)
                  ; = rest for 1/2 beat (eighth)
                →  dotted quarter tied + eighth rest
(ace)f      →  (ace)f spans 1 quarter, 2 positions:
                  (ace) = A-minor chord for 1/8
                  f     = single note for 1/8
```

#### Examples in 5/8 (beat = eighth, non-compound)

```
3abc 2ab    →  3abc spans 3 eighths, 3 chars → each = 1/8
                2ab spans 2 eighths, 2 chars → each = 1/8
                → 3 eighths + 2 eighths = 5/8, grouping 3+2
```

The `N` prefix tells the parser the metric grouping explicitly. Without prefixes in 5/8, a group with no prefix spans 1 eighth, so `a b` = 2 eighths.

#### Examples in 6/8 (beat = dotted quarter = 3 eighths, compound)

```
abc def     →  each group spans 1 dotted quarter = 3/8, 3 chars
                →  each char = 1 eighth
                → 2 groups of 3 eighths = 6/8
```

```
3abc 3def   →  same result — compound meter defaults already give
                each group 3/8, so the prefix is redundant but valid
```

---

### Sustain (`-`)

The sustain symbol extends the **previous pitch** through its position slot(s). It does not create a new note.

```
a--b  →  4 char positions in this group
          pos 0: a (attacks)
          pos 1: - (sustains a)
          pos 2: - (sustains a)
          pos 3: b (attacks)
          → a = 3/4 of beat, b = 1/4 of beat
```

Sustain across beat boundaries:

```
a -;   →  beat 1: a (1 char, whole beat)
          beat 2: -; (2 char positions)
            - sustains a for 1/2 beat
            ; = rest for 1/2 beat
          → a tied over for 1.5 beats, then 1/2 beat rest
```

If a group starts with `-`, it refers to the last pitch of the previous group (error if none).

---

### Rest (`;`)

A rest occupies a position slot and produces silence. It counts as an active position for subdivision.

```
a ;    →  2 beats → quarter note + quarter rest
a;     →  1 beat, 2 positions → eighth + eighth rest
```

---

### Chord grouping `( ... )`

Parentheses group pitches into a single chord occupying one position slot. Pitches within a chord are **strictly ascending** left-to-right.

```
(ace)f →  1 beat, 2 positions
          pos 1: A-minor triad (a, c, e simultaneous as eighths)
          pos 2: f (eighth)
```

---

### Pitch entry

- **Letters** `a b c d e f g` or `A B C D E F G` — uppercase normalized to lowercase
- **Accidentals** precede the letter: `#f`, `&b`, `%c` (sharp, flat, natural)
- UTF-8 glyphs (♯ ♭ ♮) mapped to ASCII during normalization

### Octave rules

- **Initial reference:** Middle C (MIDI 60)
- **Relative positioning** (Lilypond rule): each note picks the octave closest to the prior note, preferring a 4th or less
- **`^`** before a pitch forces it up one octave from its natural relative position
- **`/`** before a pitch forces it down one octave
- Multiple marks work: `^^c` = two octaves up
- **Chords:** Pitches within `(...)` are strictly ascending — each successive pitch is higher than the one before

---

## Plugin Architecture

### Component structure (`m4bon.qml`)

```
MuseScore (root)
 ├── Symbol constants      → All DSL chars as overridable properties
 ├── onRun                 → Entry point (checks curScore, focuses input)
 ├── Keys.onPressed        → Escape quits; Ctrl+Enter / Cmd+Enter sends
 ├── resolveBeatDuration() → Returns beat duration in quarter-note units from time sig
 ├── normalizePitchInput() → Lowercase + UTF-8 accidental mapping
 ├── parseDSL()            → Beat-group parser (TODO: implement)
 ├── insertNotes()         → Score insertion with startCmd/endCmd
 ├── UI (ColumnLayout)
 │    ├── Label (title)
 │    ├── TextEdit (DSL input, monospace)
 │    ├── Button "Send"
 │    ├── Button "Undo"
 │    └── Label (status)
```

### Pipeline (implemented in `m4bon.qml`)

```
DSL text
  → normalizePitchInput()        — lowercase, map ♯♭♮
  → tokenize()                   — split on /\S+/, track offsets
  → parseGroup() per token       — char-level state machine
       States: IDLE | IN_MULTIPLIER | IN_CHORD
       Multiplier consumed at start, then chars processed:
         a-g       → accumulate note (emitted on next non-letter)
         # & %     → adjust accidental accumulator
         ^ /       → adjust octave-shift accumulator
         -         → emit sustain slot
         ;         → emit rest slot
         (...)     → enter IN_CHORD, validate ascending
       Returns {multiplier, slots: [{type, ...}]} or {error, errorOffset}
  → resolveDurations(groups, cursor)
       ← read time sig via cursor.measure.timesigNumerator/Denominator
       ← resolveBeatDuration() → beat = {num, den} fraction of whole note
       ← for each group:
            activeCount = non-sustain slot count
            per-note duration = (multiplier × beat) / activeCount
            if per-note is a standard value (1/den or 3/den with den=powerOf2):
              → no tuplet, each position = totalTime / posCount (includes sustains)
            else:
              → tuplet container emitted:
                  cursor.addTuplet(fraction(N, L), fraction(totalNum, totalDen))
                  where N = activeCount, L = lowerPowerOf2(activeCount)
              → each active slot gets NOMINAL duration = totalTime / L
       ← sustain slots extend the most recent event's duration fraction
       ← all fractions reduced via GCD
  → resolveOctaves(events)        — Lilypond closest-interval from C4
       Skips tupletStart events and rest events.
       Single notes:  resolvePitch(letter, accidental, octaveShift, reference)
       Chords:        strictly ascending, last chord pitch becomes next reference
  → insertNotes(events)           — MuseScore API calls
       curScore.startCmd()
       for each event:
         tupletStart: addTuplet(fraction(ratioNum, ratioDen), fraction(totalNum, totalDen))
         rest:        setDuration(nominal or actual z,n) → addRest()
         note:        setDuration(nominal or actual z,n) → addNote(midi, false)
         chord:       setDuration(nominal or actual z,n) →
                        addNote(midi[0], false) → addNote(midi[1], true) ...
       curScore.endCmd()
```

### Tuplet detection

A group forms a tuplet when the per-note actual duration is **not a standard note value**:

```
per_note = (multiplier × beat) / activeCount
isStandard = after reduction: den is power of 2 AND num is 1 or 3
```

| Example | Time sig | Total time | Active notes | Per-note | Standard? | Result |
|---|---|---|---|---|---|---|
| `abc` | 4/4 | 1/4 | 3 | 1/12 | no | tuplet (triplet eighths) |
| `2abc` | 4/4 | 1/2 | 3 | 1/6 | no | tuplet (quarter-note triplet) |
| `3abc` | 5/8 | 3/8 | 3 | 1/8 | **yes** | no tuplet, plain eighths |
| `ab` | 4/4 | 1/4 | 2 | 1/8 | yes | no tuplet, plain eighths |
| `a--b` | 4/4 | 1/4 | 2 | 1/8 | yes | no tuplet, sustain extended |

When a tuplet is needed, the API call is:

```
cursor.addTuplet(fraction(activeCount, lowerPowerOf2(activeCount)),
                 fraction(totalNum, totalDen))
```

Then each note uses its **nominal** (visual) duration for `setDuration`:

```
nominal = totalTime / lowerPowerOf2(activeCount)
```

| Active notes | Ratio | Nominal base | Example |
|---|---|---|---|
| 3 | 3:2 | totalTime / 2 | triplet eighths → base = eighth |
| 5 | 5:4 | totalTime / 4 | quintuplet → base = sixteenth |
| 6 | 6:4 | totalTime / 4 | sextuplet → base = sixteenth |
| 7 | 7:4 | totalTime / 4 | septuplet → base = sixteenth |

### Sustain duration model

For a group with N character positions spanning M beats:

- Each position = `(M × beat_duration) / N` time
- A note with `k` consecutive sustain `-` slots after it has duration `(k+1) × position_duration`
- Sustain across groups: a `-` at the start of a group extends the previous group's last note

```
a--b  →  4 positions, beat/4 each
          a duration = 3/4 beat (dotted eighth)
          b duration = 1/4 beat (sixteenth)

a -;  →  beat 1: "a" = 1 position = 1/4
          beat 2: "-;" = 2 positions = 1/8 each
                   "-" extends a by 1/8 → a = 3/8 total (dotted quarter)
                   ";" = rest = 1/8
```

### Beat duration resolution

`resolveBeatDuration(cursor)` reads the score's time signature:

| Denominator | Numerator % 3 == 0? | Beat returned |
|-------------|----------------------|---------------|
| 2 | — | `{num:1, den:2}` (half) |
| 4 | — | `{num:1, den:4}` (quarter) |
| 8 | yes | `{num:3, den:8}` (dotted quarter) |
| 8 | no | `{num:1, den:8}` (eighth) |
| 16 | yes | `{num:3, den:16}` (dotted eighth) |
| 16 | no | `{num:1, den:16}` (sixteenth) |
| other | — | `{num:1, den:N}` (denominator note) |

Defaults to 4/4 if the score is inaccessible.

---

## Key API Details (MuseScore 4 Plugin)

- **`curScore.newCursor()`** — creates cursor
- **`cursor.track = (staffIdx × 4) + voice`** — or set `staffIdx` / `voice`
- **`cursor.rewind(1)`** — position at selection start (current tick with no selection)
- **`cursor.setDuration(z, n)`** — note value as fraction of whole note: `(1,4)` = quarter, `(1,8)` = eighth
- **`cursor.addNote(pitch, addToChord)`** — `false` = new chord, `true` = add to existing
- **`cursor.addRest()`** — adds rest at current position
- **`curScore.startCmd()` / `curScore.endCmd()`** — wrap modifications as single undo step
- **`curScore.undo()` does NOT exist** in MS4 plugin API
- **`cmd("undo")` does NOT work** in MS4

---

## Undo

Informational in this version. Each Send is wrapped in `startCmd()`/`endCmd()`, making the batch a single native undo step (Ctrl+Z / Cmd+Z). `lastNotes` and `lastNoteCount` stored for future implementation.

---

## UI Layout

| Element        | Details                                    |
|----------------|--------------------------------------------|
| Window         | 500×340, `pluginType: "dialog"`           |
| Input field    | `TextEdit`, monospace (Menlo/Consolas)     |
| Input focus    | `forceActiveFocus()` in `onRun`            |
| Send button    | Also triggered by **Ctrl+Enter / Cmd+Enter** |
| Escape         | Closes plugin (`Qt.quit()`)                |
| Status bar     | Parse errors, insert confirmation, undo info |

---

## Essential Commands

### Install
```bash
ln -s "$PWD" ~/Documents/MuseScore4/Plugins/m4bon
```

### Run
No build step. Enable via **Home → Plugins** then **Plugins → m4bon**.

### Validate syntax
```bash
qmllint m4bon.qml   # requires Qt SDK
```

---

## Known limitations & open items

- **No multi-staff support** — always staff 0, voice 0 (`cursor.track = 0`)
- **No note removal API** — Undo button can't manually remove inserted notes; uses `curScore.undo()` (silent no-op on official MS4)
- **No overfill detection** — parser doesn't check if DSL content exceeds remaining measure space
- **No key signature accidentals** — key sig not read; user must specify accidentals explicitly
- **No scale-degree mode** — Tbon's `1 2 3 4 5 6 7` not yet supported
- **No grace notes** — no syntax for appoggiatura/acciaccatura
- **Pickup measures** — no pickup convention yet
- **Time signature reading** — uses `cursor.measure.timesigNumerator/Denominator` with 4/4 fallback; untested against actual MuseScore builds
- **Chord ascending validation** — compares pitch letters as strings, not MIDI values; `(c#e&g)` needs MIDI-based comparison
- **Tuplet rest inside chord** — untested; `ab;` with 3 active positions triggers a tuplet with a rest, which should work via `addRest()` inside the tuplet container
- **`.gitignore` oversized** — Qt/C++ patterns irrelevant to QML-only project
