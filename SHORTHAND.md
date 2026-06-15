# Shorthand Notation Reference

A compact notation for describing musical output in test assertions and communication.

---

## Duration Codes

Single letter + optional `.` suffixes for augmentation dots.

| Code | Duration | Example |
|------|----------|---------|
| W    | whole        | `Wc4` |
| H    | half         | `Ha3` |
| Q    | quarter      | `Qc4` |
| E    | eighth       | `Eb3` |
| S    | 16th         | `Sg4` |
| T    | 32nd         | `Tb4` |
| .    | dotted (suffix)  | `Q.c4` = dotted quarter |
| ..   | double-dotted     | `H..a3` = double-dotted half |
| ...  | triple-dotted     | (if needed) |

---

## Notes

Format: `<duration><pitch>[alteration]<octave>`

- Duration: one of W, H, Q, E, S, T + optional `.` `..` `...`
- Pitch: `a-g` (case-insensitive, stored lowercase)
- Alteration: `#` sharp, `&` flat, `%` natural (optional)
- Octave: number (4 = middle C)

Examples:
- `Qc4` — quarter C4
- `Q.c4` — dotted quarter C4
- `H..a3` — double-dotted half A3
- `E#f4` — eighth F♯4
- `S&b3` — 16th B♭3

---

## Rests

Format: `<duration>;` (semicolon after duration code)

Examples:
- `Q;` — quarter rest
- `E;` — eighth rest

---

## Chords

Format: `(<pitch1>,<pitch2>,...) <duration>`

Examples:
- `(C4,E4,G4) Q` — C major triad, quarter duration
- `(D4,F4,A4) E` — D minor triad, eighth

---

## Tuplets

Format: `M/N<duration><pitch>` — M notes in the time of N of the base unit.

Examples:
- `2/3Qa3` = 2 notes in the time of 3 quarter notes
- `3/2Qa3` = standard triplet (3 notes in time of 2 quarters)

---

## Ties

A `-` separator between tokens indicates a tie from the preceding note to the following note.

Example:
- `Ha3 - Ea3 Eb3 Qc4` = half A3 tied to eighth A3, eighth B3, quarter C4

---

## Full example

DSL: `a - -b c`
Shorthand: `Ha3 - Ea3 Eb3 Qc4`
