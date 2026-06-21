package render

import (
	"fmt"

	"github.com/mellis/m4bon/frac"
	"github.com/mellis/m4bon/parser"
	"github.com/mellis/m4bon/theory"
)

// TicksPerWholeNote is the tick resolution used for position grouping.
const TicksPerWholeNote = frac.TicksPerWholeNote

// Render produces the colorized text output for a sequence of measures.
// Returns one line per measure with ANSI escape codes for colors.
func Render(measures []parser.MeasureResult) string {
	cellMeasures := BuildCells(measures)
	return FormatANSI(cellMeasures)
}

// BuildCells converts measures into the intermediate Cell representation,
// one CellSeq per measure. The core rendering logic lives here — it is
// independent of any output format (terminal, HTML, etc.).
func BuildCells(measures []parser.MeasureResult) []CellSeq {
	result := make([]CellSeq, 0, len(measures))
	offset := 1
	if len(measures) > 0 && measures[0].IsPickup {
		offset = 0
	}
	for mi, m := range measures {
		cells := buildMeasureCells(m, mi+offset)
		result = append(result, cells)
	}
	return result
}

// eventGroup holds events that share the same beat-group index.
type eventGroup struct {
	idx    int
	events []parser.Event
}

// buildMeasureCells produces cells for a single measure, including the
// measure-number prefix.
func buildMeasureCells(m parser.MeasureResult, measureNum int) CellSeq {
	var cells CellSeq

	// Measure number prefix: "N:  "
	prefix := fmt.Sprintf("%d:  ", measureNum)
	cells = append(cells, Cell{Content: prefix, Style: StyleDefault})

	// Build key signature accidental map for this measure
	keyAcc := theory.FifthsToAccidentalMap(m.Fifths)

	// Group events by beat-group index
	groups := groupEventsByGroupIdx(m.Events)

	// Iterate over expected beat-group indices, filling in sustains for gaps
	gi := 0 // index into groups slice
	firstInMeasure := true
	for expectedIdx := 0; expectedIdx < m.NumGroups; expectedIdx++ {
		if expectedIdx > 0 {
			cells = append(cells, Cell{Content: " ", Style: StyleDefault})
		}

		// Determine slot count for this group (default 1 for safety)
		slotCount := 1
		if expectedIdx < len(m.GroupSlots) {
			slotCount = m.GroupSlots[expectedIdx]
		}

		if gi < len(groups) && groups[gi].idx == expectedIdx {
			// Count non-Split events to compute start-of-group sustains
			nonSplitCount := 0
			for _, ev := range groups[gi].events {
				if !ev.Split {
					nonSplitCount++
				}
			}
			totalSustains := slotCount - nonSplitCount
			intraGroupSustains := 0
			for _, ev := range groups[gi].events {
				if !ev.Split {
					intraGroupSustains += ev.NumSlots - 1
				}
			}
			startSustains := totalSustains - intraGroupSustains
			for s := 0; s < startSustains; s++ {
				cells = append(cells, Cell{Content: "-", Style: StyleSustainRest})
			}

			// Render events for this beat group, skipping notational ties
			for _, ev := range groups[gi].events {
				if ev.Split {
					continue // notational tie, not a user-visible sustain
				}
				eventCells := eventToCells(ev, keyAcc, firstInMeasure)
				cells = append(cells, eventCells...)
				if (ev.Type == parser.EventNote || ev.Type == parser.EventChord) && !ev.Split {
					firstInMeasure = false
				}
				// For events spanning multiple slots (intra-group sustains),
				// render a "-" for each absorbed slot position.
				for s := 1; s < ev.NumSlots; s++ {
					cells = append(cells, Cell{Content: "-", Style: StyleSustainRest})
				}
			}
			gi++
		} else {
			// Pure sustain group (no events produced) — render "-"
			cells = append(cells, Cell{Content: "-", Style: StyleSustainRest})
		}
	}

	// Newline at end of measure
	cells = append(cells, Cell{Content: "\n", Style: StyleDefault})
	return cells
}

