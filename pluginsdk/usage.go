package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

// SessionUsageMeasurement is the public Host representation of one source
// observation. Optional numeric pointers preserve unknown values.
type SessionUsageMeasurement struct {
	ID                string
	WorkspaceID       string
	TaskID            string
	SessionID         string
	TranscriptID      string
	SourceRecordID    string
	UsageIdentity     string
	Model             string
	Provider          string
	SourceVersion     string
	Revision          int64
	PayloadDigest     string
	ObservedAt        string
	CollectedAt       string
	InputTokens       *int64
	OutputTokens      *int64
	CacheReadTokens   *int64
	CacheWriteTokens  *int64
	ReasoningTokens   *int64
	TotalTokens       *int64
	Turns             *int64
	CostSubcents      *int64
	Currency          string
	CostBasis         string
	CostCoverage      string
	Coverage          string
	SourceTimezone    string
	AttributionStatus string
	CoverageStart     *string
	CoverageEnd       *string
	SourceDate        string
	CoverageKey       string
	Estimated         bool
	Stale             bool
}

type SessionUsageWriteResult struct {
	SourceRecordID string
	UsageIdentity  string
	Model          string
	Provider       string
	Status         string
	Measurement    *SessionUsageMeasurement
	Error          string
}

type SessionUsageFilter struct {
	WorkspaceID    string
	SessionIDs     []string
	TaskIDs        []string
	Model          string
	Provider       string
	Start          *string
	End            *string
	Timezone       string
	GroupBy        string
	SortBy         string
	SortDirection  string
	IncludeUndated bool
	// Canonical requests the authorized merged view, including native and
	// other-source observations. It is read-only and has no effect on writes.
	Canonical bool
}

type UsageReader interface {
	UpsertBatch(ctx context.Context, workspaceID string, measurements []SessionUsageMeasurement) ([]SessionUsageWriteResult, error)
	// List returns observations owned by the connected plugin. The host derives
	// the source namespace from the plugin identity; callers cannot widen it
	// through the filter.
	List(ctx context.Context, filter SessionUsageFilter, page Page) ([]SessionUsageMeasurement, *PageInfo, error)
	// ListCanonical returns the host-selected value for each coverage unit. It
	// is a separate read because its result can include native and other-source
	// observations while writes remain source-owned.
	ListCanonical(ctx context.Context, filter SessionUsageFilter, page Page) ([]SessionUsageMeasurement, *PageInfo, error)
}

func (m SessionUsageMeasurement) toProto() *pluginv1.SessionUsageMeasurement {
	return &pluginv1.SessionUsageMeasurement{
		Id: m.ID, WorkspaceId: m.WorkspaceID, TaskId: m.TaskID, SessionId: m.SessionID,
		TranscriptId: m.TranscriptID, SourceRecordId: m.SourceRecordID, UsageIdentity: m.UsageIdentity,
		Model: m.Model, Provider: m.Provider, SourceVersion: m.SourceVersion, Revision: m.Revision,
		PayloadDigest: m.PayloadDigest, ObservedAt: m.ObservedAt, CollectedAt: m.CollectedAt,
		InputTokens: m.InputTokens, OutputTokens: m.OutputTokens, CacheReadTokens: m.CacheReadTokens,
		CacheWriteTokens: m.CacheWriteTokens, ReasoningTokens: m.ReasoningTokens, TotalTokens: m.TotalTokens,
		Turns: m.Turns, CostSubcents: m.CostSubcents, Currency: m.Currency, CostBasis: m.CostBasis, CostCoverage: m.CostCoverage, Coverage: m.Coverage,
		SourceTimezone: m.SourceTimezone, AttributionStatus: m.AttributionStatus,
		CoverageStart: m.CoverageStart, CoverageEnd: m.CoverageEnd, SourceDate: m.SourceDate,
		CoverageKey: m.CoverageKey, Estimated: m.Estimated, Stale: m.Stale,
	}
}

func sessionUsageMeasurementFromProto(p *pluginv1.SessionUsageMeasurement) SessionUsageMeasurement {
	if p == nil {
		return SessionUsageMeasurement{}
	}
	return SessionUsageMeasurement{
		ID: p.GetId(), WorkspaceID: p.GetWorkspaceId(), TaskID: p.GetTaskId(), SessionID: p.GetSessionId(),
		TranscriptID: p.GetTranscriptId(), SourceRecordID: p.GetSourceRecordId(), UsageIdentity: p.GetUsageIdentity(),
		Model: p.GetModel(), Provider: p.GetProvider(), SourceVersion: p.GetSourceVersion(), Revision: p.GetRevision(),
		PayloadDigest: p.GetPayloadDigest(), ObservedAt: p.GetObservedAt(), CollectedAt: p.GetCollectedAt(),
		InputTokens: p.InputTokens, OutputTokens: p.OutputTokens, CacheReadTokens: p.CacheReadTokens,
		CacheWriteTokens: p.CacheWriteTokens, ReasoningTokens: p.ReasoningTokens, TotalTokens: p.TotalTokens,
		Turns: p.Turns, CostSubcents: p.CostSubcents, Currency: p.GetCurrency(), CostBasis: p.GetCostBasis(), CostCoverage: p.GetCostCoverage(), Coverage: p.GetCoverage(),
		SourceTimezone: p.GetSourceTimezone(), AttributionStatus: p.GetAttributionStatus(),
		CoverageStart: p.CoverageStart, CoverageEnd: p.CoverageEnd, SourceDate: p.GetSourceDate(),
		CoverageKey: p.GetCoverageKey(), Estimated: p.GetEstimated(), Stale: p.GetStale(),
	}
}

func sessionUsageMeasurementsFromProto(items []*pluginv1.SessionUsageMeasurement) []SessionUsageMeasurement {
	if len(items) == 0 {
		return []SessionUsageMeasurement{}
	}
	out := make([]SessionUsageMeasurement, len(items))
	for i, item := range items {
		out[i] = sessionUsageMeasurementFromProto(item)
	}
	return out
}

func (f SessionUsageFilter) toProto() *pluginv1.SessionUsageFilter {
	return &pluginv1.SessionUsageFilter{
		WorkspaceId: f.WorkspaceID, SessionIds: f.SessionIDs, TaskIds: f.TaskIDs,
		Model: f.Model, Provider: f.Provider, Start: f.Start, End: f.End,
		Timezone: f.Timezone, GroupBy: f.GroupBy, SortBy: f.SortBy, SortDirection: f.SortDirection,
		IncludeUndated: f.IncludeUndated, Canonical: f.Canonical,
	}
}

func sessionUsageFilterFromProto(p *pluginv1.SessionUsageFilter) SessionUsageFilter {
	if p == nil {
		return SessionUsageFilter{}
	}
	return SessionUsageFilter{
		WorkspaceID: p.GetWorkspaceId(), SessionIDs: p.GetSessionIds(), TaskIDs: p.GetTaskIds(),
		Model: p.GetModel(), Provider: p.GetProvider(), Start: p.Start, End: p.End,
		Timezone: p.GetTimezone(), GroupBy: p.GetGroupBy(), SortBy: p.GetSortBy(),
		SortDirection: p.GetSortDirection(), IncludeUndated: p.GetIncludeUndated(), Canonical: p.GetCanonical(),
	}
}
