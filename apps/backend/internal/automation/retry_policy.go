package automation

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const maxRetryCount int64 = 9223372036854775806

// NormalizeRetryPolicy validates and fills defaults for a persisted retry
// policy. Decimal fields are canonicalized so equivalent requests serialize
// identically and can be compared during idempotent updates.
func NormalizeRetryPolicy(policy RetryPolicy) (RetryPolicy, error) {
	if policy.Mode == "" {
		policy.Mode = RetryModeDisabled
	}
	if policy.Backoff == "" {
		policy.Backoff = RetryBackoffFixed
	}
	if policy.HistoryMode == "" {
		policy.HistoryMode = RetryHistoryAttempts
	}
	if err := validateRetryPolicyEnums(policy); err != nil {
		return RetryPolicy{}, err
	}

	delay, err := parseRetryDecimal(policy.DelaySeconds)
	if err != nil {
		return RetryPolicy{}, fmt.Errorf("invalid retry delay: %w", err)
	}
	if policy.Mode == RetryModeDisabled {
		policy.MaxRetries = "0"
		policy.DelaySeconds = "0"
		policy.Backoff = RetryBackoffFixed
		policy.HistoryMode = RetryHistoryAttempts
		return policy, nil
	}
	if policy.Mode == RetryModeFinite {
		maxRetries, parseErr := parseRetryDecimal(policy.MaxRetries)
		if parseErr != nil || maxRetries == 0 || maxRetries > maxRetryCount {
			return RetryPolicy{}, fmt.Errorf("finite retry count must be between 1 and %d", maxRetryCount)
		}
		policy.MaxRetries = strconv.FormatInt(maxRetries, 10)
	} else {
		policy.MaxRetries = "0"
	}
	policy.DelaySeconds = strconv.FormatInt(delay, 10)
	return policy, nil
}

func validateRetryPolicyEnums(policy RetryPolicy) error {
	switch policy.Mode {
	case RetryModeDisabled, RetryModeFinite, RetryModeInfinite:
	default:
		return fmt.Errorf("invalid retry mode %q", policy.Mode)
	}
	switch policy.Backoff {
	case RetryBackoffFixed, RetryBackoffExponential:
	default:
		return fmt.Errorf("invalid retry backoff %q", policy.Backoff)
	}
	switch policy.HistoryMode {
	case RetryHistoryAttempts, RetryHistoryTimeline:
	default:
		return fmt.Errorf("invalid retry history mode %q", policy.HistoryMode)
	}
	return nil
}

func parseRetryDecimal(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%q is not a non-negative decimal", raw)
		}
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is outside signed 64-bit range", raw)
	}
	return value, nil
}

func decodeRetryPolicy(raw string) (RetryPolicy, error) {
	var policy RetryPolicy
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &policy); err != nil {
			return RetryPolicy{}, fmt.Errorf("decode retry policy: %w", err)
		}
	}
	return NormalizeRetryPolicy(policy)
}
