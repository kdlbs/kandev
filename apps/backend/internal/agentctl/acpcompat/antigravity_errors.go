package acpcompat

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// AntigravityConfigRelDir is the vendor config root, home-relative. It
	// contains the shared native conversation store.
	AntigravityConfigRelDir = ".gemini/antigravity-acp"
	// AntigravityConversationsDir holds one SQLite journal per native session.
	AntigravityConversationsDir = "conversations"
	// AntigravityErrorStepType is the private native trajectory step type for
	// an error message. It is not an ACP contract.
	AntigravityErrorStepType = 17
	// AntigravityMaxErrorPayloadBytes bounds the native error payload read.
	AntigravityMaxErrorPayloadBytes = 16 * 1024
	// AntigravityMaxErrorMessageBytes bounds the extracted provider message.
	AntigravityMaxErrorMessageBytes = 2048
	// AntigravityQuotaRefusalVersion is the qualified native release whose
	// typed refusal stop reason maps to quota exhaustion.
	AntigravityQuotaRefusalVersion = "1.4.5"
)

const antigravityQuotaReason = "QUOTA_EXHAUSTED"

// AntigravityErrorEnvelope is the bounded structured evidence extracted from
// one Antigravity error. Unknown fields stay zero; parsing never invents a
// status, reason, or reset value.
type AntigravityErrorEnvelope struct {
	RPCCode        int
	HTTPStatus     int
	Status         string
	Reason         string
	Message        string
	QuotaExhausted bool
	ResetHint      *time.Time
	ResetDelay     time.Duration
}

var (
	// The Go harness renders a Google API error as this bounded line. The
	// message may span lines before the status token.
	antigravityAPIErrorPattern = regexp.MustCompile(
		`(?s)Error\s+(\d{3}),\s*Message:\s*(.+?),\s*Status:\s*([A-Z][A-Z0-9_]*)`,
	)
	antigravityReasonPattern         = regexp.MustCompile(`\breason:([A-Z][A-Z0-9_]*)\b`)
	antigravityResetTimestampPattern = regexp.MustCompile(`\bquotaResetTimeStamp:([^\s\]]+)`)
	antigravityResetDelayPattern     = regexp.MustCompile(
		`\bquotaResetDelay:((?:\d+(?:\.\d+)?(?:ns|us|µs|μs|ms|h|m|s))+)`,
	)
)

// ParseAntigravityACPMessage extracts bounded evidence from a terminal ACP
// prompt error. data is adapter-defined and is read defensively: only known
// scalar fields and nested error objects are inspected.
func ParseAntigravityACPMessage(code int, message string, data any) AntigravityErrorEnvelope {
	env := AntigravityErrorEnvelope{RPCCode: code}
	mergeAntigravityData(&env, data, 0)
	parseAntigravityErrorText(&env, message)
	if env.Message == "" {
		env.Message = boundAntigravityMessage(message)
	}
	normalizeAntigravityQuota(&env)
	return env
}

// ParseAntigravityNativeErrorPayload extracts bounded evidence from the
// payload of a native error step. ok is false for an empty payload.
func ParseAntigravityNativeErrorPayload(payload []byte) (AntigravityErrorEnvelope, bool) {
	if len(payload) == 0 {
		return AntigravityErrorEnvelope{}, false
	}
	text := boundAntigravityText(string(payload))
	if strings.TrimSpace(text) == "" {
		return AntigravityErrorEnvelope{}, false
	}
	env := AntigravityErrorEnvelope{}
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		mergeAntigravityData(&env, text, 0)
	}
	parseAntigravityErrorText(&env, text)
	env.Message = boundAntigravityMessage(env.Message)
	normalizeAntigravityQuota(&env)
	return env, true
}

func normalizeAntigravityQuota(env *AntigravityErrorEnvelope) {
	if env == nil {
		return
	}
	if env.QuotaExhausted && env.Reason == "" {
		env.Reason = antigravityQuotaReason
	}
	if strings.EqualFold(env.Reason, antigravityQuotaReason) {
		env.QuotaExhausted = true
	}
}

func parseAntigravityErrorText(env *AntigravityErrorEnvelope, text string) {
	if env == nil {
		return
	}
	text = boundAntigravityText(text)
	if strings.TrimSpace(text) == "" {
		return
	}
	parseAntigravityAPIError(env, text)
	parseAntigravityReason(env, text)
	parseAntigravityResetEvidence(env, text)
	if env.Message == "" {
		env.Message = boundAntigravityMessage(firstAntigravityLine(text))
	}
}

func parseAntigravityAPIError(env *AntigravityErrorEnvelope, text string) {
	match := antigravityAPIErrorPattern.FindStringSubmatch(text)
	if match == nil {
		return
	}
	if env.HTTPStatus == 0 {
		env.HTTPStatus = antigravityInt(match[1])
	}
	if env.Message == "" {
		env.Message = boundAntigravityMessage(match[2])
	}
	if env.Status == "" {
		env.Status = match[3]
	}
}

