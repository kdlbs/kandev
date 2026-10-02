package sqlite

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestUpdateTaskSessionWorkspaceBindingIfCurrentAttempt(t *testing.T) {
	tests := []struct {
		name          string
		state         models.TaskSessionState
		storedAttempt string
		expectedState models.TaskSessionState
		attemptID     string
		wantChanged   bool
	}{
		{
			name:          "agent resume owns starting attempt",
			state:         models.TaskSessionStateStarting,
			storedAttempt: "attempt-current",
			expectedState: models.TaskSessionStateStarting,
			attemptID:     "attempt-current",
			wantChanged:   true,
		},
		{
			name:          "workspace-only resume owns unchanged state",
			state:         models.TaskSessionStateWaitingForInput,
			expectedState: models.TaskSessionStateWaitingForInput,
			wantChanged:   true,
		},
		{
			name:          "cancelled session is left untouched",
			state:         models.TaskSessionStateCancelled,
			expectedState: models.TaskSessionStateStarting,
			attemptID:     "attempt-current",
		},
		{
			name:          "replaced startup attempt is left untouched",
			state:         models.TaskSessionStateStarting,
			storedAttempt: "attempt-successor",
			expectedState: models.TaskSessionStateStarting,
			attemptID:     "attempt-current",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			const (
				taskID        = "task-workspace-binding"
				sessionID     = "session-workspace-binding"
				environmentID = "environment-workspace-binding"
			)
			if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Workspace binding"}); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			metadata := map[string]interface{}{"retained": "value"}
			if tt.storedAttempt != "" {
				metadata[models.SessionMetaKeyAgentStartAttemptID] = tt.storedAttempt
			}
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID:       sessionID,
				TaskID:   taskID,
				State:    tt.state,
				Metadata: metadata,
			}); err != nil {
				t.Fatalf("CreateTaskSession: %v", err)
			}

			update := &models.TaskSession{
				ID:                sessionID,
				TaskID:            taskID,
				TaskEnvironmentID: environmentID,
				WorkspacePath:     "/tasks/workspace-binding",
			}
			changed, _, err := repo.UpdateTaskSessionWorkspaceBindingIfCurrentAttempt(
				ctx, update, tt.expectedState, tt.attemptID,
			)
			if err != nil {
				t.Fatalf("UpdateTaskSessionWorkspaceBindingIfCurrentAttempt: %v", err)
			}
			if changed != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tt.wantChanged)
			}

			var gotEnvironmentID, gotWorkspacePath, gotState string
			if err := repo.db.QueryRowContext(ctx, `
				SELECT task_environment_id, workspace_path, state
				FROM task_sessions WHERE id = ?
			`, sessionID).Scan(&gotEnvironmentID, &gotWorkspacePath, &gotState); err != nil {
				t.Fatalf("read raw task session: %v", err)
			}
			if tt.wantChanged {
				if gotEnvironmentID != environmentID || gotWorkspacePath != "/tasks/workspace-binding" {
					t.Fatalf("raw binding = (%q, %q), want (%q, %q)",
						gotEnvironmentID, gotWorkspacePath, environmentID, "/tasks/workspace-binding")
				}
			} else if gotEnvironmentID != "" || gotWorkspacePath != "" {
				t.Fatalf("rejected write changed raw binding to (%q, %q)", gotEnvironmentID, gotWorkspacePath)
			}
			if gotState != string(tt.state) {
				t.Fatalf("state = %q, want unchanged state %q", gotState, tt.state)
			}

			persisted, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				t.Fatalf("GetTaskSession: %v", err)
			}
			if got := persisted.Metadata["retained"]; got != "value" {
				t.Fatalf("unrelated metadata = %v, want retained value", got)
			}
			if tt.storedAttempt != "" {
				if got := models.StringFromAny(persisted.Metadata[models.SessionMetaKeyAgentStartAttemptID]); got != tt.storedAttempt {
					t.Fatalf("startup attempt = %q, want unchanged %q", got, tt.storedAttempt)
				}
			}
		})
	}
}

