---
id: "01-complete-member-inventory"
title: "Load every matching Linear team member"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-LINEAR-MEMBERS-001
acceptance_criteria:
  - AC-INTEGRATIONS-LINEAR-MEMBERS-001.1
  - AC-INTEGRATIONS-LINEAR-MEMBERS-001.2
  - AC-INTEGRATIONS-LINEAR-MEMBERS-001.3
  - AC-INTEGRATIONS-LINEAR-MEMBERS-001.4
system_design:
  - ../../specs/integrations/system-design/linear-member-inventory.md
---

# Task 01: Load Every Matching Linear Team Member

## Summary

Make the real Linear member client traverse all pages before returning Creator
filter inventory. ROOT released implementation after reviewing the design package. The
permanent HTTP regression was independently authored in this same session.

## In scope

- Read the paired requirement/design and the manifest's evidence/test matrix.
- Mark this work order `in_progress` after the implementation release.
- First write `TestGraphQLClient_ListUsers_Pagination` in a focused new test file;
  observe the expected missing-Creator251 RED before production edits.
- Extract the users section into `graphql_users.go`, extend member query/data,
  and implement scoped/authenticated cursor traversal with whole-operation
  failure and repeated-cursor guards. Correct only the now-obsolete comment
  alongside `linearMaxPageSize` in `graphql_client.go`.
- Add the controls, failure, context, and malformed-cursor cases in the plan.
- Record actual command exits, joins, and current known-owned process absence;
  synchronize this work order and manifest results/status after verification.
- After all checks pass and the implementation matches the design, promote the
  requirement to `active`, design to `current`, work order to `done`, and plan to
  `implemented` before the final documentation gate.

## Out of scope

- All non-member pagination, global labels, issue filters, assignee changes,
  mutations, cache/workspace freshness or preservation, UI changes, DTOs,
  dependencies, storage, and unrelated refactoring.
- Live Linear requests, ROOT reproduction source reuse, delegated workers,
  broad frontend suites, full local backend lint/tests, and validator-test replay.
- Any merge without a later separate expected-head normal squash grant.

## Acceptance

1. Permanent real-client RED proves the full250+1 member defect; GREEN proves all
   members/fields, preserved wire scope/auth/order, and ordinary controls.
2. Relevant later-page failures, context cancellation/deadline, missing metadata,
   and repeated/cyclic cursors terminate with error and nil members. Terminal
   pages and nonterminal empty pages follow the owning design.
3. Targeted checks and the owning race suite pass once; complete-package changed
   Go lint covers every actual owned Go file; docs and diff gates pass. Record
   unchanged consumer/mobile/public-doc assessments without extra UI work.

## Verification

Run from the repository root after the later implementation release and under
the supplied sole-child global heavy lease. Commands are sequential. Use local
Go 1.26.0; record any unavailable tool/resource as a ROOT checkpoint, not a silent
toolchain install or an unbounded retry. Each long native session is retained
and actually joined, with its exit recorded before starting the next command.

First run this RED command after the permanent pagination regression is added
and before production changes; expected exit 1, missing Creator251:

```bash
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 280s go test -trimpath -race -tags=fts5 -p 1 ./internal/linear -run '^TestGraphQLClient_ListUsers_Pagination$' -count=1 -timeout=4m -v)
```

After the correction and focused edge cases, run meaningful affected GREEN,
then the owning package race suite once:

```bash
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 280s go test -trimpath -race -tags=fts5 -p 1 ./internal/linear -run '^TestGraphQLClient_ListUsers_' -count=1 -timeout=4m -v)
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 280s go test -trimpath -race -tags=fts5 -p 1 ./internal/linear -count=1 -timeout=4m)
env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=3GiB timeout --kill-after=10s 360s bash scripts/lint-go-changed apps/backend/internal/linear/graphql_client.go apps/backend/internal/linear/graphql_users.go apps/backend/internal/linear/graphql_users_test.go
```

