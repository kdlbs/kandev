---
id: "01-stabilize-file-tree-rows"
title: "Stabilize file-tree row inputs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/file-tree-chat-context.md"
system_design: "../../specs/ui/system-design/task-surface-render-isolation.md"
---

# Task 01: Stabilize file-tree row inputs

## Acceptance

- `onToggleExpand` and `onDrop` keep their identities during unrelated file-browser renders.
- `useFileBrowserTree` keeps stable aggregate identities when its fields do not change.
- `TreeNodeItem` skips renders when all visible and interactive inputs remain equal.
- Selection, expansion, active-file, drag, edit, and context-action changes still update the affected
  row.
- Existing file-tree behavior remains unchanged.

This task preserves `AC-UI-FILE-TREE-CHAT-CONTEXT-001.1`, `.2`, and `.7`.

## TDD sequence

1. Add a render-count test that fails after an unrelated owner update.
2. Add identity tests for the tree aggregate and the two traced callbacks.
3. Stabilize callback dependencies and the hook result.
4. Add the memoized row boundary with complete semantic inputs.
5. Run the focused file-tree regression tests.

## Verification

```bash
cd apps
rtk pnpm --filter @kandev/web test components/task/file-browser-render-identity.test.tsx components/task/file-browser-actions.test.ts components/task/file-browser-toggle-expand.test.ts components/task/file-browser-restore-expanded.test.tsx
rtk pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/file-browser-hooks.ts`
- `apps/web/components/task/file-browser-parts.tsx`
- `apps/web/components/task/file-browser-render-identity.test.tsx`
- Existing focused file-browser tests

## Dependencies

None.

## Inputs

- `REQ-UI-FILE-TREE-CHAT-CONTEXT-001`
- Task Surface Render Isolation system design
- Trace evidence for `onToggleExpand`, `onDrop`, and complete row render waves

## Output contract

Report the stable dependency strategy, the row equality inputs, changed files, focused test results,
and risks. Do not add time-based unit assertions.
