package parser

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mellis/m4bon/frac"
	"github.com/mellis/m4bon/theory"
)

// --- Duration resolution ---

// BeatDuration represents the duration of one beat as a fraction of a whole note.
type BeatDuration struct {
	Num, Den int
}

// ResolveBeatDuration returns the beat duration for a given time signature.
// num/den = time signature numerator/denominator (e.g. 4/4).
func ResolveBeatDuration(num, den int) BeatDuration {
	var z, n int
	switch den {
	case 2:
		z, n = 1, 2
	case 4:
		z, n = 1, 4
	case 8:
		if num%3 == 0 {
			z, n = 3, 8
		} else {
			z, n = 1, 8
		}
	case 16:
		if num%3 == 0 {
			z, n = 3, 16
		} else {
			z, n = 1, 16
		}
	default:
		z, n = 1, den
	}
	return BeatDuration{Num: z, Den: n}
}

// resolveDurationsWithPrior resolves beat groups into events, accepting an optional
// map of per-voice prior events for cross-measure sustain ties.
func resolveDurationsWithPrior(groups []ParseResult, beat BeatDuration, priorEvents map[int]*Event) ([]Event, error) {
	var events []Event
	voiceLastIdx := make(map[int]int) // per-voice last event index for sustain extension

	for gi, group := range groups {
		if group.Err != nil {
			return nil, group.Err
		}

		posCount := len(group.Slots)
		if posCount == 0 {
			continue
		}

		activeCount := countActivePositions(group.Slots)

		// Sustain-only group (all positions are sustains)
		if activeCount == 0 && len(group.Slots) > 0 {
			if len(events) == 0 {
				// Cross-measure sustain: use prior event for voice 1
				var pe *Event
				if priorEvents != nil {
					pe = priorEvents[1]
					if pe == nil {
						for v := 2; v <= 4; v++ {
							if priorEvents[v] != nil {
								pe = priorEvents[v]
								break
							}
						}
					}
					// nil sentinel means voice existed but had no pitch (rest);
					// skip silently rather than returning an error.
					if priorEvents[1] == nil {
						continue
					}
				}
				if pe == nil || (pe.Type != EventNote && pe.Type != EventChord) {
					return nil, fmt.Errorf("sustain with no prior note")
				}
				sdNum := group.Multiplier * beat.Num
				sdDen := beat.Den
				ev := Event{
					Type:            pe.Type,
					Duration:        Fraction{Num: sdNum, Den: sdDen},
					Letter:          pe.Letter,
					Accidental:      pe.Accidental,
					OctaveShift:     0, // sustain continues same pitch, no shift
					ExplicitNatural: pe.ExplicitNatural,
					Split:           true,
					Voice:           1,
					GroupIdx:        gi,
					NumSlots:        len(group.Slots),
				}
				if pe.Pitches != nil {
					ev.Pitches = slices.Clone(pe.Pitches)
				}
				events = append(events, ev)
				voiceLastIdx[1] = len(events) - 1
			} else {
				sdNum := group.Multiplier * beat.Num
				sdDen := beat.Den
				last := &events[len(events)-1]
				last.Duration.Num = last.Duration.Num*sdDen + sdNum*last.Duration.Den
				last.Duration.Den = last.Duration.Den * sdDen
				gv := frac.GCD(last.Duration.Num, last.Duration.Den)
				last.Duration.Num /= gv
				last.Duration.Den /= gv
				if last.Nominal != nil {
					last.Nominal.Num = last.Nominal.Num*sdDen + sdNum*last.Nominal.Den
					last.Nominal.Den = last.Nominal.Den * sdDen
					ng2 := frac.GCD(last.Nominal.Num, last.Nominal.Den)
					last.Nominal.Num /= ng2
					last.Nominal.Den /= ng2
				}
			}
			continue
		}

		if activeCount == 0 {
			continue
		}

		totalNum := group.Multiplier * beat.Num
		totalDen := beat.Den

		posNum := totalNum
		posDen := totalDen * posCount

		perNoteNum := totalNum
		perNoteDen := totalDen * posCount
		needsTuplet := !frac.IsStandardDuration(perNoteNum, perNoteDen)

		var nomNum, nomDen int
		var ratioNum, ratioDen int

		if needsTuplet {
			ratioNum = activeCount
			ratioDen = frac.LowerPowerOf2(activeCount)
			nomNum = totalNum
			nomDen = totalDen * ratioDen
			ng := frac.GCD(nomNum, nomDen)
			nomNum /= ng
			nomDen /= ng

			tg := frac.GCD(totalNum, totalDen)
			events = append(events, NewTupletStartEvent(
				Fraction{Num: totalNum / tg, Den: totalDen / tg},
				gi, ratioNum, ratioDen,
			))
		}

		for s := 0; s < posCount; s++ {
			slot := group.Slots[s]
			if slot.Type == SlotSustain {
				if len(events) == 0 {
					// Cross-measure sustain within a mixed group
					var pe *Event
					if priorEvents != nil {
						pe = priorEvents[1]
						if pe == nil {
							for v := 2; v <= 4; v++ {
								if priorEvents[v] != nil {
									pe = priorEvents[v]
									break
								}
							}
						}
						// nil sentinel: voice existed but had no pitch (rest); skip
						if priorEvents[1] == nil {
							continue
						}
					}
					if pe == nil || (pe.Type != EventNote && pe.Type != EventChord) {
						return nil, fmt.Errorf("sustain with no prior note across groups")
					}
					ev := Event{
						Type:            pe.Type,
						Duration:        Fraction{Num: posNum, Den: posDen},
						Letter:          pe.Letter,
						Accidental:      pe.Accidental,
						OctaveShift:     0, // sustain continues same pitch, no shift
						ExplicitNatural: pe.ExplicitNatural,
						Split:           true,
						Voice:           1,
						GroupIdx:        gi,
					}
					if pe.Pitches != nil {
						ev.Pitches = slices.Clone(pe.Pitches)
					}
					events = append(events, ev)
					voiceLastIdx[1] = len(events) - 1
				} else {
					last := &events[len(events)-1]
					if last.GroupIdx == gi {
						last.NumSlots++
					}
					last.Duration.Num = last.Duration.Num*posDen + posNum*last.Duration.Den
					last.Duration.Den = last.Duration.Den * posDen
					gVal := frac.GCD(last.Duration.Num, last.Duration.Den)
					last.Duration.Num /= gVal
					last.Duration.Den /= gVal
					if last.Nominal != nil {
						last.Nominal.Num = last.Nominal.Num*posDen + posNum*last.Nominal.Den
						last.Nominal.Den = last.Nominal.Den * posDen
						ng2 := frac.GCD(last.Nominal.Num, last.Nominal.Den)
						last.Nominal.Num /= ng2
						last.Nominal.Den /= ng2
					}
				}
			} else if slot.Type == SlotChord && slot.Entries != nil {
				// Voice-poly chord: expand into per-voice events
				for vi, entry := range slot.Entries {
					voice := vi + 1 // voices are 1-based
					switch entry.Type {
					case SlotNote:
						ev := NewNoteEvent(entry.Letter, entry.Accidental, entry.OctaveShift, entry.ExplicitNatural, Fraction{Num: posNum, Den: posDen}, nil, voice, gi)
						if needsTuplet {
							ev.Nominal = &Fraction{Num: nomNum, Den: nomDen}
						}
						events = append(events, ev)
						voiceLastIdx[voice] = len(events) - 1

					case SlotSustain:
						// Extend the last event for this voice
						idx, ok := voiceLastIdx[voice]
						if !ok && priorEvents != nil {
							// Check for cross-measure sustain
							if pe, hasPrior := priorEvents[voice]; hasPrior {
								if pe == nil {
									// Voice existed but had a rest — skip silently
									continue
								}
								ev := Event{
									Type:            pe.Type,
									Duration:        Fraction{Num: posNum, Den: posDen},
									Letter:          pe.Letter,
									Accidental:      pe.Accidental,
									OctaveShift:     0, // sustain continues same pitch, no shift
									ExplicitNatural: pe.ExplicitNatural,
									Split:           true,
									Voice:           voice,
									GroupIdx:        gi,
									NumSlots:        1,
								}
								if pe.Pitches != nil {
									ev.Pitches = slices.Clone(pe.Pitches)
								}
								events = append(events, ev)
								voiceLastIdx[voice] = len(events) - 1
								continue
							}
						}
						if !ok {
							return nil, fmt.Errorf("sustain in voice %d with no prior note", voice)
						}
						last := &events[idx]
						if last.GroupIdx == gi {
							last.NumSlots++
						}
						last.Duration.Num = last.Duration.Num*posDen + posNum*last.Duration.Den
						last.Duration.Den = last.Duration.Den * posDen
						gVal := frac.GCD(last.Duration.Num, last.Duration.Den)
						last.Duration.Num /= gVal
						last.Duration.Den /= gVal
						if last.Nominal != nil {
							last.Nominal.Num = last.Nominal.Num*posDen + posNum*last.Nominal.Den
							last.Nominal.Den = last.Nominal.Den * posDen
							ng2 := frac.GCD(last.Nominal.Num, last.Nominal.Den)
							last.Nominal.Num /= ng2
							last.Nominal.Den /= ng2
						}

					case SlotRest:
						ev := NewRestEvent(Fraction{Num: posNum, Den: posDen}, nil, voice, gi)
						if needsTuplet {
							ev.Nominal = &Fraction{Num: nomNum, Den: nomDen}
						}
						events = append(events, ev)
						// Rests don't establish a voice for future sustains
					}
				}
			} else {
				// Traditional note, chord, or rest — single-voice (Voice=1)
				switch slot.Type {
				case SlotNote:
					ev := NewNoteEvent(slot.Letter, slot.Accidental, slot.OctaveShift, slot.ExplicitNatural, Fraction{Num: posNum, Den: posDen}, nil, 1, gi)
					if needsTuplet {
						ev.Nominal = &Fraction{Num: nomNum, Den: nomDen}
					}
					events = append(events, ev)
				case SlotChord:
					ev := NewChordEvent(slot.Pitches, Fraction{Num: posNum, Den: posDen}, nil, 1, gi)
					if needsTuplet {
						ev.Nominal = &Fraction{Num: nomNum, Den: nomDen}
					}
					events = append(events, ev)
				case SlotRest:
					ev := NewRestEvent(Fraction{Num: posNum, Den: posDen}, nil, 1, gi)
					if needsTuplet {
						ev.Nominal = &Fraction{Num: nomNum, Den: nomDen}
					}
					events = append(events, ev)
				}
				voiceLastIdx[1] = len(events) - 1
			}
		}
	}

	return events, nil
}

