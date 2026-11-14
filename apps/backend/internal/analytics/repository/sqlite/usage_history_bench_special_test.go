//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
)

func (f *usageBenchFixture) seedSpecial(t testing.TB) {
	t.Helper()
	for _, instant := range []string{"2025-03-09T06:59:00Z", "2025-03-09T07:01:00Z", "2025-11-02T05:30:00Z", "2025-11-02T06:30:00Z"} {
		at, err := time.Parse(time.RFC3339, instant)
		usageBenchMust(t, err)
		m := usageBenchMeasurement(f.subjects[0], "dst-"+instant, at, usageSourceNative)
		f.reportSpecial(t, f.subjects[0], m, nil)
		f.recordOracle(usageBenchExpectedRecord{Measurement: m, Native: true})
	}
	for _, day := range []string{"2025-03-30", "2025-12-15"} {
		location, err := time.LoadLocation("Europe/Lisbon")
		usageBenchMust(t, err)
		local, err := time.ParseInLocation(time.DateOnly, day, location)
		usageBenchMust(t, err)
		at, end := local.UTC(), local.AddDate(0, 0, 1).UTC()
		m := usageBenchMeasurement(f.subjects[6], "coarse-"+day, at, "plugin:coarse")
		m.Model, m.Provider, m.SourceTimezone, m.SourceDate = "coarse-model", "coarse-provider", "Europe/Lisbon", day
		m.CoverageEnd, m.CoverageKey = &end, day
		f.reportSpecial(t, f.subjects[6], m, nil)
		f.recordOracle(usageBenchExpectedRecord{Measurement: m})
	}
	at := time.Date(2025, 12, 15, 12, 0, 0, 0, time.UTC)
	f.seedLifetime(t, at)
	unknown := usageBenchMeasurement(f.subjects[9], "unknown-amounts", at, "plugin:unknown")
	unknown.Model, unknown.Provider = "unknown-model", ""
	unknown.InputTokens, unknown.OutputTokens, unknown.TotalTokens, unknown.SourceTotalTokens = nil, nil, nil, nil
	unknown.CacheReadTokens, unknown.CacheWriteTokens, unknown.ReasoningTokens, unknown.CostSubcents = nil, nil, nil, nil
	unknown.Coverage, unknown.CostCoverage = usageCoveragePartial, models.UsageCoverageMissing
	f.reportSpecial(t, f.subjects[9], unknown, nil)
	f.recordOracle(usageBenchExpectedRecord{Measurement: unknown})
	f.seedCorrection(t, at)
	f.seedMixed(t, at)
}

func (f *usageBenchFixture) seedLifetime(t testing.TB, at time.Time) {
	t.Helper()
	for _, index := range []int{7, 8} {
		m := usageBenchMeasurement(f.subjects[index], f.subjects[index].task+"-lifetime", at, "plugin:lifetime")
		m.Model, m.Provider = "lifetime-model", "lifetime-provider"
		if index == 8 {
			m.Model = "alternative-model"
			f.reportSpecial(t, f.subjects[index], m, nil)
			f.recordOracle(usageBenchExpectedRecord{Measurement: m, AllTimeExcluded: true})
		}
		m.ID += "-undated"
		m.SourceRecordID, m.UsageIdentity = m.ID, m.ID
		m.TotalTokens, m.InputTokens, m.OutputTokens, m.SourceTotalTokens = models.Int64(100), models.Int64(70), models.Int64(30), models.Int64(100)
		m.CoverageStart, m.CoverageEnd = nil, nil
		m.SourceDate, m.SourceTimezone, m.CoverageKey = "", "", ""
		f.reportSpecial(t, f.subjects[index], m, nil)
		f.recordOracle(usageBenchExpectedRecord{Measurement: m})
	}
	// Two declared model members share one transcript and coverage, for both
	// dated and lifetime measurements. Provider labels never establish members.
	for _, lifetime := range []bool{false, true} {
		index, source := 10, "plugin:correction"
		if lifetime {
			index, source = 7, "plugin:lifetime"
		}
		for i, model := range []string{"fixture-sonnet", "fixture-opus"} {
			m := usageBenchMeasurement(f.subjects[index], model+source, at, source)
			m.Model, m.Provider = model, "segment-provider"
			m.InputTokens, m.OutputTokens, m.TotalTokens, m.SourceTotalTokens = models.Int64(int64(i+1)*100), models.Int64(0), models.Int64(int64(i+1)*100), models.Int64(int64(i+1)*100)
			if lifetime {
				m.CoverageStart, m.CoverageEnd, m.SourceDate, m.SourceTimezone, m.CoverageKey = nil, nil, "", "", ""
			}
			f.reportSpecial(t, f.subjects[index], m, func(input *pluginsdk.UsageObservationV1) { input.Segment.SegmentID = model })
			f.recordOracle(usageBenchExpectedRecord{Measurement: m})
		}
	}
}

