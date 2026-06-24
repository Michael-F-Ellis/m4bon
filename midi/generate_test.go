//go:build darwin && cgo

package midi

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/mellis/m4bon/parser"
	"gitlab.com/gomidi/midi/v2/smf"
)

func parseDSL(t *testing.T, dsl string) []parser.MeasureResult {
	t.Helper()
	result := parser.ParseDSL(dsl)
	if result.Err != nil {
		t.Fatalf("ParseDSL(%q): %v", dsl, result.Err)
	}
	return result.Measures
}

func TestGenerateSMF_BasicNotes(t *testing.T) {
	measures := parseDSL(t, "c d e f")
	data, tl, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GenerateSMF returned empty data")
	}
	// 4 quarter notes at 120 BPM = 2 seconds
	if tl.TempoBPM != 120 {
		t.Fatalf("TempoBPM = %f, want 120", tl.TempoBPM)
	}
	if tl.TotalDuration < time.Second || tl.TotalDuration > 3*time.Second {
		t.Fatalf("TotalDuration = %v, want ~2s", tl.TotalDuration)
	}
	if len(tl.MeasureStarts) != 1 {
		t.Fatalf("MeasureStarts len = %d, want 1", len(tl.MeasureStarts))
	}

	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	// Check note-on events (C=60, D=62, E=64, F=65)
	noteOns := filterEvents(events, "note_on", 0)
	if len(noteOns) < 4 {
		t.Fatalf("got %d note_on events on ch1, want >= 4", len(noteOns))
	}
	expected := []uint8{60, 62, 64, 65}
	for i, n := range noteOns[:4] {
		if n.Note != expected[i] {
			t.Errorf("note_on[%d] pitch = %d, want %d", i, n.Note, expected[i])
		}
	}

	// Check metronome: 4 clicks per measure in 4/4
	metroOns := filterEvents(events, "note_on", 9)
	if len(metroOns) < 4 {
		t.Fatalf("got %d metronome note_on events, want >= 4", len(metroOns))
	}
	if metroOns[0].Note != 76 {
		t.Errorf("downbeat note = %d, want 76", metroOns[0].Note)
	}
	for i := 1; i < 4; i++ {
		if metroOns[i].Note != 77 {
			t.Errorf("weak beat[%d] note = %d, want 77", i, metroOns[i].Note)
		}
	}
}

func TestGenerateSMF_WithRests(t *testing.T) {
	measures := parseDSL(t, "c ; d ;")
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}
	// 2 note_on events (c and d), rests produce no MIDI
	noteOns := filterEvents(events, "note_on", 0)
	if len(noteOns) < 2 {
		t.Fatalf("got %d note_on events, want 2", len(noteOns))
	}
}

func TestGenerateSMF_Sustains(t *testing.T) {
	// a-- in 4/4: a sustained across 3 position slots = 1 quarter note total
	// then b for a quarter
	measures := parseDSL(t, "a-- b")
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	noteOns := filterEvents(events, "note_on", 0)
	if len(noteOns) < 2 {
		t.Fatalf("got %d note_on events, want >= 2 (a and b)", len(noteOns))
	}
	// a with reference C4(60): closest A is A3=57 (diff=3) vs A4=69 (diff=9)
	if noteOns[0].Note != 57 {
		t.Errorf("first note_on pitch = %d, want 57 (A3)", noteOns[0].Note)
	}
	// b with reference 57: closest B is B3=59 (diff=2) vs B4=71 (diff=14)
	if noteOns[1].Note != 59 {
		t.Errorf("second note_on pitch = %d, want 59 (B3)", noteOns[1].Note)
	}

	// Exactly 2 NoteOff events on ch1 (one for a, one for b; split events suppressed)
	noteOffs := filterEvents(events, "note_off", 0)
	if len(noteOffs) < 2 {
		t.Fatalf("got %d note_off events, want 2", len(noteOffs))
	}
}

