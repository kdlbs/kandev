package replayfixtures

import "testing"

// validFixture returns a minimal fixture that satisfies every structural rule
// validateFixture enforces elsewhere, so a sanitisation test that mutates one
// field fails only on sanitisation, never on an unrelated structural check.
func validFixture() Fixture {
	return Fixture{
		Gateway:     GatewayClaudeDirect,
		AgentID:     "claude-acp",
		Classifying: true,
		Case:        CaseMatched,
		Capture:     CaptureReconstructed,
		Source:      "internal/orchestrator/dynamic_evidence_test.go",
		Identity: Identity{
			SessionID:        "sanitisation-test",
			ExecutionID:      "exec-sanitisation-test",
			PromptGeneration: 1,
		},
		Frames: []Frame{
			{Kind: FrameMessageChunk, Role: "assistant", Text: "API Error: 500 Internal server error."},
			{Kind: FramePromptError, Code: -32603, Message: "API Error: 500 Internal server error."},
		},
		Expect: Expect{
			Events:         []string{"message_chunk:diagnostic", "error"},
			ProviderError:  ProviderErrorExpectation{Source: "acp_prompt", RPCCode: -32603, ProviderID: "claude-acp"},
			DiagnosticCode: "provider_unavailable",
			PreResultSafe:  true,
		},
	}
}

func TestValidateSanitisationAllowsCleanFixture(t *testing.T) {
	if err := validateSanitisation(validFixture()); err != nil {
		t.Fatalf("validateSanitisation() error = %v, want nil", err)
	}
}

// TestValidateFixtureRejectsForbiddenPayloadShapes pins AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.18:
// the payload sanitisation rule from provider-error-recovery-02.md#fixture-document
// ("Sanitisation, scoped") is the enforced definition, not an illustrative
// sample. Each case injects one forbidden shape into a frame's text and
// asserts Load-time rejection through validateFixture, the real enforcement
// path, not just the scanning helper.
func TestValidateFixtureRejectsForbiddenPayloadShapes(t *testing.T) {
	cases := map[string]string{
		"bearer token":           "Authorization: Bearer abc123XYZ",
		"vendor key prefix sk":   "leaked key sk-abcdef123456",
		"vendor key prefix sess": "leaked key sess-abcdef123456",
		"vendor key prefix org":  "leaked key org-abcdef123456",
		"opencode ses id":        "workspace ses_abc123",
		"opencode wrk id":        "workspace wrk_abc123",
		"url":                    "see https://internal.example.com/status for detail",
		"absolute path Users":    "wrote to /Users/alice/.config/kandev/token",
		"absolute path home":     "wrote to /home/alice/.config/kandev/token",
		"absolute path var":      "wrote to /var/secrets/kandev/token",
		"email address":          "contact ops@example.com for access",
		"rfc4122 uuid":           "account 4b1f6f1a-4a2b-4c3d-8e9f-0123456789ab is suspended",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			f := validFixture()
			f.Frames[0].Text = f.Frames[0].Text + " " + text
			if err := validateFixture(f); err == nil {
				t.Fatalf("validateFixture() error = nil, want a sanitisation error for %q", text)
			}
		})
	}
}

// TestValidateFixtureRejectsForbiddenPayloadShapesInExpect pins that the scan
// covers the expect subtree too, not only frames.
func TestValidateFixtureRejectsForbiddenPayloadShapesInExpect(t *testing.T) {
	f := validFixture()
	f.Expect.DiagnosticCode = "provider_unavailable leaked sk-abcdef123456"
	if err := validateFixture(f); err == nil {
		t.Fatalf("validateFixture() error = nil, want a sanitisation error for a forbidden shape in expect")
	}
}

