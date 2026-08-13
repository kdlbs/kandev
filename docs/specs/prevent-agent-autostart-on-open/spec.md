---
status: draft
created: 2026-08-11
owner: platform
---

# Prevent Agent Auto-Start On Open

## Why

Opening a task after a kandev restart silently starts (or resumes) the agent:
a session in a normal active state with no running process is resumed the
moment its task page loads, and a task in the final workflow step can have a
fresh agent auto-started when the step's on-enter action allows it. Users who
want to inspect the task first, or tasks that are effectively done, get an
agent spinning up unprompted. There is no way to keep the agent stopped on
open and start it explicitly instead.

## What

- A new per-user setting, `prevent_auto_start_agent_on_open` (default `false`),
  surfaced in Settings → Task Actions as a switch.
- When the setting is ON, opening a task whose session exists but whose agent
  is not running (the post-restart / recovered-idle shape) MUST NOT
  automatically resume the agent. The task opens with the agent stopped and a
  Start agent button is shown in the chat; clicking it resumes the agent.
  (The existing session-menu resume and chat composer remain available too.)
  For FAILED-but-resumable sessions the existing recovery actions (Resume
  session / Start fresh session buttons) remain the manual affordance; the
  new Start agent button is not rendered for FAILED sessions.
- When the setting is ON, opening a task whose current workflow step is the
  final step of its workflow MUST NOT auto-start the agent when the task has
  no session. The session is created in the never-started (prepared) state
  and the Start agent button is shown.
- When the setting is OFF (default), opening a task behaves exactly as today.
- The setting gates only automatic starts triggered by *opening* a task. It
  MUST NOT affect: workflow step transitions whose on-enter actions include
  `auto_start_agent`, explicit user actions (Start agent, Resume, sending a
  message), or watcher/automation/PR-triggered starts.

## Data model

The setting lives in the existing per-user settings blob (the `settings`
column of the `users` table, serialized as JSON). No schema migration.

```
users.settings.prevent_auto_start_agent_on_open   bool   default false
```

It is read by the web SPA from the boot payload and the `GET
/api/v1/user/settings` response, and written via `PATCH /api/v1/user/settings`.

## API surface

### User settings

- `GET /api/v1/user/settings` returns the field as
  `prevent_auto_start_agent_on_open` inside `settings`.
- `PATCH /api/v1/user/settings` accepts an optional
  `prevent_auto_start_agent_on_open: bool`; omitted means "leave unchanged".
- The SSR boot payload exposes it as `preventAutoStartAgentOnOpen` (camelCase),
  matching the other boot-payload settings keys.

### WebSocket `session.ensure`

The `session.ensure` WS request gains an optional `auto_start: bool` override.

- Request shape: `{ task_id: string, auto_start?: boolean }` (plus the
  existing optional fields).
- When `auto_start` is explicitly `false`, the backend creates the session in
  the workspace-only (prepared / `created_prepare`) mode even when the task's
  workflow step would otherwise allow auto-start (`created_start`). The
  session is created in the never-started (`CREATED`) state.
- When `auto_start` is absent or `true`, the current behavior is unchanged:
  the backend decides start-vs-prepare from the resolved agent profile and the
  step's `auto_start_agent` on-enter action.
- The response shape is unchanged.

## State machine

No session states are added or removed. The setting changes only whether an
auto-start or auto-resume is *triggered by opening the task page*:

| Trigger | Setting OFF (default) | Setting ON |
|---|---|---|
| Open task, no session, step allows auto-start | agent starts (`created_start`) | non-final step: unchanged; final step: `created_prepare`, Start agent button shown |
| Open task, session exists, agent not running, session resumable | auto-resume (`session.launch` intent=resume) | no auto-resume; session stays stopped and a Start agent button (resume action) is shown in the chat |
| Open task, agent already running | no-op | no-op (unchanged) |
| Workflow step transition with `auto_start_agent` on-enter | agent starts | agent starts (unchanged) |
| User clicks Start agent / Resume / sends a message | agent starts | agent starts (unchanged) |

