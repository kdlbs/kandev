package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type recordingTaskTransferService struct {
	commands    []models.TaskTransferCommand
	audits      []models.TaskTransferCommand
	err         error
	replayActor models.TaskTransferActor
	replayOK    bool
	replayErr   error
	auditErr    error
	auditCtxErr []error
}

func (s *recordingTaskTransferService) ResolveTaskTransferReplayActor(
	_ context.Context,
	_ models.TaskTransferCommand,
) (models.TaskTransferActor, bool, error) {
	return s.replayActor, s.replayOK, s.replayErr
}

func (s *recordingTaskTransferService) TransferTask(
	_ context.Context,
	command models.TaskTransferCommand,
) (*models.TaskTransferReceipt, error) {
	s.commands = append(s.commands, command)
	if s.err != nil {
		return nil, s.err
	}
	return &models.TaskTransferReceipt{OperationID: "operation-1", TaskID: command.TaskID}, nil
}

func (s *recordingTaskTransferService) AuditTaskTransferAttempt(
	ctx context.Context,
	command models.TaskTransferCommand,
	_ string,
) error {
	s.audits = append(s.audits, command)
	s.auditCtxErr = append(s.auditCtxErr, ctx.Err())
	return s.auditErr
}

type fixedTaskTransferAuthorizer struct {
	actor       models.TaskTransferActor
	ok          bool
	replayActor models.TaskTransferActor
	replayOK    bool
}

func (a fixedTaskTransferAuthorizer) AttestTaskTransferCoordinatorReplay(
	context.Context,
	mcpscope.Principal,
	models.TaskTransferCommand,
	models.TaskTransferActor,
) (models.TaskTransferActor, bool) {
	return a.replayActor, a.replayOK
}

func (a fixedTaskTransferAuthorizer) AttestTaskTransferCoordinator(
	context.Context,
	mcpscope.Principal,
) (models.TaskTransferActor, bool) {
	return a.actor, a.ok
}

func TestHandleTransferTaskAuditsMalformedAndInvalidRequests(t *testing.T) {
	transfer := &recordingTaskTransferService{}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	for _, msg := range []*ws.Message{
		{ID: "bad-json", Action: ws.ActionMCPTransferTask, Payload: []byte(`{"task_id":`)},
		makeWSMessage(t, ws.ActionMCPTransferTask, map[string]interface{}{"task_id": "task-1"}),
	} {
		response, err := h.handleTransferTask(context.Background(), msg)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeError, response.Type)
	}
	require.Len(t, transfer.audits, 2)
}