// TestValidateFixtureRejectsForbiddenPayloadShapesInIdentity pins that a
// captured identifier pasted into identity.sessionId or identity.executionId
// is caught the same as one appearing in frame text — these fields are part
// of the replayed payload, not exempt from the scan.
func TestValidateFixtureRejectsForbiddenPayloadShapesInIdentity(t *testing.T) {
	cases := map[string]func(*Fixture){
		"sessionId":   func(f *Fixture) { f.Identity.SessionID = "4b1f6f1a-4a2b-4c3d-8e9f-0123456789ab" },
		"executionId": func(f *Fixture) { f.Identity.ExecutionID = "sess-abcdef123456" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := validFixture()
			mutate(&f)
			if err := validateFixture(f); err == nil {
				t.Fatalf("validateFixture() error = nil, want a sanitisation error for a forbidden shape in identity.%s", name)
			}
		})
	}
}

// TestValidateFixtureRejectsForbiddenProvenanceShapes pins the narrower
// provenance rule over capture/source/capturedAt: the same credential and
// identifier shapes, plus a private-host check, but — unlike the payload
// rule — a bare http(s) URL alone is not forbidden there.
func TestValidateFixtureRejectsForbiddenProvenanceShapes(t *testing.T) {
	cases := map[string]string{
		"bearer token":      "Bearer abc123XYZ",
		"vendor key prefix": "sk-abcdef123456",
		"opencode id":       "ses_abc123",
		"email address":     "ops@example.com",
		"rfc4122 uuid":      "4b1f6f1a-4a2b-4c3d-8e9f-0123456789ab",
		"userinfo in url":   "https://user:pass@example.com/docs",
		"localhost":         "http://localhost:4173/docs",
		"loopback literal":  "http://127.0.0.1:4173/docs",
		"rfc1918 10":        "http://10.1.2.3/docs",
		"rfc1918 172":       "http://172.16.0.5/docs",
		"rfc1918 192":       "http://192.168.1.5/docs",
		"internal host":     "http://kandev.internal/docs",
		"local host":        "http://kandev.local/docs",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			f := validFixture()
			f.Source = source
			if err := validateFixture(f); err == nil {
				t.Fatalf("validateFixture() error = nil, want a provenance sanitisation error for %q", source)
			}
		})
	}
}

// TestValidateFixtureAllowsPublicDocumentationURLInProvenance pins that the
// provenance rule permits a public documentation URL, since a reconstructed
// fixture must cite one and the payload rule's blanket URL prohibition does
// not apply here.
func TestValidateFixtureAllowsPublicDocumentationURLInProvenance(t *testing.T) {
	f := validFixture()
	f.Source = "https://openrouter.ai/docs/api-reference/errors"
	if err := validateFixture(f); err != nil {
		t.Fatalf("validateFixture() error = %v, want nil for a public documentation URL", err)
	}
}

// TestValidateFixtureRejectsAbsolutePathRegardlessOfPrecedingPunctuation pins
// that the "absolute host path" shape is caught by every boundary a real log
// line or stack trace uses, not only whitespace and quotes. Every other
// forbidden shape anchors on \b, which is punctuation-agnostic since it fires
// between any word/non-word transition; this shape can't use \b directly
// because '/' is not a word character, so its own boundary check must cover
// the same ground by other means.
func TestValidateFixtureRejectsAbsolutePathRegardlessOfPrecedingPunctuation(t *testing.T) {
	cases := map[string]string{
		"key-value form":  "env dump: path=/Users/alice/.ssh/id_rsa",
		"file URI form":   "resolved to file:///Users/alice/.config/kandev/token",
		"bracketed form":  "error[path:/home/bob/.aws/credentials]",
		"paren form, var": "wrote to(/var/secrets/kandev/token)",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			f := validFixture()
			f.Frames[0].Text = f.Frames[0].Text + " " + text
			if err := validateFixture(f); err == nil {
				t.Fatalf("validateFixture() error = nil for %q, want a sanitisation error (absolute host path leaked through an unblocked boundary character)", text)
			}
		})
	}
}
