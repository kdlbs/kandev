package replay

import (
	"context"
	"errors"
	"math"
	"sync"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
)

// errBudget stops a replay: the spend cannot be measured, or the next call
// could reach the ceiling.
var errBudget = errors.New("replay: budget")

// estimateTokens is the byte-based token estimate used when an executor
// reports no tokens and when a call is reserved.
func estimateTokens(bytes int) int64 { return int64(bytes) / 3 }

// callCost prices a run. A run that reports no tokens at all is priced from
// the byte estimates of its prompt and response. A cost whose arithmetic
// overflows is the largest representable one, which exhausts any ceiling.
func callCost(p commoncosts.ModelPricing, r Reply, promptBytes int) int64 {
	in, out := r.PromptTokens, r.ResponseTokens
	if in == 0 && out == 0 {
		in, out = estimateTokens(promptBytes), estimateTokens(len(r.Text))
	}
	cost, ok := commoncosts.CalculateCostSubcentsChecked(in, 0, 0, out, p)
	if !ok {
		return math.MaxInt64
	}
	return cost
}

// callBound is the most a call is expected to cost: the prompt estimate in and
// MaxOutputTokens out.
func callBound(p commoncosts.ModelPricing, promptBytes int) int64 {
	cost, ok := commoncosts.CalculateCostSubcentsChecked(estimateTokens(promptBytes), 0, 0, MaxOutputTokens, p)
	if !ok {
		return math.MaxInt64
	}
	return cost
}

func saturatingAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// budget admits calls under the coordinator's cost ceiling.
type budget struct {
	spend         Spend
	coordinatorID string
	clock         Clock

	mu        sync.Mutex
	inflight  int64
	unwritten int64
}

// reserve admits a call of at most bound, or refuses it with errBudget when
// the 24 hour spend cannot be measured or spend plus the bounds in flight, the
// cost not yet written and this bound reaches the ceiling. No ceiling admits.
func (b *budget) reserve(ctx context.Context, bound int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	reading, err := b.spend.Reading(ctx, b.coordinatorID, b.clock.Now())
	if err != nil || !reading.Measurable {
		return errBudget
	}
	if reading.CeilingSubcents != nil {
		committed := saturatingAdd(saturatingAdd(reading.WindowSubcents, b.inflight), b.unwritten)
		if saturatingAdd(committed, bound) >= *reading.CeilingSubcents {
			return errBudget
		}
	}
	b.inflight = saturatingAdd(b.inflight, bound)
	return nil
}

// claimUnwritten takes the cost of finished calls whose write failed, so
// exactly one write carries it.
func (b *budget) claimUnwritten() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.unwritten
	b.unwritten = 0
	return c
}

// settle ends a call: its bound leaves the in-flight sum. A failed write
// returns the claimed carry and this call's cost to the unwritten sum, so the
// reading keeps seeing them.
func (b *budget) settle(bound, cost, claimed int64, written bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inflight -= bound
	if !written {
		b.unwritten = saturatingAdd(b.unwritten, saturatingAdd(cost, claimed))
	}
}
