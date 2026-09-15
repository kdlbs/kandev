package dashboard

import (
	"expvar"
	"strings"

	"github.com/kandev/kandev/internal/office/service"
)

// loopCounterMapNames lists every office_loop_* expvar.Map counter
// name (REQ-OFFICE-LOOP-LIVENESS-003, "Counters" table). Zero-filled
// at counter-name granularity (AC-003.8's malformed-key note and the
// design's "Zero-filling is at counter-name granularity" rule): a
// name absent from a reader's response would be indistinguishable
// from "never observed".
var loopCounterMapNames = []string{
	"office_loop_trigger_claimed_total",
	"office_loop_routine_run_total",
	"office_loop_wakeup_created_total",
	"office_loop_run_claimed_total",
	"office_loop_launch_total",
	"office_loop_launch_without_session_total",
	"office_loop_session_persist_failed_total",
	"office_loop_terminal_total",
	"office_loop_last_run_at_write_failed_total",
	"office_loop_liveness_degraded_total",
}

// LoopCountersResponse is the GET .../loop-counters payload. workspace
// carries every counter above labelled to the requesting workspace
// (secondary labels like source/disposition/shape/reason preserved,
// but only as observed); process carries the cron-tick counter, the
// two published instants, and the _unattributed totals — identical
// for every reader, no workspace's data (AC-003.5, AC-003.8).
type LoopCountersResponse struct {
	Workspace map[string]map[string]int64 `json:"workspace"`
	Process   LoopCountersProcessDTO      `json:"process"`
}

// LoopCountersProcessDTO is the process-scoped block of the counter
// read: never filtered by workspace, identical for every caller.
type LoopCountersProcessDTO struct {
	CronTickTotal    int64                       `json:"cron_tick_total"`
	CronTickAt       string                      `json:"cron_tick_at"`
	ProcessStartedAt string                      `json:"process_started_at"`
	Unattributed     map[string]map[string]int64 `json:"unattributed"`
	Malformed        map[string]map[string]int64 `json:"malformed"`
}

// ReadLoopCounters parses every office_loop_* expvar.Map back into a
// workspace-filtered, zero-filled response (AC-003.4, AC-003.5,
// AC-003.7). A key that fails to parse into the expected "k=v;k=v"
// label shape lands in the process block's malformed bucket rather
// than being dropped or attributed to any workspace, the same
// treatment AC-003.8 gives an unresolved workspace.
func ReadLoopCounters(workspaceID string) LoopCountersResponse {
	resp := LoopCountersResponse{
		Workspace: make(map[string]map[string]int64, len(loopCounterMapNames)),
		Process: LoopCountersProcessDTO{
			Unattributed: make(map[string]map[string]int64),
			Malformed:    make(map[string]map[string]int64),
		},
	}
	for _, name := range loopCounterMapNames {
		resp.Workspace[name] = map[string]int64{}
	}

	if v := expvar.Get("office_loop_cron_tick_total"); v != nil {
		if m, ok := v.(*expvar.Map); ok {
			resp.Process.CronTickTotal = expvarMapTotal(m)
		}
	}
	if v := expvar.Get("office_loop_cron_tick_at"); v != nil {
		resp.Process.CronTickAt = v.String()
		resp.Process.CronTickAt = strings.Trim(resp.Process.CronTickAt, `"`)
	}
	if v := expvar.Get("office_loop_process_started_at"); v != nil {
		resp.Process.ProcessStartedAt = strings.Trim(v.String(), `"`)
	}

	for _, name := range loopCounterMapNames {
		v := expvar.Get(name)
		if v == nil {
			continue
		}
		m, ok := v.(*expvar.Map)
		if !ok {
			continue
		}
		partitionLoopCounterMap(&resp, name, m, workspaceID)
	}
	return resp
}

// partitionLoopCounterMap walks one counter's labelled entries and
// routes each into the workspace block, the process unattributed
// bucket, or the process malformed bucket.
func partitionLoopCounterMap(resp *LoopCountersResponse, mapName string, m *expvar.Map, workspaceID string) {
	m.Do(func(kv expvar.KeyValue) {
		iv, ok := kv.Value.(*expvar.Int)
		if !ok {
			return
		}
		labels, ok := parseLoopCounterKey(kv.Key)
		if !ok {
			addLoopCounterBucket(resp.Process.Malformed, mapName, kv.Key, iv.Value())
			return
		}
		ws, hasWorkspace := labels["workspace"]
		switch {
		case hasWorkspace && ws == workspaceID:
			resp.Workspace[mapName][kv.Key] = iv.Value()
		case hasWorkspace && ws == service.LoopUnattributedWorkspace:
			addLoopCounterBucket(resp.Process.Unattributed, mapName, kv.Key, iv.Value())
		case hasWorkspace:
			// A different workspace's value — filtered out entirely
			// (AC-003.5): not returned in any form to this reader.
		default:
			// No workspace label at all (process-only counter reached
			// via this generic loop, or a future label-less counter) —
			// treat as unattributed rather than silently dropped.
			addLoopCounterBucket(resp.Process.Unattributed, mapName, kv.Key, iv.Value())
		}
	})
}

func addLoopCounterBucket(bucket map[string]map[string]int64, mapName, key string, value int64) {
	if bucket[mapName] == nil {
		bucket[mapName] = map[string]int64{}
	}
	bucket[mapName][key] = value
}

// parseLoopCounterKey parses the "k1=v1;k2=v2" expvar map key back
// into labels. Total parse (AC-003.8's argument extended to this
// read): any segment that doesn't split into exactly one "=" fails
// the whole key rather than silently dropping that label.
func parseLoopCounterKey(key string) (map[string]string, bool) {
	if key == "" {
		return map[string]string{}, true
	}
	labels := make(map[string]string)
	for _, part := range strings.Split(key, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 || kv[0] == "" {
			return nil, false
		}
		labels[kv[0]] = kv[1]
	}
	return labels, true
}

func expvarMapTotal(m *expvar.Map) int64 {
	var total int64
	m.Do(func(kv expvar.KeyValue) {
		if iv, ok := kv.Value.(*expvar.Int); ok {
			total += iv.Value()
		}
	})
	return total
}
