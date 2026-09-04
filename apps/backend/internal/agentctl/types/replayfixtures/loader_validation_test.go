package replayfixtures

import "testing"

// withFrameBeforeError inserts frame immediately before f.Frames' trailing
// prompt_error frame, so validFixture()'s "exactly one prompt_error, and it
// is last" invariant stays satisfied while a new frame kind is exercised.
func withFrameBeforeError(f *Fixture, frame Frame) {
	last := len(f.Frames) - 1
	f.Frames = append(f.Frames[:last:last], frame, f.Frames[last])
}

// TestValidateFixtureRejectsStructuralViolations is a table-driven negative
// test over validateFixture's structural rejection branches. Sanitisation
// rejection is covered separately by sanitisation_test.go. Each case mutates
// one otherwise-valid fixture so it fails for exactly the reason named;
// deleting the corresponding check in loader.go should turn its case green.
func TestValidateFixtureRejectsStructuralViolations(t *testing.T) {
	cases := map[string]func(*Fixture){
		"unknown gateway":                   func(f *Fixture) { f.Gateway = "unknown-gateway" },
		"classifying disagrees with matrix": func(f *Fixture) { f.Classifying = false },
		"unknown case":                      func(f *Fixture) { f.Case = "unknown-case" },
		"empty agentId":                     func(f *Fixture) { f.AgentID = "" },

		"recorded fixture requires source": func(f *Fixture) {
			f.Capture, f.Source, f.CapturedAt = CaptureRecorded, "", "2026-01-01T00:00:00Z"
		},
		"recorded fixture requires capturedAt": func(f *Fixture) {
			f.Capture, f.Source, f.CapturedAt = CaptureRecorded, "internal/x_test.go", ""
		},
		"reconstructed fixture requires source": func(f *Fixture) {
			f.Source = ""
		},
		"reconstructed source citing a capture command": func(f *Fixture) {
			f.Source = "scripts/capture.sh"
		},
		"reconstructed fixture must not carry capturedAt": func(f *Fixture) {
			f.CapturedAt = "2026-01-01T00:00:00Z"
		},
		"unknown capture": func(f *Fixture) { f.Capture = "unknown-capture" },

		"empty identity.sessionId":       func(f *Fixture) { f.Identity.SessionID = "" },
		"empty identity.executionId":     func(f *Fixture) { f.Identity.ExecutionID = "" },
		"zero identity.promptGeneration": func(f *Fixture) { f.Identity.PromptGeneration = 0 },

		"empty frames":       func(f *Fixture) { f.Frames = nil },
		"unknown frame kind": func(f *Fixture) { withFrameBeforeError(f, Frame{Kind: "unknown"}) },

		"message_chunk requires non-empty text": func(f *Fixture) { f.Frames[0].Text = "   " },
		"message_chunk unknown role":            func(f *Fixture) { f.Frames[0].Role = "system" },

		"thought_chunk requires non-empty text": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameThoughtChunk, Text: "  "})
		},
		"thought_chunk role must be unset": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameThoughtChunk, Text: "thinking", Role: "assistant"})
		},

		"tool_call requires toolCallId": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameToolCall, Title: "Read"})
		},
		"tool_call role must be unset": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameToolCall, ToolCallID: "t1", Role: "assistant"})
		},
		"tool_update requires toolCallId": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameToolUpdate, Status: "completed"})
		},
		"tool_update names an unintroduced toolCallId": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameToolUpdate, ToolCallID: "never-called", Status: "completed"})
		},
		"tool_update role must be unset": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameToolCall, ToolCallID: "t1"})
			withFrameBeforeError(f, Frame{Kind: FrameToolUpdate, ToolCallID: "t1", Status: "completed", Role: "assistant"})
		},

		"model_settled requires modelId": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameModelSettled})
		},
		"model_settled role must be unset": func(f *Fixture) {
			withFrameBeforeError(f, Frame{Kind: FrameModelSettled, ModelID: "m1", Role: "assistant"})
		},

		"prompt_error must be the last frame": func(f *Fixture) {
			f.Frames = append(f.Frames, Frame{Kind: FrameMessageChunk, Role: "assistant", Text: "trailing"})
		},
		"prompt_error requires message": func(f *Fixture) {
			f.Frames[len(f.Frames)-1].Message = ""
		},
		"frames must carry exactly one prompt_error frame, got 0": func(f *Fixture) {
			f.Frames = f.Frames[:len(f.Frames)-1]
		},
		"frames must carry exactly one prompt_error frame, got 2": func(f *Fixture) {
			f.Frames = append(f.Frames, Frame{Kind: FramePromptError, Code: -1, Message: "second error"})
		},

		"expect.events is required": func(f *Fixture) { f.Expect.Events = nil },
		"expect.events unknown token": func(f *Fixture) {
			f.Expect.Events = []string{"not_a_real_token", "error"}
		},
		"expect.events must end with the error token": func(f *Fixture) {
			f.Expect.Events = []string{"message_chunk:diagnostic"}
		},
		"expect.providerError.source is required":     func(f *Fixture) { f.Expect.ProviderError.Source = "" },
		"expect.providerError.providerId is required": func(f *Fixture) { f.Expect.ProviderError.ProviderID = "" },
		"expect.diagnosticCode is required":           func(f *Fixture) { f.Expect.DiagnosticCode = "" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := validFixture()
			mutate(&f)
			if err := validateFixture(f); err == nil {
				t.Fatalf("validateFixture() error = nil, want an error for %q", name)
			}
		})
	}
}

