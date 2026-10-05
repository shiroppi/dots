package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

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

var conflictChoices = []string{choiceYes, choiceNo}

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

// Prompter implements apply.Prompter with a huh form of a 2-choice Select and a "process the rest" checkbox.
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
// way of leaving the prompt without submitting, yields ErrAborted.
func (p *Prompter) Confirm(item apply.Item) (apply.Decision, error) {
	var choice string
	var rest []bool
	form := newForm(p.In, p.Out,
		huh.NewSelect[string]().Title(conflictTitle(item, p.Home)).
			Options(options(conflictChoices)...).Value(&choice),
		huh.NewMultiSelect[bool]().
			Options(huh.NewOption(labelRest, true)).Value(&rest),
	)
	if err := runForm(form); err != nil {
		return 0, err
	}
	return decisionFor(choice, len(rest) > 0)
}

// ConfirmRestore asks whether to back up the current content and restore.
// It returns true for backup+restore.
func ConfirmRestore(in io.Reader, out io.Writer, home string, plan *backup.RestorePlan) (bool, error) {
	title := fmt.Sprintf("%s already exists (%s).\nRestore %s over it?",
		displayPath(home, plan.Target, hostOS), plan.ExistingKind, displayPath(home, plan.ArchivePath, hostOS))
	choice, err := runSelect(in, out, title, []string{choiceRestore, choiceSkip})
	if err != nil {
		return false, err
	}
	return restoreFor(choice)
}

func options(choices []string) []huh.Option[string] {
	opts := make([]huh.Option[string], len(choices))
	for i, c := range choices {
		opts[i] = huh.NewOption(c, c)
	}
	return opts
}

func runSelect(in io.Reader, out io.Writer, title string, choices []string) (string, error) {
	var choice string
	form := newForm(in, out, huh.NewSelect[string]().Title(title).Options(options(choices)...).Value(&choice))
	if err := runForm(form); err != nil {
		return "", err
	}
	return choice, nil
}

func newForm(in io.Reader, out io.Writer, fields ...huh.Field) *huh.Form {
	form := huh.NewForm(huh.NewGroup(fields...))
	if in != nil {
		form = form.WithInput(in)
	}
	if out != nil {
		form = form.WithOutput(out)
	}
	return form
}

// runForm runs form and returns ErrAborted unless the user submitted it.
// SIGHUP is not handled by bubbletea, so it is turned into a cancellation of
// the form here instead of killing the process mid-prompt.
func runForm(form *huh.Form) error {
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
	return formResult(form.State, form.RunWithContext(ctx))
}

// formResult decides the outcome of a finished form. huh returns nil from
// Run when bubbletea quit without the user submitting (SIGTERM), and wraps
// tea.ErrInterrupted (SIGINT without a raw-mode terminal), so only a
// completed form counts as an answer.
func formResult(state huh.FormState, err error) error {
	switch {
	case err == nil:
		if state != huh.StateCompleted {
			return ErrAborted
		}
		return nil
	case errors.Is(err, huh.ErrUserAborted), errors.Is(err, huh.ErrTimeout), errors.Is(err, tea.ErrInterrupted):
		return ErrAborted
	}
	return err
}
