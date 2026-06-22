package ics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxCalendarBytes = 4 << 20
	lookAhead        = 365 * 24 * time.Hour
	maxExpanded      = 100000
	maxReturned      = 64
)

var userCacheDir = os.UserCacheDir

type Event struct {
	Summary string
	Start   time.Time
	End     time.Time
	AllDay  bool
}

func Fetch(ctx context.Context, source string, now time.Time) ([]Event, error) {
	data, err := FetchData(ctx, source)
	if err != nil {
		return nil, err
	}
	return Parse(data, now)
}

func FetchCached(ctx context.Context, source string, now time.Time, refreshInterval time.Duration) ([]Event, error) {
	data, err := FetchCachedData(ctx, source, refreshInterval)
	if err != nil {
		return nil, err
	}
	return Parse(data, now)
}

// FetchData downloads and returns the raw data for an HTTP(S) calendar source.
func FetchData(ctx context.Context, source string) ([]byte, error) {
	return fetchData(ctx, source)
}

// FetchCachedData returns fresh cached calendar data when possible, refreshes a
// stale cache, and falls back to stale data when the refresh fails.
func FetchCachedData(ctx context.Context, source string, refreshInterval time.Duration) ([]byte, error) {
	if refreshInterval <= 0 {
		refreshInterval = 15 * time.Minute
	}
	if data, ok := readFreshCache(source, refreshInterval); ok {
		return data, nil
	}

	data, err := fetchData(ctx, source)
	if err == nil {
		_ = writeCache(source, data)
		return data, nil
	}

	if data, ok := readCache(source); ok {
		return data, nil
	}
	return nil, err
}

func fetchData(ctx context.Context, source string) ([]byte, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("calendar URL must be http or https")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create calendar request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch calendar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch calendar: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCalendarBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read calendar: %w", err)
	}
	if len(data) > maxCalendarBytes {
		return nil, fmt.Errorf("calendar is larger than %d bytes", maxCalendarBytes)
	}
	return data, nil
}

func readFreshCache(source string, maxAge time.Duration) ([]byte, bool) {
	path, err := cachePath(source)
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || time.Since(info.ModTime()) > maxAge {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil
}

func readCache(source string) ([]byte, bool) {
	path, err := cachePath(source)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil
}

func writeCache(source string, data []byte) error {
	path, err := cachePath(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".ics-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(path)
		return os.Rename(tempPath, path)
	}
	return nil
}

func cachePath(source string) (string, error) {
	dir, err := userCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(source)))
	return filepath.Join(dir, "clk", "ics", hex.EncodeToString(sum[:])+".ics"), nil
}

func Parse(data []byte, now time.Time) ([]Event, error) {
	rawEvents := parseRawEvents(data)
	candidates := make([]Event, 0)
	for _, event := range rawEvents {
		candidates = append(candidates, event.expand(now)...)
	}

	sortEvents(candidates)

	events := make([]Event, 0, len(candidates))
	var previous Event
	hasPrevious := false
	horizon := now.Add(lookAhead)
	for _, event := range candidates {
		if event.End.After(now) && event.Start.Before(horizon) {
			events = append(events, event)
			continue
		}
		if !event.End.After(now) && (!hasPrevious || event.End.After(previous.End)) {
			previous = event
			hasPrevious = true
		}
	}
	if hasPrevious {
		events = append([]Event{previous}, events...)
	}
	if len(events) > maxReturned {
		events = events[:maxReturned]
	}
	return events, nil
}

// ParseRange returns every event that intersects the half-open interval
// [from, until). Unlike Parse, it does not retain a completed baseline event or
// impose the renderer-oriented 64 event limit.
func ParseRange(data []byte, from, until time.Time) ([]Event, error) {
	if from.IsZero() || until.IsZero() || !until.After(from) {
		return nil, fmt.Errorf("invalid event range")
	}

	rawEvents := parseRawEvents(data)
	events := make([]Event, 0)
	for _, event := range rawEvents {
		events = append(events, event.expandRange(from, until)...)
	}
	sortEvents(events)
	return events, nil
}

func parseRawEvents(data []byte) []rawEvent {
	lines := unfoldLines(string(data))
	events := make([]rawEvent, 0)
	var current *rawEvent

	for _, line := range lines {
		name, params, value, ok := parseContentLine(line)
		if !ok {
			continue
		}
		switch name {
		case "BEGIN":
			if strings.EqualFold(value, "VEVENT") {
				current = &rawEvent{}
			}
		case "END":
			if strings.EqualFold(value, "VEVENT") && current != nil {
				events = append(events, *current)
				current = nil
			}
		default:
			if current != nil {
				current.set(name, params, value)
			}
		}
	}
	return events
}

