---
id: "02-executor-mode-delivery"
title: "Install modes in every executor"
status: done
wave: 2
depends_on:
  - "01-safe-mode-overlay"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.10
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.13
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.14
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 02: Install modes in every executor

## Summary

Install the resolved overlay after selected configuration bundles and before
the agent starts. Report delivery only when the agent-visible path contains the
final mode.

## In scope

- Order Docker seeding and mode merge correctly.
- Upload to SSH and Kubernetes session homes and set matching launch paths.
- Preserve selected bundle fields and warm-resume behavior.
- Return installation failure to lifecycle delivery reporting.

## Out of scope

- Changes to bundle selection UI or unrelated credential transfer.
- Provider mode-report timing, owned by Task 03.

## Acceptance

1. Docker, SSH, and Kubernetes tests read the final agent-visible file and see the requested mode.
2. A selected bundle's unrelated settings survive; an unselected bundle is absent.
3. Failed transfer or a path mismatch yields `Delivered: false` and a session-visible reason.

## Verification

```bash
cd apps/backend
go test ./internal/agent/runtime/lifecycle -run 'Test(Docker|SSH|Kubernetes|InitialMode|PortableConfig)' -count=1
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_credentials.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_files.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_session_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_credentials_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_test.go`

## Dependencies

Task 01 defines the overlay input and outcome contract.

## Risks

The SSH remote account and Kubernetes pod can use a home different from the
backend host or Docker target. Warm resumes must not recopy host settings.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/agent-permission-control-integrity.md), `002.7`, `002.10`, `002.13`, `002.14`.
- [System design](../../specs/agents/system-design/agent-permission-control-integrity.md), initial-mode delivery.
- [Portable configuration contract](../../specs/agents/requirements/portable-agent-configuration.md).

## Results

Implemented the start-mode overlay in each supported container or remote
executor. Docker installs after session and bundle seeding. SSH and Kubernetes
write to their session-owned agent configuration paths. Each executor confirms
delivery only after the exported path matches the installed path; failures and
unavailable paths remain unconfirmed and emit a session-visible preparation
warning with the requested mode and reason.

Regression coverage reads the final Docker settings after selected bundle
preparation, verifies SSH through the in-process SFTP server, and checks the
Kubernetes upload path against the session environment. It also covers path
mismatch and no-copy behavior for unselected host configuration.

Validation passed:

```bash
cd apps/backend
go test ./internal/agent/runtime/lifecycle -run 'Test(Docker|SSH|Kubernetes|InitialMode|PortableConfig)' -count=1
```
