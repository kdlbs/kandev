---
created: 2026-10-03
status: in_progress
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-002
system_design:
  - ../../specs/workspaces/system-design/branch-policies.md
legacy_specs: []
---

# Implementation plan: Serialize Gitflow starter admission

## Outcome and ownership

Restore one atomic repository-scoped Gitflow admission decision, four committed
starter rows and truthful typed conflict results through the service and
registered REST/WS transports. Workspaces owns reusable repository policies and
their initialization, so this package reuses the existing
[requirement](../../specs/workspaces/requirements/branch-policies.md) and
[design](../../specs/workspaces/system-design/branch-policies.md#atomic-starter-admission).
Clarify AC-WORKSPACES-BRANCH-POLICIES-002.4 by admission order and add the missing
observable transport/event criterion 002.6. Preserve the broad pair's draft
status, original feature history and just-merged patch ownership. No new ADR is
needed to apply established transaction primitives to the existing contract.

## Phase barrier and identity

<!-- kandev-system: task=c9711fb9-259e-447e-ae5c-dc92e4c27a6b session=96357481-e114-4ff7-abe4-f140023ee782 parent=14825981-b175-411d-999a-31ddc2aa5fc3 -->

The design turn ended with exactly four uncommitted artifacts. ROOT reviewed
the complete package and explicitly released implementation in this SAME
primary session. Task 01 is in progress; normal delivery through actual merge
is authorized. No delegation, new agents/tasks/tabs/sessions,
model-switch or operator approval prompt. Keep title **Serialize Gitflow starter
admission**, selected Sol6.1 profile, marker/IDs, user edits and question barriers.
Critical unsafe blockers use the parent-question tool as the final action;
no tool or work follows it. Child-to-parent messages use queued/omitted delivery.
Hardware crash immunity is not promised; checkpoint and recover this session.

## Accepted evidence

Actual clean base `9b6328210ff5c6098727d344ac7e7754495f516b`.
`CreateRepositoryBranchPoliciesIfEmpty` begins a transaction and counts rows
before any admission lock. Two real independent PostgreSQL initializers can
both observe zero; the loser then returns a raw unique-name SQLSTATE `23505`.
Only `ErrRepositoryBranchPoliciesExist` becomes the service already-seeded
error and registered conflict mapping. Four winner rows remain intact.

Read-only parent archive `/tmp/kandev-gitflow-admission-repro_test.go` was
inspected and accepted, never replayed. `TestRootGitflowAdmissionProof` used the
real store and three observed distinct backend PIDs. A worker-only Feature
insert trigger delayed the loser after its empty check; another starter with
different values committed four. Parent handle `34571` joined exit `1`, test
2.39s/package 2.433s: sequential custom-policy control PASS, only concurrent
loser's typed-conflict assertion RED. The earlier five-row ordinary overlap
is not evidence of corruption or forbidden serialization.

Parent's exact bounded PG16-alpine container
`6da0bbfdbee6da5aa1eb4e49734b10c27cf8d951072b60f5a3586dc2356e38fa`
and private bind data were removed, zero anonymous volumes; inspected receipt
`/tmp/kandev-root22-fixture-cleanup.json`. No root handles/DB remain. Do not use
foreign databases or the unproved child20 volume `2c48e791...`.

## Scope

- One cohesive conditional starter operation, repository admission shared by
  ordinary inserts, canonical empty decision, all-or-none four inserts and
  narrow typed conflicts. Preserve service Git validation/defaults/tuples,
  authorization, repository scope, read-only behavior and failure rollback.
- Real independent-store SQLite/PG evidence, actual PG wait before canonical
  read, both legal starter/custom-create orders and repository isolation.
- Real service and registered REST/WS responses, winning rows and after-commit
  events; bounded failure controls and existing patch/CRUD compatibility checks.
- Minimal existing public paragraph clarification after implementation.

Exclude UI draft/CAS/copy/layout, browser/E2E/build, task snapshots, prompts,
worktree/runtime changes, branchPolicyPatch redesign, legacy write/delete
redesign, schema/history/migrations/ETags/global event revisions, general
concurrency/cancel/retry engines, moving-main rebase, synthetic compatibility
tests, new docs pages/manifests and infrastructure marathons.

## Technical approach and producer inventory

Production inserts occur only in
`apps/backend/internal/task/repository/sqlite/repository_branch_policy.go`:
`CreateRepositoryBranchPolicy` (ordinary) and
`CreateRepositoryBranchPoliciesIfEmpty` (starter). Service methods in
`service_repository_branch_policies.go` and both registered handler producers
converge there; direct store callers also participate. `repository.Provide`
returns concrete `sqlite.Repository`; the existing interface remains sufficient.
Reconfirm this inventory after release before editing.

1. Use one narrow helper for transaction admission with namespace
   `repository-branch-policy-admission:` + repository ID. PostgreSQL acquires
   `pg_advisory_xact_lock(hashtextextended($1, 0))` before COUNT/insert. SQLite
   obtains writer admission with repository-scoped no-op policy UPDATE before
   predicate reading, including zero rows. Never use the reader pool for this
   decision. Ordinary creation becomes a transaction around the same admission
   plus its single insert, without a create-if-empty predicate.
2. Starter checks the canonical current count and rejects any positive count
   with existing existence sentinel; otherwise insert the four service-built
   policies and commit. Capture successful stored identities/timestamps with
   existing transport precision; use transaction-bound capture only if needed,
   never a post-commit reader reconstruction. Keep ID/time/default semantics.
3. Reuse `isBranchPolicyNameConflict` with a typed unique-constraint guard for actual named uniqueness collisions
   (ordinary names may collide after admission). Keep name-conflict distinct
   from already-seeded; do not map every constraint/SQL/cancel error to either.
   Preserve foreign-key/parent deletion and rollback behavior. No retry loop.
4. Keep `PatchRepositoryBranchPolicy` on its existing per-policy advisory key
   and row lock. Full updates/deletes do not insert rows. No causal evidence
   justifies changing their lock ownership or semantics. Retain current service
   normalization and registered handlers. Their existing typed conflict path
   needs no new public transport schema.
5. Verify the complete vertical result with real stores, service/Git and
   registered Gin/WS dispatch, then clarify existing public docs and deliver
   after task-defined checks. Existing original and patch plans stay historical.

## Compatibility and end-to-end evidence

| Boundary | Contract | Evidence |
| --- | --- | --- |
| SQLite independent pools | Writer admission before count; atomic inserts | Real separate pools on one private file, both admission orders and rollback |
| PostgreSQL independent PIDs | Repository advisory wait before count; fresh read after release | Three distinct physical PIDs, real lock observation, different repositories proceed |
| Starter vs starter | One four-row winner, typed loser, winning branch tuple | Distinct production/development pairs, returned and stored rows |
| Starter vs ordinary create | Custom-first rejects starter; starter-first permits distinct fifth row | Actual writers in controlled admission orders, normal uniqueness/default controls |
| Service / Git | Existing defaults, refs, membership, read-only | Real isolated Git repo, actual service validation and no-event failures |
| REST / WS | Created response vs meaningful conflict | Registered POST/dispatcher, real service/stores, rows and four after-commit events |
| Patch / legacy writes / deletion | Existing ownership and field behavior | Exact existing patch/CRUD controls; no redesigned locking |

User-visible outcome is covered end to end at registered transport/storage
boundaries. Mobile exception: backend state/data only; no rendered surface,
navigation, copy, touch, scrolling or breakpoint changes. No browser or E2E.

## Tests and traceability

All named entry points now exist in focused permanent test files. Keep each test
files under the 800-effective-line limit; do not add production hooks.

| Acceptance | Planned tests | Outcome |
| --- | --- | --- |
| AC-002.3, .4 | `TestGitflowAdmissionSQLite`, `TestPostgresGitflowAdmissionConcurrency` | Independent stores, different starter pairs, typed loser, four winner tuples; custom-first one row, starter-first distinct custom fifth row |
| AC-002.4, .6 | `TestPostgresGitflowAdmissionBeforeRead` | Actual advisory wait before read; holder commits ordinary policy, starter rejects from fresh count; cancellation releases lock |
| AC-002.3, .6 | `TestPostgresGitflowAdmissionBehavior`, SQLite test controls | Insert failure rolls all rows back, missing/deleted parent, named uniqueness vs unrelated errors, later valid work succeeds; different-repository PG progress |
| AC-002.1, .2, .3, .6 | `TestGitflowAdmissionService`, `TestGitflowAdmissionControls` | Real Git defaults/trim/existing remote refs/different pairs; validation, scope, foreign/missing parent and read-only/no events |
| AC-002.3, .4, .6 | `TestGitflowAdmissionHTTP`, `TestGitflowAdmissionWS` | Concurrent registered requests with distinct pairs, winner response/stored tuple/four created events after commit, loser 409/WS conflict and no loser events |

AC abbreviations mean `AC-WORKSPACES-BRANCH-POLICIES-*`.
Store test files: `repository_branch_policy_admission_test.go` and
`repository_branch_policy_admission_postgres_test.go` beside the store.
Service test file: `service_repository_branch_policy_admission_test.go`.
Handler test file: `repository_branch_policy_admission_test.go`.

Use actual lock holds/observed waits and bounded fixture-only SQL observation
or faithful transport-store barriers; no sleep-based scheduling, serialized
pool falseproof, business-logic substitutes or parent proof replay. Permanent
RED must assert changed behavior (fresh admission/conflict/rows), not only SQL
spelling. Reuse existing PG isolated database, second connection and wait
helpers; all worker contexts have strict deadlines and cancel/release/join
cleanup on every failure path. Retain concurrent event observations safely.
Avoid a Cartesian repetition of all controls through every layer.

## Public documentation audit

`docs/public/git-operations.md#named-branch-policies` owns the starter paragraph.
The existing paragraph now clarifies empty-set admission, competing starter
conflict and permission to add custom policies afterwards.
This is explanation/reference copy. `tasks-and-workflows.md#branch-policies`
already covers selection/snapshots and needs no unrelated rewrite; root README
and screenshot catalog contain no affected contract. No public edit in design.

## Work orders

- [ ] [Task 01: Serialize repository starter admission and transport outcomes](task-01-serialize-starter-admission.md)

One sequential vertical work order, no dependencies. Store/service/registered
transport outcomes form one independently verifiable repair.

## Verification and execution bounds

[Task 01 verification](task-01-serialize-starter-admission.md#verification)
owns exact anchored selectors and commands. After release: meaningful permanent
RED, smallest correction, scoped GREEN, SQLguard and conformance as separate
checks. One local heavy command at a time, each in a separate call; retain and
actually join every session handle before another command. Go uses
`-trimpath -tags fts5 -race -p=1`, GOMAXPROCS 2, GOMEMLIMIT 512MiB.
Scoped lint concurrency 2, allow serial runners, CLI 5m under GNU hard 6m/
kill-after 10s, GOMAXPROCS 2/GOMEMLIMIT 1GiB. A timeout/lost/nonzero-with-zero-
issues receipt is not PASS and must not trigger an automatic retry.

At most one practical proven-owned PG fixture, bounded 2 CPU/512MiB, private
bind data preferred. Capture exact container/labels/mount/volume ownership
upfront and register cleanup immediately. Join connections/goroutines/commands
before removing only proven-owned resources. No foreign/root/old-volume use.
An absent DSN is a truthful skip; require hosted execution receipts for actual
new PG tests before merge, not merely boot/skip logs. No infrastructure marathon.

After release only, missing `apps/node_modules` requires one pinned pnpm 9.15.9
frozen install from apps with Node 24.21 explicit PATH, bash login=false, before
hooks. Preserve lockfile/shared caches. Design lightweight docs checks need
no install. Keep exact handles, terminal exits and next action in platform plan.

## Delivery and completion gates (after later release)

Reviewed release authorizes normal-hook commit/push/ready PR and full delivery
in this primary session without operator reauthorization. Freeze published SHA
except actual valid findings; never rebase for main advancing. Retain one
`scripts/pr-await --mode all-terminal` monitor and join before any necessary
replacement. No manual timer polling. Actual backend PR fixup requires one full
CHANGED `./...` lint at exact live PR base, same bounds, before push; no automatic
retry or inferred pass.

Require authenticated configured CodeRabbit App `347564`, Organization UI/
QUIET/Advanced, substantive FULL ALL actual changed paths at CURRENT HEAD with
`sourceCommitId=coveredCommitId=head`, `kind=reviewed`. Inspect completed automatic
report before at most one necessary full request; no duplicate request, optional
second Claude wait, acknowledgment/skipped review or coverage substitution.
Classify/disposition all actual threads and grouped findings. Defer optional
style without head churn. Unrelated CI requires exact leaf/source/artifact
evidence and bounded ROOT direction before expansion/retry/assertion/timeout/
race weakening. No admin, hook or check bypass.

Completion only normal expected-head squash actual MERGED SHA/time plus
independently fetched merge tree, all owned blobs and remote-main inclusion,
all owned handles joined and proven-owned cleanup. Preserve clean managed
worktree/deps/shared caches for ROOT archive; parent proof remains read-only
until root post-merge cleanup. Messages only concrete design handoff, blocker/
recovery needing direction, or actual merged+joined cleanup. PR publication or
local green alone is not completion.

## Verification results

Design checks passed with terminal exit 0: catalog validation (347 decisions,
1329 specifications), specification lint --all, all 36 spec-linter unit checks,
whitespace, all relative artifact links and exact anchored selector inventory.
Existing control test names exist; eight new entry points are explicitly planned.
The actual four documentation paths are `exempt` in the repository PR-docs
`validateCoverage` API; its frontmatter parser independently proved the
REQ/AC/design/plan chain and exactly one pending sequential work order. No
fictional triggering paths were supplied. Existing Node 24.18.0 ran this
lightweight API check with explicit binary path and bash login=false, no install;
the later Node 24.21 dependency/hook constraint remains unchanged.

The design checkpoint ended with exactly four uncommitted artifacts and no
implementation work. ROOT subsequently reviewed and released the package.
Permanent PostgreSQL RED used independent physical connections: the old worker
returned success before the held admission lock and left five rows instead of
the holder's one row and typed existence conflict (joined handle 86327, exit 1).
The parent proof was never replayed. An additional SQLite trigger-message RED
proved that arbitrary constraint text must not become a typed name conflict
(joined handle 21121, exit 1); the insert-only typed guard fixes it while leaving
patch classification ownership unchanged.

Scoped race-enabled store checks passed on real SQLite and PostgreSQL, including
existing patch/CRUD/deletion controls. Final new-store run joined handle 97082,
exit 0 (10.548s), with actual independent PID/wait observations and both legal
ordinary admission orders. Final new-service run joined handle 99029, exit 0
(1.736s). Registered transport controls previously passed (83158, exit 0);
final registered transport run joined 39896, exit 0 (1.432s).
SQLguard passed separately (12267, exit 0), and SQLite/PG storeconformance passed
separately (19364, exit 0, 119.587s). These persistence checks preceded the final
insert-only error-classification refinement, which changed no SQL or transaction
ordering. Earlier fixture-only soft-deletion and canceled-connection search-path
assumptions were corrected; affected tests then passed (30173, exit 0).

Public documentation validation passed (62 validator tests and 47 pages), and
the actual implementation diff is covered by the repository PR-docs API using
real changed paths. Requirements and design retain their broader draft status.
All local implementation checks passed. Delivery remains in progress through
normal publication, required hosted gates/review and independently verified merge.

## Risks

- COUNT before writer admission recreates the race; independent pools/PIDs and
  actual waits must prove the boundary rather than timing luck.
- Repository lock keys must be distinct from per-policy patches, with no new
  nested lock order. SQLite's single-writer scope is inherent, not a promised
  different-repository concurrency capability.
- Overbroad uniqueness mapping hides actual failures; starter existence and
  ordinary name conflict must remain distinct.
- Event assertions must use safe collectors and prove post-commit rows, not
  inferred publication or a global event-order guarantee.
- PG skips leave material coverage outstanding until hosted actual tests run.

Owned PostgreSQL fixture cleanup completed after all test handles joined: exact
container `d5d87c9f105bc60dbd49c39e46b05f544d4ebe631f2876bc74d31a9323e559ae`
and its single private bind data directory were removed, zero anonymous volumes.
Receipt: `/tmp/kandev-child22-pg-owner.json`. One frozen dependency install
with Node 24.21.0 and pnpm 9.15.9 joined handle 57016, exit 0; lockfile unchanged.

Initial scoped lint joined handle 19446, exit 0, zero issues, with concurrency 2,
allow-serial-runners, CLI 5m and GNU hard 6m/kill-after 10s under GOMAXPROCS 2
and GOMEMLIMIT 1GiB. It included all staged new files and compared the actual
accepted base `9b6328210ff5c6098727d344ac7e7754495f516b`. Final catalog/spec
lint, whitespace and actual eleven-path documentation coverage also passed.
Normal publication and hosted review/check gates remain external pending work.
