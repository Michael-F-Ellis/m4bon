package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the module root directory (containing go.mod).

func TestMain(m *testing.M) {
	if err := os.Chdir(repoRoot()); err != nil {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestCLIBasicNotes(t *testing.T) {
	out, err := exec.Command("./m4bon", "c d e f").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	checks := []string{
		`<step>C</step>`,
		`<step>D</step>`,
		`<step>E</step>`,
		`<step>F</step>`,
		`<octave>4</octave>`,
		`<duration>480</duration>`,
		`<type>quarter</type>`,
		`<beats>4</beats>`,
		`<beat-type>4</beat-type>`,
	}
	for _, c := range checks {
		if !strings.Contains(xml, c) {
			t.Errorf("expected output to contain %q", c)
		}
	}
}

func TestCLISustainChain(t *testing.T) {
	out, err := exec.Command("./m4bon", "a - -b c").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	// Should have half note A (960 ticks) tied to eighth A, then B, then C
	if !strings.Contains(xml, `<duration>960</duration>`) {
		t.Errorf("expected half note (960 ticks) for sustained A")
	}
	if !strings.Contains(xml, `<duration>240</duration>`) {
		t.Errorf("expected eighth note (240 ticks)")
	}
	if !strings.Contains(xml, `<tie type="start">`) {
		t.Errorf("expected tie start on first A fragment")
	}
	if !strings.Contains(xml, `<tie type="stop">`) {
		t.Errorf("expected tie stop on second A fragment")
	}
}

func TestCLITuplet(t *testing.T) {
	out, err := exec.Command("./m4bon", "abc").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	checks := []string{
		`<time-modification>`,
		`<actual-notes>3</actual-notes>`,
		`<normal-notes>2</normal-notes>`,
		`<type>eighth</type>`,
		`<duration>160</duration>`,
		`<tuplet type="start"`,
		`<tuplet type="stop"`,
	}
	for _, c := range checks {
		if !strings.Contains(xml, c) {
			t.Errorf("expected tuplet output to contain %q", c)
		}
	}
}

func TestCLIChord(t *testing.T) {
	out, err := exec.Command("./m4bon", "(ace)f").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	checks := []string{
		`<chord></chord>`,
		`<step>A</step>`,
		`<step>C</step>`,
		`<step>E</step>`,
		`<step>F</step>`,
	}
	for _, c := range checks {
		if !strings.Contains(xml, c) {
			t.Errorf("expected chord output to contain %q", c)
		}
	}
}

func TestCLICompoundTime(t *testing.T) {
	out, err := exec.Command("./m4bon", "M6/8 abc def").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	checks := []string{
		`<beats>6</beats>`,
		`<beat-type>8</beat-type>`,
		`<type>eighth</type>`,
	}
	for _, c := range checks {
		if !strings.Contains(xml, c) {
			t.Errorf("expected 6/8 output to contain %q", c)
		}
	}
	// Should have 6 notes, not 2
	count := strings.Count(xml, "<note>")
	if count != 6 {
		t.Errorf("expected 6 notes in 6/8, got %d", count)
	}
}

func TestCLIFileInput(t *testing.T) {
	// Write a temp DSL file
	dir := t.TempDir()
	dslPath := filepath.Join(dir, "test.dsl")
	os.WriteFile(dslPath, []byte("# comment\nc d e f\n"), 0644)

	out, err := exec.Command("./m4bon", "-f", dslPath).Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	if !strings.Contains(xml, `<step>C</step>`) {
		t.Errorf("expected file input to produce notes")
	}
}

func TestCLIOutputFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.mxl")

	_, err := exec.Command("./m4bon", "-o", outPath, "c d e f").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("cannot read output file: %v", err)
	}
	if !strings.Contains(string(data), `<step>C</step>`) {
		t.Errorf("output file should contain notes")
	}
}

func TestCLIEmptyInput(t *testing.T) {
	// No input should produce error
	err := exec.Command("./m4bon").Run()
	if err == nil {
		t.Errorf("expected error for empty input")
	}
}

func TestCLIKeySignature(t *testing.T) {
	out, err := exec.Command("./m4bon", "KE& c d e f").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	if !strings.Contains(xml, `<fifths>-3</fifths>`) {
		t.Errorf("expected E-flat major key signature (fifths=-3)")
	}
}

func TestCLIAccidentals(t *testing.T) {
	out, err := exec.Command("./m4bon", "#c &b %c").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	checks := []string{
		`<alter>1</alter>`,
		`<accidental>sharp</accidental>`,
		`<alter>-1</alter>`,
		`<accidental>flat</accidental>`,
	}
	for _, c := range checks {
		if !strings.Contains(xml, c) {
			t.Errorf("expected output to contain %q", c)
		}
	}
}

func TestCLIDoubleAccidentals(t *testing.T) {
	// Within one beat group
	out, err := exec.Command("./m4bon", "##c&&e").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	checks := []string{
		`<alter>2</alter>`,
		`<accidental>double-sharp</accidental>`,
		`<alter>-2</alter>`,
		`<accidental>flat-flat</accidental>`,
	}
	for _, c := range checks {
		if !strings.Contains(xml, c) {
			t.Errorf("expected output to contain %q", c)
		}
	}
}

func TestCLIInvalidInput(t *testing.T) {
	// No input should produce error
	err := exec.Command("./m4bon").Run()
	if err == nil {
		t.Errorf("expected error for empty input")
	}
}

func TestCLIDottedNote(t *testing.T) {
	// "a -b" should produce a dotted quarter A (3/8) + eighth B
	out, err := exec.Command("./m4bon", "a -b").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	if !strings.Contains(xml, `<duration>720</duration>`) {
		t.Errorf("expected dotted quarter (720 ticks) for A")
	}
	if !strings.Contains(xml, `<dot></dot>`) {
		t.Errorf("expected dot element for dotted quarter")
	}
	if !strings.Contains(xml, `<duration>240</duration>`) {
		t.Errorf("expected eighth (240 ticks) for B")
	}
}

func TestCLIBarlineSplit(t *testing.T) {
	// "a b - -c" - B extends across the invisible barline (beat 2→3)
	// Should split as quarter B tied to dotted quarter B
	out, err := exec.Command("./m4bon", "a b - -c").Output()
	if err != nil {
		t.Fatalf("m4bon failed: %v", err)
	}
	xml := string(out)

	// Two B notes: quarter (480) tied to dotted quarter (720)
	if !strings.Contains(xml, `<step>B</step>`) {
		t.Errorf("expected B notes")
	}
	// Count tie elements: should have tie-start on quarter B,
	// tie-stop on dotted quarter B
	startCount := strings.Count(xml, `<tie type="start">`)
	stopCount := strings.Count(xml, `<tie type="stop">`)
	if startCount < 1 {
		t.Errorf("expected at least 1 tie start, got %d", startCount)
	}
	if stopCount < 1 {
		t.Errorf("expected at least 1 tie stop, got %d", stopCount)
	}
}
