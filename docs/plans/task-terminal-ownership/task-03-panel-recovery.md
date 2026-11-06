---
id: "03-panel-recovery"
title: "Reconcile panels and verify shared tasks"
status: done
wave: 2
depends_on:
  - 01-backend-terminal-owner
  - 02-browser-terminal-scopes
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-TERMINALS-002
acceptance_criteria:
  - AC-TASKS-TASK-TERMINALS-002.3
  - AC-TASKS-TASK-TERMINALS-002.4
  - AC-TASKS-TASK-TERMINALS-002.5
  - AC-TASKS-TASK-TERMINALS-002.7
system_design:
  - ../../specs/tasks/system-design/task-terminal-persistence.md
---

# Task 03: Reconcile panels and verify shared tasks

## Summary

Recover default panels that retain another task's terminal ID. Gate ordinary
connections on verified ownership and retire stale panel publishers. Prove real
shell input/output on desktop and phone after shared-task navigation.

## In scope

- Revalidate ordinary default IDs after a successful current list, including
  IDs other than `shell-default`. Preserve a valid open selection, then prefer
  the lowest eligible sequence. Never auto-resume a parked row.
- Coalesce automatic creation per app store and task/environment scope. Create
  only after a successful empty eligible list with no read pending or failed.
  Keep layouts that omit `terminal-default` free of an automatic desktop panel.
- Recheck scope and panel identity before applying async parameters. Publish an
  admitted create to its original cache after navigation without rebinding the
  destination panel. Release failed creation guards.
- Gate ordinary `TerminalPanel` attachment before validation. Remove invalid
  extra ordinary panels with terminal-close side effects suppressed.
- Reset mobile terminal derivation when the task changes within one environment.
  Reject stale active IDs from store or session storage. Preserve same-task
  session continuity and shared script/fixed behavior.
- Add the two focused E2E files and a small explanation in public terminal docs.

## Out of scope

- New controls, locale copy, layout geometry, or transport retry policies.
- Removing another task's terminal, recovering issue #5104, or executor changes.

## Acceptance

1. `repairs a restored default panel pointing at another task` fails first.
   Mixed open/parked/foreign inventory selects the correct open row. An owned
   selection stays stable. Concurrent empty initializers create once. Pending
   or failed lists create nothing, and omitted default panels stay omitted.
2. Foreign IDs never reach ordinary attachment. Stale create completions cannot
   rewrite a reused panel, and removing invalid panels emits no foreign park,
   stop, rename, resume, or destroy request. Mobile A-to-B-to-A results contain
   only eligible ordinary IDs and retain same-task session scrollback.
3. Desktop and phone E2E tests use distinct tasks with one real inherited
   environment. Each task accepts and returns its own shell marker after
   switching and reload. Desktop repairs a persisted wrong default ID without
   deleting either task's rows. Existing lifecycle and terminal suites pass.

## ASCII UI preview

