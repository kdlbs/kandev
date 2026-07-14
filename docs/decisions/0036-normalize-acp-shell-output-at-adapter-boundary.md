# ADR-0036: Normalize ACP Shell Output at the Adapter Boundary

**Status:** accepted
**Date:** 2026-07-14
**Area:** backend, frontend, protocol

## Context

ACP agents expose shell output and exit status through incompatible shapes. Codex uses terminal metadata and a structured final result, Claude gates terminal metadata behind a client capability, OpenCode sends cumulative content and a nested exit value, and Auggie embeds the result in XML-like text. Some agents also report ACP status `completed` for a nonzero process exit.

Kandev currently normalizes shell tool calls but defaults plain output to exit code `0`, ignores live terminal updates, and loses several provider-native exit fields. Passing raw ACP frames through to chat would make persistence and React responsible for provider protocol details.

## Decision

The ACP adapter owns shell-output normalization.

- Provider-specific fields are converted into the existing provider-neutral `shell_exec` payload before stream events leave the adapter.
- Exit code is nullable. Missing or malformed exit data remains unknown and is never converted to `0`.
- A known nonzero exit makes the normalized tool result a failure even when the provider reports ACP status `completed`.
- Live output is accumulated by tool-call ID. Delta updates append, cumulative updates replace, and a final aggregate replaces live text without duplication. A final update without output preserves the accumulated text.
- Output is stored as a combined terminal stream unless the provider explicitly supplies separate stdout and stderr values.
- Each output field is bounded before persistence. Tail retention preserves the most recent valid UTF-8 content and records truncation in the normalized payload.
- Kandev advertises the ACP `_meta.terminal_output` client capability so providers can return terminal output and exit metadata. It does not advertise or implement client-owned terminal RPC methods as part of this feature.
- The frontend renders only the normalized contract and does not inspect provider names or ACP metadata.

## Consequences

- Persisted task messages and live WebSocket events share one representation, so reloads render the same command output as the active session.
- Provider quirks are tested in the ACP package rather than duplicated across the orchestrator and frontend.
- Unknown exit status becomes an explicit neutral UI state. This changes the previous implicit-success behavior for plain output.
- The adapter keeps bounded per-tool accumulation state until the tool reaches a terminal state or the existing prompt cleanup runs.
- Output absent from upstream ACP frames cannot be recovered by Kandev.

## Alternatives Considered

### Parse provider payloads in React

Rejected because raw ACP shapes would leak into the persisted message contract and require frontend changes for every provider revision.

### Store raw frames and normalize only when rendering

Rejected because live and reloaded messages could diverge, storage would grow without a bound, and non-UI consumers would still lack a reliable exit status.

### Use ACP tool status as the command result

Rejected because OpenCode can report `completed` for a nonzero shell exit and because status does not encode the exact exit code.

### Treat a missing exit code as zero

Rejected because absence is not evidence of success and caused false success indicators in chat.

## References

- [ACP shell command output spec](../specs/ui/acp-shell-command-output.md)
- [Implementation plan](../plans/acp-shell-command-output/plan.md)
- [ADR-0034: Agent Client Protocol Codex ACP Bridge](0034-agentclientprotocol-codex-acp.md)
