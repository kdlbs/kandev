package routingerr

// providerNeutralRules contain stable evidence that can be emitted by more
// than one adapter. Keep these rules independent from provider IDs so adding a
// new ACP provider does not require copying the same transient fingerprints.
var providerNeutralRules = []rule{
	// A gateway that restricted an account rejects every request with 403 and
	// an account-restriction code or message. The envelope must open the line,
	// optionally inside a labeled JSON message, so an agent quoting the error
	// in prose cannot take its own profile out of rotation; a plain 403 or the
	// code on any other status is a different failure.
	mustRule(
		"provider.account_blocked.v1",
		`(?im)^\s*(?:(?:Internal\s+error|API\s+Error|AI_APICallError|Error):\s*)?(?:\{\s*"message"\s*:\s*")?`+
			`Error\s+from\s+provider\s+[^\s:]+:\s*403\s+Forbidden\b[^\n]*?`+
			`(?:\\?"code\\?"\s*:\s*\\?"user_blocked\\?"|\byour\s+access\s+has\s+been\s+restricted\b)`,
		CodeAccountBlocked,
		ConfHigh,
	),
	mustRule(
		"provider.model_capacity.v1",
		`(?i)(?:\bselected\s+model\b[^\n]{0,96}\bat\s+capacity\b|\bmodel\b[^\n]{0,96}\bat\s+capacity\b|\b(?:model|provider)[_-]?capacity(?:_reached)?\b|\b(?:model|provider)\s+capacity\s+(?:is\s+)?(?:reached|unavailable)\b)`,
		CodeModelCapacity,
		ConfHigh,
	),
	mustRule(
		"provider.network_unavailable.v1",
		`(?i)(?:\b(?:ECONNRESET|ECONNREFUSED|ENETUNREACH|EHOSTUNREACH|ETIMEDOUT|ENOTFOUND)\b|network\s+is\s+unreachable|no\s+route\s+to\s+host|connection\s+(?:reset|refused|timed\s+out)|temporary\s+failure\s+in\s+name\s+resolution|i/o\s+timeout|failed\s+to\s+fetch)`,
		CodeNetworkUnavailable,
		ConfHigh,
	),
	// An AI-SDK error envelope carries the provider's own classification in a
	// structured "error_type" field. The underscore-joined "provider_unavailable"
	// never matches the space-separated prose in provider.unavailable.v1. The
	// error label is required: an assistant chunk is classified on its own, so a
	// bare JSON object that merely carries the field is not a provider
	// diagnostic. It sits before the generic rule so the structured signal
	// outranks bare prose. The line may open with the "Agent encountered an
	// error:" status prefix.
	mustRule(
		"provider.structured_error_type.v1",
		`(?im)^\s*(?:Agent\s+encountered\s+an\s+error:\s*)?(?:(?:Internal\s+error|API\s+Error|AI_APICallError|Error):\s*)\{[^\n]*"error_type"\s*:\s*"provider_unavailable"`,
		CodeProviderUnavailable,
		ConfHigh,
	),
	mustRule(
		"provider.unavailable.v1",
		`(?i)(?:\b(?:provider|service|upstream|gateway)\b[^\n]{0,64}\b(?:temporarily\s+)?unavailable\b|\btemporarily\s+unavailable\b)`,
		CodeProviderUnavailable,
		ConfMedium,
	),
}

func matchProviderNeutralRules(text string) (*Error, bool) {
	if text == "" {
		return nil, false
	}
	for _, r := range providerNeutralRules {
		if r.pattern.MatchString(text) {
			return &Error{
				Code:           r.code,
				Confidence:     r.confidence,
				ClassifierRule: r.id,
			}, true
		}
	}
	return nil, false
}
