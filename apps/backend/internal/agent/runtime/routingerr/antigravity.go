package routingerr

import (
	"net/http"
	"strings"
	"time"
)

const antigravityResourceExhausted = "RESOURCE_EXHAUSTED"

// Antigravity failure causes. These are the only display prefixes the
// Antigravity classifier emits; the provider rules in rules.go recognize them
// so the existing Kanban and Office consumers classify the same failure from
// its normalized message.
const (
	AntigravityAuthMessage               = "Antigravity authentication required"
	AntigravitySubscriptionMessage       = "Antigravity subscription required"
	AntigravityModelAccessMessage        = "Antigravity model unavailable"
	AntigravityQuotaMessage              = "Antigravity quota reached"
	AntigravityThrottleMessage           = "Antigravity temporarily throttled"
	AntigravityProviderMessage           = "Antigravity provider unavailable"
	AntigravityProviderOverloadedMessage = "Antigravity provider overloaded"
	AntigravityUnknownMessage            = "Antigravity unknown provider failure"
	AntigravityOutcomeUnverifiedMessage  = "Antigravity turn outcome could not be verified"
	AntigravityRuntimeFailureMessage     = "Antigravity runtime failure"
	AntigravityGenericProviderMessage    = "Antigravity provider failure"
)

// AntigravityFailure is the bounded structured evidence available for one
// Antigravity failure. It is built from an ACP error envelope or a native
// error step, never from assistant prose or arbitrary tool output.
type AntigravityFailure struct {
	// RPCCode is the JSON-RPC error code. It is context only: a generic
	// -32000 is never authentication evidence by itself.
	RPCCode int
	// AuthMethodsCached records that the adapter had cached auth methods when
	// the failure arrived. It is context only and never changes the result.
	AuthMethodsCached bool
	HTTPStatus        int
	Status            string
	Reason            string
	Message           string
	// QuotaExhausted is an explicit provider quota token. It is distinct from
	// QuotaRefusal, which is only set for a qualified typed refusal.
	QuotaExhausted bool
	// QuotaRefusal is set only when a qualified native release returned the
	// typed refusal stop reason for this turn.
	QuotaRefusal bool
	ResetHint    *time.Time
	ResetDelay   time.Duration
	OccurredAt   time.Time
	Phase        Phase
}

// ClassifyAntigravity maps structured Antigravity evidence to the existing
// provider-routing catalogue. Missing reset evidence stays unknown; the
// classifier never derives an allowance window from an error code.
func ClassifyAntigravity(in AntigravityFailure) *Error {
	code, confidence, rule := antigravityEvidence(in)
	phase := in.Phase
	if phase == "" {
		phase = PhasePromptSend
	}
	e := &Error{
		Code:           code,
		Confidence:     confidence,
		Phase:          phase,
		ClassifierRule: rule,
		ResetHint:      antigravityResetHint(in),
		RawExcerpt:     Sanitize(in.Message),
	}
	return applyInvariants(e)
}

// AntigravityFailureMessage renders the bounded, normalized display cause for
// a classified Antigravity failure. detail is provider-supplied text that the
// caller has already sanitized and bounded.
func AntigravityFailureMessage(code Code, detail string) string {
	prefix := antigravityMessagePrefix(code)
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return prefix
	}
	lowerPrefix := strings.ToLower(prefix)
	lowerDetail := strings.ToLower(detail)
	if strings.HasPrefix(lowerDetail, lowerPrefix) {
		return detail
	}
	if strings.Contains(lowerPrefix, lowerDetail) {
		return prefix
	}
	return prefix + ": " + detail
}

func antigravityMessagePrefix(code Code) string {
	switch code {
	case CodeAuthRequired:
		return AntigravityAuthMessage
	case CodeSubscriptionRequired:
		return AntigravitySubscriptionMessage
	case CodeModelUnavailable:
		return AntigravityModelAccessMessage
	case CodeQuotaLimited:
		return AntigravityQuotaMessage
	case CodeRateLimited:
		return AntigravityThrottleMessage
	case CodeProviderOverloaded:
		return AntigravityProviderOverloadedMessage
	case CodeProviderUnavailable:
		return AntigravityProviderMessage
	case CodeUnknownProvider:
		return AntigravityUnknownMessage
	case CodeAgentRuntime:
		return AntigravityRuntimeFailureMessage
	default:
		return AntigravityGenericProviderMessage
	}
}

