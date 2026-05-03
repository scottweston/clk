package app

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"clk/internal/config"
	"clk/internal/render"
	"clk/internal/theme"
)

type Model struct {
	cfg              config.Config
	manager          config.Manager
	width            int
	height           int
	now              time.Time
	settings         bool
	helpOpen         bool
	cursor           int
	scrollOffset     int
	dayScrollOffset  int
	workdays         bool
	dayCursor        int
	fontPicker       bool
	fontPickerKind   string
	fontQuery        string
	fontCursor       int
	fontScrollOffset int
	saveError        string
	help             help.Model
	progress         progress.Model
	keys             keyMap
	themeName        string
	accentName       string
}

type tickMsg time.Time

const progressEmptyCharacter = '⣿'

func New(cfg config.Config, manager config.Manager) Model {
	cfg.Normalize()
	return Model{
		cfg:        cfg,
		manager:    manager,
		now:        time.Now(),
		help:       help.New(),
		progress:   newProgressModel(cfg),
		keys:       keys,
		themeName:  cfg.Theme.Name,
		accentName: cfg.Theme.Accent,
	}
}

func (m Model) Init() tea.Cmd {
	return tick()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) View() string {
	p := theme.Lookup(m.cfg.Theme.Name)
	styles := theme.Styles(p, m.cfg.Theme.Accent)
	content := m.clockView(styles, p.Background)

	if m.settings {
		content = joinVerticalWithBackground(lipgloss.Center, p.Background, content, "", m.settingsView(styles))
	}
	if m.helpOpen {
		content = joinVerticalWithBackground(lipgloss.Center, p.Background, content, "", styles.Panel.Render(m.help.View(m.keys)))
	}
	if m.saveError != "" {
		content = joinVerticalWithBackground(lipgloss.Center, p.Background, content, styles.Error.Render(m.saveError))
	}

	if m.width <= 0 || m.height <= 0 {
		return content
	}

	return lipgloss.Place(
		m.width,
		m.height,
		horizontalPosition(m.cfg.Display.Alignment),
		lipgloss.Center,
		content,
		lipgloss.WithWhitespaceBackground(lipgloss.Color(p.Background)),
	)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.settings && m.fontPicker:
		m.handleFontPickerKey(msg)
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.helpOpen = !m.helpOpen
	case key.Matches(msg, m.keys.Settings):
		m.settings = !m.settings
		m.helpOpen = false
	case key.Matches(msg, m.keys.Back):
		if m.helpOpen {
			m.helpOpen = false
		} else if m.workdays {
			m.workdays = false
		} else if m.settings {
			m.settings = false
		}
	case m.settings && m.workdays && key.Matches(msg, m.keys.Up):
		if m.dayCursor > 0 {
			m.dayCursor--
		}
		m.adjustDayScroll()
	case m.settings && m.workdays && key.Matches(msg, m.keys.Down):
		if m.dayCursor < len(config.WorkdayNames)-1 {
			m.dayCursor++
		}
		m.adjustDayScroll()
	case m.settings && m.workdays && key.Matches(msg, m.keys.Choose):
		m.toggleWorkday(config.WorkdayNames[m.dayCursor])
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		m.adjustScroll(len(m.settingItems()))
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.settingItems())-1 {
			m.cursor++
		}
		m.adjustScroll(len(m.settingItems()))
	case m.settings && !m.workdays && msg.String() == "/":
		m.openFontPicker()
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Left):
		m.changeSetting(-1)
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Right):
		m.changeSetting(1)
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Choose):
		m.changeSetting(1)
	}
	return m, nil
}

func (m *Model) handleFontPickerKey(msg tea.KeyMsg) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.fontPicker = false
	case key.Matches(msg, m.keys.Up):
		if m.fontCursor > 0 {
			m.fontCursor--
		}
		m.adjustFontScroll()
	case key.Matches(msg, m.keys.Down):
		if m.fontCursor < len(m.filteredFontChoices())-1 {
			m.fontCursor++
		}
		m.adjustFontScroll()
	case key.Matches(msg, m.keys.Choose):
		m.chooseFont()
	case msg.Type == tea.KeyBackspace || msg.Type == tea.KeyCtrlH:
		if m.fontQuery != "" {
			runes := []rune(m.fontQuery)
			m.fontQuery = string(runes[:len(runes)-1])
			m.fontCursor = 0
			m.fontScrollOffset = 0
		}
	case msg.Type == tea.KeyRunes:
		m.fontQuery += string(msg.Runes)
		m.fontCursor = 0
		m.fontScrollOffset = 0
	}
}

