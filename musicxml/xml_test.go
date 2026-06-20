package musicxml

import (
	"strings"
	"testing"

	"github.com/mellis/m4bon/parser"
)

// --- Pure function tests ---

func TestNoteTypeForDuration(t *testing.T) {
	cases := []struct {
		f    parser.Fraction
		want string
	}{
		{parser.Fraction{Num: 1, Den: 1}, "whole"},
		{parser.Fraction{Num: 1, Den: 2}, "half"},
		{parser.Fraction{Num: 1, Den: 4}, "quarter"},
		{parser.Fraction{Num: 1, Den: 8}, "eighth"},
		{parser.Fraction{Num: 1, Den: 16}, "16th"},
		{parser.Fraction{Num: 1, Den: 32}, "32nd"},
		{parser.Fraction{Num: 1, Den: 64}, "64th"},
		{parser.Fraction{Num: 1, Den: 128}, "128th"},
		// Dotted
		{parser.Fraction{Num: 3, Den: 4}, "half"},
		{parser.Fraction{Num: 3, Den: 8}, "quarter"},
		{parser.Fraction{Num: 3, Den: 16}, "eighth"},
		{parser.Fraction{Num: 3, Den: 32}, "16th"},
		{parser.Fraction{Num: 3, Den: 64}, "32nd"},
		{parser.Fraction{Num: 3, Den: 128}, "64th"},
		// Non-standard
		{parser.Fraction{Num: 5, Den: 8}, ""},
		{parser.Fraction{Num: 1, Den: 6}, ""},
	}
	for _, tc := range cases {
		got := noteTypeForDuration(tc.f)
		if got != tc.want {
			t.Errorf("noteTypeForDuration(%d/%d) = %q, want %q", tc.f.Num, tc.f.Den, got, tc.want)
		}
	}
}

