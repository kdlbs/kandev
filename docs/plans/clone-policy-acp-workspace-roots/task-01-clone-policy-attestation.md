---
id: "01-clone-policy-attestation"
title: "Clone policy attestation"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/task-git-metadata-permissions.md"
---

# Task 01: Clone Policy Attestation

## Acceptance

- Docker, SSH, and Sprites receive a path-free mutable-repository policy requirement when they clone a task checkout; host `GitMetadataProjection` paths never cross that boundary.
- Each runtime derives and validates its canonical regular checkout after preparation and before any agent child starts; unsupported, stale, linked, or invalid checkout policy fails closed with recovery text that contains no host path.
- Initial launch, resume/reset, attachment failure rollback, terminal cleanup, `git add`, `git commit`, ref/reflog locks, and common metadata denial have deterministic lifecycle coverage.

## Verification

```bash
cd apps/backend
go test ./internal/worktree ./internal/agent/runtime/lifecycle
go test -race ./internal/agent/runtime/lifecycle
make lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/git_metadata_permissions.go`
- `apps/backend/internal/agent/runtime/lifecycle/git_metadata_remote.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites.go`
- associated lifecycle and worktree tests

## Dependencies

None.

## Parallelism

Sequential: this changes the shared executor request consumed by task 02.

## Inputs

Task Git Metadata Permissions spec, ADR-2026-08-19, ADR-2026-08-20, and existing remote regular-checkout probe helpers.

## Output contract

Summary, files changed, red/green test receipts, exact policy/error boundary, cleanup evidence, blockers, risks, and task/plan status update.

## Results

Implemented a typed, path-free clone-policy requirement and executor-side attestation for Docker, SSH, and Sprites. Docker bootstrap, SSH, and Sprites validate the actual regular clone after preparation; immediately before `ConfigureAgent`/`Start`, lifecycle asks agentctl to batch-attest every canonical primary and materialized secondary checkout. `CODEX_CONFIG` is rendered only from that returned checkout/Git-directory set, and an unexpected, foreign, swapped, or symlinked root fails closed. Docker, SSH, and Sprites reuse is bypassed when fresh attestation is required. Focused/package Go tests, race verification, and backend lint pass; Docker/SSH E2E remains environment-gated in task 03.
