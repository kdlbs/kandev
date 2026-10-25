---
created: 2026-10-18
status: implemented
requirements:
  - REQ-WORKSPACES-BRANCH-INVENTORY-001
system_design:
  - ../../specs/workspaces/system-design/repository-branch-inventory.md
legacy_specs: []
---

# Implementation Plan: Branch Inventory Identities

## Overview

Deliver a complete mixed local and remote-tracking branch inventory by correcting
its private identity keys. One sequential work order owns the service regression,
minimal production correction, compatibility checks, and final contract status.
This package is design-only until ROOT review and a later explicit implementation
interrupt to the same primary session. No delegates, new tasks/sessions, or model
switches are authorized.

## Admission and ownership

Startup was clean on `feature/list-colliding-local-2ic`, with HEAD equal to
authoritative remote main `286a5e0af7d566f6c2708fd174ff52310dc7198a` after
PR #4545. The unpublished owned branch already materialized the requested base;
no checkout, foreign ref, or worktree mutation was needed. All four implicated
source blobs matched the supplied admission identities. Qualification evidence
was supplied by ROOT and was not replayed. The admission receipt is
`/tmp/kandev-root-branch-identity-discovery-aapy521u/post135-admission.json`.

Workspaces owns repository identity and branch data. Existing
[branch reads](../../specs/workspaces/requirements/repository-branch-reads.md)
excludes backend behavior and owns freshness; branch policies owns policy
selection, and executors owns branch resolution for scripts. Their contracts
remain separate. No current requirement states the general inventory set
guarantee, so the new vertical pair supplies that missing contract.

## Scope

### In scope

- [Inventory requirements](../../specs/workspaces/requirements/repository-branch-inventory.md),
  all four acceptance criteria, and the paired design.
- Local loose, remote loose, and packed-ref map identity and exact deduplication.
- Real-Git service tests for both causal storage forms, both disjoint controls,
  mixed storage, and linked-worktree compatibility.

### Out of scope

- Frontend production, option-value ambiguity, freshness, cache/save ordering,
  schema, public API shape, layout, localization, and browser execution.
- Ref validation/normalization, Git global configuration/prefix/textconv/signature
  policy, commit display, submodules, branch execution, fetch/push/pull/rebase.
- Authorization, paths/grants, providers, pagination, routing, or lifecycle changes.
- Permanent code/tests, install, product build, heavy checks, staging,
  commit/push/PR in the design turn.

## Technical approach

The current local collector stores raw names while the remote collector stores
`remotes/<remote>/<name>`. Thus local `remotes/origin/topic` and remote
`origin/topic` address one key. Remote loose collection overwrites the local;
packed refs can instead keep the local and suppress the remote. Supplied ROOT
service evidence establishes both forms; current source independently confirms
the mechanism. The noncolliding `feature/topic` controls do not share this key.

Apply the design's full-ref internal keys consistently in all collectors.
Retain collector order, absent-only packed insertion, public projection, and
sorting. Add regressions in a new test file rather than enlarging the existing
oversized discovery test file. No new dependency or generalized abstraction.

## Tests

All new tests live in
`apps/backend/internal/task/service/repository_branch_inventory_test.go`.

| Criteria | Planned test methods and evidence |
| --- | --- |
| AC-WORKSPACES-BRANCH-INVENTORY-001.1, .2, .3, .4 | `TestListBranchesWithCurrent_DistinctBranchIdentities`: loose/packed collision and disjoint-control subtests, both local entry points, complete ordered identity/metadata equality and checked-out branch |
| AC-WORKSPACES-BRANCH-INVENTORY-001.1, .2, .3 | `TestListBranchesWithCurrent_MixedBranchStorage`: native packed refs plus loose overrides of both exact identities, one result per identity, unrelated entries and ordering retained |
| AC-WORKSPACES-BRANCH-INVENTORY-001.3, .4 | `TestListBranchesWithCurrent_LinkedInventory`: real linked worktree, common refs, worktree HEAD, sorting and remote HEAD exclusion |

Require the causal loose and packed cases to fail for missing opposite identities
before the production correction, while both disjoint controls pass. Use genuine
distinct commit tips, isolated Git environment, and explicit fixture oracles.
Service output contains no tip field: do not claim it proves a returned tip.

## End-to-end evidence boundary

The real service against native disposable Git repositories is the bounded
end-to-end read path for this backend inventory change. It includes path
resolution, enumeration, metadata, and result ordering. No HTTP handler, browser,
remote executor, or rendered UI execution is claimed or required. Frontend files
are read-only consumers. Mobile composition and interaction remain unchanged.

## Work orders

- [x] [Task 01: List distinct branch identities](task-01-list-distinct-branch-identities.md)

