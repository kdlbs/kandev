package orchestrator

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestHandleAgentStartFailed_AuthenticationEvidenceSurvivesRepositoryReopen(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "authentication-recovery.db")
	var activeDB *sqlx.DB
	t.Cleanup(func() {
		if activeDB != nil {
			_ = activeDB.Close()
		}
	})

	openRepository := func() (*sqliterepo.Repository, *sqlx.DB) {
		rawDB, err := db.OpenSQLite(databasePath)
		require.NoError(t, err)
		database := sqlx.NewDb(rawDB, "sqlite3")
		repo, err := sqliterepo.NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo, database
	}

	repo, database := openRepository()
	activeDB = database
	seedSession(t, repo, "task-auth-reopen", "session-auth-reopen", "step1")
	messageCreator := newServiceBackedMessageCreator(repo)
	manager := &mockAgentManager{repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-auth-reopen", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
	svc.messageCreator = messageCreator
	t.Cleanup(svc.stopDynamicSuccessorWorkers)

	bootstrapFailure := &lifecycle.BootstrapFailure{
		Operation: models.AgentErrorCauseOperationStart,
		Code:      models.AgentErrorCauseCodeAuthenticationRequired,
		Detail:    "Agent authentication is required.",
		SessionFailure: &agentctltypes.SessionOperationFailure{
			Code: agentctltypes.SessionOperationFailureCodeAuthenticationRequired,
			AuthMethods: []streams.AuthMethodInfo{{
				ID:   "cursor_login",
				Name: "Cursor login",
			}},
		},
	}
	require.True(t, svc.handleAgentStartFailed(ctx, "task-auth-reopen", "session-auth-reopen", "execution-auth-reopen", bootstrapFailure, false))
	waitForFailureRecovery(t, svc)

	initial, err := repo.GetTaskSession(ctx, "session-auth-reopen")
	require.NoError(t, err)
	lastError, found := models.LoadLastAgentError(initial.Metadata)
	require.True(t, found)
	require.Equal(t, models.LaunchErrorCategoryGenericLaunchFailure, lastError.Code)
	require.Equal(t, "execution-auth-reopen", lastError.ExecutionID)
	require.Len(t, lastError.Causes, 1)
	require.Equal(t, models.AgentErrorCauseCodeAuthenticationRequired, lastError.Causes[0].Code)
	require.Equal(t, "Agent authentication is required.", lastError.Causes[0].Detail)

	require.NoError(t, database.Close())
	activeDB = nil
	reopenedRepo, reopenedDB := openRepository()
	activeDB = reopenedDB

	reopened, err := reopenedRepo.GetTaskSession(ctx, "session-auth-reopen")
	require.NoError(t, err)
	reopenedError, found := models.LoadLastAgentError(reopened.Metadata)
	require.True(t, found)
	require.Equal(t, lastError.Code, reopenedError.Code)
	require.Equal(t, lastError.Causes, reopenedError.Causes)
	require.Equal(t, lastError.ExecutionID, reopenedError.ExecutionID)

	messages, err := reopenedRepo.ListMessages(ctx, "session-auth-reopen")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	metadata := messages[0].Metadata
	require.Equal(t, true, metadata["is_auth_error"])
	require.Equal(t, false, metadata["has_resume_token"])
	methodsJSON, err := json.Marshal(metadata["auth_methods"])
	require.NoError(t, err)
	require.JSONEq(t, `[{"id":"cursor_login","name":"Cursor login"}]`, string(methodsJSON))
	actionsJSON, err := json.Marshal(metadata["actions"])
	require.NoError(t, err)
	var actions []map[string]interface{}
	require.NoError(t, json.Unmarshal(actionsJSON, &actions))
	require.Len(t, actions, 1)
	require.Equal(t, recoveryRestartButtonTestID, actions[0]["test_id"])
}