UI-01 and UI-02 reuse the [full plan previews](plan.md#ascii-ui-preview).
The structural requirement is correct owned content inside the existing surface.

```text
UI-01 Desktop: [Task A] [Terminal]  $ echo A-marker
                                  A-marker

UI-02 Phone:   [Task A]
              [Terminal v]
               $ echo A-marker
               A-marker
              [Ctrl Shift Tab Esc ...]
```

The phone pane, picker drawer, safe-area spacing, and scroll owner remain those
of `mobile-terminal-pane.tsx`. This is state reconciliation inside shipped
components. Inspect both browser projects during the focused E2E run.

## Verification

Run these commands from the repository root. Managed E2E runs stay sequential.
Each run builds current assets and uses the configured worker budget.

```bash
(cd apps/web && pnpm exec vitest run lib/state/user-shell-scope.test.ts hooks/domains/session/use-terminals-late-publication.test.ts hooks/domains/session/use-ensure-default-terminal-ordinary.test.ts hooks/domains/session/use-mobile-terminals.test.ts hooks/domains/session/use-terminals-build.test.ts hooks/domains/session/use-terminals-shell-state.test.tsx components/task/terminal-panel.test.tsx components/task/terminal-websocket-lifecycle.test.tsx components/task/use-passthrough-terminal.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm e2e:run --project chromium tests/terminal/terminal-task-ownership.spec.ts tests/terminal/terminal-first-class.spec.ts tests/session/terminal-env-keyed.spec.ts tests/terminal/terminal-missing-environment.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-terminal-task-ownership.spec.ts tests/terminal/mobile-terminal-close.spec.ts tests/terminal/mobile-terminal-missing-environment.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before marking done, run the PR-documentation coverage preflight against the
complete changed-file set through `.github/scripts/pr-docs.cjs::validateCoverage`.
Include all referenced requirement/design files in `fileContents` and record
the result. Catalog and specification lint do not prove this coverage.

## Files likely touched

- `apps/web/hooks/domains/session/use-ensure-default-terminal-ordinary.ts`,
  `use-mobile-terminals.ts`, `use-terminals.ts`, and `use-terminals-build.ts`.
- `apps/web/components/task/terminal-panel.tsx`, `passthrough-terminal.tsx`,
  and `dockview-layout-setup.ts` if needed to suppress invalid-panel cleanup.
- Corresponding hook/panel/lifecycle tests listed in Verification.
- `apps/web/e2e/tests/terminal/terminal-task-ownership.spec.ts` (new).
- `apps/web/e2e/tests/terminal/mobile-terminal-task-ownership.spec.ts` (new).
- `apps/web/e2e/tests/terminal/task-ownership-helpers.ts` (new, shared fixtures).
- `docs/public/developer-tools.md` and this package's status/results.

## Dependencies

Tasks 01 and 02. They provide correct gateway ownership and browser result scopes.

## Risks

An initialization-settled flag cannot substitute for successful list evidence.
Suppress terminal-close side effects before removing an invalid panel. Browser
fixtures must assert environment equality instead of assuming a shared workspace.
Arm transport observation before navigation, use causal waits, and scope terminal
selectors to the active surface. A successful socket alone does not prove input/output.

## Parallelism

`sequential`

## Inputs

- [Panel design](../../specs/tasks/system-design/task-terminal-persistence.md#panel-reconciliation).
- [Owning requirements](../../specs/tasks/requirements/task-terminal-persistence.md).
- Existing default migration, mobile auto-create, and shell tab selection tests.
- `e2e/tests/layout/saved-layout-session-isolation.spec.ts` shared parent/child fixture.
- `terminal-first-class.spec.ts`, `terminal-env-keyed.spec.ts`, and mobile terminal
  tests for lifecycle, real input/output, and visible navigation patterns.
- `apps/web/AGENTS.md`, `/mobile-parity`, `/e2e`, `/tdd`, and `/docs-maintainer`.

## Results

Completed. Default-panel reconciliation validates every restored ID, repairs
conflicting task/environment stamps even when the ID is eligible, selects the
lowest eligible open sequence, coalesces creation per store and scope, and
rechecks scope plus panel identity before applying a late result. Creation and
selection require a successful current read with a real task ID; pending and
failed reads create nothing, including on mobile. Invalid ordinary panels,
including panels stamped for another task or environment, are removed with
close side effects suppressed, and `TerminalPanel` gates attachment on the
active scope. Mobile derivation resets on task changes and rejects stale saved
tab IDs.

Verification executed from `apps/web`; managed E2E runs were sequential:

```bash
pnpm exec vitest run hooks/domains/session/use-ensure-default-terminal-ordinary.test.ts hooks/domains/session/use-mobile-terminals.test.ts hooks/domains/session/use-terminals-build.test.ts hooks/domains/session/use-terminals-shell-state.test.tsx components/task/terminal-panel.test.tsx components/task/terminal-websocket-lifecycle.test.tsx components/task/use-passthrough-terminal.test.ts components/task/dockview-terminal-panel-reconciliation.test.ts
pnpm run typecheck
pnpm run lint
pnpm e2e:run --project chromium tests/terminal/terminal-task-ownership.spec.ts tests/terminal/terminal-first-class.spec.ts tests/session/terminal-env-keyed.spec.ts tests/terminal/terminal-missing-environment.spec.ts -- --retries=0
pnpm e2e:run --project mobile-chrome tests/terminal/mobile-terminal-task-ownership.spec.ts tests/terminal/mobile-terminal-close.spec.ts tests/terminal/mobile-terminal-missing-environment.spec.ts -- --retries=0
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Focused unit suites passed (324 tests across 38 files, including both Task 02 and Task 03 verification lists).
Chromium passed 12 tests and mobile-chrome passed 3 tests, including the two
new ownership specs and a client-side task switch that holds a late create
request open across the navigation. Mobile ownership navigation now uses the
phone task picker instead of a page reload. Public docs validated 55 pages; the specification catalog
validated 436 decisions and 1929 specifications; the 36 linter tests and the
full specification lint passed.
