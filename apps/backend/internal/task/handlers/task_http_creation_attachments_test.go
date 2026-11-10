package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type creationAttachmentEventBus struct {
	bus.EventBus
	repo  *sqliterepo.Repository
	state []*models.TaskMessageAttachment
}

func (b *creationAttachmentEventBus) Publish(ctx context.Context, _ string, event *bus.Event) error {
	if event == nil || event.Type != "task.created" {
		return nil
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}
	taskID, _ := data["task_id"].(string)
	rows, err := b.repo.ListTaskCreationAttachments(ctx, taskID)
	if err != nil {
		return err
	}
	b.state = rows
	return nil
}

func newCreationAttachmentHandler(
	t *testing.T,
) (*TaskHandlers, *sqliterepo.Repository, *service.AttachmentService, *creationAttachmentEventBus) {
	t.Helper()
	database, _ := workflowHandlerTemplate.Open(t)
	repo := sqliterepo.NewWithInitializedDB(database, database, nil)
	ctx := t.Context()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-http-creation", Name: "HTTP creation"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-http-creation", WorkspaceID: "ws-http-creation", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	log := newTestLogger(t)
	eventBus := &creationAttachmentEventBus{repo: repo}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, eventBus, log, service.RepositoryDiscoveryConfig{})
	attachmentService, err := service.NewAttachmentService(repo, t.TempDir(), nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	svc.SetAttachmentService(attachmentService)
	return &TaskHandlers{service: svc, logger: log}, repo, attachmentService, eventBus
}

func stageHTTPCreationAttachment(
	t *testing.T, svc *service.AttachmentService, ctx context.Context, name, mime, kind, mode string,
) *models.TaskMessageAttachment {
	t.Helper()
	attachment, err := svc.Stage(ctx, "owner-http", "ws-http-creation", name, mime, kind, mode, strings.NewReader(name))
	if err != nil {
		t.Fatalf("stage %s: %v", name, err)
	}
	return attachment
}

func descriptorForHTTP(attachment *models.TaskMessageAttachment) v1.MessageAttachment {
	return v1.MessageAttachment{
		Type: attachment.Kind, AttachmentID: attachment.ID, MimeType: attachment.MimeType,
		Name: attachment.Name, SizeBytes: attachment.SizeBytes, DeliveryMode: attachment.DeliveryMode,
	}
}

func TestHTTPCreateTaskOfficeAttachmentsBeforeWake(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo, attachmentService, eventBus := newCreationAttachmentHandler(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner-http"})
	first := stageHTTPCreationAttachment(t, attachmentService, ctx, "screenshot.png", "image/png", "image", "prompt")
	second := stageHTTPCreationAttachment(t, attachmentService, ctx, "logs.zip", "application/zip", "resource", "path")
	payload, err := json.Marshal(map[string]interface{}{
		"workspace_id": "ws-http-creation",
		"workflow_id":  "wf-http-creation",
		"title":        "Office attachment create",
		"attachments": []v1.MessageAttachment{
			descriptorForHTTP(second), descriptorForHTTP(first),
		},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(string(payload)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request = c.Request.WithContext(ctx)
	h.httpCreateTask(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotEmpty(t, response.ID)
	require.Len(t, eventBus.state, 2, "task.created must observe the committed creation set")
	require.Equal(t, second.ID, eventBus.state[0].ID)
	require.Equal(t, first.ID, eventBus.state[1].ID)

	rows, err := repo.ListTaskCreationAttachments(ctx, response.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, models.AttachmentStateClaimed, row.State)
		require.Equal(t, response.ID, row.TaskID)
		require.Empty(t, row.SessionID)
	}
}

func TestHTTPCreateTaskRejectsInvalidOfficeAttachmentWithoutTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo, attachmentService, _ := newCreationAttachmentHandler(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner-http"})
	valid := stageHTTPCreationAttachment(t, attachmentService, ctx, "valid.png", "image/png", "image", "prompt")
	foreign := &models.TaskMessageAttachment{
		ID: "missing-http-attachment", OwnerID: "owner-http", WorkspaceID: "ws-http-creation",
		Name: "foreign.png", MimeType: "image/png", Kind: "image", DeliveryMode: "prompt",
		SizeBytes: 3, StorageKey: "missing-http-attachment",
	}
	payload, err := json.Marshal(map[string]interface{}{
		"workspace_id": "ws-http-creation",
		"workflow_id":  "wf-http-creation",
		"title":        "Rejected",
		"attachments": []v1.MessageAttachment{
			descriptorForHTTP(valid), descriptorForHTTP(foreign),
		},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(string(payload)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request = c.Request.WithContext(ctx)
	h.httpCreateTask(c)

	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
	stored, err := repo.GetMessageAttachment(ctx, valid.ID)
	require.NoError(t, err)
	require.Equal(t, models.AttachmentStateStaged, stored.State)
	require.Empty(t, stored.TaskID)

	var taskCount int
	require.NoError(t, repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`, "ws-http-creation").Scan(&taskCount))
	require.Zero(t, taskCount)
}