func TestSanitizeDSL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"c d e f", "c d e f"},
		{"# comment\nc d\ne f", "c d e f"},        // comment lines stripped
		{"#\nc d e f", "c d e f"},                  // bare # comment
		{"#c d e f", "#c d e f"},                   // #c is NOT a comment (pitch)
		{"#&b c d e f", "#&b c d e f"},             // #&b is NOT a comment
		{"", ""},                                    // empty stays empty
		{"   ", ""},                                 // whitespace-only → empty
		{"c d e f\n\n\ng a b c", "c d e f g a b c"}, // blank lines stripped
	}
	for _, tc := range cases {
		got := parser.SanitizeDSL(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeDSL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMidiToStep(t *testing.T) {
	cases := []struct {
		midi     int
		step     string
		octave   int
		alter    int
	}{
		{60, "C", 4, 0},
		{61, "C", 4, 1},
		{62, "D", 4, 0},
		{63, "D", 4, 1},
		{64, "E", 4, 0},
		{65, "F", 4, 0},
		{66, "F", 4, 1},
		{67, "G", 4, 0},
		{68, "G", 4, 1},
		{69, "A", 4, 0},
		{70, "A", 4, 1},
		{71, "B", 4, 0},
		{72, "C", 5, 0},
		{0, "C", -1, 0},
		{127, "G", 9, 0},
	}
	for _, tc := range cases {
		step, oct, alter := midiToStep(tc.midi)
		if step != tc.step || oct != tc.octave || alter != tc.alter {
			t.Errorf("midiToStep(%d) = (%s, %d, %d), want (%s, %d, %d)",
				tc.midi, step, oct, alter, tc.step, tc.octave, tc.alter)
		}
	}
}

func TestAccidentalString(t *testing.T) {
	cases := []struct {
		acc  int
		want string
	}{
		{0, ""},
		{1, "sharp"},
		{-1, "flat"},
		{2, "double-sharp"},
		{-2, "flat-flat"},
		{3, ""},
		{-3, ""},
	}
	for _, tc := range cases {
		got := accidentalString(tc.acc)
		if got != tc.want {
			t.Errorf("accidentalString(%d) = %q, want %q", tc.acc, got, tc.want)
		}
	}
}

func TestDurationToTicks(t *testing.T) {
	cases := []struct {
		f    parser.Fraction
		want int
	}{
		{parser.Fraction{Num: 1, Den: 4}, 480},   // quarter
		{parser.Fraction{Num: 1, Den: 2}, 960},   // half
		{parser.Fraction{Num: 1, Den: 8}, 240},   // eighth
		{parser.Fraction{Num: 1, Den: 1}, 1920},  // whole
		{parser.Fraction{Num: 3, Den: 4}, 1440},  // dotted half
		{parser.Fraction{Num: 3, Den: 8}, 720},   // dotted quarter
		{parser.Fraction{Num: 5, Den: 4}, 2400},  // 5 quarter notes
		{parser.Fraction{Num: 1, Den: 16}, 120},  // 16th
		{parser.Fraction{Num: 1, Den: 32}, 60},   // 32nd
		{parser.Fraction{Num: 0, Den: 1}, 0},     // zero
	}
	for _, tc := range cases {
		got := durationToTicks(tc.f)
		if got != tc.want {
			t.Errorf("durationToTicks(%d/%d) = %d, want %d", tc.f.Num, tc.f.Den, got, tc.want)
		}
	}
}

func TestIsPowerOf2(t *testing.T) {
	cases := []struct {
		n    int
		want bool
	}{
		{1, true}, {2, true}, {4, true}, {8, true}, {16, true}, {32, true}, {64, true}, {128, true},
		{0, false}, {3, false}, {5, false}, {6, false}, {7, false}, {9, false}, {-4, false},
	}
	for _, tc := range cases {
		got := isPowerOf2(tc.n)
		if got != tc.want {
			t.Errorf("isPowerOf2(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestDotCount(t *testing.T) {
	cases := []struct {
		f    parser.Fraction
		want int
	}{
		{parser.Fraction{Num: 1, Den: 4}, 0},   // plain quarter
		{parser.Fraction{Num: 3, Den: 4}, 1},   // dotted half (3 = 1+2)
		{parser.Fraction{Num: 7, Den: 8}, 2},   // double-dotted half (7 = 1+2+4)
		{parser.Fraction{Num: 15, Den: 16}, 3}, // triple-dotted half
		{parser.Fraction{Num: 1, Den: 6}, 0},   // non-power-of-2 den
		{parser.Fraction{Num: 5, Den: 8}, 0},   // 5 is not 1, 3, 7, or 15
	}
	for _, tc := range cases {
		got := dotCount(tc.f)
		if got != tc.want {
			t.Errorf("dotCount(%d/%d) = %d, want %d", tc.f.Num, tc.f.Den, got, tc.want)
		}
	}
}

func TestMakeDots(t *testing.T) {
	if got := makeDots(0); got != nil {
		t.Errorf("makeDots(0) should be nil, got %v", got)
	}
	if got := makeDots(1); len(got) != 1 {
		t.Errorf("makeDots(1) should have 1 dot, got %d", len(got))
	}
	if got := makeDots(3); len(got) != 3 {
		t.Errorf("makeDots(3) should have 3 dots, got %d", len(got))
	}
}

// --- Generator tests ---

func TestGenerateSingleMeasureNotes(t *testing.T) {
	measures := []parser.MeasureResult{{
		Events: []parser.Event{
			parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 1, Den: 4}, nil, 1, 0),
			parser.NewNoteEvent("d", 0, 0, false, parser.Fraction{Num: 1, Den: 4}, nil, 1, 0),
			parser.NewNoteEvent("e", 0, 0, false, parser.Fraction{Num: 1, Den: 4}, nil, 1, 0),
			parser.NewNoteEvent("f", 0, 0, false, parser.Fraction{Num: 1, Den: 4}, nil, 1, 0),
		},
		TimeNum:   4,
		TimeDen:   4,
		Fifths:    0,
		NumGroups: 4,
	}}

	// Resolve octaves manually
	for i := range measures[0].Events {
		ev := &measures[0].Events[i]
		if ev.Letter == "c" {
			ev.Midi = 60
		} else if ev.Letter == "d" {
			ev.Midi = 62
		} else if ev.Letter == "e" {
			ev.Midi = 64
		} else if ev.Letter == "f" {
			ev.Midi = 65
		}
	}

	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(xml, `<step>C</step>`) {
		t.Error("XML should contain C step")
	}
	if !strings.Contains(xml, `<octave>4</octave>`) {
		t.Error("XML should contain octave 4")
	}
	if !strings.Contains(xml, `<duration>480</duration>`) {
		t.Error("XML should contain duration 480 (quarter note)")
	}
	if !strings.Contains(xml, `<type>quarter</type>`) {
		t.Error("XML should contain quarter note type")
	}
	if !strings.Contains(xml, `<divisions>480</divisions>`) {
		t.Error("XML should contain 480 divisions per quarter")
	}
	// Should have 4 notes
	if count := strings.Count(xml, "<note>"); count != 4 {
		t.Errorf("expected 4 <note> elements, got %d", count)
	}
}

func TestGenerateWithRest(t *testing.T) {
	ev := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 1, Den: 2}, nil, 1, 0)
	ev.Midi = 60
	re := parser.NewRestEvent(parser.Fraction{Num: 1, Den: 2}, nil, 1, 1)
	measures := []parser.MeasureResult{{
		Events:    []parser.Event{ev, re},
		TimeNum:   4,
		TimeDen:   4,
		Fifths:    0,
		NumGroups: 2,
	}}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(xml, "<rest>") {
		t.Error("XML should contain rest element")
	}
}

func TestGenerateChord(t *testing.T) {
	ev := parser.NewChordEvent(
		[]parser.Pitch{
			{Letter: "c"}, {Letter: "e"}, {Letter: "g"},
		},
		parser.Fraction{Num: 1, Den: 4}, nil, 1, 0,
	)
	ev.Midis = []int{60, 64, 67}
	measures := []parser.MeasureResult{{
		Events:    []parser.Event{ev},
		TimeNum:   4,
		TimeDen:   4,
		Fifths:    0,
		NumGroups: 1,
	}}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Should have 3 note elements, first without chord flag, rest with
	// Should have chord=true on second and third notes
	if count := strings.Count(xml, "<chord>"); count != 2 {
		t.Errorf("expected 2 <chord> markers for chord notes, got %d", count)
	}
	if count := strings.Count(xml, "<note>"); count != 3 {
		t.Errorf("expected 3 <note> elements for chord, got %d", count)
	}
}

func TestGenerateKeySignature(t *testing.T) {
	ev := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 4, Den: 4}, nil, 1, 0)
	ev.Midi = 60
	measures := []parser.MeasureResult{{
		Events:    []parser.Event{ev},
		TimeNum:   4,
		TimeDen:   4,
		Fifths:    3, // A major (3 sharps)
		NumGroups: 1,
	}}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(xml, `<fifths>3</fifths>`) {
		t.Error("XML should contain fifths=3 for A major")
	}
}

