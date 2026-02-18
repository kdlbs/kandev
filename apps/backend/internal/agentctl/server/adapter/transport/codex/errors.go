package codex

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// codexErrorRegex matches Codex error log format:
// TIMESTAMP ERROR module: error=HTTP_ERROR: Some("JSON")
var codexErrorRegex = regexp.MustCompile(`error=(.+?):\s*Some\("(.+)"\)\s*$`)

// CodexParsedError contains the parsed error information from Codex stderr.
type CodexParsedError struct {
	// Message is the user-friendly error message
	Message string
	// HTTPError is the HTTP error string (e.g., "http 429 Too Many Requests")
	HTTPError string
	// RawJSON contains all fields from the error JSON (generic, captures any structure)
	RawJSON map[string]any
	// ErrorType is the error type from the JSON (e.g., "usage_limit_reached") - may be empty
	ErrorType string
	// ResetsInSeconds is the number of seconds until the limit resets - may be 0
	ResetsInSeconds int64
}

// extractErrorFields extracts errorType, errorMessage, and resetsInSeconds
// from a parsed JSON map, checking both nested "error" object and top-level fields.
func extractErrorFields(rawData map[string]any) (errorType, errorMessage string, resetsInSeconds int64) {
	// First, check for nested error object (standard format)
	if errObj, ok := rawData["error"].(map[string]any); ok {
		if t, ok := errObj["type"].(string); ok {
			errorType = t
		}
		if m, ok := errObj["message"].(string); ok {
			errorMessage = m
		}
		// JSON numbers are float64 in Go
		if r, ok := errObj["resets_in_seconds"].(float64); ok {
			resetsInSeconds = int64(r)
		}
	}

	// Fall back to top-level fields if nested fields not found (flat error format)
	if errorMessage == "" {
		if m, ok := rawData["message"].(string); ok {
			errorMessage = m
		}
	}
	if errorType == "" {
		if t, ok := rawData["type"].(string); ok {
			errorType = t
		}
	}
	return errorType, errorMessage, resetsInSeconds
}

// appendResetTime appends a human-readable reset duration to msg when resetsInSeconds > 0.
func appendResetTime(msg string, resetsInSeconds int64) string {
	if resetsInSeconds <= 0 {
		return msg
	}
	duration := time.Duration(resetsInSeconds) * time.Second
	switch {
	case duration.Hours() >= 1:
		return fmt.Sprintf("%s (resets in %.0f hours)", msg, duration.Hours())
	case duration.Minutes() >= 1:
		return fmt.Sprintf("%s (resets in %.0f minutes)", msg, duration.Minutes())
	default:
		return fmt.Sprintf("%s (resets in %d seconds)", msg, int(duration.Seconds()))
	}
}

// buildErrorMessage constructs a user-friendly message from the parsed error fields.
func buildErrorMessage(errorMessage, errorType, httpError string, resetsInSeconds int64, rawData map[string]any) string {
	switch {
	case errorMessage != "":
		return appendResetTime(errorMessage, resetsInSeconds)
	case errorType != "":
		return fmt.Sprintf("Error: %s", errorType)
	default:
		jsonBytes, _ := json.MarshalIndent(rawData, "", "  ")
		return fmt.Sprintf("%s\n\n%s", httpError, string(jsonBytes))
	}
}

// ParseCodexStderrError attempts to parse a Codex stderr error line and extract
// error information. Returns nil if parsing fails.
//
// Example input:
//
//	2026-01-23T22:57:08.953223Z ERROR codex_api::endpoint::responses: error=http 429 Too Many Requests: Some("{\"error\":{...}}")
func ParseCodexStderrError(line string) *CodexParsedError {
	matches := codexErrorRegex.FindStringSubmatch(line)
	if len(matches) < 3 {
		return nil
	}

	httpError := strings.TrimSpace(matches[1])
	jsonStr := matches[2]

	// Unescape the JSON string (it's double-escaped in the log)
	unescaped := strings.ReplaceAll(jsonStr, `\"`, `"`)
	unescaped = strings.ReplaceAll(unescaped, `\\`, `\`)

	result := &CodexParsedError{
		HTTPError: httpError,
	}

	// Try to parse the JSON into a generic map first (captures all fields)
	var rawData map[string]any
	if err := json.Unmarshal([]byte(unescaped), &rawData); err != nil {
		// Couldn't parse JSON at all, just return HTTP error
		result.Message = httpError
		return result
	}

	result.RawJSON = rawData

	errorType, errorMessage, resetsInSeconds := extractErrorFields(rawData)
	result.ErrorType = errorType
	result.ResetsInSeconds = resetsInSeconds
	result.Message = buildErrorMessage(errorMessage, errorType, httpError, resetsInSeconds, rawData)
	return result
}

// ParseCodexStderrLines attempts to parse Codex stderr lines and extract
// error information. Searches from the end (most recent) first.
// Returns nil if no parseable error is found.
func ParseCodexStderrLines(lines []string) *CodexParsedError {
	for i := len(lines) - 1; i >= 0; i-- {
		if parsed := ParseCodexStderrError(lines[i]); parsed != nil {
			return parsed
		}
	}
	return nil
}
