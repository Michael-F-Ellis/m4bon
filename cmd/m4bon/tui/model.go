//go:build darwin && cgo

package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Michael-F-Ellis/macaudio"
	"github.com/mellis/m4bon/midi"
	"github.com/mellis/m4bon/parser"
	"github.com/mellis/m4bon/render"
)

// model is the BubbleTea model for the m4bon TUI.
type model struct {
	// DSL source
	dslText  string
	dslLabel string // display label (filename or "untitled")

	// Parsed data
	measures    []parser.MeasureResult
	renderLines []string // ANSI-colored, one per measure (from render.Render)

	// MIDI & timeline
	smfBytes []byte
	midiFile string // path to temp file for MIDI playback
	timeline midi.Timeline
	bpm      float64
	volume   float64 // 0.0 - 1.0

	// macaudio
	transport  *macaudio.Transport
	midiPlayer macaudio.MIDIPlayer

	// BubbleTea program reference — set after NewProgram for scheduler msg sending
	program *tea.Program

	// Playback state
	isPlaying bool
	isPaused  bool

	// Indicators
	startMeasure   int // 0-based start indicator position (moved by ↑/↓)
	endMeasure     int // 0-based end indicator position (moved by ⇧↑/⇧↓)
	currentMeasure int // 0-based live playback position (updated during play)

	// Recording
	isRecording bool
	recordStart int // measure number to start recording (0-based)
	recordEnd   int // measure number to stop recording (inclusive)
	recorder    macaudio.Recorder
	recording   *macaudio.Recording

	// UI state
	width         int
	height        int
	viewportStart int // scroll offset for measure lines
	showHelp      bool
	quitting      bool

	// Time display
	elapsed time.Duration
}

func initialModel(dslText, dslLabel string, measures []parser.MeasureResult, smfBytes []byte, tl midi.Timeline) *model {
	// Generate ANSI-colored render lines using the same render pipeline as -render
	ansiOutput := render.Render(measures)
	renderLines := strings.Split(ansiOutput, "\n")
	// Remove trailing empty line from split
	if len(renderLines) > 0 && renderLines[len(renderLines)-1] == "" {
		renderLines = renderLines[:len(renderLines)-1]
	}

	m := &model{
		dslText:         dslText,
		dslLabel:        dslLabel,
		measures:        measures,
		renderLines:     renderLines,
		smfBytes:        smfBytes,
		timeline:        tl,
		bpm:             tl.TempoBPM,
		volume:          0.8,
		transport:       macaudio.NewTransport(),
		startMeasure:    0,
		currentMeasure:  0,
	}

	if len(measures) > 0 {
		m.endMeasure = len(measures) - 1
	}

	return m
}

// Init initializes the BubbleTea program.
func (m *model) Init() tea.Cmd {
	return nil
}

// loadMIDIPlayer creates a temp file, writes SMF bytes, and loads into MIDIPlayer.
func (m *model) loadMIDIPlayer() error {
	if m.midiPlayer != nil {
		m.transport.SetMIDIPlayer(nil)
		m.midiPlayer.Close()
		m.midiPlayer = nil
	}

	if m.midiFile != "" {
		os.Remove(m.midiFile)
	}

	tmpFile, err := os.CreateTemp("", "m4bon-*.mid")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	m.midiFile = tmpFile.Name()
	if _, err := tmpFile.Write(m.smfBytes); err != nil {
		tmpFile.Close()
		os.Remove(m.midiFile)
		return fmt.Errorf("write SMF: %w", err)
	}
	tmpFile.Close()

	player, err := macaudio.NewMIDIPlayer()
	if err != nil {
		os.Remove(m.midiFile)
		return fmt.Errorf("create MIDI player: %w", err)
	}

	if err := player.Load(m.midiFile); err != nil {
		player.Close()
		os.Remove(m.midiFile)
		return fmt.Errorf("load MIDI: %w", err)
	}

	player.SetVolume(m.volume)

	m.midiPlayer = player
	m.transport.SetMIDIPlayer(player)

	return nil
}

// regenerateSMF regenerates the SMF at the current BPM and reloads it.
func (m *model) regenerateSMF() error {
	data, tl, err := midi.GenerateSMF(m.measures, m.bpm)
	if err != nil {
		return fmt.Errorf("regenerate SMF: %w", err)
	}
	m.smfBytes = data
	m.timeline = tl

	var currentPos time.Duration
	if m.midiPlayer != nil {
		currentPos = m.midiPlayer.Position()
	}

	if err := m.loadMIDIPlayer(); err != nil {
		return err
	}

	if m.midiPlayer != nil && currentPos > 0 {
		m.midiPlayer.Seek(currentPos)
	}

	return nil
}

// elapsedTick returns a command that polls position periodically.
func (m *model) elapsedTick() tea.Cmd {
	return tea.Every(100*time.Millisecond, func(t time.Time) tea.Msg {
		return positionMsg{time.Now()}
	})
}

// positionMsg carries a timestamp for position polling.
type positionMsg struct{ at time.Time }

// cleanup removes temp files and releases audio resources.
func (m *model) cleanup() {
	if m.midiPlayer != nil {
		m.transport.SetMIDIPlayer(nil)
		m.midiPlayer.Close()
	}
	if m.recorder != nil {
		m.recorder.Close()
	}
	if m.midiFile != "" {
		os.Remove(m.midiFile)
	}
}
