package config

import (
	"strings"
	"testing"
)

func TestUIDefault(t *testing.T) {
	if !DefaultUI.FoldUnchanged || DefaultUI.FoldUnchangedKeep != 2 {
		t.Fatalf("DefaultUI unexpected values: %+v", DefaultUI)
	}
	sources := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"comment only", "# just a comment\n"},
		{"dots only", "[dots]\n\"~/.a\" = \"a\"\n"},
		{"empty ui", "[ui]\n"},
	}
	for _, tc := range sources {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.UI != DefaultUI {
				t.Errorf("got UI = %+v, want DefaultUI = %+v", cfg.UI, DefaultUI)
			}
		})
	}
}

func TestUIOmittedKeys(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want UI
	}{
		{
			name: "fold_unchanged false",
			src:  "[ui]\nfold_unchanged = false\n",
			want: UI{FoldUnchanged: false, FoldUnchangedKeep: 2},
		},
		{
			name: "fold_unchanged true",
			src:  "[ui]\nfold_unchanged = true\n",
			want: UI{FoldUnchanged: true, FoldUnchangedKeep: 2},
		},
		{
			name: "fold_unchanged_keep 0",
			src:  "[ui]\nfold_unchanged_keep = 0\n",
			want: UI{FoldUnchanged: true, FoldUnchangedKeep: 0},
		},
		{
			name: "fold_unchanged_keep 10",
			src:  "[ui]\nfold_unchanged_keep = 10\n",
			want: UI{FoldUnchanged: true, FoldUnchangedKeep: 10},
		},
		{
			name: "all specified",
			src:  "[ui]\nfold_unchanged = false\nfold_unchanged_keep = 5\n",
			want: UI{FoldUnchanged: false, FoldUnchangedKeep: 5},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.UI != tc.want {
				t.Errorf("got UI = %+v, want %+v", cfg.UI, tc.want)
			}
		})
	}
}

func TestUIValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "fold_unchanged string",
			src:  "[ui]\nfold_unchanged = \"true\"\n",
			want: []string{"ui.fold_unchanged", "must be a boolean", "a string"},
		},
		{
			name: "fold_unchanged integer",
			src:  "[ui]\nfold_unchanged = 1\n",
			want: []string{"ui.fold_unchanged", "must be a boolean", "an integer"},
		},
		{
			name: "fold_unchanged_keep negative",
			src:  "[ui]\nfold_unchanged_keep = -1\n",
			want: []string{"ui.fold_unchanged_keep", "must be 0 or more", "-1"},
		},
		{
			name: "fold_unchanged_keep float",
			src:  "[ui]\nfold_unchanged_keep = 1.5\n",
			want: []string{"ui.fold_unchanged_keep", "must be an integer", "a float"},
		},
		{
			name: "fold_unchanged_keep string",
			src:  "[ui]\nfold_unchanged_keep = \"5\"\n",
			want: []string{"ui.fold_unchanged_keep", "must be an integer", "a string"},
		},
		{
			name: "fold_unchanged_keep boolean",
			src:  "[ui]\nfold_unchanged_keep = true\n",
			want: []string{"ui.fold_unchanged_keep", "must be an integer", "a boolean"},
		},
		{
			name: "fold_unchanged false still validates keep negative",
			src:  "[ui]\nfold_unchanged = false\nfold_unchanged_keep = -5\n",
			want: []string{"ui.fold_unchanged_keep", "must be 0 or more", "-5"},
		},
		{
			name: "fold_unchanged false still validates keep float",
			src:  "[ui]\nfold_unchanged = false\nfold_unchanged_keep = 3.14\n",
			want: []string{"ui.fold_unchanged_keep", "must be an integer", "a float"},
		},
		{
			name: "fold_unchanged false still validates keep string",
			src:  "[ui]\nfold_unchanged = false\nfold_unchanged_keep = \"none\"\n",
			want: []string{"ui.fold_unchanged_keep", "must be an integer", "a string"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.src))
			if err == nil {
				t.Fatalf("expected error, got config %+v", cfg)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err.Error(), w)
				}
			}
		})
	}
}

func TestUIUnknownKeys(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "unknown key in ui",
			src:  "[ui]\nunknown_key = 1\n",
			want: []string{"ui: unknown key \"unknown_key\"", "allowed: fold_unchanged, fold_unchanged_keep"},
		},
		{
			name: "sub-table ui.linux",
			src:  "[ui.linux]\nfold_unchanged = false\n",
			want: []string{"ui: unknown key \"linux\"", "allowed: fold_unchanged, fold_unchanged_keep"},
		},
		{
			name: "ui is integer",
			src:  "ui = 1\n",
			want: []string{"ui: must be a table", "an integer"},
		},
		{
			name: "ui is array",
			src:  "ui = [\"a\"]\n",
			want: []string{"ui: must be a table", "an array"},
		},
		{
			name: "ui is string",
			src:  "ui = \"table\"\n",
			want: []string{"ui: must be a table", "a string"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.src))
			if err == nil {
				t.Fatalf("expected error, got config %+v", cfg)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err.Error(), w)
				}
			}
		})
	}
}

func TestUnknownTopLevelListsUI(t *testing.T) {
	for _, key := range []string{"bogus", "general"} {
		t.Run(key, func(t *testing.T) {
			src := "[" + key + "]\n"
			_, err := Parse([]byte(src))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			want := `unknown top-level key "` + key + `" (only "dots", "auto" and "ui" are allowed)`
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q lacks %q", err.Error(), want)
			}
		})
	}
}
