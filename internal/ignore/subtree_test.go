package ignore

import (
	"slices"
	"testing"
)

func TestWithSubtrees(t *testing.T) {
	base, err := Compile([]string{"*.md"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("exact and below", func(t *testing.T) {
		m := base.WithSubtrees([]string{"config/darwin", "scripts"}, false)
		tests := []struct {
			path string
			want bool
		}{
			{"config/darwin", true},
			{"config/darwin/karabiner.json", true},
			{"scripts", true},
			{"scripts/run.sh", true},
			{"config", false},
			{"config/darwinx", false}, // sibling with common prefix
			{"config/linux", false},
			{"README.md", true}, // from base
			{"main.go", false},
		}
		for _, tc := range tests {
			if got := m.Match(tc.path); got != tc.want {
				t.Errorf("Match(%q) = %v, want %v", tc.path, got, tc.want)
			}
		}
	})

	t.Run("empty string matches everything", func(t *testing.T) {
		m := base.WithSubtrees([]string{""}, false)
		if !m.Match("anything") || !m.Match("config/darwin") {
			t.Errorf("empty string should match everything")
		}
	})

	t.Run("fold case", func(t *testing.T) {
		mFold := base.WithSubtrees([]string{"Config/Darwin"}, true)
		if !mFold.Match("config/darwin/karabiner.json") {
			t.Errorf("fold=true should match different case")
		}
		if !mFold.Match("CONFIG/DARWIN") {
			t.Errorf("fold=true should match different case")
		}

		mNoFold := base.WithSubtrees([]string{"Config/Darwin"}, false)
		if mNoFold.Match("config/darwin/karabiner.json") {
			t.Errorf("fold=false should not match different case")
		}
	})

	t.Run("original not mutated", func(t *testing.T) {
		_ = base.WithSubtrees([]string{"mutate"}, false)
		if base.Match("mutate") {
			t.Errorf("original matcher was mutated")
		}
		if !slices.Equal(base.Patterns(), []string{"*.md"}) {
			t.Errorf("original matcher patterns mutated")
		}
	})

	t.Run("nil matcher", func(t *testing.T) {
		var nilM *Matcher
		m := nilM.WithSubtrees([]string{"a"}, false)
		if !m.Match("a") {
			t.Errorf("nil matcher WithSubtrees should match a")
		}
		if m.Match("b") {
			t.Errorf("nil matcher WithSubtrees should not match b")
		}
	})
}
