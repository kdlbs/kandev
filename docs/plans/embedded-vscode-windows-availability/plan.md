---
spec: docs/specs/ui/requirements/embedded-vscode-windows-availability.md
created: 2026-07-29
status: complete
---

# Implementation Plan: Embedded VS Code Windows Availability

## Overview

Expose the Kandev backend's Go runtime OS in the existing SPA boot runtime, then filter the
task-detail topbar's editor candidates with that host-owned value before resolving the saved
default or rendering menu entries. Cover the boot contract and editor fallback with targeted tests,
prove that browser platform does not control the behavior with Playwright, and document the
Windows-host boundary.

## Backend

### Boot runtime host platform

- Add `HostOS string` with JSON key `hostOS` to `webapp.RuntimeConfig` in
  `apps/backend/internal/webapp/payload.go`.
- In `apps/backend/internal/backendapp/helpers.go`, construct the web runtime config from
  `runtime.GOOS` and reuse it for both the embedded/static handler and `bootPayload`, including dev
  mode. This keeps the injected shell and `/api/v1/app-state` fallback payload consistent.
- Extend `apps/backend/internal/backendapp/helpers_test.go` to assert that the serialized boot
  runtime carries the running backend's `runtime.GOOS`.
- Do not infer the host from request headers, user agent data, the desktop WebView, or a task
  executor.

## Frontend

### Boot payload parsing

- Extend `BootRuntime` and `readRuntime` in `apps/web/src/boot-payload.ts` to accept `hostOS`.
- Add a focused `readBackendHostOS` accessor over the normalized boot payload so UI components do
  not parse the injected global themselves.
- Extend `apps/web/src/boot-payload.test.ts` to preserve valid `hostOS` values and ignore invalid
  non-string values.

### Topbar editor availability

- Add `apps/web/components/task/editors-menu-availability.ts` with focused helpers that:
  - apply the existing enabled/installed rules;
  - exclude `internal_vscode` only when the backend `hostOS` is `windows`; and
  - resolve a saved default against the filtered set, falling back to the first compatible editor.
- Update `apps/web/components/task/editors-menu.tsx` to use the boot payload's backend `hostOS` for
  both the primary action and dropdown entries.
- Keep `apps/web/lib/keyboard/utils.ts` and browser `detectPlatform()` out of this feature.
- Keep `apps/web/components/editors/file-actions-dropdown.tsx`, editor settings, layout presets,
  Dockview add-panel actions, and backend editor discovery unchanged.

### Mobile design contract

- Desktop outcome: the task-detail split-editor control remains in the desktop topbar, with the
  unsupported embedded entry removed only when the Kandev backend host is Windows.
- Mobile entry point and hierarchy: `TaskPageInner` continues to render
  `SessionMobileTopBar` instead of `TaskTopBar`; the mobile header has no editor action and its
  existing task title, repository, git, plugin, and task-switcher hierarchy remains unchanged.
- Nearest shipped exemplar: `apps/web/components/task/mobile/session-mobile-top-bar.tsx` and
  `apps/web/e2e/tests/task/mobile-task-topbar-long-title.spec.ts`.
- Presentation, scrolling, and state: no new drawer, route, scroll owner, safe-area behavior, touch
  target, or mobile-specific state is introduced.

## Tests

- **What:** the Go boot runtime exposes the backend's OS.
  **File:** `apps/backend/internal/backendapp/helpers_test.go`.
  **How:** serialize `bootPayload`, decode `runtime.hostOS`, and compare it with `runtime.GOOS`.
- **What:** the frontend boot parser preserves string `hostOS` values and rejects invalid values.
  **File:** `apps/web/src/boot-payload.test.ts`.
  **How:** focused Vitest cases exercise `readBootPayload` and the host-OS accessor.
- **What:** Windows hosts exclude `internal_vscode`; macOS, Linux, and unknown hosts retain it; all
  hosts retain compatible custom/built-in editors under existing availability rules.
  **File:** `apps/web/components/task/editors-menu-availability.test.ts`.
  **How:** table-driven Vitest cases pass explicit backend host values and mixed `EditorOption`
  fixtures to the availability helper.
- **What:** an unavailable saved embedded default falls back to the first compatible editor, and an
  empty compatible set resolves to no editor.
  **File:** `apps/web/components/task/editors-menu-availability.test.ts`.
  **How:** focused unit cases exercise the resolver with the already-filtered editor set.

No persistence or editor-service integration test applies because those contracts do not change.
The boot serialization test covers the backend boundary, and the Playwright task-detail flow below
provides full-path UI coverage.

