---
id: 03-round-robin
title: Committed task/session round-robin
status: in_progress
wave: 3
depends_on: [02-cost-random-preview]
plan: plan.md
requirements:
  - REQ-AGENTS-ORDERED-TIERS-001
acceptance_criteria:
  - AC-AGENTS-ORDERED-TIERS-001.4
  - AC-AGENTS-ORDERED-TIERS-001.6
  - AC-AGENTS-ORDERED-TIERS-001.7
system_design:
  - ../../specs/agents/system-design/ordered-profile-tiers.md
legacy_specs: []
---

# Task/session round-robin work order

Extend the existing tier ranker and attempt owner. Add one nullable BIGINT
`selection_ordinal` to `dynamic_route_attempts` and a profile/ordinal index after
the idempotent ADD COLUMN migration. Preserve UUID identity, original history,
legacy NULL values and task-session FKs. All newly committed task/session route
attempts receive profile-local MAX+1 in their existing decision transaction,
including other modes. PostgreSQL reuses its existing transaction advisory lock
pattern before ordinal read-modify-write. Rollback and stale CAS consume nothing.

Within the existing Engine mutex, read positive committed ordinals only when a
profile uses RR, rank each tier after its last selected current member, then use
the existing final eligibility/probe/CAS/commit. No valid ordinal means the first
eligible row in saved order. Reorder changes adjacency, not history identities;
disabled or blocked last members remain valid anchors. Do not use LRU, frequency,
timestamps or UUID order. Propagate query failure. Preview uses the same read and
ranking without runtime/probe claims or writes. OFF/order/cost/random do not gain
an RR history-read dependency. Preserve preferred retry and durable no-revisit.

Expose RR and its exact task/session scope in the existing editor and registry.
Preserve unknown modes and nested metadata. Do not add a cursor/table, new engine,
queue, poller, fingerprint or permanent tests. Root owns Git and integration.

## Scope and original remainder

The Engine mutex serializes selection through commit within one engine. The PG
transaction lock serializes ordinal allocation, but two independent backends can
rank the same last choice before either commits; distributed choice is not
claimed. Legacy NULL history is retained without causal backfill. Utility,
review and taskless Office synthetic routes are currently excluded from durable
attempt writes because that owner has a task-session FK. Their common RR owner
bridge remains required original work, alongside pace and keep-model behavior.
No synthetic task session, FK removal or alternate ledger is introduced.

## Verification plan

Use the existing disposable native tier probe area outside collectors. Prove
real SQLite migration/replay and committed ordinal allocation, rollback/stale
CAS, tied/backwards clocks, engine restart/current reorder, preferred retry and
eligibility/chain retention, read failure propagation and read-only preview.
Reuse existing affected native checks and schema/sqlguard tooling. No real model
or operating DB/profile changes. Record exact commands and raw paths after the
shared heavy-execution slot is released. PostgreSQL runtime evidence requires an
actual test DSN; absence must not be reported as a pass. Preserve known Windows
store-conformance and native Go budget limitations accurately.

## Handoff implementation verification

Preparing all tier rows eagerly made an earlier eligible order tier depend on
an unreached RR history query. The disposable actual Engine case reproduced
history unavailable blocking candidate A with one history read and no append
(RED, dynamic 0.023s). Ordered traversal now yields candidates lazily and reads
history only on reaching an RR tier, sharing one observation across reached RR
tiers. Successor traversal propagates that error instead of treating it as an
empty candidate list. Both boundary cases passed under race (GREEN, 1.047s).

Original RR selector plus the lazy-history cases then passed under race in
dynamic 1.050s. The existing SQLite sequence/reorder/restart/read-only history
cases completed, but its migration replay case failed at an unrelated defaults
backfill: the old disposable rrSession created a task with empty workspace and
no workflow, so replay attempted to write NULL to tasks.workspace_id. The
collector-excluded fixture copy now supplies the existing default workspace;
no production migration or original evidence was changed. Only that failed
case was rerun: race PASS 1.526s, two schema replays preserved legacy NULL and
task/history, failed insert rolled back generation/ordinal, stale CAS and duplicate
append consumed no ordinal, and the order-mode commit received ordinal3.

