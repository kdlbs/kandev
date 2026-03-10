---
status: draft
created: 2026-05-21
owner: jcfs
---

# Automations in Settings

## Why

Users want to schedule an agent to run a prompt on a cron (e.g. "every weekday at 9am, run claude on this repo with the prompt 'check for stale branches'") without first learning Office's vocabulary — workspaces, projects, agent instances, roles, budgets, skills. Office routines already deliver this capability end-to-end, but the UI to reach them is buried inside `/office/<workspace>/routines`, behind the `KANDEV_FEATURES_OFFICE` flag and a workspace-selection flow that doesn't apply when the user just wants "a cron + a prompt".

Automations-in-settings is a flat, low-ceremony entry point under `/settings/automations` that reuses the Office routines backend and the existing create-routine wizard, but hides the workspace / project / Office-agent concepts behind an auto-provisioned default workspace + assignee. Power users keep full editing access via `/office/<workspace>/routines`; new users get a "schedule a prompt" surface with no Office onboarding.

## What

- A new settings page at `/settings/automations` lists the user's automations as a flat table: name, schedule (human-readable cron + timezone), assignee agent type (e.g. `claude`), last run (status + relative time), next run, enabled toggle, row actions (edit, run now, delete).
- A "New automation" button opens the existing 3-step routine create wizard with the workspace selector **hidden** and pre-filled to the default automations workspace.
- On first use (no automations workspace exists yet), the act of opening the create wizard auto-provisions a hidden Office workspace + a default assignee Office agent without prompting the user. Provisioning is idempotent and atomic from the user's perspective: either the wizard opens with the workspace already attached, or it surfaces an error and creates nothing.
- Automations created from settings are indistinguishable on the backend from routines created via `/office/<workspace>/routines` — they are the same `office_routines` + `office_routine_triggers` rows, scheduled by the same dispatcher, retried by the same policy. Editing the same automation from either UI yields the same result.
- The settings UI exposes only the **schedule (cron)** trigger kind. Webhook and manual triggers remain available in the Office routines UI but are not surfaced in settings.
- The settings UI exposes only **lightweight** routines (no task template). The agent receives the user-provided prompt and runs once per fire. Heavy routines (task-template, blockers, projects) remain available in the Office routines UI but are hidden from settings.
- The settings UI MUST be reachable independently of the `KANDEV_FEATURES_OFFICE` flag state — when the flag is off, the rest of `/office/*` stays gated, but `/settings/automations` works and auto-provisioning treats the underlying Office subsystem as a private implementation detail.
- An automation can be paused / resumed by toggling the row's enabled control, which flips the routine status between `active` and `paused`.
- "Run now" on a row fires the routine immediately via the existing manual-fire endpoint and shows the launched run inline (status badge updates without page reload).
- Editing an automation in settings opens the same wizard pre-filled. Fields surfaced: name, description (optional), assignee agent type, prompt, cron expression, timezone, enabled.
- A link in the page header — "Need more control? Open the full automations editor" — deep-links to the corresponding `/office/<workspace>/routines` page for the auto-provisioned workspace, so power users can reach concurrency policies, catch-up policies, variables, webhook triggers, and heavy routines.

## Data model

No new tables. Automations-in-settings is a UI projection over existing entities from `scheduler.md`:

- `office_routines` row — represents one automation. Filter for the settings list: `workspace_id = <default-automations-workspace-id> AND task_template = '' AND status != 'archived'`.
- `office_routine_triggers` row — one schedule trigger per automation (`kind = 'schedule'`).
- `office_routine_runs` row — execution history. Settings shows only the most recent row's `status` and `started_at` per routine.
- `office_workspaces` row — exactly one is the default automations workspace per user, identified by a marker on the row. Field added: `office_workspaces.is_settings_default BOOL NOT NULL DEFAULT FALSE` with a partial unique index ensuring at most one default per user.
- `office_agent_instances` row — exactly one is the default assignee for the default automations workspace, identified by `office_agent_instances.is_settings_default BOOL NOT NULL DEFAULT FALSE` with a partial unique index ensuring at most one default per workspace.

