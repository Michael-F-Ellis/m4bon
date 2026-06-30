// Package theory provides music theory functions for the m4bon pipeline.
package theory

import (
	"strings"
)

// Unicode character constants for chord display.
const (
	ChrDelta   = "∆"  // U+2206 major 7th
	ChrHalfDim = "ø"  // U+00F8 half-diminished
	ChrDim     = "°"  // U+00B0 diminished
	ChrAug     = "⁺"  // U+207A augmented
	ChrMinus   = "⁻"  // U+207B minor
	ChrSup7    = "⁷"  // U+2077
	ChrSup9    = "⁹"  // U+2079
	ChrSup11   = "¹¹" // superscript 11
	ChrSup13   = "¹³" // superscript 13
	ChrSup6    = "⁶"  // U+2076
	ChrSup2    = "²"  // U+00B2
	ChrSup4    = "⁴"  // U+2074
	ChrSharp   = "♯"  // U+266F
	ChrFlat    = "♭"  // U+266D
)

// NormalizeChordSymbol converts a raw chord token to its display form.
// Sustain markers "-" and rest markers ";" are passed through unchanged.
// rootAcc: accidental for the root note (-1=flat, 0=none/natural, 1=sharp).
// bassAcc: accidental for the bass note in slash chords, 0 if no slash.
// Callers should style root and bass parts independently for slash chords.
func NormalizeChordSymbol(raw string) (display string, rootAcc int, bassAcc int) {
	if raw == "-" || raw == ";" {
		return raw, 0, 0
	}

	// Extract bass note if present (after /)
	bass := ""
	bassAcc = 0
	rest := raw
	if idx := strings.LastIndex(rest, "/"); idx >= 0 {
		bassPart := rest[idx+1:]
		rest = rest[:idx]
		if bp, ba := normalizeRoot(bassPart); bp != "" {
			bass = bp
			bassAcc = ba
		}
	}

	// Extract root and its accidental
	root, rootAcc := normalizeRoot(rest)
	if root == "" {
		return raw, 0, 0 // unrecognized, pass through
	}

	// Remove the root (including accidental prefix) from rest
	cleanRest := removeRoot(rest, root)

	// Parse quality and extension
	quality, ext := parseQualityAndExt(cleanRest)

	// Build display
	var sb strings.Builder
	sb.WriteString(root)
	sb.WriteString(quality)
	sb.WriteString(ext)
	if bass != "" {
		sb.WriteString("/")
		sb.WriteString(bass)
		if bassAcc != 0 {
			if bassAcc > 0 {
				sb.WriteString(ChrSharp)
			} else {
				sb.WriteString(ChrFlat)
			}
		}
	}

	return sb.String(), rootAcc, bassAcc
}

// normalizeRoot extracts the root letter and its accidental indicator.
// Accidental can be before the letter (#c) or after (c#). Both are supported.
// Returns the display letter (without accidental) and the accidental direction.
func normalizeRoot(raw string) (letter string, acc int) {
	raw = normalizePitchChars(raw) // now only # / &
	runes := []rune(raw)

	for i, ch := range runes {
		if ch == '#' || ch == '&' {
			continue
		}
		if (ch >= 'a' && ch <= 'g') || (ch >= 'A' && ch <= 'G') {
			letter = strings.ToUpper(string(ch))
			// Count accidentals before this letter
			for j := range i {
				switch runes[j] {
				case '#':
					acc++
				case '&':
					acc--
				}
			}
			// Check for accidental immediately after this letter
			if i+1 < len(runes) && (runes[i+1] == '#' || runes[i+1] == '&') {
				switch runes[i+1] {
				case '#':
					acc++
				case '&':
					acc--
				}
			}
			if acc > 1 {
				acc = 1
			}
			if acc < -1 {
				acc = -1
			}
			return letter, acc
		}
	}
	return "", acc
}

// removeRoot strips the root letter (and any accidental prefix/suffix) from raw.
func removeRoot(raw, root string) string {
	rawLower := strings.ToLower(raw)
	rootLower := strings.ToLower(root)
	runes := []rune(rawLower)

	// Find the root letter
	for i, ch := range runes {
		if string(ch) == rootLower {
			// Check if there's an accidental immediately after
			end := i + 1
			if end < len(runes) && (runes[end] == '#' || runes[end] == '&' || runes[end] == '♯' || runes[end] == '♭') {
				end++
			}
			// Convert rune index to byte index
			bytePos := 0
			for j := 0; j < end && j < len(runes); j++ {
				bytePos += len(string(runes[j]))
			}
			if bytePos < len(raw) {
				return string([]rune(raw)[end:])
			}
			return ""
		}
	}
	return raw
}

