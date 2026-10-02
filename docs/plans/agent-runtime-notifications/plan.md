---
created: 2026-10-01
status: in_progress
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
  - REQ-AGENTS-RUNTIME-NOTIFY-002
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
legacy_specs: []
---
# Implementation Plan: All-agent Runtime Update Awareness and Automation

## Overview

Extend the existing runtime status/activation boundaries, then integrate durable opt-in policy and notification delivery, and finally expose app-wide localized controls. The user authorized unattended implementation and normal-policy delivery through merge in this task; the usual design handoff pause is waived. Work stays sequential in this session.

## Scope

All registered agents receive truthful capability/status coverage. Eligible managed packages receive opt-in automatic activation. Native/vendor mechanisms receive verified source checks where available and manual guidance where safe activation cannot be established. Existing sessions, remote ownership, exact manual selection, rollback, and defaults remain protected. Model discovery and vendor configuration migrations are excluded.

## Technical approach

- Add trusted runtime update metadata/resolution in agents and extend the cached status projection, preserving source-level single-flight and cancellation.
- Persist policies/outcomes in SystemSettings, reuse AgentUpdateJobStore candidate activation and maintenance admission, add one cancellable background scheduler.
- Reuse notifications.Service delivery preferences and claims, extend structured local payloads and app toast navigation.
- Share frontend status state, add a persistent app indicator and per-runtime policy controls with Settings save coordination.
- Update [all-agent coverage](coverage.md), public agents documentation, requirements/design lifecycle, and scoped guidance only when contracts change.

## ASCII UI preview

### UI-01: Runtime awareness outside Settings

Desktop and phone use the same notification/indicator entry; phone action height is at least 44px and stays above safe-area navigation.

```text
[Agent runtime updates: 2] [View updates]
Toast: Gemini (@google/gemini-cli) 0.62.0 available
       [Review runtime]
```

### UI-02: Settings runtime policy (desktop)

```text
Agent runtime updates
Claude | @agentclientprotocol/claude-agent-acp | Kandev-managed
Current 0.81.2  Latest 0.82.0   [Manage versions]
Automatic updates [off]  Future launches only
Cursor | cursor-agent | Externally managed
Current unknown  Latest unknown   [Vendor update guidance]
Automatic updates unavailable. Installation is externally managed.
[Save changes] [Discard]
```

### UI-02: Settings runtime policy (phone)

```text
Agent runtime updates
Claude
@agentclientprotocol/claude-agent-acp
Kandev-managed
Current 0.81.2 / Latest 0.82.0
[Manage versions                    ]
Automatic updates               [off]
Future launches only

Cursor
Externally managed. Version unknown.
[Vendor update guidance             ]
[Save changes                       ]
```

Rows wrap within the page scroller; managed version browsing uses the existing phone drawer. Policy save uses the shared Settings coordinator. Failure rows show old/target versions and recovery. Ordering, clear ownership, and reachable actions are required; spacing is illustrative. Covers AC-AGENTS-RUNTIME-NOTIFY-001.4 and 002.7.

## Tests and E2E tests

Task 01: capability coverage and cached status/source tests. Task 02: policy persistence, candidate admission/outcomes, delivery preferences/deduplication, running-session invariants. Task 03: hook/API tests and desktop/mobile agent runtime notification specs, plus existing runtime update specs. Task 04: documentation and specification validators. Exact commands are owned by each work order.

## Work orders

- [x] [Task 01: Capabilities and sources](task-01-capabilities.md)
- [x] [Task 02: Automatic policy and delivery](task-02-automation.md)
- [x] [Task 03: App-wide UI and localization](task-03-ui.md)
- [ ] [Task 04: Documentation and delivery](task-04-delivery.md)

## Verification results

Implementation and local acceptance checks are complete; delivery gates remain in progress. Results and exact commands are recorded in each work order. Workspace dependencies installed with pnpm install --frozen-lockfile. Initial base: 08e4ffdb99caf40b0df5baa67b29cf4313188f15; integrated main: 68542f03983a56b9c9c42fd1afed10842e1beff0. Expanded issue body and jcfs assignment verified. Open PR #4014 overlaps OpenCode selection; #3940 overlaps host discovery. Neither is a dependency or current-main contract.

## Risks

The previous native OpenCode update path inferred global npm ownership from PATH; the reviewed capability boundary now uses vendor guidance for that external installation. Existing managed selections and remote executor command resolution are preserved. Native version metadata can be non-SemVer or omit the runtime package version, so comparison must remain unknown. Opt-out/manual selection can race candidate staging, requiring an activation guard. Release source changes must not inherit consent. Shared settings and notification stores must preserve per-user notification ownership and install-wide permission boundaries.

CI remediation integrates current main after its existing scoped Git-status fix was reproduced against the old handler. Root diff enrichment must compare the root tracker rather than the latest child tracker. This is upstream behavior, with no new runtime-update requirement or ownership change. Exact post-integration CI and merge evidence remains in the Kandev task plan.

The completed CI retry audit exposed two stale E2E assertions: the restored right pane must expose Files when selected, and phone touch-target geometry must ignore browser floating-point noise before checking the unchanged 44px minimum. Remediation is limited to those test assertions and this delivery record; public runtime behavior and screenshots remain unchanged. The full affected specs and the six desktop retry cases are validated without retries before another exact-head CI run.

A further retry audit led to narrow test-only fixes for asynchronous table geometry, e2e cursor seeding after active-view tracking stops, settled fixture turns and initial Git/Dockview hydration before selecting Files, and review progress when live files are added. The zero-reviewed and file-tree/error-retry behavior checks remain intact. Current-main atomic notification-provider saves were composed and race-tested with the runtime notification service. Delivery results and remaining exact-head gates are recorded externally.
