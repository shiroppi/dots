package ui

import (
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/shiroppi/dots/internal/apply"
)

func TestFormResult(t *testing.T) {
	tests := []struct {
		name  string
		state huh.FormState
		err   error
		want  error
	}{
		{"completed", huh.StateCompleted, nil, nil},
		{"normal", huh.StateNormal, nil, ErrAborted},
		{"aborted", huh.StateAborted, nil, ErrAborted},
		{"user aborted", huh.StateNormal, huh.ErrUserAborted, ErrAborted},
		{"timeout", huh.StateNormal, huh.ErrTimeout, ErrAborted},
		{"interrupted", huh.StateNormal, fmt.Errorf("huh: %w", tea.ErrInterrupted), ErrAborted},
		{"other error", huh.StateNormal, errors.New("other error"), errors.New("other error")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formResult(tc.state, tc.err)
			switch tc.want {
			case ErrAborted:
				if !errors.Is(got, ErrAborted) {
					t.Errorf("formResult(%v, %v) = %v, want %v", tc.state, tc.err, got, tc.want)
				}
			case nil:
				if got != nil {
					t.Errorf("formResult(%v, %v) = %v, want nil", tc.state, tc.err, got)
				}
			default:
				if got == nil || got.Error() != tc.want.Error() {
					t.Errorf("formResult(%v, %v) = %v, want %v", tc.state, tc.err, got, tc.want)
				}
			}
		})
	}
}

func TestPromptDecisionFor(t *testing.T) {
	tests := []struct {
		choice string
		rest   bool
		want   apply.Decision
		err    bool
	}{
		{choiceYes, false, apply.Yes, false},
		{choiceYes, true, apply.YesToAll, false},
		{choiceNo, false, apply.No, false},
		{choiceNo, true, apply.NoToAll, false},
		{"unknown", false, 0, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s_rest%v", tc.choice, tc.rest), func(t *testing.T) {
			got, err := decisionFor(tc.choice, tc.rest)
			if (err != nil) != tc.err {
				t.Errorf("decisionFor(%q, %v) error = %v, wantErr %v", tc.choice, tc.rest, err, tc.err)
			}
			if err == nil && got != tc.want {
				t.Errorf("decisionFor(%q, %v) = %v, want %v", tc.choice, tc.rest, got, tc.want)
			}
		})
	}
}
