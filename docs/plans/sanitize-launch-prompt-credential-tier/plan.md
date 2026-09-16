---
created: 2026-09-17
status: done
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-04.md
legacy_specs: []
---

# Implementation Plan: Sanitize Launch Prompt With Credential Tier

## Overview

`Conductor.launchWithFallback` rendered the dynamic-agent primary launch
prompt through `ContinuationPrompt` on every downstream launch attempt,
including attempt 0. `ContinuationPrompt` ran that prompt through
`routingerr.Sanitize`, the diagnostic tier meant for provider output: it
collapses any 32-plus-character run to `***` (destroying file paths, commit
SHAs, PR URLs, and the task/session UUIDs the injected `<kandev-system>`
block carries), rewrites URLs down to scheme and host, and truncates at
`MaxRawExcerptBytes` (4096 bytes). Because the task/session UUIDs are how a
fallback provider's MCP tool calls address the task, this broke the fallback
path it was meant to protect, and it silently mangled ordinary user-authored
prompt content on the very first launch attempt.

This is a defect in already-shipped behavior (PR #3407 / commit `9dea4b175`),
not a new capability: fix the tier, not the shape of the feature.

## Scope

### In scope

- Route the primary launch prompt through `routingerr.SanitizeCredentials`'s
  rule set instead of `routingerr.Sanitize`.
- Add an unbounded variant so the primary prompt is not truncated at
  `MaxRawExcerptBytes` (unlike the continuation package's bounded fields).
- Regression coverage proving a long user-authored prompt survives whole on
  every launch attempt while embedded credentials are still redacted.
- Record the tier split in the provider-error-recovery system design and
  requirements.

### Out of scope

- Hardening the redaction rule sets themselves (signed-URL query-credential
  gap, tightened patterns) — tracked separately as `76e284d2`.
- `sanitizeContinuation` and the continuation package's `Conversation`,
  `ToolSummary`, and `FailureReason` fields, which keep the diagnostic tier
  unchanged.
- Any change to retry policy, provider selection, or fallback ordering.

## Technical approach

- `apps/backend/internal/agent/runtime/routingerr/sanitize.go`: add
  `SanitizeCredentialsUnbounded`, calling the existing
  `applyRedactionsUnbounded` helper with the existing `credentialRedactions`
  rule set (no new rules).
- `apps/backend/internal/agent/runtime/dynamic/conductor.go`: swap the tier
  `ContinuationPrompt` applies to the primary prompt from
  `routingerr.Sanitize` to `routingerr.SanitizeCredentialsUnbounded`.
- `docs/specs/platform/system-design/provider-error-recovery-04.md`: extend
  the existing continuation-package sanitization-tiers section to cover the
  primary launch prompt.
- `docs/specs/platform/requirements/provider-error-recovery.md`: add
  `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.29` describing the credential-only,
  unbounded tier for the primary launch prompt.

## Tests

- **What:** A prompt longer than `MaxRawExcerptBytes` survives whole (no
  truncation, no path/SHA/URL/UUID scrambling) on every downstream launch
  attempt, while `sk-`/`ghp_`/`API_KEY=`-shaped credentials in that same
  prompt are still redacted.
  **File:**
  `apps/backend/internal/agent/runtime/dynamic/conductor_test.go`
  (`TestConductorPreservesLongUserAuthoredPromptOnEveryDownstreamLaunch`).
  **How:** Go table test asserting the rendered prompt at attempt 0 and a
  fallback attempt.
- **What:** `SanitizeCredentialsUnbounded` preserves length/content and still
  redacts credential shapes.
  **File:** `apps/backend/internal/agent/runtime/routingerr/sanitize_test.go`
  (`TestSanitizeCredentialsUnbounded_PreservesLengthAndContent`,
  `TestSanitizeCredentialsUnbounded_RedactsCredentials`).

## Work orders

- [x] [Task 01: Sanitize launch prompt with credential tier](task-01-sanitize-launch-prompt-credential-tier.md)

## Verification results

```
make -C apps/backend lint            -> 0 issues.
make typecheck                       -> exit 0
go test ./internal/agent/runtime/dynamic/... ./internal/agent/runtime/routingerr/...  -> PASS
```

Mutation check: reverting the tier swap in `conductor.go` fails
`TestConductorPreservesLongUserAuthoredPromptOnEveryDownstreamLaunch` with
`launch 0 prompt lost "/Users/alice/work/repo/main.go"`.

## Open questions

None.
