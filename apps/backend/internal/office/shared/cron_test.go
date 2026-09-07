package shared

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNextCronTime_EveryMinute(t *testing.T) {
	after := time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC)
	next, err := NextCronTime("* * * * *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 25, 10, 31, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextCronTime_DailyAt9(t *testing.T) {
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	next, err := NextCronTime("0 9 * * *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 26, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextCronTime_MondayAt9(t *testing.T) {
	// 2026-04-25 is a Saturday
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	next, err := NextCronTime("0 9 * * 1", "", after)
	if err != nil {
		t.Fatal(err)
	}
	// Next Monday is 2026-04-27
	want := time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v (weekday=%s), want %v", next, next.Weekday(), want)
	}
}

func TestNextCronTime_WithTimezone(t *testing.T) {
	// 12:00 UTC = 8:00 AM EDT, so next 9am EDT is same day at 13:00 UTC.
	after := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	next, err := NextCronTime("0 9 * * *", "America/New_York", after)
	if err != nil {
		t.Fatal(err)
	}
	// 9am EDT on Apr 25 = 13:00 UTC
	want := time.Date(2026, 4, 25, 13, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextCronTime_InvalidExpression(t *testing.T) {
	_, err := NextCronTime("invalid", "", time.Now())
	if err == nil {
		t.Error("expected error for invalid expression")
	}
}

func TestNextCronTime_StepExpression(t *testing.T) {
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	next, err := NextCronTime("*/15 * * * *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 25, 10, 15, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextCronTime_DayNames(t *testing.T) {
	// 2026-04-25 is a Saturday; first MON-FRI window opens on Mon 2026-04-27.
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		expr string
		want time.Time
	}{
		{"single MON", "0 9 * * MON", time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)},
		{"single lower mon", "0 9 * * mon", time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)},
		{"range MON-FRI", "0 9 * * MON-FRI", time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)},
		{"range lower mon-fri", "0 9 * * mon-fri", time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)},
		{"list Mon,WED,fri", "0 9 * * Mon,WED,fri", time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)},
		{"numeric equivalent 1-5", "0 9 * * 1-5", time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, err := NextCronTime(tc.expr, "", after)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.expr, err)
			}
			if !next.Equal(tc.want) {
				t.Errorf("expr %q: got %v, want %v", tc.expr, next, tc.want)
			}
		})
	}
}

func TestNextCronTime_MonthNames(t *testing.T) {
	// 2026-04-25 — past April; next JAN 1st is 2027-01-01.
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		expr string
		want time.Time
	}{
		{"single jan", "0 0 1 jan *", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"range JAN-MAR", "0 0 1 JAN-MAR *", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"list Feb,Apr,Jun", "0 0 1 Feb,Apr,Jun *", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, err := NextCronTime(tc.expr, "", after)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.expr, err)
			}
			if !next.Equal(tc.want) {
				t.Errorf("expr %q: got %v, want %v", tc.expr, next, tc.want)
			}
		})
	}
}

func TestNextCronTime_MixedCase(t *testing.T) {
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	exprs := []string{
		"0 9 * * MoN-FrI",
		"0 9 * * mON-fri",
		"0 9 * * Mon-Fri",
		"0 0 1 jAn *",
		"0 0 1 JaN *",
	}
	wantMonFri := time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)
	wantJan := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, expr := range exprs {
		next, err := NextCronTime(expr, "", after)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", expr, err)
		}
		want := wantMonFri
		if i >= 3 {
			want = wantJan
		}
		if !next.Equal(want) {
			t.Errorf("expr %q: got %v, want %v", expr, next, want)
		}
	}
}

func TestNextCronTime_InvalidDayName(t *testing.T) {
	_, err := NextCronTime("* * * * MOO", "", time.Now())
	if err == nil {
		t.Fatal("expected error for unknown day name MOO")
	}
	if !strings.Contains(err.Error(), "MOO") {
		t.Errorf("error %q should mention the invalid token MOO", err.Error())
	}
}

