# Newline as Measure Separator — Implementation Plan

**Date:** 2026-06-18
**Status:** Done (0.18.0)

## Objective

Eliminate `|` barlines from the DSL grammar. Measures are now separated by newlines
(one measure per line). End of file also terminates the final measure. Blank lines
are stripped. The `|` character has no special meaning and should not appear in valid input.

## Design Decision: SanitizeDSL returns `[]string`

Currently `SanitizeDSL` joins lines with spaces, collapsing all measure boundaries.
The core change: `SanitizeDSL` returns a slice of clean lines (`[]string`), and
`ParseDSL` iterates over them directly.

```
Before: SanitizeDSL(text string) string         → "c d e f a b c d"
After:  SanitizeDSL(text string) []string        → ["c d e f", "a b c d"]
```

## File-by-File Changes

### 1. `parser/parse.go` — SanitizeDSL return type

**`SanitizeDSL`** (line ~277):
- Change return type from `string` to `[]string`
- Remove `strings.Join(lines, " ")` — return `lines` directly
- No other logic changes (blank line stripping, comment stripping stay the same)

```go
func SanitizeDSL(text string) []string {
    var lines []string
    for _, line := range strings.Split(text, "\n") {
        trimmed := strings.TrimSpace(line)
        if trimmed == "" {
            continue
        }
        if strings.HasPrefix(trimmed, "#") && (len(trimmed) == 1 || trimmed[1] == ' ') {
            continue
        }
        lines = append(lines, trimmed)
    }
    return lines
}
```

### 2. `parser/pipeline.go` — ParseDSL, splitMeasures, stripDirectives

**`ParseDSL`** (line ~1051):
- Signature: `func ParseDSL(lines []string) DSLResult`
- Remove call to `stripDirectives` — the first line's directives are handled by `scanMeasureDirectives`
- Remove call to `splitMeasures` — lines are already separated
- `hasBarline` → `len(lines) > 1`
- `hasInitialMeter` → determined by pre-scanning the first line's tokens with `scanMeasureDirectives`
- Default key/meter: C major, 4/4
- Each line is tokenized separately via `tokenize(line)`
- Validation and pickup detection still use `hasSecondMeasure` (= `mi < len(lines)-1`)
- The per-measure loop iterates `for mi, line := range lines`

**`splitMeasures`** (line ~724):
- Delete entirely. No longer needed.

**`stripDirectives`** (line ~534):
- Delete entirely. The first line's directives are extracted by the normal per-measure `scanMeasureDirectives` call.

**`hasInitialMeter` determination** (new logic at top of ParseDSL):
```go
lines := sanitized  // from caller
if len(lines) == 0 {
    return DSLResult{Err: fmt.Errorf("no input")}
}

currentFifths := 0
currentTimeNum := 4
currentTimeDen := 4

// Pre-scan first line for initial meter/key
firstTokens := tokenize(lines[0])
md0 := scanMeasureDirectives(firstTokens, currentFifths, currentTimeNum, currentTimeDen)
currentFifths = md0.fifths
if md0.explicitMeter {
    currentTimeNum = md0.timeNum
    currentTimeDen = md0.timeDen
}
hasMultipleLines := len(lines) > 1
hasInitialMeter := md0.explicitMeter
```

**Update comment** (line ~1050):
```
// Key signature (K...), meter (M...), and beat duration (B...) directives
// are parsed from the DSL itself. Defaults: C major, 4/4.
// Measures are separated by newlines. Each measure can have its own directives.
```

### 3. `m4bon.go` — Compile and RenderOptions

Update both functions:
```go
func Compile(dsl string) (string, error) {
    lines := parser.SanitizeDSL(dsl)
    if len(lines) == 0 {
        return "", fmt.Errorf("empty DSL after sanitization")
    }
    result := parser.ParseDSL(lines)
    // ...
}
```

Update comment block (line ~9):
Remove `|` from example: `m4bon.Compile("KE& (c) (-e) (-g)\n(-f) (d-) (b-)\n(ce) - -")`

### 4. `cmd/m4bon/main.go` — CLI entry

In the `main()` function, the DSL string from arg or file is passed through `Compile`/`Render`.
The `SanitizeDSL` + `ParseDSL` calls happen inside those library functions, so no direct
change to main.go is needed.

However, check for any `-time` flag handling (if it still exists) and usage text that
mentions `|`.

### 5. `cmd/m4bon/tui/model.go` and `cmd/m4bon/tui/main.go` — TUI

Update the direct `parser.SanitizeDSL` + `parser.ParseDSL` calls:

```go
// model.go ~line 294
sanitized := parser.SanitizeDSL(dsl)
result := parser.ParseDSL(sanitized)
```

```go
// main.go ~line 36
sanitized := parser.SanitizeDSL(dslText)
result := parser.ParseDSL(sanitized)
```

### 6. Test data files — `test/cases/*.dsl`

Replace `|` with newlines in all 17 affected files:

