---
id: "05-provider-failures"
title: "Preserve provider and current-turn failures"
status: in_progress
wave: 3
depends_on:
  - 02-subscription-policy
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-013
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-013.1
  - AC-AGENTS-ANTIGRAVITY-ACP-013.2
  - AC-AGENTS-ANTIGRAVITY-ACP-013.3
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 05: Preserve provider and current-turn failures

## Summary

Make provider failures reach existing recovery behavior in normal chat and Office.
Detect current-turn native error records when ACP otherwise returns a misleading successful stop.

## In scope

- Add pure structured mappings for auth, entitlement, model access, quota, throttling, and provider failure.
- Stop treating every -32000 with cached auth methods as an auth failure for Antigravity.
- Preserve valid reset/retry hints and unknown values without guessing allowance windows.
- Qualify the chosen runtime's native journal location/schema and create a narrow bounded read-only outcome reader.
- Record the pre-prompt high-water mark and inspect only new active-session error records, with path containment and identity checks.
- Use existing SQLite support, respect WAL, bound busy time and payloads, and reject links/replaced files.
- Treat validated native `/usage` as a non-model command using its RPC result. Do not require a trajectory row or exempt arbitrary slash-prefixed prompts.
- Return explicit outcome-verification failure if required native evidence is unavailable; never auto-replay or emit false success.
- Keep provider error handling in existing routing/recovery paths and retain a live native conversation for recoverable prompt errors.
- Add rendered error/recovery coverage on desktop and phone, preserving details after reload.

## Out of scope

General history parsing, token accounting from SQLite, assistant-text error heuristics, a new routing engine, quota polling, and automatic retry/model fallback.

## Acceptance

1. Structured failures produce the right existing recovery category and evidence-based reset hints in normal tasks and Office.
2. A new native error cannot become successful end_turn; old records and prose cannot create false failures. Unverifiable evidence withholds success explicitly.
3. Cancellation, process-generation ownership, and recoverable prompt errors preserve the conversation without automatic replay or workflow completion.

## ASCII UI preview

Error state within [UI-03](plan.md#ui-03-task-human-question-and-usage-details), covering 013.1-013.3.

```text
Desktop task recovery row             Phone stacked recovery details
+--------------------------------+    +----------------------------+
| Antigravity quota reached      |    | Antigravity quota reached  |
| Reset: provider value, if known|    | Reset information, if known|
| [Existing recovery actions]    |    | [Existing recovery action] |
+--------------------------------+    +----------------------------+
```

Use existing recovery controls for the normalized error. Unknown reset stays unknown.
The cause remains visible, wraps, and survives reload; no successful completion appears for the failed turn.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/routingerr -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run 'TestAntigravity|Test.*Prompt.*Error|Test.*Recovery' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp -run 'TestAntigravity' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/antigravity-failure.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-antigravity-failure.spec.ts)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

Named tests: `TestAntigravityErrorClassification`, `TestAntigravityCurrentTurnOutcome`, `TestAntigravityOutcomeEvidenceUnavailable`, and `TestAntigravityPromptErrorKeepsSession`.
Classification tests assert both existing Kanban and Office contexts.
Outcome tests cover a new database, stale errors, active-turn boundaries, successful follow-up, cancellation, replaced/symlinked files, schema mismatch, busy reads, and unknown error/reset data.
Browser tests assert the visible cause before dismissal and after reload, plus no successful completion for hidden failure.
Use fabricated journal fixtures. Run changed-Go lint and targeted web lint/typecheck if rendering or types change.

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/` Antigravity signatures and tests.
- `apps/backend/internal/agentctl/acpcompat/antigravity.go` pure error parsing.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/antigravity_outcome.go`, tests (new), and prompt lifecycle integration.
- Existing normalized task/Office error consumers only where wiring is missing; do not add parallel policy.
- `apps/web/e2e/tests/chat/antigravity-failure.spec.ts`, `mobile-antigravity-failure.spec.ts`, and synthetic provider fixtures.
- Existing recovery presentation and seven locale catalogs only if specific guidance needs new copy.

## Dependencies

Task 02 supplies provider policy and session/process identity. Native schema qualification may need a working vendor runtime from Task 01's platform evidence.

## Risks

- Private native schemas can change. Fail visibly when the necessary outcome proof is unavailable.
- Unbounded or cross-session journal reads can misattribute errors and expose unrelated data.
- Broad string matching can confuse assistant output with provider failure; parse only bounded native error envelopes.

## Parallelism

`sequential`

## Inputs

- Design section Failure and recovery and the accepted prompt-error runtime-lifetime ADR.
- T3 `AntigravityTurnOutcome.ts`, pinned in the design's evidence links.
- Existing routing error context tests and native ACP prompt lifecycle tests.

## Results

Implemented with a live-schema caveat.

- Added pure Antigravity structured error classification for authentication, entitlement, model access, quota, throttling, overload, provider failure, and unknown, preserving only provider-supplied reset hints. Generic `-32000` no longer implies authentication failure.
- Added bounded native ACP error-envelope parsing and a read-only journal reader for the candidate `conversations/<session>.db` `steps` schema, with containment, regular-file, identity, busy-time, and 16 KiB payload bounds. Only new current-turn error steps establish failure.
- Integrated the outcome check into the ACP prompt path. Unverifiable evidence produces an explicit outcome-verification failure and withholds success; missing native roots do not invent a failure. Recoverable prompt errors keep the same process and conversation.
- Added `TestAntigravityErrorClassification`, `TestAntigravityCurrentTurnOutcome`, `TestAntigravityOutcomeEvidenceUnavailable`, and `TestAntigravityPromptErrorKeepsSession`.

Verification: targeted and race tests passed; changed-Go lint 0 issues.

The exact vendor 1.4.5 journal location and schema were not re-qualified against a live runtime in this environment. Task 07 must confirm or correct the schema before claiming live hidden-error detection.

Desktop and phone failure-rendering E2E specs remain open; deterministic classification and outcome coverage is complete.
