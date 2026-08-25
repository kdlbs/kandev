package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// A workflow step stores caller-supplied IDs *inside* its payload: a
// `move_to_step` action names the step to transition to, and a `queue_run`
// action names the task to queue work on. Authorizing the step being written
// says nothing about those, and the engine dereferences them later with no
// request identity to check against — so write time is the only place they can
// be checked.
//
// `pull_from_step_id` already had the same-workflow rule; these are the
// references that did not.

// moveToStepBody builds a step payload whose on_turn_complete transitions to
// the named step.
func moveToStepBody(workflowID, name, targetStepID string) map[string]any {
	return map[string]any{
		"workflow_id": workflowID,
		"name":        name,
		"position":    5,
		"events": map[string]any{
			"on_turn_complete": []map[string]any{
				{"type": "move_to_step", "config": map[string]any{"step_id": targetStepID}},
			},
		},
	}
}

// queueRunBody builds a step payload whose on_comment queues a run against the
// named task.
func queueRunBody(workflowID, name, taskID string) map[string]any {
	return map[string]any{
		"workflow_id": workflowID,
		"name":        name,
		"position":    6,
		"events": map[string]any{
			"on_comment": []map[string]any{
				{"type": "queue_run", "config": map[string]any{"task_id": taskID, "target": "primary"}},
			},
		},
	}
}

// TestMoveToStepTargetCannotLeaveTheWorkflow covers the transition target on
// both the create and the update path, over HTTP.
func TestMoveToStepTargetCannotLeaveTheWorkflow(t *testing.T) {
	t.Run("create cannot target another user's step", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			moveToStepBody("wf-b", "Smuggler", h.stepA))
		requireNotFound(t, rec)
		requireNoStepWrites(t, h)
	})

	t.Run("update cannot target another user's step", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflow/steps/"+h.stepB,
			map[string]any{"events": map[string]any{
				"on_turn_complete": []map[string]any{
					{"type": "move_to_step", "config": map[string]any{"step_id": h.stepA}},
				},
			}})
		requireNotFound(t, rec)
		requireNoStepWrites(t, h)
	})

	t.Run("a foreign target answers exactly like a nonexistent one", func(t *testing.T) {
		h := setupScopedRouter(t)
		foreign := doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			moveToStepBody("wf-b", "Smuggler", h.stepA))
		missing := doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			moveToStepBody("wf-b", "Smuggler", "no-such-step"))
		if foreign.Code != missing.Code || foreign.Body.String() != missing.Body.String() {
			t.Fatalf("foreign target %d %s differs from missing target %d %s",
				foreign.Code, foreign.Body.String(), missing.Code, missing.Body.String())
		}
	})

	// A step in the caller's *other* workflow is visible to them, so there is
	// nothing to hide: this is an integrity rejection, and it must read as one
	// rather than as a 404, or the UI cannot tell the user what is wrong.
	t.Run("a step in the caller's other workflow is a validation error", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			moveToStepBody("wf-b", "Crosser", h.stepB2))
		requireStatus(t, rec, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "same workflow") {
			t.Fatalf("body = %s, want the same-workflow reason", rec.Body.String())
		}
		requireNoStepWrites(t, h)
	})

	// The invariant is not an authentication feature: a single-user install
	// must not be able to point a transition at another workflow's step
	// either, exactly as it cannot for pull_from_step_id.
	t.Run("the same-workflow rule holds with auth disabled", func(t *testing.T) {
		h := setupScopedRouter(t)
		rec := doAs(t, h, asSyntheticUser(), http.MethodPost, "/api/v1/workflow/steps",
			moveToStepBody("wf-b", "Crosser", h.stepA))
		requireStatus(t, rec, http.StatusBadRequest)
	})

	t.Run("a target in the same workflow is accepted", func(t *testing.T) {
		h := setupScopedRouter(t)
		rec := doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			moveToStepBody("wf-b", "Neighbour", h.stepB))
		requireStatus(t, rec, http.StatusCreated)
	})
}