func (m *Model) openFontPicker() {
	items := m.settingItems()
	if m.cursor < 0 || m.cursor >= len(items) {
		return
	}
	switch items[m.cursor].label {
	case "Figlet font":
		m.fontPickerKind = "figlet"
	case "Toilet font":
		m.fontPickerKind = "toilet"
	default:
		return
	}
	m.fontPicker = true
	m.fontQuery = ""
	m.fontCursor = 0
	m.fontScrollOffset = 0
	m.alignFontCursorToCurrent()
}

func (m *Model) changeSetting(delta int) {
	items := m.settingItems()
	if m.cursor < 0 || m.cursor >= len(items) {
		return
	}
	if items[m.cursor].submenu == "workdays" {
		m.workdays = true
		return
	}
	items[m.cursor].change(delta, &m.cfg)
	m.cfg.Normalize()
	m.updateProgressColors()
	if err := m.manager.Save(m.cfg); err != nil {
		m.saveError = fmt.Sprintf("config save failed: %v", err)
		return
	}
	m.saveError = ""
}

func (m *Model) toggleWorkday(day string) {
	if containsString(m.cfg.Workday.Days, day) {
		if len(m.cfg.Workday.Days) <= 1 {
			return
		}
		m.cfg.Workday.Days = removeString(m.cfg.Workday.Days, day)
	} else {
		m.cfg.Workday.Days = append(m.cfg.Workday.Days, day)
	}
	m.cfg.Normalize()
	if err := m.manager.Save(m.cfg); err != nil {
		m.saveError = fmt.Sprintf("config save failed: %v", err)
		return
	}
	m.saveError = ""
}

func newProgressModel(cfg config.Config) progress.Model {
	palette := theme.Lookup(cfg.Theme.Name)
	accent := theme.Accent(palette, cfg.Theme.Accent)
	bar := progress.New(
		progress.WithGradient(accent, palette.Warning),
		progress.WithFillCharacters('█', progressEmptyCharacter),
	)
	bar.EmptyColor = halfBrightnessHex(accent)
	bar.PercentageStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Foreground)).
		Background(lipgloss.Color(palette.Background))
	return bar
}

func halfBrightnessHex(color string) string {
	if len(color) != 7 || color[0] != '#' {
		return color
	}
	r, err := strconv.ParseUint(color[1:3], 16, 8)
	if err != nil {
		return color
	}
	g, err := strconv.ParseUint(color[3:5], 16, 8)
	if err != nil {
		return color
	}
	b, err := strconv.ParseUint(color[5:7], 16, 8)
	if err != nil {
		return color
	}
	return fmt.Sprintf("#%02x%02x%02x", r/2, g/2, b/2)
}

func (m *Model) updateProgressColors() {
	if m.themeName == m.cfg.Theme.Name && m.accentName == m.cfg.Theme.Accent {
		return
	}
	m.themeName = m.cfg.Theme.Name
	m.accentName = m.cfg.Theme.Accent
	m.progress = newProgressModel(m.cfg)
}

