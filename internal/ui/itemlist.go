package ui

import (
	"slices"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/shiroppi/dots/internal/model"
)

// arrow introduces the source of an item ("+ target  ← source").
const arrow = "←"

// itemList buffers the lines of one listing so that the arrows of its rows
// can be aligned in one column. Rows without a source and detail lines are
// printed unchanged.
type itemList struct {
	p     *Printer
	lines []listLine
	// grouped makes flush order the item blocks (an item and its detail
	// lines) by outcome: ok, created, replaced, then everything else. The
	// order inside a group is kept.
	grouped bool
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

func (p *Printer) newList() *itemList { return &itemList{p: p} }

// item adds "  <symbol> [word ]<target>", followed by "← source" when
// withSource is set.
func (l *itemList) item(t tone, sym, word string, e model.Entry, withSource bool) {
	mark := sym
	if word != "" {
		mark += " " + word
	}
	target := l.p.display(e.Target)
	ln := listLine{
		head:   itemIndent + l.p.paint(t, mark) + " " + target,
		headW:  textWidth(itemIndent + mark + " " + target),
		rank:   symbolRank(sym),
		isItem: true,
	}
	if withSource {
		ln.hasSrc = true
		ln.src = l.p.displaySource(e.Source)
		ln.srcW = textWidth(ln.src)
	}
	l.lines = append(l.lines, ln)
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
	case symCreate:
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
	l.lines = nil
	for i, b := range blocks {
		// One blank line separates the ok rows from the changed ones.
		if i > 0 && b[0].rank > 0 && blocks[i-1][0].rank == 0 {
			l.lines = append(l.lines, listLine{rank: b[0].rank})
		}
		l.lines = append(l.lines, b...)
	}
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
