package adapter

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

// parseCodexStderrError attempts to parse a Codex stderr error line and extract
// error information. Returns nil if parsing fails.
//
// Example input:
//
//	2026-01-23T22:57:08.953223Z ERROR codex_api::endpoint::responses: error=http 429 Too Many Requests: Some("{\"error\":{...}}")
func parseCodexStderrError(line string) *CodexParsedError {
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

	// Extract common fields - check both nested "error" object and top-level fields
	var errorType, errorMessage string
	var resetsInSeconds int64

	// First, check for nested error object (standard format)
	if errObj, ok := rawData["error"].(map[string]any); ok {
		if t, ok := errObj["type"].(string); ok {
			errorType = t
		}
		if m, ok := errObj["message"].(string); ok {
			errorMessage = m
		}
		// Handle both int and float64 (JSON numbers are float64 in Go)
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

	result.ErrorType = errorType
	result.ResetsInSeconds = resetsInSeconds

	// Build a user-friendly message from available fields
	var msg string

	if errorMessage != "" {
		// We have a specific error message from the API
		msg = errorMessage
		// Add reset time info if available
		if resetsInSeconds > 0 {
			duration := time.Duration(resetsInSeconds) * time.Second
			if duration.Hours() >= 1 {
				msg = fmt.Sprintf("%s (resets in %.0f hours)", msg, duration.Hours())
			} else if duration.Minutes() >= 1 {
				msg = fmt.Sprintf("%s (resets in %.0f minutes)", msg, duration.Minutes())
			} else {
				msg = fmt.Sprintf("%s (resets in %d seconds)", msg, int(duration.Seconds()))
			}
		}
	} else if errorType != "" {
		// We have an error type but no message
		msg = fmt.Sprintf("Error: %s", errorType)
	} else {
		// Unknown error format - show HTTP error and include JSON body
		jsonBytes, _ := json.MarshalIndent(rawData, "", "  ")
		msg = fmt.Sprintf("%s\n\n%s", httpError, string(jsonBytes))
	}

	result.Message = msg
	return result
}

// parseCodexStderrLines attempts to parse Codex stderr lines and extract
// error information. Searches from the end (most recent) first.
// Returns nil if no parseable error is found.
func parseCodexStderrLines(lines []string) *CodexParsedError {
	for i := len(lines) - 1; i >= 0; i-- {
		if parsed := parseCodexStderrError(lines[i]); parsed != nil {
			return parsed
		}
	}
	return nil
}
