//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
)

// Capture goes through CreateTaskSession and the exact public target lookup.
// Neither a current task row nor a hand-written eligible flag is birth proof.
func (f *usageBenchFixture) collectionTarget(t testing.TB, subject usageBenchSubject, source string) retainedTarget {
	t.Helper()
	key := source + ":" + subject.subject
	if target, ok := f.targets[key]; ok {
		return target
	}
	page, err := f.repo.ListUsageCollectionTargets(t.Context(), strings.TrimPrefix(source, "plugin:"), pluginsdk.UsageCollectionTargetQueryV1{
		WorkspaceID: subject.workspace, CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: subject.session}, Limit: 1,
	})
	usageBenchMust(t, err)
	if !page.CurrentTarget || page.Preparing || page.HasMore || page.SnapshotID != "" || page.NextCursor != "" || len(page.Targets) != 1 || page.Targets[0].SourceScopeStatus != usageSourceEligible {
		t.Fatal("benchmark needs one current target with genuine immutable source eligibility")
	}
	var target retainedTarget
	usageBenchMust(t, f.database.Get(&target, f.database.Rebind(`SELECT * FROM usage_collection_bindings WHERE binding_id=?`), page.Targets[0].SubjectBinding))
	if target.SubjectID != subject.subject || target.ScopeID != subject.scope || target.ExternalSessionID != subject.transcript {
		t.Fatal("benchmark target does not match its captured scope, incarnation and producer thread")
	}
	f.targets[key] = target
	return target
}

func usageBenchNative(m models.SessionUsageMeasurement) usagehistory.NativeUsage {
	return usagehistory.NativeUsage{
		BindingID: m.UsageBindingID, UsageEventID: m.SourceRecordID, TaskID: m.TaskID, SessionID: m.SessionID,
		AgentClient: m.AgentClient, Model: m.Model, Provider: m.Provider, ProviderThreadID: m.TranscriptID,
		NativeScope: "response", MeasurementSource: "provider_usage", UsageCompleteness: m.Coverage,
		UsageSchemaVersion: 1, ContractVersion: 1, InputTokens: *m.InputTokens, OutputTokens: m.OutputTokens,
		ThoughtTokens: m.ReasoningTokens, CacheReadTokens: m.CacheReadTokens, CacheWriteTokens: m.CacheWriteTokens,
		ReportedTotalTokens: m.SourceTotalTokens, TotalTokens: *m.TotalTokens,
		CostPresent: m.CostSubcents != nil, CostSubcents: valueOrZero(m.CostSubcents), CostBasis: m.CostBasis,
		Estimated: m.Estimated, TokenSemanticsVersion: m.TokenSemanticsVersion,
		OccurredAt: *m.CoverageStart, CollectedAt: m.CollectedAt,
	}
}

// This producer explicitly declares a single meter stream. Different exact
// point ranges are additive; arbitrary model/provider labels do not prove that.
func usageBenchObservation(m models.SessionUsageMeasurement, binding string) pluginsdk.UsageObservationV1 {
	input := pluginsdk.UsageObservationV1{
		SubjectBinding: binding, ItemID: m.SourceRecordID, Revision: max(m.Revision, 1), SourceRecordID: m.SourceRecordID,
		SourceVersion: m.SourceVersion, ModelID: &m.Model, ModelProvider: &m.Provider, AgentClient: &m.AgentClient,
		Segment: &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "query-fixture-meter-v1", SegmentID: "meter"},
		Tokens: &pluginsdk.UsageTokenAmountsV1{InputTokens: m.InputTokens, OutputTokens: m.OutputTokens,
			CacheReadTokens: m.CacheReadTokens, CacheWriteTokens: m.CacheWriteTokens, ReasoningTokens: m.ReasoningTokens, TotalTokens: m.TotalTokens},
		Cost:    &pluginsdk.UsageCostV1{Amount: m.CostSubcents, Currency: &m.Currency, Basis: pluginsdk.UsageCostBasisV1(m.CostBasis)},
		Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: m.Coverage == usageCoverageComplete, CostCoverageComplete: m.CostCoverage == usageCoverageComplete, Stale: m.Stale},
	}
	if m.CostSubcents == nil {
		input.Cost.Basis = pluginsdk.UsageCostBasisUnknown
	}
	if m.CoverageStart != nil {
		if m.CoverageEnd.Sub(*m.CoverageStart) <= time.Microsecond {
			input.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: nonemptyPtr(m.CoverageStart.Format(time.RFC3339Nano))}
		} else {
			input.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: nonemptyPtr(m.CoverageStart.Format(time.RFC3339Nano)), CoverageEnd: nonemptyPtr(m.CoverageEnd.Format(time.RFC3339Nano)), SourceDate: nonemptyPtr(m.SourceDate), SourceTimezone: nonemptyPtr(m.SourceTimezone)}
		}
	}
	return input
}

