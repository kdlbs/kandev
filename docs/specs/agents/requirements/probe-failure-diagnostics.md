---
status: active
system: agents
created: 2026-10-14
updated: 2026-10-16
owners:
  - kandev
---
# Probe Failure Diagnostics Requirements

## Overview

When a capability or profile probe fails, the child process usually knows why:
an `npx` resolution failure, a missing runtime, a panic, an authentication
error. Before this contract existed, the backend reported only
"peer disconnected before response", and the agent log the UI points at never
carried the child-side cause, so an operator could not diagnose the failure
without rerunning the probe by hand.

These requirements make probe failures diagnosable from backend logs while
keeping client responses sanitized and keeping the agentctl child's stdout
ingestible by the launcher that supervises it.

## Requirements

### REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001: Probe failures remain diagnosable in backend logs

**Intent:** A failed probe leaves child-side failure evidence in the backend
log, sanitized for the log boundary, while the client response keeps its
sanitized shape.

As an operator, I want the backend log to tell me why a probe failed, so that I
can fix the agent configuration without rerunning the probe by hand.

#### Acceptance criteria

- **AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.1:** When a capability or
  profile-context probe fails before completing its protocol exchange, the
  backend log entry for that failure shall carry the child process's trailing
  stderr output and the failure cause, so the child-side reason is available
  wherever the failure is observed. Sanitized diagnostics shall remain visible
  after the launcher relay projects the child log record.
- **AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.2:** Probe stderr and provider-supplied errors written to the
  backend log shall mask likely credentials, including opaque token-shaped
  values, and remove private URL details and local filesystem paths. Error
  codes and ordinary diagnostic text shall remain useful. Direct probe error
  responses shall use the same redaction; profile responses keep their generic
  message and code.
- **AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.3:** A profile-context probe
  failure response shall keep its sanitized client message and failure code,
  while the backend log entry additionally retains the pre-sanitization
  managed-runtime failure classification.
- **AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.4:** Captured probe stderr shall be
  bounded to a trailing window, so a chatty or looping child process cannot
  grow the captured output or the resulting log entry without bound. A line
  whose beginning was discarded shall not be logged, because truncation can
  separate a credential from the marker that identifies it.

### REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002: Agentctl child log format stays launcher-owned

**Intent:** The agentctl child's stdout feeds the supervising launcher's relay,
so the child's log format is the launcher's contract. An ambient environment
value must not be able to change that format and break record ingestion.

#### Acceptance criteria

- **AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002.1:** When the backend launches a
  managed agentctl child, the launcher shall pin the child's log format to the
  format its stdout relay parses, overriding any inherited value of that format
  variable, so the operator's ambient environment cannot break lifecycle
  record ingestion.
- **AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002.2:** The managed child format
  shall preserve the relay's JSON envelope allowlist, so unrelated structured
  fields such as session identifiers, workspace paths and command arguments do
  not reach backend logs.

## Exclusions

- Client-visible probe error copy and provider-gateway status surfacing. The
  sanitized upstream status contract is owned by
  [OpenAI-compatible providers](openai-compatible-providers.md)
  (AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003.3), and probe coverage of
  operator-registered commands by
  [Operator-Registered Agents](custom-acp-agents.md)
  (AC-AGENTS-CUSTOM-ACP-001.7).
- The relay's parsing of agentctl log records, which landed separately
  upstream.
- Persistent-session stderr streaming to the task UI; this contract covers
  one-shot probe subprocesses.
