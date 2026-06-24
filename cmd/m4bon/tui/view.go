//go:build darwin && cgo

package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/mellis/m4bon"
)


// visibleLen returns the length of a string with ANSI escape codes removed
// and all Unicode combining characters (like subscripts) counted as zero-width.
func visibleLen(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Skip ANSI sequence: ESC[...m
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		r, _ := runeAt(s, i)
		if r != -1 {
			// Subscripts and combining marks are zero-width
			if !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Me, r) {
				n++
			}
		}
	}
	return n
}

// runeAt returns the rune at byte index i in s, or -1 if invalid.
func runeAt(s string, i int) (rune, int) {
	if i >= len(s) {
		return -1, 0
	}
	r, size := rune(s[i]), 1
	if r > 0x7f {
		r, size = decodeRune(s[i:])
	}
	return r, size
}
func decodeRune(s string) (rune, int) {
	if len(s) == 0 {
		return -1, 0
	}
	b := s[0]
	switch {
	case b < 0x80:
		return rune(b), 1
	case b < 0xC0:
		return rune(b), 1 // continuation byte, treat as latin1
	case b < 0xE0:
		if len(s) < 2 {
			return rune(b), 1
		}
		return rune(b&0x1F)<<6 | rune(s[1]&0x3F), 2
	case b < 0xF0:
		if len(s) < 3 {
			return rune(b), 1
		}
		return rune(b&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	default:
		if len(s) < 4 {
			return rune(b), 1
		}
		return rune(b&0x07)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F), 4
	}
}

// truncateVisible truncates s to at most maxVis visible characters,
// preserving ANSI codes and avoiding mid-rune splits.
func truncateVisible(s string, maxVis int) string {
	visCount := 0
	end := len(s)
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Skip ANSI sequence
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++ // skip 'm'
			}
			continue
		}
		if visCount >= maxVis {
			end = i
			break
		}
		r, size := runeAt(s, i)
		if r == -1 {
			i++
			continue
		}
		if !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Me, r) {
			visCount++
		}
		i += size
	}
	if end >= len(s) {
		return s
	}
	return s[:end] + "..."
}

