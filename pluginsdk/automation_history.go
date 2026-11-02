package pluginsdk

import (
	"context"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
)

// AutomationDefinitionFilter narrows a definition snapshot. CreatedFrom is
// inclusive and CreatedBefore is exclusive; nil bounds are omitted. Times are
// UTC RFC3339Nano strings.
type AutomationDefinitionFilter struct {
	AutomationID  string
	CreatedFrom   *string
	CreatedBefore *string
}

// AutomationRunFilter narrows a run snapshot. CreatedFrom is inclusive and
// CreatedBefore is exclusive; nil bounds are omitted. Times are UTC
// RFC3339Nano strings.
type AutomationRunFilter struct {
	AutomationID  string
	CreatedFrom   *string
	CreatedBefore *string
}

type AutomationDefinitionQuery struct {
	RequestID   string
	WorkspaceID string
	Filter      AutomationDefinitionFilter
	Page        ExactReadPage
}

type AutomationRunQuery struct {
	RequestID   string
	WorkspaceID string
	Filter      AutomationRunFilter
	Page        ExactReadPage
}

type AutomationRunGetQuery struct {
	RequestID   string
	WorkspaceID string
	RunID       string
}

// AutomationTriggerObservation is the safe public trigger projection.
// CronExpression and Timezone are only meaningful when ScheduleAvailable is
// true; an absent configured timezone stays nil.
type AutomationTriggerObservation struct {
	ID                string
	Kind              string
	Enabled           bool
	CronExpression    string
	Timezone          *string
	ScheduleAvailable bool
}

type AutomationDefinitionObservation struct {
	ID              string
	WorkspaceID     string
	Name            string
	Enabled         bool
	CreatedAt       string
	UpdatedAt       string
	ResourceVersion string
	ObservedAt      string
	Triggers        []AutomationTriggerObservation
}

// AutomationRunObservation preserves source status and observed status
// separately. Status values remain strings so unknown source values survive
// transport. A nil attempt number means the source did not record one.
type AutomationRunObservation struct {
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
	CreatedAt             string
	RetryScheduledAt      *string
	RetryClaimedAt        *string
	Summary               string
	ErrorExcerpt          string
	ResourceVersion       string
	ObservedAt            string
}

type AutomationDefinitionPage struct {
	Items    []AutomationDefinitionObservation
	PageInfo ExactReadPageInfo
}

type AutomationRunPage struct {
	Items    []AutomationRunObservation
	PageInfo ExactReadPageInfo
}

// AutomationHistoryReader is the optional workspace-authorized automation
// history reader.
type AutomationHistoryReader interface {
	ListAutomations(context.Context, AutomationDefinitionQuery) (AutomationDefinitionPage, error)
	ListRuns(context.Context, AutomationRunQuery) (AutomationRunPage, error)
	GetRun(context.Context, AutomationRunGetQuery) (AutomationRunObservation, HostReadReceipt, error)
}

// AutomationHistoryHost is an optional Host extension. The base Host interface
// stays unchanged so existing implementations keep working.
type AutomationHistoryHost interface {
	AutomationHistory() AutomationHistoryReader
}

// HostAutomationHistory reports whether host exposes the optional automation
// history reader. A remote Host wrapper always reports true; a call against an
// older server then fails with Unimplemented.
func HostAutomationHistory(host Host) (AutomationHistoryReader, bool) {
	extended, ok := host.(AutomationHistoryHost)
	if !ok || extended.AutomationHistory() == nil {
		return nil, false
	}
	return extended.AutomationHistory(), true
}

type grpcAutomationHistoryReader struct{ client pluginv1.HostClient }

func (h *grpcHostClient) AutomationHistory() AutomationHistoryReader {
	return grpcAutomationHistoryReader{client: h.client}
}

