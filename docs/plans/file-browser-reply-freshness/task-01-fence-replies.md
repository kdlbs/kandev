---
id: "01-fence-replies"
title: "Fence obsolete Files replies"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
  - ../../specs/ui/system-design/file-browser-reply-freshness.md
---

# Task 01: Fence Obsolete Files Replies

## Summary

Add permanent deferred regressions for Files search and file-watch refreshes,
confirm RED on current production logic, then implement owner/intent fencing
and per-folder publication ordering. Preserve existing composition and retained
tree behavior in one focused PR.

## In scope

- `useFileBrowserSearch` query/session/context generations and debounce cleanup.
- `useFileChangeSubscription` per-owner/path ordering, including a partially
  superseded batch and retirement before pending publication.
- Requested-directory read-error propagation at `currentDepth == 0`, retaining
  the existing HTTP/WS error shape and descendant partial-tree behavior.
- Privileged-safe directory-disappearance and HTTP-client regressions.
- Authoritative direct children with loaded descendant/sibling preservation.
- Production-hook tests for query reversal, clear/close, pending debounce,
  session/context changes, A-to-B-to-A, unmount, stale success/error/finally,
  current error, reversed create/delete, sibling independence, queued updater
  retirement, and mixed current/stale folder replies.

## Out of scope

Backend changes outside requested-directory read errors, tree-loader scheduling,
general caches, layout, new UI copy, and
unrelated concurrent fix scopes. Do not operate on a live instance or delegate.

## Acceptance

1. Only the current search intent/owner can alter results or searching state.
   Clear and close restore tree mode; obsolete replies and timers cannot
   repopulate it or settle a newer request.
2. For every refreshed path, only its latest active-owner request can publish.
   A newer root deletion cannot be undone by an older root reply. Independent
   siblings and current paths in mixed batches still publish; retirement
   changes neither tree nor load state.
3. Refresh merges use the latest tree and preserve loaded descendants beneath
   depth-one placeholders; authoritative empty direct children clear rows.
   Failed requested-directory reads reject through existing transport errors,
   preserve the loaded folder, and allow current siblings to publish. Descendant
   partial trees and depth/path semantics remain unchanged.
   Existing desktop/touch composition and tests remain valid.

## Verification

Dependency installation was completed in the design turn. In a fresh checkout,
first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process -run "^(TestFileTree|TestGetFileTree_)" -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agent/runtime/agentctl ./internal/agent/handlers -run "^(TestRequestFileTree|TestWorkspaceFileHandlers.*Error)" -count=1)
(cd apps/web && pnpm exec vitest run components/task/file-browser-refresh-freshness.test.ts components/task/file-browser-apply-changes.test.ts)
(cd apps/web && pnpm exec vitest run components/task/file-browser-search-freshness.test.ts components/task/file-browser-refresh-freshness.test.ts components/task/file-browser-apply-changes.test.ts components/task/file-browser-search-context-action.test.tsx components/task/file-browser-restore-loader.test.tsx components/task/file-browser-tree-state.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/file-browser-hooks.ts components/task/file-browser-data.ts components/task/file-browser-search.ts components/task/file-browser-refresh.ts components/task/file-browser-search-freshness.test.ts components/task/file-browser-refresh-freshness.test.ts)
(cd apps/web && pnpm exec prettier --check components/task/file-browser-hooks.ts components/task/file-browser-data.ts components/task/file-browser-search.ts components/task/file-browser-refresh.ts components/task/file-browser-search-freshness.test.ts components/task/file-browser-refresh-freshness.test.ts)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --check -- docs/plans/file-browser-reply-freshness
git status --short -- docs/plans/file-browser-reply-freshness
(cd apps && pnpm exec node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
process.chdir('..');
const { validateCoverage } = require(process.cwd() + '/.github/scripts/pr-docs.cjs');
const changed = execFileSync('git', ['diff', '--name-only', 'origin/main'], { encoding: 'utf8' });
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' });
const changedFiles = [...new Set((changed + untracked).trim().split('\n').filter(Boolean))].map(filename => ({ filename, status: 'modified' }));
const paths = [
  'docs/plans/file-browser-reply-freshness/plan.md',
  'docs/plans/file-browser-reply-freshness/task-01-fence-replies.md',
  'docs/specs/ui/requirements/task-navigation-responsiveness.md',
  'docs/specs/ui/system-design/task-navigation-responsiveness.md',
  'docs/specs/ui/system-design/file-browser-reply-freshness.md',
];
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE
)
```

Include extracted helper files in lint/format and their tests in the targeted
Vitest command if extraction is needed. The final Node command runs PR
documentation coverage against the actual changed-file set before publication.
Keep the repository's configured worker budget; do not overlap broad suites.
No new mobile E2E is required under the shared state-only exception recorded
in the [plan](plan.md#e2e-tests-and-mobile-parity).

## Files likely touched

- `apps/web/components/task/file-browser-hooks.ts`
- `apps/web/components/task/file-browser-data.ts`
- `apps/web/components/task/file-browser-search-freshness.test.ts`
- `apps/web/components/task/file-browser-refresh-freshness.test.ts`
- `apps/web/components/task/file-browser-search.ts`
- `apps/web/components/task/file-browser-refresh.ts`
- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_tree_read_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client_tree_read_test.go`
- This work order, `plan.md`, and the unique design supplement.

