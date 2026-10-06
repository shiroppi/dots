// Package config locates, loads and strictly validates dots.toml (spec §3, §5).
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/ignore"
	"github.com/shiroppi/dots/internal/pathx"
)

// FileName is the name of the configuration file.
const FileName = "dots.toml"

// ErrNotFound is wrapped by Find when no dots.toml exists.
var ErrNotFound = errors.New("dots.toml not found")

// Find returns the absolute path of the first dots.toml found in dir or any
// parent (spec §3). It stops at the volume/filesystem root. A dots.toml that
// is a directory is an error.
func Find(fsys fs.Manager, dir string) (string, error) {
	start, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", dir, err)
	}
	cur := start
	for {
		cand := filepath.Join(cur, FileName)
		info, err := fsys.Stat(cand)
		switch {
		case err == nil:
			if info.IsDir() {
				return "", fmt.Errorf("%s is a directory, not a file", cand)
			}
			return cand, nil
		case errors.Is(err, fs.ErrNotExist):
			// keep searching in the parent
		default:
			return "", fmt.Errorf("cannot check %s: %w", cand, err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("%w in %s or any parent directory", ErrNotFound, start)
		}
		cur = parent
	}
}

// Config is the parsed dots.toml.
type Config struct {
	// Dots holds [dots]; DotsOS holds [dots.<goos>] keyed by GOOS name.
	Dots   []Link
	DotsOS map[string][]Link
	Auto   []AutoGroup
}

// Link is one flat-notation entry ("target" = "source").
type Link struct {
	RawTarget string       // exactly as written
	Target    pathx.Target // parsed
	Source    string       // cleaned slash-separated relative source
}

// AutoGroup is one [[auto]] element with its [[auto.<goos>]] rules.
type AutoGroup struct {
	Index  int                   // position in the [[auto]] array
	Common *AutoRule             // nil when the element has neither source nor target
	OS     map[string][]AutoRule // [[auto.<goos>]] rules in file order
}

// AutoRule is one auto expansion rule.
type AutoRule struct {
	RawTarget string
	Target    pathx.Target
	Source    string
	Ignore    *ignore.Matcher // never nil
	OSOnly    bool            // os_only (auto.<os> rules only)
}

var knownOS = map[string]bool{}

func init() {
	for _, n := range strings.Fields("aix android darwin dragonfly freebsd illumos ios js linux netbsd openbsd plan9 solaris wasip1 windows") {
		knownOS[n] = true
	}
}

// Load reads and parses the file at path. Errors are prefixed with path.
func Load(fsys fs.Manager, path string) (*Config, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, prefixErr(path, err)
	}
	return cfg, nil
}

// prefixErr prefixes every leaf of a joined error with path.
func prefixErr(path string, err error) error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range j.Unwrap() {
			out = append(out, prefixErr(path, e))
		}
		return errors.Join(out...)
	}
	return fmt.Errorf("%s: %w", path, err)
}

type parser struct{ errs []string }

func (p *parser) errf(format string, a ...interface{}) {
	p.errs = append(p.errs, fmt.Sprintf(format, a...))
}

// Parse parses and strictly validates configuration data. All problems are
// collected and returned joined, sorted by message.
func Parse(data []byte) (*Config, error) {
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		var de *toml.DecodeError
		if errors.As(err, &de) {
			row, col := de.Position()
			return nil, fmt.Errorf("TOML syntax error at line %d, column %d: %s", row, col, de.Error())
		}
		return nil, fmt.Errorf("TOML syntax error: %w", err)
	}
	p := &parser{}
	cfg := &Config{DotsOS: map[string][]Link{}}

	for _, k := range sortedKeys(raw) {
		switch k {
		case "dots":
			p.parseDots(raw[k], cfg)
		case "auto":
			p.parseAuto(raw[k], cfg)
		default:
			p.errf("unknown top-level key %q (only \"dots\" and \"auto\" are allowed)", k)
		}
	}
	if len(p.errs) > 0 {
		sort.Strings(p.errs)
		errs := make([]error, len(p.errs))
		for i, m := range p.errs {
			errs[i] = errors.New(m)
		}
		return nil, errors.Join(errs...)
	}
	sortLinks(cfg.Dots)
	for _, l := range cfg.DotsOS {
		sortLinks(l)
	}
	return cfg, nil
}

