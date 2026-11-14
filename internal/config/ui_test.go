package config

import (
	"strings"
	"testing"
)

func TestUIDefault(t *testing.T) {
	if DefaultUI.ShowUnchanged != 2 {
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

func TestUIShowUnchangedValues(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want UI
	}{
		{
			name: "show_unchanged all",
			src:  "[ui]\nshow_unchanged = \"all\"\n",
			want: UI{ShowUnchanged: ShowAllUnchanged},
		},
		{
			name: "show_unchanged 0",
			src:  "[ui]\nshow_unchanged = 0\n",
			want: UI{ShowUnchanged: 0},
		},
		{
			name: "show_unchanged 2",
			src:  "[ui]\nshow_unchanged = 2\n",
			want: UI{ShowUnchanged: 2},
		},
		{
			name: "show_unchanged 10",
			src:  "[ui]\nshow_unchanged = 10\n",
			want: UI{ShowUnchanged: 10},
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
			name: "show_unchanged negative",
			src:  "[ui]\nshow_unchanged = -1\n",
			want: []string{"ui.show_unchanged", "must be 0 or more", "-1"},
		},
		{
			name: "show_unchanged string other than all",
			src:  "[ui]\nshow_unchanged = \"none\"\n",
			want: []string{"ui.show_unchanged", "must be a non-negative integer or \"all\"", "\"none\""},
		},
		{
			name: "show_unchanged numeric string",
			src:  "[ui]\nshow_unchanged = \"5\"\n",
			want: []string{"ui.show_unchanged", "must be a non-negative integer or \"all\"", "\"5\""},
		},
		{
			name: "show_unchanged empty string",
			src:  "[ui]\nshow_unchanged = \"\"\n",
			want: []string{"ui.show_unchanged", "must be a non-negative integer or \"all\"", "\"\""},
		},
		{
			name: "show_unchanged boolean true",
			src:  "[ui]\nshow_unchanged = true\n",
			want: []string{"ui.show_unchanged", "must be a non-negative integer or \"all\"", "a boolean"},
		},
		{
			name: "show_unchanged boolean false",
			src:  "[ui]\nshow_unchanged = false\n",
			want: []string{"ui.show_unchanged", "must be a non-negative integer or \"all\"", "a boolean"},
		},
		{
			name: "show_unchanged float",
			src:  "[ui]\nshow_unchanged = 1.5\n",
			want: []string{"ui.show_unchanged", "must be a non-negative integer or \"all\"", "a float"},
		},
		{
			name: "show_unchanged array",
			src:  "[ui]\nshow_unchanged = [2]\n",
			want: []string{"ui.show_unchanged", "must be a non-negative integer or \"all\"", "an array"},
		},
		{
			name: "show_unchanged table",
			src:  "[ui.show_unchanged]\nk = 1\n",
			want: []string{"ui.show_unchanged", "must be a non-negative integer or \"all\"", "a table"},
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
			want: []string{"ui: unknown key \"unknown_key\"", "allowed: show_unchanged"},
		},
		{
			name: "old key fold_unchanged is rejected",
			src:  "[ui]\nfold_unchanged = true\n",
			want: []string{"ui: unknown key \"fold_unchanged\"", "allowed: show_unchanged"},
		},
		{
			name: "old key fold_unchanged_keep is rejected",
			src:  "[ui]\nfold_unchanged_keep = 2\n",
			want: []string{"ui: unknown key \"fold_unchanged_keep\"", "allowed: show_unchanged"},
		},
		{
			name: "sub-table ui.linux",
			src:  "[ui.linux]\nshow_unchanged = \"all\"\n",
			want: []string{"ui: unknown key \"linux\"", "allowed: show_unchanged"},
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
