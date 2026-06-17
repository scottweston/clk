package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	Version = 1
	AppDir  = "clk"
)

type Config struct {
	Version  int            `yaml:"version"`
	Time     TimeConfig     `yaml:"time"`
	Display  DisplayConfig  `yaml:"display"`
	Workday  WorkdayConfig  `yaml:"workday"`
	Calendar CalendarConfig `yaml:"calendar"`
	Theme    ThemeConfig    `yaml:"theme"`
	UI       UIConfig       `yaml:"ui"`
}

type TimeConfig struct {
	Format   string `yaml:"format"`
	ShowDate bool   `yaml:"show_date"`
	Timezone string `yaml:"timezone"`
}

type DisplayConfig struct {
	DigitStyle              string `yaml:"digit_style"`
	FigletFont              string `yaml:"figlet_font"`
	ToiletFont              string `yaml:"toilet_font"`
	FclkFont                string `yaml:"fclk_font"`
	SecondsStyle            string `yaml:"seconds_style"`
	InlineSeconds           bool   `yaml:"inline_seconds"`
	ProgressEmptyBackground string `yaml:"progress_empty_background"`
	PrideMode               string `yaml:"pride"`
	PrideUnlocked           bool   `yaml:"pride_unlocked,omitempty"`
	Alignment               string `yaml:"alignment"`
	Size                    string `yaml:"size,omitempty"`
	BlinkSeparator          bool   `yaml:"blink_separator"`
}

type WorkdayConfig struct {
	Schedule        map[string]WorkdayDayConfig `yaml:"schedule"`
	ShowProgress    bool                        `yaml:"show_progress"`
	legacyStartTime string
	legacyEndTime   string
	legacyDays      []string
}

type WorkdayDayConfig struct {
	Enabled   bool   `yaml:"enabled"`
	StartTime string `yaml:"start_time"`
	EndTime   string `yaml:"end_time"`
}

type CalendarConfig struct {
	ShowProgress   bool                   `yaml:"show_progress"`
	URL            string                 `yaml:"url,omitempty"`
	Sources        []CalendarSourceConfig `yaml:"sources,omitempty"`
	Mode           string                 `yaml:"mode"`
	RefreshMinutes int                    `yaml:"refresh_minutes"`
	LastEvent      CalendarEventConfig    `yaml:"last_event,omitempty"`
}

type CalendarSourceConfig struct {
	URL       string              `yaml:"url"`
	LastEvent CalendarEventConfig `yaml:"last_event,omitempty"`
}

type CalendarEventConfig struct {
	SourceURL string    `yaml:"source_url,omitempty"`
	Summary   string    `yaml:"summary,omitempty"`
	Start     time.Time `yaml:"start,omitempty"`
	End       time.Time `yaml:"end,omitempty"`
}

type ThemeConfig struct {
	Name   string `yaml:"name"`
	Accent string `yaml:"accent"`
}

type UIConfig struct {
	NerdFont bool `yaml:"nerd_font"`
	Emoji    bool `yaml:"emoji"`
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
			DigitStyle:              "block",
			FigletFont:              "standard",
			ToiletFont:              "standard",
			FclkFont:                "",
			SecondsStyle:            "progress_bar",
			InlineSeconds:           false,
			ProgressEmptyBackground: "theme",
			PrideMode:               "auto",
			PrideUnlocked:           false,
			Alignment:               "center",
			BlinkSeparator:          false,
		},
		Workday: WorkdayConfig{
			Schedule:     defaultWorkdaySchedule(),
			ShowProgress: false,
		},
		Calendar: CalendarConfig{
			ShowProgress:   false,
			URL:            "",
			Mode:           "merged",
			RefreshMinutes: 15,
		},
		Theme: ThemeConfig{
			Name:   "tokyo-night",
			Accent: "cyan",
		},
		UI: UIConfig{NerdFont: false, Emoji: false},
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
	fclkFonts := FclkFontChoices()
	if len(fclkFonts) > 0 && !contains(fclkFonts, c.Display.FclkFont) {
		c.Display.FclkFont = fclkFonts[0]
	}
	if len(fclkFonts) == 0 {
		c.Display.FclkFont = ""
	}
	if c.Display.SecondsStyle == "inline" {
		c.Display.InlineSeconds = true
		c.Display.SecondsStyle = "hidden"
	}
	if c.Display.SecondsStyle == "workday" {
		c.Workday.ShowProgress = true
		c.Display.SecondsStyle = "hidden"
	}
	if !contains(SecondsStyles, c.Display.SecondsStyle) {
		c.Display.SecondsStyle = "progress_bar"
	}
	if !contains(ProgressEmptyBackgrounds, c.Display.ProgressEmptyBackground) {
		c.Display.ProgressEmptyBackground = "theme"
	}
	if !contains(PrideModes, c.Display.PrideMode) {
		c.Display.PrideMode = "auto"
	}
	if !contains(Alignments, c.Display.Alignment) {
		c.Display.Alignment = "center"
	}
	c.Workday.Normalize()
	c.Calendar.Normalize()
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
	"braille_thin_cmf_2x",
	"braille_thin_3x",
	"braille_thin_cmf_3x",
	"braille_4x",
	"braille_thin_4x",
	"braille_thin_cmf_4x",
	"box",
	"box_2x",
	"half_block",
	"half_block_2x",
	"nerd_segment",
	"fclk",
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
}

