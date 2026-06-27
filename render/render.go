package render

import (
	"fmt"
	"strings"

	"github.com/mellis/m4bon/frac"
	"github.com/mellis/m4bon/parser"
	"github.com/mellis/m4bon/theory"
)

// TicksPerWholeNote is the tick resolution used for position grouping.
const TicksPerWholeNote = frac.TicksPerWholeNote

// Render produces the colorized text output for a sequence of measures.
// Returns one line per measure with ANSI escape codes for colors, in a
// three-column layout (CHORDS : NOTES : LYRICS) when chord or lyric
// directives are present.
func Render(measures []parser.MeasureResult, asciiLeaps bool, showSubscripts bool, showComments bool) string {
	rows, maxCW, maxNW, maxLW := BuildRows(measures, showSubscripts, showComments)
	return FormatANSIRows(rows, maxCW, maxNW, maxLW, asciiLeaps)
}

// BuildCells converts measures into the intermediate Cell representation,
// one CellSeq per measure. The core rendering logic lives here — it is
// independent of any output format (terminal, HTML, etc.).
func BuildCells(measures []parser.MeasureResult, showSubscripts bool, showComments bool) []CellSeq {
	result := make([]CellSeq, 0, len(measures))
	offset := 1
	if len(measures) > 0 && measures[0].IsPickup {
		offset = 0
	}
	for mi, m := range measures {
		commentCells, noteCells, trailingCells := buildMeasureCells(m, mi+offset, showSubscripts, showComments)
		cells := make(CellSeq, 0, len(commentCells)+len(noteCells)+len(trailingCells)+2)
		cells = append(cells, commentCells...)
		if len(commentCells) > 0 {
			cells = append(cells, Cell{Content: "\n", Style: StyleDefault})
		}
		cells = append(cells, noteCells...)
		if len(trailingCells) > 0 {
			cells = append(cells, Cell{Content: "\n", Style: StyleDefault})
			cells = append(cells, trailingCells...)
		}
		result = append(result, cells)
	}
	return result
}

// eventGroup holds events that share the same beat-group index.
type eventGroup struct {
	idx    int
	events []parser.Event
}

