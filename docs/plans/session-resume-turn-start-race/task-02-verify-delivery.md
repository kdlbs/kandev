---
id: "02-verify-delivery"
title: "Verify resumed message delivery"
status: done
wave: 2
depends_on:
  - "01-preserve-startup"
plan: "plan.md"
requirements:
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
acceptance_criteria:
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.3
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.4
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.5
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.8
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.9
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.10
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.11
system_design:
  - ../../specs/tasks/system-design/resume-prompt-queue.md
---

# Task 02: Verify resumed message delivery

## Summary

Prove direct and queued user-message delivery during an actual delayed resume.
Use desktop and mobile browser tests with the existing mock ACP provider.
Retain the backend credential-boundary test as the exact incident regression.

## In scope

- Desktop direct `message.add` during `STARTING` with an `on_turn_start` transition.
- Mobile startup queue admission, touch submission, and workflow-triggered drain.
- Real resume, current task/session identity, one transition, and one delivered response.
- Existing paused-queue behavior, retained conversation, and isolated fixture cleanup.
- Final package traceability, results, and specification lifecycle reconciliation.

## Out of scope

- Production UI changes, new copy, new test delay settings, and provider adapter changes.
- Live user data, external provider credentials, or a production runtime restart.
- A claim that mock-agent load delay reproduces credential issuance.

## Acceptance

1. Desktop direct submission preserves `STARTING` until actual readiness.
   The resulting workflow transition and response each occur once without a new launch or prompt error.
2. Mobile Send admits input during startup and preserves queue policy.
   A real tap produces one response after readiness, with a reachable target and no horizontal overflow.
3. Focused old and new E2E cases pass with retries disabled.
   Every work-order command has recorded results, and all added criteria have executable evidence.

## Fixture and scenario design

Use `ApiClient.addUserMessage` to send the desktop direct action.
This request models a browser's direct-mode decision before the startup event
arrives. Do not bypass the real message handler or modify browser state.

Use `E2E_MOCK_AGENT_RESUME_DELAY` through the existing delayed-resume profile.
Seed a dedicated workflow whose current step has `on_turn_start: move_to_next`.
The destination keeps the same recipient profile and has no competing entry prompt.
Restore a saved conversation after restart and submit while the backend row is `STARTING`.

Capture state events for the exact session. Assert no workflow-induced waiting,
Review, or failure projection before genuine readiness. Poll backend and browser
state together. After delivery, correlate the prompt, turn, transition, and
response rather than accepting an HTTP success or a readiness banner as proof.

For mobile, use the existing task-chat composer and `.tap()`.
Submit with Auto-run ON for automatic delivery. Include an OFF case that remains
pending until explicit enablement. Assert one transition after queue drain,
one response, the same eligible conversation, and retained queued input on reload.

Use bounded semantic polling and zero retries. Clean up only this fixture's
task, profile, and workflow. Reuse helper teardown order and worker isolation.
The existing mock delay begins after the credential snapshot. Task 01 supplies
the missing earlier-boundary evidence, so no new production fixture hook is needed.

## Verification

If this worktree lacks dependencies, run this command once from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run these commands sequentially from the repository root:

```bash
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/session/session-resume-turn-start.spec.ts e2e/tests/session/session-resume-prompt-queue.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-session-resume-turn-start.spec.ts e2e/tests/session/mobile-session-resume-prompt-queue.spec.ts -- --retries=0)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed runner rebuilds artifacts and enforces resource limits.
Confirm project discovery for every named file.
Run the new desktop regression against an isolated pre-fix checkout to record Red.
Then run the final command on the corrected revision.
Keep the shared workspace on its current branch.
Keep the exact Go Red evidence from Task 01.
Run the PR-documentation coverage preflight from the plan's verification notes.
If implementation changes planned file ownership, reconcile that preflight input.

## Files likely touched

- `apps/web/e2e/tests/session/session-resume-turn-start.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-session-resume-turn-start.spec.ts` (new)
- `apps/web/e2e/helpers/session-resume-turn-start.ts` (new)
- `docs/plans/session-resume-turn-start-race/plan.md`
- This work order and Task 01, for final results.
- `docs/specs/tasks/requirements/resume-prompt-queue.md` and its paired design, for lifecycle status after conformance.

Use `session-resume-prompt-queue.ts` as fixture input. Avoid changing the
completed original scenario matrix unless a shared fixture change requires it.

## Dependencies

[Task 01](task-01-preserve-startup.md) must pass first.

## Risks

- A browser response delay can leave the backend ready and conceal the regression.
- Readiness helpers that only exclude `STARTING` can mistake `FAILED` for success.
- An automatic destination prompt can create a second turn and invalidate the fixture.
- A passing count with retries can conceal the timing defect.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/resume-prompt-queue.md).
- [Design](../../specs/tasks/system-design/resume-prompt-queue.md), Regression evidence and Responsive behavior.
- [Plan](plan.md), E2E tests.
- `apps/web/AGENTS.md`, `/e2e`, and `/mobile-parity`.
- Existing `session-resume-prompt-queue.spec.ts` and `mobile-session-resume-prompt-queue.spec.ts`.
- `e2e/helpers/api-client.ts`: `addUserMessage` and queue identity helpers.
- `e2e/helpers/session-resume-prompt-queue.ts` and `cmd/mock-agent/AGENTS.md`.

## Results

The isolated pre-fix desktop regression at `c8273683132` reproduced the
failure: the test expected `STARTING` while both the API and browser state
settled at `WAITING_FOR_INPUT`.

The exact isolated command was:

```bash
MAKEFLAGS='GOFLAGS=-buildvcs=false' pnpm e2e:run --host --project chromium e2e/tests/session/session-resume-turn-start.spec.ts -- --retries=0
```

It reported one failure after 40.3 seconds at `waitForSessionStarting`, with
the API and browser both in `WAITING_FOR_INPUT` instead of `STARTING`.

The corrected revision passed the focused managed browser checks with retries
disabled:

- `(cd apps/web && pnpm e2e:run --project chromium e2e/tests/session/session-resume-turn-start.spec.ts e2e/tests/session/session-resume-prompt-queue.spec.ts -- --retries=0)`: 4 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-session-resume-turn-start.spec.ts e2e/tests/session/mobile-session-resume-prompt-queue.spec.ts -- --retries=0)`: 3 passed.
- `(cd apps/web && pnpm run typecheck)`: passed.

After the final E2E helper reused the existing workflow-history API method, the
new regressions passed again through the managed runner with `--no-build`:

- `pnpm e2e:run --no-build --project chromium e2e/tests/session/session-resume-turn-start.spec.ts -- --retries=0`: 1 passed.
- `pnpm e2e:run --no-build --project mobile-chrome e2e/tests/session/mobile-session-resume-turn-start.spec.ts -- --retries=0`: 2 passed.

Review follow-up on 2026-10-04 reran the delivery suites against the corrected
backend:

- `(cd apps/web && pnpm e2e:run --project chromium e2e/tests/session/session-resume-turn-start.spec.ts e2e/tests/session/session-resume-prompt-queue.spec.ts -- --retries=0)`: 4 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-session-resume-turn-start.spec.ts e2e/tests/session/mobile-session-resume-prompt-queue.spec.ts -- --retries=0)`: 3 passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `make -C apps/backend build`: passed.

Final documentation validation and coverage results are recorded in the plan.
