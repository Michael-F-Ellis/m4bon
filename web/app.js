// m4bon Web TUI — Application Logic

const KEY_NAMES = {
  '-7': 'C♭ major', '-6': 'G♭ major', '-5': 'D♭ major', '-4': 'A♭ major',
  '-3': 'E♭ major', '-2': 'B♭ major', '-1': 'F major', '0': 'C major',
  '1': 'G major', '2': 'D major', '3': 'A major', '4': 'E major',
  '5': 'B major', '6': 'F♯ major', '7': 'C♯ major'
};

// --- WASM Bootstrap ---
const go = new Go();

async function bootstrapWASM() {
  try {
    const resp = await fetch('m4bon.wasm');
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const bytes = await resp.arrayBuffer();
    const result = await WebAssembly.instantiate(bytes, go.importObject);
    // Start the Go program. This will run main(), set up the exported
    // globals (m4bonParse, etc.), set _m4bonReady, and then block.
    // go.run() is async and won't resolve until the program exits,
    // but after the first yield the globals are all available.
    go.run(result.instance);
  } catch (e) {
    document.getElementById('measures').innerHTML =
      '<div class="placeholder" style="color:var(--red)">Failed to load WASM: ' + e.message + '</div>';
    console.error('WASM bootstrap error:', e);
    return;
  }
}

// --- M4bonApp ---

class M4bonApp {
  constructor() {
    this.dsl = '';
    this.bpm = 120;
    this.startMeasure = 0;
    this.endMeasure = 0;
    this.showSubscripts = true;
    this.showComments = true;
    this.metronomeOn = true;
    this.rootsOn = false;
    this.backbeatsOn = false;
    this.velocity = 90;
    this.isPlaying = false;
    this.isRecording = false;
    this.parsedData = null;
    this.midiOutput = null;
    this.audioCtx = null;
    this.synthReady = false;
    this.wafPlayer = null;
    this.pianoPreset = null;
    this.masterGain = null;
    this.activeEnvelopes = {};
    this._bassNodes = [];
    this._bassTimers = [];
    this._bassSamples = null;
    this.mediaRecorder = null;
    this.audioChunks = [];
    this.recordingBlob = null;
    this.recordingURL = null;
    this.recordingAudio = null;
    this._recHighlightTimer = null;
    this._lastRecPlayingIdx = -1;
    this._recHighlightStarts = null;
    this._recordMeasureSecs = null;
    this._recordCountInSec = 0;
    this._recMimeType = '';
    this.playbackTimer = null;
    this.measureHighlightTimer = null;
    this.debounceTimer = null;
    this._scheduledNotes = null;
    this._schedulerIdx = 0;
    this._schedulerTimer = null;
    this._playbackEndTime = 0;
    this._keepAliveOsc = null;
    this._keepAliveGain = null;

    this.initDOM();
    this.loadState();
    this.render();
  }

  initDOM() {
    this.dslInput = document.getElementById('dsl-input');
    this.measuresEl = document.getElementById('measures');
    this.timeSigEl = document.getElementById('time-sig');
    this.keySigEl = document.getElementById('key-sig');
    this.tempoEl = document.getElementById('tempo');
    this.statusText = document.getElementById('status-text');
    this.statusError = document.getElementById('status-error');
    this.tempoDisplay = document.getElementById('tempo-display');
    this.rangeDisplay = document.getElementById('range-display');
    this.volumeSlider = document.getElementById('volume-slider');
    this.chkMetronome = document.getElementById('chk-metronome');
    this.chkSubscripts = document.getElementById('chk-subscripts');
    this.chkComments = document.getElementById('chk-comments');
    this.chkRoots = document.getElementById('chk-roots');
    this.chkBackbeats = document.getElementById('chk-backbeats');

    this.dslInput.addEventListener('input', () => this.onDSLChange());
    this.dslInput.addEventListener('keyup', () => this.highlightCursorMeasure());
    this.dslInput.addEventListener('click', () => this.highlightCursorMeasure());
    this.dslInput.addEventListener('blur', () => this.reformatInput());
    this.volumeSlider.addEventListener('input', () => {
      this.velocity = parseInt(this.volumeSlider.value);
    });

    document.getElementById('btn-play').addEventListener('click', () => this.togglePlay());
    document.getElementById('btn-stop').addEventListener('click', () => this.stop());
    document.getElementById('btn-record').addEventListener('click', () => this.toggleRecord());
    document.getElementById('btn-play-recording').addEventListener('click', () => this.playRecording());

    document.getElementById('btn-tempo-down').addEventListener('click', () => this.adjustTempo(-5));
    document.getElementById('btn-tempo-up').addEventListener('click', () => this.adjustTempo(5));
    document.getElementById('btn-tempo-down1').addEventListener('click', () => this.adjustTempo(-1));
    document.getElementById('btn-tempo-up1').addEventListener('click', () => this.adjustTempo(1));
    document.getElementById('btn-tempo-reset').addEventListener('click', () => this.setTempo(120));

    document.getElementById('btn-start-up').addEventListener('click', () => this.moveStartMeasure(-1));
    document.getElementById('btn-start-down').addEventListener('click', () => this.moveStartMeasure(1));
    document.getElementById('btn-end-up').addEventListener('click', () => this.moveEndMeasure(-1));
    document.getElementById('btn-end-down').addEventListener('click', () => this.moveEndMeasure(1));

    this.chkMetronome.addEventListener('change', () => {
      this.metronomeOn = this.chkMetronome.checked;
    });
    this.chkSubscripts.addEventListener('change', () => {
      this.showSubscripts = this.chkSubscripts.checked;
      this.updateMeasures();
    });
    this.chkComments.addEventListener('change', () => {
      this.showComments = this.chkComments.checked;
      this.updateMeasures();
    });
    this.chkRoots.addEventListener('change', () => {
      this.rootsOn = this.chkRoots.checked;
      if (this.rootsOn && this.synthReady && !this.bassPreset) {
        this.loadBassInstrument();
      }
    });
    this.chkBackbeats.addEventListener('change', () => {
      this.backbeatsOn = this.chkBackbeats.checked;
    });

    document.getElementById('btn-load').addEventListener('click', () => this.loadFile());
    document.getElementById('btn-save-mxl').addEventListener('click', () => this.saveMXL());
    document.getElementById('btn-save-dsl').addEventListener('click', () => this.saveDSL());
    document.getElementById('btn-copy').addEventListener('click', () => this.copyDSL());

    document.addEventListener('keydown', (e) => this.onKeyDown(e));
    window.addEventListener('resize', () => this.autoResizeTextarea());
  }