// buildMeasureCells produces cells for a single measure. Returns a
// separate comment block for any '!' comment preceding this measure.
func buildMeasureCells(m parser.MeasureResult, measureNum int, showSubscripts bool, showComments bool) (commentCells CellSeq, noteCells CellSeq, trailingCells CellSeq) {
	if showComments {
		for _, cl := range m.CommentLines {
			commentCells = append(commentCells, Cell{Content: "! ", Style: StyleComment, Italic: true})
			commentCells = append(commentCells, Cell{Content: cl, Style: StyleComment, Italic: true})
			commentCells = append(commentCells, Cell{Content: "\n", Style: StyleDefault})
		}
	}

	// Measure number prefix: "N:  "
	prefix := fmt.Sprintf("%d:  ", measureNum)
	noteCells = append(noteCells, Cell{Content: prefix, Style: StyleDefault})

	// Build key signature accidental map for this measure
	keyAcc := theory.FifthsToAccidentalMap(m.Fifths)

	// Group events by beat-group index
	groups := groupEventsByGroupIdx(m.Events)

	// Iterate over expected beat-group indices, filling in sustains for gaps
	gi := 0 // index into groups slice
	firstInMeasure := true
	for expectedIdx := 0; expectedIdx < m.NumGroups; expectedIdx++ {
		if expectedIdx > 0 {
			noteCells = append(noteCells, Cell{Content: " ", Style: StyleDefault})
		}

		// Determine slot count for this group (default 1 for safety)
		slotCount := 1
		if expectedIdx < len(m.GroupSlots) {
			slotCount = m.GroupSlots[expectedIdx]
		}

		if gi < len(groups) && groups[gi].idx == expectedIdx {
			// Prepend beat multiplier if > 1
			if expectedIdx < len(m.GroupMults) && m.GroupMults[expectedIdx] > 1 {
				noteCells = append(noteCells, Cell{Content: fmt.Sprintf("%d", m.GroupMults[expectedIdx]), Style: StyleSustainRest})
			}

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
				noteCells = append(noteCells, Cell{Content: "-", Style: StyleSustainRest})
			}

			// Render events for this beat group, skipping notational ties
			for _, ev := range groups[gi].events {
				if ev.Split {
					continue // notational tie, not a user-visible sustain
				}
				eventCells := eventToCells(ev, keyAcc, firstInMeasure, showSubscripts)
				noteCells = append(noteCells, eventCells...)
				if (ev.Type == parser.EventNote || ev.Type == parser.EventChord) && !ev.Split {
					firstInMeasure = false
				}
				// For events spanning multiple slots (intra-group sustains),
				// render a "-" for each absorbed slot position.
				for s := 1; s < ev.NumSlots; s++ {
					noteCells = append(noteCells, Cell{Content: "-", Style: StyleSustainRest})
				}
			}
			gi++
		} else {
			// Pure sustain group (no events produced) — render "-"
			noteCells = append(noteCells, Cell{Content: "-", Style: StyleSustainRest})
		}
	}

	// Newline at end of measure
	noteCells = append(noteCells, Cell{Content: "\n", Style: StyleDefault})

	// Trailing comment after the measure — separate from noteCells
	if showComments {
		for _, cl := range m.TrailingCommentLines {
			trailingCells = append(trailingCells, Cell{Content: "! ", Style: StyleComment, Italic: true})
			trailingCells = append(trailingCells, Cell{Content: cl, Style: StyleComment, Italic: true})
			trailingCells = append(trailingCells, Cell{Content: "\n", Style: StyleDefault})
		}
	}

	return commentCells, noteCells, trailingCells
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
// Leaps are indicated by OctaveShift: ^ → LeapUp, / → LeapDown.
func eventToCells(ev parser.Event, keyAcc map[string]int, firstInMeasure bool, showSubscripts bool) []Cell {
	switch ev.Type {
	case parser.EventRest:
		return []Cell{{Content: ";", Style: StyleSustainRest}}

	case parser.EventNote:
		if ev.Split {
			return []Cell{{Content: "-", Style: StyleSustainRest}}
		}
		style := noteStyle(ev.Letter, ev.EffAccidental, ev.ExplicitNatural, keyAcc)
		sub := octaveSubscript(ev.ResolvedOctave-1, showSubscripts && (firstInMeasure || ev.OctaveShift != 0))
		return []Cell{{Content: ev.Letter, Style: style, Subscript: sub, Leap: leapFromShift(ev.OctaveShift)}}

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
				sub = octaveSubscript(ev.ResolvedOctaves[0]-1, showSubscripts && needSub)
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
				Leap:      leapFromShift(p.OctaveShift),
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

// leapFromShift converts an OctaveShift value to a LeapDir.
// ^ (positive) → LeapUp, / (negative) → LeapDown, 0 → LeapNone.
func leapFromShift(shift int) LeapDir {
	if shift > 0 {
		return LeapUp
	}
	if shift < 0 {
		return LeapDown
	}
	return LeapNone
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



// octaveSubscript returns the Unicode subscript string for the given MusicXML
// octave, or empty string if show is false or the octave is out of range.
func octaveSubscript(mxlOctave int, show bool) string {
	if !show {
		return ""
	}
	if mxlOctave < 0 || mxlOctave > 9 {
		return ""
	}
	return subscriptDigit(mxlOctave)
}

// subscriptDigit returns the Unicode subscript character for a digit 0-9.
var subscriptDigits = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}

func subscriptDigit(d int) string {
	if d < 0 || d > 9 {
		return ""
	}
	return subscriptDigits[d]
}

// --- Three-Column Layout ---

// BuildRows converts measures into a three-column MeasureRow representation
// and computes the maximum visible widths for each column.
func BuildRows(measures []parser.MeasureResult, showSubscripts bool, showComments bool) (rows []MeasureRow, maxChordW, maxNoteW, maxLyricW int) {
	offset := 1
	if len(measures) > 0 && measures[0].IsPickup {
		offset = 0
	}

	anyChords := false
	anyLyrics := false
	for _, m := range measures {
		if m.HasChords {
			anyChords = true
		}
		if m.HasLyrics {
			anyLyrics = true
		}
	}

	for mi, m := range measures {
		var row MeasureRow

		if anyChords {
			row.ChordCells = buildChordCells(m)
		}
		row.CommentCells, row.NoteCells, row.TrailingCommentCells = buildMeasureCells(m, mi+offset, showSubscripts, showComments)
		// Strip trailing newline cell for column width computation
		row.NoteCells = stripTrailingNewline(row.NoteCells)
		if anyLyrics {
			row.LyricCells = buildLyricCells(m)
		}

		rows = append(rows, row)

		cw := visibleLen(row.ChordCells)
		if cw > maxChordW {
			maxChordW = cw
		}
		nw := visibleLen(row.NoteCells)
		if nw > maxNoteW {
			maxNoteW = nw
		}
		lw := visibleLen(row.LyricCells)
		if lw > maxLyricW {
			maxLyricW = lw
		}
	}
	return rows, maxChordW, maxNoteW, maxLyricW
}

// buildChordCells produces cells for the chord symbols of a measure.
func buildChordCells(m parser.MeasureResult) CellSeq {
	if !m.HasChords || len(m.Chords) == 0 {
		return nil
	}
	var cells CellSeq
	for i, raw := range m.Chords {
		if i > 0 {
			cells = append(cells, Cell{Content: " ", Style: StyleDefault})
		}
		if raw == "-" {
			cells = append(cells, Cell{Content: "-", Style: StyleSustainRest})
		} else if raw == ";" {
			cells = append(cells, Cell{Content: ";", Style: StyleSustainRest})
		} else {
			display, rootAcc, bassAcc := theory.NormalizeChordSymbol(raw)
			if bassAcc != 0 && strings.Contains(display, "/") {
				// Slash chord: style root and bass parts independently
				parts := strings.SplitN(display, "/", 2)
				// Root part
				rootStyle := chordStyleForAccidental(rootAcc)
				cells = append(cells, Cell{Content: parts[0], Style: rootStyle})
				cells = append(cells, Cell{Content: "/", Style: StyleDefault})
				bassStyle := chordStyleForAccidental(bassAcc)
				cells = append(cells, Cell{Content: parts[1], Style: bassStyle})
			} else {
				style := chordStyleForAccidental(rootAcc)
				cells = append(cells, Cell{Content: display, Style: style})
			}
		}
	}
	return cells
}

// chordStyleForAccidental returns a StyleClass for a chord's root accidental.
func chordStyleForAccidental(acc int) StyleClass {
	switch {
	case acc > 0:
		return StyleSharp
	case acc < 0:
		return StyleFlat
	default:
		return StyleDefault
	}
}

// buildLyricCells produces cells for the lyric syllables of a measure.
// Lyrics map 1:1 to positions in the measure (including rests and sustains).
func buildLyricCells(m parser.MeasureResult) CellSeq {
	if !m.HasLyrics || len(m.Lyrics) == 0 {
		return nil
	}

	var cells CellSeq
	for i, token := range m.Lyrics {
		if i > 0 {
			cells = append(cells, Cell{Content: " ", Style: StyleDefault})
		}
		if token == "-" {
			cells = append(cells, Cell{Content: "-", Style: StyleSustainRest})
		} else if token == ";" {
			cells = append(cells, Cell{Content: ";", Style: StyleSustainRest})
		} else if token == "*" || strings.Trim(token, "*") == "" {
			cells = append(cells, Cell{Content: "*", Style: StyleSustainRest})
		} else if strings.Contains(token, "_") {
			parts := strings.Split(token, "_")
			for pi, p := range parts {
				if pi > 0 {
					cells = append(cells, Cell{Content: "_", Style: StyleDefault})
				}
				cells = append(cells, Cell{Content: p, Style: StyleDefault})
			}
		} else {
			cells = append(cells, Cell{Content: token, Style: StyleDefault})
		}
	}
	return cells
}

// visibleLen returns the visible character length of a cell sequence,
// counting display width (number of Unicode code points) rather than bytes.
func visibleLen(cells CellSeq) int {
	n := 0
	for _, c := range cells {
		n += len([]rune(c.Content))
		if c.Subscript != "" {
			n += len([]rune(c.Subscript))
		}
	}
	return n
}

// stripTrailingNewline is defined in ansi.go
