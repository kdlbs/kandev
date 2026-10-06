---
created: 2026-09-26
status: done
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
system_design:
  - ../../specs/agents/system-design/harness-self-update.md
legacy_specs: []
---

# Implementation Plan: OMP Harness Updates

## Overview

Add the package-manager-independent OMP self-update command to the existing Agents settings update surface. Resolve status metadata without requiring the `npm` executable. Display upstream stable latest only as a reference, since approval runs OMP's configured channel updater and may install a different version. Preserve OMP's current launch and remote install behavior. The backend capability, status, preview, and target-free approval contract must land before the UI adds a self-update dialog variant; browser coverage and public documentation follow.

## Scope

### In scope

- Declare a trusted harness-owned update capability and register OMP's `omp update` command and metadata package.
- Reuse the existing update status, preview, streamed maintenance job, ACP re-probe, and capability publication flow.
- Show the current ACP-reported version and upstream stable latest as a reference, explicitly state that updates follow OMP's configured channel and may install a different version, and omit exact-version selection, rollback, and Kandev version persistence.
- Preserve OMP's `omp acp` launch surfaces and existing `bun install -g @oh-my-pi/pi-coding-agent` install script.

### Out of scope

- Changing OMP's runtime command, container command, passthrough command, session behavior, or `InstallScript`.
- Adding OMP to the managed npm runtime version catalogue, default-generation reconciliation, or pin-maintenance workflow.
- Kandev-owned artifact staging, package-manager fallback, exact-version install, rollback, channel switching, or Nix update support.
- Installing OMP in task Docker, Sprite, SSH, or Kubernetes environments from the Settings job.

## Technical approach

### Backend update contract and job

- Add `HarnessUpdateSpec` and `HarnessUpdateAgent` in the agents capability area. Use a trusted package only for stable release metadata and direct argv for the self-update command.
- Implement the capability on `OmpACP` with `@oh-my-pi/pi-coding-agent` and `omp update`. Do not implement `ManagedNPMRuntimeAgent`; keep `BuildCommand`, `Runtime().Cmd`, `InferenceConfig`, and `InstallScript` unchanged.
- Adapt `agent/settings/controller` discovery, status, preview, and enqueue paths to distinguish `pinned` and `self_update` modes. Fetch OMP stable metadata directly over HTTPS from the trusted npm registry without invoking `npm`; report update available only when stable latest is newer, and report unknown when it is equal to or older than the ACP-reported version because the configured channel is unknown. The preview exposes stable latest as `stable_latest_version` reference only, not `target_version`. Infer mode from built-in capability metadata and accept a target-free approval body only for self-update, rejecting a non-empty target or `use_default: true`.
- Reuse the existing job lifecycle/output stream; run the trusted `omp update` argv without selecting a version or channel, then ACP-probe the existing `omp acp` command. The installed version follows OMP's configured channel and may differ from the stable reference. Publish the actual ACP-reported version and capabilities only after probe success. Do not write a managed version selection or run npm cache repair.
- Add backend coverage for available/unknown status, direct metadata resolution with `npm` unavailable, target-free self-update approval, rejected target/default requests, command failure, unchanged-version failure, probe failure without capability publication, successful publication, actual post-update version differing from stable reference, and protection against request-supplied commands/packages.

### Settings UI

- Add `update_mode` to runtime-update/status/preview/job wire types and use a closed discriminant (`pinned`, `self_update`).
- Reuse the existing trigger and dialog/drawer. For `self_update`, show current ACP version and stable latest as a reference, hide `RuntimeVersionPicker`, and state that updates follow OMP's configured channel and may install a different version. Equal or newer stable metadata does not disable the trusted update action.
- Update the API client and approval hook to send `{}` for `self_update` approval and the existing target/default payload for pinned runtimes. Add focused client/hook tests for both request shapes.
- For any terminal `up_to_date` response with an empty `job_id`, return the DTO without shared job-store insertion, start runtime-update status refresh without awaiting it, and show/reset the result in dialog-local state. Stable metadata does not produce this result for self-update mode.
- A refresh failure must not delay or hide the terminal result and must keep the last good status map; only the latest-started status refresh may replace the map. Cover failed and out-of-order refreshes in status-hook tests.
- Coalesce pending successful-job refreshes; each request snapshots covered IDs at start, later job successes stay pending, and late/superseded IDs share one successor. Only an applied response marks the IDs in its snapshot observed.
- Add localized copy only where existing copy cannot express the self-update state. Add the new keys in all six complete locales and maintain the pseudo-locale/i18n checks.