| File | Before | After |
|------|--------|-------|
| `voice-poly.dsl` | `M4/4 (c) (-e) (-g) \| (-f) (d-) (b-) \| (ce) - -` | 3 lines |
| `voice-poly-sustain.dsl` | `M4/4 (cde) \| (--g)` | 2 lines |
| `render-rest-note.dsl` | `KF M4/4 ^&d &c/&e ^b ;a \| 2g^ff fd b/d \|` | 3 lines |
| `render-multi.dsl` | `M4/4 c d e f \| a b c d` | 2 lines |
| `render-lyrics.dsl` | `M4/4 c d e f :L My heart is sad \|` | 2 lines (trailing `|` removed) |
| `render-chords.dsl` | `M4/4 c d e f :H C - G7 - \|` | 2 lines (trailing `|` removed) |
| `render-chords-lyrics.dsl` | `M4/4 c d e f :H C - G7 - :L My heart is sad \|` | 2 lines (trailing `|` removed) |
| `pickup-measure.dsl` | `M4/4 c d \| a b c d \|` | 2 lines |
| `multi-measure.dsl` | `M4/4 c d e f \| a b c d \|` | 2 lines |
| `error-voicepoly-no-antecedent.dsl` | `M4/4 (c - e) \| (- - g)` | 2 lines |
| `error-underfill.dsl` | `M4/4 c d \|` | 2 lines (trailing `|` removed) |
| `error-overfill.dsl` | `M4/4 c d e f g \|` | 2 lines (trailing `|` removed) |
| `error-no-sustain-pitch.dsl` | `M4/4 - \|` | 2 lines (trailing `|` removed) |
| `cross-bar-tie.dsl` | `M4/4 c d e f \| - g a \|` | 2 lines |
| `cross-bar-chord-tie.dsl` | `a b c (dfa) \| - - - - \|` | 2 lines |
| `beat-directive.dsl` | `BQ. a b c \| d e f \|` | 2 lines |
| `beat-change-tie.dsl` | `a b c d \| BQ. -f g \|` | 2 lines |

**Important**: Each measure must be on its own line. Trailing `|` after the last measure
is dropped entirely. Single-line files (like `basic-notes.dsl`: one line, no `|`) stay unchanged.

**Golden files** (`*.expected.mxml`): No changes needed — the MusicXML output is identical
since the same events are produced.

### 7. Go test files — inline DSL strings with `|`

**`parser/parse_test.go`** — Inline `ParseDSL` calls:
- `ParseDSL("M4/4 c d e f :H C - G7 - |")` → use `\n` or pass as `[]string`
- `ParseDSL("M4/4 c d e f :L My heart is sad |")` → same
- `ParseDSL("M4/4 c d e f :H C - G7 - :L My heart is sad |")` → same
- Any other calls with `|`

Since `ParseDSL` now takes `[]string`, convert inline single-string calls to either:
```go
r := ParseDSL([]string{"M4/4 c d e f :H C - G7 -"})
// or
r := ParseDSL(strings.Split(strings.ReplaceAll("M4/4 c d e f", "|", "\n"), "\n"))
```

The simplest approach for tests: pass `[]string` literals directly.
For multi-measure tests: `ParseDSL([]string{"c d e f", "a b c d"})`

**`render/render_test.go`** — Same treatment.
Multi-measure: `parser.ParseDSL([]string{"M4/4 c d e f", "a b c d"})`
Single-measure with `|` trim: remove trailing `|`
E.g.: `"M4/4 c d e f :H C - G7 - |"` → `[]string{"M4/4 c d e f :H C - G7 -"}`

**`render/html_test.go`** — Same treatment.

**`midi/generate_test.go`** — Same treatment. The `parseDSL` helper changes:
```go
func parseDSL(t *testing.T, lines ...string) []parser.MeasureResult {
    t.Helper()
    result := parser.ParseDSL(lines)
    // ...
}
```
Then: `parseDSL(t, "c d e f")` (single-line, works)
And: `parseDSL(t, "M4/4 c d e f", "a b c d")` (multi-line)

**`cmd/m4bon/main_test.go`** — TestCLIBarlineSplit uses `exec.Command` with inline DSL.
Since the CLI arg joins with spaces: `"a b - -c"` is a single-line single-measure input.
No change needed — it tests the invisible barline split, not `|` separator.

### 8. `m4bon.go` — comment block

Update the `Compile` example to show multi-line DSL:
```go
//  xml, err := m4bon.Compile("KE&\n(c) (-e) (-g)\n(-f) (d-) (b-)\n(ce) - -")
```
Or remove the `|` examples and use single-line examples only.

### 9. `AGENTS.md` — documentation

Update:
- Remove `|` from "Barline (ignored)" row in symbol table
- Update Pipeline description
- Update any example snippets using `|`
- Update "one measure per line" note in Render Format section
- Remove "multi-measure barlines" from Known Limitations since it's moot

### 10. `README.md` — examples

The README uses `|` only in markdown table formatting, not in DSL examples.
No changes needed.

## Validation Checklist

After all changes:
1. `go build -o m4bon ./cmd/m4bon/` — builds cleanly
2. `go test ./parser/...` — all parser unit tests pass
3. `go test ./render/...` — all render unit tests pass
4. `go test ./cmd/m4bon/` — golden file and integration tests pass
5. `go test ./midi/...` — MIDI tests pass
6. `go test ./...` — full suite green
7. Verify all error test cases still produce correct error messages with correct measure numbers
8. Manual smoke test: `./m4bon -render "M4/4 c d e f\n a b c d"` shows two measures

## Known Risks / Edge Cases

- **Single-line, no directives**: Must still work as one measure (default 4/4).
- **Trailing/leading newlines**: Stripped by SanitizeDSL.
- **Multiple blank lines**: Stripped to nothing by SanitizeDSL.
- **Directives on every line**: Works — `scanMeasureDirectives` handles per-line extraction.
- **Cross-line sustains**: Must still work (same as cross-`|` sustains). The `priorEvents` chain is measure-based, unchanged.
- **CLI arg quoting**: In shell, `\n` in an arg requires quoting. `m4bon "line1\nline2"` may need `$'...'` in bash or the flag handles it (reads from stdin/file). But `cmd/m4bon/main.go` joins args with spaces: `strings.Join(flag.Args(), " ")`. This means CLI arg multi-line requires using `-f` with a file. For now, test via file input or Go test strings. A future enhancement could accept `\\n` in the arg string.
