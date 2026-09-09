package shared

import (
	"testing"
	"time"
)

// TestMatchesCronExpression covers the design's "match check": whether a
// computed time is one the expression actually names, in the trigger's
// timezone. This is what AC-OFFICE-ROUTINE-STATUS-002.1/-002.8 route a
// mismatch through, catching findNextMatch's silent `after + 24h` fallback.
func TestMatchesCronExpression(t *testing.T) {
	after := mustParseTime(t, "2026-04-25T10:30:00Z")
	next, err := NextCronTime("* * * * *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := MatchesCronExpression("* * * * *", "", next)
	if err != nil {
		t.Fatalf("MatchesCronExpression: %v", err)
	}
	if !ok {
		t.Error("expected the computed next-minute slot to match its own expression")
	}
}

// TestMatchesCronExpression_CatchesFallback exercises the case the check
// exists for: an expression that names no slot within findNextMatch's
// 366-day search horizon (Feb 30 never occurs), whose silent `after + 24h`
// fallback must NOT be reported as a match.
func TestMatchesCronExpression_CatchesFallback(t *testing.T) {
	after := mustParseTime(t, "2026-04-25T10:30:00Z")
	fallback, err := NextCronTime("0 0 30 2 *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := MatchesCronExpression("0 0 30 2 *", "", fallback)
	if err != nil {
		t.Fatalf("MatchesCronExpression: %v", err)
	}
	if ok {
		t.Error("expected the after+24h fallback time to NOT match a never-satisfiable expression")
	}
}

// TestMatchesCronExpression_NonUTCTimezone is Decision 8's worked example:
// 09:00 America/New_York is 13:00 UTC, so the check must convert into the
// trigger's timezone before comparing fields, or every non-UTC schedule
// fails on every suppressed slot.
func TestMatchesCronExpression_NonUTCTimezone(t *testing.T) {
	after := mustParseTime(t, "2026-04-25T10:00:00Z") // before 9am ET on 4/25
	next, err := NextCronTime("0 9 * * *", "America/New_York", after)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := MatchesCronExpression("0 9 * * *", "America/New_York", next)
	if err != nil {
		t.Fatalf("MatchesCronExpression: %v", err)
	}
	if !ok {
		t.Errorf("expected %v to match '0 9 * * *' in America/New_York", next)
	}
}

// TestMatchesCronExpression_EmptyTimezoneIsUTC covers the empty-timezone
// case both NextCronTime and the match check must treat as UTC.
func TestMatchesCronExpression_EmptyTimezoneIsUTC(t *testing.T) {
	after := mustParseTime(t, "2026-04-25T10:00:00Z")
	next, err := NextCronTime("0 9 * * *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := MatchesCronExpression("0 9 * * *", "", next)
	if err != nil {
		t.Fatalf("MatchesCronExpression: %v", err)
	}
	if !ok {
		t.Errorf("expected %v to match '0 9 * * *' with empty timezone (UTC)", next)
	}
}

func TestMatchesCronExpression_MalformedExpression(t *testing.T) {
	if _, err := MatchesCronExpression("not a cron expression", "", mustParseTime(t, "2026-04-25T10:00:00Z")); err == nil {
		t.Error("expected an error for a malformed cron expression")
	}
}

func TestMatchesCronExpression_UnloadableTimezone(t *testing.T) {
	if _, err := MatchesCronExpression("* * * * *", "Not/A_Zone", mustParseTime(t, "2026-04-25T10:00:00Z")); err == nil {
		t.Error("expected an error for an unloadable timezone")
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return ts
}
