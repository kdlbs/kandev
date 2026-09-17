package service

// TestCarrierSetCompleteness pins AC-OFFICE-RUN-CAUSATION-001.18: the
// task-boundary causation carrier is a fixed 7-value set (causation id,
// causation depth, creating run id, human-rooted flag, routine
// attribution, actor kind, actor identifier) that must survive a task
// creation boundary intact. carrierKeys (used by carrierPresent to detect
// whether a task carries a carrier at all), TaskBoundaryCarrier (the
// read-side decode target), and carrierMetadataFromRun (the write side)
// are three independent, hand-maintained representations of that same
// set — nothing in the Go type system keeps them in sync. The first four
// tests below fail the day one of those three drifts from the other two,
// e.g. a carrier value added to TaskBoundaryCarrier without a matching
// entry in carrierKeys, or a write-side key carrierFromTaskMetadata never
// learns to decode.
//
// Those three representations could still drift from the AC in lockstep
// (all three losing the same value together) and pass every one of those
// checks. TestCarrierSetCompleteness_EveryACNamedValueIsCarriedOrDocumentedRederivable
// closes that gap by checking against runCausationNamedValues, a table
// hand-maintained against the spec text (AC-OFFICE-RUN-CAUSATION-001.1,
// .13, .14, .19, .20) rather than against the carrier's own
// representations.
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
		ID:               "run-completeness-1",
		ChainCausationID: "causation-completeness-1",
		CausationDepth:   3,
		HumanRooted:      true,
		RoutineID:        "routine-completeness-1",
		ActorKind:        models.ActorKindAgent,
		ActorID:          "agent-completeness-1",
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

// runCausationNamedValue captures one persisted run value named by
// AC-OFFICE-RUN-CAUSATION-001.1, .13, .14, .19, or .20 — the closed set
// AC-OFFICE-RUN-CAUSATION-001.18 requires every value to be checked
// against, independent of carrierKeys/TaskBoundaryCarrier/
// carrierMetadataFromRun themselves (the three tests above only
// cross-check those three representations against each other, so all
// three could drift from the AC in lockstep and still pass). This table
// is hand-maintained against the spec text, not derived from the carrier.
type runCausationNamedValue struct {
	acRef    string // the acceptance criterion naming this value, for failure messages only
	dbColumn string // models.Run's db tag for this value; existence is checked via reflection
	// carrierKey is the carrierKeys entry carrying this value across the
	// task boundary. Empty means this value is instead documented as
	// re-derivable via rederivedReason.
	carrierKey      string
	rederivedReason string
}

var runCausationNamedValues = []runCausationNamedValue{
	{acRef: "AC-OFFICE-RUN-CAUSATION-001.1", dbColumn: "chain_causation_id", carrierKey: taskmodels.MetaKeyOfficeCarrierCausationID},
	// The carrier does not carry parent_run_id directly: the causing
	// run's own identifier (creating_run_id) becomes the new run's
	// parent_run_id (AC-OFFICE-RUN-CAUSATION-001.5), so creating_run_id
	// is parent_run_id's named source.
	{acRef: "AC-OFFICE-RUN-CAUSATION-001.1", dbColumn: "parent_run_id", carrierKey: taskmodels.MetaKeyOfficeCarrierCreatingRunID},
	{acRef: "AC-OFFICE-RUN-CAUSATION-001.1", dbColumn: "causation_depth", carrierKey: taskmodels.MetaKeyOfficeCarrierCausationDepth},
	{acRef: "AC-OFFICE-RUN-CAUSATION-001.13", dbColumn: "human_rooted", carrierKey: taskmodels.MetaKeyOfficeCarrierHumanRooted},
	{acRef: "AC-OFFICE-RUN-CAUSATION-001.14", dbColumn: "routine_id", carrierKey: taskmodels.MetaKeyOfficeCarrierRoutineID},
	{acRef: "AC-OFFICE-RUN-CAUSATION-001.19", dbColumn: "actor_kind", carrierKey: taskmodels.MetaKeyOfficeCarrierActorKind},
	{acRef: "AC-OFFICE-RUN-CAUSATION-001.19", dbColumn: "actor_id", carrierKey: taskmodels.MetaKeyOfficeCarrierActorID},
	{
		acRef: "AC-OFFICE-RUN-CAUSATION-001.20", dbColumn: "workspace_id",
		rederivedReason: "set at enqueue from the run's agent profile workspace, which every enqueue path supplies",
	},
}

