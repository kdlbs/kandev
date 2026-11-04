package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/stretchr/testify/require"
)

func TestBuiltinMCPTaskCreationFailsClosedWithoutPolicyWriter(t *testing.T) {
	svc, bus, repo := createTestService(t)
	wf := seedWorkspaceAndWorkflowForCreate(t, t.Context(), repo, "builtin-create")
	var req CreateTaskRequest
	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"builtin-create","title":"explicit package request","builtin_mcp_preferences":{"version":1,"expected_workspace_policy_revision":0,"packages":{"configuration":"on"}}}`), &req))
	req.WorkflowID = wf
	bus.ClearEvents()
	_, err := svc.CreateTask(t.Context(), &req)
	require.Error(t, err, "creation must not silently drop explicit native package consent")
	require.Empty(t, bus.GetPublishedEvents())
}

type builtinCreationTestWriter struct {
	validate func() error
	apply    func(string) error
}

func (w builtinCreationTestWriter) ValidateTaskCreation(context.Context, string, *mcpconfig.BuiltinCreationPreferences) error {
	return w.validate()
}
func (w builtinCreationTestWriter) ApplyTaskCreation(_ context.Context, id string, _ *mcpconfig.BuiltinCreationPreferences) error {
	return w.apply(id)
}

func TestBuiltinMCPTaskCreationPersistsBeforePublishingAndRollsBackFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failed-policy-save"}[fail], func(t *testing.T) {
			svc, bus, repo := createTestService(t)
			wf := seedWorkspaceAndWorkflowForCreate(t, t.Context(), repo, "builtin-create")
			bus.ClearEvents()
			var appliedID string
			failure := errors.New("policy revision changed")
			svc.SetBuiltinCreationWriter(builtinCreationTestWriter{validate: func() error { return nil }, apply: func(id string) error {
				require.Empty(t, bus.GetPublishedEvents(), "no task-created event can start a process before package consent is persisted")
				_, err := repo.GetTask(t.Context(), id)
				require.NoError(t, err)
				appliedID = id
				if fail {
					return failure
				}
				return nil
			}})
			result, err := svc.CreateTask(t.Context(), &CreateTaskRequest{WorkspaceID: "builtin-create", WorkflowID: wf, Title: "package task", BuiltinMCPPreferences: &mcpconfig.BuiltinCreationPreferences{Version: 1}})
			require.NotEmpty(t, appliedID)
			if fail {
				require.ErrorIs(t, err, failure)
				_, err = repo.GetTask(t.Context(), appliedID)
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, appliedID, result.Task.ID)
				require.NotEmpty(t, bus.GetPublishedEvents())
			}
		})
	}
}
