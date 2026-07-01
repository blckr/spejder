package tui

import (
	"fmt"
	"strings"

	"codeberg.org/blckr/spejder/internal/db"
)

// lineChart renders a line chart for time-series data.
// Numbers are shown above the line; labels below.
func lineChart(data []db.DayCount, width int) string {
	if len(data) == 0 {
		return dim.Render("no data yet")
	}

	maxVal := 0
	for _, d := range data {
		if d.Count > maxVal {
			maxVal = d.Count
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	// How many data points fit given the width.
	// Each point needs at least 1 char; we may subsample for very wide datasets.
	n := len(data)
	step := 1
	if n > width {
		step = max((n+width-1)/width, 1)
		// rebuild a subsampled slice by summing within each step
		var sub []db.DayCount
		for i := 0; i < n; i += step {
			end := min(i+step, n)
			sum := 0
			for _, d := range data[i:end] {
				sum += d.Count
			}
			sub = append(sub, db.DayCount{Label: data[i].Label, Count: sum})
		}
		data = sub
		n = len(data)
	}

	chartH := 6 // rows for the chart body
	cols := n

	// Normalise heights to chartH rows.
	heights := make([]int, cols)
	for i, d := range data {
		heights[i] = d.Count * chartH / maxVal
		if d.Count > 0 {
			heights[i] = max(heights[i], 1)
		}
	}

	// Build chart rows top-down.
	rows := make([][]rune, chartH)
	for r := range chartH {
		rows[r] = make([]rune, cols)
		for c := range cols {
			rows[r][c] = ' '
		}
	}
	for c, h := range heights {
		for r := range h {
			rows[chartH-1-r][c] = '█'
		}
	}

	var lines []string

	// Values above each bar (only the peak value and selective labels to avoid clutter).
	valLine := make([]byte, cols)
	for i := range valLine {
		valLine[i] = ' '
	}
	// Show the max value label.
	peakStr := fmt.Sprintf("%d", maxVal)
	peakIdx := 0
	for i, d := range data {
		if d.Count == maxVal {
			peakIdx = i
			break
		}
	}
	if peakIdx+len(peakStr) <= cols {
		copy(valLine[peakIdx:], peakStr)
	}
	lines = append(lines, dim.Render(string(valLine)))

	// Chart body.
	for _, row := range rows {
		lines = append(lines, bar.Render(string(row)))
	}

	// Bottom axis.
	axis := strings.Repeat("─", cols)
	lines = append(lines, dim.Render(axis))

	// Label row: first and last label.
	if n > 0 {
		first := data[0].Label
		last := data[n-1].Label
		gap := max(cols-len(first)-len(last), 1)
		labelLine := first + strings.Repeat(" ", gap) + last
		lines = append(lines, dim.Render(labelLine))
	}

	return strings.Join(lines, "\n")
}
