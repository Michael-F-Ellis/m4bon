package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBasicNotes(t *testing.T) {
	r := ParseDSL([]string{"c d e f"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	validateEvents(t, r.Measures[0].Events)
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
	r := ParseDSL([]string{"a - -b c"})
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
	r := ParseDSL([]string{"abc"})
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
	r := ParseDSL([]string{"(ace)f"})
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
			r := ParseDSL([]string{tc.dsl})
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
	r := ParseDSL([]string{"&&d&d%d#d##d"})
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
	r := ParseDSL([]string{"M6/8 abc def"})
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
			r := ParseDSL([]string{tc.dsl})
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
			// Read lines, strip comments, pass directly to ParseDSL
			var lines []string
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				lines = append(lines, line)
			}
			if len(lines) == 0 {
				t.Skip("empty DSL file")
			}
			r := ParseDSL(lines)
			if r.Err != nil {
				t.Fatalf("parse error for %s: %v", name, r.Err)
			}
			if len(r.Measures) == 0 {
				t.Fatalf("no measures produced for %s", name)
			}
		})
	}
}

func TestParseVoicePolyChord(t *testing.T) {
	// (c) (-e) starts voice 1 with C, then extends v1 and starts v2 with E
	r := ParseDSL([]string{"(c) (-e)"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	evs := r.Measures[0].Events
	// (c) = C quarter v1; (-e) = sustain extends v1 by quarter + E quarter v2
	if len(evs) != 2 {
		t.Fatalf("expected 2 events (C half v1 + E quarter v2), got %d", len(evs))
	}
	if evs[0].Type != EventChord {
		t.Errorf("event 0: expected EventChord, got %s", evs[0].Type)
	}
	if evs[1].Type != EventNote || evs[1].Voice != 2 || evs[1].Letter != "e" {
		t.Errorf("event 1: expected E note voice 2")
	}
	// Voice 1's C should be extended to a half note by the sustain
	if evs[0].Duration.Num != 1 || evs[0].Duration.Den != 2 {
		t.Errorf("voice 1 C: expected 1/2 duration, got %d/%d", evs[0].Duration.Num, evs[0].Duration.Den)
	}
}

func TestParseVoicePolyThreeEntry(t *testing.T) {
	// (c - e) as a single group errors because sustain at entry 1 is voice 2 with no prior
	r := ParseDSL([]string{"(c-e)"})
	if r.Err == nil {
		t.Errorf("expected error: sustain in voice 2 with no prior note")
	}
}

func TestParseVoicePolyWithRest(t *testing.T) {
	r := ParseDSL([]string{"(c;e)"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	evs := r.Measures[0].Events
	// 3 entries: C (v1), rest (v2, standalone), E (v3)
	if len(evs) != 3 {
		t.Fatalf("expected 3 events (C v1 + rest v2 + E v3), got %d", len(evs))
	}
	if evs[0].Type != EventNote || evs[0].Voice != 1 || evs[0].Letter != "c" {
		t.Errorf("event 0: expected C note voice 1")
	}
	if evs[1].Type != EventRest || evs[1].Voice != 2 {
		t.Errorf("event 1: expected rest voice 2")
	}
	if evs[2].Type != EventNote || evs[2].Voice != 3 || evs[2].Letter != "e" {
		t.Errorf("event 2: expected E note voice 3")
	}
}

func TestParseVoicePolyTraditionalUnchanged(t *testing.T) {
	r := ParseDSL([]string{"(ceg)"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	evs := r.Measures[0].Events
	if len(evs) != 1 {
		t.Fatalf("expected 1 chord event, got %d", len(evs))
	}
	if evs[0].Type != EventChord {
		t.Errorf("expected EventChord, got %s", evs[0].Type)
	}
	if len(evs[0].Midis) != 3 {
		t.Errorf("expected 3 midi pitches, got %d", len(evs[0].Midis))
	}
}


func TestParseVoicePolySustainError(t *testing.T) {
	// (-e) has sustain at entry 0 (voice 1) with no prior
	r := ParseDSL([]string{"(-e)"})
	if r.Err == nil {
		t.Errorf("expected error for sustain with no prior note")
	}
	// (c-e) has sustain at entry 1 (voice 2) with no prior voice 2
	r2 := ParseDSL([]string{"(c-e)"})
	if r2.Err == nil {
		t.Errorf("expected error for sustain in voice 2 with no prior note")
	}
}

func TestParseVoicePolyThreeGroupSustain(t *testing.T) {
	// (c) (-e) (-g) → v1: C dotted half (3/4) split at midpoint of 4/4,
	// so C half + C eighth(continuation) + v2: E+G quarters
	r := ParseDSL([]string{"(c) (-e) (-g)"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	evs := r.Measures[0].Events
	// Voice 1 C gets split at barline: 5 events total (C half + C eighth-tie +
	// E + G + tupletStart? no, no tuplet)
	if len(evs) != 4 {
		t.Fatalf("expected 4 events (C half + C eighth tied + E + G), got %d", len(evs))
	}
	// Voice 1 C split into half (960) + eighth tied (480) = 3/4 total
	// events[0] = C first half (Voice=1, Dur=1/2, Split=false)
	// events[1] = C second half (Voice=1, Dur=1/4, Split=true)
	// events[2] = E quarter (Voice=2)
	// events[3] = G quarter (Voice=2)
	if evs[0].Duration.Num != 1 || evs[0].Duration.Den != 2 {
		t.Errorf("event 0 (C first half): expected 1/2, got %d/%d", evs[0].Duration.Num, evs[0].Duration.Den)
	}
	if evs[1].Duration.Num != 1 || evs[1].Duration.Den != 4 {
		t.Errorf("event 1 (C second half): expected 1/4, got %d/%d", evs[1].Duration.Num, evs[1].Duration.Den)
	}
	if !evs[1].Split {
		t.Errorf("event 1 should be a split continuation")
	}
	if evs[2].Voice != 2 || evs[2].Letter != "e" {
		t.Errorf("event 2: expected E voice 2")
	}
	if evs[3].Voice != 2 || evs[3].Letter != "g" {
		t.Errorf("event 3: expected G voice 2")
	}
}

func TestExtractChordDirective(t *testing.T) {
	r := ParseDSL([]string{"M4/4 c d e f :H C - G7 -"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Measures) == 0 {
		t.Fatal("expected at least one measure")
	}
	m := r.Measures[0]
	if !m.HasChords {
		t.Error("expected HasChords to be true")
	}
	if m.HasLyrics {
		t.Error("expected HasLyrics to be false")
	}
	expected := []string{"C", "-", "G7", "-"}
	if len(m.Chords) != len(expected) {
		t.Fatalf("expected %d chords, got %d", len(expected), len(m.Chords))
	}
	for i, e := range expected {
		if m.Chords[i] != e {
			t.Errorf("chord %d: expected %q, got %q", i, e, m.Chords[i])
		}
	}
	// Existing events still parse correctly
	if len(m.Events) != 4 {
		t.Errorf("expected 4 note events, got %d", len(m.Events))
	}
}

func TestExtractLyricDirective(t *testing.T) {
	r := ParseDSL([]string{"M4/4 c d e f :L My heart is sad"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Measures) == 0 {
		t.Fatal("expected at least one measure")
	}
	m := r.Measures[0]
	if !m.HasLyrics {
		t.Error("expected HasLyrics to be true")
	}
	if m.HasChords {
		t.Error("expected HasChords to be false")
	}
	expected := []string{"My", "heart", "is", "sad"}
	if len(m.Lyrics) != len(expected) {
		t.Fatalf("expected %d lyrics, got %d", len(expected), len(m.Lyrics))
	}
	for i, e := range expected {
		if m.Lyrics[i] != e {
			t.Errorf("lyric %d: expected %q, got %q", i, e, m.Lyrics[i])
		}
	}
	if len(m.Events) != 4 {
		t.Errorf("expected 4 note events, got %d", len(m.Events))
	}
}

func TestExtractBothOrderIndependent(t *testing.T) {
	// :H before :L
	r := ParseDSL([]string{"M4/4 c d e f :H C - G7 - :L My heart is sad"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	m := r.Measures[0]
	if !m.HasChords || !m.HasLyrics {
		t.Errorf("expected both HasChords and HasLyrics, got HasChords=%v HasLyrics=%v", m.HasChords, m.HasLyrics)
	}
	if len(m.Chords) != 4 {
		t.Errorf("expected 4 chords, got %d", len(m.Chords))
	}
	if len(m.Lyrics) != 4 {
		t.Errorf("expected 4 lyrics, got %d", len(m.Lyrics))
	}

	// :L before :H (order-independent)
	r2 := ParseDSL([]string{"M4/4 c d e f :L My heart is sad :H C - G7 -"})
	if r2.Err != nil {
		t.Fatalf("unexpected error: %v", r2.Err)
	}
	m2 := r2.Measures[0]
	if !m2.HasChords || !m2.HasLyrics {
		t.Errorf("expected both HasChords and HasLyrics, got HasChords=%v HasLyrics=%v", m2.HasChords, m2.HasLyrics)
	}
	if len(m2.Chords) != 4 {
		t.Errorf("expected 4 chords, got %d", len(m2.Chords))
	}
	if len(m2.Lyrics) != 4 {
		t.Errorf("expected 4 lyrics, got %d", len(m2.Lyrics))
	}
}

func TestNoDirectives(t *testing.T) {
	r := ParseDSL([]string{"M4/4 c d e f"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	m := r.Measures[0]
	if m.HasChords {
		t.Error("expected HasChords to be false")
	}
	if m.HasLyrics {
		t.Error("expected HasLyrics to be false")
	}
	if len(m.Chords) != 0 {
		t.Errorf("expected empty chords, got %v", m.Chords)
	}
	if len(m.Lyrics) != 0 {
		t.Errorf("expected empty lyrics, got %v", m.Lyrics)
	}
}

func TestEmptyDirectiveIgnored(t *testing.T) {
	// Empty :H and :L directives are accepted (no tokens between them and |)
	// but produce empty slices and HasChords/HasLyrics are false.
	r := ParseDSL([]string{"M4/4 c d e f :H :L"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	m := r.Measures[0]
	if m.HasChords {
		t.Error("expected HasChords to be false (empty :H)")
	}
	if m.HasLyrics {
		t.Error("expected HasLyrics to be false (empty :L)")
	}
	if len(m.Chords) != 0 {
		t.Errorf("expected empty chords, got %v", m.Chords)
	}
	if len(m.Lyrics) != 0 {
		t.Errorf("expected empty lyrics, got %v", m.Lyrics)
	}
}

func TestMultiMeasureChordLyric(t *testing.T) {
	r := ParseDSL([]string{"M4/4 c d e f :H C - G7 -", "M4/4 a b c d :L One two three four"})
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Measures) != 2 {
		t.Fatalf("expected 2 measures, got %d", len(r.Measures))
	}
	m0 := r.Measures[0]
	m1 := r.Measures[1]
	if !m0.HasChords || m0.HasLyrics {
		t.Errorf("measure 0: expected chords only")
	}
	if !m1.HasLyrics || m1.HasChords {
		t.Errorf("measure 1: expected lyrics only")
	}
	if len(m0.Chords) != 4 {
		t.Errorf("measure 0: expected 4 chords, got %d", len(m0.Chords))
	}
	if len(m1.Lyrics) != 4 {
		t.Errorf("measure 1: expected 4 lyrics, got %d", len(m1.Lyrics))
	}
}

func validateEvents(t *testing.T, events []Event) {
	t.Helper()
	for i, ev := range events {
		if err := ev.Validate(); err != nil {
			t.Errorf("event %d invalid: %v", i, err)
		}
	}
}

// --- Comment tests ---

func TestSanitizeWithComments_Basic(t *testing.T) {
	dsl := "M4/4 c d e f\n! Chopin Etude\ng a b c"
	lines, comments := SanitizeWithComments(dsl)

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	c := comments[1]
	if len(c) != 1 || c[0] != "Chopin Etude" {
		t.Errorf("comments[1]: got %v", c)
	}
}

func TestSanitizeWithComments_MultiLineBlock(t *testing.T) {
	dsl := "! Top row\n! on two lines\nc d e f"
	lines, comments := SanitizeWithComments(dsl)

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	c := comments[0]
	if len(c) != 2 || c[0] != "Top row" || c[1] != "on two lines" {
		t.Errorf("comments[0]: got %v, want [Top row, on two lines]", c)
	}
}

func TestSanitizeWithComments_EmptyCommentBody(t *testing.T) {
	dsl := "!\nc d e f"
	lines, comments := SanitizeWithComments(dsl)

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if len(comments) != 0 {
		t.Errorf("expected no comments, got %v", comments)
	}
}

func TestSanitizeWithComments_HashComments(t *testing.T) {
	dsl := "# hash comment\nc d e f\n! bang comment\ng a b c"
	lines, comments := SanitizeWithComments(dsl)

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	c := comments[1]
	if len(c) != 1 || c[0] != "bang comment" {
		t.Errorf("comments[1]: got %v", c)
	}
}

func TestSanitizeWithComments_TrailingBang(t *testing.T) {
	dsl := "c d e f\n! orphan comment"
	lines, comments := SanitizeWithComments(dsl)

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	c := comments[1]
	if len(c) != 1 || c[0] != "orphan comment" {
		t.Errorf("comments[1]: got %v, want [orphan comment]", c)
	}
}

func TestParseDSLWithComments(t *testing.T) {
	dsl := "! Study notes\nM4/4 c d e f\n! Dynamic contrast\ng a b c"
	lines, comments := SanitizeWithComments(dsl)
	r := ParseDSLWithComments(lines, comments)

	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Measures) != 2 {
		t.Fatalf("expected 2 measures, got %d", len(r.Measures))
	}
	if l := r.Measures[0].CommentLines; len(l) != 1 || l[0] != "Study notes" {
		t.Errorf("measure 0 CommentLines: got %v", l)
	}
	if l := r.Measures[1].CommentLines; len(l) != 1 || l[0] != "Dynamic contrast" {
		t.Errorf("measure 1 CommentLines: got %v", l)
	}
}

func TestParseDSL_NoComments(t *testing.T) {
	r := ParseDSL([]string{"c d e f", "g a b c"})

	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Measures) != 2 {
		t.Fatalf("expected 2 measures, got %d", len(r.Measures))
	}
	if l := r.Measures[0].CommentLines; len(l) != 0 {
		t.Errorf("expected empty CommentLines, got %v", l)
	}
}

func TestParseDSL_TrailingComment(t *testing.T) {
	dsl := "! Before\nc d e f\n! After"
	lines, comments := SanitizeWithComments(dsl)
	r := ParseDSLWithComments(lines, comments)

	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if l := r.Measures[0].CommentLines; len(l) != 1 || l[0] != "Before" {
		t.Errorf("measure 0 CommentLines: got %v", l)
	}
	if l := r.Measures[0].TrailingCommentLines; len(l) != 1 || l[0] != "After" {
		t.Errorf("measure 0 TrailingCommentLines: got %v", l)
	}
}
