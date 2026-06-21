package render

import (
	"strings"
	"testing"

	"github.com/mellis/m4bon/parser"
	"github.com/mellis/m4bon/theory"
)

// rawCells calls buildMeasureCells on parsed DSL and returns cells for the first measure.
func rawCells(t *testing.T, dsl string) CellSeq {
	t.Helper()
	r := parser.ParseDSL(dsl)
	if r.Err != nil {
		t.Fatalf("ParseDSL(%q): %v", dsl, r.Err)
	}
	if len(r.Measures) == 0 {
		t.Fatalf("no measures for %q", dsl)
	}
	return buildMeasureCells(r.Measures[0], 1, true)
}

// allMeasuresCells calls BuildCells on parsed DSL for all measures.
func allMeasuresCells(t *testing.T, dsl string) []CellSeq {
	t.Helper()
	r := parser.ParseDSL(dsl)
	if r.Err != nil {
		t.Fatalf("ParseDSL(%q): %v", dsl, r.Err)
	}
	return BuildCells(r.Measures, true)
}

// cellByContent finds the first cell with the given Content prefix.
func cellByContent(cells CellSeq, content string) *Cell {
	for i := range cells {
		if cells[i].Content == content {
			return &cells[i]
		}
	}
	return nil
}

// TestBasicNotes verifies plain quarter notes in 4/4.
func TestBasicNotes(t *testing.T) {
	cells := rawCells(t, "M4/4 c d e f")

	// Measure prefix
	if len(cells) == 0 || cells[0].Content != "1:  " {
		t.Errorf("expected prefix '1:  ', got %q", cells[0].Content)
	}

	// Check cells contain c, d, e, f in order
	contents := ""
	for _, c := range cells {
		contents += c.Content
	}
	if !strings.Contains(contents, "c") || !strings.Contains(contents, "d") ||
		!strings.Contains(contents, "e") || !strings.Contains(contents, "f") {
		t.Errorf("expected c,d,e,f in cells, got %q", contents)
	}

	// First pitch (c) should have subscript
	cCell := cellByContent(cells, "c")
	if cCell == nil {
		t.Fatal("no 'c' cell found")
	}
	if cCell.Subscript != "₄" {
		t.Errorf("expected subscript '₄' for c4, got %q", cCell.Subscript)
	}
	if cCell.Style != StyleDefault {
		t.Errorf("expected default style for c, got %v", cCell.Style)
	}

	// Subsequent pitches should not have subscript
	dCell := cellByContent(cells, "d")
	if dCell == nil {
		t.Fatal("no 'd' cell found")
	}
	if dCell.Subscript != "" {
		t.Errorf("expected no subscript for d, got %q", dCell.Subscript)
	}
}

// TestAccidentals verifies that explicit accidentals produce correct styles.
func TestAccidentals(t *testing.T) {
	cells := rawCells(t, "M4/4 #f &b %c")

	fCell := cellByContent(cells, "f")
	if fCell == nil {
		t.Fatal("no 'f' cell found")
	}
	if fCell.Style != StyleSharp {
		t.Errorf("expected StyleSharp for #f, got %v", fCell.Style)
	}

	bCell := cellByContent(cells, "b")
	if bCell == nil {
		t.Fatal("no 'b' cell found")
	}
	if bCell.Style != StyleFlat {
		t.Errorf("expected StyleFlat for &b, got %v", bCell.Style)
	}

	cCell := cellByContent(cells, "c")
	if cCell == nil {
		t.Fatal("no 'c' cell found")
	}
	if cCell.Style != StyleDefault {
		t.Errorf("expected StyleDefault for %%c, got %v", cCell.Style)
	}
}

// TestKeySignature checks that key signature alters cell styles.
func TestKeySignature(t *testing.T) {
	// E-flat major: Bb, Eb, Ab
	cells := rawCells(t, "KE& M4/4 e f g a")

	eCell := cellByContent(cells, "e")
	if eCell == nil {
		t.Fatal("no 'e' cell found")
	}
	if eCell.Style != StyleFlat {
		t.Errorf("expected StyleFlat for e (key of Eb), got %v", eCell.Style)
	}

	fCell := cellByContent(cells, "f")
	if fCell == nil {
		t.Fatal("no 'f' cell found")
	}
	if fCell.Style != StyleDefault {
		t.Errorf("expected StyleDefault for f (natural in Eb), got %v", fCell.Style)
	}

	aCell := cellByContent(cells, "a")
	if aCell == nil {
		t.Fatal("no 'a' cell found")
	}
	if aCell.Style != StyleFlat {
		t.Errorf("expected StyleFlat for a (flat in Eb), got %v", aCell.Style)
	}
}

