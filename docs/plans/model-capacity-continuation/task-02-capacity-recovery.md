---
id: "02-capacity-recovery"
title: "Dispatch capacity continuation and prove recovery"
status: done
wave: 2
depends_on:
  - "01-capacity-evidence"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-003.1
  - AC-PLATFORM-TURN-CONTINUITY-003.2
  - AC-PLATFORM-TURN-CONTINUITY-003.3
  - AC-PLATFORM-TURN-CONTINUITY-003.4
  - AC-PLATFORM-TURN-CONTINUITY-003.5
  - AC-PLATFORM-TURN-CONTINUITY-003.6
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 02: Dispatch capacity continuation and prove recovery

## Summary

Admit capacity continuation after known completed effects and dispatch it on the same usable runtime.
Reuse the existing retry episode, status notice, and ordinary prompt seam.
Prove recovery, cancellation, and exhaustion through service tests and desktop/phone Chat.

## In scope

- Add policy-specific capacity binding, validation, instruction, and dispatch to the existing continuation owner.
- Preserve the original replay fence and transport-loss restoration policy.
- Refuse live capacity continuation when runtime, generation, configuration, queue, or work evidence changes.
- Keep five actual dispatch attempts across output and tools, with existing timing hints and local delays.
- Keep supported ownership boundaries and interruption settlement without successful workflow processing.
- Reuse current inline retry feedback and composer behavior.
- Update mock fixtures to respond to continuation instructions and count completed side effects.
- Retain unsupported after-tools refusal fixtures while adding positively supported scenarios.
- Update `docs/public/tasks-and-workflows.md` with capacity recovery and its limits when code lands.

## Out of scope

New UI composition, model switching, retry settings, provider-name policy branches, native restore after effects, and real-user task mutations.

## Acceptance

1. Completed-tool capacity recovers through one continuation instruction on the same runtime, without replay or duplicate effects.
2. The bounded episode survives progress, stops on supersession or unknown outcomes, and cannot restore after runtime loss.
3. Desktop and phone show one cancellable notice, retire it correctly, and retain usable Chat with truthful final attempt counts.

## ASCII UI preview

UI-01: Existing task Chat, capacity after completed tools. The [plan](plan.md#ascii-ui-preview) owns the full preview.

```text
Desktop and phone, waiting:
  [Completed tool calls]
  Model at capacity
  Continuing in 5s. Attempt 1 of 5.
  [Cancel]
  [Normal composer]

Success:
  [Completed tool calls]
  [Continued response]
  [Normal composer]
```

The existing phone Chat scroll owner, safe-area composer, and inline touch-accessible Cancel remain the composition.
Labels illustrate localized copy. No new hardcoded product strings are permitted.
Map this preview to criterion `.5` and both rendered scenario suites.

## Verification

Start with `TestCapacityContinuationAfterCompletedToolsUsesSameRuntime` in the orchestrator suite.
RED must show current admission refuses the supported completed-tool snapshot with zero started attempts.
Use Task 01's snapshot and actual retained failure identity.

Required service regressions:

- `TestCapacityContinuationAfterCompletedToolsUsesSameRuntime`.
- `TestCapacityContinuationBudgetSurvivesProgress`: five delays and dispatches, no sixth attempt after output/tools.
- `TestCapacityContinuationSuccessResetsEpisode`: a later failure receives a new budget.
- `TestCapacityContinuationSupersession`: cancellation, queued work, new prompt, archive/reset, and configuration changes.
- `TestCapacityContinuationRuntimeLossRefusesRestore`: loss or inconclusive probe causes no stop/resume/restore call.
- `TestCapacityContinuationFinalDisposition`: actual attempts, refusal, cancellation, and exhaustion.

Use barrier-controlled races for stale callbacks and ambiguous acceptance.
Keep existing Cursor and pre-result replay suites in the targeted command.
Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'Capacity|Continuation|TransientReplay|Retained|TurnFailure' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./cmd/mock-agent -run 'Capacity|Continuation' -count=1)
make -C apps/backend build
(cd apps/web && pnpm e2e:run tests/session/transient-turn-runtime-continuity.spec.ts --project chromium)
(cd apps/web && pnpm e2e:run tests/session/mobile-transient-turn-runtime-continuity.spec.ts --project mobile-chrome)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The configured desktop project is `chromium` in `apps/web/e2e/playwright.config.ts`.
Run desktop and mobile sequentially with the managed runner. Do not override its worker budget.
If `apps/node_modules` is absent, run `(cd apps && pnpm install --frozen-lockfile)` before pnpm commands.
Run the plan's documentation coverage preflight after all edits.

