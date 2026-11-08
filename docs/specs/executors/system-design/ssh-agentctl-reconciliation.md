---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
created: 2026-11-07
owners:
  - kandev
---

# SSH controller reconciliation after directory loss

## Purpose and boundaries

The executor system owns remote controller process identity and teardown.
The [SSH foundation](ssh-executor.md) remains authoritative for ordinary stops.
This extension supplies a directory-independent path for a recorded controller.

The task system owns cleanup intent, authorization, claims, and inventory
retention. Its [runtime cleanup design](../../tasks/system-design/runtime-cleanup.md)
continues to govern row settlement and workspace cleanup. This extension adds
no public endpoint, UI, task-directory removal, or process discovery sweep.

## Requirement mapping

All criteria below belong to `REQ-EXECUTORS-SSH-EXECUTOR-001`.

| Criteria | Design section |
| --- | --- |
| AC-001.18 | Launch receipt and persistence |
| AC-001.19, AC-001.22 | Remote reconciliation |
| AC-001.20, AC-001.24 | Ownership and race boundaries |
| AC-001.21, AC-001.23 | Outcomes and compatibility |

## Existing failure

`SSHExecutor.stopPersistedRemoteAgentctl` reads the recorded target and calls
`verifyRemoteAgentctlIdentity`. A matching command/workdir still requires
`remoteAgentctlPidFileConfirms`. A positively absent session directory returns
an error. The caller preserves the runtime and never reaches its stop command.

The missing fact is the original process birth identity. Retrying cannot recover
that launch-time fact from a pidfile that no longer exists. The guard remains
correct for records with only PID, command, and workdir evidence.

## Launch receipt and persistence

Introduce a versioned receipt under `ssh_agentctl_launch_receipt` in executor
metadata. Shared wire types belong in a small `internal/common/sshlaunch`
package. Both the backend and standalone helper use that neutral contract.

The receipt contains:

- Version `1` and a fresh random 128-bit launch generation.
- Task ID, session ID, and the original controller instance ID.
- Canonical task and session paths, plus the control server's opaque identity.
- `processidentity.Identity`, captured by the controller for its own PID.
- Remote effective UID and a binding to the saved SSH target tuple.

The birth token uses the existing Linux boot-ID/start-ticks representation.
The target binding covers the host, port, SSH user, and trusted fingerprint.
The receipt contains no token, bootstrap nonce, credential, or environment dump.

On a Linux remote, before each managed launch attempt, the backend supplies a
fresh private `KANDEV_INTERNAL_SSH_LAUNCH_GENERATION` value through
`sshAgentctlLaunchEnv`. macOS launches do not issue proof because the
race-safe pidfd operation is Linux-only; they retain the legacy guarded stop
and confirmed-absence paths.
The task cannot choose this value. It is excluded from agent subprocess
environments. It is an identity marker, not a credential or operator setting.
An optional `SSHLaunchBinding` in the typed `AgentctlStartupConfig` carries the
owner IDs, paths, target binding, and matching generation. This private managed
child contract does not add a public configuration key.

The controller captures its identity before accepting the bootstrap handshake.
Its nonce-authenticated response includes the receipt beside the existing token.
The backend checks the generation, wrapper PID, owner IDs, paths, and target
binding before accepting it. A receipt for a different PID is a launch error.
Unauthenticated `/identity` does not expose paths or process details.
Receipt fields are optional for non-SSH launches. New Linux SSH launches require
the receipt. Retained legacy controllers continue through their existing paths.

The existing `registerAndPublishExecution` persistence barrier stores the receipt
before publishing a successful launch. A persistence error uses the existing
registration rollback and retains failed-stop evidence. No transaction spans SSH.
Crashes before a successful handshake do not acquire directory-loss authority.

The receipt key joins `persistentMetadataKeys`. Same-controller resume preserves
it, even when the local recovered execution ID changes. A replacement, confirmed
dead-controller reset, or broker-refresh launch clears it before creation.
Handshake and port retries cannot reuse another attempt's receipt.

