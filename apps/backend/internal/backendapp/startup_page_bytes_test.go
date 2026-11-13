package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/startup"
)

// @covers AC-PLATFORM-STARTUP-PROGRESS-003.14
func TestBuildStartupPageViewCountingBytes(t *testing.T) {
	cases := []struct {
		done int64
		want string
	}{
		{done: 15389753344, want: "14.3 GB"},
		// The same large value the web formatter and the embedded poll script
		// must render; see apps/backend/internal/startup/bytes_test.go.
		{done: 4504644163416883, want: "4096.9 TB"},
	}
	for _, tc := range cases {
		snap := startup.Snapshot{
			Phase: startup.BackingUpDatabase,
			Step: &startup.StepSnapshot{
				LabelKey: "startup.step.database_backup",
				Measure:  startup.MeasureCounting,
				Unit:     startup.UnitBytes,
				Done:     int64Ptr(tc.done),
			},
		}
		view := buildStartupPageView("en", snap)
		if view.Step.Progress != tc.want {
			t.Fatalf("counting byte progress for %d = %q, want %q", tc.done, view.Step.Progress, tc.want)
		}
		if view.Step.HasBar {
			t.Fatalf("counting step %d (no total) must render no bar", tc.done)
		}
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-003.14
func TestBuildStartupPageViewCountedBytesFormatsDoneAndTotalIndependently(t *testing.T) {
	snap := startup.Snapshot{
		Phase: startup.ApplyingMigrations,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.stores_repositories",
			Measure:  startup.MeasureCounted,
			Unit:     startup.UnitBytes,
			Done:     int64Ptr(1099511627776),
			Total:    int64Ptr(2199023255552),
		},
	}
	view := buildStartupPageView("en", snap)
	if view.Step.Progress != "1.0 TB of 2.0 TB" {
		t.Fatalf("counted byte progress = %q, want both sizes formatted separately", view.Step.Progress)
	}
	if !view.Step.HasBar || view.Step.PercentDoneStr != "50.00" {
		t.Fatalf("bar = %v %q, want the raw-count percentage", view.Step.HasBar, view.Step.PercentDoneStr)
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-003.14
func TestBuildStartupPageViewByteProgressIsLocalized(t *testing.T) {
	snap := startup.Snapshot{
		Phase: startup.ApplyingMigrations,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.stores_repositories",
			Measure:  startup.MeasureCounted,
			Unit:     startup.UnitBytes,
			Done:     int64Ptr(1099511627776),
			Total:    int64Ptr(2199023255552),
		},
	}
	view := buildStartupPageView("zh-cn", snap)
	if view.Step.Progress != "1.0 TB / 2.0 TB" {
		t.Fatalf("localized counted byte progress = %q, want the translated template with display sizes", view.Step.Progress)
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-003.16
func TestStartupPageCarriesByteProgressTemplates(t *testing.T) {
	translations := translationsFor("en")
	for _, key := range []string{"startup.page.countingBytes", "startup.page.countedBytes"} {
		if translations[key] == "" || translations[key] == key {
			t.Fatalf("embedded translations missing %q: %#v", key, translations)
		}
	}
}
