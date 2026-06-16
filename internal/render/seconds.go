package render

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type SecondsOptions struct {
	Time            time.Time
	Style           string
	Width           int
	NerdFont        bool
	Emoji           bool
	Background      string
	Accent          string
	Foreground      string
	Muted           string
	Progress        func(float64, int) string
	WorkdayProgress func(float64, int, ProgressDirection) string
	Workday         WorkdayOptions
}

type WorkdayOptions struct {
	Schedule map[string]WorkdayDayOptions
}

type WorkdayDayOptions struct {
	Enabled   bool
	StartTime string
	EndTime   string
}

type CalendarOptions struct {
	Events   []CalendarEventOptions
	Baseline time.Time
	Emoji    bool
}

type CalendarEventOptions struct {
	Summary string
	Start   time.Time
	End     time.Time
	AllDay  bool
}

type ProgressDirection string

const (
	ProgressDirectionUp   ProgressDirection = "up"
	ProgressDirectionDown ProgressDirection = "down"
)

const minimumProgressWidth = 12

func Seconds(now time.Time, style string, width int, nerdFont bool) string {
	return SecondsStyled(SecondsOptions{
		Time:     now,
		Style:    style,
		Width:    width,
		NerdFont: nerdFont,
	})
}

func SecondsStyled(opts SecondsOptions) string {
	now := opts.Time
	sec := now.Second()
	progress := float64(sec*1_000_000_000+now.Nanosecond()) / float64(60*time.Second)
	switch opts.Style {
	case "hidden":
		return ""
	case "numeric":
		return fmt.Sprintf("%02d", sec)
	case "bubble_progress":
		return progressView(opts.Progress, progress, opts.Width)
	case "ascii_circle":
		return asciiCircle(progress)
	case "braille_circle":
		return brailleCircle(progress)
	case "nerd_pulse":
		if opts.NerdFont {
			return nerdPulse(progress)
		}
		return progressView(opts.Progress, progress, opts.Width)
	case "pomodoro":
		return pomodoro(now, opts.Width, opts.Progress)
	case "workday":
		return workday(now, opts.Width, opts.Progress, opts.WorkdayProgress, opts.NerdFont, opts.Emoji, opts.Workday)
	default:
		return progressView(opts.Progress, progress, opts.Width)
	}
}

func progressBar(progress float64, width int) string {
	width = normalizeBarWidth(width)
	progress = clamp(progress)
	filled := int(math.Round(progress * float64(width)))
	return "[" + strings.Repeat("=", filled) + strings.Repeat("-", width-filled) + "]"
}

func normalizeBarWidth(width int) int {
	if width < minimumProgressWidth {
		return minimumProgressWidth
	}
	return width
}

func pomodoro(now time.Time, width int, progress func(float64, int) string) string {
	info := PomodoroProgress(now)
	remaining := fmt.Sprintf("%02d:%02d", int(info.Remaining.Minutes()), int(info.Remaining.Seconds())%60)
	label := info.Label + " " + remaining
	barWidth := width - lipgloss.Width(label) - 2
	if barWidth < 1 {
		barWidth = 1
	}
	bar := progressView(progress, info.Percent, normalizeBarWidth(barWidth))
	return label + " " + bar
}

type ProgressInfo struct {
	Percent   float64
	Label     string
	Remaining time.Duration
	Direction ProgressDirection
}

func PomodoroProgress(now time.Time) ProgressInfo {
	const (
		focus     = 25 * time.Minute
		breakTime = 5 * time.Minute
		cycle     = focus + breakTime
	)

	elapsedToday := time.Duration(now.Hour())*time.Hour +
		time.Duration(now.Minute())*time.Minute +
		time.Duration(now.Second())*time.Second +
		time.Duration(now.Nanosecond())
	elapsed := elapsedToday % cycle

	label := "focus"
	period := focus
	if elapsed >= focus {
		label = "break"
		elapsed -= focus
		period = breakTime
	}

	remaining := period - elapsed
	if remaining < 0 {
		remaining = 0
	}
	return ProgressInfo{
		Percent:   float64(elapsed) / float64(period),
		Label:     label,
		Remaining: remaining,
	}
}

