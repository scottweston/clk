package ics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseReturnsUpcomingEvents(t *testing.T) {
	now := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	data := []byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Standup
DTSTART:20260606T090000Z
DTEND:20260606T093000Z
END:VEVENT
END:VCALENDAR
`)

	events, err := Parse(data, now)
	if err != nil {
		t.Fatalf("parse calendar: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %+v", events)
	}
	if events[0].Summary != "Standup" || events[0].Start.Hour() != 9 || events[0].End.Minute() != 30 {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}

func TestParseUnfoldsAndUnescapesSummary(t *testing.T) {
	now := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	data := []byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:Planning\\, roadmap\\; and\r\n notes\r\nDTSTART:20260606T090000Z\r\nDURATION:PT1H\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")

	events, err := Parse(data, now)
	if err != nil {
		t.Fatalf("parse calendar: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %+v", events)
	}
	if events[0].Summary != "Planning, roadmap; andnotes" {
		t.Fatalf("expected unfolded escaped summary, got %q", events[0].Summary)
	}
}

func TestParseMarksAllDayEvents(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.Local)
	data := []byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Holiday
DTSTART;VALUE=DATE:20260606
END:VEVENT
END:VCALENDAR
`)

	events, err := Parse(data, now)
	if err != nil {
		t.Fatalf("parse calendar: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %+v", events)
	}
	if !events[0].AllDay {
		t.Fatalf("expected all-day event flag, got %+v", events[0])
	}
	if events[0].Start.Hour() != 0 || events[0].End.Sub(events[0].Start) != 24*time.Hour {
		t.Fatalf("expected all-day date boundaries, got %+v", events[0])
	}
}

func TestParseExpandsWeeklyByDayRecurrence(t *testing.T) {
	now := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	data := []byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Office
DTSTART:20260601T100000Z
DTEND:20260601T110000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=4
END:VEVENT
END:VCALENDAR
`)

	events, err := Parse(data, now)
	if err != nil {
		t.Fatalf("parse calendar: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected remaining recurring events, got %+v", events)
	}
	if events[0].Start.Weekday() != time.Wednesday || events[1].Start.Weekday() != time.Monday || events[2].Start.Weekday() != time.Wednesday {
		t.Fatalf("expected previous wednesday plus monday and wednesday occurrences, got %+v", events)
	}
}

func TestParseKeepsMostRecentCompletedEvent(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	data := []byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Older
DTSTART:20260606T080000Z
DTEND:20260606T090000Z
END:VEVENT
BEGIN:VEVENT
SUMMARY:Recent
DTSTART:20260606T100000Z
DTEND:20260606T110000Z
END:VEVENT
BEGIN:VEVENT
SUMMARY:Next
DTSTART:20260606T150000Z
DTEND:20260606T160000Z
END:VEVENT
END:VCALENDAR
`)

	events, err := Parse(data, now)
	if err != nil {
		t.Fatalf("parse calendar: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected previous and next events, got %+v", events)
	}
	if events[0].Summary != "Recent" || events[1].Summary != "Next" {
		t.Fatalf("expected most recent completed event before next event, got %+v", events)
	}
}

func TestParseRangeReturnsEveryIntersectingEventWithoutRendererLimit(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var calendar strings.Builder
	calendar.WriteString("BEGIN:VCALENDAR\n")
	for i := 0; i < 70; i++ {
		start := from.Add(time.Duration(i) * time.Hour)
		end := start.Add(30 * time.Minute)
		_, _ = fmt.Fprintf(&calendar, "BEGIN:VEVENT\nSUMMARY:Event %02d\nDTSTART:%s\nDTEND:%s\nEND:VEVENT\n", i, start.Format("20060102T150405Z"), end.Format("20060102T150405Z"))
	}
	calendar.WriteString("END:VCALENDAR\n")

	events, err := ParseRange([]byte(calendar.String()), from, from.Add(4*24*time.Hour))
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	if len(events) != 70 {
		t.Fatalf("expected all 70 range events, got %d", len(events))
	}
}

