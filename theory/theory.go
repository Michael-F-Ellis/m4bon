// Package theory provides music theory functions for the m4bon pipeline.
package theory

// NoteOffsets maps pitch letter names to their chromatic offset within an octave.
var NoteOffsets = map[string]int{
	"c": 0, "d": 2, "e": 4, "f": 5, "g": 7, "a": 9, "b": 11,
}

// sharpOrder is the order in which sharps are added in key signatures.
var sharpOrder = []string{"f", "c", "g", "d", "a", "e", "b"}

// flatOrder is the order in which flats are added in key signatures.
var flatOrder = []string{"b", "e", "a", "d", "g", "c", "f"}

// FifthsToAccidentalMap builds a map from pitch letter to its key-signature accidental.
// fifths: circle of fifths position (positive=sharps, negative=flats).
func FifthsToAccidentalMap(fifths int) map[string]int {
	m := make(map[string]int)
	if fifths > 0 {
		for i := range min(fifths, len(sharpOrder)) {
			m[sharpOrder[i]] = 1
		}
	} else if fifths < 0 {
		n := min(-fifths, 7)
		for i := range n {
			m[flatOrder[i]] = -1
		}
	}
	return m
}

// EffectiveAccidental computes the accidental to use for pitch resolution:
//   - If ExplicitNatural, use 0 (natural)
//   - If explicit Accidental != 0, use that
//   - Otherwise check the key signature
//   - Default 0
func EffectiveAccidental(letter string, explicitAcc int, explicitNatural bool, keyAcc map[string]int) int {
	if explicitNatural {
		return 0
	}
	if explicitAcc != 0 {
		return explicitAcc
	}
	if acc, ok := keyAcc[letter]; ok {
		return acc
	}
	return 0
}
