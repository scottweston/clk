package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLookupFallback(t *testing.T) {
	if Lookup("missing").Name != "tokyo-night" {
		t.Fatal("expected fallback theme")
	}
}

func TestThemesIncludeTenPalettes(t *testing.T) {
	if len(All()) < 10 {
		t.Fatalf("expected at least 10 themes, got %d", len(All()))
	}
}

func TestAccentFallback(t *testing.T) {
	p := Lookup("tokyo-night")
	if Accent(p, "missing") != p.Accent["cyan"] {
		t.Fatal("expected accent fallback")
	}
}

func TestPanelBorderUsesThemeBackground(t *testing.T) {
	p := Lookup("tokyo-night")
	styles := Styles(p, "cyan")

	if got := styles.Panel.GetBorderTopBackground(); got != lipgloss.Color(p.Background) {
		t.Fatalf("expected top border background %q, got %q", p.Background, got)
	}
	if got := styles.Panel.GetBorderRightBackground(); got != lipgloss.Color(p.Background) {
		t.Fatalf("expected right border background %q, got %q", p.Background, got)
	}
	if got := styles.Panel.GetBorderBottomBackground(); got != lipgloss.Color(p.Background) {
		t.Fatalf("expected bottom border background %q, got %q", p.Background, got)
	}
	if got := styles.Panel.GetBorderLeftBackground(); got != lipgloss.Color(p.Background) {
		t.Fatalf("expected left border background %q, got %q", p.Background, got)
	}
}
