//revive:disable:file-length-limit // Usage persistence, reconciliation, and projections share one schema boundary.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
)

const (
	maxSessionUsageBatch   = 200
	usageMeasurementsTable = "session_usage_measurements"
	usageBucketsTable      = "session_usage_buckets"
	usageGroupDay          = "day"
	usageGroupMonth        = "month"
	usageGroupModel        = "model"
	usageGroupDaily        = "daily"
	usageGroupMonthly      = "monthly"
	usageMixedValue        = "mixed"
	usageWorkFallback      = "task"
	usageSourceNative      = "native"
)

var ErrInvalidSessionUsage = usagehistory.ErrInvalidBatch

type storedUsage struct {
	ID                       string
	WorkspaceID              string
	TaskID                   string
	SessionID                string
	ScopeID                  string
	SubjectID                string
	UsageBindingID           string
	ActorID                  string
	ActorStatus              string
	ActorKind                string
	TranscriptID             string
	Source                   string
	SourceRecordID           string
	UsageIdentity            string
	Model                    string
	Provider                 string
	AgentClient              string
	SourceVersion            string
	NativeScope              string
	MeasurementSource        string
	UsageCompleteness        string
	UsageSchemaVersion       int
	PricingCatalogVersion    string
	Revision                 int64
	PayloadDigest            string
	ObservedAt               time.Time
	CollectedAt              time.Time
	InputTokens              *int64
	OutputTokens             *int64
	CacheReadTokens          *int64
	CacheWriteTokens         *int64
	ReasoningTokens          *int64
	ReasoningOutputTokens    *int64
	TotalTokens              *int64
	SourceTotalTokens        *int64
	ReportedCacheWriteTokens *int64
	TokenSemanticsVersion    int
	ReasoningInsideOutput    bool
	CacheReadInsideInput     bool
	Turns                    *int64
	CostSubcents             *int64
	Currency                 string
	CostBasis                string
	CostCoverage             string
	Coverage                 string
	SourceTimezone           string
	AttributionStatus        string
	CoverageStart            *time.Time
	CoverageEnd              *time.Time
	SourceDate               string
	CoverageKey              string
	Estimated                bool
	Stale                    bool
}