func antigravityEvidence(in AntigravityFailure) (Code, Confidence, string) {
	status := strings.ToUpper(strings.TrimSpace(in.Status))
	reason := strings.ToUpper(strings.TrimSpace(in.Reason))
	message := strings.ToLower(in.Message)
	confidence := antigravityEvidenceConfidence(in)

	switch {
	case antigravityAuthEvidence(in, status, reason, message):
		return CodeAuthRequired, confidence, "antigravity.structured.auth.v1"
	case antigravityQuotaEvidence(in, status, reason, message):
		return CodeQuotaLimited, confidence, "antigravity.structured.quota.v1"
	case antigravitySubscriptionEvidence(in, status, reason, message):
		return CodeSubscriptionRequired, confidence, "antigravity.structured.subscription.v1"
	case antigravityModelAccessEvidence(in, status, reason, message):
		return CodeModelUnavailable, confidence, "antigravity.structured.model_access.v1"
	case antigravityThrottleEvidence(in, status, reason, message):
		return CodeRateLimited, confidence, "antigravity.structured.throttle.v1"
	case antigravityProviderOverloadedEvidence(in, status, message):
		return CodeProviderOverloaded, confidence, "antigravity.structured.provider_overloaded.v1"
	case antigravityProviderFailureEvidence(in, status, reason, message):
		return CodeProviderUnavailable, confidence, "antigravity.structured.provider_failure.v1"
	default:
		return CodeUnknownProvider, ConfLow, "antigravity.structured.unknown.v1"
	}
}

func antigravityEvidenceConfidence(in AntigravityFailure) Confidence {
	if in.Status != "" || in.Reason != "" || in.HTTPStatus != 0 ||
		in.QuotaExhausted || in.QuotaRefusal {
		return ConfHigh
	}
	return ConfMedium
}

func antigravityAuthEvidence(in AntigravityFailure, status, reason, message string) bool {
	if in.HTTPStatus == http.StatusUnauthorized {
		return true
	}
	if antigravityTokenMatches(status,
		"UNAUTHENTICATED", "AUTHENTICATION_REQUIRED", "ACCESS_TOKEN_EXPIRED",
		"INVALID_CREDENTIALS", "LOGIN_REQUIRED") {
		return true
	}
	if antigravityTokenMatches(reason,
		"UNAUTHENTICATED", "AUTHENTICATION_REQUIRED", "ACCESS_TOKEN_EXPIRED",
		"INVALID_CREDENTIALS", "LOGIN_REQUIRED") {
		return true
	}
	return antigravityMessageMatches(message,
		"authentication required", "not authenticated", "please sign in",
		"sign-in required", "failed to authenticate", "token has expired",
		"invalid_grant", "invalid api key", "unauthorized")
}

func antigravitySubscriptionEvidence(in AntigravityFailure, status, reason, message string) bool {
	if in.HTTPStatus == http.StatusPaymentRequired {
		return true
	}
	if antigravityTokenMatches(status,
		"PAYMENT_REQUIRED", "SUBSCRIPTION_REQUIRED", "NOT_ENTITLED", "ENTITLEMENT_REQUIRED") {
		return true
	}
	if antigravityTokenMatches(reason,
		"PAYMENT_REQUIRED", "SUBSCRIPTION_REQUIRED", "NOT_ENTITLED", "ENTITLEMENT_REQUIRED") {
		return true
	}
	return antigravityMessageMatches(message,
		"subscription required", "subscription has expired", "no active subscription",
		"subscription is not active", "not entitled", "entitlement", "payment required",
		"upgrade your plan", "plan does not include", "billing")
}

