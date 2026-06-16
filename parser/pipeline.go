package parser

import (
	"fmt"
	"math"
	"regexp"
	"strings"
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

func resolveDurations(groups []ParseResult, beat BeatDuration) ([]Event, error) {
	return resolveDurationsWithPrior(groups, beat, nil)
}

// resolveDurationsWithPrior is like resolveDurations but accepts an optional
// prior event for cross-measure sustain ties. When the first group is a
// sustain-only group, the prior event's pitch is used as the reference.
func resolveDurationsWithPrior(groups []ParseResult, beat BeatDuration, priorEvent *Event) ([]Event, error) {
	var events []Event

	for _, group := range groups {
		if group.Err != nil {
			return nil, group.Err
		}

		posCount := len(group.Slots)
		if posCount == 0 {
			continue
		}

		activeCount := countActivePositions(group.Slots)

		// Sustain-only group
		if activeCount == 0 && len(group.Slots) > 0 {
			if len(events) == 0 {
				if priorEvent == nil || (priorEvent.Type != EventNote && priorEvent.Type != EventChord) {
					return nil, fmt.Errorf("sustain with no prior note")
				}
				// Cross-measure sustain: create a new event based on the prior event
				sdNum := group.Multiplier * beat.Num
				sdDen := beat.Den
				ev := Event{
					Type:        priorEvent.Type,
					Duration:    Fraction{Num: sdNum, Den: sdDen},
					Letter:      priorEvent.Letter,
					Accidental:  priorEvent.Accidental,
					OctaveShift: priorEvent.OctaveShift,
					Split:       true,
				}
				if priorEvent.Pitches != nil {
					ev.Pitches = make([]Pitch, len(priorEvent.Pitches))
					copy(ev.Pitches, priorEvent.Pitches)
				}
				events = append(events, ev)
			} else {
				sdNum := group.Multiplier * beat.Num
				sdDen := beat.Den
				last := &events[len(events)-1]
				last.Duration.Num = last.Duration.Num*sdDen + sdNum*last.Duration.Den
				last.Duration.Den = last.Duration.Den * sdDen
				gv := gcd(last.Duration.Num, last.Duration.Den)
				last.Duration.Num /= gv
				last.Duration.Den /= gv
				if last.Nominal != nil {
					last.Nominal.Num = last.Nominal.Num*sdDen + sdNum*last.Nominal.Den
					last.Nominal.Den = last.Nominal.Den * sdDen
					ng2 := gcd(last.Nominal.Num, last.Nominal.Den)
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
		perNoteDen := totalDen * activeCount
		needsTuplet := !isStandardDuration(perNoteNum, perNoteDen)

		var nomNum, nomDen int
		var ratioNum, ratioDen int

		if needsTuplet {
			ratioNum = activeCount
			ratioDen = lowerPowerOf2(activeCount)
			nomNum = totalNum
			nomDen = totalDen * ratioDen
			ng := gcd(nomNum, nomDen)
			nomNum /= ng
			nomDen /= ng

			tg := gcd(totalNum, totalDen)
			events = append(events, Event{
				Type:     EventTupletStart,
				Duration: Fraction{Num: totalNum / tg, Den: totalDen / tg},
			})
			// Store tuplet ratio in the dummy event — we encode it in the
			// next notes' time-modification in the MusicXML layer.
			events[len(events)-1].Midi = ratioNum     // temporary: actual-notes
			events[len(events)-1].OctaveShift = ratioDen // temporary: normal-notes
		}

		for s := 0; s < posCount; s++ {
			slot := group.Slots[s]
			if slot.Type == SlotSustain {
				if len(events) == 0 {
					// Cross-measure sustain within a mixed group: create a
					// starting event from the prior measure's last note.
					if priorEvent == nil || (priorEvent.Type != EventNote && priorEvent.Type != EventChord) {
						return nil, fmt.Errorf("sustain with no prior note across groups")
					}
					ev := Event{
						Type:        priorEvent.Type,
						Duration:    Fraction{Num: posNum, Den: posDen},
						Letter:      priorEvent.Letter,
						Accidental:  priorEvent.Accidental,
						OctaveShift: priorEvent.OctaveShift,
						Split:       true,
					}
					if priorEvent.Pitches != nil {
						ev.Pitches = make([]Pitch, len(priorEvent.Pitches))
						copy(ev.Pitches, priorEvent.Pitches)
					}
					events = append(events, ev)
				} else {
					last := &events[len(events)-1]
					last.Duration.Num = last.Duration.Num*posDen + posNum*last.Duration.Den
					last.Duration.Den = last.Duration.Den * posDen
					gVal := gcd(last.Duration.Num, last.Duration.Den)
					last.Duration.Num /= gVal
					last.Duration.Den /= gVal
					if last.Nominal != nil {
						last.Nominal.Num = last.Nominal.Num*posDen + posNum*last.Nominal.Den
						last.Nominal.Den = last.Nominal.Den * posDen
						ng2 := gcd(last.Nominal.Num, last.Nominal.Den)
						last.Nominal.Num /= ng2
						last.Nominal.Den /= ng2
					}
				}
			} else {
				ev := Event{
					Type:     EventType(slot.Type),
					Duration: Fraction{Num: posNum, Den: posDen},
				}
				if needsTuplet {
					ev.Nominal = &Fraction{Num: nomNum, Den: nomDen}
				}
				switch slot.Type {
				case SlotNote:
					ev.Letter = slot.Letter
					ev.Accidental = slot.Accidental
					ev.OctaveShift = slot.OctaveShift
				case SlotChord:
					ev.Pitches = slot.Pitches
				}
				events = append(events, ev)
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
			g := gcd(pNum, pDen)
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
			g := gcd(pNum, pDen)
			pNum /= g
			pDen /= g
			continue
		}

		// Split at barline
		// before = barline - start = bNum/bDen - pNum/pDen
		//       = (bNum*pDen - pNum*bDen) / (bDen*pDen)
		beforeNum := bNum*pDen - pNum*bDen
		beforeDen := bDen * pDen
		g1 := gcd(beforeNum, beforeDen)
		beforeNum /= g1
		beforeDen /= g1

		// after = duration - before = dNum/dDen - beforeNum/beforeDen
		//      = (dNum*beforeDen - beforeNum*dDen) / (dDen*beforeDen)
		afterNum := dNum*beforeDen - beforeNum*dDen
		afterDen := dDen * beforeDen
		g2 := gcd(afterNum, afterDen)
		afterNum /= g2
		afterDen /= g2

		ev1 := ev
		ev1.Duration = Fraction{Num: beforeNum, Den: beforeDen}
		// ev1.Split retains its original value (false for first fragment)

		ev2 := ev
		ev2.Duration = Fraction{Num: afterNum, Den: afterDen}
		ev2.Split = true

		result = append(result, ev1, ev2)

		// Advance past ev2
		pNum = pNum*dDen + dNum*pDen
		pDen = pDen * dDen
		g := gcd(pNum, pDen)
		pNum /= g
		pDen /= g
	}

	return result
}

// --- Split non-standard durations ---

var standardDurations = []Fraction{
	{1, 2}, {1, 4}, {1, 8}, {1, 16}, {1, 32}, {1, 64}, {1, 128},
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
		if isStandardDuration(dur.Num, dur.Den) {
			result = append(result, ev)
			continue
		}

		remains := float64(dur.Num) / float64(dur.Den)
		first := true
		for remains > 0.00001 {
			for _, sd := range standardDurations {
				sv := float64(sd.Num) / float64(sd.Den)
				if remains >= sv-0.00001 {
					ne := ev
					ne.Duration = sd
					if ev.Nominal != nil {
						ne.Nominal = &sd
					}
					ne.Split = !first
					result = append(result, ne)
					remains -= sv
					first = false
					break
				}
			}
		}
	}
	return result
}

// --- Directive parsing ---

var keySigMap = map[string]int{
	"c": 0, "g": 1, "d": 2, "a": 3, "e": 4, "b": 5,
	"f#": 6, "c#": 7,
	"f": -1, "b&": -2, "e&": -3, "a&": -4, "d&": -5, "g&": -6, "c&": -7,
}

// stripDirectives extracts K (key) and M (meter) directives
// from the start of the DSL string, returning the stripped DSL and parsed metadata.
// Returns whether an M directive was found at the global level.
func stripDirectives(text string) (stripped string, fifths int, timeNum, timeDen int, hasMeter bool) {
	fifths = 0   // default: C major
	timeNum = 4  // default: 4/4
	timeDen = 4

	re := regexp.MustCompile(`^(K\S+\s*)?(M\S+\s*)?`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return text, fifths, timeNum, timeDen, false
	}

	directives := ""
	for i := 1; i < len(m); i++ {
		directives += strings.TrimSpace(m[i]) + " "
	}
	directives = strings.TrimSpace(directives)

	if directives != "" {
		// Parse K directive
		for _, part := range strings.Fields(directives) {
			if strings.HasPrefix(part, "K") && len(part) > 1 {
				body := strings.ToLower(part[1:])
				// Map to key signature
				// Try exact match first, then try various orderings
				canon := canonicalKey(body)
				if f, ok := keySigMap[canon]; ok {
					fifths = f
				}
			}
			if strings.HasPrefix(part, "M") && len(part) > 1 {
				body := part[1:]
				if n, err := fmt.Sscanf(body, "%d/%d", &timeNum, &timeDen); err == nil && n == 2 {
					hasMeter = true
				}
			}
		}

		// Remove directives from text
		text = strings.TrimSpace(re.ReplaceAllString(text, ""))
	}

	return text, fifths, timeNum, timeDen, hasMeter
}

// canonicalKey normalizes a key signature body (e.g. "e&", "&e", "eb", "&e")
// to its canonical form for lookup in keySigMap.
func canonicalKey(body string) string {
	// Extract letter and accidentals
	letter := ""
	acc := ""
	for _, ch := range body {
		if ch >= 'a' && ch <= 'g' {
			letter = string(ch)
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

var noteOffsets = map[string]int{
	"c": 0, "d": 2, "e": 4, "f": 5, "g": 7, "a": 9, "b": 11,
}

// nextHigherPitch finds the smallest MIDI note with the given letter and accidental
// that is strictly higher than the reference pitch. Used for chord voicing where
// each subsequent chord tone must be the next octave higher.
func nextHigherPitch(letter string, accidental, octaveShift int, reference int) int {
	base := noteOffsets[letter]
	raw := base + accidental
	refOctave := reference / 12
	for oct := refOctave; oct <= refOctave+4; oct++ {
		candidate := oct*12 + raw
		if candidate > reference {
			candidate += octaveShift * 12
			if candidate < 0 {
				candidate = 0
			}
			if candidate > 127 {
				candidate = 127
			}
			return candidate
		}
	}
	return reference + 12
}

func resolvePitch(letter string, accidental, octaveShift int, reference int) int {
	base := noteOffsets[letter]
	refOctave := reference / 12
	raw := base + accidental

	bestOctave := refOctave
	bestDiff := 999
	for oct := refOctave - 2; oct <= refOctave+2; oct++ {
		candidate := oct*12 + raw
		diff := int(math.Abs(float64(candidate - reference)))
		if diff < bestDiff {
			bestDiff = diff
			bestOctave = oct
		}
	}
	bestOctave += octaveShift
	midi := bestOctave*12 + raw
	if midi < 0 {
		midi = 0
	}
	if midi > 127 {
		midi = 127
	}
	return midi
}

// timeSigTicks returns the total ticks for a measure of the given time signature.
func timeSigTicks(num, den int) int {
	return TicksPerWholeNote * num / den
}

// totalTicks returns the total duration of events in ticks.
func totalTicks(events []Event) int {
	total := 0
	for _, ev := range events {
		if ev.Type == EventTupletStart {
			continue
		}
		total += TicksPerWholeNote * ev.Duration.Num / ev.Duration.Den
	}
	return total
}

// deriveTimeSig derives a time signature from a number of beats and a beat duration.
// The result is numBeats * beat.Num / beat.Den, preserving musical meaning (no simplification).
func deriveTimeSig(numBeats int, beat BeatDuration) (int, int) {
	return numBeats * beat.Num, beat.Den
}

// --- Main parse entry point ---

// ParseDSL parses m4bon DSL text into a sequence of measures.
// Key signature (K...), meter (M...), and beat duration (B...) directives
// are parsed from the DSL itself. Defaults: C major, 4/4.
// Measures are separated by |. Each measure can have its own directives.
func ParseDSL(text string) DSLResult {
	text, fifths, timeNum, timeDen, hasInitialMeter := stripDirectives(text)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return DSLResult{Err: fmt.Errorf("no input")}
	}

	// Split tokens at | boundaries
	var measureTokenGroups [][]Token
	var curGroup []Token
	hasBarline := false
	for _, tok := range tokens {
		if tok.Raw == "|" {
			hasBarline = true
			if len(curGroup) > 0 {
				measureTokenGroups = append(measureTokenGroups, curGroup)
				curGroup = nil
			}
			continue
		}
		curGroup = append(curGroup, tok)
	}
	if len(curGroup) > 0 {
		measureTokenGroups = append(measureTokenGroups, curGroup)
	}

	// If no | separators, treat everything as one measure
	if len(measureTokenGroups) == 0 {
		measureTokenGroups = [][]Token{tokens}
	}

	currentFifths := fifths
	currentTimeNum := timeNum
	currentTimeDen := timeDen
	var measures []MeasureResult
	var errs []string
	lastMeasureHadNote := false

	for mi, group := range measureTokenGroups {
		// Scan tokens for directives
		var beatTokens []Token
		mFifths := currentFifths
		mTimeNum := 0
		mTimeDen := 0
		beatCode := ""

		for _, tok := range group {
			raw := tok.Raw
			// Tokens are lowercased by tokenizer, so check lowercase prefixes
			if strings.HasPrefix(raw, "k") && len(raw) > 1 {
				body := raw[1:] // already lowercased
				canon := canonicalKey(body)
				if f, ok := keySigMap[canon]; ok {
					mFifths = f
				}
				continue
			}
			if strings.HasPrefix(raw, "m") && len(raw) > 1 {
				body := raw[1:]
				if n, err := fmt.Sscanf(body, "%d/%d", &mTimeNum, &mTimeDen); err == nil && n == 2 {
					// parsed OK
				}
				continue
			}
			if strings.HasPrefix(raw, "b") && len(raw) > 1 {
				beatCode = strings.ToUpper(raw[1:])
				continue
			}
			beatTokens = append(beatTokens, tok)
		}

		// Determine effective time sig and beat for this measure
		effectiveTimeNum := currentTimeNum
		effectiveTimeDen := currentTimeDen

		if mTimeNum > 0 {
			effectiveTimeNum = mTimeNum
			effectiveTimeDen = mTimeDen
		}

		var beat BeatDuration
		if beatCode != "" {
			if bd, ok := BeatDurationCodes[beatCode]; ok {
				beat = bd
			} else {
				beat = BeatDuration{1, 4} // fallback
			}
			// Derive time sig from content if no explicit M
			if mTimeNum == 0 {
				effectiveTimeNum, effectiveTimeDen = deriveTimeSig(len(beatTokens), beat)
			}
		} else {
			beat = ResolveBeatDuration(effectiveTimeNum, effectiveTimeDen)
		}

		// Parse beat groups — priorPitch carries across measures for sustain ties
		priorPitch := lastMeasureHadNote
		var groups []ParseResult
		for _, tok := range beatTokens {
			result := parseGroup(tok.Raw, priorPitch)
			if result.Err != nil {
				errs = append(errs, fmt.Sprintf("Measure %d: group '%s': %v", mi+1, tok.Raw, result.Err))
				if len(errs) >= 10 {
					break
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

		if len(errs) >= 10 {
			break
		}

		// Build prior event for cross-measure sustain
		var priorEvent *Event
		if len(measures) > 0 {
			for i := len(measures[len(measures)-1].Events) - 1; i >= 0; i-- {
				ev := measures[len(measures)-1].Events[i]
				if ev.Type == EventNote || ev.Type == EventChord {
					priorEvent = &ev
					break
				}
			}
		}

		// Resolve durations
		events, err := resolveDurationsWithPrior(groups, beat, priorEvent)
		if err != nil {
			errs = append(errs, fmt.Sprintf("Measure %d: %v", mi+1, err))
			if len(errs) >= 10 {
				break
			}
			continue
		}

		// If the first event is a cross-measure sustain (Split && !Nominal continuation),
		// mark the previous measure's last note for tie-start
		if len(events) > 0 && events[0].Split && len(measures) > 0 {
			prevMeas := &measures[len(measures)-1]
			for i := len(prevMeas.Events) - 1; i >= 0; i-- {
				ev := &prevMeas.Events[i]
				if ev.Type == EventNote || ev.Type == EventChord {
					ev.TieNext = true
					break
				}
			}
		}

		// Auto-detect time sig from resolved content when no explicit directive
		// and content doesn't fill the inherited meter. Only in multi-measure mode.
		if mTimeNum == 0 && beatCode == "" && hasBarline && !(mi == 0 && hasInitialMeter) {
			actualTicks := totalTicks(events)
			expectedTicks := timeSigTicks(effectiveTimeNum, effectiveTimeDen)
			if actualTicks != expectedTicks && actualTicks > 0 {
				g := gcd(actualTicks, TicksPerWholeNote)
				effectiveTimeNum = actualTicks / g
				effectiveTimeDen = TicksPerWholeNote / g
			}
		}

		// Validate against explicit M directive
		// Only validate when there's an explicit M directive and the content
		// doesn't fill the measure. Skip for pickup measures (first measure of
		// multi-group input that's shorter than expected).
		hasSecondMeasure := len(measureTokenGroups) > 1
		hasExplicitMeter := (mi == 0 && hasInitialMeter && hasBarline) || mTimeNum > 0
		if hasExplicitMeter {
			expectedTicks := timeSigTicks(effectiveTimeNum, effectiveTimeDen)
			actualTicks := totalTicks(events)
			if actualTicks != expectedTicks {
				// Skip error for potential pickup (first measure, shorter, has second measure)
				if mi == 0 && hasSecondMeasure && actualTicks < expectedTicks {
					// will be handled by pickup detection below
				} else {
					var inputBuilder strings.Builder
					for _, tok := range group {
						if inputBuilder.Len() > 0 {
							inputBuilder.WriteString(" ")
						}
						inputBuilder.WriteString(tok.Raw)
					}
					errs = append(errs, fmt.Sprintf("Measure %d: expected %d/%d (%d ticks), got %d ticks\n  Input: %q\n  Suggestion: check beat grouping", mi+1, effectiveTimeNum, effectiveTimeDen, expectedTicks, actualTicks, inputBuilder.String()))
					if len(errs) >= 10 {
						break
					}
				}
			}
		}

		// Split at barline (with this measure's time sig)
		events = splitAtBarline(events, effectiveTimeNum, effectiveTimeDen)

		// Split non-standard durations
		events = splitNonStandardDurations(events)

		// Pickup detection (only for first measure when there are multiple measures)
		isPickup := false
		if mi == 0 && hasSecondMeasure {
			capacity := timeSigTicks(effectiveTimeNum, effectiveTimeDen)
			actualTicks := totalTicks(events)
			if actualTicks < capacity {
				isPickup = true
			}
		}

		measures = append(measures, MeasureResult{
			Events:   events,
			TimeNum:  effectiveTimeNum,
			TimeDen:  effectiveTimeDen,
			Fifths:   mFifths,
			IsPickup: isPickup,
		})

		// Track whether this measure had any note/chord for cross-measure sustain
		lastMeasureHadNote = false
		for _, ev := range events {
			if ev.Type == EventNote || ev.Type == EventChord {
				lastMeasureHadNote = true
				break
			}
		}

		currentFifths = mFifths
		currentTimeNum = effectiveTimeNum
		currentTimeDen = effectiveTimeDen
	}

	// Resolve octaves across all measures
	lastPitch := 60
	for mi := range measures {
		for i := range measures[mi].Events {
			ev := &measures[mi].Events[i]
			if ev.Type == EventTupletStart || ev.Type == EventRest {
				continue
			}
			if ev.Type == EventNote {
				ev.Midi = resolvePitch(ev.Letter, ev.Accidental, ev.OctaveShift, lastPitch)
				lastPitch = ev.Midi
			} else if ev.Type == EventChord {
				// For split continuations (within same measure or across barline),
				// copy MIDI from the predecessor so tied chord fragments keep
				// the same voicing.
				if ev.Split {
					var prev Event
					if i > 0 && measures[mi].Events[i-1].Type == EventChord {
						prev = measures[mi].Events[i-1]
					} else if mi > 0 {
						// Cross-measure split: look at previous measure's last chord
						for j := len(measures[mi-1].Events) - 1; j >= 0; j-- {
							if measures[mi-1].Events[j].Type == EventChord {
								prev = measures[mi-1].Events[j]
								break
							}
						}
					}
					if len(prev.Midis) == len(ev.Pitches) {
						ev.Midis = make([]int, len(prev.Midis))
						copy(ev.Midis, prev.Midis)
						continue
					}
				}
				chordRef := lastPitch
				for p := range ev.Pitches {
					pi := ev.Pitches[p]
					var m int
					if p == 0 {
						m = resolvePitch(pi.Letter, pi.Accidental, pi.OctaveShift, chordRef)
					} else {
						m = nextHigherPitch(pi.Letter, pi.Accidental, pi.OctaveShift, chordRef)
					}
					ev.Midis = append(ev.Midis, m)
					chordRef = m
				}
				lastPitch = ev.Midis[len(ev.Midis)-1]
			}
		}
	}

	// Build final error
	var finalErr error
	if len(errs) > 0 {
		finalErr = fmt.Errorf("%s", strings.Join(errs, "\n"))
	}

	return DSLResult{
		Measures: measures,
		Key:      KeySignature{Fifths: fifths},
		TimeNum:  timeNum,
		TimeDen:  timeDen,
		Err:      finalErr,
	}
}
