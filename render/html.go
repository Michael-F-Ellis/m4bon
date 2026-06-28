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
	case StyleComment:
		return "m4bon-comment"
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
// three-column layout for cross-measure alignment, using flexbox
// and explicit column widths so comment lines don't affect spacing.
func FormatHTMLRows(rows []MeasureRow, maxChordW, maxNoteW, maxLyricW int, asciiLeaps bool) string {
	var b strings.Builder
	b.WriteString(`<div class="m4bon-measures-list">`)

	for _, row := range rows {
		// Render comment block before measure
		if len(row.CommentCells) > 0 {
			b.WriteString(`<div class="m4bon-comment-line">`)
			writeCommentCells(&b, row.CommentCells, asciiLeaps)
			b.WriteString("</div>")
		}

		b.WriteString(`<div class="m4bon-measure">`)

		if maxChordW > 0 {
			b.WriteString(fmt.Sprintf(`<span class="m4bon-chord-col" style="min-width:%dch">`, maxChordW))
			for _, c := range row.ChordCells {
				b.WriteString(cellToHTML(c, asciiLeaps))
			}
			b.WriteString("</span>")
		}

		b.WriteString(fmt.Sprintf(`<span class="m4bon-note-col" style="min-width:%dch">`, maxNoteW))
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
			b.WriteString(fmt.Sprintf(`<span class="m4bon-lyric-col" style="min-width:%dch">`, maxLyricW))
			for _, c := range row.LyricCells {
				b.WriteString(cellToHTML(c, asciiLeaps))
			}
			b.WriteString("</span>")
		}

		b.WriteString("</div>")

		// Render trailing comment block after the measure
		if len(row.TrailingCommentCells) > 0 {
			b.WriteString(`<div class="m4bon-comment-line">`)
			writeCommentCells(&b, row.TrailingCommentCells, asciiLeaps)
			b.WriteString("</div>")
		}
	}
	b.WriteString("</div>")
	return b.String()
}

// writeCommentCells writes comment cells to the builder, converting
// newline cells to <br> for HTML line breaks.
func writeCommentCells(b *strings.Builder, cells CellSeq, asciiLeaps bool) {
	for _, c := range cells {
		if c.Content == "\n" {
			b.WriteString("<br>")
		} else {
			b.WriteString(cellToHTML(c, asciiLeaps))
		}
	}
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
	switch c.Leap {
	case LeapUp:
		leapClass = "m4bon-leap m4bon-leap-up"
	case LeapDown:
		leapClass = "m4bon-leap m4bon-leap-down"
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
