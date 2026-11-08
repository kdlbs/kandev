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
