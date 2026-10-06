package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/apply"
	dotsfs "github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/ui"
)

func TestExitCodeFor(t *testing.T) {
	printer := &ui.Printer{Home: "/home/user"}

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil-safe fallback (not directly usage/aborted)", nil, 1},
		{"plain error", errors.New("plain"), 1},
		{"silent error", silentError{}, 1},
		{"usage error", usageError{errors.New("usage")}, 2},
		{"ErrAborted", ui.ErrAborted, 130},
		{"wrapped ErrAborted", fmt.Errorf("wrap: %w", ui.ErrAborted), 130},
		{"PartialError with ErrAborted", &apply.PartialError{Err: fmt.Errorf("confirmation for X failed: %w", ui.ErrAborted)}, 130},
		{"joined ErrAborted", errors.Join(errors.New("a"), ui.ErrAborted), 130},
		{"Printer wrapped ErrAborted", printer.WrapError(ui.ErrAborted), 130},
		{"wrapped usage error", fmt.Errorf("wrap: %w", usageError{errors.New("usage")}), 2},
		{"joined usage error", errors.Join(errors.New("a"), usageError{errors.New("usage")}), 2},
		{"precedence", errors.Join(usageError{errors.New("usage")}, ui.ErrAborted), 130},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeFor(tt.err); got != tt.want {
				t.Errorf("exitCodeFor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   int
		wantStderr string
		notStderr  string
	}{
		{"nil", nil, 0, "", "dots:"},
		{"silent error", silentError{}, 1, "", "dots:"},
		{"usage error", usageError{errors.New("bad flag")}, 2, "Run \"dots --help\" for usage.", ""},
		{"aborted PartialError", &apply.PartialError{Err: fmt.Errorf("confirmation for X failed: %w", ui.ErrAborted)}, 130, "aborted by user", "Run \"dots --help\""},
		{"generic error", errors.New("boom"), 1, "dots: boom\n", "Run \"dots --help\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			got := ExitCode(tt.err, &buf)
			if got != tt.wantCode {
				t.Errorf("ExitCode() = %v, want %v", got, tt.wantCode)
			}
			out := buf.String()
			if tt.wantStderr != "" && !strings.Contains(out, tt.wantStderr) {
				t.Errorf("ExitCode() stderr = %q, want to contain %q", out, tt.wantStderr)
			}
			if tt.wantStderr == "" && out != "" {
				t.Errorf("ExitCode() stderr = %q, want empty", out)
			}
			if tt.notStderr != "" && strings.Contains(out, tt.notStderr) {
				t.Errorf("ExitCode() stderr = %q, want NOT to contain %q", out, tt.notStderr)
			}
		})
	}
}

func TestEndToEndExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"unknown command", []string{"nosuchcommand"}, 2},
		{"unknown flag", []string{"apply", "--nosuchflag"}, 2},
		{"extra positional arg on apply", []string{"apply", "extra"}, 2},
		{"missing arg on restore", []string{"restore"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setupEnv(t)
			if code := te.run(tt.args...); code != tt.want {
				t.Errorf("run(%v) = %d, want %d", tt.args, code, tt.want)
			}
		})
	}

	t.Run("apply aborted", func(t *testing.T) {
		te := setupEnv(t)
		if err := dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.a" = "a"`+"\n"+`"~/.b" = "a"`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := dotsfs.WriteFile(te.fsys, p("a"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := dotsfs.WriteFile(te.fsys, p("home", ".a"), []byte("conflict_a"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := dotsfs.WriteFile(te.fsys, p("home", ".b"), []byte("conflict_b"), 0o644); err != nil {
			t.Fatal(err)
		}

		te.app.Interactive = true
		te.app.Prompter = &fakePrompter{err: ui.ErrAborted}

		code := te.run("apply")
		if code != 130 {
			t.Errorf("apply aborted = %d, want 130", code)
		}

		out := te.out.String()
		if !strings.Contains(out, "! not processed") {
			t.Errorf("expected '! not processed' in stdout, got %q", out)
		}
	})
}