func sortEvents(events []Event) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].Start.Equal(events[j].Start) {
			return events[i].End.Before(events[j].End)
		}
		return events[i].Start.Before(events[j].Start)
	})
}

type rawEvent struct {
	summary     string
	start       time.Time
	end         time.Time
	hasStart    bool
	hasEnd      bool
	allDay      bool
	duration    time.Duration
	hasDuration bool
	rrule       map[string]string
	exdates     map[string]bool
}

func (e *rawEvent) set(name string, params map[string]string, value string) {
	switch name {
	case "SUMMARY":
		e.summary = unescapeText(value)
	case "DTSTART":
		start, allDay, ok := parseICSTime(value, params, time.Local)
		if ok {
			e.start = start
			e.allDay = allDay
			e.hasStart = true
		}
	case "DTEND":
		end, _, ok := parseICSTime(value, params, time.Local)
		if ok {
			e.end = end
			e.hasEnd = true
		}
	case "DURATION":
		duration, ok := parseICSDuration(value)
		if ok {
			e.duration = duration
			e.hasDuration = true
		}
	case "RRULE":
		e.rrule = parseRule(value)
	case "EXDATE":
		for _, part := range strings.Split(value, ",") {
			exdate, _, ok := parseICSTime(strings.TrimSpace(part), params, e.start.Location())
			if !ok {
				continue
			}
			if e.exdates == nil {
				e.exdates = make(map[string]bool)
			}
			e.exdates[eventKey(exdate)] = true
		}
	}
}

func (e rawEvent) expand(now time.Time) []Event {
	if !e.hasStart {
		return nil
	}
	end := e.eventEnd()
	duration := end.Sub(e.start)
	if duration <= 0 {
		duration = time.Minute
	}

	if len(e.rrule) == 0 {
		event := Event{Summary: e.summary, Start: e.start, End: e.start.Add(duration), AllDay: e.allDay}
		if event.End.After(now.Add(-lookAhead)) && event.Start.Before(now.Add(lookAhead)) {
			return []Event{event}
		}
		return nil
	}

	return e.expandRecurring(now, duration)
}

func (e rawEvent) expandRange(from, until time.Time) []Event {
	if !e.hasStart {
		return nil
	}
	duration := e.eventEnd().Sub(e.start)
	if duration <= 0 {
		duration = time.Minute
	}

	if len(e.rrule) == 0 {
		event := Event{Summary: e.summary, Start: e.start, End: e.start.Add(duration), AllDay: e.allDay}
		if event.End.After(from) && event.Start.Before(until) {
			return []Event{event}
		}
		return nil
	}

	return e.expandRecurringRange(from, until, duration)
}

func (e rawEvent) eventEnd() time.Time {
	if e.hasEnd {
		return e.end
	}
	if e.hasDuration {
		return e.start.Add(e.duration)
	}
	if e.allDay {
		return e.start.Add(24 * time.Hour)
	}
	return e.start.Add(time.Hour)
}

func (e rawEvent) expandRecurring(now time.Time, duration time.Duration) []Event {
	freq := strings.ToUpper(e.rrule["FREQ"])
	interval := parsePositiveInt(e.rrule["INTERVAL"], 1)
	countLimit := parsePositiveInt(e.rrule["COUNT"], 0)
	until, hasUntil := parseUntil(e.rrule["UNTIL"], e.start.Location())
	horizon := now.Add(lookAhead)
	earliestPrevious := now.Add(-lookAhead)
	out := make([]Event, 0)
	var previous Event
	hasPrevious := false
	generated := 0

	addOccurrence := func(start time.Time) bool {
		if countLimit > 0 && generated >= countLimit {
			return false
		}
		if hasUntil && start.After(until) {
			return false
		}
		if start.After(horizon) {
			return false
		}
		generated++
		end := start.Add(duration)
		if e.exdates[eventKey(start)] {
			return true
		}
		if !end.After(now) {
			if end.After(earliestPrevious) && (!hasPrevious || end.After(previous.End)) {
				previous = Event{Summary: e.summary, Start: start, End: end, AllDay: e.allDay}
				hasPrevious = true
			}
			return true
		}
		if !start.Before(horizon) {
			return true
		}
		out = append(out, Event{Summary: e.summary, Start: start, End: end, AllDay: e.allDay})
		return len(out) < maxReturned
	}

	byday := parseByDay(e.rrule["BYDAY"])
	if (freq == "WEEKLY" || freq == "DAILY") && len(byday) > 0 {
		current := dateOnly(e.start)
		for i := 0; i < maxExpanded; i++ {
			candidate := time.Date(current.Year(), current.Month(), current.Day(), e.start.Hour(), e.start.Minute(), e.start.Second(), e.start.Nanosecond(), e.start.Location())
			if !candidate.Before(e.start) && weekdayAllowed(candidate.Weekday(), byday) && recurrenceIntervalMatches(freq, interval, e.start, candidate) {
				if !addOccurrence(candidate) {
					break
				}
			}
			current = current.AddDate(0, 0, 1)
			if current.After(horizon) {
				break
			}
		}
		if hasPrevious {
			out = append([]Event{previous}, out...)
		}
		return out
	}

	current := e.start
	for i := 0; i < maxExpanded; i++ {
		if !addOccurrence(current) {
			break
		}
		next, ok := nextRecurringTime(current, freq, interval)
		if !ok {
			break
		}
		current = next
	}
	if hasPrevious {
		out = append([]Event{previous}, out...)
	}
	return out
}

