---
spec: docs/specs/tasks/requirements/prevent-agent-autostart-on-open.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Prevent Agent Auto-Start On Open

## Overview

Add a per-user setting, `prevent_auto_start_agent_on_open`, that stops the web
UI from automatically starting or resuming an agent when a task is *opened* in
two situations: the post-restart recovered-idle shape (session exists, agent
process not running) and tasks sitting in the final step of their workflow.
When the setting is on, those tasks open with the agent stopped and the
existing manual start affordance (the "Start agent" button for never-started
sessions) is shown instead.

The change spans: backend settings plumbing (models/DTO/service/controller/boot
payload), a small opt-in `auto_start` override on the `session.ensure` WS
contract, frontend settings plumbing (types/SSR/store/API client), the settings
UI card, and the two open-time gates (`useEnsureTaskSession` for the final-step
no-session case, `useSessionResumption` for the resume case). Order follows the
dependency chain: backend contracts first, settings plumbing, UI, then the
gating hooks, then E2E.

---

## Backend

### 1. User settings field

- `apps/backend/internal/user/models/models.go` — add
  `PreventAutoStartAgentOnOpen bool` with json tag
  `prevent_auto_start_agent_on_open` to `UserSettings` (after
  `ConfirmTaskArchive`). No DB migration: settings are a JSON blob.
- `apps/backend/internal/user/dto/dto.go` — add the field to `UserSettingsDTO`
  (`FromUserSettings` at `:238`) and `*bool` to `UpdateUserSettingsRequest`
  (after `ConfirmTaskArchive` at `:105`).
- `apps/backend/internal/user/service/service.go` — add `PreventAutoStartAgentOnOpen *bool`
  to the service-level `UpdateUserSettingsRequest` (`:52`); apply it in
  `applyTaskActionPreferences` (`:346` block); emit it in
  `publishUserSettingsEvent` (`:773` map).
- `apps/backend/internal/user/controller/controller.go` — map
  `req.PreventAutoStartAgentOnOpen` in `UpdateUserSettings` (`:61` block).
- `apps/backend/internal/user/store/sqlite.go` — persist the field in the
  settings JSON blob: add `"prevent_auto_start_agent_on_open"` to the
  `marshalUserSettingsPayload` map (`:519-573`) and a
  `PreventAutoStartAgentOnOpen *bool` field to the `scanUserSettings` payload
  struct (`:707-760`) with the pointer-guarded assignment. Default is `false`
  (the zero value in `defaultUserSettings` at `:651`).
- `apps/backend/internal/backendapp/boot_state_routes.go` — add
  `"preventAutoStartAgentOnOpen": settings.PreventAutoStartAgentOnOpen` to the
  boot-payload map (`:459` block).

### 2. `session.ensure` `auto_start` override

- `apps/backend/internal/orchestrator/session_ensure.go` — add
  `AutoStart *bool` to `EnsureSessionOptions`; in `EnsureSession`, when
  `o.AutoStart != nil`, override the step-derived decision:
  `autoStart := stepAllowsAutoStart(step); if o.AutoStart != nil { autoStart = *o.AutoStart }`.
- `apps/backend/internal/orchestrator/session_launch.go` — add a prepare-only
  marker (e.g. `NoAgentLaunch bool`) to `LaunchSessionRequest`; `EnsureSession`
  sets it when `AutoStart == &false`, and `shouldUpgradePassthroughPrepare`
  (`:172-174`) returns false when it is set. Passthrough profiles are otherwise
  eagerly upgraded into `launchStart`, which would start the agent despite the
  override.
- `apps/backend/internal/orchestrator/handlers/handlers.go` — add
  `AutoStart *bool \`json:"auto_start,omitempty"\`` to `wsEnsureSessionRequest`
  and pass it through as `EnsureSessionOptions{AutoStart: req.AutoStart}`.
- Behavior: `auto_start: false` → `IntentPrepare` / `created_prepare` even when
  the step has `auto_start_agent`, and never upgrades to a launch (including
  passthrough profiles); absent/`true` → unchanged.

