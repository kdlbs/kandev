package coordinator

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	spendWindow       = 24 * time.Hour
	spendMeanWindow   = 7 * 24 * time.Hour
	spendMeanDays     = 7
	spendChunkSize    = 500
	turnCostGrace     = 10 * time.Minute
	spendReadTasks    = "tasks"
	spendReadWindow   = "window"
	spendReadMean     = "mean"
	spendMetricFailed = "coordinator_spend_read_failed_total"
)

// ErrSpendScope reports a caller scope error: an empty coordinator or
// workspace id. It is unmeasurable, runs no query and is not a failed read.
var ErrSpendScope = errors.New("coordinator: spend scope is empty")

// SpendLedger is the ledger read surface spend and per-turn cost need.
// Satisfied by the task service.
type SpendLedger interface {
	SumUsageForTasks(ctx context.Context, taskIDs []string, from, to time.Time) (taskmodels.UsageSum, error)
	SumUsageForTurn(ctx context.Context, sessionID, turnID string, notAfter time.Time) (int64, error)
}

// SpendReading is the coordinator's measured spend. Amounts are hundredths of
// a cent. Measurable is false when the read failed (non-nil error, all
// amounts zero) or when the 24-hour window holds an unpriced row (nil error,
// Degraded true, WindowSubcents the priced lower bound).
type SpendReading struct {
	WindowSubcents int64
	Mean7dSubcents int64
	Measurable     bool
	Degraded       bool
	Mean7dKnown    bool
}

// TurnKey identifies one unattended turn's ledger rows.
type TurnKey struct {
	SessionID     string
	SessionTurnID string
	FinishedAt    time.Time
}

var spendReadFailed = spendExpvarMap(spendMetricFailed)

// spendExpvarMap returns the published map called name, creating it when no
// other file of the process has.
func spendExpvarMap(name string) *expvar.Map {
	if m, ok := expvar.Get(name).(*expvar.Map); ok {
		return m
	}
	return expvar.NewMap(name)
}

// spendExpvarInt returns the published counter called name, creating it when
// no other file of the process has.
func spendExpvarInt(name string) *expvar.Int {
	if v, ok := expvar.Get(name).(*expvar.Int); ok {
		return v
	}
	return expvar.NewInt(name)
}

