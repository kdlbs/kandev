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
the writer session's metadata.

## Results

Runs and findings remain owned by the existing task review service.
`get_code_review_kandev` delegates to `task.review.get`; changing a source does
not erase earlier results. The review tools are registered with the task-mode
review tools. No browser UI changes are introduced.
