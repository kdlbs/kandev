---
spec: docs/specs/agents/requirements/utility-agent-profiles.md
created: 2026-08-08
status: in_progress
---

# Implementation Plan: Profile-backed Utility Agents

## Overview

Replace every utility-agent agent/model binding with an agent-profile reference, migrate only
unambiguous legacy choices, and make both sessionless and task-session-bound one-shot execution
apply the complete resolved profile. Warn users before a profile change breaks utility bindings.
Land persistence and API contracts first, then the shared profile-aware one-shot runner and its
plugin/review consumers, followed by the Settings UI, dependency dialogs, desktop/mobile E2E
coverage, and public documentation. The settings surface also needs a discoverability follow-up:
explain the one-shot UI scope, place Configuration Chat Agent next to the default profile, and use
one icon-enabled searchable profile picker everywhere on the page.

## Architecture and migration

### Canonical profile binding

`apps/backend/internal/utility/models.UtilityAgent` gains `AgentProfileID` and
`ProfileBindingState` (`inherit`, `explicit`, `unconfigured`); `AgentID` and `Model` stop
participating in execution. `DefaultUtilitySettings` becomes a default profile ID rather than an
agent/model pair, and `PromptRequest` carries the effective profile ID plus resolved one-shot
configuration. `utility_agent_calls` records the effective profile ID and resolved model.

Add a utility-profile resolver under `apps/backend/internal/utility/profilebinding/`. It reads the
agent settings repository and registry to provide two operations:

- resolve one saved profile ID and reject deleted, disabled, workspace-scoped,
  CLI-passthrough-only, or non-ACP/inference profiles;
- match a legacy agent/model pair only when exactly one eligible global profile has the matching
  parent agent and model.

The resolver is the only place utility code decides profile eligibility. HTTP handlers, plugins,
review, and frontend filtering must not recreate different fallback rules.

### Durable migration

Update `apps/backend/internal/utility/store/sqlite.go` to add nullable/default-empty
`agent_profile_id` columns and a binding-state column to `utility_agents`, plus the profile ID column
to `utility_agent_calls`. Include scans, inserts, updates, and migration tests. Keep legacy utility
columns only as read-only migration inputs.

Add `default_utility_agent_profile_id` to backend-owned portable user settings. After agent-profile
seeding/reconciliation and utility built-in seeding, run one idempotent migration that:

1. reads each legacy user default and utility-agent pair;
2. asks the canonical matcher for exactly one eligible profile;
3. writes that ID and `explicit` on one match;
4. writes `unconfigured` on zero or multiple matches, so an empty ID cannot become accidental
   default inheritance;
5. keeps the legacy values for retry or diagnostics; and
6. never revisits a non-empty new profile ID or a user-selected binding.

Migration errors fail closed and remain retryable at the next startup; they do not write a partial
raw fallback into execution configuration. The migration tests cover exact match, no match,
ambiguous match, explicit empty model, already-migrated rows, and historical calls.

## Backend

### Utility CRUD and execution preparation

Update the utility DTO/controller/service path so list/get/create/update responses and requests use
`agent_profile_id` plus `profile_binding_state`. A custom utility agent requires an eligible profile
when saved by the user. A built-in can explicitly inherit, select a profile, or remain
`unconfigured` after migration. `PreparePromptRequest` resolves the binding before creating a call
record and returns a typed configuration error when no effective profile exists.

`POST /api/v1/utility/execute` keeps the caller-facing utility-agent selector. It returns an
actionable 4xx configuration failure for absent/stale/ineligible profiles and does not dispatch or
fall back. Both the sessionless and session-bound branches receive the same effective profile ID.

### Shared profile-aware one-shot launch

Add a small shared resolver/builder under `apps/backend/internal/agent/profileexecution/` that
turns an eligible profile into a one-shot launch snapshot:

- registered inference agent type and inference command;
- profile model, mode, and dynamic config options;
- enabled `cli_flags` and validated `command_prefix` as structured argv;
- agent runtime environment merged with resolved profile environment/secret references;
- stripped environment keys; and
- explicit auto-approval policy.