// runModelDBColumns returns the set of db-tag column names declared on
// models.Run, so a value's dbColumn above is checked against the real
// struct rather than assumed to still exist.
func runModelDBColumns(t *testing.T) map[string]bool {
	t.Helper()
	typ := reflect.TypeOf(models.Run{})
	cols := make(map[string]bool, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("db")
		if tag != "" && tag != "-" {
			cols[tag] = true
		}
	}
	return cols
}

// TestCarrierSetCompleteness_EveryACNamedValueIsCarriedOrDocumentedRederivable
// is AC-OFFICE-RUN-CAUSATION-001.18's own test requirement: every
// persisted run value named by .1/.13/.14/.19/.20 either joins the
// carrier set or carries a documented re-derivation source, checked
// against the AC's closed value list rather than against the carrier's
// own representations. A value present in the table but absent from
// carrierKeys (or vice versa) fails, so the workspace-scoped exemption
// this test allows is exactly one entry, not an easy escape hatch.
func TestCarrierSetCompleteness_EveryACNamedValueIsCarriedOrDocumentedRederivable(t *testing.T) {
	runFields := runModelDBColumns(t)
	carrierKeySet := make(map[string]bool, len(carrierKeys))
	for _, k := range carrierKeys {
		carrierKeySet[k] = true
	}
	claimedCarrierKeys := make(map[string]bool, len(carrierKeys))

	for _, v := range runCausationNamedValues {
		if !runFields[v.dbColumn] {
			t.Errorf("%s names persisted run value %q, but models.Run has no such db-tagged field (renamed or removed?)",
				v.acRef, v.dbColumn)
			continue
		}
		switch {
		case v.carrierKey != "" && v.rederivedReason != "":
			t.Errorf("%s / %q declares both a carrierKey and a rederivedReason; exactly one applies",
				v.acRef, v.dbColumn)
		case v.carrierKey != "":
			if !carrierKeySet[v.carrierKey] {
				t.Errorf("%s / %q claims carrier key %q, but that key is not in carrierKeys — "+
					"it neither joins the carrier set nor has a named re-derivation source",
					v.acRef, v.dbColumn, v.carrierKey)
			}
			claimedCarrierKeys[v.carrierKey] = true
		case v.rederivedReason == "":
			t.Errorf("%s / %q has neither a carrierKey nor a rederivedReason — "+
				"it neither joins the carrier set nor has a named source, which AC-OFFICE-RUN-CAUSATION-001.18 forbids",
				v.acRef, v.dbColumn)
		}
	}

	for _, k := range carrierKeys {
		if !claimedCarrierKeys[k] {
			t.Errorf("carrierKeys contains %q, which no entry in runCausationNamedValues claims", k)
		}
	}
}

func TestCarrierSetCompleteness_ReadSideDecodesEveryWrittenKey(t *testing.T) {
	run := &models.Run{
		ID:               "run-completeness-2",
		ChainCausationID: "causation-completeness-2",
		CausationDepth:   5,
		HumanRooted:      true,
		RoutineID:        "routine-completeness-2",
		ActorKind:        models.ActorKindAgent,
		ActorID:          "agent-completeness-2",
	}
	written := carrierMetadataFromRun(run)

	// carrierFromTaskMetadata expects JSON-shaped values (metadata
	// round-trips through JSON before this function ever sees it), so
	// mirror that for the one field whose Go and JSON types diverge.
	written[taskmodels.MetaKeyOfficeCarrierCausationDepth] = float64(run.CausationDepth)

	decoded := carrierFromTaskMetadata(written)
	want := TaskBoundaryCarrier{
		CausationID:    run.ChainCausationID,
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
