package routines_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routines"
)

func trig(mods ...func(*models.RoutineTrigger)) *models.RoutineTrigger {
	t := &models.RoutineTrigger{
		ID:             "t-default",
		Kind:           "cron",
		CronExpression: "*/5 * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		CreatedAt:      time.Unix(0, 0).UTC(),
	}
	for _, m := range mods {
		m(t)
	}
	return t
}

func withID(id string) func(*models.RoutineTrigger) {
	return func(t *models.RoutineTrigger) { t.ID = id }
}

func withCreatedAt(ts time.Time) func(*models.RoutineTrigger) {
	return func(t *models.RoutineTrigger) { t.CreatedAt = ts }
}

func withKind(kind string) func(*models.RoutineTrigger) {
	return func(t *models.RoutineTrigger) { t.Kind = kind }
}

func withEnabled(enabled bool) func(*models.RoutineTrigger) {
	return func(t *models.RoutineTrigger) { t.Enabled = enabled }
}

func withCron(expr, tz string) func(*models.RoutineTrigger) {
	return func(t *models.RoutineTrigger) { t.CronExpression = expr; t.Timezone = tz }
}

func withNextRunAt(ts *time.Time) func(*models.RoutineTrigger) {
	return func(t *models.RoutineTrigger) { t.NextRunAt = ts }
}

func withLastFiredAt(ts *time.Time) func(*models.RoutineTrigger) {
	return func(t *models.RoutineTrigger) { t.LastFiredAt = ts }
}

func timePtr(t time.Time) *time.Time { return &t }

// TestClassifyRoutine_Rules covers the nine-rule table of
// AC-OFFICE-ROUTINE-ARMING-001.1 in isolation, one rule per case, plus the
// precedence and boundary criteria named alongside it.
func TestClassifyRoutine_Rules(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		triggers []*models.RoutineTrigger
		want     routines.ScheduleState
	}{
		{
			name:     "rule 8: no triggers",
			triggers: nil,
			want:     routines.ScheduleStateUnscheduledNoTrigger,
		},
		{
			name: "rule 7: manual trigger only",
			triggers: []*models.RoutineTrigger{
				trig(withKind("manual")),
			},
			want: routines.ScheduleStateUnscheduledManualOnly,
		},
		{
			name: "rule 7: disabled webhook trigger, no cron trigger",
			triggers: []*models.RoutineTrigger{
				trig(withKind("webhook"), withEnabled(false)),
			},
			want: routines.ScheduleStateUnscheduledManualOnly,
		},
		{
			name: "rule 7: disabled manual trigger only (rule 7 is enabled-independent)",
			triggers: []*models.RoutineTrigger{
				trig(withKind("manual"), withEnabled(false)),
			},
			want: routines.ScheduleStateUnscheduledManualOnly,
		},
		{
			name: "rule 6: enabled webhook, no cron trigger",
			triggers: []*models.RoutineTrigger{
				trig(withKind("webhook"), withEnabled(true)),
			},
			want: routines.ScheduleStateEventOnly,
		},
		{
			name: "rule 5: cron trigger exists, none enabled",
			triggers: []*models.RoutineTrigger{
				trig(withEnabled(false)),
			},
			want: routines.ScheduleStateTriggerDisabled,
		},
		{
			name: "rule 4: enabled cron, schedulable, never fired (AC-001.11 excludes rule 2)",
			triggers: []*models.RoutineTrigger{
				trig(withNextRunAt(nil), withLastFiredAt(nil)),
			},
			want: routines.ScheduleStateTriggerUnscheduled,
		},
		{
			name: "rule 4: enabled cron, schedulable, null next_run_at, last fired outside grace",
			triggers: []*models.RoutineTrigger{
				trig(withNextRunAt(nil), withLastFiredAt(timePtr(now.Add(-2*time.Minute)))),
			},
			want: routines.ScheduleStateTriggerUnscheduled,
		},
		{
			name: "rule 3: enabled cron, not schedulable (bad expression), precedes rule 4",
			triggers: []*models.RoutineTrigger{
				trig(withCron("not a cron", "UTC"), withNextRunAt(nil)),
			},
			want: routines.ScheduleStateTriggerInvalid,
		},
		{
			name: "rule 3: enabled cron, not schedulable (bad timezone)",
			triggers: []*models.RoutineTrigger{
				trig(withCron("*/5 * * * *", "Not/AZone"), withNextRunAt(nil)),
			},
			want: routines.ScheduleStateTriggerInvalid,
		},
		{
			name: "rule 3: enabled cron, syntactically valid but impossible occurrence",
			triggers: []*models.RoutineTrigger{
				trig(withCron("0 0 30 2 *", "UTC"), withNextRunAt(nil)),
			},
			want: routines.ScheduleStateTriggerInvalid,
		},
		{
			name: "rule 2: enabled cron, schedulable, null next_run_at, within dispatch grace",
			triggers: []*models.RoutineTrigger{
				trig(withNextRunAt(nil), withLastFiredAt(timePtr(now.Add(-30*time.Second)))),
			},
			want: routines.ScheduleStateArmed,
		},
		{
			name: "rule 2: dispatch grace boundary, exactly 60s elapsed is still armed",
			triggers: []*models.RoutineTrigger{
				trig(withNextRunAt(nil), withLastFiredAt(timePtr(now.Add(-60*time.Second)))),
			},
			want: routines.ScheduleStateArmed,
		},
		{
			name: "rule 4: dispatch grace boundary, 60s plus 1ns elapsed is unscheduled",
			triggers: []*models.RoutineTrigger{
				trig(withNextRunAt(nil), withLastFiredAt(timePtr(now.Add(-60*time.Second-time.Nanosecond)))),
			},
			want: routines.ScheduleStateTriggerUnscheduled,
		},
		{
			name: "rule 1: enabled cron, next_run_at in the past (past-due is due, not broken)",
			triggers: []*models.RoutineTrigger{
				trig(withNextRunAt(timePtr(now.Add(-time.Hour)))),
			},
			want: routines.ScheduleStateArmed,
		},
		{
			name: "rule 1: enabled cron, next_run_at in the future (scheduled, not yet due)",
			triggers: []*models.RoutineTrigger{
				trig(withNextRunAt(timePtr(now.Add(time.Hour)))),
			},
			want: routines.ScheduleStateArmed,
		},
		{
			name: "rule 1 (AC-001.4): next_run_at exactly the classification instant",
			triggers: []*models.RoutineTrigger{
				trig(withNextRunAt(timePtr(now))),
			},
			want: routines.ScheduleStateArmed,
		},
		{
			name: "rule 3 precedes rule 4 when both an invalid and a merely-unscheduled enabled cron trigger exist",
			triggers: []*models.RoutineTrigger{
				trig(withID("t-unscheduled"), withNextRunAt(nil), withLastFiredAt(nil)),
				trig(withID("t-invalid"), withCron("garbage", "UTC"), withNextRunAt(nil)),
			},
			want: routines.ScheduleStateTriggerInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state, _ := routines.ClassifyRoutine(tc.triggers, now)
			if state != tc.want {
				t.Errorf("state = %q, want %q", state, tc.want)
			}
		})
	}
}