func antigravityModelAccessEvidence(in AntigravityFailure, status, reason, message string) bool {
	if antigravityTokenMatches(status,
		"MODEL_NOT_FOUND", "MODEL_UNAVAILABLE", "MODEL_ACCESS_DENIED",
		"MODEL_NOT_AVAILABLE", "MODEL_NOT_SUPPORTED") {
		return true
	}
	if antigravityTokenMatches(reason,
		"MODEL_NOT_FOUND", "MODEL_UNAVAILABLE", "MODEL_ACCESS_DENIED",
		"MODEL_NOT_AVAILABLE", "MODEL_NOT_SUPPORTED") {
		return true
	}
	if in.HTTPStatus == http.StatusNotFound && strings.Contains(message, "model") {
		return true
	}
	if antigravityMessageMatches(message,
		"model not found", "model unavailable", "model is unavailable",
		"model not available", "model access denied", "model not supported",
		"access to this model") {
		return true
	}
	return strings.Contains(message, "model") && antigravityMessageMatches(message,
		"does not have access", "not included", "not available for this account")
}

func antigravityQuotaEvidence(in AntigravityFailure, status, reason, message string) bool {
	if in.QuotaExhausted || in.QuotaRefusal {
		return true
	}
	if antigravityTokenMatches(status,
		"QUOTA_EXHAUSTED", "QUOTA_EXCEEDED", "USAGE_LIMIT_EXCEEDED",
		"BILLING_QUOTA_EXCEEDED") {
		return true
	}
	if antigravityTokenMatches(reason,
		"QUOTA_EXHAUSTED", "QUOTA_EXCEEDED", "USAGE_LIMIT_EXCEEDED",
		"BILLING_QUOTA_EXCEEDED") {
		return true
	}
	if strings.Contains(message, "quota") &&
		antigravityMessageMatches(message,
			"quota exceeded", "quota exhausted", "quota reached",
			"usage limit", "billing quota") {
		return true
	}
	if strings.Contains(message, "usage limit") &&
		antigravityMessageMatches(message, "exceeded", "reached") {
		return true
	}
	return status == antigravityResourceExhausted && strings.Contains(message, "quota")
}

func antigravityThrottleEvidence(in AntigravityFailure, status, reason, message string) bool {
	if in.HTTPStatus == http.StatusTooManyRequests {
		return true
	}
	if antigravityTokenMatches(status,
		antigravityResourceExhausted, "RATE_LIMIT_EXCEEDED", "TOO_MANY_REQUESTS",
		"THROTTLED", "RATE_LIMITED") {
		return true
	}
	if antigravityTokenMatches(reason,
		antigravityResourceExhausted, "RATE_LIMIT_EXCEEDED", "TOO_MANY_REQUESTS",
		"THROTTLED", "RATE_LIMITED") {
		return true
	}
	return antigravityMessageMatches(message,
		"rate limit", "rate-limit", "too many requests", "throttled", "throttling")
}

func antigravityProviderOverloadedEvidence(in AntigravityFailure, status, message string) bool {
	if in.HTTPStatus == statusOverloaded || status == "OVERLOADED" {
		return true
	}
	return antigravityMessageMatches(message, "overloaded")
}

func antigravityProviderFailureEvidence(in AntigravityFailure, status, reason, message string) bool {
	switch in.HTTPStatus {
	case http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	if antigravityTokenMatches(status,
		"INTERNAL", "UNAVAILABLE", "DEADLINE_EXCEEDED", "ABORTED", "SERVER_ERROR") {
		return true
	}
	if antigravityTokenMatches(reason,
		"INTERNAL", "UNAVAILABLE", "DEADLINE_EXCEEDED", "ABORTED", "SERVER_ERROR") {
		return true
	}
	return antigravityMessageMatches(message,
		"service unavailable", "provider unavailable", "deadline exceeded", "server error")
}

func antigravityResetHint(in AntigravityFailure) *time.Time {
	if in.ResetHint != nil && !in.ResetHint.IsZero() {
		hint := *in.ResetHint
		return &hint
	}
	if in.ResetDelay > 0 && !in.OccurredAt.IsZero() {
		hint := in.OccurredAt.Add(in.ResetDelay)
		return &hint
	}
	return nil
}

func antigravityTokenMatches(value string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func antigravityMessageMatches(value string, fragments ...string) bool {
	return antigravityTokenMatches(value, fragments...)
}