// TestQueueRunTaskTargetIsAuthorized covers the other embedded ID: a
// `queue_run` action naming a literal task starts agent work on that task when
// the trigger fires, so an unchecked one is a write into another user's task.
func TestQueueRunTaskTargetIsAuthorized(t *testing.T) {
	t.Run("a foreign task is refused", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			queueRunBody("wf-b", "Queuer", "task-a"))
		requireNotFound(t, rec)
		requireNoStepWrites(t, h)
		if _, _, tasks := h.owner.calls(); len(tasks) == 0 {
			t.Fatal("the task-domain check was never consulted")
		}
	})

	t.Run("the caller's own task is accepted", func(t *testing.T) {
		h := setupScopedRouter(t)
		rec := doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			queueRunBody("wf-b", "Queuer", "task-b"))
		requireStatus(t, rec, http.StatusCreated)
	})

	// "this" and an absent task_id both mean "the task the trigger fired on",
	// which is the shape every built-in template uses. Neither names a task,
	// so neither may be sent to the task service (an empty ID there means "no
	// scoping applies").
	t.Run("the this sentinel names no task", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.owner.resetCalls()
		requireStatus(t, doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			queueRunBody("wf-b", "Sentinel", "this")), http.StatusCreated)
		requireStatus(t, doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			queueRunBody("wf-b", "Empty", "")), http.StatusCreated)
		if _, _, tasks := h.owner.calls(); len(tasks) != 0 {
			t.Fatalf("sentinel task ids were sent to the task service: %v", tasks)
		}
	})
}

// TestReorderRequiresStepsToBelongToTheWorkflow closes the gap between the ID
// that is authorized (the workflow in the URL) and the IDs that are written
// (the step list in the body). Naming a step of a different workflow moved
// that step's position, and pointing the URL at a mutable workflow while
// naming a read-only workflow's steps walked straight past the read-only
// guard as well.
func TestReorderRequiresStepsToBelongToTheWorkflow(t *testing.T) {
	t.Run("a step from the caller's other workflow is refused", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflows/wf-b/workflow/steps/reorder",
			map[string]any{"step_ids": []string{h.stepB, h.stepB2}})
		requireNotFound(t, rec)
		requireNoStepWrites(t, h)
	})

	t.Run("the rule holds with auth disabled", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doAs(t, h, asSyntheticUser(), http.MethodPut, "/api/v1/workflows/wf-b/workflow/steps/reorder",
			map[string]any{"step_ids": []string{h.stepB2}})
		requireNotFound(t, rec)
		requireNoStepWrites(t, h)
	})

	t.Run("the workflow's own steps still reorder", func(t *testing.T) {
		h := setupScopedRouter(t)
		second := doAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workflow/steps",
			map[string]any{"workflow_id": "wf-b", "name": "Second", "position": 1})
		requireStatus(t, second, http.StatusCreated)
		var created map[string]any
		decodeJSON(t, second, &created)
		secondID, _ := created["id"].(string)

		rec := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflows/wf-b/workflow/steps/reorder",
			map[string]any{"step_ids": []string{secondID, h.stepB}})
		requireStatus(t, rec, http.StatusOK)
		steps, err := h.repo.ListStepsByWorkflow(context.Background(), "wf-b")
		if err != nil {
			t.Fatalf("list steps: %v", err)
		}
		if len(steps) != 2 || steps[0].ID != secondID || steps[1].ID != h.stepB {
			t.Fatalf("stored order = %#v, want the reversed order", steps)
		}
	})
}

