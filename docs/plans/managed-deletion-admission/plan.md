---
created: 2026-10-04
status: in_progress
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-013
system_design:
  - ../../specs/plugins/system-design/managed-deletion-admission.md
legacy_specs: []
---

# Implementation plan: Guard managed deletion against stale revisions

## Overview

Protect installation-owned retained conversations against delayed exact
deletion across independent services. One coherent sequential vertical work
order owns existing-API RED, the selected typed lifecycle/native admission,
participating writers, actual Host receipts, cleanup observations, and native
SQLite/PG16 evidence. ROOT reviewed the selected four-artifact package and issued
the later explicit implementation release in this same primary. The one order
is in progress; local validation passes, while publication and verified merge remain outstanding.
Current phase is implementation under ROOT's later reviewed DB-boundary release.
Local acceptance checks pass; hosted review and verified delivery remain pending. The single order is in progress.

Four owning documents comprise this package, plus the explicitly
authorized [accepted shared-DB ADR](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).
The existing Platform persistence design is reconciled, not duplicated. The existing
Plugins managed-coordination owner remains authoritative for broad behavior;
its 32,687-byte design justifies the focused owning supplement. Broad draft
statuses, earlier atomic-settings delivery, and original coordinator plan/history
are preserved. The DB ADR is the authorized exception to the earlier four-file
inventory. No second work order, production plugin, schema, wire, auth,
UI, model switch, delegation, task, tab, or session is introduced.

## Evidence and assumption check

Confirmed: expected revision and current retained ownership must govern actual
deletion admission, stale/detached winners retain transcript and settings,
rejected attempts have no destructive lifecycle effects, accepted removal uses
real host lifecycle, and independent SQLite/PG16 and registered Host evidence
are required. Verified: service-local locks and a task-ID-only lifecycle call
leave a race; current lifecycle mutates canvas/environment before final deletion.

Authoritative main/proofbase and initial clean HEAD:
`d803d6f209030789d14751db90c5e9eaf5ba74a6`.
Immutable read-only proof `/tmp/kandev-managed-delete-revision-repro_test.go`,
SHA256 `7a8fce63f7b8d953199aae9d2312b49dffd737a6784c3498264196112de12866`,
receipt `/tmp/kandev-root29-managed-delete-proof-receipt.json`.
Accepted ROOT joined handle 73573 exit 1, test .31s/package .378s, without replay.
`accepted_update_after_check` removes revision 2 after delayed revision-1 check;
`detach_after_check` removes independently detached retained task. Current-delete
and already-stale controls pass. Earlier joined 88460 was a diagnostic wrapper
type mismatch (`*sql.DB` vs `*sqlx.DB`), not behavioral evidence. Temporary
repository proof was removed by ROOT; existing workers and DB handles were joined.