`buildRunningFromExecution` must not restore a removed receipt from prior
metadata. Its current fallback merge needs an explicit exclusion for this key.
Current launch state owns it. JSON round trips preserve birth ticks without
floating-point conversion. Keep the existing string birth token intact.

The authoritative runtime row, and any cleanup snapshot that retains that row,
carry identical receipt bytes. Do not add SQL columns or a second receipt store.
Legacy cleanup snapshots remain readable and never receive invented proof.

## Remote reconciliation

Add a private `agentctl ssh-reconcile` utility command. Dispatch it before normal
server startup. The backend resolves the verified helper for the remote platform
through the existing resolver/upload path. It does not assume the cached binary
supports the command. Executing the utility does not restart the target server.

The request is size-bounded JSON on SSH stdin. It carries the receipt, the
operation, and the saved owner binding. The response is bounded JSON with an
outcome and closed reason code. Missing support, malformed output, channel
failure, and timeout are unknown outcomes. Never substitute a shell `kill`
command after such an error.

Two operations share the same admission and identity checks. `terminate` is the
directory-loss route below. `verify` re-proves the live process and returns
`verified` without signalling, removing, or settling anything; the caller uses
it before its existing guarded pidfile stop or a recovery adoption. A present
receipt therefore always receives a live birth/generation check before any
destructive branch, and only an absent key is legacy evidence. A malformed,
foreign, or PID-mismatched receipt never falls back to the legacy verdict.

For a complete Linux receipt, the helper performs these steps:

1. Check the receipt schema, expected task/session/controller, SSH UID, and paths.
2. Use `lstat`-style inspection of the canonical session path and its parents.
   Require confirmed absence. A present directory, file, symlink, or unreadable
   parent refuses this route before any signal.
3. Open a Linux pidfd for the recorded positive PID. Then recheck the process
   birth token, group/session identity, UID, command/workdir, and launch marker.
   Read procfs through an opened process-directory handle. Keep raw environment
   data only in memory, select the one marker, and discard the remainder.
4. Recheck directory absence immediately before signalling. If ownership changed
   or any probe is ambiguous, close all handles and return without a signal.
5. Send SIGTERM through the retained pidfd. Observe that handle for exit within
   five seconds, bounded by the existing 20-second SSH cleanup context.
6. After exit, require every process in the recorded session to be gone. Return
   success only after confirmed controller exit and session-child drainage. A
   controller that exits while a child survives returns an unresolved outcome,
   and a retry that finds the controller absent still retains that evidence.
   Do not escalate to SIGKILL in this first version.

The helper never creates, removes, renames, or recreates a session directory.
A directory that appears after final admission remains untouched. The pidfd
cannot redirect a signal to its replacement process. Before admission, any
observed replacement path prevents signalling the original process as well.

An absent process plus an absent session directory returns success without a
signal only when no recorded-session child remains. A reused PID, changed boot
identity, or different generation returns an identity-conflict outcome. A
matching command alone never grants authority.

`internal/common/processidentity` already supplies capture, birth-token parsing,
and pidfd signalling patterns. Add only the narrow single-process operation
needed here. `TerminateOwnedSession` requires an exited isolated leader and
cannot stop this live SSH controller. Do not repurpose it.

Normal SIGTERM shutdown remains responsible for stopping agent subprocesses.
The new route adds no detached-process or process-group killer. Integration
coverage must prove normal shutdown drains a child before cleanup settles.
An unconfirmed stop must retain inventory and block later destructive cleanup.

## Ownership and race boundaries

`stopPersistedRemoteAgentctl` validates any present receipt, including its
PID, before every branch. A typed missing-directory result can select the
terminate operation only when a complete receipt belongs to this stop target.
A receipt-bearing stop with a matching pidfile uses the verify operation before
its guarded legacy kill. A receipt-bearing abandoned branch uses the verify
operation to prove both the recorded process and its session children absent
before removing the session directory. Other verification errors retain their
current refusal behavior. Receipt-bearing recovery must check the stronger
identity evidence for a candidate and cannot use PID similarity alone. An
invalid or conflicting receipt never falls back to legacy proof. The caller
does not run `stopRemoteAgentctl` or directory removal after helper success.