The default workspace's `name` is fixed (`"Automations"`); its `display_name` is user-editable from `/office/<workspace>/routines` but the marker field is read-only. The default agent's role is `worker`, `skip_idle_wakeups = false` (since automations should fire even with no actionable tasks), `max_concurrent_sessions = 1`, `cooldown_sec = 10`.

## API surface

No new routes. Settings reuses the existing Office routines endpoints from `scheduler.md`:

- `GET /api/office/routines?workspace_id=<default-automations-workspace-id>&task_template_empty=true` — settings list (server filter on lightweight routines).
- `POST /api/office/routines` — create. Settings injects `workspace_id` and `assignee_agent_id` from the resolved default.
- `PATCH /api/office/routines/:id` — toggle enabled / edit name / description / prompt / cron.
- `DELETE /api/office/routines/:id` — delete an automation.
- `POST /api/office/routines/:id/fire` — "Run now".

One new endpoint resolves (creating if missing) the default automations workspace + assignee:

- `POST /api/settings/automations/ensure-default` → `{ workspace_id, assignee_agent_id }`. Idempotent: returns the existing pair if already provisioned. Errors if the underlying Office subsystem is unavailable. Called by the settings page on first open of the create dialog (or list, whichever comes first that needs an ID).

The `task_template_empty=true` filter parameter is the only addition to the existing list endpoint; everything else is the same call surface the Office routines UI uses today.

## State machine

Automations inherit the routine lifecycle from `scheduler.md` (`active <-> paused`, `active -> archived`). Settings exposes only `active` and `paused`; deleting an automation calls `DELETE` rather than archiving. The settings list query filters out `archived` rows but does not surface them — power users who want to archive use the Office routines UI.

## Permissions

- A user can manage their own automations only — the settings page filters by the user's default automations workspace, which is owned by that user.
- The same user, when navigating to `/office/<workspace>/routines` for the default automations workspace, sees the same routines plus any heavy / webhook ones they created via the Office surface.
- The auto-provisioned default workspace + agent cannot be deleted from settings. Deletion is reachable only via the Office workspace deletion flow (`docs/specs/workspaces/deletion.md`); doing so removes all automations.

## Failure modes

| Dependency / invariant | Behavior |
|---|---|
| `ensure-default` call fails (DB write error) | Settings page renders the list (which may be empty) and a banner: "Couldn't initialize automations. Retry." Create button is disabled until retry succeeds. |
| Office subsystem is structurally unavailable (e.g. migration in progress) | Settings page renders the same banner; existing automation rows still display from the last successful list fetch. |
| Default workspace marker exists but the underlying row is missing (drift) | `ensure-default` clears the stale marker and provisions a new workspace + agent. Existing automations under the missing workspace are not recoverable from settings (the user is told to use `/office/*` if they have other workspaces). |
| User deletes the default agent via Office UI | Next `ensure-default` call re-creates a default agent. Existing automations whose `assignee_agent_id` pointed to the deleted agent are surfaced in the settings list with an "Assignee missing — edit to reassign" indicator; firing is blocked until reassigned. |
| User toggles `KANDEV_FEATURES_OFFICE` off after creating automations | The scheduler may stop dispatching depending on how the flag gates the dispatcher (see Open questions). Settings page remains reachable and shows existing automations; UI surfaces a warning if backend reports the dispatcher is disabled. |
| Cron expression invalid | Caught client-side and by the existing routines validator; the create / edit dialog blocks save with the existing error display. |
| Two settings tabs open in the same browser race the "first create" | `ensure-default` is idempotent on the unique marker, so the second call returns the same pair; no duplicate workspace is created. |

## Persistence guarantees

Settings automations are routines — they survive restart with the same guarantees as `scheduler.md`. The default workspace + agent markers survive restart; provisioning runs exactly once per user across the lifetime of the install unless the marker is cleared by the drift-recovery path above.

## Scenarios

- **GIVEN** a user who has never used Office, **WHEN** they open `/settings/automations` and click "New automation" for the first time, **THEN** an Office workspace named `Automations` and a worker agent are auto-provisioned, the create wizard opens with the workspace selector hidden, and the user enters name + prompt + cron and saves. The new automation appears in the settings list immediately.