Reuse existing CLI flag tokenization, command-prefix validation, profile-secret resolution, and
agent registry command rules. Do not copy secret values into utility rows, responses, logs, or call
history. A malformed command prefix or unresolved required secret fails before launching; the
runner never retries without the profile setting.

Extend `apps/backend/internal/agentctl/server/utility.PromptRequest` with profile-derived config
options and an explicit non-interactive permission policy. `ACPInferenceExecutor` applies dynamic
config options after `session/new`; when auto-approval is enabled it approves requests, and when it
is disabled it rejects the first unresolved permission request and returns a typed actionable error
instead of using the normal interactive timeout.

Update `apps/backend/internal/agent/hostutility.Manager` to resolve a profile before selecting its
existing warm per-agent-type instance. Update
`apps/backend/internal/agent/runtime/lifecycle.ExecuteInferencePrompt` to take a profile ID and use
the same launch snapshot inside the active task's agentctl/workspace. The current host fallback and
task-workspace semantics remain unchanged.

### Plugin and review consumers

Change `apps/backend/internal/plugins` utility adapters to carry `AgentProfileID` and invoke the
profile-aware runner. Plugin configuration still stores a utility-agent ID and still maps missing,
deleted, disabled, or newly unconfigured selections to gRPC `FailedPrecondition`.

Change `apps/backend/internal/review` identity resolution so an explicit workflow profile, the
`code-review` utility override, or the default utility profile all yield a profile ID. Remove the
agent/model-pair compatibility and same-agent model borrowing. `reviewInference` passes the profile
ID to session-bound execution and to host fallback, while review-run accounting continues to record
the resolved agent/model returned by execution.

### Profile dependency checks

Extend the agent-profile dependency service and existing profile-in-use error to include utility
agent references. A disable request checks references before saving and returns warning details for
the profile page. A delete request returns the same details through the existing conflict dialog;
the confirmed force path leaves utility IDs stale and does not auto-reassign them. Lookup errors
block both mutations.

## Frontend

### Settings data and API types

Update `apps/web/lib/api/domains/utility-api.ts` and the user-settings transport/store mapping to use
`agent_profile_id`, `profile_binding_state`, and `default_utility_agent_profile_id`. Remove the utility page's inference-agent
probe/model-list data dependency. Load the established `agentProfiles` Settings slice through
`useSettingsData` and derive eligible utility choices in one helper: enabled, non-passthrough
global profiles whose parent agent is ACP/inference-capable. Keep a currently saved stale ID visible
as an unavailable selection with repair copy rather than blanking or auto-changing it.

### Utility Agents settings surface

Replace `DefaultModelSection` with a self-documenting default-profile card explaining that the
profile controls the model, permissions, flags, environment, and cost. Replace every built-in
model override with **Default profile** or one profile selection. Keep default and built-in changes
inside the existing shared Settings save/discard contributor and include only profile IDs in its
revision.

Replace the custom-agent dialog's two-column agent/model controls and live probe status with one
agent-profile picker. Gate create/save on name, prompt, and eligible profile. Custom rows display
the selected profile label; missing/deleted/disabled selections show localized repair copy.

### Utility Agents settings discoverability follow-up

Keep the page's existing Settings save/discard ownership and move the Configuration Chat Agent card
into the Utility Agents card sequence directly after the default profile card. The rendered order is
default utility profile, Configuration Chat Agent, Actions, and custom utility agents. Improve the
page and Actions descriptions so they explain that utility agents perform one Kandev UI operation at
a time and are separate from agents that run inside task sessions.

Replace the page's remaining Radix profile selects with one shared profile-picker composition based
on the existing `Combobox` and `AgentLogo` components. The picker must render the icon in the trigger
and each option, search by profile label and parent agent name, preserve unavailable selected IDs for
repair, and retain the existing utility eligibility rules. Apply the same interaction to the default
profile, built-in action overrides, custom utility-agent dialog, and Configuration Chat Agent selector.
Keep the popover width bounded to the trigger, keep the list internally scrollable on phones, and
preserve keyboard and touch behavior without document horizontal overflow.

