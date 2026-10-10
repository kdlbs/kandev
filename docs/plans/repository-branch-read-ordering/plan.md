---
created: 2026-10-02
status: in_progress
requirements:
  - REQ-WORKSPACES-BRANCH-READS-001
system_design:
  - ../../specs/workspaces/system-design/repository-branch-reads.md
legacy_specs: []
---

# Implementation Plan: Repository Branch Read Ordering

## Overview

Protect accepted branch data and loading ownership with one shared request owner
per actual store/source key. Deliver as one sequential TDD work order after a
later explicit implementation continuation. The parent reviewed all four
package documents and authorized implementation and delivery on 2026-10-02.
Local implementation evidence is recorded here; current-head hosted review,
merge, and cleanup receipts remain in the live task plan and PR.

## Evidence and root cause

At base `69fa561795f52ee69ef15513e9f55c3c65ddc3f7`, both initial loads and
refreshes unconditionally publish branches and clear loading in `finally`.
The production `setRepositoryBranches` action also clears loading. Initial
requests have only a per-hook in-flight set, so neither completion path fences
newer requests from the same or sibling hooks.

Accept parent-provided actual-hook/production-store evidence: deferred network
responses reproduced initial-versus-refresh stale overwrite, reverse-order
refresh overwrite, and older refresh prematurely clearing newer loading. The
ordinary initial-load control passed. Parent run 89015 was joined with exit 1
(three failed, one passed; 104 ms tests, 3.78 s total, one worker). Its temporary
source was removed; `/tmp/kandev-branch-refresh-freshness-repro.test.ts` remains
parent-owned until parent verification of actual merge. Do not replay this proof
merely to acknowledge it.

## Scope

### In scope

- Store/source ownership for initial reads and explicit refreshes, including
  data acceptance, loading release, terminal failure, and cleanup.
- Permanent real-hook/production-store deferred regression tests and a rendered
  shared branch-list integration control.
- Exact targeted checks, documentation traceability, ready PR, current-head
  hosted CI/semantic review, normal merge, and owned cleanup after continuation.

### Out of scope

- Backend, public API/schema, Git commands, branch filtering/selection/policies,
  consumer layouts, authentication/environment/active-selection semantics.
- New dependency, generic fetching abstraction, polling, retry redesign, extra
  workers/tasks/sessions, broad audits/suites, optional polish, or browser E2E.

## Technical approach

