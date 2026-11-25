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
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-008
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
  - REQ-INTEGRATIONS-PROMPT-SYNC-010
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
| 5 | [Task 07: View synced prompt content](task-07-view-synced-prompts.md) | 05, 06 |
| 6 | [Task 08: Review remediation](task-08-review-remediation.md) | 03, 05 |
| 7 | [Task 09: Local overrides](task-09-local-overrides.md) | 07, 08 |
| 8 | [Task 10: Override switch](task-10-override-switch.md) | 09 |
| 9 | [Task 11: Name conflicts, lossless switching, and agent guidance](task-11-conflicts-and-agents.md) | 10 |
| 10 | [Task 12: Instant switch, compact review, and two-version removal](task-12-compact-review.md) | 11 |
| 11 | [Task 13: Override switch as a pending edit, and repository-only cleanup on removal](task-13-pending-switch-and-cleanup.md) | 12 |
| 12 | [Task 14: Shared Local \| Repository control, several pending switches, and read-only styling](task-14-shared-version-control.md) | 13 |
| 13 | [Task 15: Rows open independently of the switch, several drafts, single outline, and read-only contrast](task-15-independent-rows-and-drafts.md) | 14 |
| 14 | [Task 16: PR review fixes for references, missing repository versions, and conflict choices](task-16-pr-review-fixes.md) | 15 |

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
| | @principles  [Synced]                                    [view]  | |
| | Always prefer the simplest implementation that ...               | |
| +------------------------------------------------------------------+ |
| | @code-review  [Built-in]                          [edit] [del]   | |
| | Review the diff for correctness ...                              | |
| +------------------------------------------------------------------+ |
| | @my-notes                                         [edit] [del]   | |
| +------------------------------------------------------------------+ |
+----------------------------------------------------------------------+
  Hovering or focusing [Synced] shows "Synced from prompts/principles.md".
  [view] toggles the full content read-only below the name, with a note
  naming the source file (Task 07).
```

### UI-05: Local overrides (desktop)

```text
| +------------------------------------------------------------------+ |
| | @post-pr-review [Synced]          (o ) Local override  [view]     | |
| | Run the default reviewer after the PR opens.                      | |
| +------------------------------------------------------------------+ |
| | @principles [Overridden]          ( o) Local override  [edit][del]| |
| | My local principles ...                                           | |
| +------------------------------------------------------------------+ |
| | @house-rules [Overridden] [Conflict] ( o) Local override [edit][del]|
| +------------------------------------------------------------------+ |
  The switch is always the first trailing control, so it never moves. Both
  directions apply at once; a toast names the version now in use.
  Status card adds: "Overridden locally: @principles" below warnings.
  [Overridden] and [Conflict] tooltips name the repository file.
```

### UI-06: Name conflicts (desktop and phone)

```text
+---------------------------------------------------+
| Review name conflicts                          [x]|
| The repository has prompts with the same names... |
| 2 prompts need a choice.                          |
| Local: your version stays in use as an override.  |
| Repository: the repository version is used; yours |
|   is kept.                                        |
| +-----------------------------------------------+ |
| | @house-rules       Local (o ) Repository  [v] | |
| | .kandev/prompts/house-rules.md                | |
| |-----------------------------------------------| |
| | @late-rule         Local (o ) Repository  [v] | |
| +-----------------------------------------------+ |
| [Back]                          [Save and sync]   |
+---------------------------------------------------+

Settings > Prompts, after a later sync found a conflict (first on the page,
amber):
| [!] 2 prompt conflicts                        [Review conflicts] |
|     @house-rules, @late-rule share their names with files in the |
|     prompt repository. The repository versions are not used ...  |
  The Prompts settings row, the sidebar Settings button, and the phone
  navigation Settings row show an amber dot until it is resolved. Review
  opens the same list with [Cancel] [Apply choices]. On phones the two
  choices stack and each is at least 44px tall.

Remove sync, when prompts have both versions (synced with a kept local
version, or overrides): "Choose which versions to keep" uses the same compact
rows, each starting on the version in use, then [Back] [Remove].
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
| | @principles [Synced] [o] | |
| | Always prefer the ...    | |
| +--------------------------+ |
| | @my-notes     [edit][del]| |
| +--------------------------+ |
+------------------------------+
  On phones the actions wrap below the status line, and the dialog follows the
  existing Workflow Sync dialog's mobile presentation, which is a full-height
  sheet with stacked fields. Tapping the badge shows the source path, and the
  touch-sized [o] (view) toggles the full read-only content.
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
- E2E: `apps/web/e2e/tests/settings/prompt-sync.spec.ts` and
  `prompt-sync-conflicts.spec.ts` (chromium) and `mobile-prompt-sync.spec.ts`
  (mobile-chrome) against the mock GitLab provider (the same fixture
  `workflow-sync-gitlab.spec.ts` uses).
- Specs: `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all`.