// TestSustainChain verifies sustains render as grey '-' and are separate cells.
func TestSustainChain(t *testing.T) {
	cells := rawCells(t, "M4/4 a - -b c")

	// Find all '-' cells
	var dashCells []Cell
	for _, c := range cells {
		if c.Content == "-" {
			dashCells = append(dashCells, c)
		}
	}
	if len(dashCells) == 0 {
		t.Fatal("expected at least one '-' cell for sustain")
	}
	for _, dc := range dashCells {
		if dc.Style != StyleSustainRest {
			t.Errorf("expected StyleSustainRest for '-', got %v", dc.Style)
		}
	}

	// Verify the plain text output includes expected characters
	plain := FormatPlain([]CellSeq{cells})
	if !strings.Contains(plain, "a") || !strings.Contains(plain, "-") || !strings.Contains(plain, "b") || !strings.Contains(plain, "c") {
		t.Errorf("plain output should contain a, -, b, c; got %q", plain)
	}
}

// TestChord verifies chord pitches have parentheses, italic, and correct styles.
func TestChord(t *testing.T) {
	cells := rawCells(t, "M4/4 (ace)f")

	var italicCells []Cell
	for _, c := range cells {
		if c.Italic {
			italicCells = append(italicCells, c)
		}
	}
	if len(italicCells) != 3 {
		t.Errorf("expected 3 italic cells (a,c,e), got %d", len(italicCells))
	}

	// First chord tone should have subscript
	if len(italicCells) > 0 && italicCells[0].Subscript != "₃" {
		t.Errorf("expected subscript '₃' for A3, got %q", italicCells[0].Subscript)
	}

	// Should have parens around chord
	hasOpen := false
	hasClose := false
	for _, c := range cells {
		if c.Content == "(" && c.Style == StyleParen {
			hasOpen = true
		}
		if c.Content == ")" && c.Style == StyleParen {
			hasClose = true
		}
	}
	if !hasOpen {
		t.Errorf("expected opening paren for chord")
	}
	if !hasClose {
		t.Errorf("expected closing paren for chord")
	}

	// f should not be italic
	fCell := cellByContent(cells, "f")
	if fCell == nil {
		t.Fatal("no 'f' cell found")
	}
	if fCell.Italic {
		t.Errorf("f should not be italic")
	}
}

// TestOctaveSubscript verifies subscripts appear when octave shift is used.
func TestOctaveSubscript(t *testing.T) {
	cells := rawCells(t, "M4/4 c^c")

	// Find all pitch letter cells
	var pitchCells []Cell
	for _, c := range cells {
		if c.Content == "c" && c.Subscript != "" {
			pitchCells = append(pitchCells, c)
		}
	}
	// First c is first-in-measure, second c has OctaveShift=1, so both get subscripts
	if len(pitchCells) < 2 {
		t.Fatalf("expected 2 c cells with subscripts, got %d", len(pitchCells))
	}
	for i, pc := range pitchCells {
		if pc.Subscript == "" {
			t.Errorf("c cell %d should have subscript", i)
		}
	}
}

// TestOctaveSubscriptFirstOnly verifies subsequent pitches don't get subscripts.
func TestOctaveSubscriptFirstOnly(t *testing.T) {
	cells := rawCells(t, "M4/4 c d e f")

	// First pitch is c with subscript
	cCell := cellByContent(cells, "c")
	if cCell == nil || cCell.Subscript == "" {
		t.Error("expected subscript on first pitch c")
	}

	// d, e, f should have no subscript
	for _, letter := range []string{"d", "e", "f"} {
		cell := cellByContent(cells, letter)
		if cell == nil {
			t.Errorf("no cell for %s", letter)
			continue
		}
		if cell.Subscript != "" {
			t.Errorf("unexpected subscript on %s: %q", letter, cell.Subscript)
		}
	}
}

