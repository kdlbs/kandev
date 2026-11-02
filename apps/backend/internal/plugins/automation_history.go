package plugins

import (
	"context"
	"errors"
	"time"
)

// ErrAutomationHistoryNotFound collapses missing and foreign automation
// history identities into one public outcome.
var ErrAutomationHistoryNotFound = errors.New("plugins: automation history target not found")

// AutomationHistoryTrigger is the safe trigger projection exposed to plugins.
type AutomationHistoryTrigger struct {
	ID                string
	Kind              string
	Enabled           bool
	CronExpression    string
	Timezone          string
	ScheduleAvailable bool
}

// AutomationHistoryDefinition is one safe definition observation.
type AutomationHistoryDefinition struct {
	ID              string
	WorkspaceID     string
	Name            string
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ResourceVersion string
	Triggers        []AutomationHistoryTrigger
}

// AutomationHistoryRun is one safe retained-run observation. SourceStatus is
// the stored run status; Status is the automation service's read projection.
type AutomationHistoryRun struct {
	ID                    string
	WorkspaceID           string
	AutomationID          string
	AutomationName        string
	TriggerID             string
	TriggerKind           string
	TaskID                string
	SessionID             string
	TurnID                string
	ManagedConversationID string
	SourceStatus          string
	Status                string
	RetryState            string
	DeliveryStatus        string
	RetryGroupID          string
	RetryParentRunID      string
	AttemptNumber         *int64
	CreatedAt             time.Time
	RetryScheduledAt      *time.Time
	RetryClaimedAt        *time.Time
	Summary               string
	ErrorExcerpt          string
	ResourceVersion       string
}

type AutomationHistoryDefinitionList struct {
	Items    []AutomationHistoryDefinition
	Overflow bool
}

type AutomationHistoryRunList struct {
	Items    []AutomationHistoryRun
	Overflow bool
}

// AutomationHistoryDataSource is the plugin-owned seam over the automation
// service. Automation models stay behind this interface so the plugin package
// does not import the automation package.
type AutomationHistoryDataSource interface {
	ListAutomationHistoryDefinitions(ctx context.Context, workspaceID, automationID string,
		createdFrom, createdBefore *time.Time, limit int) (AutomationHistoryDefinitionList, error)
	ListAutomationHistoryRuns(ctx context.Context, workspaceID, automationID string,
		createdFrom, createdBefore *time.Time, limit int) (AutomationHistoryRunList, error)
	GetAutomationHistoryRun(ctx context.Context, workspaceID, runID string) (AutomationHistoryRun, error)
}
