package testharness

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/agents"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

type seedCommentRequest struct {
	TaskID     string  `json:"task_id"`
	AuthorType string  `json:"author_type"`
	AuthorID   string  `json:"author_id"`
	Body       string  `json:"body"`
	Source     string  `json:"source,omitempty"`
	CreatedAt  *string `json:"created_at,omitempty"`
}

type mintRuntimeTokenRequest struct {
	AgentProfileID string `json:"agent_profile_id"`
	TaskID         string `json:"task_id"`
	WorkspaceID    string `json:"workspace_id"`
	RunID          string `json:"run_id"`
	SessionID      string `json:"session_id"`
	Capabilities   string `json:"capabilities"`
}

func mintRuntimeTokenHandler(agentSvc *agents.AgentService, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req mintRuntimeTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.AgentProfileID == "" || req.WorkspaceID == "" || req.RunID == "" {
			c.JSON(http.StatusBadRequest,
				gin.H{"error": "agent_profile_id, workspace_id, run_id are required"})
			return
		}
		token, err := agentSvc.MintRuntimeJWT(
			req.AgentProfileID,
			req.TaskID,
			req.WorkspaceID,
			req.RunID,
			req.SessionID,
			req.Capabilities,
		)
		if err != nil {
			log.Error("test harness: mint runtime token failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": token})
	}
}

// seedCommentHandler creates a TaskComment row directly so E2E tests can
// drive comment-rendering scenarios without launching an agent.
// Specifically: agent session-bridged comments need author_type="agent",
// source="session", and an author_id matching a real agent instance —
// the production createComment route hardcodes user/user.
func seedCommentHandler(
	repo *officesqlite.Repository,
	eventBus bus.EventBus,
	log *logger.Logger,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedCommentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.TaskID == "" || req.AuthorType == "" || req.AuthorID == "" || req.Body == "" {
			c.JSON(http.StatusBadRequest,
				gin.H{"error": "task_id, author_type, author_id, body are required"})
			return
		}
		comment := &officemodels.TaskComment{
			ID:         uuid.New().String(),
			TaskID:     req.TaskID,
			AuthorType: req.AuthorType,
			AuthorID:   req.AuthorID,
			Body:       req.Body,
			Source:     req.Source,
		}
		if req.CreatedAt != nil && *req.CreatedAt != "" {
			t, err := time.Parse(time.RFC3339, *req.CreatedAt)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "created_at must be RFC3339"})
				return
			}
			comment.CreatedAt = t.UTC()
		}
		ctx := c.Request.Context()
		if err := repo.CreateTaskComment(ctx, comment); err != nil {
			log.Error("test harness: create comment failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		publishCommentCreated(ctx, eventBus, comment, log)
		c.JSON(http.StatusOK, gin.H{"comment_id": comment.ID})
	}
}

func publishCommentCreated(
	ctx context.Context,
	eventBus bus.EventBus,
	comment *officemodels.TaskComment,
	log *logger.Logger,
) {
	if eventBus == nil {
		return
	}
	data := map[string]interface{}{
		"comment_id":  comment.ID,
		"task_id":     comment.TaskID,
		"author_type": comment.AuthorType,
		"author_id":   comment.AuthorID,
		"body":        comment.Body,
		"source":      comment.Source,
		"created_at":  comment.CreatedAt.Format(time.RFC3339),
	}
	if err := eventBus.Publish(
		ctx,
		events.OfficeCommentCreated,
		bus.NewEvent(events.OfficeCommentCreated, "e2e-mock", data),
	); err != nil {
		log.Warn("test harness: publish comment created failed", zap.Error(err))
	}
}

type seedAgentFailureRequest struct {
	TaskID         string `json:"task_id"`
	AgentProfileID string `json:"agent_profile_id"`
	ErrorMessage   string `json:"error_message"`
}