No full repository suite or PostgreSQL DSN run is claimed. The prior original
utility/review/taskless owner bridge, pace/keep-model and distributed choice
remainders above are unchanged. Final normal hooks and independent source review
remain separate from these selected behavior checks. No operating model order,
profile, feature activation, database or runtime setting was changed.

The first normal commit check reported one cyclomatic limit in prepareOrderedPlan
(17, limit15). The existing retained-chain validity/deep-copy logic is now a pure
helper with identical values. Only the affected RR Engine cases were repeated:
race PASS1.045s. Existing normal format/web lint/i18n checks had passed; final
normal commit still owns complete hook acceptance.

## Native owner bridge implementation

The actual utility-call probe fails the task-session FK without fabricating a
session. Extend the existing two route tables with nullable native `route_id`;
keep the original `session_id` FK for actual task sessions. Each row has exactly
one identity. Logical route identity is COALESCE(route_id, session_id), unique
for state and generation, with the same immutable attempt UUID and ordinal.
A transactional replayable SQLite table cutover preserves every original field;
PostgreSQL changes only the identity constraint/nullability and retains the FK.
Native initial decisions validate the actual utility call, review run or Office
run session inside their existing decision transaction. Unknown/missing owners
fail closed without consuming generation or ordinal. Existing legacy utility:
routes remain transient while the feature is off. Native rows are excluded from
task-session startup recovery; their native owners retain recovery authority.

Create the actual utility call before durable profile selection only for the
ordered-tier native-owner path, then update concrete attribution in that same
existing call. Classified fallback retains the route owner. Review and taskless
Office use their actual run and run-session identities, not synthetic sessions.
Preserve existing feature OFF, task generation and caller cancellation semantics.
No alternate cursor, counter, ledger or permanent tests. The two existing tables
remain the sole route history. No operating database migration or activation.


## Native owner verification checkpoint

Actual native utility-call ownership failed before the bridge with the original
FOREIGN KEY constraint (RED0.356s), then passed using the same existing attempt
ledger, A/B and ordinal1/2, with zero fake task sessions (race1.847s).
The actual Service -> native ProfileExecutionResolver -> SQLite path passed
race1.430s: exactly one real call per preparation, correct concrete model/history,
feature-OFF transient behavior, and missing/unknown owner rejection with no
state/ordinal consumption. Its first temporary input omitted the required routing
policy version/document and was rejected before selection; the input was fixed,
not production policy validation. Existing affected utility/service, route-state,
claim/attempt/history and legacy migration checks passed in the selected run.

A disposable legacy-shape cutover probe preserved original UUID, NULL ordinal,
continuation/policy and real task FK/cascade across two replays. A deliberately
invalid legacy fixture failed the attempt copy after state copy, rolling back the
entire schema/data cutover. The actual reserved Office-run-session -> native
resolver -> original attempt ledger passed race1.830s, and task startup recovery
returned no native starting rows. No permanent tests or fixture fingerprints.
SQL guard passed. Normal hook and store-conformance outcomes remain separately
recorded; PostgreSQL actual execution is unavailable without a test DSN.
Review caller composition, distributed choice, pace and keep-model remain open.

Mandatory store-conformance returned exit1 twice (52.632s, detailed53.038s).
The actual failures are unchanged storage_temp_artifacts fixtures using /tmp/id,
rejected on Windows as non-absolute paths: CRUD/timestamp/conflict/transaction
and the same storage scenario in previous-stable upgrade. Task-owner fresh,
replay, CRUD, boolean, timestamp, conflict and transaction all passed. No storage
fixture or validation change was made. PostgreSQL scenarios were skipped without
a DSN. The full JSONL receipt is retained in the source manager task workspace;
this is not a whole store-conformance PASS or a PostgreSQL execution claim.
