package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if cfg.Time.Format != "24h" || cfg.Display.DigitStyle != "block" || cfg.Display.FigletFont != "standard" || cfg.Display.ToiletFont != "standard" || cfg.Display.SecondsStyle != "progress_bar" || cfg.Display.Size != "normal" {
		t.Fatalf("invalid values were not normalized: %+v", cfg)
	}
	if cfg.Workday.StartTime != "09:00" || cfg.Workday.EndTime != "17:00" || strings.Join(cfg.Workday.Days, ",") != "mon,tue,wed,thu,fri" {
		t.Fatalf("invalid workday values were not normalized: %+v", cfg.Workday)
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