## Failure modes

- The kanban workflow steps are not (yet) loaded in the client store when the
  final-step check runs: the task is treated as "not in the final step", so
  the ensure gate does not apply and behavior matches the setting OFF state.
  Safe default: never silently block a start because of missing metadata.
- The user settings fail to hydrate (SSR fallback): the setting reads `false`
  and behavior matches today.
- `session.ensure` with `auto_start: false` on a task whose agent profile
  cannot be resolved: the same prepare-only outcome as today's no-profile
  path; no error is surfaced beyond the existing behavior.

## Persistence guarantees

The setting is per-user, stored in `users.settings` (JSON blob serialized by
`internal/user/store/sqlite.go`), and survives kandev restarts. It is hydrated
into the web boot payload on every page load. There is no per-workspace or
per-task variant in this feature.

## Scenarios

- **GIVEN** the setting is ON and a task's session is in an active state with a
  resume token but no running agent (post-restart shape), **WHEN** the user
  opens the task page, **THEN** no `session.launch` resume request is sent on
  open, the session stays stopped, and the task page shows a Start agent
  button that resumes the agent when clicked.
- **GIVEN** the setting is ON and a task's session is FAILED but resumable,
  **WHEN** the user opens the task page, **THEN** no auto-resume is sent on
  open and the session keeps the existing recovery actions (Resume session and
  Start fresh session buttons). The new Start agent button is not rendered for
  FAILED sessions.
- **GIVEN** the setting is ON and the task's current workflow step is the final
  step, **WHEN** the user opens the task and it has no session, **THEN**
  `session.ensure` is called with `auto_start: false`, the session is created
  in the prepared (`CREATED`) state, and the Start agent button is shown
  without any agent process starting.
- **GIVEN** the setting is ON and the task's current workflow step is NOT the
  final step, **WHEN** the user opens the task and it has no session, **THEN**
  auto-start behavior is unchanged (the agent starts when the step's on-enter
  action allows it).
- **GIVEN** the setting is OFF (default), **WHEN** the user opens any task,
  **THEN** today's auto-start and auto-resume behavior is unchanged.
- **GIVEN** the setting is ON and a task in the final step already has a
  never-started (`CREATED`) session, **WHEN** the task is opened, **THEN** the
  Start agent button is shown and no agent starts.
- **GIVEN** the setting is ON, **WHEN** the user explicitly starts or resumes
  the agent (Start agent button, session menu Resume, or sending a message),
  **THEN** the agent starts normally.
- **GIVEN** the setting is ON, **WHEN** a workflow step transition with
  `auto_start_agent` in its on-enter actions moves the task into that step,
  **THEN** the agent still auto-starts.
- **GIVEN** the setting is ON and a task is opened in the kanban preview
  panel, **WHEN** the preview opens, **THEN** the same gates apply (no
  auto-resume, no final-step auto-start).

## Out of scope

- The office board task pages (simple and advanced mode). They do not
  auto-start or auto-resume agents on open today (`useEnsureTaskSession` and
  `useSessionResumption` are not wired into those flows); the latent
  `ensure_execution` gap in the `session.ensure` WS handler is left as-is.
- Watcher / automation / PR-triggered auto-starts. They are not "opening a
  task".
- Workflow on-enter `auto_start_agent` transitions. Only open-time behavior is
  gated.
- Server-side enforcement of the preference for non-web clients. The gate is
  implemented by the web client not issuing the resume/start request; the
  backend contract change is an opt-in request override.
- A per-workspace or per-workflow variant of the setting.

## Open questions

- Whether the option should also gate the office advanced mode's
  `ensureExecution` resume once that WS field is actually wired. Currently a
  no-op; intentionally out of scope until the office flow auto-starts.
