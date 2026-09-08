package service

import (
	"github.com/kandev/kandev/internal/office/models"
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
