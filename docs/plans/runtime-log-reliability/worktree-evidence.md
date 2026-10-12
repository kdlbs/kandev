# Worktree cleanup evidence

## Historical observation

The retained backend log at `/root/.kandev/logs/backend-logs-2026-10-05-000005.log:20589` records one cleanup refusal for worktree `54202d4a-5b17-411a-82bb-91c7c0ac113d` under task `13f1a547-415a-47f6-9cc9-41ebeb17515e` on 2026-10-05 at 08:06:15 Lisbon time. The classified reason is `registration_inspection (competing_registration)`. The nearby summary and retry-job records refer to the same request, not separate cleanup attempts.

A search of the retained backend log files found no second attempt for these IDs. The plan's earlier claim of two attempts about twelve hours apart is therefore not independently confirmed by the available files.

## Identity evidence

The historical record does not include the saved or observed repository ID, repository path, worktree path, branch, HEAD, common Git directory, ownership marker, or active-consumer state. Those identities are **unavailable**. The exact worktree registration cannot be inspected without the installation database and checkout path. This investigation did not read or modify the active database, markers, checkout, Git refs, or runtime state.

## Fixture results

The existing disposable fixtures passed. Safe preview preserves database rows and Git refs. Refusal coverage includes changed or foreign repository/branch/marker/row identities, a competing inventory claim for the same path, a live process, a nonterminal consumer, and cleanup-scope changes. Cleanup tests preserve dirty checkouts and reject post-audit path replacement. The repair fixture also preserves the predecessor cleanup snapshot and progress while recording a successor. No implementation changes were needed.

Commands passed with the race detector:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/inventoryrepair ./cmd/worktree-inventory-repair -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/worktree -run 'Test.*(Cleanup|Registration)' -count=1)
```

## Classification and eligibility

Classification: **unresolved historical identity; current behavior is a correct fail-closed ownership refusal under incomplete evidence**. The log does not establish a cleanup defect or authorize an explicit repair. Repair eligibility is **none** until a separately authorized, read-only evidence capture can establish exact saved and observed identities and the existing preview accepts those observations. No live repair was run.
