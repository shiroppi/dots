package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var (
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyJ     = tea.KeyPressMsg{Code: 'j', Text: "j"}
	keyK     = tea.KeyPressMsg{Code: 'k', Text: "k"}
	keySpace = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyCtrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

func conflictQuestion() Question {
	return Question{
		Title:       "~/.vimrc already exists as a regular file.\nReplace it with a link to vimrc?",
		Label:       "~/.vimrc",
		Options:     conflictChoices,
		Initial:     1,
		Checkbox:    labelRest,
		CheckedNote: " (and the rest)",
	}
}

func press(m *confirmModel, keys ...tea.Msg) {
	for _, k := range keys {
		m.Update(k)
	}
}

func TestConfirmNavigation(t *testing.T) {
	tests := []struct {
		name       string
		keys       []tea.Msg
		wantCursor int
		wantChoice int
		wantCheck  bool
	}{
		{"initial is No", nil, 1, 1, false},
		{"up to Yes", []tea.Msg{keyUp}, 0, 0, false},
		{"k to Yes", []tea.Msg{keyK}, 0, 0, false},
		{"up clamps at top", []tea.Msg{keyUp, keyUp, keyUp}, 0, 0, false},
		{"down reaches checkbox", []tea.Msg{keyDown}, 2, 1, false},
		{"j reaches checkbox", []tea.Msg{keyJ}, 2, 1, false},
		{"down clamps at bottom", []tea.Msg{keyDown, keyDown}, 2, 1, false},
		{"space toggles on", []tea.Msg{keyDown, keySpace}, 2, 1, true},
		{"space toggles off", []tea.Msg{keyDown, keySpace, keySpace}, 2, 1, false},
		{"space on an option is ignored", []tea.Msg{keySpace}, 1, 1, false},
		{"choice is kept on checkbox", []tea.Msg{keyUp, keyDown, keyDown, keySpace}, 2, 1, true},
		{"checkbox row keeps last option", []tea.Msg{keyUp, keyDown, keyDown}, 2, 1, false},
		{"back up from checkbox", []tea.Msg{keyDown, keyUp, keyUp}, 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newConfirmModel(conflictQuestion(), false)
			press(m, tc.keys...)
			if m.cursor != tc.wantCursor || m.choice != tc.wantChoice || m.checked != tc.wantCheck {
				t.Errorf("cursor=%d choice=%d checked=%v, want %d %d %v",
					m.cursor, m.choice, m.checked, tc.wantCursor, tc.wantChoice, tc.wantCheck)
			}
			if m.state != stateAsking {
				t.Errorf("state = %v, want asking", m.state)
			}
		})
	}
}

func TestConfirmAnswers(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.Msg
		want Answer
	}{
		{"enter keeps No", []tea.Msg{keyEnter}, Answer{1, false}},
		{"LF typeahead is Enter", []tea.Msg{tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}}, Answer{1, false}},
		{"Yes", []tea.Msg{keyUp, keyEnter}, Answer{0, false}},
		{"Yes to all", []tea.Msg{keyDown, keySpace, keyUp, keyUp, keyEnter}, Answer{0, true}},
		{"No to all", []tea.Msg{keyDown, keySpace, keyEnter}, Answer{1, true}},
		{"Enter on unchecked checkbox", []tea.Msg{keyUp, keyDown, keyDown, keyEnter}, Answer{1, false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newConfirmModel(conflictQuestion(), false)
			press(m, tc.keys...)
			if m.state != stateConfirmed || m.answer() != tc.want {
				t.Errorf("state=%v answer=%+v, want confirmed %+v", m.state, m.answer(), tc.want)
			}
		})
	}
}

func TestConfirmNoCheckbox(t *testing.T) {
	m := newConfirmModel(Question{Label: "x", Options: restoreChoices, Initial: 1}, false)
	press(m, keyDown, keyDown, keySpace)
	if m.cursor != 1 || m.checked {
		t.Errorf("cursor=%d checked=%v, want 1 false", m.cursor, m.checked)
	}
	press(m, keyUp, keyEnter)
	if m.state != stateConfirmed || m.answer() != (Answer{0, false}) {
		t.Errorf("state=%v answer=%+v", m.state, m.answer())
	}
	if v := newConfirmModel(Question{Options: restoreChoices}, false).View().Content; strings.Contains(v, "space toggle") {
		t.Errorf("help mentions space without a checkbox:\n%s", v)
	}
}

func TestConfirmCancel(t *testing.T) {
	m := newConfirmModel(conflictQuestion(), false)
	press(m, keyUp, keyCtrlC)
	if m.state != stateCancelled {
		t.Errorf("state = %v, want cancelled", m.state)
	}
	if got := m.View().Content; got != "? ~/.vimrc  cancelled\n" {
		t.Errorf("view = %q", got)
	}
}

func TestConfirmView(t *testing.T) {
	m := newConfirmModel(conflictQuestion(), false)
	got := m.View().Content
	want := `~/.vimrc already exists as a regular file.
Replace it with a link to vimrc?

    Yes – back up and replace
  > No – skip
    [ ] Process the rest of the files similarly

  ↑/↓ move · space toggle · enter confirm · ctrl+c cancel
`
	if got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}

	press(m, keyDown, keySpace)
	got = m.View().Content
	if !strings.Contains(got, "  > [x] Process the rest") {
		t.Errorf("checked view lacks cursor on [x]:\n%s", got)
	}

	press(m, keyUp, keyUp, keyEnter)
	if got := m.View().Content; got != "? ~/.vimrc  Yes – back up and replace (and the rest)\n" {
		t.Errorf("done view = %q", got)
	}
}

func TestConfirmColor(t *testing.T) {
	plain := newConfirmModel(conflictQuestion(), false).View().Content
	if strings.Contains(plain, "\x1b") {
		t.Errorf("NO_COLOR view has escapes: %q", plain)
	}
	colored := newConfirmModel(conflictQuestion(), true).View().Content
	if !strings.Contains(colored, "\x1b[36m> No – skip") {
		t.Errorf("colored view lacks accent cursor: %q", colored)
	}
}

func TestAskResult(t *testing.T) {
	other := errors.New("no tty")
	tests := []struct {
		name  string
		state confirmState
		err   error
		want  error
	}{
		{"confirmed", stateConfirmed, nil, nil},
		{"confirmed then killed", stateConfirmed, tea.ErrProgramKilled, nil},
		{"asking, quit (SIGTERM)", stateAsking, nil, ErrAborted},
		{"cancelled (Ctrl-C)", stateCancelled, nil, ErrAborted},
		{"interrupted", stateAsking, fmt.Errorf("run: %w", tea.ErrInterrupted), ErrAborted},
		{"killed", stateAsking, tea.ErrProgramKilled, ErrAborted},
		{"context cancelled (SIGHUP)", stateAsking, fmt.Errorf("%w: %w", tea.ErrProgramKilled, context.Canceled), ErrAborted},
		{"input EOF", stateAsking, io.EOF, ErrAborted},
		{"other error", stateAsking, other, other},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := askResult(tc.state, tc.err); !errors.Is(got, tc.want) {
				t.Errorf("askResult(%v, %v) = %v, want %v", tc.state, tc.err, got, tc.want)
			}
		})
	}
}
