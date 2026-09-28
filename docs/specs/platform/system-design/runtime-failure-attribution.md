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
