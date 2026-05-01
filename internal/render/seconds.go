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
	if width > 60 {
		return 60
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
