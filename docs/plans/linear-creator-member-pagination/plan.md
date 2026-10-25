---
created: 2026-10-14
status: implemented
requirements:
  - REQ-INTEGRATIONS-LINEAR-MEMBERS-001
system_design:
  - ../../specs/integrations/system-design/linear-member-inventory.md
legacy_specs: []
---

# Implementation Plan: Linear Creator Member Pagination

## Overview

Return the complete team member inventory to the existing Linear watch Creator
filter. One sequential work order adds independent regression coverage, corrects
the real GraphQL client, and verifies only the owning package. ROOT reviewed the design package and released implementation in this same
primary session after the completed design checkpoint.

## Evidence and ownership

On clean ROOT main `bc0aa190a3e31b8b14297565c6251b33c334d2c0`, a disposable
actual HTTP fixture returned 250 members plus continuation metadata, then offered
member 251. Actual `GraphQLClient.ListUsers("ENG")` returned only 250 members
after one request. Empty (0) and single-page (3) controls passed: one causal
failure. ROOT's qualification exited 1, package time 0.088s; native handle
`1909` was actually joined as `7f127e`, temporary source removed, current owned
process group absent, ROOT clean.

Evidence receipt: `/tmp/kandev-root-linear-creator-pagination-20261014-w56rkel6/qualified-proof.json`,
`qualification-receipt.json`, and `qualification.log`. These are supplied
qualification evidence, not this child's permanent RED. Do not read, copy,
replay, or delete ROOT's `candidate_test.go` or `source.json`.

Root cause: `teamMembersQuery` requests only `first: 250`, without `after` or
`pageInfo`; `ListUsers` performs a single request. The Creator consumer maps the
returned users without a second cap. Assignee selection is sentinel-only.

The integration system owns this provider inventory contract. Catalog and
adjacent/legacy discovery found no existing Linear member-inventory acceptance
criterion, so the paired capability requirement defines the missing contract;
this plan records its repair. No companion implementation package owns it.
Confirmed scope and source inspection resolve the assumption check without an
operator question or a new ADR.

## Scope

### In scope

- [REQ-INTEGRATIONS-LINEAR-MEMBERS-001](../../specs/integrations/requirements/linear-member-inventory.md)
  and AC `.1` through `.4`.
- Only `ListUsers`, its query/response data, focused tests, and the obsolete
  shared page-size comment. Extract the member section to `graphql_users.go`.
- Context propagation, whole-operation failure, and cursor-cycle termination.

### Out of scope

- Labels (including global labels), teams, states, `SearchIssues`, watcher poll
  bounds, assignee behavior, mutations, membership policy, and response DTOs.
- Frontend cache/workspace freshness, preservation races, redesign,
  virtualization, new dependencies, configuration, or persistence.
- Production/permanent test changes, installs, product tests, Go lint/build,
  hooks, staging, commits, pushes, or PRs during this design phase.
- Delegation, new sessions, model switches, or protected/foreign resource work.

## Technical approach

Follow the [owning design](../../specs/integrations/system-design/linear-member-inventory.md).
`graphql_users.go` owns `teamMembersQuery`, `usersData`, and `ListUsers` after a
focused extraction from `graphql_client.go` (currently 779 physical lines).
Keep the existing request filter, `first`, ordering, member mapping, and `do`.
Traverse fresh pages with presence-aware metadata and a local used-cursor set.
Require completion before publishing; return nil members on every error.

This is a first-party Linear GraphQL path, not a shared provider adapter. Mock
clients, other providers, REST response shapes, services, handlers, and UI remain
compatible without edits. Invalid completion metadata produces the existing
ordinary-error/HTTP-500 path; upstream errors retain their current identity.

## Tests

New `apps/backend/internal/linear/graphql_users_test.go` independently exercises
the real client against `httptest.Server`, reusing existing endpoint/server
helpers when useful. No live API or credentials.

