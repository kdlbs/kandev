package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	acp "github.com/coder/acp-go-sdk"
)

func TestMockInterruptionGateReleaseAndCancellation(t *testing.T) {
	for _, cancelPrompt := range []bool{false, true} {
		t.Run(map[bool]string{false: "release", true: "cancel"}[cancelPrompt], func(t *testing.T) {
			gate := filepath.Join(t.TempDir(), "interruption.gate")
			t.Setenv("E2E_MOCK_AGENT_CONTINUATION_GATE_FILE", gate)
			if err := os.WriteFile(gate, nil, 0600); err != nil {
				t.Fatal(err)
			}
			sid := acp.SessionId(gate)
			t.Cleanup(func() { _ = os.Remove(mockContinuationPath(sid)) })
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				updater := &mockUpdater{}
				agent := &mockAgent{conn: updater}
				result := make(chan error, 1)
				go func() {
					_, err, _ := agent.emitMockInterruption(ctx, sid, mockContinuationReadScenario)
					result <- err
				}()
				synctest.Wait()
				if len(updater.getUpdates()) == 0 {
					t.Fatal("gate prevented the initial visible prompt evidence")
				}
				select {
				case err := <-result:
					t.Fatalf("interruption returned before release: %v", err)
				default:
				}
				if cancelPrompt {
					cancel()
				} else if err := os.Remove(gate); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				err := <-result
				if cancelPrompt {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancelled prompt returned %v", err)
					}
				} else {
					var requestError *acp.RequestError
					if !errors.As(err, &requestError) || requestError.Code != -32603 {
						t.Fatalf("released prompt returned %v, want the original interruption", err)
					}
				}
			})
		})
	}
}
