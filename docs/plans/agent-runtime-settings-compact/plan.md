---
created: 2026-10-02
status: implemented
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
  - REQ-AGENTS-RUNTIME-NOTIFY-002
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
legacy_specs: []
---

# Implementation Plan: Compact Agent Runtime Settings

## Overview

Move runtime policy below installed agents and make it a compact collapsed disclosure. Deliver the presentation, fragment navigation and desktop/phone regression coverage together in one sequential work order. This is a follow-up to the [original notification package](../agent-runtime-notifications/plan.md); notifications and automatic-update semantics retain their existing contracts. The user explicitly authorized implementation through merge on 2026-10-02.

## Follow-up contract

The [floating indicator removal package](../remove-agent-runtime-update-indicator/plan.md) implements AC-AGENTS-RUNTIME-NOTIFY-001.7 in place of retired 001.4. Indicator previews and positive indicator assertions below describe the earlier implementation, not the current result. The follow-up owns the shared desktop/phone helper changes; historical verification results and completed work-order statuses remain delivery evidence for this package.

## Scope

### In scope

- AC-AGENTS-RUNTIME-NOTIFY-001.5 and 001.6: section ordering, collapsed entry, compact rows, fragment reveal, and draft retention.
- Preserve AC-AGENTS-RUNTIME-NOTIFY-001.1, 001.3, 001.4 and 002.7 through existing notification, ownership, save/reload and recovery flows.
- Focused desktop and phone rendered proof and the location/disclosure instruction in the public agents guide.

### Out of scope

- Backend checks, notifications, indicator presentation, permissions, policy persistence, version selection and update scheduling changes.
- Shared Settings component redesign, new overlays and dependency changes. Delivery through PR checks and merge is authorized.

## Technical approach

Move `AgentRuntimePolicies` below `InstalledAgentsSection` in `apps/web/app/settings/agents/page.tsx`. Use `SettingsGroup`'s existing disclosure with a closed initial state. Correct the shared SettingsGroup chevron scope to its owning details element: rendered verification found that summary-owned group state never rotates the expanded icon. This is a presentation correction without a shared API redesign. Keep children mounted so `useRuntimeAutoUpdatePolicy` retains its contributors, authoritative baseline and unsaved drafts; closed heading keeps its inherited dirty badge.

In `agent-runtime-policies.tsx`, move the section description into the expanded body and render automatic help once. Present each runtime with name/version information, muted source/ownership, explicit management action and labelled automatic switch where supported. Preserve unknown/unavailable and external/fallback distinctions and show outcomes only when present. Keep inactive registrations behind their current additional disclosure.

Register row fragments with `useSettingsTargetRegistration`. Register the group fragment on a title wrapper inside the summary, preserving `id="runtime-updates"` on the section without duplicate target registration. The existing `SettingsTargetProvider` and `revealSettingsTarget` open ancestor disclosures, focus and scroll even when status rows arrive after navigation. No custom hash event lifecycle is needed.

Phone composition uses a single column in the existing Settings page scroller, source wrapping, full-width actions and an inline labelled policy switch. Existing `settingsActionClassName` and switch touch wrappers preserve 44px targets; fine-pointer desktop actions stay 28px. The curated direct-navigation exemplar is `kanban-with-preview.tsx`; nearest local controls are `InstalledAgentCard`, `SettingsGroup`, and the existing managed version drawer. Persistent policy stays inline; temporary version selection retains its drawer. No additional safe-area or scroll owner is introduced.

Update the **Runtime notifications and automatic updates** reference in `docs/public/agents-and-profiles.md` during implementation to explain bottom placement, expansion and automatic reveal from notifications. Reuse existing localized copy; any necessary new copy must update all locale catalogs and pseudo output.

## ASCII UI preview

### UI-01: Current section order (source verified)

```text
Agents                              [Install agents]
Agent runtime updates
  Expanded runtime rows and repeated help
Installed agents
  Agent cards and profiles
```

### UI-02: Ordinary visit, desktop and phone

```text
Agents                              [Install agents]
Installed agents
  Agent cards and profiles

> Agent runtime updates
```

### UI-03: Expanded desktop or runtime notification entry

```text
v Agent runtime updates
  Shared explanation and automatic-update help
  Claude  Selected: 0.62.0  Latest: 0.64.0
                        Automatic updates [off] [Manage versions]
  @agentclientprotocol/claude-agent-acp | Kandev-managed
  [Retained outcome when present]
  -----------------------------------------------------------
  Kimi    Observed: 1.0.0   Latest: 2.0.0 [Manual update guidance]
  github:MoonshotAI/kimi-cli | Managed externally
  Manual management limitation
  > Other registrations (N)
```

