package render

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestClockRendersFiveRows(t *testing.T) {
	out := Clock("12:34", "block", false)
	lines := Lines(out)
	if len(lines) != 5 {
		t.Fatalf("expected 5 rows, got %d: %q", len(lines), out)
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("expected non-empty line in %q", out)
		}
	}
}

func TestNerdSegmentFallsBackWithoutNerdFont(t *testing.T) {
	out := Clock("1", "nerd_segment", false)
	if !strings.Contains(out, "│") {
		t.Fatalf("expected box fallback, got %q", out)
	}
}

func TestExternalStylesFallBackWhenCommandIsMissing(t *testing.T) {
	out := ClockStyled(ClockOptions{
		Value:      "12:34",
		Style:      "figlet",
		FigletFont: "standard",
	})
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected figlet style to render fallback output")
	}
}

func TestFigletStyleUsesSelectedFontViaToiletFallback(t *testing.T) {
	if _, ok := externalClock("12:34", "toilet", "slant", 1); !ok {
		t.Skip("toilet figlet-compatible rendering is not available")
	}

	standard := ClockStyled(ClockOptions{
		Value:      "12:34",
		Style:      "figlet",
		FigletFont: "standard",
	})
	slant := ClockStyled(ClockOptions{
		Value:      "12:34",
		Style:      "figlet",
		FigletFont: "slant",
	})
	if standard == slant {
		t.Fatal("expected selected figlet fonts to produce different output")
	}
}

func TestToiletStyleRendersWhenAvailable(t *testing.T) {
	if _, ok := externalClock("12:34", "toilet", "standard", 1); !ok {
		t.Skip("toilet is not available")
	}

	out := ClockStyled(ClockOptions{
		Value:      "12:34",
		Style:      "toilet",
		ToiletFont: "standard",
	})
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected toilet style output")
	}
}

func TestHiddenSeparatorKeepsExternalClockWidthStable(t *testing.T) {
	visible, ok := externalClock("13:14", "toilet", "standard", 1)
	if !ok {
		t.Skip("toilet is not available")
	}
	hidden, ok := externalClock("13"+string(HiddenSeparator)+"14", "toilet", "standard", 1)
	if !ok {
		t.Skip("toilet is not available")
	}

	assertSameLineWidths(t, visible, hidden)
}

func TestBrailleClockUsesBrailleCells(t *testing.T) {
	out := Clock("12:34", "braille", false)
	lines := Lines(out)
	if len(lines) != 2 {
		t.Fatalf("expected braille output to use 2 rows, got %d: %q", len(lines), out)
	}
	if strings.Contains(out, "█") || strings.Contains(out, "⣿") {
		t.Fatalf("expected dot-packed braille output, got %q", out)
	}
	if !containsBrailleCell(out) {
		t.Fatalf("expected braille code points in %q", out)
	}
}

func TestBrailleClockScalesByRepackingDots(t *testing.T) {
	out := ClockScaled("12:34", "braille", false, 2)
	lines := Lines(out)
	if len(lines) != 4 {
		t.Fatalf("expected doubled braille output to use 4 rows, got %d: %q", len(lines), out)
	}
	if strings.Contains(out, "█") {
		t.Fatalf("expected dot-packed doubled braille output, got %q", out)
	}
	if !containsBrailleCell(out) {
		t.Fatalf("expected braille code points in doubled output %q", out)
	}
	for _, line := range lines {
		if lipgloss.Width(line) != lipgloss.Width(lines[0]) {
			t.Fatalf("expected equal-width doubled braille lines, got %q", out)
		}
	}
}

func TestHiddenSeparatorKeepsBrailleClockWidthStable(t *testing.T) {
	visible := Clock("13:14", "braille", false)
	hidden := Clock("13"+string(HiddenSeparator)+"14", "braille", false)

	visibleLines := Lines(visible)
	hiddenLines := Lines(hidden)
	if len(visibleLines) != len(hiddenLines) {
		t.Fatalf("expected same line count, got %d and %d", len(visibleLines), len(hiddenLines))
	}
	for i := range visibleLines {
		if lipgloss.Width(visibleLines[i]) != lipgloss.Width(hiddenLines[i]) {
			t.Fatalf("expected stable width on line %d, visible %q hidden %q", i, visibleLines[i], hiddenLines[i])
		}
	}
}

