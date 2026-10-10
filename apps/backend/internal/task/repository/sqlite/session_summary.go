package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) BatchGetTaskSessionSummaryObservations(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSessionSummaryObservation, error) {
	result := make(map[string][]*models.TaskSessionSummaryObservation, len(taskIDs))
	ids := uniqueSessionSummaryTaskIDs(taskIDs)
	for _, taskID := range ids {
		result[taskID] = []*models.TaskSessionSummaryObservation{}
	}
	for _, chunk := range chunkIDs(ids, sqliteMaxHostParams) {
		placeholders, args := buildInPlaceholders(chunk)
		query := `SELECT ` + taskSessionSummarySelectCols(r.ro.DriverName()) + ` ` + taskSessionFromClause +
			` LEFT JOIN executors e ON e.id = ts.executor_id` +
			` WHERE ts.task_id IN (` + placeholders + `) ORDER BY ts.task_id, ts.started_at DESC`
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, fmt.Errorf("query task session summaries: %w", err)
		}
		for rows.Next() {
			observation, err := scanTaskSessionSummaryObservation(rows)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan task session summary: %w", err)
			}
			result[observation.TaskID] = append(result[observation.TaskID], observation)
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if rowsErr != nil {
			return nil, fmt.Errorf("iterate task session summaries: %w", rowsErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close task session summaries: %w", closeErr)
		}
	}
	return result, nil
}

func (r *Repository) ListTaskSessionSummaryObservations(
	ctx context.Context,
	taskID string,
) ([]*models.TaskSessionSummaryObservation, error) {
	if taskID == "" {
		return []*models.TaskSessionSummaryObservation{}, nil
	}
	byTask, err := r.BatchGetTaskSessionSummaryObservations(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	observations := byTask[taskID]
	if len(observations) == 0 {
		return observations, nil
	}
	sessions := make([]*models.TaskSession, len(observations))
	for index, observation := range observations {
		sessions[index] = observation.ToTaskSession()
	}
	loaded, err := r.loadWorktreesBatch(ctx, sessions)
	if err != nil {
		return nil, err
	}
	for index := range observations {
		observations[index].Worktrees = loaded[index].Worktrees
	}
	return observations, nil
}

func uniqueSessionSummaryTaskIDs(taskIDs []string) []string {
	seen := make(map[string]struct{}, len(taskIDs))
	ids := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if _, exists := seen[taskID]; exists {
			continue
		}
		seen[taskID] = struct{}{}
		ids = append(ids, taskID)
	}
	return ids
}

func taskSessionSummarySelectCols(driver string) string {
	metadata := "CASE WHEN ts.metadata IS NULL OR ts.metadata = 'null' OR ts.metadata = '' THEN '{}' ELSE ts.metadata END"
	if dialect.IsPostgres(driver) {
		metadata = "CASE WHEN ts.metadata IS NULL OR ts.metadata = 'null' OR ts.metadata = '' THEN '{}'::jsonb ELSE ts.metadata::jsonb END"
	}
	return strings.Join([]string{
		"ts.id", "ts.task_id", "ts.queue_incarnation_id",
		"COALESCE(er.agent_execution_id, '')", "COALESCE(er.container_id, '')",
		"ts.agent_profile_id", "ts.execution_profile_id", "ts.route_generation", "ts.route_state", "ts.route_reason",
		"ts.executor_id", "ts.executor_profile_id", "ts.environment_id",
		"ts.repository_id", "ts.base_branch", "ts.base_commit_sha",
		"COALESCE(NULLIF(te.workspace_path, ''), ts.workspace_path)",
		dialect.JSONExtract(driver, "ts.agent_profile_snapshot", "name"),
		"e.type AS executor_type", "e.name AS executor_name",
		dialect.JSONExtract(driver, "ts.repository_snapshot", "path"),
		"ts.state", "ts.error_message", dialect.JSONExtract(driver, metadata, models.SessionMetaKeyLastAgentError),
		"ts.started_at", "ts.completed_at", "ts.updated_at", "ts.is_primary", "ts.review_status",
		"ts.is_passthrough", "ts.task_environment_id", "ts.name", "ts.last_read_message_id",
	}, ", ")
}