### UI-04: Expanded phone

```text
v Agent runtime updates
  Shared explanation and help
  Claude
  Selected: 0.62.0   Latest: 0.64.0
  @agentclientprotocol/claude-agent-acp
  Kandev-managed
  Automatic updates                        [off]
  [Manage versions                             ]
  [Retained outcome when present]
  ---------------------------------------------
  Kimi
  Observed: 1.0.0   Latest: 2.0.0
  github:MoonshotAI/kimi-cli | Managed externally
  Manual management limitation
  [Manual update guidance                      ]
  > Other registrations (N)
```

UI-02 through UI-04 cover AC-AGENTS-RUNTIME-NOTIFY-001.5 and 001.6 and preserve 001.1, 001.4 and 002.7. Structure, bottom placement, collapsed ordinary entry, revealed notification destination, explicit unknown states, retained dirty state and reachable controls are required. Spacing and example data are illustrative. The page owns vertical scrolling; no nested list scrollbar. Inactive-target navigation opens both disclosures. Long source identifiers wrap; failed outcomes remain visible after expansion. The summary and all phone/coarse-pointer actions have at least 44px active targets.

## Tests

- Extend `agent-runtime-policies.test.tsx` with real Settings disclosure/provider behavior: ordinary collapsed entry, keyboard expansion, group/row fragments, late and inactive targets, draft retention after closing/reopening, and unchanged external/fallback actions. These exercise 001.5 and 001.6, and preserve 001.1 and 002.7.
- Retain `use-runtime-auto-update-policy.test.ts` for save/discard/failure behavior (002.7). Do not duplicate its persistence tests just for styling.
- Existing notification helpers retain named notice, indicator, saved consent/reload, unknown version, outcome recovery, and version dialog/drawer assertions (001.1, 001.3, 001.4, 002.7).

## E2E tests

Extend `e2e/tests/settings/agent-runtime-notifications.spec.ts` and `mobile-agent-runtime-notifications.spec.ts`, reusing their shared fixture/helper, with ordinary Agents entry (closed and below installed cards), expansion/collapse and notification/indicator auto-reveal. Cover initial hash, same-page hash after manually collapsing, and an inactive target. Preserve existing policy save/reload, unknown, fallback and outcome assertions. Assert phone summary/switch/action hitboxes, wrapping, no horizontal overflow and visible target after navigation; include a narrow fine-pointer viewport and 767/768px boundary checks where the new layout differs. Capture expanded desktop/phone screenshots through `prCapture` and compare their structure with UI-03/UI-04. Retain the managed runtime update suites as compatibility proof.

## Work orders

- [x] [Task 01: Compact runtime settings and navigation](task-01-compact-runtime-settings.md) (`done`, sequential, no dependencies).

## Verification results

Design checks passed on 2026-10-02: `python3 scripts/list-docs.py validate` (340 decisions, 1292 specifications), `python3 scripts/lint-spec-files.test.py` (36 tests), `python3 scripts/lint-spec-files.py --all`, and `git diff --check`. The repository PR documentation validator's `validateCoverage` accepted the new work order against its planned runtime component path and resolved both requirements, all acceptance criteria and the paired design (`status: covered`). Node 24.21.0 from the installed mise runtime ran that check because Node was absent from this shell's PATH. `git status --short` confirms the new plan/order are untracked and all edits are unstaged.

Implementation is complete. Final targeted checks passed: 17 unit tests, changed-file lint, typecheck, i18n checks, 20 desktop and 7 phone E2E tests with fresh builds, 62 public-doc validator tests, 47 published-page checks, specification validation and diff checks. [Task 01 results](task-01-compact-runtime-settings.md#results) record the exact validation and capture procedure. Screenshots confirm ordinary collapsed entry and the compact desktop/phone expanded composition. External PR/review/merge evidence is tracked in the Kandev task plan.

## Risks

- Registering the enclosing section as the group target would not open its own child disclosure; register inside the summary instead.
- Unmounting closed content would lose drafts/contributors and prevent delayed fragment reveal.
- Existing notification helpers assume visible rows after following a notice; auto-reveal must preserve those assertions rather than weakening them.
- Reducing text must preserve runtime identity, unknown state and host/fallback ownership distinctions.