E2E must assert the side effect occurs once, the original request appears once, and all retained native/runtime identities stay unchanged.
Cover successful continuation, Cancel during backoff, exhaustion after five dispatches, reload, and a second viewer.
Keep mixed completed/pending work manual and verify that the composer remains available.
Inspect the rendered phone flow for wrapping, a 44px Cancel target, and no horizontal document overflow.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_transient.go` and retained-runtime dispatch files.
- `apps/backend/internal/orchestrator/provider_interruption_continuation.go` and `provider_interruption_dispatch.go`.
- `apps/backend/internal/orchestrator/event_handlers_turn_failure.go` and capacity/retained-runtime tests.
- `apps/backend/cmd/mock-agent/capacity_continuity.go` and its tests.
- `apps/web/e2e/helpers/transient-turn-runtime-continuity.ts` and helper tests if logic changes.
- `apps/web/e2e/tests/session/transient-turn-runtime-continuity.spec.ts` and `mobile-transient-turn-runtime-continuity.spec.ts`.
- `docs/public/tasks-and-workflows.md`.
- This package's status/results and the paired requirement/design lifecycle if needed.

## Dependencies

Task 01 and its positive native compatibility evidence.

## Risks

The existing continuation code checks the experimental toggle and can fall through to restoration.
The new capacity policy must change neither transport-loss support nor its conservative outcome handling.
Mock scenarios currently recognize the original capacity command. Continuation requires explicit episode state, not replay of that command.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/transient-turn-runtime-continuity.md), requirement `003`.
- [Design](../../specs/platform/system-design/transient-turn-runtime-continuity.md#capacity-recovery-admission).
- Task 01 evidence and native trace.
- `provider_interruption_retained_runtime_test.go`, `event_handlers_transient_retained_runtime_test.go`, and current desktop/phone continuity E2E helpers.

## Results

Implemented capacity-specific admission and dispatch through the existing retained-runtime retry owner. Safe completed-tool evidence continues on the same provider conversation; mixed, ambiguous, unsupported, or stale states refuse, and runtime loss cannot reach restore. The retry episode survives progress and retains the existing five-dispatch budget.

The mock ACP provider now exercises completed-tool continuation without replaying the side effect, mixed completed/pending refusal, cancellation, and exhaustion. Desktop and phone regressions verify runtime identity, reload and second-viewer continuity, actual attempt counts, usable Chat, touch-target size, and phone overflow.

Validation passed:

- Race-enabled orchestrator/lifecycle capacity and continuation regressions.
- Race-enabled mock-provider capacity and continuation regressions.
- `make -C apps/backend build`.
- Desktop `chromium` continuity E2E: 4 passed.
- Phone `mobile-chrome` continuity E2E: 4 passed.
- Public documentation tests and validator, specification catalog validation and lint, Prettier for changed E2E files, Go formatting check, documentation coverage preflight, and `git diff --check`.

Review follow-up closed three admission gaps. Capacity dispatch now bypasses lazy resume and restore, and checks the expected execution/native identity again at provider admission; transport-loss restoration keeps its prior path. ACP snapshots include unresolved session-owned background children, shells, and monitors across prompt boundaries, and authoritative completion clears that fence. Automation-owned turns cannot enter the after-effects capacity policy, while pre-result replay and transport-loss policies remain unchanged.

The PR review follow-up also rechecks queued work and automation eligibility immediately before provider admission, and counts an attempt only after the provider accepts it. Permission evidence is recorded after the bounded ToolCall notification wait. Completing a child removes its child fence, while a separately retained detached shell stays fenced until an authoritative process exit. Capacity continuations retain the initiating caller identity and reauthorize `session.prompt` at final provider admission; they refuse when scoped authorization is wired but the initiating identity is unavailable. Transport-loss restore admission remains unchanged. Public guidance now says continuation requests the agent to avoid repeating completed actions without promising exactly-once execution, and the ADR records the automation exclusion.

Follow-up validation passed:

- Race-enabled orchestrator, ACP adapter, and runtime lifecycle capacity/continuation regressions, including barrier-controlled runtime loss/readiness and final-identity changes.
- Race-enabled mock-agent capacity and continuation regressions.
- Race-enabled PostgreSQL cancellation barrier regression, repeated three times after bounding the wait for the canceled SQL connection to return to the pool.
- `make -C apps/backend build`.
- Go formatting check and `git diff --check`.
- Specification catalog validation and lint, plus public documentation tests and validation.
- Desktop `chromium` continuity E2E: 4 passed; phone `mobile-chrome` continuity E2E: 4 passed.