func TestCreateRecoveryStatusMessage_AuthenticationCauseSurvivesMissingMethods(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-auth-code-only", "session-auth-code-only", "step1")
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	creator := &mockMessageCreator{}
	svc.messageCreator = creator

	require.NoError(t, svc.createRecoveryStatusMessage(ctx, watcher.AgentEventData{
		TaskID:       "task-auth-code-only",
		SessionID:    "session-auth-code-only",
		ErrorMessage: "Agent authentication is required.",
		FailureCode:  models.LaunchErrorCategoryGenericLaunchFailure,
		Causes: []models.AgentErrorCause{{
			Operation: models.AgentErrorCauseOperationStart,
			Code:      models.AgentErrorCauseCodeAuthenticationRequired,
			Detail:    "Agent authentication is required.",
		}},
	}, ""))

	require.Len(t, creator.sessionMessages, 1)
	metadata := creator.sessionMessages[0].metadata
	require.Equal(t, true, metadata["is_auth_error"])
	require.NotContains(t, metadata, "auth_methods")
	actions := metadata["actions"].([]map[string]interface{})
	require.Len(t, actions, 1)
	require.Equal(t, recoveryRestartButtonTestID, actions[0]["test_id"])
}

func TestIsAuthenticationRecoveryFailure_UsesTypedEvidenceBeforeText(t *testing.T) {
	cases := []struct {
		name string
		data watcher.AgentEventData
		want bool
	}{
		{
			name: "legacy text",
			data: watcher.AgentEventData{ErrorMessage: "authentication required"},
			want: true,
		},
		{
			name: "typed cause with generic detail",
			data: watcher.AgentEventData{
				ErrorMessage: "The agent could not start.",
				FailureCode:  models.LaunchErrorCategoryGenericLaunchFailure,
				Causes: []models.AgentErrorCause{{
					Operation: models.AgentErrorCauseOperationStart,
					Code:      models.AgentErrorCauseCodeAuthenticationRequired,
					Detail:    "Agent authentication is required.",
				}},
			},
			want: true,
		},
		{
			name: "contradictory typed cause blocks legacy text",
			data: watcher.AgentEventData{
				ErrorMessage: "authentication required",
				Causes: []models.AgentErrorCause{{
					Operation: models.AgentErrorCauseOperationStart,
					Code:      models.AgentErrorCauseCodePermissionDenied,
					Detail:    "The required contribution access was denied.",
				}},
			},
			want: false,
		},
		{
			name: "authoritative routing code",
			data: watcher.AgentEventData{
				ErrorMessage: "The agent could not start.",
				FailureCode:  string(routingerr.CodeAuthRequired),
			},
			want: true,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, isAuthenticationRecoveryFailure(test.data))
		})
	}
}

type recoveryAuthMethodAgentManager struct {
	*mockAgentManager
	methods []streams.AuthMethodInfo
}

func (m *recoveryAuthMethodAgentManager) GetSessionAuthMethods(string) []streams.AuthMethodInfo {
	return m.methods
}

func TestRecoveryAuthMethodsUsesCachedMethodsOnlyForCurrentExecution(t *testing.T) {
	manager := &recoveryAuthMethodAgentManager{
		mockAgentManager: &mockAgentManager{
			getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
				return "execution-current", nil
			},
		},
		methods: []streams.AuthMethodInfo{{
			ID:   "cursor_login",
			Name: "Cursor login",
			Meta: map[string]interface{}{"private": "must be removed"},
		}},
	}
	svc := &Service{agentManager: manager}

	current := svc.recoveryAuthMethods(context.Background(), watcher.AgentEventData{
		SessionID:        "session",
		AgentExecutionID: "execution-current",
	})
	require.Len(t, current, 1)
	require.Equal(t, "cursor_login", current[0].ID)
	require.Nil(t, current[0].Meta)

	stale := svc.recoveryAuthMethods(context.Background(), watcher.AgentEventData{
		SessionID:        "session",
		AgentExecutionID: "execution-stale",
	})
	require.Empty(t, stale)
}

