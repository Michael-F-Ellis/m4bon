# Measure Actions Popover + Action Bar [DONE]

## Problem

On iPad (and desktop), there's no way to interact with a specific measure from the rendered display. To add a comment or edit a measure's DSL, the user must locate the corresponding line in the textarea — a visually disconnected and fiddly process on touch.

## Overview

Two complementary entry points for measure-level actions:

1. **Measure number popover** — tap the measure number in the rendered display → popover with "Edit", "Add comment", "Clear comments"
2. **Action bar** — a persistent toolbar between the textarea and the rendered display showing the same actions for the currently cursor-linked measure

Both call the same underlying methods.

---

## Changes

### 1. Go: `render/html.go` — `data-idx` + clickable measure number

**`FormatHTMLRows`**: Add `data-idx="N"` to each `.m4bon-measure` div.

Wrap the `.m4bon-measure-num` span content in a clickable element:

```go
// Before:
b.WriteString(`<span class="m4bon-measure-num">`)
b.WriteString(cellToHTML(noteCells[0], asciiLeaps))
b.WriteString("</span>")

// After:
idx := row.MeasureIdx  // need to thread this through
b.WriteString(fmt.Sprintf(`<span class="m4bon-measure-num" data-idx="%d" tabindex="0" role="button">`, idx))
b.WriteString(cellToHTML(noteCells[0], asciiLeaps))
b.WriteString("</span>")
```

This requires threading the measure index through `BuildRows` → `MeasureRow` (new field `MeasureIdx int`) and `FormatHTMLRows` → the loop variable. Currently `BuildRows` doesn't pass the index — it's available from the `measures` slice index.

### 2. Go: `render/cell.go` — add `MeasureIdx` to `MeasureRow`

```go
type MeasureRow struct {
    MeasureIdx            int      // new
    CommentCells          CellSeq
    ChordCells            CellSeq
    NoteCells             CellSeq
    LyricCells            CellSeq
    TrailingCommentCells  CellSeq
}
```

Set it in `BuildRows`:

```go
row := MeasureRow{MeasureIdx: mi, ...}
```

### 3. Go: `render/html.go` — update `FormatHTMLRows` signature

`FormatHTMLRows` already takes `maxChordW, maxNoteW, maxLyricW`. No signature change needed — the index comes from `row.MeasureIdx`.

> **FormatHTML** (non-rows variant) also renders `.m4bon-measure-num` — add `data-idx` there too for consistency, even though the web app uses `FormatHTMLRows`.

### 4. CSS: `web/app.css` — measure number as interactive trigger

```css
.m4bon-measure-num {
  cursor: pointer;
}
.m4bon-measure-num:hover {
  color: var(--accent);
}
.m4bon-measure-num:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
  border-radius: 2px;
}

/* Action bar between textarea and measures */
#measure-actions {
  display: none;
  padding: 6px 16px;
  gap: 8px;
  flex-shrink: 0;
}
#measure-actions.visible {
  display: flex;
  align-items: center;
}
#measure-actions .action-label {
  color: var(--muted);
  font-size: 12px;
  margin-right: 4px;
}
#measure-actions button {
  font-size: 12px;
  padding: 3px 8px;
}

/* Popover overlay */
.m4bon-popover {
  position: absolute;
  z-index: 100;
  background: var(--surface);
  border: 1px solid var(--overlay);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0,0,0,0.4);
  padding: 4px;
  min-width: 140px;
}
.m4bon-popover button {
  display: block;
  width: 100%;
  background: none;
  border: none;
  color: var(--text);
  padding: 8px 14px;
  text-align: left;
  font-size: 14px;
  border-radius: 4px;
  cursor: pointer;
  min-height: 38px;  /* touch target */
}
.m4bon-popover button:hover,
.m4bon-popover button:focus-visible {
  background: var(--overlay);
}
.m4bon-popover .popover-divider {
  height: 1px;
  background: var(--overlay);
  margin: 2px 8px;
}
.m4bon-popover .popover-danger {
  color: var(--red);
}
```

### 5. JS: `web/app.js` — new methods

**Event wiring** (in `updateMeasures()`, after setting innerHTML):

```js
// Measure number clicks → popover
this.measuresEl.querySelectorAll('.m4bon-measure-num').forEach(el => {
  el.addEventListener('click', (e) => this.showPopover(e, parseInt(el.dataset.idx)));
  el.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      this.showPopover(e, parseInt(el.dataset.idx));
    }
  });
});
```

