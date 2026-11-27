---
id: "11-workspace-admin"
title: "Typed workspace configuration commands"
status: complete
wave: 11
depends_on: ["01-exact-host-foundation", "06-workspace-observations"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-007
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-007.1
  - AC-PLUGINS-MANAGED-COORDINATION-007.2
  - AC-PLUGINS-MANAGED-COORDINATION-007.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 11: Typed workspace configuration commands

## Summary

Expose bounded workflow, step, and repository administration through existing domain services. Keep destructive changes behind native confirmation.

## In scope

- Add typed workspace default updates, workflow/step create/update/reorder, and repository registration/attachment/base-branch operations with capability discovery. Keep script and secret-binding edits in native human-only settings.
- Preserve synchronized workflow and live task/worktree invariants; expose destructive preview/confirm only for supported existing domain operations.
- Add exact conflict, foreign-workspace, active-resource, and stale-confirmation coverage. Publish the supported subset explicitly.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Approved configuration changes use shared domain validation with versioned receipts.
- Active or synchronized resources cannot be damaged by stale or unsupported operations.
- No cross-workspace or credential authority is gained through configuration APIs.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/plugins -run TestExactWorkspaceAdministration -count=1)
(cd apps/backend && go test ./pkg/pluginsdk/...)
```

Evidence to create:

- `apps/backend/internal/plugins/host_exact_workspace_test.go`: `TestExactWorkspaceAdministration` with workspace defaults, workflow ordering, step admission, repository validation, and human-only settings cases.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/plugins/host_write.go`
- `apps/backend/internal/task/service/`
- `apps/backend/internal/worktree/`
- `apps/backend/pkg/pluginsdk/`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 06](task-06-workspace-observations.md)

## Risks

An arbitrary JSON configuration writer would bypass synchronized workflow and active worktree rules. Keep operations typed and scope destructive support to existing confirmation services.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Implemented typed exact workspace-default, workflow, workflow-step, and repository commands. Exact writes carry observed resource versions through shared service validation and storage compare-and-set checks; stale writes roll back, while synchronized workflows and active task/session invariants retain native guards. Script and secret-binding changes remain native human-only settings. The supported capability and operation subset is documented in the manifest reference, plugin authoring guide, and gRPC contract.

Validation passed:

- `TMPDIR=/root/.cache/kandev-tmp go test -race ./internal/plugins -run '^TestExactWorkspaceAdministration$' -count=1`
- `TMPDIR=/root/.cache/kandev-tmp go test ./pkg/pluginsdk/...`
- `TMPDIR=/root/.cache/kandev-tmp go test ./internal/backendapp -run '^TestPluginsWorkspaceAdmin' -count=1`
- `TMPDIR=/root/.cache/kandev-tmp go test ./internal/task/repository/sqlite -run '^TestExactWorkspaceConfigurationWritesFenceStoredVersions$' -count=1`
- `TMPDIR=/root/.cache/kandev-tmp go test ./internal/workflow/repository -run '^TestExactWorkflowStepWritesFenceStoredVersions$' -count=1`
- `TMPDIR=/root/.cache/kandev-tmp go test -p 1 ./internal/task/service ./internal/task/repository/sqlite ./internal/workflow/controller ./internal/workflow/service ./internal/workflow/repository -count=1`
- Public-doc validation passed: 62 validator tests and 47 published pages.
- `gofmt -l` on touched Go files and `git diff --check` passed.
