---
id: "04-surface-recoverable-failure-detail"
title: "Surface sanitized failure detail on post-start recoverable failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.10
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 04: Surface sanitized failure detail on post-start recoverable failures

## Summary

A post-start recoverable failure (for example the model provider rejecting a
dispatched prompt with `Tool schema does not support oneOf, allOf, or anyOf at
the top level`) shows only the short summary line; the collapsed technical-details
disclosure is empty because `createRecoveryStatusMessage` only populates
`error_output` for bootstrap, managed-runtime-npm, and provider-quota failures.
Populate `error_output` with the sanitized failure detail for post-start
recoverable failures so the existing disclosure renders it. No frontend change.

## In scope

- In `createRecoveryStatusMessage` (`internal/orchestrator/event_handlers_agent.go`),
  set `meta["error_output"] = routingerr.Sanitize(data.FailureDetails)` for
  post-start recoverable failures when the sanitized result is non-empty, matching
  the bootstrap / managed-runtime-npm / quota paths already present.
- Keep the existing paths intact: do not double-populate or override
  `error_output` when a more specific path (quota) already set it, and keep
  `remediation_url` a separate field.
- Omit `error_output` when sanitization yields an empty string, leaving the
  generic recovery card.

## Out of scope

- Any frontend change: `ActionMessageDetails` / `TechnicalDetails`
  (`apps/web/components/task/chat/messages/action-message-details.tsx`) already
  render `error_output` in an initially collapsed disclosure.
- Changing sanitization rules in `routingerr.Sanitize`, adding new metadata
  fields, or changing the summary line copy.
- The empty-turn settlement owned by Task 03.

## Acceptance

- A post-start recoverable failure with usable detail persists a recovery entry
  whose `error_output` metadata carries the sanitized detail, and the collapsed
  disclosure renders it.
- When sanitization leaves nothing usable, `error_output` is omitted and the
  generic recovery card is shown.
- `error_output` never contains raw URLs, credentials, identifiers, or raw agent
  stderr; `remediation_url` remains a distinct field.

## Verification

Add failing tests first, then implement. Run from the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'RecoveryStatus|ErrorOutput|Recoverable' -count=1)
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- Orchestrator recovery-status tests
  (`recovery_actions_test.go` or an adjacent test file), asserting the populated
  and omitted `error_output` cases.

## Dependencies

None. Independent of Tasks 01, 02, and 03.

## Risks

The change must not widen exposure: `error_output` stays sanitized and is omitted
when empty. Guard against overriding a more specific classification (quota) that
already sets `error_output`, and cover both the populated and empty-sanitization
cases so raw detail never leaks.

## Parallelism

`sequential`. Owns the `error_output` population in `createRecoveryStatusMessage`.

## Inputs

- [Session recovery failures requirement](../../specs/agents/requirements/session-recovery-failures.md),
  AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.10.
- [Session recovery failures design](../../specs/agents/system-design/session-recovery-failures.md),
  post-start recoverable failure detail section.
- `createRecoveryStatusMessage`, `persistRecoveryStatusMessage`,
  `applyProviderQuotaMetadata`, `routingerr.Sanitize`, and the existing
  `error_output` renderer `ActionMessageDetails` / `TechnicalDetails`.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Implemented. Extracted `applyRecoverableFailureDetail(meta, data)` in
`event_handlers_agent.go`: it sets `meta["error_output"] =
routingerr.Sanitize(data.FailureDetails)` only when non-empty and when a more
specific path (quota, via `applyProviderQuotaMetadata`) has not already set it.
The duplicate inline `error_output` blocks on the managed-runtime-npm and
bootstrap paths were folded into this single call, so every non-quota class now
surfaces its sanitized detail through one seam (the npm path keeps setting
`failure_kind`). Tests in `recovery_actions_test.go`:
`TestCreateRecoveryStatusMessage_PostStartFailureSurfacesSanitizedDetail`
(sanitized detail populated, embedded credential redacted, no `failure_kind`)
and `TestCreateRecoveryStatusMessage_PostStartFailureOmitsEmptyDetail` (omitted
when empty). Existing npm, quota, and generic-remediation tests still pass.
Changed-code `golangci-lint` on `./internal/orchestrator/...` is clean and the
full orchestrator package passes.
