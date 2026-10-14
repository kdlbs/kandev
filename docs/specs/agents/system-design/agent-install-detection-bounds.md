---
status: current
system: agents
requirements:
  - REQ-AGENTS-INSTALL-DETECTION-001
created: 2026-10-12
owners:
  - kandev
---

# Agent Install Detection Bounds System Design

## Purpose and boundaries

`internal/agent/agents/detect.go` owns how a detection option runs a capability
check. `internal/agent/discovery` owns the sweep that runs every enabled agent's
`IsInstalled` concurrently under a 15s budget. This design distinguishes an
elapsed bound from a nonzero exit and gives Hermes one second measurement.
Host CLI version checks start as installation results arrive so that a slow
installation check does not consume other agents' version-check budget. The
sweep's overall budget, installation concurrency, and cache are unchanged.

Detection answers only whether the executable and its capability check are
present and healthy. Authentication and protocol compatibility still belong to
the ACP probe path.

## Components

| Surface | Responsibility |
| --- | --- |
| `runCommandCheck` | Runs one bounded check and reports `found`, `timedOut`, and caller cancellation |
| `WithCommandCheck` | Unchanged contract, now expressed through `runCommandCheck` |
| `withHermesACPCheck` | Staged measurement for Hermes, the one agent that needs it |
| `Registry.detectAll` | Starts version checks per completed installation result and joins them before returning |

## Bounded check

`runCommandCheck(ctx, path, timeout, args...)` returns four distinct outcomes:

| Outcome | `found` | `timedOut` | `err` |
| --- | --- | --- | --- |
| Command exited 0 | `true` | `false` | `nil` |
| Command exited nonzero or could not start | `false` | `false` | `nil` |
| Bound elapsed first | `false` | `true` | `nil` |
| Caller cancelled | `false` | `false` | `ctx.Err()` |

`timedOut` requires the check context's own deadline and a successful
cancellation of the subprocess, after the parent context is ruled out. A
deadline observed after a conclusive exit cannot trigger another measurement.
The cancellation callback records successful termination atomically; `Run`
joins the process and context watcher before classification. POSIX additionally
preserves a naturally exited process state when a kill races the exit. Windows
reports terminated processes as exited, so it uses the cancellation callback's
result. A caller cancellation is never reported as a timeout.
A joined successful process state remains available even if the context watcher
returned a late deadline error; a successful exit is a conclusive answer too.
If the check's own deadline prevents the process from starting at all, that
remains an inconclusive timeout; there is no command exit to classify.
`WithCommandCheck` keeps its previous contract exactly: everything except
cancellation maps to unavailable. Agents that use it, such as MiniMax, Muse, and
Pi, see no behavior change.

## Staged measurement

```text
LookPath(hermes) -- missing -------------------------------> unavailable
      |
      +-- runCommandCheck(first = 5s)
      |     +-- exited 0 ----------------------------------> available
      |     +-- exited nonzero ----------------------------> unavailable (conclusive)
      |     +-- caller cancelled --------------------------> error
      |     +-- bound elapsed
      |
      +-- runCommandCheck(second = 8s) -- final ----------> available | unavailable | error
```

The second measurement exists only because the first bound elapsed. The first
bound is the fast path and the second is a fallback, so an install that answers
promptly is never measured twice.

## Why Hermes needs the staged form

`hermes acp --check` resolves the ACP extra on every run, and discovery starts
every enabled agent's probe in its own goroutine. At sweep width the check was
measured at 5.5s to 12.1s while exiting 0, against a shared 5s bound, which is
why an installed Hermes was reported as missing on every sweep.

The bounds sum to 13s, inside the 15s sweep budget, so the sweep stays bounded
even when both measurements run.

## Version checks within the sweep

Each installation worker sends exactly one result carrying its original slice
index. The collector writes that result into its own slot and starts a host CLI
version worker for that slot immediately. Version workers only update their
own slot; the collector joins every started worker before assembling partial
results or publishing them to the detection cache.

Version checks retain the shared four-process semaphore, five-second version
timeout, bounded output and generation-fenced cache. They use the same sweep
context, so cancellation also stops queued or active version work. Completed
installation results remain in a partial response when a version check is
cancelled. Registry lookups and host CLI cache access retain their existing
locks; no lock spans a subprocess invocation.

## Latency trade-off

Only a Hermes check whose first bound elapses costs more time. Callers that
check agents one at a time, such as inference agent listing, host utility
bootstrap and refresh pre-checks, and the settings capability synthesis, wait up
to 13s for Hermes in that case instead of 5s. Fast, failed, and missing installs
take the same time as before. This is accepted in exchange for reporting a slow
but healthy install correctly; a check that keeps hanging is still reported
unavailable after the second bound.

## Invariants

- A conclusive failure is measured at most once.
- An elapsed bound is the only reason a second measurement happens.
- A caller cancellation is an error at either measurement and is never turned
  into "unavailable".
- Every other agent's detection path and bound are unchanged.
- No configuration key, setting, or exported API exists for a bound.

## Alternatives rejected

- **Raise the shared bound for every agent.** Slows every detection sweep,
  including for the many agents that are not installed, to fix one agent.
- **One longer Hermes-specific bound with no retry.** Smaller, but it cannot
  tell a slow check from a failed one, and a hanging Hermes would delay every
  caller by the full bound.
- **Retry on any failure.** Re-runs conclusive failures and adds latency for an
  install that is already known to be unusable.

## Testing

Unit tests cover `runCommandCheck` directly with a real subprocess for success,
nonzero exit, elapsed bound, caller cancellation, and a binary that cannot
start. Scripted staged measurements assert exact attempt counts independently
of subprocess scheduling. Channel barriers prove the second check has started
before caller cancellation or a parent deadline, with `testing/synctest` driving
the deadline deterministically. Both cases assert exactly two attempts, the
unchanged caller error, an unavailable result and joined check cleanup.
The nonzero-exit boundary regression uses a real helper's terminal process
state and a deterministic expired check context, without a timing race. It
proves a deadline alone cannot turn a conclusive failure into a retry.
A real successful helper's process state also remains available when a local
deadline is observed afterward, with and without a racing cancellation callback.
An expired-before-start bound remains a timeout and starts no subprocess.

Discovery tests prove a healthy version check starts while another installation
check is blocked, and that cancellation retains completed availability after
joining the version worker.

The Hermes test helper copies the test binary instead of hard-linking it. On
Windows a hard link to the running image stays locked for the test's lifetime,
so the temporary-directory cleanup could not remove it.
The Backend workflow's bounded native Windows step executes `TestHermesACP`
and `TestRunCommandCheck` with race detection, including helper cleanup. The
matching `test-windows` Make target and workflow-selection contract preserve
this coverage; compilation alone does not verify it.
