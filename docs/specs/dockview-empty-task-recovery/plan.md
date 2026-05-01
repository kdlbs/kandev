# Plan — Dockview Recovery for Sessionless Task Switches

> Implementation plan for the `dockview-empty-task-recovery` spec.
> Workflow: TDD (Red → Green → Refactor) per `.claude/skills/tdd/SKILL.md`.

## 1. Overview

Tasks created via the kandev MCP tool with `start_agent=false` have no session/environment.
Selecting such a task in the task-details sidebar today causes:

1. The Dockview layout to corrupt (wrong widths on the central group).
2. The top bar (task title, branch) to stay on the previous task.
3. The corruption to persist across subsequent task switches because the broken
   layout is saved back to per-session storage.

Goals:

- Make sessionless task selection update the active task, URL, and Dockview layout deterministically.
- Detect a corrupted Dockview layout after restore and self-heal by rebuilding the default.
- Cover with one E2E test (mock-agent + MCP `create_task_kandev` with `start_agent=false`)
  and targeted unit tests on the restore + switch logic.

Out of scope:

- Refactoring `task-session-sidebar.tsx` beyond what the fix requires.
- Changing how layouts are persisted to storage.
- Touching backend MCP behavior beyond the existing `start_agent=false` path (already correct).

## 2. Prerequisites

- No DB migrations or config changes.
- The mock-agent's `e2e:mcp:kandev:create_task_kandev(...)` already works
  (`apps/backend/cmd/mock-agent/script.go:120`, `executeMCPCommand`); the new test
  exercises that path with `start_agent: false`.
- `__dockviewApi__` is exposed on `window` for E2E inspection
  (`apps/web/lib/state/dockview-store.ts:614`).

## 3. Implementation Steps

### Step 1 — Failing E2E: sessionless-task selection (RED)

Description: write the spec that reproduces the bug end-to-end.

- **File (new)**: `apps/web/e2e/tests/task/sessionless-task-switch.spec.ts`
- Flow:
  1. Seed task A with an agent (`apiClient.createTaskWithAgent`,
     `description: "/e2e:simple-message"`).
  2. Navigate to task A; wait for `session.idleInput()` and stable layout.
  3. Use the agent to create task B sessionless via mock script:
     `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"Empty Task","description":"…","start_agent":false})`
     (mock-agent forwards to the real MCP handler).
  4. Click task B in the sidebar via `session.clickTaskInSidebar("Empty Task")`.
  5. Assert breadcrumb shows "Empty Task" within 10s.
  6. Assert URL matches `/t/<taskBId>`.
  7. Assert Dockview central group width ≥ a sane minimum (use a new helper,
     see Step 2). Specifically: no group in the live grid has `width <= 1` and
     the sum of root children widths ≈ `api.width`.
  8. Click task A back: `idleInput()` becomes visible again and central group
     widths remain valid (regression on layout-corruption persistence).

Expectation: this test fails on `main` (title stays on A, central group width is broken).

### Step 2 — E2E helper: layout sanity assertion

Description: add a small helper to assert no Dockview groups have zero/near-zero width.

- **File (modify)**: `apps/web/e2e/pages/session-page.ts`
- Add method `expectLayoutHealthy()` that does a `page.evaluate` on `__dockviewApi__`,
  walking `api.groups` and asserting `g.width > 1` for every group, and that
  `Math.abs(sum(rootChildrenWidth) - api.width) <= 4` (rounding tolerance).
- Reuse `expectNoLayoutGap()` patterns where possible — keep additions <40 lines.

### Step 3 — Failing unit test: `tryRestoreLayout` rejects corrupted layouts (RED)

Description: pin the contract that a structurally-broken saved layout is discarded.

- **File (new)**: `apps/web/components/task/dockview-layout-restore.test.ts`
- Cases:
  - Returns `false` when `getSessionLayout` yields a layout where any leaf has
    `views: []` after sanitization (covered today by `cleanNode` returning null,
    keep as a regression).
  - **NEW failing case**: returns `false` when the sanitized layout's
    `grid.root.size <= 0` or any branch child has `size <= 0` (zero-width groups).
  - Mock `getSessionLayout` from `@/lib/local-storage` and `applyLayoutFixups`
    from `@/lib/state/dockview-layout-builders` (mirror existing
    `dockview-session-switch.test.ts` mock style).

### Step 4 — Implement layout sanity check in `sanitizeLayout` (GREEN)

Description: extend the existing sanitizer to reject grids with zero/missing sizes,
so a corrupted persisted layout is dropped instead of restored.

- **File (modify)**: `apps/web/components/task/dockview-layout-restore.ts`
- Add an internal `hasValidSizes(node): boolean` walker:
  - For `leaf`: `typeof node.size === "number" && node.size > 0` (or size missing
    and parent is the root single-leaf — handle by treating undefined size at root
    as valid since dockview infers it from container width).
  - For `branch`: every child `hasValidSizes` and `children.length > 0`.
- In `sanitizeLayout`, after `cleanNode` produces `cleanedRoot`, return `null`
  when `!hasValidSizes(cleanedRoot)`.
- This causes `tryRestoreLayout` to fall through to the default-build path.

### Step 5 — Failing unit test: sidebar select-task always updates store + URL for sessionless tasks (RED)

