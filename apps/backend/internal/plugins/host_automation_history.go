package plugins

import (
	"context"
	"errors"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	automationHistoryListDefinitionsMethod = "ListAutomationDefinitionsExact"
	automationHistoryListRunsMethod        = "ListAutomationRunsExact"
	automationHistoryGetRunMethod          = "GetAutomationRunExact"
	automationHistoryCapability            = "host.v2.read:automations"
	// automationHistorySourceLimit is the shared snapshot row bound plus one,
	// so an oversized filtered result is detected before a partial page is
	// returned as complete.
	automationHistorySourceLimit = exactSnapshotMaxRows + 1
)

type pluginHostAutomationHistory struct{ host *pluginHost }

type normalizedAutomationHistoryFilter struct {
	automationID  string
	createdFrom   *time.Time
	createdBefore *time.Time
}

type automationHistoryFilterDigest struct {
	AutomationID  string `json:"automation_id,omitempty"`
	CreatedFrom   string `json:"created_from,omitempty"`
	CreatedBefore string `json:"created_before,omitempty"`
}

func (h *pluginHost) AutomationHistory() pluginsdk.AutomationHistoryReader {
	return pluginHostAutomationHistory{host: h}
}

func (h *pluginHost) automationHistoryDataSource() (AutomationHistoryDataSource, error) {
	if h.automationHistoryDataDep == nil {
		return nil, status.Error(codes.Unavailable, "automation history source is unavailable")
	}
	source := h.automationHistoryDataDep()
	if source == nil {
		return nil, status.Error(codes.Unavailable, "automation history source is unavailable")
	}
	return source, nil
}

func (r pluginHostAutomationHistory) ListAutomations(ctx context.Context, q pluginsdk.AutomationDefinitionQuery) (pluginsdk.AutomationDefinitionPage, error) {
	items, info, err := listAutomationHistoryPage(ctx, r.host, q.RequestID, q.WorkspaceID,
		automationHistoryListDefinitionsMethod, q.Filter.AutomationID, q.Filter.CreatedFrom, q.Filter.CreatedBefore, q.Page,
		func(ctx context.Context, source AutomationHistoryDataSource, filter normalizedAutomationHistoryFilter) ([]pluginsdk.AutomationDefinitionObservation, bool, error) {
			result, err := source.ListAutomationHistoryDefinitions(ctx, q.WorkspaceID, filter.automationID,
				filter.createdFrom, filter.createdBefore, automationHistorySourceLimit)
			if err != nil {
				return nil, false, err
			}
			return automationDefinitionObservations(result.Items), result.Overflow, nil
		})
	return pluginsdk.AutomationDefinitionPage{Items: items, PageInfo: info}, err
}

func (r pluginHostAutomationHistory) ListRuns(ctx context.Context, q pluginsdk.AutomationRunQuery) (pluginsdk.AutomationRunPage, error) {
	items, info, err := listAutomationHistoryPage(ctx, r.host, q.RequestID, q.WorkspaceID,
		automationHistoryListRunsMethod, q.Filter.AutomationID, q.Filter.CreatedFrom, q.Filter.CreatedBefore, q.Page,
		func(ctx context.Context, source AutomationHistoryDataSource, filter normalizedAutomationHistoryFilter) ([]pluginsdk.AutomationRunObservation, bool, error) {
			result, err := source.ListAutomationHistoryRuns(ctx, q.WorkspaceID, filter.automationID,
				filter.createdFrom, filter.createdBefore, automationHistorySourceLimit)
			if err != nil {
				return nil, false, err
			}
			return automationRunObservations(result.Items), result.Overflow, nil
		})
	return pluginsdk.AutomationRunPage{Items: items, PageInfo: info}, err
}