func parseAntigravityReason(env *AntigravityErrorEnvelope, text string) {
	if env.Reason == "" {
		if match := antigravityReasonPattern.FindStringSubmatch(text); match != nil {
			env.Reason = match[1]
		}
	}
}

func parseAntigravityResetEvidence(env *AntigravityErrorEnvelope, text string) {
	if env.ResetHint == nil {
		if match := antigravityResetTimestampPattern.FindStringSubmatch(text); match != nil {
			env.ResetHint = parseAntigravityResetTimestamp(match[1])
		}
	}
	if env.ResetDelay > 0 {
		return
	}
	match := antigravityResetDelayPattern.FindStringSubmatch(text)
	if match == nil {
		return
	}
	if delay, err := time.ParseDuration(match[1]); err == nil && delay > 0 {
		env.ResetDelay = delay
	}
}

func mergeAntigravityData(env *AntigravityErrorEnvelope, data any, depth int) {
	if env == nil || data == nil || depth > 2 {
		return
	}
	switch value := data.(type) {
	case map[string]any:
		mergeAntigravityMap(env, value, depth)
	case string:
		trimmed := strings.TrimSpace(value)
		if strings.HasPrefix(trimmed, "{") {
			var decoded any
			if json.Unmarshal([]byte(trimmed), &decoded) == nil {
				mergeAntigravityData(env, decoded, depth+1)
				return
			}
		}
		if antigravityAPIErrorPattern.MatchString(trimmed) {
			parseAntigravityErrorText(env, value)
		}
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(value, &decoded) == nil {
			mergeAntigravityData(env, decoded, depth+1)
		}
	case []byte:
		var decoded any
		if json.Unmarshal(value, &decoded) == nil {
			mergeAntigravityData(env, decoded, depth+1)
		}
	}
}

func mergeAntigravityMap(env *AntigravityErrorEnvelope, value map[string]any, depth int) {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, nested := range []bool{false, true} {
		for _, key := range keys {
			if antigravityNestedErrorField(key) != nested {
				continue
			}
			mergeAntigravityField(env, strings.ToLower(strings.TrimSpace(key)), value[key], depth)
		}
	}
}

func antigravityNestedErrorField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "error", "details", "cause", "data":
		return true
	}
	return false
}

func mergeAntigravityField(env *AntigravityErrorEnvelope, key string, value any, depth int) {
	switch key {
	case "status", "errorstatus", "googleapisstatus":
		mergeAntigravityString(&env.Status, value)
	case "reason", "errorreason", "errorkind", "error_kind":
		mergeAntigravityString(&env.Reason, value)
	case "message", "errormessage", "error_message":
		mergeAntigravityString(&env.Message, value)
	case "httpstatus", "http_status", "statuscode":
		if env.HTTPStatus == 0 {
			env.HTTPStatus = antigravityInt(antigravityString(value))
		}
	case "resetat", "reset_at", "quotaresettimestamp", "quota_reset_timestamp":
		if env.ResetHint == nil {
			env.ResetHint = parseAntigravityResetTimestamp(antigravityString(value))
		}
	case "quotaresetdelay", "quota_reset_delay":
		if env.ResetDelay <= 0 {
			if delay, err := time.ParseDuration(antigravityString(value)); err == nil && delay > 0 {
				env.ResetDelay = delay
			}
		}
	case "error", "details", "detail":
		mergeAntigravityData(env, value, depth+1)
	}
}

func mergeAntigravityString(target *string, value any) {
	if *target == "" {
		*target = boundAntigravityMessage(antigravityString(value))
	}
}

func antigravityString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func antigravityInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func parseAntigravityResetTimestamp(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	if raw == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
	} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return &parsed
		}
	}
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil && seconds > 0 {
		parsed := time.Unix(seconds, 0).UTC()
		return &parsed
	}
	return nil
}

func firstAntigravityLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "reason:") ||
			strings.HasPrefix(line, "quotaReset") || strings.HasPrefix(line, "goroutine ") {
			continue
		}
		return line
	}
	return ""
}

func boundAntigravityText(text string) string {
	if len(text) > AntigravityMaxErrorPayloadBytes {
		text = strings.ToValidUTF8(text[:AntigravityMaxErrorPayloadBytes], "")
	}
	return strings.ToValidUTF8(text, "\uFFFD")
}

func boundAntigravityMessage(text string) string {
	text = strings.TrimSpace(strings.ToValidUTF8(text, " "))
	if len(text) <= AntigravityMaxErrorMessageBytes {
		return text
	}
	return strings.TrimSpace(strings.ToValidUTF8(text[:AntigravityMaxErrorMessageBytes], ""))
}
