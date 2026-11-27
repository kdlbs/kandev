package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// The disposable build overlay sets this flag and redirects only the measured
// transaction entry points. Normal builds never reference these helpers.
var writerWorkloadInstrumented = false

type writerWorkloadObservationKey struct{}
type writerWorkloadSpan struct {
	operation string
	admitted  time.Time
}
type writerWorkloadSamples struct {
	Count   int64   `json:"count"`
	TotalMS float64 `json:"total_ms"`
	MaxMS   float64 `json:"max_ms"`
	P50MS   float64 `json:"p50_ms"`
	P95MS   float64 `json:"p95_ms"`
	P99MS   float64 `json:"p99_ms"`
	samples []float64
}
type writerWorkloadObservation struct {
	mu     sync.Mutex
	values map[string]*writerWorkloadSamples
	active map[*sqlx.Tx]writerWorkloadSpan
	random *rand.Rand
}

func newWriterWorkloadObservation() *writerWorkloadObservation {
	return &writerWorkloadObservation{values: make(map[string]*writerWorkloadSamples),
		active: make(map[*sqlx.Tx]writerWorkloadSpan), random: rand.New(rand.NewPCG(1, 2))}
}

func (o *writerWorkloadObservation) record(key string, elapsed time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := o.values[key]
	if s == nil {
		s = &writerWorkloadSamples{}
		o.values[key] = s
	}
	ms := float64(elapsed) / float64(time.Millisecond)
	s.Count++
	s.TotalMS += ms
	s.MaxMS = max(s.MaxMS, ms)
	const capacity = 65536
	if len(s.samples) < capacity {
		s.samples = append(s.samples, ms)
	} else if i := o.random.Int64N(s.Count); i < capacity {
		s.samples[i] = ms
	}
}

func writerWorkloadObserver(ctx context.Context) *writerWorkloadObservation {
	o, _ := ctx.Value(writerWorkloadObservationKey{}).(*writerWorkloadObservation)
	return o
}

func writerWorkloadMeasure(ctx context.Context, operation string, fn func() error) error {
	started := time.Now()
	err := fn()
	if o := writerWorkloadObserver(ctx); o != nil {
		o.record(operation+".call."+writerWorkloadOutcome(err), time.Since(started))
	}
	return err
}

func writerWorkloadOutcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	default:
		return "error"
	}
}

func writerWorkloadBeginTx(ctx context.Context, database *sqlx.DB, operation string) (*sqlx.Tx, error) {
	started := time.Now()
	tx, err := database.BeginTxx(ctx, nil)
	admitted := time.Now()
	if o := writerWorkloadObserver(ctx); o != nil {
		o.record(operation+".entry."+writerWorkloadOutcome(err), admitted.Sub(started))
		if err == nil {
			o.mu.Lock()
			o.active[tx] = writerWorkloadSpan{operation, admitted}
			o.mu.Unlock()
		}
	}
	return tx, err
}

func writerWorkloadCommit(ctx context.Context, tx *sqlx.Tx) error {
	err := tx.Commit()
	writerWorkloadFinishTx(ctx, tx, "commit."+writerWorkloadOutcome(err))
	return err
}

func writerWorkloadRollback(ctx context.Context, tx *sqlx.Tx) error {
	err := tx.Rollback()
	outcome := "rollback." + writerWorkloadOutcome(err)
	if errors.Is(err, sql.ErrTxDone) {
		outcome = "settlement_unobserved"
	}
	writerWorkloadFinishTx(ctx, tx, outcome)
	return err
}

func writerWorkloadFinishTx(ctx context.Context, tx *sqlx.Tx, outcome string) {
	ended := time.Now()
	if o := writerWorkloadObserver(ctx); o != nil {
		o.mu.Lock()
		span, ok := o.active[tx]
		delete(o.active, tx)
		o.mu.Unlock()
		if ok {
			o.record(span.operation+".span."+outcome, ended.Sub(span.admitted))
		}
	}
}

func (o *writerWorkloadObservation) report(t testing.TB, metadata map[string]any) {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, s := range o.values {
		sort.Float64s(s.samples)
		for p, target := range map[float64]*float64{0.50: &s.P50MS, 0.95: &s.P95MS, 0.99: &s.P99MS} {
			if len(s.samples) > 0 {
				*target = s.samples[int(float64(len(s.samples)-1)*p)]
			}
		}
	}
	metadata["measurements"] = o.values
	metadata["instrumented"] = writerWorkloadInstrumented
	metadata["unsettled_observations"] = len(o.active)
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("WRITER_WORKLOAD %s", data)
}

func TestWriterWorkloadObserverTransactions(t *testing.T) {
	repo := newRepoForSessionTests(t)
	observer := newWriterWorkloadObservation()
	ctx := context.WithValue(t.Context(), writerWorkloadObservationKey{}, observer)
	committed, err := writerWorkloadBeginTx(ctx, repo.db, "commit_control")
	if err != nil {
		t.Fatal(err)
	}
	if err := writerWorkloadCommit(ctx, committed); err != nil {
		t.Fatal(err)
	}
	if err := writerWorkloadRollback(ctx, committed); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("settled rollback: %v", err)
	}
	rolledBack, err := writerWorkloadBeginTx(ctx, repo.db, "rollback_control")
	if err != nil {
		t.Fatal(err)
	}
	if err := writerWorkloadRollback(ctx, rolledBack); err != nil {
		t.Fatal(err)
	}
	if len(observer.active) != 0 {
		t.Fatal("observer retained a settled transaction")
	}
	for _, key := range []string{"commit_control.entry.ok", "commit_control.span.commit.ok", "rollback_control.entry.ok", "rollback_control.span.rollback.ok"} {
		if sample := observer.values[key]; sample == nil || sample.Count != 1 {
			t.Fatalf("missing single transaction observation for %s", key)
		}
	}
}
