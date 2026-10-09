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
	"github.com/kandev/kandev/internal/db"
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

var ErrInvalidSessionUsage = errors.New("invalid session usage batch")

type storedUsage struct {
	ID                string
	WorkspaceID       string
	TaskID            string
	SessionID         string
	TranscriptID      string
	Source            string
	SourceRecordID    string
	UsageIdentity     string
	Model             string
	Provider          string
	SourceVersion     string
	Revision          int64
	PayloadDigest     string
	ObservedAt        time.Time
	CollectedAt       time.Time
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
	CoverageStart     *time.Time
	CoverageEnd       *time.Time
	SourceDate        string
	CoverageKey       string
	Estimated         bool
	Stale             bool
}

func (s storedUsage) measurement() models.SessionUsageMeasurement {
	return models.SessionUsageMeasurement{
		ID: s.ID, WorkspaceID: s.WorkspaceID, TaskID: s.TaskID, SessionID: s.SessionID,
		TranscriptID: s.TranscriptID, Source: s.Source, SourceRecordID: s.SourceRecordID,
		UsageIdentity: s.UsageIdentity, Model: s.Model, Provider: s.Provider,
		SourceVersion: s.SourceVersion, Revision: s.Revision, PayloadDigest: s.PayloadDigest,
		ObservedAt: s.ObservedAt, CollectedAt: s.CollectedAt, InputTokens: cloneInt64(s.InputTokens),
		OutputTokens: cloneInt64(s.OutputTokens), CacheReadTokens: cloneInt64(s.CacheReadTokens),
		CacheWriteTokens: cloneInt64(s.CacheWriteTokens), ReasoningTokens: cloneInt64(s.ReasoningTokens),
		TotalTokens: cloneInt64(s.TotalTokens), Turns: cloneInt64(s.Turns), CostSubcents: cloneInt64(s.CostSubcents), Currency: s.Currency,
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
	value.TotalTokens = cloneInt64(value.TotalTokens)
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
		result := models.SessionUsageUpsertResult{
			SourceRecordID: item.SourceRecordID, UsageIdentity: item.UsageIdentity,
			Model: item.Model, Provider: item.Provider,
		}
		stored, found, err := r.findUsageTx(ctx, tx, item)
		if err != nil {
			return nil, err
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

//nolint:cyclop // Normalization validates independent identity, cost, coverage, and numeric fields.
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
	if item.AttributionStatus == "" {
		item.AttributionStatus = models.UsageAttributionAttributed
	}
	if item.CoverageKey == "" && item.SourceDate != "" {
		item.CoverageKey = item.SourceDate
	}
	for name, value := range map[string]*int64{
		"input_tokens": item.InputTokens, "output_tokens": item.OutputTokens,
		"cache_read_tokens": item.CacheReadTokens, "cache_write_tokens": item.CacheWriteTokens,
		"reasoning_tokens": item.ReasoningTokens, "total_tokens": item.TotalTokens,
		"turns":         item.Turns,
		"cost_subcents": item.CostSubcents,
	} {
		if value != nil && *value < 0 {
			return item, fmt.Errorf("%s cannot be negative", name)
		}
	}
	if item.TotalTokens != nil && !tokenCategoriesFitTotal(item.InputTokens, item.OutputTokens, item.CacheReadTokens, item.CacheWriteTokens, item.ReasoningTokens, *item.TotalTokens) {
		return item, errors.New("token categories cannot exceed total_tokens")
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

func usageBaseKey(item models.SessionUsageMeasurement) string {
	// UsageIdentity names the representation (lifetime versus a dated bucket),
	// so it must not split those representations into separate selection groups.
	// SourceRecordID plus the attributed work/model/provider identifies the
	// underlying source record while CoverageKey remains representation-specific.
	return strings.Join([]string{item.Source, item.SourceRecordID, item.TaskID, item.SessionID, item.Model, item.Provider}, "\x00")
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
	id, workspace_id, task_id, COALESCE(session_id, ''), transcript_id,
	source, source_record_id, usage_identity, model, provider, source_version,
	revision, payload_digest, CAST(observed_at AS TEXT), CAST(collected_at AS TEXT),
	input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
	reasoning_tokens, total_tokens, turns, cost_subcents, currency, cost_basis,
	cost_coverage, coverage, source_timezone, attribution_status,
	CAST(coverage_start AS TEXT), CAST(coverage_end AS TEXT), source_date,
	coverage_key, estimated, stale`

func (r *Repository) findUsageTx(ctx context.Context, tx *sqlx.Tx, item models.SessionUsageMeasurement) (storedUsage, bool, error) {
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		query := `SELECT ` + usageSelectColumns + ` FROM ` + table + ` WHERE workspace_id = ? AND source = ? AND source_record_id = ? AND usage_identity = ? AND model = ? AND provider = ?`
		args := []any{item.WorkspaceID, item.Source, item.SourceRecordID, item.UsageIdentity, item.Model, item.Provider}
		if table == usageBucketsTable {
			query += ` AND coverage_key = ?`
			args = append(args, item.CoverageKey)
		} else if item.CoverageKey != "" {
			continue
		}
		if r.db.DriverName() == "pgx" {
			query += ` FOR UPDATE`
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
	ID                string         `db:"id"`
	WorkspaceID       string         `db:"workspace_id"`
	TaskID            string         `db:"task_id"`
	SessionID         string         `db:"coalesce(session_id, '')"`
	TranscriptID      string         `db:"transcript_id"`
	Source            string         `db:"source"`
	SourceRecordID    string         `db:"source_record_id"`
	UsageIdentity     string         `db:"usage_identity"`
	Model             string         `db:"model"`
	Provider          string         `db:"provider"`
	SourceVersion     string         `db:"source_version"`
	Revision          int64          `db:"revision"`
	PayloadDigest     string         `db:"payload_digest"`
	ObservedAt        string         `db:"cast(observed_at as text)"`
	CollectedAt       string         `db:"cast(collected_at as text)"`
	InputTokens       sql.NullInt64  `db:"input_tokens"`
	OutputTokens      sql.NullInt64  `db:"output_tokens"`
	CacheReadTokens   sql.NullInt64  `db:"cache_read_tokens"`
	CacheWriteTokens  sql.NullInt64  `db:"cache_write_tokens"`
	ReasoningTokens   sql.NullInt64  `db:"reasoning_tokens"`
	TotalTokens       sql.NullInt64  `db:"total_tokens"`
	Turns             sql.NullInt64  `db:"turns"`
	CostSubcents      sql.NullInt64  `db:"cost_subcents"`
	Currency          string         `db:"currency"`
	CostBasis         string         `db:"cost_basis"`
	CostCoverage      string         `db:"cost_coverage"`
	Coverage          string         `db:"coverage"`
	SourceTimezone    string         `db:"source_timezone"`
	AttributionStatus string         `db:"attribution_status"`
	CoverageStart     sql.NullString `db:"cast(coverage_start as text)"`
	CoverageEnd       sql.NullString `db:"cast(coverage_end as text)"`
	SourceDate        string         `db:"source_date"`
	CoverageKey       string         `db:"coverage_key"`
	Estimated         int            `db:"estimated"`
	Stale             int            `db:"stale"`
}

type usageRowScanner interface {
	Scan(dest ...any) error
}

func scanUsageRow(scanner usageRowScanner) (sqlRow, error) {
	var row sqlRow
	err := scanner.Scan(
		&row.ID, &row.WorkspaceID, &row.TaskID, &row.SessionID, &row.TranscriptID,
		&row.Source, &row.SourceRecordID, &row.UsageIdentity, &row.Model, &row.Provider,
		&row.SourceVersion, &row.Revision, &row.PayloadDigest, &row.ObservedAt,
		&row.CollectedAt, &row.InputTokens, &row.OutputTokens, &row.CacheReadTokens,
		&row.CacheWriteTokens, &row.ReasoningTokens, &row.TotalTokens, &row.Turns, &row.CostSubcents,
		&row.Currency, &row.CostBasis, &row.CostCoverage, &row.Coverage, &row.SourceTimezone,
		&row.AttributionStatus, &row.CoverageStart, &row.CoverageEnd, &row.SourceDate,
		&row.CoverageKey, &row.Estimated, &row.Stale,
	)
	return row, err
}

func (r sqlRow) stored() storedUsage {
	return storedUsage{
		ID: r.ID, WorkspaceID: r.WorkspaceID, TaskID: r.TaskID, SessionID: r.SessionID,
		TranscriptID: r.TranscriptID, Source: r.Source, SourceRecordID: r.SourceRecordID,
		UsageIdentity: r.UsageIdentity, Model: r.Model, Provider: r.Provider,
		SourceVersion: r.SourceVersion, Revision: r.Revision, PayloadDigest: r.PayloadDigest,
		ObservedAt: parseTimeString(r.ObservedAt), CollectedAt: parseTimeString(r.CollectedAt),
		InputTokens: nullableIntPtr(r.InputTokens), OutputTokens: nullableIntPtr(r.OutputTokens),
		CacheReadTokens: nullableIntPtr(r.CacheReadTokens), CacheWriteTokens: nullableIntPtr(r.CacheWriteTokens),
		ReasoningTokens: nullableIntPtr(r.ReasoningTokens), TotalTokens: nullableIntPtr(r.TotalTokens), Turns: nullableIntPtr(r.Turns),
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
		item.ID, item.WorkspaceID, item.TaskID, nullableString(item.SessionID), item.TranscriptID,
		item.Source, item.SourceRecordID, item.UsageIdentity, item.Model, item.Provider, item.SourceVersion,
		item.Revision, item.PayloadDigest, item.ObservedAt, item.CollectedAt,
		item.InputTokens, item.OutputTokens, item.CacheReadTokens, item.CacheWriteTokens,
		item.ReasoningTokens, item.TotalTokens, item.Turns, item.CostSubcents, item.Currency, item.CostBasis,
		item.CostCoverage, item.Coverage, item.SourceTimezone, item.AttributionStatus, item.CoverageStart, item.CoverageEnd,
		item.SourceDate, item.CoverageKey, boolInt(item.Estimated), boolInt(item.Stale),
	}
}

const usageWriteColumns = `
	id, workspace_id, task_id, session_id, transcript_id, source, source_record_id,
	usage_identity, model, provider, source_version, revision, payload_digest,
	observed_at, collected_at, input_tokens, output_tokens, cache_read_tokens,
	cache_write_tokens, reasoning_tokens, total_tokens, turns, cost_subcents, currency,
	cost_basis, cost_coverage, coverage, source_timezone, attribution_status, coverage_start,
	coverage_end, source_date, coverage_key, estimated, stale`

func (r *Repository) insertUsageTx(ctx context.Context, tx *sqlx.Tx, item models.SessionUsageMeasurement) (int64, error) {
	table := usageMeasurementsTable
	if item.CoverageKey != "" {
		table = usageBucketsTable
	}
	query := `INSERT INTO ` + table + ` (` + usageWriteColumns + `) VALUES (` + placeholders(35) + `) ON CONFLICT DO NOTHING`
	result, err := tx.ExecContext(ctx, r.db.Rebind(query), usageArgs(item)...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *Repository) updateUsageTx(ctx context.Context, tx *sqlx.Tx, item models.SessionUsageMeasurement, expectedRevision int64) (int64, error) {
	table := usageMeasurementsTable
	where := `workspace_id = ? AND source = ? AND source_record_id = ? AND usage_identity = ? AND model = ? AND provider = ?`
	args := []any{item.WorkspaceID, item.TaskID, nullableString(item.SessionID), item.TranscriptID, item.SourceVersion,
		item.Revision, item.PayloadDigest, item.ObservedAt, item.CollectedAt, item.InputTokens, item.OutputTokens,
		item.CacheReadTokens, item.CacheWriteTokens, item.ReasoningTokens, item.TotalTokens, item.Turns, item.CostSubcents,
		item.Currency, item.CostBasis, item.CostCoverage, item.Coverage, item.SourceTimezone, item.AttributionStatus,
		item.CoverageStart, item.CoverageEnd, item.SourceDate, item.CoverageKey, boolInt(item.Estimated), boolInt(item.Stale),
		item.WorkspaceID, item.Source, item.SourceRecordID, item.UsageIdentity, item.Model, item.Provider,
	}
	if item.CoverageKey != "" {
		table = usageBucketsTable
		where += ` AND coverage_key = ?`
		args = append(args, item.CoverageKey)
	}
	where += ` AND revision = ?`
	args = append(args, expectedRevision)
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+table+` SET
		workspace_id = ?, task_id = ?, session_id = ?, transcript_id = ?, source_version = ?,
		revision = ?, payload_digest = ?, observed_at = ?, collected_at = ?, input_tokens = ?,
		output_tokens = ?, cache_read_tokens = ?, cache_write_tokens = ?, reasoning_tokens = ?,
		total_tokens = ?, turns = ?, cost_subcents = ?, currency = ?, cost_basis = ?, cost_coverage = ?, coverage = ?,
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

type usageContribution struct {
	measurement models.SessionUsageMeasurement
	period      string
	fromNative  bool
}

//nolint:cyclop,funlen // The report boundary combines source selection, metadata, sorting, and pagination.
func (r *Repository) ListSessionUsage(ctx context.Context, filter models.SessionUsageFilter) (*models.TokenUsageReport, error) {
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	if filter.Limit <= 0 {
		filter.Limit = 200
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	external, _, _, _, err := r.externalUsageContributions(ctx, filter)
	if err != nil {
		return nil, err
	}
	nativeHistory, err := r.hasNativeUsage(ctx, filter)
	if err != nil {
		return nil, err
	}
	native, err := r.nativeUsageContributions(ctx, filter)
	if err != nil {
		return nil, err
	}
	contributions := canonicalUsageContributions(external, native)

	contributions = markMonthlyRangeCoverage(contributions, filter)
	rows, totals := aggregateUsage(contributions, filter)
	if filter.Start == nil && filter.End == nil && isDatedUsageGroup(filter.GroupBy) {
		// Dated all-time rows intentionally use the available bucket
		// projection. Keep the cumulative lifetime projection as the summary
		// total when the bucket set is partial, so a 20-token imported day
		// cannot shrink a 100-token lifetime total.
		totalFilter := filter
		totalFilter.GroupBy = usageGroupModel
		totalExternal, _, _, _, totalErr := r.externalUsageContributions(ctx, totalFilter)
		if totalErr != nil {
			return nil, totalErr
		}
		totalNative, totalErr := r.nativeUsageContributions(ctx, totalFilter)
		if totalErr != nil {
			return nil, totalErr
		}
		_, totals = aggregateUsage(canonicalUsageContributions(totalExternal, totalNative), totalFilter)
	}
	for i := range rows {
		rows[i].EffectiveCostPerMillion = effectiveCostPerMillion(
			rows[i].CostSubcents, rows[i].TotalTokens, rows[i].CostBasis,
			rows[i].Currency, rows[i].Coverage, rows[i].CostCoverage,
		)
	}
	totals.EffectiveCostPerMillion = effectiveCostPerMillion(
		totals.CostSubcents, totals.TotalTokens, totals.CostBasis,
		totals.Currency, usageTotalsCoverage(totals.Partial), totals.CostCoverage,
	)
	// Availability and freshness describe the authorized dataset, even when the
	// selected range has no rows. Keep range data separate so an older saved
	// measurement produces the normal empty-range state instead of the install
	// placeholder, and so native-only history supplies the same metadata.
	metadataFilter := filter
	metadataFilter.Start = nil
	metadataFilter.End = nil
	metadataFilter.IncludeUndated = true
	// Metadata describes the complete stored scope, independently of the
	// selected breakdown. Use the lifetime/model projection here so a daily or
	// monthly request cannot hide a lifetime-only observation from freshness.
	metadataFilter.GroupBy = usageGroupModel
	metadataExternal, metadataExternalDated, metadataExternalUndated, _, err := r.externalUsageContributions(ctx, metadataFilter)
	if err != nil {
		return nil, err
	}
	metadataNative, err := r.nativeUsageContributions(ctx, metadataFilter)
	if err != nil {
		return nil, err
	}
	metadataContributions := canonicalUsageContributions(metadataExternal, metadataNative)
	nativeDated, nativeUndated, canonicalUpdated := usageMetadata(metadataContributions)
	dated := metadataExternalDated || nativeDated
	undated := metadataExternalUndated || nativeUndated
	// Freshness belongs to the rows the canonical selector actually exposes.
	// A superseded source observation must not make the saved report appear
	// newer than its accepted value. The raw external flags above still expose
	// that dated and undated projections exist when a partial import has both.
	lastUpdated := canonicalUpdated
	hasExternalHistory, err := r.hasExternalUsage(ctx, filter)
	if err != nil {
		return nil, err
	}
	sortBy := strings.TrimSpace(filter.SortBy)
	if sortBy == "" {
		switch strings.ToLower(filter.GroupBy) {
		case usageGroupDay, usageGroupDaily, usageGroupMonth, usageGroupMonthly:
			sortBy = "period"
		default:
			sortBy = "cost"
		}
	}
	sortUsageRows(rows, sortBy, strings.EqualFold(filter.SortDirection, "asc"))
	totalRows := len(rows)
	start := filter.Offset
	if start > totalRows {
		start = totalRows
	}
	end := start + filter.Limit
	if end > totalRows {
		end = totalRows
	}
	pageRows := rows[start:end]
	report := &models.TokenUsageReport{
		Summary: totals, Rows: pageRows, Totals: totals, TotalRows: totalRows,
		HasMore: end < totalRows, HasHistory: hasExternalHistory || nativeHistory,
		HasRangeData: hasDatedRangeData(contributions, filter), HasNativeHistory: nativeHistory,
		DatedCoverage: dated, UndatedCoverage: undated,
	}
	providerFilter := filter
	providerFilter.Provider = ""
	providerExternal, _, _, _, err := r.externalUsageContributions(ctx, providerFilter)
	if err != nil {
		return nil, err
	}
	providerNative, err := r.nativeUsageContributions(ctx, providerFilter)
	if err != nil {
		return nil, err
	}
	report.Providers = usageProviderOptions(canonicalUsageContributions(providerExternal, providerNative))
	if !lastUpdated.IsZero() {
		report.LastUpdated = &lastUpdated
	}
	return report, nil
}

// ListSessionUsageMeasurements is the raw source-observation read used by the
// plugin Host API. It applies the same lifetime-versus-bucket selection as the
// report query and uses the filter's limit/offset as a bounded page.
func (r *Repository) ListSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	contributions, _, _, _, err := r.externalUsageContributions(ctx, filter)
	if err != nil {
		return nil, err
	}
	measurements := make([]models.SessionUsageMeasurement, 0, len(contributions))
	for _, contribution := range contributions {
		measurements = append(measurements, contribution.measurement)
	}
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	if start >= len(measurements) {
		return []models.SessionUsageMeasurement{}, nil
	}
	end := len(measurements)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return measurements[start:end], nil
}

// ListCanonicalSessionUsageMeasurements returns the accepted measurement for
// each selected coverage unit, including native ledger events. It is a read
// path for authorized Host consumers; it does not broaden write ownership.
func (r *Repository) ListCanonicalSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	external, _, _, _, err := r.externalUsageContributions(ctx, filter)
	if err != nil {
		return nil, err
	}
	native, err := r.nativeUsageContributions(ctx, filter)
	if err != nil {
		return nil, err
	}
	selected := canonicalUsageContributions(external, native)
	measurements := make([]models.SessionUsageMeasurement, 0, len(selected))
	for _, contribution := range selected {
		measurements = append(measurements, contribution.measurement)
	}
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	if start >= len(measurements) {
		return []models.SessionUsageMeasurement{}, nil
	}
	end := len(measurements)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return measurements[start:end], nil
}

func (r *Repository) externalUsageContributions(ctx context.Context, filter models.SessionUsageFilter) ([]usageContribution, bool, bool, time.Time, error) {
	buckets, err := r.listUsageTable(ctx, usageBucketsTable, filter)
	if err != nil {
		return nil, false, false, time.Time{}, err
	}
	lifetime, err := r.listUsageTable(ctx, usageMeasurementsTable, filter)
	if err != nil {
		return nil, false, false, time.Time{}, err
	}
	all := chooseExternalRepresentations(buckets, lifetime, filter)

	contributions := make([]usageContribution, 0, len(all))
	var dated, undated bool
	var lastUpdated time.Time
	for _, stored := range all {
		item := stored.measurement()
		if !usageMatchesTime(item, filter) {
			continue
		}
		contributions = append(contributions, usageContribution{measurement: item, period: usagePeriod(item, filter.Timezone)})
	}
	contributions = selectUsageContributions(contributions)
	for _, contribution := range contributions {
		item := contribution.measurement
		if item.CoverageKey == "" && item.SourceDate == "" && item.CoverageStart == nil {
			undated = true
		} else {
			dated = true
		}
		if item.CollectedAt.After(lastUpdated) {
			lastUpdated = item.CollectedAt
		}
	}
	if filter.Start == nil && filter.End == nil {
		// Lifetime and dated buckets are alternative all-time projections. A
		// partial bucket set still provides dated chart coverage while the
		// lifetime snapshot remains the authoritative all-time total.
		dated, undated = externalRepresentationCoverage(buckets, lifetime)
		for _, stored := range append(append([]storedUsage{}, buckets...), lifetime...) {
			if stored.CollectedAt.After(lastUpdated) {
				lastUpdated = stored.CollectedAt
			}
		}
	}
	return contributions, dated, undated, lastUpdated, nil
}

func externalRepresentationCoverage(buckets, lifetime []storedUsage) (dated, undated bool) {
	bucketByBase := make(map[string][]storedUsage, len(buckets))
	for _, bucket := range buckets {
		key := usageBaseKey(bucket.measurement())
		bucketByBase[key] = append(bucketByBase[key], bucket)
	}
	lifetimeByBase := make(map[string][]storedUsage, len(lifetime))
	for _, item := range lifetime {
		key := usageBaseKey(item.measurement())
		lifetimeByBase[key] = append(lifetimeByBase[key], item)
	}
	keys := make(map[string]struct{}, len(bucketByBase)+len(lifetimeByBase))
	for key := range bucketByBase {
		keys[key] = struct{}{}
	}
	for key := range lifetimeByBase {
		keys[key] = struct{}{}
	}
	for key := range keys {
		group := bucketByBase[key]
		lifetimeGroup := lifetimeByBase[key]
		switch {
		case len(group) == 0 && len(lifetimeGroup) > 0:
			undated = true
		case len(group) > 0 && len(lifetimeGroup) == 0:
			dated = true
		case len(group) > 0 && bucketSetEquivalent(lifetimeGroup[0].measurement(), group):
			dated = true
		case len(group) > 0:
			dated = true
			undated = true
		}
	}
	return dated, undated
}

func chooseExternalRepresentations(buckets, lifetime []storedUsage, filter models.SessionUsageFilter) []storedUsage {
	bucketByBase := make(map[string][]storedUsage, len(buckets))
	for _, bucket := range buckets {
		key := usageBaseKey(bucket.measurement())
		bucketByBase[key] = append(bucketByBase[key], bucket)
	}
	lifetimeByBase := make(map[string][]storedUsage, len(lifetime))
	for _, item := range lifetime {
		key := usageBaseKey(item.measurement())
		lifetimeByBase[key] = append(lifetimeByBase[key], item)
	}
	keys := make(map[string]struct{}, len(bucketByBase)+len(lifetimeByBase))
	for key := range bucketByBase {
		keys[key] = struct{}{}
	}
	for key := range lifetimeByBase {
		keys[key] = struct{}{}
	}
	allTime := filter.Start == nil && filter.End == nil
	result := make([]storedUsage, 0, len(buckets)+len(lifetime))
	for key := range keys {
		group := bucketByBase[key]
		lifetimeGroup := lifetimeByBase[key]
		if allTime {
			if isDatedUsageGroup(filter.GroupBy) {
				// Dated table/chart queries need the source buckets even when a
				// partial import also has a lifetime snapshot. ListSessionUsage
				// computes all-time totals from the lifetime projection below. An
				// undated lifetime value cannot be placed on a dated axis when no
				// bucket exists.
				result = append(result, group...)
				continue
			}
			switch {
			case len(lifetimeGroup) == 0:
				result = append(result, group...)
			case bucketSetEquivalent(lifetimeGroup[0].measurement(), group):
				result = append(result, group...)
			default:
				// A partial dated import cannot stand in for the lifetime
				// snapshot. Keep the cumulative value as its own representation.
				result = append(result, lifetimeGroup...)
			}
			continue
		}

		// A range is represented by dated buckets. Include a lifetime value
		// only when the range has no dated bucket for this source identity;
		// it is then marked as undated by the report metadata.
		result = append(result, group...)
		if filter.IncludeUndated && len(group) == 0 {
			result = append(result, lifetimeGroup...)
		}
	}
	return result
}

func isDatedUsageGroup(group string) bool {
	switch strings.ToLower(group) {
	case usageGroupDay, usageGroupDaily, usageGroupMonth, usageGroupMonthly:
		return true
	default:
		return false
	}
}

func bucketSetEquivalent(lifetime models.SessionUsageMeasurement, buckets []storedUsage) bool {
	if len(buckets) == 0 {
		return false
	}
	var aggregate *models.SessionUsageMeasurement
	for _, stored := range buckets {
		item := stored.measurement()
		if item.Coverage != models.UsageCoverageComplete || item.SourceDate == "" {
			return false
		}
		if aggregate == nil {
			copy := item
			aggregate = &copy
			continue
		}
		mergeMeasurementValues(aggregate, item)
	}
	return aggregate != nil && measurementValuesEqual(lifetime, *aggregate)
}

func usageMetadata(contributions []usageContribution) (dated, undated bool, lastUpdated time.Time) {
	for _, contribution := range contributions {
		item := contribution.measurement
		if item.CoverageKey == "" && item.SourceDate == "" && item.CoverageStart == nil {
			undated = true
		} else {
			dated = true
		}
		if item.CollectedAt.After(lastUpdated) {
			lastUpdated = item.CollectedAt
		}
	}
	return dated, undated, lastUpdated
}

func hasDatedRangeData(contributions []usageContribution, filter models.SessionUsageFilter) bool {
	if filter.Start == nil && filter.End == nil {
		return len(contributions) > 0
	}
	for _, contribution := range contributions {
		item := contribution.measurement
		if item.CoverageKey != "" || item.SourceDate != "" || item.CoverageStart != nil {
			return true
		}
	}
	return false
}

// markMonthlyRangeCoverage records when a monthly row is clipped by the
// selected instant range. The source buckets remain unchanged; this only
// describes the requested projection so callers do not mistake a partial
// calendar month for a complete one or calculate a full-month rate from a
// partial numerator and denominator.
func markMonthlyRangeCoverage(contributions []usageContribution, filter models.SessionUsageFilter) []usageContribution {
	if !isMonthlyUsageGroup(filter.GroupBy) || (filter.Start == nil && filter.End == nil) {
		return contributions
	}
	for i := range contributions {
		item := &contributions[i].measurement
		period := monthUsagePeriod(contributions[i].period)
		if period == "" || !monthlyRangeIsPartial(*item, period, filter) {
			continue
		}
		item.Coverage = mergeCoverage(item.Coverage, models.UsageCoveragePartial)
		item.CostCoverage = mergeCoverage(item.CostCoverage, models.UsageCoveragePartial)
	}
	return contributions
}

func isMonthlyUsageGroup(group string) bool {
	switch strings.ToLower(group) {
	case "month", "monthly":
		return true
	default:
		return false
	}
}

func monthlyRangeIsPartial(item models.SessionUsageMeasurement, period string, filter models.SessionUsageFilter) bool {
	location := time.UTC
	zone := item.SourceTimezone
	if item.SourceDate == "" {
		zone = filter.Timezone
	}
	if zone != "" {
		if loaded, err := time.LoadLocation(zone); err == nil {
			location = loaded
		}
	}
	monthStart, err := time.ParseInLocation("2006-01", period, location)
	if err != nil {
		return true
	}
	monthEnd := monthStart.AddDate(0, 1, 0)
	if filter.Start != nil && filter.Start.After(monthStart.UTC()) {
		return true
	}
	if filter.End != nil && filter.End.Before(monthEnd.UTC()) {
		return true
	}
	return false
}

func usageProviderOptions(contributions []usageContribution) []string {
	seen := make(map[string]struct{})
	for _, contribution := range contributions {
		provider := contribution.measurement.Provider
		if provider == "" {
			provider = unknownProviderFilter
		}
		seen[provider] = struct{}{}
	}
	providers := make([]string, 0, len(seen))
	for provider := range seen {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

func selectUsageContributions(contributions []usageContribution) []usageContribution {
	type selectedUsage struct {
		contribution usageContribution
		ambiguous    bool
	}
	selected := make(map[string]selectedUsage, len(contributions))
	for _, contribution := range contributions {
		key := usageContributionKey(contribution.measurement)
		current, exists := selected[key]
		if !exists {
			selected[key] = selectedUsage{contribution: contribution}
			continue
		}
		if contribution.measurement.Source != current.contribution.measurement.Source ||
			contribution.measurement.SourceRecordID != current.contribution.measurement.SourceRecordID {
			current.ambiguous = true
		}
		merged := mergeEquivalentUsageMeasurements(
			current.contribution.measurement,
			contribution.measurement,
		)
		if usageTokenCandidateBetter(contribution.measurement, current.contribution.measurement) {
			current.contribution = contribution
		}
		current.contribution.measurement = merged
		selected[key] = current
	}
	result := make([]usageContribution, 0, len(selected))
	for _, item := range selected {
		if item.ambiguous {
			item.contribution.measurement.Coverage = mergeCoverage(
				item.contribution.measurement.Coverage, models.UsageCoveragePartial,
			)
			item.contribution.measurement.AttributionStatus = models.UsageAttributionAmbiguous
		}
		result = append(result, item.contribution)
	}
	return resolveOverlappingUsageContributions(sortUsageContributions(result))
}

func sortUsageContributions(result []usageContribution) []usageContribution {
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].measurement, result[j].measurement
		if result[i].period != result[j].period {
			return result[i].period < result[j].period
		}
		if left.TaskID != right.TaskID {
			return left.TaskID < right.TaskID
		}
		if left.SessionID != right.SessionID {
			return left.SessionID < right.SessionID
		}
		if left.Model != right.Model {
			return left.Model < right.Model
		}
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		return left.SourceRecordID < right.SourceRecordID
	})
	return result
}

// resolveOverlappingUsageContributions prevents two source records that claim
// the same session/model/provider interval from being added together. Native
// ledger events are handled separately by canonicalUsageContributions and may
// legitimately share an instant, so this pass only resolves external rows.
func resolveOverlappingUsageContributions(contributions []usageContribution) []usageContribution {
	for i := 0; i < len(contributions); i++ {
		for j := i + 1; j < len(contributions); j++ {
			left, right := contributions[i].measurement, contributions[j].measurement
			if !usageWorkMatches(left, right) || !usageCoverageOverlaps(left, right) {
				continue
			}
			winner, loser := i, j
			if usageCandidateBetter(right, left) {
				winner, loser = j, i
			}
			contributions[winner].measurement.Coverage = mergeCoverage(
				contributions[winner].measurement.Coverage, models.UsageCoveragePartial,
			)
			contributions[winner].measurement.CostCoverage = mergeCoverage(
				contributions[winner].measurement.CostCoverage, models.UsageCoveragePartial,
			)
			contributions[winner].measurement.AttributionStatus = models.UsageAttributionAmbiguous
			contributions = append(contributions[:loser], contributions[loser+1:]...)
			if loser < i {
				i--
				break
			}
			j--
		}
	}
	return sortUsageContributions(contributions)
}

// canonicalUsageContributions resolves external observations against native
// ledger coverage. A native event replaces an external value only when the
// native events cover the same unit. One native event inside a fuller
// external lifetime/day observation therefore cannot erase the external
// measurement.
func canonicalUsageContributions(external, native []usageContribution) []usageContribution {
	external = selectUsageContributions(external)
	if len(native) == 0 {
		return external
	}
	dropNative := make(map[int]bool, len(native))
	keepExternal := make([]usageContribution, 0, len(external))
	for _, candidate := range external {
		matching := make([]int, 0)
		for index, nativeContribution := range native {
			if !nativeCoverageMatches(candidate.measurement, nativeContribution.measurement) {
				continue
			}
			if usageCoverageOverlaps(candidate.measurement, nativeContribution.measurement) {
				matching = append(matching, index)
			}
		}
		if len(matching) == 0 {
			keepExternal = append(keepExternal, candidate)
			continue
		}
		if nativeCoverageEquivalent(candidate.measurement, matching, native) {
			nativeMeasurement := aggregateNativeMeasurements(matching, native)
			// Token and cost authority are selected independently. A native event
			// can provide the authoritative token totals while an external report
			// supplies a provider-reported correction for the same coverage unit.
			merged := mergeEquivalentUsageMeasurements(candidate.measurement, nativeMeasurement)
			candidate.measurement = merged
		} else if !externalCoverageSupersedesNative(candidate.measurement, matching, native) {
			// The external observation overlaps native coverage but does not prove
			// that it covers the same or a larger unit. Keep the complete native
			// events and do not let a partial external row erase them.
			continue
		}
		for _, index := range matching {
			dropNative[index] = true
		}
		keepExternal = append(keepExternal, candidate)
	}
	selected := make([]usageContribution, 0, len(keepExternal)+len(native))
	for index, contribution := range native {
		if !dropNative[index] {
			selected = append(selected, contribution)
		}
	}
	selected = append(selected, keepExternal...)
	return sortUsageContributions(selected)
}

// externalCoverageSupersedesNative selects a complete external projection
// when it covers the whole native interval (or is an unbounded lifetime
// projection). A partial external observation cannot replace a complete native
// event merely because their intervals overlap.
func externalCoverageSupersedesNative(
	candidate models.SessionUsageMeasurement,
	indices []int,
	native []usageContribution,
) bool {
	if usageCoverageRank(candidate.Coverage) < usageCoverageRank(models.UsageCoverageComplete) {
		return false
	}
	start, end, dated := usageCoverageInterval(candidate)
	if !dated {
		return true
	}
	for _, index := range indices {
		nativeStart, nativeEnd, nativeDated := usageCoverageInterval(native[index].measurement)
		if !nativeDated || start.After(nativeStart) || end.Before(nativeEnd) {
			return false
		}
	}
	return true
}

// nativeCoverageEquivalent proves that the native events cover the same
// lifetime total while later native events are still missing. An unbounded
// lifetime observation therefore remains authoritative unless another source
// supplies the same explicit bounded coverage.
func nativeCoverageEquivalent(candidate models.SessionUsageMeasurement, indices []int, native []usageContribution) bool {
	candidateStart, candidateEnd, candidateDated := usageCoverageInterval(candidate)
	if !candidateDated || len(indices) == 0 {
		return false
	}
	type interval struct{ start, end time.Time }
	intervals := make([]interval, 0, len(indices))
	for _, index := range indices {
		start, end, dated := usageCoverageInterval(native[index].measurement)
		if !dated || !start.Before(end) || start.Before(candidateStart) || end.After(candidateEnd) {
			return false
		}
		intervals = append(intervals, interval{start: start, end: end})
	}
	sort.Slice(intervals, func(i, j int) bool {
		if intervals[i].start.Equal(intervals[j].start) {
			return intervals[i].end.Before(intervals[j].end)
		}
		return intervals[i].start.Before(intervals[j].start)
	})
	if !intervals[0].start.Equal(candidateStart) {
		return false
	}
	cursor := candidateStart
	for _, current := range intervals {
		if current.start.After(cursor) {
			return false
		}
		if current.end.After(cursor) {
			cursor = current.end
		}
	}
	return cursor.Equal(candidateEnd)
}

func aggregateNativeMeasurements(indices []int, native []usageContribution) models.SessionUsageMeasurement {
	// The measurement fields are pointers. Clone the first value before merging
	// so aggregation cannot mutate the contribution retained by canonical
	// selection.
	aggregate := cloneUsageMeasurement(native[indices[0]].measurement)
	for _, index := range indices[1:] {
		mergeMeasurementValues(&aggregate, native[index].measurement)
	}
	return aggregate
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

func measurementValuesEqual(left, right models.SessionUsageMeasurement) bool {
	return tokenMeasurementsEqual(left, right) &&
		nullableMeasurementsEqual(left.CostSubcents, right.CostSubcents) &&
		left.Currency == right.Currency && left.CostBasis == right.CostBasis &&
		left.CostCoverage == right.CostCoverage && left.Coverage == right.Coverage
}

func tokenMeasurementsEqual(left, right models.SessionUsageMeasurement) bool {
	return tokenValuesEqual(left, right) && left.Coverage == right.Coverage
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

func usageContributionKey(item models.SessionUsageMeasurement) string {
	coverage := "lifetime"
	if item.CoverageKey != "" {
		coverage = "bucket:" + item.CoverageKey
	} else if item.CoverageStart != nil || item.CoverageEnd != nil {
		start, end := "", ""
		if item.CoverageStart != nil {
			start = item.CoverageStart.UTC().Format(time.RFC3339Nano)
		}
		if item.CoverageEnd != nil {
			end = item.CoverageEnd.UTC().Format(time.RFC3339Nano)
		}
		coverage = "interval:" + start + ":" + end
	}
	return strings.Join([]string{item.TaskID, usageWorkIdentity(item), item.Model, item.Provider, coverage}, "\x00")
}

// usageWorkIdentity keeps source rows distinct after a session is pruned. A
// missing session_id cannot collapse every transcript for the task into one
// contribution; transcript_id or the collector's stable usage identity is the
// remaining work identity in that case.
func usageWorkIdentity(item models.SessionUsageMeasurement) string {
	identities := make([]string, 0, 3)
	if item.SessionID != "" {
		identities = append(identities, "session:"+item.SessionID)
	}
	if item.TranscriptID != "" {
		identities = append(identities, "transcript:"+item.TranscriptID)
	}
	if item.UsageIdentity != "" {
		identities = append(identities, "usage:"+item.UsageIdentity)
	}
	if len(identities) > 0 {
		return strings.Join(identities, "\x1f")
	}
	return usageWorkFallback
}

func usageWorkMatches(left, right models.SessionUsageMeasurement) bool {
	return left.TaskID == right.TaskID &&
		left.Model == right.Model &&
		usageOptionalIdentityMatches(left.SessionID, right.SessionID) &&
		usageWorkTranscriptMatches(left, right) &&
		usageProviderMatches(left.Provider, right.Provider)
}

func usageWorkTranscriptMatches(left, right models.SessionUsageMeasurement) bool {
	if left.TranscriptID != "" || right.TranscriptID != "" {
		return usageOptionalIdentityMatches(left.TranscriptID, right.TranscriptID)
	}
	return usageOptionalIdentityMatches(left.UsageIdentity, right.UsageIdentity)
}

func usageOptionalIdentityMatches(left, right string) bool {
	return left == "" || right == "" || left == right
}

func usageProviderMatches(left, right string) bool {
	return left == right || left == "" || right == ""
}

func mergeEquivalentUsageMeasurements(left, right models.SessionUsageMeasurement) models.SessionUsageMeasurement {
	token := left
	if usageTokenCandidateBetter(right, left) {
		token = right
	}
	cost := left
	if usageCostCandidateBetter(right, left) {
		cost = right
	}
	merged := cloneUsageMeasurement(token)
	merged.CostSubcents = cloneInt64(cost.CostSubcents)
	merged.Currency = cost.Currency
	merged.CostBasis = cost.CostBasis
	merged.CostCoverage = cost.CostCoverage
	if merged.Currency == usageMixedValue {
		merged.CostSubcents = nil
	}
	merged.Estimated = token.Estimated || cost.Estimated
	merged.Stale = token.Stale || cost.Stale
	if token.Source != cost.Source || token.SourceRecordID != cost.SourceRecordID {
		merged.Source = usageMixedValue
	}
	return merged
}

func usageTokenCandidateBetter(candidate, current models.SessionUsageMeasurement) bool {
	if usageCoverageRank(candidate.Coverage) != usageCoverageRank(current.Coverage) {
		return usageCoverageRank(candidate.Coverage) > usageCoverageRank(current.Coverage)
	}
	if tokenCompletenessRank(candidate) != tokenCompletenessRank(current) {
		return tokenCompletenessRank(candidate) > tokenCompletenessRank(current)
	}
	if candidate.Source == usageSourceNative && current.Source != usageSourceNative {
		return true
	}
	if current.Source == usageSourceNative && candidate.Source != usageSourceNative {
		return false
	}
	if candidate.Stale != current.Stale {
		return !candidate.Stale
	}
	if candidate.Source != current.Source {
		return candidate.Source < current.Source
	}
	if candidate.Revision != current.Revision {
		return candidate.Revision > current.Revision
	}
	return candidate.SourceRecordID < current.SourceRecordID
}

func usageCostCandidateBetter(candidate, current models.SessionUsageMeasurement) bool {
	if usageCoverageRank(candidate.CostCoverage) != usageCoverageRank(current.CostCoverage) {
		return usageCoverageRank(candidate.CostCoverage) > usageCoverageRank(current.CostCoverage)
	}
	candidateAvailable := candidate.CostSubcents != nil
	currentAvailable := current.CostSubcents != nil
	if candidateAvailable != currentAvailable {
		return candidateAvailable
	}
	if usageCostBasisRank(candidate.CostBasis) != usageCostBasisRank(current.CostBasis) {
		return usageCostBasisRank(candidate.CostBasis) > usageCostBasisRank(current.CostBasis)
	}
	if candidate.Stale != current.Stale {
		return !candidate.Stale
	}
	if candidate.Revision != current.Revision {
		return candidate.Revision > current.Revision
	}
	return candidate.SourceRecordID < current.SourceRecordID
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

func usageCandidateBetter(candidate, current models.SessionUsageMeasurement) bool {
	if usageCoverageRank(candidate.Coverage) != usageCoverageRank(current.Coverage) {
		return usageCoverageRank(candidate.Coverage) > usageCoverageRank(current.Coverage)
	}
	if tokenCompletenessRank(candidate) != tokenCompletenessRank(current) {
		return tokenCompletenessRank(candidate) > tokenCompletenessRank(current)
	}
	if usageCostCandidateBetter(candidate, current) {
		return true
	}
	if usageCostCandidateBetter(current, candidate) {
		return false
	}
	if candidate.Source == usageSourceNative && current.Source != usageSourceNative {
		return true
	}
	if current.Source == usageSourceNative && candidate.Source != usageSourceNative {
		return false
	}
	if candidate.Stale != current.Stale {
		return !candidate.Stale
	}
	if candidate.Source != current.Source {
		return candidate.Source < current.Source
	}
	if candidate.Revision != current.Revision {
		return candidate.Revision > current.Revision
	}
	return candidate.SourceRecordID < current.SourceRecordID
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

func (r *Repository) listUsageTable(ctx context.Context, table string, filter models.SessionUsageFilter) ([]storedUsage, error) {
	query := `SELECT ` + usageSelectColumns + ` FROM ` + table + ` WHERE workspace_id = ?`
	args := []any{filter.WorkspaceID}
	if filter.Source != "" {
		query += ` AND source = ?`
		args = append(args, filter.Source)
	}
	if len(filter.TaskIDs) > 0 {
		query += ` AND task_id IN (` + placeholders(len(filter.TaskIDs)) + `)`
		for _, id := range filter.TaskIDs {
			args = append(args, id)
		}
	}
	if len(filter.SessionIDs) > 0 {
		query += ` AND session_id IN (` + placeholders(len(filter.SessionIDs)) + `)`
		for _, id := range filter.SessionIDs {
			args = append(args, id)
		}
	}
	if filter.Model != "" {
		query += ` AND model = ?`
		args = append(args, filter.Model)
	}
	if filter.Provider != "" {
		if filter.Provider == unknownProviderFilter {
			query += ` AND (provider = '' OR provider IS NULL)`
		} else {
			query += ` AND provider = ?`
			args = append(args, filter.Provider)
		}
	}
	query += ` ORDER BY COALESCE(coverage_start, observed_at), source_date, collected_at DESC, source_record_id, model, provider`
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]storedUsage, 0)
	for rows.Next() {
		row, err := scanUsageRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row.stored())
	}
	return result, rows.Err()
}

//nolint:cyclop,nestif // Range matching handles lifetime, source-date, and interval coverage forms.
func usageMatchesTime(item models.SessionUsageMeasurement, filter models.SessionUsageFilter) bool {
	if filter.Start == nil && filter.End == nil {
		return true
	}
	if item.CoverageStart == nil {
		if item.SourceDate == "" {
			return filter.IncludeUndated
		}
		location := time.UTC
		if item.SourceTimezone != "" {
			if loaded, err := time.LoadLocation(item.SourceTimezone); err == nil {
				location = loaded
			}
		}
		date, err := time.ParseInLocation("2006-01-02", item.SourceDate, location)
		if err != nil {
			return false
		}
		if filter.Start != nil && !date.AddDate(0, 0, 1).After(*filter.Start) {
			return false
		}
		if filter.End != nil && !date.Before(*filter.End) {
			return false
		}
		return true
	}
	if filter.Start != nil && item.CoverageEnd != nil && !item.CoverageEnd.After(*filter.Start) {
		return false
	}
	if filter.End != nil && !item.CoverageStart.Before(*filter.End) {
		return false
	}
	return true
}

func usageCoverageOverlaps(left, right models.SessionUsageMeasurement) bool {
	leftStart, leftEnd, leftDated := usageCoverageInterval(left)
	rightStart, rightEnd, rightDated := usageCoverageInterval(right)
	if !leftDated || !rightDated {
		// A lifetime or otherwise unbounded measurement may overlap the
		// native event. Keep the native candidate instead of guessing.
		return true
	}
	return leftStart.Before(rightEnd) && rightStart.Before(leftEnd)
}

func usageCoverageInterval(item models.SessionUsageMeasurement) (time.Time, time.Time, bool) {
	if item.CoverageStart != nil && item.CoverageEnd != nil {
		return item.CoverageStart.UTC(), item.CoverageEnd.UTC(), true
	}
	if item.SourceDate == "" {
		return time.Time{}, time.Time{}, false
	}
	location := time.UTC
	if item.SourceTimezone != "" {
		if loaded, err := time.LoadLocation(item.SourceTimezone); err == nil {
			location = loaded
		}
	}
	start, err := time.ParseInLocation("2006-01-02", item.SourceDate, location)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return start.UTC(), start.AddDate(0, 0, 1).UTC(), true
}

func usagePeriod(item models.SessionUsageMeasurement, timezone string) string {
	if item.SourceDate != "" {
		return item.SourceDate
	}
	if item.CoverageStart == nil {
		return ""
	}
	location := time.UTC
	if timezone != "" {
		if loaded, err := time.LoadLocation(timezone); err == nil {
			location = loaded
		}
	}
	return item.CoverageStart.In(location).Format("2006-01-02")
}

// nativeCoverageMatches treats an absent provider as an unknown provider. A
// source report that merged providers cannot be matched only by exact string
// equality, or the same host event would be counted beside the merged report.
// A known provider remains a distinct coverage unit from a different known
// provider.
func nativeCoverageMatches(left, right models.SessionUsageMeasurement) bool {
	if left.TaskID != right.TaskID || left.SessionID != right.SessionID || left.Model != right.Model {
		return false
	}
	return left.Provider == right.Provider || left.Provider == "" || right.Provider == ""
}

func usageTotalsCoverage(partial bool) string {
	if partial {
		return models.UsageCoveragePartial
	}
	return models.UsageCoverageComplete
}

func effectiveCostPerMillion(cost, total *int64, costBasis, currency, coverage, costCoverage string) *float64 {
	if cost == nil || total == nil || *total <= 0 || costBasis == models.UsageCostBasisUnknown || costBasis == models.UsageCostBasisMixed ||
		currency == usageMixedValue || coverage != models.UsageCoverageComplete || costCoverage != models.UsageCoverageComplete {
		return nil
	}
	rate := (float64(*cost) / 10000 / float64(*total)) * 1000000
	return &rate
}

func (r *Repository) hasNativeUsage(ctx context.Context, filter models.SessionUsageFilter) (bool, error) {
	if !r.tableExists(ctx, "task_usage_events") {
		return false, nil
	}
	query := `SELECT EXISTS(
		SELECT 1 FROM task_usage_events e JOIN tasks t ON t.id = e.task_id
		WHERE t.workspace_id = ?`
	args := []any{filter.WorkspaceID}
	query, args = appendNativeUsageFilters(query, args, filter, false)
	query += `)`
	var found bool
	err := r.ro.GetContext(ctx, &found, r.ro.Rebind(query), args...)
	return found, err
}

func (r *Repository) hasExternalUsage(ctx context.Context, filter models.SessionUsageFilter) (bool, error) {
	conditions := ""
	conditionArgs := make([]any, 0)
	if filter.Source != "" {
		conditions += ` AND source = ?`
		conditionArgs = append(conditionArgs, filter.Source)
	}
	if len(filter.TaskIDs) > 0 {
		conditions += ` AND task_id IN (` + placeholders(len(filter.TaskIDs)) + `)`
		for _, id := range filter.TaskIDs {
			conditionArgs = append(conditionArgs, id)
		}
	}
	if len(filter.SessionIDs) > 0 {
		conditions += ` AND session_id IN (` + placeholders(len(filter.SessionIDs)) + `)`
		for _, id := range filter.SessionIDs {
			conditionArgs = append(conditionArgs, id)
		}
	}
	if filter.Model != "" {
		conditions += ` AND model = ?`
		conditionArgs = append(conditionArgs, filter.Model)
	}
	if filter.Provider != "" {
		if filter.Provider == unknownProviderFilter {
			conditions += ` AND (provider = '' OR provider IS NULL)`
		} else {
			conditions += ` AND provider = ?`
			conditionArgs = append(conditionArgs, filter.Provider)
		}
	}
	query := `SELECT EXISTS(
		SELECT 1 FROM ` + usageMeasurementsTable + ` WHERE workspace_id = ?` + conditions + `
		UNION ALL
		SELECT 1 FROM ` + usageBucketsTable + ` WHERE workspace_id = ?` + conditions + `)`
	args := []any{filter.WorkspaceID}
	args = append(args, conditionArgs...)
	args = append(args, filter.WorkspaceID)
	args = append(args, conditionArgs...)
	var found bool
	err := r.ro.GetContext(ctx, &found, r.ro.Rebind(query), args...)
	return found, err
}

func appendNativeUsageFilters(query string, args []any, filter models.SessionUsageFilter, includeTime bool) (string, []any) {
	if len(filter.TaskIDs) > 0 {
		query += ` AND e.task_id IN (` + placeholders(len(filter.TaskIDs)) + `)`
		for _, id := range filter.TaskIDs {
			args = append(args, id)
		}
	}
	if len(filter.SessionIDs) > 0 {
		query += ` AND e.session_id IN (` + placeholders(len(filter.SessionIDs)) + `)`
		for _, id := range filter.SessionIDs {
			args = append(args, id)
		}
	}
	if filter.Model != "" {
		query += ` AND e.model = ?`
		args = append(args, filter.Model)
	}
	if filter.Provider != "" {
		if filter.Provider == unknownProviderFilter {
			query += ` AND (e.provider = '' OR e.provider IS NULL)`
		} else {
			query += ` AND e.provider = ?`
			args = append(args, filter.Provider)
		}
	}
	if includeTime {
		if filter.Start != nil {
			query += ` AND e.occurred_at >= ?`
			args = append(args, filter.Start.UTC())
		}
		if filter.End != nil {
			query += ` AND e.occurred_at < ?`
			args = append(args, filter.End.UTC())
		}
	}
	return query, args
}

func (r *Repository) tableExists(ctx context.Context, table string) bool {
	found, err := db.TableExistsContext(ctx, r.ro, table)
	return err == nil && found
}

func (r *Repository) nativeUsageContributions(ctx context.Context, filter models.SessionUsageFilter) ([]usageContribution, error) {
	if !r.tableExists(ctx, "task_usage_events") {
		return nil, nil
	}
	query := `SELECT e.id, e.task_id, COALESCE(e.session_id, ''), e.model, e.provider,
		CAST(e.occurred_at AS TEXT), e.tokens_in, e.tokens_cached_read,
		e.tokens_cached_write, e.tokens_out, e.tokens_thought, e.tokens_total,
		e.cost_subcents, COALESCE(e.cost_source, 'unpriced'), e.estimated, CAST(e.created_at AS TEXT)
		FROM task_usage_events e JOIN tasks t ON t.id = e.task_id
		WHERE t.workspace_id = ?`
	args := []any{filter.WorkspaceID}
	query, args = appendNativeUsageFilters(query, args, filter, true)
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]usageContribution, 0)
	for rows.Next() {
		var eventID, taskID, sessionID, model, provider, occurred, costSource, created string
		var input int64
		var cachedRead, cachedWrite, output, thought, total, cost sql.NullInt64
		var estimated int
		if err := rows.Scan(&eventID, &taskID, &sessionID, &model, &provider, &occurred, &input, &cachedRead, &cachedWrite, &output, &thought, &total, &cost, &costSource, &estimated, &created); err != nil {
			return nil, err
		}
		occurredAt := parseTimeString(occurred)
		createdAt := parseTimeString(created)
		invalidTokenValue := input < 0
		if input < 0 {
			input = 0
		}
		for _, value := range []*sql.NullInt64{&cachedRead, &cachedWrite, &output, &thought} {
			if value.Valid && value.Int64 < 0 {
				value.Valid = false
				invalidTokenValue = true
			}
		}
		coverage := models.UsageCoverageComplete
		attribution := models.UsageAttributionAttributed
		if cost.Valid && cost.Int64 < 0 {
			cost = sql.NullInt64{}
		}
		costValue, costBasis, costCoverage := nativeCostFields(costSource, cost)
		if invalidTokenValue || !nativeTokenTotalsConsistent(input, cachedRead, cachedWrite, output, thought, total) {
			// Native events predate the source-aware validation boundary. Preserve
			// their usable category values, but reject an inconsistent total from
			// aggregation so a corrupt ledger row cannot inflate or contradict the
			// report.
			total = sql.NullInt64{}
			coverage = models.UsageCoveragePartial
			attribution = models.UsageAttributionAmbiguous
		}
		item := models.SessionUsageMeasurement{
			WorkspaceID: filter.WorkspaceID, TaskID: taskID, SessionID: sessionID,
			Source: usageSourceNative, SourceRecordID: "native:" + eventID,
			UsageIdentity: "native:" + eventID,
			Model:         model, Provider: provider, ObservedAt: occurredAt, CollectedAt: createdAt,
			InputTokens: models.Int64(input), OutputTokens: nullableIntPtr(output),
			CacheReadTokens: nullableIntPtr(cachedRead), CacheWriteTokens: nullableIntPtr(cachedWrite),
			ReasoningTokens: nullableIntPtr(thought), TotalTokens: nullableIntPtr(total),
			Turns:        models.Int64(1),
			CostSubcents: costValue, Currency: "USD", CostBasis: costBasis, CostCoverage: costCoverage,
			Coverage: coverage, AttributionStatus: attribution,
			CoverageStart: &occurredAt, CoverageEnd: timePtr(occurredAt.Add(time.Nanosecond)),
			// Estimated describes the usage event independently of how its
			// cost was priced. Preserve it for unpriced events too.
			Estimated: estimated != 0,
		}
		if item.CollectedAt.IsZero() {
			item.CollectedAt = item.ObservedAt
		}
		result = append(result, usageContribution{measurement: item, period: usagePeriod(item, filter.Timezone), fromNative: true})
	}
	return result, rows.Err()
}

func nativeCostFields(source string, cost sql.NullInt64) (*int64, string, string) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "provider_reported":
		return nullableIntPtr(cost), models.UsageCostBasisReported, costCoverageFor(cost)
	case "models_dev_list":
		return nullableIntPtr(cost), models.UsageCostBasisEstimated, costCoverageFor(cost)
	default:
		// cost_source=unpriced is intentionally different from a reported
		// zero. Keep the token event and make cost unavailable.
		return nil, models.UsageCostBasisUnknown, models.UsageCoverageMissing
	}
}