func TestGenerateSMF_CrossMeasureTie(t *testing.T) {
	measures := parseDSL(t, "a - | a b")
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	// note_on events: first measure a, second measure a+b = 3 total
	noteOns := filterEvents(events, "note_on", 0)
	// One a in measure 1, then when a continues in m2 it may get combined
	if len(noteOns) < 2 {
		t.Fatalf("got %d note_on events, want >= 2", len(noteOns))
	}
	// First note should be A3 (57)
	if noteOns[0].Note != 57 {
		t.Errorf("first note = %d, want 57 (A3)", noteOns[0].Note)
	}
	// Last note should be B3 (59)
	last := noteOns[len(noteOns)-1]
	if last.Note != 59 {
		t.Errorf("last note = %d, want 59 (B3)", last.Note)
	}
}

func TestGenerateSMF_Chords(t *testing.T) {
	measures := parseDSL(t, "(ceg) f")
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	noteOns := filterEvents(events, "note_on", 0)
	if len(noteOns) < 4 {
		t.Fatalf("got %d note_on events, want >= 4 (C,E,G + F)", len(noteOns))
	}
	// C=60, E=64, G=67 (with reference C4=60)
	if noteOns[0].Note != 60 {
		t.Errorf("first note = %d, want 60 (C)", noteOns[0].Note)
	}
	if noteOns[1].Note != 64 {
		t.Errorf("second note = %d, want 64 (E)", noteOns[1].Note)
	}
	if noteOns[2].Note != 67 {
		t.Errorf("third note = %d, want 67 (G)", noteOns[2].Note)
	}
}

func TestGenerateSMF_VoicePoly(t *testing.T) {
	// Traditional chord: (ce) is a chord with no spaces (C and E in voice 1).
	// The tokenizer splits on whitespace, so (c e) would be three tokens.
	measures := parseDSL(t, "(ce) f")
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	// Both notes on channel 1
	ch1NoteOns := filterEvents(events, "note_on", 0)
	if len(ch1NoteOns) < 3 {
		t.Fatalf("got %d note_on on ch1, want >= 3 (C,E + F)", len(ch1NoteOns))
	}
}

func TestGenerateSMF_Metronome(t *testing.T) {
	measures := parseDSL(t, "M4/4 c d e f")
	_, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
}

func TestGenerateSMF_CompoundMeter(t *testing.T) {
	measures := parseDSL(t, "M6/8 abc def")
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}
	// 6/8 compound → 2 metronome clicks (downbeat + weak)
	metroOns := filterEvents(events, "note_on", 9)
	if len(metroOns) < 2 {
		t.Fatalf("got %d metronome events in 6/8, want >= 2", len(metroOns))
	}
	if metroOns[0].Note != 76 {
		t.Errorf("downbeat note = %d, want 76", metroOns[0].Note)
	}
	if metroOns[1].Note != 77 {
		t.Errorf("weak beat note = %d, want 77", metroOns[1].Note)
	}
}

func TestGenerateSMF_Timeline(t *testing.T) {
	measures := parseDSL(t, "M4/4 c d e f | a b c d")
	data, tl, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	_ = data

	if len(tl.MeasureStarts) != 2 {
		t.Fatalf("MeasureStarts len = %d, want 2", len(tl.MeasureStarts))
	}
	if tl.MeasureStarts[0] != 0 {
		t.Errorf("MeasureStarts[0] = %v, want 0", tl.MeasureStarts[0])
	}
	// M1 = 4 quarter notes at 120 BPM = 2 seconds
	expected := 2 * time.Second
	got := tl.MeasureStarts[1]
	diff := got - expected
	if diff < -100*time.Millisecond || diff > 100*time.Millisecond {
		t.Errorf("MeasureStarts[1] = %v, want ~%v (diff=%v)", got, expected, diff)
	}
}

func TestGenerateSMF_Tuplets(t *testing.T) {
	measures := parseDSL(t, "M4/4 abc def")
	_, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
}

