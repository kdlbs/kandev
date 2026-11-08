---
status: current
system: platform
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
created: 2026-09-24
updated: 2026-11-04
owners:
  - kandev
---

# Runtime failure attribution system design

## Purpose and boundaries

This design adds structured, bounded attribution to existing failure paths.
It does not redefine [required-store health](postgres-domain-store-parity.md),
[task status projection](bounded-task-status-delivery.md),
[startup progress](startup-progress-visibility.md), plugin webhook behavior, or
task repository identity checks.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001` | [Diagnostic sites](#diagnostic-sites), [Safety](#safety) |

AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1 and .8 also use
[Database phase diagnostics](#database-phase-diagnostics).
AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9 uses
[Agentctl failure producers](#agentctl-failure-producers).

## Diagnostic sites

| Site | Bounded fields | Evidence boundary |
| --- | --- | --- |
| `internal/persistence/requiredstores/Health` | stage (`writer_acquire`, `writer_ping`, `reader_acquire`, `reader_ping`, `table_probe`), elapsed milliseconds, classified error, writer/reader `sql.DB.Stats` snapshots (`OpenConnections`, `InUse`, `WaitCount`, `WaitDuration`) | One diagnostic per failed sweep. Acquisition and acquired-connection failures remain distinct. Pool pressure does not identify the writer owner. |
| `internal/db/DurabilityBarrier` | fixed operation, outcome, failed stage, error class, numeric phase durations | One warning per failed or slow shared run, after connection release. Per-caller cancellation is separate from the run result. |
| `internal/plugins/Controller.webhook` | plugin ID, HTTP status, origin (`host_lifecycle`, `host_rpc`, `plugin_response`), safe error class for host failures | Record only after the authenticated/declaration gate. A plugin response status is not recast as a host error. |
| `internal/orchestrator/executor` PR-base resolution | task-repository ID, PR number, mismatch reason enum | Have the comparison return a reason while retaining the current boolean decision. Test bound and unbound fork identities and every failure branch. |
| `internal/agent/runtime/lifecycle` recovery | candidate count, retracked count, not-retracked count, classified known reasons, unknown count | Summarize only outcomes the recovery code can prove. A missing backend return is `not_retracked_unknown` unless a specific reason is available; it is not evidence that the process is dead. |

The recovery summary remains separate from counted startup progress.
The [startup progress design](startup-progress-visibility.md#measuring-each-step)
counts conclusively resolved candidates under AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.7.
Retracking success does not determine completed work.
The summary remains one event per pass, including zero-candidate passes.

## Database phase diagnostics

Platform owns this diagnostic contract because it covers shared database capacity.
The task repository retains delivery and journal-release authority.
The [write-cost design](durable-delivery-write-cost.md) retains its durability boundary.

### Probe acquisition

`Health.pingObserved` acquires each connection explicitly through its existing pool.
It calls `PingContext` on the acquired connection and releases it before the next stage.
The writer and reader retain the same shared probe context and two-second runtime deadline.
Maintenance deferral, catalog probes, readiness transitions, and the probe interval remain unchanged.

Acquisition failures use `writer_acquire` or `reader_acquire`.
A failed ping after acquisition keeps `writer_ping` or `reader_ping`.
`elapsed_ms` measures the failing stage.
Connection acquisition can include opening a connection and applying its initialization work.
It is not a measurement of pure pool wait or native lock time.
Existing cumulative pool counters remain correlation evidence.
The health diagnostic does not identify an uninstrumented transaction as the owner.

### Durability run

`DurabilityBarrier.run` owns one local diagnostic record for its actual shared run.
The record measures work after the coalescing window.
`NewDurabilityBarrier` accepts the existing logger as a constructor argument.
The task repository supplies `r.log` when its existing `sync.Once` creates the barrier.
A nil logger preserves operation without diagnostic emission.
The database package already uses this logger for migration diagnostics.
No observer framework, driver wrapper, new metric registry, or background timer is required.

The record contains these fixed fields:

| Field | Meaning |
| --- | --- |
| `operation` | Always `durability_barrier` |
| `outcome` | `success` or `error` for the actual run |
| `stage` | `complete`, `writer_acquire`, `set_full`, `begin`, `generation_write`, `commit`, or `restore_normal` |
| `error_class` | `none`, `context_canceled`, `deadline_exceeded`, or `operation_failed` |
| `elapsed_ms` | Admission through connection release, excluding the coalescing window |
| `acquire_ms` | Time in `sql.DB.Conn`, including connection opening when required |
| `hold_ms` | Successful acquisition through connection release, including discard when required |
| `begin_ms`, `write_ms`, `commit_ms` | Time in the corresponding transaction operation |

The timed phases record their elapsed duration even on error.
Stages without a timing field, such as `set_full` and `restore_normal`, are identified by `stage` only.
Unreached stages omit their timing fields.
An unacquired connection therefore has no `hold_ms` field.
Commit duration can include synchronization and checkpoint work.
It is not reported as pure filesystem synchronization time.

The existing generation helper records its begin, write, and commit boundaries.
Its SQL, transaction order, rollback, and returned errors remain unchanged.
The record emits through the existing logger after connection release and outside the barrier mutex.
The initiating caller receives the actual run result before that emission, so a blocked log sink cannot replace the result with a caller deadline.
Covered-ticket publication remains independent of diagnostic emission.
A failed run or a run lasting at least one second emits one warning.
An ordinary successful run adds no log entry.
The existing per-caller counters and duration buckets keep their current meanings.

The run keeps its independent 30-second context after a waiting caller cancels.
That caller's cancellation cannot finish, cancel, or classify the shared run.
The implementation preserves FULL-before-release and failed-connection discard.
It never acknowledges data because a diagnostic reports success.

This first increment observes probe stages and durability-barrier phases only.
A barrier waiting for the writer does not establish which other operation holds it.
Other writer owners remain unknown until a scoped observation identifies them.

## Agentctl failure producers

`Server.handleAgentConfigure` and the generic error branch of Git status add safe context at their current log sites.
They emit `operation`, `error_class`, and available task, session, and instance UUIDs from the trusted instance configuration.
An instance without task or session identity omits those fields.
No new identity lookup or tracking registry is required.

The configuration refusal in `process.Manager.configure` becomes `ErrConfigureWhileRunning` (new).
Its error text remains `cannot configure while agent is running`.
The handler classifies that sentinel as `agent_running`.
Cancellation and deadline use their existing typed identities.
Other configuration errors use `unknown` without matching arbitrary error text.
Git status reuses `fsdiagnostics.ErrorClass` and `operation=git_inspection`.
A changing-evidence failure keeps its existing warning and adds the same operation, `evidence_changed` class, and trusted identity.
Every other status failure uses the generic error record.
Both the single-repository and multi-repository status paths share that record.

The utility handler adds `operation=agent_probe` to its failure summaries.
When the API server supplies the instance's trusted identity, the summary also carries `instance_id`.
It recognizes only the four existing `ProbeFailureCode` constants.
An empty or unsupported code uses `unknown`.
Direct execution errors use typed cancellation or deadline evidence before `unknown`.
The [agent-owned probe design](../../agents/system-design/probe-failure-diagnostics.md)
continues to own stderr capture, managed classification, and provider redaction.
No new provider signatures or client-visible codes enter this package.

The parent uses the existing typed
[forwarding allowlist](runtime-diagnostic-signal.md#safe-context-allowlist).
That design owns the exact additional operation and error-class values.
Raw `error` fields remain excluded from forwarded context.
The producer does not concatenate an unsanitized error into its message to bypass that exclusion.

## Verification of phase diagnostics

File-backed tests distinguish a held connection from an acquired connection with a failed ping.
They retain maintenance, missing-schema, cancellation, and recovery controls.
Barrier tests distinguish admission from ownership and commit timings without machine-specific latency limits.
They cover coalescing, cancellation of the initiating caller, failed connection discard, and quiet fast success.
An unreached commit has no commit timing or successful result.
The existing delivery tests retain FULL-before-journal-release evidence.

Producer tests cover the configuration sentinel, classified Git errors, and known or unknown probe codes.
Forwarding tests use the real JSON encoder at an info threshold.
They retain allowed fields and exclude raw errors, paths, command arguments, credentials, and invalid UUIDs.
No browser surface changes in this package.

## Safety

### Recovery enumeration outcomes

For AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.5, carry per-pass typed outcomes
from `StandaloneExecutor.RecoverInstances` through `ExecutorRegistry` to
`Manager.Start`. Add an optional detailed recovery interface alongside
`ExecutorBackend.RecoverInstances`; existing backends and callers remain valid.
The detailed path returns recovered instances and outcomes keyed by the exact
input record identity. The registry invokes each backend once and accepts
outcomes only for records owned by that backend. A backend without detailed
outcomes retains the existing unknown fallback.

Standalone successful enumeration classifies records with no matching session
as `no_matching_instance`; failed enumeration reports `enumeration_failed`.
Correlation ambiguity, stop refusal, cancellation, and deadline remain separate
when the existing branch proves them, otherwise unknown. A winner later refused
by Manager reconstruction receives only that reconstruction reason. Never use
the total minus returned-instance count as evidence of absence. Plugin recovery
retains unknown for omissions until its own provider can prove a specific
outcome; standalone evidence cannot classify plugin records.

The summary adds fixed numeric fields for these outcomes, retains existing
fields, and resets its state on every pass. Internal record keys are not logged.
Candidate count equals retracked plus classified not-retracked plus unknown
for known inventory, including mixed-runtime and partially recovered sets.
The candidate completion ledger consumes only conclusive outcomes.
It advances each input candidate at most once and leaves unresolved work incomplete.
Recovery guards, stops, deadlines, persistence, and readiness retain their existing policy.
The existing warning remains for a genuinely incomplete pass.

### Cleanup inspection attribution

For AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6, preserve the task-owned
[cleanup contract](../../tasks/system-design/runtime-cleanup.md), including
immutable snapshots, changed-branch refusal, and bounded/cascade retry policy.
`worktree.Manager.branchExists` must return absence only for Git's documented
missing-ref result. Use quiet verification to distinguish that result from
fatal repository/command errors; retain subprocess admission and timeout bounds.
Propagate start errors and other Git exits with the underlying error intact.

The audit boundary attaches a closed inspection stage and reason to failures
from branch lookup and registration inspection. Differentiate repository-context
unavailability from executable-launch failure without claiming ENOENT proves
either one when evidence is ambiguous. Existing ownership and commit mismatch
branches get their own typed reasons. The cleanup worker logs stage/reason and
attempt alongside its existing job/task identity, without adding raw command
output, environment, or new filesystem paths. Wrapped errors remain compatible
with existing dirty-worktree classification and retry policy.

Do not refresh an audited commit to the current branch, adopt another task's
path, mark an inspection failure complete, or mutate historical cleanup jobs.
The underlying records can still require explicit operator reconciliation.

### Cleanup preparation and deletion preview

The same AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6 contract applies before a
worker exists. `CaptureCleanupHeadOIDs`, `captureCleanupBranchOID`, and
`InspectDirtyWorktrees` must preserve typed inspection attribution. A surviving
checkout can have a `.git` pointer into a deleted source repository; directory
presence alone does not establish usable Git metadata. The
[task preparation contract](../../tasks/system-design/runtime-cleanup-preparation.md)
continues to reject uncertain identity.

Use `CleanupInspectionError` with the existing `commit_lookup` and
`branch_lookup` stages and an additive `working_tree_status` stage. Reuse the
existing closed reason vocabulary. Classify a demonstrably absent recorded
source repository as `repository_context_unavailable`. When only the worktree
administrative directory is missing, use the existing linked-worktree
inspection evidence to establish that narrower failure. A bare exit 128,
permission error, malformed pointer, or ambiguous inspection remains a command
failure unless stronger evidence establishes another existing reason.
Cancellation and deadline identity retain precedence. Keep the original error
wrapped, and never include raw Git output in the outward diagnostic.

`Service.TaskDeletePreflight` preserves both
`ErrTaskDeletePreflightUnavailable` and the underlying inspection error through
wrapping. Its HTTP handler emits one bounded diagnostic for a classified
inspection failure, including stage, reason, and selection count. Archive
request logging includes the same stage/reason fields when its error chain
contains the typed error. Existing HTTP statuses and public response bodies
remain unchanged; the generic preflight response must not expose internal paths.

Mixed inventories retain every slot until an explicit lifecycle or operator
action changes it. A healthy sibling cannot turn an unknown slot into a clean
result or authorize deletion. No diagnostic path retires database rows,
recreates repositories, deletes checkouts, issues discard consent, or marks a
cleanup job successful. Repairing unavailable repository data is a separate
operator action with preservation evidence.

## Diagnostic safety

Do not log SQL, DSNs, webhook URLs or keys, request queries, headers, bodies,
tokens, local paths, repository URLs, or raw comparison identities. Use fixed
enum values and numeric measurements; keep identifiers to existing safe task,
plugin, and task-repository IDs. Diagnostics do not change any return value,
readiness state, retry, stop decision, or persisted record.

## Related decisions

- [Required internal persistence](../../../decisions/2026-09-05-required-internal-persistence.md)
- [Maintenance health-probe coordination](../../../decisions/2026-09-17-maintenance-health-probe-coordination.md)

## Implementation plan

- [Recent runtime log remediation](../../../plans/recent-runtime-log-remediation/plan.md)
- [Startup log corrections](../../../plans/startup-log-corrections/plan.md)
- [Cleanup preparation failure attribution](../../../plans/cleanup-preparation-attribution/plan.md)
- [Startup and shutdown diagnostic integrity](../../../plans/startup-shutdown-diagnostic-integrity/plan.md)
- [Runtime stall diagnostics](../../../plans/runtime-stall-diagnostics/plan.md)
