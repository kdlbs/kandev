---
status: draft
system: agents
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002
---

# Native Code Review Local Sources

## Purpose and requirement mapping

Gate-slot writers modify a Git checkout outside the Kandev task workspace.
This design extends the existing review runner rather than introducing another
review pipeline or persistence table. The task-bound fallback reader of
`REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001` is described in
[abandoned native review admission](native-code-review-recovery.md).

| Acceptance criteria | Design section |
| --- | --- |
| `AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.1` | Explicit source |
| `AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.2` | Saved source |
| `AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.3` | Saved source |
| `AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.4` | Results |
| `AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-002.5` | Agent-supplied paths |

## Explicit source

`task.review.run` accepts `worktree_path`, `base_ref`, and optional `head_ref`.
The task-mode MCP entry is `run_code_review_kandev`. A request that carries any
source field requires task write access, and the explicit source takes
precedence over workspace changes. `review.ValidateSource` requires an absolute
local path without a UNC prefix or `..` segment, resolves symlinks, and checks
that the directory is inside a Git work tree. Refs resolve to commits before
any diff runs. A head uses the merge base for the committed range; an omitted
head compares the base with the working tree, adds index-only changes, and
includes untracked files separately. The backend runs Git directly through the
managed subprocess path with the existing diff byte limits, without requiring
session agentctl. Diffs disable external diff drivers and text conversion.

## Saved source

`set_review_source_kandev` dispatches `task.review.source.set`. The handler
authorizes the target task for write before it examines the path, validates the
source like an explicit run, and stores it in `task.metadata.review_source`
through the existing task metadata update. `clear=true` stores no source and
cannot be combined with source fields.

The runner reads the saved source only after workspace collection returns no
files or fails. Task metadata takes precedence over a `review_source` carried by
the writer session's metadata. At collection it checks current task ownership,
including an additional worktree of an attached Git repository. A browser
operator with workspace-management authority can select an external checkout
through the existing setter. It stores that operator, the canonical checkout,
Git common directory and refs in a server-owned record, atomically with the
existing task metadata selection. Collection
rechecks the operator's current authority and the selected checkout. Ordinary
metadata and session metadata cannot add this approval. A legacy external
source can stay saved and needs an explicit operator selection before reuse.

## Agent-supplied paths

The MCP review tools carry a server-derived caller principal or a trusted
external MCP transport marker. When `task.review.run` or
`task.review.source.set` carries either and a `worktree_path`, the handler
first lists the target task's admitted roots: the
`local_path` of each registered task repository, plus the workspace path and
worktree paths of each session whose executor is `local`, `worktree`, or
`local_pc`. A Docker, SSH, or other remote session's paths name another machine
and admit nothing. The handler reads the authorized stored executor type without
refreshing the plugin executor catalog. `review.SourceWithinRoots` resolves symlinks on both the
supplied path and each root, lower-cases on Windows, and accepts only a root or
a directory below one by path components, so a link that leaves a root or a
sibling such as `repo-evil` is rejected.

An in-root checkout must also use a Git common directory inside the admitted
roots or one belonging to the task's registered repositories. A linked gate
worktree can use the registered common directory from outside those roots.
The collector verifies the effective worktree root and Git object store.
Repositories that borrow another object store through alternates are rejected.
The collector uses read-only `git rev-parse --path-format=absolute --git-common-dir`
on the resolved candidate and registered checkouts, with repository-binding
variables such as `GIT_DIR`, `GIT_WORK_TREE`, and `GIT_COMMON_DIR` removed from
the environment, and compares the common directories after the same symlink and
Windows-case canonicalisation. A worktree of an unrelated repository, a
non-repository, or a failed probe is rejected. A rejection is a forbidden error
before review Git validation or collection; no run starts and no source is
saved.

An authenticated browser WebSocket operator with workspace-management authority
can explicitly select a backend-local checkout. The browser UI itself sends
no source fields. A later workflow or agent review can use that saved source
only while the protected selection and the operator's authority still match.

## Results

Runs and findings remain owned by the existing task review service.
`get_code_review_kandev` delegates to `task.review.get`; changing a source does
not erase earlier results. The review tools are registered with the task-mode
review tools. No browser UI changes are introduced.
