---
created: 2026-10-14
status: done
requirements:
  - REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-001
  - REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-002
system_design:
  - ../../specs/tasks/system-design/quick-chat-agent-history-cleanup.md
legacy_specs: []
---

# Implementation Plan: Quick Chat Agent History Cleanup

## Overview

Add one best-effort step to the existing task deletion pipeline: after a Quick Chat's scratch
directory is removed, remove the Claude Code or Codex transcript that belongs to each of its
host-local sessions. Manual deletion and the expiration sweep share the pipeline, so both get
the behavior from one hook. Work order 01 delivers the pure removers with filesystem tests.
Work order 02 captures the history identity in the task service and wires the removers into
deletion and expiry.

## Work orders

| Order | Work order | Wave | Depends on | Status |
| --- | --- | --- | --- | --- |
| 01 | [Agent history removers](task-01-agent-history-removers.md) | 1 | none | `done` |
| 02 | [Wire cleanup into Quick Chat deletion and expiry](task-02-wire-quick-chat-deletion.md) | 2 | 01 | `done` |

## Risks

- **Claude path encoding drift.** A future Claude release could change the encoded project
  name. The remover then finds nothing and deletion still succeeds (no data loss).
- **Codex shared indexes.** Codex keeps a `threads` row in `state_5.sqlite` and an entry in
  `session_index.jsonl` after its rollout is removed. Kandev does not edit Codex-owned shared
  stores. Contributor-recorded check (WO-01 results): the thread is hidden from listings and an explicit resume
  fails cleanly; the hidden row keeps the thread's preview text.
- **Working directory of multi-repository Quick Chats.** Resolved: the standalone executor
  records its launch workspace path (the agent cwd) as `worktree_path`.

## Verification

Implementation work orders are complete. Hosted source-bound proof
[38620076258](https://github.com/kdlbs/kandev/actions/runs/38620076258) verifies
the combined lifecycle, identity, expiry-clock, and filesystem regressions. Full
repository and platform acceptance are recorded in the PR checks and review evidence.


From `apps/backend`:

```bash
go test -trimpath ./internal/task/service/agenthistory/... -count=1
go test -trimpath ./internal/task/service/ -run 'QuickChat' -count=1
```

From the repository root:

```bash
bash scripts/lint-go-changed $(git diff --name-only main -- '*.go')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

No E2E test: the behavior has no rendered surface; deletion UI is unchanged.