func TestPostgresUpdateTaskSessionWorkspaceBindingIfCurrentAttempt(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	const taskID = "task-workspace-binding-postgres"
	seedPostgresTask(t, repo, taskID)

	tests := []postgresWorkspaceBindingCase{
		{
			name:          "owned agent binding",
			state:         models.TaskSessionStateStarting,
			storedAttempt: "attempt-current",
			expectedState: models.TaskSessionStateStarting,
			attemptID:     "attempt-current",
			wantChanged:   true,
		},
		{
			name:          "workspace-only state guard",
			state:         models.TaskSessionStateWaitingForInput,
			expectedState: models.TaskSessionStateWaitingForInput,
			wantChanged:   true,
		},
		{
			name:          "replaced attempt rejected",
			state:         models.TaskSessionStateStarting,
			storedAttempt: "attempt-successor",
			expectedState: models.TaskSessionStateStarting,
			attemptID:     "attempt-current",
		},
		{
			name:          "cancelled session rejected",
			state:         models.TaskSessionStateCancelled,
			storedAttempt: "attempt-current",
			expectedState: models.TaskSessionStateStarting,
			attemptID:     "attempt-current",
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPostgresWorkspaceBindingCase(t, repo, ctx, taskID, index, tt)
		})
	}
}

type postgresWorkspaceBindingCase struct {
	name          string
	state         models.TaskSessionState
	storedAttempt string
	expectedState models.TaskSessionState
	attemptID     string
	wantChanged   bool
}

func assertPostgresWorkspaceBindingCase(
	t *testing.T,
	repo *Repository,
	ctx context.Context,
	taskID string,
	index int,
	tt postgresWorkspaceBindingCase,
) {
	t.Helper()
	sessionID := "session-workspace-binding-postgres-" + strconv.Itoa(index)
	metadata := map[string]interface{}{"retained": "value"}
	if tt.storedAttempt != "" {
		metadata[models.SessionMetaKeyAgentStartAttemptID] = tt.storedAttempt
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: tt.state,
		ErrorMessage: "preserve this error", Metadata: metadata,
	}))
	update := &models.TaskSession{
		ID: sessionID, TaskID: taskID,
		TaskEnvironmentID: "environment-postgres",
		WorkspacePath:     "/tasks/postgres-workspace",
	}
	changed, _, err := repo.UpdateTaskSessionWorkspaceBindingIfCurrentAttempt(ctx, update, tt.expectedState, tt.attemptID)
	require.NoError(t, err)
	require.Equal(t, tt.wantChanged, changed)
	assertPostgresWorkspaceBindingReadback(t, repo, ctx, sessionID, tt)
}

func assertPostgresWorkspaceBindingReadback(
	t *testing.T,
	repo *Repository,
	ctx context.Context,
	sessionID string,
	tt postgresWorkspaceBindingCase,
) {
	t.Helper()
	var state, message, environmentID, workspacePath, rawMetadata string
	require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`
		SELECT state, error_message, task_environment_id, workspace_path, metadata
		FROM task_sessions WHERE id = ?
	`), sessionID).Scan(&state, &message, &environmentID, &workspacePath, &rawMetadata))
	require.Equal(t, string(tt.state), state)
	require.Equal(t, "preserve this error", message)
	if tt.wantChanged {
		require.Equal(t, "environment-postgres", environmentID)
		require.Equal(t, "/tasks/postgres-workspace", workspacePath)
	} else {
		require.Empty(t, environmentID)
		require.Empty(t, workspacePath)
	}
	var metadata map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(rawMetadata), &metadata))
	require.Equal(t, "value", metadata["retained"])
	if tt.storedAttempt != "" {
		require.Equal(t, tt.storedAttempt, metadata[models.SessionMetaKeyAgentStartAttemptID])
	}
}

func TestPostgresUpdateTaskSessionResumeStateIfCurrentAttempt(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	const taskID = "task-resume-rollback-postgres"
	seedPostgresTask(t, repo, taskID)
	assertPostgresResumeRollback(t, repo, ctx, taskID, "session-resume-rollback-postgres", "attempt-rollback", true)
	assertPostgresResumeRollback(t, repo, ctx, taskID, "session-resume-rollback-successor-postgres", "attempt-successor", false)
}

func assertPostgresResumeRollback(
	t *testing.T,
	repo *Repository,
	ctx context.Context,
	taskID, sessionID, storedAttempt string,
	wantChanged bool,
) {
	t.Helper()
	message, source := "old error", "current"
	if !wantChanged {
		message, source = "successor error", "successor"
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateStarting,
		ErrorMessage: message,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyAgentStartAttemptID:   storedAttempt,
			models.SessionMetaKeyGitCredentialSnapshot: map[string]interface{}{"source": source},
			"retained": "value",
		},
	}))
	changed, _, err := repo.UpdateTaskSessionResumeStateIfCurrentAttempt(
		ctx, taskID, sessionID, "attempt-rollback",
		models.TaskSessionStateStarting, models.TaskSessionStateFailed,
		"resume failed", true, true, true, map[string]interface{}{"source": "previous", "transport": "profile"},
	)
	require.NoError(t, err)
	require.Equal(t, wantChanged, changed)
	assertPostgresResumeRollbackReadback(t, repo, ctx, sessionID, storedAttempt, message, source, wantChanged)
}

