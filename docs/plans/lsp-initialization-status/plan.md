---
spec: docs/specs/platform/requirements/lsp-file-intelligence.md
created: 2026-07-28
status: completed
---

# Implementation Plan: LSP Initialization Status and Placement

## Overview

Repair the misleading infinite initialization presentation without imposing a timeout on legitimate cold Gradle imports. First make the launched-process and long-running initialize states testable, then add the portable placement contract through the existing user-settings path, reuse the active editor's status surface in the application status bar, and finish with production-build desktop/tablet/phone E2E and public documentation.

## Confirmed root cause

`agentctl` emits its `ready` bridge status immediately after `StartPipedProcess` succeeds. The browser then records `initializingSince` and awaits `JsonRpcConnection.sendRequest("initialize", ...)`, which intentionally has no timeout. Kotlin LSP performs Gradle project import during that request and can take minutes or fail to return for some multi-module projects, so the process is alive while Kandev remains in `starting`. The current “Preparing project…” copy does not expose that distinction and never promotes a long-running wait into actionable guidance.

Smallest deterministic reproduction: install the existing fake Kotlin server with `holdInitialize: true`, open a Kotlin file, start LSP, and never create the release sentinel. The task-host process starts and receives `initialize`, while the browser remains in `starting` indefinitely.

## Backend

### Portable LSP status location

- Add `LspStatusLocation string` to `apps/backend/internal/user/models/models.go`, with `toolbar` and `status_bar` constants plus normalization to `toolbar`.
- Carry `lsp_status_location` through response and pointer-valued PATCH DTOs in `apps/backend/internal/user/dto/dto.go`, controller/service request mapping, `applyBasicSettings`, and `user.settings.updated`.
- Persist the value inside the existing user-settings JSON in `apps/backend/internal/user/store/sqlite.go`; no relational migration or endpoint is needed.
- Map it into the Go-served SPA boot state as `lspStatusLocation` in `apps/backend/internal/backendapp/boot_state_routes.go`.

## Frontend

### Honest initialization stages

- Extend `apps/web/lib/lsp/lsp-progress-view.ts` with a 60-second long-running threshold and pure presentation state that distinguishes “process started, initialize pending” from LSP readiness.
- Preserve the live connection and Stop action at every duration. Kotlin-specific copy may name Gradle import as a possible cause but must not claim indexing, percentage, completion, or ETA.
- Refactor `apps/web/components/editors/lsp-status-button.tsx` only as needed to share the same details and summary between toolbar and status-bar triggers.

### User setting and effective fallback

- Add the camel-case `lspStatusLocation` value to the settings slice, SSR/boot mapping, WS update handler, API types, carry-forward helpers, and Editors draft/save baseline.
- Add a self-documenting placement choice to Settings > Editors. `toolbar` is the default. `status_bar` explains that it requires the opt-in Application status bar and a fine pointer; unsupported runtime conditions use the toolbar without rewriting the saved preference.
- Add a pure placement helper so the toolbar and status item cannot disagree about the effective surface.

### Active-editor application status item

- Add `builtin:lsp` to the existing opaque, reorderable application status items.
- Derive the item from `activeSessionId`, the active Dockview panel and loaded text buffer, and `getMonacoLanguage`; loading, static/binary, diff, unsupported, and non-file panels render no item even when the saved editor provider is Monaco.
- Reuse the browser-owned `(session, language)` LSP connection and lifecycle control. The compact bar summary shows the active language plus readiness, elapsed initialization, or server percentage when available; opening it uses the same anchored details popover.
- Do not mount a phone LSP lease. Coarse-pointer Monaco layouts keep the existing touch-sized toolbar trigger and bottom drawer.

## Tests

