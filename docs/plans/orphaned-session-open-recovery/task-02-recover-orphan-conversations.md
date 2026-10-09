---
id: "02-recover-orphan-conversations"
title: "Recover orphan conversations on task open"
status: done
wave: 2
depends_on:
  - "01-preserve-idle-conversations"
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-STALL-VISIBILITY-001
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.8
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.9
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.10
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-002.2
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-002.3
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-002.8
system_design:
  - ../../specs/tasks/system-design/session-stall-visibility.md
  - ../../specs/tasks/system-design/restart-orphaned-session-terminalization.md
---

# Task 02: Recover orphan conversations on task open

## Summary

Treat exact system orphan cancellation as eligible for normal automatic recovery.
Keep the selected conversation, existing launch guards, and failure fallback.

## In scope

- Add `TestGetTaskSessionStatus_OrphanCancelledSessionAutoResumes` before implementation.
  Seed `CANCELLED`, the exact orphan reason, and a resumable executor row.
  Expect `is_resumable` and `needs_resume` true. Current code returns false.
- Cover no token, missing runtime row, non-resumable runtime, missing profile,
  existing live execution, archived task, deferred owner, and explicit user stop.
  Use existing archive recovery tests as the pattern for same-session initialization.
- Correct the `SessionOrphanedCancelReason` comment that currently equates it with an explicit stop.
- Verify recovery launches with automatic admission, sends no prompt, and preserves
  workflow ownership. Exercise repeated status/open calls and capacity refusal.
- Extend frontend hook tests for successful recovery and failure fallback.
- Add desktop and phone scenarios described in the plan using real backend eligibility.
  Cover restart during an active turn, focus recovery, and exactly-once delivery
  of a new message submitted during startup. Keep another interrupted task
  unopened and verify it does not launch. Cover legacy cancelled rows separately.
- Update the relevant recovery section in `docs/public/tasks-and-workflows.md` at implementation,
  or the current owning page found by `/docs-maintainer` if that path changes.
  Describe recovery, auto-start prevention, and manual fallback as a how-to section.

## Out of scope

New recovery UI, unconditional recovery of terminal states, prompt replay,
replacement sessions, or live database mutations.

## Acceptance

1. Eligible orphan cancellations recover automatically in the same conversation, including pre-existing rows.
2. Launch guards, user stops, and real failures retain their existing behavior without automatic retry loops.
3. Desktop and phone tests prove warning removal, retained history/workspace, no prompt replay, and successful follow-up messaging.

## ASCII UI preview

UI-01 uses the [full plan preview](plan.md#ascii-ui-preview), covering AC .8-.10.

```text
Desktop: [normal startup progress] -> [transcript + composer]
Phone:   [task header / session picker]
         [startup -> transcript      ]
         [composer + bottom navigation]
Failure: existing recovery error and explicit retry controls
```

Reuse the existing mobile layout and chat scroll owner. Do not add a dialog.
Retain safe-area clearance and touch-accessible retry and send controls.

## Verification

From the repository root, install dependencies once in this worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/orchestrator -run 'TestGetTaskSessionStatus_|TestAutoResumeEligibility|Test.*Resume.*|Test.*Archive.*Resume' -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.archive.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-resume-recovery.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run desktop and mobile sequentially. The managed runner builds current source.
Record RED and GREEN results, actual discovered test counts, and screenshots.
If frontend source changes, run its typecheck and targeted ESLint. If new copy
is necessary, add five-language translations and run `pnpm run i18n:check`.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_resume_test.go`
- `apps/backend/internal/task/models/resume_safety.go`
- `apps/web/hooks/domains/session/use-session-resumption*.ts`
- `apps/web/e2e/tests/session/session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`
- `apps/web/e2e/helpers/session-resume-recovery.ts`
- The existing public task recovery documentation.

## Dependencies

Task 01 establishes recoverable interruption state and prevents idle cancellation.

## Risks

Tokenless initialization cannot restore provider-native history. Retain stored
Kandev history and use the existing fallback contract. Broad reason matching
can silently restart deliberately stopped sessions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/session-stall-visibility.md)
- [Design](../../specs/tasks/system-design/session-stall-visibility.md)
- `task_operations_resume_test.go` archive-cancellation and explicit-stop cases.
- `e2e/tests/task/archived-session-recovery.spec.ts` recovery fixture pattern.

## Results

Implemented exact legacy orphan-cancellation eligibility in task-open status and
resume handling. Rows with the recorded orphan reason use the existing
same-session recovery path when their runtime is resumable; missing, tokenless,
or non-resumable runtime data uses the prompt-free fresh-start fallback. Archive,
explicit-stop, deferred-owner, live-execution, missing-profile, and capacity
guards remain authoritative. Repeated status/open evaluation is safe and does
not replay the previous prompt.

Added backend coverage for the eligibility matrix and resume guards. Existing
desktop and mobile recovery suites cover delayed startup, queued messages,
failure fallback, and same-session follow-up behavior. Public task and
configuration documentation now describes lazy focus recovery and the updated
stall behavior.

The integrated review coverage pairs the active-sweep missing-row regression
with an orchestrator regression that queries the resulting recovery state
through the normal status contract and focuses the selected conversation
without replaying its previous prompt. The existing archive, explicit-stop,
ownership, and capacity guards remain in the same path.
