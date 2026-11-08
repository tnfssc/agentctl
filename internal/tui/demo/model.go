// Package demo is a fake proxy boot sequence that shows off the TUI stack:
// an animated gradient banner, spinners, a spring-driven progress bar and
// status rows that change as scripted events arrive.
package demo

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/cliproxy-cli/internal/tui/theme"
)

const (
	fps       = 30
	rowsWidth = 58
	backoff   = 700 * time.Millisecond
)

type status int

const (
	queued status = iota
	connecting
	retrying
	ready
)

type upstream struct {
	name    string
	model   string
	startAt time.Duration // after the run begins
	connect time.Duration // how long each handshake takes
	latency time.Duration // reported once ready
	flaky   bool          // first handshake is rate limited

	status   status
	attempts int
}

// script is the fake boot sequence. Nothing here touches the network.
var script = []upstream{
	{name: "claude", model: "claude-opus-5-5", startAt: 300 * time.Millisecond, connect: 1500 * time.Millisecond, latency: 142 * time.Millisecond},
	{name: "gemini", model: "gemini-3-pro", startAt: 650 * time.Millisecond, connect: 1100 * time.Millisecond, latency: 98 * time.Millisecond},
	{name: "codex", model: "gpt-5.5-codex", startAt: 1000 * time.Millisecond, connect: 900 * time.Millisecond, latency: 171 * time.Millisecond, flaky: true},
	{name: "qwen", model: "qwen3-coder", startAt: 1350 * time.Millisecond, connect: 1700 * time.Millisecond, latency: 203 * time.Millisecond},
}

// Scripted events carry the run generation so a replay ignores timers that
// were still in flight from the previous run.
type (
	frameMsg     struct{}
	connectMsg   struct{ gen, i int }
	handshakeMsg struct{ gen, i int }
)

type keyMap struct {
	Replay key.Binding
	Quit   key.Binding
}

func (k keyMap) ShortHelp() []key.Binding  { return []key.Binding{k.Replay, k.Quit} }
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var keys = keyMap{
	Replay: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "replay")),
	Quit:   key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
}

// Model is the demo's Bubble Tea model.
type Model struct {
	speed     float64
	gen       int
	frame     int
	width     int
	height    int
	styles    theme.Styles
	upstreams []upstream
	spinner   spinner.Model
	progress  progress.Model
	help      help.Model
}

// New returns the demo with timings scaled by speed (2 runs twice as fast).
func New(speed float64) Model {
	m := Model{
		speed:    speed,
		spinner:  spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		progress: progress.New(progress.WithColors(theme.Pink, theme.Violet, theme.Cyan), progress.WithScaled(true), progress.WithWidth(48)),
		help:     help.New(),
	}
	// Assume dark until the terminal answers; some never do.
	m.setTheme(true)
	m.reset()
	return m
}

// Init asks for the background color and starts the animations and script.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.spinner.Tick, nextFrame(), m.run())
}

// Update handles keys, resizes and scripted events, and forwards animation
// ticks to the spinner and progress bar.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Replay):
			m.reset()
			return m, tea.Batch(m.progress.SetPercent(0), m.run())
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.progress.SetWidth(max(10, min(48, msg.Width-12)))
		return m, nil

	case tea.BackgroundColorMsg:
		m.setTheme(msg.IsDark())
		return m, nil

	case frameMsg:
		m.frame++
		return m, nextFrame()

	case connectMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		u := &m.upstreams[msg.i]
		u.status = connecting
		u.attempts++
		return m, m.after(u.connect, handshakeMsg(msg))

	case handshakeMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		u := &m.upstreams[msg.i]
		if u.flaky && u.attempts == 1 {
			u.status = retrying
			return m, m.after(backoff, connectMsg(msg))
		}
		u.status = ready
		return m, m.progress.SetPercent(m.readyFraction())
	}

	var spinCmd, progCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)
	m.progress, progCmd = m.progress.Update(msg)
	return m, tea.Batch(spinCmd, progCmd)
}

// View renders the whole demo centered in the alternate screen.
func (m Model) View() tea.View {
	s := m.styles
	sections := []string{
		renderBanner(m.frame, m.width-4),
		s.Subtle.Render("one endpoint · every model   ") + s.Accent.Render("(demo: nothing here is real)"),
		"",
		s.Panel.Render(m.rows()),
		"",
		m.progress.View(),
		"",
		m.statusLine(),
		"",
		m.help.View(keys),
	}
	content := lipgloss.JoinVertical(lipgloss.Center, sections...)
	if m.width > 0 && m.height > 0 {
		// Clip rather than wrap when the terminal is tiny; wrapped rows are
		// harder to read than truncated ones.
		content = lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(content)
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "cliproxy demo"
	return v
}

func (m Model) rows() string {
	s := m.styles
	width := rowsWidth
	if m.width > 0 {
		width = max(20, min(rowsWidth, m.width-s.Panel.GetHorizontalFrameSize()))
	}
	nameCol := s.Text.Bold(true).Width(9)
	modelCol := s.Subtle.Width(18)

	lines := make([]string, len(m.upstreams))
	for i, u := range m.upstreams {
		var icon, state string
		switch u.status {
		case queued:
			icon, state = s.Subtle.Render("○"), s.Subtle.Render("queued")
		case connecting:
			label := "handshaking…"
			if u.attempts > 1 {
				label = fmt.Sprintf("retry %d…", u.attempts)
			}
			icon, state = m.spinner.View(), s.Accent.Render(label)
		case retrying:
			icon, state = s.Warning.Render("↻"), s.Warning.Render("429 rate limited, backing off")
		case ready:
			icon = s.Success.Render("✓")
			state = s.Success.Render("ready ") + s.Subtle.Render(u.latency.String())
		}
		lines[i] = ansi.Truncate(icon+" "+nameCol.Render(u.name)+modelCol.Render(u.model)+state, width, "…")
	}
	// Fix the width so the panel does not jump as state labels change.
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) statusLine() string {
	s := m.styles
	if m.done() {
		return s.Success.Bold(true).Render("✓ all upstreams ready") +
			s.Subtle.Render("  ·  listening on 127.0.0.1:8317 (pretend)")
	}
	n := 0
	for _, u := range m.upstreams {
		if u.status == ready {
			n++
		}
	}
	return m.spinner.View() + s.Subtle.Render(fmt.Sprintf(" bringing upstreams online  %d/%d", n, len(m.upstreams)))
}

func (m *Model) setTheme(isDark bool) {
	m.styles = theme.New(isDark)
	m.spinner.Style = m.styles.Accent
	m.help.Styles = help.DefaultStyles(isDark)
}

// reset starts a new run generation with every upstream queued.
func (m *Model) reset() {
	m.gen++
	m.upstreams = append([]upstream(nil), script...)
}

// run schedules the first connection attempt of each upstream.
func (m Model) run() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.upstreams))
	for i, u := range m.upstreams {
		cmds[i] = m.after(u.startAt, connectMsg{gen: m.gen, i: i})
	}
	return tea.Batch(cmds...)
}

func (m Model) after(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(time.Duration(float64(d)/m.speed), func(time.Time) tea.Msg { return msg })
}

func (m Model) readyFraction() float64 {
	n := 0
	for _, u := range m.upstreams {
		if u.status == ready {
			n++
		}
	}
	return float64(n) / float64(len(m.upstreams))
}

func (m Model) done() bool { return m.readyFraction() == 1 }

func nextFrame() tea.Cmd {
	return tea.Tick(time.Second/fps, func(time.Time) tea.Msg { return frameMsg{} })
}
