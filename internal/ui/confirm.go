package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Question describes one confirmation: a question, 2-4 options and an
// optional checkbox. Every confirm prompt in dots is built from it.
type Question struct {
	// Title is the question, possibly several lines.
	Title string
	// Label names the subject in the line left after the prompt ends, as in
	// "? <Label>  <answer>".
	Label string
	// Options are the choices, 2 to 4 of them.
	Options []string
	// Initial is the index of the option the cursor starts on: the one that
	// changes nothing (No, Skip, Cancel).
	Initial int
	// Checkbox is the label of the single optional checkbox; empty for none.
	// It always starts unchecked.
	Checkbox string
	// CheckedNote is appended to the answer line when the checkbox is on.
	CheckedNote string
}

// Answer is a confirmed answer.
type Answer struct {
	Choice  int  // index into Question.Options
	Checked bool // the checkbox state
}

type confirmState int

const (
	stateAsking confirmState = iota
	stateConfirmed
	stateCancelled
)

// confirmModel is the bubbletea model of a confirm prompt. It records whether
// Enter confirmed, so any other way of ending the program counts as a
// cancellation instead of silently yielding the initial values.
type confirmModel struct {
	q       Question
	color   bool
	width   int
	cursor  int // 0..len(Options)-1: an option; len(Options): the checkbox
	choice  int // the option last selected, the answer on the checkbox row
	checked bool
	state   confirmState
}

func newConfirmModel(q Question, color bool) *confirmModel {
	return &confirmModel{q: q, color: color, cursor: q.Initial, choice: q.Initial}
}

func (m *confirmModel) Init() tea.Cmd { return nil }

func (m *confirmModel) rows() int {
	if m.q.Checkbox != "" {
		return len(m.q.Options) + 1
	}
	return len(m.q.Options)
}

func (m *confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "space", " ":
			if m.cursor == len(m.q.Options) {
				m.checked = !m.checked
			}
		case "enter", "ctrl+j": // typeahead sent before raw mode arrives as LF
			m.state = stateConfirmed
			return m, tea.Quit
		case "ctrl+c":
			m.state = stateCancelled
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *confirmModel) move(d int) {
	c := m.cursor + d
	if c < 0 || c >= m.rows() {
		return
	}
	m.cursor = c
	if c < len(m.q.Options) {
		m.choice = c
	}
}

func (m *confirmModel) answer() Answer {
	return Answer{Choice: m.choice, Checked: m.checked}
}

func (m *confirmModel) style(code, s string) string {
	if !m.color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// fit wraps s to the terminal width, because the renderer clips long lines
// instead of wrapping them.
func (m *confirmModel) fit(s string) string {
	if m.width <= 0 {
		return s
	}
	return ansi.Wrap(s, m.width, "")
}

func (m *confirmModel) View() tea.View {
	if m.state != stateAsking {
		line := m.style("36", "?") + " " + m.q.Label + "  "
		if m.state == stateCancelled {
			line += m.style("2", "cancelled")
		} else {
			line += m.q.Options[m.choice]
			if m.checked {
				line += m.q.CheckedNote
			}
		}
		if m.width > 0 {
			line = ansi.Truncate(line, m.width, "…")
		}
		return tea.NewView(line + "\n")
	}

	var b strings.Builder
	b.WriteString(m.fit(m.q.Title))
	b.WriteString("\n\n")
	row := func(i int, text string) {
		if i == m.cursor {
			b.WriteString("  " + m.style("36", "> "+text) + "\n")
		} else {
			b.WriteString("    " + text + "\n")
		}
	}
	for i, o := range m.q.Options {
		row(i, o)
	}
	help := "↑/↓ move · "
	if m.q.Checkbox != "" {
		box := "[ ] "
		if m.checked {
			box = "[x] "
		}
		row(len(m.q.Options), box+m.q.Checkbox)
		help += "space toggle · "
	}
	help += "enter confirm · ctrl+c cancel"
	b.WriteString("\n  " + m.style("2", help) + "\n")
	return tea.NewView(b.String())
}

// colorEnabled reports whether the prompt may use colors.
func colorEnabled() bool {
	_, no := os.LookupEnv("NO_COLOR")
	return !no
}

// Ask shows q on out, reads keys from in (the terminal when nil) and returns
// the confirmed answer. Anything but Enter ends it with ErrAborted: Ctrl-C,
// SIGINT/SIGTERM/SIGHUP, or the input ending.
//
// SIGHUP is not handled by bubbletea, so it is turned into a cancellation of
// the program here instead of killing the process mid-prompt.
func Ask(in io.Reader, out io.Writer, q Question) (Answer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
	}()

	m := newConfirmModel(q, colorEnabled())
	opts := []tea.ProgramOption{tea.WithContext(ctx)}
	if in != nil {
		opts = append(opts, tea.WithInput(in))
	}
	if out != nil {
		opts = append(opts, tea.WithOutput(out))
	}
	_, err := tea.NewProgram(m, opts...).Run() // must run before m.state is read
	if err := askResult(m.state, err); err != nil {
		return Answer{}, err
	}
	return m.answer(), nil
}

// askResult decides the outcome of a finished program. Only Enter counts as
// an answer: bubbletea returns nil from Run when it quit without one
// (SIGTERM), ErrInterrupted for SIGINT without a raw-mode terminal, and an
// error when the context is cancelled (SIGHUP) or the input ends.
func askResult(state confirmState, err error) error {
	switch {
	case state == stateConfirmed:
		return nil
	case err == nil,
		errors.Is(err, tea.ErrInterrupted),
		errors.Is(err, tea.ErrProgramKilled),
		errors.Is(err, context.Canceled),
		errors.Is(err, io.EOF):
		return ErrAborted
	}
	return err
}