func listAutomationHistoryPage[T any](
	ctx context.Context,
	host *pluginHost,
	requestID, workspaceID, method, automationID string,
	createdFrom, createdBefore *string,
	page pluginsdk.ExactReadPage,
	load func(context.Context, AutomationHistoryDataSource, normalizedAutomationHistoryFilter) ([]T, bool, error),
) ([]T, pluginsdk.ExactReadPageInfo, error) {
	filter, err := normalizeAutomationHistoryFilter(automationID, createdFrom, createdBefore)
	if err != nil {
		return nil, pluginsdk.ExactReadPageInfo{}, err
	}
	identity, unlock, err := host.exactReadAdmission(ctx, requestID, workspaceID, method,
		automationHistoryCapability, filter.digest())
	if err != nil {
		return nil, pluginsdk.ExactReadPageInfo{}, err
	}
	defer unlock()
	source, err := host.automationHistoryDataSource()
	if err != nil {
		return nil, pluginsdk.ExactReadPageInfo{}, err
	}
	return exactPage(host, identity, requestID, page, func() ([]T, error) {
		items, overflow, err := load(ctx, source, filter)
		if err != nil {
			return nil, automationHistorySourceError(err)
		}
		if overflow {
			return nil, automationHistoryExhaustedError()
		}
		return items, nil
	})
}

func (r pluginHostAutomationHistory) GetRun(ctx context.Context, q pluginsdk.AutomationRunGetQuery) (pluginsdk.AutomationRunObservation, pluginsdk.HostReadReceipt, error) {
	if !isBoundedApprovalIdentifier(q.RunID) {
		return pluginsdk.AutomationRunObservation{}, pluginsdk.HostReadReceipt{}, status.Error(codes.InvalidArgument, "run_id is required")
	}
	identity, unlock, err := r.host.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID,
		automationHistoryGetRunMethod, automationHistoryCapability, map[string]string{"run_id": q.RunID})
	if err != nil {
		return pluginsdk.AutomationRunObservation{}, pluginsdk.HostReadReceipt{}, err
	}
	defer unlock()
	source, err := r.host.automationHistoryDataSource()
	if err != nil {
		return pluginsdk.AutomationRunObservation{}, pluginsdk.HostReadReceipt{}, err
	}
	item, err := source.GetAutomationHistoryRun(ctx, q.WorkspaceID, q.RunID)
	if err != nil {
		return pluginsdk.AutomationRunObservation{}, pluginsdk.HostReadReceipt{}, automationHistorySourceError(err)
	}
	return automationRunObservation(item, time.Now().UTC().Format(time.RFC3339Nano)), singleReadReceipt(identity, q.RequestID), nil
}

func normalizeAutomationHistoryFilter(automationID string, createdFrom, createdBefore *string) (normalizedAutomationHistoryFilter, error) {
	filter := normalizedAutomationHistoryFilter{automationID: automationID}
	if automationID != "" && !isBoundedApprovalIdentifier(automationID) {
		return filter, status.Error(codes.InvalidArgument, "automation_id is invalid")
	}
	if createdFrom != nil {
		parsed, err := time.Parse(time.RFC3339Nano, *createdFrom)
		if err != nil {
			return filter, status.Error(codes.InvalidArgument, "created_from is not a valid RFC3339 timestamp")
		}
		utc := parsed.UTC()
		filter.createdFrom = &utc
	}
	if createdBefore != nil {
		parsed, err := time.Parse(time.RFC3339Nano, *createdBefore)
		if err != nil {
			return filter, status.Error(codes.InvalidArgument, "created_before is not a valid RFC3339 timestamp")
		}
		utc := parsed.UTC()
		filter.createdBefore = &utc
	}
	if filter.createdFrom != nil && filter.createdBefore != nil && !filter.createdFrom.Before(*filter.createdBefore) {
		return filter, status.Error(codes.InvalidArgument, "created_from must be before created_before")
	}
	return filter, nil
}

func (f normalizedAutomationHistoryFilter) digest() automationHistoryFilterDigest {
	digest := automationHistoryFilterDigest{AutomationID: f.automationID}
	if f.createdFrom != nil {
		digest.CreatedFrom = f.createdFrom.Format(time.RFC3339Nano)
	}
	if f.createdBefore != nil {
		digest.CreatedBefore = f.createdBefore.Format(time.RFC3339Nano)
	}
	return digest
}

