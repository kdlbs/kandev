---
id: "01-copy-node-path"
title: "Copy node paths"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-TREE-PATH-COPY-001
acceptance_criteria:
  - AC-UI-FILE-TREE-PATH-COPY-001.1
  - AC-UI-FILE-TREE-PATH-COPY-001.2
  - AC-UI-FILE-TREE-PATH-COPY-001.3
  - AC-UI-FILE-TREE-PATH-COPY-001.4
  - AC-UI-FILE-TREE-PATH-COPY-001.5
  - AC-UI-FILE-TREE-PATH-COPY-001.6
system_design:
  - ../../specs/ui/system-design/file-tree-path-copy.md
---

# Task 01: Copy node paths

## Summary

Expose Copy path for one Files tree node or filename result through the existing desktop and touch menus. Copy the exact node identity through the unchanged shared path-copy hook and guard; retain existing row and workspace behavior.

## In scope

- Single file and folder tree actions and file search result actions, including multi-root, whitespace and Unicode identities.
- Suppress bulk copy using the actual selection count independent of optional delete callbacks. Preserve event isolation, existing actions and phone geometry.
- Add focused component and real routed desktop/phone E2E evidence, plus minimal user guidance in the existing sessions and review how-to.

## Out of scope

- Bulk copy, directory search, generalized clipboard hardening, menu refactoring, unrelated rename extraction, and changes to other PRs or issues.

## Acceptance

1. Single-node desktop and touch actions pass exact `node.path` to the established hook; multiple selected tree nodes offer no Copy path, and filename results remain single-node actions.
2. Unsafe paths use the existing refusal, exhausted clipboard failure claims no success, and action events do not activate rows or write files; existing actions continue working.
3. Isolated real-backend desktop and phone Files flows read the actual clipboard for a file, folder and search result; phone uses a visible 44px trigger and menu item and remains contained.

## UI composition

The desktop context menu and the phone's visible row overflow menu expose Copy path beside existing conditional actions for a single tree node or filename result. Multi-selection omits it. Preserve row activation, menu dismissal, phone safe-area behavior, and 44px trigger and item targets. See the [composition](plan.md#ui-composition) (AC 001.1-001.6).

## Verification

After the later explicit implementation and heavy-work grants, install once in this fresh worktree. Every `scripts/run-quiet` invocation uses a freshly created, task-owned absolute directory outside the repository; retain and join each process handle and record actual exits. Do not overlap E2E runs. Use `--summary` only with E2E. If the browser executable is missing, report the actual prerequisite and use the existing environment setup under the granted work; report it before unrelated system installs. No additional approval ceremony is needed for routine reversible dependencies.

Add focused component cases first. The RED run must fail because the real menu action is absent; a setup, dependency, or compilation failure is not RED evidence. After implementation run the same cases GREEN. Exercise exact paths, inherited unsafe refusal, exhausted clipboard fallback without a success toast, and ensure no file, row, folder, or other action callbacks fire. Do not add new copy or locale keys. Browser tests use a task-owned isolated fixture and actual `navigator.clipboard.readText()`; include a true multi-repository fixture for the prefixed path using existing repository-set patterns, not a mocked node. Inspect copied fixture tests for nonzero `describe.configure` retries and avoid them; do not add redundant file-level zero retries.

