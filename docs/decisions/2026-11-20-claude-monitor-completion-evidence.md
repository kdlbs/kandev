# ADR-2026-11-20-claude-monitor-completion-evidence: Claude Monitor completion evidence

**Status:** accepted
**Date:** 2026-11-20
**Area:** backend

## Context

Claude's Monitor tool returns a registration while its script keeps watching.
Users rely on the composer chip to see watches across foreground turns.
Claude ACP v1 can report detached Bash terminal exits. The installed managed
bridge `0.107.0` uses the separately negotiated `asyncTasks` channel for
structured Monitor outcomes. The [rejected negotiation proposal](2026-11-01-claude-async-task-lifecycle-capability.md)
records its SDK and follow-up-turn requirements.

Claude Code `2.1.323` wrote machine-generated task notifications in the exact
native session file. The incident's Monitor outcomes matched Kandev's
registration by both provider task ID and launch tool-call ID. The captured
records are `attachment` entries with `attachment.type=queued_command`,
`attachment.origin`, and `attachment.prompt`; equivalent qualified `user`
records can duplicate the same notification.

## Decision

Prefer qualified ACP terminal evidence. For the managed v1 release, qualify
executor-local native task notifications as supplemental completion evidence
for already tracked Monitors. Read the exact session's newly appended records
under the existing runtime identity. Accept the attested queued-command
attachment and the equivalent user shape. Require the verified version,
schema, origin, native session, timestamp, and both IDs from the Monitor
binding. Map provider `stopped` to Kandev `interrupted`. Publish the normalized
workload outcome through the dedicated background-work event and existing
durable delivery.

Keep the provider file local to the owning executor. Limit access to the
regular session file resolved from the trusted launch environment and
canonical work directory. Open it through a contained nonblocking descriptor,
then verify regularity and file identity. Reject ambiguous paths or changed
file identities.
The reader has no authority over prompts, permissions, transcript messages,
or process control. Its input text and paths remain local.

## Consequences

An attached Monitor stays visible through prompt completion. The verified
completion record retires that watch while the runtime stays alive. Maintain a
captured fixture for each qualified bridge and record version. Missing or
unsupported evidence leaves the tracked state intact. Existing process-end
reconciliation supplies the fallback.

## Alternatives considered

- **ACP only.** A compatible tool-scoped terminal frame is the preferred
  producer. On the managed v1 connection, that path lacks a verified Monitor
  termination frame under the current negotiation.
- **Advertise AIR async tasks.** The capability can supply task identity and
  status. Adopting it requires SDK decoding and separate turn semantics review.
- **Infer completion or Unknown at foreground prompt end.** This hides a
  live watch that outlives its launching prompt. A prompt boundary carries no
  Monitor outcome.
