package shared

import (
	"fmt"
	"strings"
)

// langPlaintext is the fallback language identifier for files without a known extension.
const langPlaintext = "plaintext"

// GenerateUnifiedDiff creates a unified diff string from old and new content.
func GenerateUnifiedDiff(oldStr, newStr, path string, startLine int) string {
	// If both empty or identical, no diff needed
	if oldStr == "" && newStr == "" {
		return ""
	}
	if oldStr == newStr {
		return ""
	}

	oldLines := SplitLines(oldStr)
	newLines := SplitLines(newStr)

	if startLine == 0 {
		startLine = 1
	}

	// Build diff header
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", path, path)
	sb.WriteString("index 0000000..0000000 100644\n")
	fmt.Fprintf(&sb, "--- a/%s\n", path)
	fmt.Fprintf(&sb, "+++ b/%s\n", path)
	fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", startLine, len(oldLines), startLine, len(newLines))

	// Add removed lines
	for _, line := range oldLines {
		sb.WriteString("-")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Add added lines
	for _, line := range newLines {
		sb.WriteString("+")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// SplitLines splits a string into lines, normalizing line endings.
func SplitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// DetectLanguage maps file extension to language identifier.
// Used for syntax highlighting in diffs.
func DetectLanguage(path string) string {
	if path == "" {
		return langPlaintext
	}

	// Find last dot
	lastDot := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			lastDot = i
			break
		}
		if path[i] == '/' {
			break // No extension found
		}
	}

	if lastDot == -1 || lastDot == len(path)-1 {
		return langPlaintext
	}

	ext := path[lastDot+1:]

	langMap := map[string]string{
		"ts":   "typescript",
		"tsx":  "typescript",
		"js":   "javascript",
		"jsx":  "javascript",
		"py":   "python",
		"go":   "go",
		"rs":   "rust",
		"java": "java",
		"cpp":  "cpp",
		"c":    "c",
		"h":    "c",
		"hpp":  "cpp",
		"css":  "css",
		"html": "html",
		"json": "json",
		"md":   "markdown",
		"yaml": "yaml",
		"yml":  "yaml",
		"sh":   "bash",
		"bash": "bash",
	}

	if lang, ok := langMap[ext]; ok {
		return lang
	}
	return langPlaintext
}