### OMP updater behavior verified for this design

`omp update` detects Homebrew, mise, Bun, npm, or standalone-binary installation and updates through that installation method; Nix-managed installs are declined. It selects the harness's existing stable/canary channel latest, verifies the installed launcher version, and does not accept an arbitrary target version. The `stable_latest_version` from npm metadata is a display/advisory reference only; it is not passed to OMP and does not block an update when the current version is equal or newer. Kandev does not change OMP's channel. Keep the existing remote install script unchanged per the user's choice.

## ASCII UI preview

### UI-01: Desktop self-update dialog

Entry: Settings > Agents > OMP update control. Structural choices required by AC-AGENTS-RUNTIME-UPDATES-003.2 and 003.3: show current and stable latest, no version picker, primary update action, shared job output.

```text
+-------------------------------------------------------------+
| Update omp                                            [X]   |
| Review the update before applying                           |
|                                                             |
| Installed version       18.3.1                             |
| Stable latest reference 18.3.2                             |
|                                                             |
| Update follows OMP's configured channel.                    |
| Installed version may differ from stable latest.            |
|                                                             |
| Update command          omp update                          |
|                                                             |
| Job output                                                   |
| > Updated to 18.3.4                                         |
|                                                             |
| [Close]                              [Update omp]            |
+-------------------------------------------------------------+
```

Dialog header/footer stay fixed; the existing bounded body owns scrolling. The output region appears during/after a job. Stable latest remains reference-only and does not disable the self-update action. A generic terminal no-job result uses UI-03's local terminal state and refreshes status without creating or polling a job. Exact copy and spacing are illustrative and must use existing primitives and localized strings.

### UI-02: Phone self-update drawer

Entry and state: same as UI-01 at the responsive phone breakpoint. Distinct composition follows the existing bottom drawer; update action remains touch-reachable.

```text
+--------------------------------------+
| Update omp                     [X]   |
| Review the update before applying    |
|                                      |
| Installed version                   |
| 18.3.1                               |
| Stable latest reference             |
| 18.3.2                               |
| Update follows OMP's channel.        |
| Installed version may differ.        |
|                                      |
| Update command                       |
| omp update                           |
|                                      |
| Job output                           |
| > Updated to 18.3.4                  |
|                                      |
| [Update omp]                         |
+--------------------------------------+
```

The drawer header and action footer remain fixed; the center content scrolls. Touch action uses existing mobile sizing. No version picker is rendered; the local no-op result in UI-03 uses the same drawer composition.

### UI-03: Shared terminal no-job result

The shared update dialog can display a terminal `up_to_date` response with an
empty `job_id`. This result does not come from stable-version comparison in
self-update mode; the trusted updater decides whether its configured channel
has an update.

Entry: the operator approves a targetless update or repair preview, but the
backend's pre-enqueue check now reports the runtime is current.

```text
+-------------------------------------------------------------+
| Update omp                                            [X]   |
| Installed version       18.3.2                             |
| Stable latest reference 18.3.2                             |
|                                                             |
| Already up to date                                         |
|                                                             |
| [Close]                                                     |
+-------------------------------------------------------------+
```

The terminal result is local to the dialog; no job progress, output, or job
polling appears. Refresh runtime-update status for each no-job approval.


## Tests

- `apps/backend/internal/agent/agents/harness_update_test.go`: trusted OMP update capability command and package contract.
- `apps/backend/internal/agent/settings/controller/agent_update_harness_test.go`: status classification, registry resolution without `npm`, mixed-agent status isolation, preview and approval despite metadata failure, repair preview classification, updater-failure output/no-fallback assertions, unchanged-version failure, probe-failure preservation, configured-channel version recording, and a queued update that still runs after the ACP version reaches stable latest.
- `apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go`: self-update DTO and target-free request acceptance/rejected target/default requests, plus proof that stable-version equality does not suppress the configured updater.
- `apps/web/lib/agent-runtime-update.test.ts`, `apps/web/lib/api/domains/agent-update-api.test.ts`, `apps/web/components/settings/use-agent-update-dialog-state.test.ts`, and `apps/web/components/settings/agent-runtime-update-control.test.tsx`: targetless `update`/`repair` self-update actions enabled by structural operation, `up_to_date` disabled, pinned target guard, user-click approval reaching API with exact `{}`, metadata-unknown/repair states, and local display/reset of an empty-ID `up_to_date` response without job tracking.
- `apps/web/hooks/domains/settings/use-agent-runtime-updates.test.tsx` and `apps/web/app/settings/agents/page.test.tsx`: the empty-ID `up_to_date` response is returned without a shared job-store entry and every no-op approval refreshes runtime-update status.
- `apps/web/hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx` and `apps/web/app/settings/agents/page.test.tsx`: failed refresh preserves the last good map; out-of-order refreshes reject stale writes; the mixed A-job/B-late-joiner/no-job-approval failure schedule launches one successor for both pending IDs, observes both only after apply, and starts no extra request on jobs-map rerender.
- Run backend tests from `apps/backend`:

