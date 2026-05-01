package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"clk/internal/config"
	"clk/internal/render"
	"clk/internal/theme"
)

func TestModelRendersClock(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.now = time.Date(2026, 5, 1, 13, 14, 15, 0, time.Local)

	out := m.View()
	if !strings.Contains(out, "Friday") {
		t.Fatalf("expected date in output, got %q", out)
	}
	if !strings.Contains(out, "25%") {
		t.Fatalf("expected seconds progress to use current time, got %q", out)
	}
}

func TestSettingsChangeAutosavesNoConfig(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	m.cursor = 0
	before := m.cfg.Theme.Name
	m.changeSetting(1)
	if m.cfg.Theme.Name == before {
		t.Fatal("expected setting to change")
	}
	if m.saveError != "" {
		t.Fatalf("unexpected save error: %s", m.saveError)
	}
}

func TestWorkdaySettingsChangeNoConfig(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	items := m.settingItems()
	for i, item := range items {
		if item.label == "Work start" {
			m.cursor = i
			break
		}
	}

	m.changeSetting(1)
	if m.cfg.Workday.StartTime == "09:00" {
		t.Fatal("expected work start setting to change")
	}
}

func TestSettingsDoNotExposeClockSize(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	for _, item := range m.settingItems() {
		if item.label == "Clock size" {
			t.Fatal("expected clock size setting to be removed")
		}
	}
}

func TestSettingsOnlyExposeRelevantExternalFontSelector(t *testing.T) {
	cases := []struct {
		style      string
		wantFiglet bool
		wantToilet bool
	}{
		{style: "block"},
		{style: "figlet", wantFiglet: true},
		{style: "toilet", wantToilet: true},
	}
	for _, tc := range cases {
		cfg := config.Default()
		cfg.Display.DigitStyle = tc.style
		m := New(cfg, config.NewManager("", true))

		gotFiglet := hasSettingItem(m, "Figlet font")
		gotToilet := hasSettingItem(m, "Toilet font")
		if gotFiglet != tc.wantFiglet || gotToilet != tc.wantToilet {
			t.Fatalf("%s settings figlet=%v toilet=%v, want figlet=%v toilet=%v", tc.style, gotFiglet, gotToilet, tc.wantFiglet, tc.wantToilet)
		}
	}
}

func TestWorkdaySettingsOpenCheckboxSubmenu(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	m.height = 30
	items := m.settingItems()
	for i, item := range items {
		if item.label == "Work days" {
			m.cursor = i
			break
		}
	}

	m.changeSetting(1)
	if !m.workdays {
		t.Fatal("expected workdays submenu to open")
	}

	out := m.settingsView(testStyles())
	if !strings.Contains(out, "[x] Monday") || !strings.Contains(out, "[ ] Saturday") {
		t.Fatalf("expected checkbox workday rows, got %q", out)
	}
}

func hasSettingItem(m Model, label string) bool {
	for _, item := range m.settingItems() {
		if item.label == label {
			return true
		}
	}
	return false
}

func TestWorkdayCheckboxToggleNoConfig(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	m.workdays = true
	m.dayCursor = 5

	m.toggleWorkday("sat")
	if !containsString(m.cfg.Workday.Days, "sat") {
		t.Fatalf("expected saturday enabled, got %+v", m.cfg.Workday.Days)
	}
	if m.saveError != "" {
		t.Fatalf("unexpected save error: %s", m.saveError)
	}
}

func TestBackClosesWorkdaySubmenu(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	m.workdays = true

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := next.(Model)
	if updated.workdays {
		t.Fatal("expected workdays submenu to close")
	}
	if !updated.settings {
		t.Fatal("expected settings to remain open")
	}
}

func TestKeyOpensSettings(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	updated := next.(Model)
	if !updated.settings {
		t.Fatal("expected settings to open")
	}
}

func TestInlineSecondsAreIncludedInClockText(t *testing.T) {
	cfg := config.Default()
	cfg.Display.InlineSeconds = true
	m := New(cfg, config.NewManager("", true))
	now := time.Date(2026, 5, 1, 13, 14, 15, 0, time.Local)

	if got := m.clockText(now); got != "13:14:15" {
		t.Fatalf("expected inline seconds clock text, got %q", got)
	}
}

func TestInlineSecondsAreIncludedInTwelveHourClockText(t *testing.T) {
	cfg := config.Default()
	cfg.Time.Format = "12h"
	cfg.Display.InlineSeconds = true
	m := New(cfg, config.NewManager("", true))
	now := time.Date(2026, 5, 1, 13, 14, 15, 0, time.Local)

	if got := m.clockText(now); got != "01:14:15 PM" {
		t.Fatalf("expected inline seconds 12h clock text, got %q", got)
	}
}

