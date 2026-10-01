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

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.2, AC-AGENTS-CLAUDE-SESSION-LIMIT-001.3
func TestParseClaudeResetClock(t *testing.T) {
	for _, tc := range []struct {
		name string
		now  time.Time
		text string
		want time.Time
	}{
		{
			name: "observed notice on the same day",
			now:  time.Date(2026, time.October, 1, 8, 3, 0, 0, time.UTC),
			text: "Internal error: You've hit your session limit · resets 11:10am (Europe/Helsinki)",
			want: time.Date(2026, time.October, 1, 8, 10, 0, 0, time.UTC),
		},
		{
			name: "next calendar day",
			now:  time.Date(2026, time.October, 1, 8, 11, 0, 0, time.UTC),
			text: "resets 11:10am (Europe/Helsinki)",
			want: time.Date(2026, time.October, 2, 8, 10, 0, 0, time.UTC),
		},
		{
			name: "equality rolls to next day",
			now:  time.Date(2026, time.October, 1, 8, 10, 0, 0, time.UTC),
			text: "resets 11:10am (Europe/Helsinki)",
			want: time.Date(2026, time.October, 2, 8, 10, 0, 0, time.UTC),
		},
		{
			name: "omitted minutes mean zero",
			now:  time.Date(2026, time.October, 1, 7, 50, 0, 0, time.UTC),
			text: "resets 11am (Europe/Helsinki)",
			want: time.Date(2026, time.October, 1, 8, 0, 0, 0, time.UTC),
		},
		{
			name: "midnight in UTC",
			now:  time.Date(2026, time.January, 2, 23, 0, 0, 0, time.UTC),
			text: "resets 12am (UTC)",
			want: time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "noon in UTC",
			now:  time.Date(2026, time.January, 2, 11, 0, 0, 0, time.UTC),
			text: "resets 12pm (UTC)",
			want: time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "backend timezone does not affect result",
			now:  time.Date(2026, time.October, 1, 18, 3, 0, 0, time.FixedZone("backend", 10*60*60)),
			text: "resets 11:10am (Europe/Helsinki)",
			want: time.Date(2026, time.October, 1, 8, 10, 0, 0, time.UTC),
		},
		{
			name: "nested IANA name at parser boundary",
			now:  time.Date(2026, time.October, 1, 13, 0, 0, 0, time.UTC),
			text: "resets 11am (America/Argentina/Buenos_Aires)",
			want: time.Date(2026, time.October, 1, 14, 0, 0, 0, time.UTC),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseResetHintAt(tc.text, tc.now)
			if got == nil || !got.Equal(tc.want) {
				t.Fatalf("parseResetHintAt(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.3
func TestParseClaudeResetClockRollover(t *testing.T) {
	for _, tc := range []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "month boundary",
			now:  time.Date(2026, time.October, 31, 9, 11, 0, 0, time.UTC),
			want: time.Date(2026, time.November, 1, 9, 10, 0, 0, time.UTC),
		},
		{
			name: "year boundary",
			now:  time.Date(2026, time.December, 31, 9, 11, 0, 0, time.UTC),
			want: time.Date(2027, time.January, 1, 9, 10, 0, 0, time.UTC),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseResetHintAt("resets 11:10am (Europe/Helsinki)", tc.now)
			if got == nil || !got.Equal(tc.want) {
				t.Fatalf("parseResetHintAt() = %v, want %v", got, tc.want)
			}
		})
	}
}

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.4
func TestParseClaudeResetClockRejectsInvalid(t *testing.T) {
	now := time.Date(2026, time.October, 1, 8, 3, 0, 0, time.UTC)
	for _, text := range []string{
		"resets 0am (Europe/Helsinki)",
		"resets 13pm (Europe/Helsinki)",
		"resets 11:60am (Europe/Helsinki)",
		"resets 11:10 (Europe/Helsinki)",
		"resets 11:10am Europe/Helsinki",
		"resets 11:10am",
		"resets 11:10am ()",
		"resets 11:10am (Mars/Olympus)",
		"resets 11:10am (Local)",
		"resets 11:10am (EET)",
		"resets 11:10am (../Etc/UTC)",
		"resets 11:10am (/Europe/Helsinki)",
	} {
		if got := parseResetHintAt(text, now); got != nil {
			t.Errorf("parseResetHintAt(%q) = %v, want no hint", text, got)
		}
	}
}

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.3, AC-AGENTS-CLAUDE-SESSION-LIMIT-001.4
func TestParseClaudeResetClockDST(t *testing.T) {
	for _, tc := range []struct {
		name string
		now  time.Time
		text string
		want *time.Time
	}{
		{
			name: "spring gap has no hint",
			now:  time.Date(2026, time.March, 29, 0, 0, 0, 0, time.UTC),
			text: "resets 3:30am (Europe/Helsinki)",
		},
		{
			name: "elapsed spring gap rolls to next valid day",
			now:  time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC),
			text: "resets 3:30am (Europe/Helsinki)",
			want: timePtr(time.Date(2026, time.March, 30, 0, 30, 0, 0, time.UTC)),
		},
		{
			name: "autumn repeated wall time has no hint",
			now:  time.Date(2026, time.October, 25, 0, 15, 0, 0, time.UTC),
			text: "resets 3:30am (Europe/Helsinki)",
		},
		{
			name: "valid clock after spring offset change",
			now:  time.Date(2026, time.March, 28, 23, 0, 0, 0, time.UTC),
			text: "resets 4:30am (Europe/Helsinki)",
			want: timePtr(time.Date(2026, time.March, 29, 1, 30, 0, 0, time.UTC)),
		},
		{
			name: "valid clock after autumn offset change",
			now:  time.Date(2026, time.October, 24, 23, 0, 0, 0, time.UTC),
			text: "resets 4:30am (Europe/Helsinki)",
			want: timePtr(time.Date(2026, time.October, 25, 2, 30, 0, 0, time.UTC)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseResetHintAt(tc.text, tc.now)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("parseResetHintAt() = %v, want no hint", got)
				}
				return
			}
			if got == nil || !got.Equal(*tc.want) {
				t.Fatalf("parseResetHintAt() = %v, want %v", got, *tc.want)
			}
		})
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
