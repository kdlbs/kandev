---
id: "01-workspace-grants"
title: "Explicit companion workspace grants"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-DOCKER-001
  - REQ-EXECUTORS-K8S-DOCKER-003
acceptance_criteria:
  - AC-EXECUTORS-K8S-DOCKER-001.1
  - AC-EXECUTORS-K8S-DOCKER-001.2
  - AC-EXECUTORS-K8S-DOCKER-001.3
  - AC-EXECUTORS-K8S-DOCKER-001.4
  - AC-EXECUTORS-K8S-DOCKER-003.2
  - AC-EXECUTORS-K8S-DOCKER-003.3
system_design:
  - ../../specs/executors/system-design/kubernetes-docker-workloads.md
---

# Task 01: Explicit companion workspace grants

## Summary

Allow an explicitly named ordinary sidecar to share Kandev's workspace through
the existing raw template. Keep runtime/auth mounts private in the composed
spec, verify admitted grants against the requested template, and preserve
snapshot and exact-resource lifecycle behavior.

## In scope

- Use TDD for the template, composition and both-direction admission checks
  described in the system design; retain the main-container contract.
- Cover RO/RW, all three storage modes, unchanged profiles, extra sidecars,
  malicious admission and lost-Pod replacement after a profile edit.
- Keep UID, nonce, ownership and existing-claim cleanup regressions passing.
- Reconcile the foundation, ownership ADR and public executor template contract.
  Clarify README executor support and replace obsolete `docs/k8s.md` bulk-deploy
  advice with the current guide. Do not imply the full recipe is shipped yet.

## Out of scope

Image builds, live cluster changes, new API/UI/profile fields, automatic mounts,
privilege grants and broad refactors of unrelated executor lifecycle code.

## Acceptance

1. An explicit ordinary sidecar mount survives composition; ambiguous/redirected
   grants and access to reserved runtime/auth material fail before API writes.
2. Admission cannot add, remove, transfer or alter a requested grant; known
   neutral defaults and unrelated compatible additions remain supported.
3. Snapshot replacement preserves the original grant and claim identity.
   Existing managed/existing-claim cleanup and nonce checks remain unchanged.

## Verification

From the repository root, after the red/green implementation cycle:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 1 ./internal/agent/kubernetes ./internal/agent/runtime/lifecycle -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add `workspace_grant_test.go` with `TestWorkspaceGrantTemplateValidation`,
`TestWorkspaceGrantComposition`, `TestWorkspaceGrantAdmission`; add
`executor_kubernetes_workspace_grant_test.go` with
`TestKubernetesWorkspaceGrantUsesLaunchSnapshot`. Use assertions on resulting
mounts/files/inventory and rejection outcomes, not internal call counts.
Task 03 owns API-server defaulting and actual Docker bind evidence.

## Files likely touched

- `apps/backend/internal/agent/kubernetes/{template,compose,admission}.go`
- `apps/backend/internal/agent/kubernetes/workspace_grant_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_workspace_grant_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_reconnect.go`, only if tests expose a required adjustment
- `docs/public/k8s.md`, `docs/k8s.md`, `README.md`
- `docs/specs/kubernetes-executor/spec.md`
- `docs/decisions/2026-08-24-kubernetes-executor-resource-ownership.md`

## Dependencies

None. Start only after the user requests implementation of this package.

## Risks

Comparing only admitted grants misses a removed recipient; comparing container
positions permits transfer. Overly broad default normalization could hide a
changed access mode. Avoid changing existing main-container validation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/kubernetes-docker-workloads.md), requirement 001.
- [System design](../../specs/executors/system-design/kubernetes-docker-workloads.md), Workspace grant and admission; Lifecycle and evidence.
- Existing `template_validation_test.go`, `compose_test.go`, `admission_test.go`,
  `executor_kubernetes_test.go` and `executor_kubernetes_restart_test.go` patterns.

## Results

Implemented explicit RO/RW companion grants with exact mount shape, duplicate
identity rejection and admission checks in both directions. Composition retains
the requested grant without changing the managed volume or main container.
Snapshot replacement tests cover retained grants and prevent profile edits from
adding grants to recorded workloads. Existing ownership and cleanup checks pass.

Validation:

- Red: `TestWorkspaceGrant*` rejected valid grants and exposed accepted grant/recipient removal.
- Green: `GOMAXPROCS=2 go test -p 1 ./internal/agent/kubernetes ./internal/agent/runtime/lifecycle -count=1` passed.
- `python3 scripts/list-docs.py validate`: 267 decisions and 870 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed.
- `git diff --check`: passed.

Real API defaulting and Docker lifecycle evidence remain assigned to task 03.
No neutral mount default normalization has been introduced.
