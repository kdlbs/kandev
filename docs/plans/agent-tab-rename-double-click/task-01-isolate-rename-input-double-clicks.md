---
id: "01-isolate-rename-input-double-clicks"
title: "Isolate Rename Input Double-clicks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002
acceptance_criteria:
  - AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.1
  - AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.2
  - AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.3
system_design:
  - ../../specs/ui/system-design/task-agent-tab-reconciliation.md
---

# Task 01: Isolate Rename Input Double-clicks

## Summary

Add a failing desktop browser regression for the Agent-tab rename interaction,
then make the shared task tab rename editor contain double-clicks without
disabling native input selection. Preserve the existing maximize behavior for
double-clicks outside rename mode.

## In scope

- Add the focused Agent-tab rename Playwright regression.
- Stop bubbling `dblclick` events at `TabRenameInput`.
- Verify native selection and unchanged Dockview maximize state.

## Out of scope

- Rename persistence and backend session APIs.
- Dockview layout or maximize-state refactors.
- Quick Chat rename behavior.
- Phone and tablet UI changes.

## Acceptance

- The new Playwright scenario fails on the current code because an editor
  double-click maximizes the Agent-tab group.
- Double-clicking the active rename input selects text without changing the
  Dockview group's maximize state.
- Double-clicking the non-editing Agent-tab surface still toggles maximize.

## Verification

```bash
pnpm e2e:run tests/session/session-tab-rename.spec.ts
pnpm run typecheck
pnpm exec eslint components/task/tab-rename-input.tsx e2e/tests/session/session-tab-rename.spec.ts
git diff --check
```

Run the first three commands from `apps/web`. Run `git diff --check` from the
repository root.

## Files likely touched

- `apps/web/e2e/tests/session/session-tab-rename.spec.ts`
- `apps/web/components/task/tab-rename-input.tsx`

## Dependencies

None.

## Risks

- Preventing the default `dblclick` action would suppress native input text
  selection.
- A selector that observes a different Dockview group could allow the browser
  regression to pass while the Agent group still toggles.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002`
- `AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.1` through `.3`
- `docs/specs/ui/system-design/task-agent-tab-reconciliation.md`
- `apps/web/components/task/session-tab.tsx`
- `apps/web/components/task/tab-rename-input.tsx`
- `apps/web/components/task/use-tab-maximize.ts`

## Results

Completed on 2026-08-31.

- RED: `pnpm e2e:run --no-build tests/session/session-tab-rename.spec.ts`
  failed on the expected maximize-state assertion after an editor
  double-click.
- GREEN: `pnpm e2e:run tests/session/session-tab-rename.spec.ts` rebuilt the
  production frontend and passed 1 Chromium test.
- `pnpm run typecheck` passed.
- `pnpm exec eslint components/task/tab-rename-input.tsx e2e/tests/session/session-tab-rename.spec.ts`
  passed.
- `git diff --check` passed.
