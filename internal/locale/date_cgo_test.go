//go:build cgo && !windows

package locale

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFormatDateUsesInstalledSystemLocale(t *testing.T) {
	locale, weekday, month, ok := installedLocale(t)
	if !ok {
		t.Skip("no known non-English locale installed")
	}
	t.Setenv("LC_ALL", locale)

	now := time.Date(2026, 5, 1, 13, 14, 15, 0, time.UTC)
	got := FormatDate(now)
	if !strings.Contains(got, weekday) || !strings.Contains(got, month) {
		t.Fatalf("expected %q to contain weekday %q and month %q for locale %q", got, weekday, month, locale)
	}
}

func installedLocale(t *testing.T) (locale, weekday, month string, ok bool) {
	t.Helper()

	out, err := exec.Command("locale", "-a").Output()
	if err != nil {
		return "", "", "", false
	}
	installed := map[string]string{}
	for _, line := range strings.Fields(string(out)) {
		installed[normalizeLocale(line)] = line
	}

	candidates := []struct {
		locale  string
		weekday string
		month   string
	}{
		{"fr_FR.utf8", "vendredi", "mai"},
		{"de_DE.utf8", "Freitag", "Mai"},
		{"es_ES.utf8", "viernes", "may"},
	}
	for _, candidate := range candidates {
		if name, ok := installed[normalizeLocale(candidate.locale)]; ok {
			return name, candidate.weekday, candidate.month, true
		}
	}
	return "", "", "", false
}

func normalizeLocale(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, ".utf-8", ".utf8")
	return value
}