// TestReadOnlyWorkflowDoesNotLeakToAForeignCaller pins the order of the two
// guards. EnsureWorkflowMutable answers 409 with a message naming *why* the
// workflow is read-only, so running it before the ownership check would tell
// one user that another user's workflow exists and is GitHub-synced.
func TestReadOnlyWorkflowDoesNotLeakToAForeignCaller(t *testing.T) {
	h := setupScopedRouter(t)
	h.service.SetWorkflowProvider(&fakeWorkflowProvider{workflows: []*taskmodels.Workflow{
		{ID: "wf-a", WorkspaceID: "ws-a", Name: "A Flow", Source: taskmodels.WorkflowSourceGitHub},
	}})

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"create", http.MethodPost, "/api/v1/workflow/steps",
			map[string]any{"workflow_id": "wf-a", "name": "Nope", "position": 1}},
		{"template", http.MethodPost, "/api/v1/workflows/wf-a/workflow/steps",
			map[string]any{"template_id": "template-1"}},
		{"reorder", http.MethodPut, "/api/v1/workflows/wf-a/workflow/steps/reorder",
			map[string]any{"step_ids": []string{h.stepA}}},
		{"update", http.MethodPut, "/api/v1/workflow/steps/" + h.stepA, map[string]any{"name": "Nope"}},
		{"delete", http.MethodDelete, "/api/v1/workflow/steps/" + h.stepA, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doAs(t, h, asUser(userB), tc.method, tc.path, tc.body)
			requireNotFound(t, rec)
			if strings.Contains(rec.Body.String(), "GitHub") {
				t.Fatalf("read-only reason leaked to a foreign caller: %s", rec.Body.String())
			}
		})
	}
}

// TestImportCannotSmuggleAForeignTaskReference covers the payload half of the
// same defect.
//
// Transition targets are already constrained on this path: an export names
// them by position, WorkflowExport.Validate rejects a raw `step_id` under
// on_turn_start/on_turn_complete, and the importer maps each position onto a
// step it just created. A queue_run task target has no positional form, and an
// on_enter action survives the conversion verbatim, so a hand-written document
// can name any task in the install.
func TestImportCannotSmuggleAForeignTaskReference(t *testing.T) {
	const queueRunYAML = `version: 1
type: kandev_workflow
workflows:
  - name: Smuggled
    steps:
      - name: Backlog
        position: 0
        color: "#111111"
        events:
          on_enter:
            - type: queue_run
              config:
                task_id: "%s"
`
	const positionalYAML = `version: 1
type: kandev_workflow
workflows:
  - name: Positional
    steps:
      - name: Backlog
        position: 0
        color: "#111111"
        events:
          on_turn_complete:
            - type: move_to_step
              config:
                step_position: 1
      - name: Doing
        position: 1
        color: "#222222"
`

	t.Run("another user's task is refused and nothing is written", func(t *testing.T) {
		h := setupScopedRouter(t)
		h.queries.reset()
		rec := doRawAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workspaces/ws-b/workflows/import",
			fmt.Sprintf(queueRunYAML, "task-a"))
		if rec.Code == http.StatusOK {
			t.Fatalf("import accepted a foreign task reference: %s", rec.Body.String())
		}
		requireNoStepWrites(t, h)
		if len(h.provider.created) != 0 {
			t.Fatalf("refused import created %v", h.provider.created)
		}
	})

	t.Run("the caller's own task still imports", func(t *testing.T) {
		h := setupScopedRouter(t)
		rec := doRawAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workspaces/ws-b/workflows/import",
			fmt.Sprintf(queueRunYAML, "task-b"))
		requireStatus(t, rec, http.StatusOK)
		steps, err := h.repo.ListStepsByWorkflow(context.Background(), "created-Smuggled")
		if err != nil {
			t.Fatalf("list steps: %v", err)
		}
		if len(steps) != 1 || len(steps[0].Events.OnEnter) != 1 {
			t.Fatalf("imported step = %#v, want the on_enter action preserved", steps)
		}
	})

	t.Run("position-based transition targets still import", func(t *testing.T) {
		h := setupScopedRouter(t)
		rec := doRawAs(t, h, asUser(userB), http.MethodPost, "/api/v1/workspaces/ws-b/workflows/import", positionalYAML)
		requireStatus(t, rec, http.StatusOK)
		steps, err := h.repo.ListStepsByWorkflow(context.Background(), "created-Positional")
		if err != nil {
			t.Fatalf("list steps: %v", err)
		}
		if len(steps) != 2 {
			t.Fatalf("steps = %#v, want both imported", steps)
		}
		target := ""
		for _, action := range steps[0].Events.OnTurnComplete {
			if id, ok := action.Config["step_id"].(string); ok {
				target = id
			}
		}
		if target != steps[1].ID {
			t.Fatalf("remapped target = %q, want the second imported step %q", target, steps[1].ID)
		}
	})
}
