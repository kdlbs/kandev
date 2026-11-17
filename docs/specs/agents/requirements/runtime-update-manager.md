---
status: active
system: agents
created: 2026-11-10
owners:
  - Kandev
---

# Agent runtime update manager requirements

## Overview

Operators need a visible place to inspect and update agent runtimes without searching below their profiles.
Agents owns this contract because runtime identity, activation, policy, and update outcomes belong to Agents.

This contract replaces the bottom disclosure defined by AC-AGENTS-RUNTIME-NOTIFY-001.5 and 001.6 after delivery.
It also replaces the expanded-section destination in AC-AGENTS-RUNTIME-NOTIFY-003.3.
Background checks, notification delivery, runtime ownership, and validated activation retain their existing contracts.

## Terminology

- **Manager:** The runtime overview and agent details within one desktop dialog or phone drawer.
- **Available count:** Known updates with a supported individual update action. External guidance, unknown checks, unavailable runtimes, and busy agents do not contribute.
- **Bulk candidate:** An available runtime that supports an exact, validated update within its current runtime family and source.
- **Reviewed target:** The exact version displayed and selected before the operator starts an update.

## Requirements

### REQ-AGENTS-RUNTIME-MANAGER-001: Visible runtime management

**Intent:** Operators can discover updates and manage a runtime without navigating through unrelated profiles.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-MANAGER-001.1:** Settings > Agents shall show a Runtime updates button beside Install agents on desktop. Phones shall show a visible runtime button below the page heading. The button shall remain available with no updates. Its badge shall show the available count without presenting unknown checks as up to date.
- **AC-AGENTS-RUNTIME-MANAGER-001.2:** The manager shall show available updates first. Each row shall identify the agent, current and available versions, and supported actions. Up-to-date, unknown, externally managed, and unavailable registrations shall remain accessible with explicit states. Shared explanations shall appear once. Runtime source and retained outcomes shall remain inspectable.
- **AC-AGENTS-RUNTIME-MANAGER-001.3:** Manage versions and installed-card update controls shall open that agent's details directly. The overview, agent details, and Back action shall share one overlay. Opening details shall not scroll to another card or open a second update overlay.
- **AC-AGENTS-RUNTIME-MANAGER-001.4:** Pinned runtimes shall retain version browsing, rollback, repair, and return-to-default actions. Harness-owned updaters shall retain their target-free action and advisory stable reference. External installations shall retain verified guidance. Native host versions and managed fallback versions shall remain distinct. OpenCode migration shall remain an explicit individual action.
- **AC-AGENTS-RUNTIME-MANAGER-001.5:** Automatic-update controls shall remain drafts until explicit Save changes. The manager shall provide reachable Save and Reset controls while drafts exist. Closing, reopening, or navigating within the manager shall preserve drafts on the same settings route. Failed saves and edits during saving shall retain the existing manual-save behavior.
- **AC-AGENTS-RUNTIME-MANAGER-001.6:** Phones shall use a tall bottom drawer with internal overview-to-detail navigation. The header and primary footer shall remain visible around one scrolling body. The surface shall fit the dynamic viewport and safe areas without horizontal document overflow. Phone and coarse-pointer actions shall have actual targets of at least 44px. Ordinary desktop controls shall remain 28px.
- **AC-AGENTS-RUNTIME-MANAGER-001.7:** Desktop and phone users shall reach every supported action through visible controls and keyboard navigation. Closing shall return focus to the actual opener, or the runtime header button if the opener disappeared. Copy, plural counts, accessible names, and asynchronous results shall use every shipped locale.

### REQ-AGENTS-RUNTIME-MANAGER-002: Navigation and authoritative state

