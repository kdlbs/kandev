---
id: "01-workspace-access"
title: "Workspace-only access"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.7
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.10
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Workspace-only access

## Summary and scope

Restore the existing workspace while a prompt recovery block remains open. Keep permissions,
archive and ownership checks. No new controls, prompt dispatch, ACP initialization, or schema.

## Acceptance

1. Blocked repository-free workspace restoration succeeds without agent or prompt calls.
2. Restoration retains the block even with `RecoveryAction`; a supplied prompt is rejected.
3. Ordinary resume/prompt admission remains blocked without explicit recovery authorization.

## Verification

From repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'Test.*(WorkspaceRecoveryBlock|RestoreWorkspace)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_workspace_recovery_block_test.go`

## Dependencies and risks

None. Guard against implicit agent work in lower workspace restoration layers.

## Parallelism

`sequential`

## Inputs

[Design](../../specs/platform/system-design/interrupted-prompt-recovery.md), workspace-only access.

## Results

- Regression tests failed before the production change: workspace inspection hit prompt admission,
  a supplied recovery action resolved the block, and a workspace prompt was accepted.
- `go test -trimpath -race ./internal/orchestrator -run 'Test.*(WorkspaceRecoveryBlock|RestoreWorkspace)' -count=1`: passed.
- Workspace-only restore now bypasses prompt admission, rejects a prompt, and preserves the block.
- Documentation catalog/specification lint and `git diff --check`: passed.