func (m Model) clockView(styles theme.Stylesheet, background string) string {
	now := m.displayTime()
	value := m.clockText(now)
	clockArt := render.ClockStyled(render.ClockOptions{
		Value:      value,
		Style:      m.cfg.Display.DigitStyle,
		NerdFont:   m.cfg.UI.NerdFont,
		FigletFont: m.cfg.Display.FigletFont,
		ToiletFont: m.cfg.Display.ToiletFont,
	})
	clock := styles.Clock.Render(clockArt)

	lines := []string{clock}
	if m.cfg.Time.ShowDate {
		lines = append(lines, "")
		lines = append(lines, styles.Date.Render(now.Format("Monday, 02 Jan 2006")))
	}
	secondsWidth := m.progressWidth(clockArt)
	seconds := render.SecondsStyled(render.SecondsOptions{
		Time:            now,
		Style:           m.cfg.Display.SecondsStyle,
		Width:           secondsWidth,
		NerdFont:        m.cfg.UI.NerdFont,
		Background:      background,
		Accent:          theme.Accent(theme.Lookup(m.cfg.Theme.Name), m.cfg.Theme.Accent),
		Foreground:      theme.Lookup(m.cfg.Theme.Name).Foreground,
		Muted:           theme.Lookup(m.cfg.Theme.Name).Muted,
		Progress:        m.progressView,
		WorkdayProgress: m.workdayProgressView,
		Workday: render.WorkdayOptions{
			StartTime: m.cfg.Workday.StartTime,
			EndTime:   m.cfg.Workday.EndTime,
			Days:      m.cfg.Workday.Days,
		},
	})
	if seconds != "" {
		lines = append(lines, "")
		lines = append(lines, styles.Seconds.Render(seconds))
	}
	return joinVerticalWithBackground(lipgloss.Center, background, lines...)
}

func (m Model) progressWidth(clockArt string) int {
	width := lipgloss.Width(clockArt)
	cap1 := int(float64(m.width) * 0.8)
	cap2 := int(float64(width) * 1.5)
	if cap1 < cap2 {
		cap2 = cap1
	}
	if cap2 < width {
		cap2 = width
	}
	return cap2
}

func (m Model) progressView(percent float64, width int) string {
	bar := m.progress
	bar.Width = width
	background := progressEmptyBackgroundColor(m.cfg)
	view := lipgloss.NewStyle().
		Background(lipgloss.Color(background)).
		Render(bar.ViewAs(percent))
	return styleProgressEmptyCells(view, bar.EmptyColor, background)
}

func (m Model) workdayProgressView(percent float64, width int, direction render.ProgressDirection) string {
	view := m.progressView(percent, width)
	if direction != render.ProgressDirectionUp && direction != render.ProgressDirectionDown {
		return view
	}

	bar := m.progress
	bar.Width = width
	barCells := progressBarCellCount(bar, percent)
	filledCells := int(math.Round(clampPercent(percent) * float64(barCells)))
	if filledCells <= 0 || filledCells >= barCells {
		return view
	}

	emptyBackground := progressEmptyBackgroundColor(m.cfg)
	boundaryCell := filledCells - 1
	if direction == render.ProgressDirectionDown {
		boundaryCell = 0
	}
	boundaryColor := progressFillColor(m.cfg, barCells, boundaryCell)
	marker := workdayDirectionMarker(direction, boundaryColor, emptyBackground)
	if direction == render.ProgressDirectionDown {
		return strings.Replace(view, string(bar.Full), marker, 1)
	}
	emptyCell := styledProgressEmptyCell(bar.EmptyColor, emptyBackground)
	return strings.Replace(view, emptyCell, marker, 1)
}

func progressBarCellCount(bar progress.Model, percent float64) int {
	percentView := bar.PercentageStyle.
		Inline(true).
		Render(fmt.Sprintf(bar.PercentFormat, clampPercent(percent)*100))
	return max(0, bar.Width-lipgloss.Width(percentView))
}

func progressFillColor(cfg config.Config, barCells, cell int) string {
	palette := theme.Lookup(cfg.Theme.Name)
	accent := theme.Accent(palette, cfg.Theme.Accent)
	if barCells <= 1 {
		return accent
	}

	start, err := colorful.Hex(accent)
	if err != nil {
		return accent
	}
	end, err := colorful.Hex(palette.Warning)
	if err != nil {
		return accent
	}
	position := float64(cell) / float64(barCells-1)
	return start.BlendLuv(end, position).Hex()
}

func workdayDirectionMarker(direction render.ProgressDirection, boundaryColor, emptyBackground string) string {
	style := lipgloss.NewStyle()
	switch direction {
	case render.ProgressDirectionUp:
		return style.
			Foreground(lipgloss.Color(boundaryColor)).
			Background(lipgloss.Color(emptyBackground)).
			Render("")
	case render.ProgressDirectionDown:
		return style.
			Foreground(lipgloss.Color(boundaryColor)).
			Background(lipgloss.Color(emptyBackground)).
			Render("")
	default:
		return ""
	}
}