func assertPostgresResumeRollbackReadback(
	t *testing.T,
	repo *Repository,
	ctx context.Context,
	sessionID, storedAttempt, expectedMessage, expectedCredentialSource string,
	wantChanged bool,
) {
	t.Helper()
	var state, message, rawMetadata string
	require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`
		SELECT state, error_message, metadata FROM task_sessions WHERE id = ?
	`), sessionID).Scan(&state, &message, &rawMetadata))
	var metadata map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(rawMetadata), &metadata))
	expectedState := models.TaskSessionStateStarting
	wantMessage := expectedMessage
	credentialSource := expectedCredentialSource
	if wantChanged {
		expectedState = models.TaskSessionStateFailed
		wantMessage = "resume failed"
		credentialSource = "previous"
	}
	require.Equal(t, string(expectedState), state)
	require.Equal(t, wantMessage, message)
	require.Equal(t, storedAttempt, metadata[models.SessionMetaKeyAgentStartAttemptID])
	require.Equal(t, credentialSource,
		metadata[models.SessionMetaKeyGitCredentialSnapshot].(map[string]interface{})["source"])
	require.Equal(t, "value", metadata["retained"])
}

func TestUpdateTaskSessionResumeStateIfCurrentAttempt(t *testing.T) {
	for _, tt := range []struct {
		name          string
		storedAttempt string
		attemptID     string
		wantChanged   bool
	}{
		{name: "current attempt", storedAttempt: "attempt-current", attemptID: "attempt-current", wantChanged: true},
		{name: "successor attempt", storedAttempt: "attempt-successor", attemptID: "attempt-current"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			const taskID, sessionID = "task-resume-rollback", "session-resume-rollback"
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Resume rollback"}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: taskID, State: models.TaskSessionStateStarting,
				ErrorMessage: "preserve successor error",
				Metadata: map[string]interface{}{
					models.SessionMetaKeyAgentStartAttemptID:   tt.storedAttempt,
					models.SessionMetaKeyGitCredentialSnapshot: map[string]interface{}{"source": "current"},
					"retained": "value",
				},
			}))

			oldSnapshot := map[string]interface{}{"source": "previous", "transport": "profile"}
			changed, _, err := repo.UpdateTaskSessionResumeStateIfCurrentAttempt(
				ctx, taskID, sessionID, tt.attemptID,
				models.TaskSessionStateStarting, models.TaskSessionStateFailed,
				"resume failed", true, true, true, oldSnapshot,
			)
			require.NoError(t, err)
			require.Equal(t, tt.wantChanged, changed)

			stored, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			if tt.wantChanged {
				require.Equal(t, models.TaskSessionStateFailed, stored.State)
				require.Equal(t, "resume failed", stored.ErrorMessage)
				require.Equal(t, oldSnapshot, stored.Metadata[models.SessionMetaKeyGitCredentialSnapshot])
			} else {
				require.Equal(t, models.TaskSessionStateStarting, stored.State)
				require.Equal(t, "preserve successor error", stored.ErrorMessage)
				credential := stored.Metadata[models.SessionMetaKeyGitCredentialSnapshot].(map[string]interface{})
				require.Equal(t, "current", credential["source"])
				require.Equal(t, "attempt-successor", stored.Metadata[models.SessionMetaKeyAgentStartAttemptID])
			}
			require.Equal(t, "value", stored.Metadata["retained"])
		})
	}
}

func TestUpdateTaskSessionCredentialSnapshotIfCurrentAttemptPreservesState(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID, sessionID = "task-resume-credential-restore", "session-resume-credential-restore"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Credential restore"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateStarting,
		ErrorMessage: "keep this error",
		Metadata: map[string]interface{}{
			models.SessionMetaKeyAgentStartAttemptID:   "attempt-current",
			models.SessionMetaKeyGitCredentialSnapshot: map[string]interface{}{"source": "new"},
			"retained": "value",
		},
	}))

	changed, _, err := repo.UpdateTaskSessionResumeStateIfCurrentAttempt(
		ctx, taskID, sessionID, "attempt-current",
		models.TaskSessionStateStarting, models.TaskSessionStateStarting,
		"", false, true, true, map[string]interface{}{"source": "previous"},
	)
	require.NoError(t, err)
	require.True(t, changed)
	stored, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, stored.State)
	require.Equal(t, "keep this error", stored.ErrorMessage)
	require.Equal(t, map[string]interface{}{"source": "previous"}, stored.Metadata[models.SessionMetaKeyGitCredentialSnapshot])
	require.Equal(t, "value", stored.Metadata["retained"])
}