func (r grpcAutomationHistoryReader) ListAutomations(ctx context.Context, q AutomationDefinitionQuery) (AutomationDefinitionPage, error) {
	response, err := r.client.ListAutomationDefinitionsExact(ctx, &pluginv1.ListAutomationDefinitionsExactRequest{
		RequestId: q.RequestID, WorkspaceId: q.WorkspaceID,
		Filter: automationDefinitionFilterToProto(q.Filter), Page: exactReadPageToProto(q.Page),
	})
	if err != nil {
		return AutomationDefinitionPage{}, err
	}
	items := make([]AutomationDefinitionObservation, len(response.GetItems()))
	for i, item := range response.GetItems() {
		items[i] = automationDefinitionObservationFromProto(item)
	}
	return AutomationDefinitionPage{Items: items, PageInfo: exactReadPageInfoFromProto(response.GetPageInfo())}, nil
}

func (r grpcAutomationHistoryReader) ListRuns(ctx context.Context, q AutomationRunQuery) (AutomationRunPage, error) {
	response, err := r.client.ListAutomationRunsExact(ctx, &pluginv1.ListAutomationRunsExactRequest{
		RequestId: q.RequestID, WorkspaceId: q.WorkspaceID,
		Filter: automationRunFilterToProto(q.Filter), Page: exactReadPageToProto(q.Page),
	})
	if err != nil {
		return AutomationRunPage{}, err
	}
	items := make([]AutomationRunObservation, len(response.GetItems()))
	for i, item := range response.GetItems() {
		items[i] = automationRunObservationFromProto(item)
	}
	return AutomationRunPage{Items: items, PageInfo: exactReadPageInfoFromProto(response.GetPageInfo())}, nil
}

func (r grpcAutomationHistoryReader) GetRun(ctx context.Context, q AutomationRunGetQuery) (AutomationRunObservation, HostReadReceipt, error) {
	response, err := r.client.GetAutomationRunExact(ctx, &pluginv1.GetAutomationRunExactRequest{
		RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, RunId: q.RunID,
	})
	if err != nil {
		return AutomationRunObservation{}, HostReadReceipt{}, err
	}
	return automationRunObservationFromProto(response.GetItem()), hostReadReceiptFromProto(response.GetReceipt()), nil
}

func automationDefinitionFilterToProto(filter AutomationDefinitionFilter) *pluginv1.AutomationDefinitionFilter {
	return &pluginv1.AutomationDefinitionFilter{
		AutomationId: filter.AutomationID, CreatedFrom: filter.CreatedFrom, CreatedBefore: filter.CreatedBefore,
	}
}

func automationDefinitionFilterFromProto(filter *pluginv1.AutomationDefinitionFilter) AutomationDefinitionFilter {
	if filter == nil {
		return AutomationDefinitionFilter{}
	}
	return AutomationDefinitionFilter{
		AutomationID: filter.GetAutomationId(), CreatedFrom: filter.CreatedFrom, CreatedBefore: filter.CreatedBefore,
	}
}

func automationRunFilterToProto(filter AutomationRunFilter) *pluginv1.AutomationRunFilter {
	return &pluginv1.AutomationRunFilter{
		AutomationId: filter.AutomationID, CreatedFrom: filter.CreatedFrom, CreatedBefore: filter.CreatedBefore,
	}
}

func automationRunFilterFromProto(filter *pluginv1.AutomationRunFilter) AutomationRunFilter {
	if filter == nil {
		return AutomationRunFilter{}
	}
	return AutomationRunFilter{
		AutomationID: filter.GetAutomationId(), CreatedFrom: filter.CreatedFrom, CreatedBefore: filter.CreatedBefore,
	}
}

func automationDefinitionObservationToProto(value AutomationDefinitionObservation) *pluginv1.AutomationDefinitionObservation {
	result := &pluginv1.AutomationDefinitionObservation{
		Id: value.ID, WorkspaceId: value.WorkspaceID, Name: value.Name, Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		ResourceVersion: value.ResourceVersion, ObservedAt: value.ObservedAt,
	}
	for _, trigger := range value.Triggers {
		result.Triggers = append(result.Triggers, &pluginv1.AutomationTriggerObservation{
			Id: trigger.ID, Kind: trigger.Kind, Enabled: trigger.Enabled,
			CronExpression: trigger.CronExpression, Timezone: trigger.Timezone,
			ScheduleAvailable: trigger.ScheduleAvailable,
		})
	}
	return result
}