`stopPersistedSSHRecord` and pending remote-stop recovery recheck the current
runtime owner before external effects and before settlement. Compare the
identity-bearing record fields (execution, task, session, executor, runtime,
controller instance, PID, session/task paths, target, and launch receipt) and
reuse existing runtime-stop serialization. Volatile runtime metadata does not
participate: the runtime may rewrite idle activity or prepare results between
two reads of the same owner, and treating that as a successor would strand a
live controller. Do not hold SQL locks across SSH. A successor invalidates
stale settlement even when the old process has already exited.

A pending remote-recovery snapshot can move for the same owner when archive
cancels the session. Only that exact delta (session cancelled under a running
task cleanup claim, with workspace, environment ownership and generation,
route, and delivery revision unchanged) may pass the fence. Every other
snapshot change stays fenced. The stop also re-reads the persisted record and
proceeds only when its identity is unchanged. A successor execution or changed
receipt still fails both checks.

PID reuse after pidfd opening cannot redirect the signal. Generation or birth
changes before admission prevent all signals. A lost response after successful
SIGTERM remains unknown until a retry positively observes absence. Success for
one target never implies success for every target in the cleanup operation.

The synchronous helper belongs to the current stop call. Its budget derives
from `sshRemoteCleanupContext`. No background repair goroutine owns it.
The helper enforces its own five-second wait if the SSH channel disappears.
All contexts, process-directory handles, and pidfds close on every exit path.

## Outcomes and compatibility

| Evidence or platform | Outcome | Effect |
| --- | --- | --- |
| Linux, valid receipt, absent directory, original process exits | `stopped` | Settle only the recorded owner |
| Confirmed absent PID and absent directory | `absent` | No signal, idempotent completion |
| Reused PID, changed boot/generation, replacement path | `unresolved: identity_conflict` | Preserve inventory, no signal or removal |
| Live legacy process with missing directory | `unresolved: launch_identity_missing` | Operator reconciliation required |
| Darwin, old helper, or unavailable pidfd | `unresolved: unsupported_identity` | Existing guarded paths remain available |
| Permission, SSH, invalid receipt, or procfs ambiguity | `unresolved: probe_unknown` | Retry evidence remains intact |
| Target remains alive after SIGTERM | `unresolved: termination_unconfirmed` | No forced kill or cleanup success |
| Controller exited, recorded-session child survives | `unresolved: termination_unconfirmed` | Child evidence retained; cleanup not settled |
| Live process matches receipt, `verify` operation | `verified` | Caller keeps its guarded stop; no signal or removal |

These outcomes map onto the existing cleanup retry/failure policy and its error
detail. They introduce no new database state machine. Legacy confirmed-dead
cleanup still uses the existing platform-aware absence probe and directory guard.
An explicit unresolved result includes the recorded owner and a recovery action.
The operator inspects the original host and performs separately authorized
reconciliation. The operator then retries the unchanged cleanup operation.
The result never recommends PID-only killing.

The existing sweep requirement AC-001.13 through AC-001.16 is independent.
This extension does not expand its ownership rules or provide a fallback through
that sweep when persisted cleanup refuses a target.

## Observability and validation

Use fixed outcome/reason fields in the existing cleanup diagnostic. Task,
session, execution, and generation identifiers belong only in structured fields.
Never log the request environment, credential, bootstrap nonce, or raw procfs
contents. No new metric family is required.

Tests cover launch-to-SQL receipt production, same-controller resume, replacement
resets, both signal races, lost responses, legacy records, and mixed targets.
Run the real helper against owned Linux subprocesses and real SSH in the
container E2E fixture. Fake SSH tests separately exercise exact command failures.

## Related decisions and implementation

- [Proposed launch-proof decision](../../../decisions/2026-11-07-ssh-controller-launch-proof.md)
- [Runtime inventory ownership](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
- [Fail-closed cleanup](../../../decisions/0009-fail-closed-gc-semantics.md)
- [Fix package](../../../plans/ssh-controller-directory-loss/plan.md)
