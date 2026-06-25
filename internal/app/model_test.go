package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"clk/internal/config"
	"clk/internal/ics"
	"clk/internal/render"
	"clk/internal/share"
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

func runModelCommands(t *testing.T, model Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return model
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			model = runModelCommands(t, model, child)
		}
		return model
	}
	next, _ := model.Update(msg)
	return next.(Model)
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
	for _, label := range []string{"Workday bar", "ICS bar", "Data sharing", "ICS mode", "ICS URLs", "Emoji"} {
		if !hasSettingItem(m, label) {
			t.Fatalf("expected setting %q", label)
		}
	}
}

func TestCalendarDataNeededForSharingWithoutICSBar(t *testing.T) {
	cfg := config.Default()
	cfg.Calendar.ShowProgress = false
	cfg.Sharing.Enabled = true
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	if !calendarDataNeeded(m.cfg) {
		t.Fatal("expected sharing to require calendar data")
	}
	if m.fetchCalendarCmd() == nil || m.scheduleCalendarRefreshCmd() == nil {
		t.Fatal("expected calendar fetch and refresh while sharing is enabled")
	}
}

func TestDataSharingSettingStartsAndStopsSocket(t *testing.T) {
	m := New(config.Default(), config.NewManager("", true))
	path := filepath.Join(t.TempDir(), "clk.sock")
	m.sharing = share.NewAt(path)
	m.settings = true
	m.cursor = settingIndex(m, "Data sharing")

	cmd := m.changeSetting(1)
	m = runModelCommands(t, m, cmd)
	if strings.Contains(m.sharingError, "operation not permitted") {
		t.Skipf("unix sockets are not permitted in this sandbox: %s", m.sharingError)
	}
	if !m.cfg.Sharing.Enabled {
		t.Fatal("expected sharing setting to be enabled")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("expected sharing socket: %v", err)
	}

	cmd = m.changeSetting(1)
	m = runModelCommands(t, m, cmd)
	if m.cfg.Sharing.Enabled {
		t.Fatal("expected sharing setting to be disabled")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected sharing socket removal, got %v", err)
	}
}

func TestDataSharingStartFailureDoesNotDisablePreference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clk.sock")
	if err := os.WriteFile(path, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("write collision file: %v", err)
	}
	m := New(config.Default(), config.NewManager("", true))
	m.sharing = share.NewAt(path)
	m.cfg.Sharing.Enabled = true

	next, _ := m.Update(m.sharingControlCmd(true)())
	updated := next.(Model)
	if !updated.cfg.Sharing.Enabled || !strings.Contains(updated.sharingError, "data sharing failed") {
		t.Fatalf("expected enabled preference and operational error, got enabled=%v error=%q", updated.cfg.Sharing.Enabled, updated.sharingError)
	}
}

func TestCalendarFetchPublishesRawDataToSharingAPI(t *testing.T) {
	cfg := config.Default()
	cfg.Time.Format = "utc"
	cfg.Sharing.Enabled = true
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	m.now = time.Now().UTC()
	start := m.now.Add(10 * time.Minute)
	end := start.Add(30 * time.Minute)
	data := []byte("BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Shared event\nDTSTART:" + start.Format("20060102T150405Z") + "\nDTEND:" + end.Format("20060102T150405Z") + "\nEND:VEVENT\nEND:VCALENDAR\n")
	events, err := ics.Parse(data, m.now)
	if err != nil {
		t.Fatalf("parse test calendar: %v", err)
	}

	next, _ := m.Update(calendarFetchMsg{url: testCalendarURL, events: events, data: data})
	updated := next.(Model)
	recorder := httptest.NewRecorder()
	updated.sharing.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events/1h", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Shared event") {
		t.Fatalf("expected shared calendar data, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestPrideSettingHiddenUntilJuneOrUnlock(t *testing.T) {
	cfg := config.Default()
	m := New(cfg, config.NewManager("", true))
	m.now = time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)
	if hasSettingItem(m, "Pride") {
		t.Fatal("expected pride setting to be hidden before unlock outside pride month")
	}

	m.now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.Local)
	if !hasSettingItem(m, "Pride") {
		t.Fatal("expected pride setting to be visible during pride month")
	}

	cfg.Display.PrideUnlocked = true
	m = New(cfg, config.NewManager("", true))
	m.now = time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)
	if !hasSettingItem(m, "Pride") {
		t.Fatal("expected pride setting to stay visible after unlock")
	}
}