func TestParseRangeIncludesActiveAndExcludesHalfOpenBoundaries(t *testing.T) {
	from := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	until := from.Add(time.Hour)
	data := []byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Ended
DTSTART:20260606T090000Z
DTEND:20260606T100000Z
END:VEVENT
BEGIN:VEVENT
SUMMARY:Active
DTSTART:20260606T093000Z
DTEND:20260606T103000Z
END:VEVENT
BEGIN:VEVENT
SUMMARY:At end
DTSTART:20260606T110000Z
DTEND:20260606T113000Z
END:VEVENT
END:VCALENDAR
`)

	events, err := ParseRange(data, from, until)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "Active" || !events[0].Start.Before(from) {
		t.Fatalf("unexpected half-open range events: %+v", events)
	}
}

func TestParseRangeFastForwardsOldUnboundedRecurrence(t *testing.T) {
	from := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	data := []byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Daily
DTSTART:19000101T090000Z
DURATION:PT1H
RRULE:FREQ=DAILY
END:VEVENT
END:VCALENDAR
`)

	events, err := ParseRange(data, from, from.Add(3*24*time.Hour))
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	if len(events) != 3 || events[0].Start.Year() != 2026 || events[0].Start.Day() != 6 {
		t.Fatalf("expected three current recurring events, got %+v", events)
	}
}

func TestParseRangeRejectsInvalidRange(t *testing.T) {
	now := time.Now()
	if _, err := ParseRange(nil, now, now); err == nil {
		t.Fatal("expected invalid range error")
	}
}

func TestFetchRejectsNonHTTPURL(t *testing.T) {
	_, err := Fetch(context.Background(), "file:///tmp/calendar.ics", time.Now())
	if err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("expected URL validation error, got %v", err)
	}
}

func TestFetchParsesCalendarFromHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Demo
DTSTART:20260606T090000Z
DTEND:20260606T100000Z
END:VEVENT
END:VCALENDAR
`))
	}))
	t.Cleanup(server.Close)

	events, err := Fetch(context.Background(), server.URL, time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetch calendar: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "Demo" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestFetchCachedUsesFreshCacheWithoutHTTP(t *testing.T) {
	useTestCacheDir(t)
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		_, _ = w.Write([]byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Cached Demo
DTSTART:20260606T090000Z
DTEND:20260606T100000Z
END:VEVENT
END:VCALENDAR
`))
	}))
	t.Cleanup(server.Close)

	now := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	events, err := FetchCached(context.Background(), server.URL, now, time.Hour)
	if err != nil {
		t.Fatalf("fetch calendar: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "Cached Demo" {
		t.Fatalf("unexpected fetched events: %+v", events)
	}

	events, err = FetchCached(context.Background(), server.URL, now, time.Hour)
	if err != nil {
		t.Fatalf("fetch cached calendar: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "Cached Demo" {
		t.Fatalf("unexpected cached events: %+v", events)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected fresh cache to avoid second HTTP request, got %d requests", got)
	}
}

func TestFetchCachedFallsBackToStaleCacheOnHTTPError(t *testing.T) {
	useTestCacheDir(t)
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&requests, 1) > 1 {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Offline Demo
DTSTART:20260606T090000Z
DTEND:20260606T100000Z
END:VEVENT
END:VCALENDAR
`))
	}))
	t.Cleanup(server.Close)

	now := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	events, err := FetchCached(context.Background(), server.URL, now, time.Hour)
	if err != nil {
		t.Fatalf("fetch calendar: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "Offline Demo" {
		t.Fatalf("unexpected fetched events: %+v", events)
	}
	path, err := cachePath(server.URL)
	if err != nil {
		t.Fatalf("cache path: %v", err)
	}
	stale := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatalf("mark cache stale: %v", err)
	}

	events, err = FetchCached(context.Background(), server.URL, now, time.Hour)
	if err != nil {
		t.Fatalf("fetch stale cached calendar after HTTP error: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "Offline Demo" {
		t.Fatalf("unexpected stale cached events: %+v", events)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected stale cache to try HTTP before fallback, got %d requests", got)
	}
}

func useTestCacheDir(t *testing.T) {
	t.Helper()
	old := userCacheDir
	dir := t.TempDir()
	userCacheDir = func() (string, error) {
		return dir, nil
	}
	t.Cleanup(func() {
		userCacheDir = old
	})
}
