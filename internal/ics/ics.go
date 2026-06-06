package ics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

type Event struct {
	Summary string
	Start   time.Time
	End     time.Time
}

func Fetch(ctx context.Context, source string, now time.Time) ([]Event, error) {
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
	return Parse(data, now)
}

func Parse(data []byte, now time.Time) ([]Event, error) {
	lines := unfoldLines(string(data))
	candidates := make([]Event, 0)
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
				candidates = append(candidates, current.expand(now)...)
				current = nil
			}
		default:
			if current != nil {
				current.set(name, params, value)
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Start.Equal(candidates[j].Start) {
			return candidates[i].End.Before(candidates[j].End)
		}
		return candidates[i].Start.Before(candidates[j].Start)
	})

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
		event := Event{Summary: e.summary, Start: e.start, End: e.start.Add(duration)}
		if event.End.After(now.Add(-lookAhead)) && event.Start.Before(now.Add(lookAhead)) {
			return []Event{event}
		}
		return nil
	}

	return e.expandRecurring(now, duration)
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
				previous = Event{Summary: e.summary, Start: start, End: end}
				hasPrevious = true
			}
			return true
		}
		if !start.Before(horizon) {
			return true
		}
		out = append(out, Event{Summary: e.summary, Start: start, End: end})
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
	days := int(dateOnly(candidate).Sub(dateOnly(start)).Hours() / 24)
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
