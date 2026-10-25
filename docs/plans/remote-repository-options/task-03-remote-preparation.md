---
id: "03-remote-preparation"
title: "Remote preparation"
status: done
wave: 3
depends_on: ["02-host-preparation"]
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

# Task 03: Remote preparation

## Summary and scope

Own option propagation in `apps/backend/internal/agent/runtime/lifecycle/types.go`,
`workspace_materialization.go`, `default_scripts.go`, the relevant built-in
executor preparation adapters, `internal/agent/runtime/agentctl/client_workspace_sources.go`,
`internal/agentctl/server/api/workspace_materialize.go`, and associated tests.
Own `repository_checkout_options_container_test.go`, which runs the generated
built-in script in the existing E2E image with an authenticated Git fixture. Read scoped agentctl guidance
before editing the API implementation.

Carry policy to the primary repository bootstrap as well as every sibling.
Configure sparse checkout before materialization and retain staged publication.
Use executor-scoped managed credentials for subsequent lazy reads. Do not edit
user-defined preparation scripts; reject unsupported non-default policy rather
than bypassing it. Enable only paths covered by tests. Exclude UI changes.

## Dependencies

Complete the preceding work order; no parallel execution is planned.

## Implementation acceptance

1. Primary and sibling repositories honor independent settings and survive resume/retry with authenticated lazy reads and no token persistence.
2. Remote preparation cannot advertise support while discarding policy in scripts, transport, or materialization; unsupported paths return stable capability reasons.
3. The real container scenario proves sparse files and deferred objects, while targeted adapter tests cover each additionally advertised built-in executor.

## Verification

Use TDD: reproduce the required failure before implementation, then verify the
real behavior. New files named below are created by this work order. Commands
run from the repository root; install once with `(cd apps && pnpm install
--frozen-lockfile)` before any package command if this worktree lacks dependencies.

```sh
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api -run 'Test.*(RepositoryCheckoutOptions|Materialize.*Options|CheckoutCapabilities)' -count=1)
(cd apps/backend && KANDEV_E2E_CONTAINERS=1 go test ./internal/agent/runtime/lifecycle -run TestRepositoryCheckoutOptionsContainer -count=1)
```

## Risks

Preserve task identity, provider authorization, credential redaction, existing
branch/contribution semantics, and user work. A capability or environment blocker
is not a passing result; record it and do not advertise unverified support.

## Results

The container scenario uses the generated production shell directly inside the
existing `kandev-agent:e2e` image. This keeps fixture authentication and Git
transport isolated without relying on a public GitHub endpoint. Native API tests
cover sibling materialization separately. Targeted race tests passed for lifecycle, agentctl client, and agentctl API.
The authenticated Docker sink test passed with later lazy reads after bootstrap
helper disposal and rejected access after revocation. Unsupported preparation
paths remain disabled.
