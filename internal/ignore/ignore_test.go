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
		{"backslash", `a\b`, true},
		{"leading slash", "/a", true},
		{"trailing slash", "a/", true},
		{"double slash", "a//b", true},
		{"dotdot", "a/../b", true},
		{"leading dotdot", "../a", true},
		{"bare dotdot", "..", true},
		{"suffix doublestar", "a**", true},
		{"prefix doublestar", "**b", true},
		{"doublestar in middle segment", "a/**b/c", true},
		{"unclosed bracket", "[", true},
		{"unclosed bracket in name", "a[b.txt", true},
		{"plain", "README.md", false},
		{"glob", "*.md", false},
		{"doublestar prefix", "**/README.md", false},
		{"doublestar suffix", "a/**", false},
		{"doublestar alone", "**", false},
		{"doublestar middle", "a/**/b", false},
		{"class", "[ab].txt", false},
		{"dotfile", ".DS_Store", false},
		{"dot element", "./a", true},
		{"inner dot element", "a/./b", true},
		{"unicode", "設定/*.txt", false},
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
		_, err := Compile([]string{"", "fine", "a//b", "a**", "[", `x\y`})
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		for _, want := range []string{"ignore[0]", "ignore[2]", `"a//b"`, "ignore[3]", `"a**"`, "ignore[4]", `"["`, "ignore[5]"} {
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
		path     string
		want     bool
	}{
		{"root file matches", []string{"README.md"}, "README.md", true},
		{"nested file not matched by bare name", []string{"README.md"}, "a/README.md", false},
		{"deep nested not matched", []string{"README.md"}, "a/b/README.md", false},
		{"globstar root", []string{"**/README.md"}, "README.md", true},
		{"globstar depth 1", []string{"**/README.md"}, "a/README.md", true},
		{"globstar depth 2", []string{"**/README.md"}, "a/b/README.md", true},
		{"globstar other name", []string{"**/README.md"}, "a/README.txt", false},
		{"star no cross slash", []string{"*.md"}, "a/b.md", false},
		{"star root", []string{"*.md"}, "b.md", true},
		{"star wrong ext", []string{"*.md"}, "b.txt", false},
		{"dir star child", []string{"a/*"}, "a/x", true},
		{"dir star grandchild", []string{"a/*"}, "a/x/y", false},
		{"double star alone root file", []string{"**"}, "a", true},
		{"double star alone deep", []string{"**"}, "a/b/c", true},
		{"dir doublestar deep", []string{"a/**"}, "a/x/y", true},
		{"dir doublestar child", []string{"a/**"}, "a/x", true},
		// doublestar v4 semantics: a trailing "/**" also matches the directory
		// itself ("zero or more path elements"), so "a/**" matches "a".
		{"dir doublestar matches dir itself", []string{"a/**"}, "a", true},
		{"dir doublestar other dir", []string{"a/**"}, "b/x", false},
		{"dir doublestar prefix sibling", []string{"a/**"}, "ab/x", false},
		{"case sensitive lower pattern", []string{"readme.md"}, "README.md", false},
		{"case sensitive upper pattern", []string{"README.md"}, "readme.md", false},
		{"ds store root", []string{".DS_Store"}, ".DS_Store", true},
		{"ds store nested needs globstar", []string{".DS_Store"}, "a/.DS_Store", false},
		{"ds store globstar nested", []string{"**/.DS_Store"}, "a/b/.DS_Store", true},
		{"class a", []string{"[ab].txt"}, "a.txt", true},
		{"class b", []string{"[ab].txt"}, "b.txt", true},
		{"class miss", []string{"[ab].txt"}, "c.txt", false},
		{"question mark", []string{"?.txt"}, "a.txt", true},
		{"question mark no slash", []string{"?"}, "a/b", false},
		{"multiple patterns second hits", []string{"x", "*.md"}, "r.md", true},
		{"multiple patterns none hit", []string{"x", "*.md"}, "r.go", false},
		{"unicode", []string{"設定/*.txt"}, "設定/a.txt", true},
		// doublestar: "*" matches the empty string (zero characters).
		{"empty path matched by star", []string{"*"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Compile(tc.patterns)
			if err != nil {
				t.Fatal(err)
			}
			if got := m.Match(tc.path); got != tc.want {
				t.Fatalf("patterns %q Match(%q)=%v want %v", tc.patterns, tc.path, got, tc.want)
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
