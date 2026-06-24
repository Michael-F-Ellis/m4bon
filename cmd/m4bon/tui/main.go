//go:build darwin && cgo

package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mellis/m4bon/midi"
	"github.com/mellis/m4bon/parser"
)

// Run starts the TUI application with the given DSL text and label.
// If dslText is empty, it starts in a non-functional state showing no measures.
// asciiLeaps controls whether leap indicators use ANSI escapes or Unicode diacritics.
func Run(dslText, label string, bpm float64, asciiLeaps bool) error {
	if dslText == "" {
		// Empty state - just show the TUI with no music loaded
		emptyMeasures := []parser.MeasureResult{
			{TimeNum: 4, TimeDen: 4, Events: nil, NumGroups: 0, GroupSlots: nil},
		}
		smfBytes, tl, err := midi.GenerateSMFWithOptions(emptyMeasures, bpm, midi.SMFOptions{Metronome: true})
		if err != nil {
			return fmt.Errorf("generate SMF: %w", err)
		}
		m := initialModel("", label, emptyMeasures, smfBytes, tl, asciiLeaps)
		m.smfBytes = nil // no SMF in empty state
		p := tea.NewProgram(m, tea.WithAltScreen())
		m.program = p
		_, err = p.Run()
		return err
	}

	// Sanitize and parse DSL
	sanitized := parser.SanitizeDSL(dslText)
	if sanitized == "" {
		return fmt.Errorf("empty DSL after sanitization")
	}

	result := parser.ParseDSL(sanitized)
	if result.Err != nil {
		return fmt.Errorf("parse error: %v", result.Err)
	}
	if len(result.Measures) == 0 {
		return fmt.Errorf("no measures produced")
	}

	// Generate SMF
	smfBytes, tl, err := midi.GenerateSMFWithOptions(result.Measures, bpm, midi.SMFOptions{Metronome: true})
	if err != nil {
		return fmt.Errorf("generate SMF: %w", err)
	}

	// Create model and load MIDI player
	m := initialModel(sanitized, label, result.Measures, smfBytes, tl, asciiLeaps)

	// Load MIDI player in background
	if err := m.loadMIDIPlayer(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load MIDI player: %v\n", err)
		fmt.Fprintf(os.Stderr, "TUI will run without audio.\n")
	}

	// Run BubbleTea
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.program = p
	_, err = p.Run()
	return err
}
