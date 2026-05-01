package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"clk/internal/config"
	"clk/internal/render"
	"clk/internal/theme"
)

type Model struct {
	cfg       config.Config
	manager   config.Manager
	width     int
	height    int
	now       time.Time
	settings  bool
	helpOpen  bool
	cursor    int
	workdays  bool
	dayCursor int
	saveError string
	help      help.Model
	keys      keyMap
}

type tickMsg time.Time

func New(cfg config.Config, manager config.Manager) Model {
	cfg.Normalize()
	return Model{
		cfg:     cfg,
		manager: manager,
		now:     time.Now(),
		help:    help.New(),
		keys:    keys,
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
	case m.settings && m.workdays && key.Matches(msg, m.keys.Down):
		if m.dayCursor < len(config.WorkdayNames)-1 {
			m.dayCursor++
		}
	case m.settings && m.workdays && key.Matches(msg, m.keys.Choose):
		m.toggleWorkday(config.WorkdayNames[m.dayCursor])
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.settingItems())-1 {
			m.cursor++
		}
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Left):
		m.changeSetting(-1)
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Right):
		m.changeSetting(1)
	case m.settings && !m.workdays && key.Matches(msg, m.keys.Choose):
		m.changeSetting(1)
	}
	return m, nil
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

func (m Model) clockView(styles theme.Stylesheet, background string) string {
	now := m.displayTime()
	value := m.clockText(now)
	clockArt := render.ClockStyled(render.ClockOptions{
		Value:      value,
		Style:      m.cfg.Display.DigitStyle,
		NerdFont:   m.cfg.UI.NerdFont,
		Scale:      m.clockScale(),
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
		Time:       now,
		Style:      m.cfg.Display.SecondsStyle,
		Width:      secondsWidth,
		NerdFont:   m.cfg.UI.NerdFont,
		Background: background,
		Accent:     theme.Accent(theme.Lookup(m.cfg.Theme.Name), m.cfg.Theme.Accent),
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
	viewportWidth := m.width - 8
	if viewportWidth < 0 {
		viewportWidth = 0
	}
	if viewportWidth > width {
		width = viewportWidth
	}
	return width
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

func (m Model) clockScale() int {
	if m.cfg.Display.Size == "double" {
		return 2
	}
	return 1
}

func (m Model) settingsView(styles theme.Stylesheet) string {
	if m.workdays {
		return m.workdaysView(styles)
	}

	items := m.settingItems()
	rows := []string{styles.Muted.Render("Settings")}
	for i, item := range items {
		row := fmt.Sprintf("%-16s %s", item.label, item.value(m.cfg))
		if i == m.cursor {
			row = styles.Selected.Render(" " + row + " ")
		} else {
			row = " " + row + " "
		}
		rows = append(rows, row)
	}
	rows = append(rows, "", styles.Muted.Render("enter/right changes • esc closes • autosaves"))
	return styles.Panel.Render(strings.Join(rows, "\n"))
}

func (m Model) workdaysView(styles theme.Stylesheet) string {
	rows := []string{styles.Muted.Render("Work Days")}
	for i, day := range config.WorkdayNames {
		box := "[ ]"
		if containsString(m.cfg.Workday.Days, day) {
			box = "[x]"
		}
		row := fmt.Sprintf("%s %-9s", box, workdayLabel(day))
		if i == m.dayCursor {
			row = styles.Selected.Render(" " + row + " ")
		} else {
			row = " " + row + " "
		}
		rows = append(rows, row)
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
	return []settingItem{
		cycleItem("Theme", func(c config.Config) string { return c.Theme.Name }, func(v string, c *config.Config) { c.Theme.Name = v }, theme.Names()),
		cycleItem("Accent", func(c config.Config) string { return c.Theme.Accent }, func(v string, c *config.Config) { c.Theme.Accent = v }, config.Accents),
		cycleItem("Time format", func(c config.Config) string { return c.Time.Format }, func(v string, c *config.Config) { c.Time.Format = v }, config.TimeFormats),
		toggleItem("Show date", func(c config.Config) bool { return c.Time.ShowDate }, func(v bool, c *config.Config) { c.Time.ShowDate = v }),
		cycleItem("Digit style", func(c config.Config) string { return c.Display.DigitStyle }, func(v string, c *config.Config) { c.Display.DigitStyle = v }, config.DigitStyles),
		cycleItem("Figlet font", func(c config.Config) string { return c.Display.FigletFont }, func(v string, c *config.Config) { c.Display.FigletFont = v }, config.FigletFonts),
		cycleItem("Toilet font", func(c config.Config) string { return c.Display.ToiletFont }, func(v string, c *config.Config) { c.Display.ToiletFont = v }, config.ToiletFonts),
		cycleItem("Clock size", func(c config.Config) string { return c.Display.Size }, func(v string, c *config.Config) { c.Display.Size = v }, config.Sizes),
		toggleItem("Blink sep", func(c config.Config) bool { return c.Display.BlinkSeparator }, func(v bool, c *config.Config) { c.Display.BlinkSeparator = v }),
		toggleItem("Inline sec", func(c config.Config) bool { return c.Display.InlineSeconds }, func(v bool, c *config.Config) { c.Display.InlineSeconds = v }),
		cycleItem("Seconds", func(c config.Config) string { return c.Display.SecondsStyle }, func(v string, c *config.Config) { c.Display.SecondsStyle = v }, config.SecondsStyles),
		cycleItem("Work start", func(c config.Config) string { return c.Workday.StartTime }, func(v string, c *config.Config) { c.Workday.StartTime = v }, config.TimeChoices),
		cycleItem("Work end", func(c config.Config) string { return c.Workday.EndTime }, func(v string, c *config.Config) { c.Workday.EndTime = v }, config.TimeChoices),
		submenuItem("Work days", func(c config.Config) string { return strings.Join(c.Workday.Days, ",") }, "workdays"),
		cycleItem("Alignment", func(c config.Config) string { return c.Display.Alignment }, func(v string, c *config.Config) { c.Display.Alignment = v }, config.Alignments),
		toggleItem("Nerd Font", func(c config.Config) bool { return c.UI.NerdFont }, func(v bool, c *config.Config) { c.UI.NerdFont = v }),
	}
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
