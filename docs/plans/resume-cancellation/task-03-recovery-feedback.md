---
id: "03-recovery-feedback"
title: "Present recoverable resume failures"
status: done
wave: 3
depends_on: ['02-startup-cancellation']
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.6
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 03: Present recoverable resume failures

## Summary

Project failed resume before dispatch through the existing recovery owner.
Prove cancellation and retry behavior in desktop and phone task chat.

## In scope

- Keep the correlated resume cause and suppress only its duplicate synthetic send error.
- Cover explicit cancellation separately from resume failure.
- Extend existing mock resume fixtures and desktop/phone recovery specs.
- Reuse the current recovery card and locale keys where possible.

## Out of scope

Provider internals, new queue policy, schema changes, and live task repair.

## Acceptance

- Load failure displays one recovery card with the cause and survives reload.
- Desktop and phone can cancel startup and retry with exactly one new response and no cancelled response.
- Retry remains disabled while pending, details wrap, and phone actions remain touch accessible.

## ASCII UI preview

### UI-01: Task chat recovery after a load timeout

Desktop:

```text
[Recovery failed]
[Retry] [existing secondary recovery actions]
[> Details]
```

Phone:

```text
[Recovery failed]
[Retry                               ]
[existing secondary recovery actions ]
[> Details                           ]
```

The illustration uses semantic labels, not final translated strings. The
existing recovery card is the required surface. Details identify the resume
load timeout. During Retry, equivalent actions remain disabled. During
cancellation, the existing cancellation progress remains visible until cleanup
settles. No new prompt starts from the cancelled attempt.

The transcript owns scrolling. Phone actions retain 44-pixel hit targets and
existing safe-area clearance. Details wrap without horizontal page overflow.
These structures cover AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.5 and .6.

See the combined [plan](plan.md#ascii-ui-preview).

## Verification

```bash
(cd apps/backend && go test -race ./internal/task/handlers ./internal/orchestrator -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/simple/components/task-launch-error-entry.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/session/session-recovery.spec.ts --grep 'cancelling delayed resume fences')
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-session-resume-recovery.spec.ts --grep 'cancel fences the delayed startup')
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-session-resume-recovery.spec.ts --grep 'failed saved-session load')
```

Run new regressions before production changes and record the expected failure.

## Files likely touched

- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_resume_readiness_test.go (existing regression file)`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/web/components/task/simple/components/task-launch-error-entry.tsx`
- `apps/web/components/task/simple/components/task-launch-error-entry.test.tsx`
- `apps/web/hooks/domains/session/use-session-recovery-feedback.ts`
- `apps/web/e2e/helpers/session-resume-recovery.ts`
- `apps/web/e2e/helpers/session-resume-prompt-queue.ts`
- `apps/web/e2e/tests/session/session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`
- `apps/web/src/locales/*/task.json (only if a cause label is required)`

## Dependencies

02-startup-cancellation.

## Risks

Preserve supported provider behavior and avoid lock inversion. Late cleanup
must not mutate the current attempt. Use deterministic barriers, not sleeps.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/session-recovery-failures.md).
- [System design](../../specs/agents/system-design/session-recovery-failures.md).
- [Plan evidence and regression map](plan.md).
- Existing resume tests, cancellation helpers, and contribution recovery card.

## Results

Implemented recovery ownership feedback in the message handler. Resume
failures remain correlated with the existing launch recovery projection, and
the generic synthetic send error is suppressed only when that recovery or a
cancelled attempt owns the outcome. The original resume or readiness error is
preserved when recovery fails before redispatch. Existing recovery UI behavior
remains available on desktop and phone, including wrapped details, reload
persistence, keyboard access, touch targets, and transcript scrolling.

The focused handler and launch-error component tests, localization checks, and
typecheck pass. Desktop delayed-resume cancel/retry, mobile delayed-resume
cancel/retry, and mobile failed saved-session load E2E tests pass. The desktop
suite retains its existing failed-resume regression using the same failing ACP
profile fixture.
