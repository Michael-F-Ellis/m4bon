// Package musicxml generates MusicXML output from parsed m4bon events.
package musicxml

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/mellis/m4bon/frac"
	"github.com/mellis/m4bon/parser"
)

// --- MusicXML types (only the subset we need) ---

type ScorePartwise struct {
	XMLName  xml.Name  `xml:"score-partwise"`
	Version  string    `xml:"version,attr"`
	PartList PartList  `xml:"part-list"`
	Parts    []Part    `xml:"part"`
}

type PartList struct {
	ScoreParts []ScorePart `xml:"score-part"`
}

type ScorePart struct {
	ID       string `xml:"id,attr"`
	PartName string `xml:"part-name"`
}

type Part struct {
	ID       string    `xml:"id,attr"`
	Measures []Measure `xml:"measure"`
}

type Measure struct {
	Number     int          `xml:"number,attr"`
	Attributes *Attributes  `xml:"attributes,omitempty"`
	Notes      []NoteEl     `xml:"note"`
	Direction  *Direction   `xml:"direction,omitempty"`
}

type Attributes struct {
	Divisions int      `xml:"divisions"`
	Key       Key      `xml:"key"`
	Time      TimeSig  `xml:"time"`
	Clef      *Clef    `xml:"clef,omitempty"`
}

type Key struct {
	Fifths int `xml:"fifths"`
}

type TimeSig struct {
	Beats    string `xml:"beats"`
	BeatType string `xml:"beat-type"`
}

type Clef struct {
	Sign string `xml:"sign"`
	Line int    `xml:"line"`
}

// NoteEl ...
type NoteEl struct {
	Chord             *struct{}        `xml:"chord,omitempty"`
	Pitch             *PitchEl         `xml:"pitch,omitempty"`
	Rest              *RestEl          `xml:"rest,omitempty"`
	Duration          int              `xml:"duration"`
	Tie               []TieEl          `xml:"tie,omitempty"`
	Type              string           `xml:"type"`
	Dots              []DotEl          `xml:"dot,omitempty"`
	Accidental        string           `xml:"accidental,omitempty"`
	TimeModification  *TimeMod         `xml:"time-modification,omitempty"`
	Beams             []BeamEl         `xml:"beam,omitempty"`
	Notations         *Notations       `xml:"notations,omitempty"`
	Voice             int              `xml:"voice,omitempty"`
	Staff             int              `xml:"staff,omitempty"`
}

type BeamEl struct {
	Number int    `xml:"number,attr,omitempty"`
	Value  string `xml:",chardata"`
}

type PitchEl struct {
	Step   string `xml:"step"`
	Octave int    `xml:"octave"`
	Alter  int    `xml:"alter,omitempty"`
}

type RestEl struct{}

type TieEl struct {
	Type string `xml:"type,attr"`
}

type DotEl struct{}

type TiedEl struct {
	Type string `xml:"type,attr"`
}

type TupletEl struct {
	Type   string `xml:"type,attr"`
	Number int    `xml:"number,attr"`
}

type Notations struct {
	Tied   []TiedEl   `xml:"tied,omitempty"`
	Tuplet []TupletEl `xml:"tuplet,omitempty"`
}

type TimeMod struct {
	ActualNotes string `xml:"actual-notes"`
	NormalNotes string `xml:"normal-notes"`
}

type Direction struct {
	DirectionType *DirectionType `xml:"direction-type"`
}

type DirectionType struct {
	Words string `xml:"words"`
}

// --- Generator ---

const DPPQ = frac.DPPQ

// noteTypeForDuration returns the MusicXML note-type name for a fraction of
// a whole note.  Returns "" for non-standard durations (tuplets).
func noteTypeForDuration(f parser.Fraction) string {
	n := f.Num
	d := f.Den
	g := gcd(n, d)
	n /= g
	d /= g

	// Dotted note: 3/den → dotted value
	if n == 3 {
		switch d {
		case 4:
			return "half"
		case 8:
			return "quarter"
		case 16:
			return "eighth"
		case 32:
			return "16th"
		case 64:
			return "32nd"
		case 128:
			return "64th"
		}
	}

	// Plain note: must be 1/den
	if n != 1 {
		return ""
	}

	switch d {
	case 1:
		return "whole"
	case 2:
		return "half"
	case 4:
		return "quarter"
	case 8:
		return "eighth"
	case 16:
		return "16th"
	case 32:
		return "32nd"
	case 64:
		return "64th"
	case 128:
		return "128th"
	}
	return ""
}

