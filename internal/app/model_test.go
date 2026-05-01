package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"clk/internal/config"
	"clk/internal/render"
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
	if !strings.Contains(out, "[") {
		t.Fatalf("expected seconds progress in output, got %q", out)
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
	if !strings.Contains(out, "[") {
		t.Fatalf("expected separate seconds progress display, got %q", out)
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
