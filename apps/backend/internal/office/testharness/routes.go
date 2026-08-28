// Package testharness exposes test-only HTTP routes that seed task sessions
// and messages directly in the database. The routes are gated behind the
// KANDEV_E2E_MOCK env var and must NOT be enabled in production builds.
//
// They exist so the Playwright suite can drive the live-presence UI without
// launching a real executor. Each route validates inputs (real task ID
// exists, state is in the canonical set, terminal states require a
// completed_at timestamp) so tests cannot accidentally corrupt the DB.
package testharness

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/agents"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// EnvVar gates registration of the test routes. Must be the literal string
// "true" to enable — matches the convention used by KANDEV_MOCK_JIRA etc.
const EnvVar = "KANDEV_E2E_MOCK"

const gitLabRemoteURLEnv = "KANDEV_E2E_GITLAB_REMOTE_URL"

// errInvalidJSONPrefix is the shared 400 message for malformed request bodies.
const errInvalidJSONPrefix = "invalid JSON: "

// respKeyError is the JSON key for error responses. Older handlers in this file
// inline the literal; new code uses errJSON so the heavily-repeated key isn't
// duplicated again.
const respKeyError = "error"

// errJSON writes a `{"error": msg}` body with the given status code.
func errJSON(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{respKeyError: msg})
}

// Enabled reports whether KANDEV_E2E_MOCK is set to "true".
func Enabled() bool {
	return os.Getenv(EnvVar) == "true"
}

// allowedStates lists the TaskSession states the harness will create.
// CREATED / STARTING are intentionally excluded — those are transient states
// the orchestrator owns; tests should seed directly into RUNNING, IDLE, or
// terminal states.
var allowedStates = map[string]bool{
	string(models.TaskSessionStateRunning):         true,
	string(models.TaskSessionStateIdle):            true,
	string(models.TaskSessionStateWaitingForInput): true,
	string(models.TaskSessionStateCompleted):       true,
	string(models.TaskSessionStateFailed):          true,
	string(models.TaskSessionStateCancelled):       true,
}

// terminalStates is the subset of allowedStates that require a completed_at.
var terminalStates = map[string]bool{
	string(models.TaskSessionStateCompleted): true,
	string(models.TaskSessionStateFailed):    true,
	string(models.TaskSessionStateCancelled): true,
}

// RegisterRoutes mounts the test-only route group. Callers MUST guard the
// call with Enabled() — this function does not check the env var so tests
// can register routes against an isolated router.
func RegisterRoutes(
	router *gin.Engine,
	repo *sqliterepo.Repository,
	officeRepo *officesqlite.Repository,
	agentSettings settingsstore.Repository,
	agentSvc *agents.AgentService,
	eventBus bus.EventBus,
	log *logger.Logger,
) {
	g := router.Group("/api/v1/_test")
	g.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	g.POST("/tasks", seedTaskHandler(repo, log))
	g.POST("/task-sessions", seedTaskSessionHandler(repo, eventBus, log))
	g.POST("/messages", seedMessageHandler(repo, eventBus, log))
	g.POST("/workflows", seedWorkflowHandler(repo, log))
	g.PUT("/repositories/:id/git-remote", configureGitRemoteHandler(repo, log))
	g.DELETE("/repositories/:id/git-remote", configureGitRemoteHandler(repo, log))
	g.GET("/repositories/:id/git-push-record", gitPushRecordHandler(repo))
	if officeRepo != nil {
		g.POST("/comments", seedCommentHandler(officeRepo, eventBus, log))
		g.POST("/agent-failures", seedAgentFailureHandler(officeRepo, eventBus, log))
		g.POST("/runs", seedRunHandler(officeRepo, log))
		g.PATCH("/runs/:id", patchRunHandler(officeRepo, eventBus, log))
		g.POST("/run-events", seedRunEventHandler(officeRepo, eventBus, log))
		g.POST("/run-skills", seedRunSkillSnapshotHandler(officeRepo, log))
		g.POST("/cost-events", seedCostEventHandler(officeRepo, log))
		g.POST("/activity", seedActivityHandler(officeRepo, log))
	}
	if agentSvc != nil {
		g.POST("/runtime-token", mintRuntimeTokenHandler(agentSvc, log))
	}
	if agentSettings != nil {
		g.POST("/agent-profiles/:id/desired-skills", setDesiredSkillsHandler(agentSettings, log))
	}
}