func (s storedUsage) measurement() models.SessionUsageMeasurement {
	return models.SessionUsageMeasurement{
		ID: s.ID, WorkspaceID: s.WorkspaceID, TaskID: s.TaskID, SessionID: s.SessionID,
		ScopeID: s.ScopeID, SubjectID: s.SubjectID, UsageBindingID: s.UsageBindingID,
		ActorID: s.ActorID, ActorStatus: s.ActorStatus, ActorKind: s.ActorKind,
		TranscriptID: s.TranscriptID, Source: s.Source, SourceRecordID: s.SourceRecordID,
		UsageIdentity: s.UsageIdentity, Model: s.Model, Provider: s.Provider,
		AgentClient:   s.AgentClient,
		SourceVersion: s.SourceVersion, Revision: s.Revision, PayloadDigest: s.PayloadDigest,
		NativeScope: s.NativeScope, MeasurementSource: s.MeasurementSource,
		UsageCompleteness: s.UsageCompleteness, UsageSchemaVersion: s.UsageSchemaVersion,
		PricingCatalogVersion: s.PricingCatalogVersion,
		ObservedAt:            s.ObservedAt, CollectedAt: s.CollectedAt, InputTokens: cloneInt64(s.InputTokens),
		OutputTokens: cloneInt64(s.OutputTokens), CacheReadTokens: cloneInt64(s.CacheReadTokens),
		CacheWriteTokens: cloneInt64(s.CacheWriteTokens), ReasoningTokens: cloneInt64(s.ReasoningTokens),
		ReasoningOutputTokens: cloneInt64(s.ReasoningOutputTokens),
		TotalTokens:           cloneInt64(s.TotalTokens), SourceTotalTokens: cloneInt64(s.SourceTotalTokens),
		ReportedCacheWriteTokens: cloneInt64(s.ReportedCacheWriteTokens),
		TokenSemanticsVersion:    s.TokenSemanticsVersion, ReasoningInsideOutput: s.ReasoningInsideOutput,
		CacheReadInsideInput: s.CacheReadInsideInput,
		Turns:                cloneInt64(s.Turns), CostSubcents: cloneInt64(s.CostSubcents), Currency: s.Currency,
		CostBasis: s.CostBasis, CostCoverage: s.CostCoverage, Coverage: s.Coverage, SourceTimezone: s.SourceTimezone,
		AttributionStatus: s.AttributionStatus, CoverageStart: s.CoverageStart,
		CoverageEnd: s.CoverageEnd, SourceDate: s.SourceDate, CoverageKey: s.CoverageKey,
		Estimated: s.Estimated, Stale: s.Stale,
	}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneUsageMeasurement(value models.SessionUsageMeasurement) models.SessionUsageMeasurement {
	value.InputTokens = cloneInt64(value.InputTokens)
	value.OutputTokens = cloneInt64(value.OutputTokens)
	value.CacheReadTokens = cloneInt64(value.CacheReadTokens)
	value.CacheWriteTokens = cloneInt64(value.CacheWriteTokens)
	value.ReasoningTokens = cloneInt64(value.ReasoningTokens)
	value.ReasoningOutputTokens = cloneInt64(value.ReasoningOutputTokens)
	value.TotalTokens = cloneInt64(value.TotalTokens)
	value.SourceTotalTokens = cloneInt64(value.SourceTotalTokens)
	value.ReportedCacheWriteTokens = cloneInt64(value.ReportedCacheWriteTokens)
	value.Turns = cloneInt64(value.Turns)
	value.CostSubcents = cloneInt64(value.CostSubcents)
	value.CoverageStart = cloneTime(value.CoverageStart)
	value.CoverageEnd = cloneTime(value.CoverageEnd)
	return value
}

//nolint:cyclop,gocognit,nestif,funlen // One transaction must classify every CAS outcome and preserve the accepted row.
func (r *Repository) UpsertSessionUsage(ctx context.Context, records []models.SessionUsageMeasurement) ([]models.SessionUsageUpsertResult, error) {
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

func (r *Repository) bindExternalUsageScopeTx(ctx context.Context, tx *sqlx.Tx, item models.SessionUsageMeasurement) (models.SessionUsageMeasurement, error) {
	var (
		workspaceID              string
		organizationID           sql.NullString
		sessionID, incarnationID string
		taskCreatedAt            time.Time
	)
	if item.SessionID != "" {
		err := tx.QueryRowxContext(ctx, r.db.Rebind(`
			SELECT t.workspace_id, w.org_id, ts.id, COALESCE(NULLIF(ts.queue_incarnation_id, ''), ts.id), t.created_at
			FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
			JOIN task_sessions ts ON ts.id = ? AND ts.task_id = t.id
		WHERE t.id = ?`), item.SessionID, item.TaskID).Scan(&workspaceID, &organizationID, &sessionID, &incarnationID, &taskCreatedAt)
		if err != nil {
			return item, fmt.Errorf("resolve trusted session ownership: %w", err)
		}
	} else {
		err := tx.QueryRowxContext(ctx, r.db.Rebind(`
			SELECT t.workspace_id, w.org_id, t.created_at FROM tasks t JOIN workspaces w ON w.id = t.workspace_id WHERE t.id = ?`),
			item.TaskID).Scan(&workspaceID, &organizationID, &taskCreatedAt)
		if err != nil {
			return item, fmt.Errorf("resolve trusted task ownership: %w", err)
		}
	}
	if workspaceID != item.WorkspaceID {
		return item, fmt.Errorf("task %q is outside workspace %q", item.TaskID, item.WorkspaceID)
	}
	item.SessionID = sessionID
	orgID := ""
	if organizationID.Valid {
		orgID = organizationID.String
	}
	item.ScopeID = usageScopeID(workspaceID, orgID)
	taskIncarnationID := usageTaskIncarnationID(taskCreatedAt)
	item.SubjectID = usageSubjectID(item.ScopeID, item.TaskID, taskIncarnationID, sessionID, incarnationID)
	item.ActorID, item.ActorKind, item.UsageBindingID = "", usageUnknown, ""
	if item.ActorStatus != models.UsageAttributionMixed {
		item.ActorStatus = models.UsageAttributionUnknown
	}
	createdAt := item.CollectedAt.UTC()
	if createdAt.IsZero() {
		createdAt = item.ObservedAt.UTC()
	}
	if err := ensureUsageScopeTx(ctx, tx, r.db, item.ScopeID, workspaceID, orgID, createdAt); err != nil {
		return item, err
	}
	if err := ensureUsageSubjectTx(ctx, tx, r.db, item.SubjectID, item.ScopeID, item.TaskID, taskIncarnationID, sessionID, incarnationID, createdAt); err != nil {
		return item, err
	}
	if sessionID != "" {
		if err := r.captureCollectionTargetTx(ctx, tx, item.SubjectID); err != nil {
			return item, err
		}
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

const usageSelectColumns = `
	id, workspace_id, task_id, COALESCE(session_id, ''), scope_id, subject_id,
	usage_binding_id, actor_id, actor_status, actor_kind, transcript_id,
	source, source_record_id, usage_identity, model, provider, agent_client, source_version,
	revision, payload_digest, CAST(observed_at AS TEXT), CAST(collected_at AS TEXT),
	input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
	reasoning_tokens, reasoning_output_tokens, total_tokens, source_total_tokens, reported_cache_write_tokens, token_semantics_version,
	reasoning_inside_output, cache_read_inside_input, native_scope, measurement_source,
	usage_completeness, usage_schema_version, pricing_catalog_version, turns, cost_subcents, currency, cost_basis,
	cost_coverage, coverage, source_timezone, attribution_status,
	CAST(coverage_start AS TEXT), CAST(coverage_end AS TEXT), source_date,
	coverage_key, estimated, stale`

func (r *Repository) findUsageTx(ctx context.Context, tx *sqlx.Tx, item models.SessionUsageMeasurement) (storedUsage, bool, error) {
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		query := `SELECT ` + usageSelectColumns + ` FROM ` + table + ` WHERE workspace_id = ? AND task_id = ? AND source = ? AND source_record_id = ? AND usage_identity = ? AND model = ? AND provider = ?`
		args := []any{item.WorkspaceID, item.TaskID, item.Source, item.SourceRecordID, item.UsageIdentity, item.Model, item.Provider}
		if table == usageBucketsTable {
			query += ` AND coverage_key = ?`
			args = append(args, item.CoverageKey)
		} else if item.CoverageKey != "" {
			continue
		}
		if r.db.DriverName() == usageDriverPostgres {
			query += usageForUpdate
		}
		row := tx.QueryRowxContext(ctx, r.db.Rebind(query), args...)
		parsed, err := scanUsageRow(row)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return storedUsage{}, false, err
		}
		return parsed.stored(), true, nil
	}
	return storedUsage{}, false, nil
}

// sqlRow uses nullable scanner values because SQLite returns NULL for the
// optional token/cost fields while PostgreSQL returns the same shape.
type sqlRow struct {
	ID                       string         `db:"id"`
	WorkspaceID              string         `db:"workspace_id"`
	TaskID                   string         `db:"task_id"`
	SessionID                string         `db:"coalesce(session_id, '')"`
	ScopeID                  string         `db:"scope_id"`
	SubjectID                string         `db:"subject_id"`
	UsageBindingID           string         `db:"usage_binding_id"`
	ActorID                  string         `db:"actor_id"`
	ActorStatus              string         `db:"actor_status"`
	ActorKind                string         `db:"actor_kind"`
	TranscriptID             string         `db:"transcript_id"`
	Source                   string         `db:"source"`
	SourceRecordID           string         `db:"source_record_id"`
	UsageIdentity            string         `db:"usage_identity"`
	Model                    string         `db:"model"`
	Provider                 string         `db:"provider"`
	AgentClient              string         `db:"agent_client"`
	SourceVersion            string         `db:"source_version"`
	NativeScope              string         `db:"native_scope"`
	MeasurementSource        string         `db:"measurement_source"`
	UsageCompleteness        string         `db:"usage_completeness"`
	UsageSchemaVersion       int            `db:"usage_schema_version"`
	PricingCatalogVersion    string         `db:"pricing_catalog_version"`
	Revision                 int64          `db:"revision"`
	PayloadDigest            string         `db:"payload_digest"`
	ObservedAt               string         `db:"cast(observed_at as text)"`
	CollectedAt              string         `db:"cast(collected_at as text)"`
	InputTokens              sql.NullInt64  `db:"input_tokens"`
	OutputTokens             sql.NullInt64  `db:"output_tokens"`
	CacheReadTokens          sql.NullInt64  `db:"cache_read_tokens"`
	CacheWriteTokens         sql.NullInt64  `db:"cache_write_tokens"`
	ReasoningTokens          sql.NullInt64  `db:"reasoning_tokens"`
	ReasoningOutputTokens    sql.NullInt64  `db:"reasoning_output_tokens"`
	TotalTokens              sql.NullInt64  `db:"total_tokens"`
	SourceTotalTokens        sql.NullInt64  `db:"source_total_tokens"`
	ReportedCacheWriteTokens sql.NullInt64  `db:"reported_cache_write_tokens"`
	TokenSemanticsVersion    int            `db:"token_semantics_version"`
	ReasoningInsideOutput    int            `db:"reasoning_inside_output"`
	CacheReadInsideInput     int            `db:"cache_read_inside_input"`
	Turns                    sql.NullInt64  `db:"turns"`
	CostSubcents             sql.NullInt64  `db:"cost_subcents"`
	Currency                 string         `db:"currency"`
	CostBasis                string         `db:"cost_basis"`
	CostCoverage             string         `db:"cost_coverage"`
	Coverage                 string         `db:"coverage"`
	SourceTimezone           string         `db:"source_timezone"`
	AttributionStatus        string         `db:"attribution_status"`
	CoverageStart            sql.NullString `db:"cast(coverage_start as text)"`
	CoverageEnd              sql.NullString `db:"cast(coverage_end as text)"`
	SourceDate               string         `db:"source_date"`
	CoverageKey              string         `db:"coverage_key"`
	Estimated                int            `db:"estimated"`
	Stale                    int            `db:"stale"`
}

type usageRowScanner interface {
	Scan(dest ...any) error
}

func scanUsageRow(scanner usageRowScanner) (sqlRow, error) {
	var row sqlRow
	err := scanner.Scan(
		&row.ID, &row.WorkspaceID, &row.TaskID, &row.SessionID,
		&row.ScopeID, &row.SubjectID, &row.UsageBindingID, &row.ActorID,
		&row.ActorStatus, &row.ActorKind, &row.TranscriptID, &row.Source, &row.SourceRecordID,
		&row.UsageIdentity, &row.Model, &row.Provider, &row.AgentClient,
		&row.SourceVersion, &row.Revision, &row.PayloadDigest, &row.ObservedAt,
		&row.CollectedAt, &row.InputTokens, &row.OutputTokens, &row.CacheReadTokens,
		&row.CacheWriteTokens, &row.ReasoningTokens, &row.ReasoningOutputTokens, &row.TotalTokens, &row.SourceTotalTokens,
		&row.ReportedCacheWriteTokens,
		&row.TokenSemanticsVersion, &row.ReasoningInsideOutput, &row.CacheReadInsideInput,
		&row.NativeScope, &row.MeasurementSource, &row.UsageCompleteness,
		&row.UsageSchemaVersion, &row.PricingCatalogVersion,
		&row.Turns, &row.CostSubcents,
		&row.Currency, &row.CostBasis, &row.CostCoverage, &row.Coverage, &row.SourceTimezone,
		&row.AttributionStatus, &row.CoverageStart, &row.CoverageEnd, &row.SourceDate,
		&row.CoverageKey, &row.Estimated, &row.Stale,
	)
	return row, err
}

func (r sqlRow) stored() storedUsage {
	return storedUsage{
		ID: r.ID, WorkspaceID: r.WorkspaceID, TaskID: r.TaskID, SessionID: r.SessionID,
		ScopeID: r.ScopeID, SubjectID: r.SubjectID, UsageBindingID: r.UsageBindingID,
		ActorID: r.ActorID, ActorStatus: r.ActorStatus, ActorKind: r.ActorKind,
		TranscriptID: r.TranscriptID, Source: r.Source, SourceRecordID: r.SourceRecordID,
		UsageIdentity: r.UsageIdentity, Model: r.Model, Provider: r.Provider,
		AgentClient:   r.AgentClient,
		SourceVersion: r.SourceVersion, NativeScope: r.NativeScope,
		MeasurementSource: r.MeasurementSource, UsageCompleteness: r.UsageCompleteness,
		UsageSchemaVersion: r.UsageSchemaVersion, PricingCatalogVersion: r.PricingCatalogVersion,
		Revision: r.Revision, PayloadDigest: r.PayloadDigest,
		ObservedAt: parseTimeString(r.ObservedAt), CollectedAt: parseTimeString(r.CollectedAt),
		InputTokens: nullableIntPtr(r.InputTokens), OutputTokens: nullableIntPtr(r.OutputTokens),
		CacheReadTokens: nullableIntPtr(r.CacheReadTokens), CacheWriteTokens: nullableIntPtr(r.CacheWriteTokens),
		ReasoningTokens: nullableIntPtr(r.ReasoningTokens), TotalTokens: nullableIntPtr(r.TotalTokens),
		ReasoningOutputTokens:    nullableIntPtr(r.ReasoningOutputTokens),
		SourceTotalTokens:        nullableIntPtr(r.SourceTotalTokens),
		ReportedCacheWriteTokens: nullableIntPtr(r.ReportedCacheWriteTokens),
		TokenSemanticsVersion:    r.TokenSemanticsVersion,
		ReasoningInsideOutput:    r.ReasoningInsideOutput != 0, CacheReadInsideInput: r.CacheReadInsideInput != 0,
		Turns:        nullableIntPtr(r.Turns),
		CostSubcents: nullableIntPtr(r.CostSubcents), Currency: r.Currency, CostBasis: r.CostBasis, CostCoverage: r.CostCoverage,
		Coverage: r.Coverage, SourceTimezone: r.SourceTimezone, AttributionStatus: r.AttributionStatus,
		CoverageStart: nullableTime(r.CoverageStart), CoverageEnd: nullableTime(r.CoverageEnd),
		SourceDate: r.SourceDate, CoverageKey: r.CoverageKey, Estimated: r.Estimated != 0, Stale: r.Stale != 0,
	}
}

func nullableIntPtr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func nullableTime(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed := parseTimeString(value.String)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func usageArgs(item models.SessionUsageMeasurement) []any {
	return []any{
		item.ID, item.WorkspaceID, item.TaskID, nullableString(item.SessionID), item.ScopeID,
		item.SubjectID, item.UsageBindingID, item.ActorID, actorStatusValue(item.ActorStatus), actorKindValue(item.ActorKind),
		item.TranscriptID, item.Source, item.SourceRecordID, item.UsageIdentity, item.Model, item.Provider,
		item.AgentClient, item.SourceVersion, item.NativeScope, item.MeasurementSource,
		item.UsageCompleteness, item.UsageSchemaVersion, item.PricingCatalogVersion,
		item.Revision, item.PayloadDigest, item.ObservedAt, item.CollectedAt,
		item.InputTokens, item.OutputTokens, item.CacheReadTokens, item.CacheWriteTokens,
		item.ReasoningTokens, item.ReasoningOutputTokens, item.TotalTokens, item.SourceTotalTokens, item.ReportedCacheWriteTokens,
		item.TokenSemanticsVersion,
		boolInt(item.ReasoningInsideOutput), boolInt(item.CacheReadInsideInput),
		item.Turns, item.CostSubcents, item.Currency, item.CostBasis,
		item.CostCoverage, item.Coverage, item.SourceTimezone, item.AttributionStatus, item.CoverageStart, item.CoverageEnd,
		item.SourceDate, item.CoverageKey, boolInt(item.Estimated), boolInt(item.Stale),
	}
}

const usageWriteColumns = `
	id, workspace_id, task_id, session_id, scope_id, subject_id, usage_binding_id,
	actor_id, actor_status, actor_kind, transcript_id, source, source_record_id,
	usage_identity, model, provider, agent_client, source_version, native_scope,
	measurement_source, usage_completeness, usage_schema_version, pricing_catalog_version,
	revision, payload_digest,
	observed_at, collected_at, input_tokens, output_tokens, cache_read_tokens,
	cache_write_tokens, reasoning_tokens, reasoning_output_tokens, total_tokens, source_total_tokens, reported_cache_write_tokens,
	token_semantics_version, reasoning_inside_output, cache_read_inside_input,
	turns, cost_subcents, currency,
	cost_basis, cost_coverage, coverage, source_timezone, attribution_status, coverage_start,
	coverage_end, source_date, coverage_key, estimated, stale`

func (r *Repository) insertUsageTx(ctx context.Context, tx *sqlx.Tx, item models.SessionUsageMeasurement) (int64, error) {
	table := usageMeasurementsTable
	if item.CoverageKey != "" {
		table = usageBucketsTable
	}
	args := usageArgs(item)
	query := `INSERT INTO ` + table + ` (` + usageWriteColumns + `) VALUES (` + placeholders(len(args)) + `) ON CONFLICT DO NOTHING`
	result, err := tx.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	changed, err := result.RowsAffected()
	if err == nil && changed > 0 {
		err = r.indexLegacyUsageTx(ctx, tx, usageTableFor(item), item)
	}
	return changed, err
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

func placeholders(count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func actorStatusValue(value string) string {
	if value == "" {
		return models.UsageAttributionUnknown
	}
	return value
}

func actorKindValue(value string) string {
	if value == "" {
		return usageUnknown
	}
	return value
}

// ListSessionUsage reads only committed retained projections.
func (r *Repository) ListSessionUsage(ctx context.Context, filter models.SessionUsageFilter) (*models.TokenUsageReport, error) {
	return r.listRetainedUsageReport(ctx, filter)
}

// ListSessionUsageMeasurements returns a bounded SQL page of source-owned
// accepted observations. Canonical selection is a separate retained read.
func (r *Repository) ListSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	return r.listSourceUsagePage(ctx, filter)
}

// ListCanonicalSessionUsageMeasurements returns the accepted measurement for
// each selected coverage unit, including native ledger events. It is a read
// path for authorized Host consumers; it does not broaden write ownership.
func (r *Repository) ListCanonicalSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	rows, _, err := r.ListRetainedCanonicalUsagePage(ctx, filter)
	return rows, err
}

func isDatedUsageGroup(group string) bool {
	switch strings.ToLower(group) {
	case usageGroupDay, usageGroupDaily, usageGroupMonth, usageGroupMonthly:
		return true
	default:
		return false
	}
}

func mergeMeasurementValues(target *models.SessionUsageMeasurement, value models.SessionUsageMeasurement) {
	addMeasurementNullable(&target.InputTokens, value.InputTokens)
	addMeasurementNullable(&target.OutputTokens, value.OutputTokens)
	addMeasurementNullable(&target.CacheReadTokens, value.CacheReadTokens)
	addMeasurementNullable(&target.CacheWriteTokens, value.CacheWriteTokens)
	addMeasurementNullable(&target.ReasoningTokens, value.ReasoningTokens)
	addMeasurementNullable(&target.TotalTokens, value.TotalTokens)
	addMeasurementNullable(&target.Turns, value.Turns)
	addMeasurementNullable(&target.CostSubcents, value.CostSubcents)
	target.CostBasis = mergeCostBasis(target.CostBasis, value.CostBasis)
	target.CostCoverage = mergeCoverage(target.CostCoverage, value.CostCoverage)
	target.Coverage = mergeCoverage(target.Coverage, value.Coverage)
	if target.Currency == "" {
		target.Currency = value.Currency
	} else if value.Currency != "" && target.Currency != value.Currency {
		target.Currency = usageMixedValue
		target.CostSubcents = nil
	}
	if target.CoverageStart == nil || (value.CoverageStart != nil && value.CoverageStart.Before(*target.CoverageStart)) {
		target.CoverageStart = value.CoverageStart
	}
	if target.CoverageEnd == nil || (value.CoverageEnd != nil && value.CoverageEnd.After(*target.CoverageEnd)) {
		target.CoverageEnd = value.CoverageEnd
	}
}

func addMeasurementNullable(target **int64, value *int64) {
	if value == nil {
		*target = nil
		return
	}
	if *target == nil {
		return
	}
	if *value > 0 && **target > math.MaxInt64-*value {
		*target = nil
		return
	}
	**target += *value
}

func tokenValuesEqual(left, right models.SessionUsageMeasurement) bool {
	return nullableMeasurementsEqual(left.InputTokens, right.InputTokens) &&
		nullableMeasurementsEqual(left.OutputTokens, right.OutputTokens) &&
		nullableMeasurementsEqual(left.CacheReadTokens, right.CacheReadTokens) &&
		nullableMeasurementsEqual(left.CacheWriteTokens, right.CacheWriteTokens) &&
		nullableMeasurementsEqual(left.ReasoningTokens, right.ReasoningTokens) &&
		nullableMeasurementsEqual(left.TotalTokens, right.TotalTokens) &&
		nullableMeasurementsEqual(left.Turns, right.Turns)
}

func nullableMeasurementsEqual(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func tokenCompletenessRank(value models.SessionUsageMeasurement) int {
	rank := 0
	for _, token := range []*int64{
		value.InputTokens, value.OutputTokens, value.CacheReadTokens,
		value.CacheWriteTokens, value.ReasoningTokens, value.TotalTokens, value.Turns,
	} {
		if token != nil {
			rank++
		}
	}
	return rank
}

func usageCoverageRank(value string) int {
	switch value {
	case models.UsageCoverageComplete:
		return 2
	case models.UsageCoveragePartial:
		return 1
	default:
		return 0
	}
}

func usageCostBasisRank(value string) int {
	switch value {
	case models.UsageCostBasisReported:
		return 2
	case models.UsageCostBasisEstimated:
		return 1
	default:
		return 0
	}
}

const unknownProviderFilter = "__unknown_provider__"

func tokenCategoriesFitTotal(input, output, cacheRead, cacheWrite, reasoning *int64, total int64) bool {
	if total < 0 {
		return false
	}
	for _, value := range []*int64{input, output, cacheRead, cacheWrite, reasoning} {
		if value == nil {
			continue
		}
		if *value < 0 || *value > total {
			return false
		}
	}
	return true
}

func mergeCostBasis(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" || left == right {
		return left
	}
	if left == models.UsageCostBasisUnknown || right == models.UsageCostBasisUnknown {
		return models.UsageCostBasisUnknown
	}
	return models.UsageCostBasisMixed
}

func mergeCoverage(left, right string) string {
	if left == "" {
		return right
	}
	if left == right {
		return left
	}
	if left == models.UsageCoverageMissing || right == models.UsageCoverageMissing {
		return models.UsageCoverageMissing
	}
	return models.UsageCoveragePartial
}

func sortUsageRows(rows []models.TokenUsageRow, sortBy string, ascending bool) {
	sort.Slice(rows, func(i, j int) bool {
		return usageRowLess(rows[i], rows[j], sortBy, ascending)
	})
}

//nolint:cyclop,gocognit,nestif,funlen // Sorting keeps nullable values after known values for every requested direction.
func usageRowLess(left, right models.TokenUsageRow, sortBy string, ascending bool) bool {
	compare := func(value int) bool {
		if value == 0 {
			return left.Key < right.Key
		}
		if ascending {
			return value < 0
		}
		return value > 0
	}
	compareNullable := func(leftValue, rightValue *int64) (bool, bool) {
		if leftValue == nil || rightValue == nil {
			if leftValue == nil && rightValue == nil {
				return false, false
			}
			// Unknown values always sort after known values. Keep this rule
			// independent of direction so a descending cost/token query does
			// not put unavailable values first.
			return leftValue != nil, true
		}
		if *leftValue == *rightValue {
			return false, false
		}
		value := -1
		if *leftValue > *rightValue {
			value = 1
		}
		return compare(value), true
	}
	switch strings.ToLower(sortBy) {
	case "cost", "cost_subcents":
		if result, decided := compareNullable(left.CostSubcents, right.CostSubcents); decided {
			return result
		}
	case "tokens", "total_tokens":
		if result, decided := compareNullable(left.TotalTokens, right.TotalTokens); decided {
			return result
		}
	case "rate", "effective_cost_per_million":
		leftRate := left.EffectiveCostPerMillion
		rightRate := right.EffectiveCostPerMillion
		if leftRate == nil || rightRate == nil {
			return leftRate != nil
		} else if *leftRate != *rightRate {
			if ascending {
				return *leftRate < *rightRate
			}
			return *leftRate > *rightRate
		}
	case "period", "date":
		if left.Period != right.Period {
			if left.Period == "" {
				return false
			}
			if right.Period == "" {
				return true
			}
			if ascending {
				return left.Period < right.Period
			}
			return left.Period > right.Period
		}
	case usageWorkFallback, "task_id":
		if left.TaskID != right.TaskID {
			if ascending {
				return left.TaskID < right.TaskID
			}
			return left.TaskID > right.TaskID
		}
	case usageSubjectSession, "session_id":
		if left.SessionID != right.SessionID {
			if ascending {
				return left.SessionID < right.SessionID
			}
			return left.SessionID > right.SessionID
		}
	case "provider":
		if left.Provider != right.Provider {
			if ascending {
				return left.Provider < right.Provider
			}
			return left.Provider > right.Provider
		}
	case usageGroupModel, "label":
		if left.Model != right.Model {
			if ascending {
				return left.Model < right.Model
			}
			return left.Model > right.Model
		}
	default:
		if left.Model != right.Model {
			if ascending {
				return left.Model < right.Model
			}
			return left.Model > right.Model
		}
	}
	return left.Key < right.Key
}
