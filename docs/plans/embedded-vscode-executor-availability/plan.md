---
spec: docs/specs/ui/requirements/embedded-vscode-executor-availability.md
created: 2026-07-30
status: complete
---

# Implementation Plan: Embedded VS Code Executor Availability

## Overview

Replace the installation-wide backend-host check with a backend-owned capability for the active
task session. Compute the capability from the executor runtime and host platform, expose it through
the existing task-session status response, enforce it in the open-editor service, and make the
desktop topbar filter and fallback follow the active session. Remove the obsolete boot
`runtime.hostOS` field, prove the backend matrix and frontend session behavior with focused tests
and Playwright, and update public executor/editor guidance.

## Backend

### Shared executor capability

- Add a small backend-owned resolver under
  `apps/backend/internal/editors/capabilities/`.
- The resolver accepts `models.ExecutorType` and the control-plane OS rather than reading browser
  or request data. It implements the spec's explicit executor matrix:
  - Local and Worktree are supported on Linux or macOS and unsupported on Windows or an unknown
    host OS;
  - Local Docker, Remote Docker, Sprites, and SSH are supported by their currently accepted task
    platforms;
  - unknown, empty, and test-only executor types fail closed.
- Cover every executor type/OS combination in
  `apps/backend/internal/editors/capabilities/capabilities_test.go`.
- Keep the policy in this shared backend package. Neither the orchestrator, editor service, nor
  frontend may reproduce the matrix.

### Session capability contract

- Add a typed capabilities DTO to
  `apps/backend/internal/orchestrator/dto/dto.go`:

  ```text
  capabilities.embedded_vscode boolean
  ```

- Populate it in `apps/backend/internal/orchestrator/task_operations.go` while resolving the
  session executor. Authorized existing sessions always return the nested capability object;
  missing or unknown executor data yields `false`.
- Extend focused task-session status tests in
  `apps/backend/internal/orchestrator/task_operations_test.go` (or the closest existing
  status-specific test file) to assert that a known executor uses the shared resolver with
  `runtime.GOOS`, the nested boolean is serialized, and unresolved executor data is false. The
  capability-package table owns simulated Windows/non-Windows matrix coverage without adding
  production injection points.

### Open-editor enforcement

- Extend the editor service's task repository boundary in
  `apps/backend/internal/editors/service/service.go` so it can resolve the selected session's
  executor.
- Before returning an `internal://vscode` sentinel, evaluate the shared capability and return
  `ErrEditorUnavailable` when false.
- Apply the check to folder-level and file-level embedded-editor opens. Do not change other editor
  kinds.
- Extend `apps/backend/internal/editors/service/service_test.go` with supported and unsupported
  session/executor fixtures, including the saved-default path. Preserve the handlers' existing
  HTTP 409 mapping for `ErrEditorUnavailable`.

## Frontend

### Consume active-session capability

- Add the nested capability shape to `SessionStatus` in
  `apps/web/hooks/domains/session/use-session-resumption.ts`.
- Read `resumption.sessionStatus.capabilities.embedded_vscode` in
  `apps/web/components/task/task-page-inner.tsx` and pass the boolean through
  `TaskTopBar` → `TopBarRight` → `TopbarToolsGroup` → `EditorsMenu`.
- The value must be derived from the current `effectiveSessionId`; a missing status or field is
  `false`, preventing a previous session's capability from leaking across a tab switch.
- Update `apps/web/components/task/task-top-bar.test.tsx` to capture the mocked `EditorsMenu` props
  and prove the capability is forwarded.

### Filter and fallback

- Change `getAvailableTaskTopbarEditors` in
  `apps/web/components/task/editors-menu-availability.ts` to accept
  `embeddedVscodeSupported: boolean` instead of a host OS.
- Continue applying enabled and built-in installed checks first, then remove only
  `internal_vscode` when the capability is false.
- Keep `resolveTaskTopbarEditorId` operating on the filtered set so an incompatible saved default
  falls back without being persisted.
