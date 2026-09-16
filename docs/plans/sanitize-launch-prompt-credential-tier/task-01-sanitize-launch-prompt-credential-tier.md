---
id: "01-sanitize-launch-prompt-credential-tier"
title: "Sanitize launch prompt with credential tier"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.29
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-04.md
---

# Task 01: Sanitize Launch Prompt With Credential Tier

## Summary

`Conductor.launchWithFallback` ran the primary dynamic-agent launch prompt
through the diagnostic-tier `routingerr.Sanitize` on every downstream launch
attempt, including attempt 0. Swap it for the credential-only tier, applied
without truncation, so the prompt is never scrambled or cut for a reason that
only makes sense for provider output.

## In scope

- New `routingerr.SanitizeCredentialsUnbounded` (existing credential rule set,
  no byte cap).
- One-line tier swap in `ContinuationPrompt`'s caller in `conductor.go`.
- Regression tests proving a long prompt survives whole while credentials are
  still redacted, on attempt 0 and a fallback attempt.
- Requirements/system-design updates recording the tier split.

## Out of scope

- Redaction rule-set hardening (signed-URL query-credential gap) — `76e284d2`.
- `sanitizeContinuation` and the continuation package's diagnostic-tier
  fields (`Conversation`, `ToolSummary`, `FailureReason`).
- Retry policy, provider selection, or fallback ordering.

## Acceptance

- A prompt longer than `MaxRawExcerptBytes` (4096 bytes) is rendered whole,
  with no path/SHA/URL/UUID collapsed to `***`, on every downstream launch
  attempt.
- A `sk-`/`ghp_`/`API_KEY=`-shaped credential embedded in that same prompt is
  still redacted on every attempt.
- `sanitizeContinuation` and `FailureReason` keep the diagnostic tier
  unchanged.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/runtime/dynamic/... -run TestConductorPreservesLongUserAuthoredPromptOnEveryDownstreamLaunch -v)
(cd apps/backend && go test ./internal/agent/runtime/routingerr/... -run TestSanitizeCredentialsUnbounded -v)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/sanitize.go`
- `apps/backend/internal/agent/runtime/routingerr/sanitize_test.go`
- `apps/backend/internal/agent/runtime/dynamic/conductor.go`
- `apps/backend/internal/agent/runtime/dynamic/conductor_test.go`
- `docs/specs/platform/system-design/provider-error-recovery-04.md`
- `docs/specs/platform/requirements/provider-error-recovery.md`

## Dependencies

None.

## Risks

- Widening the tier the wrong direction (diagnostic instead of credential)
  would repeat the original defect for a different field.
- Reusing the bounded `SanitizeCredentials` entry point instead of an
  unbounded variant would keep truncating the primary prompt.

## Parallelism

`sequential`

## Inputs

- PR #3393 review finding (deferred to this card) and the accepted D10 gap in
  `provider-error-recovery.md`.
- Live-deployment probe of `routingerr.Sanitize` against a representative
  prompt (paths, a commit SHA, a PR URL, a task UUID), confirming the
  truncation and scrambling.

## Results

Added `SanitizeCredentialsUnbounded` in `sanitize.go` and swapped
`conductor.go`'s primary-prompt call from `routingerr.Sanitize` to it. Added
`TestConductorPreservesLongUserAuthoredPromptOnEveryDownstreamLaunch` and two
`SanitizeCredentialsUnbounded` tests. Reviewed through 4 rounds (Kandev
code-review, cross-vendor Codex, security) with no findings; mutation-tested
by reverting the tier swap.

Verified with:

```bash
(cd apps/backend && go test ./internal/agent/runtime/dynamic/... -run TestConductorPreservesLongUserAuthoredPromptOnEveryDownstreamLaunch -v -count=1)
(cd apps/backend && go test ./internal/agent/runtime/routingerr/... -run TestSanitizeCredentialsUnbounded -v -count=1)
```
