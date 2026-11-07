# ADR-2026-11-05-copilot-native-compaction-evidence: Count Copilot compactions from durable native evidence

**Status:** accepted
**Date:** 2026-11-05
**Area:** backend, protocol

## Context

Copilot's ordinary ACP usage stream interleaves parent and child contexts without attribution.
The merged correction disables usage-drop inference for Copilot.
A separate opt-in stream supplies explicit outcomes and child identity, but provider omission and delivery loss remain possible.

The captured notification method is `github.com/copilot/sessionEvent`.
The pinned ACP SDK forwards only underscore-prefixed extension notifications.
A completed timeline row does not currently mutate the session count.

## Decision

Keep Copilot usage-drop inference disabled, including for attributed root usage.
Count only validated root completions with a stable attempt identity, explicit success, and applied outcome.
Absent optional outcome or identity fields fail closed until version-specific evidence supports a separately specified alternative.

Add explicit opt-in support for exact vendor notification names at the SDK boundary.
Keep default unknown-notification handling, request routing, and outbound extension validation unchanged.
Kandev opts into the Copilot method only for Copilot sessions.
Vendor observations share the existing adapter FIFO and reset/load ownership barriers.

Persist attempt identities in a session-owned ledger keyed by provider, native conversation, and compaction attempt.
Commit the receipt, lifetime increment, and durable projection cursor in one transaction.
Projection cannot depend on an in-memory lifecycle callback or a process-local seen set.
Retain receipts until the owning Kandev session is deleted.

Use attributed root usage for the gauge while preserving accounting separately.
Report observed totals and known partial/unavailable coverage without a complete-history claim.
Preserve historical inferred totals rather than relabeling them as confirmed events.

## Consequences

A captured applied parent completion can increase the count without subagent false positives.
Durable receipts prevent double increments across reconnects, streams, and process restarts.
The ledger outlives transport receipts and timeline retention.
Provider-dropped or omitted evidence can still cause undercounts.
Unqualified older runtime and retained-agentctl combinations retain the safe exclusion and report native counting unavailable.

The first work order must qualify the managed Copilot pin and publish narrow SDK support before enabling the new path.
No runtime flag or silent downgrade to usage inference is introduced.

## Alternatives considered

- Re-enable usage-drop inference for parent readings: parent resets and missing samples remain ambiguous and can double count explicit events.
- Infer identity from token size or tool lifetimes: concurrent and delayed notifications invalidate those heuristics.
- Count completed timeline rows: replay, retention, and failed metadata writes do not provide atomic counter evidence.
- Keep a process-local attempt set: restart and cross-stream replay lose deduplication.
- Forward every unknown JSON-RPC notification: this broadens unrelated protocol boundaries without a requirement.
- Recover native history from private files: this adds a second persistence authority and is outside the provider stream contract.

## References

- [Requirements](../specs/agents/requirements/copilot-native-compaction.md).
- [System design](../specs/agents/system-design/copilot-native-compaction.md).
- [ACP dialect boundary](0044-acp-agent-compatibility-dialects.md).
- [Durable harness ownership](2026-09-10-durable-harness-session-boundaries.md).
- [Investigation](https://github.com/kdlbs/kandev/issues/5089#issuecomment-6672451872).
