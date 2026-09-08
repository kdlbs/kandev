package dashboard

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
)

// Thresholds (REQ-OFFICE-LOOP-LIVENESS-004, AC-004.8 — echoed verbatim
// in every response so a caller never has to hardcode them).
const (
	officeLoopTriggerOverdueGrace  = 3 * time.Minute
	officeLoopTriggerStrandedGrace = 5 * time.Minute
	officeLoopRunQueuedGrace       = 2 * time.Minute
	officeLoopRunClaimedGrace      = 10 * time.Minute
	officeLoopEvaluationWindow     = 24 * time.Hour
	officeLoopEvidenceCap          = 50
)

// Verdict values, closed set (AC-004.1), precedence order
// unknown -> dead -> degraded -> not_armed -> healthy (AC-004.2).
const (
	VerdictUnknown  = "unknown"
	VerdictDead     = "dead"
	VerdictDegraded = "degraded"
	VerdictNotArmed = "not_armed"
	VerdictHealthy  = "healthy"
)

// Degraded reasons (AC-004.10), the enumerated set
// office_loop_liveness_degraded_total{reason} is labelled with. Per
// OPERATOR DECISION F31, the silent-success evidence read and the
// terminal-shape count read both map to ReasonTerminalReadFailed —
// both ultimately answer "what shape did this run end in", so a
// caller diagnosing a 503 reads one reason for either query rather
// than a distinction that doesn't help them act.
const (
	ReasonActivationReadFailed = "activation_read_failed"
	ReasonTriggerReadFailed    = "trigger_read_failed"
	ReasonRunReadFailed        = "run_read_failed"
	ReasonTerminalReadFailed   = "terminal_read_failed"
)

// LoopHealthDegradedError names which input could not be read
// (AC-004.10). The HTTP layer maps this to a 503 and increments
// office_loop_liveness_degraded_total{reason}.
type LoopHealthDegradedError struct {
	Reason string
	Err    error
}

func (e *LoopHealthDegradedError) Error() string {
	return "loop health degraded (" + e.Reason + "): " + e.Err.Error()
}

func (e *LoopHealthDegradedError) Unwrap() error { return e.Err }

// LoopHealthRepo is the persistence surface EvaluateLoopHealth needs.
// Implemented by *sqlite.Repository.
type LoopHealthRepo interface {
	LoopLivenessActivationChecked() (at time.Time, published bool, err error)
	CountEligibleTriggers(ctx context.Context, workspaceID string) (int, error)
	ListOverdueOrStrandedTriggers(
		ctx context.Context, workspaceID string, now time.Time,
		overdueGrace, strandedGrace time.Duration, limit int,
	) ([]sqlite.TriggerHealthRow, int, error)
	ListStuckRuns(
		ctx context.Context, workspaceID string, now time.Time,
		queuedGrace, claimedGrace time.Duration, limit int,
	) ([]sqlite.StuckRunRow, int, error)
	ListSilentSuccesses(
		ctx context.Context, workspaceID string, windowStart time.Time, limit int,
	) ([]sqlite.SilentSuccessRow, int, error)
	ListTerminalRunsInWindow(
		ctx context.Context, workspaceID string, windowStart time.Time,
	) ([]sqlite.TerminalRunShapeInputRow, error)
}

// TriggerEvidenceDTO is one overdue or stranded trigger row (AC-004.7).
type TriggerEvidenceDTO struct {
	TriggerID   string     `json:"trigger_id"`
	RoutineID   string     `json:"routine_id"`
	RoutineName string     `json:"routine_name"`
	Condition   string     `json:"condition"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`
	LastFiredAt *time.Time `json:"last_fired_at,omitempty"`
}

// StuckRunEvidenceDTO is one claimed-stuck or queued-stuck run row.
type StuckRunEvidenceDTO struct {
	RunID          string     `json:"run_id"`
	AgentProfileID string     `json:"agent_profile_id"`
	Status         string     `json:"status"`
	Condition      string     `json:"condition"`
	StuckSince     *time.Time `json:"stuck_since,omitempty"`
}

