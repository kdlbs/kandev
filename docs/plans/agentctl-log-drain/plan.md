---
created: 2026-10-06
status: done
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-003
system_design:
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
legacy_specs: []
---

# Fix plan: Continuous agentctl log draining

## Overview

Prevent an oversized agentctl log record from silently stopping its stdout or stderr reader.
One sequential work order delivers regression tests, bounded draining, and targeted compatibility validation.
Implementation is complete. The launcher preserves continuous draining with bounded memory and content-free diagnostics.

Platform owns shared diagnostic forwarding and operational safety.
The existing runtime diagnostic pair owns this extension. No separate repair specification is necessary.
The completed [JSON severity work order](../runtime-log-reliability/task-04-agentctl-json-severity.md) remains completed.
This work preserves its severity and sanitization results.

## Scope

### In scope

- Continuous draining through oversized records on both managed child streams.
- Fixed memory, complete-record forwarding, and bounded diagnostics.
- Regression evidence for pipe writers, subprocess writers, EOF, read errors, and descriptor closure.
- Compatibility with existing launch, stop, and survival tests.

### Out of scope

- Live restarts, deployment, task/session changes, commits, or pushes.
- Changing readiness budgets, coalesced cancellation ownership, or stale-execution removal policy.
- Changing resume errors, ACP transport, global log thresholds, or raw provider diagnostics.
- New runtime flags, public configuration, UI changes, log queues, or process-supervision redesign.

## Investigation evidence

The planning checkout starts at `0235c4f833`.
The incident evidence identifies deployed revision `01a7615b4bf`.
Both revisions use default scanners and omit `scanner.Err()` handling in `Launcher.pipeOutput`.

| Finding | Evidence | Interpretation |
| --- | --- | --- |
| Confirmed code defect | Isolated reproduction against the current real method: a 128 KiB record causes `bufio.ErrTooLong`, reader exit, and a blocked writer. | Increasing the ceiling only moves the failure threshold. |
| Historical missing stdout reader | Both retained backend profiles contain the stderr reader and `monitorExit`, but no stdout reader. | Supports premature stdout-reader exit. It does not identify the read error. |
| Historical control outage | Saved report records health timeouts on shared control port 39429 and instance ports 41065/41061. Main backend remained responsive. | Compatible with synchronous child logging blockage. |
| Historical cleanup failure | Retained log lines 35645 and 35852 show cleanup followed by a 30-second DELETE timeout for execution `0c7418f3-faba-48ab-a5ac-058e1a5bd4d8`. | The runtime stop never confirmed success. Tracking therefore remained. |
| Historical readiness failure | Retained log lines 48431 and 48560 identify launch deadline and `agentctl not ready` errors for task `90561f47-7307-4ade-8963-29db44c6eba7`. | Consistent with control unavailability before instance allocation. |
| Current health | At 2026-10-06 15:57 UTC, control port 39429 responded and backend reported `v0.97.0-51-g0235c4f833`. | The historical outage is not currently reproducible through health. Its recovery cause is not established here. |

Retained evidence paths:

- `/tmp/kandev-investigation-2026-10-06.json`
- `/tmp/kandev-investigation-goroutines.txt`
- `/tmp/kandev-investigation-goroutines-latest.txt`
- `/tmp/kandev-agentctl-pipe-repro_test.go`
- `/root/.kandev/logs/backend-logs.log`

These host paths are supporting pointers, not portable implementation inputs.
No raw prompts, transcripts, credentials, or tool payloads belong in this package.

### Root cause and uncertainty

The confirmed defect is silent drainer termination after an oversized Scanner token.
The open read descriptor leaves later child writes blocked after the kernel pipe fills.
`logger.NewLogger` writes ordinary agentctl logs synchronously to stdout.
`httpmw.RequestLogger` logs after `c.Next()` and before handler return.
A blocked log write can delay response completion, including small health responses.
Existing WebSocket events can continue through paths that do not reach the blocked logger.

The exact live oversized record and its emitter remain unknown.
The available snapshots are backend profiles, not an agentctl stack proving a blocked stdout write.
The reproduction proves the failure mechanism, not the historical trigger.

Alternative reader-exit causes include child stdout closure, EOF, and another pipe read error.
Those causes also disappear silently because the current loop omits error handling.
The still-waiting process monitor argues against normal child exit as the explanation for the saved profiles.
A parent logger blockage normally leaves `pipeOutput` in the stack rather than removing the reader goroutine.
The saved resource readings do not establish OOM, disk exhaustion, database admission, or CPU saturation as the cause.
No live restart or mutation was necessary for this investigation.

### Related readiness and resume findings

`StandaloneExecutor.waitForReady` first calls `Health(ctx)` before creating its fallback timeout.
The control HTTP client permits a 30-second request.
An absent caller deadline therefore does not bound the initial probe to ten seconds.
With a caller deadline, retries use that entire budget instead of an independent readiness deadline.
The active setup contract permits ten minutes plus a five-minute launch allowance (`AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.7`).
That budget explains the approximately fifteen-minute launch wait during control unavailability.
The first-probe budget gap is independent hardening work. This package does not change it.
`coalescedExecutionContext` intentionally removes individual caller cancellation and retains manager shutdown cancellation.
It is not evidence of an accidental leak by itself.