- **Long-running initialize presentation:** `apps/web/lib/lsp/lsp-progress-view.test.ts` covers just below and at 60 seconds, Kotlin Gradle guidance, elapsed time, no ETA, and unchanged lifecycle action.
- **Placement resolution:** a focused pure-helper test covers default toolbar, enabled fine-pointer status bar, feature-disabled fallback, coarse-pointer fallback, and phone exclusion.
- **Portable backend contract:** focused tests in `apps/backend/internal/user/{dto,service,store}` cover default normalization, valid round-trip, invalid PATCH rejection, omission semantics, and event payload.
- **Boot and client hydration:** `apps/backend/internal/backendapp/boot_state_user_settings_test.go`, `apps/web/lib/ssr/user-settings.test.ts`, and `apps/web/lib/ws/handlers/users.test.ts` cover missing, saved, and live-updated values.
- **Editors draft:** `apps/web/components/settings/settings-dirty.test.ts` and focused settings-state tests cover dirty comparison, PATCH payload, saved baseline, and response mapping.
- **Status item:** focused app-status tests cover `builtin:lsp` ordering identity, active supported file summary, hidden loading/static/diff/unsupported/non-file states, and lifecycle disclosure reuse.

## E2E Tests

- **Slow Kotlin initialization:** extend `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts`; a held fake initialize proves the process-started stage immediately and the long-running warning after advancing browser time, with Stop still enabled and no ETA.
- **Placement persistence and behavior:** the same desktop spec changes the Editors setting to `status_bar`, saves, opens a Kotlin file, proves the toolbar control is absent, operates LSP from `builtin:lsp`, reloads, and proves the preference persists.
- **Active-file scoping:** switch from the Kotlin editor to binary Kotlin, unsupported text, and non-file panels and back; the status item hides and restores without exposing an inert lifecycle action or starting a different server.
- **Tablet fallback:** extend `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts` with saved `status_bar`; the coarse-pointer tablet still gets the 44px toolbar trigger and contained LSP drawer.
- **Phone boundary:** save `status_bar` plus Kotlin auto-start, open Kotlin in the phone viewer, and prove no LSP control, status item, WebSocket, or process appears.
- Restore all user settings changed by a spec so the worker-scoped backend does not leak preferences.

## Mobile Design Contract

- **Desktop outcome:** a fine-pointer user chooses toolbar or application status bar and opens the same active-editor LSP details.
- **Mobile entry point:** phone has none because CodeMirror remains LSP-free. Coarse-pointer tablet uses the existing Monaco toolbar control.
- **Nearest exemplars:** `AppStatusBar` provides the persistent fine-pointer item and opaque ordering; the shipped LSP tablet drawer provides the touch surface.
- **Hierarchy and action:** the bar shows language plus one live summary; disclosure shows connection stage, project work, elapsed time, and one Start/Stop/Retry action.
- **Surface rationale:** a 24px application bar suits glanceable desktop status, while the coarse-pointer bar cannot provide a 44px target, so tablet falls back to the toolbar drawer.
- **Geometry:** tablet retains one `max-h-[80dvh]` internal scroll owner, safe-area-aware shared drawer behavior, 44px trigger/actions, and no document horizontal overflow.
- **Shared logic:** status/progress snapshots, placement resolution, summary formatting, and lifecycle actions are shared; only the trigger composition differs.
- **Responsive persistence:** runtime fallbacks never overwrite `lsp_status_location`.

## Public documentation

- Update `docs/public/developer-tools.md` and `docs/public/feature-status.md` with the launched-process/initialize distinction, Kotlin Gradle-import caveat, no automatic timeout/ETA, and placement fallback behavior.
- Run both public-doc validators.

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation because the tasks share the LSP presentation contract and user-settings types.

- [x] [Task 01: Initialization stage disclosure](task-01-initialization-stage.md)
- [x] [Task 02: Portable placement setting](task-02-placement-setting.md)
- [x] [Task 03: Active-editor status-bar item](task-03-status-bar-item.md)
- [x] [Task 04: Desktop and mobile E2E plus docs](task-04-e2e-docs.md)
