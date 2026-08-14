---
spec: ../../specs/e2e-parallel-worker-flakiness/spec.md
created: 2026-08-12
updated: 2026-08-14
status: done
---

# Fix Plan: E2E task-panel spec reliability under parallel worker load

## Root cause

The rename flow scheduled a 150 ms focus timer before Radix restored focus to
the context-menu trigger. Under load, that timer and focus restoration raced
the E2E assertion. The repair defers entry to rename mode until
`ContextMenuContent.onCloseAutoFocus`, suppressing restoration only for Rename,
then focuses and selects the mounted textbox immediately.

Workspace-source submission performs real backend registration and worktree
setup before the dialog closes. The desktop spec started its visibility poll
before that request completed. It now waits for the exact successful POST and
response body before retaining the dialog-close assertion.

## Approach

Use Radix close-autofocus as the deterministic lifecycle boundary for Rename,
with a focused component regression that proves focus and selection without
advancing a 150 ms timer. Gate both valid source submissions on the exact POST
response and its body completion. Do not increase timeouts, add retries, or
change global Playwright settings.

## Verification strategy

Each task's targeted command must be run enough times, under contention, to
exercise the original race:

```sh
cd apps/web
pnpm run build:e2e
pnpm run e2e:raw tests/task/file-tree-rename.spec.ts tests/task/add-workspace-sources.spec.ts --project=chromium --workers=2
pnpm run e2e:raw tests/task tests/kanban tests/gitlab --project=chromium --workers=4
```

The second command is the original 4-worker reproduction; it should complete
with zero failures in the two target specs. Repeat it (results vary run to
run under contention) rather than treating one green run as sufficient.

## Tasks

| Task | Title | Depends on | Parallel-safe |
|---|---|---|---|
| 01 | Deterministic rename focus handoff | none | no |
| 02 | Response-gated workspace-source close wait | 01 | no |

Implementation was completed sequentially. The prior session's Go 1.19.8 toolchain
block did not reproduce in this session's environment (Go 1.26.0); full browser
verification ran to completion.

While implementing, `origin/main` was merged into this branch mid-session (by a
concurrent session sharing this worktree) and had already landed its own fix for
the same `add-workspace-sources.spec.ts` race, using a blind `{ timeout: 30_000 }`
bump on the dialog-close assertion (Approach B, the one this plan's Approach A
explicitly rejects). Main also landed a formal causal-wait helper library
(`apps/web/e2e/helpers/causal-waits.ts`, notably `waitForHttp`). The merge
conflict was resolved by keeping this plan's response-gated `submitWorkspaceSources`
helper (which supersedes and removes the need for main's timeout bump) rewritten
against the new canonical `waitForHttp` primitive instead of a hand-rolled
`page.waitForResponse` call, matching current repository convention.

## Status

- [x] Task 01: Deterministic rename focus handoff (component suite + focused E2E + 2x loaded 4-worker runs all green)
- [x] Task 02: Response-gated workspace-source close wait, rewritten on `waitForHttp` post-merge (focused E2E + 2x loaded 4-worker runs all green)

## Results

- Component regression (`file-context-menu.test.tsx`): 9/9 passed.
- Focused desktop E2E: `file-tree-rename.spec.ts` 4/4 passed; `add-workspace-sources.spec.ts` 2/2 passed (`--retries=0`).
- Mobile parity (`mobile-add-workspace-sources.spec.ts`, `--project=mobile-chrome`): fails on an unrelated, pre-existing assertion (`Add folder` button gated on `activeTask.primaryExecutorType` never becomes visible). Reproduced identically with this task's changes reverted (stashed) — confirmed pre-existing and out of scope, not a regression from this fix.
- Loaded 4-worker regression (`tests/task tests/kanban tests/gitlab --project=chromium --workers=4 --retries=0`), two fresh runs after the main merge:
  - Run 1: 368 passed, 8 failed. All failures unrelated (`task-dependencies.spec.ts`, `wip-overflow-queue.spec.ts`, `create-task-dependency-selector.spec.ts`, `subtask.spec.ts`) — a newly merged dependency feature hitting `404 page not found` on its API, plus other varying failures characteristic of host oversubscription. Both target specs' 6 tests all green.
  - Run 2: 366 passed, 10 failed. Different unrelated failure set/count (same dependency-feature 404s plus `workspace-switch-sidebar-isolation.spec.ts`), confirming these are contention/environment noise, not determinism. Both target specs' 6 tests all green again.
- Per the plan's Acceptance Criterion 8 fallback: the loaded runs show varying unrelated failures characteristic of host oversubscription and an unrelated, newly merged dependency-API bug; these are reported here rather than fixed, and did not motivate any timeout/retry/scope changes.
