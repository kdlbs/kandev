# ADR-2026-09-25-managed-remote-agent-runtime: Managed remote-agent execution

**Status:** proposed
**Date:** 2026-09-25
**Area:** backend

## Context

Kandev executors currently create agentctl instances with workspace access.
Cursor Cloud owns its agent process and exposes a remote conversation API.
Pretending that this API is an agentctl instance would make workspace and lifecycle contracts false.
The shared runtime facade exists, but normal task execution still has direct lifecycle dependencies.

## Decision

Propose a managed execution path at the agent runtime boundary, with explicit capabilities and persistent provider bindings.
Keep the existing ExecutorBackend contract for environments that run agentctl.
Route task orchestration through a compatibility boundary that handles both execution kinds.
Do not manufacture agentctl clients, local paths, or process-liveness evidence for managed agents.

A Kandev session owns one remote conversation. A prompt turn owns one submission operation and its remote run, once identified.
Unknown submission outcomes block automatic resubmission. Provider status does not replace workflow completion authority.
Cloud callbacks use scoped execution grants, not broad user tokens.

This decision remains proposed with the draft design package. It does not claim an implemented or accepted repository-wide migration.

## Consequences

Existing executors retain their behavior. Managed runtimes can participate without providing terminal or filesystem access.
Orchestration, recovery, and UI capability handling require explicit integration work.
The first implementation supports Cursor Cloud only; no plugin extension API or generalized provider SDK is introduced.
Live account testing is required before rollout. Remote callback hosting remains an operator responsibility.

## Alternatives Considered

- **Ordinary ExecutorBackend:** Rejected because its agentctl and workspace guarantees cannot be satisfied through the documented API.
- **Fake local agentctl bridge:** Rejected because it conceals unavailable capabilities and creates a second process/protocol adapter to maintain.
- **Independent task integration:** Rejected because it duplicates messages, lifecycle, and workflow completion handling.
- **Cursor CLI on SSH or containers:** Already fits the existing model, but does not provide Cursor-hosted Cloud Agents execution.

## Related documents

- [Executor requirements](../specs/executors/requirements/cursor-cloud.md).
- [System design](../specs/executors/system-design/cursor-cloud.md).
- [Implementation plan](../plans/cursor-cloud/plan.md).
