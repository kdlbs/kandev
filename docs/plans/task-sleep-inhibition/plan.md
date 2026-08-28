---
spec: docs/specs/platform/requirements/task-sleep-inhibition.md
created: 2026-08-04
status: done
---

# Implementation Plan: Task Sleep Inhibition

## Overview

Add an install-wide, disabled-by-default setting that drives one backend-owned native sleep-inhibition lease whenever the authoritative session repository contains a `STARTING` or `RUNNING` session. Build the event-driven lifecycle and native OS adapters first, expose them through the existing System settings API and composition, then add the Task Actions card, focused desktop/mobile E2E coverage, and public operational documentation.

---

## Backend

### Sleep-inhibition lifecycle

Create `apps/backend/internal/system/sleepinhibition/` with:

- `types.go`: `Settings`, `Status`, `Response`, stable issue codes, `SessionReader`, `Inhibitor`, and `Lease` contracts. `Lease` must expose release and unexpected-termination notification so a dead native request cannot remain reported as active.
- `store.go`: a typed adapter over `internal/system/settings.Store` using key `task_sleep_inhibition`; an absent value resolves to `{enabled:false}` and malformed persisted JSON returns a typed error that the service logs and treats as disabled.
- `service.go`: one owned reconciliation loop with explicit `Start(ctx)` / `Stop()` and `sync.WaitGroup` drain. Subscribe to `events.TaskSessionStateChanged`, coalesce event signals through a buffered channel, and periodically reconcile to repair missed events and retry failed acquisitions. Each reconcile loads the configured setting, calls `ListActiveTaskSessions`, filters through `orchestrator/sessionstate.IsWorking`, and transitions a single shared lease. `Update` persists and synchronously reconciles before returning the response. Shutdown always releases the lease.

The loop, rather than event-payload ordering, owns the truth: session events are only wakeups; the repository is authoritative. A failed settings/session read leaves the last successfully reconciled lease unchanged. A failed native acquire leaves tasks unaffected, records a stable issue code plus structured warning, and retries without a tight loop.

### Native platform implementations

Add build-tagged adapters behind `NewPlatformInhibitor`:

- `inhibitor_darwin.go`: start `/usr/bin/caffeinate -i -w <kandev-pid>` without a shell. `-i` prevents idle system sleep while allowing display sleep, and `-w` bounds the helper to the backend process. Release terminates and joins the child; unexpected exit closes `Lease.Done()` with an error.
- `inhibitor_windows.go`: call `SetThreadExecutionState(ES_CONTINUOUS | ES_SYSTEM_REQUIRED)` through `golang.org/x/sys/windows` on a goroutine locked to one OS thread. Clear on the same thread with `SetThreadExecutionState(ES_CONTINUOUS)`, then join it. Do not request `ES_DISPLAY_REQUIRED` or away mode.
- `inhibitor_linux.go`: use `github.com/godbus/dbus/v5` to call `org.freedesktop.login1.Manager.Inhibit("sleep", "Kandev", "A Kandev task is running", "block")`; retain the returned Unix file descriptor for the lease and close it on release. Missing/denied system bus or logind maps to `system_service_unavailable` or `request_failed`.
- `inhibitor_other.go`: report `unsupported_platform` without side effects.

Inject OS-call/process seams so lifecycle behavior is unit-testable on the host CI platform. Add compile-only checks for Darwin and Windows implementations.

### System settings API and application wiring

Extend `apps/backend/internal/system/system.go`:

- Add the task-session reader to `system.Wiring` and a `SleepInhibition *sleepinhibition.Service` field.
- Construct the service from the shared install-wide settings store and event bus.
- Start it in `StartBackground` after the orchestrator has completed startup recovery, and stop/join it from `StopBackground`.
- Register `GET /api/v1/system/sleep-inhibition` on the authenticated System group and `PATCH /api/v1/system/sleep-inhibition` on its admin subgroup.

