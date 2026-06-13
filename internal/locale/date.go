package locale

import "time"

const dateLayout = "Monday, 02 Jan 2006"

func FormatDate(t time.Time) string {
	if formatted, ok := formatDate(t); ok {
		return formatted
	}
	return t.Format(dateLayout)
}
