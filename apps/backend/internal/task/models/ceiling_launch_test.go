package models

import "testing"

func ceilingTask(record map[string]interface{}) *Task {
	return &Task{Metadata: map[string]interface{}{MetaKeyDeferredLaunch: record}}
}

func TestIsCeilingRecordKeyPartitionsByPrefix(t *testing.T) {
	for _, key := range []string{CeilingDeferredKey, CeilingQueuedAtKey, CeilingLaunchKindKey,
		CeilingLaunchPayloadKey, CeilingLaunchOriginKey, CeilingReasonCodeKey,
		CeilingSurfaceWrittenAtKey, CeilingSurfaceAttemptCountKey, "ceiling_added_later"} {
		if !IsCeilingRecordKey(key) {
			t.Errorf("IsCeilingRecordKey(%q) = false, want true", key)
		}
	}
	for _, key := range []string{DeferredLaunchStartWhenUnblockedKey, DeferredLaunchUserIDKey,
		DeferredLaunchRecordRecentUseKey, "added_later"} {
		if IsCeilingRecordKey(key) {
			t.Errorf("IsCeilingRecordKey(%q) = true, want false: it is not the ceiling's to touch", key)
		}
	}
}

func TestHasCeilingDeferredIntent(t *testing.T) {
	for _, tc := range []struct {
		name string
		task *Task
		want bool
	}{
		{name: "nil task", task: nil, want: false},
		{name: "no metadata", task: &Task{}, want: false},
		{name: "no record", task: &Task{Metadata: map[string]interface{}{}}, want: false},
		{name: "record is not an object", task: &Task{Metadata: map[string]interface{}{MetaKeyDeferredLaunch: "scalar"}}, want: false},
		{name: "wip record only", task: ceilingTask(map[string]interface{}{DeferredLaunchUserIDKey: "u1"}), want: false},
		{name: "flag false", task: ceilingTask(map[string]interface{}{CeilingDeferredKey: false}), want: false},
		{name: "flag true", task: ceilingTask(map[string]interface{}{CeilingDeferredKey: true}), want: true},
		{name: "alongside a chain intent", task: ceilingTask(map[string]interface{}{
			CeilingDeferredKey:                  true,
			DeferredLaunchStartWhenUnblockedKey: true,
		}), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasCeilingDeferredIntent(tc.task); got != tc.want {
				t.Errorf("HasCeilingDeferredIntent = %v, want %v", got, tc.want)
			}
		})
	}
}

// An unrelated WIP admission must not delete a ceiling deferral. Left unguarded it
// drops a launch that is never retried and never logged.
func TestDropWIPDeferredLaunchLeavesACeilingDeferral(t *testing.T) {
	task := ceilingTask(map[string]interface{}{CeilingDeferredKey: true, CeilingLaunchKindKey: "start"})
	DropWIPDeferredLaunch(task)
	if _, present := task.Metadata[MetaKeyDeferredLaunch]; !present {
		t.Fatal("DropWIPDeferredLaunch deleted a ceiling deferral")
	}
}

func TestDropWIPDeferredLaunchStillGuardsAChainIntentAndStillDropsAPlainWIPRecord(t *testing.T) {
	chain := ceilingTask(map[string]interface{}{DeferredLaunchStartWhenUnblockedKey: true})
	DropWIPDeferredLaunch(chain)
	if _, present := chain.Metadata[MetaKeyDeferredLaunch]; !present {
		t.Fatal("DropWIPDeferredLaunch deleted a chain intent")
	}

	wip := ceilingTask(map[string]interface{}{DeferredLaunchUserIDKey: "u1"})
	DropWIPDeferredLaunch(wip)
	if _, present := wip.Metadata[MetaKeyDeferredLaunch]; present {
		t.Fatal("DropWIPDeferredLaunch left a plain WIP record in place: existing behaviour changed")
	}
}

// The replay payload carries the launch-scoped environment, the composed prompt and
// its attachments, so it must not reach an API client. The discriminators and
// bookkeeping stay projected: a client can still see that a deferral is pending, of
// which kind, since when and why.
func TestPublicTaskMetadataRedactsTheCeilingPayloadButKeepsTheDiscriminators(t *testing.T) {
	metadata := map[string]interface{}{
		MetaKeyDeferredLaunch: map[string]interface{}{
			DeferredLaunchUserIDKey:          "u1",
			DeferredLaunchRecordRecentUseKey: true,
			CeilingDeferredKey:               true,
			CeilingLaunchKindKey:             "start",
			CeilingQueuedAtKey:               "2026-09-11T00:00:00Z",
			CeilingReasonCodeKey:             "ceiling",
			CeilingSurfaceWrittenAtKey:       "2026-09-11T00:00:01Z",
			CeilingSurfaceAttemptCountKey:    2,
			CeilingLaunchPayloadKey: map[string]interface{}{
				"env":    map[string]interface{}{"SECRET_TOKEN": "hunter2"},
				"prompt": "the composed prompt",
			},
		},
	}

	public := PublicTaskMetadata(metadata)
	deferred, ok := public[MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatal("public projection lost the deferred_launch record")
	}
	if _, leaked := deferred[CeilingLaunchPayloadKey]; leaked {
		t.Error("public projection leaked the ceiling launch payload")
	}
	for _, key := range []string{CeilingDeferredKey, CeilingLaunchKindKey, CeilingQueuedAtKey,
		CeilingReasonCodeKey, CeilingSurfaceWrittenAtKey, CeilingSurfaceAttemptCountKey} {
		if _, present := deferred[key]; !present {
			t.Errorf("public projection dropped %q, which AC-22(a)'s observable depends on", key)
		}
	}

	// The projection is detached: redacting must not mutate the caller's map.
	original, _ := metadata[MetaKeyDeferredLaunch].(map[string]interface{})
	if _, present := original[CeilingLaunchPayloadKey]; !present {
		t.Error("PublicTaskMetadata mutated the source record instead of projecting a copy")
	}
}
