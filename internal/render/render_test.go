package render

import (
	"os"
	"path/filepath"
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

func TestLargeDigitStyleVariants(t *testing.T) {
	cases := []struct {
		style string
		rows  int
	}{
		{style: "block_2x", rows: 10},
		{style: "braille_2x", rows: 4},
		{style: "braille_4x", rows: 7},
		{style: "braille_thin_2x", rows: 4},
		{style: "braille_thin_3x", rows: 6},
		{style: "braille_thin_4x", rows: 7},
		{style: "braille_thin_cmf_2x", rows: 4},
		{style: "braille_thin_cmf_3x", rows: 6},
		{style: "braille_thin_cmf_4x", rows: 7},
		{style: "box_2x", rows: 10},
		{style: "half_block_2x", rows: 10},
	}
	for _, tc := range cases {
		out := Clock("12:34", tc.style, false)
		lines := Lines(out)
		if len(lines) != tc.rows {
			t.Fatalf("%s expected %d rows, got %d: %q", tc.style, tc.rows, len(lines), out)
		}
		for _, line := range lines {
			if lipgloss.Width(line) != lipgloss.Width(lines[0]) {
				t.Fatalf("%s expected equal-width lines, got %q", tc.style, out)
			}
		}
	}
}

func TestExternalClockIgnoresScale(t *testing.T) {
	normal, ok := externalClock("12:34", "toilet", "standard", 1)
	if !ok {
		t.Skip("toilet is not available")
	}
	scaled, ok := externalClock("12:34", "toilet", "standard", 2)
	if !ok {
		t.Skip("toilet is not available")
	}
	if normal != scaled {
		t.Fatal("expected external fonts to ignore scale")
	}
}

func TestExternalFontArgsUseDirectoryForFontPaths(t *testing.T) {
	path := filepath.Join("tmp", "figlet", "custom-clock.flf")
	got := externalFontArgs(path, "1")
	want := []string{"-d", filepath.Join("tmp", "figlet"), "-f", "custom-clock", "1"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("external font args = %#v, want %#v", got, want)
	}
}

func TestFclkClockRendersUserFont(t *testing.T) {
	font := writeTestFclkFont(t)
	out := ClockStyled(ClockOptions{
		Value:    "10:01",
		Style:    "fclk",
		FclkFont: font,
	})
	if !strings.Contains(out, "one") || !strings.Contains(out, "zero") || !strings.Contains(out, "co") {
		t.Fatalf("expected fclk glyphs, got %q", out)
	}
}

func TestHiddenSeparatorKeepsFclkClockWidthStable(t *testing.T) {
	font := writeTestFclkFont(t)
	visible := ClockStyled(ClockOptions{
		Value:    "10:01",
		Style:    "fclk",
		FclkFont: font,
	})
	hidden := ClockStyled(ClockOptions{
		Value:    "10" + string(HiddenSeparator) + "01",
		Style:    "fclk",
		FclkFont: font,
	})

	assertSameLineWidths(t, visible, hidden)
}

func writeTestFclkFont(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.fclk")
	data := strings.Join([]string{
		":: 0000 1111",
		"co zero one!",
		"lo zero one!",
		"on zero one!",
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write fclk font: %v", err)
	}
	return strings.TrimSuffix(path, filepath.Ext(path))
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
		Style:   "workday",
		Width:   48,
		Workday: testWorkdayOptions(),
	}

	opts.Time = time.Date(2026, 5, 1, 8, 30, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "☼ 00:30") || strings.Contains(got, "workday") || !strings.Contains(got, "[") {
		t.Fatalf("expected pre-workday with bar, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 1, 13, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "☼ 04:00") || strings.Contains(got, "workday") || !strings.Contains(got, "[") {
		t.Fatalf("expected mid-workday with bar, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 1, 9, 8, 30, 0, time.Local)
	friday := opts.Workday.Schedule["fri"]
	friday.EndTime = "18:00"
	opts.Workday.Schedule["fri"] = friday
	if got := SecondsStyled(opts); !strings.Contains(got, "☼ 08:52") {
		t.Fatalf("expected partial remaining minute to round up, got %q", got)
	}
	friday.EndTime = "17:00"
	opts.Workday.Schedule["fri"] = friday

	opts.Time = time.Date(2026, 5, 1, 18, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "☼ 2d15h") || !strings.Contains(got, "=") || !strings.Contains(got, "-") {
		t.Fatalf("expected after-workday bar to drain toward next start, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 2, 13, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "☾ 1d20h") || strings.Contains(got, "off") || !strings.Contains(got, "=") || !strings.Contains(got, "-") {
		t.Fatalf("expected off day bar to drain toward next start, got %q", got)
	}
}

func TestWorkdayCanRenderEmojiLabels(t *testing.T) {
	opts := SecondsOptions{
		Style:   "workday",
		Width:   48,
		Emoji:   true,
		Workday: testWorkdayOptions(),
	}

	opts.Time = time.Date(2026, 5, 1, 13, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "💼 04:00") {
		t.Fatalf("expected workday emoji, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 2, 13, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "🏖️ 1d20h") {
		t.Fatalf("expected off-day emoji, got %q", got)
	}
}

func TestLongRemainingDurationsUseDaysAndHours(t *testing.T) {
	cases := []struct {
		duration time.Duration
		want     string
	}{
		{23*time.Hour + 59*time.Minute + time.Second, "24:00"},
		{24 * time.Hour, "1d0h"},
		{92*time.Hour + 20*time.Minute, "3d20h"},
	}

	for _, tc := range cases {
		if got := formatHoursMinutesCeil(tc.duration); got != tc.want {
			t.Fatalf("formatHoursMinutesCeil(%s) = %q, want %q", tc.duration, got, tc.want)
		}
	}
}

func TestOffWorkEmojiRotatesByDate(t *testing.T) {
	cases := []struct {
		now  time.Time
		want string
	}{
		{time.Date(2026, 5, 2, 13, 0, 0, 0, time.Local), "🏖️"},
		{time.Date(2026, 5, 3, 13, 0, 0, 0, time.Local), "🌴"},
		{time.Date(2026, 5, 4, 13, 0, 0, 0, time.Local), "🏡"},
	}

	for _, tc := range cases {
		if got := offWorkEmoji(tc.now); got != tc.want {
			t.Fatalf("expected %s for %s, got %s", tc.want, tc.now.Format("2006-01-02"), got)
		}
	}
}

func TestWorkdayUsesPerDayScheduleBoundaries(t *testing.T) {
	opts := SecondsOptions{
		Style:   "workday",
		Width:   48,
		Workday: testWorkdayOptions(),
	}
	monday := opts.Workday.Schedule["mon"]
	monday.StartTime = "12:00"
	monday.EndTime = "14:00"
	opts.Workday.Schedule["mon"] = monday

	opts.Time = time.Date(2026, 5, 4, 11, 30, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "☼ 00:30") {
		t.Fatalf("expected monday start boundary, got %q", got)
	}

	opts.Time = time.Date(2026, 5, 4, 13, 0, 0, 0, time.Local)
	if got := SecondsStyled(opts); !strings.Contains(got, "☼ 01:00") {
		t.Fatalf("expected monday end boundary, got %q", got)
	}
}

func TestCalendarRendersUpcomingAndActiveEvents(t *testing.T) {
	opts := CalendarOptions{Events: []CalendarEventOptions{
		{
			Summary: "Standup",
			Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 9, 30, 0, 0, time.Local),
		},
	}}

	before := Calendar(time.Date(2026, 5, 1, 8, 30, 0, 0, time.Local), 48, nil, nil, false, opts)
	if !strings.Contains(before, "◇ Standup 00:30") || strings.Contains(before, "event") || !strings.Contains(before, "[") {
		t.Fatalf("expected upcoming calendar event, got %q", before)
	}

	during := Calendar(time.Date(2026, 5, 1, 9, 15, 0, 0, time.Local), 48, nil, nil, false, opts)
	if !strings.Contains(during, "◇ Standup 00:15") || strings.Contains(during, "event") || !strings.Contains(during, "[") {
		t.Fatalf("expected active calendar event, got %q", during)
	}
}

func TestCalendarCanRenderEmojiLabel(t *testing.T) {
	opts := CalendarOptions{
		Emoji: true,
		Events: []CalendarEventOptions{
			{
				Summary: "Standup",
				Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
				End:     time.Date(2026, 5, 1, 9, 30, 0, 0, time.Local),
			},
		},
	}

	got := Calendar(time.Date(2026, 5, 1, 8, 30, 0, 0, time.Local), 48, nil, nil, false, opts)
	if !strings.Contains(got, "📅 Standup 00:30") {
		t.Fatalf("expected calendar emoji, got %q", got)
	}
}

func TestCalendarAllDayRowsDoNotConsumeProgress(t *testing.T) {
	now := time.Date(2026, 5, 1, 8, 30, 0, 0, time.Local)
	opts := CalendarOptions{Events: []CalendarEventOptions{
		{
			Summary: "Holiday",
			Start:   time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 2, 0, 0, 0, 0, time.Local),
			AllDay:  true,
		},
		{
			Summary: "Tomorrow",
			Start:   time.Date(2026, 5, 2, 0, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 3, 0, 0, 0, 0, time.Local),
			AllDay:  true,
		},
		{
			Summary: "Standup",
			Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 9, 30, 0, 0, time.Local),
		},
	}}

	progress := Calendar(now, 48, nil, nil, false, opts)
	if !strings.Contains(progress, "Standup") || strings.Contains(progress, "Holiday") {
		t.Fatalf("expected timed event progress to ignore all-day event, got %q", progress)
	}
	rows := CalendarAllDayRows(now, 48, opts)
	if len(rows) != 1 || rows[0] != "📌 All day: Holiday" {
		t.Fatalf("expected active all-day row, got %#v", rows)
	}
}

func TestCalendarEmptySummaryDoesNotRenderEventText(t *testing.T) {
	opts := CalendarOptions{Events: []CalendarEventOptions{
		{
			Start: time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
			End:   time.Date(2026, 5, 1, 9, 30, 0, 0, time.Local),
		},
	}}

	got := Calendar(time.Date(2026, 5, 1, 8, 30, 0, 0, time.Local), 48, nil, nil, false, opts)
	if !strings.Contains(got, "◇ 00:30") || strings.Contains(got, "event") {
		t.Fatalf("expected symbol-only empty event summary, got %q", got)
	}
}

func TestCalendarTitleUsesAtMostOneThirdWidth(t *testing.T) {
	const width = 60
	opts := CalendarOptions{Events: []CalendarEventOptions{
		{
			Summary: "VeryLongMeetingTitleThatWouldEatTheProgressBar",
			Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 9, 30, 0, 0, time.Local),
		},
	}}

	got := Calendar(time.Date(2026, 5, 1, 8, 30, 0, 0, time.Local), width, nil, nil, false, opts)
	label, _, ok := strings.Cut(got, " [")
	if !ok {
		t.Fatalf("expected calendar progress bar, got %q", got)
	}
	summary := strings.TrimPrefix(label, "◇ ")
	summary = strings.TrimSuffix(summary, " 00:30")
	if lipgloss.Width(summary) > width/3 {
		t.Fatalf("expected summary width <= %d, got %d in %q", width/3, lipgloss.Width(summary), got)
	}
	if !strings.Contains(got, "…") || !strings.Contains(got, "[") {
		t.Fatalf("expected truncated event with progress bar, got %q", got)
	}
}

func TestCalendarProgressCountsDownThenUp(t *testing.T) {
	baseline := time.Date(2026, 5, 1, 8, 0, 0, 0, time.Local)
	opts := CalendarOptions{Events: []CalendarEventOptions{
		{
			Summary: "Focus",
			Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
			End:     time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local),
		},
	}, Baseline: baseline}

	before, ok := CalendarProgress(time.Date(2026, 5, 1, 8, 30, 0, 0, time.Local), opts)
	if !ok || before.Direction != ProgressDirectionDown || before.Remaining != 30*time.Minute || before.Percent != 0.5 {
		t.Fatalf("expected countdown before event, got %+v ok=%v", before, ok)
	}

	during, ok := CalendarProgress(time.Date(2026, 5, 1, 9, 30, 0, 0, time.Local), opts)
	if !ok || during.Direction != ProgressDirectionUp || during.Percent != 0.5 {
		t.Fatalf("expected countup during event, got %+v ok=%v", during, ok)
	}
}

func TestCalendarProgressCountsDownFromPreviousCompletedEvent(t *testing.T) {
	opts := CalendarOptions{
		Events: []CalendarEventOptions{
			{
				Summary: "Previous",
				Start:   time.Date(2026, 5, 1, 9, 0, 0, 0, time.Local),
				End:     time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local),
			},
			{
				Summary: "Next",
				Start:   time.Date(2026, 5, 1, 14, 0, 0, 0, time.Local),
				End:     time.Date(2026, 5, 1, 15, 0, 0, 0, time.Local),
			},
		},
		Baseline: time.Date(2026, 5, 1, 8, 0, 0, 0, time.Local),
	}

	info, ok := CalendarProgress(time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local), opts)
	if !ok || info.Label != "Next" || info.Direction != ProgressDirectionDown || info.Percent != 0.5 {
		t.Fatalf("expected countdown from previous event end, got %+v ok=%v", info, ok)
	}
}

func testWorkdayOptions() WorkdayOptions {
	return WorkdayOptions{Schedule: map[string]WorkdayDayOptions{
		"mon": {Enabled: true, StartTime: "09:00", EndTime: "17:00"},
		"tue": {Enabled: true, StartTime: "09:00", EndTime: "17:00"},
		"wed": {Enabled: true, StartTime: "09:00", EndTime: "17:00"},
		"thu": {Enabled: true, StartTime: "09:00", EndTime: "17:00"},
		"fri": {Enabled: true, StartTime: "09:00", EndTime: "17:00"},
		"sat": {Enabled: false, StartTime: "09:00", EndTime: "17:00"},
		"sun": {Enabled: false, StartTime: "09:00", EndTime: "17:00"},
	}}
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
