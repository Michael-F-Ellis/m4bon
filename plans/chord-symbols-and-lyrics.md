# Chord Symbols & Lyrics — Implementation Plan

**Date:** 2026-06-18
**Status:** Draft — ready for execution

## Objective

Extend the m4bon DSL with two optional per-measure directives that appear **after** the notation content, before the barline `|`:

- **`:H`** — chord symbols for the measure, one per beat position
- **`:L`** — lyrics for the measure, one syllable per active-note attack

Both are optional, order-independent. MusicXML output is deferred; the TUI and `-render` CLI flag produce a three-column layout: **CHORDS : NOTES : LYRICS**.

## Examples

```
KD♭ M4/4 ;e fe f e :H E♭m7 - B♭7 - :L My heart is sad and |

KD♭ M4/4 ;e fe f e |                                  # notation only
KD♭ M4/4 ;e fe f e :H E♭m7 - B♭7 - |                  # + chords
KD♭ M4/4 ;e fe f e :L My heart is sad and |            # + lyrics
KD♭ M4/4 ;e fe f e :L My heart is sad and :H E♭m7 - B♭7 - |  # both, any order

KD♭ M4/4 c d e f :H C - G7 - |
M4/4 g - ag fe :L Glo - ** ** |
M4/4 cd ec ^g /c :L no_thing more_than feel ings. |
```

---

## Architecture

```
DSL text
  → tokenize
    → splitMeasures (on |)
      → for each measure token group:
          extractDirectivesTail (new: peel :H/:L tokens from tail)
          scanMeasureDirectives (existing: K/M/B from head)
          parseBeatTokens (existing: notation tokens → Slots)
          resolveDurationsWithPrior
          ...
  → MeasureResult { ..., Chords, Lyrics, HasChords, HasLyrics }

Render pipeline:
  → BuildRows (new: two-pass, computes column widths)
    → for each measure:
        buildChordCells
        buildNoteCells (existing buildMeasureCells, minus prefix)
        buildLyricCells
  → FormatANSIRows (new: three-column layout with padding)
  → TUI / -render consume via render.Render()
```

---

## Phase 1 — Parser: Extract `:H` and `:L` directives

### 1a — Add fields to `MeasureResult` (`parser/parse.go`)

```go
type MeasureResult struct {
    Events     []Event
    TimeNum    int
    TimeDen    int
    Fifths     int
    IsPickup   bool
    NumGroups  int
    GroupSlots []int

    // Chord symbols & lyrics (added in this feature)
    Chords     []string // one per beat group; nil/empty if no :H directive
    Lyrics      []string // one entry per active-note attack; nil/empty if no :L directive
    HasChords  bool
    HasLyrics  bool
}
```

`Chords` has `len == NumGroups` — each entry maps to a beat group. `"-"` = sustain, `";"` = rest, anything else = chord symbol. `Lyrics` are variable length — each entry maps to an active (non-sustain) note/event within the measure, in order.

### 1b — Token extraction (`parser/pipeline.go`)

Add a function `extractDirectivesTail` called **before** `scanMeasureDirectives` in the per-measure loop:

```go
// extractDirectivesTail peels :H and :L tokens from the end of a measure's
// token group. Returns the remaining tokens (notation + K/M/B directives),
// and the extracted chord/lyric token strings.
func extractDirectivesTail(tokens []Token) (remaining []Token, chordTokens, lyricTokens []string) {
    // Scan backward. :H and :L tokens appear after notation, before |.
    // A token starting with : followed by H or L is a directive token.
    // Everything after the last :H/:L token (and before those tokens) is part of
    // that directive's payload. Directives are parsed in tail-to-head order
    // so the payload tokens after a directive marker belong to it.
    
    // State machine scanning from right to left:
    //   - looking for :H or :L markers
    //   - tokens to the right of a marker (until next marker or start) are its payload
}
```