func (e rawEvent) expandRecurringRange(from, until time.Time, duration time.Duration) []Event {
	freq := strings.ToUpper(e.rrule["FREQ"])
	interval := parsePositiveInt(e.rrule["INTERVAL"], 1)
	countLimit := parsePositiveInt(e.rrule["COUNT"], 0)
	untilRule, hasUntil := parseUntil(e.rrule["UNTIL"], e.start.Location())
	earliest := from.Add(-duration)
	out := make([]Event, 0)

	addOccurrence := func(start time.Time) {
		end := start.Add(duration)
		if e.exdates[eventKey(start)] || !end.After(from) || !start.Before(until) {
			return
		}
		out = append(out, Event{Summary: e.summary, Start: start, End: end, AllDay: e.allDay})
	}

	byday := parseByDay(e.rrule["BYDAY"])
	if (freq == "WEEKLY" || freq == "DAILY") && len(byday) > 0 {
		current := dateOnly(e.start)
		generated := 0
		if countLimit == 0 && earliest.After(e.start) {
			current = dateOnly(earliest)
		}
		for i := 0; i < maxExpanded; i++ {
			candidate := time.Date(current.Year(), current.Month(), current.Day(), e.start.Hour(), e.start.Minute(), e.start.Second(), e.start.Nanosecond(), e.start.Location())
			if candidate.After(until) || (hasUntil && candidate.After(untilRule)) {
				break
			}
			if !candidate.Before(e.start) && weekdayAllowed(candidate.Weekday(), byday) && recurrenceIntervalMatches(freq, interval, e.start, candidate) {
				generated++
				if countLimit > 0 && generated > countLimit {
					break
				}
				addOccurrence(candidate)
			}
			current = current.AddDate(0, 0, 1)
		}
		return out
	}

	current := e.start
	generated := 0
	if countLimit == 0 && earliest.After(current) {
		current = fastForwardRecurring(current, freq, interval, earliest)
	}
	for i := 0; i < maxExpanded; i++ {
		if current.After(until) || (hasUntil && current.After(untilRule)) {
			break
		}
		generated++
		if countLimit > 0 && generated > countLimit {
			break
		}
		addOccurrence(current)
		next, ok := nextRecurringTime(current, freq, interval)
		if !ok {
			break
		}
		current = next
	}
	return out
}

func fastForwardRecurring(start time.Time, freq string, interval int, target time.Time) time.Time {
	current := start
	switch freq {
	case "DAILY":
		days := calendarDayNumber(target) - calendarDayNumber(start)
		if days > interval {
			current = start.AddDate(0, 0, (days/interval)*interval)
		}
	case "WEEKLY":
		days := calendarDayNumber(target) - calendarDayNumber(start)
		step := 7 * interval
		if days > step {
			current = start.AddDate(0, 0, (days/step)*step)
		}
	case "MONTHLY":
		months := (target.Year()-start.Year())*12 + int(target.Month()-start.Month())
		if months > interval {
			current = start.AddDate(0, (months/interval)*interval, 0)
		}
	case "YEARLY":
		years := target.Year() - start.Year()
		if years > interval {
			current = start.AddDate((years/interval)*interval, 0, 0)
		}
	default:
		return start
	}
	for current.Before(target) {
		next, ok := nextRecurringTime(current, freq, interval)
		if !ok {
			return start
		}
		current = next
	}
	return current
}

func calendarDayNumber(value time.Time) int {
	return int(time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC).Unix() / int64(24*time.Hour/time.Second))
}

func unfoldLines(data string) []string {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	raw := strings.Split(data, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if len(lines) > 0 {
				lines[len(lines)-1] += line[1:]
			}
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func parseContentLine(line string) (string, map[string]string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", nil, "", false
	}
	left := line[:idx]
	value := line[idx+1:]
	parts := strings.Split(left, ";")
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, "", false
	}
	name := strings.ToUpper(parts[0])
	params := make(map[string]string)
	for _, part := range parts[1:] {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		params[strings.ToUpper(key)] = strings.Trim(value, `"`)
	}
	return name, params, value, true
}