// TestValidateCorpusRejectsDuplicatePair pins that two fixtures declaring the
// same (gateway, case) are rejected, independent of their frame content.
func TestValidateCorpusRejectsDuplicatePair(t *testing.T) {
	a := validFixture()
	a.FileName = "a.json"
	b := validFixture()
	b.FileName = "b.json"
	b.Identity.SessionID = "sanitisation-test-2"
	b.Identity.ExecutionID = "exec-sanitisation-test-2"

	if err := validateCorpus([]Fixture{a, b}); err == nil {
		t.Fatal("validateCorpus() error = nil, want an error for a duplicate (gateway, case) pair")
	}
}

// TestValidateCorpusRejectsDuplicateSessionID pins that two fixtures sharing
// identity.sessionId are rejected even when their (gateway, case) pairs
// differ, since the recovery-evidence layer keys correlation off sessionId.
func TestValidateCorpusRejectsDuplicateSessionID(t *testing.T) {
	a := validFixture()
	a.FileName = "a.json"
	b := validFixture()
	b.FileName = "b.json"
	b.Gateway = GatewayTeamClaude
	b.Case = CaseMismatched

	if err := validateCorpus([]Fixture{a, b}); err == nil {
		t.Fatal("validateCorpus() error = nil, want an error for a duplicate identity.sessionId")
	}
}

// TestValidateCorpusRejectsEquivalentFramesWithinACase pins
// AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.17's dedup rule directly:
// canonicalFrames compares the decoded, re-marshaled frame values, so two
// fixtures with byte-identical Frames content in the same case are rejected
// even though their gateway, identity and every other field differ. Deleting
// this check (or canonicalFrames itself) leaves this test the only guard
// against two fixtures silently asserting the same scenario twice.
func TestValidateCorpusRejectsEquivalentFramesWithinACase(t *testing.T) {
	a := validFixture()
	a.FileName = "a.json"
	b := validFixture()
	b.FileName = "b.json"
	b.Gateway = GatewayTeamClaude
	b.AgentID = "teamclaude-acp"
	b.Identity.SessionID = "sanitisation-test-2"
	b.Identity.ExecutionID = "exec-sanitisation-test-2"
	// b.Case and b.Frames are left identical to a's — only identity, gateway
	// and agentId differ, none of which canonicalFrames considers.

	if err := validateCorpus([]Fixture{a, b}); err == nil {
		t.Fatal("validateCorpus() error = nil, want an error for two same-case fixtures carrying equivalent frames")
	}
}

// TestValidateCorpusRejectsMissingRequiredCells pins that validateCorpus
// itself (not only the MissingRequiredCells helper it calls) surfaces a
// required-cell gap as a load error.
func TestValidateCorpusRejectsMissingRequiredCells(t *testing.T) {
	if err := validateCorpus(nil); err == nil {
		t.Fatal("validateCorpus() error = nil, want an error for an empty corpus missing every required cell")
	}
}
