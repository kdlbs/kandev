---
id: "01-verify-retained-repositories"
title: "Verify retained remote Docker repositories"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
---

# Task 01: Verify Retained Repositories

## Summary

Accept unpublished local commits on correctly identified retained repositories,
using read-only branch/origin verification. Preserve fresh materialization.

## In scope

A real-Git failing-before regression, launch integration and bounded probe
ownership. Include branch/origin/missing-repository negatives and fresh controls.

## Out of scope

Deployment, conflict resolution, credentials, journal recovery, UI and new APIs.

## Acceptance

1. Correct retained repositories with unpublished commits resume without
   materialization, fetch, checkout, reset, deletion or HEAD changes.
2. Every missing/mismatched repository or failed/cancelled probe aborts without
   fallback materialization; cleanup stops only the probe it owns.
3. Fresh and other-provider behavior and rollback preservation remain green.

## Verification

Run the regression before production edits; run this block from repository root.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p=2 ./internal/agent/runtime/lifecycle -run 'TestRetained|TestLaunchRepositoryReconciliation|TestRemoteDockerPostAttachmentLaunchFailures' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race -p=2 ./internal/agent/runtime/lifecycle -run 'TestRetained|TestLaunchRepositoryReconciliation|TestRemoteDockerPostAttachmentLaunchFailures' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/manager_launch.go apps/backend/internal/agent/runtime/lifecycle/retained_launch_workspace.go apps/backend/internal/agent/runtime/lifecycle/retained_workspace_probe.go apps/backend/internal/agent/runtime/lifecycle/retained_repository_verification_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/retained_launch_workspace.go`
- `apps/backend/internal/agent/runtime/lifecycle/retained_workspace_probe.go`
- `apps/backend/internal/agent/runtime/lifecycle/retained_repository_verification_test.go`
- Owning design and this package.

## Dependencies

None. Reuse existing retention policy and authenticated agentctl transport.

## Risks

Observe completion before process expiry; preserve exact process cleanup and
ordinary materialization's strict commit checks.

## Parallelism

sequential

## Inputs

Use requirement `REQ-EXECUTORS-REMOTE-DOCKER-001`, criterion
`AC-EXECUTORS-REMOTE-DOCKER-001.12`, and the paired retained-verification
design section. The retained-workspace and probe-lifetime regression tests are
in `apps/backend/internal/agent/runtime/lifecycle/`. Existing fake-Docker
rollback and agentctl HTTP fixtures also apply.

## Results

Implemented after design handoff approval. The actual-Git/authenticated launch
regression failed before the fix and passes afterward. Targeted identity, fresh
provider, cancellation, process-lifetime and rollback checks pass, as do the
full lifecycle package and the focused race suite. Ordinary materialization
collision checks remain unchanged. No deployment or paid agent run occurred.
