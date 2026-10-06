package ignore

import (
	"slices"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty", "", true},
		{"dot", ".", true},
		{"dotdot", "..", true},
		{"slash", "a/b", true},
		{"leading slash", "/a", true},
		{"trailing slash", "a/", true},
		{"backslash", `a\b`, true},
		{"doublestar", "**", true},
		{"doublestar prefix", "**/README.md", true},
		{"suffix doublestar", "a**", true},
		{"question mark", "?.txt", true},
		{"class", "[ab].txt", true},
		{"alternation", "{a,b}", true},
		{"plain", "README.md", false},
		{"star", "*", false},
		{"glob", "*.md", false},
		{"stars apart", "*.bak*", false},
		{"dotfile", ".DS_Store", false},
		{"unicode", "設定*", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestCompile(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		for _, in := range [][]string{nil, {}} {
			m, err := Compile(in)
			if err != nil {
				t.Fatal(err)
			}
			if m == nil {
				t.Fatal("matcher is nil")
			}
			if m.Match("anything") || m.Match("") {
				t.Fatal("empty matcher must match nothing")
			}
			if len(m.Patterns()) != 0 {
				t.Fatalf("patterns = %v", m.Patterns())
			}
		}
	})

	t.Run("single error names index and pattern", func(t *testing.T) {
		m, err := Compile([]string{"ok", "/bad"})
		if err == nil || m != nil {
			t.Fatalf("m=%v err=%v", m, err)
		}
		msg := err.Error()
		if !strings.Contains(msg, "ignore[1]") || !strings.Contains(msg, `"/bad"`) {
			t.Fatalf("message lacks index/pattern: %q", msg)
		}
		if strings.Contains(msg, "ignore[0]") {
			t.Fatalf("valid pattern reported: %q", msg)
		}
	})

	t.Run("errors are joined", func(t *testing.T) {
		_, err := Compile([]string{"", "fine", "a/b", "a**", "[", `x\y`})
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		for _, want := range []string{"ignore[0]", "ignore[2]", `"a/b"`, "ignore[3]", `"a**"`, "ignore[4]", `"["`, "ignore[5]"} {
			if !strings.Contains(msg, want) {
				t.Errorf("message lacks %q: %q", want, msg)
			}
		}
		if strings.Contains(msg, "ignore[1]") {
			t.Errorf("valid pattern reported: %q", msg)
		}
		if got := strings.Count(msg, "\n") + 1; got != 5 {
			t.Errorf("joined lines = %d want 5: %q", got, msg)
		}
	})
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		in       string
		want     bool
	}{
		{"exact", []string{"README.md"}, "README.md", true},
		{"exact other", []string{"README.md"}, "README.txt", false},
		{"star suffix", []string{"*.md"}, "b.md", true},
		{"star wrong ext", []string{"*.md"}, "b.txt", false},
		{"star matches empty", []string{"*.md"}, ".md", true},
		{"star prefix", []string{"README*"}, "README.md", true},
		{"star alone", []string{"*"}, "anything", true},
		{"stars apart", []string{"a*b*c"}, "axxbyyc", true},
		{"stars apart order", []string{"a*b*c"}, "acb", false},
		{"prefix and suffix do not overlap", []string{"ab*ba"}, "aba", false},
		{"literal question mark in name", []string{"*"}, "?", true},
		{"case sensitive lower pattern", []string{"readme.md"}, "README.md", false},
		{"case sensitive upper pattern", []string{"README.md"}, "readme.md", false},
		{"dotfile", []string{".DS_Store"}, ".DS_Store", true},
		{"multiple patterns second hits", []string{"x", "*.md"}, "r.md", true},
		{"multiple patterns none hit", []string{"x", "*.md"}, "r.go", false},
		{"unicode", []string{"設定*"}, "設定.txt", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Compile(tc.patterns)
			if err != nil {
				t.Fatal(err)
			}
			if got := m.Match(tc.in); got != tc.want {
				t.Fatalf("patterns %q Match(%q)=%v want %v", tc.patterns, tc.in, got, tc.want)
			}
		})
	}
}

func TestNilMatcher(t *testing.T) {
	var m *Matcher
	if m.Match("a") || m.Match("") {
		t.Fatal("nil matcher must match nothing")
	}
	if m.Patterns() != nil {
		t.Fatalf("nil matcher patterns = %v", m.Patterns())
	}
}

func TestPatternsIsCopy(t *testing.T) {
	in := []string{"a", "*.md"}
	m, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}

	// Mutating the input slice after Compile must not affect the matcher.
	in[0] = "zzz"
	if !m.Match("a") || m.Match("zzz") {
		t.Fatal("Compile did not copy its input")
	}

	// Mutating the returned slice must not affect the matcher.
	got := m.Patterns()
	if want := []string{"a", "*.md"}; !slices.Equal(got, want) {
		t.Fatalf("Patterns()=%v want %v", got, want)
	}
	got[0] = "zzz"
	got = append(got, "extra")
	_ = got
	if again := m.Patterns(); !slices.Equal(again, []string{"a", "*.md"}) {
		t.Fatalf("Patterns() mutated: %v", again)
	}
	if !m.Match("a") || m.Match("zzz") {
		t.Fatal("Patterns() exposed internal state")
	}
}