`cleanupStaleExecution` preserves tracking when runtime stop fails.
`TestManager_CleanupStaleExecution_PreservesTrackingOnStopError` codifies that safety rule.
Resume logs the cleanup error, retries launch, and can replace the useful stop error with an already-running error.
That error-quality issue is independent of the log reader, although the outage exposes it.
A follow-up must preserve tracking and avoid duplicate launches after unconfirmed cleanup.
No persistent follow-up task is created by this plan.

## Technical approach

Change the private drainer input from `*bufio.Scanner` to `io.Reader`.
Use fixed buffered fragments and a 64 KiB accumulation limit as defined in the linked design.
Discard an oversized record through its delimiter, then resume normal records.
Keep the existing `childLogRecord` parser and level mapping for complete, bounded records.
Use one content-free oversize warning per stream lifetime and one diagnostic for an unexpected read failure.
Leave process and pipe ownership in the launcher.

| Runtime path | Transport | Result | Evidence |
| --- | --- | --- | --- |
| Managed standalone stdout | OS pipe, trusted child logs | Skip oversized records and preserve normal levels | Pipe and subprocess regressions |
| Managed standalone stderr | OS pipe, trusted child logs | Same bounded drain, existing warning fallback | Same regressions, both streams |
| Structured JSON | Validated child envelope | Preserve severity and omit additional fields | Existing JSON observer tests |
| Console and slog | Existing recognized formats | Preserve current levels | Existing parser and relay tests |
| Adopted standalone control server | No launcher-owned output pipe | Existing adoption behavior | No claimed drainer coverage |
| Remote/container executors | Different output ownership | Existing executor behavior | Outside this launcher correction |

## Tests

| Acceptance | Planned evidence in `launcher_output_test.go` |
| --- | --- |
| AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.1 | `TestPipeOutputOversizedRecordContinuesDraining`, `TestPipeOutputOversizedRecordDoesNotBlockChild` |
| AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.2 | `TestPipeOutputRecordBoundaries`, `TestPipeOutputOversizedWithoutNewline` |
| AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.3 | `TestPipeOutputDiagnosticsAreBounded`, `TestPipeOutputReadFailure` |
| AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.4 | `TestPipeOutputEOFAndClosure` plus existing intentional/unexpected-exit and survival tests |
| AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.1 through .4 | Existing `launcher_json_log_test.go`, `launcher_relay_level_test.go`, and `launcher_loglevel_test.go` |

Boundary cases include limit minus one, exact limit, limit plus one, multi-megabyte records, CRLF, blank lines, and final records without LF.
The subprocess regression supplies integration evidence at the affected transport boundary.
It proves real child writes and exit progress without a browser or the live installation.
No rendered UI contract changes.

## Work orders

- [x] [Task 01: Preserve continuous child log draining](task-01-continuous-draining.md)

## Verification results

- Red phase: the targeted `TestPipeOutput` command failed as expected before the correction. It reproduced blocked stdout/stderr writers, loss at the scanner limit, missing bounded diagnostics, partial-record exposure on read failure, and a blocked helper child.
- Post-fix targeted `TestPipeOutput` regressions: passed.
- Complete launcher package tests: passed.
- Launcher package race tests: passed.
- Lifecycle stale-execution cleanup tests: passed (`0.017s`).
- Backend binary build with the `fts5` tag: passed.
- Specification catalog validation: passed (355 decisions and 1404 specifications).
- Specification lint: passed.
- `git diff --check`: passed.

The investigation's diagnostic overlay was removed before implementation.
The backend build artifact was written to `/tmp` and removed after verification.
The pre-existing saved investigation evidence remains intact.

## Live recovery guidance

The latest observed health endpoints respond. This investigation performs no recovery action.
If the outage recurs, preserve focused logs, runtime inventory, and both process stacks before recovery.
Use the installation's supervised restart procedure during a coordinated interruption window.
A shared agentctl restart can interrupt every instance it manages.
The historical report counted 57 active runtimes. The current count requires a fresh inventory.
Preserve worktrees, session resume identifiers, and uncertain execution tracking.
After restart, verify control health and reconcile runtime state before retrying the affected sessions.
Do not clear executions solely to bypass an already-running error.
An unfixed binary can encounter the same oversized-record defect again.

## Risks

- Oversized records lose their content by design. A bounded warning records the loss.
- Existing `Cmd.Wait` descriptor closure can race final log-tail reads. This package does not promise stronger tail retention.
- Stream diagnostics must not disclose record fragments or arbitrary error strings.
- Source and retained evidence support the historical hypothesis, but the exact triggering record remains unavailable.
- Implementation must re-read the base and linked completed severity tests before changing their private call signature.
