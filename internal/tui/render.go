package tui

import (
	"fmt"
	"strings"
	"time"
)

// eraseLocked moves the cursor back to the start of the UI block and clears
// every line that was previously rendered. It resets lastLines to 0.
// Must be called with u.mu held.
func (u *UI) eraseLocked() {
	if u.lastLines == 0 {
		return
	}
	// Move up to the first line of our block.
	fmt.Fprintf(u.out, "\x1b[%dF", u.lastLines)
	// Clear from cursor to end of screen.
	fmt.Fprint(u.out, "\x1b[0J")
	u.lastLines = 0
}

// renderAllLocked erases the previous UI block and redraws it.
// Must be called with u.mu held.
func (u *UI) renderAllLocked() {
	if !u.enabled || u.sealed {
		return
	}

	lines := u.collectLines()

	// Erase the previous block (move up + clear-to-end).
	if u.lastLines > 0 {
		fmt.Fprintf(u.out, "\x1b[%dF", u.lastLines)
		fmt.Fprint(u.out, "\x1b[0J")
	}

	// Draw every line fresh. We write a trailing newline after each line so
	// the cursor always ends on a fresh line below the last rendered line.
	for _, line := range lines {
		fmt.Fprintf(u.out, "%s\n", line)
	}

	u.lastLines = len(lines)
}

func (u *UI) renderPhaseLine() string {
	percent := 0
	color := cCyan
	icon := "⚡"

	if u.phase != nil {
		switch u.phase.status {
		case "done":
			color = cGreen
			icon = "✨"
		case "error":
			color = cRed
			icon = "❌"
		}
		if u.phase.status == "done" || u.phase.status == "error" {
			percent = 100
		} else if u.phase.total > 0 {
			percent = int(float64(u.phase.step) / float64(u.phase.total) * 100)
		}
	}

	var bar string
	if u.phase != nil && u.phase.total == 0 && u.phase.status == "running" {
		bar = renderIndeterminateBar(barWidth, u.phaseElapsed(), color)
	} else {
		bar = renderBar(barWidth, percent, color)
	}
	elapsed := formatDuration(u.phaseElapsed())

	return fmt.Sprintf("%s %s%sPHASE: %s%s %s %s(%s)%s",
		icon, cBold, color, cReset, u.phase.name, bar, cCyan, elapsed, cReset)
}

func (u *UI) renderBuildLine(label string, state buildState) string {
	elapsed := formatDuration(buildElapsed(state))

	icon, _ := iconForStatus(state.status, buildElapsed(state))
	bar := barForStatus(state.status, buildElapsed(state))

	line := fmt.Sprintf("  %s [%s] %s %s(%s)%s", icon, bar, formatBuildLabel(label), cDim, elapsed, cReset)

	if state.status == "error" && state.detail != "" {
		line += fmt.Sprintf(" %s— %s%s", cDim, cRed+state.detail, cReset)
	} else if state.detail != "" {
		line += fmt.Sprintf(" %s— %s%s", cDim, cReset+state.detail, cReset)
	}
	return line
}

func (u *UI) phaseElapsed() time.Duration {
	if u.phase == nil {
		return 0
	}
	if u.phase.status == "done" || u.phase.status == "error" {
		return u.phase.duration
	}
	return time.Since(u.phase.start)
}

func iconForStatus(status string, elapsed time.Duration) (string, string) {
	switch status {
	case "running":
		step := int(elapsed.Milliseconds() / 80)
		frame := string(spinnerFrames[step%len(spinnerFrames)])
		return cYellow + frame + cReset, cYellow
	case "done":
		return cGreen + "✓" + cReset, cGreen
	case "error":
		return cRed + "✗" + cReset, cRed
	default:
		return cDim + "○" + cReset, cDim
	}
}

func barForStatus(status string, elapsed time.Duration) string {
	percent := 0
	color := cDim

	switch status {
	case "running":
		color = cYellow
		return renderIndeterminateBar(barWidth, elapsed, color)
	case "done":
		percent = 100
		color = cGreen
	case "error":
		percent = 100
		color = cRed
	}

	return renderBar(barWidth-8, percent, color)
}

func (u *UI) collectLines() []string {
	var lines []string

	// Phase section
	if u.phase != nil {
		lines = append(lines, u.trimLine(u.renderPhaseLine()))
		for _, line := range u.phase.logs {
			lines = append(lines, fmt.Sprintf("    %s│%s %s%s%s", cCyan, cReset, cDim, u.trimLine(line), cReset))
		}
	}

	// Builds section – only add the blank separator when both sections are non-empty
	if len(u.builds) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		labels := u.order
		if len(labels) == 0 {
			labels = make([]string, 0, len(u.builds))
			for label := range u.builds {
				labels = append(labels, label)
			}
			labels = stableSort(labels)
		}
		for _, label := range labels {
			state := u.builds[label]
			lines = append(lines, u.trimLine(u.renderBuildLine(label, state)))
			for _, logLine := range state.logs {
				lines = append(lines, fmt.Sprintf("      %s│%s %s%s%s", cDim, cReset, cDim, u.trimLine(logLine), cReset))
			}
		}
	}

	return lines
}

func renderBar(width, percent int, color string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	fill := int(float64(width) * (float64(percent) / 100))

	filled := color + strings.Repeat("━", fill) + cReset
	empty := cDim + strings.Repeat("─", width-fill) + cReset

	return filled + empty
}

func renderIndeterminateBar(width int, elapsed time.Duration, color string) string {
	if width <= 2 {
		return ".."
	}
	if width > 15 {
		width = 15
	}

	runes := make([]rune, width)
	for i := range runes {
		runes[i] = '─'
	}

	step := int(elapsed.Milliseconds() / 80)
	pos := 0
	travel := width - 1
	if travel > 0 {
		period := travel * 2
		phase := step % period
		if phase <= travel {
			pos = phase
		} else {
			pos = period - phase
		}
	}

	var sb strings.Builder
	sb.WriteString(cDim)
	for i, r := range runes {
		if i == pos {
			sb.WriteString(cReset + color + "●" + cReset + cDim)
		} else {
			sb.WriteRune(r)
		}
	}
	sb.WriteString(cReset)

	return sb.String()
}

func buildElapsed(state buildState) time.Duration {
	if state.status == "done" || state.status == "error" {
		if state.dur > 0 {
			return state.dur
		}
	}
	if state.start.IsZero() {
		return 0
	}
	return time.Since(state.start)
}
