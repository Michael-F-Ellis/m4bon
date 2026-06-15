package parser

import (
	"fmt"
	"math"
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

	return splitNonStandardDurations(events), nil
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
// timeNum/timeDen specify the time signature (default 4/4).
func ParseDSL(text string, timeNum, timeDen int) DSLResult {
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

	resolveOctaves(events)

	return DSLResult{Events: events}
}