---

## Frontend

### 1. Settings plumbing

- `apps/web/lib/types/http-user-settings.ts` — add
  `prevent_auto_start_agent_on_open?: boolean` to `UserSettings` and
  `UserSettingsUpdatePayload`.
- `apps/web/lib/ssr/user-settings.ts` — add `preventAutoStartAgentOnOpen: false`
  to `createDefaultUserSettings`; hydrate in `buildBehaviorFields`
  (`s.prevent_auto_start_agent_on_open ?? current.preventAutoStartAgentOnOpen`).
- `apps/web/lib/state/slices/settings/types.ts` — add
  `preventAutoStartAgentOnOpen: boolean` to `UserSettingsState`.
- `apps/web/lib/services/session-launch-service.ts` — extend
  `ensureTaskSession` opts with `autoStart?: boolean`; include
  `auto_start: opts?.autoStart` in the `session.ensure` payload.

### 2. Settings UI

- `apps/web/components/settings/prevent-auto-start-agent-settings.tsx` (new) —
  clone the `archive-confirmation-settings.tsx` pattern (SettingsCard + Switch +
  `useSettingsSaveContributor`), persisting `prevent_auto_start_agent_on_open`.
- `apps/web/components/settings/general-settings.tsx` — render the new card in
  `TaskActionsSettings` (first slot of the task-actions section).
- `apps/web/lib/settings-discovery/catalog/preferences.ts` — new target
  `preventAutoStartOnOpen: "setting-prevent-auto-start-on-open"` on
  `GENERAL_SETTINGS_TARGETS` plus a control definition under the task-actions
  page. (The PageShell restructure #2322 moved the catalog here from
  `catalog/general.ts`.)
- i18n — add `preventAutoStartAgentOnOpen` and `preventAutoStartAgentOnOpenHelp`
  to `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json` (help text
  must use plain punctuation, no em dash).

### 3. Open-time gates

- `apps/web/hooks/domains/session/use-ensure-task-session.ts` —
  - Extend `EnsureTaskInput` with `workflowStepId?: string | null` and
    `workflowId?: string | null`.
  - Read `state.userSettings.preventAutoStartAgentOnOpen` and resolve the
    task's workflow step list workflow-aware: `state.kanban.steps` when the
    task's workflow is the active one (`state.kanban.workflowId`), otherwise
    `state.kanbanMulti.snapshots[workflowId]?.steps`. Missing workflow id or
    step list → treated as "not final" (no gate).
  - When the setting is on and the task's step is the terminal step of that
    workflow, call `ensureTaskSession(taskId, { autoStart: false })`;
    otherwise `ensureTaskSession(taskId)` as today.
  - New pure helper (exported for unit tests),
    `isFinalWorkflowStep(workflowStepId, steps)`. Terminal = max
    `(position, id)` (ties broken by max id): step positions are
    caller-supplied and not uniqueness-validated, so equal positions are
    representable and must not make both steps terminal.