var ProgressEmptyBackgrounds = []string{"theme", "accent", "muted", "foreground", "warning"}

var PrideModes = []string{"horizontal", "vertical", "diagonal", "auto", "off"}

var CalendarModes = []string{"merged", "split"}

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

func (w *WorkdayConfig) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownMappingFields(value, map[string]bool{
		"schedule":      true,
		"show_progress": true,
		"start_time":    true,
		"end_time":      true,
		"days":          true,
	}); err != nil {
		return err
	}
	type rawWorkdayConfig struct {
		Schedule     map[string]WorkdayDayConfig `yaml:"schedule"`
		ShowProgress bool                        `yaml:"show_progress"`
		StartTime    string                      `yaml:"start_time"`
		EndTime      string                      `yaml:"end_time"`
		Days         []string                    `yaml:"days"`
	}
	var raw rawWorkdayConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	w.Schedule = raw.Schedule
	w.ShowProgress = raw.ShowProgress
	w.legacyStartTime = raw.StartTime
	w.legacyEndTime = raw.EndTime
	w.legacyDays = raw.Days
	return nil
}

func (w *WorkdayDayConfig) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownMappingFields(value, map[string]bool{
		"enabled":    true,
		"start_time": true,
		"end_time":   true,
	}); err != nil {
		return err
	}
	type rawWorkdayDayConfig WorkdayDayConfig
	var raw rawWorkdayDayConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*w = WorkdayDayConfig(raw)
	return nil
}

func (c *CalendarConfig) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownMappingFields(value, map[string]bool{
		"show_progress":   true,
		"url":             true,
		"sources":         true,
		"mode":            true,
		"refresh_minutes": true,
		"last_event":      true,
	}); err != nil {
		return err
	}
	type rawCalendarConfig CalendarConfig
	var raw rawCalendarConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*c = CalendarConfig(raw)
	return nil
}

func (s *CalendarSourceConfig) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownMappingFields(value, map[string]bool{
		"url":        true,
		"last_event": true,
	}); err != nil {
		return err
	}
	type rawCalendarSourceConfig CalendarSourceConfig
	var raw rawCalendarSourceConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*s = CalendarSourceConfig(raw)
	return nil
}

func (e *CalendarEventConfig) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownMappingFields(value, map[string]bool{
		"source_url": true,
		"summary":    true,
		"start":      true,
		"end":        true,
	}); err != nil {
		return err
	}
	type rawCalendarEventConfig CalendarEventConfig
	var raw rawCalendarEventConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*e = CalendarEventConfig(raw)
	return nil
}

func rejectUnknownMappingFields(value *yaml.Node, allowed map[string]bool) error {
	if value.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i].Value
		if !allowed[key] {
			return fmt.Errorf("field %s not found", key)
		}
	}
	return nil
}

