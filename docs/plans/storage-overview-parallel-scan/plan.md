---
spec: docs/specs/system-page/requirements/storage-overview-parallel-scan.md
created: 2026-08-08
status: completed
---

# Implementation Plan: Faster storage analysis, capacity visibility, and dependency cleanup

## Overview

Deliver three related Storage maintenance improvements: overlap the four expensive overview
measurements, expose a lightweight filesystem-capacity endpoint and accessible progress card, and
add an opt-in, ownership-aware dependency-directory cleanup provider for archived/deleted task
workspaces. Preserve existing overview contracts, per-source error degradation, process-local
overview cache behavior, progressive page loading, and activity-coordinator idle semantics.

## Backend design

- Refactor the overview composition in
  `apps/backend/internal/backendapp/storage_maintenance.go` around a small measurement seam that
  can run workspace, Go-cache, quarantine, and Docker analysis concurrently.
- Use explicit result slots or equivalent ownership-safe state so each measurement writes only its
  own result and error before the parent goroutine assembles `Summary`.
- Wait for all measurements rather than cancelling siblings on an expected provider error. Preserve
  the current behavior where unavailable sources are represented in the returned summary and other
  sources still contribute their data.
- Keep settings loading before the measurement fan-out because provider construction and configured
  paths depend on that snapshot.
- Keep disk capacity outside the overview cache so the capacity card is cheap and can refresh
  independently while a cold scan is pending. Reuse the existing cross-platform statfs/free-space
  semantics in `apps/backend/internal/system/metrics` through a small capacity result seam rather
  than duplicating platform-specific calculations.
- Add a `GET /api/v1/system/storage/disk` handler that measures the filesystem containing the
  Kandev storage root and returns total, used, available, percentage, path, and availability/error
  fields. A capacity failure must not fail the overview or other Storage sections.

### Workspace dependency cleanup

- Add a persisted, default-off `workspaces.dependency_cleanup_enabled` setting and surface it in the
  Workspaces policy card with localized warning/help copy. Always display the exact fixed allowlist
  in a code-styled disclosure labeled as folders Kandev will check; pair it with the existing
  `StorageSettingHelp` information control so hover, keyboard focus, and tap expose the same
  explanation.
- Add a `workspace_dependencies` cleanup provider and resource-specific action. It uses the same
  maintenance lease, busy response, Run anyway behavior, cancellation, retry, and run-history
  result shape as other destructive providers.
- Reuse the workspace provider's fail-closed ownership, authoritative inventory, path containment,
  and no-follow-symlink checks. Introduce a small eligibility source if needed to distinguish
  archived task rows from deleted tasks whose ownership marker remains on disk.
- Prune only the fixed allowlist from the spec. Count directories and reclaimed bytes, preserve
  source/Git/recovery metadata, skip protected or state-changing workspaces, and make the operation
  idempotent so retrying after interruption is safe.
- Keep full workspace orphan grace and quarantine retention unchanged. Dependency pruning is a
  separate provider and must never be implied by the disk percentage or by read-only Analyze.
- Coordinate pruning with quarantine restore and task reactivation so a restore/unarchive cannot
  race a dependency deletion. Revalidate ownership and task state immediately before each target
  removal; fail closed on incomplete inventory.

## Tests

- Add a deterministic barrier-based backend test for the overview fan-out. It must prove all four
  measurements enter before release, then assert the assembled summary and independent errors.
- Retain or update composition fakes only as needed to inject blocking providers; avoid tests that
  depend on `time.Sleep` or host filesystem size.
- Add capacity provider/handler tests for byte math, clamping, unavailable filesystems, timeout or
  cancellation, and the selected storage-root path. Extend API fixtures for the new disk response.
- Add workspace-provider tests for allowlist-only deletion, archived/deleted eligibility, active
  inventory protection, symlink/path escape rejection, byte/count results, cancellation, retry,
  restore-after-prune, and default-off settings validation.
- Run the existing cache, handler, operations, scheduler, and activity-coordinator suites to guard
  the unchanged contracts.
