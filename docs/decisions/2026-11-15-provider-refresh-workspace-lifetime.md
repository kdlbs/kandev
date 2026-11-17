# ADR-2026-11-15-provider-refresh-workspace-lifetime: Preserve workspace services during provider refresh

**Status:** accepted
**Date:** 2026-11-15
**Area:** backend, protocol

## Context

A human task MCP save must reach an existing agent conversation without destroying work.
The current agentctl process manager owns both the provider process and workspace services.
Its ordinary Stop closes terminals, VS Code, and workspace processes. Instance teardown also closes preview servers.
Retained helpers bind their built-in MCP profile once, so restarting only the child with the old helper profile is insufficient.

The initial task MCP design proposed replacing the helper at an idle boundary.
A follow-up proposed waiting until the helper had no terminal or preview resources.
The user challenged this coupling and requested the revised provider-only design.
This lifetime boundary must constrain later runtime maintenance callers, not only this feature's plan.

## Decision

Provider refresh keeps the workspace runtime alive and replaces only the provider generation.
Independent terminals, previews, workspace processes, editor services, and their connections remain owned by the retained runtime.
Their presence and activity do not block refresh. Provider-owned work must settle first.
Existing explicit full-stop and instance teardown operations retain their complete cleanup behavior.
This decision does not migrate runtime resources between helpers or redefine task deletion semantics.

Keep task, session, workspace, helper, execution, and native conversation identities.
Give each replacement provider an immutable MCP binding scope and process generation.
Retire the old scope before granting the successor authority. Preserve the backend control channel and workspace environment.
Keep the same conversation journal stream, incarnation, and SQL harness generation.
A provider package refresh does not create a new conversation or delivery owner.
The scope records the journal generation at acceptance as provenance.
An independently authorized context reset can advance the journal owner within the same provider process.
It does not change the frozen MCP selection. Verify current delivery ownership separately from the immutable scope.
Fresh processes cannot inherit a predecessor's accepted scope through task or executor metadata.
Use a new provider process generation and binding ID for stale-tool and effect fencing.
Ordinary helper-loss recovery keeps its existing continuity rules.
Do not infer permission from a live helper, cached tool catalog, or historical execution ID alone.

Unsupported helpers and providers leave settings pending with a reason.
Automatic application cannot fall back to whole-instance teardown or a fresh conversation.

## Consequences

Opening a terminal cannot prevent MCP changes indefinitely. A provider restart failure does not close that terminal.
The process manager needs separate provider and instance lifecycle operations.
MCP dispatch, attachment evidence, and recovery must include provider scope identity.
The implementation needs durable provider-operation receipts, unchanged-delivery-owner checks, and resource-survival tests.
A helper crash remains a runtime failure. This decision cannot preserve processes after their actual host disappears.

## Alternatives Considered

- Delete and recreate the helper. This stops unrelated workspace services and violates preservation.
- Wait for an empty helper. This protects resources but makes an ordinary open terminal a permanent settings blocker.
- Restart the provider while keeping its old MCP binding. This preserves the stale catalog and cannot apply the selection.
- Mutate the live provider's binding. Old transports and cached tools can cross permission generations.
- Move all workspace services to a new daemon first. This adds migration and ownership work that provider-only operations do not require.

## References

- [Task MCP application design](../specs/agents/system-design/task-mcp-application.md).
- [Task-owned worktree lifetime](2026-08-08-task-owned-worktree-lifetime.md).
- [Durable harness boundaries](2026-09-10-durable-harness-session-boundaries.md).
- [Agent process-group termination proof](2026-11-02-agent-process-group-termination-proof.md).
