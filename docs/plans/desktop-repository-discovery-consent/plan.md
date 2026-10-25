---
created: 2026-08-27
status: completed
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-004
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
legacy_specs: []
---

# Implementation Plan: Desktop Repository Discovery Consent

## Overview

Add the desktop picker boundary first. Then add backend policy and state before
the shared user interface. Add diagnostics and idle polling after the primary
prompt fix because these changes use the new operation context.

## Scope

### In scope

- Add a narrow native folder-selection boundary for the Tauri WebView.
- Add launch-aware discovery policy, install-wide desktop roots, and caching.
- Migrate repository-selection surfaces to one activation coordinator.
- Add operation-level access diagnostics.
- Complete one final workspace scan before idle polling stops.
- Update public recovery and configuration documentation.

### Out of scope

- Sign or notarize the desktop application.
- Grant Full Disk Access or guarantee consent after an unsigned update.
- Add general native filesystem authority to the SPA.
- Change repository exact-path grants or worktree placement.

## Technical approach

### Native boundary

Add `tauri-plugin-dialog` under the `desktop-runtime` feature in
`apps/desktop/src-tauri/Cargo.toml`. Register it behind one custom command.
Expose that command through `src/lib.rs`, a custom permission, and the default
capability. Add the desktop process marker in `src/backend.rs`.

### Discovery policy and state

Change `repository_discovery.go` to choose effective roots from backend launch
mode. Add install-wide desktop-root and migration records under
`internal/task/repository/sqlite`. Add root snapshots and a single-flight cache
in `internal/task/service`.

Extend the existing walk policy so a macOS Home root excludes its direct
`Desktop`, `Documents`, and `Downloads` children. Keep exact protected-folder
roots scannable.

### Client selection and activation

Add one shared discovery hook and one per-tab activation coordinator. The
coordinator combines its lease count with `document.visibilityState`.

Use the boot picker capability to select the Tauri adapter. Use backend launch
policy for effective roots. Keep the HTTP folder browser for ordinary browser
clients, including a browser on the loopback desktop backend.

The phone composition reuses the existing Add Workspace Sources `Drawer`. It
keeps one internal scroll owner, safe-area clearance, 44-pixel primary actions,
and shared discovery state with desktop.

### Diagnostics and polling

Pass an operation context through discovery, directory listing, validation,
file monitoring, and Git polling. Add bounded access-denial warnings.

Use `releaseActivity(executionActivityKey(executionID))` as the turn-completion
seam. Request one final file and Git scan, release runtime interest, and deliver
the calculated `paused` mode. Change the 60-second no-push fallback to final
scan and pause.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| AC-002.1, AC-002.2, AC-002.6, AC-002.14 | Go policy and walker cases in `repository_discovery_test.go` and platform-focused test files |
| AC-002.7 to AC-002.9, AC-002.15, AC-002.16 | Service and SQLite tests for root state, migration, restart, and workspace scope |
| AC-002.4, AC-002.5, AC-002.10, AC-002.12 | Rust command, origin, cancellation, and capability tests through the library target |
| AC-003.1 to AC-003.7 | Go cache tests and frontend coordinator tests with a fake clock and filesystem spy |
| AC-004.1 to AC-004.3 | Go structured-log and bounded-warning tests in the owning packages |
| AC-004.4, AC-004.5 | Lifecycle and agentctl transition tests with a fake clock and filesystem spy |

## E2E tests

- `tests/task/repository-discovery-consent.spec.ts` uses a stubbed native-picker
  adapter for AC-002.2 to AC-002.9, AC-002.12, AC-002.13, and AC-003.2 to
  AC-003.8.
- `tests/task/mobile-repository-discovery.spec.ts` runs in `mobile-chrome`. It
  covers AC-002.11 and the same selection, refresh, and recovery outcomes.
- The desktop `e2e` command remains a launch smoke test. Manual macOS QA covers
  `NSOpenPanel` and the privacy dialogs.

## Work orders

- [x] [Task 01: Add the desktop folder-selection boundary](task-01-desktop-folder-selection-boundary.md)
- [x] [Task 02: Add runtime-aware discovery state](task-02-runtime-aware-discovery-cache.md)
- [x] [Task 03: Update repository discovery user experience](task-03-repository-discovery-ux.md)
- [x] [Task 04: Add filesystem access diagnostics](task-04-filesystem-access-diagnostics.md)
- [x] [Task 05: Reduce idle workspace access](task-05-idle-workspace-access.md)
- [x] [Task 06: Document desktop discovery recovery](task-06-document-discovery-recovery.md)

Tasks 01 and 02 define shared contracts. Task 03 depends on both. Task 04
depends on Task 02 for operation context. Task 05 depends on Task 04. Task 06
follows the completed user and diagnostic behavior.

## Verification results

Completed on 2026-08-28.

- Desktop Rust formatting, library tests (66 passed), and feature checks pass.
- Backend targeted tests, race tests, and `make lint` pass with zero issues.
- Frontend focused Vitest tests (45 passed), typecheck, lint, and i18n checks pass.
- Desktop Chromium and mobile Chromium discovery E2E tests pass against rebuilt artifacts.
- Public documentation validation and specification lint pass.

The manual macOS NSOpenPanel, privacy-dialog, and unsigned-update checks remain
platform-dependent follow-up QA; this Linux workspace cannot execute them.

Final automated commands:

```bash
cd apps/desktop/src-tauri && rtk cargo fmt --check
cd apps/desktop/src-tauri && rtk cargo test --features desktop-runtime
cd apps/desktop/src-tauri && rtk cargo check --features desktop-runtime
cd apps/backend && rtk go test ./internal/task/... ./internal/agent/runtime/lifecycle/... ./internal/agent/runtime/agentctl/... ./internal/agentctl/server/...
cd apps/web && rtk pnpm exec vitest run components lib/desktop app/office/projects
cd apps/web && rtk pnpm run typecheck
cd apps/web && rtk pnpm run i18n:check
cd apps/web && rtk pnpm e2e:run tests/task/repository-discovery-consent.spec.ts
cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/task/mobile-repository-discovery.spec.ts
cd apps && rtk pnpm --filter @kandev/desktop e2e
rtk make -C apps/backend lint
cd apps && rtk pnpm --filter @kandev/web lint
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
```

Manual macOS QA records the operating-system version, unsigned build identity,
selected root, observed dialog, log event, and recovery result. It covers first
run, Home selection, protected-folder selection, replacement, denial, and
Reconnect. Linux and Windows checks cover picker selection and cancellation.

## Risks

- An unsigned replacement can receive a new macOS privacy identity. Reconnect
  remains a recovery action, not a permanent consent guarantee.
- A Home selection can recreate the complaint if the walker includes protected
  direct children. Platform tests must cover both Home and exact-root cases.
- A lost `paused` transition can leave 30-second polling active. Lifecycle and
  agentctl tests must prove the complete transition order.
