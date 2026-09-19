---
id: "01-shared-predicate-and-removal"
title: "Shared hint predicate and footer resume-button removal"
status: done
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 01: Shared hint predicate and footer resume-button removal

## Acceptance

- `shouldShowComposerAgentStartHint({ resumeSkipped, sessionState,
  hasRecoveryActions, sessionId })` is exported from
  `apps/web/lib/session-state.ts` and returns true only when ALL of:
  `resumeSkipped === true`, `hasRecoveryActions === false`, `sessionState` is
  not `FAILED` / `RUNNING` / `STARTING`, and `sessionId` is non-null. This is
  the exact condition set the footer button used.
- Unit tests in `apps/web/lib/session-state.test.ts` cover: visible for a
  resume-skipped `WAITING_FOR_INPUT` session; hidden when not resume-skipped;
  hidden for `FAILED`; hidden for `RUNNING` and `STARTING`; hidden when
  recovery actions are present; hidden when `sessionId` is null.
- `apps/web/components/task/chat/session-resume-start-button.tsx` is deleted
  and no file imports `session-resume-start-button` anymore.
- `apps/web/components/task/chat/message-list-footer.tsx` no longer renders
  `SessionResumeStartButton` and no longer reads
  `resumeSkippedSessionIds` / computes `hasRecoveryActions` /
  `showResumeStartButton` (those existed only for the button).
- `apps/web/components/task/chat/message-list-footer.test.tsx` has no
  `SessionResumeStartButton` mock and no resume-button describe blocks; the
  remaining footer tests (AgentStatus, failure presentation) still pass.
- `apps/web/components/task/chat/message-renderer.tsx`:
  `TaskDescriptionStartButton`'s `handleStart` drops the now-unreachable
  `resumeSkipped ? buildResumeRequest : buildStartCreatedRequest` branch and
  always dispatches `buildStartCreatedRequest`; `shouldShowDescriptionStartButton`
  stays CREATED-only. The test comment "the footer owns that surface" in
  `message-renderer.test.ts` is updated to name the composer hint.

## Verification

```bash
(cd apps/web && pnpm run typecheck)
```

```bash
(cd apps/web && pnpm vitest run lib/session-state.test.ts components/task/chat/message-list-footer.test.tsx components/task/chat/message-renderer.test.ts)
```

## Files Likely Touched

- `apps/web/lib/session-state.ts` (+ `session-state.test.ts`)
- `apps/web/components/task/chat/session-resume-start-button.tsx` (deleted)
- `apps/web/components/task/chat/message-list-footer.tsx` (+ test)
- `apps/web/components/task/chat/message-renderer.tsx` (+ test)

## Dependencies

None. The predicate is new and the removal is independent of the hint UI.

## Inputs

- Spec: "What" bullets 1 and 3, scenarios 1 and 5.
- Current footer predicate at `message-list-footer.tsx:62-88`
  (`resumeSkipped`, `hasRecoveryActions`, `showResumeStartButton`).
- Current `TaskDescriptionStartButton` branch at `message-renderer.tsx:90-95`
  and `shouldShowDescriptionStartButton` at `message-renderer.tsx:67-72`.

## Output Contract

No UI change yet (the hint renders in task 02): the resume-skipped session
simply has no Start agent button anywhere, and the shared predicate pins the
exact condition set the hint will reuse. The CREATED description button is
behaviorally unchanged.
