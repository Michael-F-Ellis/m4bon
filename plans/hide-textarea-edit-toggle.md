# Hide Textarea by Default + Remove Action Bar

## Problem

Two UX annoyances after the measure actions popover was added:

1. **The textarea is always visible**, consuming ~40% of vertical space on iPad. The rendered display is what the user wants to read during rehearsal; the textarea is only needed when editing.

2. **The action bar (`#measure-actions`) is redundant.** The popover (triggered by tapping a measure number) already provides Edit/Add Comment/Clear Comments. A second, always-visible bar keyed to cursor position adds clutter without value — especially since the user's primary interaction is "see measure, tap number, act."

## Design

### Three display modes

| Mode | Textarea | Keypad | Trigger |
|---|---|---|---|
| **View** | hidden | hidden | Default state on load |
| **Keypad** | visible | visible | Tap "Edit" button or popover "Edit measure" |
| **Text-only** | visible | hidden | Popover "Edit measure" on desktop (no keypad) |

### Interaction flow

- **App loads in View mode.** Rendered measures fill the full vertical space between top-bar and transport. No textarea, no keypad, no action bar.

- **Tap a measure number** → popover appears (same as today). Actions:
  - "Edit measure" → switches to Keypad/Text-only mode, textarea cursor at that measure's line
  - "Add comment above" → inserts `! ` line, switches to Keypad/Text-only mode, cursor after `! `
  - "Clear comments" → removes `!` lines, stays in current mode, re-renders

- **"Edit" button in transport** replaces the current "⌨️ Keypad" button. Toggles between View mode and Keypad mode. Label: "✎ Edit" in View mode, "✓ Done" in edit modes.

- **Escape key** or focus loss on textarea → return to View mode.

- **Comments toggle** When comments are visible in View mode, the `! ` lines appear above measures as today. Hidden when `Cmts` checkbox is off.

### Popover gets an update

Remove the divider and restyle to three clear options:

```
┌─────────────────┐
│ ✎ Edit measure   │
│ + Add comment    │
│ — Clear comments │
└─────────────────┘
```

No divider. No separate "danger" style on clear — it's already clear from context. All three are same-height touch targets (38px min).

---

## Changes

### 1. HTML: `web/index.html`

**Remove** the `#measure-actions` block entirely (lines 22-27).

**Change** `#btn-toggle-keypad` to `#btn-toggle-edit` with label "✎ Edit".

Rename in JS too: `btnToggleKeypad` → `btnToggleEdit`.

### 2. CSS: `web/app.css`

**Remove** all `#measure-actions` rules (`.action-label`, `#measure-actions button`, `#measure-actions.visible`, etc.).

**Add** `#dsl-area` view-mode hiding:

```css
/* Hide textarea + keypad area in view mode */
#app.view-mode #dsl-area {
  display: none;
}
#app.view-mode #virtual-keypad {
  display: none;
}
#app.edit-mode #dsl-area {
  display: block;
}
```

Use a class on `#app` to toggle modes — this cascades cleanly to both `#dsl-area` and `#virtual-keypad`:

```css
#app.view-mode #dsl-area,
#app.view-mode #virtual-keypad {
  display: none;
}
```

**Update** `#measures` to fill available height in view mode:

```css
#app.view-mode #measures {
  flex: 1;
}
```

Currently `#measures` already has `flex: 1` and `overflow-y: auto` — this should work as-is. The textarea disappearing means the flex container reallocates that space.

**Update** popover button styles (already exist as `.m4bon-popover button`). Remove the divider and danger-specific rules if they exist.

**Remove** `.popover-divider` and `.popover-danger` rules.

### 3. JS: `web/app.js`

#### New state and elements

```js
this.editMode = false;  // replaces keypadActive
```

Remove `this.keypadActive`. Replace in constructor init.

`initDOM` changes:

- `this.btnToggleEdit = document.getElementById('btn-toggle-edit');`
- Remove `this.measureActionsEl`, `this.actionMeasureIdxEl`, `this.btnEditMeasure`, `this.btnAddComment`, `this.btnClearComments`

#### New method: `toggleEditMode(forceState)`

