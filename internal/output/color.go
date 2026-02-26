// Package output provides styled terminal rendering helpers for claudewatch.
package output

import "github.com/charmbracelet/lipgloss"

// Color constants for consistent styling across the CLI.
var (
	// ColorPrimary is used for headers and emphasis.
	ColorPrimary = lipgloss.Color("#64b5f6")

	// ColorSuccess is used for positive indicators and improvements.
	ColorSuccess = lipgloss.Color("#66bb6a")

	// ColorError is used for negative indicators and regressions.
	ColorError = lipgloss.Color("#ef5350")

	// ColorWarning is used for caution indicators.
	ColorWarning = lipgloss.Color("#fff59d")

	// ColorMuted is used for secondary text and borders.
	ColorMuted = lipgloss.Color("#888888")

	// ColorWhite is used for primary text.
	ColorWhite = lipgloss.Color("#ffffff")
)

// Styles provides reusable lipgloss styles.
var (
	// StyleHeader is used for section headers.
	StyleHeader = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	// StyleSuccess is used for positive values.
	StyleSuccess = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	// StyleError is used for negative values.
	StyleError = lipgloss.NewStyle().
			Foreground(ColorError)

	// StyleWarning is used for cautionary values.
	StyleWarning = lipgloss.NewStyle().
			Foreground(ColorWarning)

	// StyleMuted is used for de-emphasized text.
	StyleMuted = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// StyleBold is used for emphasized text.
	StyleBold = lipgloss.NewStyle().
			Bold(true)

	// StyleLabel is used for metric labels.
	StyleLabel = lipgloss.NewStyle().
			Width(24)

	// StyleValue is used for metric values.
	StyleValue = lipgloss.NewStyle().
			Bold(true).
			Width(12)
)

// noColor tracks whether color output is disabled.
var noColor bool

// SetNoColor disables or enables color output globally.
// When disabled, all package-level styles are reassigned to unstyled renderers.
func SetNoColor(disabled bool) {
	noColor = disabled
	if disabled {
		plain := lipgloss.NewStyle()
		StyleHeader = plain
		StyleSuccess = plain
		StyleError = plain
		StyleWarning = plain
		StyleMuted = plain
		StyleBold = plain
		StyleLabel = plain.Width(24)
		StyleValue = plain.Width(12)
	}
}

// IsNoColor returns whether color output is currently disabled.
func IsNoColor() bool {
	return noColor
}