// TestClassifyRoutine_ArmedWithUnarmedSiblings covers AC-OFFICE-ROUTINE-ARMING-001.9:
// a routine may be armed on one trigger while another remains on the unarmed list.
func TestClassifyRoutine_ArmedWithUnarmedSiblings(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	armed := trig(withID("t-armed"), withNextRunAt(timePtr(now.Add(time.Hour))))
	disabled := trig(withID("t-disabled"), withEnabled(false))

	state, unarmed := routines.ClassifyRoutine([]*models.RoutineTrigger{armed, disabled}, now)

	if state != routines.ScheduleStateArmed {
		t.Fatalf("state = %q, want armed", state)
	}
	if len(unarmed) != 1 || unarmed[0].TriggerID != "t-disabled" {
		t.Fatalf("unarmed = %+v, want exactly t-disabled", unarmed)
	}
}

// TestClassifyRoutine_UnarmedReasonsOrderAndCombination covers
// AC-OFFICE-ROUTINE-ARMING-001.8: every applicable reason, in the fixed order.
func TestClassifyRoutine_UnarmedReasonsOrderAndCombination(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		trigger *models.RoutineTrigger
		want    []routines.UnarmedReason
	}{
		{
			name:    "disabled and not schedulable both apply, in order",
			trigger: trig(withEnabled(false), withCron("garbage", "UTC")),
			want:    []routines.UnarmedReason{routines.UnarmedReasonDisabled, routines.UnarmedReasonNotSchedulable},
		},
		{
			name:    "disabled only",
			trigger: trig(withEnabled(false)),
			want:    []routines.UnarmedReason{routines.UnarmedReasonDisabled},
		},
		{
			name:    "not schedulable only",
			trigger: trig(withCron("garbage", "UTC")),
			want:    []routines.UnarmedReason{routines.UnarmedReasonNotSchedulable},
		},
		{
			name:    "stalled only: enabled, schedulable, null next_run_at, past grace",
			trigger: trig(withNextRunAt(nil), withLastFiredAt(timePtr(now.Add(-5*time.Minute)))),
			want:    []routines.UnarmedReason{routines.UnarmedReasonStalled},
		},
		{
			name:    "stalled only: enabled, schedulable, null next_run_at, never fired",
			trigger: trig(withNextRunAt(nil), withLastFiredAt(nil)),
			want:    []routines.UnarmedReason{routines.UnarmedReasonStalled},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Pair with a second armed trigger so the routine's overall state
			// doesn't erase the unarmed sibling under test.
			armed := trig(withID("t-armed-sibling"), withNextRunAt(timePtr(now.Add(time.Hour))))
			_, unarmed := routines.ClassifyRoutine([]*models.RoutineTrigger{tc.trigger, armed}, now)
			if len(unarmed) != 1 {
				t.Fatalf("unarmed entries = %d, want 1: %+v", len(unarmed), unarmed)
			}
			if unarmed[0].TriggerID != tc.trigger.ID {
				t.Fatalf("unarmed trigger id = %q, want %q", unarmed[0].TriggerID, tc.trigger.ID)
			}
			if len(unarmed[0].Reasons) != len(tc.want) {
				t.Fatalf("reasons = %v, want %v", unarmed[0].Reasons, tc.want)
			}
			for i, r := range tc.want {
				if unarmed[0].Reasons[i] != r {
					t.Errorf("reasons[%d] = %q, want %q", i, unarmed[0].Reasons[i], r)
				}
			}
		})
	}
}

