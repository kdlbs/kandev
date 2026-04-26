package service

import (
	"testing"
	"time"
)

func TestNextCronTick_EveryMinute(t *testing.T) {
	after := time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC)
	next, err := nextCronTick("* * * * *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 25, 10, 31, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextCronTick_DailyAt9(t *testing.T) {
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	next, err := nextCronTick("0 9 * * *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 26, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextCronTick_MondayAt9(t *testing.T) {
	// 2026-04-25 is a Saturday
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	next, err := nextCronTick("0 9 * * 1", "", after)
	if err != nil {
		t.Fatal(err)
	}
	// Next Monday is 2026-04-27
	want := time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v (weekday=%s), want %v", next, next.Weekday(), want)
	}
}

func TestNextCronTick_WithTimezone(t *testing.T) {
	// 12:00 UTC = 8:00 AM EDT, so next 9am EDT is same day at 13:00 UTC.
	after := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	next, err := nextCronTick("0 9 * * *", "America/New_York", after)
	if err != nil {
		t.Fatal(err)
	}
	// 9am EDT on Apr 25 = 13:00 UTC
	want := time.Date(2026, 4, 25, 13, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextCronTick_InvalidExpression(t *testing.T) {
	_, err := nextCronTick("invalid", "", time.Now())
	if err == nil {
		t.Error("expected error for invalid expression")
	}
}

func TestNextCronTick_StepExpression(t *testing.T) {
	after := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	next, err := nextCronTick("*/15 * * * *", "", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 25, 10, 15, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestParseCronField_Wildcard(t *testing.T) {
	vals, err := parseCronField("*", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 60 {
		t.Errorf("expected 60 values, got %d", len(vals))
	}
}

func TestParseCronField_Range(t *testing.T) {
	vals, err := parseCronField("1-5", 0, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 5 {
		t.Errorf("expected 5 values, got %d", len(vals))
	}
}

func TestParseCronField_CommaList(t *testing.T) {
	vals, err := parseCronField("1,3,5", 0, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 3 {
		t.Errorf("expected 3 values, got %d", len(vals))
	}
}