func automationDefinitionObservationFromProto(value *pluginv1.AutomationDefinitionObservation) AutomationDefinitionObservation {
	if value == nil {
		return AutomationDefinitionObservation{}
	}
	result := AutomationDefinitionObservation{
		ID: value.GetId(), WorkspaceID: value.GetWorkspaceId(), Name: value.GetName(), Enabled: value.GetEnabled(),
		CreatedAt: value.GetCreatedAt(), UpdatedAt: value.GetUpdatedAt(),
		ResourceVersion: value.GetResourceVersion(), ObservedAt: value.GetObservedAt(),
		Triggers: make([]AutomationTriggerObservation, len(value.GetTriggers())),
	}
	for i, trigger := range value.GetTriggers() {
		result.Triggers[i] = AutomationTriggerObservation{
			ID: trigger.GetId(), Kind: trigger.GetKind(), Enabled: trigger.GetEnabled(),
			CronExpression: trigger.GetCronExpression(), Timezone: trigger.Timezone,
			ScheduleAvailable: trigger.GetScheduleAvailable(),
		}
	}
	return result
}

func automationRunObservationToProto(value AutomationRunObservation) *pluginv1.AutomationRunObservation {
	return &pluginv1.AutomationRunObservation{
		Id: value.ID, WorkspaceId: value.WorkspaceID, AutomationId: value.AutomationID,
		AutomationName: value.AutomationName, TriggerId: value.TriggerID, TriggerKind: value.TriggerKind,
		TaskId: value.TaskID, SessionId: value.SessionID, TurnId: value.TurnID,
		ManagedConversationId: value.ManagedConversationID,
		SourceStatus:          value.SourceStatus, Status: value.Status,
		RetryState: value.RetryState, DeliveryStatus: value.DeliveryStatus,
		RetryGroupId: value.RetryGroupID, RetryParentRunId: value.RetryParentRunID,
		AttemptNumber: value.AttemptNumber, CreatedAt: value.CreatedAt,
		RetryScheduledAt: value.RetryScheduledAt, RetryClaimedAt: value.RetryClaimedAt,
		Summary: value.Summary, ErrorExcerpt: value.ErrorExcerpt,
		ResourceVersion: value.ResourceVersion, ObservedAt: value.ObservedAt,
	}
}

func automationRunObservationFromProto(value *pluginv1.AutomationRunObservation) AutomationRunObservation {
	if value == nil {
		return AutomationRunObservation{}
	}
	return AutomationRunObservation{
		ID: value.GetId(), WorkspaceID: value.GetWorkspaceId(), AutomationID: value.GetAutomationId(),
		AutomationName: value.GetAutomationName(), TriggerID: value.GetTriggerId(), TriggerKind: value.GetTriggerKind(),
		TaskID: value.GetTaskId(), SessionID: value.GetSessionId(), TurnID: value.GetTurnId(),
		ManagedConversationID: value.GetManagedConversationId(),
		SourceStatus:          value.GetSourceStatus(), Status: value.GetStatus(),
		RetryState: value.GetRetryState(), DeliveryStatus: value.GetDeliveryStatus(),
		RetryGroupID: value.GetRetryGroupId(), RetryParentRunID: value.GetRetryParentRunId(),
		AttemptNumber: value.AttemptNumber, CreatedAt: value.GetCreatedAt(),
		RetryScheduledAt: value.RetryScheduledAt, RetryClaimedAt: value.RetryClaimedAt,
		Summary: value.GetSummary(), ErrorExcerpt: value.GetErrorExcerpt(),
		ResourceVersion: value.GetResourceVersion(), ObservedAt: value.GetObservedAt(),
	}
}
