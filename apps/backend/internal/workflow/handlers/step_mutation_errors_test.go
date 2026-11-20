package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/service"
)

// @covers AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.2, AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.6
func TestStepMutationDependencyFailureHTTP(t *testing.T) {
	dependencyErrors := []struct {
		name string
		err  error
	}{
		{name: "plain missing dependency", err: errors.New("artifact manifest not found")},
		{name: "wrapped missing dependency", err: fmt.Errorf("nested lookup: %w", errors.New("artifact manifest not found: private-profile"))},
		{name: "mixed-case missing dependency", err: errors.New("Private ARTIFACT Not Found: secret")},
		{name: "untyped step sentinel text", err: errors.New("workflow step not found")},
	}

	type routeCase struct {
		name          string
		workflowID    string
		prepare       func(*testing.T, *workflowHarness) string
		invoke        func(*testing.T, *workflowHarness, string) *httptest.ResponseRecorder
		preflightStep bool
		templateApply bool
	}
	routes := []routeCase{
		{
			name:       "create",
			workflowID: "workflow-1",
			invoke: func(t *testing.T, h *workflowHarness, _ string) *httptest.ResponseRecorder {
				return doJSON(t, h.router, http.MethodPost, "/api/v1/workflow/steps", map[string]any{
					"workflow_id": "workflow-1", "name": "Ready", "position": 0,
				})
			},
		},
		{
			name:       "update",
			workflowID: "workflow-1",
			prepare: func(t *testing.T, h *workflowHarness) string {
				return createStepViaHTTP(t, h.router, map[string]any{
					"workflow_id": "workflow-1", "name": "Ready", "position": 0,
				}).ID
			},
			preflightStep: true,
			invoke: func(t *testing.T, h *workflowHarness, stepID string) *httptest.ResponseRecorder {
				return doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/"+stepID, map[string]any{
					"name": "Renamed",
				})
			},
		},
		{
			name:       "delete",
			workflowID: "workflow-1",
			prepare: func(t *testing.T, h *workflowHarness) string {
				return createStepViaHTTP(t, h.router, map[string]any{
					"workflow_id": "workflow-1", "name": "Ready", "position": 0,
				}).ID
			},
			invoke: func(t *testing.T, h *workflowHarness, stepID string) *httptest.ResponseRecorder {
				return doJSON(t, h.router, http.MethodDelete, "/api/v1/workflow/steps/"+stepID, nil)
			},
		},
		{
			name:       "reorder",
			workflowID: "workflow-1",
			prepare: func(t *testing.T, h *workflowHarness) string {
				createStepViaHTTP(t, h.router, map[string]any{"workflow_id": "workflow-1", "name": "Ready", "position": 0})
				createStepViaHTTP(t, h.router, map[string]any{"workflow_id": "workflow-1", "name": "Done", "position": 1})
				return "workflow-1"
			},
			invoke: func(t *testing.T, h *workflowHarness, workflowID string) *httptest.ResponseRecorder {
				steps, err := h.repo.ListStepsByWorkflow(context.Background(), workflowID)
				if err != nil {
					t.Fatalf("list steps before reorder: %v", err)
				}
				stepIDs := make([]string, 0, len(steps))
				for _, step := range steps {
					stepIDs = append(stepIDs, step.ID)
				}
				return doJSON(t, h.router, http.MethodPut, "/api/v1/workflows/"+workflowID+"/workflow/steps/reorder", map[string]any{
					"step_ids": stepIDs,
				})
			},
		},
		{
			name:          "template application",
			workflowID:    "workflow-1",
			templateApply: true,
			prepare: func(t *testing.T, h *workflowHarness) string {
				seedTemplate(t, h, "template-1", "Seeded Flow")
				return "workflow-1"
			},
			invoke: func(t *testing.T, h *workflowHarness, workflowID string) *httptest.ResponseRecorder {
				return doJSON(t, h.router, http.MethodPost, "/api/v1/workflows/"+workflowID+"/workflow/steps", map[string]any{
					"template_id": "template-1",
				})
			},
		},
	}

	for _, route := range routes {
		route := route
		for _, dependencyFailure := range dependencyErrors {
			dependencyFailure := dependencyFailure
			t.Run(route.name+"/"+dependencyFailure.name, func(t *testing.T) {
				h := setupStepRouter(t)
				resourceID := route.workflowID
				if route.prepare != nil {
					resourceID = route.prepare(t, h)
				}

				before, err := h.repo.ListStepsByWorkflow(context.Background(), route.workflowID)
				if err != nil {
					t.Fatalf("list initial steps: %v", err)
				}
				recorder := recordStepEvents(t, h.eventBus)
				if route.templateApply {
					h.service.SetWorkflowMutationReadOnlyChecker(func(context.Context, string) (bool, bool, error) {
						return false, false, dependencyFailure.err
					})
				} else {
					h.service.SetExecutorMutationChecker(func(context.Context, string) error {
						return dependencyFailure.err
					})
				}

				if route.preflightStep {
					preflight := doJSON(t, h.router, http.MethodGet, "/api/v1/workflow/steps/"+resourceID, nil)
					if preflight.Code != http.StatusOK {
						t.Fatalf("preflight get status = %d, want 200; body = %s", preflight.Code, preflight.Body.String())
					}
				}

				resp := route.invoke(t, h, resourceID)
				requireJSONError(t, resp, http.StatusInternalServerError, "Failed to save step")

				after, err := h.repo.ListStepsByWorkflow(context.Background(), route.workflowID)
				if err != nil {
					t.Fatalf("list steps after rejection: %v", err)
				}
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("rejected mutation changed stored steps:\nbefore: %#v\nafter: %#v", before, after)
				}
				if len(recorder.events) != 0 {
					t.Fatalf("rejected mutation published successful step events %v", recorder.subjects())
				}
			})
		}
	}
}

