package routingerr

import "regexp"

// runtimeEnvironmentRules match failures caused by the local execution
// environment (npm/npx cache state, missing binaries' install steps,
// etc.) rather than by the provider or remote API. They are tried by
// Classify after provider-specific rules and before the phase-based
// fallback, so a specific environment fingerprint wins over the
// low-confidence "phase.prestart.unknown" verdict.
//
// Each entry pairs a regex matcher with a builder that can extract
// signal-specific metadata (e.g. RemediationPath) from the raw text.
var runtimeEnvironmentRules = []runtimeRule{
	{
		// npm 9 and npm 10 use different casing for the error prefix. Require
		// both the ETARGET code and the matching package@version diagnostic in
		// one bounded sample so generic disconnects and registry errors do not
		// trigger managed-runtime recovery.
		id:            "npm.etarget.managed_runtime.v1",
		pattern:       managedRuntimeNpmResolutionRe,
		sanitizedOnly: true,
		build: func(string) *Error {
			return &Error{
				Code:       CodeManagedRuntimeNpmResolution,
				Confidence: ConfHigh,
			}
		},
	},
	{
		id:      "npm.enotempty.npx.v1",
		pattern: regexp.MustCompile(`(?s)npm error code ENOTEMPTY.*?_npx/[0-9a-f]+`),
		build: func(text string) *Error {
			path := extractNpxCachePath(text)
			if path == "" {
				return nil
			}
			return &Error{
				Code:            CodeNpxCacheCorrupted,
				Confidence:      ConfHigh,
				RemediationPath: path,
			}
		},
	},
	{
		// Anthropic 400 surfaced by the claude-agent-acp adapter on a
		// prompt after session/load: the reconstructed history loses the
		// extended-thinking block signatures, so the API rejects the
		// modified `thinking`/`redacted_thinking` blocks. Provider-agnostic
		// because the signature is unique enough to never false-positive,
		// and any adapter routing to Anthropic models can hit it.
		id:      resumeCorruptedRuleID,
		pattern: thinkingBlocksImmutableRe,
		build: func(string) *Error {
			return &Error{
				Code:            CodeResumeCorrupted,
				Confidence:      ConfHigh,
				RemediationPath: RemediationStartFreshSession,
			}
		},
	},
	{
		// Anthropic 529 Overloaded surfaced over ACP as a prompt-time error
		// event: a transient, server-side condition the orchestrator should
		// retry with backoff rather than tear down. Provider-agnostic because
		// the "529 ... overloaded" / "overloaded_error" signature is unique
		// enough not to false-positive on unrelated logs.
		id:      overloadedRuleID,
		pattern: overloadedRe,
		build: func(string) *Error {
			return &Error{
				Code:       CodeProviderOverloaded,
				Confidence: ConfHigh,
			}
		},
	},
	{
		// Cursor emits this exact control prefix as an assistant message chunk
		// when its upstream HTTP/2 stream resets. Keep the fingerprint anchored
		// to the control frame and bounded so user-authored prose cannot turn
		// into an automatic retry.
		id:      cursorRetriableStreamResetRuleID,
		pattern: cursorRetriableStreamResetRe,
		build: func(text string) *Error {
			if cancellationOrDeadlineRe.MatchString(text) {
				return nil
			}
			return &Error{
				Code:       CodeAgentTransportLost,
				Confidence: ConfHigh,
			}
		},
	},
	{
		// The ACP transport pipe died mid-turn: the upstream provider service
		// dropped the connection before a response arrived. This is
		// provider-agnostic wire-level transport death, not a model or
		// availability problem — recoverable by retrying the same provider.
		// Appended last so the more specific resume-corrupted and
		// 529-overloaded rules above still win when signatures co-occur.
		id:      transportLostRuleID,
		pattern: transportLostRe,
		build: func(text string) *Error {
			// A context cancellation or deadline can be reported alongside the
			// same "connection closed" wording the transport-lost signature
			// matches on (e.g. "context canceled: connection closed"). Those
			// envelopes must keep falling through to manual recovery, so
			// reject them here rather than trust the substring match alone.
			if cancellationOrDeadlineRe.MatchString(text) {
				return nil
			}
			return &Error{
				Code:       CodeAgentTransportLost,
				Confidence: ConfHigh,
			}
		},
	},
}

var managedRuntimeNpmResolutionRe = regexp.MustCompile(
	`(?ism)^\s*npm\s+(ERR!|error)\s+code\s+ETARGET\b[\s\S]{0,999}^\s*npm\s+(ERR!|error)\s+notarget\s+No matching version found for\s+\S+@\S+`,
)

// cancellationOrDeadlineRe matches context-cancellation and deadline
// signatures that can co-occur with the transport-lost wording in the same
// error string. Kept separate from transportLostRe so the two can be
// combined without the transport-lost pattern itself growing lookahead
// complexity.
var cancellationOrDeadlineRe = regexp.MustCompile(`(?i)\bcontext (?:canceled|deadline exceeded)\b|\bcancel escalated\b`)

