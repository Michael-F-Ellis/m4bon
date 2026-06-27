// Package m4bon converts beat-oriented DSL text to MusicXML.
//
// Usage:
//
//	import "github.com/mellis/m4bon"
//
//	xml, err := m4bon.Compile("c d e f")
//	xml, err := m4bon.Compile("M6/8 abc def")
//	xml, err := m4bon.Compile("KE&\n(c) (-e) (-g)\n(-f) (d-) (b-)\n(ce) - -")
package m4bon

import (
	"fmt"

	"github.com/mellis/m4bon/musicxml"
	"github.com/mellis/m4bon/parser"
	"github.com/mellis/m4bon/render"
)

// Compile parses m4bon DSL text and returns the MusicXML output as a string.
// It accepts the same DSL syntax as the m4bon CLI tool.
func Compile(dsl string) (string, error) {
	lines := parser.SanitizeDSL(dsl)
	if len(lines) == 0 {
		return "", fmt.Errorf("empty DSL after sanitization")
	}
	result := parser.ParseDSL(lines)
	if result.Err != nil {
		return "", result.Err
	}
	if len(result.Measures) == 0 {
		return "", fmt.Errorf("no events produced")
	}
	return musicxml.Generate(result.Measures, result.Key.Fifths)
}

// Render parses m4bon DSL text and returns colorized text output
// in the FQS-inspired format: one measure per line with colored
// accidentals, octave subscripts, and chord overlines.
// Leap indicators use Unicode combining diacritics by default.
func Render(dsl string) (string, error) {
	return RenderOptions(dsl, false)
}

// RenderOptions parses DSL text and returns colorized output with
// configurable leap indicator rendering.
// asciiLeaps uses ANSI escapes (overline/underline) instead of
// Unicode combining diacritics for leap indicators.
func RenderOptions(dsl string, asciiLeaps bool) (string, error) {
	lines := parser.SanitizeDSL(dsl)
	if len(lines) == 0 {
		return "", fmt.Errorf("empty DSL after sanitization")
	}
	result := parser.ParseDSL(lines)
	if result.Err != nil {
		return "", result.Err
	}
	if len(result.Measures) == 0 {
		return "", fmt.Errorf("no events produced")
	}
	return render.Render(result.Measures, asciiLeaps, true), nil
}
