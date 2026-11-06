package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
)

func normalizeRetainedNativeUsage(native usagehistory.NativeUsage, scope trustedUsageScope, binding retainedNativeUsageBinding) (models.SessionUsageMeasurement, error) {
	collectedAt := native.CollectedAt.UTC()
	if collectedAt.IsZero() {
		collectedAt = native.OccurredAt.UTC()
	}
	occurredAt := native.OccurredAt.UTC()
	if occurredAt.IsZero() {
		return models.SessionUsageMeasurement{}, errors.New("native usage occurrence time is required")
	}
	endAt := occurredAt.Add(time.Nanosecond)
	input := native.InputTokens
	output := cloneInt64(native.OutputTokens)
	cacheRead := cloneInt64(native.CacheReadTokens)
	cacheWrite := cloneInt64(native.CacheWriteTokens)
	reasoning := cloneInt64(native.ThoughtTokens)
	if native.CostPresent && native.CostSubcents < 0 {
		return models.SessionUsageMeasurement{}, errors.New("native usage cost cannot be negative")
	}
	if native.TotalTokens < 0 || input < 0 || !nativeValuesNonNegative(output, cacheRead, cacheWrite, reasoning,
		native.ReasoningOutputTokens, native.ReportedTotalTokens, native.ReportedCacheWriteTokens) {
		return models.SessionUsageMeasurement{}, errors.New("native token values cannot be negative")
	}
	if err := tokenValuesFitTotal(input, output, cacheRead, cacheWrite, reasoning, native.ReasoningOutputTokens, native.TotalTokens); err != nil {
		return models.SessionUsageMeasurement{}, err
	}
	total := native.TotalTokens
	cost, costBasis, costCoverage := nativeUsageCostFields(native)
	return models.SessionUsageMeasurement{
		ID:          uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev-native-usage:"+native.UsageEventID)).String(),
		WorkspaceID: scope.WorkspaceID, TaskID: native.TaskID, SessionID: binding.SessionID,
		ScopeID: binding.ScopeID, SubjectID: binding.SubjectID, UsageBindingID: native.BindingID,
		ActorID: binding.ActorID, ActorStatus: binding.ActorStatus, ActorKind: binding.ActorKind,
		TranscriptID: native.ProviderThreadID,
		Source:       usageSourceNative, SourceRecordID: native.UsageEventID, UsageIdentity: "native:" + native.UsageEventID,
		Model: native.Model, Provider: native.Provider, AgentClient: native.AgentClient,
		SourceVersion: strconv.Itoa(native.ContractVersion), Revision: 0,
		ObservedAt: occurredAt, CollectedAt: collectedAt,
		InputTokens: int64Pointer(input), OutputTokens: output, CacheReadTokens: cacheRead,
		CacheWriteTokens: cacheWrite, ReasoningTokens: reasoning, TotalTokens: &total,
		SourceTotalTokens: cloneInt64(native.ReportedTotalTokens), ReportedCacheWriteTokens: cloneInt64(native.ReportedCacheWriteTokens),
		ReasoningOutputTokens: cloneInt64(native.ReasoningOutputTokens),
		TokenSemanticsVersion: native.TokenSemanticsVersion,
		ReasoningInsideOutput: native.ReasoningInsideOutput, CacheReadInsideInput: native.CacheReadInsideInput,
		CostSubcents: cost, Currency: "USD", CostBasis: costBasis,
		CostCoverage: costCoverage, Coverage: nativeUsageCoverage(native.UsageCompleteness), AttributionStatus: models.UsageAttributionAttributed,
		CoverageStart: &occurredAt, CoverageEnd: &endAt, SourceTimezone: "UTC",
		SourceDate: occurredAt.Format("2006-01-02"), CoverageKey: occurredAt.Format("2006-01-02"),
		Estimated:   native.Estimated,
		NativeScope: native.NativeScope, MeasurementSource: native.MeasurementSource,
		UsageCompleteness: native.UsageCompleteness, UsageSchemaVersion: native.UsageSchemaVersion,
		PricingCatalogVersion: native.PricingCatalogVersion,
	}, nil
}

func nativeUsageCostFields(native usagehistory.NativeUsage) (*int64, string, string) {
	if !native.CostPresent {
		return nil, models.UsageCostBasisUnknown, models.UsageCoverageMissing
	}
	costBasis := native.CostBasis
	switch native.CostSource {
	case usageCostSourceReported:
		costBasis = models.UsageCostBasisReported
	case usageCostSourceEstimated:
		costBasis = models.UsageCostBasisEstimated
	}
	switch costBasis {
	case models.UsageCostBasisReported, models.UsageCostBasisEstimated:
		return &native.CostSubcents, costBasis, models.UsageCoverageComplete
	default:
		return nil, models.UsageCostBasisUnknown, models.UsageCoverageMissing
	}
}

func nativeUsageCoverage(completeness string) string {
	if completeness == "" || strings.EqualFold(completeness, models.UsageCoveragePartial) || strings.EqualFold(completeness, "incomplete") {
		return models.UsageCoveragePartial
	}
	return models.UsageCoverageComplete
}

func (r *Repository) insertRetainedNativeUsageTx(ctx context.Context, tx *sqlx.Tx, measurement models.SessionUsageMeasurement) error {
	inserted, err := r.insertUsageTx(ctx, tx, measurement)
	if err != nil {
		return fmt.Errorf("insert retained native usage: %w", err)
	}
	if inserted == 0 {
		stored, found, readErr := r.findUsageTx(ctx, tx, measurement)
		if readErr != nil {
			return readErr
		}
		if !found {
			return fmt.Errorf("native usage %q conflicted with an existing identity", measurement.SourceRecordID)
		}
		if stored.PayloadDigest != measurement.PayloadDigest {
			return fmt.Errorf("native usage %q conflicts with a different retained observation", measurement.SourceRecordID)
		}
	}
	if inserted > 0 {
		if err := r.markNativeUsageAccountingCurrentTx(ctx, tx, usageBucketsTable, measurement.ID); err != nil {
			return err
		}
		indexed, err := r.loadIndexedObservationTx(ctx, tx, usageBucketsTable, measurement.ID)
		if err != nil {
			return err
		}
		if _, err := r.recordUsageMutationTx(ctx, tx, "observation", nil, &indexed); err != nil {
			return err
		}
	}
	return nil
}