// TestDoubleAccidentals verifies double-sharp and double-flat styles.
func TestDoubleAccidentals(t *testing.T) {
	cells := rawCells(t, "M4/4 &&d ##c")

	dCell := cellByContent(cells, "d")
	if dCell == nil {
		t.Fatal("no 'd' cell found")
	}
	if dCell.Style != StyleDoubleFlat {
		t.Errorf("expected StyleDoubleFlat for &&d, got %v", dCell.Style)
	}

	cCell := cellByContent(cells, "c")
	if cCell == nil {
		t.Fatal("no 'c' cell found")
	}
	if cCell.Style != StyleDoubleSharp {
		t.Errorf("expected StyleDoubleSharp for ##c, got %v", cCell.Style)
	}
}

// TestMultiMeasure verifies multi-measure output.
func TestMultiMeasure(t *testing.T) {
	allCells := allMeasuresCells(t, "M4/4 c d e f | a b c d")
	if len(allCells) != 2 {
		t.Fatalf("expected 2 measures, got %d", len(allCells))
	}

	// First measure prefix
	if len(allCells[0]) == 0 || allCells[0][0].Content != "1:  " {
		t.Errorf("first measure: expected prefix '1:  ', got %q", allCells[0][0].Content)
	}
	// Second measure prefix
	if len(allCells[1]) == 0 || allCells[1][0].Content != "2:  " {
		t.Errorf("second measure: expected prefix '2:  ', got %q", allCells[1][0].Content)
	}

	// First measure starts with c (has subscript), second starts with a (has subscript)
	cCell := cellByContent(allCells[0], "c")
	if cCell == nil || cCell.Subscript == "" {
		t.Error("first measure: expected subscript on c")
	}
	aCell := cellByContent(allCells[1], "a")
	if aCell == nil || aCell.Subscript == "" {
		t.Error("second measure: expected subscript on a")
	}
}

// TestRest verifies rests render as ';' in grey.
func TestRest(t *testing.T) {
	cells := rawCells(t, "M4/4 c ; d e")

	semiCell := cellByContent(cells, ";")
	if semiCell == nil {
		t.Fatal("no ';' cell found")
	}
	if semiCell.Style != StyleSustainRest {
		t.Errorf("expected StyleSustainRest for ';', got %v", semiCell.Style)
	}
}

// TestPlainOutput verifies the plain-text formatted output for a simple case.
func TestPlainOutput(t *testing.T) {
	allCells := allMeasuresCells(t, "M4/4 c d e f")
	plain := FormatPlain(allCells)
	// Should contain "1:  c₄ d e f\n"
	if !strings.HasPrefix(plain, "1:  ") {
		t.Errorf("expected '1:  ' prefix, got %q", plain[:4])
	}
	if !strings.Contains(plain, "c₄") {
		t.Errorf("expected c₄ in plain output, got %q", plain)
	}
	if !strings.HasSuffix(plain, "\n") {
		t.Errorf("expected newline at end")
	}
}

// TestEffectiveAccidental tests the helper directly.
func TestEffectiveAccidental(t *testing.T) {
	// C major — no key alterations
	empty := map[string]int{}
	if eff := theory.EffectiveAccidental("c", 0, false, empty); eff != 0 {
		t.Errorf("c in C major should be 0, got %d", eff)
	}
	if eff := theory.EffectiveAccidental("f", 1, false, empty); eff != 1 {
		t.Errorf("#f should be 1, got %d", eff)
	}

	// G major — F# in key
	gMajor := map[string]int{"f": 1}
	if eff := theory.EffectiveAccidental("f", 0, false, gMajor); eff != 1 {
		t.Errorf("f in G major should be 1, got %d", eff)
	}
	if eff := theory.EffectiveAccidental("c", 0, false, gMajor); eff != 0 {
		t.Errorf("c in G major should be 0, got %d", eff)
	}
	// Explicit natural cancels key sig
	if eff := theory.EffectiveAccidental("f", 0, true, gMajor); eff != 0 {
		t.Errorf("%%f should cancel key sig, got %d", eff)
	}
}

