---
created: 2026-11-19
status: done
requirements:
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-002
system_design:
  - ../../specs/agents/system-design/utility-agent-profiles.md
legacy_specs: []
---

# Implementation plan: Bounded utility requests and settled calls

## Overview

"Enhance prompt" sends the whole session diff to the utility agent. On a task
with a large change set the rendered prompt exceeds the provider's context
window and the call fails, although the action only needs to know which files
changed. Other utility actions have the same exposure through the diff, the
commit log, the task description, and the conversation history. Separately, a
utility call whose caller disconnects stays `pending` forever: the handler
records the failure through the request context, which is already cancelled.
This package implements `REQ-AGENTS-UTILITY-AGENT-PROFILES-002` as described in
"Request bounds and call settlement" in the system design.

## Scope

In scope: the Enhance prompt payload and template, client and server context
caps, newest-first conversation-history retention, stored Enhance prompt
refresh, atomic profile attribution at call creation, and terminal call state
updates on success, failure, and cancellation.

Out of scope: per-provider context budgets, a client-side cancel control,
other built-in templates, and the settings UI.

## Work orders

- [x] [Task 01: Bound utility requests and settle cancelled calls](task-01-bound-and-settle.md)

One work order: the payload, the server caps, and the settlement share one
request path and one verification boundary.

## Verification strategy

Backend unit tests for truncation, prompt refresh (shipped, dotted-placeholder,
unknown, customized, and profile-only-updated rows), and call settlement through
the production router; Vitest for the hook payload; a Quick Chat browser
check that the active-session Enhance action omits the diff from its request;
and a task-session browser test with 205 changed files that checks the request
lists 200 names plus the remaining count.

## Risks

- A truncated diff can omit context a commit or PR generator would have used.
  The cap is far above typical change sets and the marker states the cut.
- An install whose stored Enhance prompt was normalized by a system write
  keeps the old template; without a diff it renders that section empty, which
  is still within bounds.
- Conversation history keeps the newest 200,000 UTF-8 bytes. The omitted-byte
  marker precedes the retained tail so the model can read recent messages in
  their original order.
- Terminal writes run with a five-second deadline that does not inherit request
  cancellation. A database write can still fail for other reasons, and the
  handler logs that failure.

## References

- [Requirement](../../specs/agents/requirements/utility-agent-profiles.md)
- [System design](../../specs/agents/system-design/utility-agent-profiles.md)
