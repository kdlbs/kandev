---
id: "03-regression-and-documentation"
title: "Regression and documentation"
status: pending
wave: 3
depends_on: ["01-clone-policy-attestation", "02-acp-additional-directories"]
plan: "plan.md"
spec: "../../specs/platform/task-git-metadata-permissions.md"
---

# Task 03: Regression and Documentation

## Acceptance

- Focused Go, race, and lint evidence is recorded; available Docker executor E2E proves the normal clone launch path.
- SSH and Sprites evidence records whether their fixture/provider credentials are present, the exact skipped gate when absent, and no fabricated pass.
- Public executor/task documentation states the capability and environment limits without exposing internal host paths; Provider Usage packages remain unchanged.

## Verification

```bash
cd apps/backend
go test ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/acp
go test -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp
make lint
cd ../web
pnpm e2e:run --project containers
```

## Files likely touched

- `docs/public/executors.md`
- `docs/public/tasks-and-workflows.md`
- `docs/specs/platform/task-git-metadata-permissions.md`
- `docs/specs/tasks/attach-workspace-sources.md`
- focused Docker/SSH/Sprites E2E files only when a deterministic regression is needed

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential: evidence must target the final behavior.

## Inputs

Docs Maintainer guidance, executor/testing docs, accepted ADRs, and completed task receipts.

## Output contract

Summary, files changed, exact terminal evidence, Docker/SSH/Sprites gates, docs classification, Provider Usage freeze confirmation, blockers, risks, and task/plan status update.

## Results

Pending.
