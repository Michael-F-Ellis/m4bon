// Package parser converts m4bon DSL text into a sequence of musical events.
package parser

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// --- Types ---

type SlotType string

const (
	SlotNote    SlotType = "note"
	SlotSustain SlotType = "sustain"
	SlotRest    SlotType = "rest"
	SlotChord   SlotType = "chord"
)

type Slot struct {
	Type            SlotType
	Letter          string
	Accidental      int
	OctaveShift     int
	ExplicitNatural bool // % was used; overrides key signature
	Pitches         []Pitch     // for traditional chords (all notes, same voice)
	Entries         []ChordEntry // for voice-poly chords (mix of notes/sustains/rests)
}

type Pitch struct {
	Letter          string
	Accidental      int
	OctaveShift     int
	ExplicitNatural bool // % was used; overrides key signature
}

// ChordEntry represents a single entry in a voice-poly chord (one voice's
// contribution). A chord is voice-poly when any entry is a sustain or rest;
// otherwise it's a traditional single-voice chord.
type ChordEntry struct {
	Type            SlotType
	Letter          string
	Accidental      int
	OctaveShift     int
	ExplicitNatural bool // % was used; overrides key signature
}

type Fraction struct {
	Num int
	Den int
}

type EventType string

const (
	EventNote       EventType = "note"
	EventRest       EventType = "rest"
	EventChord      EventType = "chord"
	EventTupletStart EventType = "tupletStart"
)

type Event struct {
	Type            EventType
	Duration        Fraction
	Nominal         *Fraction // for tuplet notes
	Letter          string
	Accidental      int
	OctaveShift     int
	ExplicitNatural bool   // % was used; overrides key signature
	Pitches         []Pitch // for chords
	Midi            int     // resolved pitch for single notes
	Midis           []int   // resolved pitches for chords
	Split           bool    // continuation from splitNonStandardDurations
	TieNext         bool    // cross-measure tie to next measure's first note
	Voice           int     // 0=default, 1,2,3 for voice-poly voices
	GroupIdx        int     // original beat-group index, for render grouping
}

// ParseResult holds the output of parsing a single beat group.
type ParseResult struct {
	Multiplier int
	Slots      []Slot
	Err        error
	ErrOffset  int
}

// DSLResult holds the full parse output.
type DSLResult struct {
	Measures []MeasureResult
	Key      KeySignature // parsed or default
	TimeNum  int          // parsed or default (initial)
	TimeDen  int          // parsed or default (initial)
	Err      error
}

// MeasureResult holds the parsed events and metadata for a single measure.
type MeasureResult struct {
	Events    []Event
	TimeNum   int
	TimeDen   int
	Fifths    int
	IsPickup  bool
	NumGroups int // number of beat groups in the DSL input for this measure
}

// BeatDuration codes for B directive.
var BeatDurationCodes = map[string]BeatDuration{
	"W":  {1, 1},
	"H":  {1, 2},
	"Q":  {1, 4},
	"Q.": {3, 8},
	"E":  {1, 8},
	"E.": {3, 16},
	"S":  {1, 16},
	"T":  {1, 32},
}

// TicksPerWholeNote is the number of ticks in a whole note (DPPQ * 4).
const TicksPerWholeNote = 1920

// KeySignature represents a key signature via its position on the circle of fifths.
// Positive values = sharps, negative values = flats, zero = C major.
type KeySignature struct {
	Fifths int
}

// --- Helpers ---

func gcd(a, b int) int {
	a = int(math.Abs(float64(a)))
	b = int(math.Abs(float64(b)))
	for b > 0 {
		a, b = b, a%b
	}
	return a
}

func isPowerOf2(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}

func lowerPowerOf2(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p*2 < n {
		p *= 2
	}
	return p
}

func isStandardDuration(z, n int) bool {
	g := gcd(z, n)
	z /= g
	n /= g
	if !isPowerOf2(n) {
		return false
	}
	return z == 1 || z == 3
}

func countActivePositions(slots []Slot) int {
	n := 0
	for _, s := range slots {
		if s.Type != SlotSustain {
			n++
		}
	}
	return n
}

// --- Normalize ---

var accidentalReplacements = map[rune]string{
	'♯': "#",
	'♭': "&",
	'♮': "%",
}

func normalizePitchInput(text string) string {
	t := strings.ToLower(text)
	for r, s := range accidentalReplacements {
		t = strings.ReplaceAll(t, string(r), s)
	}
	return t
}

// --- Tokenize ---

type Token struct {
	Raw    string
	Offset int
}

func tokenize(text string) []Token {
	normalized := normalizePitchInput(text)
	var tokens []Token
	re := regexp.MustCompile(`\S+`)
	matches := re.FindAllStringSubmatchIndex(normalized, -1)
	for _, m := range matches {
		tokens = append(tokens, Token{Raw: normalized[m[0]:m[1]], Offset: m[0]})
	}
	return tokens
}

// --- Parse Group ---

