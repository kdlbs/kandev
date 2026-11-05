---
status: draft
system: platform
created: 2026-11-01
owners:
  - kandev
---

# ACP Debug Log Retention Requirements

## Overview

When ACP debug logging is on, maintainers use the per-session frame logs to
diagnose agent sessions. The platform owns this contract because agentctl writes
the logs for every agent and executor. Retention must not remove the evidence
for a session that is still writing in the same agentctl process, and agent
subprocesses must not consume the log directory with inherited debug settings.

## Terminology

An **open writer** is a currently registered writer for an active log segment in
one `acpLogManager` in the agentctl process that runs retention. Rotated segments
and writers owned by another agentctl process are not open writers for this
requirement.

## Requirements

### REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-001: Keep logs for sessions in use

**Intent:** A debug log survives for as long as its session is writing to it.

**User story:** As a maintainer, I want a live session's frames retained, so that
I can diagnose it after it misbehaves.

#### Acceptance criteria

- **AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-001.1:** When the log directory exceeds
  its file-count limit, the system shall delete the oldest eligible files that
  no open writer in the same agentctl process holds and shall retain every
  active segment held by one of those writers.
- **AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-001.2:** When the files held by open
  writers alone exceed the limit, the system shall keep those active segments
  and delete other eligible files until the limit is met or no candidates remain.

### REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-002: Keep unrelated output out of the log directory

**Intent:** Work an agent does in its own workspace does not displace Kandev's
diagnostic logs.

**User story:** As a maintainer running Kandev in debug mode, I want agents' own
tools isolated from Kandev's debug settings, so that their output cannot displace
session logs.

#### Acceptance criteria

- **AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-002.1:** Agent subprocesses shall not
  inherit Kandev's ACP debug logging settings through any ACP entry point. The
  owning agentctl shall keep logging normally.
- **AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-002.2:** ACP adapter tests shall not write
  to the host's real ACP log directory, even when debug logging is enabled in the
  environment that runs them.

## Out of scope

- Changing the default file-count, age, or per-file size limits.
- Retaining rotated segments of a live session beyond the limits.
- Coordinating retention across multiple agentctl processes that share a log directory.
- Retaining logs for a session with no open writer beyond those limits.
- Changing whether debug logging is on in each profile.
