---
id: "01-restart-safe-cleanup"
title: "Recover shared control credentials before stop"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.9
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.10
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.11
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.12
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.13
system_design:
  - ../../specs/executors/system-design/kubernetes-failure-recovery.md
---

# Restart-safe shared cleanup

## Summary

Stop an exact retained agent instance after agentctl restarts, without waiting
for refresh or losing repairable state.

## Scope and exclusions

Factor the existing authenticated control operation for attachment and stop;
reuse environment serialization and token recovery persistence. Move attachment
retirement after authenticated DELETE/absence and durable persistence. Preserve
identity, sibling, stale completion and shutdown rules. Exclude resume error
propagation/UI (change 2), agentctl nonce protocol changes and destructive cleanup.

## Implementation acceptance

1. A restart-before-stop regression fails first, then passes with one handshake,
   authenticated DELETE/404 and no Pod/PVC/create-instance mutation.
2. Authentication/identity/delete/persistence errors retain retryable exact
   tracking and recovery credentials; stale cleanup cannot remove a successor.
3. Sibling stop/attach/refresh share one recovered token, preserve unaffected
   clients and complete without lock inversion, duplicate agents or prompt replay.

## Likely files

Existing: `apps/backend/internal/agent/runtime/lifecycle/` files
`executor_kubernetes_shared_cleanup.go`, `executor_kubernetes_shared_control.go`,
`executor_kubernetes_control_recovery.go`, `executor_kubernetes_refresh.go`,
`executor_kubernetes_task_pod_recovery_test.go`, `manager_lifecycle_test.go`.
Inspect `manager_lifecycle.go` before changing it; existing stop-error retention
should remain sufficient. Future output:
`executor_kubernetes_shared_cleanup_recovery_test.go` in that same package.
Do not grow existing files past scoped limits; extract a small authenticated
operation helper if required. Do not modify `executor_resume.go`.

## Regression matrix

Stop before status polling; 401 then handshake; authenticated missing instance;
403/missing nonce; foreign UID/labels; DELETE timeout; canonical write failure
with recovery-secret success; backend reload of that secret; all writes fail;
concurrent sibling stop/attach/refresh; stale attachment completion; cancellation;
existing manager preserves tracking on real stop error. Use production lifecycle
entry points with fake Kubernetes/control transport and failing secret storage.

## Verification

Run from repository root, sequentially. New tests must use the
`TestKubernetesSharedCleanup` prefix; require non-zero selection.

```bash
(cd apps/backend && GOMAXPROCS=2 GOFLAGS=-p=1 go test -trimpath ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'TestControlInstanceErrorsExposeStatus|TestKubernetesSharedCleanup|TestKubernetesTaskPod|TestManager_CleanupStaleExecution' -count=1 -timeout=180s)
(cd apps/backend && GOMAXPROCS=2 GOFLAGS=-p=1 go test -trimpath -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'TestControlInstanceErrorsExposeStatus|TestKubernetesSharedCleanup|TestKubernetesTaskPod|TestManager_CleanupStaleExecution' -count=1 -timeout=240s)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Dependencies, risks and parallelism

No implementation dependency. Sequential only. Confirm instance -> environment
control lock ordering across all callers; retain bounded durable persistence.
Every-durable-write failure cannot claim crash-safe credential recovery.

## Inputs

[Plan](plan.md), [recovery design](../../specs/executors/system-design/kubernetes-failure-recovery.md),
[requirements](../../specs/executors/requirements/kubernetes-failure-recovery.md),
scoped backend AGENTS and existing shared-token tests.

## Results

Targeted non-race and race checks passed for agentctl and lifecycle, including
all eight new cleanup tests, task-Pod controls and manager stop-error retention.
The initial regression run failed with stale-token 401, removed recovery tracking,
and missing credential-persistence retry. A transport regression separately
failed when a non-JSON 401 lost its status; typed control errors now retain it.
Foreign identity, 403/500 deletion failures, absent nonce, concurrent sibling
recovery, canonical/recovery persistence failure and late replacement are covered.
No resume-error/UI change or production rollout occurred.

Commands passed:
`GOMAXPROCS=2 GOFLAGS=-p=1 go test -trimpath ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'TestControlInstanceErrorsExposeStatus|TestKubernetesSharedCleanup|TestKubernetesTaskPod|TestManager_CleanupStaleExecution' -count=1 -timeout=180s`
and the same command with `-race` and `-timeout=240s`, from apps/backend.
Catalog validation, all specification lint and diff whitespace passed.
Native continuation/workspace acceptance remains owned by 04.

PR review follow-up: a missing bootstrap secret reference left the original
401 in the lookup error variable. The new missing-reference cleanup regression
was observed red (raw401 instead of nonce unavailable); lookup errors now have
a separate variable. Clearing the canonical bootstrap reference and instance
metadata must retain the exact attachment, consume no handshake, and delete no
Pod/PVC. The targeted trimpath/race command above passed after the fix:
agentctl1.122s and lifecycle8.377s, including the ninth cleanup regression.
This changes no resume-error or UI behavior.
