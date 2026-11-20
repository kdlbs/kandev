package main

import (
	"context"
	acp "github.com/coder/acp-go-sdk"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestMockAgentCancelHoldDefersPromptCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const sessionID = acp.SessionId("cancel-hold-session")
		t.Setenv("KANDEV_E2E_CANCEL_HOLD_DURATION", "30ms")
		const acceptanceMarker = "queued-pause-provider-accepted-test-token"
		updater := newCapturingUpdater()
		agent := &mockAgent{
			model:             "mock-fast",
			conn:              updater,
			sessions:          map[acp.SessionId]bool{sessionID: true},
			promptCancels:     make(map[acp.SessionId]context.CancelFunc),
			promptCancelHolds: make(map[acp.SessionId]chan struct{}),
			commandsEmitted:   make(map[acp.SessionId]bool),
		}

		result := make(chan struct {
			response acp.PromptResponse
			err      error
		}, 1)
		go func() {
			response, err := agent.Prompt(context.Background(), acp.PromptRequest{
				SessionId: sessionID,
				Prompt:    []acp.ContentBlock{acp.TextBlock("/e2e:cancel-hold " + acceptanceMarker)},
			})
			result <- struct {
				response acp.PromptResponse
				err      error
			}{response: response, err: err}
		}()

		synctest.Wait()
		select {
		case <-updater.anySeen:
		default:
			t.Fatal("hold prompt did not start")
		}
		if texts := updater.textMessages(); len(texts) != 1 || strings.TrimSpace(texts[0]) != acceptanceMarker {
			t.Fatalf("provider acceptance text before cancellation = %v, want [%q]", texts, acceptanceMarker)
		}
		if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: sessionID}); err != nil {
			t.Fatalf("cancel prompt: %v", err)
		}
		synctest.Wait()
		select {
		case <-result:
			t.Fatal("cancel-hold prompt completed before its configured hold elapsed")
		default:
		}

		time.Sleep(29 * time.Millisecond)
		synctest.Wait()
		select {
		case <-result:
			t.Fatal("cancel-hold prompt completed before its configured hold elapsed")
		default:
		}

		time.Sleep(time.Millisecond)
		synctest.Wait()
		outcome := <-result
		if outcome.err != nil {
			t.Fatalf("prompt returned error: %v", outcome.err)
		}
		if outcome.response.StopReason != acp.StopReasonCancelled {
			t.Fatalf("stop reason = %q, want cancelled", outcome.response.StopReason)
		}
		if texts := updater.textMessages(); len(texts) != 1 || strings.TrimSpace(texts[0]) != acceptanceMarker {
			t.Fatalf("provider acceptance text after cancellation = %v, want [%q]", texts, acceptanceMarker)
		}
	})
}

func TestMockAgentCancelHoldWaitsForReleaseFile(t *testing.T) {
	release := filepath.Join(t.TempDir(), "cancel-release")
	t.Setenv("E2E_MOCK_AGENT_CANCEL_RELEASE_FILE", release)
	t.Setenv("KANDEV_E2E_CANCEL_HOLD_DURATION", "1ms")
	synctest.Test(t, func(t *testing.T) {
		const sessionID = acp.SessionId("cancel-release-session")
		agent := &mockAgent{model: "mock-fast", conn: newCapturingUpdater(), sessions: map[acp.SessionId]bool{sessionID: true}, commandsEmitted: make(map[acp.SessionId]bool)}
		result := make(chan acp.PromptResponse, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			response, err := agent.Prompt(ctx, acp.PromptRequest{SessionId: sessionID, Prompt: []acp.ContentBlock{acp.TextBlock("/e2e:cancel-hold")}})
			if err != nil {
				t.Errorf("prompt: %v", err)
			}
			result <- response
		}()
		synctest.Wait()
		if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: sessionID}); err != nil {
			t.Fatal(err)
		}
		// ACP also cancels the incoming request context before provider acknowledgement.
		cancel()
		time.Sleep(20 * time.Millisecond)
		synctest.Wait()
		select {
		case <-result:
			t.Fatal("cancellation acknowledged before the test released it")
		default:
		}
		if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
			t.Fatal(err)
		}
		if response := <-result; response.StopReason != acp.StopReasonCancelled {
			t.Fatalf("stop reason = %q", response.StopReason)
		}
	})
}
