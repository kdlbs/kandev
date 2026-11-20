package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrHistoryAutomationNotFound covers both a missing automation filter target
// and one that belongs to another workspace, so a reader cannot distinguish
// the two outcomes.
var ErrHistoryAutomationNotFound = errors.New("automation: history automation not found")

// ErrHistoryRunNotFound covers both a missing run and one whose automation
// belongs to another workspace.
var ErrHistoryRunNotFound = errors.New("automation: history run not found")

// HistoryDefinitionFilter selects a bounded set of automation definitions.
// CreatedFrom is inclusive and CreatedBefore is exclusive; a nil bound is
// omitted. Limit must be positive and is capped by the caller's snapshot
// contract.
type HistoryDefinitionFilter struct {
	WorkspaceID   string
	AutomationID  string
	CreatedFrom   *time.Time
	CreatedBefore *time.Time
	Limit         int
}

// HistoryRunFilter selects a bounded set of retained run observations.
type HistoryRunFilter struct {
	WorkspaceID   string
	AutomationID  string
	CreatedFrom   *time.Time
	CreatedBefore *time.Time
	Limit         int
}

// HistoryTrigger is the public-safe trigger projection. Configuration is
// only decoded for scheduled triggers; every other kind keeps its identity
// without exposing its configuration.
type HistoryTrigger struct {
	ID                string
	Kind              TriggerType
	Enabled           bool
	CronExpression    string
	Timezone          string
	ScheduleAvailable bool
}

// HistoryDefinition is one automation definition observation.
type HistoryDefinition struct {
	ID          string
	WorkspaceID string
	Name        string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ObservedAt  time.Time
	Triggers    []HistoryTrigger
}

// HistoryRun is one retained run observation. SourceStatus retains the stored
// run status while Status carries the automation service's read projection.
type HistoryRun struct {
	ID                    string
	WorkspaceID           string
	AutomationID          string
	AutomationName        string
	TriggerID             string
	TriggerKind           TriggerType
	TaskID                string
	SessionID             string
	TurnID                string
	ManagedConversationID string
	SourceStatus          RunStatus
	Status                RunStatus
	RetryState            RetryState
	DeliveryStatus        ManagedDeliveryStatus
	RetryGroupID          string
	RetryParentRunID      string
	AttemptNumber         *int64
	CreatedAt             time.Time
	RetryScheduledAt      *time.Time
	RetryClaimedAt        *time.Time
	Summary               string
	ErrorMessage          string
	ObservedAt            time.Time
}

// HistoryDefinitionList reports whether the bounded query had more matching
// rows than its caller requested.
type HistoryDefinitionList struct {
	Items    []HistoryDefinition
	Overflow bool
}

// HistoryRunList reports whether the bounded query had more matching rows
// than its caller requested.
type HistoryRunList struct {
	Items    []HistoryRun
	Overflow bool
}

// ListHistoryAutomations returns a bounded, workspace-scoped definition
// snapshot. An optional automation filter must name a definition in the
// workspace; unknown and foreign identities share ErrHistoryAutomationNotFound.
func (s *Service) ListHistoryAutomations(ctx context.Context, filter HistoryDefinitionFilter) (*HistoryDefinitionList, error) {
	if filter.Limit <= 0 {
		return nil, errors.New("automation: history definition limit must be positive")
	}
	if err := s.authorizeWs(ctx, filter.WorkspaceID); err != nil {
		return nil, err
	}
	rows, overflow, err := s.store.ListHistoryAutomations(ctx, filter)
	if err != nil {
		return nil, err
	}
	observedAt := time.Now().UTC()
	items := make([]HistoryDefinition, 0, len(rows))
	for _, row := range rows {
		items = append(items, historyDefinitionFromAutomation(row, observedAt))
	}
	return &HistoryDefinitionList{Items: items, Overflow: overflow}, nil
}