// seedAgentFailureHandler enqueues a run, marks it failed via the
// production HandleAgentFailure path, and returns the run id.
// Lets E2E exercise the FailureService (counter, auto-pause, inbox)
// without launching an agent.
func seedAgentFailureHandler(
	repo *officesqlite.Repository,
	eventBus bus.EventBus,
	log *logger.Logger,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedAgentFailureRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.TaskID == "" || req.AgentProfileID == "" {
			c.JSON(http.StatusBadRequest,
				gin.H{"error": "task_id and agent_profile_id are required"})
			return
		}
		if req.ErrorMessage == "" {
			req.ErrorMessage = "seeded failure"
		}
		ctx := c.Request.Context()
		// Insert the run directly so we don't need to plumb the
		// office service into the testharness; the row is enough to
		// drive HandleAgentFailure → MarkRunFailed +
		// IncrementAgentConsecutiveFailures.
		run := &officemodels.Run{
			ID:             uuid.New().String(),
			AgentProfileID: req.AgentProfileID,
			Reason:         "task_assigned",
			Payload:        `{"task_id":"` + req.TaskID + `"}`,
			Status:         "claimed",
			CoalescedCount: 1,
			RequestedAt:    time.Now().UTC().Add(-1 * time.Minute),
		}
		if err := repo.CreateRun(ctx, run); err != nil {
			log.Error("test harness: create run failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := repo.MarkRunFailed(ctx, run.ID, req.ErrorMessage); err != nil {
			log.Error("test harness: mark run failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Update the agent counter and possibly auto-pause.
		count, err := repo.IncrementAgentConsecutiveFailures(ctx, req.AgentProfileID)
		if err != nil {
			log.Warn("test harness: increment counter failed", zap.Error(err))
		}
		threshold, err := repo.GetEffectiveFailureThreshold(ctx, req.AgentProfileID)
		if err != nil || threshold <= 0 {
			threshold = officesqlite.DefaultAgentFailureThreshold
		}
		if count >= threshold {
			pauseReason := "Auto-paused: " +
				strconv.Itoa(count) + " consecutive failures. Last error: " + req.ErrorMessage
			if err := repo.UpdateAgentStatusFields(ctx, req.AgentProfileID, "paused", pauseReason); err != nil {
				log.Warn("test harness: set pause reason failed", zap.Error(err))
			}
		}
		_ = eventBus
		c.JSON(http.StatusOK, gin.H{
			"run_id":               run.ID,
			"consecutive_failures": count,
			"threshold":            threshold,
		})
	}
}

type seedRunRequest struct {
	AgentProfileID string `json:"agent_profile_id"`
	Reason         string `json:"reason,omitempty"`
	Status         string `json:"status,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	CommentID      string `json:"comment_id,omitempty"`
	// RoutineID populates `payload.routine_id` so seeded routine_dispatch
	// rows round-trip into the office run summary DTO and surface the
	// Linked-column deeplink in the runs UI.
	RoutineID      string  `json:"routine_id,omitempty"`
	SessionID      string  `json:"session_id,omitempty"`
	Capabilities   string  `json:"capabilities,omitempty"`
	InputSnapshot  string  `json:"input_snapshot,omitempty"`
	ErrorMessage   string  `json:"error_message,omitempty"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
	RequestedAt    *string `json:"requested_at,omitempty"`
	ClaimedAt      *string `json:"claimed_at,omitempty"`
	FinishedAt     *string `json:"finished_at,omitempty"`
}

// seedRunHandler creates an office_runs row directly so the
// /office/agents/:id/runs paginated list and the /runs/:runId detail
// page can be exercised without launching an agent. Defaults Reason
// to task_assigned and Status to finished so a single field
// (agent_profile_id) is enough for happy-path tests.
func seedRunHandler(repo *officesqlite.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := decodeSeedRunRequest(c)
		if !ok {
			return
		}
		runID, ok := createSeededRun(c, repo, log, req)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{"run_id": runID})
	}
}

// decodeSeedRunRequest binds the JSON body, validates required fields, and
// applies reason/status defaults. On invalid input it writes the 400 response
// and returns ok=false; callers must abort.
func decodeSeedRunRequest(c *gin.Context) (seedRunRequest, bool) {
	var req seedRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return seedRunRequest{}, false
	}
	if req.AgentProfileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_profile_id is required"})
		return seedRunRequest{}, false
	}
	if req.Reason == "" {
		req.Reason = "task_assigned"
	}
	if req.Status == "" {
		req.Status = "finished"
	}
	return req, true
}

