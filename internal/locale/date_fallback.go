//go:build !cgo || windows

package locale

import "time"

func formatDate(time.Time) (string, bool) {
	return "", false
}
