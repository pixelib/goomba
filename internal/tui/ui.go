package tui

import (
	"fmt"
	"io"
	"sync"
	"time"
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
