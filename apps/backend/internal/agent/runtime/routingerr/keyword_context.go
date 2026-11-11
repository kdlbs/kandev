package routingerr

import "time"

// KeywordContext is invocation-owned evidence for the opt-in fallback classifier.
// Diagnostic identifies a provider failure, never assistant prose or a quote.
// Unknown observations retain their zero values and cannot widen a suspension.
type KeywordContext struct {
	// HTTPStatus is newly projected metadata, used after legacy known rules.
	HTTPStatus          int
	Enabled             bool
	Diagnostic          bool
	AccountID           string
	Tier                string
	ModelID             string
	PathID              string
	FreeDaily           bool
	ConsecutiveFailures int
	EmptyOutput         bool
	OtherPaths          int
	OtherModels         int
}

// KeywordDecision describes fallback defaults. Saved policies and replay
// admission remain authoritative at the recovery boundary.
type KeywordDecision struct {
	UntilModelChange bool
	Group            string
	Scope            string
	Backoff          time.Duration
	Action           string
	Context          KeywordContext
}

func keywordDecision(group keywordGroup, in Input) *KeywordDecision {
	c := in.KeywordContext
	decision := &KeywordDecision{Group: string(group), Scope: "path_model", Action: "try_next", Context: c}
	decision.UntilModelChange = group == groupGone && (in.HTTPStatus == 404 || c.HTTPStatus == 404)
	if in.Phase == PhaseStreaming && c.ConsecutiveFailures < 2 {
		decision.Action = "retry_same"
	}
	if group == groupAccount {
		decision.Action = "stop"
	}
	if group == groupLimit && c.AccountID != "" && c.Tier == "subscription" {
		decision.Scope = "account"
	}
	if group == groupCapacity && c.OtherPaths > 0 && c.OtherModels == 0 && c.ModelID != "" {
		decision.Scope = "model"
	}
	decision.Backoff = keywordBackoff(group, c.ConsecutiveFailures)
	return decision
}

func keywordBackoff(group keywordGroup, consecutiveFailures int) time.Duration {
	if group != groupCapacity && group != groupLimit {
		return 0
	}
	steps := []time.Duration{5 * time.Minute, 15 * time.Minute, 30 * time.Minute}
	index := consecutiveFailures - 1
	if index < 0 {
		index = 0
	}
	if index >= len(steps) {
		index = len(steps) - 1
	}
	return steps[index]
}
