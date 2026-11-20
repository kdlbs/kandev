package launcher

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/startup"
)

func testInt64(v int64) *int64 { return &v }

func testFloat64(v float64) *float64 { return &v }

// TestFormatStartupLineByteUnits covers AC-PLATFORM-STARTUP-PROGRESS-003.14 and
// 003.15 for the terminal: a byte step renders readable sizes instead of raw
// counts, and a byte rate gains a scaled unit. Non-byte steps keep their raw
// counts and bare rate (see TestFormatStartupLineCountingStepWithNoTotal).
func TestFormatStartupLineByteUnits(t *testing.T) {
	cases := []struct {
		name string
		step *startup.StepSnapshot
		want string
	}{
		{
			name: "counting bytes with a scaled rate",
			step: &startup.StepSnapshot{
				LabelKey: "startup.step.database_backup", Measure: startup.MeasureCounting, Unit: startup.UnitBytes,
				Done: testInt64(15389753344), RatePerSecond: testFloat64(1048576),
			},
			want: ": Database backup: 14.3 GB (1.0 MB/s)",
		},
		{
			name: "counted bytes format done and total independently",
			step: &startup.StepSnapshot{
				LabelKey: "startup.step.database_backup", Measure: startup.MeasureCounted, Unit: startup.UnitBytes,
				Done: testInt64(1048576), Total: testInt64(15389753344),
			},
			want: ": Database backup: 1.0 MB/14.3 GB",
		},
		{
			name: "zero rate keeps a scaled unit",
			step: &startup.StepSnapshot{
				LabelKey: "startup.step.database_backup", Measure: startup.MeasureCounting, Unit: startup.UnitBytes,
				Done: testInt64(0), RatePerSecond: testFloat64(0),
			},
			want: ": Database backup: 0 B (0.0 B/s)",
		},
		{
			name: "absent rate stays absent",
			step: &startup.StepSnapshot{
				LabelKey: "startup.step.database_backup", Measure: startup.MeasureCounting, Unit: startup.UnitBytes,
				Done: testInt64(1280),
			},
			want: ": Database backup: 1.3 KB",
		},
		{
			name: "large count keeps the shared rounding down",
			step: &startup.StepSnapshot{
				LabelKey: "startup.step.database_backup", Measure: startup.MeasureCounting, Unit: startup.UnitBytes,
				Done: testInt64(4504644163416883),
			},
			want: ": Database backup: 4096.9 TB",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := formatStartupLine(startup.Snapshot{
				Phase: startup.BackingUpDatabase, ElapsedMS: 5000, PhaseElapsedMS: 5000, Step: tc.step,
			})
			if !strings.Contains(line, tc.want) {
				t.Fatalf("formatStartupLine() = %q, want it to contain %q", line, tc.want)
			}
		})
	}
}

// TestFormatStartupLineNonByteUnitsStayRaw keeps the AC-PLATFORM-STARTUP-
// PROGRESS-003.16 regression boundary: only `bytes` is reformatted, so a
// session count and its rate keep their existing rendering.
func TestFormatStartupLineNonByteUnitsStayRaw(t *testing.T) {
	line := formatStartupLine(startup.Snapshot{
		Phase: startup.RecoveringSessions, ElapsedMS: 1000, PhaseElapsedMS: 1000,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.runtime_inventory", Measure: startup.MeasureCounted, Unit: startup.UnitSessions,
			Done: testInt64(12), Total: testInt64(50), RatePerSecond: testFloat64(2),
		},
	})
	if !strings.Contains(line, ": Runtime inventory: 12/50 (2.0/s)") {
		t.Fatalf("formatStartupLine() = %q, want raw counts and a unitless rate", line)
	}
}
