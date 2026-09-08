---
id: "04-e2e-and-verification"
title: "E2E and full verification"
status: done
wave: 4
depends_on: ["03-frontend-wiring-and-affordances"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/subtask-reparenting-drag-drop.md"
---

# Task 04: E2E and full verification

## Acceptance

- Desktop Playwright spec covers: subtask dragged onto another root's nest zone re-parents (assert API `parent_id` and sidebar nesting, no reload); `inherit_parent` subtask ends with `shared_group` (assert via API metadata); root dragged onto root nests; a task with children offers no nest zone; a drop on an invalid row is a no-op; sibling reorder still works.
- Mobile-chrome touch variant covers the same re-parent flow via `Input.dispatchTouchEvent`.
- `make fmt` then `make typecheck test lint` pass from the repository root.

## Verification

```bash
cd apps/web && pnpm e2e:run e2e/tests/task/subtask-reparent-drag-drop.spec.ts
cd /home/clem/.kandev/tasks/re-parenting-from-dr_0b8/kdlbs-kandev && make fmt && make typecheck && make test && make lint
```

Note: run `pnpm install --frozen-lockfile` from `apps/` once if this is a fresh worktree (missing `node_modules` breaks pnpm commands and the commit hook).

## Files likely touched

- `apps/web/e2e/tests/task/subtask-reparent-drag-drop.spec.ts` (new) — model structure, seeding, and sidebar interaction on `apps/web/e2e/tests/task/subtask-detachment.spec.ts`; drag mechanics via `locator.hover()` + `page.mouse.down()` + stepped `mouse.move()` + `mouse.up()` (dnd-kit needs >8px movement before activation).
- Mobile variant: same spec gated to `mobile-chrome` project or a sibling spec, using the `Input.dispatchTouchEvent` pattern from `apps/web/e2e/tests/mobile-automations-scroll.spec.ts`.

## Dependencies

Tasks 01–03.

## Inputs

- Spec Scenarios (each maps to a test), plan E2E section, existing `subtask-detachment` e2e specs and helpers (`apps/web/e2e/helpers/api-client.ts` createTask/updateTask with `parent_id` and `workspace_mode`).

## Output contract

Report the spec files, exact commands and results (including per-scenario pass counts), files changed, blockers, residual risks; update this task and `plan.md` when acceptance passes.

## Results

- Desktop: `pnpm e2e:run tests/task/subtask-reparent-drag-drop.spec.ts --shards 1` — 5 passed (subtask→root reparent + workspace-mode normalization, root→root nest, no nest zones for a task with children, subtask-row drop no-op, sibling reorder regression).
- Mobile: `pnpm e2e:run --project mobile-chrome tests/task/mobile-subtask-reparent-drag-drop.spec.ts --shards 1` — 1 passed (touch drag via CDP `Input.dispatchTouchEvent`; source handle and target zone are scrolled into view first — the sheet scrolls and off-viewport coordinates make raw mouse/touch events no-ops).
- Root verification: `make fmt`, `make typecheck`, `make lint` pass (umask 022). `make test`: all changed packages pass; residual failures are pre-existing environment issues (launchd/systemd/cli-shim backend tests, Docker-bridge-dependent `http-git-server.test.ts`, load-sensitive file-browser timeouts that pass in isolation) — verified failing on the stash-clean base.
- Files changed: `e2e/tests/task/subtask-reparent-drag-drop.spec.ts` (new), `e2e/tests/task/mobile-subtask-reparent-drag-drop.spec.ts` (new). Temporary diagnostic spec `mobile-drag-diag.spec.ts` created and removed.
