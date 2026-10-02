package lifecycle

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const cursorNativeContinuationOptIn = "KANDEV_TEST_CURSOR_NATIVE_CONTINUATION"

func TestCursorNativeSessionResume(t *testing.T) {
	binary, accountToken := requireCursorNativeContinuationGate(t)
	smoke := newCursorNativeSmokeContext(t, binary, accountToken)
	first := startCursorNativeSmokeACP(t, binary, smoke.workspace, smoke.env)
	first.primeSession(t)
	sessionID := first.sessionID
	first.stop()

	replacement := startCursorNativeSmokeACPWithoutSession(t, binary, smoke.workspace, smoke.env)
	t.Logf("native resume negotiation: %s", replacement.client.initializeSummary())
	replacement.client.send(t, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/resume", "params": map[string]any{
		"sessionId": sessionID, "cwd": smoke.workspace, "mcpServers": []any{},
	}})
	for {
		var frame cursorNativeSmokeACPFrame
		require.NoError(t, replacement.client.decoder.Decode(&frame))
		if frame.Method != "" {
			replacement.client.captureSessionUpdate(frame)
			replacement.client.handleServerRequest(t, frame)
			continue
		}
		if !cursorNativeSmokeJSONIDMatches(frame.ID, 2) {
			continue
		}
		if len(frame.Error) > 0 {
			var rpcError struct {
				Code int `json:"code"`
			}
			require.NoError(t, json.Unmarshal(frame.Error, &rpcError))
			t.Logf("native session/resume accepted=false error_code=%d", rpcError.Code)
			require.Equal(t, -32601, rpcError.Code, "only method-not-found is a supported negative probe result")
			return
		}
		t.Log("native session/resume accepted=true same_requested_session=true")
		replacement.client.output.Reset()
		replacement.client.request(t, 3, "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt":    []map[string]string{{"type": "text", "text": "What single word did you reply with in the previous request? Do not use tools."}},
		})
		t.Logf("native session/resume prior_completed_output_recalled=%t", strings.Contains(strings.ToLower(replacement.client.assistantOutput.String()), "ready"))
		return
	}
}

func TestCursorNativeConversationRestore(t *testing.T) {
	binary, accountToken := requireCursorNativeContinuationGate(t)
	version, err := exec.Command(binary, "--version").Output()
	require.NoError(t, err)
	versionText := strings.TrimSpace(string(version))
	require.NotEmpty(t, versionText)

	smoke := newCursorNativeSmokeContext(t, binary, accountToken)
	readResult := "native-continuation-read-result-7d31"
	readPath := filepath.Join(smoke.workspace, "continuation-read.txt")
	require.NoError(t, os.WriteFile(readPath, []byte(readResult+"\n"), 0o600))

	sessionID := probeCursorNativeInterruptedRead(t, binary, smoke, readPath, readResult, versionText)
	probeCursorNativeReadContinuation(t, binary, smoke, sessionID, readResult, versionText)
	probeCursorNativeOutputContinuation(t, binary, smoke)
}

