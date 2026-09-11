---
id: "01-contribution-preflight"
title: "Classify contribution resume preflight"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-002
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.1
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.2
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.3
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.4
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.5
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
---

# Task 01: Classify contribution resume preflight

## Summary

Add a typed, exact-destination history rejection reason to the preflight result. Keep rejected push semantics and admit only explicit existing-session resume. Cover cold launch and workspace promotion without using token presence as the policy switch.

## In scope

Add a typed, exact-destination history rejection reason to the preflight result. Keep rejected push semantics and admit only explicit existing-session resume. Cover cold launch and workspace promotion without using token presence as the policy switch.

## Out of scope

Initial creation policy, real push behavior, automatic history reconciliation, and new UI warnings.

## Acceptance

- History-only rejection permits resume while local HEAD, dirty files, and remote HEAD remain unchanged.
- Authentication, missing ref, permission, transport, malformed/mixed output, and a second repository failure remain blocking.
- Default repository routing retains its empty internal key but uses a safe nonempty display label.

## Regression and verification

Add TestPushPreflightHistoryClassification using a bare source and two local clones. Cover behind, diverged, aligned, ahead, wrong ref, unknown output, and authentication/transport failures. Add TestContributionResumePreflight for cold resume, workspace promotion, initial launch, and mixed repositories. Prove the RED failure is blocked resume, not a missing test symbol.

Run from the repository root. Use the listed test names for new regressions.
If any existing test is changed beyond this list, add its exact command here
before marking results complete.

```bash
(cd apps/backend && go test ./internal/agentctl/server/process -run 'Test(PushPreflightHistoryClassification|GitOperatorRemoteContribution)' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/api -run TestHandleGitPushPreflight -count=1)
(cd apps/backend && go test ./internal/agent/runtime/agentctl -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run TestContributionResumePreflight -count=1)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_test.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/git_handlers_test.go`
- `apps/backend/internal/agent/runtime/agentctl/ (GitPushPreflight response DTO)`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_contribution_preflight_test.go (new)`

## Dependencies

None.
Execution is sequential.

## Inputs

- [Package evidence and design](plan.md).
- [System design](../../specs/tasks/system-design/remote-contribution-tasks.md).
- Applicable REQ and AC identifiers in frontmatter.
- Read scoped AGENTS.md and the existing adjacent tests before implementation.

## Risks

Older agentctl binaries omit the reason: fail closed. Porcelain classification must reject unknown or mixed failures. The exception does not certify write access.

## Parallelism

`sequential`

## Results

Completed. Push preflight now classifies an exact, history-only remote update
without changing local or remote Git state. Only explicit existing-session
resume admits that result; initial launches, mixed repositories, malformed
output, authentication, permission, transport, missing-ref, and unknown
failures remain blocking. The default repository keeps its empty internal key
while using a safe display label.

Verification passed:

- `go test ./internal/agentctl/server/process -run 'Test(PushPreflightHistoryClassification|GitOperatorRemoteContribution)' -count=1`
- `go test ./internal/agentctl/server/api -run TestHandleGitPushPreflight -count=1`
- `go test ./internal/agent/runtime/agentctl -count=1`
- `go test -race ./internal/agent/runtime/lifecycle -run TestContributionResumePreflight -count=1`
