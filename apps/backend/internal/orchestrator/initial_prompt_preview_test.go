package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PROMPT-ATTACHMENTS-001.8
// @covers AC-TASKS-PROMPT-ATTACHMENTS-001.9
// @covers AC-TASKS-PROMPT-ATTACHMENTS-001.10
func TestInitialPromptPreviewPersistsOnlyInPreparedSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task1", WorkspaceID: "ws1", Title: "Task"}))
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Task"}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	eventBus := &mockEventBus{}
	svc.eventBus = eventBus
	preview := models.NewInitialPromptPreview("submitted text", []v1.MessageAttachment{
		{AttachmentID: "image-1", Type: "image", Name: "screen.png", MimeType: "image/png", SizeBytes: 32},
	})
	response, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: "task1", AgentProfileID: "profile1", Intent: IntentPrepare,
		DeferredStart: true, InitialPromptPreview: preview,
	})
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, response.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCreated, session.State)
	encoded, err := json.Marshal(session.Metadata[models.SessionMetaKeyInitialPromptPreview])
	require.NoError(t, err)
	expected, err := json.Marshal(preview)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(encoded))
	published := eventBus.published()
	require.NotEmpty(t, published)
	foundPreview := false
	for _, event := range published {
		data, ok := event.Event.Data.(map[string]interface{})
		if !ok || data["new_state"] != string(models.TaskSessionStateCreated) {
			continue
		}
		metadata, ok := data["session_metadata"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, session.Metadata[models.SessionMetaKeyInitialPromptPreview], metadata[models.SessionMetaKeyInitialPromptPreview])
		foundPreview = true
	}
	require.True(t, foundPreview, "created-session publication must already contain the preview")

	// A later session on the same task must not inherit submission display data.
	environment, err := repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	require.NoError(t, err)
	environment.Status = models.TaskEnvironmentStatusReady
	environment.MaterializationSessionID = ""
	require.NoError(t, repo.UpdateTaskEnvironment(ctx, environment))
	siblingID, err := svc.PrepareTaskSession(ctx, "task1", "profile1", "", "", "", false)
	require.NoError(t, err)
	sibling, err := repo.GetTaskSession(ctx, siblingID)
	require.NoError(t, err)
	require.NotEqual(t, session.ID, sibling.ID)
	require.NotContains(t, sibling.Metadata, models.SessionMetaKeyInitialPromptPreview)

	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, "unrelated", "kept"))
	session.State = models.TaskSessionStateFailed
	// Fetch the current metadata before persisting the state, as production does.
	failed, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	failed.State = session.State
	require.NoError(t, repo.UpdateTaskSession(ctx, failed))
	reloaded, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, session.Metadata[models.SessionMetaKeyInitialPromptPreview], reloaded.Metadata[models.SessionMetaKeyInitialPromptPreview])
	require.Equal(t, "kept", reloaded.Metadata["unrelated"])
}

type initialPreviewFailureStore struct {
	sessionExecutorStore
	sessionID string
}

func (s *initialPreviewFailureStore) SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error {
	if key == models.SessionMetaKeyInitialPromptPreview {
		s.sessionID = sessionID
		return errors.New("preview storage unavailable")
	}
	return s.sessionExecutorStore.SetSessionMetadataKey(ctx, sessionID, key, value)
}

func TestInitialPromptPreviewWriteFailureSettlesSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task1", WorkspaceID: "ws1", Title: "Task"}))
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Task"}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	failing := &initialPreviewFailureStore{sessionExecutorStore: svc.repo}
	svc.repo = failing
	_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: "task1", AgentProfileID: "profile1", Intent: IntentPrepare,
		DeferredStart: true, InitialPromptPreview: models.NewInitialPromptPreview("submitted text", nil),
	})
	require.ErrorContains(t, err, "preview storage unavailable")
	session, err := repo.GetTaskSession(ctx, failing.sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, session.State)
}