Update English and Chinese catalogs and regenerate pseudo-locale output. Keep test IDs stable where
they describe cards/rows rather than the removed model controls; add explicit profile-picker IDs
for robust E2E selection.

The agent profile page and existing profile-in-use dialog show utility-agent references when a
disable or delete would break them. Disable requires cancel or confirmation. Delete uses the existing
explicit force confirmation. The dialog does not offer an automatic replacement.

### Mobile design contract

- **Desktop outcome:** users choose one default profile and optional per-action profiles in the
  existing bounded Settings cards; custom prompt editing remains in the existing dialog.
- **Mobile entry point:** the same `/settings/utility-agents` route and cards remain the entry point.
  Cards stack vertically; the existing Radix Select phone treatment provides touch choices.
- **Nearest shipped exemplar:** the current `mobile-utility-agents.spec.ts` card/row composition and
  global responsive Select treatment in `apps/web/app/globals.css` supply width containment and
  touch behavior. No new drawer, route, or desktop workbench is introduced.
- **Information hierarchy:** profile selection precedes action overrides, followed by custom agents;
  profile labels remain readable and unavailable-state copy remains adjacent to the control.
- **Scroll and geometry:** the settings document remains the single vertical scroll owner; selects
  and edit/add actions have visible touch targets, cards create no document horizontal overflow,
  and the custom dialog remains viewport-contained with an internal scroll region if its prompt
  editor exceeds `100dvh`.
- **Shared versus specialized behavior:** profile eligibility, saved state, and mutations are shared;
  only responsive card/control sizing and the existing phone Select presentation specialize.
- **Parity proof:** mobile Playwright changes the default and an action override, opens the custom
  dialog, and verifies controls are reachable and the document does not overflow horizontally.

## Tests

- **Persistence and migration:** extend utility store migration tests and user-settings tests for
  the new fields, exact/ambiguous/no-match migration, binding-state idempotency, legacy retry data,
  and historical call rows.
- **Utility validation/resolution:** table-driven service/controller/handler tests cover custom
  required profile, built-in inheritance, explicit override, missing default, disabled/deleted/
  workspace/passthrough/non-inference rejection, and no call dispatch on configuration failure.
- **Launch snapshot:** unit tests prove model, mode, config options, flags, prefix, environment,
  secret resolution, strip-env, and auto-approval are copied from one profile; bad prefixes and
  missing secrets fail closed.
- **Agentctl permissions:** executor tests prove auto-approved permission requests continue, denied
  non-interactive requests fail promptly, and config options apply before the prompt.
- **Host/session parity:** hostutility and lifecycle tests assert both paths receive equivalent
  profile-derived launch configuration while keeping their respective temporary/task workdirs.
- **Plugin/review:** update plugin host, backend adapters, review resolver, and review runner tests to
  pass profile IDs and preserve typed failure/source precedence.
- **Frontend state/components:** utility dirty/revision helpers, eligible-profile derivation, stale
  selection rendering, default/action save-discard, custom dialog validation, dependency warning
  and delete-conflict rendering, and responsive row rendering receive focused Vitest coverage.

## E2E Tests

- **Desktop scenario:** GIVEN enabled agent profiles, WHEN the user selects a default profile, sets
  one action override, saves, and reloads, THEN both profile IDs persist and no agent/model controls
  render. **File:** `apps/web/e2e/tests/settings/utility-agents.spec.ts`.
- **Desktop scenario:** GIVEN a custom utility agent dialog, WHEN the user chooses a profile and
  creates the agent, THEN the row shows the profile label and editing restores it. **File:**
  `apps/web/e2e/tests/settings/utility-agents.spec.ts`.