// spendCounterValue reads one key of a spend metric map, or the counter itself
// for an empty key.
func spendCounterValue(name, key string) int64 {
	if key == "" {
		if v, ok := expvar.Get(name).(*expvar.Int); ok {
			return v.Value()
		}
		return 0
	}
	m, ok := expvar.Get(name).(*expvar.Map)
	if !ok {
		return 0
	}
	if v, ok := m.Get(key).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

// SetSpendDeps wires the ledger reader, the active-turn reader and the turn
// canceller that spend and the ceiling stop use. Any may be nil; a nil
// dependency makes the operation that needs it fail closed.
func (s *Service) SetSpendDeps(ledger SpendLedger, turns ActiveTurnReader, canceller TurnCanceller) {
	s.spendLedger = ledger
	s.activeTurns = turns
	s.turnCanceller = canceller
}

// Spend measures the coordinator's spend over [now-24h, now) across every
// conversation task it owns, current and archived. Unmeasurable (non-nil
// error, or Measurable false) is the fail-closed reading; no decision may
// inspect the amounts of an unmeasurable reading.
func (s *Service) Spend(ctx context.Context, c *Coordinator, now time.Time) (SpendReading, error) {
	if c == nil || c.ID == "" || c.WorkspaceID == "" {
		s.logger.Warn("coordinator spend: empty scope")
		return SpendReading{}, ErrSpendScope
	}
	ids, err := s.spendTaskIDs(ctx, c)
	if err != nil {
		return SpendReading{}, err
	}
	window, err := s.sumChunks(ctx, ids, now.Add(-spendWindow), now)
	if err != nil {
		spendReadFailed.Add(spendReadWindow, 1)
		s.logger.Warn("coordinator spend: window read failed", zap.String("coordinator_id", c.ID), zap.Error(err))
		return SpendReading{}, fmt.Errorf("coordinator spend window: %w", err)
	}
	extraWindow, err := s.store.ExtraSpend(ctx, c.ID, now.Add(-spendWindow), now)
	if err != nil {
		spendReadFailed.Add(spendReadWindow, 1)
		s.logger.Warn("coordinator spend: replay spend read failed", zap.String("coordinator_id", c.ID), zap.Error(err))
		return SpendReading{}, fmt.Errorf("coordinator spend replay: %w", err)
	}
	reading := SpendReading{
		WindowSubcents: saturatingAdd(window.CostSubcents, extraWindow),
		Measurable:     !window.HasUnpriced,
		Degraded:       window.HasUnpriced,
	}
	mean, err := s.sumChunks(ctx, ids, now.Add(-spendMeanWindow), now)
	if err != nil {
		spendReadFailed.Add(spendReadMean, 1)
		s.logger.Warn("coordinator spend: 7-day read failed", zap.String("coordinator_id", c.ID), zap.Error(err))
		return reading, nil
	}
	extraMean, err := s.store.ExtraSpend(ctx, c.ID, now.Add(-spendMeanWindow), now)
	if err != nil {
		spendReadFailed.Add(spendReadMean, 1)
		s.logger.Warn("coordinator spend: replay 7-day read failed", zap.String("coordinator_id", c.ID), zap.Error(err))
		return reading, nil
	}
	reading.Mean7dSubcents = saturatingAdd(mean.CostSubcents, extraMean) / spendMeanDays
	reading.Mean7dKnown = true
	return reading, nil
}

func (s *Service) spendTaskIDs(ctx context.Context, c *Coordinator) ([]string, error) {
	if s.conversationTasks == nil || s.spendLedger == nil {
		spendReadFailed.Add(spendReadTasks, 1)
		s.logger.Warn("coordinator spend: dependencies are not wired", zap.String("coordinator_id", c.ID))
		return nil, errors.New("coordinator spend: dependencies are not wired")
	}
	tasks, err := s.conversationTasks.ListCoordinatorOriginTasks(ctx, c.WorkspaceID)
	if err != nil {
		spendReadFailed.Add(spendReadTasks, 1)
		s.logger.Warn("coordinator spend: list conversation tasks failed", zap.String("coordinator_id", c.ID), zap.Error(err))
		return nil, fmt.Errorf("coordinator spend tasks: %w", err)
	}
	var ids []string
	for _, task := range tasks {
		if conversationTaskCoordinatorID(task) == c.ID {
			ids = append(ids, task.ID)
		}
	}
	return ids, nil
}

// sumChunks adds SumUsageForTasks over consecutive chunks of at most 500 ids,
// saturating at MaxInt64 and ORing the unpriced flags. Any chunk failure is a
// failure of the whole sum.
func (s *Service) sumChunks(ctx context.Context, ids []string, from, to time.Time) (taskmodels.UsageSum, error) {
	var total taskmodels.UsageSum
	for start := 0; start < len(ids); start += spendChunkSize {
		end := min(start+spendChunkSize, len(ids))
		part, err := s.spendLedger.SumUsageForTasks(ctx, ids[start:end], from, to)
		if err != nil {
			return taskmodels.UsageSum{}, err
		}
		total.CostSubcents = saturatingAdd(total.CostSubcents, part.CostSubcents)
		total.HasUnpriced = total.HasUnpriced || part.HasUnpriced
	}
	return total, nil
}

func saturatingAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// CheckSpendMeasurable is admission check 3: false (spend_unmeasured) when the
// read failed or the window is degraded.
func CheckSpendMeasurable(reading SpendReading, err error) bool {
	return err == nil && reading.Measurable
}

// CheckCeilingNotReached is admission check 4: false (ceiling_reached) when
// the reading is unmeasurable or the window is at or above the ceiling.
func CheckCeilingNotReached(reading SpendReading, ceilingSubcents int64) bool {
	return reading.Measurable && reading.WindowSubcents < ceilingSubcents
}

// TurnCost sums the priced ledger cost of one turn: rows of its session and
// turn id recorded no later than ten minutes after FinishedAt. known is false,
// with no query, when either id is empty; a read error is (0, false, err).
func (s *Service) TurnCost(ctx context.Context, turn TurnKey) (subcents int64, known bool, err error) {
	if turn.SessionID == "" || turn.SessionTurnID == "" {
		return 0, false, nil
	}
	if s.spendLedger == nil {
		return 0, false, errors.New("coordinator turn cost: ledger is not wired")
	}
	cost, err := s.spendLedger.SumUsageForTurn(ctx, turn.SessionID, turn.SessionTurnID, turn.FinishedAt.Add(turnCostGrace))
	if err != nil {
		return 0, false, fmt.Errorf("coordinator turn cost: %w", err)
	}
	return cost, true, nil
}
