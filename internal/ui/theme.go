package ui

import "github.com/charmbracelet/lipgloss"

// Palette keeps the CLI visually consistent: one green accent, one muted tone
// and a couple of semantic colors. Adaptive values keep it readable on light
// and dark terminals.
var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#1A7F4E", Dark: "#4ADE80"}
	// colorAccent2 is the blue end of the brand gradient: green marks the
	// source side of things, blue marks the output side.
	colorAccent2 = lipgloss.AdaptiveColor{Light: "#0E7490", Dark: "#38BDF8"}
	colorSoft    = lipgloss.AdaptiveColor{Light: "#4D7C63", Dark: "#86D5A4"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#6C6A67", Dark: "#8B8885"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#9A9793", Dark: "#5F5D5A"}
	colorOK      = lipgloss.AdaptiveColor{Light: "#2F855A", Dark: "#63C08A"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#B7791F", Dark: "#E0B341"}
	colorErr     = lipgloss.AdaptiveColor{Light: "#C53030", Dark: "#F07171"}
)

var (
	styleTitle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleBrand = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleStep  = lipgloss.NewStyle().Foreground(colorFaint)

	styleHeading  = lipgloss.NewStyle().Foreground(colorSoft).Bold(true)
	styleText     = lipgloss.NewStyle()
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleFaint    = lipgloss.NewStyle().Foreground(colorFaint)
	styleAccent   = lipgloss.NewStyle().Foreground(colorAccent)
	styleAccent2  = lipgloss.NewStyle().Foreground(colorAccent2)
	styleOK       = lipgloss.NewStyle().Foreground(colorOK)
	styleWarn     = lipgloss.NewStyle().Foreground(colorWarn)
	styleErr      = lipgloss.NewStyle().Foreground(colorErr).Bold(true)
	styleSelected = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	// Source panels wear the green side of the gradient, output panels the
	// blue side, echoing the input → output flow of the brand artwork.
	stylePanelSource = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(0, 2)
	stylePanelTarget = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent2).
				Padding(0, 2)

	styleHelp = lipgloss.NewStyle().Foreground(colorFaint)
	// styleKey makes the pressable key stand out from its action label.
	styleKey = lipgloss.NewStyle().Foreground(colorMuted).Bold(true)
)

// cursor renders the pointer used by every selectable list.
func cursor(selected bool) string {
	if selected {
		return styleAccent.Render("❯ ")
	}
	return "  "
}