func TestPrideUnlockPersistsDuringJune(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	manager := config.NewManager(path, false)
	m := New(config.Default(), manager)
	m.now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.Local)

	next, _ := m.Update(prideUnlockMsg{})
	updated := next.(Model)
	if !updated.cfg.Display.PrideUnlocked {
		t.Fatal("expected pride unlock to update model config")
	}
	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !loaded.Display.PrideUnlocked {
		t.Fatalf("expected pride unlock to be persisted, got %+v", loaded.Display)
	}
}

func TestPrideClockAutoChoosesOrientationFromClockSize(t *testing.T) {
	background := "#000000"
	vertical := prideClockArt("abcdef", "auto", background)
	if !strings.Contains(vertical, prideCellStyle(prideColors[0], background).Render("a")) ||
		!strings.Contains(vertical, prideCellStyle(prideColors[5], background).Render("f")) {
		t.Fatalf("expected one-line auto pride clock to render vertical stripes, got %q", vertical)
	}

	horizontal := prideClockArt("a\nb\nc\nd\ne\nf", "auto", background)
	if !strings.Contains(horizontal, prideCellStyle(prideColors[0], background).Render("a")) ||
		!strings.Contains(horizontal, prideCellStyle(prideColors[5], background).Render("f")) {
		t.Fatalf("expected six-line auto pride clock to render horizontal stripes, got %q", horizontal)
	}
}

func TestPrideClockOnlyActiveOutsideJuneAfterUnlock(t *testing.T) {
	display := config.Default().Display
	july := time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)
	if prideClockActive(display, july) {
		t.Fatal("expected pride clock to be inactive outside June before unlock")
	}
	display.PrideUnlocked = true
	if !prideClockActive(display, july) {
		t.Fatal("expected pride clock to stay active outside June after unlock")
	}
	display.PrideMode = "off"
	if prideClockActive(display, july) {
		t.Fatal("expected off pride mode to disable pride clock")
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

func TestEventPeekTogglesAndListsUpcomingEvents(t *testing.T) {
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.Local)
	cfg := config.Default()
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	m.width = 90
	m.height = 24
	m.now = now
	setTestCalendarEvents(&m, []ics.Event{
		{
			Summary: "Finished",
			Start:   now.Add(-2 * time.Hour),
			End:     now.Add(-time.Hour),
		},
		{
			Summary: "Planning",
			Start:   now.Add(3 * time.Hour),
			End:     now.Add(4 * time.Hour),
		},
		{
			Summary: "Dinner",
			Start:   time.Date(2026, 5, 4, 19, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 4, 20, 0, 0, 0, time.Local),
		},
		{
			Summary: "Tomorrow Standup",
			Start:   now.AddDate(0, 0, 1),
			End:     now.AddDate(0, 0, 1).Add(time.Hour),
		},
		{
			Summary: "Sunday Review",
			Start:   now.AddDate(0, 0, 6),
			End:     now.AddDate(0, 0, 6).Add(time.Hour),
		},
		{
			Summary: "Too Far",
			Start:   now.AddDate(0, 0, 7),
			End:     now.AddDate(0, 0, 7).Add(time.Hour),
		},
	})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	updated := next.(Model)
	out := updated.View()
	for _, want := range []string{"╭", "Schedule for the next week", "in 3 hours", "this evening", "tomorrow", "Sunday", "Planning"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected event peek to include %q, got %q", want, out)
		}
	}
	for _, notWant := range []string{"Finished", "Too Far"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("expected event peek to exclude %q, got %q", notWant, out)
		}
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	updated = next.(Model)
	if out := updated.View(); strings.Contains(out, "Planning") {
		t.Fatalf("expected second e key to close event peek, got %q", out)
	}
}

