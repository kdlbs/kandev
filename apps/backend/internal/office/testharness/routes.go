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
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/agents"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// EnvVar gates registration of the test routes. Must be the literal string
// "true" to enable — matches the convention used by KANDEV_MOCK_JIRA etc.
const EnvVar = "KANDEV_E2E_MOCK"

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
	g.POST("/task-sessions", seedTaskSessionHandler(repo, eventBus, log))
	g.POST("/messages", seedMessageHandler(repo, eventBus, log))
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

type seedTaskSessionRequest struct {
	TaskID         string  `json:"task_id"`
	State          string  `json:"state"`
	AgentProfileID string  `json:"agent_profile_id,omitempty"`
	StartedAt      *string `json:"started_at,omitempty"`
	CompletedAt    *string `json:"completed_at,omitempty"`
	CommandCount   int     `json:"command_count,omitempty"`
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
		if existing, _ := repo.GetTaskSessionByTaskAndAgent(ctx, req.TaskID, req.AgentProfileID); existing != nil {
			existing.State = session.State
			existing.CompletedAt = session.CompletedAt
			existing.UpdatedAt = time.Now().UTC()
			if err := repo.UpdateTaskSession(ctx, existing); err != nil {
				log.Error("test harness: update session failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			session = existing
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
	if req.AgentProfileID != "" {
		metadata["agent_profile_id"] = req.AgentProfileID
	}
	// Leave AgentProfileID empty when the caller didn't supply one — that
	// makes the seeded row a non-office (kanban / quick-chat) session, so
	// it renders inline in the task chat timeline rather than collapsing
	// into a per-agent sibling tab. Tests that want office (per-agent)
	// behaviour pass the agent profile id explicitly.
	return &models.TaskSession{
		ID:             uuid.New().String(),
		TaskID:         req.TaskID,
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

func publishSessionStateChanged(ctx context.Context, eventBus bus.EventBus, session *models.TaskSession, log *logger.Logger) {
	if eventBus == nil {
		return
	}
	data := map[string]interface{}{
		"task_id":          session.TaskID,
		"session_id":       session.ID,
		"old_state":        "",
		"new_state":        string(session.State),
		"agent_profile_id": session.AgentProfileID,
	}
	if session.Metadata != nil {
		data["session_metadata"] = session.Metadata
	}
	if err := eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "e2e-mock", data)); err != nil {
		log.Warn("test harness: publish state changed failed", zap.Error(err))
	}
}

type seedMessageRequest struct {
	SessionID string                 `json:"session_id"`
	Type      string                 `json:"type"`
	Content   string                 `json:"content,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

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

		turnID, err := ensureSeededTurn(ctx, repo, session.ID, session.TaskID)
		if err != nil {
			log.Error("test harness: ensure turn failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		meta := req.Metadata
		if meta == nil {
			meta = make(map[string]interface{})
		}
		meta["seeded_by_e2e_mock"] = true

		msg := &models.Message{
			ID:            uuid.New().String(),
			TaskSessionID: session.ID,
			TaskID:        session.TaskID,
			TurnID:        turnID,
			AuthorType:    models.MessageAuthorAgent,
			Type:          models.MessageType(req.Type),
			Content:       req.Content,
			Metadata:      meta,
			CreatedAt:     time.Now().UTC(),
		}
		if err := repo.CreateMessage(ctx, msg); err != nil {
			log.Error("test harness: create message failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		publishMessageAdded(ctx, eventBus, msg, log)
		c.JSON(http.StatusOK, gin.H{"message_id": msg.ID})
	}
}

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
		"created_at":     msg.CreatedAt.Format(time.RFC3339),
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
