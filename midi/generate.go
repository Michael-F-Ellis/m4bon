// Package midi converts parsed m4bon measures into Standard MIDI File bytes
// and a timeline of measure start times.
package midi

import (
	"fmt"
	"slices"
	"cmp"
	"strings"
	"time"

	"github.com/mellis/m4bon/frac"
	"github.com/mellis/m4bon/parser"
	"github.com/mellis/m4bon/theory"
	"gitlab.com/gomidi/midi/v2/smf"
)

// DPPQ (divisions per quarter note) matches the MusicXML generator.
const DPPQ = frac.DPPQ

// TicksPerWholeNote at the given DPPQ resolution.
const TicksPerWholeNote = frac.TicksPerWholeNote

// Timeline maps measure indices to their wall-clock start times at a given BPM.
type Timeline struct {
	MeasureStarts []time.Duration // index = measure number (0-based)
	TotalDuration time.Duration
	TempoBPM      float64
}

// timedEvent is a single MIDI event at an absolute tick position.
type timedEvent struct {
	tick int64
	msg  []byte
}

// voiceKey identifies a MIDI channel + pitch for pending NoteOff tracking.
type voiceKey struct {
	channel uint8
	pitch   uint8
}

func voiceToChannel(voice int) uint8 {
	switch voice {
	case 1:
		return 0 // MIDI channel 1
	case 2:
		return 1 // MIDI channel 2
	case 3:
		return 2 // MIDI channel 3
	default:
		return 0
	}
}

// metroChannel is the MIDI channel for GM percussion (channel 9, 0-indexed).
const metroChannel uint8 = 9

func extractPitches(ev parser.Event) []int {
	switch ev.Type {
	case parser.EventNote:
		return []int{ev.Midi}
	case parser.EventChord:
		return ev.Midis
	default:
		return nil
	}
}

// ticksToDuration converts absolute MIDI ticks to wall-clock time at the given BPM.
func ticksToDuration(ticks int64, bpm float64) time.Duration {
	seconds := float64(ticks) * 60.0 / (float64(DPPQ) * bpm)
	return time.Duration(seconds * float64(time.Second))
}

// buildTrackFromEvents sorts timedEvents by tick and produces an SMF track.
func buildTrackFromEvents(events []timedEvent) smf.Track {
	slices.SortFunc(events, func(a, b timedEvent) int {
		return cmp.Compare(a.tick, b.tick)
	})

	var track smf.Track
	var lastTick int64
	for _, e := range events {
		delta := uint32(0)
		if e.tick > lastTick {
			delta = uint32(e.tick - lastTick)
		}
		lastTick = e.tick
		track.Add(delta, e.msg)
	}
	return track
}

// SMFOptions controls which auxiliary tracks are included in the SMF.
type SMFOptions struct {
	Metronome bool // include metronome track (MIDI channel 9)
	Roots     bool // include chord root track (MIDI channel 8, bass range)
	Backbeats bool // metronome clicks only on even-numbered beats (1-based: 2, 4, 6...)
}

// rootChannel is the MIDI channel for chord roots (channel 8, 0-indexed).
const rootChannel uint8 = 8

// GenerateSMF produces a Standard MIDI File with metronome (backward compat).
func GenerateSMF(measures []parser.MeasureResult, bpm float64) ([]byte, Timeline, error) {
	return GenerateSMFWithOptions(measures, bpm, SMFOptions{Metronome: true, Roots: false})
}