func TestGenerateAccidentalDisplay(t *testing.T) {
	ev := parser.NewNoteEvent("f", 1, 0, false, parser.Fraction{Num: 1, Den: 4}, nil, 1, 0) // F#
	ev.Midi = 66
	measures := []parser.MeasureResult{{
		Events:    []parser.Event{ev},
		TimeNum:   4,
		TimeDen:   4,
		Fifths:    0,
		NumGroups: 1,
	}}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(xml, `<accidental>sharp</accidental>`) {
		t.Error("XML should contain sharp accidental for F#")
	}
	if !strings.Contains(xml, `<alter>1</alter>`) {
		t.Error("XML should contain alter=1 for F#")
	}
}

func TestGenerateEmptyMeasures(t *testing.T) {
	xml, err := Generate(nil, 0)
	if err != nil {
		t.Fatalf("Generate failed for empty input: %v", err)
	}
	if !strings.Contains(xml, "<score-partwise") {
		t.Error("XML should contain score-partwise root element")
	}
	if !strings.Contains(xml, "MusicXML") {
		t.Error("XML should contain MusicXML header")
	}
}

func TestGenerateMultiMeasure(t *testing.T) {
	ev1 := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 4, Den: 4}, nil, 1, 0)
	ev1.Midi = 60
	ev2 := parser.NewNoteEvent("d", 0, 0, false, parser.Fraction{Num: 4, Den: 4}, nil, 1, 0)
	ev2.Midi = 62

	measures := []parser.MeasureResult{
		{
			Events:    []parser.Event{ev1},
			TimeNum:   4,
			TimeDen:   4,
			Fifths:    0,
			NumGroups: 1,
		},
		{
			Events:    []parser.Event{ev2},
			TimeNum:   4,
			TimeDen:   4,
			Fifths:    0,
			NumGroups: 1,
		},
	}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if count := strings.Count(xml, `<measure`); count != 2 {
		t.Errorf("expected 2 measures, got %d", count)
	}
}

func TestGeneratePickupMeasure(t *testing.T) {
	ev := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 1, Den: 4}, nil, 1, 0)
	ev.Midi = 60
	ev2 := parser.NewNoteEvent("d", 0, 0, false, parser.Fraction{Num: 4, Den: 4}, nil, 1, 0)
	ev2.Midi = 62

	measures := []parser.MeasureResult{
		{
			Events:    []parser.Event{ev},
			TimeNum:   4,
			TimeDen:   4,
			Fifths:    0,
			IsPickup:  true,
			NumGroups: 1,
		},
		{
			Events:    []parser.Event{ev2},
			TimeNum:   4,
			TimeDen:   4,
			Fifths:    0,
			NumGroups: 1,
		},
	}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Pickup measure should be numbered 0
	if !strings.Contains(xml, `number="0"`) {
		t.Error("Pickup measure should have number=\"0\"")
	}
}