func costCoverageFor(cost sql.NullInt64) string {
	if !cost.Valid {
		return models.UsageCoverageMissing
	}
	return models.UsageCoverageComplete
}

func nativeTokenTotalsConsistent(input int64, cachedRead, cachedWrite, output, thought, total sql.NullInt64) bool {
	values := []sql.NullInt64{{Valid: true, Int64: input}, cachedRead, cachedWrite, output, thought, total}
	if !nativeTokenValuesNonNegative(values) {
		return false
	}
	if !total.Valid {
		return true
	}
	for _, value := range values[:len(values)-1] {
		if value.Valid && value.Int64 > total.Int64 {
			return false
		}
	}
	return true
}

func nativeTokenValuesNonNegative(values []sql.NullInt64) bool {
	for _, value := range values {
		if value.Valid && value.Int64 < 0 {
			return false
		}
	}
	return true
}

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

func timePtr(value time.Time) *time.Time { return &value }

type usageAccumulator struct {
	row               models.TokenUsageRow
	providers         map[string]struct{}
	inputMissing      bool
	outputMissing     bool
	cacheReadMissing  bool
	cacheWriteMissing bool
	reasoningMissing  bool
	totalMissing      bool
	turnsMissing      bool
	costMissing       bool
}

//nolint:cyclop,gocognit,funlen // The accumulator tracks nullable token fields and coverage dimensions together.
func aggregateUsage(contributions []usageContribution, filter models.SessionUsageFilter) ([]models.TokenUsageRow, models.UsageTotals) {
	group := strings.ToLower(filter.GroupBy)
	if group == "" {
		group = usageGroupModel
	}
	acc := make(map[string]*usageAccumulator)
	for _, contribution := range contributions {
		m := contribution.measurement
		key := usageGroupKey(m, contribution.period, group)
		period := contribution.period
		if group == "month" || group == "monthly" {
			period = monthUsagePeriod(period)
		}
		current := acc[key]
		if current == nil {
			current = &usageAccumulator{row: models.TokenUsageRow{
				Key: key, Period: period, Model: m.Model, Provider: m.Provider,
				TaskID: m.TaskID, SessionID: m.SessionID, Source: m.Source,
				Currency: m.Currency, CostBasis: m.CostBasis, CostCoverage: m.CostCoverage, Coverage: m.Coverage,
				Estimated: m.Estimated, Stale: m.Stale, Undated: contribution.period == "",
			}, providers: map[string]struct{}{m.Provider: {}}}
			acc[key] = current
		}
		current.providers[m.Provider] = struct{}{}
		addNullable(&current.row.InputTokens, &current.inputMissing, m.InputTokens)
		addNullable(&current.row.OutputTokens, &current.outputMissing, m.OutputTokens)
		addNullable(&current.row.CacheReadTokens, &current.cacheReadMissing, m.CacheReadTokens)
		addNullable(&current.row.CacheWriteTokens, &current.cacheWriteMissing, m.CacheWriteTokens)
		addNullable(&current.row.ReasoningTokens, &current.reasoningMissing, m.ReasoningTokens)
		addNullable(&current.row.TotalTokens, &current.totalMissing, m.TotalTokens)
		addNullable(&current.row.Turns, &current.turnsMissing, m.Turns)
		addNullable(&current.row.CostSubcents, &current.costMissing, m.CostSubcents)
		current.row.Estimated = current.row.Estimated || m.Estimated
		current.row.Stale = current.row.Stale || m.Stale
		current.row.Undated = current.row.Undated || contribution.period == ""
		if m.AttributionStatus != "" && m.AttributionStatus != models.UsageAttributionAttributed {
			current.row.Coverage = mergeCoverage(current.row.Coverage, models.UsageCoveragePartial)
		}
		if current.row.Currency != m.Currency && m.Currency != "" {
			current.row.Currency = usageMixedValue
		}
		current.row.CostBasis = mergeCostBasis(current.row.CostBasis, m.CostBasis)
		current.row.CostCoverage = mergeCoverage(current.row.CostCoverage, m.CostCoverage)
		current.row.Coverage = mergeCoverage(current.row.Coverage, m.Coverage)
		if current.row.Model == "" {
			current.row.Model = m.Model
		} else if m.Model != "" && current.row.Model != m.Model && current.row.Model != usageMixedValue {
			current.row.Model = usageMixedValue
		}
		if current.row.Source == "" {
			current.row.Source = m.Source
		} else if m.Source != "" && current.row.Source != m.Source && current.row.Source != usageMixedValue {
			current.row.Source = usageMixedValue
		}
		if len(current.providers) > 1 {
			current.row.Provider = usageMixedValue
		}
	}
	rows := make([]models.TokenUsageRow, 0, len(acc))
	for _, item := range acc {
		if item.inputMissing {
			item.row.InputTokens = nil
		}
		if item.outputMissing {
			item.row.OutputTokens = nil
		}
		if item.cacheReadMissing {
			item.row.CacheReadTokens = nil
		}
		if item.cacheWriteMissing {
			item.row.CacheWriteTokens = nil
		}
		if item.reasoningMissing {
			item.row.ReasoningTokens = nil
		}
		if item.totalMissing {
			item.row.TotalTokens = nil
		}
		if item.turnsMissing {
			item.row.Turns = nil
		}
		if item.costMissing {
			item.row.CostSubcents = nil
		}
		if item.row.Currency == usageMixedValue {
			item.row.CostSubcents = nil
		}
		rows = append(rows, item.row)
	}
	var totals models.UsageTotals
	var inputMissing, outputMissing, cacheReadMissing, cacheWriteMissing bool
	var reasoningMissing, totalMissing, turnsMissing, costMissing bool
	for _, row := range rows {
		addNullable(&totals.InputTokens, &inputMissing, row.InputTokens)
		addNullable(&totals.OutputTokens, &outputMissing, row.OutputTokens)
		addNullable(&totals.CacheReadTokens, &cacheReadMissing, row.CacheReadTokens)
		addNullable(&totals.CacheWriteTokens, &cacheWriteMissing, row.CacheWriteTokens)
		addNullable(&totals.ReasoningTokens, &reasoningMissing, row.ReasoningTokens)
		addNullable(&totals.TotalTokens, &totalMissing, row.TotalTokens)
		addNullable(&totals.Turns, &turnsMissing, row.Turns)
		addNullable(&totals.CostSubcents, &costMissing, row.CostSubcents)
		totals.Estimated = totals.Estimated || row.Estimated
		totals.Partial = totals.Partial || row.Coverage != models.UsageCoverageComplete
		totals.Undated = totals.Undated || row.Undated
		if totals.Currency == "" {
			totals.Currency = row.Currency
		} else if row.Currency != "" && totals.Currency != row.Currency {
			totals.Currency = usageMixedValue
			totals.CostSubcents = nil
		}
		totals.CostBasis = mergeCostBasis(totals.CostBasis, row.CostBasis)
		totals.CostCoverage = mergeCoverage(totals.CostCoverage, row.CostCoverage)
	}
	if totals.Currency == usageMixedValue {
		totals.CostSubcents = nil
	}
	if inputMissing {
		totals.InputTokens = nil
	}
	if outputMissing {
		totals.OutputTokens = nil
	}
	if cacheReadMissing {
		totals.CacheReadTokens = nil
	}
	if cacheWriteMissing {
		totals.CacheWriteTokens = nil
	}
	if reasoningMissing {
		totals.ReasoningTokens = nil
	}
	if totalMissing {
		totals.TotalTokens = nil
	}
	if turnsMissing {
		totals.Turns = nil
	}
	if costMissing {
		totals.CostSubcents = nil
	}
	return rows, totals
}

