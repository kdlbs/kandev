package acp

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const (
	opencodeAgentID             = "opencode-acp"
	maxProviderMessageBytes     = 2048
	genericProviderErrorMessage = "provider prompt error"
)

var (
	openCodeIdentifierPattern = regexp.MustCompile(`\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b`)
	openCodeURLPattern        = regexp.MustCompile(`https?://[^\s]+`)
	openCodeResetPattern      = regexp.MustCompile(`(?i)\bresets?\s+in\s+(?:(\d+)\s*(?:days?|d)\b)?\s*(?:(\d+)\s*(?:hours?|hrs?|h)\b)?\s*(?:(\d+)\s*(?:minutes?|mins?|m|min)\b)?`)
)

type openCodeStderrDiagnostic struct {
	SessionID     string
	ProviderError streams.ProviderError
}

type providerPromptError struct {
	ProviderError streams.ProviderError
}

func (e *providerPromptError) Error() string {
	if e == nil || e.ProviderError.Message == "" {
		return "provider stream error"
	}
	return e.ProviderError.Message
}

// ProviderErrorFromError extracts the safe provider diagnostic from a prompt
// error without exposing the provider-specific wrapper to lifecycle callers.
// It first unwraps the correlated stderr diagnostic; for a structured ACP
// service-failure it reads only the explicit `action_url` field and the safe
// message — never the raw error string. providerID and modelID are the
// adapter's own state at the moment of projection and are merged onto
// whichever projection wins, filling only fields that projection left empty
// so a richer provider-specific extractor is never overwritten by the generic
// allowlisted metadata.
func ProviderErrorFromError(err error, providerID, modelID string) *streams.ProviderError {
	projection := winningProviderErrorProjection(err)
	if projection == nil {
		return nil
	}
	mergeAllowlistedProviderErrorMetadata(projection, err, providerID, modelID)
	return projection
}

func winningProviderErrorProjection(err error) *streams.ProviderError {
	var providerErr *providerPromptError
	if errors.As(err, &providerErr) && providerErr != nil && providerErr.ProviderError.Valid() {
		copy := providerErr.ProviderError
		return &copy
	}
	if providerErr := providerErrorFromACPActionURL(err); providerErr != nil {
		return providerErr
	}
	return providerErrorFromACPPrompt(err)
}

// providerErrorMetadataPattern bounds an allowlisted metadata field to at most
// 64 bytes of `[A-Za-z0-9_.-]`, applied identically to error_kind, provider_id
// and model_id: a malformed or oversized field is dropped rather than
// invalidating the projection.
var providerErrorMetadataPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func validProviderErrorMetadataField(value string) string {
	if providerErrorMetadataPattern.MatchString(value) {
		return value
	}
	return ""
}

// acpErrorKindFromData reads the allowlisted `errorKind` field from a terminal
// ACP prompt error's structured Data. Data is adapter-defined `any`; only the
// exact map[string]any shape encoding/json produces is accepted, and no other
// field of Data is ever read.
func acpErrorKindFromData(data any) string {
	m, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	kind, ok := m["errorKind"].(string)
	if !ok {
		return ""
	}
	return validProviderErrorMetadataField(kind)
}

// mergeAllowlistedProviderErrorMetadata fills provider_id and model_id from
// the adapter's own state, and rpc_code/error_kind from the underlying
// *acp.RequestError when err is (or wraps) one. Raw RequestError.Data never
// crosses this call other than through the validated error_kind extraction.
func mergeAllowlistedProviderErrorMetadata(projection *streams.ProviderError, err error, providerID, modelID string) {
	if projection.ProviderID == "" {
		projection.ProviderID = validProviderErrorMetadataField(providerID)
	}
	if projection.ModelID == "" {
		projection.ModelID = validProviderErrorMetadataField(modelID)
	}
	var reqErr *acp.RequestError
	if !errors.As(err, &reqErr) || reqErr == nil {
		return
	}
	if projection.RPCCode == 0 {
		projection.RPCCode = reqErr.Code
	}
	if projection.ErrorKind == "" {
		projection.ErrorKind = acpErrorKindFromData(reqErr.Data)
	}
}