func workday(now time.Time, width int, progress func(float64, int) string, directionalProgress func(float64, int, ProgressDirection) string, nerdFont, emoji bool, opts WorkdayOptions) string {
	info := WorkdayProgress(now, opts)
	remaining := formatHoursMinutesCeil(info.Remaining)
	label := fmt.Sprintf("%s %s", statusSymbol(info.Label, emoji, now), remaining)
	barWidth := width - lipgloss.Width(label) - 2
	if barWidth < 1 {
		barWidth = 1
	}
	barWidth = normalizeBarWidth(barWidth)
	bar := progressView(progress, info.Percent, barWidth)
	if nerdFont && directionalProgress != nil {
		bar = directionalProgress(info.Percent, barWidth, info.Direction)
	}
	return label + " " + bar
}

func Calendar(now time.Time, width int, progress func(float64, int) string, directionalProgress func(float64, int, ProgressDirection) string, nerdFont bool, opts CalendarOptions) string {
	info, ok := CalendarProgress(now, opts)
	if !ok {
		return ""
	}
	remaining := formatHoursMinutesCeil(info.Remaining)
	label := calendarLabel(info.Label, remaining, width, opts.Emoji)
	barWidth := width - lipgloss.Width(label) - 2
	if barWidth < 1 {
		barWidth = 1
	}
	barWidth = normalizeBarWidth(barWidth)
	bar := progressView(progress, info.Percent, barWidth)
	if nerdFont && directionalProgress != nil {
		bar = directionalProgress(info.Percent, barWidth, info.Direction)
	}
	return label + " " + bar
}

func CalendarAllDayRows(now time.Time, width int, opts CalendarOptions) []string {
	events := calendarAllDayEvents(now, opts.Events)
	rows := make([]string, 0, len(events))
	for _, event := range events {
		rows = append(rows, calendarAllDayLabel(event.Summary, width))
	}
	return rows
}

func calendarLabel(summary, remaining string, width int, emoji bool) string {
	summary = strings.TrimSpace(summary)
	prefix := statusSymbol("event", emoji, time.Time{})
	base := fmt.Sprintf("%s %s", prefix, remaining)
	baseWidth := lipgloss.Width(base)
	available := width - minimumProgressWidth - 2 - baseWidth - 1
	if available < 0 {
		available = 0
	}
	if summary != "" {
		summaryLimit := width / 3
		if summaryLimit < available {
			available = summaryLimit
		}
	}
	if summary == "" || available <= 0 {
		return base
	}
	summary = truncateCells(summary, available)
	if summary == "" {
		return base
	}
	return fmt.Sprintf("%s %s %s", prefix, summary, remaining)
}

func calendarAllDayLabel(summary string, width int) string {
	summary = strings.TrimSpace(summary)
	prefix := "📌 All day"
	if summary == "" {
		return truncateCells(prefix, width)
	}
	base := prefix + ": "
	available := width - lipgloss.Width(base)
	if available <= 0 {
		return truncateCells(prefix, width)
	}
	summary = truncateCells(summary, available)
	if summary == "" {
		return truncateCells(prefix, width)
	}
	return base + summary
}

func statusSymbol(kind string, emoji bool, now time.Time) string {
	if emoji {
		switch kind {
		case "workday":
			return "💼"
		case "off":
			return offWorkEmoji(now)
		case "event":
			return "📅"
		}
	}
	switch kind {
	case "workday":
		return "☼"
	case "off":
		return "☾"
	case "event":
		return "◇"
	default:
		return kind
	}
}

func offWorkEmoji(now time.Time) string {
	choices := []string{"🏖️", "🌴", "🏡"}
	if now.IsZero() {
		return choices[0]
	}
	return choices[(now.Year()+now.YearDay())%len(choices)]
}

func truncateCells(value string, maxWidth int) string {
	value = strings.TrimSpace(value)
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= maxWidth {
		return value
	}
	ellipsis := "…"
	ellipsisWidth := lipgloss.Width(ellipsis)
	if maxWidth < ellipsisWidth {
		return ""
	}
	if maxWidth == ellipsisWidth {
		return ellipsis
	}
	limit := maxWidth - ellipsisWidth
	var b strings.Builder
	used := 0
	for _, r := range value {
		part := string(r)
		cellWidth := lipgloss.Width(part)
		if cellWidth == 0 {
			if b.Len() > 0 {
				b.WriteRune(r)
			}
			continue
		}
		if used+cellWidth > limit {
			break
		}
		b.WriteRune(r)
		used += cellWidth
	}
	if b.Len() == 0 {
		return ellipsis
	}
	return b.String() + ellipsis
}

