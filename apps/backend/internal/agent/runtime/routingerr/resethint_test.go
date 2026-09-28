package routingerr

import (
	"testing"
	"time"
)

func TestParseResetHintRequiresExplicitTimezone(t *testing.T) {
	now := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	for _, notice := range []string{
		"try again at Sep 27th, 2026 3:09 AM",
		"try again at Sep 27th, 2026 3:09 AM.",
	} {
		if got := parseResetHintAt(notice, now); got != nil {
			t.Fatalf("parseResetHintAt(%q) = %v, want no hint without a timezone", notice, got)
		}
	}
}

func TestParseResetHintUsesExplicitTimezone(t *testing.T) {
	now := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		text string
		want time.Time
	}{
		{
			name: "UTC",
			text: "try again at Sep 27th, 2026 3:09 AM UTC",
			want: time.Date(2026, time.September, 27, 3, 9, 0, 0, time.UTC),
		},
		{
			name: "numeric offset",
			text: "try again at Sep 27th, 2026 3:09 PM -04:30",
			want: time.Date(2026, time.September, 27, 15, 9, 0, 0, time.FixedZone("", -(4*60+30)*60)),
		},
		{
			name: "24-hour clock",
			text: "try again at Sep 27th, 2026 15:09 -04:30",
			want: time.Date(2026, time.September, 27, 15, 9, 0, 0, time.FixedZone("", -(4*60+30)*60)),
		},
		{
			name: "leap date",
			text: "try again at Feb 29th, 2028 12:00 AM UTC",
			want: time.Date(2028, time.February, 29, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseResetHintAt(tc.text, now)
			if got == nil || !got.Equal(tc.want) {
				t.Fatalf("parseResetHintAt(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestParseResetHintRejectsInvalidDateAndTime(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, notice := range []string{
		"try again at Feb 30th, 2028 3:09 PM UTC",
		"try again at Feb 29th, 2027 3:09 PM UTC",
		"try again at Sep 27rd, 2026 3:09 PM UTC",
		"try again at Sep 27th, 2026 13:09 PM UTC",
		"try again at Sep 27th, 2026 00:09 AM UTC",
		"try again at Sep 27th, 2026 24:09 UTC",
		"try again at Sep 27th, 2026 3:60 PM UTC",
		"try again at Sep 27th, 2026 3:09 PM +15:00",
		"try again at Sep 27th, 2026 3:09 PM +14:01",
	} {
		if got := parseResetHintAt(notice, now); got != nil {
			t.Errorf("parseResetHintAt(%q) = %v, want no hint", notice, got)
		}
	}
}

func TestParseResetHintYearlessDateRollsForward(t *testing.T) {
	zone := time.FixedZone("provider", -5*60*60)
	tests := []struct {
		name string
		now  time.Time
		text string
		want time.Time
	}{
		{
			name: "next year across December 31",
			now:  time.Date(2026, time.December, 31, 23, 30, 0, 0, zone),
			text: "try again at Jan 1st 1:05 AM -05:00",
			want: time.Date(2027, time.January, 1, 1, 5, 0, 0, zone),
		},
		{
			name: "current year when the date is future",
			now:  time.Date(2026, time.August, 1, 0, 0, 0, 0, zone),
			text: "try again at Sep 1st 1:05 AM -05:00",
			want: time.Date(2026, time.September, 1, 1, 5, 0, 0, zone),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseResetHintAt(tc.text, tc.now)
			if got == nil || !got.Equal(tc.want) {
				t.Fatalf("parseResetHintAt() = %v, want %v", got, tc.want)
			}
		})
	}
}
