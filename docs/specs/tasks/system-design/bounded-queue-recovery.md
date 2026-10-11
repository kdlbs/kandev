---
status: current
system: tasks
requirements:
  - REQ-TASKS-QUEUE-RECOVERY-001
---

# Bounded queue recovery design

## Mapping and ownership
All ACs map to observation, authority, pagination and transport below. [ADR](../../../decisions/2026-10-11-core-bounded-queue-recovery.md) fixes core/plugin/deployment ownership.

## Observation
Add a dedicated pure adapter in internal/orchestrator/messagequeue. Existing Repository.Snapshot increments status generations and memory ResolveSessionIdentity binds/clears state; neither is recovery. Memory uses mutex-protected deep copies. SQL uses consistent read-only transactions with size preflight and capped scans, no lock upsert or generation writes. Bind exact task/session/incarnation. Child inventory requires bounded consistent session discovery and observation. Removed dispatch sources remain outside FIFO coverage.

## Authority
Derive mcpscope.Principal server-side and validate live task/session/workspace. Terminal target uses exact own-task predecessor and existing terminal predicate. Child requires exact ParentID/workspace/session even when caller sees other tasks. Authorize before reads and fence authority/incarnation across observation. Deny missing/foreign identically, with no fallback.

## Pagination
Versioned canonical identity/order/body/attachment/settings/reservation evidence defines hash; QueuedMessage JSON alone omits private fields. Cursor binds source/digest/offset, never grants authority. Validate length/version/schema/index/UTF-8 boundary; reauthorize before comparing complete hash. Optional expected_snapshot_sha256 pins inventory-to-first-page. Enforce source/fragments/raw-body/serialized-response bounds. No persisted cache or generation mutation.

## Transport and failures
Register exact terminal/child readers with read-only/idempotent annotations. #3377 owns exact current census; coordinate reuse without disposal/wake imports. Internal mcp.* actions remain raw-WS denied. Only existing task surfaces/managed policies expose readers. Safe typed errors distinguish invalid page, changed snapshot, limits and unavailable storage; logs contain bounded counts/identities, not content.

## Delivery
Docker hosting of server/local agents is separate from task Docker executor. Independent Review/QA cover committed source. Canonical merge/release plus isolated unmodified-artifact acceptance precede deployment-owner overlay removal. Retirement preserves independent archive/claim/intake gates.
