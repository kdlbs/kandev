package dashboard

import (
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
)

func TestReadLoopCounters_FiltersToRequestingWorkspace(t *testing.T) {
	wsA := "ws-counters-a"
	wsB := "ws-counters-b"
	service.IncLoopRunClaimed(wsA)
	service.IncLoopRunClaimed(wsB)

	resp := ReadLoopCounters(wsA)

	got := resp.Workspace["office_loop_run_claimed_total"]
	if _, ok := got[service.LoopMetricLabel("workspace", wsB)]; ok {
		t.Errorf("workspace block contains another workspace's key: %+v", got)
	}
	if v, ok := got[service.LoopMetricLabel("workspace", wsA)]; !ok || v < 1 {
		t.Errorf("workspace block missing own key, got %+v", got)
	}
}

func TestReadLoopCounters_UnobservedCounterIsZeroFilledNotOmitted(t *testing.T) {
	resp := ReadLoopCounters("ws-counters-never-seen")

	for _, name := range loopCounterMapNames {
		if _, ok := resp.Workspace[name]; !ok {
			t.Errorf("counter %q missing from workspace block, want zero-filled empty map", name)
		}
	}
}

func TestReadLoopCounters_UnattributedAndProcessInstantsPresent(t *testing.T) {
	service.IncLoopTriggerClaimed(service.LoopUnattributedWorkspace)
	service.IncLoopCronTick("2026-05-01T12:00:00Z")
	service.RecordLoopProcessStarted("2026-05-01T11:00:00Z")

	resp := ReadLoopCounters("ws-counters-some-other-workspace")

	unattr := resp.Process.Unattributed["office_loop_trigger_claimed_total"]
	if _, ok := unattr[service.LoopMetricLabel("workspace", service.LoopUnattributedWorkspace)]; !ok {
		t.Errorf("unattributed bucket missing entry, got %+v", unattr)
	}
	if _, ok := resp.Workspace["office_loop_trigger_claimed_total"][service.LoopMetricLabel("workspace", service.LoopUnattributedWorkspace)]; ok {
		t.Errorf("unattributed key leaked into workspace block: %+v", resp.Workspace["office_loop_trigger_claimed_total"])
	}
	if resp.Process.CronTickTotal < 1 {
		t.Errorf("cron tick total = %d, want >= 1", resp.Process.CronTickTotal)
	}
	if resp.Process.CronTickAt == "" {
		t.Error("cron tick at is empty, want the published instant")
	}
	if resp.Process.ProcessStartedAt == "" {
		t.Error("process started at is empty, want the published instant")
	}
}

func TestReadLoopCounters_MalformedKeyCountsIntoMalformedBucket(t *testing.T) {
	m := expvar.Get("office_loop_run_claimed_total").(*expvar.Map)
	m.Add("not-a-valid-label", 1)

	resp := ReadLoopCounters("ws-counters-malformed-probe")

	malformed := resp.Process.Malformed["office_loop_run_claimed_total"]
	if v, ok := malformed["not-a-valid-label"]; !ok || v < 1 {
		t.Errorf("malformed bucket missing key, got %+v", malformed)
	}
	if _, ok := resp.Workspace["office_loop_run_claimed_total"]["not-a-valid-label"]; ok {
		t.Error("malformed key leaked into workspace block")
	}
}