| AC suffix | Planned named test and meaningful evidence |
| --- | --- |
| `.1`, `.2` | `TestGraphQLClient_ListUsers_Pagination`: full first page 250 + Creator251; compare all member IDs, order, and DTO fields; same ENG membership filter, first250, updatedAt, auth and endpoint every page; exact after token and two requests |
| `.2` | `TestGraphQLClient_ListUsers_Controls`: empty, three-member single page, exact250 terminal, terminal missing/null cursor, empty team without requests; preserve non-nil empty result |
| `.1`, `.4` | Pagination subcase: nonterminal empty page with a fresh opaque cursor still reaches the final member |
| `.3` | `TestGraphQLClient_ListUsers_PageFailure`: later-page HTTP 401/500, GraphQL error, and malformed JSON return nil members and preserve error classification |
| `.3` | `TestGraphQLClient_ListUsers_Context`: canceled/deadline contexts at admission, between pages, and during a blocked later HTTP page; deterministic channels/context coordination and joined cleanup, no sleeps |
| `.4` | `TestGraphQLClient_ListUsers_InvalidPagination`: missing/null pageInfo or hasNextPage, nonterminal missing/null/empty cursor, immediate repetition and A-B-A cycle; bounded requests, error and nil members |

Test names may use table subcases for the listed behavior. Assert outcomes and
wire contracts, not private accumulator/set implementation. Malformed fixtures
stop serving continuation after a small request budget to fail safely.

## E2E evidence and mobile assessment

The real-client HTTP regression covers Linear's wire response through complete
`[]LinearUser`, including Creator251. Read-only consumer assessment confirms
`httpListUsers` forwards it, `listLinearUsers` returns it, `useTeamsAndStates`
stores it, and `AssigneeAndCreatorRow` maps all users to Creator options. No
additional consumer test is materially needed for unchanged pass-through code.
No browser E2E or frontend suite is scheduled. This is the `mobile-parity`
state/data exception; desktop and phone share the same inventory, with no
geometry, touch, scrolling, navigation, or breakpoint change. No UI preview.

## Public documentation assessment

`docs/public/integrations.md#linear` already states that watches support Creator
filters and makes no member-cap claim. Its issue-poll pagination bound describes
a separate operation. README and screenshot-catalog checks found no affected
claim. Keep public docs unchanged unless implementation exposes an actual
contradiction. Internal requirements/design/plan provide the durable update.

## Work orders

- [x] [Task 01: Load every matching Linear team member](task-01-complete-member-inventory.md)

## Verification results

Task 01 is implemented; requirements are active and the design is current.

- Independent permanent pagination RED: exit 1, 250 members/one request instead
  of 251; package 0.094s, native77003 actually joined (`d95667`), owned group
  1100901 absent. Edge-case RED also failed for the expected missing traversal,
  metadata checks, and cancellation boundaries (native52585 joined `f66b91`).
- Affected GREEN after fixture cleanup: exit 0, package 2.150s; native50219 joined
  (`293b82`), owned group1109597 absent. Six named regression tests cover AC .1-.4.
- Owning `internal/linear` race suite, once: exit 0, package 2.361s; native45690
  joined (`649691`), owned group1110305 absent.
- Canonical `scripts/lint-go-changed` with all three owned Go paths: exit 0,
  zero issues, native22868 joined (`ecb68e`), owned group1111022 absent.
- Every Go test used local Go1.26.0, `-trimpath -race -tags=fts5 -p 1`,
  GOMAXPROCS2/GOMEMLIMIT512MiB, internal4m/outer280s/kill10s. Lint used
  GOMAXPROCS2/GOMEMLIMIT3GiB, internal5m/outer360s/kill10s.
- One initial GREEN command was launched from the repository root and exited1
  before compiling (no go.mod); it was corrected to apps/backend. A later
  affected run exposed an HTTP test-fixture shutdown hang. Its owned group was
  stopped and actually joined before adding a deterministic handler release;
  no production change was needed. Both attempts' groups were absent on join.
- Documentation-only catalog/all-spec lint/coverage/diff gates follow final
  lifecycle edits, with their actual receipts in the external task plan. Normal
  hooks also run at commit. No broad local/backend/frontend/validator-test suites.

Full command receipts are in `/tmp/kandev-child131-receipts/` and the external
Kandev task plan. No ROOT reproduction source was read or reused. Public-doc,
mobile state/data exception, and unchanged consumer assessments above stand.

## Risks

- More member pages increase request count/latency; existing context and HTTP
  timeout apply. Do not silently cap the result to reduce work.
- Linear can change membership during traversal; this follows upstream cursors
  and ordering without adding snapshot or deduplication semantics.
- Malformed cursor guards must include longer cycles and allow terminal pages
  without a cursor. Valid metadata must not be inferred from zero values.
- Every long native handle must be retained and actually joined before another
  owning heavy command begins; preserve shared caches and foreign processes.
