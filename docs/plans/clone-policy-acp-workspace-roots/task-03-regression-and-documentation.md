---
id: "03-regression-and-documentation"
title: "Regression and documentation"
status: in_progress
wave: 3
depends_on: ["01-clone-policy-attestation", "02-acp-additional-directories"]
plan: "plan.md"
spec: "../../specs/task-git-metadata-permissions/spec.md"
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
- `docs/specs/task-git-metadata-permissions/spec.md`
- `docs/specs/tasks/system-design/attach-workspace-sources.md`
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

- PASS: `GOCACHE=/tmp/kandev-gocache go test ./internal/agent/runtime/lifecycle ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/acp ./internal/agent/runtime/agentctl`
- PASS: `GOCACHE=/tmp/kandev-gocache go test -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp`
- PASS: `GOCACHE=/tmp/kandev-gocache make lint`
- PASS: `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`
- GATED: `pnpm e2e:run --project containers tests/docker/add-workspace-sources.spec.ts tests/docker/plugin-git-credentials.spec.ts` built the backend and web assets, then failed before tests ran because `apt-get update` in the disposable `node:22-slim` fixture image received Debian `NOSPLIT` metadata errors stating that network authentication is required. A diagnostic no-build retry produced the same result. Docker daemon access is available; SSH uses this Docker fixture and is gated by the same failure. No Sprites environment credential is present.

Public docs updated: `docs/public/executors.md` (reference) and `docs/public/tasks-and-workflows.md` (how-to) state the clone-policy and ACP capability boundaries. Provider Usage dependents were not modified.
