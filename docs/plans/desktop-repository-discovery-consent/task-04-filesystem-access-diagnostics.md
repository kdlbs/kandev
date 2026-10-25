---
id: "04-filesystem-access-diagnostics"
title: "Add filesystem access diagnostics"
status: completed
wave: 4
depends_on: ["02-runtime-aware-discovery-cache"]
plan: "plan.md"
requirements:
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-004"
acceptance_criteria:
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-004.1"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-004.2"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-004.3"
system_design:
  - "../../specs/workspaces/system-design/local-repositories.md"
---

# Task 04: Add Filesystem Access Diagnostics

## Summary

Add structured operation context to filesystem access. Bound repeated access
warnings and preserve enough data to identify the triggering operation.

## In scope

- Add operation, trigger, target, runtime, and identity fields.
- Use the context in discovery, directory listing, validation, monitoring, and Git polling.
- Add bounded warning output and suppressed-count reporting.
- Add focused tests for denied and repeated operations.

## Out of scope

- Change poll-mode transitions.
- Change root consent, cache, or retry policy.
- Export diagnostics automatically.

## Acceptance

- Tests assert all required fields for each named filesystem operation.
- Repeated-denial tests assert the warning bound and suppressed count.
- Log tests do not depend on macOS to produce a deterministic denial.

## Verification

```bash
cd apps/backend && rtk go test ./internal/task/service ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process
rtk make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/task/service/repository_discovery.go`
- `apps/backend/internal/task/service/directory_listing.go`
- Repository validation service files
- `apps/backend/internal/agentctl/server/process/workspace_monitor.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_poll.go`
- New operation-context and bounded-warning helpers
- Focused tests beside each owning package

## Dependencies

- Task 02 defines discovery triggers and access-failure states.

## Risks

- Warning fields can expose local paths in exported diagnostics.
- A shared logging helper can hide ownership if it becomes too generic.

## Parallelism

`sequential`

## Inputs

- REQ-WORKSPACES-LOCAL-REPOSITORIES-004.
- Existing zap logging conventions in the owning packages.
- Current discovery, directory-listing, validation, monitor, and Git paths.

## Results

Completed.

- Added structured filesystem operation context, bounded access-denial
  warnings, discovery/listing/validation diagnostics, and access-loss pause
  behavior for workspace tracking.
- Added operation-specific coverage for monitor, files, Git status, and
  discovery paths without exposing unbounded local-path diagnostics.
- Verification passed: targeted backend tests, race-enabled lifecycle/process/
  task-service tests, and `make lint` with zero issues.