Replaces `toggleKeypad`. Logic:

```js
toggleEditMode(forceState) {
  this.editMode = forceState !== undefined ? forceState : !this.editMode;
  if (this.editMode) {
    this.appEl.classList.remove('view-mode');
    this.appEl.classList.add('edit-mode');
    this.btnToggleEdit.textContent = '✓ Done';
    this.btnToggleEdit.style.background = 'var(--purple)';
    this.keypadEl.classList.remove('hidden');
    this.dslInput.inputMode = 'none';
    this.dslInput.focus();
  } else {
    this.appEl.classList.remove('edit-mode');
    this.appEl.classList.add('view-mode');
    this.btnToggleEdit.textContent = '✎ Edit';
    this.btnToggleEdit.style.background = '';
    this.keypadEl.classList.add('hidden');
    this.dslInput.inputMode = '';
    this.dslInput.blur();
  }
  localStorage.setItem('m4bon-edit-mode', this.editMode);
}
```

#### Updated `focusMeasure(idx)`

After setting cursor position:

```js
// Switch to edit mode so user can start typing
this.toggleEditMode(true);
```

This ensures tapping "Edit measure" in the popover opens the textarea at the right spot.

#### Updated `insertCommentBefore(idx)`

After inserting `! ` line and setting cursor:

```js
this.toggleEditMode(true);
```

Same — entering a comment should open the editor.

#### Updated popover content

```js
popover.innerHTML =
  '<button data-action="edit">✎ Edit measure</button>' +
  '<button data-action="comment">+ Add comment above</button>' +
  '<button data-action="clear">— Clear comments</button>';
```

No divider. All three are flat buttons.

#### Remove action bar wiring

Remove:
- `this._updateActionBar()` method
- `_updateActionBar` call from `highlightCursorMeasure`
- `_activeActionIdx` tracking
- All event listeners for action bar buttons from `initDOM`

#### Remove `_updateActionBar` method entirely

#### Update `parseAndRender` / `updateMeasures`

The action bar no longer exists, so no need to update it after render.

#### Update `loadState` / `saveState`

- `localStorage.setItem('m4bon-edit-mode', this.editMode)` instead of `m4bon-keypad-active`
- On load: if `editMode` was true, call `toggleEditMode(true)`

#### Update `onDSLChange`

Remove any action bar references. The debounce + parse cycle is unchanged.

### 4. Popover HTML change

Remove the `popover-divider` from the popover template in `showPopover()`. Remove the `popover-divider` and `popover-danger` CSS classes (or leave them as dead CSS, harmless).

### 5. Edge cases

- **Playback during edit mode**: Tapping Play should switch to View mode (or at least not break). Simple: `play()` can call `this.toggleEditMode(false)` before starting playback. The textarea doesn't need to be visible during playback.

- **Recording**: Same — switch to View mode when recording starts. The measure highlight handles visual feedback.

- **Empty state**: On load with no DSL, View mode shows "Enter DSL above to see rendered output" placeholder. The "✎ Edit" button is available to start typing. Also: consider auto-switching to edit mode if no DSL is entered yet (first-time user).

- **Desktop keyboard shortcuts**: `Esc` exits edit mode. `Cmd/Ctrl+S` reformats (already works). Space plays/pauses (already works). When in edit mode, textarea captures most keys; when in view mode, keyboard shortcuts for transport work without textarea interference.

---

## Files changed

| File | Change |
|---|---|
| `web/index.html` | Remove `#measure-actions` div, rename `btn-toggle-keypad` to `btn-toggle-edit`, update label |
| `web/app.css` | Remove action bar styles, add `.view-mode`/`.edit-mode` rules for `#dsl-area` and `#virtual-keypad`, remove `.popover-divider`/`.popover-danger` |
| `web/app.js` | Replace `keypadActive` with `editMode`, replace `toggleKeypad` with `toggleEditMode`, update `showPopover` template, add edit-mode entry to `focusMeasure`/`insertCommentBefore`, remove `_updateActionBar` and all action bar wiring, update `loadState`/`saveState`, update `togglePlay` to exit edit mode, update keyboard shortcut for edit toggle |

