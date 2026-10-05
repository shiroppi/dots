// Package ui renders doctor/plan/report output and asks interactive
// confirmations (spec §6-§8). It contains no business logic.
package ui

import (
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/backup"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/state"
)

var hostOS = runtime.GOOS

// Printer renders results to W.
type Printer struct {
	W     io.Writer
	Color bool   // false: plain text, no ANSI escapes
	Home  string // when non-empty, paths under Home are shown as ~/... (display only)
}

type tone int

const (
	toneNone tone = iota
	toneGreen
	toneYellow
	toneRed
	toneDim
)

var toneCode = map[tone]string{toneGreen: "32", toneYellow: "33", toneRed: "31", toneDim: "2"}

// paint wraps s in an SGR sequence only when color is enabled.
func (p *Printer) paint(t tone, s string) string {
	if !p.Color || t == toneNone {
		return s
	}
	return "\x1b[" + toneCode[t] + "m" + s + "\x1b[0m"
}

func (p *Printer) badge(t tone, label string) string {
	return p.paint(t, fmt.Sprintf("%-12s", "["+label+"]"))
}

func (p *Printer) printf(format string, a ...interface{}) {
	fmt.Fprintf(p.W, format, a...)
}

func (p *Printer) display(path string) string {
	return displayPath(p.Home, path, hostOS)
}

// displayPath abbreviates a leading home directory as "~". The separator
// after the prefix is kept as is. Matching is case-insensitive on windows
// and darwin.
func displayPath(home, path, goos string) string {
	if home == "" || path == "" {
		return path
	}
	home = filepath.Clean(home)
	if len(path) < len(home) {
		return path
	}
	head, rest := path[:len(home)], path[len(home):]
	if goos == "windows" || goos == "darwin" {
		if !strings.EqualFold(head, home) {
			return path
		}
	} else if head != home {
		return path
	}
	if rest == "" {
		return "~"
	}
	sep := string(filepath.Separator)
	switch {
	case strings.HasPrefix(rest, sep):
		return "~" + rest
	case strings.HasSuffix(home, sep): // home is a filesystem root
		return "~" + sep + rest
	}
	return path
}

// Flatten returns one error per leaf of an errors.Join tree.
func Flatten(errs ...error) []error {
	var out []error
	for _, e := range errs {
		if e == nil {
			continue
		}
		if m, ok := e.(interface{ Unwrap() []error }); ok {
			out = append(out, Flatten(m.Unwrap()...)...)
			continue
		}
		out = append(out, e)
	}
	return out
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", " ")
}

// Errors prints one problem per line, flattening errors.Join trees.
func (p *Printer) Errors(errs []error) {
	for _, e := range Flatten(errs...) {
		p.printf("%s %s\n", p.paint(toneRed, "error:"), oneLine(e.Error()))
	}
}

func (p *Printer) itemErrors(err error) {
	for _, e := range Flatten(err) {
		p.printf("             %s %s\n", p.paint(toneRed, "!"), oneLine(e.Error()))
	}
}

func (p *Printer) entryLine(badge string, e model.Entry) {
	if e.Kind == model.KindDir {
		p.printf("%s %s (directory)  %s\n", badge, p.display(e.Target), p.paint(toneDim, "("+e.Origin.String()+")"))
		return
	}
	p.printf("%s %s -> %s  %s\n", badge, p.display(e.Target), p.display(e.Source), p.paint(toneDim, "("+e.Origin.String()+")"))
}

func (p *Printer) overrides(ovs []model.Override) {
	if len(ovs) == 0 {
		return
	}
	p.printf("\nOverridden rules:\n")
	for _, o := range ovs {
		p.printf("  %s\n", p.display(o.Target))
		p.printf("    winner: %s\n", o.Winner.String())
		if o.Loser.Kind == model.KindDir {
			p.printf("    loser:  %s (directory)\n", o.Loser.Origin.String())
		} else {
			p.printf("    loser:  %s -> %s\n", o.Loser.Origin.String(), p.display(o.Loser.Source))
		}
	}
}

func doctorBadge(r state.Result) (tone, string) {
	if r.Err != nil {
		return toneRed, "ERROR"
	}
	switch r.Status {
	case state.ValidLink, state.ValidDir:
		return toneGreen, "OK"
	case state.NotExist:
		return toneYellow, "MISSING"
	case state.InvalidLink:
		return toneYellow, "WRONG LINK"
	case state.FileOrDir:
		return toneYellow, "CONFLICT"
	default:
		return toneRed, "ERROR"
	}
}

