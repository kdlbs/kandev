# ADR-2026-11-07-ssh-controller-launch-proof: Preserve SSH controller launch proof

**Status:** accepted
**Date:** 2026-11-07
**Area:** backend, protocol

## Context

[Issue #5145](https://github.com/kdlbs/kandev/issues/5145) describes a live SSH
controller whose session directory disappeared. Persisted cleanup requires that
directory's pidfile. Every retry returns the same identity error.

The guard protects sibling sessions and replacement launches that share a task
workdir. Removing it permits signals based on incomplete ownership evidence.
A timestamp sampled during recovery cannot establish which launch created a
process. A plain PID signal also permits reuse between inspection and signalling.

## Decision

The executor records immutable, self-reported process identity during the
nonce-authenticated launch handshake. The record binds the host process birth
identity, launch generation, task, session, and controller instance.

The existing `executors_running.metadata` stores this proof outside the
disposable session directory. Durable cleanup retains the same proof with its
owned runtime inventory. Recovery never upgrades an incomplete legacy record
from observations made during cleanup.

A private remote `agentctl` helper can reconcile a missing-directory owner on
Linux. It opens a pidfd, rechecks launch evidence, and signals through that
handle. It never removes a directory. A missing or contradictory proof keeps
the operation unresolved. Existing guarded paths remain available.

The first version requests SIGTERM, observes controller exit, then requires
the recorded session's children to be gone within the existing cleanup
deadline. It does not force-kill an unresponsive controller or infer authority
over detached descendants. The existing controller shutdown owns child
teardown. Unconfirmed controller or child shutdown keeps cleanup pending.

A verify-only helper operation re-proves the live process before the existing
guarded pidfile stop and before recovery adoption. Present proof that conflicts
with the record never degrades to legacy evidence.

## Consequences

Successfully recorded launches have a recovery path after directory loss.
Replacement PIDs cannot receive a signal intended for the original process.
The helper needs no remote Python, shell `kill` extension, or privileged daemon.

Legacy entries remain unresolved until the original process exits or an operator
performs a separate authorized intervention. Recreating a pidfile cannot grant
authority. Unsupported kernels and Darwin retain conservative behavior.

The private receipt and helper protocol become compatibility contracts. Launch,
resume, metadata replacement, cleanup snapshots, and retries must preserve their
generation boundaries.

## Alternatives Considered

- Accept command/workdir similarity after directory loss. Rejected because
  siblings and replacement controllers share those values.
- Record process start identity during cleanup. Rejected because the process
  can already be a replacement.
- Recreate the missing pidfile. Rejected because it fabricates the missing proof.
- Use authenticated HTTP shutdown alone. It is useful when the control server
  responds, but it cannot replace durable OS identity for a failed control path.
- Add a second remote receipt registry. Deferred because the durable backend
  inventory already survives session-directory loss. Pre-handshake launch
  crashes remain outside this repair.
- Use a PID signal after a successful probe. Rejected because the PID can change
  owners before delivery. Linux pidfds retain the process reference.

## References

- [Executor reconciliation design](../specs/executors/system-design/ssh-agentctl-reconciliation.md)
- [Runtime inventory ownership](0025-runtime-cleanup-uses-executors-running.md)
- [Fail-closed cleanup](0009-fail-closed-gc-semantics.md)
- [Linux pidfd signal semantics](https://man7.org/linux/man-pages/man2/pidfd_send_signal.2.html)
