package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"clk/internal/config"
	"clk/internal/ics"
	"clk/internal/render"
	"clk/internal/theme"
)

const testCalendarURL = "https://example.com/work.ics"

func addTestCalendarSource(cfg *config.Config) {
	cfg.Calendar.URL = testCalendarURL
}

func setTestCalendarEvents(m *Model, events []ics.Event) {
	if m.calendarEvents == nil {
		m.calendarEvents = make(map[string][]ics.Event)
	}
	m.calendarEvents[testCalendarURL] = events
}

func firstCalendarLastEvent(cfg config.CalendarConfig) config.CalendarEventConfig {
	if len(cfg.Sources) == 0 {
		return config.CalendarEventConfig{}
	}
	return cfg.Sources[0].LastEvent
}

func TestModelRendersClock(t *testing.T) {
	t.Setenv("LC_ALL", "C")

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

func TestWorkdayScheduleSettingsChangeNoConfig(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	m.workdays = true

	m.changeWorkdayTime(1)
	if m.cfg.Workday.Schedule["mon"].StartTime == "09:00" {
		t.Fatal("expected monday work start setting to change")
	}
	m.workdayEditEnd = true
	m.changeWorkdayTime(1)
	if m.cfg.Workday.Schedule["mon"].EndTime == "17:00" {
		t.Fatal("expected monday work end setting to change")
	}
}

func TestProgressBackgroundSettingsChangeNoConfig(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	m.cursor = settingIndex(m, "Bar bg")

	m.changeSetting(1)
	if m.cfg.Display.ProgressEmptyBackground != "accent" {
		t.Fatalf("expected progress empty background to change to accent, got %q", m.cfg.Display.ProgressEmptyBackground)
	}
	if got := progressEmptyBackgroundColor(m.cfg); got != theme.Accent(theme.Lookup(m.cfg.Theme.Name), m.cfg.Theme.Accent) {
		t.Fatalf("expected accent progress empty background color, got %q", got)
	}
	if m.saveError != "" {
		t.Fatalf("unexpected save error: %s", m.saveError)
	}
}

func TestSettingsExposeScheduleProgressControls(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	for _, label := range []string{"Workday bar", "ICS bar", "ICS mode", "ICS URLs", "Emoji"} {
		if !hasSettingItem(m, label) {
			t.Fatalf("expected setting %q", label)
		}
	}
}

func TestCalendarURLSettingsEditorSavesNoConfig(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	m.cursor = settingIndex(m, "ICS URLs")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(Model)
	if !updated.urlEditor {
		t.Fatal("expected calendar URL editor to open")
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" https://example.com/work.ics ")})
	updated = next.(Model)
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	updated = next.(Model)
	if updated.urlEditor {
		t.Fatal("expected calendar URL editor to close after save")
	}
	if len(updated.cfg.Calendar.Sources) != 1 || updated.cfg.Calendar.Sources[0].URL != "https://example.com/work.ics" {
		t.Fatalf("expected trimmed calendar URL to be saved as source, got %+v", updated.cfg.Calendar.Sources)
	}
	if updated.saveError != "" {
		t.Fatalf("unexpected save error: %s", updated.saveError)
	}
}

