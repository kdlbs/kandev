package dynamic

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"sort"
	"time"
)

const (
	tierModeCost       = "cost"
	tierModeRandom     = "random"
	tierModeRoundRobin = "round_robin"
)

// WithTierRandomPick supplies a deterministic picker without exposing a wire seed.
func WithTierRandomPick(pick func(int) int) EngineOption {
	return func(e *Engine) { e.tierPick = pick }
}

// WithTierPreviewPick supplies an independent deterministic picker for the
// read-only preview. Preview never consumes the execution picker, so sampling
// a preview cannot change the next execution's random sequence.
func WithTierPreviewPick(pick func(int) int) EngineOption {
	return func(e *Engine) { e.tierPreviewPick = pick }
}

type rankedTierRow struct {
	candidate Candidate
	entry     TierPreviewEntry
}

// TierPreview contains only public candidate identities and configured metadata.
type TierPreview struct {
	State       string             `json:"state"`
	CandidateID string             `json:"candidate_id,omitempty"`
	ObservedAt  time.Time          `json:"observed_at"`
	Considered  []TierPreviewEntry `json:"considered"`
}

type TierPreviewEntry struct {
	CandidateID           string   `json:"candidate_id"`
	Tier                  int      `json:"tier"`
	Mode                  string   `json:"mode"`
	CostClass             string   `json:"cost_class"`
	Eligible              bool     `json:"eligible"`
	Reason                string   `json:"reason,omitempty"`
	Selected              bool     `json:"selected"`
	UsageKnown            bool     `json:"usage_known"`
	RemainingPct          *float64 `json:"remaining_pct,omitempty"`
	ConsumptionRateKnown  bool     `json:"consumption_rate_known"`
	ConsumptionPctPerHour *float64 `json:"consumption_pct_per_hour,omitempty"`
	// resetAt is the proved reset behind the remaining reading. It is kept
	// internal because the preview's public shape is unchanged by this work.
	resetAt      time.Time
	resetAtKnown bool
}

func candidateCost(candidate Candidate) string {
	var selection struct {
		Model struct {
			Cost string `json:"cost"`
		} `json:"model"`
	}
	if json.Unmarshal(candidate.SelectionJSON, &selection) != nil {
		return "unknown"
	}
	switch selection.Model.Cost {
	case "free", "subscription", "metered":
		return selection.Model.Cost
	default:
		return "unknown"
	}
}

func configuredCostRank(cost string) int {
	switch cost {
	case "free":
		return 0
	case "subscription":
		return 1
	case "metered":
		return 2
	default:
		return 3
	}
}

// rankTierRows is pure: disabled/health/chain exclusions precede a random draw.
// It never changes the source rows or probes a provider.
func tierEligibilityReason(candidate Candidate, excluded map[string]bool, preferred string, unavailable map[string]string) string {
	switch {
	case !candidate.Enabled:
		return "disabled"
	case excluded[candidate.ID] && candidate.ID != preferred:
		return "tried"
	case preferred != "" && candidate.ID != preferred:
		return "not_requested"
	default:
		return unavailable[candidate.ID]
	}
}

func tierHasEligibleRows(tier orderedTier, excluded map[string]bool, preferred string, unavailable map[string]string) bool {
	for _, candidate := range tier.rows {
		if tierEligibilityReason(candidate, excluded, preferred, unavailable) == "" {
			return true
		}
	}
	return false
}

func rankTierRows(tier orderedTier, number int, excluded map[string]bool, preferred string, unavailable map[string]string, pick func(int) int, history map[string]int64, now time.Time) []rankedTierRow {
	rows := make([]rankedTierRow, 0, len(tier.rows))
	for _, candidate := range tier.rows {
		reason := tierEligibilityReason(candidate, excluded, preferred, unavailable)
		rows = append(rows, rankedTierRow{candidate: candidate, entry: TierPreviewEntry{
			CandidateID: candidate.ID, Tier: number, Mode: tier.mode, CostClass: candidateCost(candidate), Eligible: reason == "", Reason: reason,
		}})
	}
	applyTierUsage(rows, now)
	if tier.mode == tierModeCost {
		sort.SliceStable(rows, func(i, j int) bool {
			return configuredCostRank(rows[i].entry.CostClass) < configuredCostRank(rows[j].entry.CostClass)
		})
	}
	if tier.mode == tierModeRandom {
		promoteRandomTierRow(rows, pick)
	}
	if tier.mode == tierModeRoundRobin {
		rows = rotateRoundRobinRows(rows, history)
	}
	return rankUsageRows(rows, tier.mode, now)
}