Pass `repos.Task` through `apps/backend/internal/backendapp/main.go`. Add handler validation and the same read-vs-admin permission coverage used by `queuesettings` in `apps/backend/internal/system/system_routes_test.go`. A supported/active status is runtime observation only and must not become another persisted or environment-controlled setting.

---

## Frontend

### Task Actions setting card

Create `apps/web/components/settings/sleep-inhibition-settings.tsx` and mount it in `TaskActionsSettings` in `apps/web/components/settings/general-settings.tsx`.

- Fetch and patch the dedicated System endpoint through `apps/web/lib/api/domains/settings-api.ts`; define the wire types in `apps/web/lib/types/system.ts`.
- Use a local saved/draft snapshot and `useSettingsSaveContributor`; do not mutate the shared user-settings slice because this preference is install-wide.
- Only admins can edit. Preserve the configured value on unsupported hosts and show localized status for available/inactive, active, unsupported platform, unavailable system service, and request failure.
- Explain beside the switch that the default is off, it may consume additional battery/power, it affects only the backend host, and container/Kubernetes or always-on deployments normally leave it disabled.
- Add all copy to `apps/web/src/locales/en/settings.json` and regenerate `apps/web/src/locales/pseudo/settings.json`.

The existing Task Actions page and `SettingsCard` are the desktop and mobile entry points. This is an inline card with the page's existing single scroll owner; it needs no new drawer, route, or viewport branch. Reuse the existing `min-h-11` label/switch row, keep the status/caveat text wrapping, and let the shared floating save control own persistence.

---

## Tests

- **What:** absent settings default off; valid values round-trip; malformed persisted data degrades to disabled.
  - **File:** `apps/backend/internal/system/sleepinhibition/store_test.go`
  - **How:** table-driven tests with the real System settings store on a temporary SQLite database.
- **What:** disabled sessions never acquire; the first `STARTING`/`RUNNING` session acquires once; multiple working sessions share the lease; the last settled session, setting disable, and shutdown release it.
  - **File:** `apps/backend/internal/system/sleepinhibition/service_test.go`
  - **How:** injected fake session reader/inhibitor and `testing/synctest` or explicit channels; include startup reconciliation, event coalescing, periodic repair, unexpected lease exit, acquisition failure/retry, and read-error behavior.
- **What:** native adapters request only system-sleep inhibition and release their owned resource.
  - **Files:** `apps/backend/internal/system/sleepinhibition/inhibitor_linux_test.go` plus build-tagged Darwin/Windows tests where host-independent seams permit.
  - **How:** fake D-Bus caller/process/syscall seams; compile-only Darwin/Windows package checks. Do not attempt to suspend a CI host.
- **What:** GET is readable, PATCH is admin-only, malformed requests fail, and PATCH returns reconciled configured/runtime status.
  - **Files:** `apps/backend/internal/system/sleepinhibition/handler_test.go`, `apps/backend/internal/system/system_routes_test.go`
  - **How:** Gin handler/service tests plus the existing auth route harness.
- **What:** the card participates in shared manual save, preserves remote snapshots, disables mutation for members, renders capability/failure states, and rejects failed saves.
  - **File:** `apps/web/components/settings/sleep-inhibition-settings.test.tsx`
  - **How:** Vitest/Testing Library with mocked API and settings-save contributor.

## E2E Tests

