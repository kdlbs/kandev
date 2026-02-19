package portutil

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Regex matches $PORT, ${PORT}, $API_PORT, ${API_PORT}, etc.
// Pattern: $VAR or ${VAR} where VAR contains PORT (with optional prefix/suffix)
var placeholderRegex = regexp.MustCompile(`\$\{?([A-Z_]*PORT[A-Z0-9_]*)\}?`)

// AllocatePort allocates an available port using OS assignment.
// This approach is thread-safe and avoids port conflicts.
func AllocatePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to allocate port: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// TransformCommand detects port placeholders in a command string,
// allocates ports for each unique placeholder, and returns the transformed
// command with placeholders replaced and an environment variable map.
//
// Supports both $PORT and ${PORT} syntax.
// Multiple occurrences of the same placeholder get the same port.
//
// Examples:
//
//	Input:  "npm run dev -- --port $PORT"
//	Output: "npm run dev -- --port 54321", {"PORT": "54321"}
//
//	Input:  "vite --port ${PORT}"
//	Output: "vite --port 54321", {"PORT": "54321"}
//
//	Input:  "npm run dev" (no placeholder)
//	Output: "npm run dev", {}
func TransformCommand(command string) (string, map[string]string, error) {
	// Find all unique placeholder names
	uniquePlaceholders := findUniquePlaceholders(command)

	if len(uniquePlaceholders) == 0 {
		// No placeholders found, return unchanged
		return command, make(map[string]string), nil
	}

	// Allocate a port for each unique placeholder
	portEnv := make(map[string]string)
	for _, placeholder := range uniquePlaceholders {
		port, err := AllocatePort()
		if err != nil {
			return "", nil, fmt.Errorf("failed to allocate port for %s: %w", placeholder, err)
		}
		portEnv[placeholder] = strconv.Itoa(port)
	}

	// Replace all occurrences of placeholders in the command
	// We need to replace both ${VAR} and $VAR forms
	transformedCommand := command
	for placeholder, portStr := range portEnv {
		// Replace ${PLACEHOLDER}
		transformedCommand = strings.ReplaceAll(transformedCommand, "${"+placeholder+"}", portStr)
		// Replace $PLACEHOLDER (but not if it's part of ${PLACEHOLDER})
		// Use a simple approach: replace all remaining $PLACEHOLDER
		transformedCommand = strings.ReplaceAll(transformedCommand, "$"+placeholder, portStr)
	}

	return transformedCommand, portEnv, nil
}

// findUniquePlaceholders extracts unique placeholder names from a command string.
// Returns placeholder names without the $ or ${} prefix/suffix.
func findUniquePlaceholders(command string) []string {
	matches := placeholderRegex.FindAllStringSubmatch(command, -1)

	if len(matches) == 0 {
		return []string{}
	}

	// Use a map to track unique placeholders
	uniqueMap := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			placeholderName := match[1] // Capture group contains the variable name
			uniqueMap[placeholderName] = true
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(uniqueMap))
	for placeholder := range uniqueMap {
		result = append(result, placeholder)
	}

	return result
}
