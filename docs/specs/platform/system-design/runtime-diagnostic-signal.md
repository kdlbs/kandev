---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-003
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-004
created: 2026-10-05
updated: 2026-11-04
owners:
  - kandev
---

# Runtime diagnostic signal system design

## Purpose and boundaries

This design extends [expected severity](expected-runtime-log-severity.md) with the JSON format that agentctl emits.
It also removes three known sources of routine repetition.
It also defines continuous child log draining under a fixed memory bound.
It preserves process control, retention, runtime reclaim decisions, and MCP capability selection.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001 | Child JSON forwarding; Verification |
| REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002 | Routine diagnostic sites; Verification |
| REQ-PLATFORM-DIAGNOSTIC-SIGNAL-003 | Child pipe draining; Verification |
| REQ-PLATFORM-DIAGNOSTIC-SIGNAL-004 | Shutdown attribution; Verification |

## Child JSON forwarding

`Launcher.pipeOutput` uses `childLogRecord` in `internal/agent/runtime/agentctl/launcher/launcher.go`.
The parser recognizes JSON, console, and slog records.

Recognize one complete JSON object with string fields `level`, `timestamp`, `caller`, and `msg`.
Validate the timestamp as an agentctl timestamp and require a nonempty caller.
Use only the root `level` field and the existing recognized level set.
Reject trailing content, duplicate required fields, unknown levels, and mismatched field types.
Decode and ignore additional fields, including repeated fields, so nested logger context does not block severity recognition.
Do not recursively inspect JSON text embedded in a message.

Reconstruct the record from the validated envelope and the typed context allowlist below.
Drop all other child fields before installation-wide logging and diagnostic bundles.
Preserve the stream field on the parent entry.
The child pipe reader bounds the line size before parsing a record.
Do not persist raw provider stderr through a new path.
Error-class levels map to the parent's error method, not fatal or panic methods.
Unknown stdout remains debug, and unknown stderr remains warning.

### Safe context allowlist

The parent keeps its launcher component and stream fields.
Child context remains inside the reconstructed JSON message and cannot overwrite parent context.
Use fixed key dispatch and typed validation, without recursive object forwarding.

| Fields | Accepted values |
| --- | --- |
| `component` | `process-manager`, `workspace-tracker`, `instance-manager`, `control-server`, `api-server`, `process-runner`, `interactive-runner`, `shell`, `shell-manager`, `git-operator`, `status-tracker`, `workspace-preview`, `vscode-manager`, `wakeup-scheduler` |
| `task_id`, `session_id`, `instance_id`, `execution_id`, `submission_id`, `request_id` | Canonical lowercase UUID strings, exactly 36 bytes |
| `state` | `healthy`, `pressure`, `guarded`, `cancelling`, `interrupted`, `cancellation_failed`, `storage_error`, `delivery_failed` |
| `error_class`, `reason` | `permission_denied`, `not_found`, `context_canceled`, `deadline_exceeded`, `git_start_failed`, `git_command_failed`, `repository_context_unavailable`, `io_error`, `unknown`, `agent_running`, `managed_runtime_npm_resolution`, `managed_runtime_npm_policy`, `unsupported_profile_context`, `authentication_required`, `evidence_changed` |
| `operation` | `read_directory`, `read_file`, `stat`, `git_inspection`, `submodule_discovery`, `agent_configure`, `agent_probe` |
| `trigger` | `manual_refresh`, `user_select`, `turn_complete`, `startup_grace`, `poll` |
| `runtime` | `desktop`, `server` |
| `poll_mode` | `fast`, `slow`, `paused` |
| `exit_code` | JSON integer from -1 through 4294967295 |
| `retained_bytes`, `limit_bytes` | Nonnegative JSON integers that fit signed 64-bit storage |

