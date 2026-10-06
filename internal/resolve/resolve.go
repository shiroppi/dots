// Package resolve expands a parsed dots.toml into the concrete symlinks dots
// manages on the current OS (spec §4, §5).
package resolve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shiroppi/dots/internal/config"
	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/pathx"
	"github.com/shiroppi/dots/internal/platform"
)

// Resolve expands cfg for env.GOOS into concrete entries (spec §4, §5).
// root = filepath.Dir(configPath). Returns (nil, err) when any configuration
// problem exists; err is an errors.Join of all problems.
func Resolve(fsys fs.Manager, env platform.Env, configPath string, cfg *config.Config) (*model.Resolution, error) {
	root := filepath.Dir(configPath)
	var errs []error
	var cands []model.Entry

	// Flat notation.
	flat := func(links []config.Link, layer model.Layer, prefix string) {
		for _, l := range links {
			rule := fmt.Sprintf("%s.%q", prefix, l.RawTarget)
			if l.Target.IsBase() {
				errs = append(errs, fmt.Errorf("%s: target %q is %s itself and cannot be a symlink", rule, l.RawTarget, l.Target.BaseName()))
				continue
			}
			target, err := l.Target.Resolve(root, env.Home)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", rule, err))
				continue
			}
			cands = append(cands, model.Entry{
				Target: target,
				Source: pathx.ResolveSource(root, l.Source),
				Origin: model.Origin{Layer: layer, Rule: rule},
			})
		}
	}
	flat(cfg.Dots, model.LayerDots, "dots")
	flat(cfg.DotsOS[env.GOOS], model.LayerDotsOS, "dots."+env.GOOS)

	// Auto notation.
	osOnly := osOnlySources(cfg)
	for _, g := range cfg.Auto {
		if g.Common != nil {
			rule := fmt.Sprintf("auto[%d]", g.Index)
			excl, nestedErrs := osOnlyExclusions(env.GOOS, rule, g.Common, osOnly)
			errs = append(errs, nestedErrs...)
			c, e := expand(fsys, env, root, rule, model.LayerAuto, g.Common, excl)
			cands = append(cands, c...)
			errs = append(errs, e...)
		}
		for j := range g.OS[env.GOOS] {
			rule := fmt.Sprintf("auto[%d].%s[%d]", g.Index, env.GOOS, j)
			c, e := expand(fsys, env, root, rule, model.LayerAutoOS, &g.OS[env.GOOS][j], nil)
			cands = append(cands, c...)
			errs = append(errs, e...)
		}
	}

	// Merge by normalised target.
	groups := map[string][]model.Entry{}
	var keys []string
	for _, c := range cands {
		k := pathx.Key(env.GOOS, c.Target)
		if _, seen := groups[k]; !seen {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], c)
	}
	sort.Strings(keys)

	var winners []model.Entry
	var overrides []model.Override
	for _, k := range keys {
		g := groups[k]
		top := g[0].Origin.Layer
		for _, c := range g {
			if c.Origin.Layer > top {
				top = c.Origin.Layer
			}
		}
		var best []model.Entry
		for _, c := range g {
			if c.Origin.Layer == top {
				best = append(best, c)
			}
		}
		if len(best) > 1 {
			rules := make([]string, len(best))
			for i, b := range best {
				rules[i] = b.Origin.Rule
			}
			sort.Strings(rules)
			errs = append(errs, fmt.Errorf("duplicate target %s: %s have the same priority (%s)", filepath.Clean(best[0].Target), joinRules(rules), top))
			continue
		}
		w := best[0]
		winners = append(winners, w)
		for _, c := range g {
			if c.Origin.Layer == top {
				continue
			}
			overrides = append(overrides, model.Override{Target: w.Target, Winner: w.Origin, Loser: c})
		}
	}

	// Parent/child conflicts among winners.
	byKey := map[string]model.Entry{}
	for _, w := range winners {
		byKey[pathx.Key(env.GOOS, w.Target)] = w
	}
	for _, w := range winners {
		cur := filepath.Clean(w.Target)
		for {
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
			if pw, ok := byKey[pathx.Key(env.GOOS, cur)]; ok {
				errs = append(errs, fmt.Errorf("target %s (%s) is inside target %s (%s); a target cannot be placed below another managed target",
					filepath.Clean(w.Target), w.Origin.Rule, filepath.Clean(pw.Target), pw.Origin.Rule))
			}
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	sort.Slice(winners, func(i, j int) bool {
		return filepath.Clean(winners[i].Target) < filepath.Clean(winners[j].Target)
	})
	sort.SliceStable(overrides, func(i, j int) bool {
		a, b := overrides[i], overrides[j]
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Loser.Origin.Rule < b.Loser.Origin.Rule
	})
	for i := range winners {
		winners[i].Target = filepath.Clean(winners[i].Target)
		winners[i].Source = filepath.Clean(winners[i].Source)
	}
	return &model.Resolution{ConfigPath: configPath, Root: root, Entries: winners, Overrides: overrides}, nil
}

// osOnlySource is the source of one os_only rule and its locator.
type osOnlySource struct {
	source string
	rule   string // e.g. auto[1].darwin[0]
}

// osOnlySources returns the os_only rules of every auto.<os> table, including
// OSes other than the running one, in a deterministic order.
func osOnlySources(cfg *config.Config) []osOnlySource {
	var out []osOnlySource
	for _, g := range cfg.Auto {
		for goos, rules := range g.OS {
			for j, r := range rules {
				if r.OSOnly {
					out = append(out, osOnlySource{source: r.Source, rule: fmt.Sprintf("auto[%d].%s[%d]", g.Index, goos, j)})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rule < out[j].rule })
	return out
}

// exclusions lists what os_only rules remove from one common auto rule.
type exclusions struct {
	all   bool            // an os_only source equals the common source
	names map[string]bool // normalised first-level names excluded
}

// osOnlyExclusions computes the first-level items of the common rule that the
// os_only sources exclude (spec §5.2.1). An os_only source nested two or more
// levels below the common source is a configuration error unless the
// first-level item containing it is already excluded by another os_only rule
// or by the common rule's ignore. No file system access or symlink
// resolution takes place.
func osOnlyExclusions(goos, rule string, common *config.AutoRule, sources []osOnlySource) (*exclusions, []error) {
	ex := &exclusions{names: map[string]bool{}}
	depth := len(strings.Split(common.Source, "/"))
	type nested struct {
		first string // first-level name below the common source
		src   osOnlySource
	}
	var deep []nested
	for _, s := range sources {
		switch {
		case pathx.Key(goos, s.source) == pathx.Key(goos, common.Source):
			ex.all = true
		case pathx.Contains(goos, common.Source, s.source):
			parts := strings.Split(s.source, "/")[depth:]
			if len(parts) == 1 {
				ex.names[nameKey(goos, parts[0])] = true
			} else {
				deep = append(deep, nested{first: parts[0], src: s})
			}
		}
	}
	var errs []error
	for _, d := range deep {
		if ex.all || ex.names[nameKey(goos, d.first)] || common.Ignore.Match(d.first) {
			continue
		}
		errs = append(errs, fmt.Errorf("%s: os_only source %s is nested more than one level below %s source %s; auto links only the first level, so %s/%s would still be linked on every OS (point os_only at %s/%s or ignore %q in %s)",
			d.src.rule, d.src.source, rule, common.Source, common.Source, d.first, common.Source, d.first, d.first, rule))
	}
	return ex, errs
}

func nameKey(goos, name string) string {
	if pathx.FoldsCase(goos) {
		return strings.ToLower(name)
	}
	return name
}

func joinRules(r []string) string {
	s := ""
	for i, x := range r {
		if i > 0 {
			s += ", "
		}
		s += x
	}
	return s
}

// expand links every first-level item of one auto rule's source directory
// (spec §5.3). ex lists the items removed by os_only rules (nil for none).
func expand(fsys fs.Manager, env platform.Env, root, rule string, layer model.Layer, r *config.AutoRule, ex *exclusions) ([]model.Entry, []error) {
	sourceRoot := pathx.ResolveSource(root, r.Source)
	targetRoot, err := r.Target.Resolve(root, env.Home)
	if err != nil {
		return nil, []error{fmt.Errorf("%s: %w", rule, err)}
	}
	info, err := fsys.Lstat(sourceRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, []error{fmt.Errorf("%s: auto source %s does not exist", rule, sourceRoot)}
		}
		return nil, []error{fmt.Errorf("%s: cannot read auto source %s: %w", rule, sourceRoot, err)}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, []error{fmt.Errorf("%s: auto source %s must be a directory (not a file or symlink)", rule, sourceRoot)}
	}
	entries, err := fsys.ReadDir(sourceRoot)
	if err != nil {
		return nil, []error{fmt.Errorf("%s: cannot read directory %s: %w", rule, sourceRoot, err)}
	}
	var out []model.Entry
	var errs []error
	fail := func(format string, a ...interface{}) {
		errs = append(errs, fmt.Errorf("%s: %s", rule, fmt.Sprintf(format, a...)))
	}
	for _, e := range entries {
		name := e.Name()
		if r.Ignore.Match(name) {
			continue
		}
		if ex != nil && (ex.all || ex.names[nameKey(env.GOOS, name)]) {
			continue
		}
		full := filepath.Join(sourceRoot, name)
		mode := e.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			if _, err := fsys.Stat(full); err != nil {
				switch {
				case errors.Is(err, fs.ErrLinkLoop):
					fail("symlink loop in auto source: %s", full)
				case errors.Is(err, fs.ErrNotExist):
					fail("broken symlink in auto source: %s", full)
				default:
					fail("cannot check symlink %s: %v", full, err)
				}
				continue
			}
		case mode.IsRegular(), mode.IsDir():
		default:
			fail("unsupported file type (%s) in auto source: %s", mode.Type(), full)
			continue
		}
		out = append(out, model.Entry{
			Target: filepath.Join(targetRoot, name),
			Source: full,
			Origin: model.Origin{Layer: layer, Rule: rule},
		})
	}
	return out, errs
}
