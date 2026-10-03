---
created: 2026-10-03
status: in_progress
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-001
  - REQ-WORKSPACES-BRANCH-POLICIES-004
system_design:
  - ../../specs/workspaces/system-design/branch-policies.md
legacy_specs: []
---

# Implementation plan: Preserve branch-policy workflow edits

## Outcome and ownership

Preserve partial policy edits through the current saved workflow and actual task
creation. Workspaces owns reusable repository branch policies; Tasks consumes
immutable snapshots. Extend the existing [requirements](../../specs/workspaces/requirements/branch-policies.md)
and [design](../../specs/workspaces/system-design/branch-policies.md), especially
REQ-001 and REQ-004. The original [delivery package](../branch-policies/plan.md)
is historical completed feature work; none of its six packages is reopened.
The owning pair stays draft because this patch does not certify unrelated
sections or promote the original proposed snapshot ADR.

## Phase barrier and identity

<!-- kandev-system: task=20478f62-1f6c-4af8-9057-cbedc56991a4 session=647b0064-81a0-4c55-9ff6-27a8de401152 parent=14825981-b175-411d-999a-31ddc2aa5fc3 -->

The design turn delivered four unstaged/uncommitted artifacts and ended. Root
reviewed them and explicitly released implementation in this same primary
session. Production changes and permanent tests are now locally verified;
normal delivery through actual merge remains authorized and pending.
No agents, extra tasks/tabs, delegation, model switch or operator
approval prompt. Keep the title **Preserve branch-policy workflow edits**.
Preserve user edits, plan identity, this marker, question barriers and completion
gates. Critical unsafe blockers use the parent-question tool as the final action;
no work follows that call. Parent handoff messages are queued/omitted delivery,
not child-to-parent interrupts. No routine acknowledgment/heartbeat messaging.

## Accepted defect evidence

Base: `b0dc2bef512eda8def545ba5cf2edc67bbedf73d`. The root's real SQLite/service
proof paused a description-only update after its read/validation at the actual
store write. Another successful request changed base `develop` to `main`,
template `feature/{title}-{suffix}` to `hotfix/{title}-{suffix}`, and target
`develop` to `release`. The delayed description then restored the old tuple,
and actual `validateTaskRepositoryPolicies` copied it to a new input snapshot.
Both requests succeeded; the sequential control passed.

Accepted read-only archive: `/tmp/kandev-branch-policy-patch-repro_test.go`.
Root command: `GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^TestRootBranchPolicyPatchProof$' -count=1`.
Root handle `19659` joined exit `1`, expected overlapping snapshot assertion
only, no race; test 0.28s, package 0.349s. Archive inspected, not replayed.
Root removed temporary source; initial worktree clean. Parent owns proof cleanup
after actual merge. Permanent coverage must add persisted task outcomes rather
than re-run the accepted proof.

## Scope

Implement one atomic partial-policy mutation through service, storage and existing
REST/WS transports. Preserve current effective-policy validation, authorization,
read-only behavior, repository scope, uniqueness, delete errors and truthful
success publication. Prove the complete post-edit tuple in newly persisted task
repositories and preservation of pre-existing snapshots. Add a minimal existing
public-doc clarification after implementation.

Exclude Create/Delete/Gitflow redesign, initializer concurrency, repository-set
work, general concurrency or loader audits, UI draft/dirty-field/CAS repairs,
new endpoints, DB schema/history/manifests, ETags/global revisions, runtime
flags, new ADRs, prompt/worktree/provider changes without a proven causal defect,
frontend copy/layout/build/browser/E2E, moving-main rebases and synthetic
compatibility tests.

## Technical approach and inventory

1. `UpdateRepositoryBranchPolicyRequest` and handler update bodies preserve five
   optional pointers already. Add the narrow proposed
   `models.RepositoryBranchPolicyPatch` and `PatchRepositoryBranchPolicy` seam
   in `repository/interface.go`; do not reconstruct a full row in the service.