// groupEventsByGroupIdx groups consecutive events by their original
// beat-group index (GroupIdx). Events from the same DSL beat group
// are rendered without spaces between them, preserving the DSL's
// rhythmic subdivision structure.
func groupEventsByGroupIdx(events []parser.Event) []eventGroup {
	var groups []eventGroup
	for _, ev := range events {
		if ev.Type == parser.EventTupletStart {
			continue
		}
		if ev.Type != parser.EventNote && ev.Type != parser.EventChord && ev.Type != parser.EventRest {
			continue
		}
		if len(groups) > 0 && groups[len(groups)-1].idx == ev.GroupIdx {
			groups[len(groups)-1].events = append(groups[len(groups)-1].events, ev)
		} else {
			groups = append(groups, eventGroup{idx: ev.GroupIdx, events: []parser.Event{ev}})
		}
	}
	return groups
}

// eventToCells converts a single event into one or more cells.
func eventToCells(ev parser.Event, keyAcc map[string]int, firstInMeasure bool) []Cell {
	switch ev.Type {
	case parser.EventRest:
		return []Cell{{Content: ";", Style: StyleSustainRest}}

	case parser.EventNote:
		if ev.Split {
			return []Cell{{Content: "-", Style: StyleSustainRest}}
		}
		style := noteStyle(ev.Letter, ev.EffAccidental, false, keyAcc)
		sub := octaveSubscript(ev.Midi, firstInMeasure || ev.OctaveShift != 0)
		return []Cell{{Content: ev.Letter, Style: style, Subscript: sub}}

	case parser.EventChord:
		if ev.Split {
			return []Cell{{Content: "-", Style: StyleSustainRest}}
		}
		var cells []Cell
		needSub := firstInMeasure
		if !needSub {
			for _, p := range ev.Pitches {
				if p.OctaveShift != 0 {
					needSub = true
					break
				}
			}
		}
		for i, p := range ev.Pitches {
			style := noteStyle(p.Letter, p.Accidental, p.ExplicitNatural, keyAcc)
			sub := ""
			if i == 0 && len(ev.Midis) > 0 {
				sub = octaveSubscript(ev.Midis[0], needSub)
			}
			if i == 0 {
				// Opening paren for chord group
				cells = append(cells, Cell{Content: "(", Style: StyleParen})
			}
			cells = append(cells, Cell{
				Content:   p.Letter,
				Style:     style,
				Italic:    true,
				Subscript: sub,
			})
			if i == len(ev.Pitches)-1 {
				// Closing paren for chord group
				cells = append(cells, Cell{Content: ")", Style: StyleParen})
			}
		}
		return cells
	}
	return nil
}

// noteStyle determines the style class for a pitch based on its effective
// accidental (key signature + explicit accidental + explicit natural).
func noteStyle(letter string, explicitAcc int, explicitNatural bool, keyAcc map[string]int) StyleClass {
	eff := theory.EffectiveAccidental(letter, explicitAcc, explicitNatural, keyAcc)
	switch eff {
	case 1:
		return StyleSharp
	case -1:
		return StyleFlat
	case 2:
		return StyleDoubleSharp
	case -2:
		return StyleDoubleFlat
	default:
		return StyleDefault
	}
}



// octaveSubscript returns the Unicode subscript string for the given MIDI
// pitch, or empty string if show is false or the octave is out of range.
func octaveSubscript(midi int, show bool) string {
	if !show {
		return ""
	}
	oct := midi/12 - 1
	if oct < 0 || oct > 9 {
		return ""
	}
	return subscriptDigit(oct)
}

// subscriptDigit returns the Unicode subscript character for a digit 0-9.
var subscriptDigits = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}

func subscriptDigit(d int) string {
	if d < 0 || d > 9 {
		return ""
	}
	return subscriptDigits[d]
}
