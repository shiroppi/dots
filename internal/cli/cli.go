// Package cli wires the dots commands (init, status, apply, clean, restore)
// to the config, resolve, apply, backup and ui packages.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/backup"
	"github.com/shiroppi/dots/internal/config"
	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/platform"
	"github.com/shiroppi/dots/internal/resolve"
	"github.com/shiroppi/dots/internal/statefile"
	"github.com/shiroppi/dots/internal/ui"
)

// App holds every external dependency of the commands so tests can inject
// fakes.
type App struct {
	FS     fs.Manager
	Env    platform.Env
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Interactive is true when stdin AND stdout are terminals.
	Interactive bool
	// Color is true when Interactive and NO_COLOR is unset.
	Color bool
	// Prompter is used only when Interactive.
	Prompter apply.Prompter
	// ConfirmRestore is used only when Interactive; true means back up the
	// current content and restore.
	ConfirmRestore func(*backup.RestorePlan) (bool, error)
	// Ask shows a confirmation; used only when Interactive.
	Ask func(ui.Question) (ui.Answer, error)
	// BackupDir overrides backup.DefaultDir(Env) when non-empty.
	BackupDir string
	// StatePath overrides statefile.DefaultPath(Env) when non-empty.
	StatePath string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
}

// silentError is returned when the output was already printed and only the
// exit code remains.
type silentError struct{}

func (silentError) Error() string { return "exit status 1" }

// usageError marks a command-line usage mistake (unknown command, bad flag,
// wrong arguments); ExitCode appends a pointer to --help for it and exits 2.
type usageError struct{ err error }

func (u usageError) Error() string { return u.err.Error() }
func (u usageError) Unwrap() error { return u.err }

// Process exit codes.
const (
	exitFailure = 1   // failure or problems detected
	exitUsage   = 2   // command-line usage error
	exitAborted = 130 // a confirmation prompt ended without an answer
)

// exitCodeFor classifies a non-nil error: 130 when a prompt was aborted, else
// 2 for a usage error, else 1. It looks through wrapping, joins and
// apply.PartialError.
func exitCodeFor(err error) int {
	var u usageError
	switch {
	case errors.Is(err, ui.ErrAborted):
		return exitAborted
	case errors.As(err, &u):
		return exitUsage
	}
	return exitFailure
}

// ExitCode maps the error returned by a command to a process exit code and
// prints it to stderr as `dots: <message>` (one line per joined error). A nil
// error yields 0; a silent error (output already printed) yields 1 without
// printing anything. Other codes are chosen by exitCodeFor.
func ExitCode(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	var s silentError
	if errors.As(err, &s) {
		return exitFailure
	}
	hint := false
	for _, e := range ui.Flatten(err) {
		fmt.Fprintf(stderr, "dots: %s\n", strings.ReplaceAll(strings.TrimRight(e.Error(), "\n"), "\n", " "))
		var u usageError
		if errors.As(e, &u) {
			hint = true
		}
	}
	if hint {
		fmt.Fprintln(stderr, `Run "dots --help" for usage.`)
	}
	return exitCodeFor(err)
}

func (a *App) printer() *ui.Printer {
	return &ui.Printer{W: a.Stdout, Color: a.Color, Home: a.Env.Home, Width: terminalWidth(a.Stdout)}
}

// terminalWidth is the column count of w when it is a terminal, else 0.
func terminalWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if cols, _, err := term.GetSize(int(f.Fd())); err == nil {
			return cols
		}
	}
	return 0
}

// printerFor is printer with the repository root of res, for output that
// shows sources.
func (a *App) printerFor(res *model.Resolution) *ui.Printer {
	pr := a.printer()
	pr.Root = res.Root
	return pr
}

// store builds the backup store; the directory is computed only when needed.
func (a *App) store() (*backup.Store, error) {
	dir := a.BackupDir
	if dir == "" {
		d, err := backup.DefaultDir(a.Env)
		if err != nil {
			return nil, err
		}
		dir = d
	}
	return &backup.Store{FS: a.FS, Dir: dir}, nil
}