Resolved by ROOT's design-only INTERRUPT: committed exclusive admission
authorizes and excludes competing writers before preparation; physical deletion
is a separate commit. Clean rejection applies before exclusive admission only.
Admitted failure may leave baseline environment/canvas effects while task,
session and transcript rows remain. Canvas failure still aborts removal; no
postcommit canvas engine. A prepared job remains reversible/non-runnable, and
release is owner/state/operation fenced. The selected
[design](../../specs/plugins/system-design/managed-deletion-admission.md#selected-boundary-and-compatibility-decision)
records the contract and conservative restart limitation.

## Scope and technical approach

1. Extend the managed repository with typed `DeleteRequest`/`DeleteClaim` and
   native admit/inspect/finalize/release methods. Validate current identity,
   revision, detach, hierarchy and cleanup authority atomically before inserting
   the PREPARED exclusive owner row in existing cleanup storage. Add versioned
   `managed_delete` snapshot envelope, invocation owner token and phases; no
   schema or wire change. Unsupported typed lifecycle support is unavailable.
2. Wire real `Service.DeleteManagedConversationTask` via `SetTaskDeleter(taskSvc)`.
   Factor/reuse lifecycle preparation and postcommit work; acquire admission
   before effects, inventory after it, owner-aware borrower transfer, real canvas
   preparation, and owner-CAS update of the SAME prepared cleanup snapshot.
3. Native finalization reuses `DeleteTaskWithVacatedStep` hierarchy/step/task,
   prompt/queue/session deletion logic and atomically records snapshot `deleted`
   in that transaction. Reconcile known rollback/commit/unknown separately;
   existing activation and worker attempt machinery remain the cleanup engine.
4. Exclude foreign owners in native managed ensure/state, retained full-row
   replacement while claimed, hierarchy child/reparent, session/runtime admission
   and cleanup transition paths. Early owner reservation for ordinary lifecycle
   deletion of retained managed rows prevents racing unowned preparation; other
   ordinary/legacy behavior remains baseline. Necessary typed helper extensions
   are in task/environment/session/executor/cleanup native boundaries, not
   provider cleanup implementations.
5. Recognize admitted/uncertain typed errors before Host generic status mapping.
   Return existing Unavailable with precise reasons, keep receipt incomplete,
   and replace incomplete-replay absence inference with committed-operation
   evidence. Same-operation cancelled reuse revalidates current rows/new owner;
   foreign live PREPARED ownership is never stolen by retry or startup cutoff.

PG uses READ COMMITTED hierarchy/workspace -> step -> task -> cleanup owner ->
session/resource locks and reads current rows after physical wait. SQLite uses
factory-owned immediate BEGIN before predicate reads under the accepted DB
extension; the hierarchy helper remains. Injected deferred pools fail safely
BUSY/Unavailable and do not inherit a physical-wait guarantee. SQL locks end before external
IO. Proven rollback releases own barrier under bounded WithoutCancel; commit
marker activates exact complete snapshot; unknown retains a safe barrier.
Restart recognizes marker evidence, not task absence or age as owner authority.

| Boundary | Required behavior and evidence |
| --- | --- |
| SQLite independent services/pools and Store | Both revision and detach winner orders; reservation/current read/cancellation/rollback |
| PostgreSQL 16 independent physical connections | Actual PID/lock waits, READ COMMITTED, current-after-wait, cancellation and final deletion |
| Registered Host and SQL CommandStore | Existing SDK/protocol command, real authority/service, durable statuses/replays/ack failures |
| Local/worktree, Docker, SSH, Kubernetes cleanup | Existing resource handle/lifecycle adapters preserved; rejected calls never invoke destruction |
| Unsupported native admission | Typed Unavailable before effects, no bare-delete fallback |

## Tests

Every criterion .1 through .8 maps to the real tests and observations in the
single work order. New proof is permanent existing-API RED before correction;
ROOT's immutable archive is never recopied or replayed as a surrogate. Test
gates must reflect the ROOT-selected actual admission boundary. A competing
winner is never synchronously awaited after deletion already holds its authority.
Resource, event, task, primary-session, transcript, and Host receipt observations
are mandatory; fake business predicates are not evidence.

No local full browser/E2E/build. There are no rendered outcomes to test through
a browser; backend Host production-path evidence covers this API behavior.

## Work orders

- [ ] [Task 01: Guard managed deletion](task-01-guard-managed-deletion.md)
  (`in_progress`, sequential, no predecessor; ROOT reviewed implementation release
  received).

## Documentation and phase gates

Run catalog validation, all 36 specification-linter tests, full spec lint, actual
repository `validateCoverage` for the four documents and planned production
trigger paths, frontmatter REQ/AC/design/plan cross-reference validation,
whitespace, and the four owning-document plus ADR inventory. ROOT reviewed and
released the DB extension. Product tests and the conditional install follow
only the reviewed serial resource limits; public/pure-data mobile audits remain.

Implementation is current and incomplete. ROOT's callback queue is full: no
message/question/routing retries. Any resource or missing-seam/material design
blocker requires an exact durable checkpoint and WAITING for ROOT's bounded
decision. Do not mark premerge work complete or infer actual merge from local
validation/publication.

## Standing delivery constraints after later DB-boundary release

One local heavy command at a time; retain and actually join every returned
handle before dependent work or another heavy command. Anchored Go tests use
`-trimpath -tags fts5 -race -p=1`, GOMAXPROCS=2/GOMEMLIMIT=512MiB. Install pinned
pnpm 9.15.9 frozen dependencies once from apps only if absent, existing Node PATH
and non-login bash; no lockfile/harness/cache edits.

Mandatory original installed full `./...` changed-revision lint against the
actual immutable PR base uses `--concurrency 2 --allow-serial-runners --timeout
10m`, GOMAXPROCS=2/GOMEMLIMIT=1GiB and GNU `timeout -k 10s 11m`. Record independent
original binary/hash/PID/duration/exit receipts; hooks/scoped checks do not
replace it. Resource timeout/lost handle requires an exact durable checkpoint
and WAITING; no resource retry, wipe, foreign kill, or assertion weakening.

PG16 setup requires upfront exact own ID/labels/immutable image/all mounts and
volume/credential receipts, private schema/tmpfs, no anonymous volume. Join all
clients before cleanup only proven own resources. Foreign volume
`2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`
is forbidden and untouched. PG16 ownership, native waits and full conformance were validated with joined, proven-only fixture cleanup.

Normal Conventional hooks remain active, no bypass/amend. Ready PR, live body
readback, exact local/remote/upstream/PR SHA, then freeze published head except
causal required fixes; no main-only rebase or synthetic merged replay. One
retained 90-minute all-terminal `scripts/pr-await` must be joined before
replacement, no duplicate/timer GitHub polling or hosted cancel. Inspect
completed semantic reviews during CI. Accept authenticated CodeRabbit App347564
substantive full current-head coverage of all actual files; explicit gaps allow
at most one necessary full new-head request. Disposition every real finding;
no optional review/settings churn. Unrelated hosted failure requires exact
job/log/artifact/source checkpoint and WAITING before scope expansion/rerun.

Normal expected-head squash requires actual terminal required policy and full
current semantic review, no hidden/actionable errors, no admin/bypass. Verify
MERGED/time/SHA/parent/tree/all owned blobs and authoritative remote inclusion
independently. Published head differs from merge SHA. Completion requires actual
verified merge, every handle joined, only owned cleanup, and clean managed
worktree; ROOT owns proof verification/archive and next child.

## Verification results

Design-only checks passed: catalog validated 348 decisions and 1,343
specifications; all 36 specification-linter tests passed; full specification
lint passed. Actual `.github/scripts/pr-docs.cjs` `validateCoverage` returned
`covered`, zero errors, for the revised four draft documents and 18 verified
planned production/public-reference trigger paths. REQ/AC/design/plan frontmatter
and reference ordering passed that real evaluator. This proves documentation
linkage, not implementation correctness.

Verified all 48 named existing compatibility tests against actual source; nine
additional names are explicitly proposed. Every named test appears in exact
future verification commands. Exactly four new unstaged files, no tracked/index
changes, and untracked-file whitespace checks passed. The evaluator used
existing Node v24.18.0 in non-login bash without installation. Focused design
is 28,236 bytes, within 32 KiB; broad draft/history remain unchanged.

The preceding results describe the completed design checkpoint. Current
implementation results are recorded below. Immutable proof remains read-only
and was hash-verified again without replay.

## Risks and explicit limitations

- Existing prepared cleanup reservation cannot prove irreversible deletion.
- Admitted environment/canvas preparation can have partial effects on failure;
  rollback of those effects is explicitly not promised.
- Final native deletion must not bypass hierarchy, resource preservation,
  queue/prompt purge, or postcommit notification behavior.
- Existing incomplete Host replay absence inference needs a causal narrowing:
  only persisted operation deletion evidence proves already applied.
- Existing cleanup state CAS/startup age do not prove live managed owner death.
  Unproven PREPARED ownership remains fail-closed on restart; no new lease or
  automatic force-recovery promise. If implementation cannot fence the typed
  snapshot transitions, checkpoint the exact missing seam instead of expansion.
- Public reference wording in `docs/public/plugins-authoring.md` must explain
  pre-admission rejection, admitted partial effects/Unavailable, and safe replay
  at implementation. No UI/mobile/localization/README/screenshot change.


## Current implementation results

Factory RED failed behaviorally before correction: all three entry modes read
revision 1 while an independent writer held accepted changes. After supported
writer-factory correction, four factory tests pass, including actual BEGIN
waiting/current-after-wait, cancellation settling while the holder remains
locked, failed-entry/rollback pooled reuse and dedicated reader progress.
Native conformance and actual independent SQLite admission/final-delete waits
pass for revision, detach, retained identity and cancellation. Cancellation while
locked returns real BUSY after the configured approximately five seconds; no
instant-cancel guarantee. Handles are joined; no live owned resources remain.

Original permanent existing-API RED preceded lifetime correction; ROOT's proof
was not replayed. Debug receipts/history and exact command results live in the
external plan. Current service, registered Host, independent PG16, affected compatibility,
SQL guard and full SQLite/PG store conformance checks pass with joined handles.
The original conformance attempt failed when its owned 256 MiB PG tmpfs filled;
ROOT authorized one bounded 2 GiB replacement and the unchanged command passed.
Both exact fixtures were removed after all clients joined. Public plugin
reference and backend transaction guidance are updated. Final documentation
validation and the original full immutable-base lint pass. Small native and
lifecycle phase helpers preserve current validation, transaction ownership,
attachment release and effects order after the reported complexity diagnostics.
Publication, current-head hosted review and verified merge remain pending;
the single order stays in progress.
