# ADR-2026-10-02-safe-interrupted-conversation-continuation: Separate replay from interrupted conversation continuation

**Status:** accepted
**Date:** 2026-10-02
**Area:** backend, frontend, protocol

The completed-tool admission boundary and continuation prompt are amended by
[ADR-2026-10-05-hidden-completed-tool-continuation](2026-10-05-hidden-completed-tool-continuation.md).
The [capacity continuation amendment](2026-10-05-capacity-continuation-after-completed-tools.md)
adds a narrow exception for completed effects on the same usable runtime. This
decision retains authority over transport-loss continuation and native
restoration. The original replay, native identity, ownership, and rollout
constraints remain.

## Context

Cursor emits a terminal transient diagnostic after a network interruption even
when its ACP subprocess remains reachable. Kandev classifies this correctly but
its pre-result replay fence rejects turns containing thoughts, messages, or
tools. Increasing the replay budget cannot change that admission decision.

The [provider-neutral recovery decision](2026-08-08-provider-neutral-agent-error-recovery.md)
allows replay before work or under a provider retry guarantee. Cursor's
documented `session/load` restores history, but it does not promise resumption
of the same model invocation or exactly-once tool effects. A new generic
`continue` prompt therefore cannot justify replaying completed writes.

## Decision

Use a distinct same-conversation continuation mode for positively supported
interactive concrete profiles. It restores provider history and submits one
new instruction to continue unfinished work. Original-prompt replay retains
its existing no-output/no-tool fence for Cursor without a provider retry
guarantee; the prior decision's provider-guarantee exception remains intact.
Continuation allows output and confirmed,
completed read-only tools, but never state-changing, pending, or ambiguous work.
Native restoration uses advertised `session/resume` when available, otherwise
`session/load` with historical notifications suppressed. Kandev keeps persisted
transcript/tool records; missing native interrupted read results can be recovered
by safe re-reading. Neither method promises preservation of in-flight output.
Provider dialects supply typed capability and tool evidence; orchestration and
UI consume provider-neutral policy and do not inspect provider names or prose.

Both modes share the existing single backend retry owner and bounded episode
budget. A missing native conversation, ambiguous prompt acceptance, or failed
identity restoration stops for manual recovery. Continuation never falls back
to a new provider session. Shipped defaults remain off behind a runtime toggle.

This amends only the interactive post-output admission boundary of the
earlier decision. Dynamic routing, Office, utility calls, and the existing
classifier taxonomy remain under their current contracts. The prior recovery
specification cross-references this distinct lifecycle while preserving replay
admission. The experimental implementation remains off in shipped profiles.

## Consequences

Output-only and supported read-only interruptions can recover automatically.
Completed writes and uncertain tools still need user judgment, so some reported
Cursor failures remain manual. Restore support must be demonstrated with
native Cursor evidence as well as fixtures. Process-local recovery is abandoned
safely on backend restart rather than becoming a second durable scheduler.

The continuation instruction is guidance, not an exactly-once guarantee. The
safety argument rests on the absence of previous state-changing work within
the episode and positive restoration evidence, not the wording of the prompt.

## Alternatives considered

- Remove the output/effect fence or resend every `RetriableError`: repeats
  original work and makes transport classification an execution permission.
- Keep all post-output recovery manual: preserves safety but forces user
  intervention for harmless partial output and completed reads.
- Automatically continue after completed writes with an opt-in policy: useful
  but needs a separate accepted risk contract and action-outcome model.
- Add an independent Cursor retry loop or durable job table: duplicates
  ownership, cancellation, and counters without strengthening restoration.

## Related contract

- [Requirements](../specs/platform/requirements/provider-interruption-continuation.md)
- [System design](../specs/platform/system-design/provider-interruption-continuation.md)
