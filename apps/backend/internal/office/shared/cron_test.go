package shared

import (
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

func TestParseCronField_Wildcard(t *testing.T) {
	vals, err := parseCronField("*", 0, 59, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 60 {
		t.Errorf("expected 60 values, got %d", len(vals))
	}
}

func TestParseCronField_Range(t *testing.T) {
	vals, err := parseCronField("1-5", 0, 6, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 5 {
		t.Errorf("expected 5 values, got %d", len(vals))
	}
}

func TestParseCronField_CommaList(t *testing.T) {
	vals, err := parseCronField("1,3,5", 0, 6, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 3 {
		t.Errorf("expected 3 values, got %d", len(vals))
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
