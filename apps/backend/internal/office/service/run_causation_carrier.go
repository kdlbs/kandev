package service

import (
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// carrierMetadataFromRun builds the task-boundary causation carrier set
// (AC-OFFICE-RUN-CAUSATION-001.18) from the run that created a task, for
// persisting on that task's metadata. AC-OFFICE-RUN-CAUSATION-001.17: the
// system writes this itself from the server-side run record, never from
// creating-agent input.
func carrierMetadataFromRun(run *models.Run) map[string]interface{} {
	return map[string]interface{}{
		taskmodels.MetaKeyOfficeCarrierCausationID:    run.CausationID,
		taskmodels.MetaKeyOfficeCarrierCausationDepth: run.CausationDepth,
		taskmodels.MetaKeyOfficeCarrierCreatingRunID:  run.ID,
		taskmodels.MetaKeyOfficeCarrierHumanRooted:    run.HumanRooted,
		taskmodels.MetaKeyOfficeCarrierRoutineID:      run.RoutineID,
		taskmodels.MetaKeyOfficeCarrierActorKind:      string(run.ActorKind),
		taskmodels.MetaKeyOfficeCarrierActorID:        run.ActorID,
	}
}

// TaskBoundaryCarrier is the validated result of reading the carrier set
// (AC-OFFICE-RUN-CAUSATION-001.18) off a task's metadata, ready to attach
// to a runs/service.QueueRunRequest for a run queued because of that
// task. Every field's Go zero value is that value's most restrictive
// reading, so a caller that never resolved a carrier at all (the common
// case: this task never went through carrier-writing) can hand over a
// zero-value TaskBoundaryCarrier and get root/system/empty behavior for
// free.
type TaskBoundaryCarrier struct {
	CausationID    string
	CausationDepth int
	CreatingRunID  string
	HumanRooted    bool
	RoutineID      string
	ActorKind      models.ActorKind
	ActorID        string
}

// carrierKeys is every key carrierMetadataFromRun writes, in no
// particular order. Used to detect whether a task's metadata carries a
// carrier at all, since metadata commonly holds unrelated keys
// (auto_start_on_create, agent_profile_id, ...) for tasks that never
// went through carrier-writing.
var carrierKeys = []string{
	taskmodels.MetaKeyOfficeCarrierCausationID,
	taskmodels.MetaKeyOfficeCarrierCausationDepth,
	taskmodels.MetaKeyOfficeCarrierCreatingRunID,
	taskmodels.MetaKeyOfficeCarrierHumanRooted,
	taskmodels.MetaKeyOfficeCarrierRoutineID,
	taskmodels.MetaKeyOfficeCarrierActorKind,
	taskmodels.MetaKeyOfficeCarrierActorID,
}

// carrierPresent reports whether metadata contains at least one
// carrier-set key. Non-nilness alone is not a reliable "a carrier was
// attempted here" signal — an ordinary task a human created directly, or
// a routine-fire task carrying only auto_start_on_create, has non-nil
// metadata with zero carrier keys. Only when at least one carrier key is
// present do the other, missing keys count as
// AC-OFFICE-RUN-CAUSATION-001.10 malformed values; otherwise this is the
// AC-OFFICE-RUN-CAUSATION-001.24 "no creating run at all" case, and
// resolving quietly to root is correct, not a defect to count.
func carrierPresent(metadata map[string]interface{}) bool {
	for _, key := range carrierKeys {
		if _, ok := metadata[key]; ok {
			return true
		}
	}
	return false
}

// carrierFromTaskMetadata reads and validates the task-boundary carrier
// set off a task's metadata. Each value is resolved independently per
// AC-OFFICE-RUN-CAUSATION-001.10: a key that is present but the wrong
// shape (non-string, non-bool, non-numeric, negative), or missing
// outright once carrierPresent has established this task does carry a
// carrier, increments office_launch_causation_invalid_total labelled by
// the value name and reason, and resolves to that value's most
// restrictive reading, without discarding the other keys.
func carrierFromTaskMetadata(metadata map[string]interface{}) TaskBoundaryCarrier {
	if !carrierPresent(metadata) {
		return TaskBoundaryCarrier{}
	}
	c := TaskBoundaryCarrier{
		HumanRooted: carrierBoolValue(metadata, taskmodels.MetaKeyOfficeCarrierHumanRooted),
		RoutineID:   carrierStringValue(metadata, taskmodels.MetaKeyOfficeCarrierRoutineID),
		ActorKind:   carrierActorKindValue(metadata, taskmodels.MetaKeyOfficeCarrierActorKind),
		ActorID:     carrierStringValue(metadata, taskmodels.MetaKeyOfficeCarrierActorID),
	}

	// The creating run identifier and causation depth decide the whole
	// lineage triple together (AC-OFFICE-RUN-CAUSATION-001.10/.24): a
	// malformed depth can't be trusted to compute a child's depth, and a
	// malformed creating-run identifier can't be trusted as a parent, so
	// either failing roots the triple rather than resolving the two
	// independently. The causation identifier decodes on its own and
	// self-heals to the creating run id when empty, the same way
	// causingCausationID does for a direct causing-run enqueue
	// (AC-OFFICE-RUN-CAUSATION-001.8) — an empty causation id is a
	// legitimate legacy value, not a defect.
	creatingRunID, creatingRunIDOK := carrierTypedString(metadata, taskmodels.MetaKeyOfficeCarrierCreatingRunID)
	depth, depthOK := carrierNonNegativeInt(metadata, taskmodels.MetaKeyOfficeCarrierCausationDepth)
	if creatingRunIDOK && depthOK && creatingRunID != "" {
		c.CreatingRunID = creatingRunID
		c.CausationDepth = depth
		c.CausationID = carrierStringValue(metadata, taskmodels.MetaKeyOfficeCarrierCausationID)
		if c.CausationID == "" {
			c.CausationID = creatingRunID
		}
	}
	return c
}

// carrierInvalid records an AC-OFFICE-RUN-CAUSATION-001.10 malformed
// carrier value.
func carrierInvalid(key, reason string) {
	shared.LaunchCausationInvalidTotal.Add(shared.LaunchSafetyLabel("value", key, "reason", reason), 1)
}

// carrierTypedString returns (value, ok). ok is false, and the malformed
// counter fires, when the key is missing or not a string.
func carrierTypedString(metadata map[string]interface{}, key string) (string, bool) {
	v, present := metadata[key]
	if !present {
		carrierInvalid(key, "absent")
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		carrierInvalid(key, "non_string")
		return "", false
	}
	return s, true
}

func carrierStringValue(metadata map[string]interface{}, key string) string {
	s, _ := carrierTypedString(metadata, key)
	return s
}

func carrierBoolValue(metadata map[string]interface{}, key string) bool {
	v, present := metadata[key]
	if !present {
		carrierInvalid(key, "absent")
		return false
	}
	b, ok := v.(bool)
	if !ok {
		carrierInvalid(key, "non_bool")
		return false
	}
	return b
}

// carrierNonNegativeInt decodes a JSON-numeric carrier value. Metadata
// round-trips through JSON, so a well-formed depth always decodes as
// float64.
func carrierNonNegativeInt(metadata map[string]interface{}, key string) (int, bool) {
	v, present := metadata[key]
	if !present {
		carrierInvalid(key, "absent")
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		carrierInvalid(key, "non_numeric")
		return 0, false
	}
	if f < 0 {
		carrierInvalid(key, "negative")
		return 0, false
	}
	return int(f), true
}

func carrierActorKindValue(metadata map[string]interface{}, key string) models.ActorKind {
	s, ok := carrierTypedString(metadata, key)
	if !ok {
		return ""
	}
	kind := models.ActorKind(s)
	if !kind.Valid() {
		carrierInvalid(key, "unrecognized")
		return ""
	}
	return kind
}