- Update `apps/web/components/task/editors-menu-availability.test.ts` with supported,
  unsupported, missing-capability, fallback, and other-editor cases.

### Retire host-wide boot metadata

- Remove `HostOS` and its construction/assertions from:
  - `apps/backend/internal/webapp/payload.go`
  - `apps/backend/internal/backendapp/helpers.go`
  - `apps/backend/internal/backendapp/helpers_test.go`
- Remove `BootRuntime.hostOS`, `readBackendHostOS`, and their tests from:
  - `apps/web/src/boot-payload.ts`
  - `apps/web/src/boot-payload.test.ts`
- Remove the now-unused Playwright boot rewrite helper
  `apps/web/e2e/helpers/boot-payload.ts`.
- Confirm there are no remaining editor-availability reads of backend host metadata.

## Mobile design contract

- Desktop outcome: the existing split editor control remains in the desktop task topbar and
  changes availability when the active session changes.
- Mobile entry point and hierarchy: `TaskPageInner` continues to render
  `SessionMobileTopBar` instead of `TaskTopBar`; no editor action, drawer, route, or touch target is
  added.
- Nearest shipped exemplar:
  `apps/web/components/task/mobile/session-mobile-top-bar.tsx`.
- Presentation and state: the current mobile title, repository, git, plugin, executor, and task
  switcher ordering remains unchanged. Capability loading introduces no mobile scroll owner,
  safe-area, or persistence change.
- Coverage: retain a phone-viewport Playwright assertion that desktop editor controls are absent
  and the document does not overflow horizontally.

## Tests

- **What:** executor and host-OS combinations produce the capability matrix.
  **File:** `apps/backend/internal/editors/capabilities/capabilities_test.go`.
  **How:** table-driven tests cover every executor type, Windows/Linux/macOS host-backed execution,
  and unknown values.
