package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	Version = 1
	AppDir  = "clk"
)

type Config struct {
	Version int           `yaml:"version"`
	Time    TimeConfig    `yaml:"time"`
	Display DisplayConfig `yaml:"display"`
	Workday WorkdayConfig `yaml:"workday"`
	Theme   ThemeConfig   `yaml:"theme"`
	UI      UIConfig      `yaml:"ui"`
}

type TimeConfig struct {
	Format   string `yaml:"format"`
	ShowDate bool   `yaml:"show_date"`
	Timezone string `yaml:"timezone"`
}

type DisplayConfig struct {
	DigitStyle     string `yaml:"digit_style"`
	FigletFont     string `yaml:"figlet_font"`
	ToiletFont     string `yaml:"toilet_font"`
	SecondsStyle   string `yaml:"seconds_style"`
	InlineSeconds  bool   `yaml:"inline_seconds"`
	Alignment      string `yaml:"alignment"`
	Size           string `yaml:"size,omitempty"`
	BlinkSeparator bool   `yaml:"blink_separator"`
}

type WorkdayConfig struct {
	StartTime string   `yaml:"start_time"`
	EndTime   string   `yaml:"end_time"`
	Days      []string `yaml:"days"`
}

type ThemeConfig struct {
	Name   string `yaml:"name"`
	Accent string `yaml:"accent"`
}

type UIConfig struct {
	NerdFont bool `yaml:"nerd_font"`
}

type Manager struct {
	path     string
	noConfig bool
}

func NewManager(path string, noConfig bool) Manager {
	return Manager{path: path, noConfig: noConfig}
}

func Default() Config {
	return Config{
		Version: Version,
		Time: TimeConfig{
			Format:   "24h",
			ShowDate: true,
			Timezone: "Local",
		},
		Display: DisplayConfig{
			DigitStyle:     "block",
			FigletFont:     "standard",
			ToiletFont:     "standard",
			SecondsStyle:   "progress_bar",
			InlineSeconds:  false,
			Alignment:      "center",
			BlinkSeparator: false,
		},
		Workday: WorkdayConfig{
			StartTime: "09:00",
			EndTime:   "17:00",
			Days:      []string{"mon", "tue", "wed", "thu", "fri"},
		},
		Theme: ThemeConfig{
			Name:   "tokyo-night",
			Accent: "cyan",
		},
		UI: UIConfig{NerdFont: false},
	}
}

func (m Manager) Load() (Config, error) {
	cfg := Default()
	if m.noConfig {
		return cfg, nil
	}

	path, err := m.Path()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	cfg.Normalize()
	return cfg, nil
}

func (m Manager) Save(cfg Config) error {
	if m.noConfig {
		return nil
	}

	cfg.Normalize()
	path, err := m.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (m Manager) Path() (string, error) {
	if m.path != "" {
		return m.path, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, AppDir, "config.yaml"), nil
}

func (c *Config) Normalize() {
	if c.Version == 0 {
		c.Version = Version
	}
	if !contains(TimeFormats, c.Time.Format) {
		c.Time.Format = "24h"
	}
	if c.Time.Timezone == "" {
		c.Time.Timezone = "Local"
	}
	if !contains(DigitStyles, c.Display.DigitStyle) {
		c.Display.DigitStyle = "block"
	}
	if c.Display.Size == "double" {
		c.Display.DigitStyle = doubleDigitStyle(c.Display.DigitStyle)
	}
	c.Display.Size = ""
	if !contains(FigletFontChoices(), c.Display.FigletFont) {
		c.Display.FigletFont = "standard"
	}
	if !contains(ToiletFontChoices(), c.Display.ToiletFont) {
		c.Display.ToiletFont = "standard"
	}
	if c.Display.SecondsStyle == "inline" {
		c.Display.InlineSeconds = true
		c.Display.SecondsStyle = "hidden"
	}
	if !contains(SecondsStyles, c.Display.SecondsStyle) {
		c.Display.SecondsStyle = "progress_bar"
	}
	if !contains(Alignments, c.Display.Alignment) {
		c.Display.Alignment = "center"
	}
	if !contains(TimeChoices, c.Workday.StartTime) {
		c.Workday.StartTime = "09:00"
	}
	if !contains(TimeChoices, c.Workday.EndTime) {
		c.Workday.EndTime = "17:00"
	}
	c.Workday.Days = normalizeWorkdays(c.Workday.Days)
	if c.Theme.Name == "" {
		c.Theme.Name = "tokyo-night"
	}
	if !contains(Accents, c.Theme.Accent) {
		c.Theme.Accent = "cyan"
	}
}

var TimeFormats = []string{"24h", "12h", "utc"}

var DigitStyles = []string{
	"block",
	"block_2x",
	"braille",
	"braille_2x",
	"braille_thin_2x",
	"braille_thin_3x",
	"braille_4x",
	"braille_thin_4x",
	"box",
	"box_2x",
	"half_block",
	"half_block_2x",
	"nerd_segment",
	"figlet",
	"toilet",
}

var SecondsStyles = []string{
	"hidden",
	"numeric",
	"progress_bar",
	"bubble_progress",
	"ascii_circle",
	"braille_circle",
	"nerd_pulse",
	"pomodoro",
	"workday",
}

var Alignments = []string{"left", "center", "right"}

var TimeChoices = []string{
	"00:00", "00:30", "01:00", "01:30", "02:00", "02:30",
	"03:00", "03:30", "04:00", "04:30", "05:00", "05:30",
	"06:00", "06:30", "07:00", "07:30", "08:00", "08:30",
	"09:00", "09:30", "10:00", "10:30", "11:00", "11:30",
	"12:00", "12:30", "13:00", "13:30", "14:00", "14:30",
	"15:00", "15:30", "16:00", "16:30", "17:00", "17:30",
	"18:00", "18:30", "19:00", "19:30", "20:00", "20:30",
	"21:00", "21:30", "22:00", "22:30", "23:00", "23:30",
}

var WorkdayNames = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

var Accents = []string{"cyan", "green", "pink", "purple", "yellow", "orange", "red", "blue"}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func doubleDigitStyle(style string) string {
	switch style {
	case "block", "block_2x":
		return "block_2x"
	case "braille", "braille_2x", "braille_4x":
		return "braille_2x"
	case "braille_thin_2x", "braille_thin_3x", "braille_thin_4x":
		return "braille_thin_2x"
	case "box", "box_2x":
		return "box_2x"
	case "half_block", "half_block_2x":
		return "half_block_2x"
	default:
		return style
	}
}

func normalizeWorkdays(days []string) []string {
	seen := make(map[string]bool, len(days))
	out := make([]string, 0, len(days))
	for _, day := range WorkdayNames {
		if contains(days, day) && !seen[day] {
			out = append(out, day)
			seen[day] = true
		}
	}
	if len(out) == 0 {
		return []string{"mon", "tue", "wed", "thu", "fri"}
	}
	return out
}