func TestNextCronTime_InvalidMonthName(t *testing.T) {
	_, err := NextCronTime("* * * MOO *", "", time.Now())
	if err == nil {
		t.Fatal("expected error for unknown month name MOO")
	}
	if !strings.Contains(err.Error(), "MOO") {
		t.Errorf("error %q should mention the invalid token MOO", err.Error())
	}
}

func TestNextCronTime_DayNameInMonthField_Rejected(t *testing.T) {
	_, err := NextCronTime("* * * MON *", "", time.Now())
	if err == nil {
		t.Fatal("expected error when day name MON appears in month field")
	}
}

// TestNextCronTime_DomDowOred verifies crontab(5) semantics: when both
// day-of-month and day-of-week are restricted, a match on either field
// fires, not just a match on both ("Friday the 13th").
func TestNextCronTime_DomDowOred(t *testing.T) {
	after := time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC)
	want := []time.Time{
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),  // Friday
		time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC),  // Friday
		time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC), // the 13th (Tuesday)
		time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), // Friday
		time.Date(2026, 1, 23, 0, 0, 0, 0, time.UTC), // Friday
	}
	cursor := after
	for i, w := range want {
		next, err := NextCronTime("0 0 13 * 5", "", cursor)
		if err != nil {
			t.Fatalf("fire %d: unexpected error: %v", i+1, err)
		}
		if !next.Equal(w) {
			t.Errorf("fire %d: got %v, want %v", i+1, next, w)
		}
		cursor = next
	}
}

// TestNextCronTime_DSTSpringForward_Skip verifies that a wall-clock slot
// which does not exist (spring-forward gap hour) is skipped, not fired at
// an adjusted time.
func TestNextCronTime_DSTSpringForward_Skip(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 2026-03-08 is spring-forward in America/New_York; 02:00 does not exist.
	after := time.Date(2026, 3, 7, 12, 0, 0, 0, loc)
	next, err := NextCronTime("0 2 * * *", "America/New_York", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 3, 9, 2, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v (2026-03-08 should be skipped)", next, want)
	}
}

