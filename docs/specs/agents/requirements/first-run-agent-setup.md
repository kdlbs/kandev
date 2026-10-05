---
status: active
system: agents
created: 2026-10-05
owners:
  - kandev
---

# First-run agent setup requirements

## Overview

First-run setup lets a user choose a detected agent's starting model and whether
to use its terminal interface. The full agent profile page owns subsequent
configuration. Agents owns this capability because discovery and edits apply to
saved agent profiles, even though the tour presents them.

This is a quick setup surface, not the full profile configuration editor defined
by [profile capability discovery](profile-capability-discovery.md) and
[dynamic provider options](dynamic-provider-options.md).

## Requirements

### REQ-AGENTS-FIRST-RUN-SETUP-001: Focused initial configuration

**Intent:** Users can finish initial setup without understanding advanced profile settings.

#### Acceptance criteria

- **AC-AGENTS-FIRST-RUN-SETUP-001.1:** Expanding an installed agent shall show one
  model-list selector and a CLI passthrough toggle when the agent supports
  passthrough. Unsupported passthrough shall not appear as an editable setting.
- **AC-AGENTS-FIRST-RUN-SETUP-001.2:** The expanded row and its model picker shall
  omit permission controls, Auto Approve, mode selection, provider configuration
  options, fallback and exact-model policy, CLI flags, environment variables,
  command prefixes, and advanced settings disclosures.
- **AC-AGENTS-FIRST-RUN-SETUP-001.3:** Dynamic-model refresh shall be a compact icon action
  beside the selector, with a localized accessible name and visible busy state.
  At the standard font size, selector and refresh action shall measure 28px high
  on fine-pointer desktop and at least 44px on coarse-pointer surfaces.
  Static catalogs shall omit the no-op refresh. The supported passthrough toggle
  shall have an associated accessible label and a 44px coarse-pointer activation
  target, which may be its clickable row rather than the visual switch.
- **AC-AGENTS-FIRST-RUN-SETUP-001.4:** Help shall direct users to Settings > Agents
  for further configuration. The existing read-only warning about default
  Auto Approve behavior shall remain; hiding its setting shall not change
  permission defaults.
- **AC-AGENTS-FIRST-RUN-SETUP-001.5:** The agent list shall scroll within a
  viewport-contained tour while navigation actions remain reachable, including
  when an agent is expanded on a short desktop viewport.

### REQ-AGENTS-FIRST-RUN-SETUP-002: Immediately usable model discovery

**Intent:** Choosing a model requires no preliminary manual refresh.

#### Acceptance criteria

- **AC-AGENTS-FIRST-RUN-SETUP-002.1:** Opening a saved dynamic-model agent in the
  tour shall automatically discover choices for that profile's saved launch
  context. The selector shall become usable after discovery succeeds without
  requiring Refresh. Discovery shall retain the saved model label while loading.
- **AC-AGENTS-FIRST-RUN-SETUP-002.2:** For supported OpenCode installations that
  can provide ACP capabilities, model discovery shall return advertised choices
  without a failure caused by Kandev's built-in logging arguments. Native and
  managed runtime selection shall retain their existing precedence.
- **AC-AGENTS-FIRST-RUN-SETUP-002.3:** A failed, empty, authentication-required, or
  unavailable discovery shall show a localized explanation and an explicit
  retry or Settings recovery path. It shall preserve the selected model and
  shall not substitute unrelated agent-wide choices for the profile catalog.
  Probe failures shall be announced. Authentication recovery shall link directly
  to the saved profile's settings without saving the tour draft.
- **AC-AGENTS-FIRST-RUN-SETUP-002.4:** Selecting an advertised model shall update
  the draft and close the list. It shall not expose or require dependent model
  options. Refresh alone shall not select another model or mark the draft dirty.
- **AC-AGENTS-FIRST-RUN-SETUP-002.5:** Late discovery responses from a previously
  expanded agent, another profile, or an earlier refresh shall not overwrite
  current choices or edits. An agent-wide failed status shall not override a
  successful discovery for the expanded profile.
  The row shall carry the matching profile error with its status; unsupported
  discovery shall not appear as healthy when collapsed.
- **AC-AGENTS-FIRST-RUN-SETUP-002.6:** Discovery shall not save profiles, change
  permission settings, create task sessions, send prompts, install packages, or
  activate another runtime.

### REQ-AGENTS-FIRST-RUN-SETUP-003: Narrow saves and responsive continuity

**Intent:** Quick setup changes only the fields the user deliberately edits.

#### Acceptance criteria

- **AC-AGENTS-FIRST-RUN-SETUP-003.1:** Next and completion shall save only changed
  model and CLI passthrough values. Hidden profile settings, including permissions,
  CLI flags, environment, prefix, modes, provider options, and fallback policy,
  shall retain their stored values.
- **AC-AGENTS-FIRST-RUN-SETUP-003.2:** Skip shall discard unsaved tour edits and
  retain the existing browser-local completion behavior. Discovery and opening
  an agent shall not create a dirty save.
- **AC-AGENTS-FIRST-RUN-SETUP-003.3:** A failed save shall retain the current step
  and draft, show the existing error handling, and allow retry. Concurrent
  navigation actions shall not submit duplicate saves.
- **AC-AGENTS-FIRST-RUN-SETUP-003.4:** Agent setup shall follow the existing
  [first-run availability contract](../../ui/requirements/first-run-dialog-availability.md).
  A larger-screen draft shall survive a phone-width visit and the return to a
  larger screen without an automatic save or completion.
- **AC-AGENTS-FIRST-RUN-SETUP-003.5:** Full profile settings shall retain their
  existing advanced controls, dependent-option resolution, and saved-context
  refresh behavior on desktop and phone.

## Out of scope

- Enabling the first-run tour on phones or redesigning later tour steps.
- Changing profile permission defaults, exact-model policy, live task model
  selection, or remote-executor discovery.
- Provider version detection, new runtime flags, package updates, or a new
  discovery API, cache, or persistence schema.
- Removing the existing read-only permission warning.

## Implementation plans

- [First-run agent setup](../../../plans/first-run-agent-setup/plan.md)
