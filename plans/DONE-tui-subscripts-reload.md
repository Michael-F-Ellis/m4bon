# TUI Subscript Toggle & File Auto-Reload

## Goal

1. **Subscript toggle**: Press `o` to show/hide octave subscripts in the TUI.
   Defaults to off. Toggle state survives across reloads.
2. **Auto-reload**: When input is from a file (`-f` flag), watch the file and
   re-parse when it changes on disk. Poll mtime every 500ms.
3. **Manual reload**: Press `u` to force re-parse of the current DSL (works
   for both file and non-file sources).

## File Changes

### 1. `cmd/m4bon/tui/model.go` — New fields

```go
type model struct {
    // ... existing fields ...
    showSubscripts bool          // toggled by 'o'
    sourceFile     string        // path to .dsl file (empty if from arg)
    sourceFileMod  time.Time     // last known mtime of source file
}
```

Set `sourceFile` from `dslLabel` when it's a file path (not "arg" or empty).

### 2. `cmd/m4bon/tui/model.go` — Regenerate render lines

Add a helper that re-renders the renderLines from measures, respecting
`showSubscripts`. Since subscripts are embedded in the ANSI string output
by the render pipeline, we can't toggle them post-hoc. Instead:

**Option A**: Re-render from measures when toggling. Fast since measures
are already parsed.

**Option B**: Store render output for both on/off states.

Option A is simpler. The render pass is cheap.

Actually, `render.Render` always includes subscripts. We need a way to
suppress them. Easiest: add a `showSubscripts` parameter to the render
pipeline, or strip subscripts from the Cell output.

**Decision**: Add `subscripts bool` to `render.Render()` → `BuildCells()` →
`buildMeasureCells()`. When false, `octaveSubscript` returns "" always.

```go
// render/render.go
func Render(measures []parser.MeasureResult, asciiLeaps bool, subscripts bool) string
```

Actually, we need to keep the render pipeline testable without the TUI.
Simpler: make `BuildCells` accept `showSubscripts`, and `Render` passes
it through.

### 3. `cmd/m4bon/tui/update.go` — New key handlers

- `o` → toggle `m.showSubscripts`, re-render

### 4. `cmd/m4bon/tui/update.go` — Reload function

- `u` → re-read source file (if any) or rebuild from stored `m.dslText`,
  re-parse, regenerate SMF

### 5. `cmd/m4bon/tui/model.go` — File watcher tick

```go
func (m *model) watchFileTick() tea.Cmd {
    return tea.Every(500*time.Millisecond, func(t time.Time) tea.Msg {
        if m.sourceFile == "" {
            return nil
        }
        info, err := os.Stat(m.sourceFile)
        if err != nil {
            return nil
        }
        if info.ModTime().After(m.sourceFileMod) {
            return fileChangedMsg{info.ModTime()}
        }
        return nil
    })
}
```

On `fileChangedMsg`: re-read file, re-parse, re-render, regenerate SMF,
seek to top of score.

### 6. Help text update

Add `o` and `u` to the help view.

## Step Order

1. Add `showSubscripts` field to model, set false
2. Thread `showSubscripts` through `render.Render` → `BuildCells` →
   `buildMeasureCells` → `octaveSubscript`
3. Update callers (CLI `-render` always passes true, TUI uses toggle state)
4. Add `o` key handler for subscript toggle
5. Add `sourceFile`/`sourceFileMod` fields to model
6. Add `watchFileTick` and `fileChangedMsg`
7. Handle `fileChangedMsg` in Update
8. Add `u` key handler for manual reload
9. Add reload logic (re-read, re-parse, re-SMF)
10. Update help text with new keybindings
11. Update golden render tests
12. Run full test suite