Wave 1 contains this one sequential work order with no work-order dependencies.
The later primary session follows red, green, bounded package checks, then
updates the work order to `done`, requirement to `active`, design to `current`,
and plan to `implemented` only when all criteria pass.

## Verification strategy and results

Design checks completed on 2026-10-18:

- `python3 scripts/list-docs.py validate`: exit 0; 386 decisions and 1,633
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all specification files passed.
- Explicit work-order/plan/design/requirement/AC and relative-link checks passed.
- Actual tracked plus untracked documentation-coverage preflight: `exempt`,
  `errors: []`, exactly the four owned documents. This is docs-only evidence,
  not implementation coverage. Receipt:
  `/tmp/kandev-child136-branch-inventory-design-1XdJ93/design-preflight.json`.
  Corrected-package light-check and coverage receipt:
  `/tmp/kandev-child136-branch-inventory-design-1XdJ93/design-review-preflight.json`.
- Diff checks, including no-index whitespace checks for the four untracked
  documents, passed. Only those four documents are changed; all remain unstaged.
- Existing toolchains verified at canonical mise paths:
  `/home/jcfs/.local/share/mise/installs/go/1.26.0/bin` and
  `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin`.
  All later Go tests and lint explicitly set `GOTOOLCHAIN=local` to prohibit
  automatic toolchain download. No install or product test/build ran.

Historical extra check from the initial design pass only:
`python3 scripts/lint-spec-files.test.py` exited 0 with 36 passing tests.
The linter implementation is unchanged; its self-tests are not a required
design or execution gate and will not be replayed for this package.

Later execution commands and resource bounds are in Task 01. They run only the
canonical `internal/task/service` race package and changed-Go complete-package
lint, sequentially. No full backend local gate or frontend build is scheduled.
If `scripts/run-quiet` is used later, first set `KANDEV_RUN_QUIET_DIR` to a unique
task-owned directory; never invoke its shared-temp retention default.

Implementation completed after ROOT's later explicit release in the same primary.
All four acceptance criteria are covered by the three independently authored
native-Git service tests, including both local entry points and fourteen service
reads across the collision, control, mixed-storage, and linked-worktree cases.

- One frozen pnpm install joined exit 0 (2.072s), reusing existing caches.
- Causal RED joined exit 1 as expected: loose local identity absent and packed
  remote identity absent; both disjoint controls passed. No fixture/setup error.
- Narrow GREEN joined exit 0 (package 3.925s).
- One complete service race package joined exit 0 (package 90.069s).
- Canonical changed-Go complete-package lint joined exit 0 (113.785s command).
- Final catalog/all-spec checks, actual tracked plus untracked six-path coverage,
  and diff checks passed. Coverage is `covered` with `errors: []`.

Exact commands, native handles, joins, and result details are recorded in
[Task 01](task-01-list-distinct-branch-identities.md). Raw startup, stdout, stderr,
and join receipts are retained under
`/tmp/kandev-child136-branch-inventory-implementation-5c4AGR/`.
No full-backend local gate, linter self-test replay, product build, browser run,
private-fixture access, or frontend production change occurred.

## Delivery constraints

After later implementation release, normal commit/push/ready PR delivery is
already authorized. Preserve active hooks and staged i18n selectors. If hook
dependencies are absent, run the frozen pnpm install once from `apps/` with
Node 24.21.0 and pnpm 9.15.9, only in that later phase. The canonical pnpm
executable is `/home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm`;
place that directory before Node's bin in PATH for hooks. Record actual owned tracked
and untracked code/test coverage as `covered` with `errors: []` before delivery.

Continue directly in the same turn with one original attached 90-minute monitor
(GNU 91-minute timeout, kill after 10 seconds, 60-second cadence, all-terminal).
At launch persist separate progress stderr and terminal stdout plus original
startup and join receipts. Do not restart a live monitor for logging or add
manual timed status polling. Read completed substantive review feedback while
CI runs; rerun only checks justified by actual findings or changes. Required
checks, backend/frontend/E2E parents, review dispositions, and clean exact local,
remote, and PR heads must satisfy ROOT's readiness gate. ROOT's static review
precedes a later separate expected-head normal squash grant. No merge is
authorized here; no admin/bypass, amend, branch deletion, cache wipe, foreign
kill, resource escalation, drift rebase, or synthetic merged tests.

## Risks

- Partial key conversion can retain the collision or introduce loose/packed
  duplicates; one work order changes all three sites together.
- Service metadata cannot distinguish same-identity tips. Native Git fixture
  checks establish genuine overrides; code retains loose precedence.
- Private ROOT fixture reuse would invalidate independent coverage. Never read,
  copy, replay, or delete private `candidate_test.go` or `run.py`.
- Preserve protected stopped tasks/worktrees, the protected anonymous volume,
  foreign refs/processes/dependencies/caches, and known evidence receipts.
