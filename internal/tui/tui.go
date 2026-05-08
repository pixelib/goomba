package tui

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type UI struct {
	enabled   bool
	out       io.Writer
	mu        sync.Mutex
	lastLines int
	phaseID   int
	phase     *phaseState
	builds    map[string]buildState
	order     []string
	ticker    *time.Ticker
	tickDone  chan struct{}
	logLimit  int
	verbose   bool
}

type phaseState struct {
	id       int
	name     string
	status   string
	step     int
	total    int
	start    time.Time
	duration time.Duration
	logs     []string
}

type Phase struct {
	ui *UI
	id int
}

type buildState struct {
	status string
	detail string
	logs   []string
	start  time.Time
	dur    time.Duration
}

// Visual and color constants
const (
	cReset   = "\x1b[0m"
	cBold    = "\x1b[1m"
	cDim     = "\x1b[2m"
	cRed     = "\x1b[31m"
	cGreen   = "\x1b[32m"
	cYellow  = "\x1b[33m"
	cBlue    = "\x1b[34m"
	cMagenta = "\x1b[35m"
	cCyan    = "\x1b[36m"

	cursorHide = "\x1b[?25l"
	cursorShow = "\x1b[?25h"

	barWidth = 24
)

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func New(enabled bool, out io.Writer) *UI {
	ui := &UI{enabled: enabled, out: out, builds: map[string]buildState{}, logLimit: 5}
	if enabled {
		fmt.Fprint(ui.out, cursorHide)
		ui.startTicker()
	}
	return ui
}