- **What:** task-session status exposes a backend-owned capability for the session executor.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go` or its nearest focused
  sibling.
  **How:** executor fixtures assert the known executor's resolver result for `runtime.GOOS`, the
  serialized nested field, and missing executor data.
- **What:** direct embedded-editor opens cannot bypass capability filtering.
  **File:** `apps/backend/internal/editors/service/service_test.go`.
  **How:** service tests select `internal_vscode` explicitly and through the saved default for
  supported and unsupported sessions, while a non-embedded editor remains unchanged.
- **What:** the topbar editor set and default fallback use capability state.
  **File:** `apps/web/components/task/editors-menu-availability.test.ts`.
  **How:** table-driven Vitest cases pass true/false and mixed `EditorOption` fixtures.
- **What:** the active-session capability reaches the menu and missing data fails closed.
  **Files:** `apps/web/components/task/task-top-bar.test.tsx` and, if needed for the
  `TaskPageInner` boundary, a focused helper test extracted beside
  `apps/web/components/task/task-page-inner.tsx`.
  **How:** assert the prop chain for supported, unsupported, and absent status data.
- **What:** boot host metadata is fully retired.
  **Files:** `apps/backend/internal/backendapp/helpers_test.go` and
  `apps/web/src/boot-payload.test.ts`.
  **How:** remove obsolete assertions and retain normalization/serialization coverage for the
  remaining runtime fields.

## E2E tests

- Replace
  `apps/web/e2e/tests/task/windows-host-embedded-vscode-availability.spec.ts` with
  `apps/web/e2e/tests/task/executor-embedded-vscode-availability.spec.ts`.
- Add a small WebSocket routing helper in
  `apps/web/e2e/helpers/session-capabilities.ts` that proxies the real socket and rewrites only the
  matching `task.session.status` response's `capabilities.embedded_vscode` value. Backend unit tests
  own the executor matrix; this helper owns the browser's response to the contract.
- **Scenario:** an unsupported active session hides **VS Code (Embedded)**, retains a hosted custom
  editor, and uses the custom editor when the saved embedded default is unavailable.
- **Scenario:** a supported active session shows **VS Code (Embedded)** even when
  `navigator.platform` reports Windows, proving browser platform is irrelevant.
- **Scenario:** switching the active session from an unsupported capability to a supported
  capability updates the menu without a page reload, if the existing session fixture can express
  the switch without broad setup. Otherwise cover the stale-session guard in focused Vitest and
  keep Playwright to the two contract cases above.
- Rename
  `apps/web/e2e/tests/task/mobile-windows-host-embedded-vscode-availability.spec.ts` to
  `apps/web/e2e/tests/task/mobile-executor-embedded-vscode-availability.spec.ts`; remove the boot
  payload rewrite and retain the compact-header/no-overflow assertions at the phone viewport.
- Existing `apps/web/e2e/tests/settings/vscode-open-panel.spec.ts` remains the supported-runtime
  launch smoke test.

## Public documentation

- Update `docs/public/developer-tools.md` to say that embedded VS Code runs in the active task
  environment. Native Windows Local/Worktree sessions are unsupported, while Linux Docker and
  supported remote executors remain available even when the Kandev app runs on Windows.
- Update the **Embedded VS Code** row in `docs/public/feature-status.md` with the executor-specific
  boundary.
- Update `docs/public/executors.md` with a concise availability matrix or cross-link if that makes
  the executor distinction easier to find.
- Keep the code-server release link and existing `--auth none` network-isolation warning.

## Implementation waves and parallel candidates

Wave 1:

- [x] [Task 01: Add and enforce the session capability](task-01-add-session-capability.md)
  (`done`)

Wave 2:

- [x] [Task 02: Wire active-session editor availability](task-02-wire-editor-availability.md)
  (`done`, depends on Task 01)

Wave 3:

- [x] [Task 03: Prove executor-specific behavior](task-03-executor-availability-e2e.md)
  (`done`, depends on Task 02)
- [x] [Task 04: Document executor-specific availability](task-04-document-executor-availability.md)
  (`done`, depends on Tasks 01 and 02; parallel-safe with Task 03 only if the user explicitly
  authorizes subagents)

The primary conversation executes these tasks sequentially unless the user explicitly authorizes
subagents.

## Risks

- `task.session.status` is asynchronous. The frontend must fail closed for embedded VS Code without
  disabling unrelated editors or retaining the previous session's capability.
- Executor type is not permanently equivalent to operating system. SSH is currently safe because
  Kandev accepts only Linux/macOS remotes; future Windows SSH support must add remote-platform-aware
  capability resolution.
- UI filtering alone is insufficient because file-level and other callers share the open-editor
  API. The editor service guard must use the same backend resolver.
- Removing `runtime.hostOS` spans Go boot serialization, TypeScript normalization, and old
  Playwright helpers. A repository-wide search is required before deletion is considered complete.
- A true capability means the runtime is supported, not that a particular download or startup will
  succeed. Existing runtime errors must remain visible.
- E2E WebSocket rewriting must preserve every unrelated frame and correlate the exact status
  request/response ID; otherwise the test could hide real connection behavior.

## Verification

From `apps/backend/`:

```bash
go test ./internal/editors/capabilities ./internal/orchestrator ./internal/editors/service ./internal/editors/handlers
```

From `apps/`:

```bash
pnpm --filter @kandev/web test -- src/boot-payload.test.ts components/task/editors-menu-availability.test.ts components/task/task-top-bar.test.tsx
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint
pnpm --filter @kandev/web e2e:run -- tests/task/executor-embedded-vscode-availability.spec.ts -- --project=chromium
pnpm --filter @kandev/web e2e:run -- tests/task/mobile-executor-embedded-vscode-availability.spec.ts -- --project=mobile-chrome
```

From the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Implementation records the red/green results in each task file, updates task/plan statuses, and
moves the spec from `approved` to `building` at start and `shipped` only after every acceptance
scenario and verification command passes.

## Completion record

All four tasks completed on 2026-07-30. Backend capability and enforcement tests, focused web
tests, typecheck, lint, both targeted Playwright suites, and public-doc validation passed. See the
individual task records for exact commands and results.
