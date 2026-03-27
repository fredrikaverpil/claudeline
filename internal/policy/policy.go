// Package policy determines which operational constraints are currently active,
// such as peak-hour quota burn rates.
package policy

import (
	"strings"
	"time"
)

// IsPeakHours reports whether t falls within the peak usage window for the
// given subscription type. During peak hours, quota limits burn faster for
// affected subscription types (free, pro and max plans).
//
// The peak window is weekdays 13:00–19:00 UTC.
//
// Source: https://xcancel.com/trq212/status/2037254607001559305#m
// TODO: add case for "free" plan (we don't know the key yet).
func IsPeakHours(t time.Time, subType string) bool {
	lower := strings.ToLower(subType)
	if !strings.Contains(lower, "pro") && !strings.Contains(lower, "max") {
		return false
	}
	utc := t.UTC()
	switch utc.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	h := utc.Hour()
	return h >= 13 && h < 19
}