// stateStore builds the state file store; the path is computed only when
// needed.
func (a *App) stateStore() (*statefile.Store, error) {
	path := a.StatePath
	if path == "" {
		p, err := statefile.DefaultPath(a.Env)
		if err != nil {
			return nil, err
		}
		path = p
	}
	return &statefile.Store{FS: a.FS, Path: path, GOOS: a.Env.GOOS}, nil
}

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// updateState records the links rep created or confirmed and drops records of
// vanished targets (spec §5.3). loadErr is the result of Load before the
// plan ran. A broken state file is moved aside first and reported on stderr.
// A failure is returned as a warning and never undoes any link.
func (a *App) updateState(pr *ui.Printer, store *statefile.Store, st *statefile.State, loadErr error, rep *apply.Report, root string) error {
	broken := errors.Is(loadErr, statefile.ErrBroken)
	if loadErr != nil && !broken {
		return fmt.Errorf("warning: state was not updated: %w", loadErr)
	}
	if !apply.UpdateState(a.FS, st, rep, root, a.now()) && !broken {
		return nil
	}
	if broken {
		dest, err := store.MoveAside(a.now())
		if err != nil {
			return fmt.Errorf("warning: state was not updated: %w", err)
		}
		fmt.Fprintf(a.Stderr, "warning: %s\n         moved it to %s and started a new one\n", pr.DisplayText(loadErr.Error()), pr.Display(dest))
	}
	if err := store.Save(st); err != nil {
		return fmt.Errorf("warning: cannot update state file %s: %w", pr.Display(store.Path), err)
	}
	return nil
}

// load finds, parses and resolves dots.toml starting from Cwd. It also
// returns the [general] settings.
func (a *App) load() (*model.Resolution, config.General, error) {
	path, err := config.Find(a.FS, a.Env.Cwd)
	if err != nil {
		return nil, config.General{}, a.printer().WrapError(err)
	}
	// From here on the repository root is known, so messages show sources
	// relative to it.
	pr := a.printer()
	pr.Root = filepath.Dir(path)
	cfg, err := config.Load(a.FS, path)
	if err != nil {
		return nil, config.General{}, pr.WrapError(err)
	}
	res, err := resolve.Resolve(a.FS, a.Env, path, cfg)
	if err != nil {
		return nil, config.General{}, pr.WrapError(err)
	}
	return res, cfg.General, nil
}

// applyPrinter is printerFor with the [general] settings of apply: how rows
// show their source, and the folding of unchanged rows unless verbose.
func (a *App) applyPrinter(res *model.Resolution, g config.General, verbose bool) *ui.Printer {
	pr := a.printerFor(res)
	switch g.SourceDisplay {
	case config.SourceOff:
		pr.Source = ui.SourceOff
	case config.SourceVerbose:
		pr.Source = ui.SourceVerbose
	default:
		pr.Source = ui.SourceSimple
	}
	pr.Fold = g.FoldUnchanged && !verbose
	pr.FoldKeep = g.FoldUnchangedKeep
	return pr
}

// usageTemplate renders both the root and the subcommand help: a description,
// the usage line, the command groups (root only) and the options. The root
// output starts with a blank line. The "help" command belongs to no group, so
// it never appears in the listing.
const usageTemplate = `{{if not .HasParent}}
{{end}}{{.Short}}

Usage:
  {{if .HasParent}}{{.Parent.CommandPath}} {{end}}{{.Use}} [options]
{{range $g := .Groups}}
{{$g.Title}}
{{range $.Commands}}{{if and (eq .GroupID $g.ID) .IsAvailableCommand}}  {{rpad .Name .NamePadding}} {{.Short}}
{{end}}{{end}}{{end}}
Options:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{if .HasAvailableSubCommands}}
Use "{{.CommandPath}} <command> --help" for more information.
{{end}}`