**Algorithm** — scan tokens right-to-left:
```
Input: [tok0, tok1, ..., :H, E♭m7, -, B♭7, -, :L, My, heart, is, sad, and]
                                                              ↑ we scan ← this way

State: NONE → see 'and', previous state=NONE, not a marker → collecting for nothing
       see 'sad' → collecting for nothing
       ...
       see ':L' → it's a marker! Everything collected so far goes to Lyrics.
       Reset collected. State=IN_LYRICS. Continue scanning left.
       see '-' → collecting for Lyrics
       see 'B♭7' → collecting for Lyrics... wait, this should belong to :H.

Problem: once we hit :L, everything to its left also looks like lyrics unless
we hit another marker.

Fix: scan right-to-left. When we hit :L:
  - everything to the RIGHT of :L (already scanned) is the lyric payload
  - continue scanning left for :H
When we hit :H:
  - everything between :H and :L (or :H and end) is the chord payload
  - everything to the left of :H (already scanned) are notation tokens
```

Simpler: the payload for a directive is the tokens *after* it, until the next directive marker or end. So:

```
Scan left-to-right, identifying directive boundaries:
[t0, t1, t2, :H, C, -, G7, -, :L, My, heart, and, |]

State machine L→R:
  NONE → see t0, t1, t2 → notation tokens
  NONE → see :H → switch to IN_CHORDS, chordTokens starts
  IN_CHORDS → see C, -, G7, - → add to chordTokens
  IN_CHORDS → see :L → switch to IN_LYRICS, lyricTokens starts
  IN_LYRICS → see My, heart, and → add to lyricTokens
  IN_LYRICS → see | → switch to NONE (end of measure)
```

This is cleaner. Tokens before the first `:H`/`:L` are notation tokens (passed to `scanMeasureDirectives`). Tokens after `:H` (until next `:H`/`:L` or `|`) are chord tokens. Tokens after `:L` (until next `:H`/`:L` or `|`) are lyric tokens. Markers `:H`/`:L` themselves are consumed, not passed through.

**Order independence**: If `:L` appears before `:H`, the state machine handles it: `IN_LYRICS` then `IN_CHORDS`. Both sets of tokens are populated in the order they appear.

### 1c — Integration in `ParseDSL`

In the per-measure loop (`parser/pipeline.go`, line ~1033):

```go
for mi, group := range measureTokenGroups {
    // NEW: Extract :H/:L from tail
    remaining, chordRaw, lyricRaw := extractDirectivesTail(group)
    
    // Existing: scan K/M/B from head
    md := scanMeasureDirectives(remaining, currentFifths, currentTimeNum, currentTimeDen)
    
    // ... rest unchanged, using md.beatTokens (which are the notation tokens)
}
```

Store parsed chord/lyric data on `MeasureResult` after the measure is built.

### 1d — Chord token validation

After extraction, validate chord token count vs beat count:

```go
if hasChords && len(chordTokens) != numGroups {
    // Warning (non-fatal): mismatch between chord count and beat count
}
```

### 1e — Lyric token handling

Lyric tokens are stored as-is on `MeasureResult.Lyrics`. Token semantics:
- `-` → syllable extension (sustain previous syllable across this note)
- `*` → melisma mark (note belongs to current syllable)
- `_` → syllable separator within a beat (e.g., `no_thing` = two syllables "no" and "thing" within one beat position)
- Any other token → a lyric syllable

The render layer resolves `-`, `*`, and `_` visually. The count of lyric tokens is independent of beat count — they map 1:1 to active note attacks within the measure.

---

## Phase 2 — Chord Symbol Normalization (`theory/chords.go`)

New file. Accepts raw chord input strings and produces normalized display strings.

### 2a — Input grammar (keyboard-friendly)

```
chord    = root [quality] [extension] [alteration]* ["/" bass]
root     = [A-Ga-g] ["#" | "&" | "♯" | "♭"]
quality  = "" | "m" | "-" | "min" | "dim" | "°" | "aug" | "+"
         | "h" | "hdim" | "ø" | "m7b5" | "m7♭5" | "maj" | "Δ" | "sus" ["2" | "4"]
extension= "7" | "9" | "11" | "13" | "6"
alteration  = "b" digit | "♭" digit | "#" digit | "♯" digit
bass     = root
```

