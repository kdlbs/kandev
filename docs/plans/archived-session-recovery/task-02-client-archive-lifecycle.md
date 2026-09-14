---
id: "02-client-archive-lifecycle"
title: "Reconcile recovery across archive transitions"
status: done
wave: 2
depends_on:
  - "01-backend-archive-gate"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.7
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 02: Reconcile recovery across archive transitions

## Summary

Prevent archived task views from initiating recovery. Recheck an existing
session after unarchive, without navigation, while preserving user preferences.

## In scope

- Add `use-session-resumption.archive.test.ts` with deferred promises. Prove
  archived/unknown task state starts no recovery; unarchive checks once;
  repeated renders do not duplicate it; failed unarchive keeps it disabled;
  preference-enabled unarchive does not start an agent.
- Cover archive after status request, after resume begins, before fallback,
  and before status refresh. Include archive/unarchive/archive and a return to
  the same task/session. Assert both ignored setters and absent downstream
  operations. Keep existing navigation-generation tests.
- Pass resolved owning-task archive state to `useSessionResumption` from every
  caller: task detail, preview, and Quick Chat. Unknown hydration defers work.
  Extend commit-phase generations and reset the one-attempt marker for a new
  active generation. Do not infer archive state from a different active task.
- Consume the typed archive conflict from Task 01, stop fallback, and refresh
  task data through the existing caller/data layer. Manual Retry cannot launch
  while archived. Clear stale feedback on archive.
- Wire successful Unarchive callbacks and task update events to the same state.
  Suppress `SessionRecoveryFeedback` while archived, preserving history and the
  existing Unarchive control. Update preview/Quick Chat test call signatures.
- Add desktop/mobile archived-session E2E files named in the plan. Seed a
  disposable task with a real mock-agent session and recoverable worktree, wait
  for settled state, archive through the API, then open its history. Capture
  task-scoped WS launch frames and assert zero while archived.
- Activate the existing Unarchive control. Verify the same session ID, preserved
  transcript, recovered workspace, and settled agent state without reload.
  Cover the auto-start preference and restore its test setting afterward.

## Out of scope

- Compact failure disclosure (Task 03), new mobile navigation, live-instance
  testing, persisted metadata repair, or changing message-driven resume policy.

## Acceptance

1. Deferred-promise regressions fail before the client correction, then prove
   stale operations cannot launch fallback or contaminate a new generation.
2. Desktop and phone history stays readable while archived; Unarchive recovers
   the same eligible session in place or leaves it stopped under user policy.
3. Existing task navigation, preview, Quick Chat, and unarchive tests pass.

## Verification

Install once from `apps/` if this worktree has not been bootstrapped:

```bash
rtk pnpm install --frozen-lockfile
```

Run from `apps/web`:

```bash
rtk pnpm test hooks/domains/session/use-session-resumption.archive.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts components/task/preview-session-tabs.test.tsx components/quick-chat/quick-chat-session-view.test.tsx
rtk pnpm run typecheck
rtk pnpm e2e:run --project chromium tests/task/archived-session-recovery.spec.ts tests/task/unarchive-detail-topbar.spec.ts tests/task/unarchive-storage-recovery.spec.ts -- --retries=0
rtk pnpm e2e:run --project mobile-chrome tests/task/mobile-archived-session-recovery.spec.ts -- --retries=0
```

Run the new regression first for RED, then the listed checks after the changes.
Use the managed runner to rebuild; run desktop and mobile sequentially. Record
test discovery counts. Do not add retries or increase timeouts to hide failures.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-resumption.ts`
- `apps/web/hooks/domains/session/use-session-resumption-request-guard.ts`
- `apps/web/hooks/domains/session/use-session-resumption.archive.test.ts` (new)
- `apps/web/hooks/domains/session/use-session-resumption.test.ts`
- `apps/web/hooks/domains/session/use-session-resumption.navigation.test.ts`
- `apps/web/components/task/task-page-content.tsx`
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/preview-session-tabs.tsx` and its tests
- `apps/web/components/quick-chat/quick-chat-session-view.tsx` and its tests
- `apps/web/components/task/task-unarchive-button.tsx` and its tests, if needed
- `apps/web/e2e/tests/task/archived-session-recovery.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-archived-session-recovery.spec.ts` (new)
- `apps/web/e2e/helpers/archived-session-recovery.ts` (new shared fixture helper)

## Dependencies

Task 01 supplies authoritative eligibility and the typed archive conflict.

## Risks

- The resumption hook is outside the archive context provider on the task page.
- Unknown task hydration must not become permanently disabled recovery.
- Ignore late responses from before unarchive even if task/session IDs match.
- Mobile Unarchive must be reachable through existing task chrome. If its hit
  area needs correction, scope 44px sizing to coarse pointers.

## Parallelism

`sequential`

## Inputs

- Requirements `002.7` and `004.1`, `.3` through `.5` in the linked plan.
- Design sections: Archive transition eligibility; Responsive and accessible behavior.
- Existing `use-session-resumption.navigation.test.ts`,
  `task-unarchive-button.tsx`, and `unarchive-detail-topbar.spec.ts`.
- `task-layout.tsx` and the mobile task header are the shipped layout exemplars.
- Existing E2E session/worktree helpers and `fixtures/test-base.ts`.

## Results

RED evidence:

- `rtk pnpm test hooks/domains/session/use-session-resumption.archive.test.ts`: 6 of 8 archive lifecycle cases failed before the client correction. Unknown and archived states did not defer recovery, and stale operations could continue after archive changes.
- The first managed mobile run exposed that the mobile task layout had no Unarchive action. Both mobile cases could not complete the unarchive transition.

GREEN evidence:

- `rtk pnpm test hooks/domains/session/use-session-resumption.archive.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts components/task/preview-session-tabs.test.tsx components/quick-chat/quick-chat-session-view.test.tsx`: 5 files, 71 passed.
- `rtk pnpm run typecheck`: passed.
- `rtk pnpm run lint`: passed with zero warnings after the final test cleanup.
- `rtk pnpm e2e:run --project=chromium tests/task/archived-session-recovery.spec.ts`: 3 passed. This covers archived read-only history, in-place same-session recovery, the prevent-auto-start preference, and the automatic recovery failure path.
- `rtk pnpm e2e:run --project=mobile-chrome tests/task/mobile-archived-session-recovery.spec.ts -- --retries=0`: 3 passed. This covers the touch-sized mobile Unarchive action, same-session recovery, the prevent-auto-start preference, touch disclosure, retry, and overflow checks.

- `rtk go test -race ./internal/orchestrator/executor ./internal/task/service -run 'TestStopSessionSynchronously|TestCancelActiveRuns_UsesSynchronousStopWhenAvailable' -count=1`: 4 passed. The archive lifecycle now waits for runtime teardown and records exact-execution ownership before the wait.

Scope notes:

- The hook now defers unknown and archived states, invalidates all downstream work across archive generations, consumes typed archive conflicts, and refreshes through the existing task data layer.
- The existing `TaskUnarchiveButton` is also rendered in the mobile top bar with a 44px hit area. No new navigation or drawer was added.