// NewRootCmd builds the command tree.
func NewRootCmd(app *App, version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "dots <command>",
		Short:         "A brief, declarative, flexible dotfiles manager.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.SetVersionTemplate("dots version {{.Version}}\n")
	root.SetUsageTemplate(usageTemplate)
	root.SetHelpTemplate("{{.UsageString}}")
	root.Flags().BoolP("version", "v", false, "Show version")
	root.Flags().BoolP("help", "h", false, "Show help")
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	if app.Stdin != nil {
		root.SetIn(app.Stdin)
	}
	cobra.EnableCommandSorting = false
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.AddGroup(&cobra.Group{ID: "core", Title: "Core:"}, &cobra.Group{ID: "usability", Title: "Usability:"})
	root.AddCommand(newApplyCmd(app), newStatusCmd(app), newCleanCmd(app), newInitCmd(app), newRestoreCmd(app))
	for _, c := range root.Commands() {
		c.Flags().BoolP("help", "h", false, "Show help")
		if inner := c.Args; inner != nil {
			c.Args = func(cmd *cobra.Command, args []string) error {
				if err := inner(cmd, args); err != nil {
					return usageError{err}
				}
				return nil
			}
		}
	}
	return root
}

// InitTemplate is the content written by `dots init`; it defines no links
// and sets [general] to its default values.
const InitTemplate = `# dots.toml - declarative dotfiles configuration.
#
# [dots] maps a target path to a source path (relative to this file).
# Quote target paths. "~" is the home directory.
#
#   [dots]
#   "~/.vimrc" = "vim/vimrc"
#
# [dots.<os>] overrides [dots] on one OS (linux, darwin, windows, ...):
#
#   [dots.windows]
#   "~/.vimrc" = "vim/windows-vimrc"
#
# [[auto]] links each item directly inside source (file or directory) into
# target, optionally skipping top-level names matched by ignore patterns:
#
#   [[auto]]
#   source = "config"
#   target = "~/.config"
#   ignore = ["README.md"]
#
# [[auto.<os>]] adds or overrides rules of an [[auto]] group on one OS:
#
#   [[auto.linux]]
#   source = "linux-config"
#   target = "~/.config"

# [general] tunes the output of dots apply. The values below are the defaults.

[general]
source_display = "simple"   # off | simple | verbose
fold_unchanged = true       # fold unchanged rows into "… N unchanged"
fold_unchanged_keep = 5     # unchanged rows kept visible when folding

[dots]
`

func newInitCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "init",
		Short:   "Initialize a dotfiles repository",
		GroupID: "usability",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := filepath.Join(app.Env.Cwd, config.FileName)
			f, err := app.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				if errors.Is(err, fs.ErrExist) {
					return fmt.Errorf("%s already exists; refusing to overwrite", path)
				}
				return fmt.Errorf("cannot create %s: %w", path, err)
			}
			_, werr := io.WriteString(f, InitTemplate)
			cerr := f.Close()
			if werr != nil {
				return fmt.Errorf("cannot write %s: %w", path, werr)
			}
			if cerr != nil {
				return fmt.Errorf("cannot write %s: %w", path, cerr)
			}
			fmt.Fprintf(app.Stdout, "created %s\n", path)
			return nil
		},
	}
}

func newStatusCmd(app *App) *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show the status of managed files",
		GroupID: "core",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, _, err := app.load()
			if err != nil {
				return err
			}
			plan := apply.BuildPlan(app.FS, res)
			sr, stateOK := app.staleReport(res)
			if !app.printerFor(res).StatusState(plan, sr, verbose) || !stateOK {
				return silentError{}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show the source and rule of each row")
	return cmd
}

