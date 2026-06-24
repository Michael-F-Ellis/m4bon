package render

import (
	"fmt"
	"html"
	"strings"
)

// htmlClass returns the CSS class name for a StyleClass.
func htmlClass(s StyleClass) string {
	switch s {
	case StyleSharp:
		return "m4bon-sharp"
	case StyleFlat:
		return "m4bon-flat"
	case StyleDoubleSharp:
		return "m4bon-dbl-sharp"
	case StyleDoubleFlat:
		return "m4bon-dbl-flat"
	case StyleSustainRest:
		return "m4bon-sustain-rest"
	case StyleParen:
		return "m4bon-paren"
	default:
		return ""
	}
}

// FormatHTML converts a sequence of measure cell-sequences into an
// HTML string with CSS classes. Each measure becomes a <div>.
func FormatHTML(measures []CellSeq, asciiLeaps bool) string {
	var b strings.Builder
	for _, cells := range measures {
		b.WriteString(`<div class="m4bon-measure">`)
		for i, c := range cells {
			if i == 0 && strings.HasSuffix(c.Content, ":  ") {
				b.WriteString(`<span class="m4bon-measure-num">`)
				b.WriteString(cellToHTML(c, asciiLeaps))
				b.WriteString("</span>")
			} else {
				b.WriteString(cellToHTML(c, asciiLeaps))
			}
		}
		b.WriteString("</div>")
	}
	return b.String()
}

// FormatHTMLRows converts MeasureRows into an HTML string with
// three-column CSS table layout for cross-measure alignment.
func FormatHTMLRows(rows []MeasureRow, maxChordW, maxNoteW, maxLyricW int, asciiLeaps bool) string {
	var b strings.Builder
	b.WriteString(`<div class="m4bon-measure-table">`)
	b.WriteString(`<div class="m4bon-header-row">`)
	if maxChordW > 0 {
		b.WriteString(`<span class="m4bon-chord-col"></span>`)
	}
	b.WriteString(`<span class="m4bon-note-col"></span>`)
	if maxLyricW > 0 {
		b.WriteString(`<span class="m4bon-lyric-col"></span>`)
	}
	b.WriteString("</div>")

	for _, row := range rows {
		b.WriteString(`<div class="m4bon-measure">`)

		if maxChordW > 0 {
			b.WriteString(`<span class="m4bon-chord-col">`)
			for _, c := range row.ChordCells {
				b.WriteString(cellToHTML(c, asciiLeaps))
			}
			b.WriteString("</span>")
		}

		b.WriteString(`<span class="m4bon-note-col">`)
		noteCells := row.NoteCells
		if len(noteCells) > 0 && strings.HasSuffix(noteCells[0].Content, ":  ") {
			b.WriteString(`<span class="m4bon-measure-num">`)
			b.WriteString(cellToHTML(noteCells[0], asciiLeaps))
			b.WriteString("</span>")
			noteCells = noteCells[1:]
		}
		for _, c := range noteCells {
			b.WriteString(cellToHTML(c, asciiLeaps))
		}
		b.WriteString("</span>")

		if maxLyricW > 0 {
			b.WriteString(`<span class="m4bon-lyric-col">`)
			for _, c := range row.LyricCells {
				b.WriteString(cellToHTML(c, asciiLeaps))
			}
			b.WriteString("</span>")
		}

		b.WriteString("</div>")
	}
	b.WriteString("</div>")
	return b.String()
}

// cellToHTML converts a single Cell to an HTML snippet.
func cellToHTML(c Cell, asciiLeaps bool) string {
	baseContent := html.EscapeString(c.Content)
	subscript := html.EscapeString(c.Subscript)

	if subscript != "" {
		subscript = fmt.Sprintf(`<sub class="m4bon-octave">%s</sub>`, subscript)
	}

	var unicodeLeap string
	if !asciiLeaps {
		switch c.Leap {
		case LeapUp:
			unicodeLeap = "&#x302;"
		case LeapDown:
			unicodeLeap = "&#x331;"
		}
	}

	var classes []string
	if cls := htmlClass(c.Style); cls != "" {
		classes = append(classes, cls)
	}
	if c.Italic {
		classes = append(classes, "m4bon-italic")
	}

	var leapClass string
	if asciiLeaps {
		switch c.Leap {
		case LeapUp:
			leapClass = "m4bon-leap-up"
		case LeapDown:
			leapClass = "m4bon-leap-down"
		}
	}

	content := baseContent + unicodeLeap + subscript

	if len(classes) > 0 || leapClass != "" {
		allClasses := strings.Join(append(classes, leapClass), " ")
		if allClasses != "" {
			return fmt.Sprintf(`<span class="%s">%s</span>`, allClasses, content)
		}
	}

	return content
}