func TestCalendarURLChangeClearsRememberedLastEvent(t *testing.T) {
	cfg := config.Default()
	oldURL := "https://example.com/old.ics"
	cfg.Calendar.URL = oldURL
	cfg.Calendar.LastEvent = config.CalendarEventConfig{
		SourceURL: oldURL,
		Summary:   "Old",
		Start:     time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
	}
	m := New(cfg, config.NewManager("", true))
	m.settings = true
	m.urlEditor = true
	m.urlInput = newCalendarURLInput()
	m.urlInput.SetValue("https://example.com/new.ics")
	m.calendarEvents = map[string][]ics.Event{
		oldURL: {{Summary: "Cached", Start: cfg.Calendar.LastEvent.Start, End: cfg.Calendar.LastEvent.End}},
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	updated := next.(Model)
	if len(updated.cfg.Calendar.Sources) != 1 || updated.cfg.Calendar.Sources[0].URL != "https://example.com/new.ics" {
		t.Fatalf("expected changed calendar source, got %+v", updated.cfg.Calendar.Sources)
	}
	if !updated.cfg.Calendar.Sources[0].LastEvent.Start.IsZero() {
		t.Fatalf("expected remembered calendar event to be cleared, got %+v", updated.cfg.Calendar.Sources[0].LastEvent)
	}
	if len(updated.calendarEvents) != 0 {
		t.Fatalf("expected cached calendar events to be cleared, got %+v", updated.calendarEvents)
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
		wantFclk   bool
	}{
		{style: "block"},
		{style: "figlet", wantFiglet: true},
		{style: "toilet", wantToilet: true},
		{style: "fclk", wantFclk: true},
	}
	for _, tc := range cases {
		cfg := config.Default()
		cfg.Display.DigitStyle = tc.style
		m := New(cfg, config.NewManager("", true))

		gotFiglet := hasSettingItem(m, "Figlet font")
		gotToilet := hasSettingItem(m, "Toilet font")
		gotFclk := hasSettingItem(m, "FCLK font")
		if gotFiglet != tc.wantFiglet || gotToilet != tc.wantToilet || gotFclk != tc.wantFclk {
			t.Fatalf("%s settings figlet=%v toilet=%v fclk=%v, want figlet=%v toilet=%v fclk=%v", tc.style, gotFiglet, gotToilet, gotFclk, tc.wantFiglet, tc.wantToilet, tc.wantFclk)
		}
	}
}

func TestSlashOpensFontPickerForExternalFontSetting(t *testing.T) {
	cfg := config.Default()
	cfg.Display.DigitStyle = "figlet"
	m := New(cfg, config.NewManager("", true))
	m.settings = true
	m.height = 30
	m.cursor = settingIndex(m, "Figlet font")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := next.(Model)
	if !updated.fontPicker || updated.fontPickerKind != "figlet" {
		t.Fatalf("expected figlet font picker to open, got picker=%v kind=%q", updated.fontPicker, updated.fontPickerKind)
	}
	if updated.fontQuery != "" {
		t.Fatalf("expected empty initial query, got %q", updated.fontQuery)
	}
}

func TestFontPickerFiltersAndSelectsFont(t *testing.T) {
	cfg := config.Default()
	cfg.Display.DigitStyle = "figlet"
	m := New(cfg, config.NewManager("", true))
	m.settings = true
	m.fontPicker = true
	m.fontPickerKind = "figlet"

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("slant")})
	updated := next.(Model)
	if updated.fontQuery != "slant" {
		t.Fatalf("expected query slant, got %q", updated.fontQuery)
	}
	choices := updated.filteredFontChoices()
	if len(choices) == 0 || strings.ToLower(fontChoiceLabel(choices[0])) != "slant" {
		t.Fatalf("expected slant match, got %+v", choices)
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = next.(Model)
	if strings.ToLower(fontChoiceLabel(updated.cfg.Display.FigletFont)) != "slant" {
		t.Fatalf("expected selected figlet font slant, got %q", updated.cfg.Display.FigletFont)
	}
	if updated.fontPicker {
		t.Fatal("expected picker to close after selection")
	}
}

func TestWorkdaySettingsOpenCheckboxSubmenu(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.settings = true
	m.height = 30
	items := m.settingItems()
	for i, item := range items {
		if item.label == "Work schedule" {
			m.cursor = i
			break
		}
	}

	m.changeSetting(1)
	if !m.workdays {
		t.Fatal("expected workdays submenu to open")
	}

	m.dayCursor = 5
	m.adjustDayScroll()
	out := m.settingsView(testStyles())
	if !strings.Contains(out, "[x] Tuesday") || !strings.Contains(out, "[ ] Saturday") || !strings.Contains(out, "09:00") {
		t.Fatalf("expected checkbox schedule rows, got %q", out)
	}
}

func TestSettingsViewUsesAtMostHalfHeightWhenPossible(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.height = 30

	out := m.settingsView(testStyles())
	if got, want := lipgloss.Height(out), m.height/2; got > want {
		t.Fatalf("expected settings view height <= %d, got %d in %q", want, got, out)
	}
}