func (u *UI) SetLogLimit(limit int) {
	if limit <= 0 {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.logLimit = limit
}

func (u *UI) NewPhase(name string, total int) *Phase {
	if !u.enabled {
		fmt.Fprintf(u.out, "phase: %s\n", name)
		return &Phase{}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.phaseID++
	u.phase = &phaseState{
		id:     u.phaseID,
		name:   name,
		status: "running",
		step:   0,
		total:  total,
		start:  time.Now(),
	}
	u.renderAllLocked()
	return &Phase{ui: u, id: u.phaseID}
}

func (u *UI) BuildStart(labels []string) {
	if !u.enabled {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.builds = map[string]buildState{}
	u.order = append([]string(nil), labels...)
	for _, label := range labels {
		u.builds[label] = buildState{status: "queued"}
	}
	u.renderAllLocked()
}

func (u *UI) BuildUpdate(label, status, detail string) {
	if !u.enabled {
		if status == "running" {
			fmt.Fprintf(u.out, "building %s\n", label)
		}
		if status == "error" {
			fmt.Fprintf(u.out, "error %s: %s\n", label, detail)
		}
		if status == "done" {
			fmt.Fprintf(u.out, "done %s\n", label)
		}
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	state := u.builds[label]
	state.status = status
	state.detail = detail
	if status == "running" && state.start.IsZero() {
		state.start = time.Now()
	}
	if (status == "done" || status == "error") && !state.start.IsZero() {
		state.dur = time.Since(state.start)
	}
	u.builds[label] = state
	u.renderAllLocked()
}

func (u *UI) BuildEnd() {
	if !u.enabled {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.renderAllLocked()
	fmt.Fprintln(u.out)
	fmt.Fprint(u.out, cursorShow)
	u.lastLines = 0
}

func (u *UI) Close() {
	if !u.enabled {
		return
	}
	u.stopTicker()
	fmt.Fprint(u.out, cursorShow)
}

func (p *Phase) Advance() {
	if p.ui == nil || !p.ui.enabled {
		return
	}
	p.ui.mu.Lock()
	defer p.ui.mu.Unlock()
	if p.ui.phase == nil || p.ui.phase.id != p.id {
		return
	}
	if p.ui.phase.total == 0 {
		return
	}
	p.ui.phase.step++
	p.ui.renderAllLocked()
}

func (p *Phase) Log(line string) {
	if p.ui == nil || !p.ui.enabled {
		return
	}
	p.ui.mu.Lock()
	defer p.ui.mu.Unlock()
	if p.ui.phase == nil || p.ui.phase.id != p.id {
		return
	}
	p.ui.phase.logs = appendLog(p.ui.phase.logs, line, p.ui.logLimit)
}

func (p *Phase) Done() {
	if p.ui == nil || !p.ui.enabled {
		return
	}
	p.ui.mu.Lock()
	defer p.ui.mu.Unlock()
	if p.ui.phase == nil || p.ui.phase.id != p.id {
		return
	}
	p.ui.phase.status = "done"
	p.ui.phase.duration = time.Since(p.ui.phase.start)
	if p.ui.phase.total > 0 {
		p.ui.phase.step = p.ui.phase.total
	}
	p.ui.renderAllLocked()
}

func (p *Phase) Fail(err error) {
	if p.ui == nil || !p.ui.enabled {
		return
	}
	p.ui.mu.Lock()
	defer p.ui.mu.Unlock()
	if p.ui.phase == nil || p.ui.phase.id != p.id {
		return
	}
	if err != nil {
		p.ui.phase.logs = appendLog(p.ui.phase.logs, err.Error(), p.ui.logLimit)
	}
	p.ui.phase.status = "error"
	p.ui.phase.duration = time.Since(p.ui.phase.start)
	p.ui.renderAllLocked()
}

func (u *UI) BuildLog(label, line string) {
	if !u.enabled {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	state := u.builds[label]
	state.logs = appendLog(state.logs, line, u.logLimit)
	u.builds[label] = state
}

func (u *UI) renderAllLocked() {
	if !u.enabled {
		return
	}
	// Move cursor to the start of the UI block
	if u.lastLines > 0 {
		fmt.Fprintf(u.out, "\x1b[%dF", u.lastLines)
	}

	lines := u.collectLines()
	for _, line := range lines {
		// \r = carriage return (start of line)
		// \x1b[2K = clear entire line
		fmt.Fprintf(u.out, "\r\x1b[2K%s\n", line)
	}
	u.lastLines = len(lines)
}

func (u *UI) startTicker() {
	if u.ticker != nil {
		return
	}
	u.ticker = time.NewTicker(80 * time.Millisecond) // Faster tick for smooth spinners
	u.tickDone = make(chan struct{})
	go func() {
		for {
			select {
			case <-u.ticker.C:
				u.mu.Lock()
				if u.shouldTickLocked() {
					u.renderAllLocked()
				}
				u.mu.Unlock()
			case <-u.tickDone:
				return
			}
		}
	}()
}

func (u *UI) stopTicker() {
	if u.ticker == nil {
		return
	}
	u.ticker.Stop()
	close(u.tickDone)
	u.ticker = nil
	u.tickDone = nil
}

func (u *UI) shouldTickLocked() bool {
	if u.phase != nil && u.phase.status == "running" {
		return true
	}
	for _, state := range u.builds {
		if state.status == "running" {
			return true
		}
	}
	return false
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
		case "running":
			color = cCyan
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
		line += fmt.Sprintf(" %s— %s%s", cDim, cRed, state.detail)
	} else if state.detail != "" {
		line += fmt.Sprintf(" %s— %s%s", cDim, cReset, state.detail)
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

	// Render Phase Output
	if u.phase != nil {
		lines = append(lines, trimLine(u.renderPhaseLine(), u))
		for _, line := range u.phase.logs {
			// Added colored pipe and dimmed log text
			lines = append(lines, fmt.Sprintf("    %s│%s %s%s%s", cCyan, cReset, cDim, trimLine(line, u), cReset))
		}
	}

	// Render Individual Builds Output
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
			lines = append(lines, trimLine(u.renderBuildLine(label, state), u))
			for _, logLine := range state.logs {
				// Added colored pipe for sub-items
				lines = append(lines, fmt.Sprintf("      %s│%s %s%s%s", cDim, cReset, cDim, trimLine(logLine, u), cReset))
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

	// Using dots/lines for a slimmer profile
	// Completed: ━━━ Unfinished: ───
	filled := color + strings.Repeat("━", fill) + cReset
	empty := cDim + strings.Repeat("─", width-fill) + cReset

	return filled + empty
}

func renderIndeterminateBar(width int, elapsed time.Duration, color string) string {
	// A more elegant "glimmer" effect using a moving dot on a thin line
	if width <= 2 {
		return ".."
	}

	// cap width at 15
	if width > 15 {
		width = 15
	}

	// Create the background line
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

	// We build the string manually to ensure no slicing issues
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

func clearLine(w io.Writer) {
	fmt.Fprint(w, "\r\x1b[2K")
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func trimLine(line string, ui *UI) string {
	if ui.verbose {
		return line
	}

	// get current terminal width if possible, otherwise use a reasonable default
	var termWidth int
	if w, _, err := getTerminalSize(); err == nil {
		termWidth = w
	} else {
		termWidth = 80
	}
	if len(line) <= termWidth {
		return line
	}
	return line[:termWidth-3] + "..."
}

func getTerminalSize() (width, height int, err error) {
	// This is a best effort to get terminal size, it may not work in all environments
	// and is not critical for functionality, so we ignore errors and return defaults
	type winsize struct {
		rows uint16
		cols uint16
		x    uint16
		y    uint16
	}
	ws := &winsize{}
	retCode, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdout), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if retCode != 0 {
		return 80, 24, err
	}
	return int(ws.cols), int(ws.rows), nil
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

	// Outputs clean layout: linux / amd64
	return fmt.Sprintf("%s%s%s %s/%s %s%s%s",
		osColor, osPart, cReset, cDim, cReset, cYellow, archPart, cReset)
}