func TestGenerateSMF_EmptyMeasures(t *testing.T) {
	// Empty measure with no events — manually construct MeasureResult.
	measures := []parser.MeasureResult{
		{TimeNum: 4, TimeDen: 4, Events: nil, NumGroups: 0, Fifths: 0, GroupSlots: nil},
	}
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}
	// No note_on events on channel 1
	noteOns := filterEvents(events, "note_on", 0)
	if len(noteOns) != 0 {
		t.Errorf("got %d note_on events on ch1 for empty measure, want 0", len(noteOns))
	}
	// Metronome events on ch10
	metroOns := filterEvents(events, "note_on", 9)
	if len(metroOns) < 4 {
		t.Errorf("got %d metronome events, want >= 4", len(metroOns))
	}
}

func TestGenerateSMF_Tempo(t *testing.T) {
	measures := parseDSL(t, "c d e f")
	_, tl60, err := GenerateSMF(measures, 60)
	if err != nil {
		t.Fatalf("GenerateSMF(60 BPM): %v", err)
	}
	_, tl120, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF(120 BPM): %v", err)
	}
	// 60 BPM should be ~2x duration of 120 BPM
	ratio := float64(tl60.TotalDuration) / float64(tl120.TotalDuration)
	if ratio < 1.8 || ratio > 2.2 {
		t.Errorf("60 BPM duration = %v, 120 BPM = %v (ratio=%f, want ~2.0)", tl60.TotalDuration, tl120.TotalDuration, ratio)
	}
}

func TestGenerateSMF_ErrorZeroBPM(t *testing.T) {
	measures := parseDSL(t, "c d e f")
	_, _, err := GenerateSMF(measures, 0)
	if err == nil {
		t.Fatal("expected error for BPM=0, got nil")
	}
}

