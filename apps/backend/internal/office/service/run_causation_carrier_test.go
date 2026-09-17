package service

// Unit tests for carrierFromTaskMetadata's decode/validation, covering
// AC-OFFICE-RUN-CAUSATION-001.10 (each carrier value resolves
// independently to its most restrictive reading when absent or
// malformed) and AC-OFFICE-RUN-CAUSATION-001.24 (the creating run
// identifier alone decides whether the lineage triple roots). Internal
// (package service, not service_test) because carrierFromTaskMetadata is
// unexported — end-to-end coverage through the public QueueRunFromTaskBoundary
// seam lives in task_creator_carrier_test.go and
// run_causation_from_task_test.go.

import (
	"math"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func fullCarrierMetadata() map[string]interface{} {
	return map[string]interface{}{
		taskmodels.MetaKeyOfficeCarrierCausationID:    "causation-1",
		taskmodels.MetaKeyOfficeCarrierCausationDepth: float64(2),
		taskmodels.MetaKeyOfficeCarrierCreatingRunID:  "run-1",
		taskmodels.MetaKeyOfficeCarrierHumanRooted:    true,
		taskmodels.MetaKeyOfficeCarrierRoutineID:      "routine-1",
		taskmodels.MetaKeyOfficeCarrierActorKind:      "agent",
		taskmodels.MetaKeyOfficeCarrierActorID:        "agent-1",
	}
}

func TestCarrierFromTaskMetadata_NilMetadataResolvesToZeroCarrier(t *testing.T) {
	c := carrierFromTaskMetadata(nil)
	if c != (TaskBoundaryCarrier{}) {
		t.Fatalf("carrier = %+v, want zero value for nil metadata", c)
	}
}

func TestCarrierFromTaskMetadata_MetadataWithNoCarrierKeysResolvesToZeroCarrier(t *testing.T) {
	// Unrelated metadata (e.g. a routine-fire task's auto_start_on_create)
	// must not be mistaken for a partial carrier — see carrierPresent.
	c := carrierFromTaskMetadata(map[string]interface{}{
		"auto_start_on_create": true,
		"agent_profile_id":     "some-agent",
	})
	if c != (TaskBoundaryCarrier{}) {
		t.Fatalf("carrier = %+v, want zero value for a task with no carrier keys", c)
	}
}

func TestCarrierFromTaskMetadata_FullCarrierDecodesExactly(t *testing.T) {
	c := carrierFromTaskMetadata(fullCarrierMetadata())
	want := TaskBoundaryCarrier{
		CausationID:    "causation-1",
		CausationDepth: 2,
		CreatingRunID:  "run-1",
		HumanRooted:    true,
		RoutineID:      "routine-1",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-1",
	}
	if c != want {
		t.Fatalf("carrier = %+v, want %+v", c, want)
	}
}

func TestCarrierFromTaskMetadata_EmptyCreatingRunIDRootsDespiteOtherValues(t *testing.T) {
	// AC-OFFICE-RUN-CAUSATION-001.24: an empty (but well-formed, present)
	// creating run id roots the lineage triple regardless of what
	// causation id/depth say — this is the routine-fire case, not a
	// malformed value.
	metadata := fullCarrierMetadata()
	metadata[taskmodels.MetaKeyOfficeCarrierCreatingRunID] = ""
	c := carrierFromTaskMetadata(metadata)
	if c.CreatingRunID != "" || c.CausationID != "" || c.CausationDepth != 0 {
		t.Fatalf("lineage = {creating=%q causation=%q depth=%d}, want all zero (root)",
			c.CreatingRunID, c.CausationID, c.CausationDepth)
	}
	// The routine attribution, human-rooted flag, and actor still apply.
	if c.RoutineID != "routine-1" || !c.HumanRooted || c.ActorKind != models.ActorKindAgent {
		t.Fatalf("carrier = %+v, want routine/human-rooted/actor to still apply", c)
	}
}

func TestCarrierFromTaskMetadata_MissingCreatingRunIDKeyRootsTriple(t *testing.T) {
	metadata := fullCarrierMetadata()
	delete(metadata, taskmodels.MetaKeyOfficeCarrierCreatingRunID)
	c := carrierFromTaskMetadata(metadata)
	if c.CreatingRunID != "" || c.CausationID != "" || c.CausationDepth != 0 {
		t.Fatalf("lineage = {creating=%q causation=%q depth=%d}, want all zero (root)",
			c.CreatingRunID, c.CausationID, c.CausationDepth)
	}
}

func TestCarrierFromTaskMetadata_MalformedDepthRootsTriple(t *testing.T) {
	for name, badDepth := range map[string]interface{}{
		"non_numeric":  "not-a-number",
		"negative":     float64(-1),
		"fractional":   float64(1.9),
		"out_of_range": math.MaxFloat64,
		"nan":          math.NaN(),
		"inf":          math.Inf(1),
	} {
		t.Run(name, func(t *testing.T) {
			metadata := fullCarrierMetadata()
			metadata[taskmodels.MetaKeyOfficeCarrierCausationDepth] = badDepth
			c := carrierFromTaskMetadata(metadata)
			if c.CreatingRunID != "" || c.CausationID != "" || c.CausationDepth != 0 {
				t.Fatalf("lineage = {creating=%q causation=%q depth=%d}, want all zero (root)",
					c.CreatingRunID, c.CausationID, c.CausationDepth)
			}
		})
	}
}

func TestCarrierFromTaskMetadata_EmptyCausationIDSelfHealsToCreatingRunID(t *testing.T) {
	// AC-OFFICE-RUN-CAUSATION-001.8's legacy-empty self-heal, applied to
	// the carrier path: a well-formed but empty causation id (a legacy
	// run that predates the causation-id column) adopts the creating run
	// id, rather than being treated as malformed.
	metadata := fullCarrierMetadata()
	metadata[taskmodels.MetaKeyOfficeCarrierCausationID] = ""
	c := carrierFromTaskMetadata(metadata)
	if c.CausationID != "run-1" {
		t.Fatalf("causation_id = %q, want self-healed %q", c.CausationID, "run-1")
	}
	if c.CreatingRunID != "run-1" || c.CausationDepth != 2 {
		t.Fatalf("lineage = {creating=%q depth=%d}, want run-1/2 (unaffected by the self-heal)",
			c.CreatingRunID, c.CausationDepth)
	}
}

func TestCarrierFromTaskMetadata_MalformedHumanRootedResolvesFalse(t *testing.T) {
	metadata := fullCarrierMetadata()
	metadata[taskmodels.MetaKeyOfficeCarrierHumanRooted] = "not-a-bool"
	c := carrierFromTaskMetadata(metadata)
	if c.HumanRooted {
		t.Error("human_rooted = true, want false (malformed value resolves restrictively)")
	}
	// The lineage triple, decoded independently, is unaffected.
	if c.CreatingRunID != "run-1" {
		t.Errorf("creating_run_id = %q, want unaffected %q", c.CreatingRunID, "run-1")
	}
}

func TestCarrierFromTaskMetadata_UnrecognizedActorKindResolvesEmpty(t *testing.T) {
	metadata := fullCarrierMetadata()
	metadata[taskmodels.MetaKeyOfficeCarrierActorKind] = "not-a-kind"
	c := carrierFromTaskMetadata(metadata)
	if c.ActorKind != "" {
		t.Errorf("actor_kind = %q, want empty (unrecognized resolves restrictively)", c.ActorKind)
	}
}

func TestCarrierFromTaskMetadata_MissingRoutineIDResolvesEmpty(t *testing.T) {
	metadata := fullCarrierMetadata()
	delete(metadata, taskmodels.MetaKeyOfficeCarrierRoutineID)
	c := carrierFromTaskMetadata(metadata)
	if c.RoutineID != "" {
		t.Errorf("routine_id = %q, want empty (absent resolves restrictively)", c.RoutineID)
	}
	// One malformed/absent value must not discard the others.
	if c.CreatingRunID != "run-1" || c.ActorKind != models.ActorKindAgent {
		t.Fatalf("carrier = %+v, want lineage/actor unaffected by the absent routine id", c)
	}
}
