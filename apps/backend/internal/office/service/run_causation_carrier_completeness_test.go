package service

// TestCarrierSetCompleteness pins AC-OFFICE-RUN-CAUSATION-001.18: the
// task-boundary causation carrier is a fixed 7-value set (causation id,
// causation depth, creating run id, human-rooted flag, routine
// attribution, actor kind, actor identifier) that must survive a task
// creation boundary intact. carrierKeys (used by carrierPresent to detect
// whether a task carries a carrier at all), TaskBoundaryCarrier (the
// read-side decode target), and carrierMetadataFromRun (the write side)
// are three independent, hand-maintained representations of that same
// set — nothing in the Go type system keeps them in sync. This test
// fails the day one of the three drifts from the other two, e.g. a
// carrier value added to TaskBoundaryCarrier without a matching entry in
// carrierKeys, or a write-side key carrierFromTaskMetadata never learns
// to decode.
//
// The remaining half of AC.18 — that every enqueue path (QueueRunFromTaskBoundary
// -> runs/service.QueueRunRequest -> applyCausationLineage) actually
// threads each TaskBoundaryCarrier field through rather than dropping it
// silently — is exercised end-to-end by
// run_causation_from_task_test.go's real-repo tests, not by this file.

import (
	"reflect"
	"sort"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestCarrierSetCompleteness_KeysMatchStructFieldCount(t *testing.T) {
	numFields := reflect.TypeOf(TaskBoundaryCarrier{}).NumField()
	if len(carrierKeys) != numFields {
		t.Fatalf("carrierKeys has %d entries, but TaskBoundaryCarrier has %d fields — "+
			"a carrier value was added to one without the other", len(carrierKeys), numFields)
	}
}

func TestCarrierSetCompleteness_CarrierKeysHaveNoDuplicates(t *testing.T) {
	seen := make(map[string]bool, len(carrierKeys))
	for _, key := range carrierKeys {
		if seen[key] {
			t.Fatalf("carrierKeys contains duplicate entry %q", key)
		}
		seen[key] = true
	}
}

func TestCarrierSetCompleteness_WriteSideKeySetMatchesCarrierKeys(t *testing.T) {
	run := &models.Run{
		ID:             "run-completeness-1",
		CausationID:    "causation-completeness-1",
		CausationDepth: 3,
		HumanRooted:    true,
		RoutineID:      "routine-completeness-1",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-completeness-1",
	}
	written := carrierMetadataFromRun(run)

	writtenKeys := make([]string, 0, len(written))
	for key := range written {
		writtenKeys = append(writtenKeys, key)
	}
	wantKeys := append([]string(nil), carrierKeys...)
	sort.Strings(writtenKeys)
	sort.Strings(wantKeys)

	if !reflect.DeepEqual(writtenKeys, wantKeys) {
		t.Fatalf("carrierMetadataFromRun wrote keys %v, want exactly carrierKeys %v — "+
			"the write side and the presence-detection key list have drifted apart",
			writtenKeys, wantKeys)
	}
}

func TestCarrierSetCompleteness_ReadSideDecodesEveryWrittenKey(t *testing.T) {
	run := &models.Run{
		ID:             "run-completeness-2",
		CausationID:    "causation-completeness-2",
		CausationDepth: 5,
		HumanRooted:    true,
		RoutineID:      "routine-completeness-2",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-completeness-2",
	}
	written := carrierMetadataFromRun(run)

	// carrierFromTaskMetadata expects JSON-shaped values (metadata
	// round-trips through JSON before this function ever sees it), so
	// mirror that for the one field whose Go and JSON types diverge.
	written[taskmodels.MetaKeyOfficeCarrierCausationDepth] = float64(run.CausationDepth)

	decoded := carrierFromTaskMetadata(written)
	want := TaskBoundaryCarrier{
		CausationID:    run.CausationID,
		CausationDepth: run.CausationDepth,
		CreatingRunID:  run.ID,
		HumanRooted:    run.HumanRooted,
		RoutineID:      run.RoutineID,
		ActorKind:      run.ActorKind,
		ActorID:        run.ActorID,
	}
	if decoded != want {
		t.Fatalf("carrierFromTaskMetadata(carrierMetadataFromRun(run)) = %+v, want %+v — "+
			"a value carrierMetadataFromRun writes is not reaching TaskBoundaryCarrier intact",
			decoded, want)
	}
}