```bash
CGO_ENABLED=1 go test -tags fts5 ./internal/agent/agents ./internal/agent/settings/controller ./internal/agent/settings/handlers
```

- Run focused frontend tests from `apps/web`:

```bash
pnpm exec vitest run lib/agent-runtime-update.test.ts lib/api/domains/agent-update-api.test.ts components/settings/use-agent-update-dialog-state.test.ts components/settings/agent-runtime-update-control.test.tsx hooks/domains/settings/use-agent-runtime-updates.test.tsx hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx app/settings/agents/page.test.tsx
pnpm run typecheck

```

## E2E tests

- Desktop Chromium flow: OMP shows current version and stable latest as a reference, explains configured-channel behavior, approves with an exact `{}` request body, and displays a post-update version that differs from the stable reference. The shared dialog also covers a generic empty-ID `up_to_date` response: show the local result despite a failed status refresh, make no job request, and refresh status. Map to AC-AGENTS-RUNTIME-UPDATES-003.2, .15, and .16; backend configured-channel behavior is covered by Task 02.
- Phone `mobile-chrome` flow: same reference-only stable version and configured-channel explanation in the drawer, no picker, exact target-free body, reachable action, and a generic empty-ID `up_to_date` result with status refresh and no job polling. Map to AC-AGENTS-RUNTIME-UPDATES-003.2 and .15.

Keep the desktop and mobile flows in separate spec files: `chromium` excludes `mobile-*.spec.ts`, and `mobile-chrome` selects only those files.

- Run after building the backend and web artifacts as required by `apps/web/e2e/README.md`:

```bash
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
make -C apps/backend e2e-plugin-ui
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/settings/agent-self-update.spec.ts)
(cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/settings/mobile-agent-self-update.spec.ts)
```

## Work orders

- [x] [Task 01: Add OMP self-update capability](task-01-omp-update-capability.md)
- [x] [Task 02: Integrate harness updates into the backend pipeline](task-02-backend-self-update-pipeline.md)
- [x] [Task 03: Add the self-update Settings dialog variant](task-03-self-update-settings-ui.md)
- [x] [Task 04: Cover the Settings flow and document OMP updates](task-04-self-update-e2e-docs.md)

## Verification results

All four work orders are complete. Focused Go tests (including race-focused checks), web tests (69 focused, store and WebSocket cases), TypeScript typecheck, i18n, Go/web/harness/spec/architecture lint, backend and E2E builds, new Chromium (3) and mobile-chrome (2) browser cases, pinned-update browser regressions, public-doc validators (62 tests, 47 pages), and specification validators passed. The OMP desktop dialog and unobstructed phone drawer were visually reviewed. CI browser fixtures do not change an installed OMP executable.

The repository-wide `make test` did not pass: unrelated backend suites failed in this task worktree (task-worktree path validation, inherited startup configuration, Unix socket path length, and long-running test timeouts). The complete web suite separately ran 19,846 tests with 11 failures in unrelated tests, including an unavailable Docker bridge gateway and timeouts. A full-suite green result remains unverified; every changed backend package and the focused web/browser flows passed.

## Risks

- `omp update` modifies the installed harness before Kandev's ACP probe. If that probe fails, Kandev preserves the prior capability catalogue but cannot restore the previous executable.
- Stable metadata does not identify OMP's configured channel. Kandev does not
  change OMP's channel, and it reports an unchanged ACP version after a
  successful updater exit as a failed job.
- OMP's updater declines Nix-managed installations. The Settings job must surface this failure rather than substituting another installer.
- The update targets the Kandev host installation only; it does not prepare remote/container copies.