// createSeededRun runs the full seeded-run pipeline: insert, optional
// runtime-snapshot update, optional requested_at backfill, and final
// status flip. Returns the new run ID and ok=true on success; on failure
// it writes the error response and returns ok=false.
func createSeededRun(
	c *gin.Context, repo *officesqlite.Repository, log *logger.Logger, req seedRunRequest,
) (string, bool) {
	ctx := c.Request.Context()
	var idemPtr *string
	if req.IdempotencyKey != "" {
		v := req.IdempotencyKey
		idemPtr = &v
	}
	run := &officemodels.Run{
		ID:             uuid.New().String(),
		AgentProfileID: req.AgentProfileID,
		Reason:         req.Reason,
		Payload:        buildSeededRunPayload(req.TaskID, req.SessionID, req.CommentID, req.RoutineID),
		Status:         "queued",
		CoalescedCount: 1,
		IdempotencyKey: idemPtr,
		ErrorMessage:   req.ErrorMessage,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		log.Error("test harness: create run failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return "", false
	}
	if req.Capabilities != "" || req.InputSnapshot != "" || req.SessionID != "" {
		caps := req.Capabilities
		if caps == "" {
			caps = "{}"
		}
		input := req.InputSnapshot
		if input == "" {
			input = "{}"
		}
		if err := repo.UpdateRunRuntimeSnapshot(ctx, run.ID, caps, input, req.SessionID); err != nil {
			log.Warn("test harness: update runtime snapshot", zap.Error(err))
		}
	}
	// Force the requested_at if explicitly provided so tests can build
	// deterministic ordered pages — CreateRun stamps time.Now() otherwise.
	if req.RequestedAt != nil && *req.RequestedAt != "" {
		t, err := time.Parse(time.RFC3339, *req.RequestedAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "requested_at must be RFC3339"})
			return "", false
		}
		if err := updateSeededRunRequestedAt(ctx, repo, run.ID, t.UTC()); err != nil {
			log.Warn("test harness: backfill requested_at", zap.Error(err))
		}
	}
	// Finalise status by hand so we can land non-queued rows
	// (claimed/finished/failed/cancelled) directly.
	if req.Status != "queued" {
		if err := updateSeededRunStatus(ctx, repo, run.ID, req.Status, req.ClaimedAt, req.FinishedAt); err != nil {
			log.Warn("test harness: finalise status", zap.Error(err))
		}
	}
	return run.ID, true
}

