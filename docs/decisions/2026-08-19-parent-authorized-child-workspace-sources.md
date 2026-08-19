# Parent-authorized child workspace sources

**Status:** accepted
**Date:** 2026-08-19
**Area:** backend, protocol, security

## Decision

Task-bound MCP may attach workspace sources to the calling task or to its direct child, provided both tasks belong to the same non-empty workspace. The MCP server injects its bound caller task and session IDs; these provenance fields are not callable tool arguments. The backend verifies that the session belongs to the caller task and authorizes the relationship before invoking the existing owner-scoped attachment service.

Exact normalized retries are no-ops after the idle check. They return the authoritative projection without rematerializing sources, refreshing providers, or publishing update events.

## Consequences

This permits a coordinator parent to repair an idle child's missing repository or SDK without granting sibling, ancestor, descendant, cross-workspace, or generic same-owner mutation. `add_branch_to_task_kandev` remains current-task-only because its active-turn live-rescan contract is intentionally different.

## Alternatives rejected

Keeping the tool self-only leaves blocked children unable to recover. Authorizing in agentctl would duplicate task-tree policy and bypass the backend's authoritative service boundary. Broad same-workspace authority would allow unrelated tasks to mutate one another.
