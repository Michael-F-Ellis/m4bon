package render

import (
	"strings"
	"testing"

	"github.com/mellis/m4bon/parser"
)

// parseDSL returns parsed measures for DSL input. Helper for FormatHTMLRows tests.
func parseDSL(t *testing.T, dsl string) parser.DSLResult {
	t.Helper()
	result := parser.ParseDSL(sanitizeLines(dsl))
	if result.Err != nil {
		t.Fatalf("ParseDSL(%q): %v", dsl, result.Err)
	}
	return result
}

func TestFormatHTML_BasicNotes(t *testing.T) {
	cells := rawCells(t, "M4/4 c d e f")
	html := FormatHTML([]CellSeq{cells}, false)

	if !strings.Contains(html, `<div class="m4bon-measure">`) {
		t.Error("expected measure div wrapper")
	}

	// First pitch (c) should have subscript
	if !strings.Contains(html, `<sub class="m4bon-octave">₄</sub>`) {
		t.Error("expected c₄ subscript")
	}

	// Should contain note letters
	if !strings.Contains(html, "c") || !strings.Contains(html, "d") ||
		!strings.Contains(html, "e") || !strings.Contains(html, "f") {
		t.Errorf("expected c,d,e,f in HTML")
	}
}

func TestFormatHTML_Accidentals(t *testing.T) {
	cells := rawCells(t, "#f &b %c")
	html := FormatHTML([]CellSeq{cells}, false)

	if !strings.Contains(html, "m4bon-sharp") {
		t.Error("expected sharp class for #f")
	}
	if !strings.Contains(html, "m4bon-flat") {
		t.Error("expected flat class for &b")
	}
	// %c should be natural (no color class)
}

func TestFormatHTML_Chord(t *testing.T) {
	cells := rawCells(t, "(ace)f")
	html := FormatHTML([]CellSeq{cells}, false)

	if !strings.Contains(html, "m4bon-italic") {
		t.Error("expected italic class for chord tones")
	}
	if !strings.Contains(html, "m4bon-paren") {
		t.Error("expected paren class for chord group")
	}
}

func TestFormatHTML_Sustains(t *testing.T) {
	cells := rawCells(t, "M4/4 a - -b c")
	html := FormatHTML([]CellSeq{cells}, false)

	if !strings.Contains(html, "m4bon-sustain-rest") {
		t.Error("expected sustain-rest class")
	}
	if !strings.Contains(html, "-") {
		t.Error("expected sustain dash")
	}
}

func TestFormatHTML_Leaps(t *testing.T) {
	cells := rawCells(t, "^c /d")
	html := FormatHTML([]CellSeq{cells}, false)

	if !strings.Contains(html, "&#x302;") {
		t.Error("expected upward leap diacritic")
	}
	if !strings.Contains(html, "&#x331;") {
		t.Error("expected downward leap diacritic")
	}
}

func TestFormatHTML_ASCIILeaps(t *testing.T) {
	cells := rawCells(t, "^c /d")
	html := FormatHTML([]CellSeq{cells}, true)

	if strings.Contains(html, "&#x302;") || strings.Contains(html, "&#x331;") {
		t.Error("unicode leaps should not appear in asciiLeaps mode")
	}
	if !strings.Contains(html, "m4bon-leap-up") {
		t.Error("expected leap-up class")
	}
	if !strings.Contains(html, "m4bon-leap-down") {
		t.Error("expected leap-down class")
	}
}

func TestFormatHTML_MultipleMeasures(t *testing.T) {
	cells := allMeasuresCells(t, "M4/4 c d e f | a b c d")
	html := FormatHTML(cells, false)

	count := strings.Count(html, `<div class="m4bon-measure">`)
	if count != 2 {
		t.Errorf("expected 2 measure divs, got %d", count)
	}
}

func TestFormatHTML_Empty(t *testing.T) {
	html := FormatHTML(nil, false)
	if html != "" {
		t.Errorf("expected empty string for nil, got %q", html)
	}
}

func TestFormatHTMLRows_Basic(t *testing.T) {
	result := parseDSL(t, "M4/4 c d e f")
	rows, maxCW, maxNW, maxLW := BuildRows(result.Measures, true)
	html := FormatHTMLRows(rows, maxCW, maxNW, maxLW, false)

	if !strings.Contains(html, `<span class="m4bon-note-col">`) {
		t.Error("expected note-col span")
	}
	if strings.Contains(html, "m4bon-chord-col") {
		t.Error("should not have chord-col when no :H directives")
	}
}

func TestFormatHTMLRows_WithChords(t *testing.T) {
	result := parseDSL(t, "M4/4 c d e f :H C - G7 -")
	rows, maxCW, maxNW, maxLW := BuildRows(result.Measures, true)
	html := FormatHTMLRows(rows, maxCW, maxNW, maxLW, false)

	if !strings.Contains(html, `<span class="m4bon-chord-col">`) {
		t.Error("expected chord-col span")
	}
}

func TestCellToHTML_Default(t *testing.T) {
	c := Cell{Content: "c", Style: StyleDefault}
	html := cellToHTML(c, false)
	if html != "c" {
		t.Errorf("expected plain 'c', got %q", html)
	}
}

func TestCellToHTML_Style(t *testing.T) {
	c := Cell{Content: "f", Style: StyleSharp}
	html := cellToHTML(c, false)
	if !strings.Contains(html, "m4bon-sharp") {
		t.Errorf("expected sharp class, got %q", html)
	}
}

func TestCellToHTML_Italic(t *testing.T) {
	c := Cell{Content: "c", Style: StyleDefault, Italic: true}
	html := cellToHTML(c, false)
	if !strings.Contains(html, "m4bon-italic") {
		t.Errorf("expected italic class, got %q", html)
	}
}

func TestCellToHTML_Subscript(t *testing.T) {
	c := Cell{Content: "c", Style: StyleDefault, Subscript: "₄"}
	html := cellToHTML(c, false)
	if !strings.Contains(html, `<sub class="m4bon-octave">₄</sub>`) {
		t.Errorf("expected subscript element, got %q", html)
	}
}

func TestCellToHTML_EscapesHTML(t *testing.T) {
	c := Cell{Content: "<script>", Style: StyleDefault}
	html := cellToHTML(c, false)
	if strings.Contains(html, "<script>") {
		t.Error("HTML should be escaped")
	}
}

func TestFormatHTML_MeasureNum(t *testing.T) {
	cells := rawCells(t, "M4/4 c d e f")
	html := FormatHTML([]CellSeq{cells}, false)

	if !strings.Contains(html, `<span class="m4bon-measure-num">`) {
		t.Error("expected measure-num span")
	}
	if !strings.Contains(html, `1:`) {
		t.Error("expected measure number '1:'")
	}
}

func TestFormatHTMLRows_MeasureDiv(t *testing.T) {
	result := parseDSL(t, "M4/4 c d e f | a b c d")
	rows, maxCW, maxNW, maxLW := BuildRows(result.Measures, true)
	html := FormatHTMLRows(rows, maxCW, maxNW, maxLW, false)

	count := strings.Count(html, `<div class="m4bon-measure">`)
	if count != 2 {
		t.Errorf("expected 2 measure divs, got %d", count)
	}
	if !strings.Contains(html, `<span class="m4bon-measure-num">1:`) {
		t.Error("expected measure number in FormatHTMLRows")
	}
}