  autoResizeTextarea() {
    const el = this.dslInput;
    el.style.height = 'auto';
    const maxH = window.innerHeight * 0.4;
    el.style.height = Math.min(el.scrollHeight, maxH) + 'px';
  }

  onDSLChange() {
    this.autoResizeTextarea();
    this.highlightCursorMeasure();

    clearTimeout(this.debounceTimer);
    this.debounceTimer = setTimeout(() => {
      this.dsl = this.dslInput.value;
      this.parseAndRender();
      this.saveState();
    }, 150);
  }

  parseAndRender() {
    if (!this.dsl.trim()) {
      this.parsedData = null;
      this.measuresEl.innerHTML = '<div class="placeholder">Enter DSL above to see rendered output</div>';
      this.timeSigEl.textContent = 'M4/4';
      this.keySigEl.textContent = 'K0 (C major)';
      this.statusText.textContent = 'Ready';
      this.statusError.classList.add('hidden');
      return;
    }

    try {
      const result = JSON.parse(m4bonParse(JSON.stringify({ dsl: this.dsl })));
      if (result.err) {
        this.parsedData = null;
        this.showError(result.err);
        this.measuresEl.innerHTML = '<div class="placeholder">Parse error — check DSL syntax</div>';
        return;
      }

      this.parsedData = result.ok;
      this.updateTopBar();
      this.updateMeasures();
      this.updateRangeDisplay();
      this.statusText.textContent = `${this.parsedData.measures.length} measure(s)`;
      this.statusError.classList.add('hidden');
    } catch (e) {
      this.showError('WASM not loaded yet — please wait');
    }
  }

  updateTopBar() {
    const d = this.parsedData;
    this.timeSigEl.textContent = `M${d.timeNum}/${d.timeDen}`;
    const keyName = KEY_NAMES[String(d.keyFifths)] || `K${d.keyFifths}`;
    this.keySigEl.textContent = `K${d.keyFifths >= 0 ? '+' : ''}${d.keyFifths} (${keyName})`;
    this.tempoEl.textContent = `♩=${this.bpm}`;
  }

  updateMeasures() {
    if (!this.parsedData) return;
    try {
      const result = JSON.parse(m4bonRenderHTML(JSON.stringify({
        dsl: this.dsl,
        showSubscripts: this.showSubscripts,
        showComments: this.showComments,
        asciiLeaps: false
      })));
      if (result.ok) {
        this.measuresEl.innerHTML = result.ok;
        this.highlightMeasures();
        this.highlightCursorMeasure();
      }
    } catch (e) {
      // WASM not ready
    }
  }

  highlightMeasures() {
    const divs = this.measuresEl.querySelectorAll('.m4bon-measure');
    const total = divs.length;
    if (total === 0) return;

    divs.forEach(d => {
      d.classList.remove('m4bon-start', 'm4bon-end', 'm4bon-playing');
    });

    if (this.startMeasure > 0 && this.startMeasure < total) {
      divs[this.startMeasure].classList.add('m4bon-start');
    }

    if (this.endMeasure > 0 && this.endMeasure <= total) {
      divs[this.endMeasure - 1].classList.add('m4bon-end');
    }
  }