// GenerateSMFWithOptions produces a Standard MIDI File with configurable tracks.
func GenerateSMFWithOptions(measures []parser.MeasureResult, bpm float64, opts SMFOptions) ([]byte, Timeline, error) {
	if bpm <= 0 {
		return nil, Timeline{}, fmt.Errorf("midi: BPM must be positive, got %f", bpm)
	}
	if len(measures) == 0 {
		return nil, Timeline{}, fmt.Errorf("midi: no measures to generate")
	}

	// Tempo map track
	tempoTrack := smf.Track{}
	tempoTrack.Add(0, smf.MetaTempo(bpm))

	// Score and metronome events collected with absolute ticks, then sorted later
	var scoreEvents []timedEvent
	var metroEvents []timedEvent
	var rootEvents []timedEvent

	pending := map[voiceKey]int64{}  // (ch,pitch) → tick at which to emit NoteOff
	voiceTick := map[int]int64{}      // per-voice tick accumulator

	measureStarts := make([]time.Duration, len(measures))
	var globalTick int64 // max tick across all voices at current point

	for mi, m := range measures {
		measureStarts[mi] = ticksToDuration(globalTick, bpm)

		// Time signature meta event
		if mi == 0 || m.TimeNum != measures[mi-1].TimeNum || m.TimeDen != measures[mi-1].TimeDen {
			tempoTrack.Add(uint32(globalTick), smf.MetaMeter(uint8(m.TimeNum), uint8(m.TimeDen)))
		}

		beat := parser.ResolveBeatDuration(m.TimeNum, m.TimeDen)
		beatTicks := int64(DPPQ * 4 * beat.Num / beat.Den)
		numBeats := m.TimeNum
		if m.TimeNum%3 == 0 {
			numBeats = m.TimeNum / 3
		}

		measureStartTick := globalTick

		for _, ev := range m.Events {
			if ev.Type == parser.EventTupletStart {
				continue
			}

			durTicks := int64(DPPQ * 4 * ev.Duration.Num / ev.Duration.Den)
			voice := ev.Voice

			if ev.Split {
				voiceTick[voice] += durTicks
				ch := voiceToChannel(voice)
				for _, p := range extractPitches(ev) {
					key := voiceKey{ch, uint8(p)}
					if pt, ok := pending[key]; ok {
						pending[key] = pt + durTicks
					}
				}
				if voiceTick[voice] > globalTick {
					globalTick = voiceTick[voice]
				}
				continue
			}

			if ev.Type == parser.EventRest {
				voiceTick[voice] += durTicks
				if voiceTick[voice] > globalTick {
					globalTick = voiceTick[voice]
				}
				continue
			}

			if ev.Type == parser.EventNote || ev.Type == parser.EventChord {
				ch := voiceToChannel(voice)
				atTick := voiceTick[voice]
				pitches := extractPitches(ev)

				// Emit pending NoteOffs for pitches whose time has come.
				// When pt == atTick, emit 1 tick early to avoid same-tick
				// collision with the incoming NoteOn (same pitch, same channel).
				for _, p := range pitches {
					key := voiceKey{ch, uint8(p)}
					if pt, ok := pending[key]; ok && pt <= atTick {
						offTick := pt
						if offTick == atTick && offTick > 0 {
							offTick--
						}
						scoreEvents = append(scoreEvents, timedEvent{offTick, []byte{0x80 | ch, uint8(p), 0}})
						delete(pending, key)
					}
				}

				// Emit NoteOn for pitches not still pending
				for _, p := range pitches {
					key := voiceKey{ch, uint8(p)}
					if _, ok := pending[key]; ok {
						continue
					}
					scoreEvents = append(scoreEvents, timedEvent{atTick, []byte{0x90 | ch, uint8(p), 90}})
					pending[key] = atTick + durTicks
				}

				voiceTick[voice] += durTicks
				if voiceTick[voice] > globalTick {
					globalTick = voiceTick[voice]
				}
			}
		}

		// Metronome clicks for this measure
		if opts.Metronome {
			for beatIdx := 0; beatIdx < numBeats; beatIdx++ {
				// Backbeats mode: skip odd-numbered beats in 0-based indexing
				// (beats 1, 3, 5... = 2, 4, 6... in 1-based)
				if opts.Backbeats && beatIdx%2 == 0 {
					continue
				}
				beatAbsTick := measureStartTick + int64(beatIdx)*beatTicks
				metroNote := uint8(77)
				metroVel := uint8(80)
				if !opts.Backbeats && beatIdx == 0 {
					metroNote = 76
					metroVel = 100
				}
				metroEvents = append(metroEvents,
					timedEvent{beatAbsTick, []byte{0x90 | metroChannel, metroNote, metroVel}},
					timedEvent{beatAbsTick, []byte{0x80 | metroChannel, metroNote, 0}},
				)
			}
		}

		// Chord roots for this measure (if enabled and chords present)
		if opts.Roots && len(m.Chords) > 0 {
			// Track sustain chains: chord roots sustain through "-" tokens.
			// Rests ";" produce silence and break the sustain chain.
			for beatIdx := 0; beatIdx < numBeats && beatIdx < len(m.Chords); beatIdx++ {
				raw := m.Chords[beatIdx]
				letter, acc := theory.ChordRoot(raw)
				if letter == "" {
					continue // sustain "-" or rest ";"
				}

				// Count consecutive sustain "-" tokens after this chord.
				// Stop at next letter, rest ";", end of chords, or end of measure.
				sustainBeats := 1
				for s := beatIdx + 1; s < numBeats && s < len(m.Chords); s++ {
					nextRaw := m.Chords[s]
					if nextRaw == "-" {
						sustainBeats++
					} else {
						break
					}
				}

				midi := chordRootMIDI(letter, acc)
				beatAbsTick := measureStartTick + int64(beatIdx)*beatTicks
				rootEvents = append(rootEvents,
					timedEvent{beatAbsTick, []byte{0x90 | rootChannel, uint8(midi), 90}},
				)
				rootEvents = append(rootEvents,
					timedEvent{beatAbsTick + int64(sustainBeats)*beatTicks, []byte{0x80 | rootChannel, uint8(midi), 0}},
				)
			}
		}
	}

	// Emit remaining pending NoteOffs
	for key, pt := range pending {
		scoreEvents = append(scoreEvents, timedEvent{pt, []byte{0x80 | key.channel, key.pitch, 0}})
	}

	// Build tracks from sorted events
	scoreTrack := buildTrackFromEvents(scoreEvents)
	scoreTrack.Close(0)
	tempoTrack.Close(uint32(globalTick))

	// Assemble SMF
	sf := smf.NewSMF1()
	sf.TimeFormat = smf.MetricTicks(DPPQ)
	if err := sf.Add(tempoTrack); err != nil {
		return nil, Timeline{}, fmt.Errorf("midi: add tempo track: %w", err)
	}
	if err := sf.Add(scoreTrack); err != nil {
		return nil, Timeline{}, fmt.Errorf("midi: add score track: %w", err)
	}
	if opts.Metronome {
		metroTrack := buildTrackFromEvents(metroEvents)
		metroTrack.Close(0)
		if err := sf.Add(metroTrack); err != nil {
			return nil, Timeline{}, fmt.Errorf("midi: add metronome track: %w", err)
		}
	}
	if opts.Roots && len(rootEvents) > 0 {
		// Program Change: Fingered Electric Bass (0-indexed program 33)
		rootEvents = append(rootEvents, timedEvent{0, []byte{0xC0 | rootChannel, 33}})
		rootTrack := buildTrackFromEvents(rootEvents)
		rootTrack.Close(0)
		if err := sf.Add(rootTrack); err != nil {
			return nil, Timeline{}, fmt.Errorf("midi: add root track: %w", err)
		}
	}

	data, err := sf.Bytes()
	if err != nil {
		return nil, Timeline{}, fmt.Errorf("midi: encode SMF: %w", err)
	}

	tl := Timeline{
		MeasureStarts: measureStarts,
		TotalDuration: ticksToDuration(globalTick, bpm),
		TempoBPM:      bpm,
	}

	return data, tl, nil
}