// splitAtBarline splits notes and chords that cross the invisible barline
// at the midpoint of the measure (between beats 2 and 3 in 4/4, or the
// equivalent in other meters). Notes that span the barline are split into
// two tied events so no single note-value crosses that boundary.
func splitAtBarline(events []Event, timeNum, timeDen int) []Event {
	if timeDen == 0 || timeNum%2 != 0 {
		// Only split for even-beat meters (4/4, 2/4, 6/8, etc.) where the
		// midpoint falls between two beats. For odd-beat meters (3/4, 9/8, etc.)
		// the mathematical midpoint is inside a beat and splitting there would
		// create incorrect notation (e.g. splitting a quarter note into tied
		// eighths in 3/4).
		return events
	}

	// barline = timeNum / (2 * timeDen) of a whole note
	bNum := timeNum
	bDen := timeDen * 2

	var result []Event
	// Running position as fraction of a whole note
	pNum := 0
	pDen := 1

	for _, ev := range events {
		if ev.Type != EventNote && ev.Type != EventChord {
			result = append(result, ev)
			// Advance position
			pNum = pNum*ev.Duration.Den + ev.Duration.Num*pDen
			pDen = pDen * ev.Duration.Den
			g := frac.GCD(pNum, pDen)
			pNum /= g
			pDen /= g
			continue
		}

		dNum := ev.Duration.Num
		dDen := ev.Duration.Den

		// start = pNum/pDen, end = pNum/pDen + dNum/dDen
		// Check: is start < barline < end ?
		// start < barline: pNum/pDen < bNum/bDen  → pNum*bDen < bNum*pDen
		// end > barline:  (pNum*dDen + dNum*pDen) / (pDen*dDen) > bNum/bDen
		//                → (pNum*dDen + dNum*pDen) * bDen > bNum * pDen * dDen
		startLessBarline := pNum*bDen < bNum*pDen
		endGreaterBarline := (pNum*dDen+dNum*pDen)*bDen > bNum*pDen*dDen

		if !startLessBarline || !endGreaterBarline {
			// Doesn't cross the barline
			result = append(result, ev)
			pNum = pNum*dDen + dNum*pDen
			pDen = pDen * dDen
			g := frac.GCD(pNum, pDen)
			pNum /= g
			pDen /= g
			continue
		}

		// Split at barline
		// before = barline - start = bNum/bDen - pNum/pDen
		//       = (bNum*pDen - pNum*bDen) / (bDen*pDen)
		beforeNum := bNum*pDen - pNum*bDen
		beforeDen := bDen * pDen
		g1 := frac.GCD(beforeNum, beforeDen)
		beforeNum /= g1
		beforeDen /= g1

		// after = duration - before = dNum/dDen - beforeNum/beforeDen
		//      = (dNum*beforeDen - beforeNum*dDen) / (dDen*beforeDen)
		afterNum := dNum*beforeDen - beforeNum*dDen
		afterDen := dDen * beforeDen
		g2 := frac.GCD(afterNum, afterDen)
		afterNum /= g2
		afterDen /= g2

		ev1 := ev
		ev1.Duration = Fraction{Num: beforeNum, Den: beforeDen}
		// ev1.Split retains its original value (false for first fragment)

		ev2 := ev
		if ev.Pitches != nil {
			ev2.Pitches = slices.Clone(ev.Pitches)
		}
		if ev.Midis != nil {
			ev2.Midis = slices.Clone(ev.Midis)
		}
		ev2.Duration = Fraction{Num: afterNum, Den: afterDen}
		ev2.Split = true

		result = append(result, ev1, ev2)

		// Advance past ev2
		pNum = pNum*dDen + dNum*pDen
		pDen = pDen * dDen
		g := frac.GCD(pNum, pDen)
		pNum /= g
		pDen /= g
	}

	return result
}