  highlightCursorMeasure() {
    const divs = this.measuresEl.querySelectorAll('.m4bon-measure');
    divs.forEach(d => d.classList.remove('m4bon-cursor'));

    // Suppress cursor highlight during playback or recording to avoid
    // conflicting with the moving measure-position highlight.
    if (this.isPlaying || this.isRecording) return;

    const pos = this.dslInput.selectionStart;
    const textBefore = this.dslInput.value.substring(0, pos);
    const measureIdx = (textBefore.match(/\n/g) || []).length;

    if (measureIdx < divs.length) {
      divs[measureIdx].classList.add('m4bon-cursor');
      divs[measureIdx].scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  }

  updateRangeDisplay() {
    if (!this.parsedData) {
      this.rangeDisplay.textContent = 'All';
      return;
    }
    const total = this.parsedData.measures.length;
    if (this.startMeasure === 0 && this.endMeasure === 0) {
      this.rangeDisplay.textContent = `All (${total})`;
    } else {
      const end = this.endMeasure || total;
      this.rangeDisplay.textContent = `${this.startMeasure + 1}–${end}`;
    }
  }

  // --- Auto-reformat on successful parse ---

  reformatColumns(dsl) {
    // Split raw DSL into measure lines, preserving comment lines
    const rawLines = dsl.split('\n').map(l => l.trim());
    if (rawLines.length === 0) return dsl;

    // Separate comments from measure lines, tracking their positions.
    // Each comment line is preserved separately (no concatenation).
    const commentMap = new Map(); // line index in output → array of comment lines
    const measureLines = [];

    for (let i = 0; i < rawLines.length; i++) {
      const l = rawLines[i];
      if (!l) continue;
      if (l.startsWith('!')) {
        const body = l.slice(1).trim();
        if (body) {
          if (!commentMap.has(measureLines.length)) {
            commentMap.set(measureLines.length, []);
          }
          commentMap.get(measureLines.length).push(body);
        }
        continue;
      }
      measureLines.push(l);
    }

    // Extract global directives from the first measure line (K, M, T, L at start)
    const firstTokens = measureLines[0].split(/\s+/);
    const directives = [];
    let fi = 0;
    while (fi < firstTokens.length && /^[MKTL]\d/i.test(firstTokens[fi])) {
      directives.push(firstTokens[fi]);
      fi++;
    }

    // Build measure line slices: first line may continue with notation after directives
    let measuresToParse;
    if (fi > 0) {
      measuresToParse = [];
      const rem = firstTokens.slice(fi).join(' ');
      if (rem) measuresToParse.push(rem);
      for (let j = 1; j < measureLines.length; j++) measuresToParse.push(measureLines[j]);
    } else {
      measuresToParse = measureLines.slice();
    }

    if (measuresToParse.length === 0) return dsl;

    // Parse each measure into { notation, chords, lyrics } triples
    // Following the parser's extractDirectivesTail state machine: L→R,
    // state 0=notation, 1=chords(seen :H), 2=lyrics(seen :L)
    const parsed = measuresToParse.map(m => {
      const words = m.split(/\s+/);
      const parts = { notation: [], chords: [], lyrics: [], hasH: false, hasL: false };
      let state = 0;
      for (const w of words) {
        if (w === ':H' || w === ':h') { state = 1; parts.hasH = true; continue; }
        if (w === ':L' || w === ':l') { state = 2; parts.hasL = true; continue; }
        if (state === 1) parts.chords.push(w);
        else if (state === 2) parts.lyrics.push(w);
        else parts.notation.push(w);
      }
      return parts;
    });

    const anyH = parsed.some(p => p.hasH);
    const anyL = parsed.some(p => p.hasL);

    // Compute max widths for each column
    let maxNotationW = 0, maxChordW = 0, maxLyricW = 0;
    for (const p of parsed) {
      const nw = p.notation.join(' ').length;
      const cw = p.chords.join(' ').length;
      const lw = p.lyrics.join(' ').length;
      if (nw > maxNotationW) maxNotationW = nw;
      if (cw > maxChordW) maxChordW = cw;
      if (lw > maxLyricW) maxLyricW = lw;
    }

    // Rebuild each measure line
    const out = [];

    if (directives.length > 0) {
      out.push(directives.join(' '));
    }

    for (let i = 0; i < parsed.length; i++) {
      // Reinsert comment lines before this measure if any exist
      if (commentMap.has(i)) {
        for (const cl of commentMap.get(i)) {
          out.push('! ' + cl);
        }
      }

      const p = parsed[i];
      let line = '';

      const notStr = p.notation.join(' ');
      line += notStr.padEnd(maxNotationW);

      if (anyH) {
        line += ' :H';
        const chordStr = p.chords.join(' ');
        line += ' ' + chordStr.padEnd(maxChordW);
      }

      if (anyL) {
        line += ' :L';
        if (p.lyrics.length > 0) {
          line += ' ' + p.lyrics.join(' ');
        }
      }

      out.push(line);
    }

    // Append trailing comment lines after last measure if any exist
    if (commentMap.has(parsed.length)) {
      for (const cl of commentMap.get(parsed.length)) {
        out.push('! ' + cl);
      }
    }

    return out.join('\n');
  }

  render() {
    this.parseAndRender();
  }

  reformatInput() {
    if (!this.parsedData) return;
    const pos = this.dslInput.selectionStart;
    const reformatted = this.reformatColumns(this.dslInput.value);
    if (reformatted !== this.dslInput.value) {
      this.dslInput.value = reformatted;
      this.dsl = reformatted;
      this.dslInput.selectionStart = Math.min(pos, reformatted.length);
      this.dslInput.selectionEnd = Math.min(pos, reformatted.length);
      this.autoResizeTextarea();
      this.highlightCursorMeasure();
    }
  }

  showError(msg) {
    this.statusError.textContent = msg;
    this.statusError.classList.remove('hidden');
    this.statusText.textContent = 'Error';
  }

  // --- Playback ---

  // pad zero-pads a number to the given width.
  pad(n, width) {
    const s = String(n);
    return s.length >= width ? s : '0'.repeat(width - s.length) + s;
  }

  async initOutput() {
    if (this.midiOutput || this.synthReady) return true;

    // Try Web MIDI first
    try {
      const access = await navigator.requestMIDIAccess();
      this.midiOutput = access.outputs.values().next().value;
      if (this.midiOutput) return true;
    } catch (e) { /* fall through to soft synth */ }

    // Fall back to WebAudioFontPlayer soft synth
    return this.initSoftSynth();
  }

  async initSoftSynth() {
    if (this.synthReady) return true;

    const AC = window.AudioContext || window.webkitAudioContext;
    if (!AC) {
      this.showError('AudioContext not supported in this browser');
      return false;
    }

    try {
      this.audioCtx = new AC();

      // Safari suspends AudioContext by default — resume it now while
      // we're still within the user-gesture call chain.
      if (this.audioCtx.state === 'suspended') {
        await this.audioCtx.resume();
      }

      this.wafPlayer = new WebAudioFontPlayer();

      // Master gain node
      this.masterGain = this.audioCtx.createGain();
      this.masterGain.gain.setValueAtTime(this.velocity / 127, this.audioCtx.currentTime);
      this.masterGain.connect(this.audioCtx.destination);

      // Queue critical instrument loads
      const toLoad = [0];  // 0 = piano
      const loadVars = [];

      if (this.metronomeOn) {
        [76, 77].forEach(n => {
          const info = this.wafPlayer.loader.drumInfo(n);
          if (!window[info.variable]) {
            this.wafPlayer.loader.startLoad(this.audioCtx, info.url, info.variable);
          }
        });
      }

      toLoad.forEach(prog => {
        const varName = '_tone_' + this.pad(prog, 4) + '_GeneralUserGS_sf2_file';
        loadVars.push(varName);
        if (!window[varName]) {
          this.wafPlayer.loader.startLoad(this.audioCtx,
            'https://surikov.github.io/webaudiofontdata/sound/' +
            this.pad(prog, 4) + '_GeneralUserGS_sf2_file.js',
            varName);
        }
      });

      // Wait for critical instruments only
      await new Promise((resolve) => {
        this.wafPlayer.loader.waitLoad(() => resolve());
      });

      this.pianoPreset = window['_tone_0000_GeneralUserGS_sf2_file'];

      // Load bass samples for channel 8 (multi-sampled electric bass, E1-E2 range)
      this._bassSamples = new Map();
      await this._loadBassSample('bass-E1.ogg', 28);    // E1
      await this._loadBassSample('bass-G1.ogg', 31);    // G1
      await this._loadBassSample('bass-As1.ogg', 34);   // A♯1
      await this._loadBassSample('bass-Cs2.ogg', 37);   // C♯2

      this.synthReady = true;
      this.statusText.textContent = 'Soft synth ready';
      return true;
    } catch (e) {
      this.showError('Soft synth init failed: ' + e.message);
      return false;
    }
  }

  loadInstrument(programNumber) {
    return new Promise((resolve, reject) => {
      const varName = '_tone_' + this.pad(programNumber, 4) + '_GeneralUserGS_sf2_file';
      if (window[varName]) { resolve(window[varName]); return; }

      const url = 'https://surikov.github.io/webaudiofontdata/sound/' +
        this.pad(programNumber, 4) + '_GeneralUserGS_sf2_file.js';

      this.wafPlayer.loader.startLoad(this.audioCtx, url, varName);
      this.wafPlayer.loader.waitLoad(() => {
        if (window[varName]) {
          resolve(window[varName]);
        } else {
          reject(new Error('Failed to load instrument ' + programNumber));
        }
      });
    });
  }

  loadDrum(noteNum) {
    return new Promise((resolve) => {
      const info = this.wafPlayer.loader.drumInfo(noteNum);
      if (window[info.variable]) { resolve(window[info.variable]); return; }

      this.wafPlayer.loader.startLoad(this.audioCtx, info.url, info.variable);
      this.wafPlayer.loader.waitLoad(() => {
        resolve(window[info.variable]);
      });
    });
  }

  getInstrumentForChannel(ch) {
    if (ch === 9) {
      // Drum channel — handled per-note via getDrumPreset
      return null;
    }
    return this.pianoPreset;
  }

  getDrumPreset(noteNum) {
    if (!this.wafPlayer) return null;
    const info = this.wafPlayer.loader.drumInfo(noteNum);
    return window[info.variable] || null;
  }

  async loadBassInstrument() {
    const varName = '_tone_0033_GeneralUserGS_sf2_file';
    if (window[varName]) {
      this.bassPreset = window[varName];
      return;
    }
    try {
      const script = document.createElement('script');
      script.src = 'https://surikov.github.io/webaudiofontdata/sound/0033_GeneralUserGS_sf2_file.js';
      script.onload = () => {
        this.bassPreset = window[varName] || null;
        if (this.bassPreset) {
          this.statusText.textContent = 'Bass loaded';
        }
      };
      script.onerror = () => {
        console.warn('Failed to load bass instrument');
      };
      document.head.appendChild(script);
    } catch (e) {
      console.warn('Failed to load bass instrument:', e);
    }
  }

  async togglePlay() {
    if (this.isPlaying) {
      this.stop();
      return;
    }
    await this.play();
  }

  async play() {
    if (!this.parsedData) {
      this.showError('Nothing to play — enter DSL first');
      return;
    }
    if (!await this.initOutput()) return;

    // Safari may re-suspend the AudioContext after initOutput's async
    // work completes. Resume it again now, while still in the user-gesture
    // handler chain from the button click.
    if (this.audioCtx && this.audioCtx.state === 'suspended') {
      await this.audioCtx.resume();
    }

    try {
      const result = JSON.parse(m4bonGenerateSMF(JSON.stringify({
        dsl: this.dsl,
        bpm: this.bpm,
        metronome: this.metronomeOn,
        roots: this.rootsOn,
        backbeats: this.backbeatsOn
      })));
      if (result.err) {
        this.showError(result.err);
        return;
      }

      const { events, measureStarts, tempoBPM } = result.ok;

      let startTick = 0;
      let endTick = Infinity;
      if (this.startMeasure > 0 || this.endMeasure > 0) {
        const tickToSec = 60.0 / (480.0 * tempoBPM);
        startTick = measureStarts[this.startMeasure] / tickToSec;
        if (this.endMeasure > 0 && this.endMeasure < measureStarts.length) {
          endTick = measureStarts[this.endMeasure] / tickToSec;
        }
      }

      const tickToSec = 60.0 / (480.0 * tempoBPM);
      let startWall = this.audioCtx ? this.audioCtx.currentTime + 0.05 : performance.now() / 1000;

      // Count-in: one measure of metronome clicks when starting from measure 1
      let countInSec = 0;
      if (this.startMeasure === 0) {
        const beats = this.parsedData.timeNum;
        const beatSec = 60.0 / tempoBPM;
        for (let b = 0; b < beats; b++) {
          this._scheduleClick(startWall + b * beatSec, b === 0);
        }
        countInSec = beats * beatSec;
        startWall += countInSec;
      }

      // Pre-scan: collect note-on/note-off pairs into a flat schedule array.
      // Each entry has absolute wall-clock start/duration for direct scheduling.
      const scheduledNotes = [];
      const pendingNotes = {}; // key: "ch-pitch" -> [{ tick, velocity }]

      for (const ev of events) {
        if (ev.tick < startTick || ev.tick >= endTick) continue;
        if (ev.type === 'metaTempo' || ev.type === 'metaMeter') continue;

        if (ev.type === 'noteOn') {
          const key = ev.channel + '-' + ev.pitch;
          if (!pendingNotes[key]) pendingNotes[key] = [];
          pendingNotes[key].push({ tick: ev.tick, velocity: ev.velocity || this.velocity });
        } else if (ev.type === 'noteOff') {
          const key = ev.channel + '-' + ev.pitch;
          if (pendingNotes[key] && pendingNotes[key].length > 0) {
            const onset = pendingNotes[key].shift();
            let duration = (ev.tick - onset.tick) * tickToSec;
            if (duration <= 0) duration = 0.05;
            scheduledNotes.push({
              channel: ev.channel,
              pitch: ev.pitch,
              velocity: onset.velocity,
              startTime: startWall + (onset.tick - startTick) * tickToSec,
              duration: duration
            });
          }
        }
      }

      // Flush remaining pending notes
      for (const key in pendingNotes) {
        const list = pendingNotes[key];
        const [ch, pitch] = key.split('-').map(Number);
        for (const onset of list) {
          scheduledNotes.push({
            channel: ch,
            pitch: pitch,
            velocity: onset.velocity,
            startTime: startWall + (onset.tick - startTick) * tickToSec,
            duration: 1.0
          });
        }
      }

      // Sort by startTime (should already be sorted, but ensure it)
      scheduledNotes.sort((a, b) => a.startTime - b.startTime);

      this.isPlaying = true;
      document.getElementById('btn-play').textContent = '⏸';

      // Clear cursor highlight so it doesn't conflict with play-position highlight
      this.highlightCursorMeasure();

      // Start measure highlight tracking
      const rangeOffset = measureStarts[this.startMeasure] || 0;
      this._playStartTime = startWall;
      this._playMeasureStarts = measureStarts.map(s => s - rangeOffset);
      this._playTickToSec = tickToSec;
      this._playStartTick = startTick;
      this._startHighlightTimer();

      // Save timing for recording playback highlight
      if (this.isRecording) {
        this._recordMeasureSecs = measureStarts.map(s => s - rangeOffset);
        this._recordCountInSec = countInSec;
      }

      if (this.isRecording) {
        // Recording path: schedule all notes synchronously. MediaRecorder
        // keeps the AudioContext alive, so we don't need the keep-alive
        // oscillator. Front-loading all node creation avoids audio glitches
        // from incremental queueWaveTable calls during capture.
        for (const n of scheduledNotes) {
          this.scheduleNote(n.channel, n.pitch, n.velocity, n.startTime, n.duration);
        }

        // Set a safety timeout as fallback
        const lastNoteEnd = scheduledNotes.length > 0
          ? scheduledNotes[scheduledNotes.length - 1].startTime + scheduledNotes[scheduledNotes.length - 1].duration
          : startWall;
        const safetySec = (lastNoteEnd - startWall) + 2.0 + countInSec;
        this.playbackTimer = setTimeout(() => this.onPlaybackEnd(), safetySec * 1000);
      } else {
        // Playback-only path: use look-ahead scheduler to adapt to clock
        // drift. Notes are scheduled incrementally in ~200ms windows.
        this._scheduledNotes = scheduledNotes;
        this._schedulerIdx = 0;
        this._schedulerTimer = null;

        this._scheduleLookAhead();

        // Set a safety timeout as fallback (scheduler handles normal completion)
        const lastNoteEnd = scheduledNotes.length > 0
          ? scheduledNotes[scheduledNotes.length - 1].startTime + scheduledNotes[scheduledNotes.length - 1].duration
          : startWall;
        const safetySec = (lastNoteEnd - startWall) + 2.0 + countInSec;
        this.playbackTimer = setTimeout(() => this.onPlaybackEnd(), safetySec * 1000);

        // Keep-alive oscillator prevents Safari from auto-suspending the
        // AudioContext when notes are scheduled via setTimeout callbacks
        // rather than directly from the user-gesture handler.
        if (this.audioCtx) {
          this._keepAliveOsc = this.audioCtx.createOscillator();
          this._keepAliveGain = this.audioCtx.createGain();
          this._keepAliveGain.gain.value = 0;
          this._keepAliveOsc.connect(this._keepAliveGain);
          this._keepAliveGain.connect(this.audioCtx.destination);
          this._keepAliveOsc.start();
        }
      }
    } catch (e) {
      this.showError('Playback error: ' + e.message);
      this.isPlaying = false;
    }
  }

  scheduleNote(channel, pitch, velocity, startTime, duration) {
    if (this.midiOutput) {
      // MIDI hardware path
      const vel = velocity || this.velocity;
      this.midiOutput.send([0x90 | channel, pitch, vel], startTime);
      this.midiOutput.send([0x80 | channel, pitch, 0], startTime + duration);
      return;
    }

    // Soft synth path via WebAudioFontPlayer
    if (!this.audioCtx || !this.wafPlayer) return;

    const key = channel + '-' + pitch;
    const vol = ((velocity || this.velocity) / 127) * (this.velocity / 127);

    let preset;
    if (channel === 9) {
      preset = this.getDrumPreset(pitch);
    } else if (channel === 8) {
      this._playBassNote(pitch, velocity, startTime, duration);
      return;
    } else {
      preset = this.pianoPreset;
    }
    if (!preset) return;

    try {
      const envelope = this.wafPlayer.queueWaveTable(
        this.audioCtx,
        this.masterGain,
        preset,
        startTime,
        pitch,
        duration,
        vol
      );
      // Track envelope for stop() cleanup only — don't cancel prior
      // envelopes since notes are pre-scheduled at different times.
      this.activeEnvelopes[key + '-' + startTime] = envelope;
    } catch (e) {}
  }

  async _loadBassSample(url, midiNote) {
    try {
      const resp = await fetch(url);
      if (!resp.ok) return;
      const buf = await resp.arrayBuffer();
      const decoded = await this.audioCtx.decodeAudioData(buf);
      this._bassSamples.set(midiNote, decoded);
    } catch (e) {
      console.warn('Failed to load bass sample:', url, e);
    }
  }

  _playBassNote(pitch, velocity, startTime, duration) {
    if (!this.audioCtx || !this._bassSamples || this._bassSamples.size === 0) return;

    const delay = (startTime - this.audioCtx.currentTime) * 1000;
    const timer = setTimeout(() => {
      this._playBassNoteNow(pitch, velocity, duration);
    }, Math.max(0, delay));
    this._bassTimers.push(timer);
  }

  _playBassNoteNow(pitch, velocity, duration) {
    if (!this.audioCtx || !this._bassSamples) return;

    let bestMidi = 28;
    let bestDist = Infinity;
    for (const midi of this._bassSamples.keys()) {
      const dist = Math.abs(midi - pitch);
      if (dist < bestDist) { bestDist = dist; bestMidi = midi; }
    }
    const buf = this._bassSamples.get(bestMidi);
    if (!buf) return;

    const vol = ((velocity || this.velocity) / 127) * (this.velocity / 127) * 0.7;
    const rate = Math.pow(2, (pitch - bestMidi) / 12);
    const now = this.audioCtx.currentTime;

    const src = this.audioCtx.createBufferSource();
    src.buffer = buf;
    src.playbackRate.setValueAtTime(rate, now);

    const env = this.audioCtx.createGain();
    env.gain.setValueAtTime(0, now);
    env.gain.linearRampToValueAtTime(vol, now + 0.01);
    env.gain.setValueAtTime(vol, now + duration - 0.03);
    env.gain.linearRampToValueAtTime(0.001, now + duration);

    src.connect(env);
    env.connect(this.masterGain);

    src.start(now);
    src.stop(now + duration + 0.05);

    const nodes = { src, env };
    this._bassNodes.push(nodes);
    setTimeout(() => {
      const idx = this._bassNodes.indexOf(nodes);
      if (idx >= 0) this._bassNodes.splice(idx, 1);
    }, (duration + 0.1) * 1000);
  }

  stop() {
    if (this.isRecording) this.stopRecording();
    if (this.isPlaying) {
      if (this.playbackTimer) {
        clearTimeout(this.playbackTimer);
        this.playbackTimer = null;
      }
      if (this._schedulerTimer) {
        clearTimeout(this._schedulerTimer);
        this._schedulerTimer = null;
      }
      this._scheduledNotes = null;
      this._schedulerIdx = 0;
      this._clearHighlightTimer();
      if (this.midiOutput) {
        for (let ch = 0; ch < 16; ch++) {
          this.midiOutput.send([0xB0 | ch, 123, 0]);
        }
      }
      // Cancel soft synth envelopes
      if (this.activeEnvelopes) {
        for (const key in this.activeEnvelopes) {
          try { this.activeEnvelopes[key].cancel(); } catch (e) {}
        }
        this.activeEnvelopes = {};
      }
      // Kill bass: clear pending timers + stop active nodes
      if (this._bassTimers) {
        this._bassTimers.forEach(t => clearTimeout(t));
        this._bassTimers = [];
      }
      if (this._bassNodes) {
        for (const n of this._bassNodes) {
          try { n.src.stop(); } catch (e) {}
          try { n.src.disconnect(); } catch (e) {}
        }
        this._bassNodes = [];
      }
      // Stop keep-alive oscillator
      if (this._keepAliveOsc) {
        try { this._keepAliveOsc.stop(); } catch (e) {}
        try { this._keepAliveOsc.disconnect(); } catch (e) {}
        this._keepAliveOsc = null;
        this._keepAliveGain = null;
      }
      this.isPlaying = false;
      document.getElementById('btn-play').textContent = '▶';
      this.highlightCursorMeasure();
    }
  }

  onPlaybackEnd() {
    // Stop keep-alive oscillator
    if (this._keepAliveOsc) {
      try { this._keepAliveOsc.stop(); } catch (e) {}
      try { this._keepAliveOsc.disconnect(); } catch (e) {}
      this._keepAliveOsc = null;
      this._keepAliveGain = null;
    }
    this.isPlaying = false;
    this._clearHighlightTimer();
    document.getElementById('btn-play').textContent = '▶';
    this.highlightCursorMeasure();
    if (this.isRecording) {
      this.stopRecording();
    }
  }

  _startHighlightTimer() {
    this._clearHighlightTimer();
    const self = this;
    const tick = () => {
      if (!self.isPlaying) return;
      self._updatePlayHighlight();
      self.measureHighlightTimer = requestAnimationFrame(tick);
    };
    this.measureHighlightTimer = requestAnimationFrame(tick);
  }

  _clearHighlightTimer() {
    if (this.measureHighlightTimer) {
      cancelAnimationFrame(this.measureHighlightTimer);
      this.measureHighlightTimer = null;
    }
    // Remove playing class from all measures
    if (this.measuresEl) {
      this.measuresEl.querySelectorAll('.m4bon-measure.m4bon-playing').forEach(d => {
        d.classList.remove('m4bon-playing');
      });
    }
    this._lastPlayingIdx = -1;
  }

  _scheduleLookAhead() {
    if (!this.isPlaying) return;
    if (!this.audioCtx) return;

    const SCHEDULER_INTERVAL_MS = 50;
    const LOOK_AHEAD_SEC = 0.200;

    const now = this.audioCtx.currentTime;
    const windowEnd = now + LOOK_AHEAD_SEC;
    const notes = this._scheduledNotes;
    const len = notes.length;

    let scheduled = false;
    while (this._schedulerIdx < len) {
      const n = notes[this._schedulerIdx];
      if (n.startTime < windowEnd) {
        this.scheduleNote(n.channel, n.pitch, n.velocity, n.startTime, n.duration);
        this._schedulerIdx++;
        scheduled = true;
      } else {
        break;
      }
    }

    if (!scheduled && this._schedulerIdx < len) {
      const n = notes[this._schedulerIdx];
      const start = Math.max(n.startTime, now + 0.01);
      this.scheduleNote(n.channel, n.pitch, n.velocity, start, n.duration);
      this._schedulerIdx++;
    }

    if (this._schedulerIdx >= len) {
      const lastNote = notes[len - 1];
      const tail = (lastNote.startTime + lastNote.duration) - now + 0.5;
      this.playbackTimer = setTimeout(() => this.onPlaybackEnd(), Math.max(tail * 1000, 500));
      return;
    }

    this._schedulerTimer = setTimeout(() => this._scheduleLookAhead(), SCHEDULER_INTERVAL_MS);
  }

  _scheduleClick(time, isDownbeat) {
    if (!this.audioCtx) return;
    const osc = this.audioCtx.createOscillator();
    const gain = this.audioCtx.createGain();
    osc.connect(gain);
    gain.connect(this.masterGain || this.audioCtx.destination);
    osc.frequency.value = isDownbeat ? 1200 : 900;
    gain.gain.setValueAtTime(0.08, time);
    gain.gain.exponentialRampToValueAtTime(0.001, time + 0.04);
    osc.start(time);
    osc.stop(time + 0.04);
  }

  _scrollToMeasure(idx) {
    const divs = this.measuresEl.querySelectorAll('.m4bon-measure');
    if (idx < 0 || idx >= divs.length) return;

    const container = this.measuresEl;
    const measure = divs[idx];
    const containerRect = container.getBoundingClientRect();
    const measureRect = measure.getBoundingClientRect();

    const measureBottom = measureRect.bottom - containerRect.top;
    const containerHeight = containerRect.height;
    const measureHeight = measureRect.height;

    // When the playing measure is within 2 measure-heights of the
    // container bottom, scroll it to the top so the musician can see ahead.
    if (containerHeight - measureBottom < 2 * measureHeight) {
      measure.scrollIntoView({ behavior: 'smooth', block: 'start' });
    } else {
      measure.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
  }

  _updatePlayHighlight() {
    if (!this._playMeasureStarts || !this._playStartTime) return;

    let currentTime;
    if (this.audioCtx) {
      currentTime = this.audioCtx.currentTime - this._playStartTime;
    } else {
      currentTime = performance.now() / 1000 - this._playStartTime;
    }
    if (currentTime < 0) return;

    // Find which measure we're in
    let idx = -1;
    for (let i = 0; i < this._playMeasureStarts.length; i++) {
      if (currentTime >= this._playMeasureStarts[i]) {
        if (i + 1 < this._playMeasureStarts.length) {
          if (currentTime < this._playMeasureStarts[i + 1]) {
            idx = i;
            break;
          }
        } else {
          idx = i;
        }
      }
    }

    if (idx !== this._lastPlayingIdx) {
      // Remove old
      const divs = this.measuresEl.querySelectorAll('.m4bon-measure');
      divs.forEach(d => d.classList.remove('m4bon-playing'));
      if (idx >= 0 && idx < divs.length) {
        divs[idx].classList.add('m4bon-playing');
        this._scrollToMeasure(idx);
      }
      this._lastPlayingIdx = idx;
    }
  }

  // --- Recording ---

  async toggleRecord() {
    if (this.isRecording) {
      this.stopRecording();
      return;
    }
    // Discard prior recording
    if (this.recordingURL) {
      URL.revokeObjectURL(this.recordingURL);
      this.recordingURL = null;
      this.recordingBlob = null;
    }
    document.getElementById('btn-play-recording').classList.add('hidden');
    await this.startRecording();
  }

  async startRecording() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: false,
          noiseSuppression: false,
          autoGainControl: false
        }
      });

      // Pick a MIME type that both Chrome and Safari can record and play.
      // Safari records audio/mp4; Chrome prefers audio/webm but can play mp4 too.
      const mimeTypes = [
        'audio/webm;codecs=opus',
        'audio/webm',
        'audio/mp4'
      ];
      let mimeType = '';
      for (const mt of mimeTypes) {
        if (MediaRecorder.isTypeSupported(mt)) {
          mimeType = mt;
          break;
        }
      }
      this._recMimeType = mimeType;
      this.mediaRecorder = mimeType
        ? new MediaRecorder(stream, { mimeType })
        : new MediaRecorder(stream);
      this.audioChunks = [];

      this.mediaRecorder.ondataavailable = (e) => this.audioChunks.push(e.data);
      this.mediaRecorder.onstop = () => this.processRecording();

      this.mediaRecorder.start();
      this.isRecording = true;
      document.getElementById('btn-record').classList.add('recording');
      this.statusText.textContent = 'Recording...';

      // Start MIDI playback in sync
      await this.play();
    } catch (e) {
      this.showError('Microphone access denied');
    }
  }

  stopRecording() {
    if (this.mediaRecorder && this.mediaRecorder.state !== 'inactive') {
      this.mediaRecorder.stop();
      this.mediaRecorder.stream.getTracks().forEach(t => t.stop());
    }
    this.isRecording = false;
    document.getElementById('btn-record').classList.remove('recording');
  }

  processRecording() {
    const mimeType = this._recMimeType || this.mediaRecorder.mimeType || 'audio/webm';
    const blob = new Blob(this.audioChunks, { type: mimeType });
    this.recordingBlob = blob;
    if (this.recordingURL) URL.revokeObjectURL(this.recordingURL);
    this.recordingURL = URL.createObjectURL(blob);
    document.getElementById('btn-play-recording').classList.remove('hidden');
    this.statusText.textContent = 'Recording ready';
  }

  playRecording() {
    if (!this.recordingURL) return;
    if (this.recordingAudio && !this.recordingAudio.ended) {
      this.recordingAudio.pause();
      this.recordingAudio.currentTime = 0;
      this.recordingAudio = null;
      this._clearRecordingHighlight();
      return;
    }
    const a = new Audio(this.recordingURL);
    a.onended = () => {
      this.recordingAudio = null;
      this._clearRecordingHighlight();
    };
    this.recordingAudio = a;

    if (this._recordMeasureSecs && this._recordMeasureSecs.length > 0) {
      this._recHighlightStarts = this._recordMeasureSecs.map(
        s => s + (this._recordCountInSec || 0)
      );
      this._startRecordingHighlight();
    }

    a.play().catch(e => this.showError('Playback failed'));
  }

  _startRecordingHighlight() {
    this._clearRecordingHighlight();
    const self = this;
    const tick = () => {
      if (!self.recordingAudio || self.recordingAudio.ended) {
        self._clearRecordingHighlight();
        return;
      }
      self._updateRecordingHighlight(self.recordingAudio.currentTime);
      self._recHighlightTimer = requestAnimationFrame(tick);
    };
    this._recHighlightTimer = requestAnimationFrame(tick);
  }

  _clearRecordingHighlight() {
    if (this._recHighlightTimer) {
      cancelAnimationFrame(this._recHighlightTimer);
      this._recHighlightTimer = null;
    }
    this._lastRecPlayingIdx = -1;
    if (this.measuresEl) {
      this.measuresEl.querySelectorAll('.m4bon-measure.m4bon-playing').forEach(d => {
        d.classList.remove('m4bon-playing');
      });
    }
  }

  _updateRecordingHighlight(elapsed) {
    const starts = this._recHighlightStarts;
    if (!starts) return;
    let idx = 0;
    while (idx < starts.length && starts[idx] <= elapsed) idx++;
    const divs = this.measuresEl.querySelectorAll('.m4bon-measure');
    if (idx !== this._lastRecPlayingIdx) {
      if (this._lastRecPlayingIdx >= 0 && this._lastRecPlayingIdx < divs.length) {
        divs[this._lastRecPlayingIdx].classList.remove('m4bon-playing');
      }
      if (idx > 0 && idx <= divs.length) {
        divs[idx - 1].classList.add('m4bon-playing');
        this._lastRecPlayingIdx = idx - 1;
        this._scrollToMeasure(idx - 1);
      }
    }
  }

  // --- Tempo ---

  adjustTempo(delta) {
    this.setTempo(this.bpm + delta);
  }

  setTempo(bpm) {
    this.bpm = Math.max(20, Math.min(300, bpm));
    this.tempoEl.textContent = `♩=${this.bpm}`;
    this.tempoDisplay.textContent = this.bpm;
  }

  // --- Measure range ---

  moveStartMeasure(delta) {
    if (!this.parsedData) return;
    this.startMeasure = Math.max(0, Math.min(
      this.parsedData.measures.length - 1,
      this.startMeasure + delta
    ));
    if (this.endMeasure > 0 && this.startMeasure >= this.endMeasure) {
      this.startMeasure = this.endMeasure - 1;
    }
    this.updateRangeDisplay();
    this.highlightMeasures();
  }

  moveEndMeasure(delta) {
    if (!this.parsedData) return;
    this.endMeasure = Math.max(0, Math.min(
      this.parsedData.measures.length,
      this.endMeasure + delta
    ));
    if (this.endMeasure > 0 && this.endMeasure <= this.startMeasure) {
      this.endMeasure = this.startMeasure + 1;
    }
    this.updateRangeDisplay();
    this.highlightMeasures();
  }

  // --- File operations ---

  loadFile() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.dsl,.txt';
    input.onchange = (e) => {
      const file = e.target.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = () => {
        this.dslInput.value = reader.result;
        this.dsl = reader.result;
        this.parseAndRender();
        if (this.parsedData) {
          const reformatted = this.reformatColumns(this.dsl);
          if (reformatted !== this.dslInput.value) {
            this.dslInput.value = reformatted;
            this.dsl = reformatted;
          }
        }
        this.autoResizeTextarea();
        this.saveState();
      };
      reader.readAsText(file);
    };
    input.click();
  }

  saveMXL() {
    if (!this.parsedData) {
      this.showError('Nothing to save');
      return;
    }
    try {
      const result = JSON.parse(m4bonGenerateXML(JSON.stringify({ dsl: this.dsl })));
      if (result.err) {
        this.showError(result.err);
        return;
      }
      const blob = new Blob([result.ok], { type: 'application/vnd.recordare.musicxml+xml' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'm4bon-output.mxl';
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      this.showError('Export error');
    }
  }

  saveDSL() {
    const blob = new Blob([this.dsl], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'm4bon-input.dsl';
    a.click();
    URL.revokeObjectURL(url);
  }

  copyDSL() {
    navigator.clipboard.writeText(this.dsl).then(() => {
      this.statusText.textContent = 'DSL copied to clipboard';
    });
  }

  // --- State persistence ---

  saveState() {
    try {
      localStorage.setItem('m4bon-dsl', this.dsl);
      localStorage.setItem('m4bon-bpm', this.bpm);
      localStorage.setItem('m4bon-metronome', this.metronomeOn);
      localStorage.setItem('m4bon-subscripts', this.showSubscripts);
      localStorage.setItem('m4bon-comments', this.showComments);
    } catch (e) { /* localStorage unavailable */ }
  }

  loadState() {
    try {
      const saved = localStorage.getItem('m4bon-dsl');
      if (saved) {
        this.dsl = saved;
        this.dslInput.value = saved;
        this.autoResizeTextarea();
      }
      this.bpm = parseInt(localStorage.getItem('m4bon-bpm')) || 120;
      this.metronomeOn = localStorage.getItem('m4bon-metronome') !== 'false';
      this.showSubscripts = localStorage.getItem('m4bon-subscripts') !== 'false';
      this.showComments = localStorage.getItem('m4bon-comments') !== 'false';
    } catch (e) { /* ignore */ }
    this.tempoDisplay.textContent = this.bpm;
    this.chkMetronome.checked = this.metronomeOn;
    this.chkSubscripts.checked = this.showSubscripts;
    this.chkComments.checked = this.showComments;
  }

  // --- Keyboard shortcuts ---

  onKeyDown(e) {
    // Allow textarea typing to work normally, except Esc and Ctrl/Cmd+S
    if (e.target.tagName === 'TEXTAREA' || e.target.tagName === 'INPUT') {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        this.reformatInput();
        return;
      }
      if (e.key !== 'Escape') return;
    }

    switch (e.key) {
      case ' ': e.preventDefault(); this.togglePlay(); break;
      case 's': this.stop(); break;
      case 'r': this.toggleRecord(); break;
      case '[': e.preventDefault(); this.adjustTempo(-5); break;
      case ']': e.preventDefault(); this.adjustTempo(5); break;
      case '{': e.preventDefault(); this.adjustTempo(-1); break;
      case '}': e.preventDefault(); this.adjustTempo(1); break;
      case '0': this.setTempo(120); break;
      case 'ArrowUp': e.shiftKey ? this.moveEndMeasure(-1) : this.moveStartMeasure(-1); break;
      case 'ArrowDown': e.shiftKey ? this.moveEndMeasure(1) : this.moveStartMeasure(1); break;
      case 'ArrowLeft': e.preventDefault(); this.velocity = Math.max(0, this.velocity - 5);
        this.volumeSlider.value = this.velocity; break;
      case 'ArrowRight': e.preventDefault(); this.velocity = Math.min(127, this.velocity + 5);
        this.volumeSlider.value = this.velocity; break;
      case 'm': this.metronomeOn = !this.metronomeOn;
        this.chkMetronome.checked = this.metronomeOn; break;
      case 'o': this.showSubscripts = !this.showSubscripts;
        this.chkSubscripts.checked = this.showSubscripts; this.updateMeasures(); break;
      case 'c': this.showComments = !this.showComments;
        this.chkComments.checked = this.showComments; this.updateMeasures(); break;
    }
  }
}

// --- Startup ---
// The Go program's main() sets up globals synchronously then blocks.
// go.run() hits its first yield point after main() finishes setup,
// so by the time requestAnimationFrame fires, all globals are ready.
async function start() {
  await bootstrapWASM();
  // Poll for readiness — the Go scheduler yields after main() sets up globals
  function wait() {
    if (typeof m4bonParse !== 'undefined') {
      new M4bonApp();
    } else {
      requestAnimationFrame(wait);
    }
  }
  requestAnimationFrame(wait);
}

start();