// chordRootMIDI maps a chord root letter+accidental to a MIDI note in bass range (E1-E2).
// Uses m4bon's octave convention where octave 5 = C4 (MIDI 60).
func chordRootMIDI(letter string, accidental int) int {
	// Start at octave 2 (C2 = MIDI 24, B2 = MIDI 35)
	midi := 2*12 + theory.NoteOffsets[strings.ToLower(letter)] + accidental
	// Shift to octave 3 if below E1 (MIDI 28)
	if midi < 28 {
		midi += 12
	}
	return midi
}

// GenerateMetronomeOnly produces an SMF with only metronome clicks.
func GenerateMetronomeOnly(measures []parser.MeasureResult, bpm float64) ([]byte, Timeline, error) {
	if bpm <= 0 {
		return nil, Timeline{}, fmt.Errorf("midi: BPM must be positive, got %f", bpm)
	}
	if len(measures) == 0 {
		return nil, Timeline{}, fmt.Errorf("midi: no measures")
	}

	tempoTrack := smf.Track{}
	tempoTrack.Add(0, smf.MetaTempo(bpm))

	var metroEvents []timedEvent
	measureStarts := make([]time.Duration, len(measures))
	var globalTick int64

	for mi, m := range measures {
		measureStarts[mi] = ticksToDuration(globalTick, bpm)

		if mi == 0 || m.TimeNum != measures[mi-1].TimeNum || m.TimeDen != measures[mi-1].TimeDen {
			tempoTrack.Add(uint32(globalTick), smf.MetaMeter(uint8(m.TimeNum), uint8(m.TimeDen)))
		}

		beat := parser.ResolveBeatDuration(m.TimeNum, m.TimeDen)
		beatTicks := int64(DPPQ * 4 * beat.Num / beat.Den)
		numBeats := m.TimeNum
		if m.TimeNum%3 == 0 {
			numBeats = m.TimeNum / 3
		}

		var measureTicks int64
		for _, ev := range m.Events {
			if ev.Type == parser.EventTupletStart {
				continue
			}
			measureTicks += int64(DPPQ * 4 * ev.Duration.Num / ev.Duration.Den)
		}
		if measureTicks == 0 {
			measureTicks = int64(TicksPerWholeNote * m.TimeNum / m.TimeDen)
		}

		for beatIdx := 0; beatIdx < numBeats; beatIdx++ {
			beatAbsTick := globalTick + int64(beatIdx)*beatTicks
			metroNote := uint8(77)
			metroVel := uint8(80)
			if beatIdx == 0 {
				metroNote = 76
				metroVel = 100
			}
			metroEvents = append(metroEvents,
				timedEvent{beatAbsTick, []byte{0x90 | metroChannel, metroNote, metroVel}},
				timedEvent{beatAbsTick, []byte{0x80 | metroChannel, metroNote, 0}},
			)
		}

		globalTick += measureTicks
	}

	metroTrack := buildTrackFromEvents(metroEvents)
	metroTrack.Close(0)
	tempoTrack.Close(uint32(globalTick))

	sf := smf.NewSMF1()
	sf.TimeFormat = smf.MetricTicks(DPPQ)
	if err := sf.Add(tempoTrack); err != nil {
		return nil, Timeline{}, fmt.Errorf("midi: add tempo track: %w", err)
	}
	if err := sf.Add(metroTrack); err != nil {
		return nil, Timeline{}, fmt.Errorf("midi: add metronome track: %w", err)
	}

	data, err := sf.Bytes()
	if err != nil {
		return nil, Timeline{}, fmt.Errorf("midi: encode SMF: %w", err)
	}

	tl := Timeline{
		MeasureStarts: measureStarts,
		TotalDuration: ticksToDuration(globalTick, bpm),
		TempoBPM:      bpm,
	}

	return data, tl, nil
}