var (
	styleTopBar = lipgloss.NewStyle().
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	styleIndicator = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Bold(true)

	styleEndIndicator = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Bold(true)

	styleHelp = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	styleMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// topBar renders the top bar with time sig, key sig, tempo, volume.
func (m *model) topBar() string {
	var parts []string

	// Time signature from first measure
	if len(m.measures) > 0 {
		parts = append(parts, fmt.Sprintf("M%d/%d", m.measures[0].TimeNum, m.measures[0].TimeDen))
	}

	// Key signature
	if len(m.measures) > 0 {
		f := m.measures[0].Fifths
		if f > 0 {
			parts = append(parts, fmt.Sprintf("K +%d (sharps)", f))
		} else if f < 0 {
			parts = append(parts, fmt.Sprintf("K %d (flats)", f))
		} else {
			parts = append(parts, "K C")
		}
	}

	// Tempo
	parts = append(parts, fmt.Sprintf("♩=%.0f", m.bpm))

	// Volume
	parts = append(parts, fmt.Sprintf("vol:%.0f%%", m.volume*100))

	if m.metronomeOn {
		parts = append(parts, "click:on")
	} else {
		parts = append(parts, "click:off")
	}
	if m.rootsOn {
		parts = append(parts, "roots:on")
	}
	if m.backbeatsOn {
		parts = append(parts, "backbeats")
	}

	bar := strings.Join(parts, "  │  ")

	// Truncate to fit width
	if m.width > 0 && len(bar) > m.width-4 {
		bar = bar[:m.width-7] + "..."
	}

	return styleTopBar.Render(" m4bon v" + m4bon.Version + "  │  " + bar)
}

// measureView renders the measure lines with indicator and scroll.
// Lines already contain ANSI color codes from render.Render().
func (m *model) measureView() string {
	if len(m.renderLines) == 0 {
		return styleMuted.Render("  No measures loaded.")
	}

	// Determine visible range
	visible := m.height - 4 // top bar + status bar + padding
	if visible < 1 {
		visible = 5
	}
	if visible > len(m.renderLines) {
		visible = len(m.renderLines)
	}

	// Center the current measure if possible
	end := m.viewportStart + visible
	if end > len(m.renderLines) {
		end = len(m.renderLines)
		m.viewportStart = end - visible
	}
	if m.viewportStart < 0 {
		m.viewportStart = 0
	}

	var b strings.Builder
	for i := m.viewportStart; i < end; i++ {
		line := m.renderLines[i]
		// Truncate to fit width (measured by visible characters, not bytes)
		if m.width > 10 && visibleLen(line) > m.width-6 {
			line = truncateVisible(line, m.width-9)
		}

		if i == m.currentMeasure && m.isPlaying {
			b.WriteString(styleIndicator.Render("◉ "))
		} else if i == m.startMeasure && i == m.endMeasure {
			b.WriteString(styleIndicator.Render("▶"))
			b.WriteString(styleEndIndicator.Render("▷"))
		} else if i == m.startMeasure {
			b.WriteString(styleIndicator.Render("▶ "))
		} else if i == m.endMeasure {
			b.WriteString(styleEndIndicator.Render(" ▷"))
		} else {
			b.WriteString("   ")
		}

		// Write raw line (already ANSI-colored from render.Render)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// statusBar renders the bottom status line.
func (m *model) statusBar() string {
	var transport string
	switch {
	case m.isRecording:
		transport = "● REC"
	case m.isPlaying:
		if m.transport.Active() == "recording" {
			transport = "▶ Review"
		} else {
			transport = "▶ Playing"
		}
	default:
		transport = "■ Stopped"
	}

	// Measure range
	rangeInfo := fmt.Sprintf("Measures %d-%d", m.startMeasure+1, m.endMeasure+1)

	// Elapsed time
	elapsed := formatDuration(m.elapsed)
	total := formatDuration(m.timeline.TotalDuration)
	timeInfo := fmt.Sprintf("%s / %s", elapsed, total)

	left := fmt.Sprintf("  %s  │  %s  │  %s", transport, rangeInfo, timeInfo)

	// Pad to full width
	if m.width > len(left) {
		left = left + strings.Repeat(" ", m.width-len(left))
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("248")).
		Render(left)
}

// helpView renders the help overlay.
func (m *model) helpView() string {
	helpText := `
  Key Bindings:

  space    Play / Pause
  s        Stop
  r        Start / stop recording
  m        Toggle metronome
  b        Toggle backbeats (click on 2 and 4)
  R        Toggle chord roots
  [ / ]    Tempo -5 / +5 BPM
  { / }    Tempo -1 / +1 BPM
  0        Reset tempo to 120
  ↑ / ↓    Seek -1 / +1 measure (start ▷)
  ⇧↑ / ⇧↓  End -1 / +1 measure (end ▷)
  ← / →    Volume down / up
  j / k    Scroll down / up
  o        Toggle octave subscripts
  u        Reload from source
  q        Quit
  ?        Toggle this help
`

	return styleHelp.Render(helpText)
}

// View renders the complete TUI.
func (m *model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Top bar
	titleWidth := m.width
	if titleWidth <= 0 {
		titleWidth = 80
	}
	m.width = titleWidth

	b.WriteString(m.topBar())
	b.WriteString("\n\n")

	// Main content (measures or help)
	if m.showHelp {
		b.WriteString(m.helpView())
	} else {
		b.WriteString(m.measureView())
	}

	b.WriteString("\n")

	// Bottom status bar
	b.WriteString(m.statusBar())

	return b.String()
}

// formatDuration formats a duration as mm:ss.d
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSec := d.Seconds()
	min := int(totalSec) / 60
	sec := int(totalSec) % 60
	dec := int((totalSec - float64(int(totalSec))) * 10)
	return fmt.Sprintf("%02d:%02d.%d", min, sec, dec)
}
