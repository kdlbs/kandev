---
id: "01-keep-creation-available"
title: "Keep repository creation available"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CREATE-LOCAL-REPOSITORY-001
acceptance_criteria:
  - AC-WORKSPACES-CREATE-LOCAL-REPOSITORY-001.1
  - AC-WORKSPACES-CREATE-LOCAL-REPOSITORY-001.7
  - AC-WORKSPACES-CREATE-LOCAL-REPOSITORY-001.8
system_design:
  - ../../specs/workspaces/system-design/create-local-repository.md
---

# Task 01: Keep repository creation available

## Summary

Expose Refresh and Create in each editable local picker and complete creation into that row.
Preserve multi-row executor selection so the task remains executable.

## In scope

- Creation and refresh callback visibility, explicit form context, target-row/cache updates,
  and conditional executor handling described by the system design.
- Component RED test followed by focused implementation and desktop/mobile E2E.
- Public how-to wording, companion-plan follow-up links, and accurate results.

## Out of scope

Remote-host creation, locked/edit selectors, Quick Chat,
backend/API changes, and changing the single-row executor policy.

## Acceptance

1. In opted-in editable local pickers, Refresh and Create remain available in every
   row with zero or multiple options and with search matches or no matches (.1).
   Refresh updates options without changing selection and stays visible but disabled
   while loading. Cover activation and loading on desktop and phone.
2. Success selects only the originating row, preserves siblings and multi-row
   executor/profile state, and works without a direct-local profile (.7, .8).
3. Desktop and phone create into a second row and submit using Worktree, while
   existing single-row, conflict, and caller-exclusion behavior remains covered.

## ASCII UI preview

UI-01 and UI-02, excerpt from the [full plan](plan.md#ascii-ui-preview):

```text
Picker: [Search repositories...] [Refresh] [Create +]
        [Existing repository            v]

Phone:  [Create new repository     Close]
        [Name / parent / target path   ]
        [Directories: internal scroll  ]
        [        Create repository     ]
```

Both actions stay outside filtered options (.1). Desktop opens Dialog; phone opens
the existing safe-area-aware Drawer with 44px touch targets. Dismissal returns
focus to the originating row. Refresh precedes Create in every local row.

## Verification

Run from the repository root. Install once before the first pnpm command in this
worktree. Run the component RED assertion before production edits, then all checks
after implementation. Managed E2E builds fresh assets and tears down its instance.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task-create-dialog-workspace-repo-chips.test.tsx components/task-create-dialog-handlers.test.ts components/task-create-dialog-prop-builders.test.ts components/create-local-repository-surface.test.tsx components/task-create-dialog-pill.test.tsx components/branch-refresh-button.test.tsx hooks/domains/workspace/use-repositories.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-new-local-repository.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-new-local-repository.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/repository-creation-availability
```

Add any changed additional test file to the focused command before marking done.
Record actual command results and E2E scenario counts, including RED evidence.

## Files likely touched

- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx` and its test.
- `apps/web/components/task-create-dialog-repo-chips.tsx`.
- `apps/web/components/task-create-dialog-prop-builders.ts` and its test.
- `apps/web/components/task-create-dialog-types.ts`.
- `apps/web/components/task-create-dialog-handlers.ts` and its test.
- `apps/web/components/create-local-repository-surface.tsx` and its test.
- `apps/web/components/task-create-dialog-pill.tsx` and its test (action sizing).
- `apps/web/components/branch-refresh-button.tsx` and its test (matching toolbar sizing).
- The two E2E files named in Verification; a shared helper if needed.
- `apps/web/hooks/domains/workspace/use-repositories.ts` and its test: remove the
  premature cached-data loading reset identified by the desktop regression.
- `apps/web/src/locales/*/common.json` only if existing copy cannot express the context.
- `docs/public/use-kandev.md`, `docs/public/tasks-and-workflows.md`.
- `docs/plans/create-local-repository/plan.md`, `task-02-task-create-selector.md`.
- This package and its linked requirement/design for results and status.

## Dependencies

None. The backend already returns a repository with a real initial `main` commit.

## Risks

Do not pass a multi-row draft through the direct-local executor setters. Do not
overwrite sibling branch/policy choices. Re-check the target key before applying
an asynchronous result; a removed row must not redirect the new selection.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/create-local-repository.md).
- [System design](../../specs/workspaces/system-design/create-local-repository.md).
- Existing selector tests, handler tests, and desktop/mobile creation E2E.
- `/tdd`, `/mobile-parity`, `/e2e`, and `apps/web/AGENTS.md`.

## Results

Completed on 2026-09-10.

- Dependency installation passed with the frozen lockfile.
- The seven-file Vitest command above passed: 108 tests.
- `pnpm run typecheck` passed after the final production changes.
- Targeted ESLint passed with no warnings on changed production files and the E2E helper.
- `pnpm run i18n:check` passed. Existing unused-key warnings remain informational.
- `node --test scripts/validate-public-docs.test.mjs` passed: 62 tests.
- `node scripts/validate-public-docs.mjs` passed: 46 pages.
- `python3 scripts/lint-spec-files.test.py` passed: 36 tests.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.
- The status check confirmed the new work order and plan. Files remain uncommitted.

RED evidence: the second-row toolbar test failed on the missing Refresh button;
creation tests failed without a direct-local profile; the deferred refresh test
proved premature loading reset; the sizing test detected the oversized desktop
Refresh control. Each corresponding focused test passed after its correction.

The initial full managed build was interrupted during unrelated cross-platform
compilation under host memory pressure. A host-only build then encountered a full
shared `/tmp` volume. An owned temporary directory on the workspace volume resolved
the disk constraint. No shared data or other processes were removed.

The successful build and browser commands, from the repository root, were:

```bash
TMPDIR=/root/kandev-repository-picker.IKmBOS GOTMPDIR=/root/kandev-repository-picker.IKmBOS GOMAXPROCS=2 GOFLAGS="-p=2" make -C apps/backend build-dev e2e-plugin-package
(cd apps && pnpm --filter @kandev/web build:e2e)
(cd apps/web && TMPDIR=/root/kandev-repository-picker.IKmBOS GOMAXPROCS=2 pnpm e2e:run --host --no-build --project chromium tests/task/create-task-new-local-repository.spec.ts)
(cd apps/web && TMPDIR=/root/kandev-repository-picker.IKmBOS GOMAXPROCS=2 pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-create-task-new-local-repository.spec.ts -- --timeout=180000)
```

The temporary path above records this run. Create a new owned directory on a volume
with available space before repeating those environment overrides.

Final desktop result: 3 passed (53.9s). Final phone result: 2 passed (31.9s).
The phone retry increased only the total test budget after the busy host exhausted
the default 60 seconds. Assertion and causal-wait timeouts were unchanged.
The frontend was rebuilt after each production correction; `--no-build` reused
those fresh assets and the verified host backend. Isolated runtimes were torn down,
and the owned temporary compile/Playwright caches were removed after verification.
