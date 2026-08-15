package ui

import (
	_ "embed"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// chameleonANSI is the mascot as hand drawn truecolor pixel art, designed at
// this exact size (regenerate with assets/gen.go). It needs 24-bit color, so
// the block mascot below stays as the fallback.
//
//go:embed assets/chameleon.ansi
var chameleonANSI string

// supportsTruecolor reports whether the terminal advertises 24-bit color.
func supportsTruecolor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	return strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit")
}

// The banner is a hand drawn take on the project's chameleon mascot: the
// play-button eye, the arched body and the tail dissolving into pixels with
// the same green-to-blue fade as the original artwork. Block characters only,
// so it renders in any monospace font.

// mascotLines pairs the solid body with the pixel trail that follows it.
var mascotLines = []struct{ body, trail string }{
	{body: `   ▄▄▄▄▄`, trail: ``},
	{body: ` ▄█  ▶   █▄▄▄▄▄`, trail: `     ▪ ■ ▝`},
	{body: ` ▀█▄▄▄▄▄▄▄▄▄▄▄███▄`, trail: `  ■ ▪ ■ ▘`},
	{body: `   ▀▀    ▀▀    ▚▘`, trail: ` ▪ ▝`},
}

// wordmarkLines spell "VOPT" in half blocks, small enough to never wrap on an
// 80 column terminal.
var wordmarkLines = []string{
	`█ █ █▀█ █▀█ ▀█▀`,
	`▀▄▀ █▄█ █▀▀  █ `,
}

// trailColors fade the dissolving pixels from the mascot's green into the
// blue of the original artwork.
var trailColors = []lipgloss.AdaptiveColor{
	{Light: "#22A05B", Dark: "#4ADE80"},
	{Light: "#0D9488", Dark: "#2DD4BF"},
	{Light: "#0891B2", Dark: "#22D3EE"},
	{Light: "#2563EB", Dark: "#60A5FA"},
}

// pixels colors a trail left to right so it fades green into blue.
func pixels(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		color := trailColors[i*len(trailColors)/len(runes)]
		b.WriteString(lipgloss.NewStyle().Foreground(color).Render(string(r)))
	}
	return b.String()
}

// logo renders the banner shown on the opening screens: mascot on the left,
// wordmark, version and tagline beside it.
func (m Model) logo() string {
	return lipgloss.JoinHorizontal(lipgloss.Center, m.mascot(), " ", m.brand())
}

// mascot prefers the real artwork and falls back to the hand drawn version on
// terminals without 24-bit color.
func (m Model) mascot() string {
	if supportsTruecolor() {
		return strings.TrimRight(chameleonANSI, "\n")
	}
	var b strings.Builder
	for i, line := range mascotLines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(styleAccent.Render(line.body) + pixels(line.trail))
	}
	return b.String()
}

// brand stacks the wordmark, the version and the tagline. Development builds
// stay unlabeled; only tagged releases show their version.
func (m Model) brand() string {
	var b strings.Builder
	for i, line := range wordmarkLines {
		// The wordmark sweeps the same green-to-blue fade as the tail.
		b.WriteString(pixels(line))
		if i == 0 && m.cfg.Version != "" && m.cfg.Version != "dev" {
			b.WriteString(styleFaint.Render("  " + m.cfg.Version))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + styleFaint.Render("compress videos · capture posters"))
	return b.String()
}