func promoteRandomTierRow(rows []rankedTierRow, pick func(int) int) {
	var eligible []int
	for index, row := range rows {
		if row.entry.Eligible {
			eligible = append(eligible, index)
		}
	}
	if len(eligible) < 2 {
		return
	}
	if pick == nil {
		pick = rand.IntN
	}
	index := pick(len(eligible))
	if index < 0 || index >= len(eligible) {
		index = 0
	}
	position := eligible[index]
	chosen := rows[position]
	copy(rows[1:position+1], rows[:position])
	rows[0] = chosen
}

// selectionAvailability is an observation, including whether an expired circuit
// can be probed. Only candidateSelectable may actually acquire that probe.
func (r *CircuitRegistry) selectionAvailability(candidate Candidate, now time.Time) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.circuits[accountBlockKey(candidate.ID)].held() {
		return "account_blocked"
	}
	entry := r.circuits[candidate.BindingKey]
	if candidate.BindingKey == "" || entry.state == CircuitClosed {
		return ""
	}
	// Match IsOpen before observing whether an exclusive probe is available.
	if entry.state != CircuitHalfOpen && !entry.held() && (entry.state != CircuitOpen || entry.until.IsZero()) {
		return ""
	}
	if entry.held() || now.Before(entry.until) {
		return "circuit_open"
	}
	if now.Before(entry.probeUntil) {
		return "circuit_probing"
	}
	return ""
}

func (e *Engine) tierAvailability(profile Profile, now time.Time) map[string]string {
	result := make(map[string]string, len(profile.Candidates))
	for _, candidate := range profile.Candidates {
		if normalizedCandidateUsage(candidate.Usage, now).Limited {
			result[candidate.ID] = "usage_limited"
			continue
		}
		result[candidate.ID] = e.circuits.selectionAvailability(candidate, now)
	}
	return result
}

func (e *Engine) PreviewTiers(profile Profile) (TierPreview, error) {
	return e.PreviewTiersContext(context.Background(), profile)
}

func (e *Engine) PreviewTiersContext(ctx context.Context, profile Profile) (TierPreview, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	preview := TierPreview{State: "disabled", ObservedAt: e.now(), Considered: []TierPreviewEntry{}}
	if !e.orderedTiers {
		return preview, nil
	}
	tiers, err := deriveOrderedTiers(profile.Candidates, e.tierUsageAvailable)
	if err != nil {
		return preview, err
	}
	unavailable := e.tierAvailability(profile, preview.ObservedAt)
	// A preview sample must not draw from the execution random source. With no
	// explicit preview picker, use a local source seeded for this observation.
	pick := e.tierPreviewPick
	if pick == nil {
		seed := uint64(preview.ObservedAt.UnixNano())
		pick = rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)).IntN
	}
	var history map[string]int64
	var loaded bool
	for index, tier := range tiers {
		unreached := tier.mode == tierModeRoundRobin && !loaded && preview.CandidateID != ""
		if tier.mode == tierModeRoundRobin && !loaded && !unreached && tierHasEligibleRows(tier, nil, "", unavailable) {
			history, err = e.tierSelectionHistory(ctx, profile.ID, []orderedTier{tier})
			if err != nil {
				preview.State = "observation_unknown"
				return preview, err
			}
			loaded = true
		}
		preview.appendTierRows(rankTierRows(tier, index+1, nil, "", unavailable, pick, history, preview.ObservedAt), unreached)
	}
	preview.State = "no_candidates"
	if len(preview.Considered) > 0 {
		preview.State = "no_eligible"
	}
	if preview.CandidateID != "" {
		preview.State = "selected"
	}
	return preview, nil
}

func (p *TierPreview) appendTierRows(rows []rankedTierRow, unreached bool) {
	for _, row := range rows {
		entry := row.entry
		if unreached && entry.Eligible {
			entry.Eligible = false
			entry.Reason = "not_reached"
		}
		if entry.Eligible && p.CandidateID == "" {
			entry.Selected = true
			p.CandidateID = entry.CandidateID
		}
		p.Considered = append(p.Considered, entry)
	}
}