- `apps/web/hooks/domains/session/use-session-resumption.ts` —
  - Read `state.userSettings.preventAutoStartAgentOnOpen` in
    `useSessionResumption` and thread a `preventAutoStart` boolean into
    `useSessionResetAndCheck` → `checkAndResume`.
  - In `checkAndResume`, when `preventAutoStart` is true, skip the
    `status.needs_resume && status.is_resumable` branch (do not call
    `resumeWithSilentFallback`); set `resumptionState` to `"idle"` and record
    the skip in the store (a `resumeSkippedSessionIds: Record<string, true>`
    map on the kanban tasks slice via a new `setResumeSkipped(sessionId, boolean)`
    action — NOT a native `Set`, which would break the Immer-managed,
    SSR-hydrated slice). The flag is keyed by session id, so it cannot leak
    across sessions.
  - Skip-flag semantics are confirmed-running-only and derived from live state,
    not timing:
    - RECORD only when `needs_resume && is_resumable && !is_agent_running`
      AND the live store state is not STARTING/RUNNING. `checkAndResume` must
      re-read the live session state immediately before recording
      (`applyStatusToState` at `use-session-resumption.ts:244-251` merges the
      status state into the store via `setTaskSession`
      (`session-slice.ts:136-150` spreads incoming over existing)).
    - Status hydration uses TIMESTAMP PRECEDENCE: preserve a live
      STARTING/RUNNING state when the incoming status timestamp is absent or
      not newer than the live session's `updated_at`; accept an incoming
      terminal status (FAILED/CANCELLED/COMPLETED) only when its timestamp is
      newer. Blanket protection would reject legitimate newer terminal
      responses and leave the UI stuck with no recovery actions.
    - CLEAR only on confirmed RUNNING: the WS `session.state_changed` handler
      (`lib/ws/handlers/agent-session.ts:692-750`) deletes the flag on the
      RUNNING transition only (NOT on STARTING — a failed manual resume emits
      STARTING before the launch fails); the manual paths (`resumeSession()` at
      `:491-522`, the Start button click at `message-renderer.tsx:55-64`)
      clear it only when the launch response reports `state === "RUNNING"`
      (successful resumes commonly return `state: "STARTING"` — launch
      accepted, not agent running; `executor_resume.go:585`,
      `session_launch.go:358-365`); rejections and `{ success: false }` keep
      the flag.
    - `setResumeSkipped(sessionId, true)` stays conditional in the slice
      action (refuses when the current state is STARTING/RUNNING) as a second
      line of defense.
  - The manual `resumeSession()` action is NOT gated.
- Start agent button for the recovered-idle case —
  `apps/web/components/task/chat/message-renderer.tsx`:
  `TaskDescriptionStartButton` currently renders only for
  `sessionState === "CREATED"`. Extend the visibility condition so it also
  renders when the store marks the session as resume-skipped, BUT only for
  non-FAILED sessions: `TaskDescriptionMessage` returns early for FAILED
  sessions at `:119-134` (agent-styled message, no button slot), and FAILED
  sessions keep their existing recovery actions (`recovery-resume-button` /
  `recovery-fresh-button` in `action-message.tsx`). Dispatch the matching
  intent: `buildStartCreatedRequest` for CREATED sessions, the resume request
  builder for the skipped-resume case.
- Callers:
  - `components/task/task-page-content.tsx` passes the normalized input
    `{ id: task?.id, workflowStepId: task?.workflow_step_id, workflowId: task?.workflow_id }`
    (the effective task is the HTTP `Task`, whose fields are snake_case).
  - `components/kanban-with-preview.tsx` — `useSelectedTask` must include
    `workflowId` in its returned subset (it currently drops it at `:171-180`),
    so cross-workflow preview tasks resolve their own workflow's steps.

---

## Tests

