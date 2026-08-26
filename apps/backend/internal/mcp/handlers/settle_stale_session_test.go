package handlers

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
)

func TestStaleSettlementAuthorityIsNarrowAndUsesVerifiedProvenance(t *testing.T) {
	actor := &models.Task{ID: "actor", WorkspaceID: "workspace"}
	actorSession := &models.TaskSession{ID: "actor-session", TaskID: actor.ID}
	target := &models.Task{ID: "target", WorkspaceID: "workspace", ParentID: actor.ID}
	tests := []struct {
		name    string
		actor   *models.Task
		target  *models.Task
		session *models.TaskSession
		basis   string
		allowed bool
	}{
		{name: "same task peer", target: &models.Task{ID: actor.ID, WorkspaceID: "workspace"}, session: &models.TaskSession{ID: "peer", TaskID: actor.ID}, basis: "same_task_peer", allowed: true},
		{name: "direct parent", target: target, session: &models.TaskSession{ID: "target-session", TaskID: target.ID}, basis: "direct_parent", allowed: true},
		{name: "persisted supervisor", target: &models.Task{ID: "target", WorkspaceID: "workspace"}, session: &models.TaskSession{ID: "target-session", TaskID: "target", Metadata: map[string]interface{}{models.SessionMetaKeySpawnSupervision: models.SessionSpawnSupervision{SupervisorTaskID: actor.ID, SupervisorSessionID: actorSession.ID, SpawnedAt: time.Now().UTC()}}}, basis: "persisted_supervisor", allowed: true},
		{name: "supervisor sibling session", target: &models.Task{ID: "target", WorkspaceID: "workspace"}, session: &models.TaskSession{ID: "target-session", TaskID: "target", Metadata: map[string]interface{}{models.SessionMetaKeySpawnSupervision: models.SessionSpawnSupervision{SupervisorTaskID: actor.ID, SupervisorSessionID: "other-supervisor-session", SpawnedAt: time.Now().UTC()}}}},
		{name: "self target", target: actor, session: actorSession},
		{name: "cross workspace", target: &models.Task{ID: "target", WorkspaceID: "other", ParentID: actor.ID}, session: &models.TaskSession{ID: "target-session", TaskID: "target"}},
		{name: "unrelated", target: &models.Task{ID: "target", WorkspaceID: "workspace"}, session: &models.TaskSession{ID: "target-session", TaskID: "target"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basis, allowed := staleSettlementAuthority(actor, actorSession, tt.target, tt.session)
			assert.Equal(t, tt.allowed, allowed)
			assert.Equal(t, tt.basis, basis)
		})
	}
}