func TestGenerateTuplet(t *testing.T) {
	tupletStart := parser.NewTupletStartEvent(parser.Fraction{Num: 1, Den: 4}, 0, 3, 2)
	n1 := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 1, Den: 6}, &parser.Fraction{Num: 1, Den: 8}, 1, 0)
	n1.Midi = 60
	n2 := parser.NewNoteEvent("d", 0, 0, false, parser.Fraction{Num: 1, Den: 6}, &parser.Fraction{Num: 1, Den: 8}, 1, 0)
	n2.Midi = 62
	n3 := parser.NewNoteEvent("e", 0, 0, false, parser.Fraction{Num: 1, Den: 6}, &parser.Fraction{Num: 1, Den: 8}, 1, 0)
	n3.Midi = 64

	measures := []parser.MeasureResult{{
		Events:    []parser.Event{tupletStart, n1, n2, n3},
		TimeNum:   4,
		TimeDen:   4,
		Fifths:    0,
		NumGroups: 1,
	}}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Should contain time-modification
	if !strings.Contains(xml, `<time-modification>`) {
		t.Error("Tuplet should have time-modification element")
	}
	if !strings.Contains(xml, `<actual-notes>3</actual-notes>`) {
		t.Error("Tuplet should have actual-notes=3")
	}
	if !strings.Contains(xml, `<normal-notes>2</normal-notes>`) {
		t.Error("Tuplet should have normal-notes=2")
	}
}

func TestGenerateTieAcrossSplit(t *testing.T) {
	n1 := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 1, Den: 4}, nil, 1, 0)
	n1.Midi = 60
	n2 := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 1, Den: 8}, nil, 1, 1)
	n2.Midi = 60
	n2.Split = true

	measures := []parser.MeasureResult{{
		Events:    []parser.Event{n1, n2},
		TimeNum:   4,
		TimeDen:   4,
		Fifths:    0,
		NumGroups: 1,
	}}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Should contain tie elements (start+stop = 2)
	if count := strings.Count(xml, "<tie type="); count != 2 {
		t.Errorf("expected 2 tie elements (start+stop), got %d", count)
	}
	// Should contain tied elements (start+stop = 2)
	if count := strings.Count(xml, "<tied type="); count != 2 {
		t.Errorf("expected 2 tied elements (start+stop), got %d", count)
	}
}

func TestGenerateMultiVoice(t *testing.T) {
	n1 := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 1, Den: 2}, nil, 1, 0)
	n1.Midi = 60
	n2 := parser.NewNoteEvent("e", 0, 0, false, parser.Fraction{Num: 1, Den: 2}, nil, 2, 0)
	n2.Midi = 64

	measures := []parser.MeasureResult{{
		Events:    []parser.Event{n1, n2},
		TimeNum:   4,
		TimeDen:   4,
		Fifths:    0,
		NumGroups: 1,
	}}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Should have voice=1 and voice=2
	if !strings.Contains(xml, `<voice>1</voice>`) {
		t.Error("Should have voice 1")
	}
	if !strings.Contains(xml, `<voice>2</voice>`) {
		t.Error("Should have voice 2")
	}
}

func TestGenerateChangesTimeSigInSecondMeasure(t *testing.T) {
	n1 := parser.NewNoteEvent("c", 0, 0, false, parser.Fraction{Num: 1, Den: 4}, nil, 1, 0)
	n1.Midi = 60
	n2 := parser.NewNoteEvent("d", 0, 0, false, parser.Fraction{Num: 3, Den: 4}, nil, 1, 0)
	n2.Midi = 62

	measures := []parser.MeasureResult{
		{
			Events:    []parser.Event{n1},
			TimeNum:   4,
			TimeDen:   4,
			Fifths:    0,
			NumGroups: 1,
		},
		{
			Events:    []parser.Event{n2},
			TimeNum:   3,
			TimeDen:   4,
			Fifths:    0,
			NumGroups: 1,
		},
	}
	xml, err := Generate(measures, 0)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Second measure should have new time signature
	// Check that there are two <beats> elements (one per measure attributes)
	if count := strings.Count(xml, "<beats>"); count != 2 {
		t.Errorf("expected 2 <beats> elements (time sig per measure), got %d", count)
	}
}

// --- Roundtrip sanity tests ---

func TestRoundtripBasicNotes(t *testing.T) {
	result := parser.ParseDSL("c d e f")
	if result.Err != nil {
		t.Fatalf("ParseDSL failed: %v", result.Err)
	}
	xml, err := Generate(result.Measures, result.Key.Fifths)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(xml, `<step>C</step>`) {
		t.Error("Roundtrip: expected C note")
	}
	if !strings.Contains(xml, `<duration>480</duration>`) {
		t.Error("Roundtrip: expected quarter notes")
	}
	if count := strings.Count(xml, "<note>"); count != 4 {
		t.Errorf("Roundtrip: expected 4 notes, got %d", count)
	}
}
