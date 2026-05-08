package tui

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
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
	// sealed prevents any further rendering (after Close/BuildEnd)
	sealed bool
	// buildStart is set when BuildStart is called, used for the final report
	buildStart time.Time
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
	u.buildStart = time.Now()
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

// BuildEnd finalises the build section: clears the live UI block, prints a
// static summary report, then re-enables the cursor. Calling it twice is a no-op.
func (u *UI) BuildEnd() {
	if !u.enabled {
		return
	}
	// Stop the ticker outside the lock to avoid a deadlock with the ticker
	// goroutine (which also acquires u.mu). This ensures no further renders
	// can race with the static report we are about to print.
	u.stopTicker()

	u.mu.Lock()
	defer u.mu.Unlock()
	if u.sealed {
		return
	}
	u.sealed = true

	// Erase the live block before printing the static report.
	u.eraseLocked()
	u.printReportLocked()
	fmt.Fprint(u.out, cursorShow)
}

// printReportLocked prints a static, permanent build summary.
// Must be called with u.mu held.
func (u *UI) printReportLocked() {
	total := time.Since(u.buildStart)
	if u.buildStart.IsZero() {
		total = 0
	}

	labels := u.order
	if len(labels) == 0 {
		for l := range u.builds {
			labels = append(labels, l)
		}
		labels = stableSort(labels)
	}

	ok, failed := 0, 0
	for _, l := range labels {
		if u.builds[l].status == "done" {
			ok++
		} else if u.builds[l].status == "error" {
			failed++
		}
	}

	// Header line
	if failed > 0 {
		fmt.Fprintf(u.out, "%s%s✗ Build failed%s", cBold, cRed, cReset)
	} else {
		fmt.Fprintf(u.out, "%s%s✓ Build complete%s", cBold, cGreen, cReset)
	}
	fmt.Fprintf(u.out, "  %s%d built", cDim, ok)
	if failed > 0 {
		fmt.Fprintf(u.out, "  %d failed", failed)
	}
	fmt.Fprintf(u.out, "  total %s%s\n", formatDuration(total), cReset)

	// Per-target rows
	for _, label := range labels {
		state := u.builds[label]
		dur := buildElapsed(state)

		var icon, col string
		switch state.status {
		case "done":
			icon, col = "✓", cGreen
		case "error":
			icon, col = "✗", cRed
		default:
			icon, col = "○", cDim
		}

		fmt.Fprintf(u.out, "  %s%s%s  %s  %s%s%s\n",
			col, icon, cReset,
			formatBuildLabel(label),
			cDim, formatDuration(dur), cReset,
		)

		// Print error detail and logs under any failed target
		if state.status == "error" {
			if state.detail != "" {
				fmt.Fprintf(u.out, "     %s%s%s\n", cRed, state.detail, cReset)
			}
			for _, line := range state.logs {
				fmt.Fprintf(u.out, "     %s%s%s\n", cDim, line, cReset)
			}
		}
	}
	fmt.Fprintln(u.out)
}

func (u *UI) Close() {
	if !u.enabled {
		return
	}
	u.stopTicker()
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.sealed {
		u.eraseLocked()
	}
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
	// Re-render so the new log line appears immediately.
	p.ui.renderAllLocked()
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
	// Re-render so log appears immediately.
	u.renderAllLocked()
}

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

func (u *UI) startTicker() {
	if u.ticker != nil {
		return
	}
	u.ticker = time.NewTicker(80 * time.Millisecond)
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

// trimLine truncates a line to the terminal width while accounting for ANSI
// escape codes (which have zero visible width) and multi-byte runes.
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

	// Rebuild the string rune-by-rune, skipping escape sequences, until we
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

func getTerminalSize() (width, height int, err error) {
	type winsize struct {
		rows uint16
		cols uint16
		x    uint16
		y    uint16
	}
	ws := &winsize{}
	retCode, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdout), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if retCode != 0 {
		return 80, 24, errno
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

	return fmt.Sprintf("%s%s%s %s/%s %s%s%s",
		osColor, osPart, cReset, cDim, cReset, cYellow, archPart, cReset)
}