// MidiEvent is a single MIDI event with absolute tick position, for JSON export.
type MidiEvent struct {
	Tick     int64   `json:"tick"`
	Type     string  `json:"type"`    // "noteOn", "noteOff", "metaTempo", "metaMeter"
	Channel  int     `json:"channel"` // 0-based MIDI channel
	Pitch    int     `json:"pitch"`   // MIDI note number
	Velocity int     `json:"velocity"` // note velocity (0–127); 0 on noteOff
	BPM      float64 `json:"bpm,omitempty"`
	Num      int     `json:"num,omitempty"`
	Den      int     `json:"den,omitempty"`
}

// EventListResult is the JSON-serializable result of GenerateEventList.
type EventListResult struct {
	Events        []MidiEvent `json:"events"`
	MeasureStarts []float64   `json:"measureStarts"` // in seconds
	TotalDuration int64       `json:"totalDuration"` // in ticks
	TempoBPM      float64     `json:"tempoBPM"`
}

// GenerateEventList produces a flat MIDI event list suitable for JSON export.
// Events are sorted by absolute tick. MeasureStarts are in seconds at the given BPM.
func GenerateEventList(measures []parser.MeasureResult, bpm float64, opts SMFOptions) (EventListResult, error) {
	if bpm <= 0 {
		return EventListResult{}, fmt.Errorf("midi: BPM must be positive, got %f", bpm)
	}
	if len(measures) == 0 {
		return EventListResult{}, fmt.Errorf("midi: no measures to generate")
	}

	var events []MidiEvent
	pending := map[voiceKey]int64{}
	voiceTick := map[int]int64{}

	measureStarts := make([]float64, len(measures))
	var globalTick int64

	for mi, m := range measures {
		tickToSec := 60.0 / (float64(DPPQ) * bpm)
		measureStarts[mi] = float64(globalTick) * tickToSec

		// Tempo meta event
		events = append(events, MidiEvent{Tick: globalTick, Type: "metaTempo", BPM: bpm})

		// Time signature meta event
		if mi == 0 || m.TimeNum != measures[mi-1].TimeNum || m.TimeDen != measures[mi-1].TimeDen {
			events = append(events, MidiEvent{
				Tick: globalTick, Type: "metaMeter",
				Num: m.TimeNum, Den: m.TimeDen,
			})
		}

		beat := parser.ResolveBeatDuration(m.TimeNum, m.TimeDen)
		beatTicks := int64(DPPQ * 4 * beat.Num / beat.Den)
		numBeats := m.TimeNum
		if m.TimeNum%3 == 0 {
			numBeats = m.TimeNum / 3
		}

		measureStartTick := globalTick

		for _, ev := range m.Events {
			if ev.Type == parser.EventTupletStart {
				continue
			}

			durTicks := int64(DPPQ * 4 * ev.Duration.Num / ev.Duration.Den)
			voice := ev.Voice

			if ev.Split {
				voiceTick[voice] += durTicks
				ch := voiceToChannel(voice)
				for _, p := range extractPitches(ev) {
					key := voiceKey{ch, uint8(p)}
					if pt, ok := pending[key]; ok {
						pending[key] = pt + durTicks
					}
				}
				if voiceTick[voice] > globalTick {
					globalTick = voiceTick[voice]
				}
				continue
			}

			if ev.Type == parser.EventRest {
				voiceTick[voice] += durTicks
				if voiceTick[voice] > globalTick {
					globalTick = voiceTick[voice]
				}
				continue
			}

			if ev.Type == parser.EventNote || ev.Type == parser.EventChord {
				ch := voiceToChannel(voice)
				atTick := voiceTick[voice]
				pitches := extractPitches(ev)

				for _, p := range pitches {
					key := voiceKey{ch, uint8(p)}
					if pt, ok := pending[key]; ok && pt <= atTick {
						offTick := pt
						if offTick == atTick && offTick > 0 {
							offTick--
						}
						events = append(events, MidiEvent{
							Tick:    offTick,
							Type:    "noteOff",
							Channel: int(ch),
							Pitch:   int(p),
						})
						delete(pending, key)
					}
				}

				for _, p := range pitches {
					key := voiceKey{ch, uint8(p)}
					if _, ok := pending[key]; ok {
						continue
					}
					events = append(events, MidiEvent{
						Tick:     atTick,
						Type:     "noteOn",
						Channel:  int(ch),
						Pitch:    int(p),
						Velocity: 90,
					})
					pending[key] = atTick + durTicks
				}

				voiceTick[voice] += durTicks
				if voiceTick[voice] > globalTick {
					globalTick = voiceTick[voice]
				}
			}
		}

		// Metronome clicks
		if opts.Metronome {
			for beatIdx := 0; beatIdx < numBeats; beatIdx++ {
				if opts.Backbeats && beatIdx%2 == 0 {
					continue
				}
				beatAbsTick := measureStartTick + int64(beatIdx)*beatTicks
				metroNote := 77
				metroVel := 80
				if !opts.Backbeats && beatIdx == 0 {
					metroNote = 76
					metroVel = 100
				}
				events = append(events,
					MidiEvent{Tick: beatAbsTick, Type: "noteOn", Channel: 9, Pitch: metroNote, Velocity: metroVel},
					MidiEvent{Tick: beatAbsTick, Type: "noteOff", Channel: 9, Pitch: metroNote},
				)
			}
		}

		// Chord roots
		if opts.Roots && len(m.Chords) > 0 {
			for beatIdx := 0; beatIdx < numBeats && beatIdx < len(m.Chords); beatIdx++ {
				raw := m.Chords[beatIdx]
				letter, acc := theory.ChordRoot(raw)
				if letter == "" {
					continue // sustain "-" or rest ";"
				}

				// Count consecutive sustain "-" tokens after this chord.
				sustainBeats := 1
				for s := beatIdx + 1; s < numBeats && s < len(m.Chords); s++ {
					if m.Chords[s] == "-" {
						sustainBeats++
					} else {
						break
					}
				}

				midiVal := chordRootMIDI(letter, acc)
				beatAbsTick := measureStartTick + int64(beatIdx)*beatTicks
				events = append(events,
					MidiEvent{Tick: beatAbsTick, Type: "noteOn", Channel: 8, Pitch: int(midiVal), Velocity: 90},
					MidiEvent{Tick: beatAbsTick + int64(sustainBeats)*beatTicks, Type: "noteOff", Channel: 8, Pitch: int(midiVal)},
				)
			}
		}
	}

	// Emit remaining pending NoteOffs
	for key, pt := range pending {
		events = append(events, MidiEvent{
			Tick:    pt,
			Type:    "noteOff",
			Channel: int(key.channel),
			Pitch:   int(key.pitch),
		})
	}

	slices.SortFunc(events, func(a, b MidiEvent) int {
		return cmp.Compare(a.Tick, b.Tick)
	})

	return EventListResult{
		Events:        events,
		MeasureStarts: measureStarts,
		TotalDuration: globalTick,
		TempoBPM:      bpm,
	}, nil
}
