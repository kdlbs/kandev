---
created: 2026-10-05
status: in_progress
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-003
  - REQ-PLATFORM-DELIVERY-RETENTION-004
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
legacy_specs: []
---

# Implementation plan: backend agent delivery retention

## Overview and checkpoint

Remove redundant backend delivery payloads while retaining authenticated replay, projection, terminal and queue recovery evidence.
Build format/fencing and receipt-aware consumers before activating cleanup. Work orders progress sequentially; the current boundary is recorded below.
Implementation was authorized by the user on 2026-10-09. No feature flag is planned.

## Current PR delivery scope (2026-10-11)

The user removed the original-artifact transfer prerequisite and authorized delivery of the current issue fix. Current-head required CI, valid existing findings and ownership/recovery/retention regressions remain the gates. Historical original rollback and matched-baseline latency acceptance are not claimed passed and do not block this delivery. Preserve the Ready user edit; Task 08 records the evidence limits and remaining broader validation. No extra review or broad audit is required.

## Baseline and assumptions

- Original investigation baseline: `f2668750bf6a17792b58d423f6b5b99cfeabf78e` on `feature/investigate-durable-cce`; the October 5 receipts below remain historical.
- On October 9 the user explicitly authorized rebasing after [PR #3598](https://github.com/kdlbs/kandev/pull/3598) merged, superseding the original no-rebase constraint. GitHub reports merge `349f61bc52770f0140e3b7a2df9cd66aa4231b03` at `2026-10-09T08:36:07Z`; final PR head was `7aa2c14d13a204a1e83a8df2e9679dd4e315c29b`.
- Refreshed baseline and HEAD: `d7a44e96ede93d50c6277773f4174de716272f02`, fetched main on October 9. The isolated branch remains `feature/plan-backend-retenti-icy`; merge ancestry is verified.
- The child had zero commits beyond the original baseline. Rebase used `git rebase --onto d7a44e96ede93d50c6277773f4174de716272f02 f2668750bf6a17792b58d423f6b5b99cfeabf78e feature/plan-backend-retenti-icy`, avoiding replay of the squashed feature. All 18 uncommitted files were backed up and restored; upstream reattachment additions merged cleanly. No package commit was created.
- The user confirmed backend-only retention, preserved canonical history/recovery authority, both engines, bounded work, no flag and planning-only scope.
- Source confirms unbounded inbox/prompt payloads, retained effects/cursors and absent delivery deletion cascades. Projected terminals can still be unsettled.
- One-hour payload grace, 24-hour/10,000-event replay evidence and work budgets are proposed design choices under the requested policy definition.
- No material intent blocker remains. Physical file size, sustainable cleanup capacity and compatible release versions require implementation evidence.

Platform owns this feature because its durable transport contract controls authenticated evidence and recovery, independently from agentctl storage or host-resource cleanup.

## Source contracts and companion records

- [Retention requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Paired design](../../specs/platform/system-design/backend-agent-delivery-retention.md).
- [Accepted checkpoint ADR](../../decisions/2026-10-05-backend-delivery-retirement-checkpoints.md).
- [Parent delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md), [delivery design](../../specs/platform/system-design/durable-agent-delivery.md), [reattachment](../../specs/platform/system-design/durable-agent-reattachment.md).
- [Original delivery plan](../durable-agent-sessions/plan.md): 18 completed work orders, historical results and environment gaps preserved.
- [Session reconciliation plan](../durable-agent-session-reconciliation/plan.md): five completed orders; adoption, identity and ordered ACK contracts remain prerequisites.
- [Reattachment plan](../durable-agent-reattachment/plan.md): five completed orders; exact-submission block/claim settlement must remain recoverable.
- [Streaming repair](../durable-agent-stream-repair/plan.md) and [runtime replacement](../agentctl-runtime-replacement/plan.md): preserve their recorded evidence; retention does not claim to complete adjacent work.
- [SQLite writer admission ADR](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md) and [managed deletion design](../../specs/plugins/system-design/managed-deletion-admission.md): existing transaction entry, owner/marker and external-I/O boundaries.
- [Runtime continuity](../../specs/platform/requirements/transient-turn-runtime-continuity.md) and [provider continuation](../../specs/platform/requirements/provider-interruption-continuation.md): cleanup grants no new recovery authority.

Links to this implemented extension do not reopen completed companion orders or replace their recorded verification.
At implementation time re-read overlapping work orders and current source. This refresh incorporates the explicitly authorized rebase; reconcile future base changes before implementation rather than relying on the historical October 5 receipts.

## Scope

In scope: compact inbox/prompt retention, bounded generic receipt retirement, preserved terminal/workflow receipts and cursors, replay/ACK recovery, deletion/orphans, both engines, startup format, worker, CLI integration, telemetry and operational docs.
Out of scope: agentctl journal changes, native state, canonical transcript/tool expiry, new UI, online VACUUM, flags and automatic resend.

## Technical approach and dependency order

The design owns policy details. This manifest assigns implementation boundaries:

1. Task 01 adds compatible schema/readiness and transaction fencing. No pruning starts.
2. Task 02 makes receive/project/adoption/ACK consumers understand compact and expired evidence.
3. Task 03 implements bounded event payload cleanup and atomic prefix retirement.
4. Task 04 establishes full-settlement proofs and completed-prompt cleanup.
5. Task 05 retires explicit deletion identities and drains orphans without foreground mass deletes.
6. Task 06 starts the automatic worker after all safety dependencies and adds config/metrics/quiescence.
7. Task 07 integrates offline maintenance/reporting and public compatibility guidance.
8. Task 08 proves cross-engine crashes/concurrency, long-stream bounds and desktop/phone continuity.

Merged PR #3598 and the refreshed main still have no backend retention schema or collector; they do not satisfy release N compatibility.
Release N contains Tasks 01-02 only: compatible format-1 reader/writer, retirement admission/journaling and no collector or destructive maintenance entry point. Produce and retain its real binary artifact.
Release N+1 contains Tasks 03-08 and activates pruning only after N passes rollback recovery/write/deletion/forward-upgrade tests against N+1 compact evidence. No feature flag separates these releases.
N preserves jobs/checkpoints and blocks retired-ID recreation; collection stays paused during rollback until forward upgrade. SQLite version-change snapshots keep only two copies; preserve a verified external pre-pruning backup. Require verified PostgreSQL pg_dump before N+1 upgrade and test restore.
All orders are sequential; waves do not authorize agents.
Exact current source boundaries are listed in the design evidence table and each order.
New symbols and test names are deliverables, not claims that they already exist.

## Compatibility matrix

| Shape | Behavior | Evidence | Unsafe fallback |
| --- | --- | --- | --- |
| SQLite + healthy authenticated v1 | Automatic bounded cleanup; hashes/checkpoints preserve ACK/replay | Tasks 01-08 | Never recreate zero cursor |
| PostgreSQL + healthy authenticated v1 | Same logical contract; shared transaction advisory locks | Mandatory real multi-connection tests | No SQLite-only SQL |
| Valid legacy peer | Schema remains required; no durable evidence inferred from missing capability | Startup and activation regression tests | No inferred v1 recovery |
| Supported v1 peer with journal/storage/auth failure | Preserve blocked uncertainty | Task 02 integration | No downgrade or resend |
| Local, Docker, SSH, Kubernetes retained v1 transport | Backend cleanup is executor-independent after existing ownership/capability checks | Repository/client tests plus existing recovery regressions | Does not add executor survival support |
| Release N compatible reader/writer | Supported N+1 rollback; collection paused, pending jobs preserved | Frozen real N artifact: `/tmp/kandev-delivery-retention-release-n-20261009/kandev`, SHA-256 `3df1e310220f0a7588453c062f2fa90aa0a611dd7277cc77c76dd4f8bc3a0517`; rollback matrix remains Task 08 | No rehydration or checkpoint reset |
| Older backend without retention format | Unsupported after first prune | Task 01/08 format and binary compatibility tests | Additive SQL is not rollback proof |
| Unknown/newer format | Reject before readiness/admission | Task 01 | Never delete/recreate format record |

## Tests and coverage

All acceptance criteria use prefix `AC-PLATFORM-DELIVERY-RETENTION-`.
The complete IDs appear in work-order frontmatter. The table uses suffixes for compact comparison.

| AC suffixes | Proposed file/group | Work order |
| --- | --- | --- |
| 006.1, 006.4, 003.2 | `agent_delivery_retention_schema_test.go`: Schema, Format, Serialization; storeconformance upgrade | 01 |
| 002.1-002.4, 003.2 | `agent_delivery_retention_replay_test.go`: Hash, Replay, MetadataPagination, RetirementAdmission, CompatibilityReader; lifecycle: LostAck, NoResend | 02 |
| 001.1-001.2, 005.1-005.2 | `agent_delivery_retention_events_test.go`, `agent_delivery_retention_crash_test.go`, `agent_delivery_retention_bounds_test.go`: Events, Crash, Bounds | 03 |
| 001.3, 003.1, 003.3 | repository/orchestrator retention settlement tests: Prompt, PartialTerminal, IndependentBlocks, WorkflowOutcome | 04 |
| 004.1-004.3, 003.2 | repository/service retention deletion tests: Deletion, Orphans, Replacement | 05 |
| 005.1, 006.2-006.3 | new deliveryretention package; config/catalog, Metrics, Quiescence groups | 06 |
| 005.4, 006.3-006.5 | offline maintenance tests and public-doc validators | 07 |
| 002.*, 003.*, 004.*, 005.*, 006.1, 006.4-006.5 | cross-engine Concurrency, Storage, Compatibility, Integration; browser continuity | 08 |

Use full IDs in tests where mapping is unclear. Each work order requires meaningful Red/Green evidence and exact commands.
Do not mark a selector as passing if it found no proposed tests. List tests first and record test/subtest counts.
Schema replay alone does not prove PostgreSQL behavior. Every changed dialect-sensitive method needs executed real-engine coverage.

## Performance and storage evidence

Task 08 builds a million-event stream, with 1 KiB baseline chunks and a 4 KiB variant, plus a protected-gap variant.
Use virtual age progression and bounded real cleanup until quiescence, then reopen the database.
Disable canonical `internal/system/toolretention` in disposable fixtures and assert its effective config.
Measure the same workload at the pre-retention baseline, with new fencing/cleanup idle, and with cleanup active on the same hardware:

- Byte-exact canonical output, turn count, prompt sends, workflow transitions and claim ACK counts must match.
- Expired generic effect count and inbox rows must follow the design window, including raw grace and protected exceptions. No lifetime-per-event receipt table can remain.
- Candidate work is at most 200 rows and payload work at most 4 MiB. Use one context capped at 250 ms and remaining pass time for BEGIN and SQL; never renew it after entry. Each pass stops starting work at 5 seconds.
- In a controlled benchmark, foreground commit p95 with cleanup must be at most 1.10 times the pre-retention baseline, including the indexed live-session/incarnation read and shared-lock acquisition on every streamed event; report absolute values, hardware, engine version, WAL/autovacuum state and confidence samples.
- SQLite native BEGIN may remain busy after cancellation until its existing 5-second busy timeout plus scheduling overhead. Measure that separately from admitted SQL work; no predicates or mutations may follow cancelled admission. Join failed entry/rollback before pool reuse or shutdown. A pass starts no new batch after its deadline, but an in-flight native wait can overrun it. Unexpected overruns beyond these distinct budgets keep acceptance unresolved.
- At 20 events/second and 1 KiB, cleanup must drain steady-state eligible backlog with oldest eligible age at most two normal intervals beyond grace on the controlled fixture.
- Explain plans must use the intended candidate/purge indexes. Fixed batch query/scan work must not increase linearly with 1,000,000 retained historical rows.
- Record logical payload bytes, index/page overhead, WAL/dead tuples, reusable pages and separately requested post-compaction size. Confirm deleted payload bytes can remain in backups.

The design formula estimates about 71.5 MiB raw grace plus 2.44 MiB compact receipt content for a continuously active healthy baseline stream.
Canonical history, submission/terminal/workflow summaries and cursors still grow. Protected backlog has no forced TTL or global quota.
Results are pending measurements, not current guarantees or a claim that all storage growth stops.

## Desktop and mobile evidence

No rendered controls, layout or copy changes are planned, so ASCII UI previews are unnecessary.
Existing chat history and session recovery remain authoritative. The nearest shipped tests are durable-reattachment and mobile-durable-reattachment.
Task 08 adds desktop and phone retention scenarios that actually prune data, restart/reload, verify complete saved output, then exercise Stop/state-only retry under unresolved delivery.
Reuse the existing inline recovery view, shared state and phone interaction targets. Add no drawer or navigation path.
The phone test must prove the same recovery outcome, readable history and no horizontal overflow.
If implementation needs new copy/composition, update this package and apply mobile-parity/i18n before editing UI.

## Work orders

- [x] [Task 01: Add retention schema and shared transaction fencing](task-01-schema-and-format.md)
- [x] [Task 02: Handle compact and expired replay without resending](task-02-replay-after-pruning.md)
- [x] [Task 03: Compact projected events and retire eligible prefixes](task-03-event-retention.md)
- [x] [Task 04: Prove full settlement before pruning completed prompts](task-04-settled-prompts.md)
- [x] [Task 05: Retire deleted sessions and purge delivery orphans](task-05-deletion-and-orphans.md)
- [x] [Task 06: Run bounded cleanup with configuration and telemetry](task-06-worker-and-operations.md)
- [x] [Task 07: Integrate offline maintenance and document compatibility](task-07-manual-maintenance-and-docs.md)
- [ ] [Task 08: Prove restart, cross-engine storage bounds and surface parity](task-08-integration-and-storage.md)

## Package validation

Run these exact planning checks from repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node docs/plans/backend-agent-delivery-retention/validate-package.cjs
git diff --check -- docs/specs docs/decisions docs/plans/backend-agent-delivery-retention docs/plans/durable-agent-sessions docs/plans/durable-agent-session-reconciliation docs/plans/durable-agent-reattachment
git status --short
```

The package helper runs repository PR-documentation `validateCoverage` on the uncommitted docs and on an explicitly projected implementation-path input, including untracked work orders.
It also checks AC ownership/coverage, dependencies, local links and source ownership paths. It is independent of lifecycle statuses, Results content and staged state, so Tasks 07/08 can run it after implementation.
No GitHub PR mutation or committed revision is required for this preflight.

## Historical verification results (2026-10-05)

These pre-merge checks passed at the original baseline; they are not current-head product evidence:

| Exact command | Result |
| --- | --- |
| `git merge-base --is-ancestor f2668750bf6a17792b58d423f6b5b99cfeabf78e HEAD` | Exit 0; HEAD remains exactly the baseline. Local and remote feature refs match; PR #3598 metadata agrees. |
| `python3 scripts/list-docs.py validate` | Exit 0: 351 decisions and 1348 specifications. |
| `python3 scripts/lint-spec-files.test.py` | Exit 0: all 36 tests passed. |
| `python3 scripts/lint-spec-files.py --all` | Exit 0: all specification files passed. |
| `node docs/plans/backend-agent-delivery-retention/validate-package.cjs` | Exit 0: 12 Markdown documents, eight work orders, six requirements, 22/22 ACs, 60 local links and 49 existing owned paths. Dependency order, ancestry and structure passed. Lifecycle/staged state is intentionally unconstrained; current documents remain draft/proposed/pending and unstaged. |
| `git diff --check -- docs/specs docs/decisions docs/plans/backend-agent-delivery-retention docs/plans/durable-agent-sessions docs/plans/durable-agent-session-reconciliation docs/plans/durable-agent-reattachment` | Exit 0. The package helper also checks whitespace/final newlines in new untracked Markdown files. |
| `git status --short` | Only design documents, companion links and the read-only documentation validator changed; all remain unstaged/uncommitted. |

The real documentation-only PR preflight is `exempt`. A separate explicitly projected production-path input exercises all work-order references and returns `covered`.
The validator does not claim that production code changed or that a PR exists.
An early source-path check caught the relocated E2E reset file; the order now names `internal/backendapp/e2e_reset.go`.
An early lint check caught the parent design's size limit; the new links use requirements/reattachment rather than expanding that full parent file.
That parent design was unchanged by the October 5 package. The authorized rebase now includes its upstream edits; this package still adds no edits to that file.

Implementation-time Go, PostgreSQL, crash, benchmark, binary compatibility and browser checks are intentionally not run.
Every work order owns its exact commands. Missing DSN, compatible binary or browser infrastructure remains outstanding evidence, never an automatic waiver.

## Review amendments

All 14 peer items are addressed in this planning-only revision:

- The helper checks structure across lifecycle changes, including nonempty Results, without asserting draft/pending/staged state. An ephemeral Node VM probe passed with all 12 documents transformed in memory to implemented/done/accepted and recorded Results; actual files stayed draft/proposed/pending.
- Operator visibility explicitly uses production aggregate logs on SQLite/PostgreSQL and supplemental dev/pprof expvar. No production HTTP metrics or PostgreSQL CLI dry-run is claimed.
- Task 04 owns the missing exact-bound workflow outcome writer and tests no workflow, quick chat, scheduler-owned Office, workflow-routed Office, no actions and RequiresApproval skips. Deferred gates/errors never produce settlement no-ops.
- Task 03 protects every terminal until Task 04 lands. Its early bounds tests use terminal-free streams and first-terminal pinning; Task 08 owns final multi-turn bounds.
- Startup aliases use singular `KANDEV_TASK_DELIVERY_RETENTION_*`, matching task catalog siblings. `fully_settled_at` differs from existing block-resolution `settledAt`.
- Releases N and N+1 separate compatible reader/writer expansion from cleanup activation without a flag. Actual N rollback evidence gates activation; SQLite backup rotation and mandatory pre-upgrade PostgreSQL pg_dump/restore guidance are explicit.
- The ID-reuse fence has a production caller: deterministic managed/ordinary plugin conversation primary IDs. Executor/Office use fresh UUIDs; E2E leaves ID empty. No import/restore ID writer was found.
- Metadata pagination moves from Task 01 to Task 02. Task 01 retains schema/fencing as one safety boundary with independent serialization evidence; N includes retirement journaling/admission before N+1 draining.
- Task 08 compares baseline, fenced idle and cleanup-active foreground p95, including every-event live-session reads/locks. Canonical toolretention is disabled in fixtures; scope excludes only canonical tool payload expiry, and payload-removal timing remains conditional on capacity/dependencies.
- `mobile-chrome` is verified in `apps/web/e2e/playwright.config.ts:84`; the existing command is correct.

Catalog, full spec lint, 36 lint tests and package coverage/link/ID validation all passed after these amendments. Go, real-engine, rollback-binary, crash, performance and browser checks remain implementation-time gates.

## Post-merge reconciliation (2026-10-09)

| Merged change | Package update |
| --- | --- |
| Shared SQLite writer now uses `_txlock=immediate`; separate reader stays deferred | Tasks 01/03/06/08 reuse genuine writer transactions, preserve reader progress, and distinguish 250 ms work from native BEGIN's 5-second busy wait. No custom BEGIN, row-touch reservation or new pool. |
| Managed deletion now reserves/prepares ownership before atomic final deletion | Tasks 01/02/05 hook delivery retirement into `FinalizeManagedDeletion`/`deleteTaskWithVacatedStep`, in the final delete/committed-marker transaction. Prepared/released attempts do not retire delivery identities; external cleanup stays outside SQL. |
| Managed conversation creation moved into native admission | Tasks 02/05 cover `managed_conversation_settings.go` and repository `repairManagedPrimaryTx`, as well as ordinary `createPrimarySession`. Deterministic ID reuse remains real. |
| Completion can be projected before prompt RPC returns; journal completion/state is atomic | Tasks 02/04/08 preserve terminal-first outcomes after pruning, exact peer identity and independent blocks. Existing journal semantics are regression inputs, not implementation scope. |
| Metadata value CAS moved to `session.go` and compares SQLite JSON structurally | Tasks 01/04 reuse the current primitive and test reordered object keys, mismatched members and successor notices. Never restore the older serialized-text comparison. |
| Provider capacity and transport continuation now coexist with durable recovery | Task 04 preserves failed-turn semantics; Tasks 02/08 test fresh continuation IDs, final authorization, pending tools and uncertainty. Cleanup itself never creates a recovery attempt. |
| Canonical notifications are marker-free; diagnostics remain on source events | Task 02 hashes source transport evidence, never reconstructs hashes from canonical notifications. Initial-prompt metadata/receipts stay outside retention deletion. |
| E2E reset now uses `orderE2ETasksForDeletion` and coordinator state cleanup | Task 05/06 extend current reset quiescence without replacing hierarchy/coordinator ordering. Task 08 keeps existing desktop/mobile project names and adds relevant continuity regressions. |

The inbox, projection, terminal-settlement, lag-metric and workflow-effect implementations, and manual maintenance categories, are unchanged from the original inspected baseline. Backend delivery retention remains necessary; the eight-order sequence, policy defaults, no-flag boundary and two-release rollback strategy remain proposed.
Companion reattachment orders 03-05 now contain later upstream remediation/results through October 9. Preserve those receipts and their remaining native/live-state gates; a merged PR alone does not close them.

## Current planning validation (2026-10-09)

All commands below ran from repository root after the authorized rebase and package edits:

| Exact command | Result |
| --- | --- |
| `gh pr view 3598 --json number,state,mergedAt,mergeCommit,baseRefName,headRefName,headRefOid,url` | MERGED to main at `349f61bc52770f0140e3b7a2df9cd66aa4231b03`; October 9, 08:36:07 UTC. |
| `git rev-parse HEAD` | `d7a44e96ede93d50c6277773f4174de716272f02`. |
| `git merge-base --is-ancestor 349f61bc52770f0140e3b7a2df9cd66aa4231b03 HEAD` | Exit 0. |
| `git rev-list --count d7a44e96ede93d50c6277773f4174de716272f02..HEAD` | 0; no package commit or replayed feature commits. |
| `python3 scripts/list-docs.py validate` | Exit 0: 369 decisions and 1495 specifications. |
| `python3 scripts/lint-spec-files.test.py` | Exit 0: all 36 tests passed. |
| `python3 scripts/lint-spec-files.py --all` | Exit 0 after trimming repeated design prose to fit the file-size limit. |
| `node docs/plans/backend-agent-delivery-retention/validate-package.cjs` | Exit 0: 12 Markdown documents, eight work orders, six requirements, 22/22 ACs, 65 links, 56 existing owned paths; dependency/structure and refreshed-base/merge ancestry pass. Docs preflight exempt; projected implementation coverage covered. |
| `git diff --check` | Exit 0. New Markdown whitespace/newlines are also checked by the helper. |
| `git diff --cached --name-only` | Empty. |
| `git status --short` | Only the existing planning package and companion links are modified/untracked; no production or permanent test edits. |

The temporary rebase stash restored without conflicts. A file-hash check found all original package files byte-identical immediately after restore except reattachment design, which also preserved upstream additions. The restored companion diff contains only the original retention link section.
Runtime, PostgreSQL, rollback-binary, crash, benchmark and browser commands remain implementation-time requirements, including the added merged-contract regressions. These planning checks do not close upstream release gates.

## Risks and review points

- Hash normalization must preserve existing equality semantics while keeping adapter-owned content strict.
- Every SQL delivery writer must use the shared serialization boundary; process-local maintenance admission is insufficient.
- Legacy streamless/sessionless workflow receipts need authoritative binding; unknown rows remain protected and can still grow.
- Protected terminal gaps can exceed the nominal replay window and retain subsequent evidence. This is explicit recovery protection.
- Default grace/window values trade diagnostic replay for smaller storage; changing values must not alter safety predicates.
- Pre-retention binaries are unsafe after pruning. Release N must ship first as a compatible reader/writer; a real N artifact passing the N+1 rollback matrix is an activation gate.
- Physical disk reclamation is engine/operator work and can temporarily require extra space.

## Handoff

The original design-package handoff above is historical. The user authorized implementation on 2026-10-09; execution status and results are recorded below as the work orders complete.

## Implementation execution (authorized 2026-10-09)

Current work order: Task 08. Tasks 01-07 are complete. Execute the remaining work orders sequentially. No staging, commit, push or publication is authorized.

## Resumed implementation rebase (2026-10-09)

- [x] Back up all 110 modified/untracked files externally at `/tmp/kandev-retention-resume-20261009/workspace-before-rebase.tar.gz`.
- [x] Fetch main and rebase to `d6cff57659ae5aea28e9457c587a8c9d70d0a130`, restoring the workspace without conflicts. The temporary stash and external backup are retained.
- [x] Verify frozen Release N SHA-256 still matches the Task 02 receipt.
- [x] Complete Task 07 maintenance checks and public documentation.
- [ ] Complete Task 08 integration, real-engine, storage, rollback and desktop/mobile gates.
- [ ] Build, run required checks, reconcile artifact status and inspect final changes.

## Continued execution (2026-10-10)

The user authorized rebasing and continuing the existing implementation. External backup `/tmp/kandev-retention-resume-20261009/workspace-before-rebase.tar.gz` contains all 110 changed/untracked files before rebase. The original temporary stash remains available. Rebase onto fetched main `d6cff57659ae5aea28e9457c587a8c9d70d0a130` and stash restoration completed without conflicts or staged files. Earlier baseline and check receipts remain historical. Task 08 uses the rebased base for lint.

- [x] Rebase and preserve existing changes and the frozen N artifact.
- [x] Finish Task 07 maintenance tests and public documentation checks.
- [x] Build native backend, Linux helpers, E2E web bundle and E2E plugin package.
- [x] Reproduce and fix terminal compaction, prompt pruning after event retirement, and exact terminal-summary turn/outcome protection. Integration tests pass on SQLite and real PostgreSQL.
- [x] Add sparse terminal frontier index and prove the SQLite lookup uses it.
- [ ] Complete real storage/capacity, authenticated writer benchmarks, retained-journal integration, cross-engine conformance and desktop/mobile rollback/recovery checks in Task 08.
- [ ] Complete final checks and workflow signal.

The latest direct user request permits native subagents. Separate workers own storage/performance and browser/compatibility in Task 08; the primary session retains integration, requirements, design, plan and user communication. Work-order ordering remains sequential. No staging, commit, push or publication is authorized.

## Current integration checkpoint (2026-10-10)

- [x] Pass real SQLite/PostgreSQL store conformance, owner/proof/query-plan regressions, steady-state cleanup capacity, independent SQLite cancellation and initial prompt admission.
- [x] Pass full native/helper/web/plugin build and whole-backend lint before the latest retry correction.
- [x] Pass actual PostgreSQL backup, frozen-N retained-session rollback/write/deletion, and forward upgrade on the preceding native artifact. The witness does not certify all historical launch paths; Task 08 records its implicit auto-start limitation.
- [x] Reproduce retained-runtime retry admission failures and implement shared SQL preparation before implicit durable dispatch.
- [x] Pass retry producer GREEN and fresh native/helper/web/plugin builds.
- [ ] Complete desktop/mobile suites and final-artifact PostgreSQL rollback after the covered lint refactor.
- [ ] Meet paired foreground latency, including idle fencing and actual active cleanup. The direct-prefix window passed PostgreSQL idle/active and failed both SQLite gates. The resumed sparse window with limited runtime parallelism failed all four gates; all samples remain recorded. No threshold waiver is claimed.
- [x] Bound empty-cursor attribution pages and commit durable purge checkpoints; both-engine crash/reopen/one-row-budget and actual PostgreSQL plan checks pass.
- [x] Pass productive-phase/yield-budget regressions and preceding 50-candidate background capacity on both engines with race detection; preserve the configured 200 ceiling and lower operator limits.
- [x] Repeat real default and lower-limit capacity after full-lease pass admission and scoped event barriers; default/explicit 50 and explicit 30 pass on both engines with zero failed/busy batches.
- [x] Complete fresh owner SQL-cache proof, lifecycle and connection-replacement checks; shared-barrier and authority regressions pass on that source.
- [x] Validate direct expired-prefix retirement through trigger RED, both-engine crash/reopen/replay/barrier GREEN, mixed-format accounting and lint/SQL guard.
- [x] Measure direct-prefix latency on frozen source; retain its two SQLite failures.
- [x] Measure the partial pending-index candidate with the original production runtime settings; PostgreSQL passes and SQLite fails.
- [x] Reduce fixed receipt SQL preparation overhead with TDD and measure frozen source; PostgreSQL passes and SQLite still fails.
- [ ] Resolve SQLite latency and complete fresh native/runtime/storage acceptance.
- [ ] Complete replacement/deletion measurements, million-event fixtures, the exact retention race selector and final lint/documentation checks.

Task 08 remains in progress. Its Results section owns exact receipts, failed attempts and remaining gates.


Direct-retirement design checkpoint: catalog validation passed (370 decisions/1498 specifications), full specification lint passed, package structure/coverage passed (22/22 ACs, 66 links, 56 owned paths), and whitespace checks passed. Production direct-prefix implementation and measured gates remain pending.


## Draft PR handoff (2026-10-10)

The user requested a pull request promptly for colleague handover. This supersedes the earlier no-stage/commit/push/publication constraint for a draft handoff. Task 08 remains in progress, with all open gates preserved. See [handoff](handoff.md) for the continuation order, latest source/evidence and external artifact requirements. Fresh full build/lint and normal hooks precede publication.


## Resumed PR acceptance work

The user requested continued implementation and verified pushes to [PR #4407](https://github.com/kdlbs/kandev/pull/4407). Draft status and all Task 08 limits remain. Rebased onto main `3fb026365239bbc893fa4a5272d926640c7ef1f9`; prior tested head remains in `retention-handoff-d857417`. The overlapping PostgreSQL isolation helpers now share main's robust DSN parser. Real replacement/DSN race checks passed (1.079s).

Pending replay membership now excludes projected inbox rows. Meaningful both-engine RED preceded production changes. Populated failure/cancellation/rollback/reopen and exact frozen-reader generic-plan race GREEN passed (10.051s); broader replay/admission/hash/crash/direct-prefix/index regressions passed (77.265s). Scoped lint, SQL guard, catalog (373 decisions/1524 specs), full spec lint, 22/22 AC coverage and whitespace passed. The complete receipt-plan window passes PostgreSQL and fails SQLite; source stays frozen during each comparison. See Task 08 for all failed windows and their runtime settings.

### PR fixture reconciliation (2026-10-10)

Current-head CI exposed test integration gaps. Owner-cache shutdown RED reproduces the synthetic barrier deadlock; corrected ordering preserves the real Close and publication fence. Configuration audit includes the six implemented aliases. Managed fixtures now prove refusal while retirement is pending and successful deterministic replacement after bounded purge. Queue-settlement fixtures create their exact intended incarnation and persist the required guarded workflow no-op. Five focused race package checks and the settlement/workflow-protection rerun pass; production/native code remains unchanged. Task 08 records intermediate failures, commands and timings. Browser corrections remain local and uncommitted. The latest retained-SQL rollback fixture still kills the agent runtime and fails frozen-N prompt bootstrap; the real PostgreSQL witness preserves that runtime. Apply the same explicit precondition and guaranteed baseline teardown before complete desktop/mobile reruns. Kubernetes CI cancelled-session failures, SQLite latency and final storage/runtime gates remain open.

### Shutdown handoff requested 2026-10-10

- The user requested a pushed incomplete checkpoint within ten minutes before machine shutdown. Preserve draft PR #4407 and Task 08 in progress; no acceptance limit or failure is waived, and no workflow completion signal is sent.
- Backend CI fixture corrections are published at `863b79ec96f43712236d5766e93367a4e5f67ecc`, with targeted race and scoped lint GREEN and normal hooks. The subsequent browser checkpoint changes tests/helpers only; native SHA-256 remains `1d6b09b2a65dc1bb82b055a8cac29916688ebcc29e4162705a9ca8718aa3fe92`, and frozen N remains unchanged.
- Browser fixtures now await the attributed new projected submission before deliberate runtime loss; record frozen-N/mock-helper provenance; warm the deletion session under current; preserve runtime survival across every N-case restart; and return to current survival-disabled baseline during teardown. Normal recovery/capacity/mobile defaults stay disabled. Scoped ESLint has zero warnings and formatting passes.
- Retained-runtime rollback RED: exact terminal receipt comparison observes `block_resolved`/null completion changing to `completed`/timestamp. Earlier compact assertions passed. Establish the relevant snapshot, submission and full-settlement boundary before modifying preservation assertions. Raw RED is `receipt-cache-browser-desktop-survival-case2-red.md`; desktop log is `receipt-cache-browser-desktop-survival.log`. The final mobile matrix is not started under the shutdown deadline.
- Selected immutable binaries and measurement/CI/browser evidence are copied to `/root/.kandev/task-artifacts/backend-agent-delivery-retention-20261010/`, outside `/tmp`, with a checksum manifest. This local artifact directory must be transferred separately; private runner scripts/credentials are excluded.
- Final shutdown desktop run: seven passed, one failed (frozen-N terminal effect snapshot), one interrupted (retry exhaustion) after 7.2m; runner exited 130 on user-requested stop. No final mobile run started. This is not a passing nine-test receipt. Native/frozen binaries are unchanged. Normal checkpoint hooks passed without bypass. The disposable PostgreSQL container `kandev-retention-resume-pg` is stopped, not removed; restart it explicitly before reusing local PostgreSQL runners.
- Final browser diagnosis: failure is at desktop spec line 164 after actual new N manual writes, deletion journaling/ID fencing and pending-job reopen passed. The `written` snapshot captured the new terminal receipt before its asynchronous settlement completed; forward purge was not reached. Next correction must await terminal/workflow settlement before that snapshot and preserve exact equality. Owned Playwright, backend and agent processes are verified gone; both fixture directories were removed. Final mobile was not started.