// TestClassifyRoutine_ArmedTriggerNotInUnarmedList proves an armed trigger
// (next_run_at set, or within grace) never appears on the unarmed list.
func TestClassifyRoutine_ArmedTriggerNotInUnarmedList(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	only := trig(withNextRunAt(timePtr(now.Add(time.Hour))))

	state, unarmed := routines.ClassifyRoutine([]*models.RoutineTrigger{only}, now)
	if state != routines.ScheduleStateArmed {
		t.Fatalf("state = %q, want armed", state)
	}
	if len(unarmed) != 0 {
		t.Fatalf("unarmed = %+v, want empty", unarmed)
	}
	if unarmed == nil {
		t.Fatal("unarmed is nil, want a non-nil empty slice so JSON marshals it as [] not null")
	}
	if got, err := json.Marshal(unarmed); err != nil {
		t.Fatalf("json.Marshal: %v", err)
	} else if string(got) != "[]" {
		t.Errorf("json.Marshal(unarmed) = %s, want []", got)
	}
}

// TestClassifyRoutine_ArmedButNotSchedulableTriggerStillReported covers the
// AC-OFFICE-ROUTINE-ARMING-001.8/001.9 pairing: a trigger that already has a
// next occurrence stays armed even if its expression or timezone can no
// longer be parsed, but unarmed-list membership is independent of armed
// state, so the same trigger is still reported as needing a fix.
func TestClassifyRoutine_ArmedButNotSchedulableTriggerStillReported(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	broken := trig(withCron("garbage", "UTC"), withNextRunAt(timePtr(now.Add(time.Hour))))

	state, unarmed := routines.ClassifyRoutine([]*models.RoutineTrigger{broken}, now)

	if state != routines.ScheduleStateArmed {
		t.Fatalf("state = %q, want armed (a claimed next occurrence still fires)", state)
	}
	if len(unarmed) != 1 || unarmed[0].TriggerID != broken.ID {
		t.Fatalf("unarmed = %+v, want exactly %q", unarmed, broken.ID)
	}
	want := []routines.UnarmedReason{routines.UnarmedReasonNotSchedulable}
	if len(unarmed[0].Reasons) != len(want) || unarmed[0].Reasons[0] != want[0] {
		t.Errorf("reasons = %v, want %v", unarmed[0].Reasons, want)
	}
}

// TestClassifyRoutine_TriggerOrder covers the "trigger order" terminology
// definition (created_at ascending, ties by id ascending) applied to the
// unarmed cron trigger list.
func TestClassifyRoutine_TriggerOrder(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Same created_at: tie broken by id ascending.
	second := trig(withID("b"), withEnabled(false), withCreatedAt(base))
	first := trig(withID("a"), withEnabled(false), withCreatedAt(base))
	third := trig(withID("c"), withEnabled(false), withCreatedAt(base.Add(time.Minute)))

	// Deliberately out of order as input.
	_, unarmed := routines.ClassifyRoutine([]*models.RoutineTrigger{third, second, first}, now)

	if len(unarmed) != 3 {
		t.Fatalf("unarmed count = %d, want 3", len(unarmed))
	}
	gotOrder := []string{unarmed[0].TriggerID, unarmed[1].TriggerID, unarmed[2].TriggerID}
	wantOrder := []string{"a", "b", "c"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Errorf("order[%d] = %q, want %q (got %v)", i, gotOrder[i], wantOrder[i], gotOrder)
		}
	}
}

// -- ClassifyRoutines (batch + per-routine fallback) --