// This complements cancellation: a cancelled turn may intentionally be discarded.
func TestCursorNativeAbruptDisconnectRestore(t *testing.T) {
	binary, accountToken := requireCursorNativeContinuationGate(t)
	version, err := exec.Command(binary, "--version").Output()
	require.NoError(t, err)
	versionText := strings.TrimSpace(string(version))

	t.Run("completed_read", func(t *testing.T) {
		smoke := newCursorNativeSmokeContext(t, binary, accountToken)
		const readResult = "native-continuation-read-result-7d31"
		readPath := filepath.Join(smoke.workspace, "continuation-read.txt")
		require.NoError(t, os.WriteFile(readPath, []byte(readResult+"\n"), 0o600))
		first := startCursorNativeSmokeACP(t, binary, smoke.workspace, smoke.env)
		first.client.request(t, 3, "session/prompt", map[string]any{
			"sessionId": first.sessionID,
			"prompt":    []map[string]string{{"type": "text", "text": "Reply with only CONTINUATION_RESTORE_SEED_9C2A."}},
		})
		first.client.output.Reset()
		first.promptAndDisconnectOnUpdate(t, 4, "Read continuation-read.txt and report its exact contents.", func() bool {
			return first.client.hasCompletedReadForPathSince(0, readPath) &&
				strings.Contains(first.client.output.String(), readResult)
		})
		require.True(t, cursorNativeSmokeHasCompletedRead(first.client.toolEvidence, readPath))
		probeCursorNativeReadContinuation(t, binary, smoke, first.sessionID, readResult, versionText)
	})

	t.Run("assistant_output", func(t *testing.T) {
		smoke := newCursorNativeSmokeContext(t, binary, accountToken)
		first := startCursorNativeSmokeACP(t, binary, smoke.workspace, smoke.env)
		const marker = "INTERRUPTED_OUTPUT_MARKER_51B8"
		first.promptAndDisconnectOnUpdate(t, 3,
			"Start with INTERRUPTED_OUTPUT_MARKER_51B8 and continue a long answer. Do not use tools.",
			func() bool { return strings.Contains(first.client.assistantOutput.String(), marker) })
		require.Empty(t, first.client.toolEvidence)
		probeCursorNativeRestoredOutput(t, binary, smoke, first.sessionID, marker)
	})
}

func (p *cursorNativeSmokeACPProcess) promptAndDisconnectOnUpdate(
	t *testing.T, requestID int, prompt string, observed func() bool,
) {
	t.Helper()
	p.client.send(t, map[string]any{"jsonrpc": "2.0", "id": requestID, "method": "session/prompt", "params": map[string]any{
		"sessionId": p.sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": prompt}},
	}})
	for {
		var frame cursorNativeSmokeACPFrame
		require.NoError(t, p.client.decoder.Decode(&frame), "Cursor ACP closed before the interruption boundary")
		if frame.Method != "" {
			p.client.captureSessionUpdate(frame)
			if frame.Method == "session/update" && observed() {
				p.stop()
				return
			}
			p.client.handleServerRequest(t, frame)
		} else if cursorNativeSmokeJSONIDMatches(frame.ID, requestID) {
			t.Fatal("Cursor completed the prompt before the interruption boundary")
		}
	}
}

func probeCursorNativeInterruptedRead(t *testing.T, binary string, smoke *cursorNativeSmokeContext, readPath, readResult, version string) string {
	t.Helper()
	first := startCursorNativeSmokeACP(t, binary, smoke.workspace, smoke.env)
	seedResult := first.client.request(t, 3, "session/prompt", map[string]any{
		"sessionId": first.sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": "Reply with only CONTINUATION_RESTORE_SEED_9C2A."}},
	})
	var seedOutcome map[string]any
	require.NoError(t, json.Unmarshal(seedResult, &seedOutcome))
	require.Contains(t, strings.ToLower(first.client.output.String()), "continuation_restore_seed_9c2a")
	first.client.output.Reset()

	toolEvidence, outcome, cancelSent := first.promptAndCancelAfterCompletedRead(
		t, 4, "Read continuation-read.txt and report its exact contents.", readPath, readResult,
	)
	require.True(t, cancelSent, "the test must interrupt after Cursor reports the read tool completed")
	require.True(t, cursorNativeSmokeHasCompletedRead(toolEvidence, readPath), "native Cursor must report a completed fixture read")
	requireCursorNativePromptCancelled(t, outcome, "read")
	t.Logf("native restore probe: cli=%s initialize=%s tool_frames=%v live_read_result=%t",
		version, first.client.initializeSummary(), cursorNativeSmokeToolEvidenceSummary(toolEvidence),
		strings.Contains(first.client.output.String(), readResult))
	sessionID := first.sessionID
	first.stop()
	return sessionID
}

