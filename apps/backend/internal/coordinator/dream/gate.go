package dream

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Gate results other than pass.
const (
	GatePass               = "pass"
	GateSize               = "size"
	GateThinEvidence       = "thin_evidence"
	GateCitationUnresolved = "citation_unresolved"
	GateCredential         = "credential"
	GateForbiddenTarget    = "forbidden_target"
	GateNoTarget           = "no_target"
	GateBadTarget          = "bad_target"
)

// CitedTurn is a ledger turn a citation names.
type CitedTurn struct {
	CoordinatorID string
	Trigger       string
	StartedAt     time.Time
}

// StandingOrderRef is a standing order a target names.
type StandingOrderRef struct {
	CoordinatorID string
	Retired       bool
}

// GateInput is everything the gate reads; it holds no model and no store.
type GateInput struct {
	CoordinatorID string
	WindowStart   time.Time
	WindowEnd     time.Time
	// ContextLimit is the coordinator context's rune limit.
	ContextLimit int
	// Turn resolves a cited turn id; false when it does not exist.
	Turn func(id string) (CitedTurn, bool)
	// Order resolves a standing order id; false when it does not exist.
	Order func(id string) (StandingOrderRef, bool)
	// HasCredential reports a credential pattern in text.
	HasCredential func(text string) bool
}

var protectedTargets = map[string]bool{
	"permissions": true, "watches": true, "watch_scope": true, "autonomy": true,
	"ceiling": true, "cost_ceiling": true, "tool_profile": true, "context_human": true,
}

var forbiddenPhrase = regexp.MustCompile(`(?i)\b(raise|lower|change|set|disable|enable|turn|grant|revoke|widen|narrow|edit|remove)\s+(the\s+|its\s+|your\s+)?(cost\s+)?(ceiling|autonomy|permissions?|watch(es| scope)?|tool profile)\b`)

// Check returns GatePass or the first failing reason. It is pure: the same
// item and evidence give the same result.
func Check(it Item, in GateInput) string {
	limit := MaxItemTextRunes
	if it.Kind == KindContextDiff {
		limit = in.ContextLimit
	}
	if utf8.RuneCountInString(it.Text) > limit {
		return GateSize
	}
	if distinct(it.CitedTurnIDs) < MinCitedTurns {
		return GateThinEvidence
	}
	if !citationsResolve(it.CitedTurnIDs, in) {
		return GateCitationUnresolved
	}
	if in.HasCredential != nil && (in.HasCredential(it.Text) || in.HasCredential(it.TargetID)) {
		return GateCredential
	}
	if forbidden(it) {
		return GateForbiddenTarget
	}
	return checkTarget(it, in)
}

func citationsResolve(ids []string, in GateInput) bool {
	for _, id := range ids {
		if !citationResolves(id, in) {
			return false
		}
	}
	return true
}

func checkTarget(it Item, in GateInput) string {
	switch it.Kind {
	case KindNoteUpdate, KindNoteRetire:
		return GateNoTarget
	case KindStandingOrderRetire:
		o, ok := in.Order(it.TargetID)
		if it.TargetID == "" || !ok || o.Retired || o.CoordinatorID != in.CoordinatorID {
			return GateBadTarget
		}
	}
	return GatePass
}

func distinct(ids []string) int {
	seen := map[string]struct{}{}
	for _, id := range ids {
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}

func citationResolves(id string, in GateInput) bool {
	t, ok := in.Turn(id)
	if !ok || t.CoordinatorID != in.CoordinatorID || t.Trigger == "dream" {
		return false
	}
	return !t.StartedAt.Before(in.WindowStart) && t.StartedAt.Before(in.WindowEnd)
}

func forbidden(it Item) bool {
	if protectedTargets[strings.ToLower(strings.TrimSpace(it.TargetID))] {
		return true
	}
	if it.Kind == KindStandingOrderAdd || it.Kind == KindContextDiff {
		return forbiddenPhrase.MatchString(it.Text)
	}
	return false
}
