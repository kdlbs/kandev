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
	default:
		return ""
	}
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
