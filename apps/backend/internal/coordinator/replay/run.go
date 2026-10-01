package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
)

// Harness replays decided turns against a candidate.
type Harness struct{ d Deps }

// New returns a harness over the dependencies.
func New(d Deps) *Harness { return &Harness{d: d} }

var candidateKinds = map[string]bool{
	KindContextDiff: true, KindStandingOrderAdd: true, KindStandingOrderRetir: true,
	KindNoteAdd: true, KindNoteUpdate: true, KindNoteRetire: true,
}

func (h *Harness) validate(req Request) error {
	d := h.d
	if d.Cases == nil || d.Profiles == nil || d.Prompts == nil || d.Prices == nil ||
		d.Spend == nil || d.Instructions == nil || d.Results == nil || d.Clock == nil {
		return errors.New("replay: nil dependency")
	}
	if req.CoordinatorID == "" {
		return errors.New("replay: empty coordinator id")
	}
	if !candidateKinds[req.Candidate.Kind] {
		return fmt.Errorf("replay: candidate kind %q is not replayable", req.Candidate.Kind)
	}
	if (req.DreamID == "") != (req.ItemID == "") {
		return errors.New("replay: dream id and item id are set together")
	}
	return nil
}

func candidateHash(c Candidate) string {
	raw, _ := json.Marshal(struct {
		Kind     string `json:"kind"`
		Text     string `json:"text"`
		TargetID string `json:"target_id"`
	}{c.Kind, c.Text, c.TargetID})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// run is the state of one replay.
type run struct {
	h       *Harness
	req     Request
	rowID   string
	result  Result
	spent   int64
	profile Profile
	pricing commoncosts.ModelPricing
	bud     *budget

	mu sync.Mutex
}

// Run replays the candidate over the coordinator's decided turns. An error is
// returned only when nothing was started (a caller bug, an insert failure, a
// replay already running) or the result row could not be written; then the
// Result is still valid but must not be stored as a verdict.
func (h *Harness) Run(ctx context.Context, req Request) (Result, error) {
	if err := h.validate(req); err != nil {
		return Result{}, err
	}
	now := h.d.Clock.Now()
	stored, inserted, err := h.d.Results.Insert(ctx, NewRow{
		CoordinatorID: req.CoordinatorID, DreamID: req.DreamID, ItemID: req.ItemID,
		PromptVersion: PromptVersion, CreatedAt: now,
	})
	if err != nil {
		return Result{}, fmt.Errorf("replay: insert result row: %w", err)
	}
	if !inserted {
		if stored.Status == StatusDone {
			return stored.Result, nil
		}
		return Result{RowID: stored.ID}, ErrReplayRunning
	}
	r := &run{h: h, req: req, rowID: stored.ID}
	r.result = Result{RowID: stored.ID, CandidateHash: candidateHash(req.Candidate), CitedTurnIDs: sortedCopy(req.Candidate.CitedTurnIDs)}
	r.bud = &budget{spend: h.d.Spend, coordinatorID: req.CoordinatorID, clock: h.d.Clock}
	r.execute(ctx)
	return r.finish(ctx)
}

func detached(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

// stop marks the replay unmeasured with a reason, unless one was set first.
func (r *run) stop(reason string) {
	if r.result.Reason == "" {
		r.result.Guard, r.result.Verdict, r.result.Reason = GuardUnmeasured, VerdictUnmeasured, reason
	}
}

// stopRead is the stop of a failed read: cancelled when the caller cancelled.
func (r *run) stopRead(ctx context.Context) {
	if ctx.Err() != nil {
		r.stop(ReasonCancelled)
		return
	}
	r.stop(ReasonReadFailed)
}

func (r *run) execute(ctx context.Context) {
	d := r.h.d
	runCtx, cancel := context.WithTimeout(ctx, ReplayTimeout)
	defer cancel()
	profile, err := d.Profiles.Resolve(runCtx, r.req.CoordinatorID)
	if err != nil {
		r.stopRead(ctx)
		return
	}
	r.profile = profile
	r.result.Model = profile.Model
	if profile.AutoApprove || profile.Prefix != "" || len(profile.EnabledFlags) > 0 {
		r.stop(ReasonProfileUnsafe)
		return
	}
	if !r.lookupPrice(runCtx) {
		r.stop(ReasonCostUnknown)
		return
	}
	renders, err := d.Instructions.Render(runCtx, r.req.CoordinatorID, Override{
		Kind: r.req.Candidate.Kind, Text: r.req.Candidate.Text, TargetID: r.req.Candidate.TargetID})
	if errors.Is(err, ErrNoTarget) {
		r.stop(ReasonNoTarget)
		return
	}
	if err != nil {
		r.stopRead(ctx)
		return
	}
	r.result.CandidateHash, r.result.BaselineHash = candidateHash(r.req.Candidate), renders.BaselineHash
	r.replay(ctx, runCtx, renders)
}

func (r *run) lookupPrice(ctx context.Context) bool {
	if r.profile.Model == "" {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, PriceTimeout)
	defer cancel()
	p, found, err := r.h.d.Prices.Lookup(pctx, r.profile.Model)
	if err != nil || !found {
		return false
	}
	r.pricing = p
	return true
}

// replay selects and prepares the cases, then runs them.
func (r *run) replay(ctx, runCtx context.Context, renders Renders) {
	d := r.h.d
	turns, err := d.Cases.SelectTurns(runCtx, r.req.CoordinatorID, d.Clock.Now())
	if err != nil {
		r.stopRead(ctx)
		return
	}
	var prepared []preparedCase
	for _, t := range turns {
		pc, err := prepareCase(runCtx, d.Cases, t)
		if err != nil {
			r.stopRead(ctx)
			return
		}
		prepared = append(prepared, pc)
	}
	var live []*liveCase
	for _, pc := range prepared {
		r.result.Cases = append(r.result.Cases, pc.record)
		if pc.record.Skip != "" {
			r.result.Skipped = append(r.result.Skipped, Skip{TurnID: pc.record.TurnID, Reason: pc.record.Skip})
			continue
		}
		live = append(live, &liveCase{state: pc.state, input: pc.input, rec: len(r.result.Cases) - 1})
	}
	switch {
	case len(turns) == 0:
		r.stop(ReasonNoCases)
		return
	case len(live) == 0:
		r.stop(ReasonAllSkipped)
		return
	}
	reuse, err := d.Results.BaselineAttempts(runCtx, r.req.CoordinatorID, renders.BaselineHash, r.profile.Model, PromptVersion)
	if err != nil {
		r.stopRead(ctx)
		return
	}
	r.dispatch(ctx, runCtx, renders, live, reuse)
	r.conclude(live)
}

// finish writes the row and returns the result.
func (r *run) finish(ctx context.Context) (Result, error) {
	d := r.h.d
	r.result.CostSubcents = r.spent
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), FinalWriteTimeout)
	defer cancel()
	matched, err := d.Results.Finish(wctx, r.rowID, r.result, d.Clock.Now())
	if err != nil {
		r.result.Notes = append(r.result.Notes, "final_write_failed")
		return r.result, fmt.Errorf("%w: %v", ErrResultNotStored, err)
	}
	if !matched {
		r.result.Notes = append(r.result.Notes, "final_write_matched_no_row")
		return r.result, ErrResultNotStored
	}
	return r.result, nil
}
