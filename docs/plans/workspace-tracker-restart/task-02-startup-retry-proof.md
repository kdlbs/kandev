---
id: "02-startup-retry-proof"
title: "Prove Changes recovery after startup retry"
status: completed
wave: 2
depends_on:
  - "01-tracker-lifecycle"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.19
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.20
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.29
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.43
system_design:
  - ../../specs/platform/system-design/workspace-tracker-restart.md
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 02: Prove Changes recovery after startup retry

## Summary

Prove that actual managed-runtime startup recovery restores live Changes data on desktop and phone.
Use the existing failure fixture and real filesystem, agentctl, backend, and browser delivery paths.

## In scope

- Add desktop and phone scenarios with a common small fixture helper.
- Reuse the silent-exit mode and retry-release barrier from `managed-runtime-recovery.ts`.
- Use a disposable worktree and capture task, session, environment, execution, and canonical repository identity.
- Observe real Git membership and patch publication through the retained workspace connection after recovery.
- Assert a successful foreground fresh response separately from stream delivery.
- Restore fixture state and record exact implementation validation in both work orders and the plan.

## Out of scope

- New production behavior beyond Task 01.
- Synthetic status responses, direct browser store injection, or a new product debug endpoint.
- Layout, navigation, copy, and localization changes.
- New Docker or SSH browser certification and full-suite E2E runs.

## Acceptance

1. Both browser projects cross a real first-attempt failure and second-attempt success on the same retained task/session/execution.
   The attempt counter is two and the accepted prompt is delivered once.
2. With Changes active, a tracked file mutation produces a real complete-membership event and ready patch in the rendered diff.
   No reload, focus cycle, manual retry, or additional mutation repairs the display.
3. Stream proof remains valid while competing foreground Git reads are held.
   Releasing those reads produces a successful correlated fresh result, and all fixture resources are cleaned on failure and success.

## Scenario and mobile assessment

Use `prepareManagedRuntimeProfile` with `startupMode: "silent-exit"`, `hostSubprocess: true`, and a unique retry-release file.
Use the existing worktree executor and repository seeded by the isolated E2E backend.
Wait for the barrier marker and second-attempt state before releasing the fixture.
Keep Changes mounted through that release and wait for startup completion.
Arm the existing workspace trace before launch or mutation.

Hold competing `session.git.*` read requests only during the stream proof window.
Keep real status notifications unmodified and capture the event that contains the mutated file and patch.
Use the event as the causal boundary for DOM assertions.
Then release reads and assert the existing correlated refresh response.
Record current execution identity after recovery and pair it with the fixture attempt evidence.

Desktop retains docked Changes and the existing diff viewer.
Phone uses bottom Changes navigation, taps the file, and dismisses the full-height diff through its existing control.
Use `mobile-workspace-stream-promotion.spec.ts` as the phone interaction exemplar.
There is no new rendered UI, so no ASCII preview or layout implementation is required.
Capture desktop and phone screenshots as evidence of the recovered content.

Use `finally` to release every barrier and held request.
Restore original tracked bytes and shared runtime selections, then remove disposable profiles, tasks, and fixture files.
Do not mutate a developer instance or leave the worker-scoped checkout dirty.

## Verification

Run from the repository root. Install dependencies once for this worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec eslint e2e/tests/session/managed-runtime-git-restart.spec.ts e2e/tests/session/mobile-managed-runtime-git-restart.spec.ts e2e/tests/session/managed-runtime-git-restart-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/session/managed-runtime-git-restart.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-managed-runtime-git-restart.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/workspace-tracker-restart
```

Run desktop and phone commands sequentially with fresh builds.
Confirm that each project discovers its new scenario before recording success.
Use the process regression as the initial RED gate.
Record a browser negative control against the pre-fix backend when the fixture permits a deterministic run.
If a fixture change becomes necessary, include its focused tests and lint command in this work order before implementation.

## Files likely touched

- `apps/web/e2e/tests/session/managed-runtime-git-restart.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-managed-runtime-git-restart.spec.ts` (new)
- `apps/web/e2e/tests/session/managed-runtime-git-restart-helpers.ts` (new)
- `apps/web/e2e/tests/session/workspace-stream-promotion-trace.ts`, for correlated released-read evidence
- `docs/plans/workspace-tracker-restart/plan.md`
- `docs/plans/workspace-tracker-restart/task-01-tracker-lifecycle.md`
- `docs/plans/workspace-tracker-restart/task-02-startup-retry-proof.md`
- `docs/specs/platform/system-design/workspace-tracker-restart.md`, for final lifecycle status after all checks pass

## Dependencies

[Task 01](task-01-tracker-lifecycle.md).

## Risks

- The transient retry can finish before browser navigation. The existing barrier makes the ordering deterministic.
- A later foreground refresh can conceal missing stream delivery.
- A fixture that creates a replacement execution does not prove retained-runtime restart.
- Shared runtime selection or checkout changes can leak into another E2E test without explicit restoration.

## Parallelism

`sequential`

## Inputs

- [Plan E2E matrix](plan.md#e2e-tests) and [tracker restart design](../../specs/platform/system-design/workspace-tracker-restart.md).
- `e2e/tests/session/managed-runtime-startup-retry.spec.ts` and its `mobile-` counterpart.
- `e2e/tests/session/workspace-stream-promotion-helpers.ts` and `workspace-stream-promotion-trace.ts`.
- `e2e/helpers/managed-runtime-recovery.ts`, `e2e/helpers/api-client.ts`, and `e2e/pages/session-page.ts`.
- `apps/web/AGENTS.md`, the E2E skill, the mobile-parity skill, and fixture cleanup guidance.

## Results

Completed.

- `managed-runtime-git-restart.spec.ts` and `mobile-managed-runtime-git-restart.spec.ts` cross the real silent-exit first attempt and release-barrier second attempt on the retained task, session, and execution.
- `managed-runtime-git-restart-helpers.ts` asserts attempt two, one accepted prompt, one agent response, unchanged execution identity, streamed file membership, settled detail, no foreground read response during the hold, the rendered diff, and a correlated fresh `session.git.refresh` after release.
- `workspace-stream-promotion-trace.ts` gained a release-and-await helper that records each held read's correlated frame and validates its action-specific success contract, so a `ready: false` response cannot pass.
- Desktop passed in 16.5s (test 13.4s); phone passed in 15.7s (test 13.0s). A final paired rerun after the screenshot evidence addition passed at 19.5s (desktop) and 16.0s (phone).
- A negative control with renewal disabled failed at `Waiting for the restart file membership workspace stream event`, and agentctl logged `context_canceled` Git failures.
- Focused ESLint and `pnpm run typecheck` passed.
- Screenshots and a JSON evidence attachment are captured for both projects.
