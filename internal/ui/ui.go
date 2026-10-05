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
	Root  string // repository root (absolute); when set, sources under it are shown relative to it
}

type tone int

const (
	toneNone tone = iota
	toneGreen
	toneYellow
	toneRed
	toneBlue
	toneMagenta
	toneDim
)

var toneCode = map[tone]string{toneGreen: "32", toneYellow: "33", toneRed: "31", toneBlue: "34", toneMagenta: "35", toneDim: "2"}

// paint wraps s in an SGR sequence only when color is enabled.
func (p *Printer) paint(t tone, s string) string {
	if !p.Color || t == toneNone {
		return s
	}
	return "\x1b[" + toneCode[t] + "m" + s + "\x1b[0m"
}

func (p *Printer) printf(format string, a ...interface{}) {
	fmt.Fprintf(p.W, format, a...)
}

func (p *Printer) display(path string) string {
	return displayPath(p.Home, path, hostOS)
}

// displaySource shows a source path relative to Root when it lies inside it,
// and otherwise like any other path.
func (p *Printer) displaySource(path string) string {
	if p.Root != "" {
		if rel, ok := relUnder(p.Root, path, hostOS); ok {
			return rel
		}
	}
	return p.display(path)
}

// header prints the repository line (only when Root is set).
func (p *Printer) header() {
	if p.Root != "" {
		p.printf("Repository: %s\n\n", p.display(p.Root))
	}
}