func automationHistorySourceError(err error) error {
	if errors.Is(err, ErrAutomationHistoryNotFound) {
		return status.Error(codes.NotFound, "automation history target was not found")
	}
	if status.Code(err) != codes.Unknown {
		return err
	}
	return status.Error(codes.Unavailable, "automation history source is unavailable")
}

func automationHistoryExhaustedError() error {
	return status.Error(codes.ResourceExhausted, "automation history snapshot exceeds its bounded size")
}

func automationDefinitionObservations(items []AutomationHistoryDefinition) []pluginsdk.AutomationDefinitionObservation {
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	observations := make([]pluginsdk.AutomationDefinitionObservation, 0, len(items))
	for _, item := range items {
		observations = append(observations, automationDefinitionObservation(item, observedAt))
	}
	return observations
}

func automationRunObservations(items []AutomationHistoryRun) []pluginsdk.AutomationRunObservation {
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	observations := make([]pluginsdk.AutomationRunObservation, 0, len(items))
	for _, item := range items {
		observations = append(observations, automationRunObservation(item, observedAt))
	}
	return observations
}

func automationDefinitionObservation(item AutomationHistoryDefinition, observedAt string) pluginsdk.AutomationDefinitionObservation {
	observation := pluginsdk.AutomationDefinitionObservation{
		ID: item.ID, WorkspaceID: item.WorkspaceID, Name: item.Name, Enabled: item.Enabled,
		CreatedAt:  item.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:  item.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ObservedAt: observedAt,
		Triggers:   make([]pluginsdk.AutomationTriggerObservation, 0, len(item.Triggers)),
	}
	for _, trigger := range item.Triggers {
		wire := pluginsdk.AutomationTriggerObservation{
			ID: trigger.ID, Kind: trigger.Kind, Enabled: trigger.Enabled,
			CronExpression: trigger.CronExpression, ScheduleAvailable: trigger.ScheduleAvailable,
		}
		if trigger.Timezone != "" {
			timezone := trigger.Timezone
			wire.Timezone = &timezone
		}
		observation.Triggers = append(observation.Triggers, wire)
	}
	observation.ResourceVersion = item.ResourceVersion
	if observation.ResourceVersion == "" {
		observation.ResourceVersion = digestPublicValue(observation)
	}
	return observation
}

func automationRunObservation(item AutomationHistoryRun, observedAt string) pluginsdk.AutomationRunObservation {
	observation := pluginsdk.AutomationRunObservation{
		ID: item.ID, WorkspaceID: item.WorkspaceID, AutomationID: item.AutomationID,
		AutomationName: item.AutomationName, TriggerID: item.TriggerID, TriggerKind: item.TriggerKind,
		TaskID: item.TaskID, SessionID: item.SessionID, TurnID: item.TurnID,
		ManagedConversationID: item.ManagedConversationID,
		SourceStatus:          item.SourceStatus, Status: item.Status,
		RetryState: item.RetryState, DeliveryStatus: item.DeliveryStatus,
		RetryGroupID: item.RetryGroupID, RetryParentRunID: item.RetryParentRunID,
		AttemptNumber: item.AttemptNumber,
		CreatedAt:     item.CreatedAt.UTC().Format(time.RFC3339Nano),
		Summary:       item.Summary, ErrorExcerpt: item.ErrorExcerpt,
		ObservedAt: observedAt,
	}
	if item.RetryScheduledAt != nil {
		formatted := item.RetryScheduledAt.UTC().Format(time.RFC3339Nano)
		observation.RetryScheduledAt = &formatted
	}
	if item.RetryClaimedAt != nil {
		formatted := item.RetryClaimedAt.UTC().Format(time.RFC3339Nano)
		observation.RetryClaimedAt = &formatted
	}
	observation.ResourceVersion = item.ResourceVersion
	if observation.ResourceVersion == "" {
		observation.ResourceVersion = digestPublicValue(observation)
	}
	return observation
}
