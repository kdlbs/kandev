---
status: active
system: agents
created: 2026-09-29
updated: 2026-10-05
owners:
  - kandev
---

# Profile capability discovery requirements

## Overview

An agent profile can select a different runtime through its environment, CLI flags, or command prefix.
Its model picker must describe that runtime when the user refreshes discovery.
Agents owns this contract because it owns profiles and provider capabilities.

This capability extends the launch context of discovery.
[Dynamic provider options](dynamic-provider-options.md) continues to own model-dependent option interpretation and reconciliation.
[Runtime updates](runtime-updates.md) continues to own managed package selection and activation.

## Terms

- **Launch settings:** Profile environment entries, CLI flags, and command prefix, combined with the effective managed runtime selection.
- **Draft:** The current editor values, including changes that the user has not saved.
- **Host discovery:** A temporary provider process on the Kandev host. It does not predict remote executor capabilities.

## Requirements

### REQ-AGENTS-PROFILE-DISCOVERY-001: Profile launch settings in discovery

**Intent:** A profile advertises choices from the same launch settings that its host session uses.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-DISCOVERY-001.1:** When an editor opens a saved concrete profile, discovery shall use its saved launch settings and effective managed runtime.
- **AC-AGENTS-PROFILE-DISCOVERY-001.2:** When the user refreshes an edited or new concrete profile, discovery shall use the complete current draft without saving it.
- **AC-AGENTS-PROFILE-DISCOVERY-001.3:** Both model-list discovery and model-option resolution shall apply the same validated environment entries, ordered CLI arguments, and command prefix.
- **AC-AGENTS-PROFILE-DISCOVERY-001.4:** Discovery shall enforce the existing authority for profile execution settings and secret references. Invalid or inaccessible inputs shall fail before provider startup.
- **AC-AGENTS-PROFILE-DISCOVERY-001.5:** Discovery shall not create a Kandev task session, send a model prompt, save a profile, or alter a live session.
- **AC-AGENTS-PROFILE-DISCOVERY-001.6:** Agent-wide discovery shall retain its default launch context. Profile observations shall not replace agent-wide results or another profile's results.

### REQ-AGENTS-PROFILE-DISCOVERY-002: Correct results across context changes

**Intent:** Discovery results remain attributable to the launch settings that produced them.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-DISCOVERY-002.1:** Profiles with different launch settings shall not share capability results. Identical authorized contexts can reuse bounded discovery work.
- **AC-AGENTS-PROFILE-DISCOVERY-002.2:** Refresh shall read current secret bindings and the effective runtime selection. It shall not reuse results from an older value or runtime generation.
- **AC-AGENTS-PROFILE-DISCOVERY-002.3:** When the user changes launch settings during discovery, an older response shall not replace the current context's results.
- **AC-AGENTS-PROFILE-DISCOVERY-002.4:** Invalid settings, missing runtimes, unsupported discovery, and provider failures shall remain visible. Kandev shall not silently retry with default launch settings.
- **AC-AGENTS-PROFILE-DISCOVERY-002.5:** Responses, shared caches, diagnostic logs, and metrics shall not disclose secret values or raw launch settings containing credentials.

### REQ-AGENTS-PROFILE-DISCOVERY-003: Profile editor refresh behavior

**Intent:** Users can refresh a profile without losing their saved selection or unfinished edits.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-DISCOVERY-003.1:** A launch-setting edit shall mark discovery stale and expose Refresh. Typing shall not start provider processes with each partial edit.
- **AC-AGENTS-PROFILE-DISCOVERY-003.2:** Refresh shall show progress and then either matching choices or a retryable error. Stale choices shall not appear authoritative. When discovery reports that provider authentication is required or the provider is not installed, the profile editor shall retain its existing login and host-terminal recovery actions alongside refresh.
- **AC-AGENTS-PROFILE-DISCOVERY-003.3:** Refresh, failure, and a missing selected model shall preserve draft and saved selections. Existing save and model-option reconciliation rules shall remain effective.
- **AC-AGENTS-PROFILE-DISCOVERY-003.4:** Desktop and phone editors shall offer the same refresh, selection, and retry outcomes through localized, keyboard-accessible and touch-accessible controls.
- **AC-AGENTS-PROFILE-DISCOVERY-003.5:** The profile editor's model-list refresh
  shall use a compact icon action aligned with its selectors, with a localized
  accessible name and busy feedback. At the standard font size it shall measure
  28px square on fine-pointer desktop and at least 44px square on phone or
  coarse-pointer surfaces, following the shared
  [control-sizing contract](../../ui/requirements/control-sizing.md). Error,
  retry, and authentication recovery shall remain available without enlarging
  the desktop refresh action.

## Out of scope

- Updating the Codex CLI bundled inside an ACP bridge or changing runtime updater labels.
- Selecting a runtime binary automatically or installing packages during profile refresh.
- Remote executor probes, workspace environment overlays, repository setup scripts, and executor authentication parity.
- Changing live session model selection, exact-model policy, provider gateway authentication, or dynamic-profile routing.
- Changing agent-wide or workflow-only discovery when no concrete profile context is available.

Host discovery remains an editing aid. Executor startup retains its existing model-selection authority.

## Implementation plans

- [Profile capability discovery](../../../plans/profile-capability-discovery/plan.md)
- [First-run agent setup and compact profile refresh](../../../plans/first-run-agent-setup/plan.md)