// --- Split non-standard durations ---

var standardDurations = []Fraction{
	{Num: 1, Den: 2}, {Num: 1, Den: 4}, {Num: 1, Den: 8}, {Num: 1, Den: 16}, {Num: 1, Den: 32}, {Num: 1, Den: 64}, {Num: 1, Den: 128},
}

func splitNonStandardDurations(events []Event) []Event {
	var result []Event
	for _, ev := range events {
		if ev.Type != EventNote && ev.Type != EventChord {
			result = append(result, ev)
			continue
		}
		dur := ev.Duration
		if ev.Nominal != nil {
			dur = *ev.Nominal
		}
		if frac.IsStandardDuration(dur.Num, dur.Den) {
			result = append(result, ev)
			continue
		}

		remains := Fraction{Num: dur.Num, Den: dur.Den}
		first := true
		for remains.Num > 0 {
			matched := false
			for _, sd := range standardDurations {
				if !remains.LessThan(sd) { // remains >= sd
					ne := ev
					ne.Duration = sd
					if ev.Nominal != nil {
						ne.Nominal = &sd
					}
					ne.Split = !first
					result = append(result, ne)
					remains = remains.Sub(sd)
					first = false
					matched = true
					break
				}
			}
			if !matched {
				// Safety: no standard duration fits — shouldn't happen since input is non-standard
				break
			}
		}
	}
	return result
}