func TestHandleTransferTaskAuditsMalformedRequestAfterCancellation(t *testing.T) {
	transfer := &recordingTaskTransferService{}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response, err := h.handleTransferTask(ctx, &ws.Message{
		ID: "bad-json", Action: ws.ActionMCPTransferTask, Payload: []byte(`{"task_id":`),
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Len(t, transfer.audits, 1)
	require.NoError(t, transfer.auditCtxErr[0])
}

func TestHandleTransferTaskAttributesRejectedHumanAttempt(t *testing.T) {
	transfer := &recordingTaskTransferService{}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	response, err := h.handleTransferTask(
		authn.WithIdentity(context.Background(), authn.Identity{UserID: "human-1"}),
		&ws.Message{ID: "bad-json", Action: ws.ActionMCPTransferTask, Payload: []byte(`{"task_id":`)},
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Len(t, transfer.audits, 1)
	require.Equal(t, "human-1", transfer.audits[0].Actor.ID)
}

func TestHandleTransferTaskRejectsClientAuditOnlyFieldWithoutTransfer(t *testing.T) {
	transfer := &recordingTaskTransferService{}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	message := transferTaskMessage(t)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(message.Payload, &payload))
	payload["_audit_only"] = true
	message = makeWSMessage(t, ws.ActionMCPTransferTask, payload)

	response, err := h.handleTransferTask(
		authn.WithIdentity(context.Background(), authn.Identity{UserID: "human-1"}),
		message,
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, transfer.commands)
	require.Len(t, transfer.audits, 1)
}

func TestAuditTaskTransferAttemptActionRecordsAuditWithoutTransfer(t *testing.T) {
	transfer := &recordingTaskTransferService{}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	dispatcher := ws.NewDispatcher()
	h.registerTaskConfigMutationHandlers(&guardedMCPDispatcher{Dispatcher: dispatcher, handlers: h})

	message := transferTaskMessage(t)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(message.Payload, &payload))
	payload["unknown_field"] = true
	message = makeWSMessage(t, ws.ActionMCPAuditTaskTransferAttempt, payload)
	response, err := dispatcher.Dispatch(
		authn.WithIdentity(context.Background(), authn.Identity{UserID: "human-1"}),
		message,
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Empty(t, transfer.commands)
	require.Len(t, transfer.audits, 1)
}

func TestAuditTaskTransferAttemptActionRecordsSchemaRejectedFieldTypes(t *testing.T) {
	transfer := &recordingTaskTransferService{}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	message := makeWSMessage(t, ws.ActionMCPAuditTaskTransferAttempt, map[string]interface{}{
		"task_id":                      42,
		"expected_source_workspace_id": "ws-source",
	})

	response, err := h.handleAuditTaskTransferAttempt(context.Background(), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Empty(t, transfer.commands)
	require.Len(t, transfer.audits, 1)
	require.Empty(t, transfer.audits[0].TaskID)
	require.Equal(t, "ws-source", transfer.audits[0].ExpectedSourceWorkspaceID)
}

func TestAuditTaskTransferAttemptActionReturnsErrorWhenAuditFails(t *testing.T) {
	transfer := &recordingTaskTransferService{auditErr: errors.New("audit unavailable")}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}

	response, err := h.handleAuditTaskTransferAttempt(
		context.Background(),
		&ws.Message{
			ID:      "audit",
			Action:  ws.ActionMCPAuditTaskTransferAttempt,
			Payload: transferTaskMessage(t).Payload,
		},
	)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeInternalError)
	require.Empty(t, transfer.commands)
	require.Len(t, transfer.audits, 1)
}

func TestHandleTransferTaskAllowsCoordinatorReplayFromDestinationWorkspace(t *testing.T) {
	transfer := &recordingTaskTransferService{replayOK: true, replayActor: models.TaskTransferActor{
		Kind: models.TaskTransferActorCoordinator, ID: "ceo-1", SessionID: "caller-session",
	}}
	authorizer := fixedTaskTransferAuthorizer{ok: true, actor: models.TaskTransferActor{
		Kind: models.TaskTransferActorCoordinator, ID: "ceo-1", SessionID: "caller-session",
	}}
	h := &Handlers{taskTransferSvc: transfer, transferAuthorizer: authorizer, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "ws-destination", CallerTaskID: "task-1", CallerSessionID: "caller-session",
		Surface: mcpprofile.SurfaceOfficeTask,
	})
	response, err := h.handleTransferTask(ctx, transferTaskMessage(t))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Len(t, transfer.commands, 1)
}

func TestHandleTransferTaskRejectsCoordinatorReplayAfterAuthorizationRevocation(t *testing.T) {
	transfer := &recordingTaskTransferService{replayOK: true, replayActor: models.TaskTransferActor{
		Kind: models.TaskTransferActorCoordinator, ID: "ceo-1", SessionID: "caller-session",
	}}
	h := &Handlers{
		taskTransferSvc: transfer, transferAuthorizer: fixedTaskTransferAuthorizer{ok: false}, logger: testLogger(t),
	}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "ws-source", CallerTaskID: "caller-task", CallerSessionID: "caller-session",
		Surface: mcpprofile.SurfaceOfficeTask,
	})
	response, err := h.handleTransferTask(ctx, transferTaskMessage(t))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeNotFound)
	require.Empty(t, transfer.commands)
}

func TestHandleTransferTaskReattestsCommittedCoordinatorSelfTransfer(t *testing.T) {
	transfer := &recordingTaskTransferService{replayOK: true, replayActor: models.TaskTransferActor{
		Kind: models.TaskTransferActorCoordinator, ID: "ceo-source", SessionID: "caller-session",
	}}
	authorizer := fixedTaskTransferAuthorizer{replayOK: true, replayActor: models.TaskTransferActor{
		Kind: models.TaskTransferActorCoordinator, ID: "ceo-source", SessionID: "caller-session", CallerTaskID: "task-1",
	}}
	h := &Handlers{taskTransferSvc: transfer, transferAuthorizer: authorizer, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "ws-destination", CallerTaskID: "task-1", CallerSessionID: "caller-session",
		Surface: mcpprofile.SurfaceOfficeTask,
	})
	response, err := h.handleTransferTask(ctx, transferTaskMessage(t))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Len(t, transfer.commands, 1)
	require.Equal(t, "task-1", transfer.commands[0].Actor.CallerTaskID)
}

func TestHandleTransferTaskReturnsConflictForChangedCoordinatorReplay(t *testing.T) {
	transfer := &recordingTaskTransferService{replayErr: repoerrors.ErrTaskTransferConflict}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "ws-source", CallerTaskID: "caller-task", CallerSessionID: "caller-session",
		Surface: mcpprofile.SurfaceOfficeTask,
	})
	response, err := h.handleTransferTask(ctx, transferTaskMessage(t))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeConflict)
	require.Empty(t, transfer.commands)
	require.Len(t, transfer.audits, 1)
}

