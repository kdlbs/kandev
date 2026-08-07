package acp

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const (
	opencodeAgentID         = "opencode-acp"
	maxProviderMessageBytes = 2048
)

var (
	openCodeIdentifierPattern = regexp.MustCompile(`\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b`)
	openCodeURLPattern        = regexp.MustCompile(`https?://[^\s]+`)
	openCodeResetPattern      = regexp.MustCompile(`(?i)\bresets?\s+in\s+(?:(\d+)\s*(?:hours?|hrs?|h))?\s*(?:(\d+)\s*(?:minutes?|mins?|m|min))?`)
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
func ProviderErrorFromError(err error) *streams.ProviderError {
	var providerErr *providerPromptError
	if !errors.As(err, &providerErr) || providerErr == nil || !providerErr.ProviderError.Valid() {
		return nil
	}
	copy := providerErr.ProviderError
	return &copy
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
	message := sanitizeOpenCodeMessage(fields["error.error"])
	if message == "" {
		return openCodeStderrDiagnostic{}, false
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, fields["timestamp"])
	if err != nil {
		return openCodeStderrDiagnostic{}, false
	}

	providerError := streams.ProviderError{
		Source:     streams.ProviderErrorSourceOpenCodeStderr,
		ProviderID: safeOpenCodeField(fields["providerID"]),
		ModelID:    safeOpenCodeField(fields["modelID"]),
		Message:    message,
		OccurredAt: occurredAt,
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

func sanitizeOpenCodeMessage(message string) string {
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
	if len(matches) != 3 || (matches[1] == "" && matches[2] == "") {
		return nil
	}
	hours, _ := strconv.Atoi(matches[1])
	minutes, _ := strconv.Atoi(matches[2])
	if hours == 0 && minutes == 0 {
		return nil
	}
	resetAt := occurredAt.Add(time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute)
	return &resetAt
}
