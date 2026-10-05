// Package cli wires the dots commands (init, doctor, apply, restore, edit)
// to the config, resolve, apply, backup and ui packages.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/backup"
	"github.com/shiroppi/dots/internal/config"
	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/platform"
	"github.com/shiroppi/dots/internal/resolve"
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
	// BackupDir overrides backup.DefaultDir(Env) when non-empty.
	BackupDir string
	// RunEditor runs the editor attached to the terminal.
	RunEditor func(argv []string) error
}

// silentError is returned when the output was already printed and only the
// exit code remains.
type silentError struct{}

func (silentError) Error() string { return "exit status 1" }

// usageError marks a command-line usage mistake (unknown command, bad flag,
// wrong arguments); ExitCode appends a pointer to --help for it.
type usageError struct{ err error }

func (u usageError) Error() string { return u.err.Error() }
func (u usageError) Unwrap() error { return u.err }

// ExitCode maps the error returned by a command to a process exit code and
// prints it to stderr as `dots: <message>` (one line per joined error). A nil
// error yields 0; a silent error (output already printed) yields 1 without
// printing anything.
func ExitCode(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	var s silentError
	if errors.As(err, &s) {
		return 1
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
	return 1
}

func (a *App) printer() *ui.Printer {
	return &ui.Printer{W: a.Stdout, Color: a.Color, Home: a.Env.Home}
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

// load finds, parses and resolves dots.toml starting from Cwd.
func (a *App) load() (*model.Resolution, error) {
	path, err := config.Find(a.FS, a.Env.Cwd)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(a.FS, path)
	if err != nil {
		return nil, err
	}
	return resolve.Resolve(a.FS, a.Env, path, cfg)
}

const rootHelp = `
A brief, declarative, flexible dotfiles manager.

Usage:
  dots <command> [options]

Core:
  apply       Apply dotfiles
  doctor      Check configuration and managed files

Usability:
  init        Initialize a dotfiles repository
  restore     Restore files from backups
  edit        Edit dots.toml with $EDITOR

Options:
  -h, --help      Show help
  -v, --version   Show version

Use "dots <command> --help" for more information.
`

// helpFunc prints the fixed root help, or the plain-text help of a subcommand.
func helpFunc(root *cobra.Command) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, _ []string) {
		w := cmd.OutOrStdout()
		if cmd == root {
			fmt.Fprint(w, rootHelp)
			return
		}
		fmt.Fprintf(w, "%s\n\nUsage:\n  dots %s [options]\n\nOptions:\n", cmd.Short, cmd.Use)
		cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "help" {
				return
			}
			fmt.Fprintf(w, "  %-16s%s\n", "    --"+f.Name, f.Usage)
		})
		fmt.Fprintf(w, "  %-16s%s\n", "-h, --help", "Show help")
	}
}

// NewRootCmd builds the command tree.
func NewRootCmd(app *App, version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "dots",
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
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	if app.Stdin != nil {
		root.SetIn(app.Stdin)
	}
	cobra.EnableCommandSorting = false
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpFunc(helpFunc(root))
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.AddCommand(newInitCmd(app), newDoctorCmd(app), newApplyCmd(app), newRestoreCmd(app), newEditCmd(app))
	for _, c := range root.Commands() {
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

// InitTemplate is the content written by `dots init`; it parses to an empty
// configuration.
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
# [[auto]] links every file below source into target, optionally skipping
# paths matched by ignore patterns:
#
#   [[auto]]
#   source = "config"
#   target = "~/.config"
#   ignore = ["**/*.bak"]
#
# [[auto.<os>]] adds or overrides rules of an [[auto]] group on one OS:
#
#   [[auto.linux]]
#   source = "linux-config"
#   target = "~/.config"

[dots]
`

func newInitCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a dotfiles repository",
		Args:  cobra.NoArgs,
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

func newDoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check configuration and managed files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := app.load()
			if err != nil {
				return err
			}
			plan := apply.BuildPlan(app.FS, res)
			if !app.printer().Doctor(plan) {
				return silentError{}
			}
			return nil
		},
	}
}

func newApplyCmd(app *App) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply dotfiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := app.load()
			if err != nil {
				return err
			}
			plan := apply.BuildPlan(app.FS, res)
			pr := app.printer()
			if dryRun {
				pr.Plan(plan)
				if plan.HasErrors() {
					return silentError{}
				}
				return nil
			}
			if len(plan.Items) == 0 {
				fmt.Fprintln(app.Stdout, "Nothing to do: dots.toml defines no links.")
				return nil
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
					pr.Plan(plan)
				}
				return err
			}
			pr.Report(rep)
			if err != nil {
				return err
			}
			if rep.Created() == 0 && rep.Replaced() == 0 && rep.SkippedByUser() == 0 {
				fmt.Fprintln(app.Stdout, "Nothing to do: everything is already linked.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without changing anything")
	return cmd
}

func newRestoreCmd(app *App) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "restore <archive>",
		Short: "Restore files from backups",
		Args:  cobra.ExactArgs(1),
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
					return fmt.Errorf("%s already exists and restoring over it requires an interactive terminal; run `dots restore --dry-run` to inspect (nothing was changed)", plan.Target)
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

func newEditCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Edit dots.toml with $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Find(app.FS, app.Env.Cwd)
			if err != nil {
				return err
			}
			argv := strings.Fields(app.Env.Getenv("EDITOR"))
			if len(argv) == 0 {
				return errors.New("EDITOR is not set")
			}
			if app.RunEditor == nil {
				return errors.New("no editor runner is configured")
			}
			if err := app.RunEditor(append(argv, path)); err != nil {
				return fmt.Errorf("editor failed: %w", err)
			}
			return nil
		},
	}
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
		ConfirmRestore: func(plan *backup.RestorePlan) (bool, error) {
			return ui.ConfirmRestore(os.Stdin, os.Stdout, env.Home, plan)
		},
		RunEditor: func(argv []string) error {
			c := exec.Command(argv[0], argv[1:]...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			return c.Run()
		},
	}
	return ExitCode(NewRootCmd(app, version).Execute(), os.Stderr)
}
