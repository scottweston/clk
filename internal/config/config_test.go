package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Time.Format != "24h" {
		t.Fatalf("expected default 24h format, got %q", cfg.Time.Format)
	}
	if cfg.Display.SecondsStyle != "progress_bar" {
		t.Fatalf("expected progress bar seconds, got %q", cfg.Display.SecondsStyle)
	}
	if cfg.Display.ProgressEmptyBackground != "theme" {
		t.Fatalf("expected theme progress empty background, got %q", cfg.Display.ProgressEmptyBackground)
	}
	if cfg.Workday.Schedule["mon"].StartTime != "09:00" || cfg.Workday.Schedule["mon"].EndTime != "17:00" || !cfg.Workday.Schedule["mon"].Enabled || cfg.Workday.Schedule["sat"].Enabled {
		t.Fatalf("expected default weekday 09:00-17:00 schedule, got %+v", cfg.Workday)
	}
	if cfg.Workday.ShowProgress {
		t.Fatal("expected default workday progress bar to be off")
	}
	if cfg.Calendar.ShowProgress || cfg.Calendar.URL != "" || len(cfg.Calendar.Sources) != 0 || cfg.Calendar.Mode != "merged" || cfg.Calendar.RefreshMinutes != 15 {
		t.Fatalf("unexpected default calendar config: %+v", cfg.Calendar)
	}
	if cfg.UI.Emoji {
		t.Fatal("expected emoji rendering to default to off")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clk", "config.yaml")
	manager := NewManager(path, false)
	cfg := Default()
	cfg.Display.DigitStyle = "braille"
	cfg.Calendar.ShowProgress = true
	cfg.Calendar.Mode = "split"
	cfg.Calendar.Sources = []CalendarSourceConfig{
		{
			URL: "https://example.com/work.ics",
			LastEvent: CalendarEventConfig{
				SourceURL: "https://example.com/work.ics",
				Summary:   "Standup",
				Start:     time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
				End:       time.Date(2026, 5, 1, 9, 30, 0, 0, time.UTC),
			},
		},
		{URL: "https://example.com/personal.ics"},
	}
	cfg.UI.NerdFont = true
	cfg.UI.Emoji = true

	if err := manager.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Display.DigitStyle != "braille" || !loaded.UI.NerdFont || !loaded.UI.Emoji || loaded.Calendar.Mode != "split" {
		t.Fatalf("loaded config mismatch: %+v", loaded)
	}
	if len(loaded.Calendar.Sources) != 2 {
		t.Fatalf("expected loaded calendar sources, got %+v", loaded.Calendar.Sources)
	}
	if loaded.Calendar.Sources[0].LastEvent.Summary != "Standup" || !loaded.Calendar.Sources[0].LastEvent.End.Equal(cfg.Calendar.Sources[0].LastEvent.End) {
		t.Fatalf("loaded calendar source last event mismatch: %+v", loaded.Calendar.Sources[0].LastEvent)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if strings.Contains(string(data), "start_time: \"09:00\"\n  end_time: \"17:00\"\n  days:") {
		t.Fatalf("expected saved config to use schedule shape, got:\n%s", data)
	}
}

func TestSaveDefaultConfigOmitsEmptyCalendarLastEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	manager := NewManager(path, false)

	if err := manager.Save(Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if strings.Contains(string(data), "last_event") {
		t.Fatalf("expected empty calendar last event to be omitted, got:\n%s", data)
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
	resetFclkFontCache()
	t.Cleanup(resetFclkFontCache)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := Config{
		Time:    TimeConfig{Format: "bad"},
		Display: DisplayConfig{DigitStyle: "bad", FigletFont: "bad", ToiletFont: "bad", FclkFont: "bad", SecondsStyle: "bad", ProgressEmptyBackground: "bad", Alignment: "bad", Size: "bad"},
		Workday: WorkdayConfig{Schedule: map[string]WorkdayDayConfig{
			"mon": {Enabled: true, StartTime: "bad", EndTime: "bad"},
			"wat": {Enabled: true, StartTime: "12:00", EndTime: "13:00"},
		}},
		Calendar: CalendarConfig{
			URL:            " https://example.com/cal.ics ",
			RefreshMinutes: -1,
			LastEvent: CalendarEventConfig{
				SourceURL: "https://example.com/other.ics",
				Summary:   "Old",
				Start:     time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
				End:       time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
			},
		},
		Theme: ThemeConfig{Accent: "bad"},
	}
	cfg.Normalize()
	if cfg.Time.Format != "24h" || cfg.Display.DigitStyle != "block" || cfg.Display.FigletFont != "standard" || cfg.Display.ToiletFont != "standard" || cfg.Display.FclkFont != "" || cfg.Display.SecondsStyle != "progress_bar" || cfg.Display.ProgressEmptyBackground != "theme" || cfg.Display.Size != "" {
		t.Fatalf("invalid values were not normalized: %+v", cfg)
	}
	if cfg.Workday.Schedule["mon"].StartTime != "09:00" || cfg.Workday.Schedule["mon"].EndTime != "17:00" || !cfg.Workday.Schedule["mon"].Enabled || cfg.Workday.Schedule["sat"].Enabled {
		t.Fatalf("invalid workday values were not normalized: %+v", cfg.Workday)
	}
	if cfg.Calendar.URL != "" || len(cfg.Calendar.Sources) != 1 || cfg.Calendar.Sources[0].URL != "https://example.com/cal.ics" || cfg.Calendar.Mode != "merged" || cfg.Calendar.RefreshMinutes != 15 {
		t.Fatalf("calendar values were not normalized: %+v", cfg.Calendar)
	}
	if !cfg.Calendar.Sources[0].LastEvent.Start.IsZero() {
		t.Fatalf("expected stale calendar last event to be cleared, got %+v", cfg.Calendar.Sources[0].LastEvent)
	}
}

func TestLoadMigratesLegacyCalendarURLToSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`version: 1
time:
    format: 24h
    show_date: true
    timezone: Local
display:
    digit_style: block
    figlet_font: standard
    toilet_font: standard
    fclk_font: ""
    seconds_style: progress_bar
    inline_seconds: false
    progress_empty_background: theme
    alignment: center
    blink_separator: false
workday:
    show_progress: false
    schedule:
        mon: {enabled: true, start_time: "09:00", end_time: "17:00"}
        tue: {enabled: true, start_time: "09:00", end_time: "17:00"}
        wed: {enabled: true, start_time: "09:00", end_time: "17:00"}
        thu: {enabled: true, start_time: "09:00", end_time: "17:00"}
        fri: {enabled: true, start_time: "09:00", end_time: "17:00"}
        sat: {enabled: false, start_time: "09:00", end_time: "17:00"}
        sun: {enabled: false, start_time: "09:00", end_time: "17:00"}
calendar:
    show_progress: true
    url: https://example.com/work.ics
    refresh_minutes: 15
    last_event:
        source_url: https://example.com/work.ics
        summary: Standup
        start: 2026-05-01T09:00:00Z
        end: 2026-05-01T09:30:00Z
theme:
    name: tokyo-night
    accent: cyan
ui:
    nerd_font: false
    emoji: false
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := NewManager(path, false).Load()
	if err != nil {
		t.Fatalf("load legacy calendar config: %v", err)
	}
	if cfg.Calendar.URL != "" || len(cfg.Calendar.Sources) != 1 || cfg.Calendar.Sources[0].URL != "https://example.com/work.ics" {
		t.Fatalf("expected legacy URL to migrate to source, got %+v", cfg.Calendar)
	}
	if cfg.Calendar.Sources[0].LastEvent.Summary != "Standup" {
		t.Fatalf("expected legacy last event to migrate to source, got %+v", cfg.Calendar.Sources[0].LastEvent)
	}
}

func TestNormalizeCalendarSourceLastEventAllowsOmittedSourceURL(t *testing.T) {
	cfg := Default()
	cfg.Calendar.Sources = []CalendarSourceConfig{
		{
			URL: "https://example.com/work.ics",
			LastEvent: CalendarEventConfig{
				Summary: "Standup",
				Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 5, 1, 9, 30, 0, 0, time.UTC),
			},
		},
	}

	cfg.Normalize()
	if cfg.Calendar.Sources[0].LastEvent.SourceURL != "https://example.com/work.ics" {
		t.Fatalf("expected source URL to be filled from source, got %+v", cfg.Calendar.Sources[0].LastEvent)
	}
}

func TestNormalizeMigratesLegacyWorkdaySecondsStyle(t *testing.T) {
	cfg := Default()
	cfg.Display.SecondsStyle = "workday"
	cfg.Normalize()

	if cfg.Display.SecondsStyle != "hidden" {
		t.Fatalf("expected workday seconds style to migrate to hidden, got %q", cfg.Display.SecondsStyle)
	}
	if !cfg.Workday.ShowProgress {
		t.Fatal("expected workday progress bar to be enabled")
	}
}

func TestLoadMigratesLegacyWorkdayConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`version: 1
time:
    format: 24h
    show_date: true
    timezone: Local
display:
    digit_style: block
    figlet_font: standard
    toilet_font: standard
    fclk_font: ""
    seconds_style: progress_bar
    inline_seconds: false
    progress_empty_background: theme
    alignment: center
    blink_separator: false
workday:
    start_time: "10:00"
    end_time: "16:30"
    days:
        - tue
        - thu
theme:
    name: tokyo-night
    accent: cyan
ui:
    nerd_font: false
    emoji: false
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := NewManager(path, false).Load()
	if err != nil {
		t.Fatalf("load legacy config: %v", err)
	}
	if !cfg.Workday.Schedule["tue"].Enabled || !cfg.Workday.Schedule["thu"].Enabled || cfg.Workday.Schedule["mon"].Enabled {
		t.Fatalf("expected legacy days to migrate, got %+v", cfg.Workday.Schedule)
	}
	if cfg.Workday.Schedule["tue"].StartTime != "10:00" || cfg.Workday.Schedule["tue"].EndTime != "16:30" {
		t.Fatalf("expected legacy times to migrate, got %+v", cfg.Workday.Schedule["tue"])
	}
}

func TestNormalizeMigratesDoubleSizeToDigitStyle(t *testing.T) {
	cases := map[string]string{
		"block":      "block_2x",
		"braille":    "braille_2x",
		"box":        "box_2x",
		"half_block": "half_block_2x",
		"fclk":       "fclk",
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

func TestFclkFontChoicesIncludeConfigFonts(t *testing.T) {
	resetFclkFontCache()
	t.Cleanup(resetFclkFontCache)

	xdgConfig := t.TempDir()
	fontPath := filepath.Join(xdgConfig, "clk", "fonts", "custom-clock.fclk")
	if err := os.MkdirAll(filepath.Dir(fontPath), 0o700); err != nil {
		t.Fatalf("create font dir: %v", err)
	}
	if err := os.WriteFile(fontPath, []byte("0\nX\n"), 0o600); err != nil {
		t.Fatalf("write font: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	want := strings.TrimSuffix(fontPath, filepath.Ext(fontPath))
	if !contains(FclkFontChoices(), want) {
		t.Fatalf("expected discovered fclk font %q in choices", want)
	}
}

func TestFclkFontChoicesIgnoreConfigRootFonts(t *testing.T) {
	resetFclkFontCache()
	t.Cleanup(resetFclkFontCache)

	xdgConfig := t.TempDir()
	fontPath := filepath.Join(xdgConfig, "clk", "root-clock.fclk")
	if err := os.MkdirAll(filepath.Dir(fontPath), 0o700); err != nil {
		t.Fatalf("create font dir: %v", err)
	}
	if err := os.WriteFile(fontPath, []byte("0\nX\n"), 0o600); err != nil {
		t.Fatalf("write font: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	unwanted := strings.TrimSuffix(fontPath, filepath.Ext(fontPath))
	if contains(FclkFontChoices(), unwanted) {
		t.Fatalf("expected config root fclk font %q to be ignored", unwanted)
	}
}

func TestFclkFontChoicesIncludeCurrentDirectoryFonts(t *testing.T) {
	resetFclkFontCache()
	t.Cleanup(resetFclkFontCache)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	dir := t.TempDir()
	fontPath := filepath.Join(dir, "fonts", "local-clock.fclk")
	if err := os.MkdirAll(filepath.Dir(fontPath), 0o700); err != nil {
		t.Fatalf("create font dir: %v", err)
	}
	if err := os.WriteFile(fontPath, []byte("0\nX\n"), 0o600); err != nil {
		t.Fatalf("write font: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	want := strings.TrimSuffix(fontPath, filepath.Ext(fontPath))
	if !contains(FclkFontChoices(), want) {
		t.Fatalf("expected current directory fclk font %q in choices", want)
	}
}

func TestFclkFontChoicesIgnoreCurrentDirectoryRootFonts(t *testing.T) {
	resetFclkFontCache()
	t.Cleanup(resetFclkFontCache)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	dir := t.TempDir()
	fontPath := filepath.Join(dir, "local-clock.fclk")
	if err := os.WriteFile(fontPath, []byte("0\nX\n"), 0o600); err != nil {
		t.Fatalf("write font: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	unwanted := strings.TrimSuffix(fontPath, filepath.Ext(fontPath))
	if contains(FclkFontChoices(), unwanted) {
		t.Fatalf("expected current directory root fclk font %q to be ignored", unwanted)
	}
}

func TestNormalizeAcceptsDiscoveredFclkFont(t *testing.T) {
	resetFclkFontCache()
	t.Cleanup(resetFclkFontCache)

	xdgConfig := t.TempDir()
	fontPath := filepath.Join(xdgConfig, "clk", "fonts", "custom-clock.fclk")
	if err := os.MkdirAll(filepath.Dir(fontPath), 0o700); err != nil {
		t.Fatalf("create font dir: %v", err)
	}
	if err := os.WriteFile(fontPath, []byte("0\nX\n"), 0o600); err != nil {
		t.Fatalf("write font: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	cfg := Default()
	cfg.Display.FclkFont = strings.TrimSuffix(fontPath, filepath.Ext(fontPath))
	cfg.Normalize()
	if cfg.Display.FclkFont != strings.TrimSuffix(fontPath, filepath.Ext(fontPath)) {
		t.Fatalf("expected discovered fclk font to be preserved, got %q", cfg.Display.FclkFont)
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

func resetFclkFontCache() {
	fclkFontsOnce = sync.Once{}
	fclkFonts = nil
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
