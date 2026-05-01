package render

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
)

type SecondsOptions struct {
	Time       time.Time
	Style      string
	Width      int
	NerdFont   bool
	Background string
	Accent     string
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
		return bubbleProgress(progress, opts.Width, opts.Background, opts.Accent)
	case "ascii_circle":
		return asciiCircle(progress)
	case "braille_circle":
		return brailleCircle(progress)
	case "nerd_pulse":
		if opts.NerdFont {
			return nerdPulse(progress)
		}
		return progressBar(progress, opts.Width)
	case "pomodoro":
		return pomodoro(now, opts.Width, opts.Background, opts.Accent)
	case "workday":
		return workday(now, opts.Width, opts.Background, opts.Accent, opts.Workday)
	default:
		return progressBar(progress, opts.Width)
	}
}

func bubbleProgress(value float64, width int, background, accent string) string {
	p := progress.New(
		progress.WithWidth(normalizeBarWidth(width)),
		progress.WithSolidFill(defaultColor(accent, "#7dcfff")),
		progress.WithFillCharacters('█', '█'),
	)
	p.EmptyColor = defaultColor(background, "#000000")
	p.PercentageStyle = p.PercentageStyle.Background(lipgloss.Color(defaultColor(background, "#000000")))
	return p.ViewAs(clamp(value))
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

func pomodoro(now time.Time, width int, background, accent string) string {
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
	barWidth := normalizeBarWidth(width - 16)
	bar := bubbleProgress(float64(elapsed)/float64(period), barWidth, background, accent)
	return fmt.Sprintf("%s %02d:%02d %s", label, int(remaining.Minutes()), int(remaining.Seconds())%60, bar)
}

func workday(now time.Time, width int, background, accent string, opts WorkdayOptions) string {
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
	progress := 0.0
	remaining := time.Duration(end-nowMinute)*time.Minute - time.Duration(now.Second())*time.Second - time.Duration(now.Nanosecond())
	label := "workday"

	switch {
	case !workingDay:
		progress = 0
		remaining = 0
		label = "off"
	case nowMinute < start:
		progress = 0
		remaining = time.Duration(end-start) * time.Minute
	case nowMinute >= end:
		progress = 1
		remaining = 0
	default:
		elapsed := time.Duration(nowMinute-start)*time.Minute + time.Duration(now.Second())*time.Second + time.Duration(now.Nanosecond())
		total := time.Duration(end-start) * time.Minute
		progress = float64(elapsed) / float64(total)
	}

	barWidth := normalizeBarWidth(width - 20)
	bar := bubbleProgress(progress, barWidth, background, accent)
	return fmt.Sprintf("%s %02d:%02d %s", label, int(remaining.Hours()), int(remaining.Minutes())%60, bar)
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