func TestSettingsViewShowsAtLeastThreeOptions(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	m.height = 10

	out := m.settingsView(testStyles())
	for _, label := range []string{"Theme", "Accent", "Time format"} {
		if !strings.Contains(out, label) {
			t.Fatalf("expected small settings view to include %q, got %q", label, out)
		}
	}
}

func settingIndex(m Model, label string) int {
	for i, item := range m.settingItems() {
		if item.label == label {
			return i
		}
	}
	return -1
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
	if !m.cfg.Workday.Schedule["sat"].Enabled {
		t.Fatalf("expected saturday enabled, got %+v", m.cfg.Workday.Schedule["sat"])
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

func TestCalendarProgressBarRendersFetchedEvent(t *testing.T) {
	cfg := config.Default()
	cfg.Display.SecondsStyle = "hidden"
	cfg.Calendar.ShowProgress = true
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.now = time.Date(2026, 5, 1, 9, 30, 0, 0, time.Local)
	setTestCalendarEvents(&m, []ics.Event{
		{
			Summary: "Standup",
			Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local),
		},
	})

	out := m.View()
	if !strings.Contains(out, "Standup") || !strings.Contains(out, "50%") {
		t.Fatalf("expected calendar event progress, got %q", out)
	}
}

func TestCalendarProgressBarCountsDownFromLaunchTime(t *testing.T) {
	cfg := config.Default()
	cfg.Display.SecondsStyle = "hidden"
	cfg.Calendar.ShowProgress = true
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.startedAt = time.Date(2026, 5, 1, 8, 0, 0, 0, time.Local)
	m.now = time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)
	setTestCalendarEvents(&m, []ics.Event{
		{
			Summary: "Later",
			Start:   time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 13, 0, 0, 0, time.Local),
		},
	})

	out := m.View()
	if !strings.Contains(out, "Later") || !strings.Contains(out, "50%") {
		t.Fatalf("expected calendar countdown from launch time, got %q", out)
	}
}

func TestCalendarProgressBarCountsDownFromRememberedLastEvent(t *testing.T) {
	cfg := config.Default()
	cfg.Display.SecondsStyle = "hidden"
	cfg.Calendar.ShowProgress = true
	addTestCalendarSource(&cfg)
	cfg.Calendar.LastEvent = config.CalendarEventConfig{
		SourceURL: testCalendarURL,
		Summary:   "Previous",
		Start:     time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
		End:       time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local),
	}
	m := New(cfg, config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.startedAt = time.Date(2026, 5, 1, 11, 0, 0, 0, time.Local)
	m.now = time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	setTestCalendarEvents(&m, []ics.Event{
		{
			Summary: "Later",
			Start:   time.Date(2026, 5, 1, 14, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 15, 0, 0, 0, time.Local),
		},
	})

	out := m.View()
	if !strings.Contains(out, "Later") || !strings.Contains(out, "50%") {
		t.Fatalf("expected calendar countdown from remembered last event, got %q", out)
	}
}

func TestCalendarFetchRemembersMostRecentPastEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	cfg.Display.SecondsStyle = "hidden"
	cfg.Calendar.ShowProgress = true
	addTestCalendarSource(&cfg)
	manager := config.NewManager(path, false)
	m := New(cfg, manager)
	m.now = time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)

	next, _ := m.Update(calendarFetchMsg{
		url: testCalendarURL,
		events: []ics.Event{
			{
				Summary: "Older",
				Start:   time.Date(2026, 5, 1, 8, 0, 0, 0, time.Local),
				End:     time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
			},
			{
				Summary: "Recent",
				Start:   time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local),
				End:     time.Date(2026, 5, 1, 11, 0, 0, 0, time.Local),
			},
			{
				Summary: "Next",
				Start:   time.Date(2026, 5, 1, 14, 0, 0, 0, time.Local),
				End:     time.Date(2026, 5, 1, 15, 0, 0, 0, time.Local),
			},
		},
	})
	updated := next.(Model)
	if firstCalendarLastEvent(updated.cfg.Calendar).Summary != "Recent" {
		t.Fatalf("expected most recent past event to be remembered, got %+v", firstCalendarLastEvent(updated.cfg.Calendar))
	}
	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if firstCalendarLastEvent(loaded.Calendar).Summary != "Recent" {
		t.Fatalf("expected remembered event to be persisted, got %+v", firstCalendarLastEvent(loaded.Calendar))
	}
}

