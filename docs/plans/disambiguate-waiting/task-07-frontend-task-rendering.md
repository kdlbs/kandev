# Task 07: Frontend rendering — two task-icon resolvers and the board

Spec: §M, §"Rendering contract" 1-4. ACs: AC-23, AC-34, AC-58, AC-59, AC-82.

## Step 1 — promote `BackgroundWorkTaskIcon`

Move `BackgroundWorkTaskIcon` from `task-item.tsx:165-185` (private) to
`apps/web/lib/ui/state-icons.tsx`, exported, **unchanged** (same
`IconCircleDashed`, violet, `data-testid="task-state-background-running"`,
`task:backgroundWorkIsRunning` tooltip/label — no new i18n key, AC-82).
`task-item.tsx` imports it. Follow the `InterruptedTaskIcon` precedent
(`state-icons.tsx:183-203`) exactly.

## Step 2 — Resolver A (`task-item.tsx`, private `TaskStateIcon` ladder)

Add `parkedOnBackgroundWork?: boolean` (default `false`) to the props bag
(`:187-206` today), threaded from the call site (`:483`). New branch:

```
if (parkedOnBackgroundWork) return <BackgroundWorkTaskIcon />;
```

Position: **after** the `foregroundActivity === "background"` branch
(today's #4), **before** `shouldUseQuestionTaskIcon` (today's #5) and
everything below it. Both pending-input branches (permission, clarification)
still outrank parked (AC-34).

## Step 3 — Resolver B (`getTaskStateIconConfig`, `state-icons.tsx`)

`TaskStateIconOptions` (`:246-252`, currently a private type — keep it
private unless a call site outside this file needs it) gains
`parkedOnBackgroundWork?: boolean` (default `false`). New branch between
`foregroundActivity === "background"` (`:271`) and `isWaitingForInputState`
(`:272`), returning a new sentinel `IconConfig`; `getTaskStateIcon` (`:283-295`)
special-cases that sentinel to render `<BackgroundWorkTaskIcon />`, mirroring
the `TASK_INTERRUPTED_ICON` special case at `:291-293` exactly. `false`
default means every call site that doesn't pass the option is unaffected
(AC-59).

## Step 4 — board (`kanban-card-content.tsx`)

Both early returns in `renderTaskStatusIcon` (`:263-291`) must exclude a
parked task, or AC-58 fails:

- `:275` — `if (!showRunningSpinner && !needsMe && !hasActivity &&
  !showInterrupted) return null;` — add `&& !parkedOnBackgroundWork` (this is
  the load-bearing one; a settled parked task satisfies every existing
  clause today and the board renders nothing).
- `:282` — spinner short-circuit — exclude parked the same way it already
  excludes `foregroundActivity === "background"`.
- `:285` — pass `parkedOnBackgroundWork` into the `getTaskStateIcon` options
  bag (resolver B).

## Tests

- `apps/web/components/task/task-item.test.tsx` — extend the existing
  `"TaskItem background-running indicator"` / `"TaskItem background-running
  lifecycle"` describes with the parked case; AC-23 (background id present,
  neither `waiting-for-input` nor `turn-finished` present) and AC-34
  (permission/clarification outrank parked).
- `apps/web/lib/ui/state-icons.test.tsx` — extend `"getTaskStateIcon"` /
  `"getTaskStateIcon — task-level activity tri-state"` describes with the new
  option; AC-59's first half (six named call sites, `false` default, no
  regression) runs against this file's existing matrix, extended, not a new
  one.
- Board test (locate or add `kanban-card-content.test.tsx`): AC-58, asserting
  against **both** early returns explicitly (a test that only changes `:282`
  and never exercises `:275`'s all-false case would pass an incomplete fix).
- AC-82: pseudo-locale smoke — no new translation key introduced (grep-level
  check is sufficient: no new key added to any locale JSON for this feature).