type fakeTriggerReader struct {
	byRoutine   map[string][]*models.RoutineTrigger
	batchErr    error
	singleErr   map[string]error
	batchCalls  int
	singleCalls map[string]int
}

func newFakeTriggerReader() *fakeTriggerReader {
	return &fakeTriggerReader{
		byRoutine:   map[string][]*models.RoutineTrigger{},
		singleErr:   map[string]error{},
		singleCalls: map[string]int{},
	}
}

func (f *fakeTriggerReader) ListTriggersByRoutineIDs(
	_ context.Context, routineIDs []string,
) (map[string][]*models.RoutineTrigger, error) {
	f.batchCalls++
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	out := make(map[string][]*models.RoutineTrigger, len(routineIDs))
	for _, id := range routineIDs {
		out[id] = f.byRoutine[id]
	}
	return out, nil
}

func (f *fakeTriggerReader) ListTriggersByRoutineID(
	_ context.Context, routineID string,
) ([]*models.RoutineTrigger, error) {
	f.singleCalls[routineID]++
	if err, ok := f.singleErr[routineID]; ok {
		return nil, err
	}
	return f.byRoutine[routineID], nil
}

// TestClassifyRoutines_HappyPathIsOneBatchQuery covers
// AC-OFFICE-ROUTINE-ARMING-001.12: one batch read for the whole call.
func TestClassifyRoutines_HappyPathIsOneBatchQuery(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	reader := newFakeTriggerReader()
	reader.byRoutine["r1"] = []*models.RoutineTrigger{trig(withNextRunAt(timePtr(now.Add(time.Hour))))}
	reader.byRoutine["r2"] = nil

	results, err := routines.ClassifyRoutines(context.Background(), reader, []string{"r1", "r2"}, now)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if reader.batchCalls != 1 {
		t.Fatalf("batch calls = %d, want 1", reader.batchCalls)
	}
	if len(reader.singleCalls) != 0 {
		t.Fatalf("single calls = %v, want none on the happy path", reader.singleCalls)
	}
	if results["r1"].State != routines.ScheduleStateArmed {
		t.Errorf("r1 state = %q, want armed", results["r1"].State)
	}
	if results["r2"].State != routines.ScheduleStateUnscheduledNoTrigger {
		t.Errorf("r2 state = %q, want unscheduled_no_trigger", results["r2"].State)
	}
}

// TestClassifyRoutines_BatchFailureFallsBackPerRoutine covers
// AC-OFFICE-ROUTINE-ARMING-001.13: a batch failure re-reads each named
// routine individually, once each, and only the routines whose individual
// re-read also fails become unknown.
func TestClassifyRoutines_BatchFailureFallsBackPerRoutine(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	reader := newFakeTriggerReader()
	reader.batchErr = errors.New("batch decode failure")
	reader.byRoutine["r-ok"] = []*models.RoutineTrigger{trig(withNextRunAt(timePtr(now.Add(time.Hour))))}
	reader.singleErr["r-bad"] = errors.New("row undecodable")

	results, err := routines.ClassifyRoutines(context.Background(), reader, []string{"r-ok", "r-bad"}, now)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if reader.singleCalls["r-ok"] != 1 {
		t.Errorf("r-ok single-read calls = %d, want 1", reader.singleCalls["r-ok"])
	}
	if reader.singleCalls["r-bad"] != 1 {
		t.Errorf("r-bad single-read calls = %d, want 1", reader.singleCalls["r-bad"])
	}
	if results["r-ok"].State != routines.ScheduleStateArmed {
		t.Errorf("r-ok state = %q, want armed (its own re-read succeeded)", results["r-ok"].State)
	}
	if results["r-bad"].State != routines.ScheduleStateUnknown {
		t.Errorf("r-bad state = %q, want unknown", results["r-bad"].State)
	}
}

// TestClassifyRoutines_SharesOneInstant covers the second half of
// AC-OFFICE-ROUTINE-ARMING-001.12: the per-routine fallback reuses the
// batch call's captured instant rather than recapturing it.
func TestClassifyRoutines_SharesOneInstant(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	reader := newFakeTriggerReader()
	reader.batchErr = errors.New("batch failure")
	// Within grace of `now`, but would be outside grace of any later instant
	// a buggy re-capture might use.
	reader.byRoutine["r1"] = []*models.RoutineTrigger{
		trig(withNextRunAt(nil), withLastFiredAt(timePtr(now.Add(-59*time.Second)))),
	}

	results, err := routines.ClassifyRoutines(context.Background(), reader, []string{"r1"}, now)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if results["r1"].State != routines.ScheduleStateArmed {
		t.Errorf("r1 state = %q, want armed (classified against the original instant)", results["r1"].State)
	}
}
