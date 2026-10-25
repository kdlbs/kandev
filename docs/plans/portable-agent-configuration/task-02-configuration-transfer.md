---
id: "02-configuration-transfer"
title: "Transfer selected configuration bundles"
status: done
wave: 2
depends_on: ["01-portable-config-catalog"]
plan: "plan.md"
spec: "../../specs/agents/requirements/portable-agent-configuration.md"
---

# Task 02: Transfer selected configuration bundles

## Acceptance

- Fresh Local Docker, SSH, and Sprites provisioning copies selected bundles with safe paths, limits, and mode `0600`.
- Warm resume does not copy host configuration, while an environment reset copies current host data.
- A missing, invalid, or failed optional copy writes a preparation warning and does not stop the launch.

## TDD scenarios

1. RED: Add transfer tests for regular files, symbolic links, traversal, size limits, modes, missing sources, and partial bundles.
2. RED: Add lifecycle tests for fresh provisioning, warm resume, reset, unknown IDs, and unselected bundles.
3. RED: Add adapter tests for Local Docker, SSH, and Sprites target paths.
4. GREEN: Pass `agent_config_bundles` through profile metadata and add the shared transfer engine.
5. GREEN: Connect the transfer engine to each isolated executor.
6. REFACTOR: Keep authentication upload and configuration upload independent while sharing safe file-write primitives.

## Verification

- `cd apps/backend && go test -tags fts5 -run 'Test.*PortableConfig|Test.*AgentConfigBundle' ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor`
- `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_state.go`
- `apps/backend/internal/orchestrator/executor/executor_state_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/credential_uploader.go`
- `apps/backend/internal/agent/runtime/lifecycle/credential_uploader_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/agent_session_seeder.go`
- `apps/backend/internal/agent/runtime/lifecycle/agent_session_seeder_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_credentials.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_credentials_remote_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_credentials.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_e2e_test.go`

## Dependencies

- Task 01 supplies the bundle types, IDs, and catalog lookup.

## Parallelism

Parallel-safe with Task 03 after Task 01.
This task owns backend runtime and orchestrator files.

## Inputs

- The catalog from Task 01.
- Existing credential upload adapters.
- Existing Docker session-directory seeding.
- Existing SSH and Sprites remote-home resolution.

## Output contract

Report lifecycle timing, warning behavior, security checks, RED evidence, GREEN evidence, and test results.

## Risks

- SSH writes into a persistent remote user home.
- A reused remote home can retain an older file after a skipped copy.
- A transfer error must not expose configuration contents in logs.

## Results

Implemented allowlisted regular-file transfer with traversal/symlink/size
checks, private file modes, best-effort preparation warnings, and fresh/reset
versus warm-resume behavior for Local Docker, SSH, and Sprites. Lifecycle tests
passed with 1,857 passed. The gated Docker E2E passed the selected-only,
warm-resume, and reset-copy scenario.