func isPowerOf2(n int) bool {
	return frac.IsPowerOf2(n)
}

// dotCount returns the number of augmentation dots for a duration fraction.
func dotCount(f parser.Fraction) int {
	n := f.Num
	d := f.Den
	g := gcd(n, d)
	n /= g
	d /= g
	if !isPowerOf2(d) {
		return 0
	}
	switch n {
	case 3:
		return 1
	case 7:
		return 2
	case 15:
		return 3
	}
	return 0
}

func gcd(a, b int) int {
	return frac.GCD(a, b)
}

// makeDots returns a slice of DotEl for the given dot count.
func makeDots(n int) []DotEl {
	if n <= 0 {
		return nil
	}
	return make([]DotEl, n)
}

// durationToTicks converts a fraction of a whole note to MIDI ticks (DPPQ-based).
func durationToTicks(f parser.Fraction) int {
	// duration = divisions * 4 * num / den
	// But we need integer result, so compute as: divs * 4 * num / den
	// Using DPPQ = 480, divs * 4 = 1920
	return (DPPQ * 4 * f.Num) / f.Den
}

// midiToStep converts a MIDI note number to MusicXML step + octave + alter.
func midiToStep(midi int) (step string, octave int, alter int) {
	// MIDI 60 = C4 in MusicXML
	octave = midi/12 - 1
	noteInOctave := midi % 12
	switch noteInOctave {
	case 0:
		return "C", octave, 0
	case 1:
		return "C", octave, 1
	case 2:
		return "D", octave, 0
	case 3:
		return "D", octave, 1
	case 4:
		return "E", octave, 0
	case 5:
		return "F", octave, 0
	case 6:
		return "F", octave, 1
	case 7:
		return "G", octave, 0
	case 8:
		return "G", octave, 1
	case 9:
		return "A", octave, 0
	case 10:
		return "A", octave, 1
	case 11:
		return "B", octave, 0
	}
	return "C", 4, 0
}

// accidentalString maps the parser's accidental value to a MusicXML accidental element value.
func accidentalString(acc int) string {
	switch acc {
	case 1:
		return "sharp"
	case -1:
		return "flat"
	case 2:
		return "double-sharp"
	case -2:
		return "flat-flat"
	}
	return ""
}