- **Scenario:** GIVEN the install-wide setting is at its default, WHEN an admin toggles it on, THEN the backend remains unchanged until **Save changes**, the saved value survives reload, and teardown restores the prior value.
  - **File:** `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
  - **What to verify:** visible host/container caveat, dirty card and floating save state, persisted `settings.enabled`, and configured value after reload. Native host inhibition remains covered by injected backend tests.
- **Scenario:** GIVEN a phone viewport on Task Actions, WHEN the admin edits the setting, THEN the full card, status, switch, and save action remain reachable without horizontal overflow or overlap.
  - **File:** `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`
  - **What to verify:** same saved outcome, 44px control-row geometry, card/save clearance, and document containment under the `mobile-chrome` project.

## Public documentation

Add a concise how-to/reference section to `docs/public/operations.md` covering where to enable the setting, its disabled default, `STARTING`/`RUNNING` boundary, display/user-sleep exclusions, macOS/Windows/Linux support, and why Docker/Kubernetes/remote executors cannot inhibit their physical hosts. Keep the settings card self-contained; the public page owns operational troubleshooting and deployment guidance.

---

## Verification Results

- Backend lifecycle, native adapters, and system wiring passed:
  `cd apps/backend && go test ./internal/system/sleepinhibition -count=1`,
  `cd apps/backend && go test -race ./internal/system/sleepinhibition -count=1`,
  and `cd apps/backend && go test ./internal/system/... ./internal/backendapp -run
  'Test.*(SleepInhibition|SystemRoutes)' -count=1` (19 packages).
- Darwin and Windows compile-only artifacts passed with `CGO_ENABLED=0 GOOS=darwin/windows
  GOARCH=amd64 go test -c`; both artifacts were written to a temporary directory outside
  the repository.
- Web unit tests passed (2 files, 7 tests), targeted ESLint passed with zero warnings,
  TypeScript reported no errors, and i18n pseudo/check/ratchet validation passed (four
  pre-existing orphan catalog entries remain).
- Managed production-build E2E passed for the desktop manual-save scenario and the
  `mobile-chrome` containment scenario (1 test each). Both tests restore the initial
  install-wide setting in `finally` blocks.
- Fresh synthetic screenshots were captured, inspected, and compressed at
  `apps/web/.pr-assets/settings-manual-save--sleep-inhibition-desktop-draft.png` and
  `apps/web/.pr-assets/mobile-general-settings--sleep-inhibition-mobile-draft.png`; the
  ignored asset manifest contains both entries and no credentials or personal data.
- Public documentation validation passed: 58 validator tests and
  `Validated 41 published docs pages.`
- PR review remediation passed: lease completions now carry a generation, Linux
  leases monitor logind descriptor loss, Darwin release normalizes intentional
  termination and has build-tagged lifecycle tests, and the web card reads the
  install-wide response from the settings-domain store with bounded polling.
- Focused remediation checks passed: `go test -race
  ./internal/system/sleepinhibition` (17 tests), Darwin and Windows compile-only
  checks with `CGO_ENABLED=0 GOOS=darwin/windows GOARCH=amd64 go test -c`, and
  Vitest for the sleep-inhibition card plus settings slice (9 tests), including
  remote-status refresh without draft replacement.
- `pnpm run typecheck`, targeted ESLint, `pnpm run i18n:check`,
  `pnpm run i18n:ratchet`, and `git diff --check` passed. The branch was merged
  with current `origin/main`; `apps/web/src/locales/zh-cn/chat.json` retains one
  valid attachment catalog entry for each key.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-lifecycle-service](task-01-lifecycle-service.md)

Wave 2:

- [x] [task-02-native-platform-inhibitors](task-02-native-platform-inhibitors.md)

Wave 3:

- [x] [task-03-system-api-wiring](task-03-system-api-wiring.md)

Wave 4:

- [x] [task-04-task-actions-card](task-04-task-actions-card.md)
- [x] [task-06-public-documentation](task-06-public-documentation.md) — parallel-safe with task 04 because it touches only `docs/public/operations.md`; user authorization is still required for subagents.

Wave 5:

- [x] [task-05-settings-e2e](task-05-settings-e2e.md)

The default execution order is sequential in the primary conversation. Wave labels do not authorize subagents.

## Open risks

- Linux support depends on access to the host system D-Bus and `systemd-logind`; containers and some non-systemd distributions will correctly report unavailable rather than controlling the node.
- Windows execution-state requirements are thread-owned, so acquisition and clearing must remain on the same locked OS thread.
- macOS helper lifecycle must be fully joined on disable and shutdown so repeated task runs do not leak `caffeinate` processes.
- Session-state publication has had historical gaps; the repository-backed periodic reconciliation is required to avoid holding a power assertion indefinitely after a missed settle event.
