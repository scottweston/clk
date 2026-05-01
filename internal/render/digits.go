package render

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type DigitStyle string

const (
	StyleBlock       DigitStyle = "block"
	StyleBraille     DigitStyle = "braille"
	StyleBox         DigitStyle = "box"
	StyleHalfBlock   DigitStyle = "half_block"
	StyleNerdSegment DigitStyle = "nerd_segment"
	StyleFiglet      DigitStyle = "figlet"
	StyleToilet      DigitStyle = "toilet"
)

const HiddenSeparator = '\ue000'

type ClockOptions struct {
	Value      string
	Style      string
	NerdFont   bool
	Scale      int
	FigletFont string
	ToiletFont string
}

func Clock(value string, styleName string, nerdFont bool) string {
	return ClockScaled(value, styleName, nerdFont, 1)
}

func ClockScaled(value string, styleName string, nerdFont bool, factor int) string {
	return ClockStyled(ClockOptions{
		Value:    value,
		Style:    styleName,
		NerdFont: nerdFont,
		Scale:    factor,
	})
}

func ClockStyled(opts ClockOptions) string {
	if opts.Scale < 1 {
		opts.Scale = 1
	}

	style := DigitStyle(opts.Style)
	switch style {
	case StyleFiglet:
		if out, ok := externalClock(opts.Value, "figlet", defaultFont(opts.FigletFont, "standard"), opts.Scale); ok {
			return out
		}
		if out, ok := externalClock(opts.Value, "toilet", defaultFont(opts.FigletFont, "standard"), opts.Scale); ok {
			return out
		}
	case StyleToilet:
		if out, ok := externalClock(opts.Value, "toilet", defaultFont(opts.ToiletFont, "standard"), opts.Scale); ok {
			return out
		}
	}

	return builtInClock(opts.Value, opts.Style, opts.NerdFont, opts.Scale)
}

func builtInClock(value string, styleName string, nerdFont bool, factor int) string {
	style := DigitStyle(styleName)
	var glyphs map[rune][]string
	switch style {
	case StyleBraille:
		glyphs = makeBrailleGlyphs(factor)
	case StyleBox:
		glyphs = boxGlyphs
	case StyleHalfBlock:
		glyphs = halfBlockGlyphs
	case StyleNerdSegment:
		if nerdFont {
			glyphs = nerdGlyphs
		} else {
			glyphs = boxGlyphs
		}
	default:
		glyphs = blockGlyphs
	}

	out := renderGlyphs(value, glyphs)
	if style != StyleBraille && factor > 1 {
		return Scale(out, factor)
	}
	return out
}

func defaultFont(font, fallback string) string {
	if font == "" {
		return fallback
	}
	return font
}

func renderGlyphs(value string, glyphs map[rune][]string) string {
	rows := make([]string, len(glyphs[' ']))
	for _, r := range value {
		glyph, ok := glyphs[r]
		if !ok {
			glyph = glyphs[' ']
		}
		for i := range rows {
			if rows[i] != "" {
				rows[i] += " "
			}
			rows[i] += glyph[i]
		}
	}
	return strings.Join(rows, "\n")
}

func Lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func Scale(s string, factor int) string {
	if factor <= 1 || s == "" {
		return s
	}

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines)*factor)
	for _, line := range lines {
		scaled := scaleLine(line, factor)
		for range factor {
			out = append(out, scaled)
		}
	}
	return strings.Join(out, "\n")
}

