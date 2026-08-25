package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/controller"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// importYAML is a minimal valid export document, used to prove an import into
// somebody else's workspace writes nothing.
const importYAML = `version: 1
type: kandev_workflow
workflows:
  - name: Smuggled
    description: Imported by the wrong user
    steps:
      - name: Backlog
        position: 0
        color: "#111111"
`

// TestWorkflowKeyedRoutesDenyForeignOwner covers every route that names a
// workflow or a workspace by ID. User B is authenticated and guesses user A's
// IDs; each route must answer 404 and must not reach the workflow_steps table
// at all — a guard that runs after the read has already leaked (list/export)
// or already written (create/reorder/import).
func TestWorkflowKeyedRoutesDenyForeignOwner(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"list steps by workflow", http.MethodGet, "/api/v1/workflows/wf-a/workflow/steps", nil},
		{"list steps by workspace", http.MethodGet, "/api/v1/workspaces/ws-a/workflow-steps", nil},
		{"export workflow", http.MethodGet, "/api/v1/workflows/wf-a/export", nil},
		{"export workspace workflows", http.MethodGet, "/api/v1/workspaces/ws-a/workflows/export", nil},
		{
			"create step", http.MethodPost, "/api/v1/workflow/steps",
			map[string]any{"workflow_id": "wf-a", "name": "Smuggled", "position": 9},
		},
		{
			"create steps from template", http.MethodPost, "/api/v1/workflows/wf-a/workflow/steps",
			map[string]any{"template_id": "template-1"},
		},
		{
			"reorder steps", http.MethodPut, "/api/v1/workflows/wf-a/workflow/steps/reorder",
			map[string]any{"step_ids": []string{"unknown"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := setupScopedRouter(t)
			seedTemplate(t, h.workflowHarness, "template-1", "Seeded Flow")
			h.queries.reset()

			rec := doAs(t, h, asUser(userB), tc.method, tc.path, tc.body)
			requireNotFound(t, rec)
			requireStepTableUntouched(t, h)
			if got := stepNames(t, h, "wf-a"); len(got) != 1 || got[0] != "A Backlog" {
				t.Fatalf("workflow steps = %v, want the untouched seed", got)
			}
		})
	}

	t.Run("import into a foreign workspace", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()

		rec := doRawAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workspaces/ws-a/workflows/import", importYAML)
		requireNotFound(t, rec)
		requireStepTableUntouched(t, h)
		if len(h.provider.created) != 0 {
			t.Fatalf("denied import created %v", h.provider.created)
		}
	})
}

