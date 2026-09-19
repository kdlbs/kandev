---
id: "01-upstream-status-contract"
title: "Publish complete upstream status evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 01: Publish Complete Upstream Status Evidence

Expose the local HEAD, upstream tip, and upstream-relative counts as one coherent status contract. The
backend already computes most of this data, but the frontend currently drops it and substitutes
base-branch divergence for Push state.

## Context

- `apps/backend/internal/agentctl/types/streams/git.go` already distinguishes `Ahead`/`Behind` from
  `RemoteAhead`/`RemoteBehind`.
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go` computes upstream counts with
  `HEAD...RemoteBranch` and carries them across transient failures.
- `apps/backend/internal/agent/runtime/lifecycle/events.go` publishes the remote counts.
- `apps/web/lib/types/git-events.ts` and `apps/web/lib/ws/handlers/git-status.ts` do not retain those
  counts in `GitStatusEntry`; the handler also drops existing head/base SHAs.

## TDD sequence

1. Extend backend status tests first. Assert `remote_head_commit` for aligned, local-ahead,
   remote-ahead, and diverged graphs. Assert no-upstream clears the head and counts. Assert a transient
   `rev-list` or `rev-parse` failure carries forward the complete prior upstream snapshot only while
   local HEAD is unchanged.
2. Add lifecycle projection and frontend WebSocket handler tests that fail because
   `remote_head_commit`, `remote_ahead`, `remote_behind`, `head_commit`, or `base_commit` is absent.
3. Implement the smallest contract changes, then refactor the upstream observation into a helper if
   needed to keep Go lint limits.

## Implementation

- Add `RemoteHeadCommit string \`json:"remote_head_commit,omitempty"\`` to agentctl and lifecycle Git
  status types.
- Resolve `<RemoteBranch>^{commit}` with the same sanitized ref boundary used for upstream counts. Do
  not run `git fetch` or contact the provider from the polling loop.
- Treat upstream head and counts as one carry-forward unit. Reuse the prior values only when the prior
  and current local HEAD are equal; otherwise leave failed evidence unknown rather than stale.
- Project the field through `PublishGitStatus`.
- Add `head_commit`, `base_commit`, `remote_head_commit`, `remote_ahead`, and `remote_behind` to
  `GitStatusEntry`; copy them in `buildGitStatusEntry` and include upstream values in debug evidence.
- Preserve optional/zero-value compatibility for older status payloads.

## Acceptance

- One Git status event gives the frontend enough evidence to distinguish base divergence from upstream
  divergence and identify the observed upstream tip.
- An absent upstream produces no upstream head and zero upstream counts.
- A transient secondary Git failure does not flash false zero counts, but evidence is not carried across
  a changed local HEAD.
- No remote fetch, checkout mutation, provider call, or database change is introduced.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/git.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- lifecycle Git event tests beside those files
- `apps/web/lib/types/git-events.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/ws/handlers/git-status.ts`
- `apps/web/lib/ws/handlers/git-status.test.ts`

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -run 'Test.*(RemoteAhead|RemoteBehind|RemoteHead|GitStatus)'
cd apps && pnpm --filter @kandev/web test -- lib/ws/handlers/git-status.test.ts
```

## Dependencies and parallelism

No dependencies. Run sequentially because Task 02 consumes the final wire/store shape.

## Output contract

Record red/green test evidence, the exact carry-forward rule, affected JSON fields, verification output,
and blockers. Set this task to `in_progress` before code changes and `done` after verification; update
the matching checkbox in `plan.md`.