func TestEventPeekClosesOnEscape(t *testing.T) {
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.Local)
	cfg := config.Default()
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	m.width = 90
	m.height = 24
	m.now = now
	setTestCalendarEvents(&m, []ics.Event{
		{
			Summary: "Planning",
			Start:   now.Add(time.Hour),
			End:     now.Add(2 * time.Hour),
		},
	})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	updated := next.(Model)
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated = next.(Model)
	if updated.eventPeek {
		t.Fatal("expected esc to close event peek")
	}
	if out := updated.View(); strings.Contains(out, "Planning") {
		t.Fatalf("expected closed event peek to hide event, got %q", out)
	}
}

func TestEventPeekOnlyRendersEventsThatFit(t *testing.T) {
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.Local)
	cfg := config.Default()
	addTestCalendarSource(&cfg)
	m := New(cfg, config.NewManager("", true))
	m.width = 44
	m.height = 8
	m.now = now
	setTestCalendarEvents(&m, []ics.Event{
		{
			Summary: "Alpha Bravo Charlie Delta Echo Foxtrot Golf Hotel",
			Start:   now.Add(time.Hour),
			End:     now.Add(2 * time.Hour),
		},
		{
			Summary: "Second",
			Start:   now.Add(2 * time.Hour),
			End:     now.Add(3 * time.Hour),
		},
		{
			Summary: "Third",
			Start:   now.Add(3 * time.Hour),
			End:     now.Add(4 * time.Hour),
		},
	})

	view := m.eventPeekView(testStyles(), theme.Lookup("tokyo-night"))
	if lines := strings.Split(view, "\n"); len(lines) > m.height {
		t.Fatalf("expected peek modal height <= %d, got %d in %q", m.height, len(lines), view)
	}
	if !strings.Contains(view, "Second") {
		t.Fatalf("expected second event to fit, got %q", view)
	}
	if strings.Contains(view, "Third") {
		t.Fatalf("expected third event not to fit, got %q", view)
	}
}

func TestEventPeekTimeLabelUsesDisplayTimezone(t *testing.T) {
	loc := time.FixedZone("Display", 10*60*60)
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, loc)
	event := ics.Event{
		Summary: "Dinner",
		Start:   time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC),
	}

	if got := eventPeekTimeLabel(event, now); got != "this evening 6pm" {
		t.Fatalf("expected event at 18:00 display time to be this evening 6pm, got %q", got)
	}
}

func TestEventPeekTimeLabelAddsExactAndFlooredApproximateTime(t *testing.T) {
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.Local)
	exact := ics.Event{
		Summary: "Breakfast",
		Start:   time.Date(2026, 5, 5, 8, 0, 0, 0, time.Local),
		End:     time.Date(2026, 5, 5, 9, 0, 0, 0, time.Local),
	}
	approx := ics.Event{
		Summary: "Review",
		Start:   time.Date(2026, 5, 5, 15, 45, 0, 0, time.Local),
		End:     time.Date(2026, 5, 5, 16, 45, 0, 0, time.Local),
	}

	if got := eventPeekTimeLabel(exact, now); got != "tomorrow 8am" {
		t.Fatalf("expected exact hour label, got %q", got)
	}
	if got := eventPeekTimeLabel(approx, now); got != "tomorrow ~3pm" {
		t.Fatalf("expected floored approximate hour label, got %q", got)
	}
}