// parseQualityAndExt parses quality and extension from the remainder of a
// chord symbol after the root has been stripped.
func parseQualityAndExt(raw string) (quality string, ext string) {
	if raw == "" {
		return "", "" // major triad, no marking
	}

	rest := normalizePitchChars(raw)

	// Map of quality+extension to display
	switch {
	case rest == "m" || rest == "-" || rest == "min":
		return ChrMinus, "" // minor triad

	case rest == "dim" || rest == "\u00b0":
		return ChrDim, "" // diminished triad
	case rest == "dim7" || rest == "\u00b07":
		return ChrDim, ChrSup7 // diminished 7th
	case rest == "aug" || rest == "+":
		return ChrAug, "" // augmented triad

	case rest == "hdim" || rest == "\u00f8":
		return ChrHalfDim, ChrSup7
	case rest == "m7b5" || rest == "m7&5" || rest == "-7b5" || rest == "-7&5":
		return ChrMinus + ChrSup7 + ChrFlat + "⁵", ""

	case rest == "maj" || rest == "\u0394":
		return ChrDelta, ""
	case rest == "maj7" || rest == "\u03947" || rest == "\u0394\u2077":
		return ChrDelta, ChrSup7

	case rest == "7":
		return "", ChrSup7 // dominant 7th
	case rest == "m7" || rest == "-7":
		return ChrMinus, ChrSup7
	case rest == "maj9" || rest == "\u03949":
		return ChrDelta, ChrSup9
	case rest == "9":
		return "", ChrSup9
	case rest == "m9" || rest == "-9":
		return ChrMinus, ChrSup9
	case rest == "11":
		return "", ChrSup11
	case rest == "m11" || rest == "-11":
		return ChrMinus, ChrSup11
	case rest == "13":
		return "", ChrSup13
	case rest == "m13" || rest == "-13":
		return ChrMinus, ChrSup13
	case rest == "6":
		return "", ChrSup6
	case rest == "m6" || rest == "-6":
		return ChrMinus, ChrSup6

	case rest == "sus" || rest == "sus4":
		return "sus" + ChrSup4, ""
	case rest == "sus2":
		return "sus" + ChrSup2, ""

	case strings.HasPrefix(rest, "7#") || strings.HasPrefix(rest, "7&"):
		alt := rest[2:]
		if strings.HasPrefix(rest, "7#") {
			return "", ChrSup7 + ChrSharp + altDigitSuperscript(alt)
		}
		return "", ChrSup7 + ChrFlat + altDigitSuperscript(alt)
	case strings.HasPrefix(rest, "m7#") || strings.HasPrefix(rest, "m7&") ||
		strings.HasPrefix(rest, "-7#") || strings.HasPrefix(rest, "-7&"):
		var alt string
		if rest[0] == '-' || rest[0] == 'm' {
			alt = rest[3:]
		} else {
			alt = rest[2:]
		}
		if strings.Contains(rest, "#") {
			return ChrMinus, ChrSup7 + ChrSharp + altDigitSuperscript(alt)
		}
		return ChrMinus, ChrSup7 + ChrFlat + altDigitSuperscript(alt)
	}

	// Unrecognized — return rest as-is (could be a chord with no quality marking)
	return "", ""
}

// altDigitSuperscript converts a digit string to superscript form.
func altDigitSuperscript(d string) string {
	var sb strings.Builder
	for _, ch := range d {
		switch ch {
		case '9':
			sb.WriteString(ChrSup9)
		case '1':
			if sb.Len() > 0 {
				sb.WriteString("¹")
			} else {
				sb.WriteString("¹")
			}
		case '3':
			sb.WriteString("³")
		case '5':
			sb.WriteString("⁵")
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// normalizePitchChars replaces Unicode sharps/flats with ASCII equivalents.
func normalizePitchChars(raw string) string {
	raw = strings.ReplaceAll(raw, "♯", "#")
	raw = strings.ReplaceAll(raw, "♭", "&")
	raw = strings.ReplaceAll(raw, "♮", "%")
	return raw
}

// ChordRoot extracts the root letter and accidental from a chord symbol.
// Returns empty string for sustain "-" and rest ";" markers.
// letter is uppercase (C, D, E, F, G, A, B).
// accidental is 1 for sharp, -1 for flat, 0 for natural.
func ChordRoot(raw string) (letter string, accidental int) {
	if raw == "-" || raw == ";" {
		return "", 0
	}
	return normalizeRoot(raw)
}

// ValidateChordSymbol checks whether a raw chord token is syntactically valid.
// Returns "" if valid, or a descriptive error message.
func ValidateChordSymbol(raw string) string {
	if raw == "-" || raw == ";" {
		return "" // sustain/rest markers are always valid
	}
	if raw == "" {
		return "empty chord symbol"
	}

	// Check that root letter exists
	root, _ := normalizeRoot(raw)
	if root == "" {
		return "chord symbol has no recognizable root letter"
	}

	return "" // valid
}
