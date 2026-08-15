//go:build ignore

// Command gen renders chameleon.ansi from a hand drawn pixel grid.
//
// The mascot is far too small for an automatic downscale of the artwork to
// read well, so the icon is pixel art designed at this exact size. Each rune
// in the grid below is one pixel; a character cell holds two pixels stacked
// vertically via the half block. Run it with:
//
//	go run gen.go > chameleon.ansi
package main

import (
	"fmt"
	"strings"
)

// palette maps grid runes to sRGB colors, following the brand gradient from
// the green head to the blue tail.
var palette = map[rune][3]int{
	'L': {190, 242, 100}, // highlight green, snout and belly
	'g': {74, 222, 128},  // body green
	'G': {22, 163, 74},   // deep green, legs
	't': {45, 212, 191},  // teal, mid body
	'c': {34, 211, 238},  // cyan, rear body
	'b': {59, 130, 246},  // blue, tail
	'B': {37, 99, 235},   // deep blue, tail core
	'k': {5, 46, 22},     // near black, eye ring
	'w': {240, 253, 244}, // white, eye glint
}

// grid is the mascot, one rune per pixel, dot for transparent. 20 wide by 12
// tall renders as 20 columns by 6 rows. Identity markers, left to right: the
// casque, the ringed eye with its glint, the green-to-blue body fade and the
// tail dissolving into loose pixels.
var grid = []string{
	`.......gg......b....`,
	`.....ggggg..c.......`,
	`...gggggggg.tt...c..`,
	`..ggkkkggggttc..b...`,
	`.ggkwwkgggtttcc.....`,
	`.ggkkkkggttcccc..c..`,
	`.Lggggggttcccbb.b...`,
	`..Lggggg.ccbbbB.....`,
	`..GG..GG...bBBb.....`,
	`....................`,
}

func fg(c [3]int) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c[0], c[1], c[2]) }
func bg(c [3]int) string { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c[0], c[1], c[2]) }

func main() {
	var b strings.Builder
	for row := 0; row+1 < len(grid); row += 2 {
		top, bottom := []rune(grid[row]), []rune(grid[row+1])
		for col := range top {
			upper, hasUpper := palette[top[col]]
			lower, hasLower := palette[bottom[col]]
			switch {
			case hasUpper && hasLower:
				b.WriteString(fg(upper) + bg(lower) + "▀\x1b[0m")
			case hasUpper:
				b.WriteString(fg(upper) + "▀\x1b[0m")
			case hasLower:
				b.WriteString(fg(lower) + "▄\x1b[0m")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}
	fmt.Print(b.String())
}
