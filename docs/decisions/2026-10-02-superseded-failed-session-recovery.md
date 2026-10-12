# ADR-2026-10-02-superseded-failed-session-recovery: Restrict passive recovery of superseded failures

**Status:** accepted
**Date:** 2026-10-02
**Area:** workflow

## Context

[Issue 4152](https://github.com/kdlbs/kandev/issues/4152) reports an old FAILED conversation resuming beside its replacement.
Existing policy permits recovery on open and intentional concurrent sessions.
The user selected a narrow failed-session restriction during investigation.

## Decision

Suppress passive recovery of a FAILED conversation when another conversation is primary and at least one sibling is STARTING or RUNNING.
Keep explicit recovery, ordinary automatic recovery, and remembered conversation selection.
Use the existing status and passive admission guards. Recheck current ownership at admission.

This qualifies the [conversation-open decision](2026-09-18-session-open-resumes-conversation.md) only for this failed-session condition.
It does not change the co-residency observer into an admission guard.

## Consequences

Users can inspect a superseded failure without reviving its agent beside working siblings.
Explicit recovery can still produce concurrent agents. A later sibling start remains permitted.
Unavailable ownership information blocks passive recovery of the affected candidate.
The ACP initialization failure remains a separate, unconfirmed cause.

## Alternatives Considered

- Disable all recovery on open: rejected because the user selected the narrow exception.
- Refuse all concurrent sessions: rejected because intentional concurrency remains supported.
- Always select primary: rejected because remembered conversation selection remains supported.
- Serialize npm launches or retry every ACP error: no confirmed causal evidence supports this repair.

## Related records

- [Requirements](../specs/tasks/requirements/queued-session-ownership.md)
- [Design](../specs/tasks/system-design/queued-session-ownership.md)
- [Fix package](../plans/superseded-failed-session-recovery/plan.md)
