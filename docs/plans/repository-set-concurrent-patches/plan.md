---
created: 2026-10-03
status: in_progress
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-001
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
legacy_specs: []
---

# Implementation Plan: Preserve Concurrent Repository-Set Patches

## Overview

Preserve optional-field presence from repository-set requests to database
mutation so two successful disjoint patches cannot undo one another. Workspaces
owns the contract because it owns persisted set metadata and membership.

One sequential work order delivers the service, store, transport evidence, and
documentation together. Splitting the new store seam from its service caller
would leave the defect live; separating its regression evidence would make the
repair incomplete. There is no independent migration or frontend delivery step.
Implementation starts only after a later explicit release in this same session.

## Confirmed defect

At `220e84bb8b62d860adc05891c835516fbf249f2e`,
`Service.UpdateRepositorySet` reads a set and folds optional fields into the
snapshot. `sqlite.Repository.UpdateRepositorySet` then writes both `name` and
`description`, including omitted values from that earlier read. Nil membership
already leaves item rows alone.

The accepted real SQLite/service proof paused a description-only request at its
store write after read/validation, completed a name-only rename, then released
the first request. Both returned success. The final name reverted from
`New name` to `Full-stack`; `new description` survived. Its sequential control
passed. Parent command receipt: `GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath
-tags fts5 -race -p=1 ./internal/task/service -run
'^TestRootRepositorySetPartialPatchProof$' -count=1`, handle `27624` joined,
exit 1 solely for the expected overlap assertion, no race report. Test time was
0.23s, package time 0.299s; fresh compilation took about 4m50s.

The read-only archive `/tmp/kandev-repository-set-patch-repro_test.go` was inspected
but not replayed. Its source has been removed from the parent's clean checkout.
The parent owns archive cleanup after actual merge. Permanent tests below are
new repair evidence, not another run of that accepted proof.

## Scope

### In scope

- Amend the existing owner pair with omission and disjoint-update criteria
  `.9` through `.11`, retaining atomicity `.8`, permissions `.6`, events `.7`,
  workspace membership `.1`/`.4`, and unique naming `.3`.
- Add one typed internal patch/store seam. Preserve explicit whole-set store
  callers and the public request/DTO shapes.
- Write only supplied metadata columns and explicit ordered membership in the
  same transaction; preserve omitted member identities, timestamps and bases.
- Cover real SQLite store/service behavior, registered HTTP/WS boundaries, and
  environment-gated PostgreSQL behavior on independent connections.
- Clarify the existing public API paragraph at implementation time.

### Out of scope

- Frontend production, copy, layout, touch, dirty-field tracking, or full-editor
  conflict UI. The current editor sends the complete draft.
- Schema changes, migrations, new APIs, ETags, revisions, idempotency, event
  coordinators, snapshot frameworks, global response/event ordering, batch-task
  atomicity, repository deletion policy, or other concurrency defects.
- Browser/build/E2E work, full service/backend local suites, extra broad audits,
  synthetic compatibility tests, agents, tasks, sessions, or tabs.

## Technical approach

### Audited callers and compatibility

`repository.Provide` returns the concrete shared `sqlite.Repository`. Backend
bootstrap passes `repos.Task` directly as `Repos.RepositorySets`; there is no
production forwarding adapter or second repository-set store implementation.
`service.Repos` and `Service.repositorySets` use `RepositorySetRepository`.
The only production caller of the existing store update is the task service.
Direct store callers in `repository_set_test.go`, `repository_set_review_test.go`,
and `repository_set_postgres_test.go` intentionally write both metadata fields.

REST and WS registration in `repository_set_handlers.go` forwards pointer fields
to the same service. Backend settings-domain mutation decodes
`UpdateRepositorySetRequest` and calls that service too. Its dependency
interface and public settings contract need no signature change.

The web API already sends only present fields; `repository-sets-api.test.ts`
explicitly covers name-only, membership-only and empty-description requests.
The set editor's full-draft submission stays a full update. DTO, boot-state,
event routing and frontend stores require no changes.

| Surface | Identity/presence | Intended behavior | Evidence |
| --- | --- | --- | --- |
| SQLite store | Set ID; optional typed metadata/items | Direct touched-column UPDATE, atomic item replacement | New store controls and independent-service barriers |
| PostgreSQL store | Same seam; independent connections, one isolated schema | Row UPDATE serializes writes before item replacement | New real behavior/concurrency tests under DSN |
| Legacy store update | Full metadata; optional items | Both metadata values written intentionally | Existing whole-set/rollback/deleted controls |
| REST PATCH | Path set ID; pointer body fields | Forward omitted/present/combined fields unchanged | Registered router, real service/DB, DTO/event assertions |
| WS update | Payload set ID; same pointer body | Same service and mutation semantics | Registered dispatcher, real service/DB, DTO/event assertions |
| Settings domain / web client | Existing optional service request / serialized present fields | Inherit repaired service; no contract change | Source audit plus service/boundary tests |

Both member inputs together and invalid values retain their existing rejection
paths. An absent PostgreSQL DSN produces an explicit skip; it is not evidence
that the database behavior passed.

### Mutation boundary

Create `models.RepositorySetPatch` in `models/repository_set_patch.go`, with
`Name *string`, `Description *string`, and `Items *[]RepositorySetItem`.
Add `PatchRepositorySet(ctx, id, patch) error` to `RepositorySetRepository`.
Keep `UpdateRepositorySet(ctx, set, repositoryItems) error` and its semantics.
Do not put the patch in the parent repository package: that package imports
SQLite through `provider.go`, so SQLite importing it would form a cycle.

Validate through the existing service rules, normalize present values, and
construct a patch containing only present fields. Initial reads provide
authorization and workspace identity, never omitted write values. A small
shared store transaction helper supports both entry points, including existing
caller timestamp/item population behavior for the whole-set method.

