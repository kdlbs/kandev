---
id: "06-e2e"
title: "E2E: prevent auto-start on open"
status: done
wave: 2
parallelism: sequential
depends_on: ["05-open-time-gates"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 06: E2E — prevent auto-start on open

## Acceptance

- **Final-step case:** with the setting on, opening a task whose current
  workflow step is the final step (and has `auto_start_agent` in its on-enter
  actions) shows the Start agent button
  (`[data-testid="task-description-start-button"]`) and does not start an
  agent on its own. Clicking the button starts the agent.
- **Recovered-idle case:** with the setting on, after a backend restart a
  task whose session needs resume does not auto-resume; the Start agent button
  is visible and the agent stays stopped until clicked, then resumes.
- **Setting-off control:** with the setting off, a SEPARATE fresh task in the
  same final step (never opened before, no session) auto-starts on open (no
  start button; agent reaches a running/ready state). The control must not
  reuse the setting-on task: `useEnsureTaskSession` no-ops once a session
  exists, so a reused task cannot exercise fresh auto-start.
- **Isolation:** `e2e/fixtures/test-base.ts` per-test settings reset
  (`:190-225`) gains `prevent_auto_start_agent_on_open: false` — PATCH omits
  the field as "unchanged", so without the reset a test enabling the setting
  leaks into later tests in the same worker.
- **Mobile:** `e2e/tests/settings/mobile-prevent-auto-start-on-open.spec.ts`
  (same fixtures, phone viewport, `mobile-general-settings.spec.ts` pattern)
  asserts the Start agent button for the final-step case. The gating hooks run
  on mobile through the shared responsive `TaskPageContent`
  (`useResponsiveBreakpoint` at `task-page-content.tsx:325`).

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
```

```bash
(cd apps/web && KANDEV_E2E_MOCK=true pnpm e2e:raw -- --project=chromium settings/prevent-auto-start-on-open.spec.ts settings/mobile-prevent-auto-start-on-open.spec.ts)
```

## Files Likely Touched

- `apps/web/e2e/tests/settings/prevent-auto-start-on-open.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-prevent-auto-start-on-open.spec.ts` (new, phone viewport)
- `apps/web/e2e/fixtures/test-base.ts` (per-test settings reset at `:190-225`)
- `apps/web/e2e/helpers/api-client.ts`:
  - `saveUserSettings` (`:926`) gains `prevent_auto_start_agent_on_open?: boolean`
    (there is no `updateUserSettings` helper — the settings PATCH helper is
    named `saveUserSettings`).
  - `createWorkflowStep` (`:701`) gains an `events` opt, e.g.
    `events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }> }`,
    passed through as `events` — the backend `POST /api/v1/workflow/steps`
    already accepts `events` (`internal/workflow/controller/controller.go`
    `CreateStepRequest`).

## Dependencies

Task 05 (the gating behavior under test), Task 03 (the setting is settable via
`saveUserSettings`).

## Inputs

- Spec scenarios 2, 4, 5.
- Existing patterns:
  - `apps/web/e2e/tests/settings/startup-page.spec.ts` (settings UI flow).
  - `apps/web/e2e/tests/session/session-resume.spec.ts` `:44-85` — the
    backend-restart pattern: `backend.restart()` then `testPage.reload()`
    (this is the correct restart fixture; `session-recovery.spec.ts` covers
    agent-crash recovery buttons, not backend restarts).
  - `api-client.ts` `createWorkflow` / `createWorkflowStep` / `createTask`
    with `workflow_id` + `workflow_step_id`.
- Note: built-in final steps (office-default, kanban) do NOT carry
  `auto_start_agent`; the fixture must create a custom workflow through the
  API whose last step has `on_enter: [{ type: "auto_start_agent" }]`.

## Output Contract

The spec's two user-visible gates are pinned end to end with the mock agent:
final-step no-auto-start and post-restart no-auto-resume, each with the Start
agent button visible and a working click-through. Control case (setting off)
asserts the current auto-start behavior. Cleanup: fixture-only workflows/tasks
created by the spec are removed in teardown (try/finally pattern used by
sibling specs).
