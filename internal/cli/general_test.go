package cli

import (
	"fmt"
	"strings"
	"testing"

	dotsfs "github.com/shiroppi/dots/internal/fs"
)

func setupAppliedEnv(t *testing.T, count int) (*testEnv, []string) {
	t.Helper()
	te := setupEnv(t)
	var targets []string
	var lines []string
	lines = append(lines, "[dots]")
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("f%d", i)
		if err := dotsfs.WriteFile(te.fsys, p(name), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetRel := fmt.Sprintf("~/.%s", name)
		targets = append(targets, targetRel)
		lines = append(lines, fmt.Sprintf(`%q = %q`, targetRel, name))
	}
	if err := dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Initial apply creates the links so subsequent runs see them as unchanged
	if code := te.run("apply"); code != 0 {
		t.Fatalf("initial apply failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, te.out.String(), te.errOut.String())
	}
	return te, targets
}

func writeConfig(t *testing.T, te *testEnv, generalHeader string, targets []string) {
	t.Helper()
	var lines []string
	if generalHeader != "" {
		lines = append(lines, generalHeader)
	}
	lines = append(lines, "[dots]")
	for _, tgt := range targets {
		name := strings.TrimPrefix(tgt, "~/.")
		lines = append(lines, fmt.Sprintf(`%q = %q`, tgt, name))
	}
	if err := dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCLIApplyFolding(t *testing.T) {
	te, _ := setupAppliedEnv(t, 8)

	// 1. apply: shows first 5 ✓ rows and "… 3 unchanged"
	code := te.run("apply")
	if code != 0 {
		t.Errorf("apply want 0, got %d", code)
	}
	out := te.out.String()
	for i := 1; i <= 5; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if !strings.Contains(out, "✓ "+disp) {
			t.Errorf("apply should show first 5 rows, missing %s in:\n%s", disp, out)
		}
	}
	if !strings.Contains(out, "… 3 unchanged") {
		t.Errorf("apply should show fold line '… 3 unchanged', got:\n%s", out)
	}
	for i := 6; i <= 8; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if strings.Contains(out, "✓ "+disp) {
			t.Errorf("apply should fold rows 6-8, found %s in:\n%s", disp, out)
		}
	}
	if !strings.Contains(out, "Everything is up to date.") {
		t.Errorf("apply missing 'Everything is up to date.', got:\n%s", out)
	}

	// 2. apply --verbose: shows all 8 ✓ rows
	code = te.run("apply", "--verbose")
	if code != 0 {
		t.Errorf("apply --verbose want 0, got %d", code)
	}
	outVerbose := te.out.String()
	for i := 1; i <= 8; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if !strings.Contains(outVerbose, "✓ "+disp) {
			t.Errorf("apply --verbose missing %s in:\n%s", disp, outVerbose)
		}
	}
	if strings.Contains(outVerbose, "…") {
		t.Errorf("apply --verbose should not fold rows, got:\n%s", outVerbose)
	}

	// 3. apply --dry-run: folds too (shows first 5 ✓ rows and "… 3 unchanged")
	code = te.run("apply", "--dry-run")
	if code != 0 {
		t.Errorf("apply --dry-run want 0, got %d", code)
	}
	outDryRun := te.out.String()
	for i := 1; i <= 5; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if !strings.Contains(outDryRun, "✓ "+disp) {
			t.Errorf("apply --dry-run missing %s in:\n%s", disp, outDryRun)
		}
	}
	if !strings.Contains(outDryRun, "… 3 unchanged") {
		t.Errorf("apply --dry-run missing fold line '… 3 unchanged', got:\n%s", outDryRun)
	}
	for i := 6; i <= 8; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if strings.Contains(outDryRun, "✓ "+disp) {
			t.Errorf("apply --dry-run should fold row %s, found in:\n%s", disp, outDryRun)
		}
	}

	// 4. apply --dry-run --verbose: shows all 8 rows
	code = te.run("apply", "--dry-run", "--verbose")
	if code != 0 {
		t.Errorf("apply --dry-run --verbose want 0, got %d", code)
	}
	outDryVerbose := te.out.String()
	for i := 1; i <= 8; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if !strings.Contains(outDryVerbose, "✓ "+disp) {
			t.Errorf("apply --dry-run --verbose missing %s in:\n%s", disp, outDryVerbose)
		}
	}
	if strings.Contains(outDryVerbose, "…") {
		t.Errorf("apply --dry-run --verbose should not fold rows, got:\n%s", outDryVerbose)
	}
}

func TestCLIFoldUnchangedFalse(t *testing.T) {
	te, targets := setupAppliedEnv(t, 8)
	writeConfig(t, te, "[general]\nfold_unchanged = false\n", targets)

	code := te.run("apply")
	if code != 0 {
		t.Errorf("apply want 0, got %d", code)
	}
	out := te.out.String()
	for i := 1; i <= 8; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if !strings.Contains(out, "✓ "+disp) {
			t.Errorf("fold_unchanged=false missing %s in:\n%s", disp, out)
		}
	}
	if strings.Contains(out, "…") {
		t.Errorf("fold_unchanged=false must not fold rows, got:\n%s", out)
	}
}

