# ADR-2026-10-28-turn-change-preservation-retries: Retry preservation from accepted turn snapshots

**Status:** superseded by [Git turn history and unified compaction](2026-10-28-git-turn-history-and-unified-compaction.md)
**Date:** 2026-10-28
**Area:** backend

## Context

A completed turn can retain valid start/end snapshots and file counts while an export timeout prevents durable patch storage.
The current coordinator converts export errors into final content unavailability.
Later processing skips finalized turns, so automatic retry cannot recover their content.

The earlier immutable-interval decision requires accepted endpoints and preservation before ephemeral source destruction.
The original requirement also placed content export before successor dispatch.
The current runtime already releases its prompt fence after endpoint acceptance.
This proposal explicitly separates those responsibilities while retaining the cleanup guarantee.

## Decision

Accept immutable terminal endpoints before successor dispatch.
Process historical content independently from the prompt fence, with a bounded recovery window.
Keep transient failures pending and retain their owned refs.
Retry only the original accepted object pair through the original checkout access boundary.

Preservation settlement is separate from provider completion and terminal intent.
Every repository must finish preservation or receive a final failure before ordinary source destruction.
Stop/Cancel keeps its short deadline and can settle missing content explicitly.
Restart can resume verified accepted pairs without taking another workspace snapshot.

## Consequences

Temporary latency no longer immediately removes historical review.
Successor prompts can proceed after immutable endpoint acceptance.
Cleanup, restart handling, and public pending projection must respect unfinished preservation.
The existing database records provide progress and the original recovery deadline.
Retries must preserve completed file content and accurate retained-byte accounting.

## Alternatives Considered

- Increasing only the timeout still treats temporary failures as permanent loss and leaves restart recovery incomplete.
- Retrying finalized rows without eligibility checks can reopen expiry, replace content, or loop indefinitely.
- Holding the prompt fence through all retries adds avoidable latency after the interval is immutable.
- Rebuilding from current files changes history and violates the accepted endpoint contract.
- Adding a general job system adds an unnecessary persistence owner beside the existing lifecycle scheduler.

## Related artifacts

- [Immutable turn intervals](2026-10-07-immutable-turn-change-intervals.md)
- [Requirements](../specs/tasks/requirements/turn-changed-files.md)
- [Preservation design](../specs/tasks/system-design/turn-change-preservation.md)
- [Fix plan](../plans/turn-change-preservation-retry/plan.md)
