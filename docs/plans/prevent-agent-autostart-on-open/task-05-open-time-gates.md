---
id: "05-open-time-gates"
title: "Open-time gates in ensure and resume hooks"
status: done
wave: 1
parallelism: sequential
depends_on: ["03-frontend-settings-plumbing"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 05: Open-time gates in ensure and resume hooks

## Acceptance

- `useEnsureTaskSession` accepts `{ id, workflowStepId, workflowId }` and reads
  `state.userSettings.preventAutoStartAgentOnOpen`. When the setting is on and
  the task's step is the terminal step of the task's OWN workflow, it calls
  `ensureTaskSession(taskId, { autoStart: false })`; all other cases call
  `ensureTaskSession(taskId)` exactly as today.
- The terminal-step predicate is deterministic even with equal positions:
  `isFinalWorkflowStep(workflowStepId, steps)` returns true only when the
  step's `(position, id)` is the maximum of all steps under that ordering
  (max `position`, ties broken by max `id`). Workflow-step positions are
  caller-supplied and not uniqueness-validated
  (`internal/workflow/controller/controller.go` `CreateStepRequest`,
  `internal/workflow/service/service.go:335-344`), so a tie is representable
  and MUST NOT make both steps terminal. Add an equal-position unit test.
- The step list is resolved workflow-aware: the active workflow's steps
  (`state.kanban.steps` when `task.workflowId` matches
  `state.kanban.workflowId`) or the multi-workflow snapshot's steps
  (`state.kanbanMulti.snapshots[task.workflowId]?.steps`). Missing workflow id
  or step list → treated as "not final" (no gate).
- Callers pass normalized input: `task-page-content.tsx` maps the HTTP task's
  snake_case fields (`workflow_step_id`, `workflow_id`); `kanban-with-preview.tsx`
  `useSelectedTask` includes `workflowId` in its returned subset so
  cross-workflow preview tasks resolve their own steps.
- `useSessionResumption`'s automatic check no longer auto-resumes when the
  setting is on: with `status.needs_resume && status.is_resumable` it skips
  `resumeWithSilentFallback`, settles on `"idle"`, and records the skip in the
  store as `resumeSkippedSessionIds: Record<string, true>` on the kanban tasks
  slice via a new `setResumeSkipped(sessionId, boolean)` action. Do NOT use a
  native `Set`: the kanban slice is Immer-managed (`zustand/immer`, no
  `enableMapSet` configured) and SSR-hydrated, so a `Set` breaks mutation and
  serialization. The manual `resumeSession()` action and the
  `is_agent_running` / `needs_workspace_restore` branches are unchanged.
- Skip-flag semantics are monotonic and derived from live state, not timing:
  - RECORD the skip only when `status.needs_resume && status.is_resumable &&
    !status.is_agent_running` AND the live store state for the session is not
    STARTING or RUNNING. `checkAndResume` MUST re-read the live session state
    immediately before recording (not rely on the status response alone):
    `applyStatusToState` merges the status state into the store and can
    downgrade a newer STARTING/RUNNING state to the stale status state before
    the guard runs.
  - Status hydration uses timestamp precedence, not blanket protection:
    preserve a live STARTING/RUNNING session state when the incoming
    `task.session.status` timestamp is absent or NOT newer than the live
    session's `updated_at`; ACCEPT an incoming terminal status (FAILED,
    CANCELLED, COMPLETED) when its timestamp IS newer. Blanket protection
    would reject a legitimate newer terminal response and leave the UI stuck
    in STARTING/RUNNING with no recovery actions
    (`applyStatusToState` at `use-session-resumption.ts:244-251` merges via
    `setTaskSession`; `session-slice.ts:136-150` spreads incoming over
    existing). Add tests for both a stale terminal response (rejected) and a
    newer terminal response (accepted).
  - CLEAR the flag only on CONFIRMED RUNNING: the WS `session.state_changed`
    handler (`lib/ws/handlers/agent-session.ts:692-750`) deletes it on the
    RUNNING transition only (NOT on STARTING — a failed manual resume emits
    STARTING before the launch fails, and clearing there would drop the retry
    affordance). The manual paths (`resumeSession()` at
    `use-session-resumption.ts:491-522`, the Start button click at
    `message-renderer.tsx:55-64`) clear it only when the launch response
    reports `state === "RUNNING"` — a successful resume commonly returns
    `state: "STARTING"` (launch accepted, not agent running:
    `executor_resume.go:585` builds the execution as `SessionStateStarting`,
    `executionToLaunchResponse` at `session_launch.go:358-365` copies it), so
    success alone is NOT confirmation; the WS RUNNING transition (or a later
    status response reporting RUNNING) does the clearing. Rejections and
    `{ success: false }` keep the flag.
  - `setResumeSkipped(sessionId, true)` remains conditional in the slice
    action (refuses when the current state is STARTING/RUNNING) as a second
    line of defense.