From the repository root, after the grants, set the pinned toolchain and task-owned log parent once:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local
export task_run_dir=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child037-run-quiet
mkdir -p "$task_run_dir"
(cd apps && pnpm install --frozen-lockfile)
KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task_run_dir/red.XXXXXX")" scripts/run-quiet frontend-test -- pnpm --dir apps/web exec vitest run --maxWorkers=1 --no-file-parallelism components/task/file-context-menu.test.tsx components/task/file-browser-search-context-action.test.tsx lib/utils/copy-repository-path.test.ts
```

After confirming the expected RED result, implement and run these commands sequentially:

```bash
KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task_run_dir/green.XXXXXX")" scripts/run-quiet frontend-test -- pnpm --dir apps/web exec vitest run --maxWorkers=1 --no-file-parallelism components/task/file-tree-path-copy.test.tsx components/task/file-context-menu.test.tsx components/task/file-browser-search-context-action.test.tsx components/task/file-browser-context-action.test.tsx components/task/file-browser-render-identity.test.tsx components/task/file-browser-responsive.test.tsx components/task/file-tree-editor-menu.test.tsx lib/utils/copy-repository-path.test.ts
KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task_run_dir/lint.XXXXXX")" scripts/run-quiet web-lint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/task/file-context-menu-items.tsx components/task/file-context-menu.tsx components/task/file-browser-parts.tsx components/task/file-tree-path-copy.test.tsx components/task/file-context-menu.test.tsx components/task/file-browser-search-context-action.test.tsx components/task/file-browser-context-action.test.tsx components/task/file-browser-render-identity.test.tsx e2e/tests/task/file-tree-path-copy-helpers.ts e2e/tests/task/file-tree-path-copy.spec.ts e2e/tests/task/mobile-file-tree-path-copy.spec.ts
GOMAXPROCS=2 GOFLAGS=-p=2 KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task_run_dir/desktop.XXXXXX")" scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/file-tree-path-copy.spec.ts -- --workers=1 --retries=0
GOMAXPROCS=2 GOFLAGS=-p=2 KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task_run_dir/phone.XXXXXX")" scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-file-tree-path-copy.spec.ts -- --workers=1 --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/specs/ui/requirements/file-tree-path-copy.md docs/specs/ui/system-design/file-tree-path-copy.md docs/plans/file-tree-path-copy docs/public/sessions-and-review.md
```

The ESLint command names the expected changed TS/TSX paths; adjust it to cover exactly all actual changed TS/TSX files if the implementation differs. Managed E2E builds the current backend, web assets, and fixture plugin before running; do not use freshness bypasses. Review the phone capture for hierarchy and clipping. Record each actual command/result and any limitation here; no broad suite, full typecheck, or local QA/review is required without a specific concern.

## Files likely touched

Owned production: `apps/web/components/task/file-context-menu-items.tsx`, `apps/web/components/task/file-context-menu.tsx`, `apps/web/components/task/file-browser-parts.tsx`.

Owned tests: `apps/web/components/task/file-tree-path-copy.test.tsx`, `apps/web/components/task/file-context-menu.test.tsx`, `apps/web/components/task/file-browser-search-context-action.test.tsx`, `apps/web/components/task/file-browser-context-action.test.tsx`, `apps/web/components/task/file-browser-render-identity.test.tsx`, `apps/web/e2e/tests/task/file-tree-path-copy-helpers.ts`, `apps/web/e2e/tests/task/file-tree-path-copy.spec.ts`, `apps/web/e2e/tests/task/mobile-file-tree-path-copy.spec.ts`.

Owned public documentation: `docs/public/sessions-and-review.md` (how-to). This package's four design documents are updated for status and results. Existing hook, utility, locale catalogs, and adjacent tests are inputs, not intended edits.

## Dependencies

None. Reconcile current main and overlapping concurrent edits before modifying owned files.

## Risks

A touch menu without delete still needs the real bulk state. Search display labels may differ from node identity. The inherited failed-copy path is silent. Browser clipboard permissions and multi-repository fixture readiness must be proven in the isolated environment.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/ui/requirements/file-tree-path-copy.md) and [system design](../../specs/ui/system-design/file-tree-path-copy.md).
- Existing `useCopyRepositoryPath`, `copyPathToClipboard`, Files menu and search components, and file-tree chat-context browser tests.

## Results

Before the pending test-only causal-wait correction, frozen install passed. The initial RED command failed on eight absent menu actions or triggers; its original tests were later moved into `file-tree-path-copy.test.tsx` to satisfy lint limits. The focused final Vitest run passed 60 tests in eight files, including the copy and adjacent row suites. Scoped ESLint passed after resolving complexity and test-size warnings. The initial phone E2E measured the menu during its opening animation; waiting for settled geometry made the phone test pass. Desktop and phone managed E2E capture runs each passed one test with retries disabled, real backend and workspace, and actual clipboard reads. Screenshot assets were inspected at both viewports. Specification catalog validation and full spec lint passed. Public-doc validator tests passed 63 tests, the public-doc validator passed 54 pages, and the documentation diff check passed. Those checks describe the prior head. The review correction established RED on both rows lacking tab stops, then added visible keyboard focus and passed the focused and adjacent Vitest suites (24 tests across four files). Both real-backend E2E capture runs passed one test each with one worker and no retries: desktop exercised Tab, Shift+F10, menu selection and clipboard copying for tree and search rows; both platforms awaited the matching search response and used distinct clipboard sentinels. Scoped ESLint passed. The prior menu close is awaited before reopening it so its focus restoration completes. Final desktop and phone captures, including desktop keyboard focus, were inspected. Hosted PR validation remains pending.
