package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestAssistantImprovementHumanReview(t *testing.T) {
	s, b, candidate, _, manager := maintenanceFixture(t)
	base, body := prepareMaintenanceFixture(t, s, b, candidate)
	router := assistantRouter(s)
	for _, action := range []string{"check", "commit"} {
		body["action"], body["operation_id"] = action, action
		response := runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
		require.Equal(t, 200, response.Code, response.Body.String())
	}
	current, err := s.Repo.ImprovementCandidate(context.Background(), b.ID, candidate.ID)
	require.NoError(t, err)
	review := map[string]any{"expected_binding_version": b.Version, "expected_revision": current.Revision, "action": "resolved"}
	response := runtimeRequest(t, router, "POST", base+"/review", "", "", review)
	require.Equal(t, 422, response.Code, "a model or human success assertion is insufficient: "+response.Body.String())
	review["evidence"] = models.Evidence{TaskID: "sample-0", SessionID: "success", SourceKind: "task_message", SourceID: "persisted-result"}
	tasks := &frictionTasks{testTasks: s.Tasks.(*testTasks), sessions: map[string][]*taskmodels.TaskSession{"sample-0": {{ID: "success", TaskID: "sample-0", ExecutionProfileID: "personal", State: taskmodels.TaskSessionStateCompleted, StartedAt: current.PreparedAt.Add(-time.Hour)}}}}
	tasks.tasks["sample-0"] = &taskmodels.Task{ID: "sample-0", WorkspaceID: "ws", State: v1.TaskStateCompleted}
	_, err = manager.db.Exec(`INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('sample-0','ws','Synthetic affected workflow',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	s.Tasks = tasks
	response = runtimeRequest(t, router, "POST", base+"/review", "", "", review)
	require.Equal(t, 422, response.Code, "an old successful run cannot close a new repair")
	tasks.sessions["sample-0"][0].StartedAt = current.PreparedAt.Add(time.Minute)
	_, err = manager.db.Exec(`UPDATE orchestration_improvements SET account_revision='earlier-account-configuration' WHERE id=?`, current.ID)
	require.NoError(t, err)
	response = runtimeRequest(t, router, "POST", base+"/review", "", "", review)
	require.Equal(t, 422, response.Code, "success under a changed account cannot resolve the original account's incident")
	_, err = manager.db.Exec(`UPDATE orchestration_improvements SET account_revision=? WHERE id=?`, candidate.AccountRevision, current.ID)
	require.NoError(t, err)
	response = runtimeRequest(t, router, "POST", base+"/review", "", "", review)
	require.Equal(t, 200, response.Code, response.Body.String())
	current, err = s.Repo.ImprovementCandidate(context.Background(), b.ID, candidate.ID)
	require.NoError(t, err)
	require.Equal(t, "resolved", current.State)
	require.NotNil(t, current.ResolvedAt)
	objective, err := s.Repo.Objective(context.Background(), b.ID, current.ObjectiveID)
	require.NoError(t, err)
	require.Equal(t, "complete", objective.Status)
	require.Len(t, objective.Evidence, len(objective.Acceptance))
	require.Equal(t, 409, runtimeRequest(t, router, "POST", base+"/review", "", "", review).Code)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "POST", base+"/review", "", "", review).Code)
	runtimeRouter, token, run := maintenanceRuntimeCaller(t, s, b)
	require.Equal(t, 403, runtimeRequest(t, runtimeRouter, "POST", base+"/review", token, run, review).Code)
	comments, err := s.Repo.ListComments(context.Background(), b.ConversationID, 100)
	require.NoError(t, err)
	for _, comment := range comments {
		if comment.Source != "maintenance_review" {
			continue
		}
		response = runtimeRequest(t, runtimeRouter, "POST", "/api/v1/orchestration/runtime/objectives", token, run, map[string]any{"operation_id": "reuse-review", "expected_intent_revision": 0, "source_comment_id": comment.ID, "mode": "execute", "title": "Another example task", "acceptance": []models.Criterion{{ID: "result", Description: "Example output"}}})
		require.Equal(t, 422, response.Code, "a scoped review does not authorize a general objective: "+response.Body.String())
	}
}

func TestAssistantImprovementReadArtifacts(t *testing.T) {
	s, b, candidate, sandbox, _ := maintenanceFixture(t)
	base, _ := prepareMaintenanceFixture(t, s, b, candidate)
	router := assistantRouter(s)
	query := "?expected_binding_version=1&grant_revision=1&path=scripts/example.js"
	response := runtimeRequest(t, router, "GET", base+"/file"+query, "", "", nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "synthetic file")
	require.Equal(t, 1, sandbox.reads)
	require.NoError(t, s.Repo.RevokeMaintenanceGrant(context.Background(), b, candidate.ID, 1))
	response = runtimeRequest(t, router, "GET", base+"/file"+query, "", "", nil)
	require.Equal(t, 403, response.Code, response.Body.String())
	require.Equal(t, 1, sandbox.reads)
	response = runtimeRequest(t, router, "GET", base+"/artifact", "", "", nil)
	require.Equal(t, 200, response.Code, "the owner can review a completed artifact after revoking execution: "+response.Body.String())
	require.Contains(t, response.Body.String(), "synthetic patch")
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", base+"/artifact", "", "", nil).Code)
}