func probeCursorNativeReadContinuation(t *testing.T, binary string, smoke *cursorNativeSmokeContext, sessionID, readResult, version string) {
	t.Helper()
	const interruptedPrompt = "Read continuation-read.txt and report its exact contents."
	replacement := startCursorNativeSmokeACPWithoutSession(t, binary, smoke.workspace, smoke.env)
	replacement.client.request(t, 2, "session/load", map[string]any{
		"sessionId": sessionID, "cwd": smoke.workspace, "mcpServers": []any{},
	})
	loadedHistory := replacement.client.output.String()
	require.Contains(t, loadedHistory, "CONTINUATION_RESTORE_SEED_9C2A")
	require.Contains(t, loadedHistory, interruptedPrompt)
	require.NotEmpty(t, replacement.client.sessionIDs)
	requireRestoredCursorSessionID(t, replacement.client.sessionIDs, sessionID)
	replacement.client.output.Reset()
	toolStart := len(replacement.client.toolEvidence)
	replacement.client.request(t, 3, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": "Reply with the exact contents you read from continuation-read.txt in the preceding request. Do not call any tools."}},
	})
	postRestoreToolCalls := len(replacement.client.toolEvidence) - toolStart
	proven := strings.Contains(replacement.client.output.String(), readResult) && postRestoreToolCalls == 0
	t.Logf("native restore replay: same_session=true restored_user_prompt=true restored_read_output_seed=true completed_read_result_available=%t post_restore_tool_calls=%d",
		proven, postRestoreToolCalls)
	t.Logf("native Cursor continuation evidence: cli=%s initialize=%s load=session/load same_session=true read_kind=read read_status=completed interrupted_prompt_restored=true read_continuation_proven=%t",
		version, replacement.client.initializeSummary(), proven)

	replacement.client.output.Reset()
	toolStart = len(replacement.client.toolEvidence)
	replacement.client.request(t, 4, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": "Continue the unfinished request in this conversation. Use read-only file tools to inspect current state if prior read results are missing. Do not modify files or execute commands."}},
	})
	readOnly := true
	for _, tool := range replacement.client.toolEvidence[toolStart:] {
		readOnly = readOnly && tool.Kind == "read" && tool.Status == "completed"
	}
	continued := strings.Contains(replacement.client.output.String(), readResult) && readOnly
	require.True(t, continued, "same native conversation must finish through completed read-only inspection")
	t.Logf("native saved-history continuation: same_session=true read_only=%t requested_result_produced=%t tool_calls=%d",
		readOnly, continued, len(replacement.client.toolEvidence)-toolStart)
}

func probeCursorNativeOutputContinuation(t *testing.T, binary string, smoke *cursorNativeSmokeContext) {
	t.Helper()
	interruptedOutput := startCursorNativeSmokeACP(t, binary, smoke.workspace, smoke.env)
	const marker = "INTERRUPTED_OUTPUT_MARKER_51B8"
	const prompt = "Start with INTERRUPTED_OUTPUT_MARKER_51B8 and continue a long answer. Do not use tools."
	interruptedOutput.client.output.Reset()
	interruptedOutput.client.assistantOutput.Reset()
	toolStart := len(interruptedOutput.client.toolEvidence)
	outcome, cancelSent := interruptedOutput.promptAndCancelAfterAssistantMarker(t, 3, prompt, marker)
	require.True(t, cancelSent, "the output-only probe must interrupt after assistant output")
	require.Contains(t, interruptedOutput.client.assistantOutput.String(), marker)
	require.Empty(t, interruptedOutput.client.toolEvidence[toolStart:], "output-only probe must not observe tools")
	requireCursorNativePromptCancelled(t, outcome, "output-only")
	sessionID := interruptedOutput.sessionID
	interruptedOutput.stop()
	probeCursorNativeRestoredOutput(t, binary, smoke, sessionID, marker)
}

