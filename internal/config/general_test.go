package config

import (
	"strings"
	"testing"
)

func TestGeneralDefault(t *testing.T) {
	if !DefaultGeneral.FoldUnchanged || DefaultGeneral.FoldUnchangedKeep != 5 || DefaultGeneral.SourceDisplay != SourceSimple {
		t.Fatalf("DefaultGeneral unexpected values: %+v", DefaultGeneral)
	}
	sources := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"comment only", "# just a comment\n"},
		{"dots only", "[dots]\n\"~/.a\" = \"a\"\n"},
		{"empty general", "[general]\n"},
	}
	for _, tc := range sources {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.General != DefaultGeneral {
				t.Errorf("got General = %+v, want DefaultGeneral = %+v", cfg.General, DefaultGeneral)
			}
		})
	}
}

func TestGeneralOmittedKeys(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want General
	}{
		{
			name: "fold_unchanged false",
			src:  "[general]\nfold_unchanged = false\n",
			want: General{FoldUnchanged: false, FoldUnchangedKeep: 5, SourceDisplay: SourceSimple},
		},
		{
			name: "fold_unchanged true",
			src:  "[general]\nfold_unchanged = true\n",
			want: General{FoldUnchanged: true, FoldUnchangedKeep: 5, SourceDisplay: SourceSimple},
		},
		{
			name: "fold_unchanged_keep 0",
			src:  "[general]\nfold_unchanged_keep = 0\n",
			want: General{FoldUnchanged: true, FoldUnchangedKeep: 0, SourceDisplay: SourceSimple},
		},
		{
			name: "fold_unchanged_keep 10",
			src:  "[general]\nfold_unchanged_keep = 10\n",
			want: General{FoldUnchanged: true, FoldUnchangedKeep: 10, SourceDisplay: SourceSimple},
		},
		{
			name: "source_display off",
			src:  "[general]\nsource_display = \"off\"\n",
			want: General{FoldUnchanged: true, FoldUnchangedKeep: 5, SourceDisplay: SourceOff},
		},
		{
			name: "source_display simple",
			src:  "[general]\nsource_display = \"simple\"\n",
			want: General{FoldUnchanged: true, FoldUnchangedKeep: 5, SourceDisplay: SourceSimple},
		},
		{
			name: "source_display verbose",
			src:  "[general]\nsource_display = \"verbose\"\n",
			want: General{FoldUnchanged: true, FoldUnchangedKeep: 5, SourceDisplay: SourceVerbose},
		},
		{
			name: "omit keep only",
			src:  "[general]\nfold_unchanged = false\nsource_display = \"verbose\"\n",
			want: General{FoldUnchanged: false, FoldUnchangedKeep: 5, SourceDisplay: SourceVerbose},
		},
		{
			name: "omit source_display only",
			src:  "[general]\nfold_unchanged = false\nfold_unchanged_keep = 2\n",
			want: General{FoldUnchanged: false, FoldUnchangedKeep: 2, SourceDisplay: SourceSimple},
		},
		{
			name: "omit fold_unchanged only",
			src:  "[general]\nfold_unchanged_keep = 3\nsource_display = \"off\"\n",
			want: General{FoldUnchanged: true, FoldUnchangedKeep: 3, SourceDisplay: SourceOff},
		},
		{
			name: "all specified",
			src:  "[general]\nfold_unchanged = false\nfold_unchanged_keep = 0\nsource_display = \"off\"\n",
			want: General{FoldUnchanged: false, FoldUnchangedKeep: 0, SourceDisplay: SourceOff},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.General != tc.want {
				t.Errorf("got General = %+v, want %+v", cfg.General, tc.want)
			}
		})
	}
}

func TestGeneralValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "fold_unchanged string",
			src:  "[general]\nfold_unchanged = \"true\"\n",
			want: []string{"general.fold_unchanged", "must be a boolean", "a string"},
		},
		{
			name: "fold_unchanged integer",
			src:  "[general]\nfold_unchanged = 1\n",
			want: []string{"general.fold_unchanged", "must be a boolean", "an integer"},
		},
		{
			name: "fold_unchanged_keep negative",
			src:  "[general]\nfold_unchanged_keep = -1\n",
			want: []string{"general.fold_unchanged_keep", "must be 0 or more", "-1"},
		},
		{
			name: "fold_unchanged_keep float",
			src:  "[general]\nfold_unchanged_keep = 1.5\n",
			want: []string{"general.fold_unchanged_keep", "must be an integer", "a float"},
		},
		{
			name: "fold_unchanged_keep string",
			src:  "[general]\nfold_unchanged_keep = \"5\"\n",
			want: []string{"general.fold_unchanged_keep", "must be an integer", "a string"},
		},
		{
			name: "fold_unchanged_keep boolean",
			src:  "[general]\nfold_unchanged_keep = true\n",
			want: []string{"general.fold_unchanged_keep", "must be an integer", "a boolean"},
		},
		{
			name: "fold_unchanged false still validates keep negative",
			src:  "[general]\nfold_unchanged = false\nfold_unchanged_keep = -5\n",
			want: []string{"general.fold_unchanged_keep", "must be 0 or more", "-5"},
		},
		{
			name: "fold_unchanged false still validates keep float",
			src:  "[general]\nfold_unchanged = false\nfold_unchanged_keep = 3.14\n",
			want: []string{"general.fold_unchanged_keep", "must be an integer", "a float"},
		},
		{
			name: "fold_unchanged false still validates keep string",
			src:  "[general]\nfold_unchanged = false\nfold_unchanged_keep = \"none\"\n",
			want: []string{"general.fold_unchanged_keep", "must be an integer", "a string"},
		},
		{
			name: "source_display invalid value",
			src:  "[general]\nsource_display = \"full\"\n",
			want: []string{"general.source_display", "unknown value \"full\"", "allowed:", "\"off\"", "\"simple\"", "\"verbose\""},
		},
		{
			name: "source_display empty string",
			src:  "[general]\nsource_display = \"\"\n",
			want: []string{"general.source_display", "unknown value \"\""},
		},
		{
			name: "source_display integer",
			src:  "[general]\nsource_display = 1\n",
			want: []string{"general.source_display", "must be a string", "an integer"},
		},
		{
			name: "source_display boolean",
			src:  "[general]\nsource_display = true\n",
			want: []string{"general.source_display", "must be a string", "a boolean"},
		},
		{
			name: "source_display array",
			src:  "[general]\nsource_display = [\"off\"]\n",
			want: []string{"general.source_display", "must be a string", "an array"},
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

func TestGeneralUnknownKeys(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "unknown key in general",
			src:  "[general]\nunknown_key = 1\n",
			want: []string{"general: unknown key \"unknown_key\"", "allowed: fold_unchanged, fold_unchanged_keep, source_display"},
		},
		{
			name: "sub-table general.linux",
			src:  "[general.linux]\nfold_unchanged = false\n",
			want: []string{"general: unknown key \"linux\"", "allowed: fold_unchanged, fold_unchanged_keep, source_display"},
		},
		{
			name: "general is integer",
			src:  "general = 1\n",
			want: []string{"general: must be a table", "an integer"},
		},
		{
			name: "general is array",
			src:  "general = [\"a\"]\n",
			want: []string{"general: must be a table", "an array"},
		},
		{
			name: "general is string",
			src:  "general = \"table\"\n",
			want: []string{"general: must be a table", "a string"},
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

func TestUnknownTopLevelListsGeneral(t *testing.T) {
	src := "bogus = 1\n"
	_, err := Parse([]byte(src))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := `unknown top-level key "bogus" (only "dots", "auto" and "general" are allowed)`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q lacks %q", err.Error(), want)
	}
}