// ListHistoryRuns returns a bounded, workspace-scoped run observation
// snapshot ordered newest first.
func (s *Service) ListHistoryRuns(ctx context.Context, filter HistoryRunFilter) (*HistoryRunList, error) {
	if filter.Limit <= 0 {
		return nil, errors.New("automation: history run limit must be positive")
	}
	if err := s.authorizeWs(ctx, filter.WorkspaceID); err != nil {
		return nil, err
	}
	rows, overflow, err := s.store.ListHistoryRuns(ctx, filter)
	if err != nil {
		return nil, err
	}
	observedAt := time.Now().UTC()
	items := make([]HistoryRun, 0, len(rows))
	for _, row := range rows {
		items = append(items, historyRunFromRow(row, observedAt))
	}
	return &HistoryRunList{Items: items, Overflow: overflow}, nil
}

// GetHistoryRun refreshes one run through its owning automation and requested
// workspace together. The source status and projection match the list reader.
func (s *Service) GetHistoryRun(ctx context.Context, workspaceID, runID string) (*HistoryRun, error) {
	if err := s.authorizeWs(ctx, workspaceID); err != nil {
		return nil, err
	}
	row, err := s.store.GetHistoryRun(ctx, workspaceID, runID)
	if err != nil {
		return nil, err
	}
	item := historyRunFromRow(row, time.Now().UTC())
	return &item, nil
}

func historyDefinitionFromAutomation(a *Automation, observedAt time.Time) HistoryDefinition {
	definition := HistoryDefinition{
		ID: a.ID, WorkspaceID: a.WorkspaceID, Name: a.Name, Enabled: a.Enabled,
		CreatedAt: a.CreatedAt.UTC(), UpdatedAt: a.UpdatedAt.UTC(), ObservedAt: observedAt,
		Triggers: make([]HistoryTrigger, 0, len(a.Triggers)),
	}
	for _, trigger := range a.Triggers {
		definition.Triggers = append(definition.Triggers, historyTriggerFromModel(trigger))
	}
	return definition
}

func historyTriggerFromModel(trigger AutomationTrigger) HistoryTrigger {
	observation := HistoryTrigger{ID: trigger.ID, Kind: trigger.Type, Enabled: trigger.Enabled}
	if trigger.Type != TriggerTypeScheduled {
		return observation
	}
	var config ScheduledTriggerConfig
	if err := json.Unmarshal(trigger.Config, &config); err != nil || strings.TrimSpace(config.CronExpression) == "" {
		return observation
	}
	observation.CronExpression = config.CronExpression
	observation.Timezone = config.Timezone
	observation.ScheduleAvailable = true
	return observation
}

func historyRunFromRow(row *historyRunRow, observedAt time.Time) HistoryRun {
	run := row.AutomationRun
	observation := HistoryRun{
		ID: run.ID, WorkspaceID: row.WorkspaceID, AutomationID: run.AutomationID,
		AutomationName: row.AutomationName, TriggerID: run.TriggerID, TriggerKind: run.TriggerType,
		TaskID: run.TaskID, SessionID: run.SessionID, TurnID: run.TurnID,
		ManagedConversationID: run.ManagedConversationID,
		SourceStatus:          row.SourceStatus, Status: run.Status,
		RetryState: run.RetryState, DeliveryStatus: run.DeliveryStatus,
		RetryGroupID: run.RetryGroupID, RetryParentRunID: run.RetryParentRunID,
		CreatedAt: run.CreatedAt.UTC(), RetryScheduledAt: utcTimePtr(run.RetryScheduledAt),
		RetryClaimedAt: utcTimePtr(run.RetryClaimedAt),
		Summary:        run.Summary, ErrorMessage: run.ErrorMessage,
		ObservedAt: observedAt,
	}
	if run.AttemptNumber != 0 {
		attempt := run.AttemptNumber
		observation.AttemptNumber = &attempt
	}
	return observation
}

func utcTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}