Description: lock down `handleSelectTask`'s contract for the sessionless branch.

- **File (new)**: `apps/web/components/task/task-session-sidebar-select.test.ts`
- Extract the pure decision into a testable helper (see Step 6) and assert:
  - When `prepareAndSwitchTask` resolves `false` (no session created), the helper
    invokes `setActiveTask(taskId)` AND calls `performLayoutSwitch` with the old
    session and a sentinel "reset" path (or equivalent: triggers
    `resetLayout()` — see Step 6).
  - URL is replaced exactly once (mock `replaceTaskUrl`).
  - The `oldSessionId` captured before `launchSession` is the one passed to
    layout-switch (regression on the comment at lines 322–325).

### Step 6 — Implement sessionless-switch fix (GREEN)

Description: when no session can be created, explicitly tear down the prior
session's layout instead of leaving it stale.

- **File (modify)**: `apps/web/components/task/task-session-sidebar.tsx`
  - In `prepareAndSwitchTask` (lines 316–345), when `launchSession` returns no
    `session_id` OR throws: before returning `false`, call a new helper
    `releaseLayoutForTask(oldSessionId)` that:
    - Saves the outgoing session's layout (already handled by
      `switchSessionLayout` when passed both args), then
    - Calls `useDockviewStore.getState().resetLayout()` which builds the default
      layout via `buildDefaultLayout(api)` (`dockview-store.ts:664–667`).
  - In `handleSelectTask` (lines 445–476), the `if (!switched) setActiveTask(taskId)`
    branch must run BEFORE the next paint (it already does, but ensure
    `replaceTaskUrl` is called in both branches — currently fine).
- **File (modify)**: `apps/web/lib/state/dockview-store.ts` — no API change; we
  reuse `resetLayout`. If a small helper export is cleaner, add
  `releaseSessionLayout(oldSessionId)` that wraps `saveOutgoingSession` +
  `resetLayout` and is called from the sidebar.

### Step 7 — Optional backend test (only if needed)

Description: only add if Step 1's E2E reveals the mock-agent path doesn't propagate
`start_agent=false` correctly. Otherwise skip — the existing
`TestCreateTask_ToolSchema_HasParentID` already locks the schema.

- **File (modify)**: `apps/backend/internal/agentctl/server/mcp/handlers_test.go`
  - Add `TestCreateTask_StartAgentFalse_DoesNotAutoStart` that asserts the
    forwarded payload includes `start_agent: false` when the tool arg is `false`.

### Step 8 — Verify (REFACTOR + sign-off)

- Run from `apps/`:
  - `pnpm --filter @kandev/web test -- dockview-layout-restore` (unit).
  - `pnpm --filter @kandev/web test -- task-session-sidebar-select` (unit).
  - `pnpm --filter @kandev/web e2e -- sessionless-task-switch` (E2E).
- Run `make -C apps/backend test` if Step 7 was needed.
- Run `make fmt` then `make typecheck test lint` per AGENTS.md.

## 4. File Changes Summary

Created:

- `apps/web/e2e/tests/task/sessionless-task-switch.spec.ts`
- `apps/web/components/task/dockview-layout-restore.test.ts`
- `apps/web/components/task/task-session-sidebar-select.test.ts`

Modified:

- `apps/web/components/task/dockview-layout-restore.ts` — add `hasValidSizes` and
  reject corrupted grids in `sanitizeLayout`.
- `apps/web/components/task/task-session-sidebar.tsx` — extract testable helper;
  release layout when sessionless task is selected.
- `apps/web/lib/state/dockview-store.ts` — optional `releaseSessionLayout` export
  (only if it improves call-site clarity).
- `apps/web/e2e/pages/session-page.ts` — add `expectLayoutHealthy()` helper.
- `apps/backend/internal/agentctl/server/mcp/handlers_test.go` — only if Step 7
  is required.

Deleted: none.

## 5. Testing Strategy

Unit (vitest):

- `dockview-layout-restore.test.ts`: sanitizer rejects zero-size branches/leaves;
  sanitizer keeps healthy layouts unchanged (snapshot-style structural compare).
- `task-session-sidebar-select.test.ts`: extracted `prepareAndSwitchTask` helper
  releases layout + falls back to `setActiveTask` when launch fails; captured
  `oldSessionId` is forwarded to layout-switch.

E2E (Playwright, `KANDEV_MOCK_AGENT=only`):

- `sessionless-task-switch.spec.ts`: covers the full bug scenario including
  return-trip to the original task.

Manual smoke (one-off):

1. Start kandev in dev mode.
2. From the agent, run `create_task_kandev` with `start_agent=false`.
3. Click the new task in the sidebar — confirm top bar updates and the central
   group keeps a sane width.
4. Click back to the agent task — confirm layout restored without artefacts.

## 6. Rollback Plan

- All changes are confined to frontend modules and one optional backend test.
- Revert by `git revert` of the merge commit; no DB migrations or persisted-format
  changes (the sanitizer is strictly more conservative — it only rejects layouts
  the app already cannot render).

## 7. Estimated Effort

- Complexity: **medium** (multi-layer fix: store + restore sanitizer + sidebar handler).
- Time: ~0.5–1 day including TDD cycle and CI fixups.