// TestKeySigMap tests the key signature map builder.
func TestKeySigMap(t *testing.T) {
	// C major
	m := theory.FifthsToAccidentalMap(0)
	if len(m) != 0 {
		t.Errorf("C major should have empty map, got %v", m)
	}

	// G major (1 sharp)
	m = theory.FifthsToAccidentalMap(1)
	if m["f"] != 1 {
		t.Errorf("G major should sharpen F, got %v", m)
	}

	// D major (2 sharps)
	m = theory.FifthsToAccidentalMap(2)
	if m["f"] != 1 || m["c"] != 1 {
		t.Errorf("D major should sharpen F and C, got %v", m)
	}

	// F major (1 flat)
	m = theory.FifthsToAccidentalMap(-1)
	if m["b"] != -1 {
		t.Errorf("F major should flatten B, got %v", m)
	}

	// Bb major (2 flats)
	m = theory.FifthsToAccidentalMap(-2)
	if m["b"] != -1 || m["e"] != -1 {
		t.Errorf("Bb major should flatten B and E, got %v", m)
	}
}

// TestOctaveSubscriptHelper tests the subscript helper directly.
func TestOctaveSubscriptHelper(t *testing.T) {
	if s := octaveSubscript(4, true); s != "₄" {
		t.Errorf("octave 4 should give subscript '₄', got %q", s)
	}
	if s := octaveSubscript(4, false); s != "" {
		t.Errorf("show=false should give empty, got %q", s)
	}
	if s := octaveSubscript(-1, true); s != "" {
		t.Errorf("octave -1 below range should give empty, got %q", s)
	}
	if s := octaveSubscript(9, true); s != "₉" {
		t.Errorf("octave 9 should give '₉', got %q", s)
	}
}

// --- Leap indicator tests ---

// TestLeapFromShift verifies the leapFromShift helper.
func TestLeapFromShift(t *testing.T) {
	tests := []struct {
		shift int
		want  LeapDir
	}{
		{0, LeapNone},
		{1, LeapUp},
		{2, LeapUp},
		{-1, LeapDown},
		{-2, LeapDown},
	}
	for _, tt := range tests {
		got := leapFromShift(tt.shift)
		if got != tt.want {
			t.Errorf("leapFromShift(%d) = %v, want %v", tt.shift, got, tt.want)
		}
	}
}

// TestLeapUpwardOctave verifies ^ produces LeapUp.
func TestLeapUpwardOctave(t *testing.T) {
	cells := rawCells(t, "M4/4 c ^a")
	aCell := cellByContent(cells, "a")
	if aCell == nil {
		t.Fatal("no 'a' cell found")
	}
	if aCell.Leap != LeapUp {
		t.Errorf("c ^a should be LeapUp, got %v", aCell.Leap)
	}
}

// TestLeapDownwardOctave verifies / produces LeapDown.
func TestLeapDownwardOctave(t *testing.T) {
	cells := rawCells(t, "M4/4 a /c")
	cCell := cellByContent(cells, "c")
	if cCell == nil {
		t.Fatal("no 'c' cell found")
	}
	if cCell.Leap != LeapDown {
		t.Errorf("a /c should be LeapDown, got %v", cCell.Leap)
	}
}

// TestLeapNoOctaveShift verifies no leap without ^ or /.
func TestLeapNoOctaveShift(t *testing.T) {
	cells := rawCells(t, "M4/4 c d e f g a b")
	for _, letter := range []string{"d", "e", "f", "g", "a", "b"} {
		cell := cellByContent(cells, letter)
		if cell == nil {
			t.Errorf("no cell for %s", letter)
			continue
		}
		if cell.Leap != LeapNone {
			t.Errorf("%s should be LeapNone (no octave shift), got %v", letter, cell.Leap)
		}
	}
}

// TestLeapChordOctave verifies ^ and / inside chords produce leaps.
func TestLeapChordOctave(t *testing.T) {
	cells := rawCells(t, "M4/4 (c^ga)f")
	// c→^g = LeapUp, ^g→a = LeapNone
	gCell := cellByContent(cells, "g")
	if gCell == nil {
		t.Fatal("no 'g' cell found")
	}
	if gCell.Leap != LeapUp {
		t.Errorf("c → ^g in chord should be LeapUp, got %v", gCell.Leap)
	}
	aCell := cellByContent(cells, "a")
	if aCell == nil {
		t.Fatal("no 'a' cell found")
	}
	if aCell.Leap != LeapNone {
		t.Errorf("^g → a in chord should be LeapNone, got %v", aCell.Leap)
	}
}