func TestCalendarTickRemembersEventAfterItEnds(t *testing.T) {
	cfg := config.Default()
	cfg.Calendar.ShowProgress = true
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	setTestCalendarEvents(&m, []ics.Event{
		{
			Summary: "Finished",
			Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local),
		},
	})

	next, _ := m.Update(tickMsg(time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)))
	updated := next.(Model)
	if firstCalendarLastEvent(updated.cfg.Calendar).Summary != "Finished" {
		t.Fatalf("expected ended event to be remembered on tick, got %+v", firstCalendarLastEvent(updated.cfg.Calendar))
	}
}

func TestWorkdayAndCalendarProgressBarsCanRenderTogether(t *testing.T) {
	cfg := config.Default()
	cfg.Display.SecondsStyle = "hidden"
	cfg.Workday.ShowProgress = true
	cfg.Calendar.ShowProgress = true
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.now = time.Date(2026, 5, 1, 13, 30, 0, 0, time.Local)
	setTestCalendarEvents(&m, []ics.Event{
		{
			Summary: "Planning",
			Start:   time.Date(2026, 5, 1, 13, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 14, 0, 0, 0, time.Local),
		},
	})

	out := m.View()
	if !strings.Contains(out, "☼") || !strings.Contains(out, "Planning") {
		t.Fatalf("expected workday and calendar progress bars, got %q", out)
	}
}

func TestCalendarSplitModeRendersEachSourceProgressBar(t *testing.T) {
	workURL := "https://example.com/work.ics"
	homeURL := "https://example.com/home.ics"
	cfg := config.Default()
	cfg.Display.SecondsStyle = "hidden"
	cfg.Calendar.ShowProgress = true
	cfg.Calendar.Mode = "split"
	cfg.Calendar.Sources = []config.CalendarSourceConfig{
		{URL: workURL},
		{URL: homeURL},
	}
	m := New(cfg, config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.now = time.Date(2026, 5, 1, 9, 30, 0, 0, time.Local)
	m.calendarEvents = map[string][]ics.Event{
		workURL: {
			{
				Summary: "Standup",
				Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
				End:     time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local),
			},
		},
		homeURL: {
			{
				Summary: "Appointment",
				Start:   time.Date(2026, 5, 1, 9, 15, 0, 0, time.Local),
				End:     time.Date(2026, 5, 1, 9, 45, 0, 0, time.Local),
			},
		},
	}

	out := m.View()
	if !strings.Contains(out, "Standup") || !strings.Contains(out, "Appointment") {
		t.Fatalf("expected split calendar rows for both sources, got %q", out)
	}
	calendar := m.calendarProgressView(m.now, 80)
	if !strings.Contains(calendar, "\n\n") {
		t.Fatalf("expected blank line between split calendar rows, got %q", calendar)
	}
}

func TestProgressRemainingFillUsesDimmedAccentBlock(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Name = "tokyo-night"
	cfg.Theme.Accent = "cyan"

	m := New(cfg, config.NewManager("", true))
	if m.progress.Empty != progressEmptyCharacter {
		t.Fatalf("expected remaining progress character %q, got %q", progressEmptyCharacter, m.progress.Empty)
	}
	if m.progress.EmptyColor != "#3e677f" {
		t.Fatalf("expected half-brightness cyan empty color, got %q", m.progress.EmptyColor)
	}
}

func TestProgressViewStylesEmptyCellsWithConfiguredBackground(t *testing.T) {
	cfg := config.Default()
	cfg.Display.ProgressEmptyBackground = "warning"
	m := New(cfg, config.NewManager("", true))

	background := progressEmptyBackgroundColor(cfg)
	want := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.progress.EmptyColor)).
		Background(lipgloss.Color(background)).
		Render(string(progressEmptyCharacter))

	out := m.progressView(0, 12)
	if !strings.Contains(out, want) {
		t.Fatalf("expected empty progress cells to include configured background style %q, got %q", want, out)
	}
}

