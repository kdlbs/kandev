---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001
---

# Shared prompt MCP writes

## Boundaries and mapping

Extends [saved prompt reads](external-mcp-shared-prompts.md). The integration system
owns discovery, authorization, and results; `internal/prompts` owns persistence,
validation, and agent-write restrictions. All acceptance criteria of
`REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001` map to the contracts below.

## Tools and authority

The config and external catalogs register create/update tools with required string
`name` and `content`. Both are closed-world writes. Create is non-destructive and
non-idempotent; update replaces content and is marked destructive. Both return the
saved read shape, extended with `allow_agent_edits`. No permission input is exposed.
The backend checks `org.config.manage` from the trusted authentication context.
Existing transport authentication and automation dispatch restrictions remain.

The generic settings prompt adapter uses the same guarded service update as the
new tool and includes the permission in its read projection. It cannot change agent-edit permission. Existing generic rename support
remains available only for eligible custom prompts.

## Persistence and failures

Add `custom_prompts.allow_agent_edits INTEGER NOT NULL DEFAULT 0` through an
idempotent dialect-aware migration before built-in seeding. Preserve every legacy
content value, identity, and timestamp. Model and HTTP/MCP DTOs expose a boolean.
Operator PATCH accepts an optional permission; omission preserves it. Operator
updates also compare the observed modification timestamp and return HTTP 409 on
a concurrent change, so content-only saves cannot restore revoked permission. Built-in
prompts cannot acquire effective agent-write permission. MCP creation sets it true.

Agent updates use a conditional database write over the observed ID and name,
`builtin = 0`, `allow_agent_edits = 1`, and the observed modification timestamp.
A concurrent edit, rename, deletion, or revocation rejects the write; it never restores permission from a stale model.
Unique name constraints arbitrate concurrent creation. Service validation retains
512-byte names and 1 MiB content limits and existing trim behavior.

MCP maps invalid, duplicate, missing, and protected prompts to explicit errors;
concurrent writes return conflict rather than validation errors, and
unexpected storage errors are logged without returning internals or prompt bodies.
See [permission decision](../../../decisions/2026-09-29-shared-prompt-agent-permissions.md).

## Live delivery and Settings

Successful prompt mutations publish a content-free `prompts.changed` invalidation
through the event bus and existing WebSocket broadcaster. Browser handlers invalidate
the prompts cache, and the shared prompt loader refreshes it. A per-store request
generation prevents an in-flight response from clearing newer invalidation. Existing
editor form state stays local and survives remote refreshes. Reconnection invalidates
saved prompts as well, including the initial connection to close the gap between
the first read and live subscription. Failed mutations publish nothing. Failed
reads retain cached content without marking it loaded and retry up to three
attempts with bounded backoff. A later invalidation or consumer mount can retry
after exhaustion.

Settings retains its existing inline prompt edit surface. An accessible translated
“Allow agent edits” switch appears for custom prompts, with explanatory text that
edits affect every reference. It shares the existing floating Save changes action,
dirty tracking, and error handling. Only explicitly changed permission is sent
on save; otherwise the switch follows the latest remote permission while the
content draft stays local. Phone presentation keeps the single-column
page scroller and a labelled touch target at least 44px tall; there is no new overlay.

## Verification

SQLite and PostgreSQL repository tests cover migration replay and conditional
permission enforcement. Service/handler tests cover duplicate, missing, invalid,
protected, authorized, and storage-error paths plus generic-settings bypass
prevention. Catalog tests cover all MCP modes and input annotations. Browser tests
cover external MCP create/update, live refresh, draft preservation, and
desktop/phone permission save. Service tests verify future reference expansion. Loader tests exercise invalidation during a pending response.