func TestBlinkSeparatorHidesSeparatorsOnOddSeconds(t *testing.T) {
	cfg := config.Default()
	cfg.Display.BlinkSeparator = true
	m := New(cfg, config.NewManager("", true))

	odd := time.Date(2026, 5, 1, 13, 14, 15, 0, time.Local)
	if got := m.clockText(odd); got != "13"+string(render.HiddenSeparator)+"14" {
		t.Fatalf("expected hidden separator on odd second, got %q", got)
	}

	even := time.Date(2026, 5, 1, 13, 14, 16, 0, time.Local)
	if got := m.clockText(even); got != "13:14" {
		t.Fatalf("expected visible separator on even second, got %q", got)
	}
}

func TestBlinkSeparatorWorksWithInlineSeconds(t *testing.T) {
	cfg := config.Default()
	cfg.Display.BlinkSeparator = true
	cfg.Display.InlineSeconds = true
	m := New(cfg, config.NewManager("", true))

	now := time.Date(2026, 5, 1, 13, 14, 15, 0, time.Local)
	want := "13" + string(render.HiddenSeparator) + "14" + string(render.HiddenSeparator) + "15"
	if got := m.clockText(now); got != want {
		t.Fatalf("expected hidden inline separators, got %q", got)
	}
}

func TestInlineSecondsCanCombineWithProgressDisplay(t *testing.T) {
	cfg := config.Default()
	cfg.Display.InlineSeconds = true
	cfg.Display.SecondsStyle = "progress_bar"
	m := New(cfg, config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.now = time.Date(2026, 5, 1, 13, 14, 15, 0, time.Local)

	out := m.View()
	if !strings.Contains(out, "25%") {
		t.Fatalf("expected separate seconds progress display to use current time, got %q", out)
	}
}

func TestBubbleProgressStylesUseCurrentTime(t *testing.T) {
	cases := []struct {
		style string
		now   time.Time
		want  string
	}{
		{"bubble_progress", time.Date(2026, 5, 1, 13, 14, 15, 0, time.Local), "25%"},
		{"pomodoro", time.Date(2026, 5, 1, 12, 10, 0, 0, time.Local), "40%"},
		{"workday", time.Date(2026, 5, 1, 13, 0, 0, 0, time.Local), "50%"},
	}

	for _, tc := range cases {
		cfg := config.Default()
		cfg.Display.SecondsStyle = tc.style
		m := New(cfg, config.NewManager("", true))
		m.width = 100
		m.height = 30
		m.now = tc.now

		if out := m.View(); !strings.Contains(out, tc.want) {
			t.Fatalf("expected %s to render %s from current time, got %q", tc.style, tc.want, out)
		}
	}
}

func TestProgressWidthUsesClockWidthAsMinimum(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.width = 20
	clockArt := render.Clock("13:14", "block", false)

	if got := m.progressWidth(clockArt); got != lipgloss.Width(clockArt) {
		t.Fatalf("expected progress width to match clock width, got %d want %d", got, lipgloss.Width(clockArt))
	}
}

func TestProgressWidthExpandsWithWindow(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.width = 160
	clockArt := render.Clock("13:14", "block", false)

	if got := m.progressWidth(clockArt); got <= lipgloss.Width(clockArt) {
		t.Fatalf("expected progress width to expand beyond clock width, got %d clock %d", got, lipgloss.Width(clockArt))
	}
}

func TestJoinVerticalWithBackgroundPadsToEqualWidths(t *testing.T) {
	out := joinVerticalWithBackground(lipgloss.Center, "#000000", "wide line", "short")
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for _, line := range lines {
		if lipgloss.Width(line) != 9 {
			t.Fatalf("expected visible width 9, got %d in %q", lipgloss.Width(line), line)
		}
	}
}

func TestSettingsViewPadsToEqualWidths(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.width = 100
	m.height = 40
	m.now = time.Date(2026, 5, 1, 13, 14, 15, 0, time.Local)
	m.settings = true

	out := m.View()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	expected := -1
	for _, line := range lines {
		width := lipgloss.Width(line)
		if expected == -1 {
			expected = width
			continue
		}
		if width != expected {
			t.Fatalf("expected all settings screen lines to have width %d, got %d in %q", expected, width, line)
		}
	}
}

func testStyles() theme.Stylesheet {
	return theme.Styles(theme.Lookup("tokyo-night"), "cyan")
}
