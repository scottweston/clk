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
	Size           string `yaml:"size"`
	BlinkSeparator bool   `yaml:"blink_separator"`
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
			Size:           "normal",
			BlinkSeparator: false,
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
	if !contains(FigletFonts, c.Display.FigletFont) {
		c.Display.FigletFont = "standard"
	}
	if !contains(ToiletFonts, c.Display.ToiletFont) {
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
	if !contains(Sizes, c.Display.Size) {
		c.Display.Size = "normal"
	}
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
	"braille",
	"box",
	"half_block",
	"nerd_segment",
	"figlet",
	"toilet",
}

var FigletFonts = []string{"standard", "big", "block", "bubble", "digital", "mini", "script", "slant", "small"}

var ToiletFonts = []string{"standard", "term", "future", "mono12", "smblock", "smmono12", "pagga", "emboss"}

var SecondsStyles = []string{
	"hidden",
	"numeric",
	"progress_bar",
	"bubble_progress",
	"ascii_circle",
	"braille_circle",
	"nerd_pulse",
	"pomodoro",
}

var Alignments = []string{"left", "center", "right"}

var Sizes = []string{"normal", "double"}

var Accents = []string{"cyan", "green", "pink", "purple", "yellow", "orange", "red", "blue"}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