func newApplyCmd(app *App) *cobra.Command {
	var dryRun, verbose bool
	cmd := &cobra.Command{
		Use:     "apply",
		Short:   "Apply dotfiles",
		GroupID: "core",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, general, err := app.load()
			if err != nil {
				return err
			}
			plan := apply.BuildPlan(app.FS, res)
			pr := app.applyPrinter(res, general, verbose)
			if len(plan.Items) == 0 {
				fmt.Fprintln(app.Stdout, "Nothing to do: dots.toml defines no links.")
				return nil
			}
			if dryRun {
				pr.Plan(plan)
				if plan.HasErrors() {
					return silentError{}
				}
				return nil
			}
			// State problems must surface before any link changes.
			store, storeErr := app.stateStore()
			var st *statefile.State
			var loadErr error
			if storeErr == nil {
				st, loadErr = store.Load()
				if errors.Is(loadErr, statefile.ErrNewerVersion) {
					return pr.WrapError(fmt.Errorf("%w; refusing to change anything", loadErr))
				}
			}
			opts := apply.Options{}
			if app.Interactive && app.Prompter != nil {
				opts.Prompter = app.Prompter
				if plan.Conflicts() > 0 {
					store, err := app.store()
					if err != nil {
						return err
					}
					opts.Replacer = store
				}
			}
			rep, err := apply.Execute(app.FS, plan, opts)
			if rep == nil {
				// Rejected before any change: show what is wrong.
				if err != nil {
					pr.Blocked(plan)
				}
				return pr.WrapError(err)
			}
			pr.Report(rep)
			var stateErr error
			if storeErr != nil {
				stateErr = fmt.Errorf("warning: state was not updated: %w", storeErr)
			} else {
				stateErr = app.updateState(pr, store, st, loadErr, rep, res.Root)
				if loadErr == nil || errors.Is(loadErr, statefile.ErrBroken) {
					stateErr = errors.Join(stateErr, app.staleNotice(pr, st, res))
				}
			}
			return pr.WrapError(errors.Join(err, stateErr))
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without changing anything")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show every unchanged row instead of folding them")
	return cmd
}

func newRestoreCmd(app *App) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:     "restore <archive>",
		Short:   "Restore files from backups",
		GroupID: "usability",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := config.Find(app.FS, app.Env.Cwd); err != nil {
				return err
			}
			archive := args[0]
			if !filepath.IsAbs(archive) {
				archive = filepath.Join(app.Env.Cwd, archive)
			}
			store, err := app.store()
			if err != nil {
				return err
			}
			plan, err := store.PlanRestore(archive)
			if err != nil {
				return err
			}
			pr := app.printer()
			pr.RestorePlan(plan, dryRun)
			if dryRun {
				return nil
			}
			backupExisting := false
			if plan.Exists {
				if !app.Interactive || app.ConfirmRestore == nil {
					return fmt.Errorf("%s already exists and restoring over it requires an interactive terminal; run `dots restore --dry-run` to inspect (nothing was changed)", pr.Display(plan.Target))
				}
				ok, err := app.ConfirmRestore(plan)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(app.Stdout, "skipped: nothing was changed")
					return nil
				}
				backupExisting = true
			}
			newArchive, err := store.Restore(plan, backupExisting)
			if err != nil {
				return err
			}
			fmt.Fprintf(app.Stdout, "restored %s\n", pr.Display(plan.Target))
			if newArchive != "" {
				fmt.Fprintf(app.Stdout, "previous content backed up to %s\n", pr.Display(newArchive))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be restored without changing anything")
	return cmd
}

// Main builds the real App and runs the CLI. It returns the exit code.
func Main(version string) int {
	env, err := platform.Current()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dots: %v\n", err)
		return 1
	}
	_, noColor := os.LookupEnv("NO_COLOR")
	interactive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	app := &App{
		FS:          fs.NewOS(),
		Env:         env,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: interactive,
		Color:       interactive && !noColor,
		Prompter:    &ui.Prompter{In: os.Stdin, Out: os.Stdout, Home: env.Home},
		Ask:         func(q ui.Question) (ui.Answer, error) { return ui.Ask(os.Stdin, os.Stdout, q) },
		ConfirmRestore: func(plan *backup.RestorePlan) (bool, error) {
			return ui.ConfirmRestore(os.Stdin, os.Stdout, env.Home, plan)
		},
	}
	return ExitCode(NewRootCmd(app, version).Execute(), os.Stderr)
}