// @covers AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.1, AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.2, AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.3, AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.4, AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.5
func TestStepMutationErrorClassification(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "step miss", err: models.ErrWorkflowStepNotFound, wantStatus: http.StatusNotFound, wantBody: `{"error":"Step not found"}`},
		{name: "wrapped step miss", err: fmt.Errorf("step lookup: %w", models.ErrWorkflowStepNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":"Step not found"}`},
		{name: "untyped step miss text", err: errors.New("workflow step not found"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":"Failed to save step"}`},
		{name: "template miss", err: models.ErrWorkflowTemplateNotFound, wantStatus: http.StatusNotFound, wantBody: `{"error":"Template not found"}`},
		{name: "wrapped template miss", err: fmt.Errorf("template lookup: %w", models.ErrWorkflowTemplateNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":"Template not found"}`},
		{name: "workflow miss", err: fmt.Errorf("private workflow: %w", repoerrors.ErrWorkflowNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":"Workflow not found"}`},
		{name: "workspace miss", err: fmt.Errorf("private workspace: %w", repoerrors.ErrWorkspaceNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":"Workspace not found"}`},
		{name: "executor profile miss", err: fmt.Errorf("private profile: %w", repoerrors.ErrExecutorProfileNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":"Executor profile not found"}`},
		{name: "concealed access takes precedence", err: fmt.Errorf("%w: %w", service.ErrNotVisible, repoerrors.ErrWorkflowNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":"Step not found"}`},
		{name: "workflow readonly", err: service.ErrWorkflowReadOnly, wantStatus: http.StatusConflict, wantBody: `{"error":"workflow is managed by GitHub sync and is read-only; edit its definition in the synced repository"}`},
		{name: "workspace readonly", err: service.ErrWorkflowWorkspaceReadOnly, wantStatus: http.StatusConflict, wantBody: `{"error":"this workspace is managed by Improve Kandev and is read-only"}`},
		{name: "default executor unavailable", err: taskmodels.ErrWorkspaceDefaultExecutorUnavailable, wantStatus: http.StatusConflict, wantBody: `{"error":"workspace default executor is unavailable","error_code":"workspace_default_executor_unavailable"}`},
		{name: "executor denial precedes wrapped profile miss", err: fmt.Errorf("%w: %w", taskmodels.ErrExecutorMutationDenied, repoerrors.ErrExecutorProfileNotFound), wantStatus: http.StatusForbidden, wantBody: `{"error":"executor mutation denied"}`},
		{name: "recognized validation text", err: errors.New("pull cycle would be created"), wantStatus: http.StatusBadRequest, wantBody: `{"error":"pull cycle would be created"}`},
		{name: "step order identity", err: models.ErrInvalidWorkflowStepOrder, wantStatus: http.StatusBadRequest, wantBody: `{"error":"invalid workflow step order"}`},
		{name: "mixed case unknown miss", err: errors.New("Dependency NOT FOUND: private value"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":"Failed to save step"}`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workflow/steps", nil)
			(&Handlers{}).writeStepMutationError(ctx, tc.err)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			var got map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body %s: %v", recorder.Body.String(), err)
			}
			want := map[string]string{}
			if err := json.Unmarshal([]byte(tc.wantBody), &want); err != nil {
				t.Fatalf("decode expected body: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("body = %s, want %s", recorder.Body.String(), tc.wantBody)
			}
		})
	}
}

// @covers AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.1, AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.3, AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.5
func TestStepMutationResourceMissHTTP(t *testing.T) {
	t.Run("actual missing step", func(t *testing.T) {
		h := setupStepRouter(t)
		resp := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/missing-step", map[string]any{"name": "Renamed"})
		requireJSONError(t, resp, http.StatusNotFound, "Step not found")
	})

	t.Run("missing template", func(t *testing.T) {
		h := setupStepRouter(t)
		resp := doJSON(t, h.router, http.MethodPost, "/api/v1/workflows/workflow-1/workflow/steps", map[string]any{"template_id": "missing-template"})
		requireJSONError(t, resp, http.StatusNotFound, "Template not found")
		if names := listStepNames(t, h, "workflow-1"); len(names) != 0 {
			t.Fatalf("missing template wrote steps %v", names)
		}
	})

	t.Run("missing executor profile", func(t *testing.T) {
		h := setupStepRouter(t)
		step := createStepViaHTTP(t, h.router, map[string]any{
			"workflow_id": "workflow-1", "name": "Ready", "position": 0,
		})
		before, err := h.repo.ListStepsByWorkflow(context.Background(), "workflow-1")
		if err != nil {
			t.Fatalf("list steps before mutation: %v", err)
		}
		recorder := recordStepEvents(t, h.eventBus)
		h.service.SetExecutorMutationChecker(func(context.Context, string) error {
			return fmt.Errorf("executor profile lookup: %w: private-profile", repoerrors.ErrExecutorProfileNotFound)
		})

		resp := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, map[string]any{"name": "Renamed"})
		requireJSONError(t, resp, http.StatusNotFound, "Executor profile not found")

		after, err := h.repo.ListStepsByWorkflow(context.Background(), "workflow-1")
		if err != nil {
			t.Fatalf("list steps after rejection: %v", err)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("rejected mutation changed stored steps:\nbefore: %#v\nafter: %#v", before, after)
		}
		if len(recorder.events) != 0 {
			t.Fatalf("rejected mutation published successful step events %v", recorder.subjects())
		}
	})

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "workflow", err: fmt.Errorf("%w: private-workflow", repoerrors.ErrWorkflowNotFound), want: "Workflow not found"},
		{name: "workspace", err: fmt.Errorf("%w: private-workspace", repoerrors.ErrWorkspaceNotFound), want: "Workspace not found"},
	} {
		tc := tc
		t.Run(tc.name+" miss from policy", func(t *testing.T) {
			h := setupStepRouter(t)
			h.service.SetWorkflowMutationReadOnlyChecker(func(context.Context, string) (bool, bool, error) {
				return false, false, tc.err
			})
			recorder := recordStepEvents(t, h.eventBus)

			resp := doJSON(t, h.router, http.MethodPost, "/api/v1/workflow/steps", map[string]any{
				"workflow_id": "workflow-1", "name": "Ready", "position": 0,
			})
			requireJSONError(t, resp, http.StatusNotFound, tc.want)
			if names := listStepNames(t, h, "workflow-1"); len(names) != 0 {
				t.Fatalf("rejected mutation wrote steps %v", names)
			}
			if len(recorder.events) != 0 {
				t.Fatalf("rejected mutation published successful step events %v", recorder.subjects())
			}
		})
	}
}

// @covers AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.1, AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.6
func TestStepMutationWriteRaceReturnsNotFound(t *testing.T) {
	for _, mutation := range []string{"update", "delete"} {
		mutation := mutation
		t.Run(mutation, func(t *testing.T) {
			h := setupStepRouter(t)
			step := createStepViaHTTP(t, h.router, map[string]any{
				"workflow_id": "workflow-1", "name": "Ready", "position": 0,
			})
			recorder := recordStepEvents(t, h.eventBus)
			checkerCalls := 0
			h.service.SetExecutorMutationChecker(func(ctx context.Context, _ string) error {
				checkerCalls++
				if err := h.repo.DeleteStep(ctx, step.ID); err != nil {
					return fmt.Errorf("remove step during mutation: %w", err)
				}
				return nil
			})

			var resp *httptest.ResponseRecorder
			if mutation == "update" {
				resp = doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, map[string]any{"name": "Renamed"})
			} else {
				resp = doJSON(t, h.router, http.MethodDelete, "/api/v1/workflow/steps/"+step.ID, nil)
			}

			requireJSONError(t, resp, http.StatusNotFound, "Step not found")
			if checkerCalls != 1 {
				t.Fatalf("executor mutation checker calls = %d, want 1", checkerCalls)
			}
			if len(recorder.events) != 0 {
				t.Fatalf("raced mutation published successful step events %v", recorder.subjects())
			}
			if _, err := h.repo.GetStep(context.Background(), step.ID); !errors.Is(err, models.ErrWorkflowStepNotFound) {
				t.Fatalf("raced step lookup error = %v, want ErrWorkflowStepNotFound", err)
			}
		})
	}
}