| Behavior (spec scenario) | File | How |
|---|---|---|
| PATCH accepts and persists the setting; GET/boot payload returns it | `apps/backend/internal/user/dto/dto_test.go`, `internal/user/service/service_test.go` (or existing settings-update test) | unit: round-trip `UpdateUserSettingsRequest` → model → DTO; assert blob round-trip |
| Settings blob survives reload (true/false/omitted-legacy) | `apps/backend/internal/user/store/sqlite_test.go` | repo test: `SaveUserSettings` then `GetUserSettings` preserves the value; legacy JSON without the key loads the default `false` |
| `session.ensure` with `auto_start: false` prepares instead of starts | `apps/backend/internal/orchestrator/session_ensure_test.go` (or `session_ensure_office_test.go` pattern) | integration: seed task + step with `auto_start_agent`; call `EnsureSession` with `AutoStart: &false`; assert `Source == "created_prepare"`, `State == "CREATED"`; control case without override still `created_start` |
| passthrough profile never starts on `auto_start: false` | `apps/backend/internal/orchestrator/session_launch_test.go` | integration: mock agent manager reports `CLIPassthrough`; `EnsureSession` with `AutoStart: &false` stays prepared (`CREATED`, no `startTask`); without the override the passthrough upgrade still launches |
| WS handler passes `auto_start` through | `apps/backend/internal/orchestrator/handlers/handlers_test.go` | unit: parse `{task_id, auto_start:false}` → handler calls service with the option |
| SSR defaults and hydration | `apps/web/lib/ssr/user-settings.test.ts` | unit: default `false`; hydrate from `prevent_auto_start_agent_on_open` |
| ensure payload carries `auto_start` | extend `apps/web/hooks/domains/session/use-ensure-task-session.test.ts` (mocks `ensureTaskSession`) | unit: final-step + setting-on → called with `{ autoStart: false }`; non-final → without |
| final-step helper | new test beside the helper | unit: max `(position, id)` tie-break, equal positions resolve to one terminal step, missing step/steps → false |
| caller normalization (snake → camel input) | `apps/web/components/task/task-page-content` test or hook caller test | unit: task page passes `workflowStepId` from `workflow_step_id` |
| cross-workflow preview resolves the right steps | `apps/web/hooks/domains/session/use-ensure-task-session.test.ts` + preview test | unit: task from `kanbanMulti.snapshots[w]` uses snapshot steps, not the active workflow's |
| resume gate skips auto-resume and records the skip | `apps/web/hooks/domains/session/use-session-resumption.test.ts` | unit: `checkAndResume` with `needs_resume && is_resumable` and preventAutoStart → no launch, state idle, skip recorded in store; manual `resumeSession` still launches AND clears the flag only on success |
| failed launch keeps the retry button | `apps/web/hooks/domains/session/use-session-resumption.test.ts` + message-renderer test | unit: resume/start rejects or returns `{ success: false }` → skip flag retained, Start button still rendered (including after a WS STARTING event that precedes the failure) |
| delayed-status race: running WS event before stale status | `apps/web/lib/state/slices/kanban` slice test + `use-session-resumption.test.ts` | unit: WS `state_changed` → STARTING then a stale status response attempts to apply + record a skip → live state is NOT downgraded (timestamp precedence) and the flag is NOT set |
| stale vs newer terminal status precedence | `apps/web/hooks/domains/session/use-session-resumption.test.ts` | unit: an incoming FAILED/CANCELLED/COMPLETED status with a timestamp older than the live session is rejected; the same terminal status with a newer timestamp is accepted (recovery actions appear) |
| launch response clears only when RUNNING | `apps/web/hooks/domains/session/use-session-resumption.test.ts` + message-renderer test | unit: resume returns `{ success: true, state: "STARTING" }` → flag kept (WS RUNNING clears later); `{ success: true, state: "RUNNING" }` → flag cleared; rejections / `{ success: false }` → flag kept |
| WS clear only on RUNNING | `apps/web/lib/ws/handlers/agent-session.test.ts` (or slice test) | unit: `session.state_changed` to RUNNING deletes `resumeSkippedSessionIds[sessionId]`; STARTING does NOT delete it; hydration keeps the record shape intact |
| Start button renders for resume-skipped sessions (non-FAILED) | `apps/web/components/task/chat/message-renderer` test (or component test) | unit: `sessionState === "CREATED"` OR store resume-skipped flag (non-FAILED state) → button visible; FAILED + resume-skipped → no new button (recovery actions remain); button dispatches resume for the skipped case and clears the flag on success |
| settings card renders and saves | `apps/web/components/settings/prevent-auto-start-agent-settings.test.tsx` (mirror `archive-confirmation-settings.test.tsx`) | component: switch toggles, save calls `updateUserSettings({ prevent_auto_start_agent_on_open })` |
| i18n ratchet + em-dash check | — | `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet` |

## E2E Tests

- **Scenario:** GIVEN setting ON and a task in the final workflow step with no
  session, WHEN the task page is opened, THEN the Start agent button
  (`[data-testid="task-description-start-button"]`) is visible and the agent
  never starts on its own.