func formatHoursMinutesCeil(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(math.Ceil(float64(d) / float64(time.Minute)))
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

func WorkdayProgress(now time.Time, opts WorkdayOptions) ProgressInfo {
	nowMinute := now.Hour()*60 + now.Minute()
	dayConfig, workingDay := workdayConfig(now.Weekday(), opts.Schedule)
	start, end := workdayBoundaries(dayConfig)
	label := "workday"

	if workingDay && nowMinute >= start && nowMinute < end {
		elapsed := time.Duration(nowMinute-start)*time.Minute + time.Duration(now.Second())*time.Second + time.Duration(now.Nanosecond())
		total := time.Duration(end-start) * time.Minute
		remaining := total - elapsed
		if remaining < 0 {
			remaining = 0
		}
		return ProgressInfo{
			Percent:   float64(elapsed) / float64(total),
			Label:     label,
			Remaining: remaining,
			Direction: ProgressDirectionUp,
		}
	}

	if !workingDay {
		label = "off"
	}

	nextStart, ok := nextWorkBoundary(now, opts.Schedule, true)
	if !ok {
		return ProgressInfo{
			Percent:   0,
			Label:     "off",
			Remaining: 0,
			Direction: ProgressDirectionDown,
		}
	}
	previousEnd, ok := previousWorkBoundary(nextStart, opts.Schedule)
	if !ok {
		return ProgressInfo{
			Percent:   0,
			Label:     label,
			Remaining: nextStart.Sub(now),
			Direction: ProgressDirectionDown,
		}
	}
	total := nextStart.Sub(previousEnd)
	remaining := nextStart.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	progress := 0.0
	if total > 0 {
		progress = float64(remaining) / float64(total)
	}

	return ProgressInfo{
		Percent:   progress,
		Label:     label,
		Remaining: remaining,
		Direction: ProgressDirectionDown,
	}
}

func CalendarProgress(now time.Time, opts CalendarOptions) (ProgressInfo, bool) {
	events := calendarEvents(opts.Events)
	if len(events) == 0 {
		return ProgressInfo{}, false
	}

	var previousEnd time.Time
	var hasPrevious bool
	for _, event := range events {
		if !event.End.After(event.Start) {
			event.End = event.Start.Add(time.Minute)
		}
		if !event.Start.After(now) && event.End.After(now) {
			elapsed := now.Sub(event.Start)
			total := event.End.Sub(event.Start)
			if elapsed < 0 {
				elapsed = 0
			}
			return ProgressInfo{
				Percent:   float64(elapsed) / float64(total),
				Label:     event.Summary,
				Remaining: event.End.Sub(now),
				Direction: ProgressDirectionUp,
			}, true
		}
		if event.Start.After(now) {
			remaining := event.Start.Sub(now)
			baseline := opts.Baseline
			if hasPrevious {
				baseline = previousEnd
			}
			total := remaining
			if !baseline.IsZero() && baseline.Before(event.Start) {
				total = event.Start.Sub(baseline)
			}
			percent := 0.0
			if total > 0 {
				percent = float64(remaining) / float64(total)
			}
			return ProgressInfo{
				Percent:   percent,
				Label:     event.Summary,
				Remaining: remaining,
				Direction: ProgressDirectionDown,
			}, true
		}
		if event.End.After(previousEnd) {
			previousEnd = event.End
			hasPrevious = true
		}
	}
	return ProgressInfo{}, false
}

func calendarEvents(events []CalendarEventOptions) []CalendarEventOptions {
	out := make([]CalendarEventOptions, 0, len(events))
	for _, event := range events {
		if event.AllDay || event.Start.IsZero() {
			continue
		}
		event.Summary = strings.TrimSpace(event.Summary)
		if !event.End.After(event.Start) {
			event.End = event.Start.Add(time.Minute)
		}
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].End.Before(out[j].End)
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

func calendarAllDayEvents(now time.Time, events []CalendarEventOptions) []CalendarEventOptions {
	out := make([]CalendarEventOptions, 0, len(events))
	for _, event := range events {
		if !event.AllDay || event.Start.IsZero() {
			continue
		}
		event.Summary = strings.TrimSpace(event.Summary)
		if !event.End.After(event.Start) {
			event.End = event.Start.Add(24 * time.Hour)
		}
		if event.Start.After(now) || !event.End.After(now) {
			continue
		}
		out = append(out, event)
	}
	return out
}

func nextWorkBoundary(now time.Time, schedule map[string]WorkdayDayOptions, includeToday bool) (time.Time, bool) {
	for offset := 0; offset <= 7; offset++ {
		day := now.AddDate(0, 0, offset)
		dayConfig, enabled := workdayConfig(day.Weekday(), schedule)
		if !enabled {
			continue
		}
		start, _ := workdayBoundaries(dayConfig)
		boundary := timeOnDay(day, start)
		if (offset == 0 && !includeToday) || !boundary.After(now) {
			continue
		}
		return boundary, true
	}
	return time.Time{}, false
}

func previousWorkBoundary(now time.Time, schedule map[string]WorkdayDayOptions) (time.Time, bool) {
	for offset := 0; offset <= 7; offset++ {
		day := now.AddDate(0, 0, -offset)
		dayConfig, enabled := workdayConfig(day.Weekday(), schedule)
		if !enabled {
			continue
		}
		_, end := workdayBoundaries(dayConfig)
		boundary := timeOnDay(day, end)
		if boundary.Before(now) {
			return boundary, true
		}
	}
	return time.Time{}, false
}

func timeOnDay(day time.Time, minutes int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), minutes/60, minutes%60, 0, 0, day.Location())
}

