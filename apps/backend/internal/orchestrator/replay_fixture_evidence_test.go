package orchestrator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/replayfixtures"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// replayEvidenceLayer drives the recovery-evidence layer directly from a
// fixture, following the per-frame mapping table in
// provider-error-recovery-02.md#replay-harness-semantics ("The evidence
// layer"): model_settled drives nothing here and emits no token, so frame
// order and fx.Expect.Events align position-by-position once model_settled
// frames are skipped. A message_chunk frame whose aligned token carries
// ":diagnostic" calls observeProviderDiagnostic; every other non-prompt_error
// frame calls observePromptAttempt with the table's fixed output/effect pair.
func replayEvidenceLayer(service *Service, fx replayfixtures.Fixture) watcher.AgentEventData {
	service.beginPromptAttempt(fx.Identity.SessionID, fx.Identity.ExecutionID, fx.Identity.PromptGeneration, false)

	var promptErrorFrame replayfixtures.Frame
	tokenIndex := 0
	for _, frame := range fx.Frames {
		switch frame.Kind {
		case replayfixtures.FrameModelSettled:
			continue
		case replayfixtures.FramePromptError:
			promptErrorFrame = frame
			continue
		case replayfixtures.FrameMessageChunk:
			token := fx.Expect.Events[tokenIndex]
			tokenIndex++
			if strings.HasSuffix(token, ":diagnostic") {
				service.observeProviderDiagnostic(fx.Identity.SessionID, fx.Identity.ExecutionID, fx.Identity.PromptGeneration, frame.Text)
			} else {
				service.observePromptAttempt(fx.Identity.SessionID, fx.Identity.ExecutionID, fx.Identity.PromptGeneration, true, false)
			}
		case replayfixtures.FrameThoughtChunk:
			tokenIndex++
			service.observePromptAttempt(fx.Identity.SessionID, fx.Identity.ExecutionID, fx.Identity.PromptGeneration, true, false)
		case replayfixtures.FrameToolCall, replayfixtures.FrameToolUpdate:
			tokenIndex++
			service.observePromptAttempt(fx.Identity.SessionID, fx.Identity.ExecutionID, fx.Identity.PromptGeneration, false, true)
		}
	}

	return watcher.AgentEventData{
		SessionID:           fx.Identity.SessionID,
		AgentExecutionID:    fx.Identity.ExecutionID,
		PromptGeneration:    fx.Identity.PromptGeneration,
		EvidenceKnown:       true,
		DynamicRouteAttempt: false,
		ErrorMessage:        promptErrorFrame.Message,
		ProviderError: &streams.ProviderError{
			Source:     fx.Expect.ProviderError.Source,
			ProviderID: fx.Expect.ProviderError.ProviderID,
			ModelID:    fx.Expect.ProviderError.ModelID,
			RPCCode:    fx.Expect.ProviderError.RPCCode,
			ErrorKind:  fx.Expect.ProviderError.ErrorKind,
		},
	}
}

// TestReplayFixtureEvidenceLayer drives promptAttemptPreResultSafe from the
// shared ACP replay fixture corpus. It does not re-run the transport and does
// not re-derive the event sequence; it replays the fixed per-frame mapping
// the design specifies and asserts the fence outcome the fixture declares.
func TestReplayFixtureEvidenceLayer(t *testing.T) {
	fixtures := replayfixtures.MustLoad()

	for _, fx := range fixtures {
		t.Run(fx.FileName, func(t *testing.T) {
			var service Service
			data := replayEvidenceLayer(&service, fx)

			got := service.promptAttemptPreResultSafe(data)
			if got != fx.Expect.PreResultSafe {
				t.Fatalf("promptAttemptPreResultSafe = %v, want %v", got, fx.Expect.PreResultSafe)
			}
		})
	}
}

// TestReplayFixtureEvidenceLayerIsReplayIdempotent pins the design's
// determinism claim: replaying the same fixture twice in the same process
// produces the same fence verdict, because beginPromptAttempt stores a fresh
// evidence record under the fixture's session id, replacing any record left
// behind by an earlier run.
func TestReplayFixtureEvidenceLayerIsReplayIdempotent(t *testing.T) {
	fixtures := replayfixtures.MustLoad()
	fx := fixtures[0]

	var first, second bool
	{
		var service Service
		first = service.promptAttemptPreResultSafe(replayEvidenceLayer(&service, fx))
	}
	{
		var service Service
		second = service.promptAttemptPreResultSafe(replayEvidenceLayer(&service, fx))
	}
	if first != second {
		t.Fatalf("replay was not idempotent: first=%v second=%v", first, second)
	}
	if first != fx.Expect.PreResultSafe {
		t.Fatalf("preResultSafe = %v, want %v", first, fx.Expect.PreResultSafe)
	}
}
