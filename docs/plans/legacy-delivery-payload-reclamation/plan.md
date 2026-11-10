---
created: 2026-10-11
status: in_progress
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-003
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
legacy_specs: []
---

# Implementation plan: legacy delivery payload reclamation

## Outcome and ownership

Remove redundant, independently projected raw output left by historical delivery
inconsistencies. Automatic cleanup must reach those rows after upgrade, preserve chat
history and recovery evidence, and avoid retaining later raw output behind an unrelated
old terminal. Apply the corrected cleanup to the operator's local database after
isolated verification, then measure its ongoing behavior.

Platform owns this repair because authenticated delivery and recovery determine what
transport evidence may disappear. The user explicitly requested both the code fix and
local cleanup on October 11, including automatic historical reclamation when new code
runs. The user subsequently requested implementation with `Fix`. Execution is
sequential in the primary session, including the code repair and local cleanup.

## Confirmed diagnosis

Running artifact: `v0.97.0-183-ge6692746b0`, including PR #4407. Live database:
`/home/jcfs/.kandev/data/kandev.db`, backend port 38429. Read-only queries and the
existing maintenance dry run supplied these approximately contemporaneous measurements:

| Measurement | Evidence |
| --- | --- |
| Main SQLite file | Approximately 6.8 GB; almost no fully free pages |
| Allocated inbox table | 3.66 GB |
| Allocated canonical messages table | 1.96 GB |
| Raw inbox payloads | Approximately 3.47 GB; tool updates contain about 3.27 GB |
| Payloads received since the upgraded backend started | Approximately 903 MB in its first two hours |
| Streams with an unproved first terminal | 47 |
| Payloads at/after those terminals | Approximately 3.02 GB across 363427 events |
| Events referencing absent submissions | 104919, overlapping other protection categories |
| Payloads in running sessions | Approximately 2.28 GB |
| Payloads in cancelled sessions | Approximately 1.16 GB |
| Legacy maintenance candidates | 3139 obsolete plan revisions, about 106.64 MB; 9 duplicate snapshots |

The automatic worker is active: the initial log sample recorded 7292 event bodies
compacted, 48680 retired-session rows purged and about 5.24 MB of payloads removed.
This proves activity, not sufficient reclamation or sustained capacity.

`agentDeliveryRetentionCandidateSQL` applies an existing-submission probe and the
`first_terminal_sequence` barrier to raw compaction. The barrier is appropriate for
contiguous receipt retirement, but overprotects independently completed later output.
Legacy missing prompt rows additionally keep output bodies even when the exact
canonical projection effect and authenticated cursor survive. Preserve the stronger
proof for terminal and prompt removal; separate nonterminal raw compaction from it.

`Worker.runPass` additionally repeats only phases that mutate data. Protected pages
advance discovery but stop a phase for that pass, limiting how quickly it can reach
eligible history beyond them. The repair must remain bounded while following committed
forward discovery progress.

Nine sessions were running at inspection. Their raw transport copies are distinct from
canonical history; deleting sessions or messages is outside the requested repair.

## Existing contract and design

- [Requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md),
  including clarified `AC-PLATFORM-DELIVERY-RETENTION-001.4`, `.005.5` and `.006.6`.
- [Payload proof design](../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md).
- [Parent retention design](../../specs/platform/system-design/backend-agent-delivery-retention.md).
- [Checkpoint ADR](../../decisions/2026-10-05-backend-delivery-retirement-checkpoints.md).
- [Barrier ADR](../../decisions/2026-10-10-delivery-event-retention-barriers.md).
- [Original retention delivery package](../backend-agent-delivery-retention/plan.md).

The original package and its companion durable-session/reconciliation packages retain
their historical results and exclusions. This repair does not reopen their full rollout,
original-binary transfer or broad latency audit. Recheck the base and overlapping source
before implementation. No new ADR is needed: the accepted raw-body/replay-evidence
separation remains the governing boundary, and this plan records the correction.

## Scope and technical approach

1. Correct nonterminal raw-body predicates, locked reload and preview together.
   Preserve exact hash validation, tombstones, unknown/pending-effect protection and
   the stricter terminal, prompt and contiguous-retirement checks.
2. Return committed discovery progress/exhaustion from bounded repository batches;
   let the existing worker continue fair scanning within its existing pass budget.
   Upgrade uses the normal worker and persisted cursors, without full startup scans.
3. Verify history/recovery behavior and cloned-database reclamation. During a separately
   coordinated maintenance window, use a task-specific operational runner with the
   corrected shared methods, a fresh verified backup and the existing ownership lock.
   Compact only after logical cleanup, restart the corrected backend, and observe growth.

No new runtime flag, startup setting, worker, UI, format version, prompt resend,
forced settlement, receipt leap, task deletion or canonical-message expiry is included.
Missing legacy prompt authority remains missing; pruning a proven raw copy does not
make the session resumable. Truly protected receipts and canonical history can grow.

| Boundary | Expected behavior | Evidence |
| --- | --- | --- |
| SQLite and PostgreSQL | Same raw-body eligibility; native writer/barrier rules retained | Repository integration, replay and concurrency tests on both engines |
| Compatible format-1 readers | Compact duplicate returns committed result; changed content fails | Existing receipt replay and hash tests plus mixed terminal regression |
| Unsupported/unknown payload or effect | Retain body and report aggregate skip | Mixed-state negative fixtures |
| Backend upgrade/restart | Normal bounded worker revisits historical rows | Worker/real repository restart integration |
| Desktop and phone history | Same canonical history and recovery controls | Existing delivery retention Playwright flows with historical-gap fixture |
| Physical local reclamation | Offline ownership, fresh backup, verified compaction | Isolated snapshot comparison, live integrity and measured before/after |

