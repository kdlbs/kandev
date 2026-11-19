---
status: implemented
created: 2026-11-19
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-002
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
  - REQ-INTEGRATIONS-PROMPT-SYNC-005
  - REQ-INTEGRATIONS-PROMPT-SYNC-006
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Prompt Sync

## Overview

GitHub issue #5391. Saved prompts that workflow steps reference by `@name`
cannot be versioned with the workflows that use them. Add one install-wide
Prompt Sync source (its own repository directory, schedule, and status) that
reads through a chosen workspace's existing GitHub or GitLab connection. Lock
synced prompts against edits, and have Workflow Sync warn about unresolved
`@name` references.

## Technical approach

Each layer is shippable on its own:

1. Extract the provider-neutral repository reads and the failure policy from
   `workflowsync` into `internal/reposync`, without behavior changes, so both
   syncs share one implementation.
2. Teach the prompt service about ownership (`source`, `source_path`):
   read-only guards, sync-only write methods, and reference helpers.
3. Add `internal/promptsync`: a single-row config store, service, poller, and
   routes, wired in `backendapp`.
4. Add unresolved-reference warnings to Workflow Sync.
5. Add the web UI: synced badge and read-only rows, plus the Prompt Sync card
   and dialog.
6. Add E2E coverage and public docs, then promote the specs.

## Delivery order

| Wave | Work order | Depends on |
| --- | --- | --- |
| 1 | [Task 01: Extract shared repository-sync helpers](task-01-extract-reposync.md) | none |
| 1 | [Task 02: Prompt ownership and read-only guards](task-02-prompt-ownership.md) | none |
| 2 | [Task 03: Prompt Sync backend](task-03-promptsync-backend.md) | 01, 02 |
| 2 | [Task 04: Workflow Sync unresolved-reference warnings](task-04-workflow-sync-reference-warnings.md) | 02 |
| 3 | [Task 05: Prompt Sync settings UI](task-05-prompt-sync-ui.md) | 02, 03 |
| 4 | [Task 06: E2E coverage and public docs](task-06-e2e-and-docs.md) | 03, 04, 05 |

Tasks 01 and 02 touch separate packages and can run in parallel, as can 03
and 04. Waves mark parallel-safe candidates only; they do not authorize
subagents.

## ASCII UI preview

These are structural requirements: the card sits above the prompt list, the
synced badge sits next to the name, synced rows have no edit or delete
controls, and the dialog field order is as shown. Spacing and exact styling
are illustrative; use the existing shadcn primitives and localized copy.

### UI-01: Settings > Prompts, Prompt Sync configured (desktop)

```text
+----------------------------------------------------------------------+
| Prompts                                              [+ New prompt]  |
|                                                                      |
| +------------------------------------------------------------------+ |
| | Prompt Sync                          [Sync now]  [Configure]     | |
| | acme/engineering @ main : .kandev/prompts  (via workspace: Core) | |
| | Last synced 2 min ago - 1 warning                       [v]      | |
| |   . "review" was removed from the source but is used by 2        | |
| |     workflows; it was kept                                       | |
| +------------------------------------------------------------------+ |
|                                                                      |
| [ Search prompts...                                ]                 |
|                                                                      |
| +------------------------------------------------------------------+ |
| | @principles  [Synced]                                            | |
| | Always prefer the simplest implementation that ...               | |
| +------------------------------------------------------------------+ |
| | @code-review  [Built-in]                          [edit] [del]   | |
| | Review the diff for correctness ...                              | |
| +------------------------------------------------------------------+ |
| | @my-notes                                         [edit] [del]   | |
| +------------------------------------------------------------------+ |
+----------------------------------------------------------------------+
  Hovering or focusing [Synced] shows "Synced from prompts/principles.md".
```

### UI-02: Settings > Prompts, not configured (desktop)

```text
| +------------------------------------------------------------------+ |
| | Prompt Sync                                         [Set up]     | |
| | Keep shared prompts in a repository and sync them here.          | |
| +------------------------------------------------------------------+ |
```

### UI-03: Prompt Sync dialog (desktop)

```text
+---------------------------------------------------+
| Prompt Sync                                    [x]|
|                                                   |
| Provider        ( GitHub | GitLab )               |
| Workspace       [ Core                       v ]  |
|   Reads use this workspace's GitHub connection.   |
| Repository      [ https://github.com/acme/eng  ]  |
| Branch          [ main ]                          |
| Directory       [ .kandev/prompts ]               |
| Auto-sync       [on]   Interval (s) [ 300 ]       |
|                                                   |
| [Remove sync]                  [Cancel]  [Save]   |
+---------------------------------------------------+
  "Remove sync" opens the existing inline (desktop) or mobile confirmation.
```

### UI-04: Phone composition

```text
+------------------------------+
| Prompts            [+]       |
| +--------------------------+ |
| | Prompt Sync              | |
| | acme/engineering         | |
| | main : .kandev/prompts   | |
| | Last synced 2 min ago    | |
| | 1 warning            [v] | |
| | [Sync now] [Configure]   | |
| +--------------------------+ |
| +--------------------------+ |
| | @principles  [Synced]    | |
| | Always prefer the ...    | |
| +--------------------------+ |
| | @my-notes     [edit][del]| |
| +--------------------------+ |
+------------------------------+
  On phones the actions wrap below the status line, and the dialog follows the
  existing Workflow Sync dialog's mobile presentation, which is a full-height
  sheet with stacked fields. Tapping the badge shows the source path.
```

These previews map to AC-INTEGRATIONS-PROMPT-SYNC-001.1, 001.3, 003.7,
004.3, and 004.5.

## Verification strategy

- Backend: `go test -trimpath` for `internal/reposync`, `internal/workflowsync`,
  `internal/prompts/...`, `internal/promptsync`, and `internal/mcp/handlers`
  (`-run Prompt`), `internal/persistence/storeconformance` (with `-race`), and
  `go run ./cmd/sqlguard ./internal`. Run PostgreSQL variants with
  `KANDEV_TEST_POSTGRES_DSN` where available.
- Lint the changed Go files with `bash scripts/lint-go-changed`.
- Web: vitest for the new and changed components, plus `pnpm run typecheck`,
  `pnpm run lint`, and `pnpm run i18n:check`.
- E2E: `apps/web/e2e/tests/settings/prompt-sync.spec.ts` (chromium) and
  `mobile-prompt-sync.spec.ts` (mobile-chrome) against the mock GitLab
  provider (the same fixture `workflow-sync-gitlab.spec.ts` uses).
- Specs: `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all`.