- **GIVEN** the user's default automations workspace already exists, **WHEN** they open `/settings/automations`, **THEN** no new workspace is created (`ensure-default` returns the existing IDs) and the list renders the user's automations.

- **GIVEN** an automation with cron `0 9 * * 1-5` in `Europe/Lisbon`, **WHEN** the scheduler tick crosses 09:00 Lisbon time on a weekday, **THEN** a lightweight routine wakeup fires for the default assignee, the agent receives the user-provided prompt, and the most-recent run status on the settings row updates to "Running" then "Done" without a page refresh.

- **GIVEN** an automation in `active` status, **WHEN** the user toggles the row's enabled switch off, **THEN** a PATCH sets routine `status = 'paused'`; no further cron ticks dispatch wakeups for this automation. Toggling it back to on sets `status = 'active'` and the next cron boundary fires normally.

- **GIVEN** a settings automation, **WHEN** the user clicks "Run now", **THEN** the existing `POST /api/office/routines/:id/fire` endpoint is called and the row's status badge transitions to "Running" within one WS tick.

- **GIVEN** a settings automation created via the settings UI, **WHEN** the user navigates to `/office/<default-workspace>/routines`, **THEN** the same routine appears in the full Office routines list with the same `id`. Editing it from either UI persists to the same row.

- **GIVEN** a user has created automations and `KANDEV_FEATURES_OFFICE` is then disabled, **WHEN** they open `/settings/automations`, **THEN** the page still loads and lists their automations. A banner indicates that automations may not fire because the Office subsystem is disabled.

- **GIVEN** a heavy routine and a webhook-triggered routine in the default automations workspace (created via `/office/*`), **WHEN** the user opens `/settings/automations`, **THEN** neither appears in the settings list (filter: lightweight + schedule-trigger only). The settings list count + Office routines list count differ accordingly, which is expected.

- **GIVEN** a settings automation whose assignee agent was deleted from the Office UI, **WHEN** the user opens `/settings/automations`, **THEN** the row shows "Assignee missing — edit to reassign". Firing is blocked. The next `ensure-default` call re-provisions a default agent; the user must explicitly edit existing automations to point at the new agent (no auto-reassign).

- **GIVEN** an in-flight scheduler restart in the middle of a fire, **WHEN** the backend comes back up, **THEN** the routine's queued / scheduled-retry wakeup-requests are resumed per `scheduler.md` persistence guarantees and the settings row's last-run status reflects the eventual terminal state.

## Out of scope

- Webhook-triggered automations in the settings UI (use `/office/<workspace>/routines`).
- Heavy automations with task templates, blockers, or projects in the settings UI.
- Concurrency-policy / catch-up-policy / variable controls in the settings UI — defaults applied (`coalesce_if_active`, `enqueue_missed_with_cap` with cap 25).
- Approval gates, reviewer flows, multi-agent delegation — these belong to Office proper.
- Cost budgets, skill assignment, role configuration for the default agent.
- A standalone "settings-only" scheduler decoupled from Office (the prior option 2; this spec is the lift-and-shift option).
- Multi-user workspace sharing — automations are per-user via the `is_settings_default` marker.
- Migration of existing routines created via `/office/*` into the default automations workspace.
- Importing / exporting automations as YAML in the settings UI (configloader still works for power users).

## Open questions

- Does `KANDEV_FEATURES_OFFICE = false` short-circuit the scheduler dispatcher entirely, or only gate the `/office/*` routes? If the former, the "settings works even when flag is off" requirement implies decoupling the dispatcher from the UI gate. Decide before implementation.
- Should the settings list show the most-recent run inline as a status badge (requires joining `office_routine_runs`) or only the routine's last-fired timestamp (cheaper)? Lean: status badge — matches the rest of the Kandev UI.
- Is there value in surfacing "agent type" (claude / codex / opencode) as the settings-facing primary choice instead of "agent instance"? If yes, the default Office agent needs a mapping to the selected agent type at fire time. Lean: yes, since the entire point is to avoid Office vocabulary.
