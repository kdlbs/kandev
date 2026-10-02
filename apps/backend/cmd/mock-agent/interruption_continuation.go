package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

const (
	mockContinuationOutputScenario               = "output"
	mockContinuationReadScenario                 = "read"
	mockContinuationWriteScenario                = "write"
	mockContinuationPendingScenario              = "pending"
	mockContinuationUnknownScenario              = "unknown"
	mockContinuationReadHoldScenario             = "read-hold"
	mockContinuationReadRestoreTransientScenario = "read-restore-transient"
	mockContinuationReadRestoreHardScenario      = "read-restore-hard"
	mockContinuationReadAmbiguousScenario        = "read-ambiguous"
)

type mockContinuationEpisode struct {
	Scenario        string `json:"scenario"`
	Original        int    `json:"original"`
	Continuation    int    `json:"continuation"`
	RestoreAttempts int    `json:"restore_attempts"`
}

func mockContinuationPath(sid acp.SessionId) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("kandev-mock-continuation-%x.json", sha256.Sum256([]byte(sid))))
}

func (a *mockAgent) handleMockInterruptionContinuation(ctx context.Context, sid acp.SessionId, prompt string) (acp.PromptResponse, error, bool) {
	prompt = stripKandevSystem(strings.TrimSpace(prompt))
	scenario := strings.TrimPrefix(prompt, "/continuation-")
	if strings.HasPrefix(prompt, "/continuation-") {
		switch scenario {
		case mockContinuationOutputScenario, mockContinuationReadScenario, mockContinuationWriteScenario,
			mockContinuationPendingScenario, mockContinuationUnknownScenario, mockContinuationReadHoldScenario,
			mockContinuationReadRestoreTransientScenario, mockContinuationReadRestoreHardScenario, mockContinuationReadAmbiguousScenario:
			return a.emitMockInterruption(ctx, sid, scenario)
		}
	}
	if !strings.HasPrefix(prompt, "Your previous turn was interrupted by a temporary connection failure. Continue the unfinished request") {
		return acp.PromptResponse{}, nil, false
	}
	raw, err := os.ReadFile(mockContinuationPath(sid))
	var episode mockContinuationEpisode
	if err != nil || json.Unmarshal(raw, &episode) != nil {
		return acp.PromptResponse{}, nil, false
	}
	episode.Continuation++
	if err := saveMockContinuation(sid, episode); err != nil {
		return acp.PromptResponse{}, err, true
	}
	e := &emitter{ctx: ctx, conn: a.conn, sid: sid}
	if episode.Scenario == mockContinuationReadAmbiguousScenario {
		e.text("Mock continuation acceptance uncertain.\n")
		return acp.PromptResponse{}, &acp.RequestError{Code: -32603, Message: "continuation acceptance uncertain"}, true
	}
	if episode.Scenario == mockContinuationReadHoldScenario {
		e.text(fmt.Sprintf("Mock continuation accepted: original=%d continuation=%d native=%s\n", episode.Original, episode.Continuation, sid))
		<-ctx.Done()
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil, true
	}
	e.text(fmt.Sprintf("Mock continuation complete: original=%d continuation=%d native=%s\n", episode.Original, episode.Continuation, sid))
	if ctx.Err() != nil {
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil, true
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

func mockContinuationRestoreFailure(sid acp.SessionId) error {
	raw, err := os.ReadFile(mockContinuationPath(sid))
	var episode mockContinuationEpisode
	if err != nil || json.Unmarshal(raw, &episode) != nil {
		return nil
	}
	episode.RestoreAttempts++
	if err := saveMockContinuation(sid, episode); err != nil {
		return err
	}
	if episode.Scenario == mockContinuationReadRestoreHardScenario {
		return &acp.RequestError{Code: -32603, Message: "saved conversation unavailable"}
	}
	if episode.Scenario == mockContinuationReadRestoreTransientScenario && episode.RestoreAttempts == 1 {
		return &acp.RequestError{Code: -32603, Message: "dial tcp: network is unreachable"}
	}
	return nil
}

func saveMockContinuation(sid acp.SessionId, episode mockContinuationEpisode) error {
	raw, err := json.Marshal(episode)
	if err != nil {
		return err
	}
	return os.WriteFile(mockContinuationPath(sid), raw, 0600)
}

func (a *mockAgent) emitMockInterruption(ctx context.Context, sid acp.SessionId, scenario string) (acp.PromptResponse, error, bool) {
	var episode mockContinuationEpisode
	if raw, err := os.ReadFile(mockContinuationPath(sid)); err == nil {
		_ = json.Unmarshal(raw, &episode)
	}
	episode.Scenario, episode.Original = scenario, episode.Original+1
	if err := saveMockContinuation(sid, episode); err != nil {
		return acp.PromptResponse{}, err, true
	}
	e := &emitter{ctx: ctx, conn: a.conn, sid: sid}
	e.text("Mock interruption: partial history preserved.\n")
	if scenario != mockContinuationOutputScenario {
		kind := acp.ToolKindRead
		if scenario == mockContinuationWriteScenario {
			kind = acp.ToolKindEdit
		}
		if scenario == mockContinuationUnknownScenario {
			kind = acp.ToolKindOther
		}
		e.startTool("interrupted-tool", "Fixture inspection", kind, map[string]any{"path": "fixture.txt"})
		if scenario != mockContinuationPendingScenario {
			e.completeTool("interrupted-tool", "fixture read result")
		}
	}
	return acp.PromptResponse{}, &acp.RequestError{Code: -32603, Message: "peer disconnected before response", Data: map[string]any{"kandevMock": map[string]any{"continuationInterruption": true}}}, true
}