// --- Directive parsing ---

// extractDirectivesTail peels :H and :L tokens from the end of a measure's
// token group. Returns the remaining tokens (notation + K/M/B directives),
// and the extracted chord/lyric token slices.
//
// Algorithm: L→R state machine. Tokens before the first :H/:L are notation
// tokens. After :H: payload tokens go to chordTokens. After :L: payload tokens
// go to lyricTokens. Markers themselves are consumed. The last marker seen
// determines where subsequent tokens go.
func extractDirectivesTail(tokens []Token) (remaining []Token, chordTokens, lyricTokens []string) {
	var notationTokens []Token
	state := 0 // 0=notation, 1=chords, 2=lyrics

	for _, tok := range tokens {
		raw := tok.Raw
		if raw == ":H" || raw == ":h" {
			state = 1
			continue
		}
		if raw == ":L" || raw == ":l" {
			state = 2
			continue
		}
		switch state {
		case 1:
			chordTokens = append(chordTokens, raw)
		case 2:
			lyricTokens = append(lyricTokens, raw)
		default:
			notationTokens = append(notationTokens, tok)
		}
	}
	return notationTokens, chordTokens, lyricTokens
}

var keySigMap = map[string]int{
	"c": 0, "g": 1, "d": 2, "a": 3, "e": 4, "b": 5,
	"f#": 6, "c#": 7,
	"f": -1, "b&": -2, "e&": -3, "a&": -4, "d&": -5, "g&": -6, "c&": -7,
}

// canonicalKey normalizes a key signature body (e.g. "e&", "&e", "eb", "&e")
// to its canonical form for lookup in keySigMap.
func canonicalKey(body string) string {
	// Extract letter and accidentals
	letter := ""
	acc := ""
	for _, ch := range body {
		if (ch >= 'a' && ch <= 'g') || (ch >= 'A' && ch <= 'G') {
			letter = strings.ToLower(string(ch))
		} else {
			switch ch {
			case '#', '♯':
				acc += "#"
			case '&', 'b', '♭':
				acc += "&"
			case '%', '♮':
				acc = ""
			}
		}
	}
	if letter == "" {
		return "c"
	}
	return letter + acc
}

// --- Octave resolution ---

// letterIndex maps pitch letters a-g to consecutive diatonic indices 0-6.
var letterIndex = map[string]int{"c": 0, "d": 1, "e": 2, "f": 3, "g": 4, "a": 5, "b": 6}

// resolveOctave picks the closest octave for targetLetter relative to a
// reference letter+octave, using diatonic step distance. octaveShift is
// applied after the closest octave is found.
func resolveOctave(targetLetter, refLetter string, refOctave, octaveShift int) int {
	targetIdx := letterIndex[targetLetter]
	refIdx := letterIndex[refLetter]

	bestOctave := refOctave
	bestDist := 999
	for o := refOctave - 2; o <= refOctave+2; o++ {
		dist := abs((o*7 + targetIdx) - (refOctave*7 + refIdx))
		if dist < bestDist {
			bestDist = dist
			bestOctave = o
		}
	}
	return bestOctave + octaveShift
}

// nextHigherOctave returns the octave for targetLetter such that it sits
// above refLetter+refOctave. Chords are always ascending in pitch letter;
// wraps octave when targetIdx <= refIdx. octaveShift is applied afterward.
func nextHigherOctave(refLetter, targetLetter string, refOctave, octaveShift int) int {
	targetIdx := letterIndex[targetLetter]
	refIdx := letterIndex[refLetter]
	oct := refOctave
	if targetIdx <= refIdx {
		oct++
	}
	return oct + octaveShift
}

