# ADR-2026-09-14-explicit-workspace-repository-placement: Explicit workspace repository placement

**Status:** accepted
**Date:** 2026-09-14
**Area:** backend, frontend, protocol

## Context

Issue [#3403](https://github.com/kdlbs/kandev/issues/3403) identifies a gap between the agent CWD and a promoted task workspace.
The legacy add-branch tool creates siblings during an active turn. A returned path cannot change the provider's sandbox.
The user accepted a creation-time parent-root option and three explicit placements in the existing Add dialog.
They also accepted visible tradeoffs and desktop/mobile previews.

## Decision

Single-repository Worktree startup retains its current default. Users can select parent-root startup in Advanced before creating a task.
The idle repository batch flow permits explicit nesting beneath `./kandev/` or directly beneath the current root, as well as root expansion.
Nesting preserves CWD and processes. It requires owned paths, isolated Git exclusions, and visible instruction-inheritance consequences.
Root expansion requires idle sessions and explicit continuation if native resume cannot retain the conversation.
Existing worktrees never move as a side effect of placement selection. Runtime paths and physical inventory remain durable authorities.

The new explicit batch flow narrows the blanket no-nesting statement in the [legacy add-branch ADR](2026-07-27-legacy-add-branch-live-rescan.md).
That ADR still applies in full to the legacy tool. Its caller is never restarted by attachment.
The [cross-root mutation boundary](2026-07-23-workspace-source-root-move-boundary.md) remains unchanged: this design creates entries and changes agent roots, not cross-root file moves.
The [placement design](../specs/tasks/system-design/workspace-repository-placement.md) defines implementation and the dependency on PR #3598.

## Consequences

Users can preserve an existing native conversation or choose sibling isolation with an explicit transition.
Nested worktrees require lifecycle, tracker, cleanup, exclusion, and path-reuse support. They cannot be implemented as a dialog-only change.
Provider instruction discovery and private state remain provider-owned. The UI cannot promise identical behavior across agents.
Initial-layout, nested-materialization, preview, and available UI behavior are implemented. Explicit root expansion and live native-harness validation remain blocked or unverified in the [plan](../plans/workspace-repository-placement/plan.md).

## Alternatives Considered

- Universal parent-root startup changes discovery for all single-repository users. The accepted option preserves the default.
- Automatic native-session replacement conflicts with explicit recovery and can discard private state.
- Mandatory nested placement denies users sibling isolation. Mandatory expansion interrupts processes and can lose native continuity.
- Sibling symlinks leave canonical destinations outside the original root and do not establish sandbox access.
- Broad shared ignore patterns hide unrelated files in other worktrees. Conditional worktree-scoped exclusion limits that effect.
