package replay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
)

// world is an in-memory set of dependencies for Run tests.
type world struct {
	mu sync.Mutex

	turns     []Turn
	proposals map[string]Proposal
	snapshots map[string]string
	triggers  map[string]string
	titles    map[string]string

	selectErr, profileErr, priceErr, renderErr, baselineErr error
	proposalErr                                             error
	profile                                                 Profile
	pricing                                                 *commoncosts.ModelPricing
	renders                                                 Renders
	spend                                                   SpendReading
	spendErr                                                error
	now                                                     time.Time

	// answer decides a call's reply from the prompt.
	answer  func(prompt string) (Reply, error)
	calls   int
	prompts []string

	rows       map[string]*Stored
	byKey      map[string]string
	nextID     int
	costAdds   []int64
	addCostErr error
	finishErr  error
	insertErr  error
	finishedAt time.Time
	baseline   map[string][]Attempt
}

func newWorld() *world {
	return &world{
		proposals: map[string]Proposal{}, snapshots: map[string]string{}, triggers: map[string]string{}, titles: map[string]string{},
		profile: Profile{ID: "prof", Model: "model-x"},
		pricing: &commoncosts.ModelPricing{InputPerMillion: 1_000_000, OutputPerMillion: 1_000_000},
		renders: Renders{BaselineText: "BASE", BaselineHash: "bh", CandidateText: "CAND", CandidateHash: "ch"},
		spend:   SpendReading{Measurable: true},
		now:     time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		rows:    map[string]*Stored{}, byKey: map[string]string{},
		baseline: map[string][]Attempt{},
	}
}

func (w *world) deps() Deps {
	return Deps{Cases: w, Profiles: w, Prompts: w, Prices: w, Spend: w, Instructions: w, Results: w, Clock: w}
}

func (w *world) harness() *Harness { return New(w.deps()) }

func (w *world) Now() time.Time { return w.now }

func (w *world) SelectTurns(context.Context, string, time.Time) ([]Turn, error) {
	return w.turns, w.selectErr
}

func (w *world) Proposal(_ context.Context, id string) (Proposal, error) {
	if w.proposalErr != nil {
		return Proposal{}, w.proposalErr
	}
	p, ok := w.proposals[id]
	if !ok {
		return Proposal{}, ErrNotFound
	}
	return p, nil
}

func (w *world) Snapshot(_ context.Context, hash string) (string, error) {
	b, ok := w.snapshots[hash]
	if !ok {
		return "", ErrNotFound
	}
	return b, nil
}

func (w *world) TriggerText(_ context.Context, t Turn) (string, error) {
	s, ok := w.triggers[t.ID]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}

func (w *world) TaskTitle(_ context.Context, id string) (string, error) {
	s, ok := w.titles[id]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}

func (w *world) Resolve(context.Context, string) (Profile, error) { return w.profile, w.profileErr }

func (w *world) Lookup(context.Context, string) (commoncosts.ModelPricing, bool, error) {
	if w.priceErr != nil {
		return commoncosts.ModelPricing{}, false, w.priceErr
	}
	if w.pricing == nil {
		return commoncosts.ModelPricing{}, false, nil
	}
	return *w.pricing, true, nil
}

func (w *world) Reading(context.Context, string, time.Time) (SpendReading, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	r := w.spend
	for _, c := range w.costAdds { // the replay's own rows count in the window
		r.WindowSubcents += c
	}
	return r, w.spendErr
}

func (w *world) Render(context.Context, string, Override) (Renders, error) {
	return w.renders, w.renderErr
}

func (w *world) Run(ctx context.Context, _ string, prompt string) (Reply, error) {
	w.mu.Lock()
	w.calls++
	w.prompts = append(w.prompts, prompt)
	f := w.answer
	w.mu.Unlock()
	if f == nil {
		return Reply{Text: "[]"}, nil
	}
	reply, err := f(prompt)
	if err == nil && ctx.Err() != nil {
		return Reply{}, ctx.Err() // a real runner stops at its deadline
	}
	return reply, err
}

func (w *world) Insert(_ context.Context, n NewRow) (Stored, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.insertErr != nil {
		return Stored{}, false, w.insertErr
	}
	if n.DreamID != "" {
		if id, ok := w.byKey[n.DreamID+"/"+n.ItemID]; ok {
			return *w.rows[id], false, nil
		}
	}
	w.nextID++
	id := fmt.Sprintf("row-%d", w.nextID)
	w.rows[id] = &Stored{ID: id, Status: StatusRunning}
	if n.DreamID != "" {
		w.byKey[n.DreamID+"/"+n.ItemID] = id
	}
	return *w.rows[id], true, nil
}

func (w *world) AddCost(_ context.Context, id string, c int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.addCostErr != nil {
		return w.addCostErr
	}
	w.costAdds = append(w.costAdds, c)
	return nil
}

func (w *world) Finish(_ context.Context, id string, r Result, at time.Time) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finishErr != nil {
		return false, w.finishErr
	}
	row := w.rows[id]
	if row == nil || row.Status != StatusRunning {
		return false, nil
	}
	row.Status, row.Result, w.finishedAt = StatusDone, r, at
	return true, nil
}

func (w *world) BaselineAttempts(context.Context, string, string, string, string) (map[string][]Attempt, error) {
	return w.baseline, w.baselineErr
}

func (w *world) totalAdded() int64 {
	var t int64
	for _, c := range w.costAdds {
		t += c
	}
	return t
}

// addCase adds a message turn with one outcome per expectation. Decisions are
// "approved" (reproduced), "rejected" (avoided) per proposal.
func (w *world) addCase(id string, props ...caseProp) {
	t := Turn{ID: id, Trigger: TriggerMessage, SnapshotHash: "snap-" + id}
	w.snapshots["snap-"+id] = "CASE:" + id
	w.triggers[id] = "trigger of " + id
	for i, p := range props {
		pid := fmt.Sprintf("%s-p%d", id, i+1)
		w.proposals[pid] = Proposal{Kind: ProposalMove, TargetTaskID: p.target}
		w.titles[p.target] = "title " + p.target
		t.Outcomes = append(t.Outcomes, Outcome{ProposalID: pid, Decision: p.decision})
	}
	w.turns = append(w.turns, t)
}

type caseProp struct{ target, decision string }

// scripted answers by side and case id: a function over (side, case) to keys.
func scripted(f func(side, caseID string) []string) func(string) (Reply, error) {
	return func(prompt string) (Reply, error) {
		side := "base"
		if strings.Contains(prompt, "CAND") {
			side = "cand"
		}
		caseID := ""
		if i := strings.Index(prompt, "CASE:"); i >= 0 {
			rest := prompt[i+5:]
			caseID = strings.Fields(rest)[0]
		}
		var parts []string
		for _, k := range f(side, caseID) {
			parts = append(parts, fmt.Sprintf(`{"kind":"move","target_task_id":%q}`, strings.TrimPrefix(k, "move|")))
		}
		return Reply{Text: "[" + strings.Join(parts, ",") + "]", PromptTokens: 100, ResponseTokens: 10}, nil
	}
}

var errBoom = errors.New("boom")
