---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-003
created: 2026-10-05
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

## Child JSON forwarding

`Launcher.pipeOutput` uses `childLogRecord` in `internal/agent/runtime/agentctl/launcher/launcher.go`.
The parser recognizes JSON, console, and slog records.

Recognize one complete JSON object with string fields `level`, `timestamp`, `caller`, and `msg`.
Validate the timestamp as an agentctl timestamp and require a nonempty caller.
Use only the root `level` field and the existing recognized level set.
Reject trailing content, duplicate required fields, unknown levels, and mismatched field types.
Decode and ignore additional fields, including repeated fields, so nested logger context does not block severity recognition.
Do not recursively inspect JSON text embedded in a message.

Reconstruct the forwarded record from only the four validated envelope fields.
Drop additional child fields before writing the record to installation-wide parent logs, which may be included in diagnostic bundles.
Preserve the stream field on the parent entry.
The child pipe reader bounds the line size before parsing a record.
Do not persist raw provider stderr through a new path.
Error-class levels map to the parent's error method, not fatal or panic methods.
Unknown stdout remains debug, and unknown stderr remains warning.

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
Prove JSON WARN/ERROR survive an info logger threshold and produce no parent panic.

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