## Tests and delivery sequence

- Task 01: `TestDeliveryRetentionIntegrationHistoricalCanonicalPayloads` reproduces
  a pending or legacy missing-submission terminal followed by independently committed
  nonterminal output. Before the fix, eligible output remains raw. Include an unrelated
  pending effect, mismatched cursor, unsupported type and missing exact generic effect
  in the same stream, proving that all required proofs hold for each pruned row.
  Identical replay must remain duplicate, changed replay must fail, and retirement
  must stop at the protected terminal.
- Task 02: `TestDeliveryRetentionWorkerProtectedDiscoveryProgress` reaches an eligible
  page after several protected pages within one bounded pass; committed cursor progress
  is distinct from rollback and exhaustion. Real repository fixtures survive restart.
- Task 03: existing desktop/phone browser flows, snapshot canonical digests/counts,
  replay/ACK assertions, measured raw-body reduction and verified physical compaction.

Commands and owned files are in each work order. The PostgreSQL test helper uses
`KANDEV_TEST_POSTGRES_DSN`; without a disposable instance, record its skip and do not claim
that engine passed. No broad local verify or additional review gate is introduced.

## Work orders

- [x] [Task 01: Separate canonical raw-body proof](task-01-canonical-payload-proof.md)
- [x] [Task 02: Resume fair historical scanning](task-02-historical-sweep-progress.md)
- [ ] [Task 03: Verify and repair the local database](task-03-local-database-repair.md)
- [x] [Task 04: Preserve legacy v1 event serialization](task-04-legacy-v1-serialization.md)

Task 04 is a narrow compatibility follow-up for existing v1 receipt identity.
It does not reopen completed Tasks 01-02 or claim that Task 03's database repair
has run. It changes no retention predicate, recipe, validator or stored record.

## Local evidence and safety checkpoint

Diagnostic baseline: task workspace `.kandev/diagnostics/dc305e85c9eb597477a63cc7788cfe86.zip`.
Its backend manifest was ready with no warnings.

Consistent snapshot: `/home/jcfs/.kandev/data/backups/kandev-before-size-cleanup-20261011T140100Z.db`.
Size: 6790471680 bytes. Full `PRAGMA integrity_check` returned `ok`.
The live database has not been manually pruned or replaced. Its background worker
continues normal activity. Normal backup rotation subsequently removed this snapshot;
the isolated copy already supplied the repair proof. A new consistent repair snapshot
was verified at `/home/jcfs/.kandev/retention-repair-20261011/current-before.db`,
6869843968 bytes, integrity `ok`, outside the rotated directory. Live execution takes another fresh verified backup
under the database parent before its first deletion.

The user requested removal of unnecessary delivery data, not a small plan-only cleanup.
The earlier maintenance question did not settle interruption timing: its answer asked
to reclaim the large redundant category. Coordinate timing immediately before a live
stop; this does not block code design or isolated verification.

## Verification results

Design validation passed: `python3 scripts/list-docs.py validate`,
`python3 scripts/lint-spec-files.py --all`, the 36 documentation linter tests and
`git diff --check`. Implementation and targeted regression checks passed on SQLite
and disposable PostgreSQL, including worker/concurrency race checks, sustained arrival,
backend lint, SQL guard, persistence conformance, web typecheck and affected E2E lint.
Desktop and phone history/recovery E2E each passed; the optional frozen-binary case
was skipped. The initial broad race command timed out on million-row stress fixtures;
the extended retention race run excluding those fixtures passed (147.4 seconds).

The isolated snapshot's raw inbox bytes fell from 3476691304 to 24617706. All canonical,
task, plan HEAD, turn, unsettled prompt, protected terminal, recovery, queue and
projected-cursor digests matched. Full integrity check returned `ok`. Existing CLI
maintenance removed 2119 obsolete revisions while protecting five recent non-HEAD
revisions per task, and nine duplicate snapshots. Verified compaction reduced the
file from 6790471680 to 3118968832 bytes; digests matched again afterward. The
disposable database and its backups were removed after verification. Audit manifests
and aggregate logs remain in `/home/jcfs/.kandev/retention-repair-20261011/`.

The 200-row operational run met repeated admitted deadlines after committing partial
work. Retrying the same corrected shared operations at the production 50-row quantum
finished a complete traversal with no further eligible body removals. Live cleanup
and corrected-runtime deployment await the coordinated interruption checkpoint.

## Risks

- A canonical-looking raw body alone is not projection proof. Missing or conflicting
  exact effects/cursors must remain protected.
- Do not remove an earlier terminal barrier from `can_retire`; that would discard
  unresolved recovery evidence or advance a checkpoint across a protected gap.
- Progress signals must reflect committed forward discovery, not repeated wrap or
  merely selected candidates. Preserve fairness and cancellation under contention.
- Logical raw-byte removal, allocated table pages, physical file shrinkage and total
  disk usage differ. Backups and pre-compaction files retain removed bytes.
- Observed traffic is a short interval. Verify several intervals locally and a sustained
  mixed-history fixture; do not claim a permanent total-size quota.

## Throughput follow-up

The [database maintenance throughput package](../database-maintenance-throughput/plan.md)
addresses backup rebuilding and cleanup idle time observed after PR #4418. It does
not change this package's historical verification results. The automatic live worker
is running; live physical compaction has not been performed by this session.
