---
created: 2026-10-02
status: done
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-001
  - REQ-SYSTEM-PAGE-GO-CACHE-004
  - REQ-SYSTEM-PAGE-GO-CACHE-005
system_design:
  - ../../specs/system-page/system-design/go-cache-reclamation.md
legacy_specs: []
---

# Implementation plan: Go cache reclamation

## Overview

Keep one shared cache, delete build data without quarantine, and add an explicit
setting that permits cleanup during active tasks. The user accepts possible build
failures in that mode. No consumer tracking or generation system is required.

Baseline: `504113548c8`. Preserve PR #4153's optional-cache launch fallback.
This revised package replaces the unimplemented generation proposal.
All four work orders are complete. PR #4160's trimpath changes were integrated
before final verification; the storage feedback work proceeded after Task 02.

## Inputs and ownership

- [Requirements](../../specs/system-page/requirements/go-cache-reclamation.md)
- [Design](../../specs/system-page/system-design/go-cache-reclamation.md)
- [Decision](../../decisions/2026-10-02-direct-go-cache-reclamation.md)
- [Existing fallback](../../specs/system-page/system-design/managed-go-cache-launch-fallback.md)

System-page owns cache policy, settings, and results. Repository build entry
points supply consistent compiler flags. Execution lifecycle does not change.

## Scope

### In scope

- Direct deletion in the existing cache root, preserving ownership safeguards.
- Persisted `go_cache.allow_cleanup_while_busy`, default false.
- Go-only busy admission for scheduled and manual cleanup, including preflight.
- Shared-cache compatibility, integration with PR #4160, localized desktop/phone feedback, and operations documentation.

### Out of scope

Generations, consumer leases, lifecycle changes, new database tables, hard quotas,
external cache services, automatic retries, and other resource cleanup policies.

## Technical approach

Replace `gocache.Provider` quarantine rotation with bounded content deletion.
Preserve the root and marker. Historical quarantine remains on its existing path.
Add the boolean through existing storage settings JSON and web types.

Update both `Operations.RunNow` preflight and `Runner.Run`. An opted-in Go phase
uses a shared storage-mutation mutex, not the global task activity lease.
All runner instances and conflicting adoption/Go quarantine operations share it.
Other providers still acquire normal maintenance admission. New tasks cannot
cancel the opted-in Go phase; shutdown and explicit cancellation still can.

Persist partial results when the rest of a mixed run is busy. Extend results
additively, keep old run JSON readable, and do not claim deleted bytes from a
before-minus-after measurement during concurrent writes.

