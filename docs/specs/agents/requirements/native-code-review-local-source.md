---
status: draft
system: agents
created: 2026-10-29
owners:
  - jcfs
---

# Native Code Review Local Source Requirements

## Overview

A native [code review](native-code-review.md) reads a task's changed files from
the session workspace. This requirement covers reading that source directly from
the task's own checkout when the agentctl read is unavailable, without adding a
new source authority.

## Requirements

### REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001: Task-bound local review source

**Intent:** A review for a task whose executor is local or worktree can still
collect the task's changes when agentctl is unavailable, without gaining access
to any path the task does not already own.

#### Acceptance criteria

- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.1:** When the agentctl read of a task's changed files fails and the task's executor is `local`, `worktree`, or `local_pc`, a review pass SHALL read the task's own persisted checkout to collect its changes instead of failing with `workspace_unavailable`. The read SHALL pass the existing `session.exec` authorization check before agentctl or local checkout access. An authorization denial SHALL stop the local fallback. A remote executor SHALL still require agentctl.
- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.2:** The local read SHALL stay within the existing per-file, per-output, and total diff byte limits. The per-file limit SHALL include the index blob for staged-only changes. The reader SHALL reject a worktree path that is not an absolute local path or that carries a `..` segment. It SHALL never accept a caller-supplied filesystem path.
- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.3:** The local read SHALL collect working-tree, index-only, untracked, and committed changes as distinct sources. It SHALL keep multi-repository files scoped to their own repository. For an attachment with an explicit `comparison_target`, the committed read SHALL use that target's validated `ComparisonRef`. If the exact ref is unavailable, the read SHALL report the comparison as unavailable and SHALL NOT use `origin/<branch>` or the stored base instead. Without an explicit target, the existing live-branch and stored-base behavior SHALL apply.
- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.4:** The agentctl and local readers SHALL use the same bounded untracked-diff formatter. Identical untracked content and paths SHALL produce the same normalized diff text and hash from either reader.

### REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002: Explicit and saved review sources

**Intent:** A writer that changes a backend-local Git worktree outside the
Kandev task workspace, such as a gate-slot checkout, can still get independent
review of that checkout, with results in the same task review readback.

#### Acceptance criteria

- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.1:** A review request with an explicit source SHALL name an existing absolute local Git worktree (`worktree_path`), a comparison base (`base_ref`), and optionally a committed head (`head_ref`). Without a head, the working tree, index-only, and untracked changes SHALL be reviewed against the base. With a head, the committed range from the merge base to the head SHALL be reviewed. UNC paths, `..` segments, missing or non-Git paths, and refs that do not resolve to commits SHALL fail before a run is created. A request carrying source fields SHALL require write access to the target task.
- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.2:** A writer SHALL be able to save or clear one review source per task in `task.metadata.review_source`. The target task SHALL be authorized for write before the path is examined. Clearing SHALL NOT be combined with source fields and SHALL leave other metadata unchanged.
- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.3:** A manual or workflow-step review without explicit source fields SHALL use the saved source only when workspace collection returns no changes or is unavailable. A source saved on the task SHALL take precedence over one carried by the writer session's metadata.
- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.4:** Runs and findings from an explicit or saved source SHALL stay in the existing task review service and readback. Changing or clearing a source SHALL NOT erase earlier results.
- **AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.5:** A `worktree_path` supplied by an agent through the MCP review tools, for an explicit run or a saved source, SHALL resolve, after symlinks and (on Windows) case, to the target task's registered local repository checkout or a workspace or worktree path of one of its sessions on a `local`, `worktree`, or `local_pc` executor, or to a directory inside one. Any other path, including a sibling that only shares a prefix, SHALL be rejected as forbidden before Git validation, and no run or saved source SHALL be created. A path sent by a person through the authenticated WebSocket API, without an MCP caller principal, SHALL keep the 002.1 and 002.2 rules and MAY name any backend-local Git worktree.

AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.2 continues to govern the
task-bound fallback reader. A writer-supplied path is accepted only through
REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002 and its task-write
authorization; an agent-supplied path is further limited to the task's own
local checkouts by AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.5. The design is
[Native Code Review Local Sources](../system-design/native-code-review-local-source.md).
