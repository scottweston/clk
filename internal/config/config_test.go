package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Time.Format != "24h" {
		t.Fatalf("expected default 24h format, got %q", cfg.Time.Format)
	}
	if cfg.Display.SecondsStyle != "progress_bar" {
		t.Fatalf("expected progress bar seconds, got %q", cfg.Display.SecondsStyle)
	}
	if cfg.Workday.StartTime != "09:00" || cfg.Workday.EndTime != "17:00" {
		t.Fatalf("expected default workday 09:00-17:00, got %+v", cfg.Workday)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clk", "config.yaml")
	manager := NewManager(path, false)
	cfg := Default()
	cfg.Display.DigitStyle = "braille"
	cfg.UI.NerdFont = true

	if err := manager.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Display.DigitStyle != "braille" || !loaded.UI.NerdFont {
		t.Fatalf("loaded config mismatch: %+v", loaded)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nwat: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := NewManager(path, false).Load()
	if err == nil || !strings.Contains(err.Error(), "field wat not found") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestNormalizeInvalidValues(t *testing.T) {
	cfg := Config{
		Time:    TimeConfig{Format: "bad"},
		Display: DisplayConfig{DigitStyle: "bad", FigletFont: "bad", ToiletFont: "bad", SecondsStyle: "bad", Alignment: "bad", Size: "bad"},
		Workday: WorkdayConfig{StartTime: "bad", EndTime: "bad", Days: []string{"wat"}},
		Theme:   ThemeConfig{Accent: "bad"},
	}
	cfg.Normalize()
	if cfg.Time.Format != "24h" || cfg.Display.DigitStyle != "block" || cfg.Display.FigletFont != "standard" || cfg.Display.ToiletFont != "standard" || cfg.Display.SecondsStyle != "progress_bar" || cfg.Display.Size != "" {
		t.Fatalf("invalid values were not normalized: %+v", cfg)
	}
	if cfg.Workday.StartTime != "09:00" || cfg.Workday.EndTime != "17:00" || strings.Join(cfg.Workday.Days, ",") != "mon,tue,wed,thu,fri" {
		t.Fatalf("invalid workday values were not normalized: %+v", cfg.Workday)
	}
}

func TestNormalizeMigratesDoubleSizeToDigitStyle(t *testing.T) {
	cases := map[string]string{
		"block":      "block_2x",
		"braille":    "braille_2x",
		"box":        "box_2x",
		"half_block": "half_block_2x",
		"figlet":     "figlet",
		"toilet":     "toilet",
	}
	for style, want := range cases {
		cfg := Default()
		cfg.Display.DigitStyle = style
		cfg.Display.Size = "double"
		cfg.Normalize()

		if cfg.Display.DigitStyle != want {
			t.Fatalf("style %q migrated to %q, want %q", style, cfg.Display.DigitStyle, want)
		}
		if cfg.Display.Size != "" {
			t.Fatalf("expected legacy size to be cleared, got %q", cfg.Display.Size)
		}
	}
}

func TestFigletFontChoicesIncludeUserFonts(t *testing.T) {
	resetFigletFontCache()
	t.Cleanup(resetFigletFontCache)

	xdgData := t.TempDir()
	fontPath := filepath.Join(xdgData, "figlet", "custom-clock.flf")
	if err := os.MkdirAll(filepath.Dir(fontPath), 0o700); err != nil {
		t.Fatalf("create font dir: %v", err)
	}
	if err := os.WriteFile(fontPath, []byte("flf2a$ 1 1 1 0 1\n"), 0o600); err != nil {
		t.Fatalf("write font: %v", err)
	}
	t.Setenv("XDG_DATA_HOME", xdgData)

	want := strings.TrimSuffix(fontPath, filepath.Ext(fontPath))
	if !contains(FigletFontChoices(), want) {
		t.Fatalf("expected discovered user font %q in choices", want)
	}
}

func TestNormalizeAcceptsDiscoveredUserFigletFont(t *testing.T) {
	resetFigletFontCache()
	t.Cleanup(resetFigletFontCache)

	xdgData := t.TempDir()
	fontPath := filepath.Join(xdgData, "figlet", "custom-clock.flf")
	if err := os.MkdirAll(filepath.Dir(fontPath), 0o700); err != nil {
		t.Fatalf("create font dir: %v", err)
	}
	if err := os.WriteFile(fontPath, []byte("flf2a$ 1 1 1 0 1\n"), 0o600); err != nil {
		t.Fatalf("write font: %v", err)
	}
	t.Setenv("XDG_DATA_HOME", xdgData)

	cfg := Default()
	cfg.Display.FigletFont = strings.TrimSuffix(fontPath, filepath.Ext(fontPath))
	cfg.Normalize()
	if cfg.Display.FigletFont != strings.TrimSuffix(fontPath, filepath.Ext(fontPath)) {
		t.Fatalf("expected discovered font to be preserved, got %q", cfg.Display.FigletFont)
	}
}

func TestNormalizeMigratesInlineSecondsStyle(t *testing.T) {
	cfg := Default()
	cfg.Display.SecondsStyle = "inline"
	cfg.Normalize()

	if !cfg.Display.InlineSeconds {
		t.Fatal("expected inline seconds to be enabled")
	}
	if cfg.Display.SecondsStyle != "hidden" {
		t.Fatalf("expected migrated seconds style hidden, got %q", cfg.Display.SecondsStyle)
	}
}

func resetFigletFontCache() {
	figletFontsOnce = sync.Once{}
	figletFonts = nil
}

func TestNoConfigSkipsDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.yaml")
	manager := NewManager(path, true)
	cfg, err := manager.Load()
	if err != nil {
		t.Fatalf("load no config: %v", err)
	}
	cfg.Display.DigitStyle = "box"
	if err := manager.Save(cfg); err != nil {
		t.Fatalf("save no config: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no file to be written, stat err %v", err)
	}
}