Examples: `C`, `Cm`, `C-`, `Cdim`, `C°`, `Caug`, `C+`, `Chdim`, `Cø`, `Cm7b5`, `Cmaj7`, `CΔ7`, `CΔ`, `C7`, `C9`, `Cm7`, `C-7`, `Csus`, `Csus4`, `Csus2`, `C7♭9`, `C7#9`, `C7♭13`, `Dm7/A`

### 2b — Display normalization table

| Input | Display | Color |
|---|---|---|
| `C` | `C` | default |
| `Cm`, `C-`, `Cmin` | `C⁻` | default |
| `Cdim`, `C°` | `C°` | default |
| `Chdim`, `Cø`, `Cm7♭5` | `Cø⁷` | default |
| `Caug`, `C+` | `C⁺` | default |
| `Cmaj7`, `CΔ`, `CΔ7` | `C∆⁷` | default |
| `C7` | `C⁷` | default |
| `Cm7`, `C⁻7` | `C⁻⁷` | default |
| `C9` | `C⁹` | default |
| `Csus`, `Csus4` | `Csus⁴` | default |
| `Csus2` | `Csus²` | default |
| `C6` | `C⁶` | default |
| `C7♭9` | `C⁷♭⁹` | default |
| `C7♯9` | `C⁷♯⁹` | default |
| `C7♯11` | `C⁷♯¹¹` | default |
| `C7♭13` | `C⁷♭¹³` | default |
| `C♯`, `C#` | `C` | red (sharp) |
| `C♭`, `C&` | `C` | blue (flat) |
| `Dm7/A` | `D⁻⁷/A` | default |
| `Dm7/A♭` | `D⁻⁷/A` + blue | root blue for A♭ |

Root accidentals are stripped from the display text and conveyed via color (same scheme as notation: `StyleSharp` / `StyleFlat`). Extension accidentals use the glyphs ♯ (U+266F) and ♭ (U+266D) within the display string.

### 2c — API

```go
package theory

// NormalizeChordSymbol converts a raw chord token to its display form.
// The root accidental is returned separately for color application.
// Sustain markers "-" and rest markers ";" are passed through unchanged.
func NormalizeChordSymbol(raw string) (display string, rootAccidental int)
// rootAccidental: -1=flat, 0=none/natural, 1=sharp

// ValidateChordSymbol checks whether a raw chord token is syntactically valid.
// Returns "" if valid, or an error message.
func ValidateChordSymbol(raw string) string
```

### 2d — Unicode character constants

Define as package-level constants in `theory/chords.go`:

```go
const (
    ChrDelta       = "∆"  // U+2206 major 7th
    ChrHalfDim     = "ø"  // U+00F8 half-diminished
    ChrDim         = "°"  // U+00B0 diminished
    ChrAug         = "⁺"  // U+207A augmented
    ChrMinus       = "⁻"  // U+207B minor
    ChrSup7        = "⁷"  // U+2077
    ChrSup9        = "⁹"  // U+2079
    ChrSup11       = "¹¹" // superscript 11
    ChrSup13       = "¹³" // superscript 13
    ChrSup6        = "⁶"  // U+2076
    ChrSup2        = "²"  // U+00B2
    ChrSup4        = "⁴"  // U+2074
    ChrSharp       = "♯"  // U+266F
    ChrFlat        = "♭"  // U+266D
)
```

---

## Phase 3 — Render: Three-Column Layout

### 3a — New types (`render/cell.go`)

```go
// MeasureRow contains the three columns for one measure's display.
type MeasureRow struct {
    ChordCells CellSeq
    NoteCells  CellSeq
    LyricCells CellSeq
}
```

### 3b — Column width computation

`BuildRows` replaces `BuildCells`. Two-pass algorithm:

