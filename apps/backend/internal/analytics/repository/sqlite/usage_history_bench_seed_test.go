//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
)

const usageBenchBatch = 128

type usageBenchSeedRecord struct {
	observation indexedObservation
	native      *usagehistory.NativeUsage
	input       *pluginsdk.UsageObservationV1
}

func (f *usageBenchFixture) seedRange(t testing.TB, prefix string, first, until, nativeCount, externalCount int) {
	t.Helper()
	start := time.Date(first, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(until, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, source := range []struct {
		name  string
		count int
	}{{usageSourceNative, nativeCount}, {"plugin:bench", externalCount}} {
		batch := make([]usageBenchSeedRecord, 0, usageBenchBatch)
		for index := range source.count {
			subject := f.subjects[index%4]
			if source.name != usageSourceNative {
				subject = f.subjects[4+index%2]
			}
			if prefix == "other" {
				subject = f.subjects[12+index%4]
				if source.name != usageSourceNative {
					subject = f.subjects[16+index%2]
				}
			}
			at := usageBenchOccurrence(start, end, index, source.count)
			id := fmt.Sprintf("%s-%s-%08d", prefix, source.name, index)
			measurement := usageBenchMeasurement(subject, id, at, source.name)
			if source.name != usageSourceNative {
				measurement.Model, measurement.Provider = "external-model", "external-provider"
				measurement.InputTokens, measurement.OutputTokens, measurement.TotalTokens = models.Int64(12), models.Int64(8), models.Int64(20)
				measurement.SourceTotalTokens = models.Int64(20)
				measurement.CostSubcents = models.Int64(5)
				if index%2 == 0 {
					measurement.Currency = "EUR"
				}
				if index%11 == 0 {
					measurement.TotalTokens, measurement.SourceTotalTokens, measurement.Coverage = nil, nil, usageCoveragePartial
				}
				if index%13 == 0 {
					measurement.CostSubcents, measurement.CostCoverage = nil, models.UsageCoverageMissing
				}
			}
			// Expected scalar values come from the producer recipe, before any
			// production normalizer, selector, contribution or publication builder.
			f.recordOracle(usageBenchExpectedRecord{Measurement: measurement, Native: source.name == usageSourceNative})
			batch = append(batch, f.seedRecord(t, subject, measurement))
			if len(batch) == usageBenchBatch {
				f.insertBatch(t, batch)
				batch = batch[:0]
			}
		}
		f.insertBatch(t, batch)
	}
}

func usageBenchOccurrence(start, end time.Time, index, count int) time.Time {
	// Divide before multiplying so five-year spans do not overflow int64.
	// Use whole microseconds to retain unique occurrence keys on PostgreSQL.
	span := end.Sub(start).Microseconds()
	step, remainder := span/int64(count), span%int64(count)
	offset := int64(index)*step + int64(index)*remainder/int64(count)
	return start.Add(time.Duration(offset) * time.Microsecond)
}

func usageBenchMeasurement(subject usageBenchSubject, id string, at time.Time, source string) models.SessionUsageMeasurement {
	end := at.Add(time.Nanosecond)
	m := models.SessionUsageMeasurement{
		ID: id, WorkspaceID: subject.workspace, TaskID: subject.task, SessionID: subject.session,
		ScopeID: subject.scope, SubjectID: subject.subject, UsageBindingID: subject.binding,
		ActorID: subject.actor, ActorStatus: subject.actorStatus, ActorKind: "user",
		TranscriptID: subject.transcript, Source: source, SourceRecordID: id, UsageIdentity: id, Model: "native-model", Provider: "native-provider",
		AgentClient: "fixture-agent", SourceVersion: "query-fixture-v1", Revision: 1,
		ObservedAt: at, CollectedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		InputTokens: models.Int64(7), OutputTokens: models.Int64(3), CacheReadTokens: models.Int64(0),
		CacheWriteTokens: models.Int64(0), ReasoningTokens: models.Int64(0), TotalTokens: models.Int64(10),
		SourceTotalTokens: models.Int64(10), TokenSemanticsVersion: 1,
		CostSubcents: models.Int64(2), Currency: "USD", CostBasis: models.UsageCostBasisReported,
		CostCoverage: usageCoverageComplete, Coverage: usageCoverageComplete, AttributionStatus: "attributed",
		CoverageStart: &at, CoverageEnd: &end, SourceTimezone: "UTC", SourceDate: at.Format(time.DateOnly), CoverageKey: at.Format(time.RFC3339Nano),
	}
	if subject.actorStatus == "unknown" {
		m.ActorKind = "unknown"
	}
	return m
}

// Bulk construction imports a retained snapshot for query measurement only.
// It does not claim to measure accepted observation ingestion. Canonical facts,
// all source checkpoints, migration completion, summaries and publication are
// built by the ordinary bounded production worker after this import.
func (f *usageBenchFixture) insertBatch(t testing.TB, records []usageBenchSeedRecord) {
	t.Helper()
	if len(records) == 0 {
		return
	}
	if f.ordinaryAdmission {
		f.admitSeedBatch(t, records)
	} else {
		f.insertSnapshotBatch(t, records)
	}
	for _, record := range records {
		m := record.observation.Measurement
		kind := "external"
		if m.Source == usageSourceNative {
			kind = usageSourceNative
		}
		f.retained[m.ScopeID+":"+kind]++
	}
}

func (f *usageBenchFixture) insertSnapshotBatch(t testing.TB, records []usageBenchSeedRecord) {
	t.Helper()
	ctx := t.Context()
	tx, err := f.database.BeginTxx(ctx, nil)
	usageBenchMust(t, err)
	defer func() { _ = tx.Rollback() }()
	byTable := map[string][][]any{}
	versions := map[string]int64{}
	var items, links [][]any
	for _, record := range records {
		row := record.observation
		m := &row.Measurement
		if _, ok := versions[m.ScopeID]; !ok {
			versions[m.ScopeID], err = f.repo.usageWorkVersionTx(ctx, tx, m.ScopeID)
			usageBenchMust(t, err)
		}
		if record.input != nil {
			target, rejected, err := f.repo.prepareUsageObservationTargetTx(ctx, tx, m.Source, *record.input, *m)
			usageBenchMust(t, err)
			if rejected != "" {
				t.Fatal(rejected)
			}
			usageBenchMust(t, f.repo.deriveExternalUsageAttributionTx(ctx, tx, target, "", m))
			items = append(items, []any{m.Source, row.ItemID, m.ScopeID, m.SubjectID, m.UsageBindingID, row.Table, m.ID, m.Revision, m.PayloadDigest})
		}
		// New native snapshots have passed the same normalizer used by retention;
		// E/H accounting never fabricates turn counts from responses.
		accounting := 0
		if record.native != nil {
			accounting = nativeUsageAccountingRepairVersion
		}
		args := append(usageArgs(*m), row.ItemID, row.WorkKey, row.CoverageKey, row.WorkStatus, row.Metadata, row.EventID, row.Retired, versions[m.ScopeID],
			row.Bundle, row.Segment, row.GroupKey, row.Layout, row.TokenBundle, row.CostBundle, row.ScanKey, usageSegmentSpanSeconds(row), accounting)
		byTable[row.Table] = append(byTable[row.Table], args)
		var liveTask, liveSession any = m.TaskID, m.SessionID
		if f.phase != "" && m.TaskID == f.subjects[2].task {
			liveTask, liveSession = nil, nil
		}
		links = append(links, []any{row.Table, m.ID, m.ScopeID, m.SubjectID, liveTask, liveSession})
	}
	for table, rows := range byTable {
		columns := usageWriteColumns + `,item_id,selection_partition,selection_coverage,work_status,observation_metadata,work_event_id,retired,selection_version,selection_bundle,selection_segment,selection_group,selection_layout,selection_token_bundle,selection_cost_bundle,selection_scan_key,selection_span_seconds,native_accounting_version`
		usageBenchMust(t, bulkUsageBenchRows(tx, table, columns, rows))
	}
	usageBenchMust(t, bulkUsageBenchRows(tx, "usage_observation_items", "source,item_id,scope_id,subject_id,binding_id,source_table,observation_id,revision,payload_digest", items))
	usageBenchMust(t, bulkUsageBenchRows(tx, "usage_live_links", "source_table,observation_id,scope_id,subject_id,live_task_id,live_session_id", links))
	usageBenchMust(t, tx.Commit())
}

func bulkUsageBenchRows(tx *sqlx.Tx, table, columns string, rows [][]any) error {
	for offset := 0; offset < len(rows); offset += usageBenchBatch {
		batch := rows[offset:min(offset+usageBenchBatch, len(rows))]
		var values []string
		var args []any
		for _, row := range batch {
			values = append(values, "("+placeholders(len(row))+")")
			args = append(args, row...)
		}
		query := `INSERT INTO ` + table + ` (` + columns + `) VALUES ` + strings.Join(values, ",")
		if _, err := tx.Exec(tx.Rebind(query), args...); err != nil {
			return err
		}
	}
	return nil
}
