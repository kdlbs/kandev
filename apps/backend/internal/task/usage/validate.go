package usage

// validateStage implements AC-27's validate stage: four sub-checks run in
// a fixed order, so an event failing more than one is still classified
// deterministically by whichever check it fails first. Returns "" when the
// payload passes every check.
func validateStage(p *usageEventPayload) string {
	switch {
	case p.UsageEventID == "":
		return dropReasonInvalid
	case p.TaskID == "":
		return dropReasonUnattributable
	case hasNegativeValue(p.Usage):
		return dropReasonInvalid
	case hasTokenTotalOverflow(p.Usage):
		return dropReasonOverflow
	case !validNativeUsageObservation(p):
		return dropReasonInvalid
	default:
		return ""
	}
}

func validNativeUsageObservation(p *usageEventPayload) bool {
	o := p.UsageObservation
	if o == nil {
		return true
	}
	if !validNativeUsageIdentity(p, o) || !validNativeUsageSource(p, o) {
		return false
	}
	if o.Scope != "direct" && o.Scope != "child" {
		return false
	}
	return validNativeUsageDetails(p, o)
}

func validNativeUsageIdentity(p *usageEventPayload, o *nativeUsageObservationPayload) bool {
	return p.Usage != nil && p.TurnID != "" && o.SchemaVersion == 1 && o.ProviderThreadID != "" && o.ProviderTurnID != ""
}

func validNativeUsageSource(p *usageEventPayload, o *nativeUsageObservationPayload) bool {
	switch o.Source {
	case "response":
		return o.ProviderResponseID != "" && o.Completeness == "exact"
	case "turn_fallback":
		return o.ProviderResponseID == "" && o.Completeness == "estimated" && p.Usage.Estimated
	default:
		return false
	}
}

func validNativeUsageDetails(p *usageEventPayload, o *nativeUsageObservationPayload) bool {
	if !validReasoningOutputTokens(p.Usage.OutputTokens, o.ReasoningOutputTokens) {
		return false
	}
	if !validNonNegativeOptional(o.ReportedCacheWriteTokens) || !validNonNegativeOptional(o.ReportedTotalTokens) {
		return false
	}
	return o.ReportedCacheWriteTokens == nil || *o.ReportedCacheWriteTokens == 0 || o.PriceSuppressed
}

func validReasoningOutputTokens(outputTokens int64, reasoning *int64) bool {
	return reasoning == nil || (*reasoning >= 0 && *reasoning <= outputTokens)
}

func validNonNegativeOptional(value *int64) bool {
	return value == nil || *value >= 0
}

// hasNegativeValue reports whether any token or cost value on u is
// negative (AC-27, sub-check 3). A nil Usage is a defensive-only case -
// publishPromptUsage never publishes with a nil usage object (AC-24) - and
// contributes no negative value.
func hasNegativeValue(u *promptUsagePayload) bool {
	if u == nil {
		return false
	}
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.CachedReadTokens < 0 ||
		u.CachedWriteTokens < 0 || u.ThoughtTokens < 0 {
		return true
	}
	return u.ProviderReportedCostPresent && u.ProviderReportedCostSubcents < 0
}

func hasTokenTotalOverflow(u *promptUsagePayload) bool {
	if u == nil {
		return false
	}
	_, ok := sumTokenCounts(u.InputTokens, u.CachedReadTokens, u.CachedWriteTokens, optionalOutputTokens(u), u.ThoughtTokens)
	return !ok
}

func optionalOutputTokens(u *promptUsagePayload) int64 {
	if !u.OutputTokensPresent {
		return 0
	}
	return u.OutputTokens
}