Use fixed SQL assignment fragments and parameter values, then `Rebind`.
Always UPDATE `updated_at` and check RowsAffected before member mutation.
This keeps empty-patch success/event/timestamp behavior and detects deleted
parents. Replace items only for non-nil `Items`; preserve contiguous order and
saved bases. Retain the unique-name index and all-or-nothing rollback.

No stored metadata is read or merged inside this mutation. PostgreSQL's UPDATE
locks the parent row before replacing items, so a stale validation snapshot
cannot carry omitted fields back to storage and independent writers serialize
member replacement. The read-modify-write advisory-lock rule does not apply
to this direct SQL patch. If implementation changes to reading/merging stored
values, revise the design and acquire the required PostgreSQL transaction lock
before the first read/update. A service mutex alone cannot satisfy this plan.

Keep the existing post-commit reread and publish the same model returned by it.
That observation may include later commits and separate metadata/member reads;
it is not a revision snapshot. No publication on failed mutation, no stronger
event-order or post-commit deletion guarantee.

### Documentation and decisions

Public audit found the existing API paragraph in
`docs/public/tasks-and-workflows.md#repository-sets`. During implementation,
add a small clarification covering omitted metadata, explicit empty description,
disjoint patches and overlapping writes. This is a how-to page with an existing
API reference subsection. `websocket-api.md` already links the action catalog;
keep the detailed contract in one place. Root README/screenshots have no affected
contract. No screenshot, navigation or AGENTS update is needed.

The `/record` check is satisfied by amending the existing pair and recording
the local repair rationale here; no repository-wide decision or new ADR is
needed. A process-local mutex would fail across instances; read/merge/whole-write
would need extra locking; optimistic versions would introduce a new public
contract. The direct touched-column write implements existing PATCH intent
with the smallest shared persistence boundary.

The [completed base-branch plan](../repository-set-base-branches/plan.md) remains
historical delivery evidence. It owns no pending work affected by this repair;
do not rewrite its completed counts or rerun its frontend/E2E commands.
Backend data outcomes are shared by desktop and phone; there is no new rendered
surface or mobile composition to preview/test in this work order.

## Tests

All new test names below are planned, not executed at design time. Detailed
scenarios, exact commands, resource rules and existing controls are in Task 01.

| Criteria | Permanent evidence |
| --- | --- |
| `.9`, `.10`, `.7`, saved-base preservation | `TestRepositorySetConcurrentDisjointPatches`, both metadata and membership/metadata directions; distinct initial/new sentinels, final DB state and response/events |
| `.9`, `.3`, `.4`, `.6`, `.8` | `TestRepositorySetPatchPresenceAndValidation`; omitted/explicit-empty/no-op and rejected invalid/foreign/conflict/auth changes, no partial DB/event effect |
| `.8`, `.9` | `TestRepositorySetPatchDeletedBeforeWrite`, `TestPatchRepositorySetPresence`, `TestPatchRepositorySetRollsBack`, `TestPatchRepositorySetDeleted`, `TestPatchRepositorySetNameConflict` |
| `.11` | `TestRepositorySetConcurrentSameFieldPatches`, `TestPostgresRepositorySetConcurrentPatches`; complete member winner, commit-order metadata winner |
| `.7`, `.8`, `.9` | `TestHTTPRepositorySetPatchPresenceAndEvents`, `TestWSRepositorySetPatchPresenceAndEvents`; actual registrations and real database |
| `.8` through `.11` | `TestPostgresRepositorySetPatchBehavior`, `TestPostgresRepositorySetConcurrentPatches`; real dialect behavior and actual lock contention |

The work order includes the existing affected whole-set, validation, membership,
saved-base and read-only transport controls by exact names. It does not run the
full service package or unrelated migrations. Model type declarations need no
setter-only test; exercise them through the real consumers.

## End-to-end evidence

Registered HTTP/WS request -> service -> real SQLite -> response/event coverage
proves the existing API behavior end to end. Browser rendering and wire shapes
do not change, so this backend package needs no Playwright suite.

## Work orders

- [ ] [Task 01: Preserve repository-set patch presence through persistence](task-01-preserve-patch-presence.md)

## Verification results

Design-phase checks passed:

- `python3 scripts/list-docs.py validate`: 346 decisions and 1327 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed; the four document paths are unstaged/uncommitted.
- Repository `validateCoverage` with actual design paths: correctly docs-exempt.
  With the three projected production trigger paths: covered, no reference
  errors, accepted the owning requirement/design/work order. Implementation
  must still run the actual-path preflight after code changes.
- Verified all 24 existing test names in the selectors against source; the 12
  new names are explicitly planned. Every scoped test selector is anchored.

Node preflight used existing Node v24.21.0 via Bash without login. No
production/permanent test changes, installation, proof replay or heavy checks
ran during design. The later explicit parent release authorized Task 01 in this
same session; implementation receipts are recorded in its Results section.
Hosted checks, review, actual merge and joined cleanup remain delivery gates.

## Risks

- Pointer presence can be lost while normalizing values or converting legacy
  membership. Both inputs must use one internal representation.
- An omitted membership write must not recreate items, even when the earlier
  read held a different order or saved base.
- The PostgreSQL isolated helper owns one connection. Reusing it for both
  writers would prove pool serialization instead of database behavior.
- SQLite/PG constraint failures after the metadata UPDATE must roll back that
  update, item deletion and any partial inserts, including timestamps.
- Concurrent reread/publication remains observational; this repair provides no
  globally ordered events or fully specified stale-draft conflict detection.
- Resource failure or a lost command handle is an unresolved check, never a pass.
