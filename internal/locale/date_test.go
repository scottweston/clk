package locale

import (
	"testing"
	"time"
)

func TestFormatDateFallsBackForInvalidSystemLocale(t *testing.T) {
	t.Setenv("LC_ALL", "clk_DOES_NOT_EXIST.UTF-8")

	now := time.Date(2026, 5, 1, 13, 14, 15, 0, time.UTC)
	if got, want := FormatDate(now), "Friday, 01 May 2026"; got != want {
		t.Fatalf("expected fallback date %q, got %q", want, got)
	}
}

func TestFormatDateCLocale(t *testing.T) {
	t.Setenv("LC_ALL", "C")

	now := time.Date(2026, 5, 1, 13, 14, 15, 0, time.UTC)
	if got, want := FormatDate(now), "Friday, 01 May 2026"; got != want {
		t.Fatalf("expected C locale date %q, got %q", want, got)
	}
}