- **Failure scenario:** GIVEN a saved profile becomes disabled, WHEN a bound utility action is
  invoked, THEN execution is not dispatched and the UI/API surfaces the Settings repair message.
  Cover the backend dispatch boundary in integration tests and the stale selection state in the
  Settings E2E.
- **Dependency warning scenario:** GIVEN a profile is used by utility agents, WHEN the user tries to
  disable or delete it, THEN the existing warning/conflict dialog names the affected utility agents,
  cancellation leaves the profile unchanged, and confirmation leaves stale bindings that fail closed.
- **Mobile scenario:** GIVEN the phone Settings route, WHEN the user changes default/action profiles
  and opens custom edit, THEN the controls remain tappable, the card/dialog stay viewport-contained,
  and `document.scrollWidth <= document.clientWidth`. **File:**
  `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`, `mobile-chrome` project.
- **Discoverability scenario:** GIVEN the Utility Agents page, WHEN the user reads the page and
  opens each profile selector, THEN the card order and explanatory descriptions are correct, the
  selected profile and every option show an agent icon, and typing filters by profile or agent name.
  **Files:** `apps/web/e2e/tests/settings/utility-agents.spec.ts`,
  `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`.

## Public documentation

Update the how-to/explanation content in `docs/public/developer-tools.md` and
`docs/public/sessions-and-review.md` to describe profile selection, inherited permission behavior,
eligible-profile failures, and recovery. Update `docs/features.md` terminology. Audit
`docs/public/plugins-authoring.md` and `docs/public/plugins-manifest.md`; keep their plugin-facing
utility-agent ID contract but explain that execution uses the utility agent's effective profile.
Validate public links and navigation without adding a new public page.

## Verification Results

Pending. Each task records exact commands and outcomes in its `## Results` section; synchronize
them here as implementation completes.

## Implementation Waves And Parallel Candidates

Wave 1:

- [ ] [task-01-persist-profile-bindings](task-01-persist-profile-bindings.md)

Wave 2 (parallel candidates after task 01; user authorization required):

- [ ] [task-02-profile-aware-one-shot-runtime](task-02-profile-aware-one-shot-runtime.md)
- [ ] [task-04-settings-profile-pickers](task-04-settings-profile-pickers.md)
- [ ] [task-07-profile-dependency-warnings](task-07-profile-dependency-warnings.md)

Wave 3:

- [ ] [task-03-update-utility-consumers](task-03-update-utility-consumers.md)

Wave 4:

- [ ] [task-05-utility-profile-e2e](task-05-utility-profile-e2e.md)
- [ ] [task-06-update-utility-documentation](task-06-update-utility-documentation.md)

Wave 5 (settings discoverability follow-up):

- [x] [task-08-utility-settings-discoverability](task-08-utility-settings-discoverability.md)

Execution remains sequential in the primary conversation by default. Tasks 02, 04, and 07 own
disjoint backend/frontend files after the task-01 contract lands; tasks 05 and 06 are also disjoint,
but no wave authorizes subagents without an explicit user request.

## Risks

- Legacy pairs can match several profiles with materially different permissions. The migration
  must count eligible matches and leave ambiguity unconfigured, never depend on row order.
- Host one-shot inference currently builds commands from agent-level inference config. Applying
  profile flags/prefix/environment must preserve the allowlisted executable and structured-argv
  security boundary.
- Profile secret resolution must not leak values into utility DTOs, call history, error messages, or
  logs.
- One-shot ACP currently has no interactive permission UI. Explicit allow/deny behavior must be
  tested so `auto_approve: false` cannot silently auto-approve or wait for five minutes.
- Review and plugin code currently pass raw agent/model pairs through narrow cycle-avoidance
  adapters; all adapters and fakes must change together or one consumer will keep bypassing the
  profile.
- A disabled or deleted saved profile must remain visible enough to repair but must never remain an
  executable fallback candidate.
- Dependency lookup errors must block profile disable/delete, and the binding-state field must keep
  ambiguous migration separate from intentional default inheritance.
