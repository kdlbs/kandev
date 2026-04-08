# Prepare-env UX: spinner size + file-tree/terminal wait through long prepare

**Date:** 2026-04-08
**Status:** implemented
**PR:** #564

## Problem

Two related issues in the session chat prepare-env UX:

1. **Spinner size mismatch.** The top-level "Preparing environment..." header in `PrepareProgress` used `IconLoader2 h-3 w-3`, while each child step ("Validate repository", "Sync base branch", "Create worktree") used `h-3.5 w-3.5` — the header spinner rendered visibly smaller than the rows under it.
2. **File tree and terminal give up during long prepare.** When workspace preparation took >18s (e.g. a slow `git fetch` on a large monorepo), the right-sidebar file tree exhausted its retry budget (`[1000, 2000, 5000, 10000]` ms → 18s cumulative) and transitioned to a `manual` "Load Files" state. The user was then stuck unless they clicked retry. The passthrough terminal reconnect loop also spammed failing WS attempts at the backend while agentctl wasn't ready.

Root cause for (2): `useFileBrowserTree` kicked off `loadTree()` on mount regardless of `agentctlStatus.isReady`, racing a bounded retry budget against unbounded prepare time. `PassthroughTerminal.canConnect` didn't check `agentctlStatus.isReady` either, so its reconnect loop hammered the terminal WS endpoint while agentctl was down.

## Design

Three small surgical changes, no new abstractions:

### 1. Spinner size — `apps/web/components/session/prepare-progress.tsx`

Bump the header `IconLoader2` from `h-3 w-3` → `h-3.5 w-3.5` to match `StepIcon`. Also add `data-testid="prepare-progress-header-spinner"` for E2E/test hooks.

### 2. File tree — `apps/web/components/task/file-browser-hooks.ts`

Gate the initial `loadTree()` on `agentctlStatus.isReady` inside the reset effect of `useFileBrowserTree`:

- Capture `agentctlStatus.isReady` via a render-time ref (`agentctlReadyAtMountRef`) so the reset effect can read the latest value without depending on it (depending on it would re-run the reset effect on every ready-flip and wipe the tree).
- If not ready at mount: set `loadState = "waiting"`, leave `retryAttemptRef = 0`, and skip `loadTree()` entirely.
- The sibling effect watching `agentctlStatus.isReady` already triggers `loadTree({ resetRetry: true })` when readiness flips true, so it becomes the single initial-load trigger for the cold path.

Result: during a 40s prepare the panel sits in `"waiting"` ("Preparing workspace..." with a spinner) indefinitely, then loads exactly once when agentctl is ready. The 4-retry/18s budget still guards post-ready transient failures.

### 3. Terminal — `apps/web/components/task/passthrough-terminal.tsx`

Add `agentctlStatus.isReady` to `canConnect`:

```ts
const agentctlStatus = useSessionAgentctl(sessionId);
const canConnect = Boolean(sessionId && isActive && agentctlStatus.isReady);
```

The existing "Preparing workspace..." / "Connecting terminal..." overlay already covers the `!isConnected` state, so UX is unchanged — the reconnect loop just stops hammering the backend while agentctl is down.

### Testids for regression tests

- `data-testid="prepare-progress-header-spinner"` on the header `IconLoader2`.
- `data-testid="file-tree-waiting"` on the waiting-state div in `file-browser-load-state.tsx`.
- `data-testid="file-tree-manual"` on the manual-state div.

### E2E regression test

`apps/web/e2e/tests/session/long-prepare-panels.spec.ts`:

1. Writes `25000` to `${backend.tmpDir}/git-delay-ms` so the `git` shim installed by the backend fixture makes `git fetch origin main` sleep 25s during `worktree.Manager.pullBaseBranch`. The shim is a POSIX script at `${tmpDir}/bin/git` that sleeps on `fetch`/`pull` when the delay file exists, then `exec`s the real `git` — transparent passthrough when the file is absent.
2. Creates a task with `executor_profile_id: seedData.worktreeExecutorProfileId` — required, otherwise the task falls through with `executor_type: ""`, the worktree preparer never runs, and no fetch is invoked.
3. Asserts `file-tree-waiting` is visible immediately after `waitForLoad()`.
4. Waits 19s (past the pre-fix retry budget), asserts `file-tree-manual` has count 0 and `file-tree-waiting` is still visible. **This is the assertion that flips red pre-fix** (the file tree burns through its retries and transitions to manual).
5. Asserts `file-tree-waiting` is hidden within 30s of fetch completing (recovery path).
6. Cleans up the delay file in `finally` so sibling tests in the same worker run with fast git.

The `backend` worker fixture is destructured alongside `testPage`, `apiClient`, `seedData` so the test can access `backend.tmpDir`.

## Implementation Notes

**Post-implementation gotchas:**

- **The E2E frontend uses `.next/standalone/web/server.js`** — a production build. Component changes (including new testids) don't take effect until `pnpm --filter @kandev/web build` is run. Cost me an hour of "the testid isn't being found" before I dumped the `innerHTML` and saw the old markup.
- **`PassthroughTerminal` only renders in CLI-passthrough mode.** The mock agent used in E2E isn't passthrough, so the test can't assert on `passthrough-terminal` or `passthrough-loading` testids. The terminal fix itself is covered indirectly (the `canConnect` gating is exercised whenever the shell-mode passthrough runs in any test) but not directly asserted in this spec.
- **`prepare-progress-header-spinner` isn't reliably visible in the E2E harness** — the `PrepareProgress` component depends on `prepareProgress.bySessionId[sessionId]` being populated via WS events, and in local-standalone runs the events sometimes fire before the frontend subscribes. The spinner-size fix is visually verifiable and the class change is obvious in code review; we don't assert it in the E2E.
- **`git` shim path lookup**: the shim uses `exec git "$@"` after restoring `PATH` from `$KANDEV_E2E_ORIGINAL_PATH`. If the original `PATH` doesn't contain a real git, the shim will recurse. Every dev/CI environment has git, so this is acceptable.
- **`executor_profile_id` is load-bearing for the test**: without it, `executor_type` resolves to empty string, `runEnvironmentPreparer` short-circuits at the "no preparer registered" check, no fetch runs, and the shim sleep never fires. The test logs this as a comment.
- **Why not `prepare_script: "sleep 30"` on a profile?** Tried it. The profile plumbing works and the script runs synchronously inside `env_preparer_worktree.Prepare`, but in local standalone the agentctl instance creation path is not serialised behind it (they happen in parallel via separate goroutines), so the frontend sees `isReady=true` before the sleep completes. Only the in-preparer `git fetch` genuinely blocks the entire Launch flow.

**TDD verification** performed:

- Post-fix: test passes in ~27s (`file-tree-waiting` → wait 19s → `file-tree-manual` count 0 → `file-tree-waiting` hidden after fetch).
- Pre-fix (files reverted via `git stash` + rebuild): test fails at the `fileTreeManual toHaveCount(0)` assertion at t+19s with `unexpected value "1"` — the file tree transitioned to manual, exactly the regression.
