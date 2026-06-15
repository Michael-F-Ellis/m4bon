// Package musicxml generates MusicXML output from parsed m4bon events.
package musicxml

import (
	"encoding/xml"
	"fmt"
	"math"
	"strings"

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
	Divisions int     `xml:"divisions"`
	Key       Key     `xml:"key"`
	Time      TimeSig `xml:"time"`
	Clef      Clef    `xml:"clef"`
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

type NoteEl struct {
	Chord             bool             `xml:"chord,omitempty"`
	Pitch             *PitchEl         `xml:"pitch,omitempty"`
	Rest              *RestEl          `xml:"rest,omitempty"`
	Duration          int              `xml:"duration"`
	Tie               []TieEl          `xml:"tie,omitempty"`
	Type              string           `xml:"type"`
	TimeModification  *TimeMod         `xml:"time-modification,omitempty"`
	Notations         *Notations       `xml:"notations,omitempty"`
	Voice             int              `xml:"voice,omitempty"`
	Staff             int              `xml:"staff,omitempty"`
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

type TiedEl struct {
	Type string `xml:"type,attr"`
}

type Notations struct {
	Tied []TiedEl `xml:"tied,omitempty"`
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

// DPPQ is divisions per quarter note. 480 is standard (MIDI ticks).
const DPPQ = 480

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

func gcd(a, b int) int {
	a = int(math.Abs(float64(a)))
	b = int(math.Abs(float64(b)))
	for b > 0 {
		a, b = b, a%b
	}
	return a
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

// minMeasureDuration returns the smallest note duration in the event list
// in ticks. This is used to determine whether we need a pickup measure.
func minMeasureDuration(events []parser.Event) int {
	min := int(^uint(0) >> 1) // max int
	for _, ev := range events {
		if ev.Type == parser.EventTupletStart {
			continue
		}
		d := durationToTicks(ev.Duration)
		if d < min {
			min = d
		}
	}
	if min == int(^uint(0)>>1) {
		return DPPQ // default quarter
	}
	return min
}

// totalDurationTicks returns the total duration of all events in ticks.
func totalDurationTicks(events []parser.Event) int {
	total := 0
	for _, ev := range events {
		if ev.Type == parser.EventTupletStart {
			continue
		}
		total += durationToTicks(ev.Duration)
	}
	return total
}

// Generate produces a MusicXML string from parsed events, time signature, and key signature.
func Generate(events []parser.Event, timeNum, timeDen int, fifths int) (string, error) {
	beatTicks := DPPQ * 4 / timeDen * timeNum // ticks per beat * beats per measure
	total := totalDurationTicks(events)

	// We'll place all notes in the first measure (or more if they overflow).
	// For simplicity: one measure with all notes, or split at measure boundaries.
	numMeasures := (total + beatTicks - 1) / beatTicks
	if numMeasures < 1 {
		numMeasures = 1
	}

	score := ScorePartwise{
		Version: "4.0",
		PartList: PartList{
			ScoreParts: []ScorePart{
				{ID: "P1", PartName: "Piano"},
			},
		},
	}

	var notes []NoteEl
	var inTuplet bool
	var tupletRatioNum, tupletRatioDen int

	for i, ev := range events {
		if ev.Type == parser.EventTupletStart {
			inTuplet = true
			tupletRatioNum = ev.Midi   // stored temporarily during parse
			tupletRatioDen = ev.OctaveShift
			continue
		}

		durTicks := durationToTicks(ev.Duration)
		noteType := noteTypeForDuration(ev.Duration)

		// For tuplet notes, also compute from nominal
		if ev.Nominal != nil {
			nt := noteTypeForDuration(*ev.Nominal)
			if nt != "" {
				noteType = nt
			}
		}

		// Determine ties
		var ties []TieEl
		var tieds []TiedEl
		if ev.Split {
			ties = append(ties, TieEl{Type: "stop"})
			tieds = append(tieds, TiedEl{Type: "stop"})
		}
		// Check if next event is a split continuation
		if i+1 < len(events) && events[i+1].Split &&
			ev.Type != parser.EventTupletStart {
			ties = append(ties, TieEl{Type: "start"})
			tieds = append(tieds, TiedEl{Type: "start"})
		}

		var notations *Notations
		if len(tieds) > 0 {
			notations = &Notations{Tied: tieds}
		}

		switch ev.Type {
		case parser.EventNote:
			step, oct, alter := midiToStep(ev.Midi)
			ne := NoteEl{
				Pitch:     &PitchEl{Step: step, Octave: oct, Alter: alter},
				Duration:  durTicks,
				Type:      noteType,
				Tie:       ties,
				Notations: notations,
				Voice:     1,
				Staff:     1,
			}
			if inTuplet {
				ne.TimeModification = &TimeMod{
					ActualNotes: fmt.Sprintf("%d", tupletRatioNum),
					NormalNotes: fmt.Sprintf("%d", tupletRatioDen),
				}
			}
			notes = append(notes, ne)
			if !ev.Split {
				inTuplet = false
			}

		case parser.EventRest:
			ne := NoteEl{
				Rest:     &RestEl{},
				Duration: durTicks,
				Type:     noteType,
				Voice:    1,
				Staff:    1,
			}
			notes = append(notes, ne)
			inTuplet = false

		case parser.EventChord:
			for pIdx, midi := range ev.Midis {
				step, oct, alter := midiToStep(midi)
				ne := NoteEl{
					Pitch:    &PitchEl{Step: step, Octave: oct, Alter: alter},
					Duration: durTicks,
					Type:     noteType,
					Chord:    pIdx > 0,
					Voice:    1,
					Staff:    1,
				}
				notes = append(notes, ne)
			}
			inTuplet = false
		}
	}

	// Place notes in measures
	measureNotes := make([][]NoteEl, numMeasures)
	curTick := 0
	for _, n := range notes {
		mIdx := curTick / beatTicks
		if mIdx >= numMeasures {
			mIdx = numMeasures - 1
		}
		measureNotes[mIdx] = append(measureNotes[mIdx], n)
		curTick += n.Duration
	}

	for m := 0; m < numMeasures; m++ {
		meas := Measure{Number: m + 1}

		if m == 0 {
			meas.Attributes = &Attributes{
				Divisions: DPPQ,
				Key:       Key{Fifths: fifths},
				Time: TimeSig{
					Beats:    fmt.Sprintf("%d", timeNum),
					BeatType: fmt.Sprintf("%d", timeDen),
				},
				Clef: Clef{Sign: "G", Line: 2},
			}
		}

		meas.Notes = measureNotes[m]
		score.Parts = append(score.Parts, Part{ID: "P1", Measures: []Measure{meas}})
	}

	// Marshal to XML
	output, err := xml.MarshalIndent(score, "", "  ")
	if err != nil {
		return "", fmt.Errorf("xml marshal: %w", err)
	}

	header := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!DOCTYPE score-partwise PUBLIC "-//Recordare//DTD MusicXML 4.0 Partwise//EN" "http://www.musicxml.org/dtds/partwise.dtd">` + "\n"

	return header + string(output), nil
}

// SanitizeDSL strips comments and trims whitespace from DSL text.
func SanitizeDSL(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, trimmed)
	}
	return strings.Join(lines, " ")
}