**Intent:** Existing notices and settings links remain useful as the runtime presentation changes.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-MANAGER-002.1:** The existing runtime-updates fragment shall open the overview. A runtime-update-<agent> fragment shall open that runtime's details, including manual or unavailable states. Installed-agent-<agent> links shall retain their installed-card destination. Initial navigation, same-route requests, and notification navigation shall use the same destinations.
- **AC-AGENTS-RUNTIME-MANAGER-002.2:** A destination requested before its runtime metadata arrives shall open after that metadata arrives. Repeated explicit requests shall reopen a dismissed manager. Missing or unrecognized runtime identities shall never select or update a different runtime.
- **AC-AGENTS-RUNTIME-MANAGER-002.3:** Closing the manager shall preserve local policy drafts and an active bulk run. Returning from details shall preserve overview selection and scroll position. A viewport change shall preserve the selected runtime and draft state without opening two surfaces.
- **AC-AGENTS-RUNTIME-MANAGER-002.4:** Loading and failed checks shall remain explicit. A failed refresh shall retain the last successful information without presenting it as a fresh check. Failed previews shall disable affected update actions and offer retry. Unknown versions shall retain individual recovery actions where supported. Refresh shall use the existing shared release-check policy.

### REQ-AGENTS-RUNTIME-MANAGER-003: Reviewed bulk runtime updates

**Intent:** Operators can update several eligible runtimes without repeating the same review and approval flow.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-MANAGER-003.1:** Bulk selection shall include only enabled, available, Kandev-owned pinned runtimes with a known newer version and a supported validated update. Active installs or updates shall exclude the affected agent. External installations, harness-owned updaters, managed fallbacks, migrations, rollback, repair, and unknown checks shall retain individual paths.
- **AC-AGENTS-RUNTIME-MANAGER-003.2:** The overview shall display exact reviewed targets before approval and initially select eligible candidates. Selecting all shall show Update all (N). A subset shall show Update selected (N). An empty selection shall disable approval. New targets or runtimes shall not silently enter an existing selection. Required compatibility information shall remain visible before approval.
- **AC-AGENTS-RUNTIME-MANAGER-003.3:** One approval shall attempt each selected target once through the normal per-runtime validation path. Changed runtime identity, family, source, current selection, or target shall require a new review for that row. One failed admission shall not prevent other valid rows from proceeding. Repeated clicks shall not create duplicate submissions. An existing different maintenance job shall not count as the requested update.
- **AC-AGENTS-RUNTIME-MANAGER-003.4:** Each row shall show submission, job progress, success, failure, or an unconfirmed result. Accepted or queued shall not mean succeeded. Failures shall retain their cause and available details. Retry failed shall review and attempt only failed rows. Successful rows shall remain successful. Activation failure shall preserve the previous managed version. Capability-refresh failure shall remain distinct from activation failure.
- **AC-AGENTS-RUNTIME-MANAGER-003.5:** Closing the overlay or changing routes in the same authenticated browser session shall not cancel approved submissions or admitted jobs. Reload shall recover retained server jobs and outcomes without replaying unconfirmed requests or promising a durable batch queue. Missing results shall remain unconfirmed. Browser or backend restart shall not claim aggregate success without evidence.
- **AC-AGENTS-RUNTIME-MANAGER-003.6:** Runtime updates shall require the existing agent-configuration permission. Read-only users shall retain inspection and guidance. Update actions shall not implicitly save or reset policy drafts. A selected runtime with a dirty policy shall require Save or Reset before update approval. Admitted manual updates shall retain the existing rule that disables automatic updates. Bulk approval shall never enable automatic updates or alter running sessions.

## Out of scope

- A new updater, server batch API, persistent batch queue, or atomic all-agent transaction.
- Automatic retries, automatic rollback, runtime-family migration through Update all, or arbitrary packages and commands.
- A floating app-wide update indicator, profile-page runtime controls, or replacement of external package managers.
- Changes to notification timing, release-check caching, executor installations, or the settings-wide manual-save contract.

## Design and delivery

- [System design](../system-design/runtime-update-manager.md)
- [Implementation plan and previews](../../../plans/agent-runtime-update-manager/plan.md)
- [Existing activation contract](runtime-updates.md)
- [Existing automation contract](runtime-update-notifications.md)
- [Settings manual save](../../ui/requirements/settings-manual-save.md)
