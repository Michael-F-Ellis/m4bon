# Virtual Entry Keypad for Tablet UX — Implementation Plan

## Goal

Improve the editing UX on touch-first tablet devices (specifically iPads) by providing a toggleable on-screen virtual entry pad. This solves the clumsy OS keyboard switching required to enter notation symbols like `^`, `-`, `&`, `/`, and `;`, and keeps entry lightning-fast using m4bon's compact vocabulary.

---

## Technical Approach

### 1. Suppressing the Native Keyboard
To display our virtual keypad, we must allow the `#dsl-input` textarea to have focus (preserving selection, blinking cursor, and copy/paste handles) but prevent the system keyboard from sliding up.
- **Keypad Enabled:** Add `inputmode="none"` to `#dsl-input`.
- **Keypad Disabled (Hardware / Default):** Remove `inputmode` attribute from `#dsl-input`.

### 2. Layout & Styling (CSS)
The keypad will be a CSS grid docked at the bottom of the screen.
- Layout organizes keys musically, numerically, and by modifier type to minimize hand travel.
- Keys should have `touch-action: manipulation` to eliminate the 300ms mobile tap delay.
- Sized with `vh` or dynamic flex sizing to fit beautifully beneath the `#measures` scroll area without overlapping content.

### 3. Insertion Mechanics (JS)
All virtual key taps must manipulate the real `#dsl-input` cursor range:
1. Capture `selectionStart` and `selectionEnd` of `#dsl-input`.
2. Insert character(s) at that position.
3. Advance selection cursor and restore focus to `#dsl-input`.
4. Trigger the normal input pipeline by dispatching a synthetic `InputEvent`.

---

## Detailed UI Layout

We will arrange the grid in three functional blocks:
- **Top Row (Durations & Chords):** `1` `2` `3` `4` `8` `(` `)` `:` `Backspace`
- **Middle Row (Notes & Octaves):** `c` `d` `e` `f` `g` `a` `b` `^` `/`
- **Bottom Row (Accidentals, Separators & Navigation):** `♯` `♭` `♮` `;` `-` `Space` `New Measure`

*Note: The UI labels `♯`, `♭`, and `♮` will automatically insert the normalized characters `#`, `&`, and `%` into the textarea.*

---

## Step-by-Step Execution Plan

### Phase 1: HTML Structure (`web/index.html`)

1. Add a **Keypad Toggle Button** to the transport or top bar next to the existing layout controls:
   ```html
   <button id="btn-toggle-keypad" title="Toggle On-Screen Keypad">⌨️ Keypad</button>
   ```
2. Define the **Keypad Container & Buttons** inside `#app`, positioned after `#status-bar` or inside a bottom-docking area:
   ```html
   <div id="virtual-keypad" class="hidden">
     <!-- Row 1: Numbers & Grouping -->
     <div class="keypad-row">
       <button class="key-num" data-val="1">1</button>
       <button class="key-num" data-val="2">2</button>
       <button class="key-num" data-val="3">3</button>
       <button class="key-num" data-val="4">4</button>
       <button class="key-num" data-val="8">8</button>
       <button class="key-syntax" data-val="(">(</button>
       <button class="key-syntax" data-val=")">)</button>
       <button class="key-syntax" data-val=":">:</button>
       <button class="key-edit key-backspace" data-action="backspace">⌫</button>
     </div>
     <!-- Row 2: Pitch & Octaves -->
     <div class="keypad-row">
       <button class="key-pitch" data-val="c">c</button>
       <button class="key-pitch" data-val="d">d</button>
       <button class="key-pitch" data-val="e">e</button>
       <button class="key-pitch" data-val="f">f</button>
       <button class="key-pitch" data-val="g">g</button>
       <button class="key-pitch" data-val="a">a</button>
       <button class="key-pitch" data-val="b">b</button>
       <button class="key-octave" data-val="^">^</button>
       <button class="key-octave" data-val="/">/</button>
     </div>
     <!-- Row 3: Modifiers & Spacing -->
     <div class="keypad-row">
       <button class="key-accidental" data-val="#">♯</button>
       <button class="key-accidental" data-val="&">♭</button>
       <button class="key-accidental" data-val="%">♮</button>
       <button class="key-rest" data-val=";">;</button>
       <button class="key-tie" data-val="-">-</button>
       <button class="key-space" data-val=" ">Space</button>
       <button class="key-enter" data-action="enter">⏎ Measure</button>
     </div>
   </div>
   ```

---

### Phase 2: CSS Layout & Styles (`web/app.css`)