func TestSecondsStyles(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 30, 0, time.Local)
	cases := []string{"numeric", "progress_bar", "bubble_progress", "ascii_circle", "braille_circle", "nerd_pulse", "pomodoro", "workday", "hidden"}
	for _, style := range cases {
		got := Seconds(now, style, 20, false)
		if style != "hidden" && got == "" {
			t.Fatalf("style %s rendered empty output", style)
		}
	}
}

func TestUnknownSecondsStyleFallsBackToProgressBar(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 30, 0, time.Local)
	if got := Seconds(now, "inline", 20, false); !strings.HasPrefix(got, "[") {
		t.Fatalf("expected unknown style fallback progress bar, got %q", got)
	}
}

func TestProgressBarBounds(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 59, int(999*time.Millisecond), time.Local)
	got := Seconds(now, "progress_bar", 4, false)
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Fatalf("expected bracketed progress bar, got %q", got)
	}
}

func TestProgressBarCanExceedSixtyColumns(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 30, 0, time.Local)
	got := Seconds(now, "progress_bar", 90, false)
	if len([]rune(got)) != 92 {
		t.Fatalf("expected 90-column bar plus brackets, got width %d in %q", len([]rune(got)), got)
	}
}

func TestBubbleProgressFallbackUsesBar(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 30, 0, time.Local)
	got := SecondsStyled(SecondsOptions{
		Time:       now,
		Style:      "bubble_progress",
		Width:      24,
		Background: "#1a1b26",
		Accent:     "#7dcfff",
		Foreground: "#c0caf5",
		Muted:      "#565f89",
	})
	if !strings.Contains(got, "============") {
		t.Fatalf("expected bubble progress bar, got %q", got)
	}
}

func TestPomodoroRendersCurrentCycle(t *testing.T) {
	focus := time.Date(2026, 5, 1, 12, 10, 0, 0, time.Local)
	if got := Seconds(focus, "pomodoro", 40, false); !strings.Contains(got, "focus") {
		t.Fatalf("expected focus cycle, got %q", got)
	}

	breakTime := time.Date(2026, 5, 1, 12, 26, 0, 0, time.Local)
	if got := Seconds(breakTime, "pomodoro", 40, false); !strings.Contains(got, "break") {
		t.Fatalf("expected break cycle, got %q", got)
	}
}

func TestWorkdayRendersBeforeDuringAfterAndOffDays(t *testing.T) {
	opts := SecondsOptions{
		Style: "workday",
		Width: 48,
		Workday: WorkdayOptions{
			StartTime: "09:00",
			EndTime:   "17:00",
			Days:      []string{"mon", "tue", "wed", "thu", "fri"},
		},
	}

	opts.Time = time.Date(2026, 5, 1, 8, 30, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "workday 00:30") || !strings.Contains(got, "[") {
		t.Fatalf("expected pre-workday with bar, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 1, 13, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "workday 04:00") || !strings.Contains(got, "[") {
		t.Fatalf("expected mid-workday with bar, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 1, 18, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "workday 63:00") || !strings.Contains(got, "=") || !strings.Contains(got, "-") {
		t.Fatalf("expected after-workday bar to drain toward next start, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 2, 13, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "off 44:00") || !strings.Contains(got, "=") || !strings.Contains(got, "-") {
		t.Fatalf("expected off day bar to drain toward next start, got %q", got)
	}
}

func TestScaleDoublesClockArt(t *testing.T) {
	got := Scale("ab\ncd", 2)
	want := "aabb\naabb\nccdd\nccdd"
	if got != want {
		t.Fatalf("expected scaled art %q, got %q", want, got)
	}
}

func containsBrailleCell(s string) bool {
	for _, r := range s {
		if r >= '\u2801' && r <= '\u28ff' {
			return true
		}
	}
	return false
}

func assertSameLineWidths(t *testing.T, a, b string) {
	t.Helper()

	aLines := Lines(a)
	bLines := Lines(b)
	if len(aLines) != len(bLines) {
		t.Fatalf("expected same line count, got %d and %d", len(aLines), len(bLines))
	}
	for i := range aLines {
		if lipgloss.Width(aLines[i]) != lipgloss.Width(bLines[i]) {
			t.Fatalf("expected stable width on line %d, visible %q hidden %q", i, aLines[i], bLines[i])
		}
	}
}
