//go:build cgo && !windows

package locale

/*
#include <locale.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

static char* clk_strftime_date(int year, int month, int day, int hour, int minute, int second, int weekday) {
	if (setlocale(LC_TIME, "") == NULL) {
		return NULL;
	}

	struct tm value;
	memset(&value, 0, sizeof(value));
	value.tm_year = year - 1900;
	value.tm_mon = month - 1;
	value.tm_mday = day;
	value.tm_hour = hour;
	value.tm_min = minute;
	value.tm_sec = second;
	value.tm_wday = weekday;
	value.tm_isdst = -1;

	char buffer[512];
	size_t n = strftime(buffer, sizeof(buffer), "%A, %d %b %Y", &value);
	if (n == 0) {
		return NULL;
	}

	char *out = (char*)malloc(n + 1);
	if (out == NULL) {
		return NULL;
	}
	memcpy(out, buffer, n + 1);
	return out;
}
*/
import "C"

import (
	"time"
	"unsafe"
)

func formatDate(t time.Time) (string, bool) {
	formatted := C.clk_strftime_date(
		C.int(t.Year()),
		C.int(t.Month()),
		C.int(t.Day()),
		C.int(t.Hour()),
		C.int(t.Minute()),
		C.int(t.Second()),
		C.int(t.Weekday()),
	)
	if formatted == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(formatted))
	return C.GoString(formatted), true
}