type configureGitRemoteRequest struct {
	RemoteURL string `json:"remote_url"`
}

func configureGitRemoteHandler(repo *sqliterepo.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		repository, err := repo.GetRepository(c.Request.Context(), c.Param("id"))
		if err != nil || strings.TrimSpace(repository.LocalPath) == "" {
			errJSON(c, http.StatusNotFound, "repository with a local path not found")
			return
		}
		if c.Request.Method == http.MethodDelete {
			removeGitRemote(c.Request.Context(), repository.LocalPath)
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}

		trustedRemoteURL, status, err := trustedGitRemoteURL(c)
		if err != nil {
			errJSON(c, status, err.Error())
			return
		}
		args := []string{"-C", repository.LocalPath, "remote", "add", "origin", trustedRemoteURL}
		getCmd := subproc.NewGitCommand(c.Request.Context(), "-C", repository.LocalPath,
			"remote", "get-url", "origin")
		if getErr := subproc.RunGitClass(c.Request.Context(), subproc.GitLifecycle, getCmd); getErr == nil {
			args = []string{"-C", repository.LocalPath, "remote", "set-url", "origin", trustedRemoteURL}
		}
		setCmd := subproc.NewGitCommand(c.Request.Context(), args...)
		if output, runErr := subproc.RunGitCombinedOutputClass(c.Request.Context(), subproc.GitLifecycle, setCmd); runErr != nil {
			log.Error("test harness: configure git remote failed", zap.Error(runErr))
			errJSON(c, http.StatusInternalServerError, strings.TrimSpace(string(output)))
			return
		}
		if err := writeGitLabPushMarker(trustedRemoteURL); err != nil {
			log.Error("test harness: write GitLab push marker failed", zap.Error(err))
			errJSON(c, http.StatusInternalServerError, "write GitLab push marker")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func removeGitRemote(ctx context.Context, repositoryPath string) {
	cmd := subproc.NewGitCommand(ctx, "-C", repositoryPath, "remote", "remove", "origin")
	_ = subproc.RunGitClass(ctx, subproc.GitLifecycle, cmd)
	for _, envKey := range []string{"KANDEV_E2E_GITLAB_PUSH_FILE", "KANDEV_E2E_GITLAB_PUSH_RECORD_FILE"} {
		if path := os.Getenv(envKey); path != "" {
			_ = os.Remove(path)
		}
	}
}

func trustedGitRemoteURL(c *gin.Context) (string, int, error) {
	var req configureGitRemoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return "", http.StatusBadRequest, errors.New(errInvalidJSONPrefix + err.Error())
	}
	requestedRemote, err := localE2ERemoteURL(req.RemoteURL)
	if err != nil {
		return "", http.StatusBadRequest, errors.New("remote_url must use the local E2E server")
	}
	configuredRemote, err := localE2ERemoteURL(os.Getenv(gitLabRemoteURLEnv))
	if err != nil {
		return "", http.StatusInternalServerError, errors.New("GitLab E2E remote is not configured")
	}
	if requestedRemote.String() != configuredRemote.String() {
		return "", http.StatusBadRequest, errors.New("remote_url does not match the configured E2E remote")
	}
	return configuredRemote.String(), http.StatusOK, nil
}

func localE2ERemoteURL(raw string) (*url.URL, error) {
	remote, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (remote.Hostname() != "localhost" && remote.Hostname() != "127.0.0.1") {
		return nil, errors.New("remote is not local")
	}
	return remote, nil
}

func writeGitLabPushMarker(remoteURL string) error {
	marker := os.Getenv("KANDEV_E2E_GITLAB_PUSH_FILE")
	if marker == "" {
		return nil
	}
	return os.WriteFile(marker, []byte(remoteURL), 0o600)
}

func gitPushRecordHandler(repo *sqliterepo.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := repo.GetRepository(c.Request.Context(), c.Param("id")); err != nil {
			errJSON(c, http.StatusNotFound, "repository not found")
			return
		}
		recordPath := os.Getenv("KANDEV_E2E_GITLAB_PUSH_RECORD_FILE")
		if recordPath == "" {
			errJSON(c, http.StatusNotFound, "git push was not recorded")
			return
		}
		contents, err := os.ReadFile(recordPath)
		if err != nil {
			errJSON(c, http.StatusNotFound, "git push was not recorded")
			return
		}
		c.JSON(http.StatusOK, gin.H{"args": strings.TrimSpace(string(contents))})
	}
}

type seedTaskRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
	Title          string `json:"title"`
	ParentID       string `json:"parent_id"`
	State          string `json:"state"`
}

type seedTaskResponse struct {
	TaskID string `json:"task_id"`
}

// seedTaskHandler creates a task row directly through the repository, which
// bypasses the service-layer subtask-depth guard (validateSubtaskDepth runs in
// Service.CreateTask only). This lets e2e tests build arbitrary parent_id
// chains (depth >= 2) to exercise the sidebar's multi-level tree — production
// CreateTask rejects depth > 1 for kanban tasks.
func seedTaskHandler(repo *sqliterepo.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			errJSON(c, http.StatusBadRequest, errInvalidJSONPrefix+err.Error())
			return
		}
		if req.WorkspaceID == "" || req.Title == "" {
			errJSON(c, http.StatusBadRequest, "workspace_id and title are required")
			return
		}
		ctx := c.Request.Context()
		if req.ParentID != "" {
			if _, err := repo.GetTask(ctx, req.ParentID); err != nil {
				errJSON(c, http.StatusBadRequest, "parent_id not found: "+req.ParentID)
				return
			}
		}
		state := req.State
		if state == "" {
			state = string(v1.TaskStateCreated)
		}
		task := &models.Task{
			ID:             uuid.New().String(),
			WorkspaceID:    req.WorkspaceID,
			WorkflowID:     req.WorkflowID,
			WorkflowStepID: req.WorkflowStepID,
			Title:          req.Title,
			State:          v1.TaskState(state),
			ParentID:       req.ParentID,
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			log.Error("test harness: create task failed", zap.Error(err))
			errJSON(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.JSON(http.StatusOK, seedTaskResponse{TaskID: task.ID})
	}
}

