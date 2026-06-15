import MuseScore 3.0
import QtQuick 2.9
import QtQuick.Controls 2.2

MuseScore {
    id: plugin
    title: "m4bon-runner"
    description: "Headless runner for m4bon CLI testing"
    version: "0.6.0"
    categoryCode: "composing-arranging-tools"
    pluginType: "dialog"
    width: 1
    height: 1

    function log(msg) { console.log("[m4bon-runner] " + msg); }

    function readLocalFile(filePath) {
        try {
            var xhr = new XMLHttpRequest();
            xhr.open("GET", "file://" + filePath, false);
            xhr.send(null);
            if (xhr.status === 0 || xhr.status === 200)
                return xhr.responseText.trim();
        } catch (e) { log("readLocalFile error: " + e); }
        return "";
    }

    property var noteOffsets: ({'c':0,'d':2,'e':4,'f':5,'g':7,'a':9,'b':11})

    function normalizePitchInput(text) {
        var t = text.toLowerCase();
        t = t.replace(/[♯]/g, "#");
        t = t.replace(/[♭]/g, "&");
        t = t.replace(/[♮]/g, "%");
        return t;
    }

    function resolveBeatDuration(cursor) {
        var num = 4, den = 4;
        try {
            if (cursor && cursor.measure) {
                num = cursor.measure.timesigNumerator || 4;
                den = cursor.measure.timesigDenominator || 4;
            }
        } catch (e) {}
        var z, n;
        if (den === 2) { z = 1; n = 2; }
        else if (den === 4) { z = 1; n = 4; }
        else if (den === 8) { z = (num % 3 === 0) ? 3 : 1; n = 8; }
        else if (den === 16) { z = (num % 3 === 0) ? 3 : 1; n = 16; }
        else { z = 1; n = den; }
        return {num: z, den: n};
    }

    function tokenize(text) {
        var normalized = normalizePitchInput(text);
        var tokens = [];
        var re = /\S+/g;
        var match;
        while ((match = re.exec(normalized)) !== null)
            tokens.push({raw: match[0], offset: match.index});
        return tokens;
    }

    function parseGroup(raw, priorPitchExists) {
        var multiplier = 1;
        var slots = [];
        var i = 0, acc = 0, oct = 0, hasLetter = false, letter = "";
        var inChord = false, chordPitches = [];

        function err(msg, offset) { return {multiplier:1, slots:[], error:msg, errorOffset:offset}; }
        function emitNote(offset) {
            if (inChord) {
                if (!hasLetter) return err("accidental/octave without pitch in chord", offset);
                chordPitches.push({letter:letter, accidental:acc, octaveShift:oct});
            } else {
                if (!hasLetter) return err("accidental/octave without pitch at end of group", offset);
                slots.push({type:"note", letter:letter, accidental:acc, octaveShift:oct});
            }
            acc = 0; oct = 0; hasLetter = false; letter = "";
            return null;
        }

        while (i < raw.length) {
            var ch = raw[i];
            var errResult = null;
            if (!inChord && i === 0 && ch >= '1' && ch <= '9') {
                var multStart = i;
                multiplier = 0;
                while (i < raw.length && raw[i] >= '0' && raw[i] <= '9') {
                    multiplier = multiplier * 10 + (raw[i].charCodeAt(0) - 48);
                    i++;
                }
                if (multiplier === 0) return err("beat multiplier cannot be zero", multStart);
                continue;
            }
            if (ch >= '0' && ch <= '9') return err("unexpected digit", i);
            if (ch === '#') { acc += 1; i++; continue; }
            if (ch === '&') { acc -= 1; i++; continue; }
            if (ch === '%') { acc = 0; i++; continue; }
            if (ch === '^') { oct += 1; i++; continue; }
            if (ch === '/') { oct -= 1; i++; continue; }
            if (ch === '-') {
                if (hasLetter) { errResult = emitNote(i); if (errResult) return errResult; }
                if (slots.length === 0 && !priorPitchExists) return err("sustain with no prior note", i);
                slots.push({type:"sustain"}); i++; continue;
            }
            if (ch === ';') {
                if (hasLetter) { errResult = emitNote(i); if (errResult) return errResult; }
                slots.push({type:"rest"}); i++; continue;
            }
            if (ch === '(') {
                if (inChord) return err("nested chords not allowed", i);
                if (hasLetter) { errResult = emitNote(i); if (errResult) return errResult; }
                inChord = true; chordPitches = []; i++; continue;
            }
            if (ch === ')') {
                if (!inChord) return err("unmatched closing parenthesis", i);
                if (hasLetter) { errResult = emitNote(i); if (errResult) return errResult; }
                if (chordPitches.length === 0) return err("empty chord", i);
                for (var p = 1; p < chordPitches.length; p++) {
                    if (chordPitches[p].letter <= chordPitches[p-1].letter)
                        return err("chord pitches must be strictly ascending", i);
                }
                slots.push({type:"chord", pitches:chordPitches}); inChord = false; chordPitches = []; i++; continue;
            }
            var lower = ch.toLowerCase();
            if (lower >= 'a' && lower <= 'g') {
                if (hasLetter) { errResult = emitNote(i); if (errResult) return errResult; }
                letter = lower; hasLetter = true; i++; continue;
            }
            return err("unexpected character '" + ch + "'", i);
        }
        if (inChord) return err("unclosed chord", raw.length);
        if (hasLetter) { var flushErr = emitNote(i); if (flushErr) return flushErr; }
        if (acc !== 0 || oct !== 0) return err("bare accidental/octave at end of group", raw.length);
        return {multiplier:multiplier, slots:slots, error:null, errorOffset:-1};
    }

    function isPowerOf2(n) { return n > 0 && (n & (n - 1)) === 0; }
    function lowerPowerOf2(n) { if (n <= 1) return 1; var p = 1; while (p * 2 < n) p *= 2; return p; }

    function isStandardDuration(z, n) {
        var g = gcd(z, n); z /= g; n /= g;
        return isPowerOf2(n) && (z === 1 || z === 3);
    }

    function countActivePositions(slots) { var n = 0; for (var i = 0; i < slots.length; i++) { if (slots[i].type !== "sustain") n++; } return n; }

    function resolveDurations(groups, timeSig) {
        var beat = resolveBeatDuration(timeSig);
        var events = [];
        for (var g = 0; g < groups.length; g++) {
            var group = groups[g];
            if (group.error) return group;
            var posCount = group.slots.length;
            if (posCount === 0) continue;
            var activeCount = countActivePositions(group.slots);
            if (activeCount === 0 && group.slots.length > 0) {
                if (events.length === 0) return {error:"sustain with no prior note"};
                var sdNum = group.multiplier * beat.num, sdDen = beat.den;
                var last = events[events.length - 1];
                last.duration.num = last.duration.num * sdDen + sdNum * last.duration.den;
                last.duration.den = last.duration.den * sdDen;
                var gv = gcd(last.duration.num, last.duration.den);
                last.duration.num /= gv; last.duration.den /= gv;
                if (last.nominal) {
                    last.nominal.num = last.nominal.num * sdDen + sdNum * last.nominal.den;
                    last.nominal.den = last.nominal.den * sdDen;
                    var ng2 = gcd(last.nominal.num, last.nominal.den);
                    last.nominal.num /= ng2; last.nominal.den /= ng2;
                }
                continue;
            }
            if (activeCount === 0) continue;
            var totalNum = group.multiplier * beat.num, totalDen = beat.den;
            var posNum = totalNum, posDen = totalDen * posCount;
            var perNoteNum = totalNum, perNoteDen = totalDen * activeCount;
            var needsTuplet = !isStandardDuration(perNoteNum, perNoteDen);
            if (needsTuplet) {
                var ratioNum = activeCount, ratioDen = lowerPowerOf2(activeCount);
                var nomNum = totalNum, nomDen = totalDen * ratioDen;
                var ng = gcd(nomNum, nomDen); nomNum /= ng; nomDen /= ng;
                var tg = gcd(totalNum, totalDen);
                events.push({type:"tupletStart", ratioNum:ratioNum, ratioDen:ratioDen, totalNum:totalNum/tg, totalDen:totalDen/tg});
            }
            for (var s = 0; s < posCount; s++) {
                var slot = group.slots[s];
                if (slot.type === "sustain") {
                    if (events.length === 0) return {error:"sustain with no prior note across groups"};
                    var last = events[events.length - 1];
                    last.duration.num = last.duration.num * posDen + posNum * last.duration.den;
                    last.duration.den = last.duration.den * posDen;
                    var gVal = gcd(last.duration.num, last.duration.den);
                    last.duration.num /= gVal; last.duration.den /= gVal;
                    if (last.nominal) {
                        last.nominal.num = last.nominal.num * posDen + posNum * last.nominal.den;
                        last.nominal.den = last.nominal.den * posDen;
                        var ng2 = gcd(last.nominal.num, last.nominal.den);
                        last.nominal.num /= ng2; last.nominal.den /= ng2;
                    }
                } else {
                    var ev = {type:slot.type, duration:{num:posNum, den:posDen}};
                    if (needsTuplet) ev.nominal = {num:nomNum, den:nomDen};
                    if (slot.type === "note") { ev.letter = slot.letter; ev.accidental = slot.accidental; ev.octaveShift = slot.octaveShift; }
                    else if (slot.type === "chord") { ev.pitches = slot.pitches; }
                    events.push(ev);
                }
            }
        }
        return splitNonStandardDurations(events);
    }

    function splitNonStandardDurations(events) {
        var result = [];
        for (var i = 0; i < events.length; i++) {
            var ev = events[i];
            if (ev.type !== "note" && ev.type !== "chord") { result.push(ev); continue; }
            var dur = ev.nominal || ev.duration;
            if (isStandardDuration(dur.num, dur.den)) { result.push(ev); continue; }
            var remains = dur.num / dur.den;
            var standards = [{num:1,den:2},{num:1,den:4},{num:1,den:8},{num:1,den:16},{num:1,den:32},{num:1,den:64},{num:1,den:128}];
            var first = true;
            while (remains > 0.00001) {
                for (var si = 0; si < standards.length; si++) {
                    var sv = standards[si].num / standards[si].den;
                    if (remains >= sv - 0.00001) {
                        var ne = {}; for (var k in ev) ne[k] = ev[k];
                        ne.duration = {num:standards[si].num, den:standards[si].den};
                        if (ev.nominal) ne.nominal = {num:standards[si].num, den:standards[si].den};
                        ne._split = first ? undefined : true;
                        result.push(ne); remains -= sv; first = false; break;
                    }
                }
            }
        }
        return result;
    }

    function gcd(a, b) { a = Math.abs(a); b = Math.abs(b); while (b > 0) { var t = b; b = a % b; a = t; } return a; }

    function resolvePitch(letter, accidental, octaveShift, reference) {
        var base = noteOffsets[letter];
        var refOctave = Math.floor(reference / 12), refNote = reference % 12;
        var raw = base + accidental;
        var bestOctave = refOctave, bestDiff = 999;
        for (var oct = refOctave - 2; oct <= refOctave + 2; oct++) {
            var candidate = oct * 12 + raw;
            var diff = Math.abs(candidate - reference);
            if (diff < bestDiff) { bestDiff = diff; bestOctave = oct; }
        }
        bestOctave += octaveShift;
        var midi = bestOctave * 12 + raw;
        if (midi < 0) midi = 0; if (midi > 127) midi = 127;
        return midi;
    }

    function resolveOctaves(events) {
        var lastPitch = 60;
        for (var i = 0; i < events.length; i++) {
            var ev = events[i];
            if (ev.type === "tupletStart" || ev.type === "rest") continue;
            if (ev.type === "note") {
                ev.midi = resolvePitch(ev.letter, ev.accidental, ev.octaveShift, lastPitch);
                lastPitch = ev.midi;
            } else if (ev.type === "chord") {
                var midis = [], chordRef = lastPitch;
                for (var p = 0; p < ev.pitches.length; p++) {
                    var pi = ev.pitches[p];
                    var m = resolvePitch(pi.letter, pi.accidental, pi.octaveShift, chordRef);
                    midis.push(m); chordRef = m;
                }
                ev.midis = midis; lastPitch = midis[midis.length - 1];
            }
        }
    }

    function parseDSL(text) {
        var tokens = tokenize(text);
        if (tokens.length === 0) return {events:[], error:"No input"};
        var priorPitch = false, groups = [];
        for (var t = 0; t < tokens.length; t++) {
            if (tokens[t].raw === "|") continue;
            var result = parseGroup(tokens[t].raw, priorPitch);
            if (result.error) return {events:[], error:"Group '" + tokens[t].raw + "': " + result.error};
            for (var s = result.slots.length - 1; s >= 0; s--) {
                if (result.slots[s].type === "note" || result.slots[s].type === "chord") { priorPitch = true; break; }
                if (result.slots[s].type === "rest") { priorPitch = false; break; }
            }
            groups.push(result);
        }
        var timeSigCursor = null;
        try {
            if (typeof curScore !== "undefined" && curScore) {
                timeSigCursor = curScore.newCursor();
                if (timeSigCursor) timeSigCursor.rewind(1);
            }
        } catch (e) {}
        var events = resolveDurations(groups, timeSigCursor);
        if (events.error) return {events:[], error:events.error};
        resolveOctaves(events);
        return {events:events, error:null};
    }

    function insertNotes(events) {
        if (typeof curScore === "undefined" || !curScore) return {ok:false, msg:"No score open"};
        var cursor = curScore.newCursor();
        cursor.track = 0;
        cursor.rewind(0); // Start of score — no selection needed
        curScore.startCmd();
        var count = 0;
        for (var i = 0; i < events.length; i++) {
            var ev = events[i];
            if (ev.type === "tupletStart") { cursor.addTuplet(fraction(ev.ratioNum, ev.ratioDen), fraction(ev.totalNum, ev.totalDen)); continue; }
            var dur = ev.nominal || ev.duration;
            var z = dur.num, n = dur.den;
            var g = gcd(z, n); z /= g; n /= g;
            if (ev.type === "rest") { cursor.setDuration(z, n); cursor.addRest(); count++; }
            else if (ev.type === "note") { cursor.setDuration(z, n); cursor.addNote(ev.midi, false); count++; }
            else if (ev.type === "chord") {
                cursor.setDuration(z, n);
                cursor.addNote(ev.midis[0], false);
                for (var p = 1; p < ev.midis.length; p++) cursor.addNote(ev.midis[p], true);
                count++;
            }
        }
        curScore.endCmd();
        return {ok:true, msg:"Inserted " + count + " note(s)"};
    }

    onRun: {
        log("Runner started");
        if (typeof curScore === "undefined" || !curScore) { log("No score open"); return; }
        var path = "/tmp/m4bon-dsl.txt";
        var dsl = readLocalFile(path);
        if (dsl.length === 0) { log("No DSL at " + path); return; }
        log("DSL: " + dsl);
        var result = parseDSL(dsl);
        if (result.error) { log("Parse error: " + result.error); return; }
        log("Events: " + result.events.length);
        var insResult = insertNotes(result.events);
        log(insResult.msg);
    }
}
