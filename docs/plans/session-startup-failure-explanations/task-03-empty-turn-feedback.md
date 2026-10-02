---
id: "03-empty-turn-feedback"
title: "Separate bootstrap failure from empty agent turns"
status: done
wave: 3
depends_on:
  - "02-visible-recovery-causes"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.30
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.31
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.29
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
---

# Task 03: Separate bootstrap failure from empty agent turns

## Summary

Suppress completed/no-output feedback for an actual correlated pre-readiness
failure. Keep real empty-turn and slash-command feedback, including a healthy
empty turn after recovery in a session with old startup history.

## In scope

- Read `/debug` backend triage and `/tdd`. Prove the completion/notice producer
  path with a failing isolated test before choosing the production change.
- Pass turn metadata into `computeEmptyTurnNotice` and exclude lifecycle-only
  entries. Reuse existing `TurnMetaKeyLifecycleOnly` and completed lifecycle turns.
- Inspect non-lifecycle turns closed by startup settlement. If correlation is
  absent, retain only bounded host failure/attempt/execution evidence in their
  existing metadata before completion publication. Do not infer from FAILED
  session state or suppress every turn in a session with a previous error.
- Handle both completion-before-failure and failure-before-completion. A late
  matching failure may remove only the exact synthetic notice for that turn.
- Preserve real `had_output=false` behavior, deterministic notice IDs, ephemeral
  surfaces, known/unknown commands, and ongoing subagent output guards.
- Extend real fixture flow with a completed empty mock turn as a positive control.
  Persist/reload checks distinguish lifecycle history from real turns.

## Out of scope

Deleting error history, hiding unrelated errors, changing prompt admission,
normal agent completion semantics, provider policy, success notice copy.

## Acceptance

1. A reproduced bootstrap-settlement event sequence never emits the misleading
   warning, including reversed delivery and reload.
2. A completed empty agent turn still emits its warning or command guidance,
   even after a different startup attempt in the same session failed.
3. Attempt/turn association prevents a stale failure from deleting a successor
   or sibling-session notice. Correlation omissions have an explicit safe fallback.

## ASCII UI preview

UI-01 from the [full preview](plan.md#ascii-ui-preview), criteria .30-.31:

```text
Pre-readiness failure, desktop and phone:
  Chat history: <time> Saved model unavailable > Technical details
  Composer region: [specific recovery cause and eligible actions]
  No completed-with-no-output warning for that failed attempt.

Real completed empty turn, desktop and phone:
  Chat history: The agent finished without producing any output.
  Composer region: [normal input]
```

The existing localized real-turn copy and normal chat scroll owner remain.
This work changes feedback logic and adds no new layout or touch control.
Focused desktop/mobile fixture checks verify the changed rendered outcome.

## Verification

Use Task 02's dependency installation if already done; do not reinstall it.
Run from repository root:

```bash
(cd apps/backend && go test -race ./internal/task/service ./internal/task/dto ./internal/orchestrator)
(cd apps/backend && golangci-lint run ./internal/task/service/... ./internal/task/dto/... ./internal/orchestrator/...)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/empty-turn-notice.test.ts lib/ws/handlers/turns.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/ws/handlers/empty-turn-notice.ts lib/ws/handlers/turns.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-resume-settings-recovery.spec.ts tests/session/session-error-recovery-ui.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-settings-recovery.spec.ts tests/session/mobile-session-error-recovery-ui.spec.ts)
git diff --check
```

Do not rerun full suites or overlap E2E projects. If turn metadata changes,
extend DTO serialization and replay tests in the affected package run.

## Files likely touched

- `apps/web/lib/ws/handlers/empty-turn-notice.ts`, `.test.ts`
- `apps/web/lib/ws/handlers/turns.ts`, `.test.ts`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/task/service/service_turns_test.go`
- `apps/backend/internal/task/service/service_turns_step_stamp_test.go`
- `apps/backend/internal/task/dto/converters_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- Existing orchestrator recovery history/completion tests
- `apps/web/e2e/helpers/session-resume-settings-recovery.ts`
- `apps/web/e2e/helpers/session-error-recovery-ui.ts`
- Their four desktop/mobile specs under `apps/web/e2e/tests/session/`
- Mock-agent scenario helper only if existing empty-output script is insufficient

## Dependencies

Task 01 evidence and Task 02's UI/fixture contract. Execute after Task 02 because
the same recovery helper is changed and its assertions must remain synchronized.

## Risks

The incident warning was not reproduced in the planning baseline. The work
order reproduced the event shape with the lifecycle producer contract and
covered both event orders; see the results below.

## Parallelism

`sequential`

## Inputs

- Design: Empty-turn feedback.
- Existing `computeEmptyTurnNotice`, `registerTurnsHandlers`, `CompleteTurn`,
  `createCompletedTurn`, and converter lifecycle-only tests.
- Task 01 safe bootstrap evidence and existing request/turn ownership.

## Results

- TDD red run: `pnpm exec vitest run lib/ws/handlers/empty-turn-notice.test.ts lib/ws/handlers/turns.test.ts` failed in four lifecycle/failure cases before the production change.
- TDD green run: the same focused Vitest command passed 2 files and 48 tests. Coverage includes lifecycle-only and error-terminated events, both failure/completion orders, healthy empty turns after unrelated startup history, and existing slash-command feedback.
- Backend producer contract: `go test ./internal/task/service -run 'TestCompleteTurn_(ErrorTerminatedTurnReportsHadOutput|PublishesLifecycleOnlyOwnershipMetadata)$' -v` passed both tests. The real producer persists `lifecycle_only`; failed-turn completion carries `error_terminated` and `had_output=true`.
- Race regression: `go test -race ./internal/task/service ./internal/task/dto ./internal/orchestrator` passed all three packages (service 81.943s, DTO 1.070s, orchestrator 140.289s).
- Backend lint: `golangci-lint run ./internal/task/service/... ./internal/task/dto/... ./internal/orchestrator/...` reported zero issues.
- Frontend checks: `pnpm run typecheck`; scoped ESLint with `--max-warnings 0` on the changed handler and recovery helper; and `pnpm run i18n:ratchet` passed. No new user-facing copy or catalog changes were needed.
- Desktop E2E: `pnpm e2e:run --project chromium tests/session/session-resume-settings-recovery.spec.ts tests/session/session-error-recovery-ui.spec.ts` passed 9/9.
- Mobile E2E: the guarded Docker run passed 8/9 but the second provider-restored request returned an error. The retained-artifact host rerun, `pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-session-resume-settings-recovery.spec.ts tests/session/mobile-session-error-recovery-ui.spec.ts`, passed 9/9; the settings-recovery test also passed alone. The E2E now asserts that the second request is sent and checks its correlated response.
- Root cause: the completion event carried `lifecycle_only` for synthetic lifecycle history and `error_terminated` for failed turns, but the browser helper forwarded only `had_output` to the empty-turn predicate. Passing event metadata through and excluding both markers prevents false notices without suppressing a real empty turn in a previously failed session.
- No SQL or turn-metadata migration was needed; existing persisted metadata survives reload and is emitted in the completion event.

Task 04 adds the confirmed recovery notice and ensures later success or failure
does not rewrite turn or failure history. See
[Task 04 results](task-04-recovery-success-history.md#results).