Ensure the app shell respects the virtual keyboard's space when shown, avoiding layouts that overflow the viewport or break scrolling.

1. **Flex integration:**
   Make `#app` adjust dynamically:
   ```css
   #virtual-keypad {
     background: var(--surface);
     border-top: 1px solid var(--overlay);
     padding: 8px;
     display: flex;
     flex-direction: column;
     gap: 6px;
     flex-shrink: 0;
   }
   #virtual-keypad.hidden {
     display: none;
   }
   ```
2. **Keypad Row Grid styling:**
   ```css
   .keypad-row {
     display: flex;
     gap: 6px;
     width: 100%;
   }
   .keypad-row button {
     flex: 1;
     font-size: 18px;
     font-weight: 600;
     height: 46px;
     touch-action: manipulation;
     background: var(--overlay);
     border-color: var(--muted);
     border-radius: 6px;
     display: flex;
     align-items: center;
     justify-content: center;
   }
   .keypad-row button:active {
     background: var(--purple);
     color: #fff;
   }
   .keypad-row button.key-space {
     flex: 2;
     background: var(--surface);
   }
   .keypad-row button.key-enter {
     flex: 2;
     background: var(--purple);
     color: #fff;
     border-color: var(--accent);
   }
   .keypad-row button.key-backspace {
     background: var(--red);
     color: #fff;
     border-color: var(--red);
   }
   ```

---

### Phase 3: Controller Logic (`web/app.js`)

1. **State initialization:**
   Add `this.keypadActive = false;` to `M4bonApp` constructor. Load state from local storage so preference is saved.
2. **Setup Events:**
   Wire up elements in `initDOM()`:
   ```javascript
   this.btnToggleKeypad = document.getElementById('btn-toggle-keypad');
   this.keypadEl = document.getElementById('virtual-keypad');

   this.btnToggleKeypad.addEventListener('click', () => this.toggleKeypad());

   // Event delegation for all keypad button actions
   this.keypadEl.addEventListener('click', (e) => {
     const btn = e.target.closest('button');
     if (!btn) return;
     this.handleKeypadPress(btn);
   });
   ```
3. **Toggle Handler:**
   ```javascript
   toggleKeypad(forceState) {
     this.keypadActive = forceState !== undefined ? forceState : !this.keypadActive;
     if (this.keypadActive) {
       this.btnToggleKeypad.classList.add('active');
       this.btnToggleKeypad.style.background = 'var(--purple)';
       this.dslInput.setAttribute('inputmode', 'none');
       this.keypadEl.classList.remove('hidden');
     } else {
       this.btnToggleKeypad.classList.remove('active');
       this.btnToggleKeypad.style.background = '';
       this.dslInput.removeAttribute('inputmode');
       this.keypadEl.classList.add('hidden');
     }
     localStorage.setItem('m4bon_keypad_active', this.keypadActive);
     this.autoResizeTextarea();
   }
   ```
4. **Keypress Handler (Cursor Manipulation):**
   ```javascript
   handleKeypadPress(btn) {
     const input = this.dslInput;
     const start = input.selectionStart;
     const end = input.selectionEnd;
     const text = input.value;

     const val = btn.dataset.val;
     const action = btn.dataset.action;

     let inserted = '';
     let newStart = start;

     if (val !== undefined) {
       inserted = val;
       newStart = start + val.length;
       input.value = text.substring(0, start) + val + text.substring(end);
     } else if (action === 'backspace') {
       if (start !== end) {
         input.value = text.substring(0, start) + text.substring(end);
         newStart = start;
       } else if (start > 0) {
         input.value = text.substring(0, start - 1) + text.substring(end);
         newStart = start - 1;
       }
     } else if (action === 'enter') {
       inserted = '\n';
       newStart = start + 1;
       input.value = text.substring(0, start) + '\n' + text.substring(end);
     }

     input.focus();
     input.setSelectionRange(newStart, newStart);

     // Trigger input event to re-run parser
     const event = new Event('input', { bubbles: true });
     input.dispatchEvent(event);
   }
   ```

---

## Verification & UX Checklist

- [ ] Suppressed Keyboard: When `#dsl-input` has focus and Keypad is enabled, tapping inside the textarea must NOT reveal the iOS system keyboard.
- [ ] Working Caret: Standard blinking text caret must remain visible while editing via the virtual keypad.
- [ ] Fast Rendering: Each tap on a pitch key (e.g., `c`) instantly updates the measure visualization without lag.
- [ ] Backspace Precision: Correctly deletes a single character if no text is selected, or the entire selection range.
- [ ] Responsive Height: Sizing works perfectly on both portrait and landscape iPad resolutions.
