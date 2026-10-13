package coordinator

import (
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"testing"
)

func TestAutonomyConversationAdmissionRequiresExactCoordinatorOwnership(t *testing.T) {
	for _, kind := range []string{"workspace", "task_id", "coordinator", "ordinary_origin", "dream"} {
		t.Run(kind, func(t *testing.T) {
			f := newAdmitFixture(t)
			task := f.conv.tasks[ceilingConvTask]
			switch kind {
			case "workspace":
				task.WorkspaceID = "foreign"
			case "task_id":
				task.ID = "different-task"
			case "coordinator":
				task.Metadata[taskmodels.MetaKeyCoordinatorID] = "different-coordinator"
			case "ordinary_origin":
				task.Origin = taskmodels.TaskOriginManual
			case "dream":
				task.Metadata[taskmodels.MetaKeyCoordinatorPurpose] = taskmodels.CoordinatorPurposeDream
			}
			wantHeld(t, f.admit(AdmitCounting), admitNoConversation, "")
		})
	}
	f := newAdmitFixture(t)
	if got := f.admit(AdmitCounting); !got.OK || got.TaskID != ceilingConvTask || got.SessionID != ceilingSession {
		t.Fatalf("valid owned conversation refused: %+v", got)
	}
}
