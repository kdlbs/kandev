---
id: "02-rate-throttle"
title: "Retry terminal rate throttles"
status: done
wave: 2
depends_on: ["01-opening-draft"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-RATE-SIGNATURE-001
  - REQ-PLATFORM-RATE-THROTTLE-RETRY-001
acceptance_criteria:
  - AC-AGENTS-CODEX-RATE-SIGNATURE-001.1
  - AC-AGENTS-CODEX-RATE-SIGNATURE-001.2
  - AC-AGENTS-CODEX-RATE-SIGNATURE-001.3
  - AC-PLATFORM-RATE-THROTTLE-RETRY-001.1
  - AC-PLATFORM-RATE-THROTTLE-RETRY-001.2
  - AC-PLATFORM-RATE-THROTTLE-RETRY-001.3
  - AC-PLATFORM-RATE-THROTTLE-RETRY-001.4
  - AC-PLATFORM-RATE-THROTTLE-RETRY-001.5
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 02: Retry terminal rate throttles

## Summary

Recognize the terminal Codex 429 signature. Admit safe interactive throttles
without reset metadata to the existing bounded retry owner.

## In scope

First add failing `TestCodexRetryExhausted429Classification` and
`TestInteractiveRateLimitWithoutReset` in `routingerr` test files. Cover exact
and prefixed signatures, prose, other statuses/providers, explicit quota, and
structured precedence. Test high/medium/low confidence and nil, zero, future,
expired, and long hints. Only absent hints gain the new high-confidence path;
retain conservative handling for supplied unusable hints.

Add `TestCodexRateLimitRetryEpisode` in orchestrator tests. Feed the normal
terminal projection through classification and recovery admission. Verify five
attempts and their delays using controlled time, exhaustion, cancellation,
supersession, stale frames, uncertain dispatch, and no duplicate message.
Test no-output replay and refusal with completed plus unresolved tool evidence.
Do not infer Codex continuation support from this classifier change.

Keep Office missing-hint behavior unchanged. Test the same error in both contexts
and retain dynamic policy tests. Add a deterministic mock-agent terminal-429
scenario by extending the existing overloaded scenario pattern. Do not emit
ordinary assistant prose as the only simulated failure. Verify the sanitized
terminal message reaches the shared classifier; use a synthetic fixture labelled
as such, not a claimed recorded ACP capture.

Extend desktop and phone transient-retry E2E with a Quick Chat opening prompt:
429 then success, cancel during wait, one current notice, empty accepted input,
one user message, and reload without duplicate dispatch.

## Out of scope

Provider-native retries, raw Data extraction, new continuation capability,
Office scheduling changes, provider fallback, capacity-policy changes.

## Acceptance

- Exact terminal 429 classifies, negative and quota cases retain their behavior.
- Safe high-confidence missing-hint throttles use five bounded attempts.
- Backend-owned progress and cancellation work on desktop and phone.

## ASCII UI preview

UI-01: Accepted Quick Chat opening message. Shared content order on desktop
and phone; desktop uses the existing dialog, phone the existing full-height chat.

```text
Before: transcript [opening message]    composer [opening message]
After:  transcript [opening message]    composer [empty, or newer draft]
Retry:  transcript [opening message]
        [Rate limit | attempt 1/5 | countdown | Cancel]
        composer [empty, or newer draft]
```

The transcript scrolls; the existing composer remains at the bottom. Keep phone
safe-area handling and touch targets. Reuse existing localized recovery copy.
The empty accepted draft and single retry notice are required; wording above is
illustrative. No new layout, navigation, picker, or control is proposed.

Full view: [UI-01](plan.md#ascii-ui-preview). Covers throttle .4.

## Verification

Run from the repository root. Run these commands sequentially.

```bash
(cd apps/backend && go test ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(CodexRateLimit|Transient|ProviderInterruption|Continuation|Retry)')
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp ./cmd/mock-agent)
(cd apps/web && pnpm e2e:run --project chromium tests/session/transient-retry.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-transient-retry.spec.ts)
```

Run `bash scripts/lint-go-changed` with the explicit changed Go file paths.
If additional test names do not match the targeted pattern, add their exact
names to the command before marking this work order done.

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/rules.go`
- `apps/backend/internal/agent/runtime/routingerr/policy.go`
- `apps/backend/internal/agent/runtime/routingerr/signature_rules_test.go`
- `apps/backend/internal/agent/runtime/routingerr/policy_test.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_test.go`
- `apps/backend/internal/orchestrator/provider_interruption_preparation.go` (only if integration requires it)
- ACP transport tests for the terminal projection path
- `apps/backend/cmd/mock-agent/handler.go`, `main.go`, and scenario tests
- Desktop/mobile transient-retry E2E files and shared helper

## Dependencies

Task 01, for combined Quick Chat input cleanup assertions.

## Risks

Avoid changing Office through a shared context-free predicate. A diagnostic
message may count as output; preserve the existing correlation and effect fence.

## Parallelism

`sequential`

## Inputs

Read the requirement and design documents named in frontmatter, the matching
sections of plan.md, existing source, and adjacent test patterns.

## Results

Implemented the exact terminal Codex 429 signature and high-confidence
interactive missing-reset policy. Office policy remains unchanged. The mock
Codex profile exercises the same terminal projection and shared classifier.
Quick Chat now displays the actionable retry status and cancellation control.
The accepted opening message stays in the transcript and does not return to
the composer after success.

Verification passed:

- `go test ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic`
- `go test ./internal/orchestrator -run 'Test.*(CodexRateLimit|Transient|ProviderInterruption|Continuation|Retry)'`
- `go test ./internal/agentctl/server/adapter/transport/acp ./cmd/mock-agent`
- `make build` from `apps/backend`
- `bash scripts/lint-go-changed` with all changed Go files: 0 issues.
- Desktop transient-retry E2E: 6 passed; phone E2E: 3 passed. After the
  Quick Chat message-surface extraction, the focused desktop checks passed 2
  tests and the phone check passed 1 test.
- The draft-ownership and multiline-restoration Vitest files passed 29 tests.
- Web typecheck and production Vite build passed. Vite reports chunk-size and
  ineffective dynamic-import warnings.
- ESLint and Prettier passed for the changed frontend files.
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, both public-doc validators, and
  `git diff --check` passed.

Public docs updated in `docs/public/developer-tools.md` with the five-attempt
Quick Chat retry behavior and its progress and cancel control. No preview
differences remain.
