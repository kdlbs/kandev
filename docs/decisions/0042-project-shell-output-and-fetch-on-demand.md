# ADR-0042: Project Shell Output and Fetch It on Demand

**Status:** accepted
**Date:** 2026-07-16
**Area:** backend, frontend, protocol

## Context

ADR-0036 stores a bounded normalized shell transcript on each tool message so live and reloaded chat agree. The same metadata is currently serialized into task boot state, REST and WebSocket message lists, and every live message update, even though output is collapsed or uninspected in most conversations. CSS-only collapse reduces vertical space but does not reduce serialization, transfer, parsing, or retained browser-memory costs.

## Decision

Persist the full bounded shell output in the existing message metadata, but separate its browser-facing summary from its body.

- A shared task-message projection replaces `stdout` and `stderr` with `has_output`, retained UTF-8 byte counts, `truncated`, and nullable `exit_code` before messages enter normal REST, boot, WebSocket-list, or WebSocket-notification payloads. Projection handles both the typed normalized payload used during live updates and the generic map produced by database JSON decoding, and never mutates stored metadata.
- A session-scoped HTTP endpoint, `GET /api/v1/task-sessions/:session_id/messages/:message_id/shell-output`, returns the latest full snapshot plus tool status and message `updated_at`. It returns `404` for an absent, cross-session, or non-shell message.
- The frontend fetches that endpoint only while the output disclosure is open. Completed commands require one snapshot request. Expanded running commands use one non-overlapping poll loop with a one-second base interval and a five-second maximum retry interval. A projected transition to terminal aborts the active poll and performs one final snapshot request before stopping, so the expanded transcript cannot remain on a partial running snapshot. Collapse or unmount stops the loop and aborts in-flight work without a final fetch.
- Persistence, provider normalization, output bounds, and the normal message store remain unchanged. No table or migration is added.

The observable contract is defined in [the ACP shell command output spec](../specs/ui/acp-shell-command-output.md).

## Consequences

- Large shell bodies are absent from the message paths used on nearly every task view, including repeated live updates, while command text, result status, truncation, and approximate size remain available for compact rendering.
- Expanding output incurs an additional database read and HTTP request. Running output incurs bounded polling only for disclosures the user chose to open.
- All browser-facing message paths must use the same projection helper; bypassing it would regress both payload size and the no-body contract.
- Full output remains part of the message row, so the endpoint reads and decodes the existing metadata blob. This avoids a migration but does not reduce database row size or the cost of the selected row read.
- Output snapshots are whole bounded values rather than deltas. The existing 256 KiB per-field limit caps response size and keeps polling implementation simple.

## Alternatives Considered

### Collapse only in React

Rejected because it fixes vertical space but still sends, parses, and retains every transcript in boot, list, and live-update payloads.

### Move output to a separate table or blob store

Rejected for this iteration because existing bounded metadata already provides durable ownership and lifecycle semantics. Separate storage would require a migration and coordinated deletion without first proving database-row reads are the bottleneck.

### Subscribe to output deltas over WebSocket

Rejected because it adds per-disclosure subscription lifecycle and replay complexity. Snapshot polling is sufficient for an explicitly opened, bounded diagnostic view and stops when terminal.

### Send a shortened output preview with every message

Rejected because previews still multiply payload cost across boot, pagination, backfill, and live updates, and the compact row does not require transcript content.

## References

- [ADR-0036: Normalize ACP shell output at the adapter boundary](0036-normalize-acp-shell-output-at-adapter-boundary.md)
- [ACP shell command output spec](../specs/ui/acp-shell-command-output.md)
- [Implementation plan](../plans/acp-shell-command-output/plan.md)
