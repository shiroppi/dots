package ui

import (
	"errors"
	"fmt"
	"io"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/backup"
)

// ErrAborted is returned when a prompt ends without an answer: the user
// cancelled it (Ctrl-C) or it was interrupted by a signal.
var ErrAborted = errors.New("aborted by user")

const (
	choiceYes     = "Yes – back up and replace"
	choiceNo      = "No – skip"
	choiceRestore = "Back up current content and restore"
	choiceSkip    = "Skip"

	// labelRest is the checkbox that applies the answer to the remaining
	// conflicts.
	labelRest = "Process the rest of the files similarly"
)

var (
	conflictChoices = []string{choiceYes, choiceNo}
	restoreChoices  = []string{choiceRestore, choiceSkip}
)

// decisionFor maps a conflict prompt choice and the "process the rest"
// checkbox to a Decision.
func decisionFor(choice string, rest bool) (apply.Decision, error) {
	switch choice {
	case choiceYes:
		if rest {
			return apply.YesToAll, nil
		}
		return apply.Yes, nil
	case choiceNo:
		if rest {
			return apply.NoToAll, nil
		}
		return apply.No, nil
	}
	return 0, fmt.Errorf("unknown choice %q", choice)
}

// restoreFor maps a restore prompt choice to "back up and restore".
func restoreFor(choice string) (bool, error) {
	switch choice {
	case choiceRestore:
		return true, nil
	case choiceSkip:
		return false, nil
	}
	return false, fmt.Errorf("unknown choice %q", choice)
}

// Prompter implements apply.Prompter with a confirm prompt of Yes/No and a "process the rest" checkbox.
type Prompter struct {
	In   io.Reader
	Out  io.Writer
	Home string
}

func conflictTitle(item apply.Item, home string) string {
	r := item.Result
	return fmt.Sprintf("%s already exists as %s.\nReplace it with a link to %s?",
		displayPath(home, r.Entry.Target, hostOS), describeExisting(r),
		displayPath(home, r.Entry.Source, hostOS))
}

// Confirm asks what to do with one conflicting target. Ctrl-C, or any other
// way of leaving the prompt without confirming, yields ErrAborted.
func (p *Prompter) Confirm(item apply.Item) (apply.Decision, error) {
	a, err := Ask(p.In, p.Out, Question{
		Title:       conflictTitle(item, p.Home),
		Label:       displayPath(p.Home, item.Result.Entry.Target, hostOS),
		Options:     conflictChoices,
		Initial:     1, // No
		Checkbox:    labelRest,
		CheckedNote: " (and the rest)",
	})
	if err != nil {
		return 0, err
	}
	return decisionFor(conflictChoices[a.Choice], a.Checked)
}

// ConfirmRestore asks whether to back up the current content and restore.
// It returns true for backup+restore.
func ConfirmRestore(in io.Reader, out io.Writer, home string, plan *backup.RestorePlan) (bool, error) {
	target := displayPath(home, plan.Target, hostOS)
	a, err := Ask(in, out, Question{
		Title: fmt.Sprintf("%s already exists (%s).\nRestore %s over it?",
			target, plan.ExistingKind, displayPath(home, plan.ArchivePath, hostOS)),
		Label:   target,
		Options: restoreChoices,
		Initial: 1, // Skip
	})
	if err != nil {
		return false, err
	}
	return restoreFor(restoreChoices[a.Choice])
}
