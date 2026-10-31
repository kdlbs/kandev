---
status: draft
system: platform
created: 2026-10-29
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
owners:
  - kandev
---

# Durable agent journal ownership

## Boundary and requirement mapping

Platform owns exclusive journal access, instance disposal, and diagnostic errors.
The [delivery design](durable-agent-delivery.md) retains protocol and admission authority.
Executor retention follows the
[remote-Docker design](../../executors/system-design/remote-docker-executor.md).

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001` | Exclusive ownership. Open diagnostics |
| `REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002` | Retained storage and failed attempts |
| `REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007` | Surviving instances and compatibility |

This design implements existing criteria 001.2, 001.3, 002.1, 007.4, and 007.6.
It introduces no public endpoint, storage format, configuration switch, or
conversation-replacement policy. The existing
[durable session decision](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
and [environment ownership decision](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md)
remain authoritative. No new ADR is needed for this conformance repair.

## Exclusive ownership

`process.NewManager` opens its configured journal before process admission.
`process.Manager.Stop` stops agent work but keeps the manager and its journal
available for restart. `StopForTeardown` closes admission, drains owners,
stops resources, and closes the journal. Closing a backend client does not
dispose of the remote process manager.

A session-scoped journal can outlive a backend execution ID. Reconnect must
resolve that owner before `CreateInstance` opens the same path again.
The file lock remains the final exclusion mechanism. Do not remove the lock,
unlink the database, change its path, or lengthen the timeout to bypass an owner.

## Surviving instances and compatibility

The shared Docker reconnect path carries two identities: the requested
execution and the previous recorded execution. Keep those identities distinct.
Do not change `reconnectInstanceID` to always return the previous execution.
A different session can legitimately create its own instance in a retained
container, with a different owner-scoped journal.

Resolve the requested instance and the previous same-session instance through
the authenticated control API. Validate the task, session, recorded environment,
and delivery identity before reuse or disposal. If a retry references a failed
intermediate instance, use the existing bounded control inventory to identify
the original same-session owner. Do not inspect unrelated filesystem paths.

Classify every relevant candidate before a mutation. One verified candidate
is insufficient if another candidate has unresolved ownership or liveness.
A missing instance is absence only after an authenticated not-found result.
Transport errors, failed inventory, and failed status reads do not prove absence.

| Verified state | Action |
| --- | --- |
| Matching initialized live process | Use existing authenticated adoption and delivery reconciliation. Preserve its journal handle |
| Matching stopped instance, replacement admitted | Dispose of that exact instance, then create the requested execution |
| Previous instance absent, no unresolved competing candidate | Create the admitted instance with its existing journal identity |
| Different session with a different journal owner | Keep that instance. Retain normal distinct-session creation |
| Unknown status, conflicting owner, ambiguous candidates, or failed disposal | Block recovery. Preserve the environment and all unresolved records |
| Positively identified legacy peer | Keep existing eligible legacy behavior. Missing diagnostics never authorize takeover |

For disposal, use the existing `DELETE /api/v1/instances/:id` contract.
Wait for successful teardown before another open. A lost response requires
authenticated reconciliation of that exact instance. Do not force-stop the
container to release a journal. Do not call ACP initialize, load, resume, or
new while adopting a surviving initialized process.

The [instance stop design](agentctl-instance-stop.md) governs failed cleanup,
pointer fencing, provisional ownership, and port release. Reuse its mechanisms.
Do not add another journal-handle cache or a process-wide recovery scanner.

## Retained storage and failed attempts

Keep the journal path and retained records unchanged through recovery.
The Docker container path remains owner-scoped at
`/var/lib/kandev/agentctl-journals/<owner>/delivery.bbolt`.
The backend execution ID does not select a replacement journal.

If configured journal acquisition fails during instance creation, fail before
publishing the instance or starting workspace trackers. Use the existing
provisional cleanup path to close only attempt-local resources. Preserve the
previous manager and its journal. Keep process-start admission fail-closed as
a second guard. Other sessions and instances remain usable.

Serialize reuse, disposal, and creation with existing session and instance
lifecycle ownership. Revalidate the recorded owner at the mutation boundary.
Concurrent resumes must not open two replacement managers. Delayed predecessor
cleanup must not dispose of the successor. A retry after partial teardown must
reconcile current state before another mutation.

Journal failure never selects legacy delivery. It never settles an uncertain
submission, resends an instruction, or changes the native conversation.

## Open diagnostics

Wrap journal-open failures with source-owned typed evidence and retain the
underlying cause through `Unwrap`. Prefer a small open-error type over a
write-error reason that misclassifies acquisition. Preserve `errors.Is` for
`bolt.ErrTimeout`, permission errors, and filesystem errors.

Use a closed reason set: `lock_timeout`, `permission_denied`, `filesystem_full`,
`disk_quota_exceeded`, `io_failure`, `corrupt`, `newer_version`, and `unavailable`.
Classify typed causes rather than matching provider prose.
`lock_timeout` identifies acquisition timeout, not the PID that holds the lock.
Do not claim a lock holder without direct evidence.

At the process-manager boundary, emit one bounded structured diagnostic.
Include the reason, configured timeout, elapsed duration, local PID, and owner identity.
The internal diagnostic can include the trusted journal path and known prior
instance ID. The browser/API error projection contains a safe reason and
actionable summary, without paths, prompts, event payloads, or credentials.
Do not log request environments or raw instance-create bodies.

Keep the current default open timeout. Do not run shell commands to inspect
locks automatically. Reuse `agent_delivery_journal_errors_total` with bounded
labels if classification changes require labels. Identities and paths remain
structured fields, never metric labels. Unknown disk or lock-holder evidence
remains unknown.

## Validation and delivery

Use real bbolt locks for duplicate-open and release tests. Use controlled
filesystem errors for permission and I/O classification. Root execution can
bypass permission bits, so those tests require an explicit capability probe.

Drive reconnect with different old/new execution IDs for the same session.
Cover stopped owners that still hold a journal and surviving live owners.
Add concurrent retries, failed disposal, and distinct-session cases in the same container.
Include a mixed inventory with a verified owner and an unresolved candidate.
Assert no duplicate dispatch, journal replacement, or container deletion.

The [recoverable resume package](../../../plans/remote-docker-recoverable-resume/plan.md)
owns regression delivery. Its disposable upstream reproduction proves the
failure mechanisms. Real remote-Docker recovery remains a separate acceptance
gate. Existing desktop and phone controls remain in use.
