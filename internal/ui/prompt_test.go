package ui

import (
	"fmt"
	"testing"

	"github.com/shiroppi/dots/internal/apply"
)

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
