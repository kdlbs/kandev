---
created: 2026-09-17
status: done
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-004
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
legacy_specs: []
---

# Implementation Plan: Repository Discovery Failure Recovery

## Overview

Keep accessible repositories visible when filesystem discovery encounters denied
descendants or failed roots. First correct traversal and cache recovery. Then
expose root failures in existing repository selectors and document recovery.

## Evidence and assumption check

The v0.94.0 report shows an EPERM below Home at `Pictures/Photo Booth Library`.
The clone directory is also absent. `repoWalker.visit` propagates descendant
permission errors, and `scanRootForRepos` discards accumulated repositories.
`scanDiscoveryRoots` then replaces fresh successes with the entire previous
snapshot whenever any root fails. Server pickers hide `failed_roots`.

A temporary test established a readable repository, injected EPERM through the
real walker callback, and observed an empty service result without an API error.
The reproduction and two existing discovery tests passed. The temporary test
was removed. This Linux run did not reproduce native macOS privacy enforcement.
Commit `d796cf131d` introduced the fatal descendant-error branch.

The user requested a fix after this diagnosis. Workspace discovery owns the
contract. Existing AC-003.7 requires preservation and visible recovery.
New AC-003.9 through 003.12 clarify descendant failures and mixed root outcomes.
No unresolved product choice blocks this package.

## Scope

### In scope

- Preserve accessible siblings after descendant EACCES or EPERM.
- Retain root failures, cancellation, and structured diagnostics.
- Merge fresh and cached results by exact scan root.
- Show failed paths and Refresh in server and phone repository selectors.
- Preserve saved desktop root recovery controls.
- Update public recovery guidance during implementation.

### Out of scope

- Broaden Home exclusions, filesystem permissions, or repository grants.
- Create absent clone directories or suppress their failure reports.
- Change public API fields, root persistence, or background refresh policy.
- Change native folder selection, provider discovery, or workspace polling.

## Technical approach

`repository_discovery.go` skips inaccessible descendants while retaining root
errors. The scan forwards trigger and runtime context to existing diagnostics.
`repository_discovery_state.go` retains internal result slices by root and
deduplicates only the final response. Successful empty scans clear stale entries.
Cancellation leaves the prior snapshot unchanged.

`RepositoryDiscoveryControls` renders a localized failure region for roots
without existing saved-root recovery controls. The region uses `failedRoots`
and the existing refresh coordinator. It appears in all current consumers.
It leaves available choices usable during failure and refresh.

## ASCII UI preview

UI-01: Create Task repository selector, failed scan.
Current server behavior shows only `No repositories` and the refresh icon.

```text
Desktop picker                     Phone picker (existing surface)
+--------------------------------+ +----------------------------+
| Search repositories  [Refresh] | | Search repositories        |
| Some folders could not be      | | Some folders could not be  |
| scanned.                       | | scanned.                   |
| /Users/name/.kandev/repos       | | /Users/name/.kandev/repos   |
| [Refresh]                      | | [Refresh]                  |
| project-a                      | | project-a                  |
| project-b                      | | project-b                  |
+--------------------------------+ +----------------------------+
```

UI-01 structure is required, but copy and spacing are illustrative. Localized
warning text precedes choices. Failed paths wrap inside a bounded warning list;
the title and Refresh action remain reachable while the list scrolls. Existing
picker search, selection, dismissal, and focus return remain intact. Avoid a
second Refresh button when the existing action is clearly associated with the
warning. The action is disabled during refresh and shows progress. Recovery
removes the warning. With no choices, the warning remains above the normal
empty message.

The phone entry point remains the repository selector. Reuse its current
surface, with one scroll owner and existing safe-area clearance. The Add
Workspace Sources drawer is the inline-error exemplar. No new overlay is needed.
Use 28-pixel desktop controls and at least 44-pixel touch targets. AC-003.11
maps to desktop and mobile discovery E2E tests.

## Tests

All AC references below use the prefix `AC-WORKSPACES-LOCAL-REPOSITORIES`.

| Criteria | Evidence |
| --- | --- |
| 003.9, 004.2 | New `repository_discovery_recovery_test.go`: descendant EACCES/EPERM, readable siblings before and after, root denial, exact warning target |
| 003.7, 003.10, 003.12 | Same file: cold cache, warm cache, missing clone root across two refreshes, successful empty root, overlapping roots, all roots fail, recovery |
| 003.6 | Existing discovery concurrency tests plus race-enabled recovery tests |
| 003.11, 003.8 | `repository-discovery-controls.test.tsx`: mixed success, empty failure, manual refresh, progress, recovery, desktop controls |
| 003.4, 003.5 | `use-repository-discovery.test.ts`: failed snapshots do not auto-retry, explicit refresh still runs |

## E2E tests

- Extend `tests/task/repository-discovery-consent.spec.ts` in `chromium` with
  server partial failure, usable choices, manual refresh, and recovery.
- Extend `tests/task/mobile-repository-discovery.spec.ts` in `mobile-chrome`
  with the same outcome, long paths, touch targets, and no horizontal overflow.
- Use controlled discovery responses for deterministic UI failures. Backend
  tests prove filesystem behavior without relying on root-bypassed chmod.

## Work orders

- [x] [Task 01: Preserve repositories across scan failures](task-01-scan-recovery.md) (done)
- [x] [Task 02: Show discovery failures and recovery](task-02-picker-recovery.md) (done)

Task 02 followed Task 01. Work was completed sequentially in the primary session.
The [original consent package](../desktop-repository-discovery-consent/plan.md)
retains its historical results. This package owns the additional regressions.

## Verification results

Implementation completed on 2026-09-17. Backend recovery, frontend picker
recovery, translations, E2E coverage, and public recovery guidance are in
place.

Passed checks:

- `go test -tags fts5 ./internal/task/service -count=1`
- `go test -tags fts5 -race ./internal/task/service -run 'Discovery|DiscoverLocal|RepoWalker|ScanRoot|MacOSHome' -count=1`
- `go test -tags fts5 ./internal/common/fsdiagnostics -count=1`
- Focused frontend Vitest run: 82 tests passed across 6 files
- `pnpm run typecheck`, `pnpm run i18n:check`, and targeted ESLint
- Chromium and mobile-chrome discovery E2E suites: 3 tests passed in each
- `make build-web` and `make build-backend`
- Public-doc tests and validator, specification validation and lint, and
  `git diff --check`

Native macOS privacy enforcement was not available on this Linux host. The
  regression is covered through deterministic walk-callback injection and the
  browser flows use controlled discovery responses.

## Risks

- Overlapping roots require exact scan provenance to avoid stale duplicates.
- A successful root with a denied descendant cannot prove that missing cached
  repositories were deleted. Its fresh accessible results replace its cache.
- A missing clone directory remains visible as a warning until it exists.
- Native macOS privacy dialogs require a Mac for final platform confirmation.
- New warnings need complete translations and long-path containment.
