package render

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type fclkFont struct {
	glyphs map[rune][]string
}

var fclkFontCache sync.Map

func fclkClock(value, font string) (string, bool) {
	font = fclkFontPath(font)
	if font == "" {
		return "", false
	}
	loaded, ok := loadFclkFont(font)
	if !ok {
		return "", false
	}
	return renderGlyphs(value, loaded.glyphs), true
}

func loadFclkFont(path string) (fclkFont, bool) {
	if cached, ok := fclkFontCache.Load(path); ok {
		return cached.(fclkFont), true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fclkFont{}, false
	}
	font, ok := parseFclkFont(string(data))
	if !ok {
		return fclkFont{}, false
	}
	fclkFontCache.Store(path, font)
	return font, true
}

func parseFclkFont(data string) (fclkFont, bool) {
	data = strings.TrimRight(data, "\r\n")
	lines := strings.Split(data, "\n")
	if len(lines) < 2 {
		return fclkFont{}, false
	}

	header := []rune(strings.TrimRight(lines[0], "\r"))
	if len(header) == 0 {
		return fclkFont{}, false
	}

	body := make([][]rune, 0, len(lines)-1)
	for _, line := range lines[1:] {
		row := []rune(strings.TrimRight(line, "\r"))
		if len(row) != len(header) {
			return fclkFont{}, false
		}
		body = append(body, row)
	}

	columns := make(map[rune][]int)
	order := make([]rune, 0)
	for i, r := range header {
		if r == ' ' || r == '\t' {
			continue
		}
		if _, ok := columns[r]; !ok {
			order = append(order, r)
		}
		columns[r] = append(columns[r], i)
	}
	if len(columns) == 0 {
		return fclkFont{}, false
	}

	glyphs := make(map[rune][]string, len(columns)+2)
	for _, r := range order {
		rows := make([]string, len(body))
		for rowIndex, row := range body {
			var b strings.Builder
			for _, col := range columns[r] {
				b.WriteRune(row[col])
			}
			rows[rowIndex] = b.String()
		}
		glyphs[r] = rows
	}

	width := 1
	if glyph, ok := glyphs['0']; ok && len(glyph) > 0 {
		width = len([]rune(glyph[0]))
	}
	glyphs[' '] = blankFclkGlyph(len(body), width)
	if glyph, ok := glyphs[':']; ok {
		glyphs[HiddenSeparator] = blankFclkGlyph(len(body), len([]rune(glyph[0])))
	}
	return fclkFont{glyphs: normalizeGlyphs(glyphs)}, true
}

func blankFclkGlyph(rows, width int) []string {
	blank := strings.Repeat(" ", width)
	glyph := make([]string, rows)
	for i := range glyph {
		glyph[i] = blank
	}
	return glyph
}

func fclkFontPath(font string) string {
	if font == "" {
		return ""
	}
	if filepath.Ext(font) == ".fclk" {
		return font
	}
	return font + ".fclk"
}
