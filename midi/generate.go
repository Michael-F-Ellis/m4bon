// Package midi converts parsed m4bon measures into Standard MIDI File bytes
// and a timeline of measure start times. macOS-only via build constraint.
//
//go:build darwin && cgo
package midi

import (
	"fmt"
	"sort"
	"time"

	"github.com/mellis/m4bon/parser"
	"gitlab.com/gomidi/midi/v2/smf"
)

// DPPQ (divisions per quarter note) matches the MusicXML generator.
const DPPQ = 480

// TicksPerWholeNote at the given DPPQ resolution.
const TicksPerWholeNote = 1920

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
	case 0, 1:
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
	sort.Slice(events, func(i, j int) bool {
		return events[i].tick < events[j].tick
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

// GenerateSMF produces a Standard MIDI File from parsed measures.
func GenerateSMF(measures []parser.MeasureResult, bpm float64) ([]byte, Timeline, error) {
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

				// Emit pending NoteOffs for pitches whose time has come
				for _, p := range pitches {
					key := voiceKey{ch, uint8(p)}
					if pt, ok := pending[key]; ok && pt <= atTick {
						scoreEvents = append(scoreEvents, timedEvent{pt, []byte{0x80 | ch, uint8(p), 0}})
						delete(pending, key)
					}
				}

				// Emit NoteOn for pitches not still tied from previous measure
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
		for beatIdx := 0; beatIdx < numBeats; beatIdx++ {
			beatAbsTick := measureStartTick + int64(beatIdx)*beatTicks
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
	}

	// Emit remaining pending NoteOffs
	for key, pt := range pending {
		scoreEvents = append(scoreEvents, timedEvent{pt, []byte{0x80 | key.channel, key.pitch, 0}})
	}

	// Build tracks from sorted events
	scoreTrack := buildTrackFromEvents(scoreEvents)
	metroTrack := buildTrackFromEvents(metroEvents)

	scoreTrack.Close(0)
	metroTrack.Close(0)
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
