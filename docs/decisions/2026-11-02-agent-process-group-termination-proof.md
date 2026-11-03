# ADR-2026-11-02-agent-process-group-termination-proof: Prove original-owner termination per agent process group

**Status:** accepted
**Date:** 2026-11-02
**Area:** backend

## Context

Durable delivery recovery restores or retires interrupted work only after it proves that the
original owner stopped writing. The prompt binding records the agentctl runtime process
identity. Every local session shares one standalone agentctl, and that agentctl survives
backend restarts. Its owned OS session therefore stays alive while any other task runs. In
that state, a session whose agent already exited cannot prove termination: automatic restart
recovery never runs, and the UI offers only Retry connection and Stop. The operator has to
kill the shared agentctl, and with it every other running agent.

agentctl starts each agent child with `Setpgid`, so every agent owns a distinct process group
inside the shared agentctl OS session.

## Decision

Record the agent child's process-group birth identity in the durable prompt binding, as
reported by agentctl's authenticated delivery descriptor. Original-owner termination is
proven when the runtime's owned session has terminated, or when no process remains in the
recorded agent group with a matching birth identity. Missing, malformed, or reused identities
prove nothing. Bindings that predate the group identity keep the runtime-only rule.

## Consequences

Recovery of one session no longer depends on unrelated sessions or on the agentctl lifetime.
The agentctl delivery descriptor gains an additive field, so older peers fall back to the
runtime-only rule. Descendants that leave the agent's process group are not covered by group
proof. This matches the existing containment boundary, which signals only owned groups.

## Alternatives Considered

- Keep the runtime-only proof and expose explicit Resume when agentctl reports the instance
  missing. Automatic restart recovery stays blocked for a common local setup.
- Trust agentctl's "instance not found" alone. An instance record can disappear while its
  child process still runs, so an existence check is not process evidence.
