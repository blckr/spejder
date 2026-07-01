package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type drillItem struct {
	label string
	count int
	key   string // lookup key for next level (may differ from display label)
}

type drillLevel struct {
	title  string
	items  []drillItem
	sel    int
	isLeaf bool // true when items are IPs — Enter opens detail modal instead of drilling
}

type drillPanel struct {
	stack  []drillLevel
	width  int
	height int
}

func newDrillPanel(title string, items []drillItem, w, h int) drillPanel {
	return drillPanel{
		stack:  []drillLevel{{title: title, items: items}},
		width:  w,
		height: h,
	}
}

// updateRoot replaces the items at the bottom of the stack (top-level data) without
// disturbing any drill levels the user has navigated into.
func (p *drillPanel) updateRoot(items []drillItem) {
	if len(p.stack) == 0 {
		return
	}
	p.stack[0].items = items
	if p.stack[0].sel >= len(items) {
		p.stack[0].sel = max(len(items)-1, 0)
	}
}

func (p *drillPanel) push(title string, items []drillItem, leaf bool) {
	p.stack = append(p.stack, drillLevel{title: title, items: items, isLeaf: leaf})
}

func (p *drillPanel) pop() bool {
	if len(p.stack) <= 1 {
		return false
	}
	p.stack = p.stack[:len(p.stack)-1]
	return true
}

func (p *drillPanel) current() *drillLevel {
	if len(p.stack) == 0 {
		return nil
	}
	return &p.stack[len(p.stack)-1]
}

func (p *drillPanel) moveUp() {
	cur := p.current()
	if cur == nil || cur.sel == 0 {
		return
	}
	cur.sel--
}

func (p *drillPanel) moveDown() {
	cur := p.current()
	if cur == nil || cur.sel >= len(cur.items)-1 {
		return
	}
	cur.sel++
}

func (p *drillPanel) selectedKey() string {
	cur := p.current()
	if cur == nil || len(cur.items) == 0 {
		return ""
	}
	return cur.items[cur.sel].key
}

func (p drillPanel) view(focused bool) string {
	cur := p.current()
	if cur == nil {
		return ""
	}

	// Width(n) wraps at n-leftPad-rightPad = n-2; border is applied after.
	// So the usable content width is p.width - 2.
	cw := p.width - 2

	// breadcrumb path
	var crumbs []string
	for _, l := range p.stack {
		crumbs = append(crumbs, l.title)
	}
	crumbStr := strings.Join(crumbs, " › ")
	titleStr := header.Render(crumbStr)

	lines := []string{titleStr}

	if len(cur.items) == 0 {
		lines = append(lines, dim.Render("no data yet"))
	} else if cur.isLeaf {
		for i, item := range cur.items {
			line := item.label
			if focused && i == cur.sel {
				line = active.Render(line)
			}
			lines = append(lines, line)
		}
	} else {
		// Measure the longest label and widest count so the prefix is always exact.
		maxLabelLen := 2
		maxCountLen := 1
		for _, item := range cur.items {
			if len(item.label) > maxLabelLen {
				maxLabelLen = len(item.label)
			}
			if l := len(strconv.Itoa(item.count)); l > maxCountLen {
				maxCountLen = l
			}
		}
		// prefix = label + " " + count + " "
		prefixLen := maxLabelLen + 1 + maxCountLen + 1
		bw := max(cw-prefixLen, 4)
		maxCount := 1
		for _, item := range cur.items {
			if item.count > maxCount {
				maxCount = item.count
			}
		}
		// Use log scale when the largest value is more than 10× the second-largest,
		// so dominant categories don't make everything else invisible.
		minNonZero := maxCount
		for _, item := range cur.items {
			if item.count > 0 && item.count < minNonZero {
				minNonZero = item.count
			}
		}
		useLog := maxCount > 0 && maxCount/max(minNonZero, 1) > 10
		logMax := math.Log(float64(maxCount) + 1)

		barFilled := func(count int) int {
			if useLog {
				if count == 0 {
					return 0
				}
				return min(int(math.Round(math.Log(float64(count)+1)/logMax*float64(bw))), bw)
			}
			return min(int(math.Round(float64(count)/float64(maxCount)*float64(bw))), bw)
		}

		for i, item := range cur.items {
			lbl := item.label
			if lbl == "" {
				lbl = "??"
			}
			filled := barFilled(item.count)
			b := bar.Render(strings.Repeat("█", filled)) + dim.Render(strings.Repeat("░", bw-filled))
			// Apply selection only to the text prefix so the bar colours are never overridden.
			prefix := fmt.Sprintf("%-*s %*d ", maxLabelLen, lbl, maxCountLen, item.count)
			if focused && i == cur.sel {
				prefix = selRow.Render(prefix)
			}
			lines = append(lines, prefix+b)
		}
	}

	depth := len(p.stack)
	if depth > 1 {
		lines = append(lines, "", dim.Render("← esc to go back"))
	}
	if focused && len(cur.items) > 0 {
		if cur.isLeaf {
			lines = append(lines, dim.Render("↑↓ navigate  enter: details"))
		} else {
			lines = append(lines, dim.Render("↑↓ navigate  enter: drill in"))
		}
	}

	content := strings.Join(lines, "\n")

	st := panel.Width(p.width)
	if focused {
		st = st.BorderForeground(lipgloss.Color("4"))
	}
	return st.Render(content)
}