Drop an optional field with an invalid type, unsupported value, excessive length, or duplicate occurrence.
When nested Zap loggers repeat `component`, accept the last value only if every occurrence belongs to the allowlist.
Otherwise omit the component and retain valid severity.
Required envelope duplicates retain their existing malformed-record fallback.
Never route an invalid optional field through that raw fallback.
The fixed field count and existing 64 KiB input limit bound reconstruction.
Do not forward `error`, `target`, `path`, repository names, URLs, headers, tokens, payloads, or provider stderr.

Filesystem and submodule producers add a safe `error_class` beside their existing local fields.
Normalize filesystem operation names to the closed forwarding vocabulary at that producer boundary.
Use typed cancellation, deadline, permission, missing-file, and process-start evidence before the `unknown` fallback.
Submodule failures add `operation=submodule_discovery` and an exit code only when a typed process result supplies it.
The existing raw error field stays outside the forwarding allowlist.
Existing common filesystem diagnostics own shared filesystem classification.

Configuration and probe producers use the additive values under
[failure attribution](runtime-failure-attribution.md#agentctl-failure-producers).
The probe classes reuse existing typed codes.
These additions do not admit arbitrary code strings or raw error text.
The [runtime stall work order](../../../plans/runtime-stall-diagnostics/task-02-classify-agentctl-failures.md)
defines producer and forwarding verification.

`refreshDeliveryPressure` emits info for transitions to `healthy` and retains warnings for every other transition.
It keeps its existing transition-only emission, state machine, limits, and cancellation decisions.

## Shutdown attribution

`Launcher.Stop` marks intentional stop and sends the graceful signal while retaining the parent-liveness writer.
It closes that writer on return after its existing exit or forced-cleanup wait.
Do not close the writer before signaling the child.
Signal delivery failure still closes owned descriptors and retains its existing returned error.

In `cmd/agentctl`, `waitForShutdown` records planned signal handling at info.
Unexpected monitor completion records `parent liveness pipe closed, initiating shutdown` at warning.
That message describes the observed event without inferring OS process death.
The monitor remains armed until child exit or actual parent loss.
EOF still begins the same 30-second child cleanup.
No new liveness protocol, acknowledgement, grace period, or environment variable is required.
Unix process groups and Windows containment retain their current platform behavior.
Survival-enabled launches still omit the pipe, and `Stop` remains a no-op for that launch.

## Child pipe draining

The launcher owns one stdout reader and one stderr reader for each managed agentctl process.
Agentctl normally writes synchronously to stdout through `logger.NewLogger`.
An undrained stdout pipe can therefore block HTTP middleware and lifecycle operations across multiple instances.
ACP remains on its existing socket transport.

The current default `bufio.Scanner` stops on an oversized token.
`pipeOutput` does not inspect its error, so its goroutine silently exits.
The correction changes the private `pipeOutput` input to `io.Reader` and uses fixed-size buffered reads.
It does not increase the scanner ceiling or create a new logging subsystem.

The proposed reader uses a 4 KiB `bufio.Reader` and retains at most 64 KiB of record content per stream.
The content limit excludes LF and includes a possible CR.
Complete records at the limit remain valid.
The reader removes a terminal CR before forwarding, matching the existing `ScanLines` behavior.
Normal blank lines and final records without LF retain their existing behavior.

On `bufio.ErrBufferFull`, the reader continues consuming fragments of the same record.
After the content limit, it discards all remaining fragments through LF or EOF.
It discards the entire oversized record instead of parsing or forwarding a truncated prefix.
The next record starts with empty state.
One fixed accumulation buffer and one fixed reader buffer bound memory, even without a newline.
Parsing allocations remain bounded by the retained record limit.

Each stream emits at most one oversized-record warning during its reader lifetime.
The warning contains only the stream and configured byte limit.
An unexpected read failure emits one fixed diagnostic with a closed error class, then ends the reader.
Examples of classes are `closed`, `unexpected_eof`, and `read_failure`.
The diagnostic contains no raw record, error string, path, or provider payload.
EOF is normal. Pipe closure during process teardown does not produce a failure warning.
Diagnostics use the existing parent logger and its bounded backend sink queues.
No new queue, asynchronous logger, timer, or per-record goroutine is necessary.
This correction does not promise progress through an arbitrary injected logger whose sink itself blocks.

The existing `StdoutPipe`, `StderrPipe`, `monitorExit`, and `Stop` retain descriptor and process ownership.
`Cmd.Wait` closes the command pipes after process exit, which releases blocked reads.
Stop does not wait for a newline or add an unbounded reader join.
A canceled launch caller must not abandon readers while the supervised child remains alive.
The existing survival configuration remains unchanged.
This correction does not redesign post-exit log-tail retention or standalone adoption.

When combined with in-process runtime replacement, complete bounded stderr records
also enter the existing sanitized 8 KiB exit-diagnostic tail.
An oversized stderr record contributes only the fixed discard marker, once per stream;
no truncated record prefix enters either diagnostic surface.
The launcher's owned-reader wait group remains attached to both stream goroutines.
Exit publication waits for those readers after `Cmd.Wait` releases their descriptors,
preserving the replacement coordinator's exit-evidence boundary.

## Routine diagnostic sites

`requiredstores.Health.logTransition` currently emits info on every probe.
Retain the last emitted state and sorted affected store identities under the owner's mutex.
Emit the first result and subsequent changes only.
The periodic failure warning remains independent and appears once per failed sweep.
Do not change `RecordProbe`, last-check timestamps, readiness, or maintenance admission.

`Service.reclaimIdleSession` emits a detailed debug refusal on every normal call.
Remove the routine per-session refusal entry.
Keep `classifyIdleReclaim`, all liveness guards, successful reclaim logs, and failed-probe warnings unchanged.
Avoid a per-session suppression registry or a new timer.
Tests must prove identical outcomes for every existing refusal reason.

`filterMcpServersWithDecisions` currently warns before it knows whether another transport survives.
Retain its ordered first-surviving-name selection and every decision reason.
Classify unsupported alternatives after the surviving list is known.
Use debug only when a supported entry with the same name survives.
If all entries for that name fail capability filtering, retain the warning.
Do not alter input capability checks or duplicate-name resolution.

## Verification

Parser and observer tests cover JSON, console, slog, malformed input, and both streams.
Include payload text containing fake severity words, duplicate required envelope fields, duplicate additional fields, and sensitive additional fields excluded from parent output.
Include allowlisted context, malformed optional values, duplicate components, invalid UUIDs, and numeric overflow.
Observer tests assert privacy against both the message and parent context.
Prove JSON WARN/ERROR survive an info logger threshold and produce no parent panic.

Real pipe and helper-process tests cover planned Stop, unexpected EOF, abrupt parent loss, signal failure, and bounded forced cleanup.
Keep nil-monitor, ownership-shutdown, exit callback, and survival controls.
Journal-backed tests cover initial healthy, pressure, guarded, and recovered healthy transitions.
Filesystem and submodule tests assert safe fields at their real producer boundaries.
Launcher-owned integration tests forward records from the real Zap JSON encoder with those field shapes.
Keep producer and forwarding assertions separate without importing backend runtime code into the standalone agentctl package.

Health tests cover repeated healthy results, repeated failures, recovery, and changed failing store sets.
Reclaim tests cover unchanged refusal decisions and genuine probe errors.
MCP tests cover SSE/HTTP alternatives, total refusal, and duplicate names.
Measure log entry and byte counts on synthetic repeated calls before and after the change.
No production threshold change is part of that measurement.

Pipe tests cover both streams, limit boundaries, repeated oversized records, missing LF, CRLF, blank lines, and final EOF records.
An `os.Pipe` test proves writer progress and normal-record forwarding after a 128 KiB record.
A helper subprocess test proves that oversized child output cannot prevent child exit.
Injected read failures prove bounded, content-free diagnostics.
Descriptor-closure and existing exit-callback tests cover teardown without additional production goroutines.
The [fix work order](../../../plans/agentctl-log-drain/task-01-continuous-draining.md) defines exact commands and test names.