2. Keep authorized repository identity from the initial service read and writable
   checks. In the store transaction, acquire PostgreSQL advisory lock namespace
   `repository-branch-policy:` + policy ID before canonical read/update, then a
   row lock. SQLite takes a scoped no-op writer update before canonical read.
   Read/write by policy ID and authorized repository ID, on the transaction.
3. Merge only present pointers into the current row. Run the existing service
   `normalizeRepositoryBranchPolicy` through one policy-specific pure function
   parameter, preserving service validation ownership and avoiding a models to
   worktree dependency cycle. No database/event I/O in that function and no
   generic callback engine. Check normalized name within the transaction and
   retain the DB constraint for other policy identities/creates. Map observed
   and DB uniqueness failures to existing service conflict semantics.
4. Persist the normalized current candidate atomically; retain row identity,
   creation timestamp and existing affected-row deletion guard. Return the full
   transaction result after commit; publish that result only. No post-commit
   reader-pool reconstruction, global event-order guarantee or claim that a
   result stays newest after another commit.
5. Retain intentional legacy store full-set writes. Current callers are the
   partial service and store CRUD test only. `repository.Provide` returns concrete
   `sqlite.Repository`; no branch-policy forwarder needs migration. Typed
   interface additions require a focused compilation search for embeddings.
6. Registered REST PATCH and WS dispatch continue to use the same service.
   `toBranchPolicyPayload` filters undefined values; settings intentionally sends
   fully specified drafts. Preserve those callers and boot/store event wiring.
7. `service_task_branch_policy_snapshot.go` resolves policy before insertion;
   use real `CreateTask`, `ListTaskRepositories` and event serialization to prove
   persisted complete tuples after both patches finish. Leave runtime, prompt,
   title rename and PR snapshot consumers read-only.

This composes the established transaction lock pattern in
`repository/sqlite/message_agent_plan.go` and typed presence boundary in
`repository/sqlite/repository_set.go`; it does not reopen repository sets.
No new long-lived ownership boundary warrants an ADR.

## Compatibility and end-to-end evidence

| Boundary | Preserved behavior | Evidence |
| --- | --- | --- |
| Service / SQLite | Nil omits; supplied strings normalize; disjoint changes survive | Independent services/stores and deterministic joined barriers |
| PostgreSQL | Same contract across physical connections | DSN-gated behavior, advisory wait, row locks, defaults and rollback |
| REST PATCH | Pointer presence, existing status mappings | Registered Gin route, real DB, response and event |
| WS update | Pointer presence and existing error envelope | Registered dispatcher, real DB, response and event |
| Settings / boot / store | Complete drafts and existing upserts | Read-only payload/consumer inventory; no UI mutation |
| New task selection | Complete committed policy snapshot | Actual CreateTask plus persisted TaskRepository and task event |
| Existing task / runtime / prompt / PR | Immutable snapshots and raw fallback | Existing exact service snapshot control plus persisted old row |
| Legacy full store write | Intentionally supplied complete replacement | Existing exact store CRUD test |

## Tests and traceability

New names are planned test entry points, not claims that files exist or tests ran.
Use new focused files rather than growing already-large suites.

| Acceptance | Permanent evidence in Task 01 |
| --- | --- |
| AC-001.4, AC-001.9 | `TestBranchPolicyPatchValidationRollback`, invalid refs/templates/name/description, no event or changed timestamp; observed and racing case-insensitive name conflict |
| AC-001.7, AC-001.8 | `TestBranchPolicyPatchConcurrentEdits`: description/workflow both orders, bounded name/template/target mix; `TestBranchPolicyPatchControls`: null/empty/full/same-field/delete/read-only/foreign/deleted-parent |
| AC-001.10 | `TestBranchPolicyPatchTargetDefaults`: omitted vs blank with earlier concurrent base commit, whitespace, combined base+blank, create default control |
| AC-001.7, AC-001.9 | `TestBranchPolicyPatchHTTP`, `TestBranchPolicyPatchWS`: one omitted-field integration each, response/event matches full committed DB policy; failure publication control |
| AC-004.7, AC-001.5 | `TestBranchPolicyPatchTaskSnapshots`: actual new and pre-existing task rows, complete tuple and post-delete preservation; actual task event serialization |
| AC-001.7 through AC-001.10 | `TestBranchPolicyPatchSQLite`: independent writer handles, presence/defaults/atomicity/uniqueness/deletion; `TestPostgresBranchPolicyPatchBehavior`, `TestPostgresBranchPolicyPatchConcurrency`: independent physical connections and observed advisory wait before canonical read, row-lock interaction with legacy update/delete |

