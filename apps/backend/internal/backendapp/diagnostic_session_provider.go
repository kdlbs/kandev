package backendapp

import (
	"context"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/system/logbundle"
	"github.com/kandev/kandev/internal/task/models"
)

type diagnosticSessionReader interface {
	ListWorkspaces(context.Context) ([]*models.Workspace, error)
	ListTasksByWorkspace(
		context.Context, string, string, string, string, int, int, string,
		bool, bool, bool, bool,
	) ([]*models.Task, int, error)
	ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
	GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
}

type diagnosticSessionProvider struct {
	reader diagnosticSessionReader
}

func newDiagnosticSessionProvider(reader diagnosticSessionReader) *diagnosticSessionProvider {
	return &diagnosticSessionProvider{reader: reader}
}

func (p *diagnosticSessionProvider) ListDiagnosticSessions(
	ctx context.Context,
	identity authn.Identity,
	since time.Time,
	selectedIDs []string,
) ([]logbundle.DiagnosticSession, error) {
	if p == nil || p.reader == nil {
		return []logbundle.DiagnosticSession{}, nil
	}
	readerCtx := authn.WithIdentity(ctx, identity)
	selected := selectedSessionSet(selectedIDs)
	rows, err := p.collectCatalog(readerCtx, since, selected)
	if err != nil {
		return nil, err
	}
	p.collectExplicit(readerCtx, selected, rows)
	return finalizeDiagnosticSessions(rows, selected), nil
}

const logbundleRuntimePageSize = 500

func sessionInWindow(session *models.TaskSession, since time.Time) bool {
	return !session.StartedAt.Before(since) || !session.UpdatedAt.Before(since)
}

func selectedSessionSet(selectedIDs []string) map[string]bool {
	selected := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		selected[id] = true
	}
	return selected
}

func (p *diagnosticSessionProvider) collectCatalog(
	ctx context.Context,
	since time.Time,
	selected map[string]bool,
) (map[string]logbundle.DiagnosticSession, error) {
	workspaces, err := p.reader.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	rows := make(map[string]logbundle.DiagnosticSession)
	for _, workspace := range workspaces {
		if workspace == nil || workspace.ID == "" {
			continue
		}
		if err := p.collectWorkspace(ctx, workspace.ID, since, selected, rows); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (p *diagnosticSessionProvider) collectWorkspace(
	ctx context.Context,
	workspaceID string,
	since time.Time,
	selected map[string]bool,
	rows map[string]logbundle.DiagnosticSession,
) error {
	tasks, _, err := p.reader.ListTasksByWorkspace(
		ctx, workspaceID, "", "", "", 1, logbundleRuntimePageSize,
		"updated_at", true, true, false, true,
	)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		sessions, err := p.reader.ListTaskSessions(ctx, task.ID)
		if err != nil {
			return err
		}
		for _, session := range sessions {
			if session == nil || (!selected[session.ID] && !sessionInWindow(session, since)) {
				continue
			}
			rows[session.ID] = p.diagnosticSessionRow(ctx, session, task.Title)
		}
	}
	return nil
}

// Explicit selections may be older than the normal catalogue window. Read
// those exact rows after the bounded catalogue walk; authorization happens
// inside the task service before any runtime/ACP metadata is consulted.
func (p *diagnosticSessionProvider) collectExplicit(
	ctx context.Context,
	selected map[string]bool,
	rows map[string]logbundle.DiagnosticSession,
) {
	for sessionID := range selected {
		if _, ok := rows[sessionID]; ok {
			continue
		}
		session, err := p.reader.GetTaskSession(ctx, sessionID)
		if err == nil && session != nil {
			rows[session.ID] = p.diagnosticSessionRow(ctx, session, "")
		}
	}
}

func finalizeDiagnosticSessions(
	rows map[string]logbundle.DiagnosticSession,
	selected map[string]bool,
) []logbundle.DiagnosticSession {
	result := make([]logbundle.DiagnosticSession, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].LastActivityAt.After(result[j].LastActivityAt)
	})
	if len(result) > logbundleRuntimePageSize {
		result = keepSelectedRows(result, selected, logbundleRuntimePageSize)
	}
	return result
}

func (p *diagnosticSessionProvider) diagnosticSessionRow(
	ctx context.Context, session *models.TaskSession, taskTitle string,
) logbundle.DiagnosticSession {
	row := logbundle.DiagnosticSession{
		TaskID: session.TaskID, TaskTitle: taskTitle, SessionID: session.ID, Status: string(session.State),
		StartedAt: session.StartedAt, LastActivityAt: session.UpdatedAt,
		ACPSessionID: snapshotString(session.Metadata, "acp_session_id"),
		Agent:        snapshotString(session.AgentProfileSnapshot, "agent_name", "agent_id", "agent"),
		Provider:     snapshotString(session.AgentProfileSnapshot, "provider", "provider_id"),
		Model:        snapshotString(session.AgentProfileSnapshot, "model", "model_id"),
		ExecutorType: snapshotString(session.ExecutorSnapshot, "type", "executor_type"),
	}
	if row.ACPSessionID == "" && p.reader != nil {
		if running, err := p.reader.GetExecutorRunningBySessionID(ctx, session.ID); err == nil && running != nil {
			row.ACPSessionID = running.ResumeToken
		}
	}
	switch {
	case row.ACPSessionID != "" && session.AgentExecutionID != "":
		row.ACPAvailability = "reachable"
	case row.ACPSessionID != "":
		row.ACPAvailability = "host_retained"
	default:
		row.ACPAvailability = "unavailable"
	}
	return row
}

func snapshotString(snapshot map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := snapshot[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func keepSelectedRows(
	rows []logbundle.DiagnosticSession,
	selected map[string]bool,
	limit int,
) []logbundle.DiagnosticSession {
	if len(rows) <= limit {
		return rows
	}
	result := append([]logbundle.DiagnosticSession(nil), rows[:limit]...)
	seen := make(map[string]bool, len(result))
	for _, row := range result {
		seen[row.SessionID] = true
	}
	for _, row := range rows[limit:] {
		if !selected[row.SessionID] || seen[row.SessionID] {
			continue
		}
		result[len(result)-1] = row
		seen[row.SessionID] = true
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].LastActivityAt.After(result[j].LastActivityAt)
	})
	return result
}