// TestStepKeyedRoutesDenyForeignOwner covers the routes keyed by step ID. The
// step's owning workflow has to be resolved before it can be authorized, so
// the read of that one row is expected; what must not happen is a write, a
// disclosure of the step, or a response that distinguishes "not yours" from
// "does not exist".
func TestStepKeyedRoutesDenyForeignOwner(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		h := setupScopedRouter(t)
		foreign := doAs(t, h, asUser(userB), http.MethodGet, "/api/v1/workflow/steps/"+h.stepA, nil)
		missing := doAs(t, h, asUser(userB), http.MethodGet, "/api/v1/workflow/steps/no-such-step", nil)

		requireNotFound(t, foreign)
		if foreign.Code != missing.Code || foreign.Body.String() != missing.Body.String() {
			t.Fatalf("foreign step %d %s differs from missing step %d %s",
				foreign.Code, foreign.Body.String(), missing.Code, missing.Body.String())
		}
		if strings.Contains(foreign.Body.String(), "A Backlog") {
			t.Fatalf("denied get leaked the step: %s", foreign.Body.String())
		}
	})

	t.Run("update", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflow/steps/"+h.stepA,
			map[string]any{"name": "Hijacked"})
		requireNotFound(t, rec)
		requireNoStepWrites(t, h)
		if got := stepNames(t, h, "wf-a"); len(got) != 1 || got[0] != "A Backlog" {
			t.Fatalf("workflow steps = %v, want the untouched seed", got)
		}
	})

	t.Run("delete", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asUser(userB), http.MethodDelete, "/api/v1/workflow/steps/"+h.stepA, nil)
		requireNotFound(t, rec)
		requireNoStepWrites(t, h)
		if got := stepNames(t, h, "wf-a"); len(got) != 1 {
			t.Fatalf("workflow steps = %v, want the step to survive", got)
		}
	})

	t.Run("delete of a missing step answers exactly like a foreign one", func(t *testing.T) {
		h := setupScopedRouter(t)
		foreign := doAs(t, h, asUser(userB), http.MethodDelete, "/api/v1/workflow/steps/"+h.stepA, nil)
		missing := doAs(t, h, asUser(userB), http.MethodDelete, "/api/v1/workflow/steps/no-such-step", nil)
		if foreign.Code != missing.Code || foreign.Body.String() != missing.Body.String() {
			t.Fatalf("foreign delete %d %s differs from missing delete %d %s",
				foreign.Code, foreign.Body.String(), missing.Code, missing.Body.String())
		}
	})

	t.Run("reorder cannot name a foreign step from an owned workflow", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflows/wf-b/workflow/steps/reorder",
			map[string]any{"step_ids": []string{h.stepA}})
		requireNotFound(t, rec)
		requireNoStepWrites(t, h)
	})

	// pull_from_step_id names a second step, and the validator used to answer
	// "must reference a step in the same workflow" for one that exists and
	// "invalid: workflow step not found" for one that does not — a working
	// existence oracle over every step in the install, reachable from an
	// ordinary edit of the caller's own step.
	t.Run("a foreign pull source answers exactly like a nonexistent one", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		foreign := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflow/steps/"+h.stepB,
			map[string]any{"pull_from_step_id": h.stepA})
		missing := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflow/steps/"+h.stepB,
			map[string]any{"pull_from_step_id": "no-such-step"})
		requireNotFound(t, foreign)
		if foreign.Code != missing.Code || foreign.Body.String() != missing.Body.String() {
			t.Fatalf("foreign pull source %d %s differs from missing one %d %s",
				foreign.Code, foreign.Body.String(), missing.Code, missing.Body.String())
		}
		if strings.Contains(foreign.Body.String(), "same workflow") {
			t.Fatalf("denied pull source disclosed that the step exists: %s", foreign.Body.String())
		}
		requireNoStepWrites(t, h)
	})
}

// TestWorkflowScopeWSActionsDenyForeignOwner is the half the gateway backstop
// cannot cover: these payloads name a workflow_id or a step id, neither of
// which dispatch_scope.go parses, so the WS surface is only as scoped as the
// handlers behind it.
func TestWorkflowScopeWSActionsDenyForeignOwner(t *testing.T) {
	t.Run("workflow.step.list", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		msg := dispatchAs(t, h, asUser(userB), ws.ActionWorkflowStepList, map[string]any{"workflow_id": "wf-a"})
		requireWSError(t, msg, ws.ErrorCodeNotFound)
		requireStepTableUntouched(t, h)
	})

	t.Run("workflow.step.get", func(t *testing.T) {
		h := setupScopedRouter(t)
		foreign := dispatchAs(t, h, asUser(userB), ws.ActionWorkflowStepGet, map[string]any{"id": h.stepA})
		missing := dispatchAs(t, h, asUser(userB), ws.ActionWorkflowStepGet, map[string]any{"id": "no-such-step"})
		requireWSError(t, foreign, ws.ErrorCodeNotFound)
		if string(foreign.Payload) != string(missing.Payload) {
			t.Fatalf("foreign step frame %s differs from missing step frame %s", foreign.Payload, missing.Payload)
		}
	})

	t.Run("workflow.step.create", func(t *testing.T) {
		h := setupScopedRouter(t)
		seedTemplate(t, h.workflowHarness, "template-1", "Seeded Flow")
		h.queries.reset()
		msg := dispatchAs(t, h, asUser(userB), ws.ActionWorkflowStepCreate,
			map[string]any{"workflow_id": "wf-a", "template_id": "template-1"})
		requireWSError(t, msg, ws.ErrorCodeNotFound)
		requireStepTableUntouched(t, h)
	})
}