const resumeCorruptedRuleID = "anthropic.thinking_blocks.immutable.v1"

const overloadedRuleID = "anthropic.overloaded.529.v1"

const cursorRetriableStreamResetRuleID = "cursor.retriable_stream_reset.v1"

const transportLostRuleID = "acp.transport_lost.v1"

// transportLostRe matches the narrow ACP wire-level transport-death
// signatures: the peer disconnecting before a response, or the underlying
// connection closing outright. Deliberately narrow (only these two
// substrings) so it never matches context-cancellation or shutdown-teardown
// error strings, which must keep falling through to manual recovery.
var transportLostRe = regexp.MustCompile(`(?i)peer disconnected|connection closed`)

// cursorRetriableStreamResetRe matches Cursor's complete control diagnostic.
// Requiring the whole normalized message prevents ordinary provider prose from
// combining the control prefix with an unrelated transport fragment.
var cursorRetriableStreamResetRe = regexp.MustCompile(
	`(?i)^\s*Error:\s*RetriableError:\s*HTTP/2 stream closed with error code CANCEL \(0x8\)(?:\s+\[canceled\])?\s*$`,
)

// overloadedRe matches the transient 529 Overloaded signature: either the
// numeric code adjacent to "overloaded" on a single line (in either order), or
// the Anthropic "overloaded_error" type token. Kept tight ([^\n] stays within
// one line) so a stray "529" and "overloaded" in unrelated multi-line logs
// can't bridge into a false positive.
var overloadedRe = regexp.MustCompile(`(?i)\b529\b[^\n]*overloaded|overloaded[^\n]*\b529\b|\boverloaded_error\b`)

// IsTransientProviderError reports whether a provider error is eligible for a
// short same-provider retry. It is intentionally backed by Classify so new
// provider-neutral fingerprints (capacity and network failures) become
// available to callers outside the adapter without another allow-list.
func IsTransientProviderError(message string) bool {
	if message == "" {
		return false
	}
	e := Classify(Input{Phase: PhasePromptSend, Stderr: message})
	if e == nil || !e.AutoRetryable || e.UserAction {
		return false
	}
	switch e.Code {
	case CodeProviderOverloaded, CodeModelCapacity, CodeNetworkUnavailable, CodeProviderUnavailable, CodeAgentTransportLost:
		return true
	default:
		return false
	}
}

// thinkingBlocksImmutableRe matches the Anthropic "thinking blocks cannot be
// modified" 400 in either the `thinking` or `redacted_thinking` form. The
// gaps stay bounded ([^\n] is non-greedy-friendly within a single line) so a
// stray "thinking" elsewhere in a multi-line log can't accidentally bridge to
// an unrelated "cannot be modified".
var thinkingBlocksImmutableRe = regexp.MustCompile(`(?i)(?:redacted_)?thinking[^\n]*blocks[^\n]*cannot be modified`)

// IsResumeCorrupted reports whether the error message carries the
// resume-corrupted (thinking-blocks-immutable) signature. Exposed for callers
// outside the classify path (e.g. the orchestrator's recovery UI) that need to
// steer the user toward a fresh session without re-running full Classify.
func IsResumeCorrupted(message string) bool {
	return message != "" && thinkingBlocksImmutableRe.MatchString(message)
}

type runtimeRule struct {
	id            string
	pattern       *regexp.Regexp
	build         func(text string) *Error
	sanitizedOnly bool
}

// npxCachePathRe captures the cache root, e.g.
// `/Users/cfl/.npm/_npx/d820eb7d96bc2600` from any npm error line that
// references a file beneath it. The hash segment is hex (npm uses an
// 8-byte hex of the package spec). `[^\n]*?` lets the home prefix
// contain spaces (e.g. `/Users/John Doe/...`); RemediateNpxCache's
// `EvalSymlinks` + `$HOME/.npm/_npx/` prefix guard validates the path
// before any deletion, so we don't need the regex to do that work too.
var npxCachePathRe = regexp.MustCompile(`(/[^\n]*?/\.npm/_npx/[0-9a-f]+)`)

func extractNpxCachePath(text string) string {
	m := npxCachePathRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func matchRuntimeEnvironmentRules(text string) (*Error, bool) {
	return matchRuntimeRules(text, false)
}

func matchLegacyRuntimeEnvironmentRules(text string) (*Error, bool) {
	return matchRuntimeRules(text, true)
}

func matchRuntimeRules(text string, legacyOnly bool) (*Error, bool) {
	if text == "" {
		return nil, false
	}
	for _, r := range runtimeEnvironmentRules {
		if legacyOnly && r.sanitizedOnly {
			continue
		}
		if !r.pattern.MatchString(text) {
			continue
		}
		if e := r.build(text); e != nil {
			e.ClassifierRule = r.id
			return e, true
		}
	}
	return nil, false
}
