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

On Linux and Darwin, agentctl starts each persistent agent child with `Setpgid`.
Each child owns a distinct process group inside the shared agentctl OS session.

## Decision

Record the agent child's process-group birth identity in the durable prompt binding, as
reported by agentctl's authenticated delivery descriptor. On Linux, original-owner termination
is proven when the runtime's owned session has terminated or the recorded agent group no
longer exists. Group proof requires a valid birth token. A different boot proves that the
original group ended. On the same boot, a kernel existence check must report `ESRCH`.
An existing group, including a reused group ID, does not prove termination.
Missing or malformed identities prove nothing. Bindings without group evidence keep the
runtime-only rule. Darwin retains runtime-session proof because group inspection is unsupported.
Windows retains its existing unverifiable result.

## Consequences

Linux group evidence removes the shared runtime lifetime from the original-owner proof.
Existing execution and recovery admission checks remain authoritative.
Standalone liveness can report the shared agentctl instance as alive while its session's agent
group has exited. That path requires the exact saved agent-group proof. Unknown liveness uses
the original-owner proof. Both paths retain the pinned execution check and conditional owner
revalidation before cleanup.
The agentctl delivery descriptor gains an additive field, so older peers retain the
runtime-only rule. The kernel check sends no signal. An unreaped group member can delay proof
until the operating system removes the group. Descendants that leave the agent's process group
are not covered by group proof. This matches the existing containment boundary, which signals only owned groups.

## Alternatives Considered

- Keep the runtime-only proof and expose explicit Resume when agentctl reports the instance
  missing. Automatic restart recovery stays blocked for a common local setup.
- Trust agentctl's "instance not found" alone. An instance record can disappear while its
  child process still runs, so an existence check is not process evidence.