```
Pass 1: Build all rows, compute max visible widths
  for each measure:
      row.ChordCells  = buildChordCells(m)
      row.NoteCells   = buildNoteCells(m)
      row.LyricCells  = buildLyricCells(m, m.Events)
  maxChordWidth = max(visibleLen(row.ChordCells))
  maxNoteWidth  = max(visibleLen(row.NoteCells))
  maxLyricWidth = max(visibleLen(row.LyricCells))

Pass 2: Format with padding
  for each row:
      pad chords to maxChordWidth, notes to maxNoteWidth, lyrics to maxLyricWidth
      line = leftPad(chordCells, maxChordWidth) + "    " +
             leftPad(noteCells, maxNoteWidth)  + "    " +
             lyrics (left-justified)
```

Separation: 4 spaces between columns. Each column left-justified within its width.

### 3c — Measure-number prefix placement

Measure numbers (`N:  `) are right-justified in the gap between CHORDS and NOTES columns. Implementation: prepend the measure number (in `StyleDefault`) to `NoteCells`, then right-pad the entire CHORDS column so the number sits at the boundary.

Alternative: include the measure number as part of the chord column's right-padding:

```
  C    C⁷     1:  c₄ d e f    Glo  -   **  **
  C⁻⁷         2:  a₄ b c d    My heart is sad
```

The chord column is padded to `maxChordWidth`, then the measure number prefix is placed immediately after, before the notes column.

Actually, simpler: prepend measure number to NoteCells as before, and handle the inter-column gap separately. The 4-space gap between chords and notes naturally separates the number from the chord content.

**Revised**: Measure number prefix `"N:  "` stays at the start of NoteCells. The inter-column padding between chords and notes is 4 spaces. This means chord content ends at column position `maxChordWidth`, then 4 spaces of padding, then `"N:  "`, then note content.

We may adjust after seeing it in the TUI.

### 3d — `buildChordCells` (`render/render.go`)

```go
func buildChordCells(m parser.MeasureResult) CellSeq {
    if !m.HasChords || len(m.Chords) == 0 {
        return nil
    }
    var cells CellSeq
    for i, raw := range m.Chords {
        if i > 0 {
            cells = append(cells, Cell{Content: " ", Style: StyleDefault})
        }
        if raw == "-" {
            cells = append(cells, Cell{Content: "-", Style: StyleSustainRest})
        } else if raw == ";" {
            cells = append(cells, Cell{Content: ";", Style: StyleSustainRest})
        } else {
            display, rootAcc := theory.NormalizeChordSymbol(raw)
            // Color the root letter
            cells = append(cells, Cell{
                Content: display,
                Style:   chordStyleForAccidental(rootAcc),
            })
        }
    }
    return cells
}

func chordStyleForAccidental(acc int) StyleClass {
    switch {
    case acc > 0:
        return StyleSharp
    case acc < 0:
        return StyleFlat
    default:
        return StyleDefault
    }
}
```

### 3e — `buildLyricCells` (`render/render.go`)

Lyrics map 1:1 to active note attacks in the measure's events. Walk the events, and for each non-sustain event, consume one lyric token.

```go
func buildLyricCells(m parser.MeasureResult) CellSeq {
    if !m.HasLyrics || len(m.Lyrics) == 0 {
        return nil
    }
    var cells CellSeq
    li := 0 // index into m.Lyrics
    first := true
    for _, ev := range m.Events {
        if ev.Type == parser.EventTupletStart || ev.Split {
            continue
        }
        if ev.Type == parser.EventRest {
            // Still advance the lyric position
            li++
            continue
        }
        if li >= len(m.Lyrics) {
            break
        }
        if !first {
            cells = append(cells, Cell{Content: " ", Style: StyleDefault})
        }
        // Handle '_' separator: split token on '_' for multi-syllable
        token := m.Lyrics[li]
        if token == "-" {
            // Syllable extension: no new text, keep previous
            cells = append(cells, Cell{Content: "-", Style: StyleSustainRest})
        } else if token == "*" || strings.Trim(token, "*") == "" {
            // Melisma: one or more '*' marks
            // Each event position gets a melisma marker
            cells = append(cells, Cell{Content: "*", Style: StyleSustainRest})
        } else if strings.Contains(token, "_") {
            // Multi-syllable within one note: split on '_'
            parts := strings.Split(token, "_")
            for pi, p := range parts {
                if pi > 0 {
                    // Visual separator between syllables
                    cells = append(cells, Cell{Content: "_", Style: StyleDefault})
                }
                cells = append(cells, Cell{Content: p, Style: StyleDefault})
            }
        } else {
            cells = append(cells, Cell{Content: token, Style: StyleDefault})
        }
        li++
        first = false
    }
    return cells
}
```

