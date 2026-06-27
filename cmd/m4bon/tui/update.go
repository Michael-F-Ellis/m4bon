//go:build darwin && cgo

package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Michael-F-Ellis/macaudio"
	"github.com/mellis/m4bon/render"
)

// Message types for BubbleTea.
type (
	advanceIndicatorMsg int // measure index
	playbackEndedMsg    struct{}
)

// Update handles all messages and key press events.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case advanceIndicatorMsg:
		m.currentMeasure = int(msg)
		return m, nil

	case playbackEndedMsg:
		m.isPlaying = false
		m.isPaused = false
		m.elapsed = 0
		// Reset to start position
		m.currentMeasure = m.startMeasure
		return m, nil

	case positionMsg:
		if m.transport.State() != macaudio.StateIdle || m.isPlaying {
			m.elapsed = m.transport.Position()
			// Compute current measure from elapsed time
			m.currentMeasure = m.measureAtTime(m.elapsed)
			// State-based fallback: if transport stopped internally, handle end
			if m.isPlaying && m.transport.State() == macaudio.StateIdle {
				m.isPlaying = false
				m.isPaused = false
				m.elapsed = 0
				m.currentMeasure = m.startMeasure
				return m, nil
			}
		}
		if m.isPlaying || m.isPaused {
			return m, m.elapsedTick()
		}
		return m, nil

	case fileChangedMsg:
		if m.isPlaying {
			return m, m.watchFileTick()
		}
		m.sourceFileMod = msg.modTime
		if err := m.reloadMeasures(); err != "" {
			// Error silently ignored — score stays as-is
		}
		return m, m.watchFileTick()

	case watchTickMsg:
		return m, m.watchFileTick()

	case tea.QuitMsg:
		m.quitting = true
		m.cleanup()
		return m, tea.Quit
	}

	return m, nil
}

// handleKeyMsg processes keyboard input.
func (m *model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {

	case "q":
		m.quitting = true
		m.cleanup()
		return m, tea.Quit

	case "?":
		m.showHelp = !m.showHelp
		return m, nil

	case " ":
		return m.handlePlayPause()

	case "s":
		return m.handleStop()

	case "[":
		return m.handleTempoDelta(-5)

	case "]":
		return m.handleTempoDelta(5)

	case "{":
		return m.handleTempoDelta(-1)

	case "}":
		return m.handleTempoDelta(1)

	case "0":
		return m.handleTempoDelta(0)

	case "up":
		return m.handleSeekMeasure(-1)

	case "down":
		return m.handleSeekMeasure(1)

	case "shift+up":
		return m.handleEndMeasure(-1)

	case "shift+down":
		return m.handleEndMeasure(1)

	case "left":
		return m.handleVolumeDelta(-0.05)

	case "right":
		return m.handleVolumeDelta(0.05)

	case "j":
		return m.handleScroll(1)

	case "k":
		return m.handleScroll(-1)

	case "o":
		return m.handleSubscriptsToggle()

	case "c":
		return m.handleCommentsToggle()

	case "u":
		return m.handleReload()

	case "r":
		return m.handleRecordToggle()

	case "m":
		return m.handleMetronomeToggle()

	case "R":
		return m.handleRootsToggle()

	case "b":
		return m.handleBackbeatsToggle()
	}

	return m, nil
}

// --- Action handlers ---

func (m *model) handlePlayPause() (tea.Model, tea.Cmd) {
	if m.isRecording {
		return m, nil // space ignored during recording
	}

	if m.midiPlayer == nil {
		return m, nil
	}

	if m.isPlaying {
		// Pause
		m.transport.Pause()
		m.isPlaying = false
		m.isPaused = true
		return m, nil
	}

	if m.isPaused {
		// Resume
		m.transport.Play()
		m.isPlaying = true
		m.isPaused = false
		return m, m.elapsedTick()
	}

	// Fresh start: seek to start measure position
	if len(m.timeline.MeasureStarts) > 0 {
		seekIdx := m.startMeasure
		if seekIdx > m.endMeasure {
			seekIdx = m.endMeasure
		}
		m.elapsed = m.timeline.MeasureStarts[seekIdx]
	} else {
		m.elapsed = 0
	}

	m.currentMeasure = m.startMeasure

	m.transport.Stop()
	m.transport.Seek(m.elapsed)

	// Play segment from current measure to end measure
	endTime := m.timeline.TotalDuration
	if m.endMeasure+1 < len(m.timeline.MeasureStarts) {
		endTime = m.timeline.MeasureStarts[m.endMeasure+1]
	}
	m.transport.PlaySegment(m.elapsed, endTime)

	m.isPlaying = true
	m.isPaused = false
	return m, m.elapsedTick()
}

func (m *model) handleStop() (tea.Model, tea.Cmd) {
	if m.isRecording {
		return m.handleRecordToggle() // stop recording instead
	}
	m.transport.Stop()
	m.transport.Seek(0)
	m.isPlaying = false
	m.isPaused = false
	m.currentMeasure = m.startMeasure
	m.elapsed = 0
	return m, nil
}

func (m *model) handleTempoDelta(delta float64) (tea.Model, tea.Cmd) {
	if delta == 0 {
		m.bpm = 120
	} else {
		m.bpm += delta
		if m.bpm < 20 {
			m.bpm = 20
		}
		if m.bpm > 300 {
			m.bpm = 300
		}
	}
	if !m.isPlaying && !m.isPaused {
		m.regenerateSMF()
	}
	return m, nil
}

func (m *model) handleVolumeDelta(delta float64) (tea.Model, tea.Cmd) {
	m.volume += delta
	if m.volume < 0 {
		m.volume = 0
	}
	if m.volume > 1.0 {
		m.volume = 1.0
	}
	m.transport.SetVolume(m.volume)
	return m, nil
}

