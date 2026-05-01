package render

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

type SecondsOptions struct {
	Time       time.Time
	Style      string
	Width      int
	NerdFont   bool
	Background string
	Accent     string
	Foreground string
	Muted      string
	Progress   func(float64, int) string
	Workday    WorkdayOptions
}

type WorkdayOptions struct {
	StartTime string
	EndTime   string
	Days      []string
}

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
		return workday(now, opts.Width, opts.Progress, opts.Workday)
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
	if width < 12 {
		return 12
	}
	return width
}

func pomodoro(now time.Time, width int, progress func(float64, int) string) string {
	info := PomodoroProgress(now)
	remaining := fmt.Sprintf("%02d:%02d", int(info.Remaining.Minutes()), int(info.Remaining.Seconds())%60)
	label := info.Label + " " + remaining
	barWidth := width - utf8.RuneCountInString(label) - 2
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

func workday(now time.Time, width int, progress func(float64, int) string, opts WorkdayOptions) string {
	info := WorkdayProgress(now, opts)
	remaining := fmt.Sprintf("%02d:%02d", int(info.Remaining.Hours()), int(info.Remaining.Minutes())%60)
	label := fmt.Sprintf("%s %s", info.Label, remaining)
	barWidth := width - utf8.RuneCountInString(label) - 2
	if barWidth < 1 {
		barWidth = 1
	}
	bar := progressView(progress, info.Percent, normalizeBarWidth(barWidth))
	return label + " " + bar
}

func WorkdayProgress(now time.Time, opts WorkdayOptions) ProgressInfo {
	start, ok := parseClockMinutes(opts.StartTime)
	if !ok {
		start = 9 * 60
	}
	end, ok := parseClockMinutes(opts.EndTime)
	if !ok {
		end = 17 * 60
	}
	if end <= start {
		end = start + 1
	}

	nowMinute := now.Hour()*60 + now.Minute()
	workingDay := isConfiguredWorkday(now.Weekday(), opts.Days)
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
		}
	}

	if !workingDay {
		label = "off"
	}

	nextStart := nextWorkBoundary(now, start, opts.Days, true)
	previousEnd := previousWorkBoundary(nextStart, end, opts.Days)
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
	}
}

func nextWorkBoundary(now time.Time, minutes int, days []string, includeToday bool) time.Time {
	for offset := 0; offset <= 7; offset++ {
		day := now.AddDate(0, 0, offset)
		if !isConfiguredWorkday(day.Weekday(), days) {
			continue
		}
		boundary := timeOnDay(day, minutes)
		if (offset == 0 && !includeToday) || !boundary.After(now) {
			continue
		}
		return boundary
	}
	return timeOnDay(now.AddDate(0, 0, 1), minutes)
}

func previousWorkBoundary(now time.Time, minutes int, days []string) time.Time {
	for offset := 0; offset <= 7; offset++ {
		day := now.AddDate(0, 0, -offset)
		if !isConfiguredWorkday(day.Weekday(), days) {
			continue
		}
		boundary := timeOnDay(day, minutes)
		if boundary.Before(now) {
			return boundary
		}
	}
	return timeOnDay(now.AddDate(0, 0, -1), minutes)
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

func isConfiguredWorkday(day time.Weekday, days []string) bool {
	if len(days) == 0 {
		days = []string{"mon", "tue", "wed", "thu", "fri"}
	}
	current := weekdayName(day)
	for _, configured := range days {
		if configured == current {
			return true
		}
	}
	return false
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
