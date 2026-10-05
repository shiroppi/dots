package ui

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/huh"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/backup"
)

// ErrAborted is returned when the user cancels a prompt (Ctrl-C).
var ErrAborted = errors.New("aborted by user")

const (
	choiceYes     = "Yes – back up and replace"
	choiceYesAll  = "Yes to all"
	choiceNo      = "No – skip"
	choiceNoAll   = "No to all"
	choiceRestore = "Back up current content and restore"
	choiceSkip    = "Skip"
)

var conflictChoices = []string{choiceYes, choiceYesAll, choiceNo, choiceNoAll}

// decisionFor maps a conflict prompt choice to a Decision.
func decisionFor(choice string) (apply.Decision, error) {
	switch choice {
	case choiceYes:
		return apply.Yes, nil
	case choiceYesAll:
		return apply.YesToAll, nil
	case choiceNo:
		return apply.No, nil
	case choiceNoAll:
		return apply.NoToAll, nil
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

// Prompter implements apply.Prompter with a huh Select of 4 choices.
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

// Confirm asks what to do with one conflicting target. Ctrl-C yields
// ErrAborted.
func (p *Prompter) Confirm(item apply.Item) (apply.Decision, error) {
	choice, err := runSelect(p.In, p.Out, conflictTitle(item, p.Home), conflictChoices)
	if err != nil {
		return 0, err
	}
	return decisionFor(choice)
}

// ConfirmRestore asks whether to back up the current content and restore.
// It returns true for backup+restore.
func ConfirmRestore(in io.Reader, out io.Writer, plan *backup.RestorePlan) (bool, error) {
	title := fmt.Sprintf("%s already exists (%s).\nRestore %s over it?", plan.Target, plan.ExistingKind, plan.ArchivePath)
	choice, err := runSelect(in, out, title, []string{choiceRestore, choiceSkip})
	if err != nil {
		return false, err
	}
	return restoreFor(choice)
}

func runSelect(in io.Reader, out io.Writer, title string, choices []string) (string, error) {
	var choice string
	opts := make([]huh.Option[string], len(choices))
	for i, c := range choices {
		opts[i] = huh.NewOption(c, c)
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(title).Options(opts...).Value(&choice),
	))
	if in != nil {
		form = form.WithInput(in)
	}
	if out != nil {
		form = form.WithOutput(out)
	}
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrAborted
		}
		return "", err
	}
	return choice, nil
}