// providerErrorFromACPPrompt projects the safe message from a terminal ACP
// JSON-RPC error. Error data is adapter-defined and can contain credentials,
// account identifiers, or opaque gateway details, so it never crosses this
// boundary. Provider-specific extractors may attach richer allowlisted fields
// before this generic fallback runs. It always returns non-nil for a genuine
// *acp.RequestError, so a caller never falls back to the SDK's own Error(),
// which serializes the raw Data it is this function's job to keep contained.
func providerErrorFromACPPrompt(err error) *streams.ProviderError {
	var reqErr *acp.RequestError
	if !errors.As(err, &reqErr) || reqErr == nil {
		return nil
	}
	message := sanitizeProviderMessage(reqErr.Message)
	if message == "" {
		message = genericProviderErrorMessage
	}
	return &streams.ProviderError{
		Source:     streams.ProviderErrorSourceACPPrompt,
		Message:    message,
		OccurredAt: time.Now(),
	}
}

// providerErrorFromACPActionURL projects a future ACP service-failure
// response that carries a structured `action_url` into a provider diagnostic.
// The URL must pass the shared allowlist; the message runs through the same
// sanitizer as stderr so it stays URL-free and identifier-redacted.
//
// The contract is intentionally URL-gated: a message-only ACP error (no
// valid `action_url`) falls through to the generic error path rather than
// surfacing an unsanitized ACP message, matching the stderr diagnostic's
// bounded-message rule. OpenCode's current ACP failures therefore keep the
// existing short-error fallback.
func providerErrorFromACPActionURL(err error) *streams.ProviderError {
	var reqErr *acp.RequestError
	if !errors.As(err, &reqErr) || reqErr == nil {
		return nil
	}
	data, ok := reqErr.Data.(map[string]any)
	if !ok {
		return nil
	}
	rawURL, _ := data["action_url"].(string)
	remediationURL := NormalizeOpenCodeActionURL(rawURL)
	if remediationURL == "" {
		return nil
	}
	message := sanitizeProviderMessage(reqErr.Message)
	if message == "" {
		return nil
	}
	return &streams.ProviderError{
		Source:         streams.ProviderErrorSourceOpenCodeACP,
		Message:        message,
		RemediationURL: remediationURL,
		OccurredAt:     time.Now(),
	}
}

// ConsumeStderrLine implements adapter.StderrLineConsumer. OpenCode's
// error-only stderr is an optional signal; malformed or late diagnostics are
// dropped and a full prompt channel never applies backpressure to stderr.
func (a *Adapter) ConsumeStderrLine(line string) {
	if a.agentID != opencodeAgentID {
		return
	}
	diagnostic, ok := parseOpenCodeStderrLine(line)
	if !ok {
		return
	}
	turn := a.currentPromptTurn()
	if turn == nil {
		return
	}
	select {
	case turn.providerErrorCh <- diagnostic:
	default:
		a.logger.Debug("dropping provider diagnostic because prompt channel is full",
			zap.String("session_id", diagnostic.SessionID))
	}
}

// SanitizeStderrLine keeps only a normalized provider message for OpenCode's
// generic process diagnostics. Unrecognized records are excluded because they
// may contain private URLs, identifiers, paths, or credentials.
func (a *Adapter) SanitizeStderrLine(line string) (string, bool) {
	if a.agentID != opencodeAgentID {
		return line, true
	}
	diagnostic, ok := parseOpenCodeStderrLine(line)
	if !ok {
		return "", false
	}
	return diagnostic.ProviderError.Message, true
}

