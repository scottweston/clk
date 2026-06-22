package share

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"clk/internal/config"
)

func TestRequestWindow(t *testing.T) {
	tests := []struct {
		path    string
		want    time.Duration
		matched bool
		wantErr bool
	}{
		{path: "/events", want: 24 * time.Hour, matched: true},
		{path: "/events/8h", want: 8 * time.Hour, matched: true},
		{path: "/events/2d", want: 48 * time.Hour, matched: true},
		{path: "/events/1m", want: 30 * 24 * time.Hour, matched: true},
		{path: "/events/3m", want: 90 * 24 * time.Hour, matched: true},
		{path: "/events/2160h", want: 90 * 24 * time.Hour, matched: true},
		{path: "/events/0h", matched: true, wantErr: true},
		{path: "/events/91d", matched: true, wantErr: true},
		{path: "/events/4m", matched: true, wantErr: true},
		{path: "/events/999999999999999999999999h", matched: true, wantErr: true},
		{path: "/events/1H", matched: true, wantErr: true},
		{path: "/events/", matched: true, wantErr: true},
		{path: "/events/1h/more", matched: false},
		{path: "/other", matched: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got, matched, err := requestWindow(test.path)
			if matched != test.matched || (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("requestWindow(%q) = (%v, %v, %v), want (%v, %v, error=%v)", test.path, got, matched, err, test.want, test.matched, test.wantErr)
			}
		})
	}
}

func TestEventsResponseCombinesActiveWorkdayAndICS(t *testing.T) {
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	service := New()
	service.now = func() time.Time { return now }
	cfg := config.Default()
	cfg.Time.Format = "utc"
	service.Publish(Snapshot{
		Time:    cfg.Time,
		Workday: cfg.Workday,
		Sources: []SourceData{{
			URL: " https://example.com/private-token.ics ",
			Data: []byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
SUMMARY:Active meeting
DTSTART:20260622T093000Z
DTEND:20260622T103000Z
END:VEVENT
BEGIN:VEVENT
SUMMARY:Upcoming meeting
DTSTART:20260622T104500Z
DTEND:20260622T111500Z
END:VEVENT
BEGIN:VEVENT
SUMMARY:Outside window
DTSTART:20260622T110000Z
DTEND:20260622T120000Z
END:VEVENT
END:VCALENDAR
`),
		}},
	})

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events/1h", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var got response
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.From.Equal(now) || !got.Until.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected response window: %+v", got)
	}
	if len(got.Events) != 3 {
		t.Fatalf("expected workday and two intersecting ICS events, got %+v", got.Events)
	}
	if got.Events[0].Type != "workday" || got.Events[0].Start.Hour() != 9 {
		t.Fatalf("expected active workday first, got %+v", got.Events)
	}
	if got.Events[1].Summary != "Active meeting" || got.Events[1].SourceID != sourceID("https://example.com/private-token.ics") {
		t.Fatalf("unexpected active ICS event: %+v", got.Events[1])
	}
	if got.Events[2].Summary != "Upcoming meeting" {
		t.Fatalf("unexpected upcoming ICS event: %+v", got.Events[2])
	}
	if strings.Contains(recorder.Body.String(), "private-token") {
		t.Fatalf("response exposed source URL: %s", recorder.Body.String())
	}
}

func TestEventsResponseUsesEmptyArrayAndJSONErrors(t *testing.T) {
	service := New()
	service.now = func() time.Time { return time.Date(2026, 6, 20, 20, 0, 0, 0, time.UTC) }
	service.Publish(Snapshot{Time: config.TimeConfig{Format: "utc"}})

	empty := httptest.NewRecorder()
	service.ServeHTTP(empty, httptest.NewRequest(http.MethodGet, "/events", nil))
	if !strings.Contains(empty.Body.String(), `"events":[]`) {
		t.Fatalf("expected an empty JSON array, got %s", empty.Body.String())
	}

	bad := httptest.NewRecorder()
	service.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/events/91d", nil))
	if bad.Code != http.StatusBadRequest || bad.Header().Get("Content-Type") != "application/json" || !strings.Contains(bad.Body.String(), `"error"`) {
		t.Fatalf("unexpected invalid-window response: %d %s", bad.Code, bad.Body.String())
	}

	method := httptest.NewRecorder()
	service.ServeHTTP(method, httptest.NewRequest(http.MethodPost, "/events", nil))
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("unexpected method response: %d %+v", method.Code, method.Header())
	}

	notFound := httptest.NewRecorder()
	service.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/health", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", notFound.Code)
	}
}

func TestWorkdayEndBeforeStartBecomesOneMinute(t *testing.T) {
	cfg := config.Default().Workday
	cfg.Schedule["mon"] = config.WorkdayDayConfig{Enabled: true, StartTime: "17:00", EndTime: "09:00"}
	from := time.Date(2026, 6, 22, 17, 0, 30, 0, time.UTC)
	events := workdayEvents(cfg, from, from.Add(time.Minute), time.UTC)
	if len(events) != 1 || events[0].End.Sub(events[0].Start) != time.Minute {
		t.Fatalf("expected one-minute workday event, got %+v", events)
	}
}

func TestServiceUnixSocketLifecycleAndCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "clk.sock")
	service := NewAt(path)
	if err := service.Start(); err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skipf("unix sockets are not permitted in this sandbox: %v", err)
		}
		t.Fatalf("start service: %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if info.Mode().Perm() != 0o600 || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("unexpected socket mode: %v", info.Mode())
	}

	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}}
	resp, err := client.Get("http://clk/events")
	if err != nil {
		t.Fatalf("GET over unix socket: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	other := NewAt(path)
	if err := other.Start(); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected active socket collision, got %v", err)
	}
	if err := service.Stop(); err != nil {
		t.Fatalf("stop service: %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected socket removal, got %v", err)
	}
}

func TestServiceReclaimsStaleSocketButNotRegularFile(t *testing.T) {
	dir := t.TempDir()
	stalePath := filepath.Join(dir, "stale.sock")
	listener, err := net.Listen("unix", stalePath)
	if err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skipf("unix sockets are not permitted in this sandbox: %v", err)
		}
		t.Fatalf("create stale socket: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close stale socket: %v", err)
	}
	service := NewAt(stalePath)
	if err := service.Start(); err != nil {
		t.Fatalf("reclaim stale socket: %v", err)
	}
	_ = service.Close()

	filePath := filepath.Join(dir, "file.sock")
	if err := os.WriteFile(filePath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	if err := NewAt(filePath).Start(); err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Fatalf("expected regular-file protection, got %v", err)
	}
}

func TestSocketPathUsesRuntimeThenCacheFallback(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	path, err := SocketPath()
	if err != nil || path != filepath.Join(runtimeDir, "clk", "clk.sock") {
		t.Fatalf("unexpected runtime socket path %q: %v", path, err)
	}

	cacheDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "relative")
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	path, err = SocketPath()
	if err != nil || path != filepath.Join(cacheDir, "clk", "clk.sock") {
		t.Fatalf("unexpected cache socket path %q: %v", path, err)
	}
}