func scaleLine(line string, factor int) string {
	var b strings.Builder
	for _, r := range line {
		for range factor {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type externalGlyphKey struct {
	tool string
	font string
	r    rune
}

var externalGlyphCache sync.Map

func externalClock(value, tool, font string, factor int) (string, bool) {
	if _, err := exec.LookPath(tool); err != nil {
		return "", false
	}

	glyphs := make(map[rune][]string)
	for _, r := range value {
		if _, ok := glyphs[r]; ok {
			continue
		}
		glyph, ok := externalGlyph(tool, font, r)
		if !ok {
			return "", false
		}
		glyphs[r] = glyph
	}

	out := renderGlyphs(value, normalizeGlyphs(glyphs))
	if factor > 1 {
		out = Scale(out, factor)
	}
	return out, true
}

func externalGlyph(tool, font string, r rune) ([]string, bool) {
	if r == HiddenSeparator {
		visible, ok := externalGlyph(tool, font, ':')
		if !ok {
			return nil, false
		}
		blank := make([]string, len(visible))
		for i, line := range visible {
			blank[i] = strings.Repeat(" ", len([]rune(line)))
		}
		return blank, true
	}

	key := externalGlyphKey{tool: tool, font: font, r: r}
	if cached, ok := externalGlyphCache.Load(key); ok {
		return cached.([]string), true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	args := []string{"-f", font, string(r)}
	cmd := exec.CommandContext(ctx, tool, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	lines = trimTrailingBlankLines(lines)
	if len(lines) == 0 {
		lines = []string{" "}
	}
	externalGlyphCache.Store(key, lines)
	return lines, true
}

func normalizeGlyphs(glyphs map[rune][]string) map[rune][]string {
	height := 0
	for _, glyph := range glyphs {
		height = max(height, len(glyph))
	}
	if height == 0 {
		height = 1
	}

	normalized := make(map[rune][]string, len(glyphs)+1)
	for r, glyph := range glyphs {
		normalized[r] = padGlyph(glyph, height)
	}
	if _, ok := normalized[' ']; !ok {
		width := 1
		for _, glyph := range normalized {
			if len(glyph) > 0 {
				width = max(width, len([]rune(glyph[0])))
				break
			}
		}
		blank := make([]string, height)
		for i := range blank {
			blank[i] = strings.Repeat(" ", width)
		}
		normalized[' '] = blank
	}
	return normalized
}

func padGlyph(glyph []string, height int) []string {
	width := 0
	for _, line := range glyph {
		width = max(width, len([]rune(line)))
	}
	if width == 0 {
		width = 1
	}

	out := make([]string, height)
	for i := range height {
		if i < len(glyph) {
			out[i] = padRight(glyph[i], width)
		} else {
			out[i] = strings.Repeat(" ", width)
		}
	}
	return out
}

func padRight(s string, width int) string {
	n := len([]rune(s))
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func trimTrailingBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

var blockGlyphs = map[rune][]string{
	'0':             {"██████", "██  ██", "██  ██", "██  ██", "██████"},
	'1':             {"  ██  ", "████  ", "  ██  ", "  ██  ", "██████"},
	'2':             {"██████", "    ██", "██████", "██    ", "██████"},
	'3':             {"██████", "    ██", " █████", "    ██", "██████"},
	'4':             {"██  ██", "██  ██", "██████", "    ██", "    ██"},
	'5':             {"██████", "██    ", "██████", "    ██", "██████"},
	'6':             {"██████", "██    ", "██████", "██  ██", "██████"},
	'7':             {"██████", "    ██", "   ██ ", "  ██  ", "  ██  "},
	'8':             {"██████", "██  ██", "██████", "██  ██", "██████"},
	'9':             {"██████", "██  ██", "██████", "    ██", "██████"},
	':':             {"      ", "  ██  ", "      ", "  ██  ", "      "},
	HiddenSeparator: {"      ", "      ", "      ", "      ", "      "},
	' ':             {"      ", "      ", "      ", "      ", "      "},
	'A':             {"██████", "██  ██", "██████", "██  ██", "██  ██"},
	'P':             {"██████", "██  ██", "██████", "██    ", "██    "},
	'M':             {"██  ██", "██████", "██████", "██  ██", "██  ██"},
}

var brailleGlyphs = makeBrailleGlyphs(1)

var brailleBitmaps = map[rune][]string{
	'0':             {"1111", "1001", "1001", "1001", "1001", "1001", "1111"},
	'1':             {"0010", "0110", "0010", "0010", "0010", "0010", "0111"},
	'2':             {"1111", "0001", "0001", "1111", "1000", "1000", "1111"},
	'3':             {"1111", "0001", "0001", "0111", "0001", "0001", "1111"},
	'4':             {"1001", "1001", "1001", "1111", "0001", "0001", "0001"},
	'5':             {"1111", "1000", "1000", "1111", "0001", "0001", "1111"},
	'6':             {"1111", "1000", "1000", "1111", "1001", "1001", "1111"},
	'7':             {"1111", "0001", "0010", "0010", "0100", "0100", "0100"},
	'8':             {"1111", "1001", "1001", "1111", "1001", "1001", "1111"},
	'9':             {"1111", "1001", "1001", "1111", "0001", "0001", "1111"},
	':':             {"00", "11", "11", "00", "11", "11", "00"},
	HiddenSeparator: {"00", "00", "00", "00", "00", "00", "00"},
	' ':             {"0000", "0000", "0000", "0000", "0000", "0000", "0000"},
	'A':             {"0110", "1001", "1001", "1111", "1001", "1001", "1001"},
	'P':             {"1110", "1001", "1001", "1110", "1000", "1000", "1000"},
	'M':             {"1001", "1111", "1111", "1001", "1001", "1001", "1001"},
}

func makeBrailleGlyphs(factor int) map[rune][]string {
	if factor < 1 {
		factor = 1
	}

	glyphs := make(map[rune][]string, len(brailleBitmaps))
	for r, bitmap := range brailleBitmaps {
		glyphs[r] = brailleFromBitmap(scaleBitmap(bitmap, factor))
	}
	return glyphs
}

func scaleBitmap(bitmap []string, factor int) []string {
	if factor <= 1 {
		return bitmap
	}

	out := make([]string, 0, len(bitmap)*factor)
	for _, row := range bitmap {
		var b strings.Builder
		for _, r := range row {
			for range factor {
				b.WriteRune(r)
			}
		}
		scaled := b.String()
		for range factor {
			out = append(out, scaled)
		}
	}
	return out
}

func brailleFromBitmap(bitmap []string) []string {
	height := len(bitmap)
	width := 0
	for _, row := range bitmap {
		width = max(width, len(row))
	}

	cellRows := (height + 3) / 4
	cellCols := (width + 1) / 2
	out := make([]string, cellRows)
	for cellY := range cellRows {
		var b strings.Builder
		for cellX := range cellCols {
			mask := 0
			for y := range 4 {
				for x := range 2 {
					pixelY := cellY*4 + y
					pixelX := cellX*2 + x
					if pixelY >= height || pixelX >= len(bitmap[pixelY]) || bitmap[pixelY][pixelX] != '1' {
						continue
					}
					mask |= brailleDotMask(x, y)
				}
			}
			if mask == 0 {
				b.WriteRune(' ')
			} else {
				b.WriteRune(rune(0x2800 + mask))
			}
		}
		out[cellY] = b.String()
	}
	return out
}

func brailleDotMask(x, y int) int {
	switch {
	case x == 0 && y == 0:
		return 1 << 0
	case x == 0 && y == 1:
		return 1 << 1
	case x == 0 && y == 2:
		return 1 << 2
	case x == 0 && y == 3:
		return 1 << 6
	case x == 1 && y == 0:
		return 1 << 3
	case x == 1 && y == 1:
		return 1 << 4
	case x == 1 && y == 2:
		return 1 << 5
	case x == 1 && y == 3:
		return 1 << 7
	default:
		return 0
	}
}

var boxGlyphs = map[rune][]string{
	'0':             {"┌──┐", "│  │", "│  │", "│  │", "└──┘"},
	'1':             {" ┐  ", " │  ", " │  ", " │  ", "─┴─ "},
	'2':             {"┌──┐", "   │", "┌──┘", "│   ", "└───"},
	'3':             {"┌──┐", "   │", " ──┤", "   │", "└──┘"},
	'4':             {"│  │", "│  │", "└──┤", "   │", "   │"},
	'5':             {"┌───", "│   ", "└──┐", "   │", "└──┘"},
	'6':             {"┌───", "│   ", "├──┐", "│  │", "└──┘"},
	'7':             {"┌──┐", "   │", "  ┌┘", "  │ ", "  │ "},
	'8':             {"┌──┐", "│  │", "├──┤", "│  │", "└──┘"},
	'9':             {"┌──┐", "│  │", "└──┤", "   │", "└──┘"},
	':':             {"    ", " ██ ", "    ", " ██ ", "    "},
	HiddenSeparator: {"    ", "    ", "    ", "    ", "    "},
	' ':             {"    ", "    ", "    ", "    ", "    "},
	'A':             {"┌──┐", "│  │", "├──┤", "│  │", "│  │"},
	'P':             {"┌──┐", "│  │", "├──┘", "│   ", "│   "},
	'M':             {"│  │", "├┐┌┤", "│└┘│", "│  │", "│  │"},
}

var halfBlockGlyphs = map[rune][]string{
	'0':             {"▄██▄", "█  █", "█  █", "█  █", "▀██▀"},
	'1':             {" ▄█ ", "  █ ", "  █ ", "  █ ", "▀▀▀▀"},
	'2':             {"▄██▄", "   █", "▄██▀", "█   ", "▀▀▀▀"},
	'3':             {"▄██▄", "   █", " ▀█▄", "   █", "▀██▀"},
	'4':             {"█  █", "█  █", "▀▀▀█", "   █", "   █"},
	'5':             {"████", "█   ", "▀██▄", "   █", "▀██▀"},
	'6':             {"▄██▀", "█   ", "███▄", "█  █", "▀██▀"},
	'7':             {"████", "   █", "  █ ", " █  ", " █  "},
	'8':             {"▄██▄", "█  █", "▄██▄", "█  █", "▀██▀"},
	'9':             {"▄██▄", "█  █", "▀███", "   █", "▀██▀"},
	':':             {"    ", " ██ ", "    ", " ██ ", "    "},
	HiddenSeparator: {"    ", "    ", "    ", "    ", "    "},
	' ':             {"    ", "    ", "    ", "    ", "    "},
	'A':             {"▄██▄", "█  █", "████", "█  █", "█  █"},
	'P':             {"███▄", "█  █", "███▀", "█   ", "█   "},
	'M':             {"█  █", "████", "████", "█  █", "█  █"},
}

var nerdGlyphs = map[rune][]string{
	'0':             {"󰎡󰎡󰎡", "󰎡 󰎡", "󰎡 󰎡", "󰎡 󰎡", "󰎡󰎡󰎡"},
	'1':             {" 󰎤 ", " 󰎤 ", " 󰎤 ", " 󰎤 ", " 󰎤 "},
	'2':             {"󰎧󰎧󰎧", "  󰎧", "󰎧󰎧󰎧", "󰎧  ", "󰎧󰎧󰎧"},
	'3':             {"󰎪󰎪󰎪", "  󰎪", "󰎪󰎪󰎪", "  󰎪", "󰎪󰎪󰎪"},
	'4':             {"󰎭 󰎭", "󰎭 󰎭", "󰎭󰎭󰎭", "  󰎭", "  󰎭"},
	'5':             {"󰎱󰎱󰎱", "󰎱  ", "󰎱󰎱󰎱", "  󰎱", "󰎱󰎱󰎱"},
	'6':             {"󰎳󰎳󰎳", "󰎳  ", "󰎳󰎳󰎳", "󰎳 󰎳", "󰎳󰎳󰎳"},
	'7':             {"󰎶󰎶󰎶", "  󰎶", " 󰎶 ", " 󰎶 ", " 󰎶 "},
	'8':             {"󰎹󰎹󰎹", "󰎹 󰎹", "󰎹󰎹󰎹", "󰎹 󰎹", "󰎹󰎹󰎹"},
	'9':             {"󰎼󰎼󰎼", "󰎼 󰎼", "󰎼󰎼󰎼", "  󰎼", "󰎼󰎼󰎼"},
	':':             {"   ", "  ", "   ", "  ", "   "},
	HiddenSeparator: {"   ", "   ", "   ", "   ", "   "},
	' ':             {"   ", "   ", "   ", "   ", "   "},
	'A':             {"󰀄󰀄󰀄", "󰀄 󰀄", "󰀄󰀄󰀄", "󰀄 󰀄", "󰀄 󰀄"},
	'P':             {"󰀀󰀀󰀀", "󰀀 󰀀", "󰀀󰀀󰀀", "󰀀  ", "󰀀  "},
	'M':             {"󰀄 󰀄", "󰀄󰀄󰀄", "󰀄 󰀄", "󰀄 󰀄", "󰀄 󰀄"},
}
