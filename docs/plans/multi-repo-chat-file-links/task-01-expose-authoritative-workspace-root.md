---
id: "01-expose-authoritative-workspace-root"
title: "Expose authoritative workspace root"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
---

# Task 01: Expose the authoritative workspace root

## Acceptance

- Full and summary task-session API responses include `workspace_path`.
- A session linked to a promoted task environment returns the environment's current workspace root,
  even when the persisted session value and primary worktree still point at the original repository.
- `worktree_path` remains the first/primary session worktree for backward compatibility.
- A legacy or environment-less session falls back to its persisted session workspace path.
- No database migration or file-API change is introduced.

## Verification

```bash
make -C apps/backend test
```

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/session_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/converters_test.go`

## Dependencies

None.

## Parallelism

`sequential`. Task 02 consumes the API field and its environment-backed persistence semantics.

## Inputs

- Spec sections: **API surface**, **Failure modes**, and the multi-repository chat-link scenarios.
- ADR-2026-07-22-runtime-mutable-task-workspace-sources.
- Existing patterns: `taskSessionSelectCols`, `taskSessionFromClause`, both task-session scanners,
  `FromTaskSession`, and `FromTaskSessionSummary`.

## TDD Sequence

1. Add repository regressions that create a session plus task environment, promote only the
   environment workspace root, and expect session reads to return that root while retaining the
   primary worktree. Confirm RED because reads currently return the stale session value.
2. Add converter assertions for `workspace_path` on both DTO shapes and confirm RED because the
   field is absent.
3. Add the environment-backed projection to the shared SELECT/scanners and the DTO field mappings.
4. Add the legacy no-environment fallback case and reach GREEN for both packages.
5. Run the exact verification command above.

## Output Contract

Report the expected RED failures, the query/DTO changes, confirmation that scanner order and legacy
fallbacks are covered, files changed, exact command results, risks, and blockers. Mark this task
`done` and update its plan checkbox in the primary conversation.