func addNullable(target **int64, missing *bool, value *int64) {
	if value == nil {
		if missing != nil {
			*missing = true
		}
		return
	}
	if *target == nil {
		copy := *value
		*target = &copy
	} else {
		if *value > 0 && **target > math.MaxInt64-*value {
			*target = nil
			if missing != nil {
				*missing = true
			}
			return
		}
		**target += *value
	}
}

func usageGroupKey(m models.SessionUsageMeasurement, period, group string) string {
	switch group {
	case usageGroupDay, usageGroupDaily:
		return period
	case usageGroupMonth, usageGroupMonthly:
		if len(period) >= 7 {
			return period[:7]
		}
		return period
	case "task":
		return m.TaskID
	case "session":
		return m.SessionID
	case "provider":
		return m.Provider
	case usageGroupModel, "models", "model_provider":
		return strings.Join([]string{m.Model, m.Provider}, "\x00")
	case "detail", "session_model":
		return strings.Join([]string{m.TaskID, m.SessionID, m.Model, m.Provider, m.SourceRecordID, m.CoverageKey}, ":")
	default:
		return m.Model
	}
}

func monthUsagePeriod(period string) string {
	if len(period) >= 7 {
		return period[:7]
	}
	return period
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
	case "task", "task_id":
		if left.TaskID != right.TaskID {
			if ascending {
				return left.TaskID < right.TaskID
			}
			return left.TaskID > right.TaskID
		}
	case "session", "session_id":
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