func TestWrapCellsCapsLongTitlesAtTwoLines(t *testing.T) {
	lines := wrapCells("Alpha Bravo Charlie Delta Echo Foxtrot Golf Hotel India", 18, 2)
	if len(lines) != 2 {
		t.Fatalf("expected two wrapped lines, got %d: %+v", len(lines), lines)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 18 {
			t.Fatalf("expected line width <= 18, got %d in %q", lipgloss.Width(line), line)
		}
	}
	if !strings.HasSuffix(lines[1], "...") {
		t.Fatalf("expected truncated final line, got %+v", lines)
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

func TestCalendarSplitModeRendersAllDayRowsBelowTheirSource(t *testing.T) {
	workURL := "https://example.com/work.ics"
	homeURL := "https://example.com/home.ics"
	holidayURL := "https://example.com/holiday.ics"
	now := time.Date(2026, 6, 17, 9, 7, 0, 0, time.Local)
	cfg := config.Default()
	cfg.Display.SecondsStyle = "hidden"
	cfg.Calendar.ShowProgress = true
	cfg.Calendar.Mode = "split"
	cfg.Calendar.Sources = []config.CalendarSourceConfig{
		{URL: workURL},
		{URL: homeURL},
		{URL: holidayURL},
	}
	cfg.UI.Emoji = true
	m := New(cfg, config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.now = now
	m.calendarEvents = map[string][]ics.Event{
		workURL: {
			{
				Summary: "Introduction to Laser Cutting",
				Start:   time.Date(2026, 6, 17, 9, 0, 0, 0, time.Local),
				End:     time.Date(2026, 6, 17, 9, 30, 0, 0, time.Local),
			},
		},
		homeURL: {
			{
				Summary: "Mum's Birthday",
				Start:   time.Date(2026, 6, 17, 0, 0, 0, 0, time.Local),
				End:     time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local),
				AllDay:  true,
			},
			{
				Summary: "General Waste",
				Start:   time.Date(2026, 6, 17, 22, 0, 0, 0, time.Local),
				End:     time.Date(2026, 6, 17, 23, 0, 0, 0, time.Local),
			},
		},
		holidayURL: {
			{
				Summary: "Opto test",
				Start:   time.Date(2026, 6, 22, 14, 0, 0, 0, time.Local),
				End:     time.Date(2026, 6, 22, 15, 0, 0, 0, time.Local),
			},
			{
				Summary: "Foobar Public Holiday",
				Start:   time.Date(2026, 6, 17, 0, 0, 0, 0, time.Local),
				End:     time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local),
				AllDay:  true,
			},
		},
	}

	calendar := m.calendarProgressView(now, 80)
	expected := []string{
		"Introduction to Laser",
		"General Waste",
		"📌 All day: Mum's Birthday",
		"Opto test",
		"📌 All day: Foobar Public Holiday",
	}
	previous := -1
	for _, label := range expected {
		index := strings.Index(calendar, label)
		if index < 0 {
			t.Fatalf("expected split calendar to include %q, got %q", label, calendar)
		}
		if index < previous {
			t.Fatalf("expected split calendar labels in source order %v, got %q", expected, calendar)
		}
		previous = index
	}
}

func TestCalendarMergedModeRendersAllDayRowsBelowProgress(t *testing.T) {
	workURL := "https://example.com/work.ics"
	homeURL := "https://example.com/home.ics"
	now := time.Date(2026, 6, 17, 9, 7, 0, 0, time.Local)
	cfg := config.Default()
	cfg.Display.SecondsStyle = "hidden"
	cfg.Calendar.ShowProgress = true
	cfg.Calendar.Mode = "merged"
	cfg.Calendar.Sources = []config.CalendarSourceConfig{
		{URL: workURL},
		{URL: homeURL},
	}
	cfg.UI.Emoji = true
	m := New(cfg, config.NewManager("", true))
	m.width = 100
	m.height = 30
	m.now = now
	m.calendarEvents = map[string][]ics.Event{
		workURL: {
			{
				Summary: "Mum's Birthday",
				Start:   time.Date(2026, 6, 17, 0, 0, 0, 0, time.Local),
				End:     time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local),
				AllDay:  true,
			},
			{
				Summary: "Introduction to Laser Cutting",
				Start:   time.Date(2026, 6, 17, 9, 0, 0, 0, time.Local),
				End:     time.Date(2026, 6, 17, 9, 30, 0, 0, time.Local),
			},
		},
		homeURL: {
			{
				Summary: "Foobar Public Holiday",
				Start:   time.Date(2026, 6, 17, 0, 0, 0, 0, time.Local),
				End:     time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local),
				AllDay:  true,
			},
		},
	}

	calendar := m.calendarProgressView(now, 80)
	expected := []string{
		"Introduction to Laser",
		"📌 All day: Mum's Birthday",
		"📌 All day: Foobar Public Holiday",
	}
	previous := -1
	for _, label := range expected {
		index := strings.Index(calendar, label)
		if index < 0 {
			t.Fatalf("expected merged calendar to include %q, got %q", label, calendar)
		}
		if index < previous {
			t.Fatalf("expected merged calendar labels below progress in source order %v, got %q", expected, calendar)
		}
		previous = index
	}
	if !strings.Contains(calendar, "\n\n📌 All day") {
		t.Fatalf("expected blank line before all-day rows, got %q", calendar)
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
