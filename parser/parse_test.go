package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBasicNotes(t *testing.T) {
	r := ParseDSL("c d e f")
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Measures[0].Events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(r.Measures[0].Events))
	}
	for i, ev := range r.Measures[0].Events {
		if ev.Type != EventNote {
			t.Errorf("event %d: expected note, got %s", i, ev.Type)
		}
		if ev.Duration.Num != 1 || ev.Duration.Den != 4 {
			t.Errorf("event %d: expected 1/4 duration, got %d/%d", i, ev.Duration.Num, ev.Duration.Den)
		}
	}
}

func TestParseSustainChain(t *testing.T) {
	r := ParseDSL("a - -b c")
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	// After splitNonStandardDurations: A half + A eighth (tied), B eighth, C quarter
	if len(r.Measures[0].Events) != 4 {
		t.Fatalf("expected 4 events (A half, A eighth, B eighth, C quarter), got %d", len(r.Measures[0].Events))
	}
	// First event: A half
	if r.Measures[0].Events[0].Duration.Num != 1 || r.Measures[0].Events[0].Duration.Den != 2 {
		t.Errorf("event 0: expected 1/2, got %d/%d", r.Measures[0].Events[0].Duration.Num, r.Measures[0].Events[0].Duration.Den)
	}
	// Second event: A eighth (continuation)
	if !r.Measures[0].Events[1].Split {
		t.Errorf("event 1 should be a split continuation")
	}
}

func TestParseTuplet(t *testing.T) {
	r := ParseDSL("abc")
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Measures[0].Events) != 4 { // tupletStart + 3 notes
		t.Fatalf("expected 4 events (tuplet + 3 notes), got %d", len(r.Measures[0].Events))
	}
	if r.Measures[0].Events[0].Type != EventTupletStart {
		t.Errorf("event 0 should be tupletStart")
	}
	for i := 1; i <= 3; i++ {
		if r.Measures[0].Events[i].Type != EventNote {
			t.Errorf("event %d should be note", i)
		}
		if r.Measures[0].Events[i].Nominal == nil {
			t.Errorf("event %d should have nominal duration", i)
		}
	}
}

func TestParseChord(t *testing.T) {
	r := ParseDSL("(ace)f")
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Measures[0].Events) != 2 {
		t.Fatalf("expected 2 events (chord + note), got %d", len(r.Measures[0].Events))
	}
	if r.Measures[0].Events[0].Type != EventChord {
		t.Errorf("event 0 should be chord")
	}
	if len(r.Measures[0].Events[0].Midis) != 3 {
		t.Errorf("chord should have 3 pitches, got %d", len(r.Measures[0].Events[0].Midis))
	}
}

func TestParseOctaveShift(t *testing.T) {
	cases := []struct {
		dsl      string
		desc     string
		expected []int // expected MIDI values
	}{
		{"c^c", "shift applies to next note", []int{60, 72}},
		{"^c", "prefix shift", []int{72}},
		{"c^^d^e", "multiple shifts across notes in beat", []int{60, 86, 100}},
		{"c c", "no shift between notes", []int{60, 60}},
		{"c/c", "down shift", []int{60, 48}},
		{"^c/c", "up then down", []int{72, 60}},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			r := ParseDSL(tc.dsl)
			if r.Err != nil {
				t.Fatalf("unexpected error: %v", r.Err)
			}
			// Collect note/chord events (skip tuplet markers)
			var mids []int
			for _, ev := range r.Measures[0].Events {
				if ev.Type == EventNote || ev.Type == EventChord {
					mids = append(mids, ev.Midi)
				}
			}
			if len(mids) != len(tc.expected) {
				t.Fatalf("expected %d notes, got %d (total events: %d)", len(tc.expected), len(mids), len(r.Measures[0].Events))
			}
			for i, m := range mids {
				if m != tc.expected[i] {
					t.Errorf("note %d: expected MIDI %d, got %d", i, tc.expected[i], m)
				}
			}
		})
	}
}

func TestParseAccidentalNatural(t *testing.T) {
	// &&d&d%d#d##d = Dbb, Db, Dn, D#, D##
	r := ParseDSL("&&d&d%d#d##d")
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	expected := []int{-2, -1, 0, 1, 2}
	var got []int
	for _, ev := range r.Measures[0].Events {
		if ev.Type == EventNote || ev.Type == EventChord {
			got = append(got, ev.Accidental)
		}
	}
	if len(got) != len(expected) {
		t.Fatalf("expected %d notes, got %d", len(expected), len(got))
	}
	for i, e := range expected {
		if got[i] != e {
			t.Errorf("note %d: expected accidental %d, got %d", i, e, got[i])
		}
	}
}

func TestParseCompoundTime(t *testing.T) {
	r := ParseDSL("M6/8 abc def")
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	// 6 eighths in 6/8, no tuplet needed
	if len(r.Measures[0].Events) != 6 {
		t.Fatalf("expected 6 events, got %d", len(r.Measures[0].Events))
	}
	for i, ev := range r.Measures[0].Events {
		g := gcd(ev.Duration.Num, ev.Duration.Den)
		num := ev.Duration.Num / g
		den := ev.Duration.Den / g
		if num != 1 || den != 8 {
			t.Errorf("event %d: expected 1/8, got %d/%d", i, num, den)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		dsl  string
		desc string
	}{
		{"-", "bare sustain"},
		{"(ace", "unclosed chord"},
		{")", "unmatched paren"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			r := ParseDSL(tc.dsl)
			if r.Err == nil {
				t.Errorf("expected error for %s", tc.desc)
			}
		})
	}
}

func TestParseDSLFiles(t *testing.T) {
	// Find all .dsl files in test/cases/
	matches, err := filepath.Glob("test/cases/*.dsl")
	if err != nil || len(matches) == 0 {
		// Try from repo root
		matches, err = filepath.Glob("../test/cases/*.dsl")
		if err != nil || len(matches) == 0 {
			t.Skip("no test case files found")
		}
	}
	for _, path := range matches {
		name := filepath.Base(path)
		if strings.HasPrefix(name, "error-") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("cannot read %s: %v", path, err)
			}
			// Strip comments
			var dslParts []string
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				dslParts = append(dslParts, line)
			}
			dsl := strings.Join(dslParts, " ")
			if dsl == "" {
				t.Skip("empty DSL file")
			}
			r := ParseDSL(dsl)
			if r.Err != nil {
				t.Fatalf("parse error for %s (%q): %v", name, dsl, r.Err)
			}
			if len(r.Measures) == 0 {
				t.Fatalf("no measures produced for %s (%q)", name, dsl)
			}
		})
	}
}
