package apply

import (
	"path/filepath"
	"time"

	"github.com/shiroppi/dots/internal/fs"

	"github.com/shiroppi/dots/internal/statefile"
)

// RecordedLinks returns the state records for the links rep created, replaced
// or found already valid (spec §5.3). root is the repository root.
func RecordedLinks(rep *Report, root string, now time.Time) []statefile.Link {
	if rep == nil {
		return nil
	}
	var out []statefile.Link
	for _, ir := range rep.Items {
		r := ir.Item.Result
		var text string
		switch ir.Outcome {
		case Created, Replaced:
			text = r.Link
		case AlreadyOK:
			// The existing link text is what Readlink reports, which is
			// what stale detection compares against.
			text = r.Current
		default:
			continue
		}
		if text == "" {
			continue
		}
		src := r.Entry.Source
		if rel, err := filepath.Rel(root, src); err == nil {
			src = filepath.ToSlash(rel)
		}
		out = append(out, statefile.Link{
			Target:     r.Entry.Target,
			Link:       text,
			Repository: root,
			Source:     src,
			RecordedAt: now,
		})
	}
	return out
}

// UpdateState applies the record and prune rules of spec §5.3 to st: every
// link of RecordedLinks is recorded (an identical existing record keeps its
// time), then the records whose target no longer exists are dropped. It
// reports whether st changed. fsys is only read.
func UpdateState(fsys fs.Manager, st *statefile.State, rep *Report, root string, now time.Time) bool {
	changed := false
	for _, l := range RecordedLinks(rep, root, now) {
		if old, ok := st.Find(l.Target); ok && old.Link == l.Link && old.Repository == l.Repository && old.Source == l.Source {
			continue
		}
		st.Record(l)
		changed = true
	}
	// A target that cannot be inspected keeps its record.
	if removed, _ := st.PruneMissing(fsys); len(removed) > 0 {
		changed = true
	}
	return changed
}