**`showPopover(event, idx)`**: Create a popover div positioned at `event.target`, insert into DOM, with buttons:

| Button | Action |
|---|---|
| `✎ Edit measure` | `focusMeasure(idx)` + close popover |
| `+ Add comment above` | `insertCommentBefore(idx)` + close |
| `— Clear comments` | `clearComments(idx)` + close (only shown if measure has comments) |

Click-outside and Escape dismiss the popover.

**`focusMeasure(idx)`**: Compute the line index in the textarea for the Nth measure (accounting for blank lines, `#` lines, and `!` blocks), set `selectionStart`/`selectionEnd` to start of that line, scroll textarea to that position, focus the textarea.

```js
focusMeasure(idx) {
  const lines = this.dslInput.value.split('\n');
  let measureLine = 0;
  for (let i = 0; i < lines.length; i++) {
    const t = lines[i].trim();
    if (!t || (t.startsWith('#') && (t.length === 1 || t[1] === ' '))) continue;
    if (t.startsWith('!')) continue;
    if (measureLine === idx) {
      const pos = lines.slice(0, i).join('\n').length + (i > 0 ? 1 : 0);
      this.dslInput.focus();
      this.dslInput.setSelectionRange(pos, pos);
      this.dslInput.scrollTop = ...; // approximate scroll to make line visible
      this.highlightCursorMeasure();
      return;
    }
    measureLine++;
  }
}
```

`scrollTop` approximation: `(i / totalLines) * textarea.scrollHeight` — close enough to bring the line into view; `highlightCursorMeasure` + `scrollIntoView` on the rendered element handles precise positioning visually.

**`insertCommentBefore(idx)`**: Already designed — splice `! ` into the DSL at the correct line, trigger input event.

**`clearComments(idx)`**: Walk backwards from the measure's DSL line, collecting consecutive `!` lines, then remove them from the textarea value. Trigger input event.

**`highlightCursorMeasure()`**: Already exists. No changes needed — it's the mechanism the action bar uses to know which measure is active.

**Action bar** (new DOM element added in `initDOM`):

```js
// In initDOM():
this.measureActionsEl = document.getElementById('measure-actions');
document.getElementById('btn-edit-measure').addEventListener('click', () => {
  if (this._activeActionIdx >= 0) this.focusMeasure(this._activeActionIdx);
});
document.getElementById('btn-add-comment').addEventListener('click', () => {
  if (this._activeActionIdx >= 0) this.insertCommentBefore(this._activeActionIdx);
});
document.getElementById('btn-clear-comments').addEventListener('click', () => {
  if (this._activeActionIdx >= 0) this.clearComments(this._activeActionIdx);
});
```

The action bar visibility and target index update inside `highlightCursorMeasure()`:

```js
// At end of highlightCursorMeasure(), after setting cursor class:
this._activeActionIdx = renderedIdx >= 0 && renderedIdx < totalMeasureCount ? renderedIdx : -1;
this.measureActionsEl.classList.toggle('visible', this._activeActionIdx >= 0);
document.getElementById('btn-clear-comments').style.display =
  measureHasComments ? '' : 'none';
```

### 6. HTML: `web/index.html` — new action bar element

Insert between `#dsl-area` and `#virtual-keypad`:

```html
<div id="measure-actions">
  <span class="action-label">Measure <span id="action-measure-idx">1</span>:</span>
  <button id="btn-edit-measure">✎ Edit</button>
  <button id="btn-add-comment">+ Comment</button>
  <button id="btn-clear-comments">— Clear cmts</button>
</div>
```

---

## Touch considerations (iPad)

- Popover buttons have `min-height: 38px` for touch targets
- Popover dismisses on tap outside (touch event on backdrop)
- Action bar buttons are `min-height: 32px` with text labels, not icons alone
- `tabindex` on measure number enables keyboard accessibility
- `role="button"` for screen readers

---

## Scope summary

| File | Change |
|---|---|
| `render/cell.go` | Add `MeasureIdx int` field to `MeasureRow` |
| `render/render.go` | Set `MeasureIdx` in `BuildRows` loop |
| `render/html.go` | Add `data-idx` to `.m4bon-measure` div; wrap measure num in clickable span with `tabindex`+`role` |
| `web/app.css` | ~40 lines: measure num cursor, action bar, popover, popover buttons |
| `web/app.js` | ~80 lines: `showPopover`, `focusMeasure`, `insertCommentBefore`, `clearComments`, `showPopover`, action bar wiring, event delegation in `updateMeasures` |
| `web/index.html` | ~6 lines: action bar HTML element |
