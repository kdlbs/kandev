---
status: active
system: agents
created: 2026-11-16
updated: 2026-11-16
owners:
  - kandev
---
# Unknown Bootstrap Failure Class Requirements

## Overview

An asynchronous start failure outside the allowlisted cause codes was persisted
as `cause=unknown` with the generic detail, so a failed session never said why
the agent did not start and the real error survived only in backend logs. These
requirements narrow the safe fallback of
[session recovery failures](session-recovery-failures.md)
(AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.25 and 006.26) without widening the
cause-code contract.

## Requirements

### REQ-AGENTS-UNKNOWN-BOOTSTRAP-CLASS-001: Unknown bootstrap failures name their boundary

**Intent:** A failed start that has no allowlisted cause code still records which
boundary failed, using bounded values only.

As an operator, I want a failed session to say where the start failed, so that I
can act without searching backend logs.

#### Acceptance criteria

- **AC-AGENTS-UNKNOWN-BOOTSTRAP-CLASS-001.1:** A bootstrap failure outside the
  allowlisted cause codes shall record a bounded class instead of `unknown`:
  agentctl start rejection, unreachable agentctl, unavailable passthrough CLI,
  uncertain remote initial prompt, or unclassified. One classifier shall derive
  the class on every unknown path, including resume failures without a typed
  bootstrap failure.
- **AC-AGENTS-UNKNOWN-BOOTSTRAP-CLASS-001.2:** Only an agentctl start rejection
  may add detail: its HTTP status and agentctl's sanitized error field. Other
  wrapped errors shall keep the generic detail. The cause code, message,
  recovery actions, and log text shall be unchanged.

## Exclusions

- New allowlisted cause codes or frontend presentation changes.
- Classes for further unknown boundaries; they can be added later.

## System design

[Session startup failure explanations](../system-design/session-startup-failure-explanations.md#unknown-bootstrap-classes).
