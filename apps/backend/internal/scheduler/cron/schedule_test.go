package cron

import (
	"testing"
	"time"
)

func TestNextScheduleTimeHonorsTimezoneAndDescriptors(t *testing.T) {
	after := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	got, err := NextScheduleTime("0 9 * * *", "America/New_York", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("next = %s, want %s", got, want)
	}

	got, err = NextScheduleTime("@hourly", "UTC", after)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 7, 16, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("descriptor next = %s, want %s", got, want)
	}
}
