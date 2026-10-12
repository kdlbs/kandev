---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Implementation Plan: Preserve repository checkout defaults

## Overview

A rename can restore an old default branch or pull-before-worktree policy after
another service successfully changes it. Later task worktrees can then use the
wrong base, stale code, or a remote refresh the user disabled. Preserve supplied
intent for these two choices through the repository save, including its atomic
secret-binding companion, and return the choices committed by that mutation.
One sequential work order covers storage, service, actual routes and focused CI.

The workspaces system owns the failed contract because it owns persisted
repository checkout defaults. Extend its existing worktree-base-refresh pair,
AC .20 through .23. Workspace-settings-updates owns another table and is an
implementation example, not a repository requirement. This bounded correction
does not need a new incident specification or ADR.

## Evidence and assumption check

Confirmed: only `DefaultBranch` and `PullBeforeWorktree` gain omission
preservation; exact Host fences, deliberate full writes, recovery CAS, other
fields and existing admission/error behavior retain their semantics. No material
scope question remains. Implementation requires a later explicit ROOT release
of this reviewed package in the existing primary session.

Accepted ROOT evidence is the read-only regular mode-0400 candidate at
`/tmp/kandev-root-repository-checkout-defaults-candidate_test.go`, SHA256
`d68e1e32e1820f71dc24c600f4349f29f33953c0eb71a999fd946129256f3311`.
ROOT reports two actual `Service.UpdateRepository` instances sharing real
SQLite: branch rename, refresh enable, refresh disable, both choices, and
explicit branch with omitted refresh fail stored and returned assertions;
explicit-both and uncontested controls pass. The discovery namespace is
`/tmp/kandev-root-repository-checkout-defaults-discovery-20261007`; qualification
`4a0d2f`; original `19828/83cbe9` joined `2bd504`, exit 1, Go 0.981s;
group 4000725 freshly absent, temporary source removed, root clean at
`8feffe1e17fd5ac1b079fc52469bdfff780e5ad0`.

This session verified only the supplied file's type, mode, digest and source,
without replay, import or mutation. These receipts are not registered HTTP,
physical PostgreSQL, or actual worktree-creation execution proof. Permanent
regressions must be independently authored and run RED/GREEN later.

## Scope

### In scope

- Internal two-choice intent in request-driven repository saves, including
  real secret-binding atomic replacement, dialect-safe SQL and committed result
  projection into existing response/events.
- Meaningful independent SQLite and physical PostgreSQL service/storage tests,
  registered REST/WS, guarded compact MCP routing and exact Host controls.
- Bounded downstream producer projection from persisted choices through the
  existing executor resolver. Existing worktree policy stays unchanged.
- Targeted native Windows execution, required SQLguard/store conformance,
  a small public guide clarification and scoped backend convention update.

### Out of scope

- General partial patches for other fields, universal writer coordination,
  new schema or public fields, global event revisions or arrival-order promises.
- UI, localization, agent runner, refresh/defaulting/recovery redesign;
  new browser E2E, full local QA/review/verification or broad migrations.
- Sibling73's hosted PR4305/profile-enabled CI stage. No delegates, persistent
  tasks, new sessions/tabs or model switches.
- A new physical worktree scenario: no consumer behavior changes. The backend
  registered-save regressions prove the failure and response, while the narrow
  consumer test proves production projection. Do not claim new worktree creation
  execution from those tests or from the supplied receipt.

## Technical approach

Add neutral `models.RepositoryCheckoutIntent`, its repository interface alias
and required ordinary method, plus the atomic binding
companion capability. Add focused SQL implementation alongside
`sqlite/repository_entity.go`. Preserve only the omitted pair at SQL assignment
time and capture both choices and timestamp with the write's `RETURNING` row.
Commit before mutating the returned service model. Keep full/exact helpers and
narrow branch CAS compatible. Request validation, normalization, scope and
read-only checks stay with their current owners. No read-repair or fallback to
an unsafe complete-model ordinary save is acceptable.

| Surface / consumer | Shape and intended behavior | Verification and limits |
| --- | --- | --- |
| Registered REST PATCH | Both optional choices; null means omission | Real Gin route, service/SQLite, response and service event |
| Registered repository.update WS | Branch supported, pull absent from schema | Real dispatcher route; preserve omitted pull; no new pull control |
| Compact update_settings MCP | Repository target, writable non-nullable pair, caller/workspace guards | Registered guarded dispatcher, real registry/adapter/service; null rejection and sanitized result |
| Exact plugin Host | Expected resource timestamp, subset of settings | Actual adapter and write fence/replay controls; no SDK change |
| SQLite / PostgreSQL | Current shared task store with dialect rebinding | Independent connections, causal stale reads, real PostgreSQL blocking evidence; no schema replay substitute |
| Executor / worktree | Existing persisted branch/pull consumers | Real executor resolver/projection; existing worktree tests remain policy evidence, not new creation proof |
| Legacy complete writer / recovery CAS | Intentional full model / observed branch fence | Explicit controls; outside ordinary omission guarantee |