func TestHandleAgentStartFailedAuthenticationKeepsTerminalAndSuccessorGuards(t *testing.T) {
	for _, test := range []struct {
		name       string
		state      models.TaskSessionState
		successor  bool
		wantHandle bool
	}{
		{name: "cancelled session", state: models.TaskSessionStateCancelled, wantHandle: false},
		{name: "successor execution", state: models.TaskSessionStateStarting, successor: true, wantHandle: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-auth-guard", "session-auth-guard", "step1")
			require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-auth-guard", test.state, ""))
			if test.successor {
				seedExecutorRunning(t, repo, "session-auth-guard", "task-auth-guard", "execution-successor")
			}
			manager := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			creator := &mockMessageCreator{}
			svc.messageCreator = creator
			t.Cleanup(svc.stopDynamicSuccessorWorkers)

			failure := &lifecycle.BootstrapFailure{
				Operation: models.AgentErrorCauseOperationStart,
				Code:      models.AgentErrorCauseCodeAuthenticationRequired,
				Detail:    "Agent authentication is required.",
				SessionFailure: &agentctltypes.SessionOperationFailure{
					Code: agentctltypes.SessionOperationFailureCodeAuthenticationRequired,
				},
			}
			handled := svc.handleAgentStartFailed(ctx, "task-auth-guard", "session-auth-guard", "execution-stale", failure, false)
			require.Equal(t, test.wantHandle, handled)
			require.Empty(t, creator.sessionMessages)
			session, err := repo.GetTaskSession(ctx, "session-auth-guard")
			require.NoError(t, err)
			_, found := models.LoadLastAgentError(session.Metadata)
			require.False(t, found, "guarded authentication evidence must not replace current session error state")
		})
	}
}

func TestEligibleInitialSubmissionRecoveryAllowsWaitingAuthenticationBootstrap(t *testing.T) {
	dismissedAt := time.Now().UTC()
	authenticationError := models.LastAgentError{
		Message: "Agent authentication is required.",
		Phase:   models.LaunchErrorPhaseBootstrap,
		Causes: []models.AgentErrorCause{{
			Operation: models.AgentErrorCauseOperationStart,
			Code:      models.AgentErrorCauseCodeAuthenticationRequired,
		}},
	}
	tests := []struct {
		name                string
		state               models.TaskSessionState
		failure             models.LastAgentError
		hasProviderIdentity bool
		want                bool
	}{
		{name: "failed bootstrap", state: models.TaskSessionStateFailed, failure: authenticationError, want: true},
		{name: "waiting after authentication bootstrap", state: models.TaskSessionStateWaitingForInput, failure: authenticationError, want: true},
		{
			name:  "waiting after generic bootstrap",
			state: models.TaskSessionStateWaitingForInput,
			failure: models.LastAgentError{
				Message: "The agent could not start.",
				Phase:   models.LaunchErrorPhaseBootstrap,
			},
		},
		{
			name:                "waiting authentication bootstrap with provider identity",
			state:               models.TaskSessionStateWaitingForInput,
			failure:             authenticationError,
			hasProviderIdentity: true,
		},
		{
			name:  "waiting after a non-start authentication failure",
			state: models.TaskSessionStateWaitingForInput,
			failure: models.LastAgentError{
				Message: "Agent authentication is required.",
				Phase:   models.LaunchErrorPhaseBootstrap,
				Causes: []models.AgentErrorCause{{
					Operation: models.AgentErrorCauseOperationResume,
					Code:      models.AgentErrorCauseCodeAuthenticationRequired,
				}},
			},
		},
		{
			name:  "dismissed waiting authentication bootstrap",
			state: models.TaskSessionStateWaitingForInput,
			failure: models.LastAgentError{
				Message:     authenticationError.Message,
				Phase:       authenticationError.Phase,
				Causes:      authenticationError.Causes,
				DismissedAt: &dismissedAt,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := test.failure
			if failure.OccurredAt.IsZero() {
				failure.OccurredAt = time.Now().UTC()
			}
			metadata := map[string]interface{}{
				models.SessionMetaKeyLastAgentError: failure,
			}
			if test.hasProviderIdentity {
				metadata["acp"] = map[string]interface{}{"session_id": "existing-native-session"}
			}
			require.Equal(t, test.want, eligibleInitialSubmissionRecovery(&models.TaskSession{
				State:    test.state,
				Metadata: metadata,
			}))
		})
	}
}