## Dependencies

None. Read the refreshed main and this design package before implementation.

## Risks

Late finally/error state, delayed functional setters, whole-batch invalidation,
and authoritative-empty versus descendant-placeholder semantics. Permanent
tests must resolve the actual transport promises and inspect production state.

## Parallelism

`sequential`

## Inputs

- [Navigation requirement](../../specs/ui/requirements/task-navigation-responsiveness.md), `.3`-`.6`.
- [Navigation design](../../specs/ui/system-design/task-navigation-responsiveness.md).
- [Reply freshness design](../../specs/ui/system-design/file-browser-reply-freshness.md).
- Existing apply-changes, tree-loader owner, and desktop/touch context-action tests.
- Temporary four-case RED evidence and 13 passing existing cases in the plan.

## Results

Implemented after the parent reviewed the package and supplied the explicit
later implementation instruction. Permanent RED ran against baseline production
logic: 16 expected ownership/subtree assertions failed; two session-retirement
fixtures also waited on the existing empty-root retry behavior and were corrected
to use a non-empty replacement root. Seven baseline controls passed. The original
four-case investigation independently established each approved race.

Final GREEN and local checks:

- The exact six-file Vitest command above: **65 passed**, including **29 new**
  search/refresh tests. The tests use real production hooks and file-change
  subscriptions with deferred WebSocket API promises. Queued tree/load-state
  callbacks, context reset, binding retirement, mixed siblings, loaded
  descendants, parent deletion, and bounded token release are covered.
- `pnpm run typecheck`: **passed**, including its generated release-note and
  changelog presteps. A test-helper props type found during validation was
  narrowed to the result it consumes, then the exact check passed.
- The six changed source/test files passed ESLint with **zero warnings** and
  Prettier. The search owner holds only generation/timer state, keeping the hook
  under the function limit without extra infrastructure.
- `pnpm run i18n:ratchet` after staging: **passed**, two added source files and
  two modified source files clean; guard allowlist intact (643 entries).
- `python3 scripts/list-docs.py validate`: **passed**, 339 decisions and 1282
  specifications. `python3 scripts/lint-spec-files.py --all`: **passed**.
- `git diff --check`, staged diff check, plan diff check, and plan status
  inspection: **passed**; both work orders/package paths are included.
- The Node PR documentation coverage command above evaluated the actual
  changed-file set: **covered**, no errors.

Search uses the established `FileTreeCacheBinding` and context reset key.
Success/error/finally updates check intent both before dispatch and within
functional state updates. Refresh tickets remain available until tree and
load-state updaters have run and the hook commits, then release their path
history. Mixed batches accept each current folder independently and use the
existing authoritative-folder merge to preserve descendant placeholders.

No layout, markup, touch, scroll, navigation, or localization copy changed.
The shared state-only mobile exception applies; existing desktop/touch component
checks pass. Public docs describe the restored behavior and need no update.
Screenshots would show the same composition and are not required for this
internal async-state correction. No additional workers or live data operations.

Local implementation is done. Exact-head CI/review disposition and authorized
normal merge are tracked externally and remain pending until merged. Report
PR URL and merged SHA to parent `14825981-b175-411d-999a-31ddc2aa5fc3`.

## Approved read-error remediation results

Parent explicitly approved the bounded backend extension and released this task
as sole heavy-command owner after #4141 merged. Permanent RED ran
`GOMAXPROCS=2 go test -p 2 ./internal/agentctl/server/process -run
"^TestFileTree(Requested|Descendant|Empty)" -count=1`: requested depths 0, 1,
and 2 incorrectly returned a successful directory after it disappeared. This
fixture works under privileged execution without permission-bit assumptions.

The three targeted remediation commands added above pass: producer and existing
path/symlink/depth controls under the race detector; serialized HTTP failure
versus omitted-children success and existing WebSocket error mapping; **23**
refresh/apply tests, including the new failed-read/current-sibling/genuine-empty
sequence. Only requested-directory read failures now return a wrapped error.
Descendant failures still yield placeholders. No layout or response-schema
change, new error copy, fixture/test expectation edits, or broad local replay.

Current-main synthetic merge `5a4c21b294c263111e69c0c42ada0451824b8c5e`
against main `68542f03983a56b9c9c42fd1afed10842e1beff0` is conflict-free.
The three Go packages pass their focused race checks; **46** affected Files
Vitest tests and typecheck pass. Backend changed-scope lint on that merge with
`--new-from-rev=68542f03983a56b9c9c42fd1afed10842e1beff0 --concurrency=2`
reports **0 issues**. Normal hook validation is recorded externally before
publication. Historical E2E failures are superseded
by main fixture and Git-status corrections; fresh hosted CI remains the remote
E2E gate. No late results-only commit or body edit will restart published gates.