## E2E Tests

- **Scenario:** **GIVEN** a Windows-host boot payload and a non-Windows browser, **WHEN** the user
  opens the task-detail topbar editor menu, **THEN** a compatible custom editor is visible and
  **VS Code (Embedded)** is absent.
  **File:** `apps/web/e2e/tests/task/windows-host-embedded-vscode-availability.spec.ts`.
  **What to verify:** seed a custom hosted editor, intercept the initial task document to replace
  only `runtime.hostOS` in the injected boot payload with `windows`, leave the browser platform
  unchanged, and assert the open menu without starting code-server.
- **Scenario:** **GIVEN** the real Linux E2E backend and a browser reporting Windows, **WHEN** the
  user opens the task-detail topbar editor menu, **THEN** **VS Code (Embedded)** remains visible.
  **File:** `apps/web/e2e/tests/task/windows-host-embedded-vscode-availability.spec.ts`.
  **What to verify:** override `navigator.platform` before navigation while leaving the backend
  boot payload unchanged, proving the visitor platform is ignored.
- **Scenario:** **GIVEN** a Windows-host boot payload at a phone viewport, **WHEN** task details
  render, **THEN** the intentional mobile topbar is visible, desktop editor controls are absent,
  and the document does not overflow horizontally.
  **File:** `apps/web/e2e/tests/task/mobile-windows-host-embedded-vscode-availability.spec.ts`.
  **What to verify:** use the `mobile-chrome` project and the same scoped boot-payload document
  rewrite.

Existing `apps/web/e2e/tests/settings/vscode-open-panel.spec.ts` continues to cover embedded-editor
launching on a supported non-Windows host when code-server is installed.

## Public documentation

- Update `docs/public/developer-tools.md` to explain that Kandev lacks a supported standalone
  Windows code-server installation for this integration, so the task-detail topbar does not offer
  **VS Code (Embedded)** when the Kandev backend is running on Windows, regardless of a visitor's
  browser platform, while other configured editors remain available.
- Update the **Embedded VS Code** row in `docs/public/feature-status.md` with the same host-platform
  boundary.
- Keep the wording version-independent while linking to the official code-server installation
  guidance or release information where useful.

## Implementation waves and parallel candidates

Wave 1:

- [x] [Task 01: Expose backend host OS](task-01-expose-backend-host-os.md) (`done`)

Wave 2:

- [x] [Task 02: Filter task-topbar editors](task-02-filter-task-topbar-editors.md) (`done`,
  depends on Task 01)

Wave 3:

- [x] [Task 03: Prove host-platform behavior](task-03-host-platform-e2e.md) (`done`, depends on
  Task 02)
- [x] [Task 04: Document host-platform availability](task-04-document-host-platform.md) (`done`,
  parallel-safe with Task 03; user authorization required for parallel execution)

The primary conversation executes these tasks sequentially unless the user explicitly authorizes
subagents.

## Risks

- The injected boot payload is also returned by the `/api/v1/app-state` fallback path; both sources
  must use the same backend host value.
- A saved embedded-editor default remains persisted. Filtering before default resolution is
  required so it cannot bypass the hidden dropdown entry; the fallback must not overwrite that
  cross-platform preference.
- The Windows-host E2E runs on Linux and must rewrite only the injected `runtime.hostOS` field in
  the initial document. The inverse scenario must change only the browser platform so the tests
  prove which source is authoritative.

## Verification

From the repository root:

```bash
make -C apps/backend test
```

From `apps/`:

```bash
pnpm --filter @kandev/web test -- src/boot-payload.test.ts components/task/editors-menu-availability.test.ts
pnpm --filter @kandev/web e2e:run -- tests/task/windows-host-embedded-vscode-availability.spec.ts -- --project=chromium
pnpm --filter @kandev/web e2e:run -- tests/task/mobile-windows-host-embedded-vscode-availability.spec.ts -- --project=mobile-chrome
```

From the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Recorded results

- `go test ./internal/webapp ./internal/backendapp -run 'Test.*(Boot|Runtime|HostOS)'` — passed
  (36 tests).
- `pnpm test -- src/boot-payload.test.ts components/task/editors-menu-availability.test.ts` —
  passed (15 tests).
- `pnpm run typecheck` and focused ESLint — passed.
- Desktop host-platform E2E — passed (2 tests, Chromium).
- Mobile host-platform E2E — passed (1 test, mobile-chrome).
- Public-doc tests and validator — passed (58 tests; 41 published pages).
