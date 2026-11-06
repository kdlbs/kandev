---
created: 2026-10-18
status: implemented
requirements:
  - REQ-UI-PROVIDER-WATCHER-TYPES-001
system_design:
  - ../../specs/ui/system-design/provider-watcher-task-types.md
legacy_specs: []
---

# Implementation Plan: Provider Watcher Task Types

## Overview

Correct the shared issue-watch/review-watch categories across persisted Sidebar
queries, complete local Sidebar evaluation, HTTP/WS/initial task mapping, and
Threads saved-view filtering. One sequential work order owns this complete
outcome so the server and browser do not ship different recognition rules.

Design only at creation. Leave this package uncommitted for ROOT review and
await a later explicit implementation interrupt to the same primary session.
No delegates, new persistent tasks/sessions, or model changes are authorized.

## Evidence and ownership

Qualified ROOT receipts at
`/tmp/kandev-root-watch-type-discovery-qf30nvvl/qualified-proof.json`,
`receipt.json`, `cases.json`, `sqlite.stdout`, `sqlite.stderr`,
`threads.stdout`, and `threads.stderr` establish two GitLab causal omissions
in each actual consumer path, with three passing GitHub/ordinary controls on
each side. SQLite returned zero tasks/total zero where one was expected;
mapped Threads returned no matching task for each expected category.
These are inherited observations, not child-executed tests. Never inspect,
copy, replay, or delete the private candidate sources or `run.py`.

Qualification base: `a8e982cffbb10705db5b5e03a97a4229fa67b972`.
Admitted child HEAD and actual remote main:
`d0e53c9a6ff813ba1e7f70e84a5b21eaf85260b3`; all eleven qualified dependency
blobs were rechecked unchanged. Provider writers were inspected read-only;
no provider, browser, PostgreSQL, or `BuildTaskRequest` execution is claimed.

Root cause: SQL and each frontend recognition path know only the generic
GitHub keys; existing first-party providers write their own watch identifiers.
Local Sidebar also duplicates a value predicate that differs from task mapping.
UI owns the shared saved-view classification outcome; Tasks and Integrations
retain their existing metadata/lifecycle and writer contracts.

## Scope

### In scope

- Nine existing first-party metadata keys, two categories, and shared frontend
  helpers with corresponding dialect-aware SQL recognition.
- Consistent watcher-value semantics, including the explicitly bounded malformed
  value compatibility correction described in the design.
- Permanent independent consumer regressions and focused task-defined checks.

### Out of scope

- Provider requests, watcher dispatch/intake/scheduling/deduplication, new
  providers, suffix inference, plugin registration, or metadata mutation/migration.
- API/auth changes, query architecture/pagination changes, cache/freshness work,
  layout/copy changes, and unrelated metadata normalization.
- Real external database access, full-backend lint, broad review/QA/verification,
  and replay of ROOT's private reproductions.

## Technical approach

The paired [design](../../specs/ui/system-design/provider-watcher-task-types.md)
owns the complete key matrix and value truth table. Extend existing neutral
`metadata-utils.ts` helpers and call them from local Sidebar projection.
Extend SQL `sidebarWatchFields`/`sidebarWatchField` beside the owning query,
reusing dialect JSON type/extract helpers. Existing task mappers, Sidebar
`applyFilters`, and Threads query already consume the correct booleans.
Retain their enum values, aliases, negative operators, and review-first priority.

| Provider/source shape | Transport into classification | Expected category | Evidence planned | Unsupported shape fallback |
| --- | --- | --- | --- | --- |
| GitHub issue/review IDs | Persisted JSON; HTTP/WS/initial metadata | Existing issue/review categories | SQL + mapped Threads + initial/local consumers | Unusable ID gives no flag |
| GitLab issue/review IDs | Same existing metadata paths | Issue/review respectively | Same consumers, causal RED/GREEN | Unusable ID gives no flag |
| Jira/Linear/Sentry issue IDs | Same existing metadata paths | Issue-watch only | All supported keys in SQL and frontend cases | No invented review keys |
| Azure DevOps work-item/PR IDs | Same existing metadata paths | Issue/review respectively | Both keys in SQL and frontend cases | Unusable ID gives no flag |
| Manual links or unknown keys | Existing task/link metadata | Ordinary | Negative consumer controls | No suffix/provider-name inference |

Support here means classification of existing metadata, not a claim that any
provider connection, poller, or writer was executed.

## Tests

All references below use `AC-UI-PROVIDER-WATCHER-TYPES-001`.

| Criteria | Permanent evidence |
| --- | --- |
| .1-.6 | New `sidebar_task_query_watch_test.go::TestQuerySidebarTaskPageWatcherTaskTypes`: actual task persistence/query IDs and totals, each key, negatives, invalid/mixed/both values |
| .1-.6 | Same file `TestSidebarWatchFieldsDialects`: actual generated projection for SQLite/PostgreSQL, type/extract syntax, aliases and lazy requested fields |
| .1-.4 | Extend `lib/metadata-utils.test.ts`: explicit key/value matrix and unknown/manual negatives |
| .1-.5 | Extend `lib/kanban/map-task.test.ts` and `lib/ssr/mapper.test.ts`: HTTP/WS and initial mapper outcomes |
| .1-.7 | New `lib/threads/thread-view-watch-types.test.ts`: actual mapped tasks into `queryThreadView`, task IDs/counts, standard/issue/review and negative clauses |
| .1-.7 | New `lib/sidebar/sidebar-watch-types.test.ts`: actual local projector/page plus mapped `applyFilters`; positive/negative membership and local totals |