type seedTaskSessionRequest struct {
	TaskID         string                 `json:"task_id"`
	SessionID      string                 `json:"session_id,omitempty"`
	RepositoryID   string                 `json:"repository_id,omitempty"`
	State          string                 `json:"state"`
	AgentProfileID string                 `json:"agent_profile_id,omitempty"`
	StartedAt      *string                `json:"started_at,omitempty"`
	CompletedAt    *string                `json:"completed_at,omitempty"`
	CommandCount   int                    `json:"command_count,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type seedTaskSessionResponse struct {
	SessionID string `json:"session_id"`
}

func seedTaskSessionHandler(repo *sqliterepo.Repository, eventBus bus.EventBus, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedTaskSessionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if err := validateSeedTaskSessionRequest(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx := c.Request.Context()
		if _, err := repo.GetTask(ctx, req.TaskID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "task not found: " + req.TaskID})
			return
		}

		session, err := buildSeededSession(&req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// If a session already exists for this (task, agent) pair, update its
		// state in place. Office sessions are unique per (task, agent) per the
		// schema's partial unique index, and tests routinely flip the same
		// pair RUNNING → IDLE (and back) to exercise reactive UI.
		existing := getExistingSeedSession(ctx, repo, &req)
		if existing != nil {
			session, err = updateSeededSession(ctx, repo, existing, session, req.TaskID)
			if err != nil {
				log.Error("test harness: update session failed", zap.Error(err))
				c.JSON(statusForSeedSessionUpdateError(err), gin.H{"error": err.Error()})
				return
			}
		} else if err := repo.CreateTaskSession(ctx, session); err != nil {
			log.Error("test harness: create session failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if req.CommandCount > 0 {
			if err := seedToolCalls(ctx, repo, session.ID, session.TaskID, req.CommandCount); err != nil {
				log.Error("test harness: seed tool calls failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		publishSessionStateChanged(ctx, eventBus, session, log)
		c.JSON(http.StatusOK, seedTaskSessionResponse{SessionID: session.ID})
	}
}

func updateSeededSession(
	ctx context.Context,
	repo *sqliterepo.Repository,
	existing *models.TaskSession,
	session *models.TaskSession,
	taskID string,
) (*models.TaskSession, error) {
	if existing.TaskID != taskID {
		return nil, seedSessionUpdateError{message: "session does not belong to task: " + existing.ID}
	}
	existing.State = session.State
	existing.CompletedAt = session.CompletedAt
	existing.UpdatedAt = time.Now().UTC()
	if existing.Metadata == nil {
		existing.Metadata = map[string]interface{}{}
	}
	for key, value := range session.Metadata {
		existing.Metadata[key] = value
	}
	if err := repo.UpdateTaskSessionWithMetadata(ctx, existing, existing.Metadata); err != nil {
		return nil, err
	}
	return existing, nil
}

type seedSessionUpdateError struct {
	message string
}

func (err seedSessionUpdateError) Error() string {
	return err.message
}

func statusForSeedSessionUpdateError(err error) int {
	var updateErr seedSessionUpdateError
	if errors.As(err, &updateErr) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func getExistingSeedSession(ctx context.Context, repo *sqliterepo.Repository, req *seedTaskSessionRequest) *models.TaskSession {
	if req.SessionID != "" {
		existing, _ := repo.GetTaskSession(ctx, req.SessionID)
		return existing
	}
	existing, _ := repo.GetTaskSessionByTaskAndAgent(ctx, req.TaskID, req.AgentProfileID)
	return existing
}

type seedWorkflowRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	// Style is persisted as-is (kanban / office / custom). Production has no
	// HTTP path that creates an office-style workflow — the normal create
	// endpoint always normalises to kanban — so this seed route lets the
	// Playwright suite stand up an office workflow to exercise the
	// "exclude office from settings export" behaviour (issue #1109).
	Style string `json:"style"`
}

type seedWorkflowResponse struct {
	WorkflowID string `json:"workflow_id"`
}

func seedWorkflowHandler(repo *sqliterepo.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedWorkflowRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.WorkspaceID == "" || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id and name are required"})
			return
		}
		workflow := &models.Workflow{
			ID:          uuid.New().String(),
			WorkspaceID: req.WorkspaceID,
			Name:        req.Name,
			Style:       req.Style,
		}
		if err := repo.CreateWorkflow(c.Request.Context(), workflow); err != nil {
			log.Error("test harness: create workflow failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, seedWorkflowResponse{WorkflowID: workflow.ID})
	}
}

func validateSeedTaskSessionRequest(req *seedTaskSessionRequest) error {
	if req.TaskID == "" {
		return errBadField("task_id is required")
	}
	if !allowedStates[req.State] {
		return errBadField("state must be one of RUNNING, IDLE, WAITING_FOR_INPUT, COMPLETED, FAILED, CANCELLED")
	}
	if terminalStates[req.State] && (req.CompletedAt == nil || *req.CompletedAt == "") {
		return errBadField("completed_at is required for terminal states")
	}
	return nil
}

func buildSeededSession(req *seedTaskSessionRequest) (*models.TaskSession, error) {
	startedAt := time.Now().UTC()
	if req.StartedAt != nil && *req.StartedAt != "" {
		t, err := time.Parse(time.RFC3339, *req.StartedAt)
		if err != nil {
			return nil, errBadField("started_at must be RFC3339")
		}
		startedAt = t.UTC()
	}
	var completedAt *time.Time
	if req.CompletedAt != nil && *req.CompletedAt != "" {
		t, err := time.Parse(time.RFC3339, *req.CompletedAt)
		if err != nil {
			return nil, errBadField("completed_at must be RFC3339")
		}
		ts := t.UTC()
		completedAt = &ts
	}
	metadata := map[string]interface{}{"seeded_by_e2e_mock": true}
	for key, value := range req.Metadata {
		metadata[key] = value
	}
	if req.AgentProfileID != "" {
		metadata["agent_profile_id"] = req.AgentProfileID
	}
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	// Leave AgentProfileID empty when the caller didn't supply one — that
	// makes the seeded row a non-office (kanban / quick-chat) session, so
	// it renders inline in the task chat timeline rather than collapsing
	// into a per-agent sibling tab. Tests that want office (per-agent)
	// behaviour pass the agent profile id explicitly.
	return &models.TaskSession{
		ID:             sessionID,
		TaskID:         req.TaskID,
		RepositoryID:   req.RepositoryID,
		AgentProfileID: req.AgentProfileID,
		State:          models.TaskSessionState(req.State),
		Metadata:       metadata,
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		UpdatedAt:      time.Now().UTC(),
	}, nil
}

// seedToolCalls inserts count synthetic tool_call messages, each on its own
// turn so foreign key checks succeed. The harness creates a single shared
// turn rather than one per message — task_session_messages.turn_id requires
// an existing task_session_turns row.
func seedToolCalls(ctx context.Context, repo *sqliterepo.Repository, sessionID, taskID string, count int) error {
	turnID, err := ensureSeededTurn(ctx, repo, sessionID, taskID)
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		msg := &models.Message{
			ID:            uuid.New().String(),
			TaskSessionID: sessionID,
			TaskID:        taskID,
			TurnID:        turnID,
			AuthorType:    models.MessageAuthorAgent,
			Type:          models.MessageTypeToolCall,
			Content:       "synthetic tool call",
			Metadata:      map[string]interface{}{"seeded_by_e2e_mock": true, "tool_call_id": uuid.New().String()},
			CreatedAt:     time.Now().UTC(),
		}
		if err := repo.CreateMessage(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// ensureSeededTurn returns the ID of an active turn for the session,
// creating one if none exists. Reusing the active turn keeps all seeded
// messages on the same turn so the chat embed renders them together.
func ensureSeededTurn(ctx context.Context, repo *sqliterepo.Repository, sessionID, taskID string) (string, error) {
	if existing, err := repo.GetActiveTurnBySessionID(ctx, sessionID); err == nil && existing != nil {
		return existing.ID, nil
	}
	turn := &models.Turn{
		ID:            uuid.New().String(),
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		return "", err
	}
	return turn.ID, nil
}

// createSeededTurn always creates a fresh turn rather than reusing the
// session's active one, so a spec can hold a real open turn on the session
// while seeding a second, independently-timed turn alongside it (see
// seedMessageRequest.NewTurn). startedAt/completedAt default to "now" and
// "open" respectively when nil.
func createSeededTurn(ctx context.Context, repo *sqliterepo.Repository, sessionID, taskID string, startedAt, completedAt *time.Time) (string, error) {
	now := time.Now().UTC()
	turn := &models.Turn{
		ID:            uuid.New().String(),
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if startedAt != nil {
		turn.StartedAt = startedAt.UTC()
	}
	if completedAt != nil {
		ca := completedAt.UTC()
		turn.CompletedAt = &ca
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		return "", err
	}
	return turn.ID, nil
}

func publishSessionStateChanged(ctx context.Context, eventBus bus.EventBus, session *models.TaskSession, log *logger.Logger) {
	if eventBus == nil {
		return
	}
	data := map[string]interface{}{
		"task_id":          session.TaskID,
		"session_id":       session.ID,
		"old_state":        "",
		"new_state":        string(session.State),
		"updated_at":       session.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"agent_profile_id": session.AgentProfileID,
	}
	if session.Metadata != nil {
		data["session_metadata"] = session.Metadata
	}
	if err := eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "e2e-mock", data)); err != nil {
		log.Warn("test harness: publish state changed failed", zap.Error(err))
	}
}

// messageCreatedAtKey is the JSON key for seeded message timestamps.
const messageCreatedAtKey = "created_at"

type seedMessageRequest struct {
	SessionID string                 `json:"session_id"`
	Type      string                 `json:"type"`
	Content   string                 `json:"content,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	// AuthorType defaults to "agent"; "user" seeds a prompt row whose
	// prompt_index is computed by the repository's atomic create boundary.
	AuthorType string `json:"author_type,omitempty"`
	// CreatedAt is an optional RFC3339 timestamp used for deterministic
	// ordering; when omitted the repository assigns a live timestamp.
	CreatedAt *time.Time `json:"created_at,omitempty"`
	// TurnMetadata is persisted on the ensured turn (the same turn the seeded
	// message is attached to), so e2e specs can exercise the message metadata
	// dialog's turn_metadata field end to end.
	TurnMetadata map[string]interface{} `json:"turn_metadata,omitempty"`
	// NewTurn, when true, creates a brand-new turn for this message instead
	// of reusing the session's active turn. Lets a spec construct two turns
	// on the same session (e.g. a still-open turn plus a separate,
	// later-started lifecycle-shaped turn) to exercise current-turn-authority
	// ordering (D1) end to end, which ensureSeededTurn's reuse-active-turn
	// behavior cannot produce.
	NewTurn bool `json:"new_turn,omitempty"`
	// TurnStartedAt/TurnCompletedAt set the new turn's timestamps when
	// NewTurn is true, so a spec can control D1's ordering directly. Ignored
	// when NewTurn is false.
	TurnStartedAt   *time.Time `json:"turn_started_at,omitempty"`
	TurnCompletedAt *time.Time `json:"turn_completed_at,omitempty"`
}