func sortLinks(l []Link) {
	sort.Slice(l, func(i, j int) bool { return l[i].RawTarget < l[j].RawTarget })
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func typeName(v interface{}) string {
	switch v.(type) {
	case string:
		return "a string"
	case map[string]interface{}:
		return "a table"
	case []interface{}:
		return "an array"
	case bool:
		return "a boolean"
	case int64:
		return "an integer"
	case float64:
		return "a float"
	default:
		return fmt.Sprintf("a %T value", v)
	}
}

func (p *parser) link(loc, rawTarget string, v interface{}) (Link, bool) {
	src, ok := v.(string)
	if !ok {
		p.errf("%s: must be a string (source path), got %s", loc, typeName(v))
		return Link{}, false
	}
	t, err := pathx.ParseTarget(rawTarget)
	if err != nil {
		p.errf("%s: invalid target: %v", loc, err)
		return Link{}, false
	}
	s, err := pathx.ParseSource(src)
	if err != nil {
		p.errf("%s: invalid source: %v", loc, err)
		return Link{}, false
	}
	return Link{RawTarget: rawTarget, Target: t, Source: s}, true
}

func (p *parser) parseDots(v interface{}, cfg *Config) {
	tbl, ok := v.(map[string]interface{})
	if !ok {
		p.errf("dots: must be a table, got %s", typeName(v))
		return
	}
	for _, k := range sortedKeys(tbl) {
		switch val := tbl[k].(type) {
		case string:
			if l, ok := p.link(fmt.Sprintf("dots.%q", k), k, val); ok {
				cfg.Dots = append(cfg.Dots, l)
			}
		case map[string]interface{}:
			if !knownOS[k] {
				p.errf("dots.%s: unknown OS name %q; quote target paths, e.g. \"~/.vimrc\" = \"vimrc\"", k, k)
				continue
			}
			for _, tk := range sortedKeys(val) {
				if l, ok := p.link(fmt.Sprintf("dots.%s.%q", k, tk), tk, val[tk]); ok {
					cfg.DotsOS[k] = append(cfg.DotsOS[k], l)
				}
			}
		default:
			p.errf("dots.%q: must be a string (source path), got %s", k, typeName(val))
		}
	}
}

func (p *parser) parseAuto(v interface{}, cfg *Config) {
	arr, ok := v.([]interface{})
	if !ok {
		if _, isTbl := v.(map[string]interface{}); isTbl {
			p.errf("auto: must be an array of tables; use [[auto]] instead of [auto]")
		} else {
			p.errf("auto: must be an array of tables ([[auto]]), got %s", typeName(v))
		}
		return
	}
	for i, el := range arr {
		loc := fmt.Sprintf("auto[%d]", i)
		tbl, ok := el.(map[string]interface{})
		if !ok {
			p.errf("%s: must be a table, got %s", loc, typeName(el))
			continue
		}
		g := AutoGroup{Index: i, OS: map[string][]AutoRule{}}
		_, hasSrc := tbl["source"]
		_, hasTgt := tbl["target"]
		_, hasIgn := tbl["ignore"]
		hasOS := false
		for _, k := range sortedKeys(tbl) {
			switch {
			case k == "source" || k == "target" || k == "ignore":
			case k == "os_only":
				p.errf("%s: os_only is only allowed in auto.<os> rules", loc)
			case knownOS[k]:
				hasOS = true
				sub, ok := tbl[k].([]interface{})
				if !ok {
					if _, isTbl := tbl[k].(map[string]interface{}); isTbl {
						p.errf("%s.%s: must be an array of tables; use [[auto.%s]] instead of [auto.%s]", loc, k, k, k)
					} else {
						p.errf("%s.%s: must be an array of tables ([[auto.%s]]), got %s", loc, k, k, typeName(tbl[k]))
					}
					continue
				}
				for j, se := range sub {
					sloc := fmt.Sprintf("%s.%s[%d]", loc, k, j)
					st, ok := se.(map[string]interface{})
					if !ok {
						p.errf("%s: must be a table, got %s", sloc, typeName(se))
						continue
					}
					if r, ok := p.rule(sloc, st, true); ok {
						g.OS[k] = append(g.OS[k], r)
					}
				}
			default:
				p.errf("%s: unknown key %q (allowed: source, target, ignore, or an OS name such as [[auto.darwin]])", loc, k)
			}
		}
		switch {
		case hasSrc || hasTgt:
			if r, ok := p.rule(loc, tbl, false); ok {
				g.Common = &r
			}
		case hasIgn:
			p.errf("%s: ignore requires source and target", loc)
		case !hasOS:
			p.errf("%s: empty [[auto]] group (needs source/target or an [[auto.<os>]] rule)", loc)
		}
		cfg.Auto = append(cfg.Auto, g)
	}
}

// rule validates source/target/ignore (and os_only for OS rules) of one table.
// When strictKeys is set, other keys are reported (OS rules); for the group
// table the caller has already checked keys.
func (p *parser) rule(loc string, tbl map[string]interface{}, strictKeys bool) (AutoRule, bool) {
	ok := true
	if strictKeys {
		for _, k := range sortedKeys(tbl) {
			if k != "source" && k != "target" && k != "ignore" && k != "os_only" {
				p.errf("%s: unknown key %q (allowed: source, target, ignore, os_only)", loc, k)
				ok = false
			}
		}
	}
	var osOnly bool
	if v, present := tbl["os_only"]; present && strictKeys {
		b, isBool := v.(bool)
		if !isBool {
			p.errf("%s.os_only: must be a boolean, got %s", loc, typeName(v))
			ok = false
		}
		osOnly = b
	}
	str := func(key string) (string, bool) {
		v, present := tbl[key]
		if !present {
			p.errf("%s: missing required key %q", loc, key)
			return "", false
		}
		s, isStr := v.(string)
		if !isStr {
			p.errf("%s.%s: must be a string, got %s", loc, key, typeName(v))
			return "", false
		}
		return s, true
	}
	r := AutoRule{OSOnly: osOnly}
	if rawT, got := str("target"); got {
		t, err := pathx.ParseTarget(rawT)
		if err != nil {
			p.errf("%s.target: invalid target: %v", loc, err)
			ok = false
		}
		r.RawTarget, r.Target = rawT, t
	} else {
		ok = false
	}
	if rawS, got := str("source"); got {
		s, err := pathx.ParseSource(rawS)
		if err != nil {
			p.errf("%s.source: invalid source: %v", loc, err)
			ok = false
		}
		r.Source = s
	} else {
		ok = false
	}
	var pats []string
	if v, present := tbl["ignore"]; present {
		arr, isArr := v.([]interface{})
		if !isArr {
			p.errf("%s.ignore: must be an array of strings, got %s", loc, typeName(v))
			ok = false
		} else {
			for i, e := range arr {
				s, isStr := e.(string)
				if !isStr {
					p.errf("%s.ignore[%d]: must be a string, got %s", loc, i, typeName(e))
					ok = false
					continue
				}
				pats = append(pats, s)
			}
		}
	}
	if ok {
		m, err := ignore.Compile(pats)
		if err != nil {
			for _, line := range strings.Split(err.Error(), "\n") {
				p.errf("%s.%s", loc, line)
			}
			ok = false
		}
		r.Ignore = m
	}
	return r, ok
}
