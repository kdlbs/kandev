Failed automation runs were accumulating and blocking the scheduler's `max_concurrent_runs` cap; this adds per-row and bulk deletion so the history can be cleared without touching the automation itself.

## Important Changes

- **Backend:** two new WS actions `automation.run.delete` / `automation.runs.delete_all`; for runs with a `task_id` the service routes through `task.Service.DeleteTask` first (idempotent against `ErrTaskNotFound` so stale/orphaned rows are always clearable). A `TaskDeleter` interface is injected via `SetTaskDeleter` in `backendapp` — no import cycle.
- **Frontend:** per-row trash button (`opacity-0`, revealed on row hover, `stopPropagation` so it doesn't navigate to the task); delete-all button in the section header wrapped in an `AlertDialog` confirmation showing the run count. Optimistic store updates; `toast.error` on failure with list revert.
- **E2E seed endpoints:** `POST /api/v1/e2e/automations` and `POST /api/v1/e2e/automation-runs` under the existing `KANDEV_MOCK_AGENT`-gated `/api/v1/e2e` group — HTTP so tests work on Node 20.

## Validation

```bash
# Backend unit + service tests (includes regression guard)
cd apps/backend && go test ./internal/automation/... -v

# Backend lint (changed packages only)
golangci-lint run ./internal/automation/... ./internal/backendapp/... ./pkg/websocket/...

# Frontend typecheck + lint
cd apps && pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint

# Playwright E2E (expand → delete one → confirm delete-all → empty state)
cd apps/web && KANDEV_E2E_MOCK=true pnpm e2e --project=chromium -g "delete individual and all runs"
```

`TestDeleteAllRuns_AutomationSurvives` issues real `DELETE FROM tasks` SQL against the shared in-memory DB to verify no SQL trigger or `ON DELETE CASCADE` regression can delete the parent automation. Event-handler side effects are not covered (no orchestrator in that test).

Pre-existing flake: `TestProcessRunnerCapturesOutput` in `internal/agentctl/server/process` fails independently of this change (confirmed stash-and-rerun).

## Screenshots

**Collapsed** — header shows only the refresh icon (old behaviour):

![collapsed](.pr-screenshots/1-collapsed.png)

**Expanded** — refresh + red trash icon in header; 6th column added for per-row delete:

![expanded](.pr-screenshots/2-expanded.png)

**Row hover** — per-row trash button becomes visible on hover:

![row hover](.pr-screenshots/3-row-hover.png)

**Delete-all confirmation dialog** — shows run count, requires explicit confirm:

![dialog](.pr-screenshots/4-confirm-dialog.png)

## Possible Improvements

Low risk for non-running runs; deleting a `task_created` run stops the associated task — acceptable given the feature intent is cleanup of stuck/failed history.

## Checklist

- [ ] I have performed a self-review of my code.
- [ ] I have manually tested my changes and they work as expected.
- [ ] My changes have tests that cover the new functionality and edge cases.
- [ ] If my change touches UI files (`apps/web/`), I have added or updated Playwright e2e tests in `apps/web/e2e/` and verified them with `make test-e2e`.
