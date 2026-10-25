package recorder

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/coordinator"
)

const (
	recurrenceLookup  = 30 * 24 * time.Hour
	agreementMinRated = 5
	subcentsPerDollar = 10000
)

// AgreementSource reports the owner-rated shadow items of a coordinator in a
// window and how many of them agree with their replay verdict. Until the
// shadow dream and replay work wire one, no item is rated.
type AgreementSource interface {
	Agreement(ctx context.Context, coordinatorID string, since, until time.Time) (rated, agreeing int64, err error)
}

type noAgreement struct{}

func (noAgreement) Agreement(context.Context, string, time.Time, time.Time) (int64, int64, error) {
	return 0, 0, nil
}

// Measures computes the five outcome measures from the outcome, feedback and
// ledger tables through a read connection.
type Measures struct {
	ro        *sqlx.DB
	agreement AgreementSource
}

// NewMeasures builds the reader; a nil source means no rated items.
func NewMeasures(ro *sqlx.DB, agreement AgreementSource) *Measures {
	if agreement == nil {
		agreement = noAgreement{}
	}
	return &Measures{ro: ro, agreement: agreement}
}

// medianCap bounds the proposals the median reads.
var medianCap = 2000

var _ coordinator.OutcomeMeasuresReader = (*Measures)(nil)

// Measures implements coordinator.OutcomeMeasuresReader. The window is the
// rolling days x 24 hours ending at now.
func (m *Measures) Measures(ctx context.Context, coordinatorID string, days int, now time.Time) (*coordinator.OutcomeMeasures, error) {
	until := now.UTC()
	since := until.Add(-time.Duration(days) * 24 * time.Hour)
	out := &coordinator.OutcomeMeasures{Days: days}
	var err error
	if out.ApprovalWithoutEdit, err = m.approvalWithoutEdit(ctx, coordinatorID, since, until); err != nil {
		return nil, fmt.Errorf("approval without edit: %w", err)
	}
	if out.OverrideRecurrence, err = m.overrideRecurrence(ctx, coordinatorID, since, until); err != nil {
		return nil, fmt.Errorf("override recurrence: %w", err)
	}
	if out.DollarsPerMerged, err = m.dollarsPerMerged(ctx, coordinatorID, since, until); err != nil {
		return nil, fmt.Errorf("dollars per merged task: %w", err)
	}
	if out.MedianWaitSeconds, err = m.medianWait(ctx, coordinatorID, since, until); err != nil {
		return nil, fmt.Errorf("median wait: %w", err)
	}
	if out.Agreement, err = m.agreementMeasure(ctx, coordinatorID, since, until); err != nil {
		return nil, fmt.Errorf("agreement: %w", err)
	}
	return out, nil
}

func ratio(num, den int64) coordinator.OutcomeMeasure {
	if den == 0 {
		return coordinator.OutcomeMeasure{NullReason: coordinator.NullNoData}
	}
	v := float64(num) / float64(den)
	return coordinator.OutcomeMeasure{Value: &v, Numerator: num, Denominator: den}
}

func (m *Measures) approvalWithoutEdit(ctx context.Context, coordID string, since, until time.Time) (coordinator.OutcomeMeasure, error) {
	var c struct {
		Den int64 `db:"den"`
		Num int64 `db:"num"`
	}
	err := m.ro.GetContext(ctx, &c, m.ro.Rebind(`
		SELECT COUNT(*) AS den,
		       COALESCE(SUM(CASE WHEN decision IN ('approved', 'undone') AND edited_fields IN ('[]', '') THEN 1 ELSE 0 END), 0) AS num
		FROM coordinator_outcomes
		WHERE coordinator_id = ? AND automatic = ? AND decision IN ('approved', 'edited', 'rejected', 'undone')
		  AND decided_at >= ? AND decided_at <= ?`), coordID, false, since, until)
	if err != nil {
		return coordinator.OutcomeMeasure{}, err
	}
	return ratio(c.Num, c.Den), nil
}

type feedbackKeyRow struct {
	ID        string    `db:"id"`
	Kind      string    `db:"kind"`
	Reason    string    `db:"reason_code"`
	Proposal  string    `db:"proposal_kind"`
	CreatedAt time.Time `db:"created_at"`
}

// overrideRecurrence counts window observations whose pattern key also has an
// earlier observation of the same coordinator within the 30 days before them.
func (m *Measures) overrideRecurrence(ctx context.Context, coordID string, since, until time.Time) (coordinator.OutcomeMeasure, error) {
	var rows []feedbackKeyRow
	if err := m.ro.SelectContext(ctx, &rows, m.ro.Rebind(`
		SELECT id, kind, reason_code, proposal_kind, created_at FROM coordinator_feedback
		WHERE coordinator_id = ? AND created_at >= ? AND created_at <= ? ORDER BY created_at, id`),
		coordID, since.Add(-recurrenceLookup), until); err != nil {
		return coordinator.OutcomeMeasure{}, err
	}
	type key struct{ kind, reason, proposal string }
	previous := map[key]time.Time{}
	var total, recurring int64
	for _, r := range rows {
		k := key{r.Kind, r.Reason, r.Proposal}
		prev, seen := previous[k]
		previous[k] = r.CreatedAt
		if r.CreatedAt.Before(since) {
			continue
		}
		total++
		if seen && !prev.Before(r.CreatedAt.Add(-recurrenceLookup)) {
			recurring++
		}
	}
	return ratio(recurring, total), nil
}

