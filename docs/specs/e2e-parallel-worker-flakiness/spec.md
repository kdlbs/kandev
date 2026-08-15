---
status: shipped
created: 2026-08-12
updated: 2026-08-14
---

# E2E task-panel spec reliability under parallel worker load

## Why

Two specs in `apps/web/e2e/tests/task/` fail intermittently when Playwright
runs with multiple workers sharing the host CPU, and pass reliably in
isolation. This was found during QA of an unrelated task (rich hover
previews) and reproduced independently on `main`-equivalent code, confirming
it is pre-existing flakiness, not a regression from that branch.

Every agent or CI shard that happens to run these specs under worker
contention hits an assertion that is racing a real, eventually-successful UI
transition against a timeout too short for a loaded host. Each occurrence
costs a re-run in isolation to prove it isn't the change under test. This
spec establishes the expected behavior for these specs going forward: they
tolerate host CPU contention for genuinely load-sensitive UI transitions
without becoming slower to catch true regressions where possible.

## Broken behavior

**`file-tree-rename.spec.ts`** (helper `startRenameViaContextMenu`, line 52):
after starting a rename, the row's inline `<Input>` receives focus from a
`setTimeout(..., 150)` inside `TreeNodeName`'s effect
(`apps/web/components/task/file-context-menu.tsx:504-519`). The spec asserts
`toBeFocused({ timeout: 5_000 })`. Under 4-worker CPU contention, the 150ms
timer can be delayed past the 5s assertion window even though it always
eventually fires — this is a scheduling delay, not a stuck or broken
component. Reproduced on 2026-08-12 with:

```sh
cd apps/web && pnpm run build:e2e
pnpm run e2e:raw tests/task tests/kanban tests/gitlab --project=chromium --workers=4
```

**`add-workspace-sources.spec.ts:163`** (test "adds a local repository and
folder successively, scopes Changes to Git, and persists"): after clicking
submit to attach a local Git repository source, the spec asserts
`expect(dialog).not.toBeVisible()` with the Playwright default 5000ms
timeout. Submitting calls `POST /api/v1/tasks/{taskId}/workspace-sources`
(`apps/web/lib/api/domains/kanban-api.ts:167`), which performs real backend
work (repository registration, worktree setup) before the dialog closes.
Reproduced independently on 2026-08-12 in the same 4-worker run:

```text
Error: expect(locator).not.toBeVisible() failed
Locator:  getByTestId('add-workspace-sources-dialog')
Expected: not visible
Received: visible
Timeout:  5000ms
  12 × locator resolved to ... data-state="open" ...
  - locator resolved to ... data-state="closed" ...   <- eventually closes
```

The captured call log shows the dialog transitioning from `open` to `closed`
after the 5s window — confirming this is the same root-cause class as the
first finding: a deterministic, eventually-true UI state raced against a
fixed timeout that is too tight for a CPU-contended host. The second submit
in the same test (adding a plain folder source, `add-workspace-sources.spec.ts:187-188`)
follows the identical `submit.click()` -> `expect(dialog).not.toBeVisible()`
pattern with no explicit timeout and shares the same risk, even though it has
not yet been observed to fail.

## Desired behavior

- Selecting Rename records a pending rename. When the Radix context menu closes,
  `onCloseAutoFocus` prevents trigger-focus restoration only for that pending
  action and starts rename mode. Other menu actions retain default focus behavior.
- Once the inline textbox mounts, it receives focus and selects its filename
  immediately. The separate 400 ms blur safeguard remains unchanged.
- Both valid workspace-source submissions arm an exact successful
  `POST /api/v1/tasks/{taskId}/workspace-sources` response wait before Submit,
  await completion of its response body, then retain the dialog-close and
  downstream persistence assertions.
- Neither repair broadens Playwright timeouts, retries an action, forces an
  interaction, or adds a fixed sleep.

## Regression scenario

GIVEN Playwright runs `apps/web/e2e/tests/task/file-tree-rename.spec.ts` and
`apps/web/e2e/tests/task/add-workspace-sources.spec.ts` with multiple workers
under host CPU contention (e.g. `--workers=4` alongside other spec
directories)
WHEN each spec exercises its rename-focus and source-attach-dialog-close
flows
THEN both specs pass without needing isolation, because their waits are sized
or gated to the actual condition rather than a contention-sensitive fixed
timeout.

## Out of scope

- Removing or redesigning the production 400ms blur gate
  in `file-context-menu.tsx` — this protects an unrelated blur race
  documented in the spec's own header comment and is not implicated by this
  flakiness.
- The CI shard/timing-profile work tracked in
  `docs/specs/e2e-duration-aware-sharding/spec.md` and its "seven retry
  groups" — this is a newly identified, separate pair of races, not one of
  those seven.
- Any other spec in `apps/web/e2e/tests/task/` not named above; the 4-worker
  reproduction run (357 tests) surfaced only these two failure sites across
  two separate runs.
