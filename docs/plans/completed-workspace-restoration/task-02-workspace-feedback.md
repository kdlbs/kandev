---
id: "02-workspace-feedback"
title: "Scope workspace failure feedback"
status: pending
wave: 2
depends_on:
  - 01-workspace-admission
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-002
  - REQ-TASKS-COMPLETION-003
acceptance_criteria:
  - AC-TASKS-COMPLETION-002.1
  - AC-TASKS-COMPLETION-002.3
  - AC-TASKS-COMPLETION-002.9
  - AC-TASKS-COMPLETION-003.8
  - AC-TASKS-COMPLETION-003.9
  - AC-TASKS-COMPLETION-003.10
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 02: Scope workspace failure feedback

## Summary

Represent workspace restoration independently from agent recovery. Show a
compact actionable failure in affected workspace panels, keeping Chat usable.

## In scope

- Preserve operation identity at request creation; route workspace outcomes
  through one shared environment-scoped attempt state with stale-result guards.
- Make Files, Changes, and workspace terminals consume the same pending/error
  state. Replace indefinite preparation after failure; retain cached content
  with a stale/unavailable notice when applicable.
- Exclude only workspace failures from top-level session feedback. Keep actual
  ensure/start/resume errors and completed-session Resume/New Agent intact.
- Extend `WorkspaceUnavailable` with restore wording, valid Retry, and collapsed
  sanitized Details. Retry calls restore only, disables while pending, and
  clears only the matching failure after success. No per-panel polling loops.
- Use the existing mobile Files navigation and file viewer. Keep errors inline,
  phone/coarse-pointer hitboxes at least 44 px, dense desktop controls, wrapping
  details, one panel scroll owner, and safe-area navigation.
- Localize new copy in all required catalogs and generate Traditional Chinese
  and pseudo through repository scripts. Keep technical detail out of titles.

## Out of scope

Global redesign of launch errors, new mobile navigation, persisted error history,
new runtime flags, or changes to explicit Resume policy.

## Acceptance

- Workspace failures settle loading and appear inside visible workspace content,
  never as a global session-start banner. Chat remains usable.
- Retry succeeds through workspace restoration without resuming the agent;
  duplicate actions and stale cross-environment/attempt results are rejected.
- Desktop and mobile share state/actions; phone geometry and disclosure behavior
  meet the design. Genuine agent recovery feedback is still visible.

## TDD entry

Add `use-workspace-restoration.test.ts` for failed-before-readiness, retry,
success clearing, task switches, and late ready/error results. In
`file-browser-load-state.test.tsx`, prove a restore failure replaces waiting.
Extend `ensure-session-error.test.tsx` or the owning page test to prove a restore
failure is excluded while a real session-start failure remains visible.
Do not use a missing new selector as the only failing assertion.

## Verification

Run from the repository root. Install once before the first package command
if this worktree has not been bootstrapped.

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps/web && rtk pnpm test hooks/domains/session/use-workspace-restoration.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-recovery-actions.test.ts components/task/file-browser-load-state.test.tsx components/task/ensure-session-error.test.tsx components/task/chat/session-stopped-banner.test.tsx)
(cd apps/web && rtk pnpm test lib/state/slices/session-runtime/workspace-restoration.test.ts lib/state/slices/session-runtime/migrate-env-keyed-data.test.ts lib/state/slices/session-runtime/purge-session.test.ts components/task/workspace-unavailable.test.tsx components/task/task-changes-panel.test.ts components/task/task-changes-panel-layers.test.ts)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
rtk git diff --check
```

If implementation extracts another state/helper test suite, add its exact path
to this command before completion. Run rendered desktop/mobile checks in Task 03;
unit tests alone do not complete the package's visual verification.

## Files likely touched

- `apps/web/hooks/domains/session/use-workspace-restoration.ts` and `.test.ts` (new).
- `apps/web/hooks/domains/session/use-session-resumption.ts` and `use-session-resumption-operations.ts`.
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts` and corresponding tests.
- `apps/web/lib/services/session-launch-service.ts` if operation/error typing needs extension.
- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts` and `types.ts`.
- `apps/web/lib/state/slices/session-runtime/workspace-restoration.ts` and `.test.ts` (new pure state helpers).
- `apps/web/lib/state/slices/session-runtime/migrate-env-keyed-data.test.ts` and `purge-session.test.ts`.
- `apps/web/components/task/task-page-inner.tsx`, `ensure-session-error.tsx` and tests.
- `apps/web/components/task/workspace-unavailable.tsx` and `workspace-unavailable.test.tsx` (new).
- `apps/web/components/task/file-browser-load-state.tsx` and `.test.tsx`.
- `apps/web/components/task/task-files-panel.tsx`, `task-changes-panel.tsx`, and workspace terminal content boundaries.
- `apps/web/components/task/mobile/session-mobile-layout.tsx`, `mobile-changes-panel.tsx`, and `mobile-terminal-pane.tsx` where required for shared feedback.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`.

## Dependencies

Task 01. Read scoped web guidance and `/mobile-parity` before implementation.

## Risks

Do not route errors by English substring, clear another attempt, hide all errors
on completed tasks, or mistake stale cached content for live readiness. Existing
workspace terminal permissions are unchanged; do not label access read-only.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), IDs above.
- [Design](../../specs/tasks/system-design/task-completion.md), Workspace failure feedback.
- `WorkspaceUnavailable` and the curated `task-layout.tsx` mobile composition.
- Existing recovery hooks, Files loading tests, and `/tdd`.

## Results

Pending.
