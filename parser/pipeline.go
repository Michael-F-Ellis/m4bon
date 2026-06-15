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
				return nil, fmt.Errorf("sustain with no prior note")
			}
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
					return nil, fmt.Errorf("sustain with no prior note across groups")
				}
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
	if timeDen == 0 {
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

// stripDirectives extracts K (key) and M (meter) directives from the start
// of the DSL string, returning the stripped DSL and parsed metadata.
func stripDirectives(text string) (stripped string, fifths int, timeNum, timeDen int) {
	fifths = 0   // default: C major
	timeNum = 4  // default: 4/4
	timeDen = 4

	re := regexp.MustCompile(`^(K\S+\s*)?(M\S+\s*)?`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return text, fifths, timeNum, timeDen
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
					// parsed OK
				}
			}
		}

		// Remove directives from text
		text = strings.TrimSpace(re.ReplaceAllString(text, ""))
	}

	return text, fifths, timeNum, timeDen
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

func resolveOctaves(events []Event) {
	lastPitch := 60 // C4

	for i := range events {
		ev := &events[i]
		if ev.Type == EventTupletStart || ev.Type == EventRest {
			continue
		}

		if ev.Type == EventNote {
			ev.Midi = resolvePitch(ev.Letter, ev.Accidental, ev.OctaveShift, lastPitch)
			lastPitch = ev.Midi
		} else if ev.Type == EventChord {
			chordRef := lastPitch
			for p := range ev.Pitches {
				pi := ev.Pitches[p]
				m := resolvePitch(pi.Letter, pi.Accidental, pi.OctaveShift, chordRef)
				ev.Midis = append(ev.Midis, m)
				chordRef = m
			}
			lastPitch = ev.Midis[len(ev.Midis)-1]
		}
	}
}

// --- Main parse entry point ---

// ParseDSL parses m4bon DSL text into a sequence of events.
// Key signature (K...) and meter (M...) directives are parsed from the DSL
// itself. Defaults: C major, 4/4.
func ParseDSL(text string) DSLResult {
	text, fifths, timeNum, timeDen := stripDirectives(text)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return DSLResult{Err: fmt.Errorf("no input")}
	}

	priorPitch := false
	var groups []ParseResult

	for _, tok := range tokens {
		if tok.Raw == "|" {
			continue
		}
		result := parseGroup(tok.Raw, priorPitch)
		if result.Err != nil {
			return DSLResult{Err: fmt.Errorf("group '%s': %w", tok.Raw, result.Err)}
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

	beat := ResolveBeatDuration(timeNum, timeDen)
	events, err := resolveDurations(groups, beat)
	if err != nil {
		return DSLResult{Err: err}
	}

	// Split notes that would cross the invisible barline (e.g. half note
	// starting on beat 2 in 4/4 must become two tied quarters).
	events = splitAtBarline(events, timeNum, timeDen)

	// Split any remaining non-standard durations into standard note values.
	events = splitNonStandardDurations(events)

	resolveOctaves(events)

	return DSLResult{
		Events:  events,
		Key:     KeySignature{Fifths: fifths},
		TimeNum: timeNum,
		TimeDen: timeDen,
	}
}
