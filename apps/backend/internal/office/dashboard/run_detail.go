package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// RunDetailRepo is the subset of repo functions GetRunDetail needs.
// Inlined here so the dashboard package's existing service interface
// stays untouched — Wave 1.B owns this slice and we don't want to
// edit the larger Repository interface.
type RunDetailRepo interface {
	GetRunWithCosts(ctx context.Context, runID string) (*models.Run, *sqlite.RunCostRollup, error)
	ListTasksTouchedByRun(ctx context.Context, runID string) ([]string, error)
	ListRunEvents(ctx context.Context, runID string, afterSeq, limit int) ([]*models.RunEvent, error)
	ListRunSkillSnapshots(ctx context.Context, runID string) ([]models.RunSkillSnapshot, error)
	ListRunsForAgentPaged(
		ctx context.Context, agentInstanceID string, cursor time.Time, cursorID string, limit int,
	) ([]*models.Run, error)
	GetAgentInstance(ctx context.Context, id string) (*models.AgentInstance, error)
	ListRouteAttempts(ctx context.Context, runID string) ([]models.RouteAttempt, error)
}

// ErrRunNotFound is returned when GetRunDetail can't find the run id.
var ErrRunNotFound = errors.New("run not found")

// ErrRunAgentMismatch is returned when the run does not belong to the
// requested agent. Lets the handler send a 404 rather than leaking
// runs across agents via a guessable URL.
var ErrRunAgentMismatch = errors.New("run does not belong to agent")

// runIDShort is the fixed display length for the truncated run id
// surfaced on cards and lists. Eight chars keeps two-line lists scannable.
const runIDShort = 8

// shortID truncates the run id to the first 8 chars so the
// frontend doesn't have to reproduce the slicing in every cell.
func shortID(id string) string {
	if len(id) <= runIDShort {
		return id
	}
	return id[:runIDShort]
}

// formatTime returns the RFC3339 representation of a time pointer or
// "" when nil. Centralised so every DTO consumes the same format.
func formatTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// runDuration returns the elapsed milliseconds between claimed_at
// and finished_at; falls back to (now - claimed_at) when the run is
// still active. Returns 0 when the run hasn't been claimed yet.
func runDuration(run *models.Run) int64 {
	if run.ClaimedAt == nil || run.ClaimedAt.IsZero() {
		return 0
	}
	end := time.Now().UTC()
	if run.FinishedAt != nil && !run.FinishedAt.IsZero() {
		end = *run.FinishedAt
	}
	d := end.Sub(*run.ClaimedAt)
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}

// taskIDFromPayload extracts task_id from the run's payload JSON.
// Empty string when the payload doesn't carry a task_id (e.g.
// scheduled wakeups).
func taskIDFromPayload(payload string) string {
	if payload == "" {
		return ""
	}
	var p struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return ""
	}
	return p.TaskID
}