func (m *model) handleSeekMeasure(delta int) (tea.Model, tea.Cmd) {
	if m.midiPlayer == nil || len(m.timeline.MeasureStarts) == 0 {
		return m, nil
	}

	// If we're reviewing a recording, switch back to MIDI
	if m.recording != nil {
		m.recording.Stop()
		m.recording = nil
		m.transport.SetMIDIPlayer(m.midiPlayer)
	}

	newIdx := m.startMeasure + delta
	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx > m.endMeasure {
		newIdx = m.endMeasure
	}
	m.startMeasure = newIdx
	m.currentMeasure = newIdx
	seekTime := m.timeline.MeasureStarts[newIdx]
	m.transport.Seek(seekTime)
	m.elapsed = seekTime
	return m, nil
}

func (m *model) handleSubscriptsToggle() (tea.Model, tea.Cmd) {
	m.showSubscripts = !m.showSubscripts
	// Re-render with current subscript setting
	ansiOutput := render.Render(m.measures, m.asciiLeaps, m.showSubscripts, m.showComments)
	renderLines := strings.Split(ansiOutput, "\n")
	if len(renderLines) > 0 && renderLines[len(renderLines)-1] == "" {
		renderLines = renderLines[:len(renderLines)-1]
	}
	m.renderLines = renderLines
	return m, nil
}

func (m *model) handleCommentsToggle() (tea.Model, tea.Cmd) {
	m.showComments = !m.showComments
	ansiOutput := render.Render(m.measures, m.asciiLeaps, m.showSubscripts, m.showComments)
	renderLines := strings.Split(ansiOutput, "\n")
	if len(renderLines) > 0 && renderLines[len(renderLines)-1] == "" {
		renderLines = renderLines[:len(renderLines)-1]
	}
	m.renderLines = renderLines
	return m, nil
}

func (m *model) handleReload() (tea.Model, tea.Cmd) {
	if m.isPlaying {
		return m, nil
	}
	if err := m.reloadMeasures(); err != "" {
		// Error silently ignored — score stays as-is
	}
	return m, nil
}

func (m *model) handleRecordToggle() (tea.Model, tea.Cmd) {
	if m.isRecording {
		// Stop recording
		rec, err := m.recorder.Stop()
		if err != nil {
			// Silent fail — no recording produced
			m.isRecording = false
			m.recorder.Close()
			m.recorder = nil
			return m, nil
		}
		m.recording = rec
		m.isRecording = false
		m.recorder.Close()
		m.recorder = nil

		// Stop MIDI playback
		m.midiPlayer.Stop()
		m.midiPlayer.Seek(0)
		m.isPlaying = false
		m.isPaused = false
		m.elapsed = 0

		// Switch transport to recording for review
		m.transport.SetRecording(rec)
		return m, nil
	}

	// Start recording
	if m.midiPlayer == nil {
		return m, nil
	}

	// Create recorder on demand
	if m.recorder == nil {
		r, err := macaudio.NewRecorder()
		if err != nil {
			return m, nil
		}
		m.recorder = r
	}

	// Start mic recording
	if err := m.recorder.Start(""); err != nil {
		return m, nil
	}

	// Start MIDI as backing track
	if len(m.timeline.MeasureStarts) > 0 {
		seekIdx := m.startMeasure
		if seekIdx > m.endMeasure {
			seekIdx = m.endMeasure
		}
		m.elapsed = m.timeline.MeasureStarts[seekIdx]
	}
	m.currentMeasure = m.startMeasure
	m.midiPlayer.Stop()
	m.midiPlayer.Seek(m.elapsed)
	endTime := m.timeline.TotalDuration
	if m.endMeasure+1 < len(m.timeline.MeasureStarts) {
		endTime = m.timeline.MeasureStarts[m.endMeasure+1]
	}
	m.midiPlayer.PlaySegment(m.elapsed, endTime)

	m.isRecording = true
	m.isPlaying = true
	m.isPaused = false
	return m, m.elapsedTick()
}

func (m *model) handleMetronomeToggle() (tea.Model, tea.Cmd) {
	if m.isPlaying || m.isRecording {
		return m, nil // no toggle during playback
	}
	m.metronomeOn = !m.metronomeOn
	m.regenerateSMF()
	return m, nil
}

func (m *model) handleRootsToggle() (tea.Model, tea.Cmd) {
	if m.isPlaying || m.isRecording {
		return m, nil
	}
	m.rootsOn = !m.rootsOn
	m.regenerateSMF()
	return m, nil
}

func (m *model) handleBackbeatsToggle() (tea.Model, tea.Cmd) {
	if m.isPlaying || m.isRecording {
		return m, nil
	}
	m.backbeatsOn = !m.backbeatsOn
	m.regenerateSMF()
	return m, nil
}

func (m *model) handleEndMeasure(delta int) (tea.Model, tea.Cmd) {
	if len(m.timeline.MeasureStarts) == 0 {
		return m, nil
	}
	newIdx := m.endMeasure + delta
	if newIdx < m.startMeasure {
		newIdx = m.startMeasure
	}
	if newIdx >= len(m.timeline.MeasureStarts) {
		newIdx = len(m.timeline.MeasureStarts) - 1
	}
	m.endMeasure = newIdx
	return m, nil
}

func (m *model) handleScroll(delta int) (tea.Model, tea.Cmd) {
	m.viewportStart += delta
	if m.viewportStart < 0 {
		m.viewportStart = 0
	}
	if m.viewportStart >= len(m.renderLines) {
		m.viewportStart = len(m.renderLines) - 1
	}
	return m, nil
}
