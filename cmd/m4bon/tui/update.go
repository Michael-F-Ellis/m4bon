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
		if m.midiPlayer != nil {
			m.elapsed = m.midiPlayer.Position()
			// Compute current measure from elapsed time
			m.currentMeasure = m.measureAtTime(m.elapsed)
			// State-based fallback: if player stopped internally, handle end
			if m.isPlaying && m.midiPlayer.State() == macaudio.StateIdle {
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

	case "u":
		return m.handleReload()
	}

	return m, nil
}

// --- Action handlers ---

func (m *model) handlePlayPause() (tea.Model, tea.Cmd) {
	if m.midiPlayer == nil {
		return m, nil
	}

	if m.isPlaying {
		// Pause
		m.midiPlayer.Pause()
		m.isPlaying = false
		m.isPaused = true
		return m, nil
	}

	if m.isPaused {
		// Resume
		m.midiPlayer.Play()
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

	m.midiPlayer.Stop()
	m.midiPlayer.Seek(m.elapsed)

	// Play segment from current measure to end measure
	endTime := m.timeline.TotalDuration
	if m.endMeasure+1 < len(m.timeline.MeasureStarts) {
		endTime = m.timeline.MeasureStarts[m.endMeasure+1]
	}
	m.midiPlayer.PlaySegment(m.elapsed, endTime)

	m.isPlaying = true
	m.isPaused = false
	return m, m.elapsedTick()
}

func (m *model) handleStop() (tea.Model, tea.Cmd) {
	if m.midiPlayer != nil {
		m.midiPlayer.Stop()
		m.midiPlayer.Seek(0)
	}

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
	if m.midiPlayer != nil {
		m.midiPlayer.SetVolume(m.volume)
	}
	return m, nil
}

func (m *model) handleSeekMeasure(delta int) (tea.Model, tea.Cmd) {
	if m.midiPlayer == nil || len(m.timeline.MeasureStarts) == 0 {
		return m, nil
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
	m.midiPlayer.Seek(seekTime)
	m.elapsed = seekTime
	return m, nil
}

func (m *model) handleSubscriptsToggle() (tea.Model, tea.Cmd) {
	m.showSubscripts = !m.showSubscripts
	// Re-render with current subscript setting
	ansiOutput := render.Render(m.measures, m.asciiLeaps, m.showSubscripts)
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