Expected values must be explicit and independent of production key lists.
Consumer tests are required even when helper tests pass. Existing eligibility
fixtures must provide a primary session for Threads. Tests use temporary local
databases and no provider calls. PostgreSQL behavioral subtests can use
`newRepoForSidebarConformance` and remain skipped locally without a DSN.

## E2E tests and mobile parity

The complete in-process paths above supply the end-to-end data evidence for
.1-.7. No browser/E2E run is planned or claimed: the correction changes metadata
classification only, not rendered layout, touch interaction, scrolling,
navigation, or viewport-dependent behavior. Desktop and phone use the same
corrected data under the mobile-parity data-normalization exception. No ASCII
layout preview applies. If implementation adds rendered interaction, stop and
revise this scope and its verification before doing that work.

## Work orders

- [x] [Task 01: Recognize provider watcher task types consistently](task-01-recognize-provider-watchers.md)

Task 01 is sequential with no dependency or delegation wave.

## Verification results

Design checks passed on 2026-10-18:

- `python3 scripts/list-docs.py validate`: 386 decisions and 1,635 specifications
  validated; both new specifications appear in the UI catalog.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check` and a direct whitespace/EOF check of all four untracked
  documents: passed. `git diff --no-index --check /dev/null` initially returned
  the expected new-file difference status 1 without whitespace diagnostics;
  the direct check establishes whitespace independently of that status.
- Reference check: one requirement, seven existing acceptance IDs, one declared
  design and one linked sequential order; referenced document paths exist.
- Actual four-doc `validateCoverage` preflight: `exempt`, `ok: true`,
  `errors: []`. No synthetic production triggering paths were supplied.

The actual coverage paths are:

- `docs/specs/ui/requirements/provider-watcher-task-types.md`
- `docs/specs/ui/system-design/provider-watcher-task-types.md`
- `docs/plans/provider-watcher-task-types/plan.md`
- `docs/plans/provider-watcher-task-types/task-01-recognize-provider-watchers.md`

Joined native results: catalog `f48393` exit 0; all-spec lint `c54bc7` exit 0;
whitespace/reference `d88f01` exit 0; document-only check `56c742` exit 0;
final actual four-doc coverage `e3e6ce` exit 0. All commands completed
synchronously. No task-owned background processes were launched, and no native
handles or owned process groups remain outstanding. Receipt:
`/tmp/kandev-child138-design-k2xhaabq/receipt.json` (before final PATH/ratchet
documentation corrections). Global heavy lease remains unclaimed.

Final ROOT-requested design corrections passed: native join `29485c` exit 0
for catalog/all-spec/`git diff --check`; native join `be4fd5` exit 0 for the
actual four paths above, direct whitespace, `exempt`/`errors: []`, and an empty
staging area. The order now places the actual pnpm directory before Node,
requires an explicit staged i18n ratchet, and confirms the existing frontend
conformance path and PostgreSQL DSN skip guard. These checks launched no
background groups and left no outstanding native handles.

Implementation checks passed after ROOT's later explicit release:

- Independent actual SQL and mapped Threads regressions established causal
  RED with passing GitHub/ordinary controls, then GREEN for the correction.
- Corrected sqlite3/pgx expression checks and actual SQLite consumer tests
  passed. The complete owning-package race rerun passed in 706.596s with
  `-timeout=15m`; its known process group is absent. The original implicit
  ten-minute timeout and test-driver fixture correction remain recorded.
- Scoped Go lint passed with zero issues. Six frontend suites passed, 264
  tests. After a test-only bounded-item type correction, the changed Sidebar
  suite passed again, 22 tests, and its scoped lint passed.
- Full web typecheck passed with the process-local 8 GiB heap. Its initial
  failure was a new test's TypeScript shape mismatch, not an OOM; production
  types and repository memory configuration were unchanged.
- Actual implementation coverage is `covered`, `errors: []`, with three
  production triggers and exactly one owning work order. Full command/log/join
  evidence and the staged i18n check are in Task 01.

Requirements are active, design current, Task 01 done. No provider, browser,
or live PostgreSQL execution is claimed. The mobile data-only exception and
public-doc impact remain as described above. Normal publication hooks remain
required; no separate broad verification or review gate was added.

Actual docs-only changed-path coverage may return `exempt` with `errors: []`.
Do not invent a prospective production path to force coverage. After authorized
implementation, run the same preflight with actual changed paths and require
`covered` with `errors: []`.

## Documentation impact

Internal requirements/design/planning only at this checkpoint. Existing public
integration docs explain watch setup rather than these task-type categories;
no public API, configuration, labels, or workflow steps change. Recheck actual
implementation scope through docs-maintainer before publication; no speculative
public page is part of this package.

## Risks

- Omitting local Sidebar or initial mapping would keep source-dependent results;
  each actual consumer has its own regression boundary.
- Malformed recognized watcher values intentionally become consistent; preserve
  valid GitHub IDs and test sentinel/non-string cases explicitly.
- PostgreSQL local evidence is generated SQL only; do not describe skipped
  DSN-gated behavior tests as live database coverage.