// TestWorkflowScopeOwnerKeepsFullAccess is the other half of the guard: every
// route the previous tests deny must still work for the workspace's owner.
func TestWorkflowScopeOwnerKeepsFullAccess(t *testing.T) {
	h := setupScopedRouter(t)
	seedTemplate(t, h.workflowHarness, "template-1", "Seeded Flow")
	ctx := asUser(userA)

	var listed controller.ListStepsResponse
	byWorkflow := doAs(t, h, ctx, http.MethodGet, "/api/v1/workflows/wf-a/workflow/steps", nil)
	requireStatus(t, byWorkflow, http.StatusOK)
	if err := json.Unmarshal(byWorkflow.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Steps) != 1 || listed.Steps[0].ID != h.stepA {
		t.Fatalf("steps = %#v, want the owner's step", listed.Steps)
	}

	requireStatus(t, doAs(t, h, ctx, http.MethodGet, "/api/v1/workspaces/ws-a/workflow-steps", nil), http.StatusOK)
	requireStatus(t, doAs(t, h, ctx, http.MethodGet, "/api/v1/workflow/steps/"+h.stepA, nil), http.StatusOK)
	requireStatus(t, doAs(t, h, ctx, http.MethodGet, "/api/v1/workflows/wf-a/export", nil), http.StatusOK)
	requireStatus(t, doAs(t, h, ctx, http.MethodGet, "/api/v1/workspaces/ws-a/workflows/export", nil), http.StatusOK)

	created := doAs(t, h, ctx, http.MethodPost, "/api/v1/workflow/steps",
		map[string]any{"workflow_id": "wf-a", "name": "Doing", "position": 1})
	requireStatus(t, created, http.StatusCreated)
	var step map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &step); err != nil {
		t.Fatalf("decode created step: %v", err)
	}
	newStepID, _ := step["id"].(string)

	requireStatus(t, doAs(t, h, ctx, http.MethodPut, "/api/v1/workflow/steps/"+newStepID,
		map[string]any{"name": "In Progress"}), http.StatusOK)
	requireStatus(t, doAs(t, h, ctx, http.MethodPut, "/api/v1/workflows/wf-a/workflow/steps/reorder",
		map[string]any{"step_ids": []string{newStepID, h.stepA}}), http.StatusOK)
	requireStatus(t, doAs(t, h, ctx, http.MethodPost, "/api/v1/workflows/wf-a/workflow/steps",
		map[string]any{"template_id": "template-1"}), http.StatusCreated)
	requireStatus(t, doAs(t, h, ctx, http.MethodDelete, "/api/v1/workflow/steps/"+newStepID, nil), http.StatusOK)

	requireStatus(t, doRawAs(t, h, ctx, http.MethodPost,
		"/api/v1/workspaces/ws-a/workflows/import", importYAML), http.StatusOK)
	if len(h.provider.created) != 1 || h.provider.created[0] != "Smuggled" {
		t.Fatalf("owner import created %v, want the imported workflow", h.provider.created)
	}

	var wsListed controller.ListStepsResponse
	requireWSResponse(t, dispatchAs(t, h, ctx, ws.ActionWorkflowStepList,
		map[string]any{"workflow_id": "wf-a"}), &wsListed)
	if len(wsListed.Steps) == 0 {
		t.Fatal("ws list returned no steps for the owner")
	}
	var wsStep controller.GetStepResponse
	requireWSResponse(t, dispatchAs(t, h, ctx, ws.ActionWorkflowStepGet, map[string]any{"id": h.stepA}), &wsStep)
	if wsStep.Step == nil || wsStep.Step.ID != h.stepA {
		t.Fatalf("ws get = %#v, want the owner's step", wsStep.Step)
	}
}

