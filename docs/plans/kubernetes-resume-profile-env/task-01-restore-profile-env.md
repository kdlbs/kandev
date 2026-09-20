---
id: "01-restore-profile-env"
title: "Restore recorded Kubernetes profile environment"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.6
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.7
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.8
system_design:
  - ../../specs/executors/system-design/kubernetes-failure-recovery.md
---

# Restore Recorded Kubernetes Profile Environment

## Summary and scope

Restore only environment source definitions during recorded Kubernetes resume.
Cover authoritative profile selection, retained snapshot preservation, deferred
secret resolution, deleted profiles, lookup failure, and foreign ownership.
Update the public Kubernetes guide and linked recovery contract.

## Out of scope

Provider modes, Pod/storage mutation, deployment, and unrelated cleanup.

## Acceptance

1. Recorded profile sources reach the resume launch request without substituting
   the mutable session profile or rewriting workload metadata.
2. Missing profiles and actual lookup/ownership failures have distinct outcomes.
3. Literal values and unresolved secret references retain their source identity.

## Verification

Run from the repository root with Go 1.26 and golangci-lint on PATH:

```bash
(cd apps/backend && go test ./internal/orchestrator/executor -count=1)
(cd apps/backend && go test -race ./internal/orchestrator/executor -run 'TestKubernetesResume' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_kubernetes_env.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_kubernetes_env_test.go`
- `docs/public/k8s.md`
- Linked recovery requirements and system design.

## Dependencies and parallelism

None; sequential.

## Inputs

- [Requirements](../../specs/executors/requirements/kubernetes-failure-recovery.md)
- [Design](../../specs/executors/system-design/kubernetes-failure-recovery.md#resume-environment-sources)
- Existing recorded Kubernetes resume fixtures and repository error contracts.

## Results

The regression first failed with missing profile definitions. Executor package
and targeted recovery race tests, changed-code Go lint, architecture and commit
hooks passed before publication. Public documentation validation checked 47
pages and passed all 62 validator tests. Synthetic end-to-end acceptance is
summarized in the plan without private infrastructure details.

Documentation catalog validation, full specification lint, all 36 spec-linter
tests, diff checks, and the PR documentation coverage evaluator passed. The
focused `TestKubernetesResume` race command passed again during PR follow-up.
Full executor package tests also passed on a detached synthetic merge with the
current base. Remote CI/review remains a PR delivery gate.

## Review regression coverage

Added `TestKubernetesResumeWithoutRecordedProfileSkipsLookup` for nil, empty,
and explicitly empty profile metadata. A temporary Go source overlay removing
the empty-ID guard made the test fail with the sentinel lookup error, proving
the test detects unintended repository access. This test does not require a production change.

A second review found that the resumed session could retain a conflicting
profile selection for subsequent subprocess starts. The regression
`TestKubernetesResumePreservesProfileForExistingWorkspaceStart` first reproduced
wrong-profile injection after request construction and guarded session
persistence. It uses the real lifecycle profile/secret resolver and existing
workspace environment delivery. Resume now restores the recorded profile ID
alongside the executor ID before persistence.

Post-review validation passed: `go test -race ./internal/orchestrator/executor
-count=1`, full changed-code Go lint against the current PR base (0 issues),
documentation catalog validation, specification lint, and diff checks.