func TestWorkdayProgressViewAddsNerdFontDirectionMarkers(t *testing.T) {
	cfg := config.Default()
	cfg.UI.NerdFont = true
	m := New(cfg, config.NewManager("", true))

	opts := render.SecondsOptions{
		Style:           "workday",
		Width:           48,
		NerdFont:        true,
		Progress:        m.progressView,
		WorkdayProgress: m.workdayProgressView,
		Workday:         workdayOptions(config.Default().Workday),
	}

	opts.Time = time.Date(2026, 5, 1, 13, 0, 0, 0, time.Local)
	if got := render.SecondsStyled(opts); !strings.Contains(got, "") || strings.Contains(got, "") {
		t.Fatalf("expected mid-workday up marker, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 1, 23, 0, 0, 0, time.Local)
	if got := render.SecondsStyled(opts); !strings.Contains(got, "") || strings.Contains(got, "") {
		t.Fatalf("expected after-workday down marker, got %q", got)
	}
}

func TestWorkdayProgressViewKeepsStableWidth(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))

	plain := m.progressView(0.5, 48)
	up := m.workdayProgressView(0.5, 48, render.ProgressDirectionUp)
	down := m.workdayProgressView(0.5, 48, render.ProgressDirectionDown)

	if lipgloss.Width(up) != lipgloss.Width(plain) {
		t.Fatalf("expected up marker width %d, got %d in %q", lipgloss.Width(plain), lipgloss.Width(up), up)
	}
	if lipgloss.Width(down) != lipgloss.Width(plain) {
		t.Fatalf("expected down marker width %d, got %d in %q", lipgloss.Width(plain), lipgloss.Width(down), down)
	}
}

func TestWorkdayProgressViewPlacesDownMarkerAtLeftEdge(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))

	up := m.workdayProgressView(0.5, 48, render.ProgressDirectionUp)
	down := m.workdayProgressView(0.5, 48, render.ProgressDirectionDown)

	upMarker := strings.Index(up, "")
	upFirstFill := strings.Index(up, string(m.progress.Full))
	if upMarker <= upFirstFill {
		t.Fatalf("expected up marker after filled cells, got %q", up)
	}

	downMarker := strings.Index(down, "")
	downNextFill := strings.Index(down, string(m.progress.Full))
	if downMarker < 0 {
		t.Fatalf("expected down marker, got %q", down)
	}
	if downNextFill >= 0 && downMarker >= downNextFill {
		t.Fatalf("expected down marker at the left edge before filled cells, got %q", down)
	}
}

func TestWorkdayProgressViewFallsBackWithoutNerdFont(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	opts := render.SecondsOptions{
		Style:           "workday",
		Width:           48,
		NerdFont:        false,
		Progress:        m.progressView,
		WorkdayProgress: m.workdayProgressView,
		Workday:         workdayOptions(config.Default().Workday),
		Time:            time.Date(2026, 5, 1, 13, 0, 0, 0, time.Local),
	}

	if got := render.SecondsStyled(opts); strings.Contains(got, "") || strings.Contains(got, "") {
		t.Fatalf("expected no powerline marker without Nerd Font, got %q", got)
	}
}

func TestWorkdayProgressViewOmitsMarkersAtEdges(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))

	cases := []struct {
		percent   float64
		direction render.ProgressDirection
	}{
		{0, render.ProgressDirectionUp},
		{0, render.ProgressDirectionDown},
		{1, render.ProgressDirectionUp},
		{1, render.ProgressDirectionDown},
	}

	for _, tc := range cases {
		got := m.workdayProgressView(tc.percent, 48, tc.direction)
		if strings.Contains(got, "") || strings.Contains(got, "") {
			t.Fatalf("expected no edge marker for %v %s, got %q", tc.percent, tc.direction, got)
		}
		if lipgloss.Width(got) != lipgloss.Width(m.progressView(tc.percent, 48)) {
			t.Fatalf("expected edge marker width to remain stable, got %q", got)
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
