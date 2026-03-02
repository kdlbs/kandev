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
	var args []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escape := false

	for i, r := range cmd {
		if escape {
			current.WriteRune(r)
			escape = false
			continue
		}

		switch r {
		case '\\':
			if inSingleQuote {
				// Backslash is literal in single quotes
				current.WriteRune(r)
			} else {
				escape = true
			}
		case '\'':
			if inDoubleQuote {
				current.WriteRune(r)
			} else {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if inSingleQuote {
				current.WriteRune(r)
			} else {
				inDoubleQuote = !inDoubleQuote
			}
		case ' ', '\t', '\n':
			if inSingleQuote || inDoubleQuote {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}

		// Check for unclosed quotes at end
		if i == len(cmd)-1 {
			if inSingleQuote || inDoubleQuote {
				return nil, fmt.Errorf("unclosed quote in command")
			}
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args, nil
}
