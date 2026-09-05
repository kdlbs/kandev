package orchestrator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
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
// The second return value is the diagnostic code recorded after every frame
// but before the terminal prompt_error is evaluated, read directly off the
// evidence record rather than inferred from the fence outcome, so a fixture
// can pin the recorded state independent of whether it happens to be
// pre-result safe.
func replayEvidenceLayer(service *Service, fx replayfixtures.Fixture) (watcher.AgentEventData, routingerr.Code) {
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

	var recorded routingerr.Code
	if evidence, ok := service.promptAttemptForSession(fx.Identity.SessionID); ok {
		evidence.mu.Lock()
		recorded = evidence.providerDiagnosticCode
		evidence.mu.Unlock()
	}

	// The transport layer never hands the evidence layer a raw terminal
	// message: SendErrorEventWithProviderError's caller sets the event's
	// error text to providerError.Message once a provider diagnostic exists
	// (server/api/agent.go), and that Message is always
	// streams.SanitizeProviderMessage's output. Comparing against the raw
	// fixture text here would let the fixture matrix pass even when
	// sanitization changes the terminal message enough to break AC.23
	// containment — which is exactly the CRIT-001 regression class this
	// layer exists to catch.
	sanitizedMessage := streams.SanitizeProviderMessage(promptErrorFrame.Message)

	return watcher.AgentEventData{
		SessionID:           fx.Identity.SessionID,
		AgentExecutionID:    fx.Identity.ExecutionID,
		PromptGeneration:    fx.Identity.PromptGeneration,
		EvidenceKnown:       true,
		DynamicRouteAttempt: false,
		ErrorMessage:        sanitizedMessage,
		ProviderError: &streams.ProviderError{
			Source:     fx.Expect.ProviderError.Source,
			ProviderID: fx.Expect.ProviderError.ProviderID,
			ModelID:    fx.Expect.ProviderError.ModelID,
			RPCCode:    fx.Expect.ProviderError.RPCCode,
			ErrorKind:  fx.Expect.ProviderError.ErrorKind,
			Message:    sanitizedMessage,
		},
	}, recorded
}

// TestReplayFixtureEvidenceLayer drives promptAttemptPreResultSafe from the
// shared ACP replay fixture corpus. It does not re-run the transport and does
// not re-derive the event sequence; it replays the fixed per-frame mapping
// the design specifies and asserts both outcomes provider-error-recovery-02.md
// assigns to this layer: the recorded diagnostic code after the frames, and
// the fence outcome. Asserting only the fence outcome would let a fixture
// like `*-prose-output.json` pass for the wrong reason — its `PreResultSafe:
// false` is also satisfied by containment failing on prose, so a broken
// clearing rule would go unnoticed by that assertion alone.
func TestReplayFixtureEvidenceLayer(t *testing.T) {
	fixtures := replayfixtures.MustLoad()

	for _, fx := range fixtures {
		t.Run(fx.FileName, func(t *testing.T) {
			var service Service
			data, recorded := replayEvidenceLayer(&service, fx)

			if string(recorded) != fx.Expect.RecordedDiagnosticCode {
				t.Fatalf("recorded diagnostic code = %q, want %q", recorded, fx.Expect.RecordedDiagnosticCode)
			}

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
		data, _ := replayEvidenceLayer(&service, fx)
		first = service.promptAttemptPreResultSafe(data)
	}
	{
		var service Service
		data, _ := replayEvidenceLayer(&service, fx)
		second = service.promptAttemptPreResultSafe(data)
	}
	if first != second {
		t.Fatalf("replay was not idempotent: first=%v second=%v", first, second)
	}
	if first != fx.Expect.PreResultSafe {
		t.Fatalf("preResultSafe = %v, want %v", first, fx.Expect.PreResultSafe)
	}
}