## Tests

The groups below are permanent tests. Actual execution results are recorded
in the work order; native Windows and hosted completion remain external gates.
The [work order](task-01-preserve-checkout-intent.md#test-matrix) defines their
causal fixtures, failure boundaries and exact selectors.

| Criteria | Planned test groups |
| --- | --- |
| .20 | `TestRepositoryCheckoutDefaultsConcurrentSQLite`, `TestRepositoryCheckoutDefaultsPhysicalPostgres`, `TestRegisteredRepositoryCheckoutDefaultsHTTP`, `TestRegisteredRepositoryCheckoutDefaultsWS`, `TestRepositoryCheckoutDefaultsGuardedMCP` |
| .21 | `TestRepositoryCheckoutDefaultsPresence`, concurrency groups, compact null rejection |
| .22 | `TestRepositoryCheckoutDefaultsCompanionAtomicity`, `TestRepositoryCheckoutDefaultsOwnMutationProjection`, `TestRepositoryCheckoutDefaultsPostgresFailures`, routed response/event groups |
| .23 | `TestRepositoryCheckoutDefaultsLegacyAndExact`, `TestRepositoryCheckoutDefaultsFailures`, `TestRepositoryCheckoutDefaultsExactHost`, PostgreSQL controls and real consumer projection |

## End-to-end evidence and mobile

Real registered backend save routes, guards, store writes and response/event
payloads are the end-to-end evidence for this data-only repair. They must use
actual publishers, not fabricated events or decoder-only tests. Mobile parity
was evaluated: no rendered surface, composition, copy, touch, scroll or
breakpoint changes; desktop and phone retain the same existing data paths.
No Playwright file or UI preview is required.

## Documentation impact

`docs/public/git-operations.md` currently describes omitted
`pull_before_worktree` as defaulting to true in the Git lifecycle paragraph,
without limiting that sentence to registration. The implementation work order
will clarify registration versus saves of an existing repository and explain
that a rename preserves its branch and refresh policy. Keep the existing
offline/local fallback and strict remote-materialization instructions.
This is an explanation-page clarification, not a new control or CLI contract.
`configuration.md` already accurately says task behavior uses repository
stored/detected defaults; integration watcher guidance already follows the
repository policy. No screenshot change is needed.

## Work orders

- [ ] [Task 01: Preserve checkout intent through repository saves](task-01-preserve-checkout-intent.md)

## Verification results

Design validation passed on 2026-10-07: catalog validation (360 decisions,
1433 specifications), all 36 specification-linter checks and full specification
lint. The repository's `validateCoverage` returned `covered`, no errors, for
the actual four artifact contents and anticipated service runtime path. It
resolved this work order, owning requirement and declared system design.
Local links, whitespace and size checks passed for all four files, including
the untracked plan/order; `git diff --check` passed. Every command joined with
exit 0; no original process handle remains outstanding.

The design-only checkpoint ended before implementation. A later ROOT reviewed-
package release admitted this same primary under exclusive local-heavy ownership.
Independent causal SQLite RED/GREEN, real PostgreSQL interleavings/waits and
controls, registered REST/WS, guarded compact MCP, exact Host, executor projection,
SQLguard and exact SQLite/PostgreSQL task-store conformance have now executed.
The work order records results and bounded fixture corrections. Scoped lint passed after ROOT-approved diagnostic corrections and one bounded
1GiB validation exception. Normal-hook publication remains in progress; native Windows, hosted PostgreSQL,
required checks and substantive current-head review remain external delivery
gates. No new worktree-creation or browser proof is claimed.

## Risks

- Returned-row boolean/timestamp scanning and transaction completion must work
  in both dialects; a successful query alone does not prove a successful commit.
- Internal required-method additions affect test doubles. Adapt only the
  necessary interfaces and fixtures; do not test missing-method panics.
- Exact Host replay and companion rollback must retain their existing outcomes.
- Later complete-model writers can still overwrite choices deliberately; other
  omitted fields retain their existing snapshot semantics. Event delivery can
  reorder mutations. This package promises neither universal serialization nor
  freshness upon response arrival.
- Native Windows step placement must be re-inventoried after sibling work;
  preserve its separately owned stage and existing test steps.
