// Package render converts m4bon Measures into a colorized, human-readable
// text format inspired by FQS (Fast Quality Scores). The output is one
// measure per line, with colored accidentals, octave subscripts, chord
// overlines, and measure numbers.
package render

// StyleClass identifies a color/style for a rendered glyph.
type StyleClass int

const (
	StyleDefault     StyleClass = iota // no color (natural pitch)
	StyleSharp                         // red    — rgb(209, 34, 34)
	StyleFlat                          // blue   — rgb(152, 140, 254)
	StyleDoubleSharp                   // orange — rgb(255, 165, 0)
	StyleDoubleFlat                    // green  — rgb(4, 182, 4)
	StyleSustainRest                   // grey   — rgb(160, 160, 160)
	StyleParen                         // medium-dark grey — rgb(120, 120, 120)
)

// Cell describes a single glyph to render.
type Cell struct {
	Content   string     // the character(s) to display (e.g. "c", "-", ";")
	Style     StyleClass // color/style classification
	Italic    bool       // render in italic (used for chord tones)
	Subscript string     // octave subscript text, empty if none (e.g. "4")
}

// CellSeq is a sequence of cells for one measure.
type CellSeq []Cell