// patchRunRequest mirrors PATCH /api/v1/_test/runs/:id. Both fields
// are optional; status is required for the route to do anything.
type patchRunRequest struct {
	Status       string `json:"status,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// patchRunHandler updates an existing seeded run's status (and
// optionally error_message) and publishes the matching
// OfficeRunProcessed event so the WS pipeline mirrors production.
// Used by E2E specs to drive a Queued comment badge through Working
// and Failed without going through the full agent lifecycle.
func patchRunHandler(
	repo *officesqlite.Repository, eventBus bus.EventBus, log *logger.Logger,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		runID := c.Param("id")
		if runID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "run id required"})
			return
		}
		var req patchRunRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.Status == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
			return
		}
		ctx := c.Request.Context()
		if err := updateSeededRunStatus(ctx, repo, runID, req.Status, nil, nil); err != nil {
			log.Error("test harness: patch run status failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if req.ErrorMessage != "" {
			if err := repo.SetRunErrorMessageForTest(ctx, runID, req.ErrorMessage); err != nil {
				log.Warn("test harness: patch run error_message failed", zap.Error(err))
			}
		}
		publishHarnessRunProcessed(ctx, eventBus, repo, runID, req.Status, log)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// publishHarnessRunProcessed re-fetches the row so the published
// payload carries the same {agent, task, comment, reason} shape as
// the production Service.publishRunProcessed path.
func publishHarnessRunProcessed(
	ctx context.Context,
	eventBus bus.EventBus,
	repo *officesqlite.Repository,
	runID, status string,
	log *logger.Logger,
) {
	if eventBus == nil {
		return
	}
	run, err := repo.GetRunByID(ctx, runID)
	if err != nil {
		log.Debug("test harness: get run for publish failed", zap.Error(err))
	}
	data := map[string]interface{}{
		"run_id": runID,
		"status": status,
	}
	if run != nil {
		parsed := harnessParseRunPayload(run.Payload)
		data["agent_profile_id"] = run.AgentProfileID
		data["reason"] = run.Reason
		data["task_id"] = parsed["task_id"]
		data["comment_id"] = parsed["comment_id"]
		if run.ErrorMessage != "" {
			data["error_message"] = run.ErrorMessage
		}
	}
	event := bus.NewEvent(events.OfficeRunProcessed, "test-harness", data)
	if pErr := eventBus.Publish(ctx, events.OfficeRunProcessed, event); pErr != nil {
		log.Warn("test harness: publish run processed failed", zap.Error(pErr))
	}
}

// harnessParseRunPayload is a tiny duplicate of service.ParseRunPayload
// kept here to avoid a cross-package import (testharness imports the
// office repo only). It parses the small task_id/comment_id payload
// the harness writes via buildSeededRunPayload.
func harnessParseRunPayload(payloadJSON string) map[string]string {
	out := map[string]string{}
	if payloadJSON == "" || payloadJSON == "{}" {
		return out
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(payloadJSON), &raw); err != nil {
		return out
	}
	for _, k := range []string{"task_id", "session_id", "comment_id"} {
		if v, ok := raw[k].(string); ok {
			out[k] = v
		}
	}
	return out
}

// buildSeededRunPayload assembles the JSON payload for a seeded run.
// All fields are optional; an empty payload becomes "{}" so the
// run-detail json_extract calls stay safe.
func buildSeededRunPayload(taskID, sessionID, commentID, routineID string) string {
	if taskID == "" && sessionID == "" && commentID == "" && routineID == "" {
		return "{}"
	}
	parts := map[string]string{}
	if taskID != "" {
		parts["task_id"] = taskID
	}
	if sessionID != "" {
		parts["session_id"] = sessionID
	}
	if commentID != "" {
		parts["comment_id"] = commentID
	}
	if routineID != "" {
		parts["routine_id"] = routineID
	}
	// Keys are stable so the test harness output is deterministic.
	out := "{"
	first := true
	for _, k := range []string{"task_id", "session_id", "comment_id", "routine_id"} {
		v, ok := parts[k]
		if !ok {
			continue
		}
		if !first {
			out += ","
		}
		out += `"` + k + `":"` + v + `"`
		first = false
	}
	out += "}"
	return out
}

func updateSeededRunRequestedAt(
	ctx context.Context, repo *officesqlite.Repository, runID string, ts time.Time,
) error {
	return repo.SetRunRequestedAtForTest(ctx, runID, ts)
}

func updateSeededRunStatus(
	ctx context.Context, repo *officesqlite.Repository,
	runID, status string, claimedAt, finishedAt *string,
) error {
	var claimed, finished *time.Time
	if claimedAt != nil && *claimedAt != "" {
		t, err := time.Parse(time.RFC3339, *claimedAt)
		if err != nil {
			return err
		}
		v := t.UTC()
		claimed = &v
	}
	if finishedAt != nil && *finishedAt != "" {
		t, err := time.Parse(time.RFC3339, *finishedAt)
		if err != nil {
			return err
		}
		v := t.UTC()
		finished = &v
	}
	return repo.SetRunStatusForTest(ctx, runID, status, claimed, finished)
}

type seedRunEventRequest struct {
	RunID     string `json:"run_id"`
	EventType string `json:"event_type"`
	Level     string `json:"level,omitempty"`
	Payload   string `json:"payload,omitempty"`
}

// seedRunEventHandler appends a synthetic office_run_events row and
// publishes the matching event-bus notification so the WS gateway's
// run-event broadcaster fans the row out to subscribed clients —
// mirrors the production AppendRunEvent service path. Used by the
// run-detail E2E spec to verify the events log renders the seeded
// structured events without a page reload.
func seedRunEventHandler(
	repo *officesqlite.Repository, eventBus bus.EventBus, log *logger.Logger,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedRunEventRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.RunID == "" || req.EventType == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "run_id and event_type are required"})
			return
		}
		ctx := c.Request.Context()
		evt, err := repo.AppendRunEvent(ctx, req.RunID, req.EventType, req.Level, req.Payload)
		if err != nil {
			log.Error("test harness: append run event failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if eventBus != nil && evt != nil {
			subject := events.BuildOfficeRunEventSubject(req.RunID)
			busEvent := bus.NewEvent(subject, "test-harness", map[string]interface{}{
				"run_id": evt.RunID,
				"event": map[string]interface{}{
					"seq":        evt.Seq,
					"event_type": evt.EventType,
					"level":      evt.Level,
					"payload":    evt.Payload,
					"created_at": evt.CreatedAt.UTC().Format(time.RFC3339Nano),
				},
			})
			if pErr := eventBus.Publish(ctx, subject, busEvent); pErr != nil {
				log.Warn("test harness: publish run event failed", zap.Error(pErr))
			}
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type seedRunSkillSnapshotRequest struct {
	RunID            string `json:"run_id"`
	SkillID          string `json:"skill_id"`
	Version          string `json:"version,omitempty"`
	ContentHash      string `json:"content_hash,omitempty"`
	MaterializedPath string `json:"materialized_path,omitempty"`
}

func seedRunSkillSnapshotHandler(repo *officesqlite.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedRunSkillSnapshotRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.RunID == "" || req.SkillID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "run_id and skill_id are required"})
			return
		}
		snapshot := officemodels.RunSkillSnapshot{
			RunID:            req.RunID,
			SkillID:          req.SkillID,
			Version:          req.Version,
			ContentHash:      req.ContentHash,
			MaterializedPath: req.MaterializedPath,
		}
		if err := repo.CreateRunSkillSnapshots(c.Request.Context(), []officemodels.RunSkillSnapshot{snapshot}); err != nil {
			log.Error("test harness: seed run skill snapshot failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// seedCostEventRequest seeds an office_cost_events row directly.
// Used by the agent dashboard E2E spec to drive the costs section
// (aggregate + per-run rollup) without launching an agent.
type seedCostEventRequest struct {
	AgentProfileID string  `json:"agent_profile_id"`
	TaskID         string  `json:"task_id"`
	SessionID      string  `json:"session_id,omitempty"`
	TokensIn       int64   `json:"tokens_in"`
	TokensOut      int64   `json:"tokens_out"`
	TokensCachedIn int64   `json:"tokens_cached_in"`
	CostSubcents   int64   `json:"cost_subcents"`
	Estimated      bool    `json:"estimated,omitempty"`
	OccurredAt     *string `json:"occurred_at,omitempty"`
}

// seedCostEventHandler appends a synthetic office_cost_events row.
// Tests can drive the agent dashboard's costs surface (aggregate +
// per-run rollup) by seeding rows tied to the agent + a task it
// claimed.
func seedCostEventHandler(repo *officesqlite.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedCostEventRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.AgentProfileID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent_profile_id is required"})
			return
		}
		occurred := time.Now().UTC()
		if req.OccurredAt != nil && *req.OccurredAt != "" {
			t, err := time.Parse(time.RFC3339, *req.OccurredAt)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "occurred_at must be RFC3339"})
				return
			}
			occurred = t.UTC()
		}
		ctx := c.Request.Context()
		event := &officemodels.CostEvent{
			ID:             uuid.New().String(),
			SessionID:      req.SessionID,
			TaskID:         req.TaskID,
			AgentProfileID: req.AgentProfileID,
			Model:          "test-model",
			Provider:       "test",
			TokensIn:       req.TokensIn,
			TokensCachedIn: req.TokensCachedIn,
			TokensOut:      req.TokensOut,
			CostSubcents:   req.CostSubcents,
			Estimated:      req.Estimated,
			OccurredAt:     occurred,
		}
		if err := repo.CreateCostEvent(ctx, event); err != nil {
			log.Error("test harness: create cost event failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": event.ID})
	}
}

// seedActivityRequest seeds an office_activity_log row directly. The
// agent dashboard summary aggregates these to drive the per-day
// priority + status charts and the recent-tasks list, so the E2E
// spec needs to write them without going through the production
// mutation paths.
type seedActivityRequest struct {
	WorkspaceID string  `json:"workspace_id"`
	ActorType   string  `json:"actor_type"`
	ActorID     string  `json:"actor_id"`
	Action      string  `json:"action"`
	TargetType  string  `json:"target_type"`
	TargetID    string  `json:"target_id"`
	Details     string  `json:"details,omitempty"`
	RunID       string  `json:"run_id,omitempty"`
	SessionID   string  `json:"session_id,omitempty"`
	CreatedAt   *string `json:"created_at,omitempty"`
}

// seedActivityHandler creates a single office_activity_log row.
// All inputs are written verbatim — callers control workspace, actor,
// action, target, and timestamp.
func seedActivityHandler(repo *officesqlite.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req seedActivityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		if req.WorkspaceID == "" || req.ActorType == "" || req.ActorID == "" || req.Action == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id, actor_type, actor_id, action are required"})
			return
		}
		details := req.Details
		if details == "" {
			details = "{}"
		}
		entry := &officemodels.ActivityEntry{
			ID:          uuid.New().String(),
			WorkspaceID: req.WorkspaceID,
			ActorType:   req.ActorType,
			ActorID:     req.ActorID,
			Action:      req.Action,
			TargetType:  req.TargetType,
			TargetID:    req.TargetID,
			Details:     details,
			RunID:       req.RunID,
			SessionID:   req.SessionID,
		}
		if req.CreatedAt != nil && *req.CreatedAt != "" {
			t, err := time.Parse(time.RFC3339, *req.CreatedAt)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "created_at must be RFC3339"})
				return
			}
			entry.CreatedAt = t.UTC()
		}
		ctx := c.Request.Context()
		if err := repo.CreateActivityEntry(ctx, entry); err != nil {
			log.Error("test harness: create activity entry failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": entry.ID})
	}
}
