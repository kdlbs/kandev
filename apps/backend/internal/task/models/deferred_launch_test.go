package models

import "testing"

func launchTask(record map[string]interface{}) *Task {
	return &Task{Metadata: map[string]interface{}{MetaKeyDeferredLaunch: record}}
}

// A WIP-overflow record is stale the moment the task is admitted, so admission
// drops it. That has been true since the WIP queue shipped.
func TestDropWIPDeferredLaunchRemovesAnOverflowRecord(t *testing.T) {
	task := launchTask(map[string]interface{}{"intent": "start"})

	DropWIPDeferredLaunch(task)

	if _, ok := task.Metadata[MetaKeyDeferredLaunch]; ok {
		t.Fatal("an admitted task must not keep its WIP overflow launch record")
	}
}

// The regression this helper exists for. A dependency chain step is admitted at
// create time like any other task, so deleting the shared record on admission
// erased the intent before a predecessor could ever resolve it, and the whole
// chain silently never ran. The record is waiting on a predecessor, not on
// capacity, so admission has nothing to say about it.
func TestDropWIPDeferredLaunchKeepsAStartWhenUnblockedIntent(t *testing.T) {
	task := launchTask(map[string]interface{}{
		"intent":                            "start",
		DeferredLaunchStartWhenUnblockedKey: true,
	})

	DropWIPDeferredLaunch(task)

	if !HasStartWhenUnblockedIntent(task) {
		t.Fatal("WIP admission must not consume a dependency-chain launch intent")
	}
}

func TestDropWIPDeferredLaunchToleratesEmptyTasks(t *testing.T) {
	DropWIPDeferredLaunch(nil)
	DropWIPDeferredLaunch(&Task{})
	DropWIPDeferredLaunch(&Task{Metadata: map[string]interface{}{}})
}
