// Package securityutil provides security-related utilities for command execution,
// input validation, and sanitization.
package securityutil

import (
	"fmt"
	"strings"
)

// ShellEscape escapes a string for safe use in shell commands.
// Returns the string wrapped in single quotes with internal single quotes escaped.
// If the string contains no special characters, it's returned as-is for readability.
func ShellEscape(s string) string {
	if s == "" {
		return "''"
	}
	// If no special characters, return as-is
	if !strings.ContainsAny(s, " \t\n'\"\\$`!*?[](){};<>|&") {
		return s
	}
	// Single-quote and escape internal single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// SplitShellCommand splits a shell command string into arguments,
// respecting quoted strings and escape sequences.
// This is a simple shell-word parser that handles:
// - Single quotes (no escaping inside)
// - Double quotes (backslash escaping)
// - Backslash escaping outside quotes
// - Whitespace as argument separator
func SplitShellCommand(cmd string) ([]string, error) {
	state := &parseState{
		args:    make([]string, 0),
		current: &strings.Builder{},
	}

	for i, r := range cmd {
		if state.escape {
			state.current.WriteRune(r)
			state.escape = false
			continue
		}

		if !handleSpecialChar(r, state) {
			state.current.WriteRune(r)
		}

		// Check for unclosed quotes at end
		if i == len(cmd)-1 && (state.inSingleQuote || state.inDoubleQuote) {
			return nil, fmt.Errorf("unclosed quote in command")
		}
	}

	if state.current.Len() > 0 {
		state.args = append(state.args, state.current.String())
	}

	return state.args, nil
}

// parseState tracks the current parsing state
type parseState struct {
	inSingleQuote bool
	inDoubleQuote bool
	escape        bool
	args          []string
	current       *strings.Builder
}

// handleSpecialChar processes special characters and returns true if handled
func handleSpecialChar(r rune, state *parseState) bool {
	switch r {
	case '\\':
		return handleBackslash(state)
	case '\'':
		return handleSingleQuote(state)
	case '"':
		return handleDoubleQuote(state)
	case ' ', '\t', '\n':
		return handleWhitespace(state)
	default:
		return false
	}
}

// handleBackslash processes backslash escape sequences
func handleBackslash(state *parseState) bool {
	if state.inSingleQuote {
		state.current.WriteRune('\\')
	} else {
		state.escape = true
	}
	return true
}

// handleSingleQuote processes single quote characters
func handleSingleQuote(state *parseState) bool {
	if state.inDoubleQuote {
		state.current.WriteRune('\'')
	} else {
		state.inSingleQuote = !state.inSingleQuote
	}
	return true
}

// handleDoubleQuote processes double quote characters
func handleDoubleQuote(state *parseState) bool {
	if state.inSingleQuote {
		state.current.WriteRune('"')
	} else {
		state.inDoubleQuote = !state.inDoubleQuote
	}
	return true
}

// handleWhitespace processes whitespace as argument separator
func handleWhitespace(state *parseState) bool {
	if state.inSingleQuote || state.inDoubleQuote {
		state.current.WriteRune(' ')
		return true
	}
	if state.current.Len() > 0 {
		state.args = append(state.args, state.current.String())
		state.current.Reset()
	}
	return true
}