func (f *usageBenchFixture) seedCorrection(t testing.TB, at time.Time) {
	t.Helper()
	subject := f.subjects[10]
	m := usageBenchMeasurement(subject, "corrected-item", at, "plugin:correction")
	m.Model, m.Provider = "old-model", "old-provider"
	m.InputTokens, m.OutputTokens, m.TotalTokens, m.SourceTotalTokens = models.Int64(90), models.Int64(10), models.Int64(100), models.Int64(100)
	f.reportSpecial(t, subject, m, nil)
	m.Model, m.Provider, m.Revision = "corrected-model", "correction-provider", 2
	m.InputTokens, m.TotalTokens, m.SourceTotalTokens = models.Int64(30), models.Int64(40), models.Int64(40)
	f.reportSpecial(t, subject, m, nil)
	f.recordOracle(usageBenchExpectedRecord{Measurement: m})
	// A newer independently numbered item in the same declared meter is an
	// equivalent cumulative snapshot, not an additional 40 tokens.
	m.ID, m.SourceRecordID, m.UsageIdentity, m.Revision = "corrected-overlap", "corrected-overlap", "corrected-overlap", 1
	f.reportSpecial(t, subject, m, nil)
}

func (f *usageBenchFixture) seedMixed(t testing.TB, at time.Time) {
	t.Helper()
	subject := f.subjects[11]
	// Both actors were admitted to this captured session before consumption.
	// The broader external aggregate therefore has mixed attribution, unlike
	// an exact provider_event which correctly inherits the native event actor.
	tx, err := f.database.BeginTxx(t.Context(), nil)
	usageBenchMust(t, err)
	defer func() { _ = tx.Rollback() }()
	usageBenchMust(t, f.repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{ID: subject.binding + "-other-actor", Kind: usagehistory.BindingKindExecution,
		WorkspaceID: subject.workspace, OrganizationID: subject.workspace + "-org", TaskID: subject.task, TaskIncarnationID: subject.nativeScope.TaskIncarnationID,
		SessionID: subject.session, SessionIncarnationID: subject.nativeScope.SessionIncarnationID, ActorID: "user-mixed-second", ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)}))
	usageBenchMust(t, tx.Commit())
	m := usageBenchMeasurement(subject, "mixed-native", at, usageSourceNative)
	m.Model, m.Provider, m.CostSubcents, m.CostCoverage = "mixed-model", "mixed-provider", nil, models.UsageCoverageMissing
	f.reportSpecial(t, subject, m, nil)
	f.recordOracle(usageBenchExpectedRecord{Measurement: m, Native: true, CostAbsent: true})
	day, end := time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), time.Date(2025, 12, 16, 0, 0, 0, 0, time.UTC)
	m.ID, m.SourceRecordID, m.Source, m.UsageIdentity = "mixed-external", "mixed-external", "plugin:mixed", "mixed-external"
	m.CoverageStart, m.CoverageEnd, m.SourceDate, m.CoverageKey = &day, &end, "2025-12-15", "2025-12-15"
	m.InputTokens, m.OutputTokens, m.CacheReadTokens, m.CacheWriteTokens, m.ReasoningTokens, m.TotalTokens, m.SourceTotalTokens = nil, nil, nil, nil, nil, nil, nil
	m.CostSubcents, m.CostCoverage = models.Int64(500), usageCoverageComplete
	f.reportSpecial(t, subject, m, func(input *pluginsdk.UsageObservationV1) {
		input.Segment, input.Tokens, input.Quality.TokenCoverageComplete = nil, nil, false
	})
	f.recordOracle(usageBenchExpectedRecord{Measurement: m, TokenAbsent: true})
}

// Specials always cross actual native/SDK admission. Corrections preserve item
// and segment lineage; a source import does not fabricate their accepted state.
func (f *usageBenchFixture) reportSpecial(t testing.TB, subject usageBenchSubject, m models.SessionUsageMeasurement, configure func(*pluginsdk.UsageObservationV1)) {
	t.Helper()
	record := f.seedRecord(t, subject, m)
	if configure != nil {
		configure(record.input)
		usageBenchSeal(t, record.input)
	}
	var exists int
	usageBenchMust(t, f.database.Get(&exists, f.database.Rebind(`SELECT COUNT(*) FROM `+record.observation.Table+` WHERE id=?`), record.observation.Measurement.ID))
	f.admitSeedBatch(t, []usageBenchSeedRecord{record})
	if exists == 0 {
		kind := "external"
		if m.Source == usageSourceNative {
			kind = usageSourceNative
		}
		f.retained[subject.scope+":"+kind]++
	}
}