PR [#4160](https://github.com/kdlbs/kandev/pull/4160) merged at
`341f8941376e29f21b0873ae94a989cf2a595c58`; the workspace was integrated onto
`origin/main` at `14d473d3e0cc747ab3cb9e8455da812a665d6b90`. Backend Make recipes
apply `-trimpath` directly to build and test commands. Backend CI also sets the
flag, and `scripts/go-cache-reuse.test.sh` verifies cross-worktree reuse and
source invalidation. Duplicate local Make conventions and fixture helpers were
removed during integration. No host-global Go settings or unrelated repository
flags were changed.

| Case | Behavior | Evidence |
| --- | --- | --- |
| Owned cache, busy option off | Existing idle/manual admission; direct deletion | Provider and runner regressions |
| Owned cache, busy option on | Delete despite task activity; builds can fail | Concurrent writer and active-task integration |
| Explicitly adopted cache | Same selected policy; external builds can also fail | Adoption/path sentinel tests |
| Unadopted user/default cache | Read-only analysis | Ownership rejection tests |
| Remote/container cache | Existing executor-local behavior | Existing lifecycle fallback tests |
| Historical quarantine | Existing retention and restore | Existing controller tests |
| Other cleanup providers | Existing admission and force contracts | Mixed-run runner tests |

## ASCII UI preview

UI-01: Settings > System > Storage > Go build cache.

```text
Managed Go cache                         [On]
Cache cleanup threshold (GB)             [15]
Allow cleanup while tasks are running    [Off]
Active builds may fail and need a retry.
Cache data is deleted without quarantine.
```

Desktop retains the existing label/switch row. Phone places the warning under
the label and keeps the switch reachable within the card. Both use the page
scroll owner, wrapped text, touch help, and existing responsive controls.
Phone targets are at least 44 pixels; desktop retains compact sizing.
The warning remains visible when the setting is off. This is a policy switch,
not a confirmation dialog. Labels and warning placement are structural; spacing is illustrative.

UI-02: Result in the existing Go row and run history.

```text
Removed: 6 GB
Remaining: 2 GB (partial measurement)
Go cleanup completed. Other cleanup skipped: tasks are running.
```

Show fields inline on desktop or stacked on phones. Use a partial-failure message
when deletion stops early. Never substitute zero for an unknown measurement.
Map both views to `AC-SYSTEM-PAGE-GO-CACHE-005.1` through `.5`.

## Tests

New test names below are planned evidence.

| Acceptance | Test location and coverage |
| --- | --- |
| `001.1`, `001.2`, `001.4` | Existing lifecycle managed-cache/fallback tests and script environment tests |
| `001.3` | PR #4160 build/fixture evidence plus Task 03 integration audit; retain a local reuse probe only if it adds missing durable coverage |
| `004.1`, `004.7` through `.10` | `gocache/direct_cleanup_test.go`: no quarantine, sentinel safety, partial failure, bounded concurrent writes |
| `004.11`, `004.12` | `gocache/direct_cleanup_safety_test.go`: fuzz preservation/accounting, same-device mount boundary, resumable threshold discovery and deletion progress |
| `004.2`, `004.3`, `004.5`, `004.6` | `settings_test.go`, `operations_test.go`: persisted default, preflight and trigger matrix |
| `004.4`, `004.7`, `004.8` | `runner_go_cache_test.go`, backendapp integration: task activity cannot starve opted-in cleanup; other providers stay gated |
| `005.1` through `.5` | Storage component/hook tests and desktop/phone E2E |

## E2E tests

Extend `tests/system/storage-maintenance.spec.ts` for chromium and
`tests/system/mobile-storage-maintenance.spec.ts` for mobile-chrome.
Save the switch, reload, keep a mock task active, invoke Go cleanup, and prove
real file deletion without quarantine or repeated force confirmation.
Then disable the switch and prove the ordinary busy state returns.
Verify warning visibility, partial results, and existing administrator restrictions.
Use isolated files and settings; restore the fixture state after each test.
Run managed production builds and desktop/phone commands sequentially.

## Work orders

- [x] [Task 01: Direct cache deletion](task-01-direct-deletion.md)
- [x] [Task 02: Optional cleanup during active tasks](task-02-busy-cleanup.md)
- [x] [Task 03: Integrate external trimpath work](task-03-trimmed-builds.md)
- [x] [Task 04: Storage policy and feedback](task-04-storage-feedback.md)

## Related packages

Preserve [fallback](../managed-go-cache-launch-fallback/plan.md) regression evidence.
The [quarantine repair](../go-cache-quarantine-lifecycle/plan.md) remains relevant
for historical entries. The [original maintenance package](../storage-maintenance/plan.md)
remains the baseline for non-Go providers. Do not alter their historical results.

## Verification results

Implementation and final checks passed on 2026-10-02:

- `make -C apps/backend build` and `make -C apps/backend test`.
- `go test -trimpath -race -tags fts5 ./internal/system/storage/... -count=1`.
- Targeted backendapp storage/cache integration tests and Linux race tests for cache safety.
- Darwin arm64 and Windows amd64 cache-package cross-compilation.
- `make -C apps/backend check-make-shells`, `bash scripts/go-cache-reuse.test.sh`, and `bash scripts/release/runtime-bundle.test.sh`.
- Web storage/component suite: 142 tests; desktop storage E2E: 10 passed; mobile storage E2E: 7 passed.
- Web typecheck, changed-file ESLint, Prettier, `i18n:check`, and `i18n:ratchet`.
- Public documentation tests and validator, specification catalog validation, full specification lint, and `git diff --check`.

All four work orders record their results. Requirements are active and the
system design is current. The implementation does not modify live settings or
delete storage during development.

## Risks

- Busy deletion intentionally permits build failures; Kandev does not retry tasks automatically.
- Concurrent writes and open files can reduce or delay actual disk recovery.
- Bounded deletion can require later passes for a large cache.
- `-trimpath` can expose fixture lookup and debugger source-path assumptions.
- The option must not bypass ownership checks or leak into other cleanup providers.