func TestGenerateMetronomeOnly(t *testing.T) {
	measures := parseDSL(t, "M4/4 c d e f")
	data, tl, err := GenerateMetronomeOnly(measures, 120)
	if err != nil {
		t.Fatalf("GenerateMetronomeOnly: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}
	// No notes on ch1
	ch1NoteOns := filterEvents(events, "note_on", 0)
	if len(ch1NoteOns) != 0 {
		t.Errorf("got %d note_on on ch1, want 0 (metronome only)", len(ch1NoteOns))
	}
	// Metronome events present
	metroOns := filterEvents(events, "note_on", 9)
	if len(metroOns) < 4 {
		t.Errorf("got %d metronome events, want >= 4", len(metroOns))
	}
	if len(tl.MeasureStarts) != 1 {
		t.Fatalf("MeasureStarts len = %d, want 1", len(tl.MeasureStarts))
	}
}

func TestGenerateSMF_RoundTrip(t *testing.T) {
	measures := parseDSL(t, "M4/4 c d e f | a b c d | e f g a")
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}

	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	// 3 measures × 4 beats × 4 notes per measure = 12 notes
	// Each note = note_on + note_off = 24 score events + metronome events
	ch1On := filterEvents(events, "note_on", 0)
	if len(ch1On) != 12 {
		t.Errorf("got %d note_on on ch1, want 12", len(ch1On))
	}

	// 3 measures × 4 beats = 12 metronome clicks
	ch10On := filterEvents(events, "note_on", 9)
	if len(ch10On) < 12 {
		t.Errorf("got %d note_on on ch10, want >= 12 (3 measures × 4 beats)", len(ch10On))
	}
}

func TestGenerateSMF_EmptyMeasuresNoNotes(t *testing.T) {
	// Two empty measures — manually construct MeasureResults.
	measures := []parser.MeasureResult{
		{TimeNum: 4, TimeDen: 4, Events: nil, NumGroups: 0, Fifths: 0, GroupSlots: nil},
		{TimeNum: 4, TimeDen: 4, Events: nil, NumGroups: 0, Fifths: 0, GroupSlots: nil},
	}
	data, _, err := GenerateSMF(measures, 120)
	if err != nil {
		t.Fatalf("GenerateSMF: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}
	metroOns := filterEvents(events, "note_on", 9)
	if len(metroOns) < 8 {
		t.Errorf("got %d metronome events for 2 empty measures, want >= 8", len(metroOns))
	}
}

func TestGenerateSMF_Options_MetronomeOff(t *testing.T) {
	measures := parseDSL(t, "c d e f")
	_, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Metronome: false})
	if err != nil {
		t.Fatalf("GenerateSMFWithOptions: %v", err)
	}
}

func TestGenerateSMF_Roots(t *testing.T) {
	measures := parseDSL(t, "M4/4 c d e f :H C - G7 - |")
	data, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Metronome: true, Roots: true})
	if err != nil {
		t.Fatalf("GenerateSMFWithOptions: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	// Verify events on channel 8 with correct pitches at beat positions
	ch8Ons := filterEvents(events, "note_on", 8)
	if len(ch8Ons) < 2 {
		t.Fatalf("got %d root note_on events on ch8, want >= 2", len(ch8Ons))
	}
	// C2 = MIDI 24, shifted to C3 = 36 (below E1 threshold 28)
	// G2 = MIDI 31 (stays, >= 28)
	expectedRoots := []uint8{36, 31}
	for i, e := range ch8Ons[:len(expectedRoots)] {
		if e.Note != expectedRoots[i] {
			t.Errorf("root[%d] note = %d, want %d", i, e.Note, expectedRoots[i])
		}
	}

	// Verify metronome also present
	metroOns := filterEvents(events, "note_on", 9)
	if len(metroOns) < 4 {
		t.Fatalf("got %d metronome events, want >= 4", len(metroOns))
	}

	// Verify score notes on ch1
	ch1Ons := filterEvents(events, "note_on", 0)
	if len(ch1Ons) < 4 {
		t.Fatalf("got %d note_on events on ch1, want >= 4", len(ch1Ons))
	}

	// Verify program change on root track (channel 8, Fingered Electric Bass = 33)
	pcEvents := filterEvents(events, "program_change", 8)
	if len(pcEvents) != 1 {
		t.Fatalf("got %d program_change events on ch8, want 1", len(pcEvents))
	}
	if pcEvents[0].Program != 33 {
		t.Errorf("program_change program = %d, want 33 (Fingered Electric Bass)", pcEvents[0].Program)
	}
}

func TestGenerateSMF_Roots_NoChords(t *testing.T) {
	measures := parseDSL(t, "c d e f")
	data, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Roots: true})
	if err != nil {
		t.Fatalf("GenerateSMFWithOptions: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	// No events on channel 8
	ch8Events := filterEvents(events, "note_on", 8)
	if len(ch8Events) != 0 {
		t.Errorf("got %d note_on events on ch8 for measures without chords, want 0", len(ch8Events))
	}
}

func TestGenerateSMF_Options(t *testing.T) {
	measures := parseDSL(t, "M4/4 c d e f :H C - G7 - |")

	// No metronome, no roots
	data1, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{})
	if err != nil {
		t.Fatalf("GenerateSMFWithOptions({}): %v", err)
	}
	events1, _ := ParseSMFToEvents(data1)
	if len(filterEvents(events1, "note_on", 9)) != 0 {
		t.Error("metronome events with Metronome:false")
	}
	if len(filterEvents(events1, "note_on", 8)) != 0 {
		t.Error("root events with Roots:false")
	}

	// Metronome only
	data2, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Metronome: true})
	if err != nil {
		t.Fatalf("GenerateSMFWithOptions(Metronome): %v", err)
	}
	events2, _ := ParseSMFToEvents(data2)
	if len(filterEvents(events2, "note_on", 9)) < 4 {
		t.Error("missing metronome events with Metronome:true")
	}
	if len(filterEvents(events2, "note_on", 8)) != 0 {
		t.Error("root events with Roots:false")
	}

	// Both
	data3, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Metronome: true, Roots: true})
	if err != nil {
		t.Fatalf("GenerateSMFWithOptions(both): %v", err)
	}
	events3, _ := ParseSMFToEvents(data3)
	if len(filterEvents(events3, "note_on", 9)) < 4 {
		t.Error("missing metronome events with both")
	}
	if len(filterEvents(events3, "note_on", 8)) == 0 {
		t.Error("missing root events with Roots:true")
	}
}