func ProgressPercent(now time.Time, style string, workday WorkdayOptions) (float64, bool) {
	switch style {
	case "progress_bar", "bubble_progress":
		return float64(now.Second()*1_000_000_000+now.Nanosecond()) / float64(60*time.Second), true
	case "pomodoro":
		return PomodoroProgress(now).Percent, true
	case "workday":
		return WorkdayProgress(now, workday).Percent, true
	default:
		return 0, false
	}
}

func progressView(view func(float64, int) string, percent float64, width int) string {
	if view != nil {
		return view(percent, width)
	}
	return progressBar(percent, width)
}

func parseClockMinutes(value string) (int, bool) {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

func workdayConfig(day time.Weekday, schedule map[string]WorkdayDayOptions) (WorkdayDayOptions, bool) {
	current := weekdayName(day)
	if len(schedule) == 0 {
		enabled := current != "sat" && current != "sun"
		return WorkdayDayOptions{Enabled: enabled, StartTime: "09:00", EndTime: "17:00"}, enabled
	}
	entry, ok := schedule[current]
	if !ok {
		return WorkdayDayOptions{Enabled: false, StartTime: "09:00", EndTime: "17:00"}, false
	}
	return entry, entry.Enabled
}

func workdayBoundaries(entry WorkdayDayOptions) (int, int) {
	start, ok := parseClockMinutes(entry.StartTime)
	if !ok {
		start = 9 * 60
	}
	end, ok := parseClockMinutes(entry.EndTime)
	if !ok {
		end = 17 * 60
	}
	if end <= start {
		end = start + 1
	}
	return start, end
}

func weekdayName(day time.Weekday) string {
	switch day {
	case time.Monday:
		return "mon"
	case time.Tuesday:
		return "tue"
	case time.Wednesday:
		return "wed"
	case time.Thursday:
		return "thu"
	case time.Friday:
		return "fri"
	case time.Saturday:
		return "sat"
	default:
		return "sun"
	}
}

func defaultColor(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func asciiCircle(progress float64) string {
	segments := 12
	filled := int(math.Round(clamp(progress) * float64(segments)))
	return "(" + strings.Repeat("#", filled) + strings.Repeat(".", segments-filled) + ")"
}

func brailleCircle(progress float64) string {
	segments := []rune("⠁⠂⠄⡀⢀⠠⠐⠈")
	filled := int(math.Round(clamp(progress) * float64(len(segments))))
	if filled <= 0 {
		return "⡀       "
	}
	var b strings.Builder
	for i, r := range segments {
		if i < filled {
			b.WriteRune(r)
		} else {
			b.WriteRune('·')
		}
	}
	return b.String()
}

func nerdPulse(progress float64) string {
	icons := []string{"󰝦", "󰪞", "󰪟", "󰪠", "󰪡", "󰪢", "󰪣", "󰪤", "󰪥"}
	idx := int(math.Floor(clamp(progress) * float64(len(icons))))
	if idx >= len(icons) {
		idx = len(icons) - 1
	}
	return icons[idx]
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
