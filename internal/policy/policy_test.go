package policy_test

import (
	"testing"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/policy"
)

func TestIsPeakHours(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		t       time.Time
		subType string
		want    bool
	}{
		// Time-based tests (affected sub type).
		{
			name:    "weekday at window start",
			t:       time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC), // Wednesday
			subType: "pro",
			want:    true,
		},
		{
			name:    "weekday inside window",
			t:       time.Date(2025, 1, 1, 16, 30, 0, 0, time.UTC), // Wednesday
			subType: "pro",
			want:    true,
		},
		{
			name:    "weekday at last hour of window",
			t:       time.Date(2025, 1, 1, 18, 59, 59, 0, time.UTC), // Wednesday
			subType: "pro",
			want:    true,
		},
		{
			name:    "weekday one minute before window",
			t:       time.Date(2025, 1, 1, 12, 59, 0, 0, time.UTC), // Wednesday
			subType: "pro",
			want:    false,
		},
		{
			name:    "weekday at window end",
			t:       time.Date(2025, 1, 1, 19, 0, 0, 0, time.UTC), // Wednesday
			subType: "pro",
			want:    false,
		},
		{
			name:    "weekday morning",
			t:       time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC), // Wednesday
			subType: "pro",
			want:    false,
		},
		{
			name:    "weekday night",
			t:       time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC), // Wednesday
			subType: "pro",
			want:    false,
		},
		{
			name:    "monday in window",
			t:       time.Date(2025, 1, 6, 14, 0, 0, 0, time.UTC), // Monday
			subType: "pro",
			want:    true,
		},
		{
			name:    "friday in window",
			t:       time.Date(2025, 1, 3, 18, 0, 0, 0, time.UTC), // Friday
			subType: "pro",
			want:    true,
		},
		{
			name:    "saturday in window hours",
			t:       time.Date(2025, 1, 4, 14, 0, 0, 0, time.UTC), // Saturday
			subType: "pro",
			want:    false,
		},
		{
			name:    "sunday in window hours",
			t:       time.Date(2025, 1, 5, 14, 0, 0, 0, time.UTC), // Sunday
			subType: "pro",
			want:    false,
		},

		// Subscription type tests (peak window time).
		{name: "pro lowercase", t: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), subType: "pro", want: true},
		{name: "Pro mixed case", t: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), subType: "Pro", want: true},
		{name: "max", t: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), subType: "max", want: true},
		{name: "max_5x variant", t: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), subType: "max_5x", want: true},
		{name: "Max mixed case", t: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), subType: "Max", want: true},
		{name: "team", t: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), subType: "team", want: false},
		{name: "enterprise", t: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), subType: "enterprise", want: false},
		{name: "empty string", t: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), subType: "", want: false},
		{
			name:    "unknown value",
			t:       time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC),
			subType: "something_else",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := policy.IsPeakHours(tt.t, tt.subType)
			if got != tt.want {
				t.Errorf("IsPeakHours(%v, %q) = %v, want %v", tt.t, tt.subType, got, tt.want)
			}
		})
	}
}