func TestCLIFoldUnchangedKeep(t *testing.T) {
	te, targets := setupAppliedEnv(t, 8)
	writeConfig(t, te, "[general]\nfold_unchanged_keep = 1\n", targets)

	code := te.run("apply")
	if code != 0 {
		t.Errorf("apply want 0, got %d", code)
	}
	out := te.out.String()
	disp1 := te.app.printer().Display(p("home", ".f1"))
	if !strings.Contains(out, "✓ "+disp1) {
		t.Errorf("fold_unchanged_keep=1 missing row 1 (%s) in:\n%s", disp1, out)
	}
	if !strings.Contains(out, "… 7 unchanged") {
		t.Errorf("fold_unchanged_keep=1 missing '… 7 unchanged', got:\n%s", out)
	}
	for i := 2; i <= 8; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if strings.Contains(out, "✓ "+disp) {
			t.Errorf("fold_unchanged_keep=1 should not show row %s, got:\n%s", disp, out)
		}
	}
}

func TestCLISourceDisplay(t *testing.T) {
	// source_display = "off": created rows have no "←"
	t.Run("off", func(t *testing.T) {
		te := setupEnv(t)
		if err := dotsfs.WriteFile(te.fsys, p("f_off"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
		toml := "[general]\nsource_display = \"off\"\n\n[dots]\n\"~/.new_off\" = \"f_off\"\n"
		if err := dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(toml), 0o644); err != nil {
			t.Fatal(err)
		}
		code := te.run("apply")
		if code != 0 {
			t.Errorf("apply want 0, got %d", code)
		}
		out := te.out.String()
		disp := te.app.printer().Display(p("home", ".new_off"))
		if !strings.Contains(out, "+ "+disp) {
			t.Errorf("missing created row %s in:\n%s", disp, out)
		}
		if strings.Contains(out, "←") {
			t.Errorf("source_display=off must not have '←', got:\n%s", out)
		}
	})

	// source_display = "verbose": rows show the rule (e.g. dots."~/.a")
	t.Run("verbose", func(t *testing.T) {
		te := setupEnv(t)
		if err := dotsfs.WriteFile(te.fsys, p("f_verb"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
		toml := "[general]\nsource_display = \"verbose\"\n\n[dots]\n\"~/.new_verb\" = \"f_verb\"\n"
		if err := dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(toml), 0o644); err != nil {
			t.Fatal(err)
		}
		code := te.run("apply")
		if code != 0 {
			t.Errorf("apply want 0, got %d", code)
		}
		out := te.out.String()
		disp := te.app.printer().Display(p("home", ".new_verb"))
		if !strings.Contains(out, "+ "+disp) {
			t.Errorf("missing created row %s in:\n%s", disp, out)
		}
		if !strings.Contains(out, "← f_verb  dots.\"~/.new_verb\"") {
			t.Errorf("source_display=verbose missing rule in:\n%s", out)
		}

		// Subsequent apply (unchanged row) should also show the rule in verbose mode
		code = te.run("apply")
		if code != 0 {
			t.Errorf("idempotent apply want 0, got %d", code)
		}
		out2 := te.out.String()
		if !strings.Contains(out2, "✓ "+disp) {
			t.Errorf("missing unchanged row %s in:\n%s", disp, out2)
		}
		if !strings.Contains(out2, "← f_verb  dots.\"~/.new_verb\"") {
			t.Errorf("source_display=verbose missing rule on unchanged row in:\n%s", out2)
		}
	})
}

func TestCLIInvalidGeneralConfig(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		wantSubs []string
	}{
		{
			name:     "source_display invalid value",
			toml:     "[general]\nsource_display = \"full\"\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"general.source_display", "unknown value \"full\""},
		},
		{
			name:     "fold_unchanged_keep negative",
			toml:     "[general]\nfold_unchanged_keep = -1\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"general.fold_unchanged_keep", "must be 0 or more"},
		},
		{
			name:     "fold_unchanged wrong type",
			toml:     "[general]\nfold_unchanged = \"false\"\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"general.fold_unchanged", "must be a boolean"},
		},
		{
			name:     "unknown key in general",
			toml:     "[general]\nextra = 1\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"general: unknown key \"extra\""},
		},
		{
			name:     "general sub-table",
			toml:     "[general.linux]\nfold_unchanged = false\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"general: unknown key \"linux\""},
		},
		{
			name:     "general not a table",
			toml:     "general = 1\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"general: must be a table"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setupEnv(t)
			if err := dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(tt.toml), 0o644); err != nil {
				t.Fatal(err)
			}
			code := te.run("apply")
			if code == 0 {
				t.Errorf("expected non-zero exit code, got 0")
			}
			errOutput := te.errOut.String()
			for _, sub := range tt.wantSubs {
				if !strings.Contains(errOutput, sub) {
					t.Errorf("stderr %q missing %q", errOutput, sub)
				}
			}
		})
	}
}
