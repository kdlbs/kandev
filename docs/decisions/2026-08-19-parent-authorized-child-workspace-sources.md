# ADR-2026-08-19-parent-authorized-child-workspace-sources: Parent-authorized child workspace sources

**Status:** accepted
**Date:** 2026-08-19
**Area:** backend, protocol, security

## Context

An idle child can be blocked by a missing repository or SDK and therefore cannot repair its own
workspace. The task-mode MCP transport used to reject every explicit target before the backend
could apply the task-tree authorization policy, while broad same-workspace access would let
unrelated tasks grant one another source access.

## Decision

Task-bound MCP may attach workspace sources to the calling task or to its direct child, provided both tasks belong to the same non-empty workspace. The MCP server injects its bound caller task and session IDs; these provenance fields are not callable tool arguments. The backend verifies that the session belongs to the caller task, then carries the direct-parent and workspace predicates into the source-batch transaction. A target reparented before that transaction commits is rejected before any source, repository, folder, session, or event mutation becomes durable.

Exact normalized retries are no-ops after the idle check. They return the authoritative projection without rematerializing sources, refreshing providers, or publishing update events.

## Consequences

This permits a coordinator parent to repair an idle child's missing repository or SDK without granting sibling, ancestor, descendant, cross-workspace, or generic same-owner mutation. `add_branch_to_task_kandev` remains current-task-only because its active-turn live-rescan contract is intentionally different.

## Alternatives Considered

Keeping the tool self-only leaves blocked children unable to recover. Authorizing in agentctl would duplicate task-tree policy and bypass the backend's authoritative service boundary. Broad same-workspace authority would allow unrelated tasks to mutate one another.
