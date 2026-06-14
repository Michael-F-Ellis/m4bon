import MuseScore 3.0
import QtQuick 2.9
import QtQuick.Controls 2.2
import QtQuick.Layouts 1.3

/**
 * m4bon — Beat-Oriented Note Entry for MuseScore 4
 *
 * A DSL-driven plugin where users type beat-oriented rhythmic patterns
 * and press Send to insert notes at the current cursor position.
 *
 * DSL philosophy: whitespace separates beats. Characters grouped without
 * spaces subdivide that beat equally.  See AGENTS.md for full spec.
 */
MuseScore {
    id: plugin
    title: "m4bon"
    description: "Beat-oriented note entry for MuseScore 4"
    version: "0.5.0"
    categoryCode: "composing-arranging-tools"
    thumbnailName: ""

    pluginType: "dialog"
    width: 500
    height: 340

    // ----------------------------------------------------------------
    // DSL symbol constants
    // Override these to fork the plugin with a different character set.
    // ----------------------------------------------------------------
    property string symSharp         : "#"
    property string symFlat          : "&"
    property string symNatural       : "%"
    property string symUpOctave      : "^"
    property string symDownOctave    : "/"
    property string symSustain       : "-"
    property string symRest          : ";"
    property string symChordOpen     : "("
    property string symChordClose    : ")"
    property string symBarline       : "|"

    // Logging helpers
    function log(msg) { console.log("[m4bon] " + msg); }

    // ---- Pitch helpers ----

    property var noteOffsets: ({
        'c': 0,  'd': 2,  'e': 4,  'f': 5,  'g': 7,  'a': 9,  'b': 11
    })

    function normalizePitchInput(text) {
        // Convert to lowercase, map FQS/MS4 accidental chars to our symbols
        var t = text.toLowerCase();
        t = t.replace(/[♯]/g, symSharp);
        t = t.replace(/[♭]/g, symFlat);
        t = t.replace(/[♮]/g, symNatural);
        return t;
    }

    function letterToMidi(letter, octave, accidental) {
        var base = noteOffsets[letter];
        if (base === undefined) return -1;
        return 60 + (octave - 4) * 12 + base + accidental;
    }

    // ---- DSL parser ----

    /**
     * Read time signature from the score cursor and return beat duration
     * as a fraction of a whole note. Falls back to 4/4.
     *
     * Returns: {num: int, den: int} representing beat duration.
     *   e.g. quarter beat in 4/4 → {num: 1, den: 4}
     *        dotted quarter in 6/8 → {num: 3, den: 8}
     *        eighth in 5/8 → {num: 1, den: 8}
     */
    function resolveBeatDuration(cursor) {
        // Try to read time signature from current measure
        var num = 4, den = 4;  // default
        try {
            if (cursor && cursor.measure) {
                // MuseScore 4 API: measure has timesigNumerator/Denominator
                num = cursor.measure.timesigNumerator || 4;
                den = cursor.measure.timesigDenominator || 4;
            } else if (typeof curScore !== "undefined" && curScore) {
                // Fallback: some versions expose it on the score
                num = curScore.timesigNumerator || 4;
                den = curScore.timesigDenominator || 4;
            }
        } catch (e) {
            log("Could not read time signature, defaulting to 4/4");
        }

        // Determine beat unit in whole-note fractions
        var z, n;
        if (den === 2) {
            // X/2: beat = half note
            z = 1; n = 2;
        } else if (den === 4) {
            // X/4: beat = quarter note
            z = 1; n = 4;
        } else if (den === 8) {
            if (num % 3 === 0) {
                // Compound: 6/8, 9/8, 12/8 → beat = dotted quarter = 3/8
                z = 3; n = 8;
            } else {
                // Non-compound: 5/8, 7/8 → beat = eighth
                z = 1; n = 8;
            }
        } else if (den === 16) {
            if (num % 3 === 0) {
                // Compound: 6/16, 9/16 → beat = dotted eighth = 3/16
                z = 3; n = 16;
            } else {
                z = 1; n = 16;
            }
        } else {
            z = 1; n = den;
        }

        return {num: z, den: n};
    }

    /**
     * Tokenize DSL input: split by whitespace, track offsets for errors.
     * Also handles normalizePitchInput (lowercase + UTF-8 accidentals).
     *
     * Returns: [{raw: string, offset: int}, ...]
     */
    function tokenize(text) {
        var normalized = normalizePitchInput(text);
        var tokens = [];
        var re = /\S+/g;
        var match;
        while ((match = re.exec(normalized)) !== null) {
            tokens.push({raw: match[0], offset: match.index});
        }
        return tokens;
    }

    /**
     * Parse a single beat group token into multiplier + slot list.
     *
     * States: IDLE | IN_MULTIPLIER | IN_CHORD | EXPECT_LETTER
     *
     * Returns: {multiplier: int, slots: Slot[], error: string|null, errorOffset: int}
     *
     * Slot types:
     *   {type: "note",   letter, accidental, octaveShift}
     *   {type: "sustain"}
     *   {type: "rest"}
     *   {type: "chord",  pitches: [{letter, accidental, octaveShift}, ...]}
     */
    function parseGroup(raw, priorPitchExists) {
        var multiplier = 1;
        var slots = [];
        var i = 0;

        // Accumulators reset per note/chord
        var acc = 0;
        var oct = 0;
        var hasLetter = false;
        var letter = "";

        // Chord accumulator
        var inChord = false;
        var chordPitches = [];

        function err(msg, offset) {
            return {multiplier: 1, slots: [], error: msg, errorOffset: offset};
        }

        function emitNote(offset) {
            if (inChord) {
                if (!hasLetter) return err("accidental/octave without pitch in chord", offset);
                chordPitches.push({letter: letter, accidental: acc, octaveShift: oct});
            } else {
                if (!hasLetter) return err("accidental/octave without pitch at end of group", offset);
                slots.push({type: "note", letter: letter, accidental: acc, octaveShift: oct});
            }
            acc = 0; oct = 0; hasLetter = false; letter = "";
            return null;
        }

        while (i < raw.length) {
            var ch = raw[i];
            var errResult = null;

            // --- MULTIPLIER (digits only valid at start) ---
            if (!inChord && i === 0 && ch >= '1' && ch <= '9') {
                var multStart = i;
                multiplier = 0;
                while (i < raw.length && raw[i] >= '0' && raw[i] <= '9') {
                    multiplier = multiplier * 10 + (raw[i].charCodeAt(0) - 48);
                    i++;
                }
                if (multiplier === 0)
                    return err("beat multiplier cannot be zero", multStart);
                continue;
            }

            // --- DIGIT detected beyond start ---
            if (ch >= '0' && ch <= '9')
                return err("unexpected digit — multiplier must be at start", i);

            // --- ACCIDENTALS ---
            if (ch === '#')      { acc += 1; i++; continue; }
            if (ch === '&')      { acc -= 1; i++; continue; }
            if (ch === '%')      { acc = 0;  i++; continue; }

            // --- OCTAVE SHIFTS ---
            if (ch === '^')      { oct += 1; i++; continue; }
            if (ch === '/')      { oct -= 1; i++; continue; }

            // --- SUSTAIN ---
            if (ch === '-') {
                if (hasLetter) {
                    errResult = emitNote(i);
                    if (errResult) return errResult;
                }
                if (slots.length === 0 && !priorPitchExists)
                    return err("sustain with no prior note", i);
                slots.push({type: "sustain"});
                i++; continue;
            }

            // --- REST ---
            if (ch === ';') {
                if (hasLetter) {
                    errResult = emitNote(i);
                    if (errResult) return errResult;
                }
                slots.push({type: "rest"});
                i++; continue;
            }

            // --- CHORD OPEN ---
            if (ch === '(') {
                if (inChord) return err("nested chords not allowed", i);
                if (hasLetter) {
                    errResult = emitNote(i);
                    if (errResult) return errResult;
                }
                inChord = true;
                chordPitches = [];
                i++; continue;
            }

            // --- CHORD CLOSE ---
            if (ch === ')') {
                if (!inChord) return err("unmatched closing parenthesis", i);
                if (hasLetter) {
                    errResult = emitNote(i);
                    if (errResult) return errResult;
                }
                if (chordPitches.length === 0)
                    return err("empty chord", i);
                // Validate ascending order
                for (var p = 1; p < chordPitches.length; p++) {
                    if (chordPitches[p].letter <= chordPitches[p-1].letter)
                        return err("chord pitches must be strictly ascending", i);
                }
                slots.push({type: "chord", pitches: chordPitches});
                inChord = false;
                chordPitches = [];
                i++; continue;
            }

            // --- PITCH LETTER ---
            var lower = ch.toLowerCase();
            if (lower >= 'a' && lower <= 'g') {
                if (hasLetter) {
                    // Two letters in a row outside a chord = two separate slots
                    if (inChord) {
                        errResult = emitNote(i);
                    } else {
                        errResult = emitNote(i);
                    }
                    if (errResult) return errResult;
                }
                letter = lower;
                hasLetter = true;
                i++; continue;
            }

            // --- UNKNOWN CHARACTER ---
            return err("unexpected character '" + ch + "'", i);
        }

        // End of string — flush any pending state
        if (inChord) return err("unclosed chord", raw.length);
        if (hasLetter) {
            var flushErr = emitNote(i);
            if (flushErr) return flushErr;
        }
        if (acc !== 0 || oct !== 0)
            return err("bare accidental/octave at end of group", raw.length);

        return {multiplier: multiplier, slots: slots, error: null, errorOffset: -1};
    }

    function isPowerOf2(n) {
        return n > 0 && (n & (n - 1)) === 0;
    }

    function lowerPowerOf2(n) {
        if (n <= 1) return 1;
        var p = 1;
        while (p * 2 < n) p *= 2;
        return p;
    }

    /**
     * A duration fraction is "standard" if it corresponds to a normal
     * note value (1/den) or a dotted note (3/den), where den is a power of 2.
     * Non-standard durations (e.g. 1/12, 1/24, 1/6) indicate tuplets.
     */
    function isStandardDuration(z, n) {
        var g = gcd(z, n);
        z /= g;
        n /= g;
        if (!isPowerOf2(n)) return false;
        return z === 1 || z === 3;
    }

    function countActivePositions(slots) {
        var n = 0;
        for (var i = 0; i < slots.length; i++) {
            if (slots[i].type !== "sustain") n++;
        }
        return n;
    }

    /**
     * Given parsed groups and time signature, compute event durations
     * and flatten sustain slots into extended note lengths.
     *
     * Tuplet groups emit a tupletStart event followed by notes with
     * NOMINAL durations. Non-tuplet groups emit ACTUAL durations.
     *
     * Returns: [{type: ...}, ...] or {error: string}
     */
    function resolveDurations(groups, timeSig) {
        var beat = resolveBeatDuration(timeSig);
        var events = [];

        for (var g = 0; g < groups.length; g++) {
            var group = groups[g];
            if (group.error) return group;

            var posCount = group.slots.length;
            if (posCount === 0) continue;

            var activeCount = countActivePositions(group.slots);

            // Sustain-only group: standalone "-" extends prior event by full group duration
            if (activeCount === 0 && group.slots.length > 0) {
                if (events.length === 0)
                    return {error: "sustain with no prior note"};
                var sdNum = group.multiplier * beat.num;
                var sdDen = beat.den;
                var last = events[events.length - 1];
                last.duration.num = last.duration.num * sdDen + sdNum * last.duration.den;
                last.duration.den = last.duration.den * sdDen;
                var gv = gcd(last.duration.num, last.duration.den);
                last.duration.num /= gv;
                last.duration.den /= gv;
                if (last.nominal) {
                    last.nominal.num = last.nominal.num * sdDen + sdNum * last.nominal.den;
                    last.nominal.den = last.nominal.den * sdDen;
                    var ng2 = gcd(last.nominal.num, last.nominal.den);
                    last.nominal.num /= ng2;
                    last.nominal.den /= ng2;
                }
                continue;
            }

            if (activeCount === 0) continue;

            // Total group time = multiplier × beat
            var totalNum = group.multiplier * beat.num;
            var totalDen = beat.den;

            // Per-position (raw fraction, before sustain merging)
            var posNum = totalNum;
            var posDen = totalDen * posCount;

            // Does this group need a tuplet?  Check if the per-note
            // actual duration is a standard note value.
            var perNoteNum = totalNum;
            var perNoteDen = totalDen * activeCount;
            var needsTuplet = !isStandardDuration(perNoteNum, perNoteDen);

            if (needsTuplet) {
                var ratioNum = activeCount;
                var ratioDen = lowerPowerOf2(activeCount);
                // Nominal base = total_time / ratio_denominator
                var nomNum = totalNum;
                var nomDen = totalDen * ratioDen;
                var ng = gcd(nomNum, nomDen);
                nomNum /= ng;
                nomDen /= ng;

                var tg = gcd(totalNum, totalDen);
                events.push({
                    type: "tupletStart",
                    ratioNum: ratioNum,
                    ratioDen: ratioDen,
                    totalNum: totalNum / tg,
                    totalDen: totalDen / tg
                });
            }

            for (var s = 0; s < posCount; s++) {
                var slot = group.slots[s];
                if (slot.type === "sustain") {
                    if (events.length === 0)
                        return {error: "sustain with no prior note across groups"};
                    // Extend last event's duration by one position
                    var last = events[events.length - 1];
                    last.duration.num = last.duration.num * posDen + posNum * last.duration.den;
                    last.duration.den = last.duration.den * posDen;
                    var gVal = gcd(last.duration.num, last.duration.den);
                    last.duration.num /= gVal;
                    last.duration.den /= gVal;
                    if (last.nominal) {
                        last.nominal.num = last.nominal.num * posDen + posNum * last.nominal.den;
                        last.nominal.den = last.nominal.den * posDen;
                        var ng2 = gcd(last.nominal.num, last.nominal.den);
                        last.nominal.num /= ng2;
                        last.nominal.den /= ng2;
                    }
                } else {
                    var ev = {
                        type: slot.type,
                        duration: {num: posNum, den: posDen}
                    };
                    if (needsTuplet) {
                        ev.nominal = {num: nomNum, den: nomDen};
                    }
                    if (slot.type === "note") {
                        ev.letter = slot.letter;
                        ev.accidental = slot.accidental;
                        ev.octaveShift = slot.octaveShift;
                    } else if (slot.type === "chord") {
                        ev.pitches = slot.pitches;
                    }
                    events.push(ev);
                }
            }
        }

        return splitNonStandardDurations(events);
    }

    /**
     * Split non-standard durations (e.g. 5/8) into standard note values
     * that sum to the same total (e.g. 1/2 + 1/8).  This ensures every
     * setDuration(z,n) call uses a note value MuseScore can render.
     *
     * Events that come from a split are deep-copies of the original,
     * producing tied notes of the same pitch in the output.
     */
    function splitNonStandardDurations(events) {
        var result = [];
        for (var i = 0; i < events.length; i++) {
            var ev = events[i];
            if (ev.type !== "note" && ev.type !== "chord") {
                result.push(ev);
                continue;
            }
            var dur = ev.nominal || ev.duration;
            if (isStandardDuration(dur.num, dur.den)) {
                result.push(ev);
                continue;
            }
            // Greedy split: subtract largest standard note values first
            var remains = dur.num / dur.den;
            var standards = [
                {num:1, den:2}, {num:1, den:4}, {num:1, den:8},
                {num:1, den:16}, {num:1, den:32}, {num:1, den:64},
                {num:1, den:128}
            ];
            var first = true;
            while (remains > 0.00001) {
                for (var si = 0; si < standards.length; si++) {
                    var sv = standards[si].num / standards[si].den;
                    if (remains >= sv - 0.00001) {
                        var ne = {};
                        for (var k in ev) ne[k] = ev[k];
                        ne.duration = {num: standards[si].num, den: standards[si].den};
                        if (ev.nominal)
                            ne.nominal = {num: standards[si].num, den: standards[si].den};
                        ne._split = first ? undefined : true; // mark continuation
                        result.push(ne);
                        remains -= sv;
                        first = false;
                        break;
                    }
                }
            }
        }
        return result;
    }

    /**
     * Compute GCD for fraction reduction.
     */
    function gcd(a, b) {
        a = Math.abs(a);
        b = Math.abs(b);
        while (b > 0) {
            var t = b;
            b = a % b;
            a = t;
        }
        return a;
    }

    /**
     * Resolve relative octaves for all notes/chords using Lilypond
     * "closest interval" rule. Initial reference = C4 (MIDI 60).
     *
     * Mutates events in-place, adding a 'midi' or 'midis' field.
     */
    function resolveOctaves(events) {
        var lastPitch = 60; // C4

        for (var i = 0; i < events.length; i++) {
            var ev = events[i];
            if (ev.type === "tupletStart" || ev.type === "rest") continue;

            if (ev.type === "note") {
                ev.midi = resolvePitch(ev.letter, ev.accidental, ev.octaveShift, lastPitch);
                lastPitch = ev.midi;
            } else if (ev.type === "chord") {
                var midis = [];
                var chordRef = lastPitch;
                for (var p = 0; p < ev.pitches.length; p++) {
                    var pi = ev.pitches[p];
                    var m = resolvePitch(pi.letter, pi.accidental, pi.octaveShift, chordRef);
                    midis.push(m);
                    chordRef = m; // ascending within chord
                }
                ev.midis = midis;
                lastPitch = midis[midis.length - 1]; // chord's last pitch sets ref for next event
            }
        }
    }

    /**
     * Resolve a single pitch to MIDI number.
     * Uses Lilypond closest-interval rule from reference.
     * 'octaveShift' forces up/down by that many octaves before the
     * relative rule is applied.
     */
    function resolvePitch(letter, accidental, octaveShift, reference) {
        var base = noteOffsets[letter];
        var refOctave = Math.floor(reference / 12);
        var refNote = reference % 12;

        // Compute raw semitone in C4-based absolute octave
        var raw = base + accidental;
        // Find which octave puts it closest to reference
        var bestOctave = refOctave;
        var bestDiff = 999;
        for (var oct = refOctave - 2; oct <= refOctave + 2; oct++) {
            var candidate = oct * 12 + raw;
            var diff = Math.abs(candidate - reference);
            if (diff < bestDiff) {
                bestDiff = diff;
                bestOctave = oct;
            }
        }
        // Apply forced octave shift
        bestOctave += octaveShift;
        var midi = bestOctave * 12 + raw;
        // Clamp to MIDI range
        if (midi < 0) midi = 0;
        if (midi > 127) midi = 127;
        return midi;
    }

    /**
     * Full parse pipeline: normalize → tokenize → parseGroups → resolveDurations → resolveOctaves.
     *
     * Returns: {events: [...], error: string|null}
     */
    function parseDSL(text) {
        // Pass 1: tokenize
        var tokens = tokenize(text);
        if (tokens.length === 0)
            return {events: [], error: "No input"};

        // Pass 2-3: parse each group, skipping barline tokens
        var priorPitch = false;
        var groups = [];
        for (var t = 0; t < tokens.length; t++) {
            // Barline is syntactic sugar — skip it
            if (tokens[t].raw === symBarline) continue;
            var result = parseGroup(tokens[t].raw, priorPitch);
            if (result.error)
                return {events: [], error: "Group '" + tokens[t].raw + "': " + result.error};
            // Check if this group ends with a pitch (for cross-group sustain detection)
            for (var s = result.slots.length - 1; s >= 0; s--) {
                if (result.slots[s].type === "note" || result.slots[s].type === "chord") {
                    priorPitch = true;
                    break;
                }
                if (result.slots[s].type === "rest") {
                    priorPitch = false;
                    break;
                }
            }
            groups.push(result);
        }

        // Pass 4: resolve durations
        // Need cursor to read time signature. Use a temporary cursor if possible,
        // otherwise default to 4/4.
        var timeSigCursor = null;
        try {
            if (typeof curScore !== "undefined" && curScore) {
                timeSigCursor = curScore.newCursor();
                if (timeSigCursor) timeSigCursor.rewind(1);
            }
        } catch (e) { /* ignore */ }

        var events = resolveDurations(groups, timeSigCursor);
        if (events.error)
            return {events: [], error: events.error};

        // Pass 5: resolve octaves
        resolveOctaves(events);

        return {events: events, error: null};
    }

    // ---- Score insertion ----

    property var lastInserted: []    // tracks last inserted events for undo info
    property int lastNoteCount: 0

    function insertNotes(events) {
        if (typeof curScore === "undefined" || !curScore) {
            return { ok: false, msg: "No score open" };
        }

        var cursor = curScore.newCursor();
        cursor.track = 0;  // Staff 0, voice 0
        cursor.rewind(1);  // current cursor position (or selection start)

        curScore.startCmd();

        var count = 0;
        for (var i = 0; i < events.length; i++) {
            var ev = events[i];

            // Tuplet container start
            if (ev.type === "tupletStart") {
                cursor.addTuplet(
                    fraction(ev.ratioNum, ev.ratioDen),
                    fraction(ev.totalNum, ev.totalDen)
                );
                continue;
            }

            // Use nominal duration for tuplet notes, actual duration otherwise
            var dur = ev.nominal || ev.duration;
            var z = dur.num;
            var n = dur.den;
            var g = gcd(z, n);
            z /= g;
            n /= g;

            if (ev.type === "rest") {
                cursor.setDuration(z, n);
                cursor.addRest();
                count++;
            } else if (ev.type === "note") {
                cursor.setDuration(z, n);
                cursor.addNote(ev.midi, false);
                count++;
            } else if (ev.type === "chord") {
                cursor.setDuration(z, n);
                cursor.addNote(ev.midis[0], false);
                for (var p = 1; p < ev.midis.length; p++) {
                    cursor.addNote(ev.midis[p], true);
                }
                count++;
            }
        }

        curScore.endCmd();

        lastInserted = events;
        lastNoteCount = count;

        return { ok: true, msg: "Inserted " + count + " note(s)" };
    }

    // ---- UI actions ----

    function handleSend() {
        var text = inputField.text.trim();
        if (text.length === 0) {
            statusLabel.text = "Enter DSL input first";
            return;
        }

        var result = parseDSL(text);
        if (result.error) {
            statusLabel.color = "#c00";
            statusLabel.text = result.error;
            return;
        }

        if (result.events.length === 0) {
            statusLabel.text = "No notes to insert";
            return;
        }

        var insResult = insertNotes(result.events);
        if (insResult.ok) {
            statusLabel.color = "#1a7a1a";
        } else {
            statusLabel.color = "#c00";
        }
        statusLabel.text = insResult.msg;
    }

    function handleUndo() {
        if (lastNoteCount === 0) {
            statusLabel.text = "Nothing to undo";
            return;
        }
        try {
            curScore.undo();
            statusLabel.text = "Undo attempted — verify in score";
        } catch (e) {
            statusLabel.text = "Close plugin and press Ctrl+Z (Cmd+Z) to undo";
        }
        statusLabel.color = "#888";
    }

    // ---- Window ----

    onRun: {
        log("Plugin started");
        if (typeof curScore === "undefined" || !curScore) {
            log("No score open, will show error on Send");
        }
        statusLabel.text = "Type DSL and press Send";
        statusLabel.color = "#888";
        inputField.forceActiveFocus();
    }

    Keys.onPressed: {
        if (event.key === Qt.Key_Escape) {
            Qt.quit();
        }
        // Ctrl+Enter / Cmd+Enter to send
        if ((event.key === Qt.Key_Return || event.key === Qt.Key_Enter) &&
            (event.modifiers & Qt.ControlModifier || event.modifiers & Qt.MetaModifier)) {
            handleSend();
            event.accepted = true;
        }
    }

    // ---- UI Layout ----

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 14
        spacing: 8

        Label {
            text: "m4bon — Beat-Oriented Note Entry"
            font.pixelSize: 15
            font.bold: true
            color: "#333"
        }

        Label {
            text: "Enter beat groups (whitespace = beat separator):"
            font.pixelSize: 12
            color: "#666"
        }

        // DSL input field
        Rectangle {
            Layout.fillWidth: true
            Layout.preferredHeight: 120
            color: "#fafafa"
            border.color: "#ccc"
            border.width: 1
            radius: 4

            TextEdit {
                id: inputField
                anchors.fill: parent
                anchors.margins: 8
                font.family: "Menlo, Consolas, monospace"
                font.pixelSize: 14
                wrapMode: TextEdit.Wrap
                selectByMouse: true
                persistentSelection: true
                color: "#222"

                Text {
                    anchors.fill: parent
                    text: "a b | ab | a--b | (ace)f | 2abc"
                    color: "#bbb"
                    font: parent.font
                    visible: !parent.text.length && !parent.activeFocus
                    clip: true
                }
            }
        }

        // Example label
        Label {
            text: "Try: a b | ab | a--b | (ace)f | 2abc | a -; | for 5/8: 3abc 2ab"
            font.pixelSize: 10
            color: "#999"
            wrapMode: Label.Wrap
            Layout.fillWidth: true
        }

        // Button row
        RowLayout {
            Layout.fillWidth: true
            spacing: 10

            Button {
                id: sendBtn
                text: "Send"
                Layout.preferredWidth: 90
                Layout.preferredHeight: 30
                onClicked: handleSend()
            }

            Button {
                id: undoBtn
                text: "Undo"
                Layout.preferredWidth: 90
                Layout.preferredHeight: 30
                onClicked: handleUndo()
            }

            Item { Layout.fillWidth: true }
        }

        // Status bar
        Rectangle {
            Layout.fillWidth: true
            Layout.preferredHeight: 26
            color: "#f0f0f0"
            radius: 3

            Label {
                id: statusLabel
                anchors.fill: parent
                anchors.margins: 6
                font.pixelSize: 12
                verticalAlignment: Text.AlignVCenter
                color: "#888"
            }
        }

        // Spacer
        Item { Layout.fillHeight: true }
    }
}