// Generate produces a MusicXML string from per-measure parsed events.
func Generate(measures []parser.MeasureResult, initialFifths int) (string, error) {
	score := ScorePartwise{
		Version: "4.0",
		PartList: PartList{
			ScoreParts: []ScorePart{
				{ID: "P1", PartName: "Piano"},
			},
		},
	}

	// State tracking across measures
	var prevTimeNum, prevTimeDen int
	var prevFifths int // 0 = C major (default)
	measCounter := 1
	var allMeasures []Measure

	for _, meas := range measures {
		measNum := measCounter
		if meas.IsPickup {
			measNum = 0
		} else {
			measCounter++
		}

		// Per-measure accidental tracking
		type pitchState struct {
			hasAccidental bool
		}
		pitchStates := make(map[string]*pitchState)

		// Per-measure beam tracking
		isBeamable := func(nt string) bool {
			switch nt {
			case "eighth", "16th", "32nd", "64th", "128th":
				return true
			}
			return false
		}

		// Pre-compute per-event tie and tuplet metadata (uses original event order)
		type eventMeta struct {
			hasTieStart   bool
			hasTieStop    bool
			hasTupStart   bool
			hasTupStop    bool
		}
		meta := make([]eventMeta, len(meas.Events))
		for i, ev := range meas.Events {
			if ev.Split {
				meta[i].hasTieStop = true
			}
			if i+1 < len(meas.Events) && meas.Events[i+1].Split &&
				ev.Type != parser.EventTupletStart {
				meta[i].hasTieStart = true
			}
			if ev.TieNext {
				meta[i].hasTieStart = true
			}
			if ev.Nominal != nil {
				isFirst := i > 0 && meas.Events[i-1].Type == parser.EventTupletStart
				isLast := i+1 >= len(meas.Events) || meas.Events[i+1].Nominal == nil
				meta[i].hasTupStart = isFirst
				meta[i].hasTupStop = isLast && !isFirst
			}
		}

		// Build tick-sorted note entries (interleave voices by onset time)
		type noteEntry struct {
			tick int
			note NoteEl
		}
		var entries []noteEntry
		voiceTick := make(map[int]int)
		var tupletRatioNum, tupletRatioDen int
		beatTicks := DPPQ * 4 / meas.TimeDen * meas.TimeNum

		for i, ev := range meas.Events {
			// Map event voice to MusicXML voice (1-based)
			voice := ev.Voice

			if ev.Type == parser.EventTupletStart {
				tupletRatioNum = ev.TupletActualNotes
				tupletRatioDen = ev.TupletNormalNotes
				continue
			}

			durTicks := durationToTicks(ev.Duration)
			noteType := noteTypeForDuration(ev.Duration)
			dotCount_ := dotCount(ev.Duration)

			if ev.Nominal != nil {
				nt := noteTypeForDuration(*ev.Nominal)
				if nt != "" {
					noteType = nt
				}
				dotCount_ = 0
			}

			// Ties from pre-computed metadata
			var ties []TieEl
			var tieds []TiedEl
			if meta[i].hasTieStop {
				ties = append(ties, TieEl{Type: "stop"})
				tieds = append(tieds, TiedEl{Type: "stop"})
			}
			if meta[i].hasTieStart {
				ties = append(ties, TieEl{Type: "start"})
				tieds = append(tieds, TiedEl{Type: "start"})
			}

			var notations *Notations
			if len(tieds) > 0 {
				notations = &Notations{Tied: tieds}
			}
			if meta[i].hasTupStart || meta[i].hasTupStop {
				if notations == nil {
					notations = &Notations{}
				}
				if meta[i].hasTupStart && !meta[i].hasTupStop {
					notations.Tuplet = []TupletEl{{Type: "start", Number: 1}}
				} else if meta[i].hasTupStop && !meta[i].hasTupStart {
					notations.Tuplet = []TupletEl{{Type: "stop", Number: 1}}
				} else if meta[i].hasTupStart && meta[i].hasTupStop {
					// Single tuplet note (start and stop)
					notations.Tuplet = []TupletEl{{Type: "start", Number: 1}}
				}
			}

			tick := voiceTick[voice]

			switch ev.Type {
			case parser.EventNote:
				letter := strings.ToUpper(ev.Letter)
				midiOct := ev.Midi / 12

				accidentalDisplay := accidentalString(ev.Accidental)
				if accidentalDisplay == "" && ev.Accidental == 0 {
					if ps, ok := pitchStates[letter]; ok && ps.hasAccidental {
						accidentalDisplay = "natural"
					}
				}
				if ev.Accidental != 0 {
					if pitchStates[letter] == nil {
						pitchStates[letter] = &pitchState{}
					}
					pitchStates[letter].hasAccidental = true
				}

				ne := NoteEl{
					Pitch:      &PitchEl{Step: letter, Octave: midiOct - 1, Alter: ev.Accidental},
					Duration:   durTicks,
					Type:       noteType,
					Dots:       makeDots(dotCount_),
					Accidental: accidentalDisplay,
					Tie:        ties,
					Notations:  notations,
					Voice:      voice,
					Staff:      1,
				}
				if ev.Nominal != nil {
					ne.TimeModification = &TimeMod{
						ActualNotes: fmt.Sprintf("%d", tupletRatioNum),
						NormalNotes: fmt.Sprintf("%d", tupletRatioDen),
					}
				}
				entries = append(entries, noteEntry{tick, ne})

			case parser.EventRest:
				ne := NoteEl{
					Rest:     &RestEl{},
					Duration: durTicks,
					Type:     noteType,
					Dots:     makeDots(dotCount_),
					Voice:    voice,
					Staff:    1,
				}
				entries = append(entries, noteEntry{tick, ne})

			case parser.EventChord:
				for pIdx, midi := range ev.Midis {
					step, oct, alter := midiToStep(midi)
					ne := NoteEl{
						Pitch:      &PitchEl{Step: step, Octave: oct, Alter: alter},
						Duration:   durTicks,
						Type:       noteType,
						Dots:       makeDots(dotCount_),
						Tie:        ties,
						Notations:  notations,
						Voice:      voice,
						Staff:      1,
					}
					if pIdx > 0 {
						ne.Chord = &struct{}{}
					}
					if pIdx == 0 && ev.Nominal != nil {
						ne.TimeModification = &TimeMod{
							ActualNotes: fmt.Sprintf("%d", tupletRatioNum),
							NormalNotes: fmt.Sprintf("%d", tupletRatioDen),
						}
					}
					entries = append(entries, noteEntry{tick, ne})
				}
			}

			voiceTick[voice] = tick + durTicks
		}

		// Sort entries by (tick, voice) for correct MusicXML onset order
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].tick != entries[j].tick {
				return entries[i].tick < entries[j].tick
			}
			return entries[i].note.Voice < entries[j].note.Voice
		})

		xmlNotes := make([]NoteEl, len(entries))
		for i, e := range entries {
			xmlNotes[i] = e.note
		}

		// Add beam elements — per-voice tick tracking so voices don't cross-beam
		beamVoiceTick := make(map[int]int)
		for i := range xmlNotes {
			v := xmlNotes[i].Voice
			tick := beamVoiceTick[v]
			nt := xmlNotes[i].Type
			if !isBeamable(nt) || xmlNotes[i].Chord != nil {
				beamVoiceTick[v] = tick + xmlNotes[i].Duration
				continue
			}

			beatPos := (tick % beatTicks) / (DPPQ * 4 / meas.TimeDen)

			var nextBeatSame bool
			if i+1 < len(xmlNotes) && xmlNotes[i+1].Voice == v {
				nextNT := xmlNotes[i+1].Type
				if isBeamable(nextNT) && xmlNotes[i+1].Chord == nil {
					nextTick := tick + xmlNotes[i].Duration
					nextBeatPos := (nextTick % beatTicks) / (DPPQ * 4 / meas.TimeDen)
					nextBeatSame = nextBeatPos == beatPos
				}
			}

			var prevBeatSame bool
			if i > 0 && xmlNotes[i-1].Voice == v {
				prevNT := xmlNotes[i-1].Type
				if isBeamable(prevNT) && xmlNotes[i-1].Chord == nil {
					prevStart := tick - xmlNotes[i].Duration
					prevBeatPos := (prevStart % beatTicks) / (DPPQ * 4 / meas.TimeDen)
					prevBeatSame = prevBeatPos == beatPos
				}
			}

			var beam string
			if !prevBeatSame && nextBeatSame {
				beam = "begin"
			} else if prevBeatSame && nextBeatSame {
				beam = "continue"
			} else if prevBeatSame && !nextBeatSame {
				beam = "end"
			}
			if beam != "" {
				xmlNotes[i].Beams = []BeamEl{{Number: 1, Value: beam}}
			}

			beamVoiceTick[v] = tick + xmlNotes[i].Duration
		}

		// Determine if attributes should be emitted
		var attrs *Attributes
		if measNum == 1 || measNum == 0 {
			attrs = &Attributes{
				Divisions: DPPQ,
				Key:       Key{Fifths: meas.Fifths},
				Time: TimeSig{
					Beats:    fmt.Sprintf("%d", meas.TimeNum),
					BeatType: fmt.Sprintf("%d", meas.TimeDen),
				},
				Clef: &Clef{Sign: "G", Line: 2},
			}
		} else if meas.TimeNum != prevTimeNum || meas.TimeDen != prevTimeDen || meas.Fifths != prevFifths {
			// Only emit the parts that changed. Omit clef (always G, never changes)
			// to avoid courtesy clef warnings from renderers.
			attrs = &Attributes{
				Divisions: DPPQ,
				Key:       Key{Fifths: meas.Fifths},
				Time: TimeSig{
					Beats:    fmt.Sprintf("%d", meas.TimeNum),
					BeatType: fmt.Sprintf("%d", meas.TimeDen),
				},
			}
		}

		measXML := Measure{
			Number:     measNum,
			Attributes: attrs,
			Notes:      xmlNotes,
		}

		allMeasures = append(allMeasures, measXML)

		prevTimeNum = meas.TimeNum
		prevTimeDen = meas.TimeDen
		prevFifths = meas.Fifths
	}

	score.Parts = append(score.Parts, Part{ID: "P1", Measures: allMeasures})

	output, err := xml.MarshalIndent(score, "", "  ")
	if err != nil {
		return "", fmt.Errorf("xml marshal: %w", err)
	}

	header := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!DOCTYPE score-partwise PUBLIC "-//Recordare//DTD MusicXML 4.0 Partwise//EN" "http://www.musicxml.org/dtds/partwise.dtd">` + "\n"

	return header + string(output), nil
}