- **File:** `apps/web/e2e/tests/settings/prevent-auto-start-on-open.spec.ts`
- **What to verify:**
  - Set the setting via `apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true })`.
  - Create a custom workflow whose final step has
    `on_enter: [{ type: "auto_start_agent" }]` — extend
    `e2e/helpers/api-client.ts` `createWorkflowStep` with an `events` opt
    (the backend `POST /api/v1/workflow/steps` already accepts `events`,
    `internal/workflow/controller/controller.go` `CreateStepRequest`).
  - Final-step case: create a task in that final step, open `/t/:id`, assert
    the Start agent button appears and the agent stays stopped until clicked.
  - Setting-off control: create a SEPARATE fresh task in the same final step
    (never opened before, no session) — `useEnsureTaskSession` no-ops when a
    session already exists, so reusing the setting-on task cannot exercise the
    control.
  - Recovered-idle case: seed a task+session, let the first turn finish, then
    `backend.restart()` + `testPage.reload()` (the restart pattern from
    `e2e/tests/session/session-resume.spec.ts`), assert no automatic resume
    (the agent does not reach a running state on its own and the Start agent
    button is visible), then click the button and assert the agent resumes.
  - Control: with the setting off, the same final-step task auto-starts on
    open (no start button; agent reaches a running/ready state).
  - Isolation: `e2e/fixtures/test-base.ts` per-test settings reset (`:190-225`)
    gains `prevent_auto_start_agent_on_open: false` so a test enabling the
    setting cannot leak into later tests in the same worker.
  - Mobile: add `e2e/tests/settings/mobile-prevent-auto-start-on-open.spec.ts`
    (same fixtures, phone viewport via `testPage.setViewportSize`, following
    `mobile-general-settings.spec.ts`) asserting the Start agent button on the
    final-step case. The gating hooks run on mobile through the shared
    responsive `TaskPageContent` (`useResponsiveBreakpoint` at
    `task-page-content.tsx:325`).

## Verification Results

All six tasks implemented with TDD and verified:

- Backend: `go test ./internal/user/... ./internal/orchestrator/... ./internal/backendapp/... -race` — clean. golangci-lint on the PR diff — clean (after splitting the store test file and rebasing onto the then-current origin/main).
- Frontend: `pnpm run typecheck` — clean. Vitest suites (lib/ws/handlers, lib/state/slices/kanban, hooks/domains/session, lib/ssr, settings card, message-list-footer) — 753 tests passing. `pnpm run i18n:check` + `pnpm run i18n:ratchet` — clean.
- E2E (mock agent): `KANDEV_E2E_MOCK=true pnpm e2e:raw --project=chromium settings/prevent-auto-start-on-open.spec.ts` — 3/3 passing; `--project=mobile-chrome settings/mobile-prevent-auto-start-on-open.spec.ts` — 1/1 passing.
- Pre-commit hooks (harness, architecture, gofmt, golangci, prettier, web lint, i18n guard, commitlint) — passed on every commit; no bypasses.

## Implementation Waves And Parallel Candidates

```
Wave 1 (sequential):
- [x] [task-01-backend-settings-field](task-01-backend-settings-field.md)
- [x] [task-02-backend-ensure-override](task-02-backend-ensure-override.md)
- [x] [task-03-frontend-settings-plumbing](task-03-frontend-settings-plumbing.md)
- [x] [task-04-settings-ui-card](task-04-settings-ui-card.md)
- [x] [task-05-open-time-gates](task-05-open-time-gates.md)

Wave 2:
- [x] [task-06-e2e](task-06-e2e.md)
```

All tasks are sequential: 01→02 share the backend settings surface, 03 depends
on 01's contract, 04 depends on 03's store field, 05 depends on 03's API
client change, and 06 depends on 05's behavior. No parallel-safe candidates.

## Open Questions

- Office advanced-mode `ensureExecution` resume stays a no-op (the WS handler
  drops `ensure_execution` today); the spec parks it out of scope until that
  flow is wired.