func parseOpenCodeStderrLine(line string) (openCodeStderrDiagnostic, bool) {
	fields, ok := parseOpenCodeLogFields(line)
	if !ok || fields["level"] != "ERROR" || fields["message"] != "stream error" {
		return openCodeStderrDiagnostic{}, false
	}
	if strings.EqualFold(fields["small"], "true") || strings.EqualFold(fields["agent"], "title") {
		return openCodeStderrDiagnostic{}, false
	}
	sessionID := fields["session.id"]
	if !safeOpenCodeIdentifier(sessionID, "ses_") {
		return openCodeStderrDiagnostic{}, false
	}
	// Capture the allowlisted remediation URL before sanitization. OpenCode may
	// carry it in a dedicated `action_url` field or inline in the provider
	// error text; either way only the shared validator accepts it, and the
	// sanitized message never contains the URL or workspace identifier.
	remediationURL := NormalizeOpenCodeActionURL(fields["action_url"])
	if remediationURL == "" {
		remediationURL = extractOpenCodeActionURL(fields["error.error"])
	}
	message := sanitizeProviderMessage(fields["error.error"])
	if message == "" {
		return openCodeStderrDiagnostic{}, false
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, fields["timestamp"])
	if err != nil {
		return openCodeStderrDiagnostic{}, false
	}

	providerError := streams.ProviderError{
		Source:         streams.ProviderErrorSourceOpenCodeStderr,
		ProviderID:     safeOpenCodeField(fields["providerID"]),
		ModelID:        safeOpenCodeField(fields["modelID"]),
		Message:        message,
		RemediationURL: remediationURL,
		OccurredAt:     occurredAt,
	}
	if resetAt := openCodeResetAt(message, occurredAt); resetAt != nil {
		providerError.ResetAt = resetAt
	}
	return openCodeStderrDiagnostic{SessionID: sessionID, ProviderError: providerError}, true
}

func parseOpenCodeLogFields(line string) (map[string]string, bool) {
	fields := make(map[string]string)
	for i := 0; i < len(line); {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if i == len(line) {
			break
		}
		keyStart := i
		for i < len(line) && line[i] != '=' && line[i] != ' ' {
			i++
		}
		if i == keyStart || i >= len(line) || line[i] != '=' {
			return nil, false
		}
		key := line[keyStart:i]
		i++
		value, next, ok := parseOpenCodeFieldValue(line, i)
		if !ok {
			return nil, false
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, false
		}
		fields[key] = value
		i = next
	}
	return fields, len(fields) > 0
}

func parseOpenCodeFieldValue(line string, start int) (string, int, bool) {
	if start >= len(line) {
		return "", start, false
	}
	if line[start] != '"' {
		end := start
		for end < len(line) && line[end] != ' ' {
			end++
		}
		if end == start {
			return "", end, false
		}
		return line[start:end], end, true
	}

	var value strings.Builder
	for i := start + 1; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if i+1 >= len(line) {
				return "", i, false
			}
			value.WriteByte(line[i+1])
			i++
		case '"':
			return value.String(), i + 1, true
		default:
			value.WriteByte(line[i])
		}
	}
	return "", len(line), false
}

func sanitizeProviderMessage(message string) string {
	message = openCodeURLPattern.ReplaceAllString(message, "")
	message = openCodeIdentifierPattern.ReplaceAllString(message, "[redacted]")
	message = strings.Join(strings.Fields(message), " ")
	message = strings.TrimSpace(strings.TrimRight(message, ".:;,-"))
	if len(message) > maxProviderMessageBytes {
		message = message[:maxProviderMessageBytes]
	}
	return message
}

func safeOpenCodeIdentifier(value, prefix string) bool {
	if value == "" || len(value) > 128 || !strings.HasPrefix(value, prefix) {
		return false
	}
	return safeOpenCodeField(value) != ""
}

func safeOpenCodeField(value string) string {
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("._:/-", r) {
			continue
		}
		return ""
	}
	return value
}

func openCodeResetAt(message string, occurredAt time.Time) *time.Time {
	matches := openCodeResetPattern.FindStringSubmatch(message)
	if len(matches) != 4 || (matches[1] == "" && matches[2] == "" && matches[3] == "") {
		return nil
	}
	days, _ := strconv.Atoi(matches[1])
	hours, _ := strconv.Atoi(matches[2])
	minutes, _ := strconv.Atoi(matches[3])
	if days == 0 && hours == 0 && minutes == 0 {
		return nil
	}
	resetAt := occurredAt.Add(time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute)
	return &resetAt
}
