# ADR-2026-09-14-tool-payload-removal-policy: Tool payload removal policy

**Status:** accepted
**Date:** 2026-09-14
**Area:** backend

## Context

Large tool results increase database storage and backup work. The administrator
wants an optional age policy that removes payloads but retains conversation metadata.
The existing Office retention policy deletes a different class of records.

Removing full tool rows loses ordering, tool identity, and replay deduplication.
Replacing all normalized data also loses control metadata and structured records.
Database payload reduction and filesystem reclamation are different operations.

## Decision

Use an independent policy, disabled by default, on Data & Logs settings.
Use explicit field removal rules and retain tool rows and identity metadata.
Store a durable removal marker and enforce it at message replacement boundaries.
Delayed events must not restore the removed payload.

Offer analysis without requiring enablement. Require a backup choice before the
first cleanup. A selected backup must complete verification before deletion.
Run cleanup in bounded batches after readiness. Keep full compaction explicit.

Use task inactivity as the age basis and protect all active or queued work.
Retain tasks whose eligibility cannot be proved within the query budget.
Report these passes as partial so their estimates cannot imply full coverage.

## Alternatives Considered

- **Compress payloads:** Retains all history, but requires compatible encoding,
  reads, exports, and migrations. It does not implement the requested removal policy.
- **Move payloads to cold storage:** Preserves details but adds storage credentials,
  availability, retrieval, and a separate retention lifecycle.
- **Delete tool messages:** Removes more row/index overhead but breaks the metadata
  preservation requirement and loses replay identities.
- **Remove every normalized field:** Saves more bytes but loses control records,
  attachments, and useful summaries. Explicit rules bound the removal contract.
- **Use creation age:** Easier to explain, but an old task can contain recent work.
- **Compact every pass:** Shrinks the file but introduces large database rewrites
  and foreground contention into a periodic maintenance job.

## Consequences

Removed details are unavailable in conversations. Restoration uses an existing
whole-database backup and can replace newer data. Existing backups remain large.

The policy cannot promise an exact file-size reduction. Unknown payload formats
remain intact until supported rules exist. New formats need explicit coverage.

Implementation and validation use disposable fixtures. Recording this decision
does not enable cleanup on an existing installation.

## References

- [Requirements](../specs/system-page/requirements/tool-payload-retention.md)
- [System design](../specs/system-page/system-design/tool-payload-retention.md)
- [Implementation plan](../plans/tool-payload-retention/plan.md)
