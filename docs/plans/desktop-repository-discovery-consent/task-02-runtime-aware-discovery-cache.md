---
id: "02-runtime-aware-discovery-cache"
title: "Add runtime-aware discovery state"
status: completed
wave: 2
depends_on: ["01-desktop-folder-selection-boundary"]
plan: "plan.md"
requirements:
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-002"
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-003"
acceptance_criteria:
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.1"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.2"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.6"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.7"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.8"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.9"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.14"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.15"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.16"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.1"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.3"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.4"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.5"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.6"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.7"
system_design:
  - "../../specs/workspaces/system-design/local-repositories.md"
---

# Task 02: Add Runtime-Aware Discovery State

## Summary

Add backend launch policy, install-wide desktop roots, migration state, and a
bounded discovery cache. Add the macOS Home traversal exception.

## In scope

- Resolve server and desktop effective roots.
- Persist selected roots and implicit-Home migration state at install scope.
- Retain configuration-owned roots without copying them into SQLite.
- Add root management, snapshot, and refresh endpoints.
- Add cache freshness, single-flight scans, and last-success preservation.
- Add platform-aware direct-child exclusions for a macOS Home root.

## Out of scope

- Change saved repository grants.
- Add user authorization beyond the trusted-local-user model.
- Implement the repository-selection user interface.

## Acceptance

- Filesystem-spy tests prove that a desktop backend with no effective root does not walk Home.
- Migration and restart tests preserve configured roots and require confirmation for implicit Home.
- Walker tests skip protected direct children of Home and scan an exact protected root.

## Verification

```bash
cd apps/backend && rtk go test ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite
cd apps/backend && rtk go test -race ./internal/task/service -run 'Discovery|DesktopRoot|Cache'
```

## Files likely touched

- `apps/backend/internal/task/service/repository_discovery.go`
- New discovery cache and desktop-root service modules
- `apps/backend/internal/task/handlers/`
- `apps/backend/internal/task/dto/`
- `apps/backend/internal/task/models/`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/`

## Dependencies

- Task 01 supplies the internal desktop launch marker.

## Risks

- An incorrect Home-root comparison can scan protected descendants.
- Workspace-scoped endpoints can cause an accidental workspace-scoped store.
- An upgrade migration can remove configured discovery or start a silent scan.

## Parallelism

`sequential`

## Inputs

- REQ-WORKSPACES-LOCAL-REPOSITORIES-002 and 003.
- `repository_discovery.go` and its platform tests.
- Existing task SQLite repository and migration patterns.

## Results

Completed.

- Added launch-aware effective-root policy, install-wide desktop-root SQLite
  records, legacy Home confirmation migration, scoped root APIs, and the
  30-minute single-flight discovery cache.
- Added safe Home traversal that skips direct macOS Desktop, Documents, and
  Downloads children while keeping an explicitly selected protected root
  scannable.
- Verification passed: the complete targeted task/backend test set, discovery
  policy and SQLite tests, and race-enabled task-service discovery tests.
