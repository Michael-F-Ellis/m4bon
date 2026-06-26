# Resizable & Auto-Expanding Textarea + Auto-Reformat — Implementation Plan

## Goal

Improve the web app editing experience:
1. Make the DSL textarea resizable via drag handle and auto-grow with content
2. Highlight the rendered measure corresponding to cursor position in the editor
3. Auto-reformat the DSL source in the textarea into neat columns aligned on :H (chords) and :L (lyrics) whenever the parse succeeds

## Current State

`web/app.css` line 71: `height: 60px; resize: none;`

## Changes

### 1. `web/app.css` — Resizable + auto-height foundation

- Replace `resize: none` → `resize: vertical`
- Replace `height: 60px` → `min-height: 60px; max-height: 40vh`

### 2. `web/app.css` — Cursor measure highlight style

- New `.m4bon-measure.m4bon-cursor` class: subtle blue bg + accent left border

### 3. `web/app.js` — Auto-resize textarea

- `autoResizeTextarea()`: resets to `auto`, clamps to `min(scrollHeight, 40vh)`
- Called on input, window resize, and state restore

### 4. `web/app.js` — Cursor-to-measure highlighting

- `highlightCursorMeasure()`: counts `|` before cursor to find measure index, applies `.m4bon-cursor` class, scrolls into view
- Called on `keyup`, `click`, and after `updateMeasures()`

### 5. `web/app.js` — Auto-reformat on successful parse

- `reformatColumns(dsl)`: extracts `:H` chord tokens and `:L` lyric tokens from each measure (matching parser's `extractDirectivesTail` state machine), computes max column widths, rebuilds with padding so `:H` and `:L` columns align vertically
- Called in debounced `onDSLChange` after a successful parse (checked via `this.parsedData`)
- Guarded: only updates textarea if reformatted output differs from current value

## File Changes

| File | Change |
|------|--------|
| `web/app.css` | `#dsl-input`: resize, min/max-height; new `.m4bon-cursor` class |
| `web/app.js` | `autoResizeTextarea()`, `highlightCursorMeasure()`, `reformatColumns()`, wire into debounce