func (m *Measures) dollarsPerMerged(ctx context.Context, coordID string, since, until time.Time) (coordinator.OutcomeMeasure, error) {
	var merged int64
	if err := m.ro.GetContext(ctx, &merged, m.ro.Rebind(`
		SELECT COUNT(DISTINCT task_id) FROM coordinator_outcomes
		WHERE coordinator_id = ? AND kind = 'create_task' AND task_id IS NOT NULL AND merged_at >= ? AND merged_at <= ?`), coordID, since, until); err != nil {
		return coordinator.OutcomeMeasure{}, err
	}
	if merged == 0 {
		return coordinator.OutcomeMeasure{NullReason: coordinator.NullNoData}, nil
	}
	var c struct {
		Turns   int64 `db:"turns"`
		Costed  int64 `db:"costed"`
		Subcent int64 `db:"cost"`
	}
	if err := m.ro.GetContext(ctx, &c, m.ro.Rebind(`
		SELECT COUNT(*) AS turns,
		       COALESCE(SUM(CASE WHEN n > 0 AND unpriced = 0 THEN 1 ELSE 0 END), 0) AS costed,
		       COALESCE(SUM(CASE WHEN n > 0 AND unpriced = 0 THEN cost ELSE 0 END), 0) AS cost
		FROM (
			SELECT t.id,
			       (SELECT COUNT(*) FROM task_usage_events u WHERE u.session_id = t.session_id AND u.turn_id = t.session_turn_id) AS n,
			       (SELECT COUNT(*) FROM task_usage_events u WHERE u.session_id = t.session_id AND u.turn_id = t.session_turn_id AND u.cost_source = 'unpriced') AS unpriced,
			       (SELECT COALESCE(SUM(u.cost_subcents), 0) FROM task_usage_events u WHERE u.session_id = t.session_id AND u.turn_id = t.session_turn_id) AS cost
			FROM coordinator_turns t WHERE t.coordinator_id = ? AND t.started_at >= ? AND t.started_at <= ?
		) per_turn`), coordID, since, until); err != nil {
		return coordinator.OutcomeMeasure{}, err
	}
	if c.Costed < c.Turns {
		return coordinator.OutcomeMeasure{NullReason: coordinator.NullCostUnknown, Numerator: c.Subcent, Denominator: merged}, nil
	}
	v := float64(c.Subcent) / subcentsPerDollar / float64(merged)
	return coordinator.OutcomeMeasure{Value: &v, Numerator: c.Subcent, Denominator: merged}, nil
}

const medianWhere = `FROM coordinator_outcomes o JOIN coordinator_proposals p ON p.id = o.proposal_id
	WHERE o.coordinator_id = ? AND o.automatic = ? AND o.decision IN ('approved', 'edited', 'rejected', 'undone')
	  AND o.decided_at >= ? AND o.decided_at <= ?`

func (m *Measures) medianWait(ctx context.Context, coordID string, since, until time.Time) (coordinator.OutcomeMeasure, error) {
	var qualifying int64
	if err := m.ro.GetContext(ctx, &qualifying, m.ro.Rebind(`SELECT COUNT(*) `+medianWhere), coordID, false, since, until); err != nil {
		return coordinator.OutcomeMeasure{}, err
	}
	if qualifying == 0 {
		return coordinator.OutcomeMeasure{NullReason: coordinator.NullNoData}, nil
	}
	var rows []struct {
		DecidedAt time.Time `db:"decided_at"`
		CreatedAt time.Time `db:"created_at"`
	}
	if err := m.ro.SelectContext(ctx, &rows, m.ro.Rebind(`SELECT o.decided_at AS decided_at, p.created_at AS created_at `+medianWhere+
		` ORDER BY o.decided_at DESC, o.proposal_id DESC LIMIT ?`), coordID, false, since, until, medianCap); err != nil {
		return coordinator.OutcomeMeasure{}, err
	}
	waits := make([]int64, len(rows))
	for i, r := range rows {
		waits[i] = int64(r.DecidedAt.Sub(r.CreatedAt) / time.Second)
	}
	sort.Slice(waits, func(i, j int) bool { return waits[i] < waits[j] })
	median := float64(waits[len(waits)/2])
	if len(waits)%2 == 0 {
		median = float64(floorDiv(waits[len(waits)/2-1]+waits[len(waits)/2], 2))
	}
	used := int64(len(waits))
	return coordinator.OutcomeMeasure{Value: &median, Numerator: used, Denominator: qualifying, Capped: used != qualifying}, nil
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func (m *Measures) agreementMeasure(ctx context.Context, coordID string, since, until time.Time) (coordinator.OutcomeMeasure, error) {
	rated, agreeing, err := m.agreement.Agreement(ctx, coordID, since, until)
	if err != nil {
		return coordinator.OutcomeMeasure{}, err
	}
	switch {
	case rated == 0:
		return coordinator.OutcomeMeasure{NullReason: coordinator.NullNoData}, nil
	case rated < agreementMinRated:
		return coordinator.OutcomeMeasure{NullReason: coordinator.NullTooFew, Numerator: agreeing, Denominator: rated}, nil
	}
	return ratio(agreeing, rated), nil
}