AC abbreviations in this table mean `AC-WORKSPACES-BRANCH-POLICIES-*`.
Service new files: `service_repository_branch_policy_patch_test.go` and
`service_branch_policy_patch_snapshot_test.go`. Handler new file:
`repository_branch_policy_patch_test.go`. Store new files:
`repository_branch_policy_patch_test.go` and
`repository_branch_policy_patch_postgres_test.go`.

Transport/store-only barriers can pause before invoking the real atomic store
method or observe real database locks. They must not replace business logic or
add production test hooks. Tests own deadlines, release/cancel and joins on all
failure paths. PostgreSQL tests use existing isolated database helpers and
explicit different backend connection IDs. Assertions cover the returned row
as well as persisted state. No Cartesian matrix or duplicate transport matrix.

## Mobile and public documentation

Mobile-parity exception: backend state/data only, no rendered composition,
touch, scrolling, navigation, breakpoints or localized copy changes. Registered
REST/WS and persistent task tests are end-to-end evidence at the affected
boundary. No browser launch, build or Playwright test is required.

Public audit found `docs/public/git-operations.md#named-branch-policies` and
`docs/public/tasks-and-workflows.md#branch-policies`; root README and screenshot
catalog contain no affected policy contract. During implementation add only a
small existing git-operations explanation/reference paragraph for partial
updates and omitted versus explicit blank target. Do not invent a public API
catalog, page, screenshot or manifest. Existing docs already cover immutable
snapshots. Public validators run only after that actual edit.

## Work orders

- [ ] [Task 01: Preserve atomic branch-policy patches and task snapshots](task-01-atomic-policy-patches.md)

One coherent sequential vertical slice: red real lost-update regression,
transaction/service correction, bounded store and transport proof, persisted
task proof, docs and delivery. Layers are not separate independently functional
work packages. No dependency requires another order.

## Verification and execution bounds

