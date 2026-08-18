package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newSessionHistoryManagerForTest(t *testing.T) (*SessionHistoryManager, string) {
	t.Helper()
	dir := t.TempDir()
	mgr, err := NewSessionHistoryManager(dir, "", newTestLogger())
	require.NoError(t, err)
	return mgr, dir
}

// TestGenerateResumeContextRendersEveryEntryKind pins the fork-session prompt:
// each recorded entry kind gets its own labelled section, and the user's new
// request is appended after the history so the agent knows what to do next.
func TestGenerateResumeContextRendersEveryEntryKind(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)
	now := time.Now()
	for _, entry := range []HistoryEntry{
		{Timestamp: now, Type: "user_message", Content: "add retries"},
		{Timestamp: now, Type: "agent_message", Content: "added them"},
		{Timestamp: now, Type: historyEntryTypeToolCall, ToolName: "edit_file"},
		{Timestamp: now, Type: "tool_result", ToolName: "edit_file", Content: "3 lines changed"},
		{Timestamp: now, Type: "unrecognised_kind", Content: "should be ignored"},
	} {
		require.NoError(t, mgr.AppendEntry("session-1", entry))
	}

	prompt, err := mgr.GenerateResumeContext("session-1", "now add tests")

	require.NoError(t, err)
	require.Contains(t, prompt, "[USER]: add retries")
	require.Contains(t, prompt, "[ASSISTANT]: added them")
	require.Contains(t, prompt, "[TOOL CALL: edit_file]")
	require.Contains(t, prompt, "[TOOL RESULT: edit_file] 3 lines changed")
	require.NotContains(t, prompt, "should be ignored",
		"an unknown entry kind must be skipped rather than rendered raw")
	require.Contains(t, prompt, "=== CURRENT REQUEST ===\nnow add tests")
	require.Contains(t, prompt, "Do not repeat work that was already completed.")
	require.Less(t, strings.Index(prompt, "add retries"), strings.Index(prompt, "now add tests"),
		"history must precede the new request so the agent reads it as prior context")
}

func TestGenerateResumeContextReturnsPromptUnchangedWithoutHistory(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)

	prompt, err := mgr.GenerateResumeContext("session-fresh", "do the thing")

	require.NoError(t, err)
	require.Equal(t, "do the thing", prompt,
		"a session with no history must not be given an empty RESUME CONTEXT wrapper")
}

// TestGenerateResumeContextIgnoresContentlessEntries pins the second early
// return: entries exist, but none render, so the raw prompt is used.
func TestGenerateResumeContextIgnoresContentlessEntries(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Timestamp: time.Now(), Type: "session_started",
	}))

	prompt, err := mgr.GenerateResumeContext("session-1", "do the thing")

	require.NoError(t, err)
	require.Equal(t, "do the thing", prompt)
}

func TestGenerateResumeContextSurfacesReadFailure(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)

	prompt, err := mgr.GenerateResumeContext("", "do the thing")

	require.ErrorContains(t, err, "session ID is required")
	require.Equal(t, "do the thing", prompt,
		"a failed read must still return the caller's prompt so the launch can proceed")
}

// TestGenerateResumeContextTruncatesLongEntries pins the size guard: an
// unbounded transcript would blow past the agent's context window.
func TestGenerateResumeContextTruncatesLongEntries(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Timestamp: time.Now(), Type: "user_message", Content: strings.Repeat("a", 5000),
	}))
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Timestamp: time.Now(), Type: "tool_result", ToolName: "read_file",
		Content: strings.Repeat("b", 5000),
	}))

	prompt, err := mgr.GenerateResumeContext("session-1", "continue")

	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(prompt, "... [truncated]"))
	require.NotContains(t, prompt, strings.Repeat("a", 2001),
		"user messages are capped at 2000 characters")
	require.NotContains(t, prompt, strings.Repeat("b", 501),
		"tool results are capped at 500 characters — they are the bulkiest entries")
}

func TestTruncateForContext(t *testing.T) {
	require.Equal(t, "short", truncateForContext("short", 10))
	require.Equal(t, "exactly10c", truncateForContext("exactly10c", 10))
	require.Equal(t, "0123456789... [truncated]", truncateForContext("0123456789abc", 10))
}

func TestHasHistoryReflectsStoredEntries(t *testing.T) {
	mgr, dir := newSessionHistoryManagerForTest(t)

	require.False(t, mgr.HasHistory(""), "an empty session ID has no history")
	require.False(t, mgr.HasHistory("session-1"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "session-empty.jsonl"), nil, 0o600))
	require.False(t, mgr.HasHistory("session-empty"), "a zero-byte file is not history")

	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Timestamp: time.Now(), Type: "user_message", Content: "hi",
	}))
	require.True(t, mgr.HasHistory("session-1"))
}

func TestDeleteHistoryRemovesFileAndIsIdempotent(t *testing.T) {
	mgr, dir := newSessionHistoryManagerForTest(t)
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Timestamp: time.Now(), Type: "user_message", Content: "hi",
	}))
	path := filepath.Join(dir, "session-1.jsonl")
	require.FileExists(t, path)

	require.NoError(t, mgr.DeleteHistory("session-1"))
	require.NoFileExists(t, path)

	require.NoError(t, mgr.DeleteHistory("session-1"),
		"deleting an already-absent history must not error — task cleanup runs more than once")
	require.ErrorContains(t, mgr.DeleteHistory(""), "session ID is required")
}

// TestHistoryFilePathSanitizesSessionID pins the traversal guard: a session ID
// carrying separators must not write outside the history directory.
func TestHistoryFilePathSanitizesSessionID(t *testing.T) {
	mgr, dir := newSessionHistoryManagerForTest(t)

	require.Equal(t, filepath.Join(dir, ".._.._etc_passwd.jsonl"),
		mgr.historyFilePath("../../etc/passwd"))
	require.Equal(t, filepath.Join(dir, "a_b.jsonl"), mgr.historyFilePath(`a\b`))
}
