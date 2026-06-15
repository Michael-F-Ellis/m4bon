# Key Signature Reference

Circle-of-fifths lookup table mapping DSL `K` directives to accidentals.

---

## Format

`K` accepts the tonic letter + accidentals in any order:
- `KE&`, `K&e`, `KEb`, `Ke&` all mean E♭ major (3 flats)
- `KF#`, `K#f`, `KF♯` all mean F♯ major (6 sharps)

---

## Table

| Directive   | Key       | Fifths | Accidentals                   |
|-------------|-----------|--------|-------------------------------|
| `KC` `K%`   | C major   | 0      | (none)                        |
| `KG`        | G major   | +1     | F♯                            |
| `KD`        | D major   | +2     | F♯, C♯                        |
| `KA`        | A major   | +3     | F♯, C♯, G♯                    |
| `KE`        | E major   | +4     | F♯, C♯, G♯, D♯               |
| `KB`        | B major   | +5     | F♯, C♯, G♯, D♯, A♯           |
| `KF#` `K#f` | F♯ major  | +6     | F♯, C♯, G♯, D♯, A♯, E♯       |
| `KC#` `K#c` | C♯ major  | +7     | F♯, C♯, G♯, D♯, A♯, E♯, B♯   |
| `KF`        | F major   | -1     | B♭                            |
| `KB&` `K&b` | B♭ major  | -2     | B♭, E♭                        |
| `KE&` `K&e` | E♭ major  | -3     | B♭, E♭, A♭                    |
| `KA&` `K&a` | A♭ major  | -4     | B♭, E♭, A♭, D♭               |
| `KD&` `K&d` | D♭ major  | -5     | B♭, E♭, A♭, D♭, G♭           |
| `KG&` `K&g` | G♭ major  | -6     | B♭, E♭, A♭, D♭, G♭, C♭       |
| `KC&` `K&c` | C♭ major  | -7     | B♭, E♭, A♭, D♭, G♭, C♭, F♭   |

---

## Implementation Notes

- Normalization: extract the letter and accidentals from `K...`, sort by canonical order to match the table.
- Accidentals in `K` always refer to the tonic. `KE&` = E♭ major (not E major with B♭).
- The key signature always means Ionian (major) mode.
- Key and meter can appear in either order: `KE& M6/8` or `M6/8 KE&`.
- Both directives must appear at the start of the DSL string (beginning of measure 1).
- If no `K` directive, default = C major (0 fifths).
- If no `M` directive, default = 4/4.

## Fifths encoding

The `fifths` value in MusicXML's `<key>` element:
- Positive = sharps (e.g. +1 = G major)
- Negative = flats (e.g. -3 = E♭ major)
- Zero = C major