func clampPercent(percent float64) float64 {
	if percent < 0 {
		return 0
	}
	if percent > 1 {
		return 1
	}
	return percent
}

func progressEmptyBackgroundColor(cfg config.Config) string {
	palette := theme.Lookup(cfg.Theme.Name)
	switch cfg.Display.ProgressEmptyBackground {
	case "accent":
		return theme.Accent(palette, cfg.Theme.Accent)
	case "muted":
		return palette.Muted
	case "foreground":
		return palette.Foreground
	case "warning":
		return palette.Warning
	default:
		return palette.Background
	}
}

func styleProgressEmptyCells(view, foreground, background string) string {
	return strings.ReplaceAll(view, string(progressEmptyCharacter), styledProgressEmptyCell(foreground, background))
}

func styledProgressEmptyCell(foreground, background string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(foreground)).
		Background(lipgloss.Color(background)).
		Render(string(progressEmptyCharacter))
}

func (m Model) displayTime() time.Time {
	if m.cfg.Time.Format == "utc" {
		return m.now.UTC()
	}
	if m.cfg.Time.Timezone != "" && m.cfg.Time.Timezone != "Local" {
		if loc, err := time.LoadLocation(m.cfg.Time.Timezone); err == nil {
			return m.now.In(loc)
		}
	}
	return m.now.Local()
}

