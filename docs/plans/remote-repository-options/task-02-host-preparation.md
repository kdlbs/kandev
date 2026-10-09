---
id: "02-host-preparation"
title: "Host preparation"
status: done
wave: 2
depends_on: ["01-policy-contract"]
plan: plan.md
requirements:
  - REQ-TASKS-REMOTE-OPTIONS-002
  - REQ-TASKS-REMOTE-OPTIONS-003
acceptance_criteria:
  - AC-TASKS-REMOTE-OPTIONS-002.1
  - AC-TASKS-REMOTE-OPTIONS-002.3
  - AC-TASKS-REMOTE-OPTIONS-002.4
  - AC-TASKS-REMOTE-OPTIONS-003.1
  - AC-TASKS-REMOTE-OPTIONS-003.2
  - AC-TASKS-REMOTE-OPTIONS-003.3
  - AC-TASKS-REMOTE-OPTIONS-003.4
  - AC-TASKS-REMOTE-OPTIONS-003.5
system_design:
  - ../../specs/tasks/system-design/remote-repository-options.md
---

# Task 02: Host preparation

## Summary and scope

Own `apps/backend/internal/repoclone/clone.go`, new
`clone_checkout_options_test.go`, `internal/worktree/worktree.go`,
`manager_lifecycle.go`, new `manager_checkout_options_test.go`, and option
propagation in `internal/orchestrator/executor/executor.go`,
`executor_resume.go`, and `executor_execute.go`. Extend existing managed
credential environment composition only as needed; do not weaken its account
isolation or make the personal host bridge a fallback.

Implement the mode-specific cache path, staged readiness validation, worktree
scope before checkout, and post-clone authorized lazy reads. Amend the existing
no-promisor test to preserve its guarantee for Standard and add positive tests
only for the newly supported mode. Exclude UI and remote executor scripts.
Enable host capabilities only for isolated and credential-proven paths.

## Dependencies

Complete the preceding work order; no parallel execution is planned.

## Implementation acceptance

1. A filter-capable authenticated fixture proves omitted historical blobs are not downloaded and later reads succeed after initial credential cleanup; revoked auth fails without fallback.
2. Two tasks sharing a repository retain different sparse scopes and unchanged branch/contribution authority through cancellation, retry, restart, and resume.
3. Invalid directories, incomplete clones, and dirty existing worktrees fail safely; shared repository paths and other task files remain unchanged.

## Verification

Use TDD: reproduce the required failure before implementation, then verify the
real behavior. New files named below are created by this work order. Commands
run from the repository root; install once with `(cd apps && pnpm install
--frozen-lockfile)` before any package command if this worktree lacks dependencies.

```sh
(cd apps/backend && go test -race ./internal/repoclone ./internal/worktree ./internal/orchestrator/executor ./internal/gitcredentials -run 'Test.*(CheckoutOptions|AuthenticatedClone|RepositoryCheckout|CheckoutCapabilities)' -count=1)
```

## Risks

Preserve task identity, provider authorization, credential redaction, existing
branch/contribution semantics, and user work. A capability or environment blocker
is not a passing result; record it and do not advertise unverified support.

## Results

Targeted race tests passed for repoclone, worktree, executor, and credentials.
Real Git fixtures cover filtered historical objects, lazy reads, cache identity,
independent sparse scopes, dirty reuse, and excluded submodules.
