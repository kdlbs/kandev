package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/maintenance"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

type maintenanceFixtureSandbox struct {
	prepares, patches, checks, commits int
	beforePrepare                      func()
}

func (f *maintenanceFixtureSandbox) Inspect(_ context.Context, _ string, scope models.MaintenanceScope) (models.MaintenanceScope, string, error) {
	return scope, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil
}
func (f *maintenanceFixtureSandbox) Prepare(ctx context.Context, _ string, _ models.MaintenanceGrant, guard maintenance.Guard) error {
	if f.beforePrepare != nil {
		f.beforePrepare()
	}
	if err := guard(ctx); err != nil {
		return err
	}
	f.prepares++
	return nil
}
func (f *maintenanceFixtureSandbox) Read(context.Context, models.MaintenanceGrant, string) (models.MaintenanceFile, error) {
	return models.MaintenanceFile{}, nil
}
func (f *maintenanceFixtureSandbox) Patch(ctx context.Context, _ models.MaintenanceGrant, _ models.MaintenanceFile, guard maintenance.Guard) (models.MaintenanceArtifact, error) {
	if err := guard(ctx); err != nil {
		return models.MaintenanceArtifact{}, err
	}
	f.patches++
	return models.MaintenanceArtifact{}, nil
}
func (f *maintenanceFixtureSandbox) Check(ctx context.Context, g models.MaintenanceGrant, guard maintenance.Guard) (models.MaintenanceValidation, error) {
	if err := guard(ctx); err != nil {
		return models.MaintenanceValidation{}, err
	}
	f.checks++
	return models.MaintenanceValidation{TreeOID: "tree", GrantRevision: g.Revision, Passed: true, Checks: []models.MaintenanceCheck{{Kind: "positive"}, {Kind: "negative"}}}, nil
}
func (f *maintenanceFixtureSandbox) Commit(ctx context.Context, g models.MaintenanceGrant, guard maintenance.Guard) (models.MaintenanceArtifact, error) {
	if err := guard(ctx); err != nil {
		return models.MaintenanceArtifact{}, err
	}
	f.commits++
	return models.MaintenanceArtifact{BaseOID: g.BaseOID, TreeOID: "tree", CommitOID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, nil
}

type maintenanceFixtureManager struct {
	assistantTaskManager
	db *sqlx.DB
}

func (f *maintenanceFixtureManager) ValidateAssistantContextScope(context.Context, string, models.ContextScope) error {
	return nil
}
func (f *maintenanceFixtureManager) ValidateMaintenanceScope(context.Context, string, models.MaintenanceScope) (string, error) {
	return "/synthetic/repository", nil
}
func (f *maintenanceFixtureManager) CreateWorkspaceTask(ctx context.Context, spec models.WorkspaceTaskSpec) (string, error) {
	f.creates.Add(1)
	f.lastSpec = spec
	_, err := f.db.ExecContext(ctx, `INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('maintenance-worker','ws','Example repair',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	return "maintenance-worker", err
}

func maintenanceFixture(t *testing.T) (*Service, *models.AssistantBinding, models.ImprovementCandidate, *maintenanceFixtureSandbox, *maintenanceFixtureManager) {
	t.Helper()
	s, db, _ := newRuntime(t)
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief", "execution_mode": "execute"}).Code)
	ctx := context.Background()
	b, err := s.Repo.AssistantBinding(ctx, "owner")
	require.NoError(t, err)
	revision, err := s.contextProfileRevision(ctx, "ws", "personal")
	require.NoError(t, err)
	for i := range 3 {
		require.NoError(t, s.Repo.RecordFriction(ctx, b, models.Friction{TaskID: fmt.Sprint("sample-", i), SessionID: "session", OccurrenceID: fmt.Sprint("request-", i), ProfileID: "personal", AccountRevision: revision, Origin: "provider", Operation: "permission", Reason: "pending_approval", PolicyVersion: "unknown", Outcome: "blocked"}, time.Now()))
	}
	rows, err := s.Repo.ImprovementCandidates(ctx, b.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	sandbox := &maintenanceFixtureSandbox{}
	manager := &maintenanceFixtureManager{db: db}
	s.Maintenance, s.Manager = sandbox, manager
	return s, b, rows[0], sandbox, manager
}

func maintenanceGrantBody(b *models.AssistantBinding, candidate models.ImprovementCandidate) map[string]any {
	return map[string]any{"expected_binding_version": b.Version, "expected_revision": 0, "candidate_revision": candidate.Revision, "expires_at": time.Now().Add(time.Hour),
		"scope": models.MaintenanceScope{RepositoryID: "repo", WorkflowID: "workflow", WorkflowStepID: "review", ProfileID: "personal", Files: []string{"scripts/example.js"}, Actions: []string{"read", "patch", "test", "commit"}, Image: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Positive: []string{"node", "tests/positive.js"}, Negative: []string{"node", "tests/negative.js"}}}
}

// @covers AC-ORCHESTRATION-ASSISTANT-008.2, AC-ORCHESTRATION-ASSISTANT-008.3
func TestAssistantMaintenanceGrantDispatch(t *testing.T) {
	s, b, candidate, sandbox, manager := maintenanceFixture(t)
	router := assistantRouter(s)
	base := "/api/v1/orchestration/assistant/improvements/" + candidate.ID
	body := map[string]any{"operation_id": "no-grant", "expected_intent_revision": 0, "expected_binding_version": b.Version, "candidate_revision": candidate.Revision, "grant_revision": 1, "action": "prepare"}
	response := runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 403, response.Code, response.Body.String())
	require.Zero(t, manager.creates.Load())
	require.Zero(t, sandbox.prepares)
	response = runtimeRequest(t, router, "PUT", base+"/grant", "", "", maintenanceGrantBody(b, candidate))
	require.Equal(t, 200, response.Code, response.Body.String())
	candidatePtr, err := s.Repo.ImprovementCandidate(context.Background(), b.ID, candidate.ID)
	require.NoError(t, err)
	body["operation_id"], body["candidate_revision"] = "prepare", candidatePtr.Revision
	first := runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 200, first.Code, first.Body.String())
	second := runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 200, second.Code, second.Body.String())
	require.JSONEq(t, first.Body.String(), second.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
	require.Equal(t, 1, sandbox.prepares)
	require.Equal(t, candidate.ID, manager.lastSpec.MaintenanceCandidateID)
	require.NotEmpty(t, manager.lastSpec.ObjectiveID)
	response = runtimeRequest(t, router, "DELETE", base+"/grant", "", "", map[string]any{"expected_revision": 1, "expected_binding_version": b.Version})
	require.Equal(t, 200, response.Code, response.Body.String())
	body["operation_id"], body["action"] = "after-revoke", "patch"
	response = runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 403, response.Code, response.Body.String())
	require.Zero(t, sandbox.patches)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", base, "", "", nil).Code)
}

func prepareMaintenanceFixture(t *testing.T, s *Service, b *models.AssistantBinding, candidate models.ImprovementCandidate) (string, map[string]any) {
	t.Helper()
	base := "/api/v1/orchestration/assistant/improvements/" + candidate.ID
	router := assistantRouter(s)
	response := runtimeRequest(t, router, "PUT", base+"/grant", "", "", maintenanceGrantBody(b, candidate))
	require.Equal(t, 200, response.Code, response.Body.String())
	current, err := s.Repo.ImprovementCandidate(context.Background(), b.ID, candidate.ID)
	require.NoError(t, err)
	body := map[string]any{"operation_id": "prepare", "expected_intent_revision": 0, "expected_binding_version": b.Version, "candidate_revision": current.Revision, "grant_revision": 1, "action": "prepare"}
	response = runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 200, response.Code, response.Body.String())
	return base, body
}

func TestAssistantImprovementPreparedIsNotResolved(t *testing.T) {
	s, b, candidate, sandbox, _ := maintenanceFixture(t)
	base, body := prepareMaintenanceFixture(t, s, b, candidate)
	router := assistantRouter(s)
	body["action"], body["operation_id"] = "check", "checks"
	require.Equal(t, 200, runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body).Code)
	body["action"], body["operation_id"] = "commit", "commit"
	response := runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 200, response.Code, response.Body.String())
	current, err := s.Repo.ImprovementCandidate(context.Background(), b.ID, candidate.ID)
	require.NoError(t, err)
	require.Equal(t, "prepared", current.State)
	require.NotEmpty(t, current.CommitOID)
	require.Nil(t, current.ResolvedAt, "a local artifact does not prove that the affected workflow was repaired")
	require.Equal(t, 1, sandbox.commits)
	response = runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, 1, sandbox.commits)
	body["action"], body["operation_id"] = "publish", "forbidden"
	require.Equal(t, 422, runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body).Code)
}

func TestAssistantMaintenanceGrantRevokeBeforeEffect(t *testing.T) {
	s, b, candidate, sandbox, manager := maintenanceFixture(t)
	base := "/api/v1/orchestration/assistant/improvements/" + candidate.ID
	router := assistantRouter(s)
	require.Equal(t, 200, runtimeRequest(t, router, "PUT", base+"/grant", "", "", maintenanceGrantBody(b, candidate)).Code)
	current, err := s.Repo.ImprovementCandidate(context.Background(), b.ID, candidate.ID)
	require.NoError(t, err)
	sandbox.beforePrepare = func() { require.NoError(t, s.Repo.RevokeMaintenanceGrant(context.Background(), b, candidate.ID, 1)) }
	body := map[string]any{"operation_id": "prepare-revoked", "expected_intent_revision": 0, "expected_binding_version": b.Version, "candidate_revision": current.Revision, "grant_revision": 1, "action": "prepare"}
	response := runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 403, response.Code, response.Body.String())
	require.Zero(t, manager.creates.Load())
	require.Zero(t, sandbox.prepares)
}

func maintenanceRuntimeCaller(t *testing.T, s *Service, b *models.AssistantBinding) (*gin.Engine, string, string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, b.OrchestratorID, b.ConversationID, "task_comment", "maintenance-example-run", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, assistantBrokerAudience, run.Payload, "session"))
	token, err := s.Auth.MintRuntimeJWT(b.OrchestratorID, b.ConversationID, b.WorkspaceID, run.ID, "session", assistantBrokerAudience)
	require.NoError(t, err)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration", runtimeauth.Middleware(s.Auth, s.Personas)), &Handler{Service: s})
	return router, token, run.ID
}

func TestAssistantMaintenanceBoundaryCannotUseGeneralDelegation(t *testing.T) {
	s, b, candidate, _, manager := maintenanceFixture(t)
	prepareMaintenanceFixture(t, s, b, candidate)
	current, err := s.Repo.ImprovementCandidate(context.Background(), b.ID, candidate.ID)
	require.NoError(t, err)
	links, err := s.Repo.ObjectiveTasks(context.Background(), current.ObjectiveID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	router, token, run := maintenanceRuntimeCaller(t, s, b)
	body := map[string]any{"operation_id": "escape-closed-repair", "expected_intent_revision": 0, "objective_id": current.ObjectiveID, "execution_mode": "execute", "workflow_id": "workflow", "workflow_step_id": "review", "repository_id": "repo", "assignee": "personal", "title": "Unrestricted repair", "description": "Prepare a sample change", "context_ref": links[0].ContextRef}
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, run, body)
	require.Equal(t, 422, response.Code, response.Body.String())
	require.EqualValues(t, 1, manager.creates.Load(), "the granted task is the only task created")
	objective, err := s.Repo.Objective(context.Background(), b.ID, current.ObjectiveID)
	require.NoError(t, err)
	response = runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/objectives", token, run, map[string]any{"operation_id": "launder-grant", "expected_intent_revision": 0, "source_comment_id": objective.SourceCommentID, "mode": "execute", "title": "Different task", "acceptance": []models.Criterion{{ID: "new", Description: "Different effects"}}})
	require.Equal(t, 422, response.Code, response.Body.String())
	response = runtimeRequest(t, router, "PATCH", "/api/v1/orchestration/runtime/objectives/"+objective.ID, token, run, map[string]any{"operation_id": "change-grant-goal", "expected_intent_revision": 0, "expected_revision": objective.Revision, "mode": "design"})
	require.Equal(t, 403, response.Code, response.Body.String())
	response = runtimeRequest(t, router, "PUT", "/api/v1/orchestration/assistant/improvements/"+candidate.ID+"/grant", token, run, maintenanceGrantBody(b, candidate))
	require.Equal(t, 403, response.Code, "the runtime cannot grant itself maintenance authority")
}