func parseGroup(raw string, priorPitchExists bool) ParseResult {
	multiplier := 1
	var slots []Slot
	i := 0
	acc := 0
	pendingAcc := 0
	natural := false
	pendingNatural := false
	oct := 0
	pendingOct := 0
	hasLetter := false
	letter := ""
	inChord := false
	var chordPitches []ChordEntry

	err := func(msg string, offset int) ParseResult {
		return ParseResult{Multiplier: 1, Slots: nil, Err: errors.New(msg), ErrOffset: offset}
	}

	emitNote := func(offset int) *ParseResult {
		if inChord {
			if !hasLetter {
				r := err("accidental/octave without pitch in chord", offset)
				return &r
			}
			chordPitches = append(chordPitches, ChordEntry{Type: SlotNote, Letter: letter, Accidental: acc + pendingAcc, OctaveShift: oct + pendingOct, ExplicitNatural: natural || pendingNatural})
		} else {
			if !hasLetter {
				r := err("accidental/octave without pitch at end of group", offset)
				return &r
			}
			slots = append(slots, Slot{Type: SlotNote, Letter: letter, Accidental: acc + pendingAcc, OctaveShift: oct + pendingOct, ExplicitNatural: natural || pendingNatural})
		}
		acc = 0
		pendingAcc = 0
		natural = false
		pendingNatural = false
		oct = 0
		pendingOct = 0
		hasLetter = false
		letter = ""
		return nil
	}

	for i < len(raw) {
		ch := raw[i]

		if !inChord && i == 0 && ch >= '1' && ch <= '9' {
			multStart := i
			multiplier = 0
			for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
				multiplier = multiplier*10 + int(raw[i]-'0')
				i++
			}
			if multiplier == 0 {
				return err("beat multiplier cannot be zero", multStart)
			}
			continue
		}

		if ch >= '0' && ch <= '9' {
			return err("unexpected digit — multiplier must be at start", i)
		}

		switch ch {
		case '#':
			if hasLetter {
				pendingAcc++
			} else {
				acc++
			}
			i++
			continue
		case '&':
			if hasLetter {
				pendingAcc--
			} else {
				acc--
			}
			i++
			continue
		case '%':
			if hasLetter {
				// Natural: next note gets no accidental; current note is unaffected
				pendingAcc = 0
				pendingNatural = true
			} else {
				acc = 0
				natural = true
			}
			i++
			continue
		case '^':
			if hasLetter {
				pendingOct++
			} else {
				oct++
			}
			i++
			continue
		case '/':
			if hasLetter {
				pendingOct--
			} else {
				oct--
			}
			i++
			continue
		case '-':
			if inChord {
				if hasLetter {
					if r := emitNote(i); r != nil {
						return *r
					}
				}
				if len(chordPitches) == 0 && !priorPitchExists {
					return err("sustain with no prior note", i)
				}
				chordPitches = append(chordPitches, ChordEntry{Type: SlotSustain})
				i++
				continue
			}
			if hasLetter {
				if r := emitNote(i); r != nil {
					return *r
				}
			}
			if len(slots) == 0 && !priorPitchExists {
				return err("sustain with no prior note", i)
			}
			slots = append(slots, Slot{Type: SlotSustain})
			i++
			continue
		case ';':
			if inChord {
				if hasLetter {
					if r := emitNote(i); r != nil {
						return *r
					}
				}
				chordPitches = append(chordPitches, ChordEntry{Type: SlotRest})
				i++
				continue
			}
			if hasLetter {
				if r := emitNote(i); r != nil {
					return *r
				}
			}
			slots = append(slots, Slot{Type: SlotRest})
			i++
			continue
		case '(':
			if inChord {
				return err("nested chords not allowed", i)
			}
			if hasLetter {
				if r := emitNote(i); r != nil {
					return *r
				}
			}
			inChord = true
			chordPitches = nil
			i++
			continue
		case ')':
			if !inChord {
				return err("unmatched closing parenthesis", i)
			}
			if hasLetter {
				if r := emitNote(i); r != nil {
					return *r
				}
			}
			if len(chordPitches) == 0 {
				return err("empty chord", i)
			}
			// Detect voice-poly (any entry is sustain or rest) vs traditional chord
			isPoly := false
			for _, e := range chordPitches {
				if e.Type != SlotNote {
					isPoly = true
					break
				}
			}
			if isPoly {
				slots = append(slots, Slot{Type: SlotChord, Entries: chordPitches})
			} else {
				pitches := make([]Pitch, len(chordPitches))
				for i, e := range chordPitches {
					pitches[i] = Pitch{Letter: e.Letter, Accidental: e.Accidental, OctaveShift: e.OctaveShift, ExplicitNatural: e.ExplicitNatural}
				}
				slots = append(slots, Slot{Type: SlotChord, Pitches: pitches})
			}
			inChord = false
			chordPitches = nil
			i++
			continue
		}

		// Pitch letter
		lower := strings.ToLower(string(ch))
		if lower >= "a" && lower <= "g" {
			if hasLetter {
				// Accidentals and octave shifts between notes apply to the next note, not the current one.
				// Save pending values for the new note, emit without them, then transfer.
				nextAcc := pendingAcc
				nextOct := pendingOct
				nextNat := pendingNatural
				pendingAcc = 0
				pendingOct = 0
				pendingNatural = false
				if r := emitNote(i); r != nil {
					return *r
				}
				acc = nextAcc
				oct = nextOct
				natural = nextNat
			}
			letter = lower
			hasLetter = true
			i++
			continue
		}

		return err(fmt.Sprintf("unexpected character '%c'", ch), i)
	}

	if inChord {
		return err("unclosed chord", len(raw))
	}
	if hasLetter {
		if r := emitNote(i); r != nil {
			return *r
		}
	}
	if acc != 0 || oct != 0 {
		return err("bare accidental/octave at end of group", len(raw))
	}

	return ParseResult{Multiplier: multiplier, Slots: slots, Err: nil, ErrOffset: -1}
}