The lint arguments must include ALL actual owned changed Go files if the file
set changes; the canonical helper checks their complete packages with its 5m
internal timeout. CI owns full backend validation. No broad local rerun after
passing checks unless a new change/failure requires it.

After final design edits, and again once after final implementation status
edits, run only the documentation validators (not their test suites):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/specs/integrations docs/plans/linear-creator-member-pagination
```

Check the new pair's catalog presence and REQ/AC/design/plan cross-references.
Run the existing validator export locally with the planned production change
to verify the package's complete reference chain (a documentation preflight,
not a production change or a validator-test replay):

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/linear-creator-member-pagination/plan.md',
  'docs/plans/linear-creator-member-pagination/task-01-complete-member-inventory.md',
  'docs/specs/integrations/requirements/linear-member-inventory.md',
  'docs/specs/integrations/system-design/linear-member-inventory.md',
];
const changedFiles = paths.map(filename => ({ filename, status: 'added', additions: 1, changes: 1 }));
changedFiles.push({ filename: 'apps/backend/internal/linear/graphql_client.go', status: 'modified' });
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
process.exitCode = result.ok ? 0 : 1;
NODE
```

Hosted coverage remains a delivery gate. Do not run PR network publication
during design. Public-doc validators are unnecessary while public
files remain untouched. No frontend commands are required for this change;
before normal commit hooks, install frozen apps dependencies once only if
missing (Node 24.18/pnpm 9.15.9), with no dependency/lockfile changes.

## Files likely touched

- `apps/backend/internal/linear/graphql_client.go`
- `apps/backend/internal/linear/graphql_users.go` (new)
- `apps/backend/internal/linear/graphql_users_test.go` (new)
- `docs/specs/integrations/requirements/linear-member-inventory.md`
- `docs/specs/integrations/system-design/linear-member-inventory.md`
- This work order and `plan.md`.

## Dependencies

None. ROOT's later explicit implementation release satisfied the phase prerequisite.

## Risks

- Avoid reusing a page value with stale metadata, leaking partial members, or
  treating an absent completion flag as terminal success.
- Preserve opaque cursor values and upstream error/context identities. Cursor
  cycle guards must not impose a legitimate inventory-size cap.
- Protected paused/stopped tasks, worktrees, volumes, shared dependencies/caches,
  foreign refs/processes, and private evidence remain untouched.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/linear-member-inventory.md)
- [System design](../../specs/integrations/system-design/linear-member-inventory.md)
- [Plan evidence and test matrix](plan.md)
- `apps/backend/AGENTS.md`, integration guidance, `/tdd` and its backend-test
  reference, existing `graphql_client_test.go` HTTP helpers and `do` errors.
- Official [Linear pagination](https://linear.app/developers/pagination).

## Results

Implemented after ROOT's later explicit release. The final production change
is confined to the extracted member query/response/ListUsers and the obsolete
shared page-size comment. The public member DTO and all consumer/UI code remain
unchanged.

- Permanent `TestGraphQLClient_ListUsers_Pagination` RED exited1: 250 members,
  one request, missing Creator251. Additional boundary RED confirmed omitted
  traversal, metadata validation, and cancellation behavior.
- Final affected GREEN exited0 (2.150s); the owning race suite exited0 (2.361s),
  run once. Six named test functions cover all four acceptance criteria.
- Complete-package canonical changed-Go lint exited0 with zero issues for all
  three owned Go files. No further production edits followed these checks.
- Native handles77003/52585/96774/50219/45690/22868 were actually joined;
  receipt groups are absent. Handle96774 was deliberately stopped for a fixture
  shutdown hang, repaired with an explicit handler release before final GREEN.
- No browser or frontend suite was needed: unchanged consumer mapping and
  state/data mobile exception were verified during design. Public integration
  documentation already states Creator filtering without a member cap.
- Final documentation gates and ordinary commit-hook/publication receipts are
  recorded in this task's external Kandev plan. Command receipts are preserved
  under `/tmp/kandev-child131-receipts/`.