// runLinkIDsFromPayload extracts comment_id and routine_id from the
// run payload — both optional and present per wakeup source (comment
// payloads carry the comment, routine payloads carry the routine).
// The list view uses these to deeplink each row to its triggering
// entity.
func runLinkIDsFromPayload(payload string) (commentID, routineID string) {
	if payload == "" {
		return "", ""
	}
	var p struct {
		CommentID string `json:"comment_id"`
		RoutineID string `json:"routine_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return "", ""
	}
	return p.CommentID, p.RoutineID
}

// sessionIDFromPayload extracts session_id from the run's payload
// JSON. The wakeup builder includes the claimed session id when
// available so the run detail can render the conversation embed and
// drive the live-mode subscription.
func sessionIDFromPayload(payload string) string {
	if payload == "" {
		return ""
	}
	var p struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return ""
	}
	return p.SessionID
}

// buildRunSummaryDTO converts a Run row into the list-summary DTO.
func buildRunSummaryDTO(run *models.Run) AgentRunSummaryDTO {
	commentID, routineID := runLinkIDsFromPayload(run.Payload)
	dto := AgentRunSummaryDTO{
		ID:           run.ID,
		IDShort:      shortID(run.ID),
		Reason:       run.Reason,
		Status:       run.Status,
		ErrorMessage: run.ErrorMessage,
		TaskID:       taskIDFromPayload(run.Payload),
		CommentID:    commentID,
		RoutineID:    routineID,
		RequestedAt:  run.RequestedAt.UTC().Format(time.RFC3339),
		ClaimedAt:    formatTime(run.ClaimedAt),
		FinishedAt:   formatTime(run.FinishedAt),
		DurationMs:   runDuration(run),
	}
	if run.CancelReason != nil && *run.CancelReason != "" {
		v := *run.CancelReason
		dto.CancelReason = &v
	}
	return dto
}

// ListAgentRunsPaged returns one page of runs for the given agent.
// cursorRFC and cursorID are the (requested_at, id) pair carried
// over from the previous page; both empty fetches the first page.
// Returned next_cursor is the requested_at of the last row in the
// page (RFC3339); empty when this was the last page.
func ListAgentRunsPaged(
	ctx context.Context,
	repo RunDetailRepo,
	agentID string,
	cursorRFC, cursorID string,
	limit int,
) (*AgentRunsListResponse, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	var cursor time.Time
	if cursorRFC != "" {
		t, err := time.Parse(time.RFC3339, cursorRFC)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		cursor = t.UTC()
	}
	rows, err := repo.ListRunsForAgentPaged(ctx, agentID, cursor, cursorID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]AgentRunSummaryDTO, len(rows))
	for i, r := range rows {
		out[i] = buildRunSummaryDTO(r)
	}
	resp := &AgentRunsListResponse{Runs: out}
	// Only emit a next_cursor if we filled the page; a partial page
	// means we ran out of rows and there is no further data.
	if len(rows) == limit && len(rows) > 0 {
		last := rows[len(rows)-1]
		resp.NextCursor = last.RequestedAt.UTC().Format(time.RFC3339)
		resp.NextID = last.ID
	}
	return resp, nil
}

// GetRunDetail returns the per-run aggregate used by the run detail
// page: header data + session ids + cost rollup + invocation +
// tasks_touched (union of activity-log rows and the run's payload
// task_id) + events list.
//
// Returns ErrRunNotFound when the run id is unknown and
// ErrRunAgentMismatch when the run belongs to a different agent.
func GetRunDetail(
	ctx context.Context,
	repo RunDetailRepo,
	agentID, runID string,
) (*RunDetailResponse, error) {
	run, costs, err := repo.GetRunWithCosts(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	if run.AgentProfileID != agentID {
		return nil, ErrRunAgentMismatch
	}

	touched, err := repo.ListTasksTouchedByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	taskIDs := mergeTaskIDs(touched, taskIDFromPayload(run.Payload))

	events, err := repo.ListRunEvents(ctx, runID, -1, 0)
	if err != nil {
		return nil, err
	}
	eventDTOs := make([]RunEventDTO, len(events))
	for i, e := range events {
		eventDTOs[i] = RunEventDTO{
			Seq:       e.Seq,
			EventType: e.EventType,
			Level:     e.Level,
			Payload:   e.Payload,
			CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
		}
	}

	invocation := buildInvocation(ctx, repo, run)
	sessionDTO := RunSessionDTO{SessionID: sessionIDFromPayload(run.Payload)}
	runtimeDTO, err := buildRuntimeDTO(ctx, repo, run)
	if err != nil {
		return nil, err
	}

	resp := &RunDetailResponse{
		ID:           run.ID,
		IDShort:      shortID(run.ID),
		AgentID:      run.AgentProfileID,
		Reason:       run.Reason,
		Status:       run.Status,
		ErrorMessage: run.ErrorMessage,
		TaskID:       taskIDFromPayload(run.Payload),
		RequestedAt:  run.RequestedAt.UTC().Format(time.RFC3339),
		ClaimedAt:    formatTime(run.ClaimedAt),
		FinishedAt:   formatTime(run.FinishedAt),
		DurationMs:   runDuration(run),
		Costs: RunCostSummaryDTO{
			InputTokens:  costs.InputTokens,
			OutputTokens: costs.OutputTokens,
			CachedTokens: costs.CachedTokens,
			CostSubcents: costs.CostSubcents,
		},
		Session:         sessionDTO,
		Invocation:      invocation,
		Runtime:         runtimeDTO,
		TasksTouched:    taskIDs,
		Events:          eventDTOs,
		AssembledPrompt: run.AssembledPrompt,
		SummaryInjected: run.SummaryInjected,
		ResultJSON:      run.ResultJSON,
		ContextSnapshot: run.ContextSnapshot,
		OutputSummary:   run.OutputSummary,
	}
	if run.CancelReason != nil && *run.CancelReason != "" {
		v := *run.CancelReason
		resp.CancelReason = &v
	}
	if routingBlock, rerr := buildRunRouting(ctx, repo, run); rerr == nil {
		resp.Routing = routingBlock
	}
	return resp, nil
}

// buildRunRouting populates the routing block on the run-detail response.
// Returns (nil, nil) when the run has no routing snapshot and no
// attempts — the column-typed fields are nullable on legacy rows.
func buildRunRouting(
	ctx context.Context, repo RunDetailRepo, run *models.Run,
) (*RunRouting, error) {
	attempts, err := repo.ListRouteAttempts(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if len(attempts) == 0 && !runHasRoutingSnapshot(run) {
		return nil, nil
	}
	out := &RunRouting{
		LogicalProviderOrder: decodeOrderSnapshot(run.LogicalProviderOrder),
		Attempts:             make([]RouteAttemptDTO, len(attempts)),
	}
	for i, a := range attempts {
		out.Attempts[i] = routeAttemptToDTO(a)
	}
	if run.RequestedTier != nil {
		out.RequestedTier = *run.RequestedTier
	}
	if run.ResolvedProviderID != nil {
		out.ResolvedProviderID = *run.ResolvedProviderID
	}
	if run.ResolvedModel != nil {
		out.ResolvedModel = *run.ResolvedModel
	}
	if run.RoutingBlockedStatus != nil {
		out.BlockedStatus = *run.RoutingBlockedStatus
	}
	if run.EarliestRetryAt != nil && !run.EarliestRetryAt.IsZero() {
		s := run.EarliestRetryAt.UTC().Format(time.RFC3339)
		out.EarliestRetryAt = &s
	}
	return out, nil
}

func runHasRoutingSnapshot(run *models.Run) bool {
	if run == nil {
		return false
	}
	if run.LogicalProviderOrder != nil && *run.LogicalProviderOrder != "" {
		return true
	}
	if run.ResolvedProviderID != nil && *run.ResolvedProviderID != "" {
		return true
	}
	if run.RoutingBlockedStatus != nil && *run.RoutingBlockedStatus != "" {
		return true
	}
	return false
}

func decodeOrderSnapshot(raw *string) []string {
	if raw == nil || *raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(*raw), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func buildRuntimeDTO(ctx context.Context, repo RunDetailRepo, run *models.Run) (RunRuntimeDTO, error) {
	snapshots, err := repo.ListRunSkillSnapshots(ctx, run.ID)
	if err != nil {
		return RunRuntimeDTO{}, err
	}
	skills := make([]RunSkillDTO, 0, len(snapshots))
	for _, snap := range snapshots {
		skills = append(skills, RunSkillDTO{
			SkillID:          snap.SkillID,
			Version:          snap.Version,
			ContentHash:      snap.ContentHash,
			MaterializedPath: snap.MaterializedPath,
		})
	}
	return RunRuntimeDTO{
		Capabilities:  parseJSONMap(run.Capabilities),
		InputSnapshot: parseJSONMap(run.InputSnapshot),
		SessionID:     run.SessionID,
		Skills:        skills,
	}, nil
}

func parseJSONMap(raw string) map[string]interface{} {
	if raw == "" {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]interface{}{}
	}
	if out == nil {
		return map[string]interface{}{}
	}
	return out
}

// mergeTaskIDs returns the deduplicated union of touched ids and the
// run's primary task id (if any). Order: touched ids first (in repo
// order), with the primary task appended if not already present.
func mergeTaskIDs(touched []string, primary string) []string {
	if primary == "" && len(touched) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(touched)+1)
	out := make([]string, 0, len(touched)+1)
	for _, id := range touched {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if primary != "" {
		if _, ok := seen[primary]; !ok {
			out = append(out, primary)
		}
	}
	return out
}

// buildInvocation populates the invocation panel best-effort from
// the agent instance + run payload. The agent profile carries the
// adapter family and model; the working directory is workspace-
// relative for now (orchestrator logs will fill in the rest in a
// later wave). Missing fields stay empty so the frontend can hide
// them gracefully.
func buildInvocation(
	ctx context.Context,
	repo RunDetailRepo,
	run *models.Run,
) RunInvocationDTO {
	dto := RunInvocationDTO{}
	agent, err := repo.GetAgentInstance(ctx, run.AgentProfileID)
	if err != nil || agent == nil {
		return dto
	}
	// Wave G: AgentInstance.ID == agent_profiles.id under the unified model.
	dto.Adapter = agent.ID
	// Model lives on the agent profile, not the agent instance —
	// surfacing it requires plumbing the profile reader through.
	// Wave 2.E will fold the adapter+model lookup in; for v1 we
	// surface what we have.
	return dto
}
