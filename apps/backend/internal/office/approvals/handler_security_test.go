package approvals_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/approvals"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// approvalHandlerFixture wires the minimal stack needed to exercise the
// approval handler's security checks: a shared in-memory SQLite, an
// agents service (to mint JWTs and answer CallerFromContext), and an
// approvals service backed by the same repo.
type approvalHandlerFixture struct {
	router    *gin.Engine
	agentsSvc *agents.AgentService
	approvals *approvals.ApprovalService
	repo      *sqlite.Repository
}

func newApprovalHandlerFixture(t *testing.T) *approvalHandlerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	agentsSvc := agents.NewAgentService(repo, log, nil)
	agentsSvc.SetAuth(agents.NewAgentAuth("test-key"))

	apprSvc := approvals.NewApprovalService(repo, log,
		&silentActivityLogger{}, &silentRunQueuer{})

	r := gin.New()
	r.Use(agents.AgentAuthMiddleware(agentsSvc))
	group := r.Group("/api/v1")
	approvals.RegisterRoutes(group, apprSvc)

	return &approvalHandlerFixture{router: r, agentsSvc: agentsSvc, approvals: apprSvc, repo: repo}
}

// silentActivityLogger / silentRunQueuer satisfy the approvals service
// constructor without touching the test output stream.
type silentActivityLogger struct{}

func (s *silentActivityLogger) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (s *silentActivityLogger) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

type silentRunQueuer struct{}

func (s *silentRunQueuer) QueueRun(_ context.Context, _, _, _, _ string) error { return nil }

// seedApprovalAgent creates an agent_profiles row with the given role
// and permissions, returning the persisted instance.
func seedApprovalAgent(
	t *testing.T, svc *agents.AgentService,
	id, workspaceID string, role models.AgentRole, permsJSON string,
) *models.AgentInstance {
	t.Helper()
	a := &models.AgentInstance{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        id,
		Role:        role,
		Status:      models.AgentStatusIdle,
		Permissions: permsJSON,
	}
	if err := svc.CreateAgentInstance(context.Background(), a); err != nil {
		t.Fatalf("create agent %q: %v", id, err)
	}
	return a
}

// seedPendingApproval creates a hire_agent approval row in the target
// workspace, optionally tagging requestedBy so self-approval can be tested.
func seedPendingApproval(
	t *testing.T, repo *sqlite.Repository,
	id, workspaceID, requestedBy string,
) *models.Approval {
	t.Helper()
	a := &models.Approval{
		ID:                        id,
		WorkspaceID:               workspaceID,
		Type:                      models.ApprovalTypeHireAgent,
		RequestedByAgentProfileID: requestedBy,
		Status:                    approvals.StatusPending,
		Payload:                   `{}`,
	}
	if err := repo.CreateApproval(context.Background(), a); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	return a
}

func decideReq(approvalID, token, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/approvals/"+approvalID+"/decide",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// TestDecideApproval_WorkerAgentForbidden pins the can_approve gate.
// A worker agent (role default permissions exclude PermCanApprove) must
// not be able to decide approvals, even within their own workspace.
func TestDecideApproval_WorkerAgentForbidden(t *testing.T) {
	f := newApprovalHandlerFixture(t)
	worker := seedApprovalAgent(t, f.agentsSvc, "worker-1", "ws-1",
		models.AgentRoleWorker, shared.DefaultPermissions(shared.AgentRoleWorker))
	approval := seedPendingApproval(t, f.repo, "appr-1", "ws-1", "other-agent")

	token, err := f.agentsSvc.MintRuntimeJWT(worker.ID, "task-1", worker.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, decideReq(approval.ID, token, `{"status":"approved"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDecideApproval_SelfApprovalForbidden pins the self-approval guard.
// A CEO requesting an approval cannot decide their own request — the
// approver-of-record must be a different agent.
func TestDecideApproval_SelfApprovalForbidden(t *testing.T) {
	f := newApprovalHandlerFixture(t)
	ceo := seedApprovalAgent(t, f.agentsSvc, "ceo-1", "ws-1",
		models.AgentRoleCEO, shared.DefaultPermissions(shared.AgentRoleCEO))
	approval := seedPendingApproval(t, f.repo, "appr-1", "ws-1", ceo.ID)

	token, err := f.agentsSvc.MintRuntimeJWT(ceo.ID, "task-1", ceo.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, decideReq(approval.ID, token, `{"status":"approved"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDecideApproval_CrossWorkspaceForbidden pins the workspace
// isolation guard. A CEO in workspace A cannot decide approvals that
// live in workspace B.
func TestDecideApproval_CrossWorkspaceForbidden(t *testing.T) {
	f := newApprovalHandlerFixture(t)
	ceoA := seedApprovalAgent(t, f.agentsSvc, "ceo-a", "ws-a",
		models.AgentRoleCEO, shared.DefaultPermissions(shared.AgentRoleCEO))
	approval := seedPendingApproval(t, f.repo, "appr-1", "ws-b", "other-agent")

	token, err := f.agentsSvc.MintRuntimeJWT(ceoA.ID, "task-1", ceoA.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, decideReq(approval.ID, token, `{"status":"approved"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDecideApproval_DecidedByDerivedFromJWT pins that resolveDecider
// uses the JWT-derived caller ID for decided_by, not the request body.
// Even if a CEO body-spoofs "decided_by", the persisted row records the
// authenticated identity.
func TestDecideApproval_DecidedByDerivedFromJWT(t *testing.T) {
	f := newApprovalHandlerFixture(t)
	ceo := seedApprovalAgent(t, f.agentsSvc, "ceo-1", "ws-1",
		models.AgentRoleCEO, shared.DefaultPermissions(shared.AgentRoleCEO))
	approval := seedPendingApproval(t, f.repo, "appr-1", "ws-1", "other-agent")

	token, err := f.agentsSvc.MintRuntimeJWT(ceo.ID, "task-1", ceo.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	body := `{"status":"approved","decided_by":"spoofed-id"}`
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, decideReq(approval.ID, token, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	persisted, err := f.repo.GetApproval(context.Background(), approval.ID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if persisted.DecidedBy != ceo.ID {
		t.Errorf("decided_by = %q, want %q (must come from JWT, not body)", persisted.DecidedBy, ceo.ID)
	}
	// Sanity check: the spoofed value never lands in the row.
	if persisted.DecidedBy == "spoofed-id" {
		t.Error("spoofed decided_by from request body landed in persisted approval")
	}
}