// relUnder returns path relative to root when path is strictly inside root.
// Matching is case-insensitive on windows and darwin.
func relUnder(root, path, goos string) (string, bool) {
	root = filepath.Clean(root)
	if len(path) <= len(root) {
		return "", false
	}
	head, rest := path[:len(root)], path[len(root):]
	if goos == "windows" || goos == "darwin" {
		if !strings.EqualFold(head, root) {
			return "", false
		}
	} else if head != root {
		return "", false
	}
	sep := string(filepath.Separator)
	switch {
	case strings.HasPrefix(rest, sep):
		rest = strings.TrimLeft(rest, sep)
	case strings.HasSuffix(root, sep): // root is a filesystem root
	default:
		return "", false
	}
	if rest == "" {
		return "", false
	}
	return rest, true
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

// itemIndent and detailIndent are the left margins of item and detail lines.
const (
	itemIndent   = "  "
	detailIndent = "    "
)

// Status symbols (spec §8): the same set describes states (doctor) and
// operations (dry-run, apply).
const (
	symOK      = "✓"
	symCreate  = "+"
	symReplace = "~"
	symProblem = "!"
	symMissing = "×"
)

// targetLabel is the displayed target; a real directory (KindDir) gets a
// trailing separator.
func (p *Printer) targetLabel(e model.Entry) string {
	s := p.display(e.Target)
	if e.Kind == model.KindDir {
		s += string(filepath.Separator)
	}
	return s
}

// itemLine prints "  <symbol> [word ]<target><suffix>". The symbol and the
// word share one color.
func (p *Printer) itemLine(t tone, sym, word string, e model.Entry, suffix string) {
	mark := sym
	if word != "" {
		mark += " " + word
	}
	p.printf("%s%s %s%s\n", itemIndent, p.paint(t, mark), p.targetLabel(e), suffix)
}

// mapping is the doctor-only "-> source  origin" tail of an item line.
func (p *Printer) mapping(e model.Entry) string {
	s := ""
	if e.Kind != model.KindDir {
		s = " -> " + p.displaySource(e.Source)
	}
	return s + "  " + p.paint(toneDim, e.Origin.String())
}

// detail prints one line indented under its item.
func (p *Printer) detail(t tone, text string) {
	p.printf("%s%s\n", detailIndent, p.paint(t, text))
}

// errorDetails prints every leaf of err as a detail line.
func (p *Printer) errorDetails(err error) {
	for _, e := range Flatten(err) {
		p.detail(toneRed, oneLine(p.DisplayText(e.Error())))
	}
}

func (p *Printer) overrides(ovs []model.Override) {
	if len(ovs) == 0 {
		return
	}
	p.printf("\nOverridden rules:\n")
	for _, o := range ovs {
		p.printf("%s%s\n", itemIndent, p.display(o.Target))
		p.detail(toneDim, "kept:    "+o.Winner.String())
		if o.Loser.Kind == model.KindDir {
			p.detail(toneDim, "ignored: "+o.Loser.Origin.String()+" (directory)")
		} else {
			p.detail(toneDim, "ignored: "+o.Loser.Origin.String()+" -> "+p.displaySource(o.Loser.Source))
		}
	}
}

// count is one "N noun" part of a summary line.
type count struct {
	n    int
	noun string
}

// summary joins the non-zero counts: "2 to create, 3 already ok".
func summary(parts ...count) string {
	var out []string
	for _, c := range parts {
		if c.n > 0 {
			out = append(out, fmt.Sprintf("%d %s", c.n, c.noun))
		}
	}
	return strings.Join(out, ", ")
}

const upToDate = "Everything is up to date."

// doctorMark maps a doctor result to its symbol line parts. word is empty for
// the states that need none.
func doctorMark(r state.Result) (t tone, sym, word string) {
	if r.Err != nil {
		return toneYellow, symProblem, "error"
	}
	switch r.Status {
	case state.ValidLink, state.ValidDir:
		return toneGreen, symOK, ""
	case state.NotExist:
		return toneRed, symMissing, ""
	case state.InvalidLink:
		return toneYellow, symProblem, "wrong link"
	case state.FileOrDir:
		return toneYellow, symProblem, "conflict"
	default:
		return toneYellow, symProblem, "error"
	}
}

// Doctor prints the doctor report (spec §8). ok is false when any item is not
// a valid link or has an error. doctor is the diagnostic view, so unlike
// apply it keeps the mapping (-> source) and the origin of every rule.
func (p *Printer) Doctor(plan *apply.Plan) (ok bool) {
	p.header()
	ok = true
	var nOK, nMissing, nWrong, nConflict, nErr int
	for _, it := range plan.Items {
		r := it.Result
		t, sym, word := doctorMark(r)
		switch {
		case sym == symOK:
			nOK++
		case sym == symMissing:
			nMissing++
		case word == "wrong link":
			nWrong++
		case word == "conflict":
			nConflict++
		default:
			nErr++
		}
		if sym != symOK {
			ok = false
		}
		p.itemLine(t, sym, word, r.Entry, p.mapping(r.Entry))
		if r.Err == nil && r.Status == state.InvalidLink {
			p.detail(toneDim, "current link: "+oneLine(r.Current))
		}
		if r.Err != nil {
			p.errorDetails(r.Err)
		}
	}
	p.overrides(plan.Overrides)
	p.printf("\n")
	if len(plan.Items) == 0 {
		p.printf("No links are defined.\n")
	} else {
		p.printf("%s\n", summary(count{nOK, "ok"}, count{nMissing, "missing"}, count{nWrong, "wrong link"},
			count{nConflict, "conflict"}, count{nErr, "error"}, count{len(plan.Overrides), "overridden rule(s)"}))
	}
	if ok {
		p.printf("%s\n", p.paint(toneGreen, "Everything is in order."))
	} else {
		p.printf("%s\n", p.paint(toneRed, "Not OK: run `dots apply` to fix."))
	}
	return ok
}

// Plan prints the apply --dry-run plan.
func (p *Printer) Plan(plan *apply.Plan) {
	p.header()
	var nCreate, nSkip, nReplace, nErr int
	for _, it := range plan.Items {
		r := it.Result
		switch it.Action {
		case apply.ActionCreate:
			nCreate++
			p.itemLine(toneBlue, symCreate, "", r.Entry, "")
		case apply.ActionSkip:
			nSkip++
			p.itemLine(toneGreen, symOK, "", r.Entry, "")
		case apply.ActionReplace:
			nReplace++
			p.itemLine(toneMagenta, symReplace, "", r.Entry, "")
			p.detail(toneDim, describeExisting(r)+"; will be backed up and replaced after confirmation")
		default:
			nErr++
			p.itemLine(toneYellow, symProblem, "error", r.Entry, "")
			p.errorDetails(r.Err)
		}
	}
	p.overrides(plan.Overrides)
	p.printf("\n")
	if nCreate+nReplace+nErr == 0 {
		p.printf("%s\n", upToDate)
		return
	}
	p.printf("%s\n", summary(count{nCreate, "to create"}, count{nReplace, "to replace"},
		count{nSkip, "already ok"}, count{nErr, "error(s)"}))
}

// Blocked prints the plan of an apply that was rejected before any change
// (validation errors, or conflicts without a terminal): conflicts and errors
// are marked, and everything that would have been created is not processed.
func (p *Printer) Blocked(plan *apply.Plan) {
	p.header()
	for _, it := range plan.Items {
		r := it.Result
		switch it.Action {
		case apply.ActionSkip:
			p.itemLine(toneGreen, symOK, "", r.Entry, "")
		case apply.ActionCreate:
			p.itemLine(toneYellow, symProblem, "not processed", r.Entry, "")
		case apply.ActionReplace:
			p.itemLine(toneYellow, symProblem, "conflict", r.Entry, "")
			p.detail(toneDim, describeExisting(r))
		default:
			p.itemLine(toneYellow, symProblem, "error", r.Entry, "")
			p.errorDetails(r.Err)
		}
	}
	p.overrides(plan.Overrides)
	p.printf("\n")
}

// Report prints the outcome of apply.
func (p *Printer) Report(rep *apply.Report) {
	p.header()
	warnings := 0
	for _, ir := range rep.Items {
		e := ir.Item.Result.Entry
		switch ir.Outcome {
		case apply.Created:
			p.itemLine(toneBlue, symCreate, "", e, "")
		case apply.AlreadyOK:
			p.itemLine(toneGreen, symOK, "", e, "")
		case apply.Replaced:
			p.itemLine(toneMagenta, symReplace, "", e, "")
		case apply.SkippedByUser:
			p.itemLine(toneYellow, symProblem, "skipped", e, "")
		case apply.Failed:
			p.itemLine(toneYellow, symProblem, "failed", e, "")
		default:
			p.itemLine(toneYellow, symProblem, "not processed", e, "")
		}
		if ir.Err != nil {
			p.errorDetails(ir.Err)
		}
		if ir.ArchivePath != "" {
			p.detail(toneDim, "backup: "+p.display(ir.ArchivePath))
		}
		if ir.Warning != nil {
			p.detail(toneYellow, "warning: "+oneLine(ir.Warning.Error()))
			warnings++
		}
	}
	p.printf("\n")
	if rep.Created()+rep.Replaced()+rep.SkippedByUser()+rep.Failed()+rep.NotProcessed() == 0 {
		p.printf("%s\n", upToDate)
		return
	}
	p.printf("%s\n", summary(count{rep.Created(), "created"}, count{rep.Replaced(), "replaced"},
		count{rep.AlreadyOK(), "already ok"}, count{rep.SkippedByUser(), "skipped"},
		count{rep.Failed(), "failed"}, count{rep.NotProcessed(), "not processed"}, count{warnings, "warning(s)"}))
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