// TestLeapChordFirstToneOctave verifies first chord tone with ^ is a leap.
func TestLeapChordFirstToneOctave(t *testing.T) {
	cells := rawCells(t, "M4/4 c (^ga)f")
	gCell := cellByContent(cells, "g")
	if gCell == nil {
		t.Fatal("no 'g' cell found")
	}
	if gCell.Leap != LeapUp {
		t.Errorf("c → ^g (first chord tone) should be LeapUp, got %v", gCell.Leap)
	}
}

// TestLeapFormatUnicode verifies that FormatANSI with asciiLeaps=false produces combining diacritics.
func TestLeapFormatUnicode(t *testing.T) {
	cells := rawCells(t, "M4/4 c ^a")
	aCell := cellByContent(cells, "a")
	if aCell == nil {
		t.Fatal("no 'a' cell found")
	}
	if aCell.Leap != LeapUp {
		t.Fatal("expected LeapUp for ^a")
	}

	ansi := FormatANSI([]CellSeq{cells}, false)
	if !strings.Contains(ansi, combiningCirc) {
		t.Errorf("expected combining circumflex in unicode output, got %q", ansi)
	}
}

// TestLeapFormatASCII verifies that FormatANSI with asciiLeaps=true produces ANSI escapes.
func TestLeapFormatASCII(t *testing.T) {
	cells := rawCells(t, "M4/4 c ^a")
	aCell := cellByContent(cells, "a")
	if aCell == nil {
		t.Fatal("no 'a' cell found")
	}
	if aCell.Leap != LeapUp {
		t.Fatal("expected LeapUp for ^a")
	}

	ansi := FormatANSI([]CellSeq{cells}, true)
	if !strings.Contains(ansi, ansiOverline) {
		t.Errorf("expected ANSI overline in ascii output, got %q", ansi)
	}
}

// TestLeapFormatNoLeapNoDiacritic verifies that cells without leaps produce no diacritics.
func TestLeapFormatNoLeapNoDiacritic(t *testing.T) {
	cells := rawCells(t, "M4/4 c d e f")
	ansi := FormatANSI([]CellSeq{cells}, false)
	if strings.Contains(ansi, combiningCirc) {
		t.Error("no leaps should mean no combining circumflexes")
	}
	if strings.Contains(ansi, combiningMacr) {
		t.Error("no leaps should mean no combining macrons")
	}
}

// TestRenderWithLeaps verifies the full Render pipeline with OctaveShift-based leaps.
func TestRenderWithLeaps(t *testing.T) {
	out := Render([]parser.MeasureResult{
		{TimeNum: 4, TimeDen: 4, Events: []parser.Event{
			{
				Type:           parser.EventNote,
				Letter:         "c",
				Midi:           60,
				ResolvedOctave: 5,
				Duration:       parser.Fraction{Num: 1, Den: 4},
				GroupIdx:       0,
				Voice:          1,
				NumSlots:       1,
				EffAccidental:  0,
			},
			{
				Type:           parser.EventNote,
				Letter:         "a",
				Midi:           69,
				ResolvedOctave: 5,
				OctaveShift:    1,
				Duration:       parser.Fraction{Num: 1, Den: 4},
				GroupIdx:       1,
				Voice:          1,
				NumSlots:       1,
				EffAccidental:  0,
			},
		}, NumGroups: 2, GroupSlots: []int{1, 1}},
	}, false, true)

	// ^a → LeapUp → combining circumflex
	if !strings.Contains(out, combiningCirc) {
		t.Errorf("expected combining circumflex for leap, got %q", out)
	}
}

// TestLeapMeasure3 verifies the reported bug: `&c/&e` — c has no shift, e has /.
func TestLeapMeasure3(t *testing.T) {
	// This is the third measure from the user's report
	// `&c/&e` — c should NOT be a leap, e (/&e) SHOULD be LeapDown
	cells := rawCells(t, "M4/4 &c/&e")

	cCell := cellByContent(cells, "c")
	if cCell == nil {
		t.Fatal("no 'c' cell found")
	}
	if cCell.Leap != LeapNone {
		t.Errorf("&c should be LeapNone (no octave shift), got %v", cCell.Leap)
	}

	eCell := cellByContent(cells, "e")
	if eCell == nil {
		t.Fatal("no 'e' cell found")
	}
	if eCell.Leap != LeapDown {
		t.Errorf("/&e should be LeapDown (octave down), got %v", eCell.Leap)
	}
}