### 3f — `FormatANSI` / `FormatANSIRows` (`render/ansi.go`)

New function that formats `[]MeasureRow` instead of `[][]CellSeq`:

```go
func FormatANSIRows(rows []MeasureRow, maxChordW, maxNoteW, maxLyricW int, asciiLeaps bool) string
```

For each row: emit chord cells padded to `maxChordW`, then 4 spaces, then note cells padded to `maxNoteW`, then 4 spaces, then lyric cells. Padding uses spaces, invisible to ANSI-based visible-length computation (since spaces have no ANSI codes).

### 3g — Updated `BuildRows` signature (`render/render.go`)

```go
func BuildRows(measures []parser.MeasureResult, showSubscripts bool) (rows []MeasureRow, maxChordW, maxNoteW, maxLyricW int)
```

### 3h — Updated `Render` (`render/render.go`)

```go
func Render(measures []parser.MeasureResult, asciiLeaps bool, showSubscripts bool) string {
    rows, maxCW, maxNW, maxLW := BuildRows(measures, showSubscripts)
    return FormatANSIRows(rows, maxCW, maxNW, maxLW, asciiLeaps)
}
```

`FormatPlain` is also updated for test output.

---

## Phase 4 — Integration

### 4a — `m4bon.go`

No API change needed. `Render` and `RenderOptions` internally call the updated render pipeline. `Compile` (MusicXML) ignores chord/lyric data for now.

### 4b — `cmd/m4bon/main.go`

No code changes needed — the `-render` flag path calls `render.Render(result.Measures, ...)` which now produces three-column output.

### 4c — TUI (`cmd/m4bon/tui/`)

The TUI calls `render.Render()` in `initialModel` and stores `renderLines`. The `measureView` function displays these lines. The measure indicator (▶, ▷, ◉) is prepended to each line. No TUI code changes needed beyond possibly adjusting the indicator column width.

**Exception**: The TUI's `visibleLen` and `truncateVisible` functions already handle ANSI codes. They should work unchanged with the new three-column output since each line is still a single ANSI string.

However, the measure indicator currently occupies 3 characters (`"▶ "` etc.) prepended to each line. This may visually overlap with the chord column. We could move the indicator to the gap between chords and notes, or keep it at the far left. Decision: keep indicators at far left for now (existing behavior), 3-char prefix before the chord column. We'll adjust after testing.

---

## Phase 5 — Tests

### 5a — Chord normalization tests (`theory/chords_test.go`)

Test every normalization mapping from the table in Phase 2b.

### 5b — Parser extraction tests (`parser/parse_test.go`)

```go
func TestExtractChordDirective(t *testing.T) {
    r := ParseDSL("M4/4 c d e f :H C - G7 - |")
    // Assert r.Measures[0].HasChords == true
    // Assert r.Measures[0].Chords == ["C", "-", "G7", "-"]
    // Assert existing events still parse correctly
}

func TestExtractLyricDirective(t *testing.T) {
    r := ParseDSL("M4/4 c d e f :L My heart is sad |")
    // Assert r.Measures[0].HasLyrics == true
    // Assert r.Measures[0].Lyrics == ["My", "heart", "is", "sad"]
}

func TestExtractBothOrderIndependent(t *testing.T) {
    // Test :H before :L
    // Test :L before :H
    // Test multiple measures, some with only one directive
}

func TestNoDirectives(t *testing.T) {
    r := ParseDSL("M4/4 c d e f |")
    // Assert !r.Measures[0].HasChords && !r.Measures[0].HasLyrics
}

func TestEmptyDirectiveIgnored(t *testing.T) {
    r := ParseDSL("M4/4 c d e f :H :L |")
    // Empty directives should be accepted and produce empty slices
}
```