- Extend desktop and mobile Storage E2E coverage for the independent capacity card, the progress
  bar's accessible percentage, the new policy option, visible allowlisted-folder names, the
  hover/focus/tap help explanation, and the workspace dependency action. Keep the page one-column on
  mobile and verify no horizontal overflow.

## Observability and rollout

- When a compatible post-change runtime is available, use the existing backend request-duration
  logs to capture a comparable cold overview before and after implementation. The cache must be
  cold for both measurements and the host dataset/settings must be held constant. If the running
  runtime predates the capacity route, record the baseline and deterministic/managed E2E evidence
  instead of claiming a comparable post-change timing.
- Capture disk-capacity response timing separately to confirm it is available before the cold
  overview completes.
- Record the measured result in the implementation task handoff. If workspace traversal remains
  the dominant cost, open a separate bounded directory-traversal optimization rather than expanding
  this change.

## Implementation waves

Wave 1:

- [x] [Task 01: Parallelize overview measurements](task-01-parallelize-overview-measurements.md)

Wave 2:

- [x] [Task 03: Add disk capacity and progress card](task-03-disk-capacity-progress.md)
- [x] [Task 04: Add opt-in dependency cleanup](task-04-workspace-dependency-cleanup.md)

Wave 3:

- [x] [Task 02: Integrated regression and timing validation](task-02-regression-and-timing-validation.md)

## Implementation results

- Overview measurements now fan out after settings load. A barrier-based test proves workspace,
  Go-cache, quarantine, and Docker measurements all enter before release; the backend package and
  storage packages pass with `-race`.
- `GET /api/v1/system/storage/disk` reports filesystem total, used, available, percentage, path,
  and safe unavailable states. The page loads it independently, renders the accessible progress
  bar, and applies 80% warning/90% critical presentation thresholds.
- Workspace dependency cleanup is default-off, uses the exact 12-entry allowlist, requires a
  complete authoritative task inventory, targets only archived/deleted unprotected Kandev-owned
  roots, and revalidates ownership, containment, symlink safety, and eligibility before each
  deletion. Results report affected workspaces, directories, bytes, and warnings.
- Focused backend tests, the backend race suite, backend lint, web typecheck/lint/i18n checks, 101 focused web
  tests, public-doc validation, and managed Chromium/mobile Storage E2E flows pass. The mobile
  flow verifies the allowlist disclosure through touch interaction without horizontal overflow;
  the workspace suite also verifies exact allowlist-copy behavior and restore after dependency
  pruning, and the Windows metrics package compiles.
- The captured pre-change live baseline remains approximately 103 seconds cold and effectively
  instantaneous warm because of the 15-minute overview cache. The post-change managed E2E holds
  the overview response and confirms the disk card renders independently before it is released;
  a same-dataset live timing comparison was not taken against the currently running pre-change
  backend (its disk route returned 404). Workspace traversal remains the dominant optimization
  risk to monitor after rollout.
- Idle behavior is unchanged: scheduled cleanup still requires the existing quiet period and
  blocks on active execution/shell/setup/cleanup/Docker-build resources; manual Analyze remains
  read-only and not idle-gated.

## Risks

- Goroutine-owned result state must be race-free; run the focused backend suite with `-race`.
- A provider that blocks or ignores cancellation can still determine request duration; the change
  removes top-level serialization but does not alter provider traversal behavior.
- A refactor that accidentally wraps each provider in a separate cache or changes error joining
  could regress warm reads or partial summaries; preserve the existing cache and response tests.
- Disk usage can be confused with Kandev-counted bytes; label the progress card as the filesystem
  containing Kandev storage and show raw total/available values so the distinction is explicit.
- Dependency names such as `vendor`, `target`, and `.cache` are ambiguous; keep them out of the
  initial fixed allowlist and make skipped/warned paths visible in the run result.
- Dependency pruning can race restore, unarchive, or a newly admitted task. Use the existing
  maintenance admission path plus per-target revalidation and fail closed on any state change.
- The activity gate covers instrumented runtime work. Workspace reconciliation that occurs before an
  execution activity lease is acquired remains outside this change and should be audited separately
  if it becomes a demonstrated idle-gate gap.
