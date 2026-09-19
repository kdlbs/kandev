---
id: "05-changes-identity-disclosure"
title: "Show launch credential identity in the Changes branch disclosure"
status: done
wave: 3
depends_on: ["02-launch-resume-snapshot"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 05: Show Launch Credential Identity In The Changes Branch Disclosure

## Acceptance

- The active session's valid launch snapshot renders policy, effective method/source, known actor
  or runtime-selected truth, transport, and executor alongside the existing branch comparison.
- Missing/malformed/legacy metadata degrades safely without inventing an actor or breaking branch
  editing/base-branch controls.
- Fine-pointer desktop supports hover and keyboard focus; coarse-pointer/mobile uses the same
  content in a Drawer with a 44px trigger.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/changes-git-credential-display.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/task/changes-git-credential-display.ts`
- focused parser/view-model tests
- `apps/web/components/task/changes-panel-header.tsx`
- focused header component tests
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel.tsx`
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`
- this task file and `plan.md`

## Dependencies

Task 02.

## Parallelism

Parallel-safe with Task 03: it owns frontend Changes-panel files only.

## Inputs

- Spec task-session snapshot and Changes-panel UX scenarios.
- Task 02 snapshot JSON contract.
- `useTouchDrawer` and existing responsive GitHub disclosure patterns.

## Output contract

Report snapshot parsing rules, exact displayed labels for every effective source, desktop/mobile
access behavior, RED/GREEN/typecheck results, files changed, and update task/plan status.
