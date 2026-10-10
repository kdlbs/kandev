---
id: "01-extract-mcp-contract"
title: "Extract the shared MCP contract"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-001
acceptance_criteria:
  - AC-RELEASE-COMPACT-RUNTIME-001.3
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
---

# Task 01: Extract the shared MCP contract

## Summary

Move the few task values and parser used by MCP into a dependency-light
package. Keep the service API and MCP behavior stable while removing the
task service and Kubernetes imports from `agentctl`.

## In scope

- Make `internal/task/contract` the sole definition of title length,
  plan-write modes/parser, and plan-revision page limit.
- Retain service type aliases, constants, and forwarding parser as needed by
  existing callers; migrate MCP server imports to the contract.
- Capture the tool-schema, invalid-mode, argument-order, and release-flag
  binary-size evidence. Add a checked-in dependency-graph guard to backend CI
  and the release workflow so Kubernetes cannot silently return to `agentctl`.

## Out of scope

- Runtime packaging, helper downloading, and changes to `kandev`'s
  Kubernetes clientset.

## Acceptance

- Existing MCP task-plan and title inputs and errors remain byte-for-byte
  equivalent where observable.
- `go list -deps ./cmd/agentctl` contains neither the task service nor any
  `k8s.io/` package, and the guard fails if either dependency returns; all
  native/remote builds share this command package.
- Record raw and gzip sizes for a before/after release-flag `agentctl` build.

## Verification

```bash
(cd apps/backend && go test ./internal/task/contract ./internal/task/service ./internal/mcp/server)
bash scripts/check-agentctl-deps.sh
(cd apps/backend && make build-agentctl build-agentctl-remote)
```

## Files likely touched

- `apps/backend/internal/task/contract/contract.go`
- `apps/backend/internal/task/contract/contract_test.go`
- `apps/backend/internal/task/service/plan_mode.go`
- `apps/backend/internal/task/service/task_title.go`
- `apps/backend/internal/task/service/plan_revision_recovery.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/task_plan_append_mode_test.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `scripts/check-agentctl-deps.sh`
- `.github/workflows/backend-tests.yml`
- `.github/workflows/release.yml`

## Dependencies

None.

## Risks

- The MCP schema intentionally does not use an enum for plan mode; adding
  one would change which error clients see.

## Parallelism

`sequential`

## Inputs

- `REQ-RELEASE-COMPACT-RUNTIME-001` and the shared-MCP-contract design.
- Existing service parser and MCP schema/handler tests.

## Results

Done. The new `internal/task/contract` package owns the MCP plan modes and
parser, title maximum, and revision-page maximum. The task service keeps aliases
and a forwarding parser, while MCP no longer imports `internal/task/service`.
The checked-in dependency guard passes and is wired into backend CI and the
release bundle workflow.

Validation passed:

- `go test ./internal/task/contract ./internal/task/service ./internal/mcp/server`
- `bash scripts/check-agentctl-deps.sh`
- `make build-agentctl build-agentctl-remote`

On Linux amd64 with Go 1.26.0 and `-ldflags '-s -w'`, `agentctl` changed from
69,476,808 raw bytes / 22,081,340 gzip bytes to 45,515,624 raw bytes /
15,173,845 gzip bytes. This is a 23,961,184-byte raw reduction (34.5%) and a
6,907,495-byte gzip reduction (31.3%). The post-change dependency graph has
623 packages and contains no Kubernetes package or task service package.