func (w *WorkdayConfig) Normalize() {
	if len(w.Schedule) == 0 && (w.legacyStartTime != "" || w.legacyEndTime != "" || len(w.legacyDays) > 0) {
		start := normalizeWorkdayTime(w.legacyStartTime, "09:00")
		end := normalizeWorkdayTime(w.legacyEndTime, "17:00")
		days := normalizeWorkdays(w.legacyDays)
		w.Schedule = make(map[string]WorkdayDayConfig, len(WorkdayNames))
		for _, day := range WorkdayNames {
			w.Schedule[day] = WorkdayDayConfig{
				Enabled:   contains(days, day),
				StartTime: start,
				EndTime:   end,
			}
		}
	}

	if len(w.Schedule) == 0 {
		w.Schedule = defaultWorkdaySchedule()
		return
	}

	normalized := make(map[string]WorkdayDayConfig, len(WorkdayNames))
	for _, day := range WorkdayNames {
		entry := w.Schedule[day]
		if entry.StartTime == "" && entry.EndTime == "" {
			entry = defaultWorkdayEntry(day)
		}
		entry.StartTime = normalizeWorkdayTime(entry.StartTime, "09:00")
		entry.EndTime = normalizeWorkdayTime(entry.EndTime, "17:00")
		normalized[day] = entry
	}
	w.Schedule = normalized
	w.legacyStartTime = ""
	w.legacyEndTime = ""
	w.legacyDays = nil
}

func (c *CalendarConfig) Normalize() {
	c.URL = strings.TrimSpace(c.URL)
	if !contains(CalendarModes, c.Mode) {
		c.Mode = "merged"
	}
	if c.RefreshMinutes <= 0 {
		c.RefreshMinutes = 15
	}
	if c.RefreshMinutes > 24*60 {
		c.RefreshMinutes = 24 * 60
	}
	c.Sources = normalizeCalendarSources(c.URL, c.LastEvent, c.Sources)
	c.URL = ""
	c.LastEvent = CalendarEventConfig{}
}

func (s *CalendarSourceConfig) Normalize() {
	s.URL = strings.TrimSpace(s.URL)
	s.LastEvent.Normalize(s.URL)
}

func (e *CalendarEventConfig) Normalize(sourceURL string) {
	sourceURL = strings.TrimSpace(sourceURL)
	e.SourceURL = strings.TrimSpace(e.SourceURL)
	e.Summary = strings.TrimSpace(e.Summary)
	if e.SourceURL == "" {
		e.SourceURL = sourceURL
	}
	if sourceURL == "" || e.SourceURL == "" || e.SourceURL != sourceURL || e.Start.IsZero() || e.End.IsZero() || !e.End.After(e.Start) {
		*e = CalendarEventConfig{}
	}
}

func normalizeCalendarSources(legacyURL string, legacyLastEvent CalendarEventConfig, sources []CalendarSourceConfig) []CalendarSourceConfig {
	seen := make(map[string]bool, len(sources)+1)
	index := make(map[string]int, len(sources)+1)
	out := make([]CalendarSourceConfig, 0, len(sources)+1)
	for _, source := range sources {
		source.Normalize()
		if source.URL == "" || seen[source.URL] {
			continue
		}
		index[source.URL] = len(out)
		out = append(out, source)
		seen[source.URL] = true
	}
	if legacyURL != "" {
		source := CalendarSourceConfig{URL: legacyURL, LastEvent: legacyLastEvent}
		source.Normalize()
		if source.URL == "" {
			return out
		}
		if idx, ok := index[source.URL]; ok {
			if calendarEventEmpty(out[idx].LastEvent) && !calendarEventEmpty(source.LastEvent) {
				out[idx].LastEvent = source.LastEvent
			}
		} else {
			out = append(out, source)
		}
	}
	return out
}

func calendarEventEmpty(event CalendarEventConfig) bool {
	return event.SourceURL == "" && event.Summary == "" && event.Start.IsZero() && event.End.IsZero()
}

func defaultWorkdaySchedule() map[string]WorkdayDayConfig {
	schedule := make(map[string]WorkdayDayConfig, len(WorkdayNames))
	for _, day := range WorkdayNames {
		schedule[day] = defaultWorkdayEntry(day)
	}
	return schedule
}

func defaultWorkdayEntry(day string) WorkdayDayConfig {
	return WorkdayDayConfig{
		Enabled:   contains([]string{"mon", "tue", "wed", "thu", "fri"}, day),
		StartTime: "09:00",
		EndTime:   "17:00",
	}
}

func normalizeWorkdayTime(value, fallback string) string {
	if contains(TimeChoices, value) {
		return value
	}
	return fallback
}

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
	case "braille_thin_cmf_2x", "braille_thin_cmf_3x", "braille_thin_cmf_4x":
		return "braille_thin_cmf_2x"
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
