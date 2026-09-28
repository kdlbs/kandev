---
id: "03-verify-queued-pause"
title: "Verify queued pause on desktop and phone"
status: done
wave: 3
depends_on:
  - "02-bound-cancellation-waits"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
  - REQ-UI-CANCEL-TURN-PROGRESS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.2
  - AC-UI-CANCEL-TURN-PROGRESS-001.5
  - AC-UI-CANCEL-TURN-PROGRESS-001.6
  - AC-UI-CANCEL-TURN-PROGRESS-001.7
system_design:
  - ../../specs/tasks/system-design/workflow-cancelled-turn-completion.md
  - ../../specs/ui/system-design/cancel-turn-progress.md
---

# Task 03: Verify queued pause on desktop and phone

## Summary

Prove the repaired queued-turn pause through the existing composer on desktop and phone.
Keep the real backend, queue, lifecycle, and WebSocket projection in the test path.
The deterministic backend tests remain the evidence for the exact deadlock interleaving.

## In scope

- Add desktop `queued-prompt-cancel.spec.ts` and phone `mobile-queued-prompt-cancel.spec.ts` under `e2e/tests/chat`.
- Reuse task seed helpers, the mock provider, and the existing cancel/recovery assertions.
- Verify queue parking, cancellation settlement, runtime identity, and one follow-up delivery.
- Run existing task-switch and mobile reload progress scenarios with the new cases.
- Record final package validation and implementation results.

## Out of scope

No production UI, layout, copy, navigation, locale, or public documentation changes.
Do not intercept cancellation responses or fake pending state in the browser.
Do not run tests against the user's live backend.

## Acceptance

1. Both viewports queue a prompt during streamed work, wait for a unique provider marker on that queued turn, and pause through the real control.
   Assert the captured turn closes, pending clears, the session becomes `WAITING_FOR_INPUT`, and remaining queued messages stay parked.
2. Submit one follow-up through the composer. Assert one delivery and response on the same healthy execution.
   Run with cancellation-driven workflow completion disabled to isolate pause; retain existing workflow cancellation suites as backend coverage.
3. The phone flow taps a reachable control with at least a 44px hit target and no horizontal document overflow.
   Existing desktop navigation and phone reload tests still retain backend-owned progress until settlement.

Use the shared compact composer as the mobile exemplar.
Desktop and phone composition stay unchanged; no ASCII preview is required.
Use a unique marker emitted by the cancel-hold provider fixture before it waits for cancellation.
Require the marker's persisted agent message to share the queued user message's turn ID and require
that turn to remain open before pausing. A user message and open turn can exist before provider dispatch.
Browser timing must not substitute for Task 01's controlled interleaving.

## Verification

Run from the repository root. Install dependencies once for this worktree before pnpm commands.
Run desktop and phone commands sequentially; let the managed runner build and clean up its isolated instances.
Use the owned temporary directory while the host `/tmp` remains full.

```bash
task_tmp="$(mktemp -d /root/kp.XXXXXX)"
export TMPDIR="$task_tmp" GOTMPDIR="$task_tmp"
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/queued-prompt-cancel.spec.ts tests/chat/cancel-progress-task-switch.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-queued-prompt-cancel.spec.ts tests/chat/mobile-cancel-progress-reload.spec.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

If mock behavior changes, also run:

```bash
(cd apps/backend && go test ./cmd/mock-agent/... -count=1 -timeout=120s)
```

Record actual discovered tests and results. Validate new test assertions before relying on them as evidence.
Use a temporary local reversion of the repair only if needed to prove RED; restore it before final checks.
Do not force a flaky browser race to reproduce the lock cycle.
Remove only the temporary directory created for this verification.

## Files likely touched

- `apps/web/e2e/tests/chat/queued-prompt-cancel.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-queued-prompt-cancel.spec.ts` (new)
- `apps/web/e2e/helpers/` only for shared existing fixture helpers
- `apps/backend/cmd/mock-agent/` only if a deterministic stream/queue sequence is missing
- This package's plan and work-order Results sections
- The paired task design, promoted to `current` after implementation matched it

## Dependencies

Task 02 and its successful backend checks.
Read `apps/web/AGENTS.md`, `/e2e`, and `/mobile-parity` before editing tests.

## Risks

An auto-resume can hide execution loss; compare runtime identities before pause and after follow-up.
A stale streamed message can falsely prove acceptance; correlate with the queued turn and new message IDs.
Mock timing may prove the user flow without exercising the lock cycle; the Go integration regression is mandatory.

## Parallelism

`sequential`

## Inputs

- [Plan E2E matrix](plan.md#e2e-tests)
- Existing `cancel-progress-task-switch.spec.ts` and `mobile-cancel-progress-reload.spec.ts`
- Existing `session-recovery.spec.ts` and `mobile-session-resume-recovery.spec.ts`
- [Task design](../../specs/tasks/system-design/workflow-cancelled-turn-completion.md)
- [UI progress design](../../specs/ui/system-design/cancel-turn-progress.md)

## Results

The desktop and phone flows use the real backend, queue, runtime, and WebSocket progress
projection. Each generates a prompt-correlated acceptance marker, waits for the provider's agent
message carrying that marker on the queued user turn, then pauses. The mock provider terminates the
marker line so lifecycle message buffering persists it before the fixture waits for cancellation.
Both flows confirm accepted-turn closure, parked work, same-execution follow-up, and one delivery.
The phone flow also verifies the pause target is at least 44px and checks horizontal overflow.
Auto-run and auto-merge are disabled while the fixture confirms both queued entries; after the
streaming turn settles, enabling auto-run must report that the queue head was dispatched.

Verification passed:

- Chromium: queued-prompt-cancel and cancel-progress-task-switch, 2 tests.
- Mobile Chrome: mobile-queued-prompt-cancel and mobile-cancel-progress-reload, 2 tests.
- The marker's first version remained buffered until prompt completion, so the provider-acceptance
  poll timed out. A terminating newline flushes it before the cancel hold; both viewports then pass.
- Web typecheck.
- Final catalog, specification linter tests, specification lint, delivery coverage preflight,
  local links, and diff checks are recorded in the plan results.

PR fixup verification (2026-09-28):

- The Chromium and mobile-chrome queued-pause flows passed again with the provider marker
  persisted on the open queued turn before Pause.
- The previously failing queue-reorder CI spec passed both cases with retries disabled after
  `typeWhileBusy` began waiting for the editor's `contenteditable=true` transition before click.
- Web typecheck and final documentation/coverage checks passed after updating the test helpers and
  package results. Catalog validation counted 321 decisions and 1223 specifications.

Current-base merge verification (2026-09-28):

- The conflicting editor-readiness hunk was resolved by retaining the `contenteditable=true`
  check with the PR's explanatory comment and 15-second wait.
- Managed Chromium queued-pause, task-switch, and queue-reorder specs passed all 4 tests. The
  command built the backend, web assets, and fixture plugin.
- Mobile-chrome queued-pause, cancellation reload, and queue-reorder specs passed all 3 tests using
  those freshly built artifacts.
- `pnpm run typecheck` and Prettier check for `type-while-busy.ts` passed.
