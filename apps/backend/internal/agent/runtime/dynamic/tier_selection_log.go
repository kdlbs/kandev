package dynamic

import (
	"time"

	"go.uber.org/zap"
)

// tierSelectionLogMessage is one decision's per-candidate usage facts. It is
// emitted only for a profile that ranks by usage; the facts let an operator
// explain an unexpected selection without reading provider data.
const tierSelectionLogMessage = "dynamic tier selection"

// tierSelectionLogRow is the bounded public shape of one candidate in that
// line. It carries measured numbers and route identity only: no provider text,
// account reference, credential or secret.
type tierSelectionLogRow struct {
	CandidateID    string     `json:"candidate_id"`
	Tier           int        `json:"tier"`
	Mode           string     `json:"mode"`
	Eligible       bool       `json:"eligible"`
	Reason         string     `json:"reason,omitempty"`
	UsageKnown     bool       `json:"usage_known"`
	RemainingPct   *float64   `json:"remaining_pct,omitempty"`
	RateKnown      bool       `json:"rate_known"`
	RatePctPerHour *float64   `json:"rate_pct_per_hour,omitempty"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
	PaceScore      *float64   `json:"pace_score,omitempty"`
}

// logTierSelection writes the facts observed for this decision. observe is nil
// unless a usage tier was in play, so an order-only profile writes nothing.
func (e *Engine) logTierSelection(
	profile Profile,
	sessionID string,
	generation int64,
	reason string,
	selected string,
	observe func() []TierPreviewEntry,
	now time.Time,
) {
	if e == nil || e.logger == nil || observe == nil {
		return
	}
	entries := observe()
	if len(entries) == 0 {
		return
	}
	rows := make([]tierSelectionLogRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, entry.selectionLogRow(now))
	}
	e.logger.Info(tierSelectionLogMessage,
		zap.String("session_id", sessionID),
		zap.String("logical_profile_id", profile.ID),
		zap.Int64("generation", generation),
		zap.String("reason", reason),
		zap.String("selected_candidate_id", selected),
		zap.Any("candidates", rows),
	)
}

func (entry TierPreviewEntry) selectionLogRow(now time.Time) tierSelectionLogRow {
	row := tierSelectionLogRow{
		CandidateID:    entry.CandidateID,
		Tier:           entry.Tier,
		Mode:           entry.Mode,
		Eligible:       entry.Eligible,
		Reason:         entry.Reason,
		UsageKnown:     entry.UsageKnown,
		RemainingPct:   entry.RemainingPct,
		RateKnown:      entry.ConsumptionRateKnown,
		RatePctPerHour: entry.ConsumptionPctPerHour,
	}
	if entry.resetAtKnown {
		reset := entry.resetAt
		row.ResetAt = &reset
	}
	if entry.Mode == tierModePace && entry.UsageKnown && entry.RemainingPct != nil {
		score := usageScore(entry, tierModePace, now)
		row.PaceScore = &score
	}
	return row
}
