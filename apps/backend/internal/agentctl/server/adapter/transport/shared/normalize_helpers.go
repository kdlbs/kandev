package shared

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

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

// NormalizeShellResult updates a ShellExecPayload with result data.
func NormalizeShellResult(payload *streams.ShellExecPayload, result any) {
	if payload.Output == nil {
		payload.Output = &streams.ShellExecOutput{}
	}

	switch r := result.(type) {
	case string:
		payload.Output.Stdout = r
	case map[string]any:
		if stdout, ok := r["stdout"].(string); ok {
			payload.Output.Stdout = stdout
		}
		if stderr, ok := r["stderr"].(string); ok {
			payload.Output.Stderr = stderr
		}
		if exitCode, ok := r["exit_code"].(float64); ok {
			payload.Output.ExitCode = int(exitCode)
		}
	}
}

// MaxContentLength is the maximum length for tool output content before truncation.
const MaxContentLength = 50000

// MaxFileCount is the maximum number of files to include in code search results.
const MaxFileCount = 500

// langPlaintext is the fallback language identifier for files without a known extension.
const langPlaintext = "plaintext"

// TruncateIfNeeded truncates a string if it exceeds maxLen.
func TruncateIfNeeded(s string, maxLen int) (string, bool) {
	if len(s) <= maxLen {
		return s, false
	}
	return s[:maxLen], true
}

// NormalizeReadResult populates ReadFilePayload.Output with result content.
func NormalizeReadResult(payload *streams.ReadFilePayload, result string) {
	lines := SplitLines(result)
	content, truncated := TruncateIfNeeded(result, MaxContentLength)

	payload.Output = &streams.ReadFileOutput{
		Content:   content,
		LineCount: len(lines),
		Truncated: truncated,
		Language:  DetectLanguage(payload.FilePath),
	}
}

// NormalizeCodeSearchResult populates CodeSearchPayload.Output with result content.
func NormalizeCodeSearchResult(payload *streams.CodeSearchPayload, result string) {
	result = strings.TrimSpace(result)
	if result == "" {
		payload.Output = &streams.CodeSearchOutput{
			Files:     []string{},
			FileCount: 0,
		}
		return
	}

	files := strings.Split(result, "\n")
	truncated := false
	if len(files) > MaxFileCount {
		files = files[:MaxFileCount]
		truncated = true
	}

	payload.Output = &streams.CodeSearchOutput{
		Files:     files,
		FileCount: len(files),
		Truncated: truncated,
	}
}

// NormalizeModifyResult updates ModifyFilePayload with result content for Write operations.
func NormalizeModifyResult(payload *streams.ModifyFilePayload, result string) {
	// For Write tool, store the written content confirmation
	// The tool result confirms what was written
	if len(payload.Mutations) > 0 {
		mut := &payload.Mutations[0]
		// If Content not already set from input args (for create), use result
		if mut.Content == "" && mut.NewContent == "" && result != "" {
			content, _ := TruncateIfNeeded(result, MaxContentLength)
			mut.Content = content
		}
	}
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