func parseICSTime(value string, params map[string]string, fallback *time.Location) (time.Time, bool, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false, false
	}
	loc := fallback
	if loc == nil {
		loc = time.Local
	}
	if tzid := params["TZID"]; tzid != "" {
		if loaded, err := time.LoadLocation(tzid); err == nil {
			loc = loaded
		}
	}

	if params["VALUE"] == "DATE" || (len(value) == len("20060102") && !strings.Contains(value, "T")) {
		t, err := time.ParseInLocation("20060102", value, loc)
		return t, true, err == nil
	}
	if strings.HasSuffix(value, "Z") {
		for _, layout := range []string{"20060102T150405Z", "20060102T1504Z", "20060102T15Z"} {
			if t, err := time.Parse(layout, value); err == nil {
				return t, false, true
			}
		}
		return time.Time{}, false, false
	}
	for _, layout := range []string{"20060102T150405", "20060102T1504", "20060102T15"} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, false, true
		}
	}
	return time.Time{}, false, false
}

func parseUntil(value string, loc *time.Location) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	t, allDay, ok := parseICSTime(value, nil, loc)
	if !ok {
		return time.Time{}, false
	}
	if allDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return t, true
}

func parseRule(value string) map[string]string {
	rule := make(map[string]string)
	for _, part := range strings.Split(value, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		rule[strings.ToUpper(key)] = value
	}
	return rule
}

func parseByDay(value string) []time.Weekday {
	if value == "" {
		return nil
	}
	out := make([]time.Weekday, 0)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimLeft(strings.ToUpper(strings.TrimSpace(part)), "+-0123456789")
		switch part {
		case "MO":
			out = append(out, time.Monday)
		case "TU":
			out = append(out, time.Tuesday)
		case "WE":
			out = append(out, time.Wednesday)
		case "TH":
			out = append(out, time.Thursday)
		case "FR":
			out = append(out, time.Friday)
		case "SA":
			out = append(out, time.Saturday)
		case "SU":
			out = append(out, time.Sunday)
		}
	}
	return out
}

func weekdayAllowed(day time.Weekday, allowed []time.Weekday) bool {
	for _, candidate := range allowed {
		if candidate == day {
			return true
		}
	}
	return false
}

func recurrenceIntervalMatches(freq string, interval int, start, candidate time.Time) bool {
	days := calendarDayNumber(candidate) - calendarDayNumber(start)
	if days < 0 {
		return false
	}
	switch freq {
	case "DAILY":
		return days%interval == 0
	case "WEEKLY":
		return (days/7)%interval == 0
	default:
		return true
	}
}

func nextRecurringTime(current time.Time, freq string, interval int) (time.Time, bool) {
	switch freq {
	case "DAILY":
		return current.AddDate(0, 0, interval), true
	case "WEEKLY":
		return current.AddDate(0, 0, 7*interval), true
	case "MONTHLY":
		return current.AddDate(0, interval, 0), true
	case "YEARLY":
		return current.AddDate(interval, 0, 0), true
	default:
		return time.Time{}, false
	}
}

func parsePositiveInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseICSDuration(value string) (time.Duration, bool) {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "" {
		return 0, false
	}
	sign := time.Duration(1)
	if strings.HasPrefix(value, "-") {
		sign = -1
		value = strings.TrimPrefix(value, "-")
	} else {
		value = strings.TrimPrefix(value, "+")
	}
	if !strings.HasPrefix(value, "P") {
		return 0, false
	}
	value = strings.TrimPrefix(value, "P")
	inTime := false
	number := ""
	var total time.Duration
	for _, r := range value {
		if r == 'T' {
			inTime = true
			continue
		}
		if r >= '0' && r <= '9' {
			number += string(r)
			continue
		}
		if number == "" {
			return 0, false
		}
		n, err := strconv.Atoi(number)
		if err != nil {
			return 0, false
		}
		number = ""
		switch r {
		case 'W':
			total += time.Duration(n) * 7 * 24 * time.Hour
		case 'D':
			total += time.Duration(n) * 24 * time.Hour
		case 'H':
			if !inTime {
				return 0, false
			}
			total += time.Duration(n) * time.Hour
		case 'M':
			if !inTime {
				return 0, false
			}
			total += time.Duration(n) * time.Minute
		case 'S':
			if !inTime {
				return 0, false
			}
			total += time.Duration(n) * time.Second
		default:
			return 0, false
		}
	}
	if number != "" {
		return 0, false
	}
	return total * sign, total != 0
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func eventKey(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func unescapeText(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\n`, "\n",
		`\N`, "\n",
		`\,`, ",",
		`\;`, ";",
	)
	return replacer.Replace(value)
}