func usageBenchSeal(t testing.TB, input *pluginsdk.UsageObservationV1) {
	t.Helper()
	var err error
	input.PayloadDigest, err = input.ComputePayloadDigestV1()
	usageBenchMust(t, err)
}

func (f *usageBenchFixture) seedRecord(t testing.TB, subject usageBenchSubject, m models.SessionUsageMeasurement) usageBenchSeedRecord {
	t.Helper()
	if m.Source == usageSourceNative {
		native := usageBenchNative(m)
		normalized, err := normalizeRetainedNativeUsage(native, subject.nativeScope, subject.nativeBinding)
		usageBenchMust(t, err)
		normalized.PayloadDigest = nativeUsageDigest(normalized)
		row := indexedObservation{Measurement: normalized, ItemID: native.UsageEventID, EventID: native.UsageEventID, WorkKey: subject.work, WorkStatus: usageIdentityProven, Table: usageBucketsTable}
		row.CoverageKey = "event:" + row.EventID + ":" + contractCoverageKey(normalized)
		setUsageObservationSegment(&row, nil)
		return usageBenchSeedRecord{observation: row, native: &native}
	}
	target := f.collectionTarget(t, subject, m.Source)
	input := usageBenchObservation(m, target.BindingID)
	usageBenchSeal(t, &input)
	normalized, err := validateContractObservation(input)
	usageBenchMust(t, err)
	normalized.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-observation-v1:"+m.Source+":"+input.ItemID)).String()
	normalized.Source, normalized.WorkspaceID = m.Source, target.WorkspaceID
	normalized.ScopeID, normalized.SubjectID, normalized.UsageBindingID = target.ScopeID, target.SubjectID, target.BindingID
	normalized.TaskID, normalized.SessionID, normalized.TranscriptID = target.TaskID, target.SessionID, target.ExternalSessionID
	normalized.UsageIdentity, normalized.CollectedAt = "v1:"+input.ItemID, m.CollectedAt
	work, status := contractWorkIdentity(target, m.Source, input.WorkIdentity)
	encoded, err := encodeObservationMetadata(input)
	usageBenchMust(t, err)
	row := indexedObservation{Measurement: normalized, Table: usageTableFor(normalized), ItemID: input.ItemID, WorkKey: work, WorkStatus: status, CoverageKey: contractCoverageKey(normalized), Metadata: string(encoded)}
	setUsageObservationSegment(&row, &input)
	return usageBenchSeedRecord{observation: row, input: &input}
}

func (f *usageBenchFixture) admitSeedBatch(t testing.TB, records []usageBenchSeedRecord) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.database.BeginTxx(ctx, nil)
	usageBenchMust(t, err)
	defer func() { _ = tx.Rollback() }()
	for _, record := range records {
		if record.native != nil {
			usageBenchMust(t, f.repo.RetainNativeUsageTx(ctx, tx, *record.native))
			continue
		}
		collector := strings.TrimPrefix(record.observation.Measurement.Source, "plugin:")
		result, _, err := f.repo.MutateUsageObservationsTx(ctx, tx, collector, pluginsdk.UsageObservationBatchV1{SchemaVersion: 2, Observations: []pluginsdk.UsageObservationV1{*record.input}})
		usageBenchMust(t, err)
		if result.Status != usageWriteAccepted || len(result.Results) != 1 || result.Results[0].Status != usageWriteAccepted {
			t.Fatalf("benchmark source admission was rejected: %+v", result.Results)
		}
	}
	usageBenchMust(t, tx.Commit())
}
