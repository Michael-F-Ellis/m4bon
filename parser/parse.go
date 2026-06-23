// Package parser converts m4bon DSL text into a sequence of musical events.
package parser

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/mellis/m4bon/frac"
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

// Fraction represents a rational number.
type Fraction = frac.Fraction

type EventType string

const (
	EventNote       EventType = "note"
	EventRest       EventType = "rest"
	EventChord      EventType = "chord"
	EventTupletStart EventType = "tupletStart"
)

type Event struct {
	Type              EventType
	Duration          Fraction
	Nominal           *Fraction // for tuplet notes: the nominal (display) duration
	Letter            string    // EventNote only
	Accidental        int       // EventNote only; raw accidental from DSL (0 if none)
	OctaveShift       int       // EventNote only
	ExplicitNatural   bool      // EventNote only; % was used; overrides key signature
	Pitches           []Pitch   // EventChord only
	Midi              int       // EventNote only; resolved MIDI pitch
	Midis             []int     // EventChord only; resolved MIDI pitches
	ResolvedOctave    int       // EventNote only; resolved octave (midi/12 convention)
	ResolvedOctaves   []int     // EventChord only; resolved octaves parallel to Midis
	EffAccidental     int       // effective accidental including measure-level persistence (for alter/render)
	Split             bool      // continuation from splitNonStandardDurations or barline split
	TieNext           bool      // cross-measure tie to next measure's first note
	Voice             int       // 1-based voice number (1,2,3 for voice-poly)
	GroupIdx          int       // original beat-group index, for render grouping
	NumSlots          int       // number of slot positions this event spans (for render)
	TupletActualNotes int       // EventTupletStart only
	TupletNormalNotes int       // EventTupletStart only
}

// NewNoteEvent creates a single-note event with the appropriate fields set.
func NewNoteEvent(letter string, accidental, octaveShift int, explicitNatural bool, dur Fraction, nominal *Fraction, voice, groupIdx int) Event {
	return Event{
		Type:            EventNote,
		Duration:        dur,
		Nominal:         nominal,
		Letter:          letter,
		Accidental:      accidental,
		EffAccidental:   accidental,
		OctaveShift:     octaveShift,
		ExplicitNatural: explicitNatural,
		Voice:           voice,
		GroupIdx:        groupIdx,
		NumSlots:        1,
	}
}

// NewChordEvent creates a chord event with the given pitches.
func NewChordEvent(pitches []Pitch, dur Fraction, nominal *Fraction, voice, groupIdx int) Event {
	return Event{
		Type:     EventChord,
		Duration: dur,
		Nominal:  nominal,
		Pitches:  pitches,
		Voice:    voice,
		GroupIdx: groupIdx,
		NumSlots: 1,
	}
}

// NewRestEvent creates a rest event.
func NewRestEvent(dur Fraction, nominal *Fraction, voice, groupIdx int) Event {
	return Event{
		Type:     EventRest,
		Duration: dur,
		Nominal:  nominal,
		Voice:    voice,
		GroupIdx: groupIdx,
		NumSlots: 1,
	}
}

// NewTupletStartEvent creates a tuplet bracket marker.
func NewTupletStartEvent(dur Fraction, groupIdx int, actualNotes, normalNotes int) Event {
	return Event{
		Type:              EventTupletStart,
		Duration:          dur,
		GroupIdx:          groupIdx,
		TupletActualNotes: actualNotes,
		TupletNormalNotes: normalNotes,
		Voice:             1,
	}
}

// Validate checks that the Event's fields are consistent with its Type.
func (e Event) Validate() error {
	switch e.Type {
	case EventNote:
		if e.Letter == "" {
			return fmt.Errorf("EventNote with empty Letter")
		}
		if e.Midi < 0 || e.Midi > 127 {
			return fmt.Errorf("EventNote Midi out of range: %d", e.Midi)
		}
		if len(e.Pitches) > 0 || len(e.Midis) > 0 {
			return fmt.Errorf("EventNote has Pitches or Midis set")
		}
	case EventChord:
		if len(e.Pitches) == 0 {
			return fmt.Errorf("EventChord with no Pitches")
		}
		if e.Letter != "" || e.Midi != 0 {
			return fmt.Errorf("EventChord has Letter or Midi set")
		}
	case EventRest:
		if e.Letter != "" || e.Pitches != nil || e.Midi != 0 {
			return fmt.Errorf("EventRest has note-related fields set")
		}
	case EventTupletStart:
		if e.Duration.Num <= 0 {
			return fmt.Errorf("EventTupletStart with non-positive duration")
		}
	default:
		return fmt.Errorf("unknown EventType: %s", e.Type)
	}
	if e.Voice < 0 {
		return fmt.Errorf("negative Voice: %d", e.Voice)
	}
	if e.GroupIdx < 0 {
		return fmt.Errorf("negative GroupIdx: %d", e.GroupIdx)
	}
	return nil
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
	Events     []Event
	TimeNum    int
	TimeDen    int
	Fifths     int
	IsPickup   bool
	NumGroups  int  // number of beat groups in the DSL input for this measure
	GroupSlots []int // number of slots per beat group (indexed by GroupIdx), for render
	GroupMults []int // beat multiplier per beat group (indexed by GroupIdx), for render

	// Chord symbols & lyrics (extracted from :H and :L directives)
	Chords    []string // one per beat group; nil/empty if no :H directive
	Lyrics    []string // one entry per active-note attack; nil/empty if no :L directive
	HasChords bool
	HasLyrics bool
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



// KeySignature represents a key signature via its position on the circle of fifths.
// Positive values = sharps, negative values = flats, zero = C major.
type KeySignature struct {
	Fifths int
}

// --- Helpers ---

// gcd returns the greatest common divisor of a and b.
// Deprecated: use frac.GCD
func gcd(a, b int) int {
	return frac.GCD(a, b)
}

// isPowerOf2 returns true if n is a power of two.
// Deprecated: use frac.IsPowerOf2
func isPowerOf2(n int) bool {
	return frac.IsPowerOf2(n)
}

// lowerPowerOf2 returns the largest power of two less than n.
// Deprecated: use frac.LowerPowerOf2
func lowerPowerOf2(n int) int {
	return frac.LowerPowerOf2(n)
}

// isStandardDuration returns true if the reduced fraction z/n is a standard duration.
// Deprecated: use frac.IsStandardDuration
func isStandardDuration(z, n int) bool {
	return frac.IsStandardDuration(z, n)
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

// SanitizeDSL strips comments and trims whitespace from DSL text.
// A comment is a line whose first non-whitespace character is '#' followed
// by whitespace (or just '#' alone). Bare '#c' is NOT treated as a comment.
func SanitizeDSL(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Check if line is a comment: "#" or "# text" (not "#c", "#&b", etc.)
		if strings.HasPrefix(trimmed, "#") && (len(trimmed) == 1 || trimmed[1] == ' ') {
			continue
		}
		lines = append(lines, trimmed)
	}
	return strings.Join(lines, " ")
}

func normalizePitchInput(text string) string {
	t := strings.ReplaceAll(text, "♯", "#")
	t = strings.ReplaceAll(t, "♭", "&")
	t = strings.ReplaceAll(t, "♮", "%")
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

		// Reject uppercase letters (must use lowercase for pitch names)
		if ch >= 'A' && ch <= 'G' {
			return err("uppercase notes not allowed — use lowercase", i)
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
