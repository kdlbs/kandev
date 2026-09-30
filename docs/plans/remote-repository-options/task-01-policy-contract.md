---
id: "01-policy-contract"
title: "Task policy contract"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-REMOTE-OPTIONS-002
  - REQ-TASKS-REMOTE-OPTIONS-003
acceptance_criteria:
  - AC-TASKS-REMOTE-OPTIONS-002.1
  - AC-TASKS-REMOTE-OPTIONS-002.2
  - AC-TASKS-REMOTE-OPTIONS-002.3
  - AC-TASKS-REMOTE-OPTIONS-002.4
  - AC-TASKS-REMOTE-OPTIONS-003.2
system_design:
  - ../../specs/tasks/system-design/remote-repository-options.md
---

# Task 01: Task policy contract

## Summary and scope

Own the typed options codec, request/response propagation, task metadata
persistence, capability evaluation/endpoint, and pre-side-effect validation.
Likely files: `apps/backend/internal/task/models/repository_checkout_options.go`
(new), `internal/task/service/service_requests.go`, `service_tasks.go`, task
handler request adapters, `pkg/api/v1/task.go`, task DTO converters,
and task repository round-trip tests. Follow existing workspace/provider
inspection authorization; capability evaluation performs no clone.

Keep all capabilities false until the implementing path passes its required
runtime tests. Existing/omitted options retain behavior. Exclude Git command
changes and rendered controls. No schema migration is needed.

## Dependencies

No implementation dependencies. Read the requirement and design before coding.

## Implementation acceptance

1. Typed options survive create/read/update-preservation/restart/resume round trips without writing workspace repository defaults or losing unrelated metadata.
2. Invalid enums/paths/limits, local-source misuse, unsupported executor/provider combinations, and post-materialization edits fail before launch side effects.
3. Capability results are identity/profile scoped and submission independently recomputes them.

## Verification

Use TDD: reproduce the required failure before implementation, then verify the
real behavior. New files named below are created by this work order. Commands
run from the repository root; install once with `(cd apps && pnpm install
--frozen-lockfile)` before any package command if this worktree lacks dependencies.

```sh
(cd apps/backend && go test ./internal/task/models ./internal/task/service ./internal/task/handlers ./internal/task/dto ./pkg/api/v1 -run 'Test.*(RepositoryCheckoutOptions|CheckoutCapabilities)' -count=1)
```

## Risks

Preserve task identity, provider authorization, credential redaction, existing
branch/contribution semantics, and user work. A capability or environment blocker
is not a passing result; record it and do not advertise unverified support.

## Results

Passed the targeted Go command above. Regression failures observed first:
metadata lost options, invalid options accepted, request/API conversions lost
options, updates discarded options, local repositories accepted options, and
the capabilities route returned 404. All now pass. Runtime capabilities are enabled only for GitHub Worktree and Local Docker
with built-in preparation scripts and managed credentials, verified below.

Source correction: this checkout has no `internal/task/controller` package;
HTTP/WS request adapters live in `internal/task/handlers`. The command was
corrected to the existing package rather than creating an artificial directory.
