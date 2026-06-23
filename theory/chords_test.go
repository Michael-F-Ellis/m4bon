package theory

import (
	"testing"
)

func TestNormalizeChordSymbol(t *testing.T) {
	tests := []struct {
		raw     string
		display string
		rootAcc int
		bassAcc int
	}{
		// Sustain/rest pass through
		{"-", "-", 0, 0},
		{";", ";", 0, 0},

		// Basic triads
		{"C", "C", 0, 0},
		{"c", "C", 0, 0},

		// Minor
		{"Cm", "C" + ChrMinus, 0, 0},
		{"C-", "C" + ChrMinus, 0, 0},
		{"Cmin", "C" + ChrMinus, 0, 0},

		// Diminished
		{"Cdim", "C" + ChrDim, 0, 0},
		{"C°", "C" + ChrDim, 0, 0},
		{"Cdim7", "C" + ChrDim + ChrSup7, 0, 0},

		// Half-diminished
		{"Chdim", "C" + ChrHalfDim + ChrSup7, 0, 0},
		{"Cø", "C" + ChrHalfDim + ChrSup7, 0, 0},
		{"Cm7b5", "C" + ChrMinus + ChrSup7 + ChrFlat + "⁵", 0, 0},
		{"Cm7♭5", "C" + ChrMinus + ChrSup7 + ChrFlat + "⁵", 0, 0},

		// Augmented
		{"Caug", "C" + ChrAug, 0, 0},
		{"C+", "C" + ChrAug, 0, 0},

		// Major 7th
		{"Cmaj7", "C" + ChrDelta + ChrSup7, 0, 0},
		{"CΔ", "C" + ChrDelta, 0, 0},
		{"CΔ7", "C" + ChrDelta + ChrSup7, 0, 0},

		// Dominant 7th
		{"C7", "C" + ChrSup7, 0, 0},

		// Minor 7th
		{"Cm7", "C" + ChrMinus + ChrSup7, 0, 0},
		{"C-7", "C" + ChrMinus + ChrSup7, 0, 0},

		// 9th chords
		{"C9", "C" + ChrSup9, 0, 0},
		{"Cm9", "C" + ChrMinus + ChrSup9, 0, 0},
		{"Cmaj9", "C" + ChrDelta + ChrSup9, 0, 0},

		// 11th chords
		{"C11", "C" + ChrSup11, 0, 0},

		// 13th chords
		{"C13", "C" + ChrSup13, 0, 0},

		// 6th chords
		{"C6", "C" + ChrSup6, 0, 0},

		// Suspended
		{"Csus", "Csus" + ChrSup4, 0, 0},
		{"Csus4", "Csus" + ChrSup4, 0, 0},
		{"Csus2", "Csus" + ChrSup2, 0, 0},

		// Alterations
		{"C7♭9", "C" + ChrSup7 + ChrFlat + ChrSup9, 0, 0},
		{"C7#9", "C" + ChrSup7 + ChrSharp + ChrSup9, 0, 0},
		{"C7♯9", "C" + ChrSup7 + ChrSharp + ChrSup9, 0, 0},
		{"C7♭13", "C" + ChrSup7 + ChrFlat + "¹³", 0, 0},
		{"C7♯11", "C" + ChrSup7 + ChrSharp + ChrSup11, 0, 0},
		{"Cm7♭13", "C" + ChrMinus + ChrSup7 + ChrFlat + "¹³", 0, 0},
		{"C-7♭13", "C" + ChrMinus + ChrSup7 + ChrFlat + "¹³", 0, 0},

		// Sharp root
		{"C♯", "C", 1, 0},
		{"C#", "C", 1, 0},

		// Flat root
		{"C♭", "C", -1, 0},
		{"C&", "C", -1, 0},

		// Bass notes
		{"Dm7/A", "D" + ChrMinus + ChrSup7 + "/A", 0, 0},
		{"Dm7/A♭", "D" + ChrMinus + ChrSup7 + "/A" + ChrFlat, 0, -1},
		{"D/F#", "D/F" + ChrSharp, 0, 1},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			display, rootAcc, bassAcc := NormalizeChordSymbol(tc.raw)
			if display != tc.display {
				t.Errorf("display: got %q, want %q", display, tc.display)
			}
			if rootAcc != tc.rootAcc {
				t.Errorf("rootAcc: got %d, want %d", rootAcc, tc.rootAcc)
			}
			if bassAcc != tc.bassAcc {
				t.Errorf("bassAcc: got %d, want %d", bassAcc, tc.bassAcc)
			}
		})
	}
}

func TestValidateChordSymbol(t *testing.T) {
	tests := []struct {
		raw    string
		errMsg string
	}{
		{"-", ""},
		{";", ""},
		{"C", ""},
		{"Cm7", ""},
		{"", "empty chord symbol"},
		{"#", "chord symbol has no recognizable root letter"},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			errStr := ValidateChordSymbol(tc.raw)
			if tc.errMsg == "" && errStr != "" {
				t.Errorf("expected no error, got %q", errStr)
			}
			if tc.errMsg != "" && errStr == "" {
				t.Errorf("expected error %q, got none", tc.errMsg)
			}
			if tc.errMsg != "" && errStr != "" && errStr != tc.errMsg {
				t.Errorf("expected error %q, got %q", tc.errMsg, errStr)
			}
		})
	}
}
