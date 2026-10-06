// Package resolve expands a parsed dots.toml into the concrete symlinks dots
// manages on the current OS (spec §4, §5).
package resolve

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shiroppi/dots/internal/config"
	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/ignore"
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
			common := *g.Common
			common.Ignore = common.Ignore.WithSubtrees(osOnlySubtrees(env.GOOS, common.Source, osOnly), pathx.FoldsCase(env.GOOS))
			c, e := expand(fsys, env, root, rule, model.LayerAuto, &common)
			cands = append(cands, c...)
			errs = append(errs, e...)
		}
		for j := range g.OS[env.GOOS] {
			rule := fmt.Sprintf("auto[%d].%s[%d]", g.Index, env.GOOS, j)
			c, e := expand(fsys, env, root, rule, model.LayerAutoOS, &g.OS[env.GOOS][j])
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
		allDirs := true
		for _, b := range best {
			if b.Kind != model.KindDir {
				allDirs = false
			}
		}
		if allDirs && len(best) > 1 {
			// Several rules agree on a real directory: not a duplicate.
			sort.SliceStable(best, func(i, j int) bool { return best[i].Origin.Rule < best[j].Origin.Rule })
			best = best[:1]
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
			if pw, ok := byKey[pathx.Key(env.GOOS, cur)]; ok && pw.Kind != model.KindDir {
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

// osOnlySources returns the sources of all os_only rules of every auto.<os>
// table, including OSes other than the running one.
func osOnlySources(cfg *config.Config) []string {
	var out []string
	for _, g := range cfg.Auto {
		for _, rules := range g.OS {
			for _, r := range rules {
				if r.OSOnly {
					out = append(out, r.Source)
				}
			}
		}
	}
	return out
}

// osOnlySubtrees returns, for the common rule source commonSrc, the os_only
// sources that lie inside or equal it, expressed relative to commonSrc
// (slash-separated; "" for commonSrc itself). All paths are cleaned relative
// sources; no file system access or symlink resolution takes place.
func osOnlySubtrees(goos, commonSrc string, sources []string) []string {
	depth := len(strings.Split(commonSrc, "/"))
	var out []string
	for _, s := range sources {
		if pathx.Key(goos, s) == pathx.Key(goos, commonSrc) {
			out = append(out, "")
		} else if pathx.Contains(goos, commonSrc, s) {
			out = append(out, strings.Join(strings.Split(s, "/")[depth:], "/"))
		}
	}
	return out
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

// expand walks one auto rule's source directory (spec §5.3).
func expand(fsys fs.Manager, env platform.Env, root, rule string, layer model.Layer, r *config.AutoRule) ([]model.Entry, []error) {
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
	if ents, err := fsys.ReadDir(sourceRoot); err == nil && len(ents) == 0 {
		// Physically empty root: place it as a single link (spec §5.3 rule 4),
		// unless that would replace the home directory or repository root.
		if r.Target.IsBase() {
			return nil, []error{fmt.Errorf("%s: source %s is empty; refusing to replace %s with a symlink", rule, sourceRoot, r.Target.BaseName())}
		}
		return []model.Entry{{
			Target: targetRoot,
			Source: sourceRoot,
			Origin: model.Origin{Layer: layer, Rule: rule},
		}}, nil
	}
	w := &walker{fsys: fsys, rule: rule, layer: layer, sourceRoot: sourceRoot, targetRoot: targetRoot, ign: r.Ignore}
	if n := w.walk(""); n == 0 && len(w.errs) == 0 {
		// Not physically empty but everything was ignored.
		w.emitDir("")
	}
	return w.out, w.errs
}

type walker struct {
	fsys       fs.Manager
	rule       string
	layer      model.Layer
	sourceRoot string
	targetRoot string
	ign        *ignore.Matcher
	out        []model.Entry
	errs       []error
}

func (w *walker) emit(rel string) {
	w.out = append(w.out, model.Entry{
		Target: filepath.Join(w.targetRoot, filepath.FromSlash(rel)),
		Source: filepath.Join(w.sourceRoot, filepath.FromSlash(rel)),
		Origin: model.Origin{Layer: w.layer, Rule: w.rule},
	})
}

func (w *walker) emitDir(rel string) {
	w.out = append(w.out, model.Entry{
		Target: filepath.Join(w.targetRoot, filepath.FromSlash(rel)),
		Source: filepath.Join(w.sourceRoot, filepath.FromSlash(rel)),
		Kind:   model.KindDir,
		Origin: model.Origin{Layer: w.layer, Rule: w.rule},
	})
}

func (w *walker) fail(format string, a ...interface{}) {
	w.errs = append(w.errs, fmt.Errorf("%s: %s", w.rule, fmt.Sprintf(format, a...)))
}

// walk emits the entries below relDir and returns how many it emitted.
func (w *walker) walk(relDir string) int {
	count := 0
	dir := filepath.Join(w.sourceRoot, filepath.FromSlash(relDir))
	entries, err := w.fsys.ReadDir(dir)
	if err != nil {
		w.fail("cannot read directory %s: %v", dir, err)
		return 0
	}
	for _, e := range entries {
		rel := path.Join(relDir, e.Name())
		if w.ign.Match(rel) {
			continue
		}
		full := filepath.Join(w.sourceRoot, filepath.FromSlash(rel))
		mode := e.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			if _, err := w.fsys.Stat(full); err != nil {
				switch {
				case errors.Is(err, fs.ErrLinkLoop):
					w.fail("symlink loop in auto source: %s", full)
				case errors.Is(err, fs.ErrNotExist):
					w.fail("broken symlink in auto source: %s", full)
				default:
					w.fail("cannot check symlink %s: %v", full, err)
				}
				continue
			}
			w.emit(rel)
			count++
		case mode.IsRegular():
			w.emit(rel)
			count++
		case mode.IsDir():
			errsBefore := len(w.errs)
			sub, err := w.fsys.ReadDir(full)
			if err != nil {
				w.fail("cannot read directory %s: %v", full, err)
				continue
			}
			if len(sub) == 0 {
				w.emit(rel)
				count++
			} else if n := w.walk(rel); n > 0 {
				count += n
			} else if len(w.errs) == errsBefore {
				// Everything below was ignored: manage a real directory.
				w.emitDir(rel)
				count++
			}
		default:
			w.fail("unsupported file type (%s) in auto source: %s", mode.Type(), full)
		}
	}
	return count
}
