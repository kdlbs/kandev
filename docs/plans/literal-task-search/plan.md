---
created: 2026-10-14
status: implemented
requirements:
  - REQ-TASKS-WORKSPACE-SEARCH-001
system_design:
  - ../../specs/tasks/system-design/workspace-search.md
legacy_specs: []
---

# Implementation plan: Literal task search

## Overview

One sequential work order corrects workspace task-search COUNT and SELECT
predicates and proves the result through independently authored owning
repository tests. Production and permanent test changes begin only after a
later explicit implementation request.

## Confirmed evidence and ownership

The supplied `qualified-proof.json` and `proof.log` in
`/tmp/kandev-root-task-literal-search-20261014-bjwmtq5q` show a real
`Repository.ListTasksByWorkspace` call returning 22 matches for `deploy_` and
`deploy%`, where one literal match is expected. Twenty-one newer
`deployXproduction` distractors hide that row from page one with limit twenty.
Ordinary `production` and unique-text controls pass. The proof's test source
is private and must not be accessed, copied, or replayed.

The current baseline is `2705829f097d507225290ea3b649dd2cae9de8fb`;
`task.go` blob `185bcd5ea1bb347e576e33ab984103c000705473` matches the tested
source. Root cause: `searchTasks` wraps raw text in `%` without escaping its
punctuation, and neither COUNT nor SELECT declares an escape character.

Tasks owns the shared workspace-list search contract. Existing Tasks archive
ACs exclude matching; UI board repository matching `.4` applies to a different
client-side lens. The new small vertical pair fills the missing literal
workspace-search contract without a parallel UI repair specification.

## Scope and technical approach

- Own `searchTasks` and `taskSearchSelectQuery` in
  `apps/backend/internal/task/repository/sqlite/task.go` and new
  `task_search_literal_test.go` / `task_search_literal_postgres_test.go` beside it.
- Escape backslash, percent, and underscore once before adding substring
  wildcards, following the local `SearchMessages` precedent. Declare the same
  SQL escape character for all four arms in both predicates.
- Preserve dialect selection, all current filters, DISTINCT, sort, counts,
  argument order, and pagination. Update paired document lifecycle only after
  implementation matches the reviewed design and its checks pass.

Excluded: global or adjacent search refactors, source/path normalization,
wildcard syntax or settings, schemas/migrations, dependencies, frontend code,
layouts, locale copy, and unrelated cleanup. No ADR is needed: the requirement
and local design preserve the settled literal behavior and correction rationale.

## Tests

The independently authored tests below are present. SQLite execution passed;
actual PostgreSQL execution remains a hosted CI gate after the local DSN skip.

| Acceptance | Owning test and meaningful assertion |
| --- | --- |
| `.1`, `.2`, `.5` | `TestWorkspaceTaskSearchLiteralFields`: percent, underscore, and backslash independently in title, description, repository name, and local path; literal distractors; ASCII mixed-case queries; adjacent escaped punctuation, repeated and trailing backslashes. |
| `.3` | `TestWorkspaceTaskSearchLiteralPagination`: more than twenty newer wildcard-only distractors; one older literal row for both percent and underscore; total one and first-page inclusion; multiple literal rows paged with fixed order; empty final page retains total; multi-repository matches deduplicate. |
| `.2`, `.4` | `TestWorkspaceTaskSearchLiteralFiltersAndControls`: workspace/workflow/repository scoping, archive exclusion/inclusion/onlyArchived precedence, ephemeral inclusion/onlyEphemeral, config exclusion, and automation-origin exclusion; ordinary and unique-text controls, empty query and existing whitespace handling; sort controls. |
| `.1` through `.5` | `TestPostgresWorkspaceTaskSearchLiteral`: actual isolated PostgreSQL executes the shared behavioral matrix with `KANDEV_TEST_POSTGRES_DSN`; missing DSN explicitly skips. |

Use fixture helpers around real `NewWithDB`, production creation methods, and
disposable databases. Assertions must inspect task IDs and total counts rather
than test a replica of the SQL or escaper. Reuse a compact test harness between
SQLite and PostgreSQL where it keeps the fixtures clear.

## Consumer and mobile evidence

Audited request wiring: command-panel Tasks scope page one/twenty, mentions page
one/fifty, dependency picker hundred-row pages driven by total. They transport
the query through `listTasksByWorkspace`, the HTTP handler, and the authorized
service into the real repository. The displaced-match repository regression
uses the actual command-panel page bound.

This source-only backend correction needs no frontend/E2E change to prove the
predicate. Desktop and phone share the same data semantics; mobile-parity's
state/data exception applies. There is no rendered UI change requiring a preview.
Public docs: no public operator change; internal owning specs record the contract.

## Work orders

- [x] [Task 01: Correct literal workspace task matching](task-01-literal-predicate.md)

Dependency order: this single work order, executed sequentially.

## Verification results

Design checks completed on 2026-10-14:

- `python3 scripts/list-docs.py validate`: passed, 381 decisions and 1605 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual five owned document
  changes are exempt with no errors; prospective owned `task.go` correction is
  covered by the complete requirement/design/plan/order chain with no errors.
  The prospective source check is not evidence of an implemented change.
- Owned-path `git diff --check` and whitespace inspection of the five tracked
  and untracked Markdown files: passed. Status contains the changed Tasks
  README and the four new package documents, all unstaged and uncommitted.
- `git diff --exit-code -- apps/backend`: passed; backend files are untouched.

Implementation results on 2026-10-14 after the separate grant:

- Causal RED: real SQLite returned 22 instead of one for both `deploy_` and
  `deploy%`; exit 1, package 0.389 seconds, before production changes.
- Targeted GREEN: the reviewed first verification command passed, exit 0,
  package 11.479 seconds. It covers 36 field/punctuation cases, displaced-row
  and deduplicated pages, thirteen filter/empty controls, and ordinary queries.
- Owning-package aggregate: the reviewed second command passed under race
  detection, exit 0, package 700.175 seconds, within the fifteen-minute test
  timeout and 960-second outer timeout.
- Canonical changed-Go lint: all three owned Go paths, complete affected
  package, zero issues, exit 0, using 3 GiB and the reviewed timeout.
- PostgreSQL: `TestPostgresWorkspaceTaskSearchLiteral` skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` is absent. The actual Backend Postgres CI job runs
  the owning package with a configured service and must pass before delivery.
- Final catalog validation: passed, 381 decisions and 1605 specifications.
  Specification lint, owned-path diff/status, and Go formatting checks passed.
  Actual staged documentation coverage passed with all eight owned changed
  paths, the complete linked artifact chain, and no errors.

The requirement is active, the design is current, and the single order is done
for the implemented correction and completed local checks. The local PostgreSQL
skip does not count as a PostgreSQL pass. No implementation or heavy commands
ran during the prior design phase.

## Risks

- Escaping only the final OR arm, or only one query, leaves incorrect results
  or totals. Test all arms and counts independently.
- Backslash must be escaped with the same declared SQL character before outer
  wildcards are added. Adjacent/trailing punctuation guards against subtle
  replacement-order mistakes.
- PostgreSQL `ILIKE` needs real environment-gated behavior coverage. Record a
  local DSN skip explicitly and rely on the configured PostgreSQL CI run.
- Repository filtering and case/collation behavior are existing contracts;
  preserve them rather than broadening this fix.
