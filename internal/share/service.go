package share

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"clk/internal/config"
	"clk/internal/ics"
)

const (
	defaultWindow = 24 * time.Hour
	maxWindow     = 90 * 24 * time.Hour
)

type SourceData struct {
	URL  string
	Data []byte
}

type Snapshot struct {
	Time    config.TimeConfig
	Workday config.WorkdayConfig
	Sources []SourceData
}

type Service struct {
	mu         sync.RWMutex
	snapshot   Snapshot
	server     *http.Server
	listener   net.Listener
	socketPath string
	socketInfo os.FileInfo
	now        func() time.Time
	closed     bool
}

func New() *Service {
	return &Service{now: time.Now}
}

func NewAt(socketPath string) *Service {
	return &Service{socketPath: socketPath, now: time.Now}
}

func (s *Service) Publish(snapshot Snapshot) {
	copySnapshot := Snapshot{
		Time: snapshot.Time,
		Workday: config.WorkdayConfig{
			Schedule:     make(map[string]config.WorkdayDayConfig, len(snapshot.Workday.Schedule)),
			ShowProgress: snapshot.Workday.ShowProgress,
		},
		Sources: make([]SourceData, len(snapshot.Sources)),
	}
	for day, entry := range snapshot.Workday.Schedule {
		copySnapshot.Workday.Schedule[day] = entry
	}
	for i, source := range snapshot.Sources {
		copySnapshot.Sources[i] = SourceData{
			URL:  strings.TrimSpace(source.URL),
			Data: append([]byte(nil), source.Data...),
		}
	}

	s.mu.Lock()
	s.snapshot = copySnapshot
	s.mu.Unlock()
}

func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return net.ErrClosed
	}
	if s.listener != nil {
		return nil
	}

	path := s.socketPath
	if path == "" {
		var err error
		path, err = SocketPath()
		if err != nil {
			return err
		}
		s.socketPath = path
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("secure socket directory: %w", err)
	}
	if err := prepareSocketPath(path); err != nil {
		return err
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return fmt.Errorf("secure socket: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return fmt.Errorf("inspect socket: %w", err)
	}

	server := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	s.listener = listener
	s.server = server
	s.socketInfo = info
	go func() {
		_ = server.Serve(listener)
	}()
	return nil
}

func (s *Service) Stop() error {
	s.mu.Lock()
	server := s.server
	listener := s.listener
	path := s.socketPath
	info := s.socketInfo
	s.server = nil
	s.listener = nil
	s.socketInfo = nil
	s.mu.Unlock()

	if server == nil && listener == nil {
		return nil
	}
	var closeErr error
	if server != nil {
		closeErr = server.Close()
	} else if listener != nil {
		closeErr = listener.Close()
	}
	if current, err := os.Lstat(path); err == nil && info != nil && os.SameFile(info, current) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && closeErr == nil {
			closeErr = err
		}
	}
	if errors.Is(closeErr, http.ErrServerClosed) || errors.Is(closeErr, net.ErrClosed) {
		return nil
	}
	return closeErr
}

func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.Stop()
}

func (s *Service) SocketPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.socketPath
}

func SocketPath() (string, error) {
	if runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); runtimeDir != "" && filepath.IsAbs(runtimeDir) {
		return filepath.Join(runtimeDir, config.AppDir, "clk.sock"), nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve socket directory: %w", err)
	}
	return filepath.Join(cacheDir, config.AppDir, "clk.sock"), nil
}

func prepareSocketPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect socket path: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("socket path exists and is not a socket: %s", path)
	}
	conn, dialErr := net.DialTimeout("unix", path, 150*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("socket is already in use: %s", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	return nil
}

type response struct {
	From   time.Time `json:"from"`
	Until  time.Time `json:"until"`
	Events []Event   `json:"events"`
}

type Event struct {
	Type     string    `json:"type"`
	Summary  string    `json:"summary"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	AllDay   bool      `json:"all_day"`
	SourceID string    `json:"source_id,omitempty"`
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	duration, matched, err := requestWindow(r.URL.Path)
	if !matched {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	snapshot := s.readSnapshot()
	location := displayLocation(snapshot.Time)
	now := s.now
	if now == nil {
		now = time.Now
	}
	from := now().In(location)
	until := from.Add(duration)
	events, err := snapshotEvents(snapshot, from, until, location)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "build event response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{From: from, Until: until, Events: events})
}

func (s *Service) readSnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func requestWindow(path string) (time.Duration, bool, error) {
	if path == "/events" {
		return defaultWindow, true, nil
	}
	if !strings.HasPrefix(path, "/events/") || strings.Contains(strings.TrimPrefix(path, "/events/"), "/") {
		return 0, false, nil
	}
	value := strings.TrimPrefix(path, "/events/")
	if len(value) < 2 {
		return 0, true, fmt.Errorf("invalid event window")
	}
	unit := value[len(value)-1]
	number := value[:len(value)-1]
	amount, err := strconv.ParseUint(number, 10, 64)
	if err != nil || amount == 0 {
		return 0, true, fmt.Errorf("invalid event window")
	}
	var unitDuration time.Duration
	switch unit {
	case 'h':
		unitDuration = time.Hour
	case 'd':
		unitDuration = 24 * time.Hour
	case 'm':
		unitDuration = 30 * 24 * time.Hour
	default:
		return 0, true, fmt.Errorf("invalid event window")
	}
	if amount > uint64(maxWindow/unitDuration) {
		return 0, true, fmt.Errorf("event window exceeds 90 days")
	}
	return time.Duration(amount) * unitDuration, true, nil
}

func snapshotEvents(snapshot Snapshot, from, until time.Time, location *time.Location) ([]Event, error) {
	events := workdayEvents(snapshot.Workday, from, until, location)
	for _, source := range snapshot.Sources {
		if len(source.Data) == 0 {
			continue
		}
		calendarEvents, err := ics.ParseRange(source.Data, from, until)
		if err != nil {
			return nil, err
		}
		sourceID := sourceID(source.URL)
		for _, event := range calendarEvents {
			events = append(events, Event{
				Type:     "ics",
				Summary:  event.Summary,
				Start:    event.Start.In(location),
				End:      event.End.In(location),
				AllDay:   event.AllDay,
				SourceID: sourceID,
			})
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].Start.Equal(events[j].Start) {
			return events[i].Start.Before(events[j].Start)
		}
		if !events[i].End.Equal(events[j].End) {
			return events[i].End.Before(events[j].End)
		}
		if events[i].Type != events[j].Type {
			return events[i].Type < events[j].Type
		}
		if events[i].SourceID != events[j].SourceID {
			return events[i].SourceID < events[j].SourceID
		}
		return events[i].Summary < events[j].Summary
	})
	if events == nil {
		events = []Event{}
	}
	return events, nil
}

func workdayEvents(cfg config.WorkdayConfig, from, until time.Time, location *time.Location) []Event {
	startDate := dateOnly(from.In(location), location)
	lastDate := dateOnly(until.In(location), location)
	events := make([]Event, 0)
	for day := startDate; !day.After(lastDate); day = day.AddDate(0, 0, 1) {
		entry, ok := cfg.Schedule[weekdayName(day.Weekday())]
		if !ok || !entry.Enabled {
			continue
		}
		startHour, startMinute := clockTime(entry.StartTime, 9, 0)
		endHour, endMinute := clockTime(entry.EndTime, 17, 0)
		start := time.Date(day.Year(), day.Month(), day.Day(), startHour, startMinute, 0, 0, location)
		end := time.Date(day.Year(), day.Month(), day.Day(), endHour, endMinute, 0, 0, location)
		if !end.After(start) {
			end = start.Add(time.Minute)
		}
		if end.After(from) && start.Before(until) {
			events = append(events, Event{Type: "workday", Summary: "Workday", Start: start, End: end, AllDay: false})
		}
	}
	return events
}

func clockTime(value string, fallbackHour, fallbackMinute int) (int, int) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return fallbackHour, fallbackMinute
	}
	return parsed.Hour(), parsed.Minute()
}

func displayLocation(cfg config.TimeConfig) *time.Location {
	if cfg.Format == "utc" {
		return time.UTC
	}
	if cfg.Timezone != "" && cfg.Timezone != "Local" {
		if location, err := time.LoadLocation(cfg.Timezone); err == nil {
			return location
		}
	}
	return time.Local
}

func dateOnly(value time.Time, location *time.Location) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}

func weekdayName(day time.Weekday) string {
	return [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}[day]
}

func sourceID(source string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(source)))
	return hex.EncodeToString(sum[:])
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
