package api

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// recordingAdapter embeds the interface so the fake satisfies the whole
// AgentAdapter surface while implementing only the two calls promptOrSteer can
// route to. Any other method would panic on a nil embedded interface, which is
// what we want: this test must fail loudly if routing starts touching more.
type recordingAdapter struct {
	adapter.AgentAdapter

	promptCalls []promptRecord
	steerCalls  []promptRecord
}

type promptRecord struct {
	text        string
	attachments []v1.MessageAttachment
	generation  uint64
}

func (r *recordingAdapter) Prompt(
	_ context.Context,
	message string,
	attachments []v1.MessageAttachment,
	promptGeneration uint64,
) error {
	r.promptCalls = append(r.promptCalls, promptRecord{message, attachments, promptGeneration})
	return nil
}

// steerableAdapter adds the optional SteerablePrompter half. supportsSteering
// mirrors the agent's negotiated capability advertisement.
type steerableAdapter struct {
	*recordingAdapter
	supportsSteering bool
}

func (s *steerableAdapter) SupportsSteering() bool { return s.supportsSteering }

func (s *steerableAdapter) PromptSteer(
	_ context.Context,
	message string,
	attachments []v1.MessageAttachment,
	promptGeneration uint64,
) error {
	s.steerCalls = append(s.steerCalls, promptRecord{message, attachments, promptGeneration})
	return nil
}

// TestPromptOrSteerRouting pins the server-side admission gate. The load-bearing
// property is the fallback: an adapter that cannot steer — because it does not
// implement SteerablePrompter at all, or because the connected agent never
// advertised the capability — must still deliver the message as an ordinary
// prompt. Dropping it, or erroring, would make a steer-eligible composer lose
// operator input on an agent install that is merely too old.
func TestPromptOrSteerRouting(t *testing.T) {
	attachments := []v1.MessageAttachment{{Type: "image", Name: "shot.png"}}

	tests := []struct {
		name        string
		steerable   bool
		advertised  bool
		reqSteer    bool
		wantSteered bool
	}{
		{name: "steer request on a capable adapter steers", steerable: true, advertised: true, reqSteer: true, wantSteered: true},
		{name: "steer request without advertisement falls back to prompt", steerable: true, advertised: false, reqSteer: true},
		{name: "steer request on a non-steerable adapter falls back to prompt", reqSteer: true},
		{name: "ordinary prompt on a capable adapter never steers", steerable: true, advertised: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingAdapter{}
			var adpt adapter.AgentAdapter = rec
			if tt.steerable {
				adpt = &steerableAdapter{recordingAdapter: rec, supportsSteering: tt.advertised}
			}

			req := PromptRequest{
				Text:             "read the spec first",
				Attachments:      attachments,
				PromptGeneration: 9,
				Steer:            tt.reqSteer,
			}
			if err := promptOrSteer(context.Background(), adpt, req); err != nil {
				t.Fatalf("promptOrSteer: %v", err)
			}

			steered, prompted := len(rec.steerCalls), len(rec.promptCalls)
			if tt.wantSteered && (steered != 1 || prompted != 0) {
				t.Fatalf("expected exactly one steer, got steer=%d prompt=%d", steered, prompted)
			}
			if !tt.wantSteered && (prompted != 1 || steered != 0) {
				t.Fatalf("expected exactly one ordinary prompt, got steer=%d prompt=%d", steered, prompted)
			}

			// Whichever path ran must carry the request through unchanged: the
			// generation especially, since a steer reuses the predecessor's.
			got := rec.promptCalls
			if tt.wantSteered {
				got = rec.steerCalls
			}
			if got[0].text != req.Text || got[0].generation != req.PromptGeneration {
				t.Fatalf("request not carried through: %+v", got[0])
			}
			if len(got[0].attachments) != 1 || got[0].attachments[0] != attachments[0] {
				t.Fatalf("attachments not carried through: %+v", got[0].attachments)
			}
		})
	}
}
