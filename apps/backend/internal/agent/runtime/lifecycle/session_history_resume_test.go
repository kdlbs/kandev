package lifecycle

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestGenerateResumeContextUsesNewest50ConversationMessages(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)
	now := time.Now()
	for i := 1; i <= 51; i++ {
		typ := "user_message"
		if i%2 == 0 {
			typ = "agent_message"
		}
		require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Type:      typ,
			Content:   "message-" + strconv.Itoa(i),
		}))
	}

	prompt, err := mgr.GenerateResumeContext("session-1", "now add tests")

	require.NoError(t, err)
	require.Equal(t, 50, strings.Count(prompt, "[USER]:")+strings.Count(prompt, "[ASSISTANT]:"))
	require.NotContains(t, prompt, ": message-1\n")
	require.Contains(t, prompt, "message-2")
	require.Contains(t, prompt, "message-51")
	require.Less(t, strings.Index(prompt, "message-2"), strings.Index(prompt, "message-51"),
		"the retained window must stay in chronological order")
	require.Contains(t, prompt, "=== CURRENT REQUEST ===\nnow add tests")
	require.Less(t, strings.Index(prompt, "message-51"), strings.Index(prompt, "now add tests"),
		"the current request must follow the retained history")
}

func TestGenerateResumeContextExcludesNonConversationEntriesBeforeWindowing(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)
	now := time.Now()
	for i := 1; i <= 50; i++ {
		require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Type:      "user_message",
			Content:   "conversation-" + strconv.Itoa(i),
		}))
	}
	for i := 0; i < 150; i++ {
		require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
			Timestamp: now.Add(time.Duration(100+i) * time.Second),
			Type:      historyEntryTypeToolCall,
			ToolName:  "read_file",
		}))
		require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
			Timestamp: now.Add(time.Duration(250+i) * time.Second),
			Type:      "tool_result",
			ToolName:  "read_file",
			Content:   "tool output",
		}))
	}
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Timestamp: now.Add(500 * time.Second), Type: "session_started", Content: "status event",
	}))
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Timestamp: now.Add(501 * time.Second), Type: "unknown", Content: "unknown event",
	}))

	prompt, err := mgr.GenerateResumeContext("session-1", "continue")

	require.NoError(t, err)
	require.Equal(t, 50, strings.Count(prompt, "[USER]:")+strings.Count(prompt, "[ASSISTANT]:"))
	require.Contains(t, prompt, "conversation-1")
	require.Contains(t, prompt, "conversation-50")
	require.NotContains(t, prompt, "TOOL CALL")
	require.NotContains(t, prompt, "TOOL RESULT")
	require.NotContains(t, prompt, "status event")
	require.NotContains(t, prompt, "unknown event")
}

func TestGenerateResumeContextExcludesEmptyConversationMessages(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{Type: "user_message"}))
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Type: "agent_message", Content: "kept",
	}))

	prompt, err := mgr.GenerateResumeContext("session-1", "continue")

	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(prompt, "[ASSISTANT]:"))
	require.NotContains(t, prompt, "[USER]:")
	require.Contains(t, prompt, "kept")
}

func TestGenerateResumeContextPreservesHistoryFile(t *testing.T) {
	mgr, dir := newSessionHistoryManagerForTest(t)
	require.NoError(t, mgr.AppendEntry("session-1", HistoryEntry{
		Type: "user_message", Content: "keep this file",
	}))
	path := filepath.Join(dir, "session-1.jsonl")
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	_, err = mgr.GenerateResumeContext("session-1", "continue")

	require.NoError(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// TestGenerateResumeContextPlacesCurrentRequestAfterHistory pins the fork
// prompt framing while the history selection remains independently bounded.
func TestGenerateResumeContextPlacesCurrentRequestAfterHistory(t *testing.T) {
	mgr, _ := newSessionHistoryManagerForTest(t)
	for _, entry := range []HistoryEntry{
		{Timestamp: time.Now(), Type: "user_message", Content: "add retries"},
		{Timestamp: time.Now(), Type: "agent_message", Content: "added them"},
		{Timestamp: time.Now(), Type: historyEntryTypeToolCall, ToolName: "edit_file"},
		{Timestamp: time.Now(), Type: "tool_result", ToolName: "edit_file", Content: "3 lines changed"},
		{Timestamp: time.Now(), Type: "unrecognised_kind", Content: "should be ignored"},
	} {
		require.NoError(t, mgr.AppendEntry("session-1", entry))
	}

	prompt, err := mgr.GenerateResumeContext("session-1", "now add tests")

	require.NoError(t, err)
	require.Contains(t, prompt, "[USER]: add retries")
	require.Contains(t, prompt, "[ASSISTANT]: added them")
	require.NotContains(t, prompt, "[TOOL CALL: edit_file]")
	require.NotContains(t, prompt, "[TOOL RESULT: edit_file] 3 lines changed")
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
	require.Equal(t, 1, strings.Count(prompt, "... [truncated]"))
	require.NotContains(t, prompt, strings.Repeat("a", 2001),
		"user messages are capped at 2000 characters")
	require.NotContains(t, prompt, "TOOL RESULT")
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
