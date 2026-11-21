package sqlite

// This file preserves the retired legacy measurement upsert for fixture tests.
// Production admission goes through Report and the retained observation
// contract; these helpers exist only so earlier regression coverage keeps its
// original setup semantics.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/models"
)

//nolint:cyclop,gocognit,nestif,funlen // One transaction must classify every CAS outcome and preserve the accepted row.
func (r *Repository) admitLegacyUsageSourceTx(ctx context.Context, tx *sqlx.Tx, m *models.SessionUsageMeasurement) error {
	if m.SessionID == "" {
		return nil
	}
	var target retainedTarget
	if err := tx.GetContext(ctx, &target, r.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE subject_id = ? ORDER BY sequence DESC LIMIT 1`), m.SubjectID); err != nil {
		return err
	}
	if err := r.admitUsageSourceContextTx(ctx, tx, target, ""); err != nil {
		return err
	}
	return r.deriveExternalUsageAttributionTx(ctx, tx, target, "", m)
}

//nolint:nestif // Retired upsert fixture keeps its original conflict branches.
func (r *Repository) upsertSessionUsageFixture(ctx context.Context, records []models.SessionUsageMeasurement) ([]models.SessionUsageUpsertResult, error) {
	if len(records) == 0 {
		return []models.SessionUsageUpsertResult{}, nil
	}
	if len(records) > maxSessionUsageBatch {
		return nil, fmt.Errorf("%w: maximum batch size is %d", ErrInvalidSessionUsage, maxSessionUsageBatch)
	}
	r.usageWriteMu.Lock()
	defer r.usageWriteMu.Unlock()

	normalized := make([]models.SessionUsageMeasurement, len(records))
	workspaceID := ""
	seen := make(map[string]struct{}, len(records))
	for i, input := range records {
		item, err := normalizeUsage(input)
		if err != nil {
			return nil, fmt.Errorf("%w: record %d: %v", ErrInvalidSessionUsage, i, err)
		}
		if workspaceID == "" {
			workspaceID = item.WorkspaceID
		} else if workspaceID != item.WorkspaceID {
			return nil, fmt.Errorf("%w: all records must use one workspace", ErrInvalidSessionUsage)
		}
		key := usageKey(item)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate record %q", ErrInvalidSessionUsage, key)
		}
		seen[key] = struct{}{}
		normalized[i] = item
	}
	if err := r.validateUsageOwnership(ctx, normalized); err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	results := make([]models.SessionUsageUpsertResult, len(normalized))
	for i := range normalized {
		item := normalized[i]
		item, err = r.bindExternalUsageScopeTx(ctx, tx, item)
		if err != nil {
			return nil, fmt.Errorf("bind retained usage record %q: %w", item.SourceRecordID, err)
		}
		if err := r.AdmitUsageWriteTx(ctx, tx, item.TaskID, item.WorkspaceID, item.SessionID); err != nil {
			return nil, fmt.Errorf("admit retained usage record %q: %w", item.SourceRecordID, err)
		}
		if err := r.admitLegacyUsageSourceTx(ctx, tx, &item); err != nil {
			return nil, fmt.Errorf("admit aggregate source context: %w", err)
		}
		result := models.SessionUsageUpsertResult{
			SourceRecordID: item.SourceRecordID, UsageIdentity: item.UsageIdentity,
			Model: item.Model, Provider: item.Provider,
		}
		stored, found, err := r.findUsageTx(ctx, tx, item)
		if err != nil {
			return nil, err
		}
		var oldUsage *models.SessionUsageMeasurement
		var oldIndexed *indexedObservation
		if found {
			previous := stored.measurement()
			oldUsage = &previous
			indexed, indexErr := r.loadIndexedObservationTx(ctx, tx, usageTableFor(previous), previous.ID)
			if indexErr != nil {
				return nil, indexErr
			}
			oldIndexed = &indexed
			if stored.ScopeID != item.ScopeID || stored.SubjectID != item.SubjectID {
				return nil, fmt.Errorf("usage observation %q belongs to an immutable historical scope", item.SourceRecordID)
			}
		}
		if found && oldIndexed != nil && (oldIndexed.Retired != 0 || oldIndexed.Metadata != "") {
			result.Status = models.UsageWriteConflict
			result.Measurement = ptrMeasurement(stored.measurement())
			results[i] = result
			continue
		}
		if found {
			switch {
			case item.Revision < stored.Revision:
				result.Status = models.UsageWriteStale
				result.Measurement = ptrMeasurement(stored.measurement())
			case item.Revision == stored.Revision && item.PayloadDigest == stored.PayloadDigest:
				result.Status = models.UsageWriteUnchanged
				result.Measurement = ptrMeasurement(stored.measurement())
			case item.Revision == stored.Revision:
				result.Status = models.UsageWriteConflict
				result.Measurement = ptrMeasurement(stored.measurement())
			default:
				// The database identity belongs to the accepted row. A correction
				// may arrive without the internal id, but must keep that identity
				// stable for callers that retain the upsert result.
				item.ID = stored.ID
				updated, err := r.updateUsageTx(ctx, tx, item, stored.Revision)
				if err != nil {
					return nil, err
				}
				if updated == 0 {
					// A concurrent writer may have accepted a newer revision
					// between the read and the compare-and-update. Re-read the
					// row and report the actual accepted value.
					current, currentFound, readErr := r.findUsageTx(ctx, tx, item)
					if readErr != nil {
						return nil, readErr
					}
					if !currentFound {
						return nil, fmt.Errorf("usage row disappeared during revision update")
					}
					result.Measurement = ptrMeasurement(current.measurement())
					if item.Revision < current.Revision {
						result.Status = models.UsageWriteStale
					} else {
						result.Status = models.UsageWriteConflict
					}
				} else {
					result.Status = models.UsageWriteApplied
					result.Measurement = ptrMeasurement(item)
				}
			}
		} else {
			if item.ID == "" {
				item.ID = uuid.NewString()
			}
			inserted, err := r.insertUsageTx(ctx, tx, item)
			if err != nil {
				return nil, err
			}
			if inserted == 0 {
				// Another transaction won a concurrent first insert. Read the
				// accepted row and classify this observation against it.
				current, currentFound, readErr := r.findUsageTx(ctx, tx, item)
				if readErr != nil {
					return nil, readErr
				}
				if !currentFound {
					return nil, fmt.Errorf("usage row was not inserted or found")
				}
				previous := current.measurement()
				oldUsage = &previous
				if item.Revision > current.Revision {
					item.ID = current.ID
					updated, updateErr := r.updateUsageTx(ctx, tx, item, current.Revision)
					if updateErr != nil {
						return nil, updateErr
					}
					if updated > 0 {
						result.Status = models.UsageWriteApplied
						result.Measurement = ptrMeasurement(item)
					} else {
						latest, latestFound, readErr := r.findUsageTx(ctx, tx, item)
						if readErr != nil {
							return nil, readErr
						}
						if !latestFound {
							return nil, fmt.Errorf("usage row disappeared during concurrent insert")
						}
						result.Status = classifyUsageWrite(item, latest)
						result.Measurement = ptrMeasurement(latest.measurement())
					}
				} else {
					result.Measurement = ptrMeasurement(current.measurement())
					result.Status = classifyUsageWrite(item, current)
				}
			} else {
				result.Status = models.UsageWriteApplied
				result.Measurement = ptrMeasurement(item)
			}
		}
		if result.Status == models.UsageWriteApplied {
			if err := r.indexLegacyUsageTx(ctx, tx, usageTableFor(item), item); err != nil {
				return nil, err
			}
			current, err := r.loadIndexedObservationTx(ctx, tx, usageTableFor(item), item.ID)
			if err != nil {
				return nil, err
			}
			if oldIndexed == nil && oldUsage != nil {
				previous := current
				previous.Measurement = *oldUsage
				previous.CoverageKey = contractCoverageKey(*oldUsage)
				oldIndexed = &previous
			}
			if _, err := r.recordUsageMutationTx(ctx, tx, "observation", oldIndexed, &current); err != nil {
				return nil, err
			}
		}
		if result.Measurement != nil {
			accepted := *result.Measurement
			table := usageMeasurementsTable
			if accepted.CoverageKey != "" {
				table = usageBucketsTable
			}
			if err := r.insertUsageLiveLinkTx(ctx, tx, table, accepted.ID, accepted.ScopeID, accepted.SubjectID, accepted.TaskID, accepted.SessionID); err != nil {
				return nil, fmt.Errorf("link retained usage observation %q: %w", accepted.SourceRecordID, err)
			}
		}
		results[i] = result
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func classifyUsageWrite(item models.SessionUsageMeasurement, stored storedUsage) models.SessionUsageWriteStatus {
	switch {
	case item.Revision < stored.Revision:
		return models.UsageWriteStale
	case item.Revision == stored.Revision && item.PayloadDigest == stored.PayloadDigest:
		return models.UsageWriteUnchanged
	case item.Revision == stored.Revision:
		return models.UsageWriteConflict
	default:
		return models.UsageWriteConflict
	}
}

func normalizeUsage(input models.SessionUsageMeasurement) (models.SessionUsageMeasurement, error) {
	item := input
	item.WorkspaceID = strings.TrimSpace(item.WorkspaceID)
	item.TaskID = strings.TrimSpace(item.TaskID)
	item.SessionID = strings.TrimSpace(item.SessionID)
	item.Source = strings.TrimSpace(item.Source)
	item.SourceRecordID = strings.TrimSpace(item.SourceRecordID)
	item.UsageIdentity = strings.TrimSpace(item.UsageIdentity)
	item.Model = strings.TrimSpace(item.Model)
	item.Provider = strings.TrimSpace(item.Provider)
	if strings.EqualFold(item.Source, usageSourceNative) {
		return item, errors.New("native usage can only be retained with its accepted task event")
	}
	if item.WorkspaceID == "" || item.TaskID == "" || item.Source == "" || item.SourceRecordID == "" || item.UsageIdentity == "" {
		return item, errors.New("workspace, task, source, source_record_id, and usage_identity are required")
	}
	if item.Revision < 0 {
		return item, errors.New("revision cannot be negative")
	}
	if item.ObservedAt.IsZero() {
		item.ObservedAt = time.Now().UTC()
	}
	if item.CollectedAt.IsZero() {
		item.CollectedAt = item.ObservedAt
	}
	normalizeLegacyUsageCost(&item)
	if item.AttributionStatus == "" {
		item.AttributionStatus = models.UsageAttributionAttributed
	}
	normalizeLegacyUsageAuthority(&item)
	if item.CoverageKey == "" && item.SourceDate != "" {
		item.CoverageKey = item.SourceDate
	}
	if err := validateLegacyUsageAmounts(item); err != nil {
		return item, err
	}
	return normalizeLegacyUsageCoverage(item)
}

func normalizeLegacyUsageCost(item *models.SessionUsageMeasurement) {
	if item.CostBasis == "" {
		item.CostBasis = models.UsageCostBasisUnknown
	}
	if item.CostBasis != models.UsageCostBasisReported &&
		item.CostBasis != models.UsageCostBasisEstimated &&
		item.CostBasis != models.UsageCostBasisUnknown &&
		item.CostBasis != models.UsageCostBasisMixed {
		item.CostBasis = models.UsageCostBasisUnknown
	}
	if item.Coverage == "" {
		item.Coverage = models.UsageCoverageComplete
	}
	if item.CostCoverage == "" {
		if item.CostSubcents == nil || item.CostBasis == models.UsageCostBasisUnknown {
			item.CostCoverage = models.UsageCoverageMissing
		} else {
			item.CostCoverage = item.Coverage
		}
	}
	// A numeric zero from an unpriced source is a storage sentinel, not a
	// measured cost. Keep the token observation, but make the cost nullable so
	// reports cannot present it as a reported or estimated zero.
	if item.CostBasis == models.UsageCostBasisUnknown || item.CostCoverage == models.UsageCoverageMissing {
		item.CostSubcents = nil
	}

}

func normalizeLegacyUsageAuthority(item *models.SessionUsageMeasurement) {
	// Public and collector controlled observations cannot establish scope or
	// actor authority. Core derives scope from the owned task/session inside
	// this transaction; until a proven binding contract is available, actor
	// evidence remains explicitly unknown.
	actorStatus := item.ActorStatus
	item.ScopeID, item.SubjectID, item.UsageBindingID, item.ActorID = "", "", "", ""
	item.ActorKind = usageUnknown
	if actorStatus == models.UsageAttributionMixed {
		item.ActorStatus = models.UsageAttributionMixed
	} else {
		item.ActorStatus = models.UsageAttributionUnknown
	}
	item.NativeScope, item.MeasurementSource = "", ""

}

func validateLegacyUsageAmounts(item models.SessionUsageMeasurement) error {
	for name, value := range map[string]*int64{
		"input_tokens": item.InputTokens, "output_tokens": item.OutputTokens,
		"cache_read_tokens": item.CacheReadTokens, "cache_write_tokens": item.CacheWriteTokens,
		"reasoning_tokens": item.ReasoningTokens, "total_tokens": item.TotalTokens,
		"reasoning_output_tokens":     item.ReasoningOutputTokens,
		"source_total_tokens":         item.SourceTotalTokens,
		"reported_cache_write_tokens": item.ReportedCacheWriteTokens,
		"turns":                       item.Turns,
		"cost_subcents":               item.CostSubcents,
	} {
		if value != nil && *value < 0 {
			return fmt.Errorf("%s cannot be negative", name)
		}
	}
	if item.TotalTokens != nil && !tokenCategoriesFitTotal(item.InputTokens, item.OutputTokens, item.CacheReadTokens, item.CacheWriteTokens, item.ReasoningTokens, *item.TotalTokens) {
		return errors.New("token categories cannot exceed total_tokens")
	}
	return nil
}

func normalizeLegacyUsageCoverage(item models.SessionUsageMeasurement) (models.SessionUsageMeasurement, error) {
	if item.SourceDate != "" && item.CoverageStart == nil && item.CoverageEnd == nil {
		calendar := item.SourceTimezone
		if calendar == "" {
			calendar = projectionCalendarUTC
		}
		location, err := timeLocation(calendar)
		if err != nil {
			return item, err
		}
		start, err := time.ParseInLocation(time.DateOnly, item.SourceDate, location)
		if err != nil {
			return item, err
		}
		end := start.AddDate(0, 0, 1)
		item.CoverageStart, item.CoverageEnd, item.SourceTimezone = &start, &end, calendar
	}
	if item.CoverageStart != nil && item.CoverageEnd != nil && !item.CoverageStart.Before(*item.CoverageEnd) {
		return item, errors.New("coverage interval must be half open and non-empty")
	}
	if (item.CoverageStart == nil) != (item.CoverageEnd == nil) {
		return item, errors.New("coverage start and end must be provided together")
	}
	if item.CoverageKey == "" && item.CoverageStart != nil && item.CoverageEnd != nil {
		item.CoverageKey = "interval:" + item.CoverageStart.UTC().Format(time.RFC3339Nano) + ":" + item.CoverageEnd.UTC().Format(time.RFC3339Nano)
	}
	return item, nil
}

func usageKey(item models.SessionUsageMeasurement) string {
	return strings.Join([]string{item.Source, item.SourceRecordID, item.UsageIdentity, item.Model, item.Provider, item.CoverageKey}, "\x00")
}

func ptrMeasurement(value models.SessionUsageMeasurement) *models.SessionUsageMeasurement {
	return &value
}

func (r *Repository) validateUsageOwnership(ctx context.Context, records []models.SessionUsageMeasurement) error {
	for _, item := range records {
		var taskWorkspace string
		err := r.db.GetContext(ctx, &taskWorkspace, r.db.Rebind(`SELECT workspace_id FROM tasks WHERE id = ?`), item.TaskID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: task %q does not exist", ErrInvalidSessionUsage, item.TaskID)
		}
		if err != nil {
			return fmt.Errorf("validate usage task %q: %w", item.TaskID, err)
		}
		if taskWorkspace != item.WorkspaceID {
			return fmt.Errorf("%w: task %q is outside workspace %q", ErrInvalidSessionUsage, item.TaskID, item.WorkspaceID)
		}
		if item.SessionID == "" {
			continue
		}
		var owner struct {
			TaskID      string `db:"task_id"`
			WorkspaceID string `db:"workspace_id"`
		}
		err = r.db.GetContext(ctx, &owner, r.db.Rebind(`
			SELECT s.task_id, t.workspace_id
			FROM task_sessions s JOIN tasks t ON t.id = s.task_id
			WHERE s.id = ?`), item.SessionID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: session %q does not exist", ErrInvalidSessionUsage, item.SessionID)
		}
		if err != nil {
			return fmt.Errorf("validate usage session %q: %w", item.SessionID, err)
		}
		if owner.TaskID != item.TaskID || owner.WorkspaceID != item.WorkspaceID {
			return fmt.Errorf("%w: session %q is not owned by task %q in workspace %q", ErrInvalidSessionUsage, item.SessionID, item.TaskID, item.WorkspaceID)
		}
	}
	return nil
}

func (r *Repository) updateUsageTx(ctx context.Context, tx *sqlx.Tx, item models.SessionUsageMeasurement, expectedRevision int64) (int64, error) {
	table := usageMeasurementsTable
	where := `workspace_id = ? AND task_id = ? AND source = ? AND source_record_id = ? AND usage_identity = ? AND model = ? AND provider = ?`
	args := []any{item.WorkspaceID, item.TaskID, nullableString(item.SessionID), item.ScopeID, item.SubjectID,
		item.UsageBindingID, item.ActorID, actorStatusValue(item.ActorStatus), actorKindValue(item.ActorKind),
		item.TranscriptID, item.AgentClient, item.SourceVersion, item.NativeScope, item.MeasurementSource,
		item.UsageCompleteness, item.UsageSchemaVersion, item.PricingCatalogVersion,
		item.Revision, item.PayloadDigest, item.ObservedAt, item.CollectedAt, item.InputTokens, item.OutputTokens,
		item.CacheReadTokens, item.CacheWriteTokens, item.ReasoningTokens, item.ReasoningOutputTokens, item.TotalTokens,
		item.SourceTotalTokens, item.ReportedCacheWriteTokens,
		item.TokenSemanticsVersion, boolInt(item.ReasoningInsideOutput), boolInt(item.CacheReadInsideInput),
		item.Turns, item.CostSubcents,
		item.Currency, item.CostBasis, item.CostCoverage, item.Coverage, item.SourceTimezone, item.AttributionStatus,
		item.CoverageStart, item.CoverageEnd, item.SourceDate, item.CoverageKey, boolInt(item.Estimated), boolInt(item.Stale),
		item.WorkspaceID, item.TaskID, item.Source, item.SourceRecordID, item.UsageIdentity, item.Model, item.Provider,
	}
	if item.CoverageKey != "" {
		table = usageBucketsTable
		where += ` AND coverage_key = ?`
		args = append(args, item.CoverageKey)
	}
	where += ` AND revision = ?`
	args = append(args, expectedRevision)
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+table+` SET
		workspace_id = ?, task_id = ?, session_id = ?, scope_id = ?, subject_id = ?, usage_binding_id = ?,
		actor_id = ?, actor_status = ?, actor_kind = ?, transcript_id = ?, agent_client = ?, source_version = ?,
		native_scope = ?, measurement_source = ?, usage_completeness = ?, usage_schema_version = ?, pricing_catalog_version = ?,
		revision = ?, payload_digest = ?, observed_at = ?, collected_at = ?, input_tokens = ?,
		output_tokens = ?, cache_read_tokens = ?, cache_write_tokens = ?, reasoning_tokens = ?,
		reasoning_output_tokens = ?, total_tokens = ?, source_total_tokens = ?, reported_cache_write_tokens = ?, token_semantics_version = ?, reasoning_inside_output = ?,
		cache_read_inside_input = ?, turns = ?, cost_subcents = ?, currency = ?, cost_basis = ?, cost_coverage = ?, coverage = ?,
		source_timezone = ?, attribution_status = ?, coverage_start = ?, coverage_end = ?,
		source_date = ?, coverage_key = ?, estimated = ?, stale = ? WHERE `+where), args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