// TestWorkflowScopeNoOpsWithAuthDisabled pins the single-user contract: the
// synthetic identity injected when authentication is off must reach every
// workspace exactly as it did before this guard existed, and must not even
// consult the task domain.
func TestWorkflowScopeNoOpsWithAuthDisabled(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"synthetic identity", asSyntheticUser()},
		{"no identity at all", context.Background()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := setupScopedRouter(t)
			seedTemplate(t, h.workflowHarness, "template-1", "Seeded Flow")
			h.owner.resetCalls()

			// ws-b/wf-b belong to user-b; unscoped callers see everything.
			requireStatus(t, doAs(t, h, tc.ctx, http.MethodGet, "/api/v1/workflows/wf-b/workflow/steps", nil), http.StatusOK)
			requireStatus(t, doAs(t, h, tc.ctx, http.MethodGet, "/api/v1/workspaces/ws-b/workflow-steps", nil), http.StatusOK)
			requireStatus(t, doAs(t, h, tc.ctx, http.MethodGet, "/api/v1/workflow/steps/"+h.stepB, nil), http.StatusOK)
			requireStatus(t, doAs(t, h, tc.ctx, http.MethodGet, "/api/v1/workflows/wf-b/export", nil), http.StatusOK)
			requireStatus(t, doAs(t, h, tc.ctx, http.MethodGet, "/api/v1/workspaces/ws-b/workflows/export", nil), http.StatusOK)
			requireStatus(t, doAs(t, h, tc.ctx, http.MethodPost, "/api/v1/workflow/steps",
				map[string]any{"workflow_id": "wf-b", "name": "Doing", "position": 1}), http.StatusCreated)
			requireStatus(t, doAs(t, h, tc.ctx, http.MethodPut, "/api/v1/workflow/steps/"+h.stepB,
				map[string]any{"name": "Renamed"}), http.StatusOK)
			requireStatus(t, doAs(t, h, tc.ctx, http.MethodDelete, "/api/v1/workflow/steps/"+h.stepB, nil), http.StatusOK)
			requireStatus(t, doRawAs(t, h, tc.ctx, http.MethodPost,
				"/api/v1/workspaces/ws-b/workflows/import", importYAML), http.StatusOK)

			var wsListed controller.ListStepsResponse
			requireWSResponse(t, dispatchAs(t, h, tc.ctx, ws.ActionWorkflowStepList,
				map[string]any{"workflow_id": "wf-b"}), &wsListed)

			// An unresolved-owner failure would be invisible above if the guard
			// short-circuited on a nil checker, so assert it never called out.
			if workflows, workspaces, tasks := h.owner.calls(); len(workflows) != 0 || len(workspaces) != 0 || len(tasks) != 0 {
				t.Fatalf("unscoped caller consulted the task domain: workflows=%v workspaces=%v tasks=%v",
					workflows, workspaces, tasks)
			}
		})
	}
}

// TestWorkflowTemplatesStayUnscoped pins the deliberate exception: templates
// are install-global read-only definitions with no owner, so any authenticated
// user may read them.
func TestWorkflowTemplatesStayUnscoped(t *testing.T) {
	h := setupScopedRouter(t)
	seedTemplate(t, h.workflowHarness, "template-1", "Seeded Flow")

	requireStatus(t, doAs(t, h, asUser(userB), http.MethodGet, "/api/v1/workflow/templates", nil), http.StatusOK)
	requireStatus(t, doAs(t, h, asUser(userB), http.MethodGet, "/api/v1/workflow/templates/template-1", nil), http.StatusOK)

	var listed controller.ListTemplatesResponse
	requireWSResponse(t, dispatchAs(t, h, asUser(userB), ws.ActionWorkflowTemplateList, map[string]any{}), &listed)
	if !containsTemplate(listed.Templates, "template-1") {
		t.Fatalf("templates = %#v, want the seeded row visible to any user", listed.Templates)
	}
}

// TestDeniedRequestsDoNotLogAsFailures pins the difference between a rejection
// and a fault. The delete route pre-reads the step so it can publish the
// deleted event, and that read is now authorized too — so every unauthorized
// delete used to file a Warn saying the step could not be fetched, which reads
// as infrastructure trouble in an operator's logs.
func TestDeniedRequestsDoNotLogAsFailures(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	h := setupScopedRouter(t, log)

	requireNotFound(t, doAs(t, h, asUser(userB), http.MethodDelete, "/api/v1/workflow/steps/"+h.stepA, nil))

	if entries := logs.FilterLevelExact(zapcore.WarnLevel).All(); len(entries) != 0 {
		t.Fatalf("denied delete logged %d warning(s): %+v", len(entries), entries)
	}
}

// TestWSGetStepSeparatesDeniedFromBroken is the WS twin of the HTTP history
// route's rule: an unreachable step is not-found, a failing lookup is not.
// wsGetByID answered NOT_FOUND for everything, and authorizing the step added
// a database read to that path, so an outage would have reported every step as
// missing.
func TestWSGetStepSeparatesDeniedFromBroken(t *testing.T) {
	h := setupScopedRouter(t)
	h.service.SetWorkflowAccessChecker(func(context.Context, string) error {
		return errors.New("database is locked")
	})

	msg := dispatchAs(t, h, asUser(userB), ws.ActionWorkflowStepGet, map[string]any{"id": h.stepA})
	requireWSError(t, msg, ws.ErrorCodeInternalError)
}
