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
    this.playbackTimer = null;
    this.measureHighlightTimer = null;
    this.debounceTimer = null;

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
    this.chkRoots = document.getElementById('chk-roots');
    this.chkBackbeats = document.getElementById('chk-backbeats');

    this.dslInput.addEventListener('input', () => this.onDSLChange());
    this.volumeSlider.addEventListener('input', () => {
      this.velocity = parseInt(this.volumeSlider.value);
    });

    document.getElementById('btn-play').addEventListener('click', () => this.togglePlay());
    document.getElementById('btn-stop').addEventListener('click', () => this.stop());
    document.getElementById('btn-record').addEventListener('click', () => this.toggleRecord());

    document.getElementById('btn-tempo-down').addEventListener('click', () => this.adjustTempo(-5));
    document.getElementById('btn-tempo-up').addEventListener('click', () => this.adjustTempo(5));
    document.getElementById('btn-tempo-down1').addEventListener('click', () => this.adjustTempo(-1));
    document.getElementById('btn-tempo-up1').addEventListener('click', () => this.adjustTempo(1));
    document.getElementById('btn-tempo-reset').addEventListener('click', () => this.setTempo(120));

    document.getElementById('btn-start-up').addEventListener('click', () => this.moveStartMeasure(1));
    document.getElementById('btn-start-down').addEventListener('click', () => this.moveStartMeasure(-1));
    document.getElementById('btn-end-up').addEventListener('click', () => this.moveEndMeasure(1));
    document.getElementById('btn-end-down').addEventListener('click', () => this.moveEndMeasure(-1));

    this.chkMetronome.addEventListener('change', () => {
      this.metronomeOn = this.chkMetronome.checked;
    });
    this.chkSubscripts.addEventListener('change', () => {
      this.showSubscripts = this.chkSubscripts.checked;
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
  }

  onDSLChange() {
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
        asciiLeaps: false
      })));
      if (result.ok) {
        this.measuresEl.innerHTML = result.ok;
        this.highlightMeasures();
      }
    } catch (e) {
      // WASM not ready
    }
  }

  highlightMeasures() {
    const divs = this.measuresEl.querySelectorAll('.m4bon-measure');
    const total = divs.length;
    if (total === 0) return;

    // Remove all existing indicators
    divs.forEach(d => {
      d.classList.remove('m4bon-start', 'm4bon-end', 'm4bon-playing');
    });

    // Add start indicator
    if (this.startMeasure > 0 && this.startMeasure < total) {
      divs[this.startMeasure].classList.add('m4bon-start');
    }

    // Add end indicator
    if (this.endMeasure > 0 && this.endMeasure <= total) {
      divs[this.endMeasure - 1].classList.add('m4bon-end');
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

  render() {
    this.parseAndRender();
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
      const startWall = this.audioCtx ? this.audioCtx.currentTime + 0.05 : performance.now() / 1000;

      // Pre-scan: collect note-on events. Use an array per key because
      // the same pitch on the same channel can appear in overlapping notes.
      const pendingNotes = {}; // key: "ch-pitch" -> [{ tick, velocity }]
      let lastTick = 0;

      for (const ev of events) {
        if (ev.tick < startTick || ev.tick >= endTick) continue;
        if (ev.type === 'metaTempo' || ev.type === 'metaMeter') continue;

        if (ev.tick > lastTick) lastTick = ev.tick;

        if (ev.type === 'noteOn') {
          const key = ev.channel + '-' + ev.pitch;
          if (!pendingNotes[key]) pendingNotes[key] = [];
          pendingNotes[key].push({ tick: ev.tick, velocity: ev.velocity || this.velocity });
        } else if (ev.type === 'noteOff') {
          const key = ev.channel + '-' + ev.pitch;
          if (pendingNotes[key] && pendingNotes[key].length > 0) {
            const onset = pendingNotes[key].shift();
            const startTime = startWall + (onset.tick - startTick) * tickToSec;
            let duration = (ev.tick - onset.tick) * tickToSec;
            // Minimum duration so metronome clicks are audible
            if (duration <= 0) duration = 0.05;
            this.scheduleNote(ev.channel, ev.pitch, onset.velocity, startTime, duration);
          }
        }
      }

      // Flush any remaining pending notes with 1s default duration
      for (const key in pendingNotes) {
        const list = pendingNotes[key];
        const [ch, pitch] = key.split('-').map(Number);
        for (const onset of list) {
          const startTime = startWall + (onset.tick - startTick) * tickToSec;
          this.scheduleNote(ch, pitch, onset.velocity, startTime, 1.0);
        }
      }

      this.isPlaying = true;
      document.getElementById('btn-play').textContent = '⏸';

      // Start measure highlight tracking
      const rangeOffset = measureStarts[this.startMeasure] || 0;
      this._playStartTime = startWall;
      this._playMeasureStarts = measureStarts.map(s => s - rangeOffset);
      this._playTickToSec = tickToSec;
      this._playStartTick = startTick;
      this._startHighlightTimer();

      const totalSec = (endTick === Infinity ? lastTick : endTick - startTick) * tickToSec + 0.5;
      this.playbackTimer = setTimeout(() => this.onPlaybackEnd(), totalSec * 1000);
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
    if (this.isPlaying) {
      if (this.playbackTimer) {
        clearTimeout(this.playbackTimer);
        this.playbackTimer = null;
      }
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
      this.isPlaying = false;
      document.getElementById('btn-play').textContent = '▶';
    }
  }

  onPlaybackEnd() {
    this.isPlaying = false;
    this._clearHighlightTimer();
    document.getElementById('btn-play').textContent = '▶';
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
        // Scroll into view
        divs[idx].scrollIntoView({ behavior: 'smooth', block: 'nearest' });
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
    await this.startRecording();
  }

  async startRecording() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      this.mediaRecorder = new MediaRecorder(stream);
      this.audioChunks = [];

      this.mediaRecorder.ondataavailable = (e) => this.audioChunks.push(e.data);
      this.mediaRecorder.onstop = () => this.processRecording();

      this.mediaRecorder.start();
      this.isRecording = true;
      document.getElementById('btn-record').classList.add('recording');
      this.statusText.textContent = 'Recording...';
    } catch (e) {
      this.showError('Microphone access denied');
    }
  }

  stopRecording() {
    if (this.mediaRecorder && this.isRecording) {
      this.mediaRecorder.stop();
      this.mediaRecorder.stream.getTracks().forEach(t => t.stop());
      this.isRecording = false;
      document.getElementById('btn-record').classList.remove('recording');
    }
  }

  processRecording() {
    const blob = new Blob(this.audioChunks, { type: 'audio/webm' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'm4bon-recording.webm';
    a.click();
    URL.revokeObjectURL(url);
    this.statusText.textContent = 'Recording saved';
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
    } catch (e) { /* localStorage unavailable */ }
  }

  loadState() {
    try {
      const saved = localStorage.getItem('m4bon-dsl');
      if (saved) {
        this.dsl = saved;
        this.dslInput.value = saved;
      }
      this.bpm = parseInt(localStorage.getItem('m4bon-bpm')) || 120;
      this.metronomeOn = localStorage.getItem('m4bon-metronome') !== 'false';
      this.showSubscripts = localStorage.getItem('m4bon-subscripts') !== 'false';
    } catch (e) { /* ignore */ }
    this.tempoDisplay.textContent = this.bpm;
    this.chkMetronome.checked = this.metronomeOn;
    this.chkSubscripts.checked = this.showSubscripts;
  }

  // --- Keyboard shortcuts ---

  onKeyDown(e) {
    if (e.target.tagName === 'TEXTAREA' || e.target.tagName === 'INPUT') {
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
      case 'ArrowUp': e.shiftKey ? this.moveEndMeasure(1) : this.moveStartMeasure(1); break;
      case 'ArrowDown': e.shiftKey ? this.moveEndMeasure(-1) : this.moveStartMeasure(-1); break;
      case 'ArrowLeft': e.preventDefault(); this.velocity = Math.max(0, this.velocity - 5);
        this.volumeSlider.value = this.velocity; break;
      case 'ArrowRight': e.preventDefault(); this.velocity = Math.min(127, this.velocity + 5);
        this.volumeSlider.value = this.velocity; break;
      case 'm': this.metronomeOn = !this.metronomeOn;
        this.chkMetronome.checked = this.metronomeOn; break;
      case 'o': this.showSubscripts = !this.showSubscripts;
        this.chkSubscripts.checked = this.showSubscripts; this.updateMeasures(); break;
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
