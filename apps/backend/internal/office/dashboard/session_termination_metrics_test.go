package dashboard_test

import (
	"context"
	"expvar"
	"strings"
	"testing"
)

// suppressionCounts snapshots the expvar map as label -> count.
func suppressionCounts(t *testing.T) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	v := expvar.Get("office_session_term_suppressed_total")
	if v == nil {
		t.Fatal("office_session_term_suppressed_total is not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("office_session_term_suppressed_total is %T, want *expvar.Map", v)
	}
	m.Do(func(kv expvar.KeyValue) {
		iv, ok := kv.Value.(*expvar.Int)
		if !ok {
			t.Errorf("label %q holds %T, want *expvar.Int", kv.Key, kv.Value)
			return
		}
		out[kv.Key] = iv.Value()
	})
	return out
}

func delta(before, after map[string]int64, label string) int64 {
	return after[label] - before[label]
}

// AC-OFFICE-SESSION-TERM-004.3: every suppression is counted, and the two
// suppression causes are counted apart.
func TestSuppressedTermination_IsCounted(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-metric")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-metric", "agent-dual"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	if err := deps.repo.UpdateTaskAssignee(ctx, "task-metric", "agent-dual"); err != nil {
		t.Fatalf("set runner: %v", err)
	}

	before := suppressionCounts(t)
	if got := removeReviewer(t, deps, rt, "task-metric", "agent-dual"); got != 0 {
		t.Fatalf("expected a suppression, got %d terminations", got)
	}
	// A task that does not resolve fails the determination instead.
	if err := deps.svc.RemoveTaskReviewer(ctx, "", "task-gone", "agent-dual"); err != nil {
		t.Fatalf("remove on unresolvable task: %v", err)
	}
	after := suppressionCounts(t)

	retained := "reason=participant_removed;outcome=runner"
	failed := "reason=participant_removed;outcome=read_failed"
	if got := delta(before, after, retained); got != 1 {
		t.Errorf("retained-capacity suppressions: got %d, want 1", got)
	}
	if got := delta(before, after, failed); got != 1 {
		t.Errorf("read-failure suppressions: got %d, want 1", got)
	}
}

// AC-OFFICE-SESSION-TERM-004.3 (second sentence): labels come from closed sets.
// An identifier reaching a label would grow the map without bound, so this
// scans every label the whole package's tests produced.
func TestSuppressionLabels_AreBounded(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-labels")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-labels", "agent-bounded"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	if err := deps.repo.UpdateTaskAssignee(ctx, "task-labels", "agent-bounded"); err != nil {
		t.Fatalf("set runner: %v", err)
	}
	if got := removeReviewer(t, deps, rt, "task-labels", "agent-bounded"); got != 0 {
		t.Fatalf("expected a suppression, got %d terminations", got)
	}

	reasons := map[string]bool{
		"task_reassigned": true, "participant_removed": true, "participant_seat_claimed": true,
	}
	outcomes := map[string]bool{"runner": true, "seat": true, "read_failed": true}

	labels := suppressionCounts(t)
	if len(labels) == 0 {
		t.Fatal("no labels recorded: the scan below would pass without inspecting anything")
	}
	for label := range labels {
		if strings.Contains(label, "task-labels") || strings.Contains(label, "agent-bounded") {
			t.Errorf("label %q carries an identifier", label)
		}
		fields := map[string]string{}
		for _, pair := range strings.Split(label, ";") {
			k, v, ok := strings.Cut(pair, "=")
			if !ok {
				t.Errorf("label %q is not a k=v;k=v string", label)
				continue
			}
			fields[k] = v
		}
		if len(fields) != 2 {
			t.Errorf("label %q: got %d dimensions, want 2", label, len(fields))
		}
		if !reasons[fields["reason"]] {
			t.Errorf("label %q: reason %q is outside the closed set", label, fields["reason"])
		}
		if !outcomes[fields["outcome"]] {
			t.Errorf("label %q: outcome %q is outside the closed set", label, fields["outcome"])
		}
	}
}
