package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/shiroppi/dots/internal/model"
)

// arrow introduces the source of an item ("+ target  ← source").
const arrow = "←"

// symFold starts the line that stands for folded unchanged rows (spec §8.5).
const symFold = "…"

// SourceMode is how the rows of a listing show their source.
type SourceMode int

const (
	// SourceSimple shows "← source" on the rows that change something
	// (created, replaced, removed). It is the zero value and the default of
	// apply.
	SourceSimple SourceMode = iota
	// SourceOff shows no source.
	SourceOff
	// SourceVerbose shows "← source  rule" on every row that has a source.
	SourceVerbose
)

// itemList buffers the lines of one listing so that the arrows of its rows
// can be aligned in one column. Rows without a source and detail lines are
// printed unchanged.
type itemList struct {
	p     *Printer
	lines []listLine
	// grouped makes flush order the item blocks (an item and its detail
	// lines) by outcome: ok, created (missing in status), replaced, then
	// everything else. The order inside a group is kept. It is shared by the
	// plan, the apply report and status.
	grouped bool
	// mode selects the rows that show their source.
	mode SourceMode
	// showUnchanged, when 0 or more, folds the ok rows of a grouped list
	// after the first showUnchanged of them (spec §8.5). Negative means no
	// folding.
	showUnchanged int
}

type listLine struct {
	head   string // the line up to the target, or the whole line when hasSrc is false
	headW  int    // display width of head without escapes
	src    string // source shown after the arrow
	srcW   int
	hasSrc bool
	rank   int  // outcome group of the item this line belongs to
	isItem bool // first line of an item block
}

func (p *Printer) newList(mode SourceMode) *itemList {
	return &itemList{p: p, mode: mode, showUnchanged: -1}
}

// item adds "  <symbol> [word ]<target>", followed by "← source" as mode
// says: changed marks the rows that SourceSimple annotates. SourceVerbose
// also adds the rule of the entry when it has one. Entries without a source
// never get an arrow.
func (l *itemList) item(t tone, sym, word string, e model.Entry, changed bool) {
	ln := l.head(t, sym, word, e)
	if e.Source != "" && (l.mode == SourceVerbose || l.mode == SourceSimple && changed) {
		ln.hasSrc = true
		ln.src = l.p.displaySource(e.Source)
		if l.mode == SourceVerbose && e.Origin.Rule != "" {
			ln.src += "  " + e.Origin.String()
		}
		ln.srcW = textWidth(ln.src)
	}
	l.lines = append(l.lines, ln)
}

// head builds the first line of an item up to the target.
func (l *itemList) head(t tone, sym, word string, e model.Entry) listLine {
	mark := sym
	if word != "" {
		mark += " " + word
	}
	target := l.p.display(e.Target)
	return listLine{
		head:   itemIndent + l.p.paint(t, mark) + " " + target,
		headW:  textWidth(itemIndent + mark + " " + target),
		rank:   symbolRank(sym),
		isItem: true,
	}
}

// detail adds one line indented under its item.
func (l *itemList) detail(t tone, text string) {
	rank := 0
	if n := len(l.lines); n > 0 {
		rank = l.lines[n-1].rank
	}
	l.lines = append(l.lines, listLine{head: detailIndent + l.p.paint(t, text), headW: textWidth(detailIndent + text), rank: rank})
}

// errorDetails adds every leaf of err as a detail line.
func (l *itemList) errorDetails(err error) {
	for _, e := range Flatten(err) {
		l.detail(toneRed, oneLine(l.p.DisplayText(e.Error())))
	}
}

// flush prints the buffered lines.
func (l *itemList) flush() {
	if l.grouped {
		l.sortBlocks()
	}
	var heads, srcs []int
	for _, ln := range l.lines {
		if ln.hasSrc {
			heads = append(heads, ln.headW)
			srcs = append(srcs, ln.srcW)
		}
	}
	pads := arrowPadding(heads, srcs, l.p.Width)
	i := 0
	for _, ln := range l.lines {
		if !ln.hasSrc {
			l.p.printf("%s\n", ln.head)
			continue
		}
		l.p.printf("%s%s%s\n", ln.head, strings.Repeat(" ", pads[i]), l.p.paint(toneDim, arrow+" "+ln.src))
		i++
	}
}

// symbolRank is the position of a symbol's group in a grouped list.
func symbolRank(sym string) int {
	switch sym {
	case symOK:
		return 0
	case symCreate, symMissing:
		return 1
	case symReplace:
		return 2
	}
	return 3
}

// sortBlocks stably orders item blocks by rank.
func (l *itemList) sortBlocks() {
	var blocks [][]listLine
	for _, ln := range l.lines {
		if ln.isItem || len(blocks) == 0 {
			blocks = append(blocks, nil)
		}
		blocks[len(blocks)-1] = append(blocks[len(blocks)-1], ln)
	}
	slices.SortStableFunc(blocks, func(a, b []listLine) int { return a[0].rank - b[0].rank })
	if l.showUnchanged >= 0 {
		blocks = l.fold(blocks)
	}
	l.lines = slices.Concat(blocks...)
}

// fold replaces the ok blocks after the first showUnchanged with one dim
// "… N unchanged" line (spec §8.5). The blocks are sorted, so the ok ones
// lead. Folding a single block would not save a line, so U ok blocks are
// folded only when U > showUnchanged+1.
func (l *itemList) fold(blocks [][]listLine) [][]listLine {
	u := 0
	for u < len(blocks) && blocks[u][0].rank == symbolRank(symOK) {
		u++
	}
	k := l.showUnchanged
	if u <= k+1 {
		return blocks
	}
	text := fmt.Sprintf("%s %d unchanged", symFold, u-k)
	folded := []listLine{{
		head:   itemIndent + l.p.paint(toneDim, text),
		headW:  textWidth(itemIndent + text),
		isItem: true,
	}}
	return slices.Concat(blocks[:k], [][]listLine{folded}, blocks[u:])
}

// arrowPadding returns, for each row, the spaces to print between its target
// and the arrow. headW and srcW are the display widths of "symbol + target"
// and of the source. Arrows line up at the longest head plus a gap of two. If
// the widest aligned row would not fit in width columns (width <= 0 means
// unknown), nothing is aligned and every row gets just the gap.
func arrowPadding(headW, srcW []int, width int) []int {
	const gap = 2
	longest := 0
	for _, w := range headW {
		longest = max(longest, w)
	}
	aligned := true
	if width > 0 {
		for _, w := range srcW {
			if longest+gap+textWidth(arrow)+1+w > width {
				aligned = false
				break
			}
		}
	}
	pads := make([]int, len(headW))
	for i, w := range headW {
		pads[i] = gap
		if aligned {
			pads[i] += longest - w
		}
	}
	return pads
}

// textWidth is the number of terminal columns s occupies (East Asian wide
// characters count 2).
func textWidth(s string) int { return runewidth.StringWidth(s) }
