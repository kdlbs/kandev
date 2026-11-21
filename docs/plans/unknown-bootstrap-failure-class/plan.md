---
status: done
created: 2026-11-16
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-UNKNOWN-BOOTSTRAP-CLASS-001
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
---

# Unknown bootstrap failure class

## Overview

Asynchronous start failures outside the allowlisted cause codes were persisted
as `cause=unknown` with the generic detail. The session record never said why
the agent did not start, and the real error survived only in backend logs.

## Technical approach

Classify every unknown bootstrap failure with one bounded classifier and record
the class as the cause. An agentctl start rejection also records its status and
sanitized error field. Other wrapped errors keep the generic detail.

## Waves

Wave 1:

- [x] [Task 01: Record a bounded class for unknown bootstrap failures](task-01-unknown-bootstrap-class.md)