// TestNextCronTime_DSTFallBack_FiresOnce verifies that an ambiguous
// wall-clock slot (fall-back repeated hour) fires exactly once.
func TestNextCronTime_DSTFallBack_FiresOnce(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 2026-11-01 is fall-back in America/New_York; 01:30 occurs twice.
	after := time.Date(2026, 10, 31, 12, 0, 0, 0, loc)
	fire1, err := NextCronTime("30 1 * * *", "America/New_York", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantFire1 := time.Date(2026, 11, 1, 1, 30, 0, 0, loc)
	if !fire1.Equal(wantFire1) {
		t.Fatalf("fire1: got %v, want %v", fire1, wantFire1)
	}

	fire2, err := NextCronTime("30 1 * * *", "America/New_York", fire1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantFire2 := time.Date(2026, 11, 2, 1, 30, 0, 0, loc)
	if !fire2.Equal(wantFire2) {
		t.Errorf("fire2: got %v, want %v (second 01:30 occurrence must be suppressed)", fire2, wantFire2)
	}
}

// TestNextCronTime_DSTFallBack_DenseExpression_NoDoubleFire is a regression
// test for a fall-back suppression that only re-checked the single
// replacement candidate: an expression with multiple matching slots inside
// the repeated hour must not double-fire on any of them.
func TestNextCronTime_DSTFallBack_DenseExpression_NoDoubleFire(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 2026-11-01 is fall-back in America/New_York; 01:00 and 01:30 each
	// occur twice (EDT, then EST).
	after := time.Date(2026, 10, 31, 12, 0, 0, 0, loc)

	fire1, err := NextCronTime("0,30 1 * * *", "America/New_York", after)
	if err != nil {
		t.Fatalf("fire1: unexpected error: %v", err)
	}
	wantFire1 := time.Date(2026, 11, 1, 1, 0, 0, 0, loc)
	if !fire1.Equal(wantFire1) {
		t.Fatalf("fire1: got %v, want %v", fire1, wantFire1)
	}

	fire2, err := NextCronTime("0,30 1 * * *", "America/New_York", fire1)
	if err != nil {
		t.Fatalf("fire2: unexpected error: %v", err)
	}
	wantFire2 := time.Date(2026, 11, 1, 1, 30, 0, 0, loc)
	if !fire2.Equal(wantFire2) {
		t.Fatalf("fire2: got %v, want %v", fire2, wantFire2)
	}

	// Both repeated fall-back occurrences (EST 01:00 and 01:30) must be
	// suppressed: the next fire is the following day's 01:00, not a
	// same-day repeat of either slot.
	fire3, err := NextCronTime("0,30 1 * * *", "America/New_York", fire2)
	if err != nil {
		t.Fatalf("fire3: unexpected error: %v", err)
	}
	wantFire3 := time.Date(2026, 11, 2, 1, 0, 0, 0, loc)
	if !fire3.Equal(wantFire3) {
		t.Errorf("fire3: got %v, want %v (both repeated fall-back slots must be suppressed)", fire3, wantFire3)
	}
}

// TestNextCronTime_DSTFallBack_ZeroOffsetZone is a regression test for
// disambiguation logic that assumed time.Date resolves an ambiguous wall
// clock to its pre-transition offset. That assumption does not hold in
// zones whose standard-time UTC offset is zero (e.g. Europe/London), where
// it resolves to the later, repeated occurrence instead.
func TestNextCronTime_DSTFallBack_ZeroOffsetZone(t *testing.T) {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 2026-10-25 is fall-back in Europe/London; 01:30 occurs twice (BST
	// then GMT). Build expectations from explicit UTC instants so the
	// assertions don't themselves depend on ambiguous local reconstruction.
	after := time.Date(2026, 10, 24, 12, 0, 0, 0, loc)

	fire1, err := NextCronTime("30 1 * * *", "Europe/London", after)
	if err != nil {
		t.Fatalf("fire1: unexpected error: %v", err)
	}
	// First occurrence: 01:30 BST (UTC+1) = 00:30 UTC.
	wantFire1 := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	if !fire1.Equal(wantFire1) {
		t.Fatalf("fire1: got %v (%s), want %v", fire1, fire1.In(loc), wantFire1)
	}

	fire2, err := NextCronTime("30 1 * * *", "Europe/London", fire1)
	if err != nil {
		t.Fatalf("fire2: unexpected error: %v", err)
	}
	// The second occurrence (01:30 GMT = 01:30 UTC) must be suppressed:
	// the next fire is the following day, not the same-day repeat.
	wantFire2 := time.Date(2026, 10, 26, 1, 30, 0, 0, time.UTC)
	if !fire2.Equal(wantFire2) {
		t.Errorf("fire2: got %v (%s), want %v", fire2, fire2.In(loc), wantFire2)
	}
}

// TestNextCronTime_Unsatisfiable verifies an impossible expression (Feb 30th)
// returns ErrUnsatisfiableCron instead of a silent +24h fallback.
func TestNextCronTime_Unsatisfiable(t *testing.T) {
	_, err := NextCronTime("0 0 30 2 *", "", time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for unsatisfiable expression")
	}
	if !errors.Is(err, ErrUnsatisfiableCron) {
		t.Errorf("expected ErrUnsatisfiableCron, got %v", err)
	}
}

func TestNextCronTime_InvalidTimezone(t *testing.T) {
	_, err := NextCronTime("* * * * *", "Not/AZone", time.Now())
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

// TestNextCronTime_RejectsCronTZPrefix verifies that a caller-supplied
// TZ=/CRON_TZ= prefix is rejected rather than silently accepted. robfig/cron
// strips the prefix in Parse() before any field-mask check, so without this
// guard the expression's own prefix would override the trigger's timezone
// column and bypass the DST fall-back suppression (the candidate would carry
// the column's location while the schedule actually ran in the prefix's
// zone).
func TestNextCronTime_RejectsCronTZPrefix(t *testing.T) {
	exprs := []string{
		"CRON_TZ=America/New_York 30 1 * * *",
		"TZ=Asia/Tokyo 0 9 * * *",
	}
	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			_, err := NextCronTime(expr, "UTC", time.Now())
			if err == nil {
				t.Fatalf("expected error for prefixed expression %q, got none", expr)
			}
		})
	}
}
