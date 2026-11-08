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

func writeConfig(t *testing.T, te *testEnv, uiHeader string, targets []string) {
	t.Helper()
	var lines []string
	if uiHeader != "" {
		lines = append(lines, uiHeader)
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

	// 1. apply: shows first 2 ✓ rows and "… 6 unchanged"
	code := te.run("apply")
	if code != 0 {
		t.Errorf("apply want 0, got %d", code)
	}
	out := te.out.String()
	for i := 1; i <= 2; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if !strings.Contains(out, "✓ "+disp) {
			t.Errorf("apply should show first 2 rows, missing %s in:\n%s", disp, out)
		}
	}
	if !strings.Contains(out, "… 6 unchanged") {
		t.Errorf("apply should show fold line '… 6 unchanged', got:\n%s", out)
	}
	for i := 3; i <= 8; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if strings.Contains(out, "✓ "+disp) {
			t.Errorf("apply should fold rows 3-8, found %s in:\n%s", disp, out)
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

	// 3. apply --dry-run: folds too (shows first 2 ✓ rows and "… 6 unchanged")
	code = te.run("apply", "--dry-run")
	if code != 0 {
		t.Errorf("apply --dry-run want 0, got %d", code)
	}
	outDryRun := te.out.String()
	for i := 1; i <= 2; i++ {
		disp := te.app.printer().Display(p("home", fmt.Sprintf(".f%d", i)))
		if !strings.Contains(outDryRun, "✓ "+disp) {
			t.Errorf("apply --dry-run missing %s in:\n%s", disp, outDryRun)
		}
	}
	if !strings.Contains(outDryRun, "… 6 unchanged") {
		t.Errorf("apply --dry-run missing fold line '… 6 unchanged', got:\n%s", outDryRun)
	}
	for i := 3; i <= 8; i++ {
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
	writeConfig(t, te, "[ui]\nfold_unchanged = false\n", targets)

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
	writeConfig(t, te, "[ui]\nfold_unchanged_keep = 1\n", targets)

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

func TestCLIInvalidUIConfig(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		wantSubs []string
	}{
		{
			name:     "fold_unchanged_keep negative",
			toml:     "[ui]\nfold_unchanged_keep = -1\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"ui.fold_unchanged_keep", "must be 0 or more"},
		},
		{
			name:     "fold_unchanged wrong type",
			toml:     "[ui]\nfold_unchanged = \"false\"\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"ui.fold_unchanged", "must be a boolean"},
		},
		{
			name:     "unknown key in ui",
			toml:     "[ui]\nextra = 1\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"ui: unknown key \"extra\""},
		},
		{
			name:     "ui sub-table",
			toml:     "[ui.linux]\nfold_unchanged = false\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"ui: unknown key \"linux\""},
		},
		{
			name:     "ui not a table",
			toml:     "ui = 1\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{"ui: must be a table"},
		},
		{
			name:     "general is rejected as unknown top-level table",
			toml:     "[general]\nfold_unchanged = false\n\n[dots]\n\"~/.a\" = \"a\"\n",
			wantSubs: []string{`unknown top-level key "general"`},
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