[Task 01](task-01-atomic-policy-patches.md#verification) owns exact commands and
order. Execute each heavyweight command separately and join every returned
handle to its actual terminal exit before the next test/lint/install. No replay
of the accepted root proof, repeated passed tests, broad suite, shard, bisect,
audit or polish. Run persistence SQL guard and conformance separately. Go uses
`-trimpath -tags fts5 -race -p=1`, `GOMAXPROCS=2`, `GOMEMLIMIT=512MiB`.
Lint uses concurrency 2, allow serial runners, CLI 5m and GNU hard 6m with 10s
kill-after. A nonzero exit with zero issues, timeout, crash or lost handle is
not a pass; retain receipt and request bounded parent recovery, no blind retry.

After the later release only: use Node 24.21 via explicit PATH with bash
login=false; if apps dependencies are absent, exactly one pinned pnpm 9.15.9
frozen apps install before normal hooks. No lockfile/cache edits. PostgreSQL
may use at most one proven-owned private instance with upfront directory or
container/mount/volume ownership receipts; never remove the old unproven
`2c48...` volume, foreign DB or daemon data. Prefer truthful DSN skip plus real
hosted behavioral evidence over an infrastructure marathon. Join DB test
handles and close connections before removing only owned resources.

## Delivery and completion gates (later release only)

Standing delivery authorization becomes actionable only after the later root
implementation release. Normal hooks commit/push/ready PR. Freeze head except
valid finding remediation. No moving-main rebase. Retain one `scripts/pr-await`
monitor and join it before replacement; no root timer polling. Mandatory fixup
lint uses full `./...` against one exact live PR base SHA, GOMAXPROCS 2,
GOMEMLIMIT 1GiB and the same hard/CLI limits before push.

Require authenticated configured CodeRabbit App `347564` substantive full review
of ALL changed paths at current head, source=covered=head/kind=reviewed. Inspect
completed automatic review before at most one necessary full request; no duplicate
or optional second wait, skipped incrementals, Claude acknowledgment or disabled
OpenCode substitute. Disposition actual findings; defer optional style without
head churn. Unrelated CI failure goes to parent with exact leaf log/artifact/source
before scope expansion, retry or weakening. No timeout/assertion/race/goleak
weakening or check/admin bypass. Merge by normal expected-head squash only when
actual terminal hosted gates and fresh policy/resolver evidence are clear.
Cheap final base/tree/blob compatibility checks are allowed; synthetic tests are not.

Completion is actual independently verified merged SHA/time, fetched merged tree,
owned blobs and remote-main inclusion, all handles joined and exact owned cleanup.
PR creation and local green are not completion. Leave clean managed worktree,
dependencies and shared caches for parent archive. Report only design handoff,
a concrete direction/recovery blocker, or actual merge/cleanup receipt to parent.

## Verification results and recovery checkpoint

Design checks passed: catalog validation (347 decisions / 1329 specifications),
spec lint --all, 36 spec-linter unit checks, actual four-path inventory with no
staged paths, exact REQ/AC/design/plan chain, and whitespace check. The repository
PR-docs API returned `exempt` for the actual documentation-only paths; its
frontmatter parser independently verified the one pending work order's complete
chain. Public source validation passed (47 published pages) and 62 public-validator
unit tests passed; no public source
was changed this turn. Node was existing 24.21.0, bash login=false, no install.
All calls returned terminal exits, no session handles were created.
Implementation release and permanent RED are recorded in the work order.
Final checks passed: scoped revision lint (handle 1721, zero issues), SQLite plus
actual PostgreSQL behavior/concurrency (65969, 14.060s), focused service including
real persisted task snapshots (82659, 2.199s), registered REST/WS controls (56756,
1.782s), SQLguard (24552) and store conformance with PostgreSQL (90805, 113.910s).
Every handle joined to terminal exit 0. Earlier expected RED and corrected
fixture/timestamp/lint findings remain recorded in the work order and platform
plan; no accepted root proof replay.

Exact owned PostgreSQL container and named volume were verified against labels
and mount receipts and removed after all DB handles joined. Receipt:
`/tmp/kandev-child21-pg-owner.json`. No old or foreign resource was touched.
Pinned conditional frozen install completed (handle 62949, joined exit 0):
Node 24.21.0 / pnpm 9.15.9, reused dependencies, no lockfile changes.
Catalog/spec lint/public source (47 pages), 62 public-validator tests and
whitespace checks passed. Normal hooks/publication follow.
Task 01 remains in progress until hosted checks, full current-head review,
actual merge and final clean-worktree verification. Durable external receipts
are appended to the platform plan without gratuitous published-head churn.

## Risks

- SQLite deferred reads before a writer lock can observe stale state; the first
  operation must acquire the writer lock. Independent pools need real coverage.
- PostgreSQL advisory serialization alone misses legacy writes/deletes; keep
  row locking and verify independent connections and actual lock observation.
- Per-policy locks do not serialize names across policies; DB uniqueness errors
  must retain conflict mapping and roll back the full patch.
- Normalization must execute on the complete locked row without service/storage
  import cycles, callbacks performing external work or post-commit rereads.
- PostgreSQL absence is a local coverage limitation; hosted new test execution
  must be proven before merge. Event delivery order is deliberately unversioned.