func scanTaskSessionSummaryObservation(rows *sql.Rows) (*models.TaskSessionSummaryObservation, error) {
	var observation models.TaskSessionSummaryObservation
	var (
		queueIncarnationID sql.NullString
		agentProfileID     sql.NullString
		executionProfileID sql.NullString
		routeState         sql.NullString
		routeReason        sql.NullString
		executorID         sql.NullString
		executorProfileID  sql.NullString
		environmentID      sql.NullString
		repositoryID       sql.NullString
		baseBranch         sql.NullString
		baseCommitSHA      sql.NullString
		workspacePath      sql.NullString
		agentProfileName   sql.NullString
		executorType       sql.NullString
		executorName       sql.NullString
		repositoryPath     sql.NullString
		state              sql.NullString
		errorMessage       sql.NullString
		lastAgentErrorJSON sql.NullString
		completedAt        sql.NullTime
		isPrimary          int
		reviewStatus       sql.NullString
		isPassthrough      int
		taskEnvironmentID  sql.NullString
		name               sql.NullString
		lastReadMessageID  sql.NullString
	)
	if err := rows.Scan(
		&observation.ID, &observation.TaskID, &queueIncarnationID,
		&observation.AgentExecutionID, &observation.ContainerID,
		&agentProfileID, &executionProfileID, &observation.RouteGeneration,
		&routeState, &routeReason, &executorID, &executorProfileID,
		&environmentID, &repositoryID, &baseBranch, &baseCommitSHA, &workspacePath,
		&agentProfileName, &executorType, &executorName, &repositoryPath,
		&state, &errorMessage, &lastAgentErrorJSON,
		&observation.StartedAt, &completedAt, &observation.UpdatedAt, &isPrimary,
		&reviewStatus, &isPassthrough, &taskEnvironmentID, &name, &lastReadMessageID,
	); err != nil {
		return nil, err
	}
	observation.QueueIncarnationID = sessionSummaryNullableString(queueIncarnationID)
	observation.AgentProfileID = sessionSummaryNullableString(agentProfileID)
	observation.ExecutionProfileID = sessionSummaryNullableString(executionProfileID)
	observation.RouteState = sessionSummaryNullableString(routeState)
	observation.RouteReason = sessionSummaryNullableString(routeReason)
	observation.ExecutorID = sessionSummaryNullableString(executorID)
	observation.ExecutorProfileID = sessionSummaryNullableString(executorProfileID)
	observation.EnvironmentID = sessionSummaryNullableString(environmentID)
	observation.RepositoryID = sessionSummaryNullableString(repositoryID)
	observation.BaseBranch = sessionSummaryNullableString(baseBranch)
	observation.BaseCommitSHA = sessionSummaryNullableString(baseCommitSHA)
	observation.WorkspacePath = sessionSummaryNullableString(workspacePath)
	observation.AgentProfileName = sessionSummaryNullableString(agentProfileName)
	observation.ExecutorType = sessionSummaryNullableString(executorType)
	observation.ExecutorName = sessionSummaryNullableString(executorName)
	observation.RepositoryPath = sessionSummaryNullableString(repositoryPath)
	observation.State = models.TaskSessionState(sessionSummaryNullableString(state))
	observation.ErrorMessage = sessionSummaryNullableString(errorMessage)
	observation.IsPrimary = isPrimary != 0
	observation.IsPassthrough = isPassthrough != 0
	observation.ReviewStatus = models.ReviewStatus(sessionSummaryNullableString(reviewStatus))
	observation.TaskEnvironmentID = sessionSummaryNullableString(taskEnvironmentID)
	observation.Name = sessionSummaryNullableString(name)
	observation.LastReadMessageID = sessionSummaryNullableString(lastReadMessageID)
	if completedAt.Valid {
		observation.CompletedAt = &completedAt.Time
	}
	metadata := map[string]interface{}{models.SessionMetaKeyLastAgentError: nil}
	if lastAgentErrorJSON.Valid && lastAgentErrorJSON.String != "" && lastAgentErrorJSON.String != "null" {
		var lastAgentError interface{}
		if err := json.Unmarshal([]byte(lastAgentErrorJSON.String), &lastAgentError); err != nil {
			return nil, fmt.Errorf("decode last agent error projection: %w", err)
		}
		metadata[models.SessionMetaKeyLastAgentError] = lastAgentError
	}
	observation.Metadata = metadata
	return &observation, nil
}

func sessionSummaryNullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
