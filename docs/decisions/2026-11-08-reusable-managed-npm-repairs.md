# ADR-2026-11-08-reusable-managed-npm-repairs: Reuse validated repairs without changing shared npm trees

**Status:** accepted
**Date:** 2026-11-08
**Area:** backend, agentctl, protocol

## Context

An incomplete Codex ACP execution tree repeatedly failed during saved-session
resume. Each automatic recovery installed the selected version privately.
Later cold launches still selected the incomplete shared tree.

The authorized maintenance repair rebuilt that exact shared entry. npm then
reported a valid installation, and one normal launch completed initialization.
This proves that complete installation files remove this observed failure.
It does not identify the original reason those files disappeared.

Manual maintenance can inspect current consumers before replacing shared files.
Automatic recovery cannot establish that every external process stopped using
them. Process-list inspection also races with new launches and is not portable.

## Decision

Publish successful repairs in an agentctl-owned store on the execution host.
Reuse a matching validated generation before a later cold launch starts npm.
Keep the logical managed command and exact selected runtime unchanged.
Apply only the owned project/cache paths at the final execution boundary.

Build a generation only after requirement 006's verified failure. Publish it
only after package validation and successful protocol initialization.
The publication key includes the execution user, platform, trusted package,
npm configuration, toolchain, and authorized installation environment.

Keep generation paths stable for their entire lifetime. Kandev does not rebuild
or remove published generations automatically. npm retains its ordinary cache
metadata behavior inside the selected generation.
An invalid generation loses selection eligibility without losing its files.
This protects older processes that still use those files.

Coordinate preparation and publication by installation context across local
agentctl processes. Do not introduce a global launch mutex.
Retain existing per-operation cancellation, process ownership, and retry limits.
Unsupported peers or unavailable reusable storage retain private recovery.

This decision extends the lifetime boundary in
[isolated cache recovery](2026-11-07-isolated-managed-npm-cache-recovery.md).
It preserves the prohibition on automatic shared-tree mutation in
[bounded startup retry](2026-10-02-bounded-managed-npm-startup-retry.md).
Implementation is recorded in the linked plan and work orders.

## Consequences

Compatible later launches avoid repeated preparation and attempt-2 progress.
Existing agents keep their files through session cleanup and backend adoption.
The backend does not own remote paths or copy npm trees between executors.

The store adds durable filesystem metadata, context identity, and publication
coordination. It retains published generations until explicit maintenance.
Disk use therefore follows distinct repair generations rather than session count.
Automatic garbage collection remains separate work.

## Alternatives considered

- Repeat the original npx command: it can select the same incomplete tree.
- Refresh npm metadata only: freshness does not restore absent execution files.
- Remove the shared entry after a process-list check: untracked consumers and new launches can race with removal.
- Repair and preserve shared trees through cross-process consumer leases: older peers and external npm users cannot supply complete ownership evidence.
- Keep every repair private to one operation: preserves safety but repeats installation after owner teardown.
- Select a different runtime version: changes the user's selected runtime.
- Publish before initialization: exposes installations that never demonstrated protocol startup.

## References

- [Requirements](../specs/agents/requirements/managed-npm-repair-reuse.md)
- [System design](../specs/agents/system-design/managed-npm-repair-reuse.md)
- [Plan](../plans/managed-npm-repair-reuse/plan.md)
