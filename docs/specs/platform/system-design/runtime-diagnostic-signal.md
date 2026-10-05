---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002
created: 2026-10-05
owners:
  - kandev
---

# Runtime diagnostic signal system design

## Purpose and boundaries

This design extends [expected severity](expected-runtime-log-severity.md) with the JSON format that agentctl emits.
It also removes three known sources of routine repetition.
It preserves process control, retention, runtime reclaim decisions, and MCP capability selection.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001 | Child JSON forwarding; Verification |
| REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002 | Routine diagnostic sites; Verification |

## Child JSON forwarding

`Launcher.pipeOutput` uses `childLogLevel` in `internal/agent/runtime/agentctl/launcher/launcher.go`.
The parser recognizes console and slog records, but JSON currently reaches the stdout debug fallback.

Recognize one complete JSON object with string fields `level`, `timestamp`, `caller`, and `msg`.
Validate the timestamp as an agentctl timestamp and require a nonempty caller.
Use only the root `level` field and the existing recognized level set.
Reject trailing content, duplicate required fields, unknown levels, and mismatched field types.
Decode and ignore additional fields, including repeated fields, so nested logger context does not block severity recognition.
Do not recursively inspect JSON text embedded in a message.

Reconstruct the forwarded record from only the four validated envelope fields.
Drop additional child fields before writing the record to installation-wide parent logs, which may be included in diagnostic bundles.
Preserve the stream field on the parent entry.
The existing scanner bounds the line size.
Do not persist raw provider stderr through a new path.
Error-class levels map to the parent's error method, not fatal or panic methods.
Unknown stdout remains debug, and unknown stderr remains warning.

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
