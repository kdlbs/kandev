---
id: "01-task-disclosure"
title: "Task disclosure"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 01: Task disclosure

## Acceptance

- An explicit Kubernetes Refresh shows a distinct busy state, awaits the exact
  scoped environment/session read, and joins an existing poll request without a
  duplicate request or stale cross-session publish.
- Desktop hover/focus and the touch Drawer render the same structured Pod
  identity, live status, grouped facts, and safe actions. Only technical
  identity/raw values use monospace typography.
- Executor settings is the clear primary destination, Refresh remains
  secondary, and no Kubernetes Reset/restart/delete action is introduced.

## Files likely touched

- `apps/web/hooks/domains/session/use-task-environment.ts`
- `apps/web/hooks/domains/session/use-task-environment.test.tsx`
- `apps/web/components/task/executor-settings-button.tsx`
- `apps/web/components/task/executor-settings-button.test.tsx`
- `apps/web/components/task/executor-environment-disclosure.tsx`
- `apps/web/components/task/executor-environment-info.tsx`
- `apps/web/components/task/executor-environment-info.test.tsx`

## Dependencies

None.

## Parallelism

`sequential`. The hook state contract and both disclosure presentations must
land together so the button never consumes an intermediate loading shape.

## Inputs

- Spec `What`: structured Pod summary and visible/awaitable refresh.
- Spec scenarios: loaded refresh, refresh during polling, desktop disclosure,
  and coarse-pointer Drawer parity.
- Existing request-generation guards in
  `hooks/domains/session/use-task-environment.ts`.
- Existing touch surface in `components/task/executor-settings-button.tsx`.

## TDD sequence

1. Add deferred hook tests that prove the current Refresh neither reports busy
   nor awaits an in-flight poll, and record the expected RED failures.
2. Make scoped in-flight work awaitable and add a manual-only `refreshing`
   contract without changing background polling cadence.
3. Add semantic component assertions for the proposed Pod header, grouped
   facts, action priority, and touch parity; then implement the minimal visual
   composition.
4. Run the focused checks below and update this task plus `plan.md` with the
   exact results.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- hooks/domains/session/use-task-environment.test.tsx components/task/executor-environment-info.test.tsx components/task/executor-settings-button.test.tsx && pnpm --filter @kandev/web lint -- hooks/domains/session/use-task-environment.ts hooks/domains/session/use-task-environment.test.tsx components/task/executor-settings-button.tsx components/task/executor-settings-button.test.tsx components/task/executor-environment-disclosure.tsx components/task/executor-environment-info.tsx components/task/executor-environment-info.test.tsx && cd web && pnpm run typecheck && git diff --check
```

## Output contract

Report the root RED assertions, final request-count/busy-state behavior, files
changed, exact commands and counts, blockers, risks, and synchronized task/plan
status. External side effects: none.

## Results

- RED: the explicit-refresh test observed `refreshing` as `undefined`, and the
  duplicate-refresh test proved the second call settled before the in-flight
  request. The Pod glyph test also showed Kubernetes still mapped to
  `IconCube`; semantic disclosure tests found neither the structured summary
  nor 44 px actions.
- GREEN: per-scope requests are now awaitable and deduplicated. Explicit
  Refresh owns a distinct busy state, preserves last-known Pod details, and
  joins the exact environment/session request already in flight. The desktop
  HoverCard and touch Drawer share a structured Pod summary, package-style Pod
  glyph, grouped facts, safe copy target, secondary Refresh, and primary
  Executor settings action.
- `pnpm --filter @kandev/web test -- hooks/domains/session/use-task-environment.test.tsx components/task/executor-environment-info.test.tsx components/task/executor-settings-button.test.tsx lib/executor-icons.test.ts`: 4 files, 23 tests passed.
- Focused ESLint passed with zero warnings; `pnpm --filter @kandev/web typecheck`
  and `git diff --check` passed.
- Blockers: none. External side effects: none.