Implement the [request ownership design](../../specs/workspaces/system-design/repository-branch-reads.md#request-ownership)
in `apps/web/hooks/domains/workspace/use-repository-branches.ts`, using the actual
store API and a WeakMap of per-source unique owner tokens. Guard the success
setter as well as `finally`. Replace the current mocked-store hook suite with a
`.test.tsx` suite using the real provider/store. Workspace slice production
types, setters, and all consumers should require no changes.

Compatibility matrix and exact source/API/cache contracts are in the design's
[source identity](../../specs/workspaces/system-design/repository-branch-reads.md#source-identity-and-compatibility)
section. Follow the nearest store-scoped coordination pattern without importing
Office or sidebar lifecycle rules.

## Tests

Permanent suite: `apps/web/hooks/domains/workspace/use-repository-branches.test.tsx`.

| Acceptance | Required scenarios |
| --- | --- |
| AC-WORKSPACES-BRANCH-READS-001.1 | `keeps refresh results after an older initial load`; `keeps the newest of two refreshes`; sibling initial/refresh and sibling refresh/refresh completion order |
| AC-WORKSPACES-BRANCH-READS-001.2 | `keeps loading while a newer refresh is pending`; older success and rejection; newest completion before older settles |
| AC-WORKSPACES-BRANCH-READS-001.3 | newest refresh failure preserving prior list/loaded; newest failure then older success with and without a prior accepted list; next refresh succeeds; terminal initial failure unloaded |
| AC-WORKSPACES-BRANCH-READS-001.4 | ordinary initial and refresh success; empty initial list; empty refresh then late obsolete success |
| AC-WORKSPACES-BRANCH-READS-001.5 | two repository IDs; two paths; same path in two workspaces; id versus path source; same key in independent real stores; completion affects only its own loading |
| AC-WORKSPACES-BRANCH-READS-001.6 | null/disabled/cached source; enable/open; correct id/path API arguments; transient success and exhausted five-attempt retry loop with existing delays; no continuous retries |
| AC-WORKSPACES-BRANCH-READS-001.7 | source switch while pending; initiating consumer unmount with sibling still mounted; remount while pending and after success/failure; settled ownership permits a new request |
| AC-WORKSPACES-BRANCH-READS-001.8 | actual hook/store rendered through `BranchPickerList` shows newest options and correct loading; existing component selection/filter/touch regressions |

Mock only branch API/network boundaries in the hook integration suite. Keep
React, Zustand, provider, store, and workspace setters real. Use deferred
promises rather than sleeps; join/settle every promise and restore fake timers.

## Mobile parity and E2E assessment

This is state/data normalization in existing selectors, with no composition,
touch, scroll, navigation, breakpoint, or copy change. The explicit narrow
exception in `.agents/skills/mobile-parity/SKILL.md` permits targeted unit and
component evidence. `BranchPickerList` already serves desktop and phone;
`watcher-repository-fields.test.tsx` also checks touch controls. No ASCII layout
preview or new Playwright scenario is necessary. No app/browser/build discovery.

## Documentation assessment

Internal docs updated: the owning requirement/design pair and this delivery
package. `docs/public/git-operations.md` already documents branch search and
refresh; the fix restores that behavior without changing user steps, labels,
configuration, APIs, or screenshots. Public docs, README, screenshot catalog,
and locale catalogs need no changes. Reassess if implementation changes scope.
No root/scoped engineering guide change is needed for this local existing pattern.

## Work orders

- [x] [Task 01: Fence shared branch reads](task-01-fence-shared-branch-reads.md)

## Verification results

Design validation passed on 2026-10-02:

- `python3 scripts/list-docs.py validate`: 343 decisions and 1,311 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`, with the intended hook
  change and all four package documents: covered, no errors; requirement, all
  acceptance references, plan, and design links accepted.
- Tracked diff and all four untracked documents passed whitespace checks.
  At design handoff, `git status --short` showed only this four-document package.
- Owning-system catalog discovers the new requirement/design pair. At design
  handoff, no source, permanent tests, staged files, commit, or parent archive
  had been changed.

The shell initially lacked Node in PATH; preflight passed with the existing
Node 24 binary. After continuation, the missing worktree dependencies were
installed once with `pnpm install --frozen-lockfile` in `apps/` (2 seconds,
exit 0, unchanged lockfile).

Implementation verification on 2026-10-02:

- RED hook suite: 12 failed, 13 passed. Eleven failures showed stale publication,
  premature loading release, newest-failure revival, and duplicate sibling
  reads. One rendered assertion used the wrong loading punctuation; after
  correcting that assertion, its focused RED showed `old-branch` replacing
  `new-branch`. All reproduction command handles were joined with exit 1.
- First four-file GREEN run: the three existing suites passed all 21 tests.
  The hook suite passed 24 of 25 tests; its remaining assertion incorrectly
  compared separate hooks' callback identities. It was corrected to compare
  their observable data/status. No production change was needed.
- Final hook suite: 26 passed, including the required terminal-failure remount
  control, with one worker (146 ms tests, 2.98 s total). Together with the
  unchanged existing suites, all 47 targeted tests passed. Only the changed
  hook suite was rerun after test corrections.
- Changed-file eslint with `--max-warnings 0`: passed, zero warnings/errors.
  Fixture constants and a smaller describe block resolved lint warnings
  without assertion changes.
- Final `pnpm run typecheck`: passed, exit 0. Its release-note/changelog
  generation produced no tracked changes.
- `pnpm run i18n:ratchet --base 69fa561795f52ee69ef15513e9f55c3c65ddc3f7`:
  passed, modified hook clean and all 643 allowlist entries intact.
- Implementation catalog validation, 36 spec-linter tests, and full specification
  lint passed. Final staged docs coverage and whitespace checks are recorded
  before publication in the work order and task plan.

External delivery remains pending: normal hooks/commit, ready PR, required
current-head hosted CI and FULL semantic review, expected-head squash merge,
independent merged-content verification, and joined owned cleanup. The task is
incomplete until those receipts are recorded. The parent archive is untouched.

## Risks

- Guarding only `finally` still permits the production setter to clear loading.
- Cleaning an owner on hook unmount or reusing a generation after cleanup can
  make an obsolete read authoritative again.
- Adding loading to effect dependencies can create a continuous failure loop.
- Shared render snapshots can race; initial demand checks must use current store
  state and shared ownership, with ownership registered before publication.