// midiFromPitch computes a MIDI note number from letter, accidental, and octave.
func midiFromPitch(letter string, accidental, octave int) int {
	midi := octave*12 + theory.NoteOffsets[letter] + accidental
	if midi < 0 {
		midi = 0
	}
	if midi > 127 {
		midi = 127
	}
	return midi
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// timeSigTicks returns the total ticks for a measure of the given time signature.
func timeSigTicks(num, den int) int {
	return frac.TicksPerWholeNote * num / den
}

// totalTicks returns the total duration of events in ticks.
// For multi-voice content, returns the max tick across all voices.
func totalTicks(events []Event) int {
	voiceTick := make(map[int]int)
	maxTick := 0
	for _, ev := range events {
		if ev.Type == EventTupletStart {
			continue
		}
		v := ev.Voice
		if v == 0 {
			v = 1
		}
		t := voiceTick[v]
		t += frac.TicksPerWholeNote * ev.Duration.Num / ev.Duration.Den
		voiceTick[v] = t
		if t > maxTick {
			maxTick = t
		}
	}
	return maxTick
}

// deriveTimeSig derives a time signature from a number of beats and a beat duration.
// The result is numBeats * beat.Num / beat.Den, preserving musical meaning (no simplification).
func deriveTimeSig(numBeats int, beat BeatDuration) (int, int) {
	return numBeats * beat.Num, beat.Den
}

// fifthsToAccidentalMap builds a map from pitch letter to its key-signature accidental.
// fifths: circle of fifths position (positive=sharps, negative=flats).
func fifthsToAccidentalMap(fifths int) map[string]int {
	return theory.FifthsToAccidentalMap(fifths)
}

// effectiveAccidental computes the accidental to use for pitch resolution:
//   - If ExplicitNatural, use 0 (natural)
//   - If explicit Accidental != 0, use that
//   - Otherwise check the key signature
//   - Default 0
func effectiveAccidental(letter string, explicitAcc int, explicitNatural bool, keyAcc map[string]int) int {
	return theory.EffectiveAccidental(letter, explicitAcc, explicitNatural, keyAcc)
}

// --- Main parse entry point ---

// measureDirectives holds parsed K, M, B directives for a single measure.
type measureDirectives struct {
	fifths        int
	timeNum       int
	timeDen       int
	beat          BeatDuration
	beatTokens    []Token
	explicitMeter bool // true if this measure has its own M directive
	hasBeatCode   bool // true if B directive was found
}

// splitMeasures splits tokens at | boundaries into measure groups.
// Returns the groups and whether any barline was found.
// scanMeasureDirectives scans tokens for K, M, B directives and returns
// the parsed directives along with the remaining beat tokens.
func scanMeasureDirectives(tokens []Token, defaultFifths, defaultTimeNum, defaultTimeDen int) measureDirectives {
	var md measureDirectives
	md.fifths = defaultFifths
	md.timeNum = defaultTimeNum
	md.timeDen = defaultTimeDen
	foundNotation := false

	for _, tok := range tokens {
		raw := tok.Raw
		if foundNotation {
			md.beatTokens = append(md.beatTokens, tok)
			continue
		}
		if strings.HasPrefix(raw, "k") || strings.HasPrefix(raw, "K") {
			body := raw[1:]
			canon := canonicalKey(body)
			if f, ok := keySigMap[canon]; ok {
				md.fifths = f
			}
			continue
		}
		if strings.HasPrefix(raw, "m") || strings.HasPrefix(raw, "M") {
			body := raw[1:]
			if n, err := fmt.Sscanf(body, "%d/%d", &md.timeNum, &md.timeDen); err == nil && n == 2 {
				md.explicitMeter = true
			}
			continue
		}
		if (strings.HasPrefix(raw, "b") || strings.HasPrefix(raw, "B")) && len(raw) > 1 {
			bc := strings.ToUpper(raw[1:])
			if bd, ok := BeatDurationCodes[bc]; ok {
				md.hasBeatCode = true
				md.beat = bd
				continue
			}
			// Unknown beat suffix — treat as notation token
			foundNotation = true
			md.beatTokens = append(md.beatTokens, tok)
			continue
		}
		foundNotation = true
		md.beatTokens = append(md.beatTokens, tok)
	}

	// Derive time sig from beat when no explicit M but B directive present
	if md.hasBeatCode && !md.explicitMeter {
		md.timeNum, md.timeDen = deriveTimeSig(len(md.beatTokens), md.beat)
	}

	return md
}

// parseBeatTokens parses each beat-group token into a ParseResult.
func parseBeatTokens(tokens []Token, priorPitch bool) (groups []ParseResult, numGroups int, errs []string) {
	numGroups = len(tokens)
	for _, tok := range tokens {
		result := parseGroup(tok.Raw, priorPitch)
		if result.Err != nil {
			errs = append(errs, fmt.Sprintf("group '%s': %v", tok.Raw, result.Err))
			if len(errs) >= 10 {
				return
			}
			continue
		}
		for s := len(result.Slots) - 1; s >= 0; s-- {
			if result.Slots[s].Type == SlotNote || result.Slots[s].Type == SlotChord {
				priorPitch = true
				break
			}
			if result.Slots[s].Type == SlotRest {
				priorPitch = false
				break
			}
		}
		groups = append(groups, result)
	}
	return
}

// buildPriorEvents builds a per-voice prior-event map from the last measure
// for cross-measure sustain resolution.
func buildPriorEvents(measures []MeasureResult) map[int]*Event {
	priorEvents := make(map[int]*Event)
	if len(measures) == 0 {
		return priorEvents
	}
	prevEvents := measures[len(measures)-1].Events
	for i := len(prevEvents) - 1; i >= 0; i-- {
		ev := &prevEvents[i]
		if ev.Type == EventNote || ev.Type == EventChord {
			if _, ok := priorEvents[ev.Voice]; !ok {
				priorEvents[ev.Voice] = ev
			}
		}
	}

	// Expand traditional chords: each pitch is a virtual voice (1-based).
	// This allows voice-poly sustains (e.g. (- - g)) to pick up
	// individual pitches from a prior measure's chord (e.g. (c d e)).
	for i := len(prevEvents) - 1; i >= 0; i-- {
		ev := &prevEvents[i]
		if ev.Type == EventChord && ev.Voice == 1 && len(ev.Pitches) > 1 {
			for pi := 1; pi < len(ev.Pitches); pi++ {
				v := pi + 1 // voice 2, 3, 4...
				if _, ok := priorEvents[v]; !ok {
					p := ev.Pitches[pi]
					virtual := Event{
						Type:            EventNote,
						Letter:          p.Letter,
						Accidental:      p.Accidental,
						OctaveShift:     p.OctaveShift,
						ExplicitNatural: p.ExplicitNatural,
						Voice:           v,
					}
					priorEvents[v] = &virtual
				}
			}
		}
	}

	// Track voices that had explicit rests as nil sentinels.
	for i := len(prevEvents) - 1; i >= 0; i-- {
		ev := &prevEvents[i]
		if ev.Type == EventRest {
			if _, ok := priorEvents[ev.Voice]; !ok {
				priorEvents[ev.Voice] = nil // voice exists, had no pitch
			}
		}
	}

	return priorEvents
}

// markCrossMeasureTies finds a cross-measure sustain at the start of events
// and marks the previous measure's corresponding note with TieNext.
func markCrossMeasureTies(events []Event, measures []MeasureResult) {
	if len(events) == 0 || !events[0].Split || len(measures) == 0 {
		return
	}
	sustainVoice := events[0].Voice
	prevMeas := &measures[len(measures)-1]
	for i := len(prevMeas.Events) - 1; i >= 0; i-- {
		ev := &prevMeas.Events[i]
		if (ev.Type == EventNote || ev.Type == EventChord) && ev.Voice == sustainVoice {
			ev.TieNext = true
			break
		}
	}
}

// measureHasNote returns true if any event in the slice is a note or chord.
func measureHasNote(events []Event) bool {
	for _, ev := range events {
		if ev.Type == EventNote || ev.Type == EventChord {
			return true
		}
	}
	return false
}

// validateExplicitMeter checks that events fill the expected measure duration.
// Returns a formatted error string, or empty string if valid.
func validateExplicitMeter(events []Event, timeNum, timeDen int, tokens []Token, measureIdx int, hasSecondMeasure, hasExplicitMeter, isFirstMeasure bool) string {
	if !hasExplicitMeter {
		return ""
	}
	expectedTicks := timeSigTicks(timeNum, timeDen)
	actualTicks := totalTicks(events)
	if actualTicks == expectedTicks {
		return ""
	}
	// Allow pickup (first measure, shorter, has second measure)
	if isFirstMeasure && hasSecondMeasure && actualTicks < expectedTicks {
		return ""
	}
	var inputBuilder strings.Builder
	for _, tok := range tokens {
		if inputBuilder.Len() > 0 {
			inputBuilder.WriteString(" ")
		}
		inputBuilder.WriteString(tok.Raw)
	}
	return fmt.Sprintf("Measure %d: expected %d/%d (%d ticks), got %d ticks\n  Input: %q\n  Suggestion: check beat grouping", measureIdx+1, timeNum, timeDen, expectedTicks, actualTicks, inputBuilder.String())
}

// detectPickup returns true if the first measure in a multi-measure input
// is shorter than the time signature capacity.
func detectPickup(events []Event, timeNum, timeDen int, measureIdx int, hasSecondMeasure bool) bool {
	if measureIdx != 0 || !hasSecondMeasure {
		return false
	}
	capacity := timeSigTicks(timeNum, timeDen)
	return totalTicks(events) < capacity
}

// resolveOctavesMeasures resolves octaves and MIDI pitch numbers for all events
// across all measures, using per-voice reference tracking (Lilypond "closest interval" rule).
// Octave resolution is purely diatonic (letter+octave); MIDI is derived via lookup after.
func resolveOctavesMeasures(measures []MeasureResult) {
	lastOctave := make(map[int]int)
	lastLetter := make(map[int]string)
	lastOctave[1] = 5 // default: voice 1 starts at C4 (MIDI 60/12)
	lastLetter[1] = "c"
	for mi := range measures {
		keyAcc := fifthsToAccidentalMap(measures[mi].Fifths)

		// Per-measure accidental tracking: letter → effective accidental.
		measureAcc := make(map[string]int)

		for i := range measures[mi].Events {
			ev := &measures[mi].Events[i]
			if ev.Type == EventTupletStart || ev.Type == EventRest {
				continue
			}

			v := ev.Voice
			refOct, ok := lastOctave[v]
			if !ok {
				refOct = 5
				lastOctave[v] = refOct
				lastLetter[v] = "c"
			}
			refLet := lastLetter[v]

			if ev.Type == EventNote {
				acc := measureLevelAccidental(ev.Letter, ev.Accidental, ev.ExplicitNatural, keyAcc, measureAcc)
				ev.EffAccidental = acc
				oct := resolveOctave(ev.Letter, refLet, refOct, ev.OctaveShift)
				ev.ResolvedOctave = oct
				ev.Midi = midiFromPitch(ev.Letter, acc, oct)
				lastOctave[v] = oct
				lastLetter[v] = ev.Letter
			} else if ev.Type == EventChord {
				if ev.Split {
					var prev Event
					if i > 0 && measures[mi].Events[i-1].Type == EventChord {
						prev = measures[mi].Events[i-1]
					} else if mi > 0 {
						for j := len(measures[mi-1].Events) - 1; j >= 0; j-- {
							if measures[mi-1].Events[j].Type == EventChord {
								prev = measures[mi-1].Events[j]
								break
							}
						}
					}
					if len(prev.Midis) == len(ev.Pitches) {
						ev.Midis = slices.Clone(prev.Midis)
						ev.ResolvedOctaves = slices.Clone(prev.ResolvedOctaves)
						continue
					}
				}
				chordOct := refOct
				chordLet := refLet
				for p := range ev.Pitches {
					pi := ev.Pitches[p]
					var oct int
					acc := measureLevelAccidental(pi.Letter, pi.Accidental, pi.ExplicitNatural, keyAcc, measureAcc)
					ev.Pitches[p].Accidental = acc
					if p == 0 {
						oct = resolveOctave(pi.Letter, chordLet, chordOct, pi.OctaveShift)
					} else {
						oct = nextHigherOctave(chordLet, pi.Letter, chordOct, pi.OctaveShift)
					}
					m := midiFromPitch(pi.Letter, acc, oct)
					ev.Midis = append(ev.Midis, m)
					ev.ResolvedOctaves = append(ev.ResolvedOctaves, oct)
					chordOct = oct
					chordLet = pi.Letter
				}
				lastOctave[v] = ev.ResolvedOctaves[len(ev.ResolvedOctaves)-1]
				lastLetter[v] = ev.Pitches[len(ev.Pitches)-1].Letter
			}
		}
	}
}

// measureLevelAccidental returns the effective accidental for a note, taking into
// account any prior accidental on the same letter within the current measure.
// If the note has an explicit accidental or natural sign, it updates the
// measureAcc map. Otherwise, it falls through to the measure's prior accidental
// (if any) and then to the key signature.
func measureLevelAccidental(letter string, explicitAcc int, explicitNatural bool, keyAcc map[string]int, measureAcc map[string]int) int {
	if explicitAcc != 0 || explicitNatural {
		// User explicitly specified an accidental or natural — this sets the
		// measure-level accidental for this letter.
		acc := effectiveAccidental(letter, explicitAcc, explicitNatural, keyAcc)
		measureAcc[letter] = acc
		return acc
	}
	// No explicit accidental — check measure-level tracking first, then key sig.
	if acc, ok := measureAcc[letter]; ok {
		return acc
	}
	return effectiveAccidental(letter, 0, false, keyAcc)
}

// commentForIndex returns the comment lines for the given measure index
// from the comments map, or nil if none exist.
func commentForIndex(comments map[int][]string, idx int) []string {
	if comments == nil {
		return nil
	}
	return comments[idx]
}

// ParseDSL parses m4bon DSL text into a sequence of measures.
// Key signature (K...), meter (M...), and beat duration (B...) directives
// are parsed from the DSL itself. Defaults: C major, 4/4.
// Measures are separated by newlines. Each measure can have its own directives.
func ParseDSL(lines []string) DSLResult {
	return ParseDSLWithComments(lines, nil)
}

// ParseDSLWithComments is like ParseDSL but accepts a map of comments
// keyed by measure index. Each entry is a slice of '!' line comment
// bodies appearing immediately before that measure.
func ParseDSLWithComments(lines []string, comments map[int][]string) DSLResult {
	if len(lines) == 0 {
		return DSLResult{Err: fmt.Errorf("no input")}
	}

	hasMultipleLines := len(lines) > 1

	// Pre-scan first line for initial key/meter directives
	firstTokens := tokenize(lines[0])
	md0 := scanMeasureDirectives(firstTokens, 0, 4, 4)
	currentFifths := md0.fifths
	currentTimeNum := md0.timeNum
	currentTimeDen := md0.timeDen
	hasInitialMeter := md0.explicitMeter

	// Save these for DSLResult return values
	initialFifths := currentFifths
	initialTimeNum := currentTimeNum
	initialTimeDen := currentTimeDen

	var measures []MeasureResult
	var errs []string
	lastMeasureHadNote := false

	for mi, line := range lines {
		hasSecondMeasure := mi < len(lines)-1
		tokens := tokenize(line)

		// Extract :H/:L directives from tail
		notationTokens, chordRaw, lyricRaw := extractDirectivesTail(tokens)

		// Scan directives from remaining tokens
		md := scanMeasureDirectives(notationTokens, currentFifths, currentTimeNum, currentTimeDen)

		// Override defaults if explicit M found
		effectiveTimeNum := md.timeNum
		effectiveTimeDen := md.timeDen

		// Resolve beat if no B directive
		if !md.hasBeatCode {
			md.beat = ResolveBeatDuration(effectiveTimeNum, effectiveTimeDen)
		}

		// Parse beat groups
		groups, numGroups, groupErrs := parseBeatTokens(md.beatTokens, lastMeasureHadNote)
		for _, ge := range groupErrs {
			errs = append(errs, fmt.Sprintf("Measure %d: %s", mi+1, ge))
		}
		if len(errs) >= 10 {
			break
		}

		// Build prior events for cross-measure sustain
		priorEvents := buildPriorEvents(measures)

		// Resolve durations
		events, err := resolveDurationsWithPrior(groups, md.beat, priorEvents)
		if err != nil {
			errs = append(errs, fmt.Sprintf("Measure %d: %v", mi+1, err))
			if len(errs) >= 10 {
				break
			}
			continue
		}

		// Mark cross-measure ties
		markCrossMeasureTies(events, measures)

		// Auto-detect time sig from content when no explicit directive
		if !md.explicitMeter && !md.hasBeatCode && hasMultipleLines && !(mi == 0 && hasInitialMeter) {
			actualTicks := totalTicks(events)
			expectedTicks := timeSigTicks(effectiveTimeNum, effectiveTimeDen)
			if actualTicks != expectedTicks && actualTicks > 0 {
				g := frac.GCD(actualTicks, frac.TicksPerWholeNote)
				effectiveTimeNum = actualTicks / g
				effectiveTimeDen = frac.TicksPerWholeNote / g
			}
		}

		// Validate against explicit M directive
		hasExplicitMeter := (mi == 0 && hasInitialMeter && hasMultipleLines) || md.explicitMeter
		if errStr := validateExplicitMeter(events, effectiveTimeNum, effectiveTimeDen, tokens, mi, hasSecondMeasure, hasExplicitMeter, mi == 0); errStr != "" {
			errs = append(errs, errStr)
			if len(errs) >= 10 {
				break
			}
		}

		// Split at barline and non-standard durations
		events = splitAtBarline(events, effectiveTimeNum, effectiveTimeDen)
		events = splitNonStandardDurations(events)

		// Pickup detection
		isPickup := detectPickup(events, effectiveTimeNum, effectiveTimeDen, mi, hasSecondMeasure)

		// Build GroupSlots from parsed groups
		groupSlots := make([]int, numGroups)
		groupMults := make([]int, numGroups)
		for gIdx, grp := range groups {
			if gIdx < len(groupSlots) {
				groupSlots[gIdx] = len(grp.Slots)
				groupMults[gIdx] = grp.Multiplier
			}
		}

		measures = append(measures, MeasureResult{
			Events:     events,
			TimeNum:    effectiveTimeNum,
			TimeDen:    effectiveTimeDen,
			Fifths:     md.fifths,
			IsPickup:   isPickup,
			NumGroups:  numGroups,
			GroupSlots: groupSlots,
			GroupMults: groupMults,
			Chords:     chordRaw,
			Lyrics:     lyricRaw,
			HasChords:  len(chordRaw) > 0,
			HasLyrics:  len(lyricRaw) > 0,
			CommentLines: commentForIndex(comments, mi),
		})

		lastMeasureHadNote = measureHasNote(events)
		currentFifths = md.fifths
		currentTimeNum = effectiveTimeNum
		currentTimeDen = effectiveTimeDen
	}

	// Attach trailing comment to the last measure
	if len(measures) > 0 {
		if tc, ok := comments[len(lines)]; ok {
			measures[len(measures)-1].TrailingCommentLines = tc
		}
	}

	// Resolve octaves across all measures
	resolveOctavesMeasures(measures)

	// Build final error
	var finalErr error
	if len(errs) > 0 {
		finalErr = fmt.Errorf("%s", strings.Join(errs, "\n"))
	}

	return DSLResult{
		Measures: measures,
		Key:      KeySignature{Fifths: initialFifths},
		TimeNum:  initialTimeNum,
		TimeDen:  initialTimeDen,
		Err:      finalErr,
	}
}