- The Start agent button (`TaskDescriptionStartButton` in
  `message-renderer.tsx`) renders for `sessionState === "CREATED"` AND for
  resume-skipped (recovered-idle) sessions whose state is NOT FAILED; for the
  recovered-idle case it dispatches the resume request builder instead of
  `buildStartCreatedRequest`. FAILED sessions are excluded: the renderer
  returns early at `:119-134` for FAILED, and they keep their existing
  recovery actions (`recovery-resume-button` / `recovery-fresh-button`).

## Verification

```bash
(cd apps/web && pnpm run typecheck)
```

```bash
(cd apps/web && pnpm vitest run hooks/domains/session/use-ensure-task-session.test.ts hooks/domains/session/use-session-resumption.test.ts components/task/chat/message-renderer.test.tsx)
```

## Files Likely Touched

- `apps/web/hooks/domains/session/use-ensure-task-session.ts` (+ its test; add a `isFinalWorkflowStep(workflowStepId, steps)` helper, exported for tests)
- `apps/web/hooks/domains/session/use-session-resumption.ts` (+ its test; thread `preventAutoStart` into `checkAndResume` via `useSessionResetAndCheck`)
- `apps/web/lib/state/slices/kanban/types.ts` + `kanban-slice.ts` (the slice owning `tasks.activeSessionId`): add `resumeSkippedSessionIds: Record<string, true>` state + a `setResumeSkipped` action (property assignment / `delete`)
- `apps/web/components/task/chat/message-renderer.tsx` (+ test): Start button visibility for resume-skipped non-FAILED sessions, resume intent dispatch, and flag clearing on click
- `apps/web/lib/ws/handlers/agent-session.ts` (+ test): clear the skip flag ONLY on a `session.state_changed` transition to RUNNING; never clear it on STARTING
- `apps/web/hooks/domains/session/use-session-resumption.ts`: monotonic status application + re-read-before-record + clear-on-success (covered by its test)
- `apps/web/components/task/task-page-content.tsx`: pass `{ id, workflowStepId: task?.workflow_step_id, workflowId: task?.workflow_id }`
- `apps/web/components/kanban-with-preview.tsx` (+ test): `useSelectedTask` returns `workflowId`

## Dependencies

Task 03 (store field + `ensureTaskSession` `autoStart` opt), Task 04 (Start
button copy covers both cases).

## Inputs

- Spec scenarios 1, 2, 3, 5, 7, 8 and the "State machine" table.
- `useEnsureTaskSession` current flow: fires once per `(taskId, retryToken)` when
  the task has no sessions; the latch must keep working with the widened input.
- `checkAndResume` branch order at `use-session-resumption.ts:284-336`;
  `state.kanban.steps` position semantics (sorted, `position` ascending) and
  `state.kanbanMulti.snapshots` (keyed by workflow id, each with its own
  `steps`); `TaskDescriptionStartButton` condition at
  `message-renderer.tsx:135-149` (currently `sessionState === "CREATED"` only).

## Output Contract

Opening a task honors the setting on both kanban surfaces:
final-step + no-session → prepare-only ensure (CREATED session, Start agent
button rendered); post-restart idle session (non-FAILED) → no auto-resume,
skip recorded, Start agent button rendered with a resume action; resumable
FAILED sessions → no auto-resume, existing recovery actions retained. The skip
flag is confirmed-running-only: it is recorded only when the agent is
verifiably stopped (live state re-checked; status hydration preserves live
STARTING/RUNNING unless a NEWER terminal status arrives) and cleared only on
confirmed RUNNING (WS RUNNING transition or a launch response with
`state === "RUNNING"`); failed launches and `STARTING` responses keep the
retry button. Unit tests pin each branch, including the non-gated controls,
the snake→camel caller normalization, the cross-workflow preview resolution,
the equal-position tie-break, the delayed-status race (WS STARTING then stale
status), the stale-vs-newer terminal status precedence, and the failed-launch
retention.
