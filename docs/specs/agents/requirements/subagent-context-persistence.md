---
status: draft
system: agents
created: 2026-08-12
updated: 2026-08-14
owners:
  - nova28
---
# Subagent context persistence Requirements

## Overview

When an agent fans out to subagents (Claude's `Task` tool, OpenCode's `task`, Cursor's `Task`, Auggie's `sub-agent-<type>:`), Kandev already recognises the fan-out and already receives the child's identity and usage on the completion frame. It renders them, counts them in memory, and then keeps no queryable record of them.

## Requirements

### REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001: Subagent context persistence

**Intent:** When an agent fans out to subagents (Claude's `Task` tool, OpenCode's `task`, Cursor's `Task`, Auggie's `sub-agent-<type>:`), Kandev already recognises the fan-out and already receives the child's identity and usage on the completion frame. It renders them, counts them in memory, and then keeps no queryable record of them.

#### Acceptance criteria

- **AC-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001.1:** **AC-7 given a detection rule** for `total_tokens` and `duration_ms`, which are plain `int64` and cannot express "not reported" the way `ToolUseCount *int` can.
- **AC-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001.2:** **Data** — most criteria: the outcome is a query against `task_session_subagents`, `task_session_messages`, or `kandev_meta`, or the value of an exported counter.
- **AC-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001.3:** **Process** — AC-1a, AC-20, AC-27, AC-33a: the outcome is a `WARN` log line, boot completing rather than aborting, or the enclosing message write surviving. A test asserts these with a log observer or by completing a boot; they are not queries.
- **AC-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001.4:** **Structural** — AC-14, AC-19, AC-23a, AC-23b, and *(revision 3)* AC-24d's evaluation-order clause, AC-24e, and AC-33b's second paragraph: these constrain *how* the implementation is built (a single atomic upsert with no read-then-write; `internal/db` error classification rather than string matching; one unbounded `INSERT … SELECT`; the dialect-aware JSON helpers; the `MAX` evaluated no later than the insert; a key parsed and bound as a timestamp rather than compared as a string; an end state observed from the live schema rather than inferred from a `fired` boolean) rather than what the resulting data looks like. Two implementations can produce identical rows and counters while only one satisfies them, which is the point: each rules out a mechanism that is correct today and fragile under change.
- **AC-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001.5:** `tool_use_count` is `ToolUseCount *int` on `streams.SubagentTaskPayload`. A nil pointer is "not reported" (NULL); a non-nil pointer to 0 is a reported zero (AC-8). The distinction is carried by the type.
- **AC-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001.6:** `total_tokens` and `duration_ms` are `TotalTokens int64` and `DurationMs int64` on the same struct — **plain values, not pointers**, both `json:"…,omitempty"`. They cannot express the distinction: an agent that reported 0 and an agent that reported nothing both arrive as `0`. THEREFORE, for these two fields specifically, a reported value of exactly `0` SHALL be stored as **NULL**.
- **AC-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001.7:** `attempted` = frames that reached the writer carrying a recognized payload.
- **AC-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001.8:** `skipped_no_identity` = of those, the ones rejected by AC-2 before any SQL ran.

## System design

The migrated technical source is split into [part 1](../system-design/subagent-context-persistence-01.md), [part 2](../system-design/subagent-context-persistence-02.md), [part 3](../system-design/subagent-context-persistence-03.md), [part 4](../system-design/subagent-context-persistence-04.md), [part 5](../system-design/subagent-context-persistence-05.md), [part 6](../system-design/subagent-context-persistence-06.md), [part 7](../system-design/subagent-context-persistence-07.md), [part 8](../system-design/subagent-context-persistence-08.md).