func (m *Model) adjustScroll(total int) {
	maxVisible := (m.height + 2) / 3
	if maxVisible < 1 {
		maxVisible = 1
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset+maxVisible > total {
		m.scrollOffset = total - maxVisible
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) adjustDayScroll() {
	maxVisible := (m.height + 2) / 3
	if maxVisible < 1 {
		maxVisible = 1
	}
	if m.dayCursor < m.dayScrollOffset {
		m.dayScrollOffset = m.dayCursor
	}
	if m.dayCursor >= m.dayScrollOffset+maxVisible {
		m.dayScrollOffset = m.dayCursor - maxVisible + 1
	}
	if m.dayScrollOffset < 0 {
		m.dayScrollOffset = 0
	}
	if m.dayScrollOffset+maxVisible > len(config.WorkdayNames) {
		m.dayScrollOffset = len(config.WorkdayNames) - maxVisible
	}
	if m.dayScrollOffset < 0 {
		m.dayScrollOffset = 0
	}
}

func (m *Model) adjustFontScroll() {
	choices := m.filteredFontChoices()
	if len(choices) == 0 {
		m.fontCursor = 0
		m.fontScrollOffset = 0
		return
	}
	if m.fontCursor >= len(choices) {
		m.fontCursor = len(choices) - 1
	}
	maxVisible := (m.height + 2) / 3
	if maxVisible < 3 {
		maxVisible = 3
	}
	if m.fontCursor < m.fontScrollOffset {
		m.fontScrollOffset = m.fontCursor
	}
	if m.fontCursor >= m.fontScrollOffset+maxVisible {
		m.fontScrollOffset = m.fontCursor - maxVisible + 1
	}
	if m.fontScrollOffset < 0 {
		m.fontScrollOffset = 0
	}
	if m.fontScrollOffset+maxVisible > len(choices) {
		m.fontScrollOffset = len(choices) - maxVisible
	}
	if m.fontScrollOffset < 0 {
		m.fontScrollOffset = 0
	}
}

func (m Model) clockText(now time.Time) string {
	timeFormat := "15:04"
	if m.cfg.Display.InlineSeconds {
		timeFormat = "15:04:05"
	}
	if m.cfg.Display.BlinkSeparator && now.Second()%2 == 1 {
		timeFormat = strings.ReplaceAll(timeFormat, ":", string(render.HiddenSeparator))
	}
	if m.cfg.Time.Format == "12h" {
		if m.cfg.Display.InlineSeconds {
			timeFormat = "03:04:05 PM"
			if m.cfg.Display.BlinkSeparator && now.Second()%2 == 1 {
				timeFormat = strings.ReplaceAll(timeFormat, ":", string(render.HiddenSeparator))
			}
			return strings.ToUpper(now.Format(timeFormat))
		}
		timeFormat = "03:04 PM"
		if m.cfg.Display.BlinkSeparator && now.Second()%2 == 1 {
			timeFormat = strings.ReplaceAll(timeFormat, ":", string(render.HiddenSeparator))
		}
		return strings.ToUpper(now.Format(timeFormat))
	}
	return now.Format(timeFormat)
}

func (m Model) settingsView(styles theme.Stylesheet) string {
	if m.fontPicker {
		return m.fontPickerView(styles)
	}
	if m.workdays {
		return m.workdaysView(styles)
	}

	items := m.settingItems()
	maxVisible := (m.height + 2) / 3
	if maxVisible < 1 {
		maxVisible = 1
	}
	start := m.scrollOffset
	end := start + maxVisible
	if end > len(items) {
		end = len(items)
	}

	rows := []string{styles.Muted.Render("Settings")}
	for i := start; i < end; i++ {
		row := fmt.Sprintf("%-16s %s", items[i].label, items[i].value(m.cfg))
		if i == m.cursor {
			row = styles.Selected.Render(" " + row + " ")
		} else {
			row = " " + row + " "
		}
		rows = append(rows, row)
	}
	if len(items) > maxVisible {
		remaining := len(items) - end
		if remaining > 0 {
			rows = append(rows, styles.Muted.Render(fmt.Sprintf("... %d more ▼", remaining)))
		}
	}
	rows = append(rows, "", styles.Muted.Render("enter/right changes • esc closes • autosaves"))
	return styles.Panel.Render(strings.Join(rows, "\n"))
}

func (m Model) fontPickerView(styles theme.Stylesheet) string {
	choices := m.filteredFontChoices()
	maxVisible := (m.height + 2) / 3
	if maxVisible < 3 {
		maxVisible = 3
	}
	start := m.fontScrollOffset
	end := start + maxVisible
	if end > len(choices) {
		end = len(choices)
	}

	title := "Figlet Fonts"
	if m.fontPickerKind == "toilet" {
		title = "Toilet Fonts"
	}
	rows := []string{
		styles.Muted.Render(title),
		fmt.Sprintf("/ %s", m.fontQuery),
	}
	for i := start; i < end; i++ {
		row := fontChoiceLabel(choices[i])
		if i == m.fontCursor {
			row = styles.Selected.Render(" " + row + " ")
		} else {
			row = " " + row + " "
		}
		rows = append(rows, row)
	}
	if len(choices) == 0 {
		rows = append(rows, styles.Muted.Render(" no matches "))
	}
	if len(choices) > maxVisible {
		remaining := len(choices) - end
		if remaining > 0 {
			rows = append(rows, styles.Muted.Render(fmt.Sprintf("... %d more ▼", remaining)))
		}
	}
	rows = append(rows, "", styles.Muted.Render("type filters • enter selects • esc returns"))
	return styles.Panel.Render(strings.Join(rows, "\n"))
}

func (m Model) workdaysView(styles theme.Stylesheet) string {
	maxVisible := (m.height + 2) / 3
	if maxVisible < 1 {
		maxVisible = 1
	}
	start := m.dayScrollOffset
	end := start + maxVisible
	if end > len(config.WorkdayNames) {
		end = len(config.WorkdayNames)
	}

	rows := []string{styles.Muted.Render("Work Days")}
	for i := start; i < end; i++ {
		box := "[ ]"
		if containsString(m.cfg.Workday.Days, config.WorkdayNames[i]) {
			box = "[x]"
		}
		row := fmt.Sprintf("%s %-9s", box, workdayLabel(config.WorkdayNames[i]))
		if i == m.dayCursor {
			row = styles.Selected.Render(" " + row + " ")
		} else {
			row = " " + row + " "
		}
		rows = append(rows, row)
	}
	if len(config.WorkdayNames) > maxVisible {
		remaining := len(config.WorkdayNames) - end
		if remaining > 0 {
			rows = append(rows, styles.Muted.Render(fmt.Sprintf("... %d more ▼", remaining)))
		}
	}
	rows = append(rows, "", styles.Muted.Render("enter/space toggles • esc returns • autosaves"))
	return styles.Panel.Render(strings.Join(rows, "\n"))
}

type settingItem struct {
	label   string
	value   func(config.Config) string
	change  func(delta int, cfg *config.Config)
	submenu string
}

func (m Model) settingItems() []settingItem {
	items := []settingItem{
		cycleItem("Theme", func(c config.Config) string { return c.Theme.Name }, func(v string, c *config.Config) { c.Theme.Name = v }, theme.Names()),
		cycleItem("Accent", func(c config.Config) string { return c.Theme.Accent }, func(v string, c *config.Config) { c.Theme.Accent = v }, config.Accents),
		cycleItem("Time format", func(c config.Config) string { return c.Time.Format }, func(v string, c *config.Config) { c.Time.Format = v }, config.TimeFormats),
		toggleItem("Show date", func(c config.Config) bool { return c.Time.ShowDate }, func(v bool, c *config.Config) { c.Time.ShowDate = v }),
		cycleItem("Digit style", func(c config.Config) string { return c.Display.DigitStyle }, func(v string, c *config.Config) { c.Display.DigitStyle = v }, config.DigitStyles),
	}
	if m.cfg.Display.DigitStyle == "figlet" {
		items = append(items, cycleItem("Figlet font", func(c config.Config) string { return c.Display.FigletFont }, func(v string, c *config.Config) { c.Display.FigletFont = v }, config.FigletFontChoices()))
	}
	if m.cfg.Display.DigitStyle == "toilet" {
		items = append(items, cycleItem("Toilet font", func(c config.Config) string { return c.Display.ToiletFont }, func(v string, c *config.Config) { c.Display.ToiletFont = v }, config.ToiletFontChoices()))
	}
	items = append(items,
		toggleItem("Blink sep", func(c config.Config) bool { return c.Display.BlinkSeparator }, func(v bool, c *config.Config) { c.Display.BlinkSeparator = v }),
		toggleItem("Inline sec", func(c config.Config) bool { return c.Display.InlineSeconds }, func(v bool, c *config.Config) { c.Display.InlineSeconds = v }),
		cycleItem("Seconds", func(c config.Config) string { return c.Display.SecondsStyle }, func(v string, c *config.Config) { c.Display.SecondsStyle = v }, config.SecondsStyles),
		cycleItem("Bar bg", func(c config.Config) string { return c.Display.ProgressEmptyBackground }, func(v string, c *config.Config) { c.Display.ProgressEmptyBackground = v }, config.ProgressEmptyBackgrounds),
		cycleItem("Work start", func(c config.Config) string { return c.Workday.StartTime }, func(v string, c *config.Config) { c.Workday.StartTime = v }, config.TimeChoices),
		cycleItem("Work end", func(c config.Config) string { return c.Workday.EndTime }, func(v string, c *config.Config) { c.Workday.EndTime = v }, config.TimeChoices),
		submenuItem("Work days", func(c config.Config) string { return strings.Join(c.Workday.Days, ",") }, "workdays"),
		cycleItem("Alignment", func(c config.Config) string { return c.Display.Alignment }, func(v string, c *config.Config) { c.Display.Alignment = v }, config.Alignments),
		toggleItem("Nerd Font", func(c config.Config) bool { return c.UI.NerdFont }, func(v bool, c *config.Config) { c.UI.NerdFont = v }),
	)
	return items
}

func submenuItem(label string, value func(config.Config) string, submenu string) settingItem {
	return settingItem{
		label:   label,
		value:   value,
		submenu: submenu,
		change:  func(_ int, _ *config.Config) {},
	}
}

func cycleItem(label string, get func(config.Config) string, set func(string, *config.Config), values []string) settingItem {
	return settingItem{
		label: label,
		value: get,
		change: func(delta int, cfg *config.Config) {
			current := get(*cfg)
			idx := indexOf(values, current)
			if idx < 0 {
				idx = 0
			}
			idx = (idx + delta + len(values)) % len(values)
			set(values[idx], cfg)
		},
	}
}

func toggleItem(label string, get func(config.Config) bool, set func(bool, *config.Config)) settingItem {
	return settingItem{
		label: label,
		value: func(cfg config.Config) string {
			if get(cfg) {
				return "on"
			}
			return "off"
		},
		change: func(_ int, cfg *config.Config) {
			set(!get(*cfg), cfg)
		},
	}
}

func (m Model) fontChoices() []string {
	if m.fontPickerKind == "toilet" {
		return config.ToiletFontChoices()
	}
	return config.FigletFontChoices()
}

func (m Model) filteredFontChoices() []string {
	choices := m.fontChoices()
	query := strings.ToLower(strings.TrimSpace(m.fontQuery))
	if query == "" {
		return choices
	}

	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		if strings.Contains(strings.ToLower(choice), query) || strings.Contains(strings.ToLower(fontChoiceLabel(choice)), query) {
			out = append(out, choice)
		}
	}
	sortFontChoices(out, query)
	return out
}

func sortFontChoices(choices []string, query string) {
	if query == "" {
		return
	}
	isExact := func(choice string) bool {
		return strings.ToLower(choice) == query || strings.ToLower(fontChoiceLabel(choice)) == query
	}
	sort.SliceStable(choices, func(i, j int) bool {
		return isExact(choices[i]) && !isExact(choices[j])
	})
}

func (m *Model) alignFontCursorToCurrent() {
	current := m.cfg.Display.FigletFont
	if m.fontPickerKind == "toilet" {
		current = m.cfg.Display.ToiletFont
	}
	choices := m.filteredFontChoices()
	for i, choice := range choices {
		if choice == current {
			m.fontCursor = i
			m.adjustFontScroll()
			return
		}
	}
}

func (m *Model) chooseFont() {
	choices := m.filteredFontChoices()
	if len(choices) == 0 || m.fontCursor < 0 || m.fontCursor >= len(choices) {
		return
	}
	if m.fontPickerKind == "toilet" {
		m.cfg.Display.ToiletFont = choices[m.fontCursor]
	} else {
		m.cfg.Display.FigletFont = choices[m.fontCursor]
	}
	m.cfg.Normalize()
	if err := m.manager.Save(m.cfg); err != nil {
		m.saveError = fmt.Sprintf("config save failed: %v", err)
		return
	}
	m.saveError = ""
	m.fontPicker = false
}

func fontChoiceLabel(choice string) string {
	if strings.ContainsAny(choice, `/\`) {
		parts := strings.FieldsFunc(choice, func(r rune) bool {
			return r == '/' || r == '\\'
		})
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return choice
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func workdayLabel(day string) string {
	switch day {
	case "mon":
		return "Monday"
	case "tue":
		return "Tuesday"
	case "wed":
		return "Wednesday"
	case "thu":
		return "Thursday"
	case "fri":
		return "Friday"
	case "sat":
		return "Saturday"
	default:
		return "Sunday"
	}
}

func removeString(values []string, value string) []string {
	out := values[:0]
	for _, candidate := range values {
		if candidate != value {
			out = append(out, candidate)
		}
	}
	return out
}

func tick() tea.Cmd {
	return tea.Tick(125*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func horizontalPosition(alignment string) lipgloss.Position {
	switch alignment {
	case "left":
		return lipgloss.Left
	case "right":
		return lipgloss.Right
	default:
		return lipgloss.Center
	}
}

func joinVerticalWithBackground(position lipgloss.Position, background string, blocks ...string) string {
	width := 0
	for _, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			width = max(width, lipgloss.Width(line))
		}
	}
	if width == 0 {
		return ""
	}

	pad := lipgloss.NewStyle().Background(lipgloss.Color(background))
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			lineWidth := lipgloss.Width(line)
			left, right := paddingFor(position, width-lineWidth)
			out = append(out, pad.Render(strings.Repeat(" ", left))+line+pad.Render(strings.Repeat(" ", right)))
		}
	}
	return strings.Join(out, "\n")
}

func paddingFor(position lipgloss.Position, total int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	switch position {
	case lipgloss.Left:
		return 0, total
	case lipgloss.Right:
		return total, 0
	default:
		left := total / 2
		return left, total - left
	}
}

func indexOf(values []string, value string) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return -1
}

type keyMap struct {
	Quit     key.Binding
	Settings key.Binding
	Help     key.Binding
	Back     key.Binding
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Choose   key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Settings, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Settings, k.Help, k.Back, k.Quit},
		{k.Up, k.Down, k.Left, k.Right, k.Choose},
	}
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Settings: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "settings"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "previous"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "next"),
	),
	Choose: key.NewBinding(
		key.WithKeys("enter", " "),
		key.WithHelp("enter", "change"),
	),
}