### 5c — Render tests (`render/render_test.go`)

Test three-column output format:

```go
func TestRenderChordsOnly(t *testing.T) {
    cells := buildRowCells(t, "M4/4 c d e f :H C - G7 - |")
    // Verify chord cells contain expected normalized display
    // Verify note cells unchanged
    // Verify no lyric cells
}

func TestRenderLyricsOnly(t *testing.T) { ... }

func TestRenderAllThreeColumns(t *testing.T) {
    cells := buildRowCells(t, "M4/4 c d e f :H C - G7 - :L My heart is sad |")
    // Verify all three columns present and aligned
}
```

### 5d — Golden file tests (`render_golden_test.go`)

Add `.dsl` and `.expected.render` golden files for:
- `test/cases/render-chords.dsl` / `.expected.render`
- `test/cases/render-lyrics.dsl` / `.expected.render`
- `test/cases/render-chords-lyrics.dsl` / `.expected.render`

### 5e — Existing tests must pass unchanged

All existing tests (`go test ./...`) must pass without modification. This verifies backward compatibility.

---

## Phase 6 — Documentation

### 6a — Update `AGENTS.md`

Add chord symbol and lyric directive sections to the DSL Reference table.

### 6b — Update `SHORTHAND.md`

Add chord quality shorthand table and lyric token reference.

---

## Implementation Order

```
Phase 1a-b: extractDirectivesTail + MeasureResult fields  ← start here
Phase 1c-e: integration in ParseDSL
Phase 1-test: parser extraction tests
Phase 2:     theory/chords.go (normalization)
Phase 2-test: chord normalization tests
Phase 3a-c:  render types, BuildRows, column widths
Phase 3d-f:  buildChordCells, buildLyricCells, FormatANSIRows
Phase 3g-h:  updated Render/BuildRows signatures
Phase 3-test: render tests + golden files
Phase 4:     integration (m4bon.go, cmd, TUI)
Phase 5e:    verify all existing tests pass
Phase 6:     documentation
```

**Dependency graph:**
```
1a-b → 1c-e → (all subsequent phases depend on parser changes)
         ↘ 1-test
2 → 2-test
1 + 2 + 3a-c → 3d-f → 3g-h → 3-test → 4 → 5e → 6
```

## Risks & Edge Cases

1. **Token with `:` mid-word**: The tokenizer splits on whitespace, so `:H` and `:L` are always full tokens. No risk of false positives.

2. **`:H`/`:L` inside voice-poly chords**: Not possible — parsing `:H`/`:L` happens at the token level, before `parseGroup`. The `:` inside `parseGroup` still causes an error, but `:H`/`:L` tokens are consumed before `parseGroup` sees them.

3. **Sustain/rest in chord/lyric sections**: `-` and `;` have the same semantics for chords and lyrics as they do for notation. `*` is lyric-specific (melisma).

4. **Mismatched token counts**: If a `:H` or `:L` directive has the wrong number of tokens for the measure's beat/event count, we produce a warning in `ParseDSL` but don't fail — the renderer pads or truncates as needed.

5. **Wide chord/lyric content**: If chord symbols or lyrics are wider than the terminal, the TUI's existing `truncateVisible` handles it. Column alignment may break if truncation happens, but that's acceptable.

6. **Empty measure with directives**: `":H C |"` (no notation, just chord symbol) — the measure has 0 events. Chord tokens are stored but the renderer has nothing to align against. We skip rendering chords/lyrics for measures with no events.
