---
id: "02-hooks-setting-persistence-and-propagation"
title: "Hooks setting, persistence and propagation"
status: draft
wave: 2
depends_on:
  - "01-policy-seam-and-default-isolation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-GITISOLATION-001
  - REQ-AGENTS-GITISOLATION-002
acceptance_criteria:
  - AC-AGENTS-GITISOLATION-001.12
  - AC-AGENTS-GITISOLATION-002.1
  - AC-AGENTS-GITISOLATION-002.2
  - AC-AGENTS-GITISOLATION-002.3
  - AC-AGENTS-GITISOLATION-002.4
  - AC-AGENTS-GITISOLATION-002.5
  - AC-AGENTS-GITISOLATION-002.6
  - AC-AGENTS-GITISOLATION-002.7
  - AC-AGENTS-GITISOLATION-002.8
  - AC-AGENTS-GITISOLATION-002.9
system_design:
  - ../../specs/agents/system-design/managed-git-repository-code-isolation.md
---

# Task 02: Hooks setting, persistence and propagation

## Summary

Add `run_git_hooks` to the repository model, both stores, the REST API and the
workspace clone, and deliver it to backend and agentctl Git as the hooks mark.

## In scope

- SQLite and PostgreSQL migrations (default false) and upgrade fixtures.
- Model, DTO, create/update handlers (`*bool` on update), workspace clone.
- Backend command contexts built from the repository record.
- `MetadataKeyGitHooks` through local, Docker (including remote Docker), SSH and Sprites launches, the
  agentctl create request, config overrides, and `process.Manager`.
- Tests: default false after upgrade; hooks run only in the opted-in repository
  of a two-repository workspace; submodule never runs hooks; the setting is absent
  from plugin host-API writes, and `update_settings` rejects a `run_git_hooks`
  key (field tagged `json:"-"` on the service request, read-only catalog entry);
  an update that omits the field keeps the value; export omits it and import
  leaves it off.
- `managed-git-policy.v1` in `SurvivalCapabilities`; a new instance's create is
  preceded by an unconditional `/identity` check and the launch fails when it is
  absent, times out or is malformed. Retained remote and container instances are
  not checked; the backend's local control server adoption also requires the
  capability, so a surviving pre-change local server is refused and replaced.
- `seedExecutionGitHooks` on the lifecycle `Manager` (last metadata step before the
  Manager's create call on launch and on-demand paths; deletes a stale key first),
  builders copy the key, and `collectTaskGitHooks` in the task service beside
  `collectTaskBaseBranches`; tests for each executor shape, a repository without a
  base branch, a base-branch rekey that leaves the map unchanged, and key parity
  with the launch path's base-branch keys.
- A test that a changed setting applies to a newly created instance and not to an
  existing one.
- A WebSocket `repository.update` test that the field is ignored.

## Out of scope

Web UI, public docs, any runtime endpoint that changes a repository's value (no narrowing route, no remote or container retained-instance adoption check), Kubernetes and plugin-remote executors (hooks off).

## Acceptance

`AC-AGENTS-GITISOLATION-001.12` and `AC-AGENTS-GITISOLATION-002.1` through `002.9`.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/task/... ./internal/persistence/... -count=1
go test -trimpath ./internal/agent/runtime/lifecycle ./internal/agentctl/... -count=1
bash ../../scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Likely files and risks

`internal/task/models/models.go`, `internal/task/dto/dto.go`,
`internal/task/handlers/repository_handlers.go`,
`internal/task/repository/sqlite/*`, the PostgreSQL store,
`internal/agent/runtime/lifecycle/executor_*.go`,
`internal/agentctl/server/{config,instance,process}`. Risk: migration ordering
against live databases; every repository column list that enumerates
`pull_before_worktree` must gain the new column.

## Results

Pending.
