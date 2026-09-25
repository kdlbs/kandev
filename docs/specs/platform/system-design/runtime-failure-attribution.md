---
status: current
system: platform
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
created: 2026-09-24
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

## Diagnostic sites

| Site | Bounded fields | Evidence boundary |
| --- | --- | --- |
| `internal/persistence/requiredstores/Health` | stage (`writer_ping`, `reader_ping`, `table_probe`), elapsed milliseconds, classified error, writer/reader `sql.DB.Stats` snapshots (`OpenConnections`, `InUse`, `WaitCount`, `WaitDuration`) | Capture on probe failure before cancellation; one diagnostic per failed sweep, not one per catalog store. Pool pressure is correlation evidence, not proof of the cause. |
| `internal/plugins/Controller.webhook` | plugin ID, HTTP status, origin (`host_lifecycle`, `host_rpc`, `plugin_response`), safe error class for host failures | Record only after the authenticated/declaration gate. A plugin response status is not recast as a host error. |
| `internal/orchestrator/executor` PR-base resolution | task-repository ID, PR number, mismatch reason enum | Have the comparison return a reason while retaining the current boolean decision. Test bound and unbound fork identities and every failure branch. |
| `internal/agent/runtime/lifecycle` recovery | candidate count, retracked count, not-retracked count, classified known reasons, unknown count | Summarize only outcomes the recovery code can prove. A missing backend return is `not_retracked_unknown` unless a specific reason is available; it is not evidence that the process is dead. |

The recovery summary is separate from the counted startup progress step. The
step continues to advance only at its current reconstruction points and can
end below total with the existing warning. The summary is a single event for
one recovery pass, including zero-candidate passes; it does not log one row per
old inventory record.

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
No new outcome increments startup progress or changes recovery guards, stops,
deadlines, persistence, or readiness. The existing below-total warning remains.

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
