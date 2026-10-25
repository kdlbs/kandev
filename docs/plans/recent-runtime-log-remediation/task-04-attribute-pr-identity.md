---
id: "04-attribute-pr-identity"
title: "Attribute PR identity rejection"
status: done
wave: 4
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 04: Attribute PR identity rejection

## Summary

One task launch failed at the PR identity guard with no field-specific reason.
Return a fixed reason from the comparison and record it when the current
failure or safe fallback path is taken.

## In scope

- Refactor `validPRBaseIdentity` and its bound/unbound helpers to expose a
  bounded reason while preserving their exact acceptance matrix.
- Emit a fixed `invalid_resolved_pr_base` reason when resolver data fails
  structural validation, while preserving the existing block or safe fallback.
- Test attached and fork PRs, valid cases, and every mismatch branch.

## Out of scope

- Relaxing repository binding or changing launch error text and actions.

## Acceptance

- Each mismatch emits a fixed reason with task-repository ID and PR number.
- Invalid resolver output emits a fixed reason for both unbound and bound
  targets without logging provider data.
- Valid fork PRs still launch; invalid identities still fail closed.
- Logs contain no repository URL, credential, or raw provider payload.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run 'Test.*PRBaseIdentity|Test.*PRBase' -count=1)
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_pr_base_identity_test.go`

## Dependencies

None.

## Risks

- A reason-bearing refactor must preserve short-circuit behavior and both
  expected-target and unbound association rules.

## Parallelism

`sequential`

## Inputs

- Runtime-failure-attribution and task-launch-failure-recovery designs.
- Existing PR identity and fork regression tests.

## Results

PR base identity comparisons now return fixed mismatch reasons for the
comparison branch that failed. Structurally invalid resolver output also logs
`invalid_resolved_pr_base` with only task-repository ID and PR number. Tests
cover both unbound rejection and bound fallback without provider data in the
log. Launch behavior remains unchanged. Verification passed:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run 'Test.*PRBaseIdentity|Test.*PRBase' -count=1)
```