func TestGenerateSMF_Backbeats(t *testing.T) {
	measures := parseDSL(t, "M4/4 c d e f | a b c d")
	data, _, err := GenerateSMFWithOptions(measures, 120, SMFOptions{Metronome: true, Backbeats: true})
	if err != nil {
		t.Fatalf("GenerateSMFWithOptions backbeats: %v", err)
	}
	events, err := ParseSMFToEvents(data)
	if err != nil {
		t.Fatalf("ParseSMFToEvents: %v", err)
	}

	metroOns := filterEvents(events, "note_on", 9)
	// 4/4 over 2 measures: 2 backbeats per measure = 4 total
	if len(metroOns) != 4 {
		t.Fatalf("got %d metronome note_on with backbeats, want 4 (2 per measure × 2 measures)", len(metroOns))
	}
	// All backbeat clicks should use note 77 (weak), never 76 (downbeat)
	for i, m := range metroOns {
		if m.Note != 77 {
			t.Errorf("backbeat[%d] note = %d, want 77", i, m.Note)
		}
		if m.Velocity != 80 {
			t.Errorf("backbeat[%d] velocity = %d, want 80", i, m.Velocity)
		}
	}
}

// --- Helpers ---

type SMFEvent struct {
	Tick     int64
	Channel  uint8
	Type     string
	Note     uint8
	Velocity uint8
	Program  uint8
	BPM      float64
}

func filterEvents(events []SMFEvent, typ string, channel uint8) []SMFEvent {
	var result []SMFEvent
	for _, e := range events {
		if e.Type == typ && e.Channel == channel {
			result = append(result, e)
		}
	}
	return result
}

func ParseSMFToEvents(data []byte) ([]SMFEvent, error) {
	sf, err := smf.ReadFrom(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read SMF: %w", err)
	}

	var events []SMFEvent

	for _, track := range sf.Tracks {
		var tick int64
		for _, ev := range track {
			tick += int64(ev.Delta)

			msg := ev.Message
			if len(msg) == 0 {
				continue
			}

			status := msg[0]

			if status == 0xFF {
				if len(msg) < 3 {
					continue
				}
				switch msg[1] {
				case 0x51:
					if len(msg) >= 6 {
						usPerQuarter := int(msg[3])<<16 | int(msg[4])<<8 | int(msg[5])
						bpm := 60_000_000.0 / float64(usPerQuarter)
						events = append(events, SMFEvent{Tick: tick, Type: "meta_tempo", BPM: bpm})
					}
				}
				continue
			}

			channel := status & 0x0F
			msgType := status & 0xF0

			switch msgType {
			case 0x90:
				if len(msg) < 3 {
					continue
				}
				vel := msg[2]
				if vel == 0 {
					events = append(events, SMFEvent{Tick: tick, Channel: channel, Type: "note_off", Note: msg[1]})
				} else {
					events = append(events, SMFEvent{Tick: tick, Channel: channel, Type: "note_on", Note: msg[1], Velocity: vel})
				}
			case 0x80:
				if len(msg) < 3 {
					continue
				}
				events = append(events, SMFEvent{Tick: tick, Channel: channel, Type: "note_off", Note: msg[1]})
			case 0xC0:
				if len(msg) < 2 {
					continue
				}
				events = append(events, SMFEvent{Tick: tick, Channel: channel, Type: "program_change", Program: msg[1]})
			}
		}
	}

	return events, nil
}