// SilentSuccessEvidenceDTO is one finished/processed/sessionless run.
type SilentSuccessEvidenceDTO struct {
	RunID       string     `json:"run_id"`
	RequestedAt time.Time  `json:"requested_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// EvidenceListDTO wraps a capped list with its untruncated total
// (AC-004.9): "capped" is true exactly when total > len(rows).
type triggerEvidenceListDTO struct {
	Rows   []TriggerEvidenceDTO `json:"rows"`
	Total  int                  `json:"total"`
	Cap    int                  `json:"cap"`
	Capped bool                 `json:"capped"`
}

type stuckRunEvidenceListDTO struct {
	Rows   []StuckRunEvidenceDTO `json:"rows"`
	Total  int                   `json:"total"`
	Cap    int                   `json:"cap"`
	Capped bool                  `json:"capped"`
}

type silentSuccessEvidenceListDTO struct {
	Rows   []SilentSuccessEvidenceDTO `json:"rows"`
	Total  int                        `json:"total"`
	Cap    int                        `json:"cap"`
	Capped bool                       `json:"capped"`
}

// LoopHealthThresholdsDTO echoes every threshold and the evaluation
// window used to reach the verdict (AC-004.8), in seconds so the
// response carries no ambiguous duration string.
type LoopHealthThresholdsDTO struct {
	TriggerOverdueGraceSeconds  int `json:"trigger_overdue_grace_seconds"`
	TriggerStrandedGraceSeconds int `json:"trigger_stranded_grace_seconds"`
	RunQueuedGraceSeconds       int `json:"run_queued_grace_seconds"`
	RunClaimedGraceSeconds      int `json:"run_claimed_grace_seconds"`
	EvaluationWindowSeconds     int `json:"evaluation_window_seconds"`
	EvidenceCap                 int `json:"evidence_cap"`
}

// LoopHealthResponse is the GET .../loop-health payload.
type LoopHealthResponse struct {
	Verdict               string                       `json:"verdict"`
	EvaluatedAt           time.Time                    `json:"evaluated_at"`
	Thresholds            LoopHealthThresholdsDTO      `json:"thresholds"`
	EligibleTriggerCount  int                          `json:"eligible_trigger_count"`
	Triggers              triggerEvidenceListDTO       `json:"triggers"`
	StuckRuns             stuckRunEvidenceListDTO      `json:"stuck_runs"`
	SilentSuccesses       silentSuccessEvidenceListDTO `json:"silent_successes"`
	TerminalShapeCounts   map[string]int               `json:"terminal_shape_counts"`
	EvaluationWindowStart time.Time                    `json:"evaluation_window_start"`
}

// allTerminalShapes lists every TerminalShape so the response
// zero-fills unobserved shapes rather than omitting their key,
// mirroring the counter read's zero-fill-by-name rule.
var allTerminalShapes = []service.TerminalShape{
	service.ShapePreActivation,
	service.ShapeLaunchedCompleted,
	service.ShapeLaunchedFailed,
	service.ShapeSilentSuccess,
	service.ShapeUnlaunchedSkipped,
	service.ShapeUnlaunchedFailed,
	service.ShapeUnclassified,
}

// EvaluateLoopHealth answers "is the loop alive" for one workspace
// (REQ-OFFICE-LOOP-LIVENESS-004). now is captured once by the caller
// and passed to every predicate so the trigger, run and terminal reads
// cannot disagree about the instant they were evaluated against
// (AC-004.14). Side-effect free (AC-004.11): every call is a read.
//
// On any unreadable input, returns a *LoopHealthDegradedError naming
// the failing input rather than a partial verdict (AC-004.10).
func EvaluateLoopHealth(
	ctx context.Context, repo LoopHealthRepo, workspaceID string, now time.Time,
) (*LoopHealthResponse, error) {
	thresholds := LoopHealthThresholdsDTO{
		TriggerOverdueGraceSeconds:  int(officeLoopTriggerOverdueGrace.Seconds()),
		TriggerStrandedGraceSeconds: int(officeLoopTriggerStrandedGrace.Seconds()),
		RunQueuedGraceSeconds:       int(officeLoopRunQueuedGrace.Seconds()),
		RunClaimedGraceSeconds:      int(officeLoopRunClaimedGrace.Seconds()),
		EvaluationWindowSeconds:     int(officeLoopEvaluationWindow.Seconds()),
		EvidenceCap:                 officeLoopEvidenceCap,
	}
	windowStart := now.Add(-officeLoopEvaluationWindow)

	activationAt, published, err := repo.LoopLivenessActivationChecked()
	if err != nil {
		return nil, &LoopHealthDegradedError{Reason: ReasonActivationReadFailed, Err: err}
	}
	if !published {
		return &LoopHealthResponse{
			Verdict:               VerdictUnknown,
			EvaluatedAt:           now,
			Thresholds:            thresholds,
			Triggers:              triggerEvidenceListDTO{Rows: []TriggerEvidenceDTO{}, Cap: officeLoopEvidenceCap},
			StuckRuns:             stuckRunEvidenceListDTO{Rows: []StuckRunEvidenceDTO{}, Cap: officeLoopEvidenceCap},
			SilentSuccesses:       silentSuccessEvidenceListDTO{Rows: []SilentSuccessEvidenceDTO{}, Cap: officeLoopEvidenceCap},
			TerminalShapeCounts:   zeroFilledShapeCounts(),
			EvaluationWindowStart: windowStart,
		}, nil
	}

	triggerEvidence, eligibleCount, err := evaluateLoopHealthTriggers(ctx, repo, workspaceID, now)
	if err != nil {
		return nil, err
	}
	stuckEvidence, err := evaluateLoopHealthStuckRuns(ctx, repo, workspaceID, now)
	if err != nil {
		return nil, err
	}
	silentEvidence, err := evaluateLoopHealthSilentSuccesses(ctx, repo, workspaceID, windowStart)
	if err != nil {
		return nil, err
	}
	shapeCounts, err := evaluateLoopHealthTerminalShapes(ctx, repo, workspaceID, windowStart, activationAt, published)
	if err != nil {
		return nil, err
	}

	verdict := decideLoopHealthVerdict(triggerEvidence.Total, eligibleCount, stuckEvidence.Total, silentEvidence.Total)

	return &LoopHealthResponse{
		Verdict:               verdict,
		EvaluatedAt:           now,
		Thresholds:            thresholds,
		EligibleTriggerCount:  eligibleCount,
		Triggers:              triggerEvidence,
		StuckRuns:             stuckEvidence,
		SilentSuccesses:       silentEvidence,
		TerminalShapeCounts:   shapeCounts,
		EvaluationWindowStart: windowStart,
	}, nil
}

// decideLoopHealthVerdict applies the AC-004.2 precedence
// unknown -> dead -> degraded -> not_armed -> healthy. unknown is
// decided by the caller before evidence is even read, so this
// function only sees the four remaining branches.
func decideLoopHealthVerdict(triggerTotal, eligibleCount, stuckTotal, silentTotal int) string {
	switch {
	case triggerTotal > 0:
		return VerdictDead
	case stuckTotal > 0 || silentTotal > 0:
		return VerdictDegraded
	case eligibleCount == 0:
		return VerdictNotArmed
	default:
		return VerdictHealthy
	}
}

func evaluateLoopHealthTriggers(
	ctx context.Context, repo LoopHealthRepo, workspaceID string, now time.Time,
) (triggerEvidenceListDTO, int, error) {
	eligibleCount, err := repo.CountEligibleTriggers(ctx, workspaceID)
	if err != nil {
		return triggerEvidenceListDTO{}, 0, &LoopHealthDegradedError{Reason: ReasonTriggerReadFailed, Err: err}
	}
	rows, total, err := repo.ListOverdueOrStrandedTriggers(
		ctx, workspaceID, now, officeLoopTriggerOverdueGrace, officeLoopTriggerStrandedGrace, officeLoopEvidenceCap,
	)
	if err != nil {
		return triggerEvidenceListDTO{}, 0, &LoopHealthDegradedError{Reason: ReasonTriggerReadFailed, Err: err}
	}
	dtos := make([]TriggerEvidenceDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, TriggerEvidenceDTO{
			TriggerID:   r.TriggerID,
			RoutineID:   r.RoutineID,
			RoutineName: r.RoutineName,
			Condition:   r.Condition,
			NextRunAt:   r.NextRunAt,
			LastFiredAt: r.LastFiredAt,
		})
	}
	return triggerEvidenceListDTO{
		Rows: dtos, Total: total, Cap: officeLoopEvidenceCap, Capped: total > len(dtos),
	}, eligibleCount, nil
}

func evaluateLoopHealthStuckRuns(
	ctx context.Context, repo LoopHealthRepo, workspaceID string, now time.Time,
) (stuckRunEvidenceListDTO, error) {
	rows, total, err := repo.ListStuckRuns(
		ctx, workspaceID, now, officeLoopRunQueuedGrace, officeLoopRunClaimedGrace, officeLoopEvidenceCap,
	)
	if err != nil {
		return stuckRunEvidenceListDTO{}, &LoopHealthDegradedError{Reason: ReasonRunReadFailed, Err: err}
	}
	dtos := make([]StuckRunEvidenceDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, StuckRunEvidenceDTO{
			RunID: r.RunID, AgentProfileID: r.AgentProfileID, Status: r.Status,
			Condition: r.Condition, StuckSince: r.StuckSince,
		})
	}
	return stuckRunEvidenceListDTO{
		Rows: dtos, Total: total, Cap: officeLoopEvidenceCap, Capped: total > len(dtos),
	}, nil
}

// evaluateLoopHealthSilentSuccesses reports ReasonTerminalReadFailed
// on failure, not ReasonRunReadFailed — OPERATOR DECISION F31.
func evaluateLoopHealthSilentSuccesses(
	ctx context.Context, repo LoopHealthRepo, workspaceID string, windowStart time.Time,
) (silentSuccessEvidenceListDTO, error) {
	rows, total, err := repo.ListSilentSuccesses(ctx, workspaceID, windowStart, officeLoopEvidenceCap)
	if err != nil {
		return silentSuccessEvidenceListDTO{}, &LoopHealthDegradedError{Reason: ReasonTerminalReadFailed, Err: err}
	}
	dtos := make([]SilentSuccessEvidenceDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, SilentSuccessEvidenceDTO{
			RunID: r.RunID, RequestedAt: r.RequestedAt, FinishedAt: r.FinishedAt,
		})
	}
	return silentSuccessEvidenceListDTO{
		Rows: dtos, Total: total, Cap: officeLoopEvidenceCap, Capped: total > len(dtos),
	}, nil
}

func evaluateLoopHealthTerminalShapes(
	ctx context.Context, repo LoopHealthRepo, workspaceID string, windowStart time.Time,
	activationAt time.Time, published bool,
) (map[string]int, error) {
	rows, err := repo.ListTerminalRunsInWindow(ctx, workspaceID, windowStart)
	if err != nil {
		return nil, &LoopHealthDegradedError{Reason: ReasonTerminalReadFailed, Err: err}
	}
	counts := zeroFilledShapeCounts()
	for _, r := range rows {
		shape := service.ClassifyTerminalRun(r.Status, r.Outcome, r.SessionID, r.RequestedAt, activationAt, published)
		counts[string(shape)]++
	}
	return counts, nil
}

func zeroFilledShapeCounts() map[string]int {
	counts := make(map[string]int, len(allTerminalShapes))
	for _, s := range allTerminalShapes {
		counts[string(s)] = 0
	}
	return counts
}
