package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

func stableSort(items []string) []string {
	out := append([]string(nil), items...)
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func appendLog(lines []string, line string, limit int) []string {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return lines
	}
	lines = append(lines, line)
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// stripANSI returns the visible rune count of a string, ignoring ANSI escape sequences.
func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		n++
	}
	return n
}

func (u *UI) trimLine(line string) string {
	if u.verbose {
		return line
	}

	termWidth := 80
	if w, _, err := getTerminalSize(); err == nil && w > 0 {
		termWidth = w
	}

	if visibleLen(line) <= termWidth {
		return line
	}

	// rebuild the string rune-by-rune, skipping escape sequences, until we
	// reach termWidth-3 visible characters, then append "...".
	var sb strings.Builder
	visible := 0
	inEsc := false
	limit := termWidth - 3

	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		i += size

		if inEsc {
			sb.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			sb.WriteRune(r)
			continue
		}

		if visible >= limit {
			sb.WriteString(cReset + "...")
			return sb.String()
		}
		sb.WriteRune(r)
		visible++
	}

	return sb.String()
}

func formatBuildLabel(label string) string {
	if !strings.HasPrefix(label, "Compiling ") {
		return cCyan + label + cReset
	}
	parts := strings.Fields(label)
	if len(parts) < 3 {
		return cCyan + label + cReset
	}

	osPart := strings.TrimPrefix(parts[1], "os:")
	archPart := strings.TrimPrefix(parts[2], "arch:")

	osColor := cCyan
	switch osPart {
	case "linux":
		osColor = cGreen
	case "windows":
		osColor = cBlue
	case "macos", "darwin":
		osColor = cMagenta
	}

	return fmt.Sprintf("%s%s%s %s/%s %s%s%s",
		osColor, osPart, cReset, cDim, cReset, cYellow, archPart, cReset)
}