// Doctor prints the doctor report (spec §8). ok is false when any item is not
// a valid link or has an error.
func (p *Printer) Doctor(plan *apply.Plan) (ok bool) {
	ok = true
	counts := map[string]int{}
	for _, it := range plan.Items {
		r := it.Result
		t, label := doctorBadge(r)
		counts[label]++
		if label != "OK" {
			ok = false
		}
		p.entryLine(p.badge(t, label), r.Entry)
		if r.Err == nil && r.Status == state.InvalidLink {
			p.printf("             current link: %s\n", oneLine(r.Current))
		}
		if r.Err != nil {
			p.itemErrors(r.Err)
		}
	}
	p.overrides(plan.Overrides)
	p.printf("\n%d link(s): %d ok, %d missing, %d wrong link, %d conflict, %d error",
		len(plan.Items), counts["OK"], counts["MISSING"], counts["WRONG LINK"], counts["CONFLICT"], counts["ERROR"])
	if n := len(plan.Overrides); n > 0 {
		p.printf(", %d overridden rule(s)", n)
	}
	p.printf("\n")
	if ok {
		p.printf("%s\n", p.paint(toneGreen, "Everything is in order."))
	} else {
		p.printf("%s\n", p.paint(toneRed, "Not OK: run `dots apply` to fix."))
	}
	return ok
}

// Plan prints the apply --dry-run plan.
func (p *Printer) Plan(plan *apply.Plan) {
	var nCreate, nSkip, nReplace, nErr int
	for _, it := range plan.Items {
		r := it.Result
		var t tone
		var label string
		switch it.Action {
		case apply.ActionCreate:
			t, label = toneGreen, "create"
			nCreate++
		case apply.ActionSkip:
			t, label = toneDim, "skip"
			nSkip++
		case apply.ActionReplace:
			t, label = toneYellow, "replace"
			nReplace++
		default:
			t, label = toneRed, "error"
			nErr++
		}
		p.entryLine(p.badge(t, label), r.Entry)
		switch it.Action {
		case apply.ActionSkip:
			if r.Entry.Kind == model.KindDir {
				p.printf("             already a directory\n")
			} else {
				p.printf("             already linked\n")
			}
		case apply.ActionReplace:
			p.printf("             %s (backed up first)\n", describeExisting(r))
		case apply.ActionError:
			p.itemErrors(r.Err)
		}
	}
	p.overrides(plan.Overrides)
	p.printf("\n%d link(s): %d to create, %d already linked, %d to replace, %d error\n",
		len(plan.Items), nCreate, nSkip, nReplace, nErr)
	if nReplace > 0 {
		p.printf("%d conflict(s) will ask for confirmation\n", nReplace)
	}
}

// Report prints the outcome of apply.
func (p *Printer) Report(rep *apply.Report) {
	warnings := 0
	for _, ir := range rep.Items {
		var t tone
		var label string
		switch ir.Outcome {
		case apply.Created:
			t, label = toneGreen, "created"
		case apply.AlreadyOK:
			t, label = toneDim, "ok"
		case apply.Replaced:
			t, label = toneYellow, "replaced"
		case apply.SkippedByUser:
			t, label = toneDim, "skipped"
		case apply.Failed:
			t, label = toneRed, "failed"
		default:
			t, label = toneDim, "not processed"
		}
		p.entryLine(p.badge(t, label), ir.Item.Result.Entry)
		if ir.ArchivePath != "" {
			p.printf("             archive: %s\n", p.display(ir.ArchivePath))
		}
		if ir.Err != nil {
			p.itemErrors(ir.Err)
		}
		if ir.Warning != nil {
			p.printf("             %s %s\n", p.paint(toneYellow, "warning:"), oneLine(ir.Warning.Error()))
			warnings++
		}
	}
	p.printf("\n%d created, %d already ok, %d replaced, %d skipped by user, %d failed, %d not processed",
		rep.Created(), rep.AlreadyOK(), rep.Replaced(), rep.SkippedByUser(), rep.Failed(), rep.NotProcessed())
	if warnings > 0 {
		p.printf(", %d warning(s)", warnings)
	}
	p.printf("\n")
}

// RestorePlan prints what restoring an archive would do.
func (p *Printer) RestorePlan(plan *backup.RestorePlan, dryRun bool) {
	would := ""
	if dryRun {
		would = "would "
	}
	p.printf("Archive:  %s\n", p.display(plan.ArchivePath))
	p.printf("Target:   %s\n", p.display(plan.Target))
	p.printf("Archived: %s", plan.Manifest.Kind)
	if plan.Manifest.Created != "" {
		p.printf(" (created %s)", plan.Manifest.Created)
	}
	p.printf("\n")
	if plan.Exists {
		p.printf("Current:  exists (%s)\n", plan.ExistingKind)
		p.printf("Action:   %sback up the current content, then restore the archive\n", would)
	} else {
		p.printf("Current:  does not exist\n")
		p.printf("Action:   %srestore the archive\n", would)
	}
	if dryRun {
		p.printf("%s\n", p.paint(toneDim, "dry run: nothing was changed"))
	}
}

// describeExisting says what currently occupies the target of a conflict.
func describeExisting(r state.Result) string {
	switch r.Status {
	case state.InvalidLink:
		return "existing symbolic link -> " + oneLine(r.Current)
	case state.FileOrDir:
		return "existing file or directory"
	default:
		return "existing item"
	}
}

// Display returns path as shown to the user (home abbreviated to ~).
func (p *Printer) Display(path string) string { return p.display(path) }