// seedMessageHandler inserts a synthetic message, ensuring an active turn
// exists for the session and applying optional turn_metadata to it so e2e
// specs can exercise the message metadata dialog end to end. author_type
// defaults to agent; user seeds are read back through GetMessageWithPromptIndex
// so the WS event carries the same prompt_index the HTTP list path reports.
func seedMessageHandler(repo *sqliterepo.Repository, eventBus bus.EventBus, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.SessionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
			return
		}
		if req.Type == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "type is required"})
			return
		}
		ctx := c.Request.Context()
		session, err := repo.GetTaskSession(ctx, req.SessionID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session not found: " + req.SessionID})
			return
		}

		var turnID string
		if req.NewTurn {
			turnID, err = createSeededTurn(ctx, repo, session.ID, session.TaskID, req.TurnStartedAt, req.TurnCompletedAt)
		} else {
			turnID, err = ensureSeededTurn(ctx, repo, session.ID, session.TaskID)
		}
		if err != nil {
			log.Error("test harness: ensure turn failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if req.TurnMetadata != nil {
			turn, err := repo.GetTurn(ctx, turnID)
			if err != nil {
				log.Error("test harness: get turn failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			turn.Metadata = req.TurnMetadata
			if err := repo.UpdateTurn(ctx, turn); err != nil {
				log.Error("test harness: update turn metadata failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		meta := req.Metadata
		if meta == nil {
			meta = make(map[string]interface{})
		}
		meta["seeded_by_e2e_mock"] = true

		authorType := models.MessageAuthorAgent
		if req.AuthorType == string(models.MessageAuthorUser) {
			authorType = models.MessageAuthorUser
		}
		msg := &models.Message{
			ID:            uuid.New().String(),
			TaskSessionID: session.ID,
			TaskID:        session.TaskID,
			TurnID:        turnID,
			AuthorType:    authorType,
			Type:          models.MessageType(req.Type),
			Content:       req.Content,
			Metadata:      meta,
		}
		// CreatedAt stays zero unless explicitly supplied: the repository
		// boundary treats a zero timestamp as a LIVE create (advancing a
		// colliding key), while explicit timestamps must be strictly after
		// the session's newest user message.
		if req.CreatedAt != nil {
			msg.CreatedAt = req.CreatedAt.UTC()
		}
		if err := repo.CreateMessage(ctx, msg); err != nil {
			log.Error("test harness: create message failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// User seeds carry the repository-computed ordinal (and any timestamp
		// advance the boundary applied), so the WS event matches the HTTP
		// list path exactly.
		if msg.AuthorType == models.MessageAuthorUser {
			if indexed, err := repo.GetMessageWithPromptIndex(ctx, msg.ID); err == nil {
				msg = indexed
			} else {
				log.Warn("test harness: re-read indexed seed failed", zap.Error(err))
			}
		}

		publishMessageAdded(ctx, eventBus, msg, log)
		c.JSON(http.StatusOK, gin.H{"message_id": msg.ID})
	}
}

// publishMessageAdded emits a message-added bus event for a seeded message so connected clients receive it without a refresh.
func publishMessageAdded(ctx context.Context, eventBus bus.EventBus, msg *models.Message, log *logger.Logger) {
	if eventBus == nil {
		return
	}
	data := map[string]interface{}{
		"message_id":     msg.ID,
		"session_id":     msg.TaskSessionID,
		"task_id":        msg.TaskID,
		"turn_id":        msg.TurnID,
		"author_type":    string(msg.AuthorType),
		"content":        msg.Content,
		"type":           string(msg.Type),
		"requests_input": msg.RequestsInput,
		// RFC3339Nano preserves fractional precision so seeded user prompts
		// order deterministically on the client.
		messageCreatedAtKey: msg.CreatedAt.Format(time.RFC3339Nano),
	}
	// User rows carry their stable prompt ordinal in the live WS event.
	if msg.PromptIndex > 0 {
		data["prompt_index"] = msg.PromptIndex
	}
	if msg.Metadata != nil {
		data["metadata"] = msg.Metadata
	}
	if err := eventBus.Publish(ctx, events.MessageAdded, bus.NewEvent(events.MessageAdded, "e2e-mock", data)); err != nil {
		log.Warn("test harness: publish message added failed", zap.Error(err))
	}
}

// errBadField wraps a 400-style validation error.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

type setDesiredSkillsRequest struct {
	Slugs []string `json:"slugs"`
}

// setDesiredSkillsHandler updates the desired_skills column on an
// agent_profiles row. It exists so the e2e suite can drive skill
// injection without going through the office onboarding flow (which
// can only modify office-scoped agents).
func setDesiredSkillsHandler(repo settingsstore.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
			return
		}
		var req setDesiredSkillsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		ctx := c.Request.Context()
		profile, err := repo.GetAgentProfile(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		slugs := req.Slugs
		if slugs == nil {
			slugs = []string{}
		}
		encoded, err := json.Marshal(slugs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		profile.DesiredSkills = string(encoded)
		if err := repo.UpdateAgentProfile(ctx, profile); err != nil {
			log.Error("test harness: update desired_skills failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "desired_skills": profile.DesiredSkills})
	}
}

func errBadField(msg string) error { return &validationError{msg: msg} }
