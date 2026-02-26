package output

import (
	"fmt"
	"strings"
)

// ScoreBar renders a visual progress bar for a 0-100 score.
// Example: "████████░░ 80/100"
func ScoreBar(score float64, width int) string {
	if width <= 0 {
		width = 20
	}
	filled := int((score / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)

	var style func(string) string
	switch {
	case score >= 70:
		style = func(s string) string { return StyleSuccess.Render(s) }
	case score >= 40:
		style = func(s string) string { return StyleWarning.Render(s) }
	default:
		style = func(s string) string { return StyleError.Render(s) }
	}

	return fmt.Sprintf("%s %s", style(bar), StyleMuted.Render(fmt.Sprintf("%.0f/100", score)))
}

// TrendArrow returns a styled trend indicator for a delta value.
// Positive delta shows an up arrow, negative shows down, zero shows a dash.
// The improved parameter indicates whether higher values are better.
func TrendArrow(delta float64, higherIsBetter bool) string {
	if delta == 0 {
		return StyleMuted.Render("─")
	}

	isPositive := delta > 0
	isImproved := (isPositive && higherIsBetter) || (!isPositive && !higherIsBetter)

	var arrow string
	if isPositive {
		arrow = fmt.Sprintf("▲ +%.1f", delta)
	} else {
		arrow = fmt.Sprintf("▼ %.1f", delta)
	}

	if isImproved {
		return StyleSuccess.Render(arrow)
	}
	return StyleError.Render(arrow)
}

// TrendArrowPercent returns a styled trend indicator for a percentage delta.
func TrendArrowPercent(delta float64, higherIsBetter bool) string {
	if delta == 0 {
		return StyleMuted.Render("─")
	}

	isPositive := delta > 0
	isImproved := (isPositive && higherIsBetter) || (!isPositive && !higherIsBetter)

	var arrow string
	if isPositive {
		arrow = fmt.Sprintf("▲ +%.0f%%", delta)
	} else {
		arrow = fmt.Sprintf("▼ %.0f%%", delta)
	}

	if isImproved {
		return StyleSuccess.Render(arrow)
	}
	return StyleError.Render(arrow)
}

// Section prints a styled section header with a horizontal rule.
func Section(title string) string {
	header := StyleHeader.Render(title)
	rule := StyleMuted.Render(strings.Repeat("─", 66))
	return fmt.Sprintf("\n %s\n %s", header, rule)
}