func TestHandleTransferTaskDeniesFreshCoordinatorRequestFromDestinationWorkspace(t *testing.T) {
	transfer := &recordingTaskTransferService{}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "ws-destination", CallerTaskID: "task-1", CallerSessionID: "caller-session",
		Surface: mcpprofile.SurfaceOfficeTask,
	})
	response, err := h.handleTransferTask(ctx, transferTaskMessage(t))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeNotFound)
	require.Empty(t, transfer.commands)
}

func TestHandleTransferTaskAttestsHumanAndCoordinator(t *testing.T) {
	tests := []struct {
		name      string
		context   func() context.Context
		authorize TaskTransferCoordinatorAuthorizer
		wantKind  models.TaskTransferActorKind
		wantID    string
	}{
		{name: "human", context: func() context.Context {
			return authn.WithIdentity(context.Background(), authn.Identity{UserID: "human-1"})
		}, wantKind: models.TaskTransferActorHuman, wantID: "human-1"},
		{name: "server attested coordinator", context: func() context.Context {
			return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
				WorkspaceID: "ws-source", CallerTaskID: "caller-task", CallerSessionID: "caller-session",
				Surface: mcpprofile.SurfaceOfficeTask,
			})
		}, authorize: fixedTaskTransferAuthorizer{ok: true, actor: models.TaskTransferActor{
			Kind: models.TaskTransferActorCoordinator, ID: "ceo-1", SessionID: "caller-session",
		}}, wantKind: models.TaskTransferActorCoordinator, wantID: "ceo-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transfer := &recordingTaskTransferService{}
			h := &Handlers{taskTransferSvc: transfer, transferAuthorizer: tt.authorize, logger: testLogger(t)}
			response, err := h.handleTransferTask(tt.context(), transferTaskMessage(t))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			require.Len(t, transfer.commands, 1)
			require.Equal(t, tt.wantKind, transfer.commands[0].Actor.Kind)
			require.Equal(t, tt.wantID, transfer.commands[0].Actor.ID)
		})
	}
}

func TestHandleTransferTaskDeniesUnattestedSessionAndRedactsFailures(t *testing.T) {
	tests := []struct {
		name     string
		surface  mcpprofile.Surface
		service  error
		wantCode string
	}{
		{name: "ordinary task agent", surface: mcpprofile.SurfaceKanbanTask, wantCode: ws.ErrorCodeNotFound},
		{name: "automation", surface: mcpprofile.SurfaceAutomation, wantCode: ws.ErrorCodeNotFound},
		{name: "stale request", surface: mcpprofile.SurfaceOfficeTask,
			service: repoerrors.ErrTaskTransferConflict, wantCode: ws.ErrorCodeConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transfer := &recordingTaskTransferService{err: tt.service}
			authorizer := fixedTaskTransferAuthorizer{ok: true, actor: models.TaskTransferActor{
				Kind: models.TaskTransferActorCoordinator, ID: "ceo-1", SessionID: "session-1",
			}}
			h := &Handlers{taskTransferSvc: transfer, transferAuthorizer: authorizer, logger: testLogger(t)}
			ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
				WorkspaceID: "ws-source", CallerTaskID: "caller-task", CallerSessionID: "session-1", Surface: tt.surface,
			})
			response, err := h.handleTransferTask(ctx, transferTaskMessage(t))
			require.NoError(t, err)
			assertWSError(t, response, tt.wantCode)
			if (tt.surface == mcpprofile.SurfaceKanbanTask || tt.surface == mcpprofile.SurfaceAutomation) && len(transfer.commands) != 0 {
				t.Fatal("unattested session reached transfer service")
			}
			if tt.surface == mcpprofile.SurfaceKanbanTask || tt.surface == mcpprofile.SurfaceAutomation {
				require.Len(t, transfer.audits, 1)
				require.Equal(t, models.TaskTransferActorRejected, transfer.audits[0].Actor.Kind)
			}
		})
	}

	transfer := &recordingTaskTransferService{err: errors.New("database contains secret detail")}
	h := &Handlers{taskTransferSvc: transfer, logger: testLogger(t)}
	response, err := h.handleTransferTask(context.Background(), transferTaskMessage(t))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeInternalError)
}

func transferTaskMessage(t *testing.T) *ws.Message {
	t.Helper()
	return makeWSMessage(t, ws.ActionMCPTransferTask, map[string]interface{}{
		"task_id": "task-1", "expected_source_workspace_id": "ws-source",
		"expected_source_workflow_id": "wf-source", "expected_source_workflow_step_id": "step-source",
		"expected_task_updated_at": time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		"destination_workspace_id": "ws-destination", "destination_workflow_id": "wf-destination",
		"destination_workflow_step_id": "step-destination", "idempotency_key": "key-1",
		"preservation_policy": models.TaskTransferPreservationPolicyV1,
	})
}