func probeCursorNativeRestoredOutput(t *testing.T, binary string, smoke *cursorNativeSmokeContext, sessionID, marker string) {
	t.Helper()
	restored := startCursorNativeSmokeACPWithoutSession(t, binary, smoke.workspace, smoke.env)
	restored.client.request(t, 2, "session/load", map[string]any{
		"sessionId": sessionID, "cwd": smoke.workspace, "mcpServers": []any{},
	})
	requireRestoredCursorSessionID(t, restored.client.sessionIDs, sessionID)
	restored.client.output.Reset()
	toolStart := len(restored.client.toolEvidence)
	restored.client.request(t, 3, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": "Reply with the unique marker at the start of your immediately preceding interrupted response. Do not use tools."}},
	})
	recalled := strings.Contains(restored.client.output.String(), marker) && len(restored.client.toolEvidence[toolStart:]) == 0
	t.Logf("native output-only restore: same_session=true interrupted_assistant_output_replayed=%t interrupted_assistant_output_recalled=%t",
		strings.Contains(restored.client.assistantOutput.String(), marker), recalled)
}

func cursorNativeSmokeHasCompletedRead(evidence []cursorNativeSmokeToolEvidence, path string) bool {
	for _, item := range evidence {
		if item.Kind == "read" && item.Status == "completed" && item.Path == path {
			return true
		}
	}
	return false
}

func requireRestoredCursorSessionID(t *testing.T, sessionIDs []string, expected string) {
	t.Helper()
	require.NotEmpty(t, sessionIDs, "native restoration must provide observed session identity")
	for _, sessionID := range sessionIDs {
		require.Equal(t, expected, sessionID, "session/load must retain provider identity")
	}
}

func requireCursorNativePromptCancelled(t *testing.T, outcome json.RawMessage, probe string) {
	t.Helper()
	require.NotEmpty(t, outcome, "%s must return a cancellation result", probe)
	var result struct {
		StopReason string `json:"stopReason"`
	}
	require.NoError(t, json.Unmarshal(outcome, &result))
	require.Equal(t, "cancelled", result.StopReason, "%s prompt should remain interrupted", probe)
}

func requireCursorNativeContinuationGate(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("native Cursor continuation check runs only on macOS")
	}
	if os.Getenv(cursorNativeContinuationOptIn) != "1" {
		t.Skip("set " + cursorNativeContinuationOptIn + "=1 to run the native Cursor continuation check")
	}
	return cursorNativeSmokeExecutable(t), readCursorNativeSmokeAccountToken(t)
}

type cursorNativeSmokeToolEvidence struct {
	Kind   string
	Status string
	ID     string
	Path   string
}

func (p *cursorNativeSmokeACPProcess) promptAndCancelAfterCompletedRead(
	t *testing.T,
	requestID int,
	prompt string,
	readPath string,
	responseMarker string,
) ([]cursorNativeSmokeToolEvidence, json.RawMessage, bool) {
	t.Helper()
	before := len(p.client.toolEvidence)
	p.client.send(t, map[string]any{"jsonrpc": "2.0", "id": requestID, "method": "session/prompt", "params": map[string]any{
		"sessionId": p.sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": prompt}},
	}})
	cancelSent := false
	for {
		var frame cursorNativeSmokeACPFrame
		err := p.client.decoder.Decode(&frame)
		require.NoError(t, err, "Cursor ACP closed before the read prompt settled")
		if frame.Method != "" {
			p.client.captureSessionUpdate(frame)
			if !cancelSent && frame.Method == "session/update" &&
				p.client.hasCompletedReadForPathSince(before, readPath) && strings.Contains(string(frame.Params), responseMarker) {
				p.client.send(t, map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]string{"sessionId": p.sessionID}})
				cancelSent = true
			}
			p.client.handleServerRequest(t, frame)
			continue
		}
		if !cursorNativeSmokeJSONIDMatches(frame.ID, requestID) {
			continue
		}
		if len(frame.Error) > 0 {
			return append([]cursorNativeSmokeToolEvidence(nil), p.client.toolEvidence[before:]...), nil, cancelSent
		}
		return append([]cursorNativeSmokeToolEvidence(nil), p.client.toolEvidence[before:]...), frame.Result, cancelSent
	}
}

func cursorNativeSmokeToolEvidenceSummary(evidence []cursorNativeSmokeToolEvidence) []string {
	result := make([]string, 0, len(evidence))
	for _, item := range evidence {
		result = append(result, item.Kind+":"+item.Status)
	}
	return result
}
