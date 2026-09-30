---
id: "07-desktop-and-phone"
title: "Expose cloud configuration and task controls on desktop and phone"
status: complete
wave: 7
depends_on:
  - "06-observation-and-results"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-001
  - REQ-EXECUTORS-CURSOR-CLOUD-002
  - REQ-EXECUTORS-CURSOR-CLOUD-004
  - REQ-EXECUTORS-CURSOR-CLOUD-006
  - REQ-EXECUTORS-CURSOR-CLOUD-003
acceptance_criteria:
  - AC-EXECUTORS-CURSOR-CLOUD-001.1
  - AC-EXECUTORS-CURSOR-CLOUD-001.2
  - AC-EXECUTORS-CURSOR-CLOUD-001.3
  - AC-EXECUTORS-CURSOR-CLOUD-001.4
  - AC-EXECUTORS-CURSOR-CLOUD-002.3
  - AC-EXECUTORS-CURSOR-CLOUD-002.5
  - AC-EXECUTORS-CURSOR-CLOUD-004.2
  - AC-EXECUTORS-CURSOR-CLOUD-004.3
  - AC-EXECUTORS-CURSOR-CLOUD-004.4
  - AC-EXECUTORS-CURSOR-CLOUD-006.1
  - AC-EXECUTORS-CURSOR-CLOUD-006.2
  - AC-EXECUTORS-CURSOR-CLOUD-006.3
  - AC-EXECUTORS-CURSOR-CLOUD-006.4
  - AC-EXECUTORS-CURSOR-CLOUD-001.5
  - AC-EXECUTORS-CURSOR-CLOUD-004.5
  - AC-EXECUTORS-CURSOR-CLOUD-003.1
  - AC-EXECUTORS-CURSOR-CLOUD-003.2
  - AC-EXECUTORS-CURSOR-CLOUD-003.3
  - AC-EXECUTORS-CURSOR-CLOUD-003.4
  - AC-EXECUTORS-CURSOR-CLOUD-003.5
  - AC-EXECUTORS-CURSOR-CLOUD-001.6
  - AC-EXECUTORS-CURSOR-CLOUD-001.7
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
---

# Task 07: Expose cloud configuration and task controls on desktop and phone

## Summary

Users can configure and operate the complete normal cloud task flow on desktop and phone using UI-01 through UI-04.
Use TDD for changed logic. Keep results pending until the listed checks pass.

## In scope

- Extend the profile editor with secret reference, callback, model, and connection status through shared hooks and API clients.
- Add eligible cloud choices and repository/ref disclosure to task start. PR creation defaults off.
- Render conversation, stop/follow-up controls, provider links, remote results, and recovery states without mounting unavailable workspace panels.
- Add the unknown-submission resolution surface and its explicit retry acknowledgment. Share domain state across desktop and phone.
- Use the named mobile exemplars, safe-area geometry, accessible controls, and translations in all required catalogs.

- Create isolated Cursor HTTP/SSE fixtures and dedicated cursor-cloud and cursor-cloud-mobile projects. Exclude those specs from chromium/mobile-chrome.
- Restart the dedicated backend from baseline with feature=true and its allocated mock URL; restore baseline before fixture teardown. Never enable the feature in ordinary projects.
- Own rendered UI-04 components and unit tests for all states, including submission unknown. Work order 08 alone owns failure injection and end-to-end unknown-submission resolution.
- Register both projects in CI runner and shard-manifest discovery, and test enumeration/exclusion so no cloud spec is silently skipped or duplicated.

- Gate Agents-page type discovery on an accessible, saved cloud executor with valid required configuration. Keep executor setup independent of agent discovery.
- Cover absent, incomplete, newly saved, last removed, inaccessible, and temporarily disconnected executor states. Preserve saved agent profiles and history.

## Out of scope

- Work assigned to later tasks, unrelated refactors, and release promotion.
- Paid cloud execution during automated tests.

## Acceptance

- Users can configure and operate the complete normal cloud task flow on desktop and phone using UI-01 through UI-04.
- Capability gating suppresses workspace requests and preserves saved layouts, while server-derived state controls cancellation and recovery actions.
- New text is localized; phone controls meet touch geometry and both layouts preserve keyboard/focus behavior.

## ASCII UI preview

The labels and grouping below are structural requirements; spacing and copy are illustrative.
All final user-facing copy uses localization. Shared state and permission logic serve both layouts.

### UI-00: Executor-first discovery

Entry: Agents page, on desktop and phone. Configure the executor through executor settings first.

```text
No configured executor:   Agents [existing agent types]
After executor is saved:  Agents [existing types] [Cursor Cloud]
Last executor removed:    Agents [existing agent types]
```

No placeholder Cursor Cloud agent card appears before configuration.
Temporary connection errors keep the configured type visible with its error state.
Map: AC-EXECUTORS-CURSOR-CLOUD-001.6 and -001.7.

### UI-01: Profile configuration, ready and error states

Entry: Settings > agent profile > executor configuration. Desktop uses the existing profile page.
Phone uses direct navigation with Back, one scrolling form, and a reachable Save action.

```text
Desktop                              Phone
+--------------------------------+   +--------------------------+
| Cursor Cloud                   |   | < Profiles  Cursor Cloud |
| API key [saved secret v]       |   | API key [saved secret v] |
| Callback [https://host/...   ] |   | Callback [https://...  ] |
| Model [provider model v]      |   | Model [provider model v] |
| [Test connection] Ready       |   | [Test connection]        |
|                        [Save] |   | Ready                    |
+--------------------------------+   |                   [Save] |
                                     +--------------------------+
Checking: Test shows progress; repeat clicks are disabled.
Error:    The failing field shows a readable error and a retry action.
No key:   Select or create a secret; starting work remains unavailable.
Billing:  A visible notice explains that profile users share the key owner's Cursor billing.
```

Map: AC-EXECUTORS-CURSOR-CLOUD-001.1 through -001.5 and -006.1 through -006.4.

### UI-02: Start a cloud task

Entry: existing task start surface. Phone choices use an inset picker drawer; the form remains a focused surface.

```text
[Executor: Cursor Cloud v] [Agent: Cursor Cloud v]
Repository: owner/repo     Published ref: [main v]
Uses published repository content. Local edits stay here.
[ ] Create a pull request automatically
[Cancel]                                    [Start]
```

Starting disables duplicate submission. Invalid profile, repository, model, or callback errors keep the form open.
Map: AC-EXECUTORS-CURSOR-CLOUD-002.1, -004.1, -004.4, and -006.1.

### UI-03: Active conversation and results

```text
Desktop
+-----------------------------------------------------------+
| Task / Cursor Cloud / Running    [Open in Cursor] [Stop]    |
+----------------------------------+------------------------+
| Conversation and tool activity    | Remote results         |
|                                  | Branch: cursor/...     |
| (one scroll region)              | PR: Open when present  |
+----------------------------------+------------------------+
| [Follow-up message                                    ]   |
|                                                [Send]     |
+-----------------------------------------------------------+

Phone
+----------------------------+
| < Tasks  Cursor Cloud  [v] |
| Running             [Stop] |
+----------------------------+
| Conversation               |
| Tool activity              |
| (one scroll region)        |
+----------------------------+
| [Message              ]    |
| [Results]           [Send] |
+----------------------------+
       safe-area clearance

Phone Results drawer
+----------------------------+
| Remote results         [x] |
| Branch: cursor/...         |
| [Open pull request]        |
| [Open in Cursor]           |
+----------------------------+
```

Headers and composer remain fixed within a dynamic-viewport layout. The conversation owns vertical scrolling.
Phone targets are at least 44px. Desktop ordinary controls retain 28px sizing.
Workspace panels and their requests are absent for cloud sessions; desktop layout preferences remain stored.
Map: AC-EXECUTORS-CURSOR-CLOUD-003.1, -004.2, -004.3, and -006.1 through -006.4.

### UI-04: Recovery and uncertain submission

```text
Reconnecting:      Activity connection lost. Retrying...
Cancelling:        Stopping remote work... [Send disabled]
History gap:       Some activity is unavailable. Saved history remains.
Submission unknown:
  Cursor may have received your message.
  [Open in Cursor] [Resolve submission] [Send disabled]

Resolve submission (desktop dialog / phone full-height surface)
  Remote runs: [candidate ID, start time, status]
  [Bind selected run]
  OR [ ] I understand retrying may start duplicate work
     [Retry this message]
```

Resolution uses one scroll owner and preserves focus on dismissal. Candidate binding requires server-side identity checks.
Retry is an explicit acknowledgment, never an automatic response to a timeout.
Map: AC-EXECUTORS-CURSOR-CLOUD-002.3, -002.5, -003.2 through -003.5, and -006.2.


## Verification

Run from the repository root. New test paths are implementation outputs, not tests available during this planning turn.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- components/settings/profile-edit/cursor-cloud-config.test.ts lib/cursor-cloud-capabilities.test.ts components/task/cursor-cloud-recovery.test.tsx)
(cd apps && pnpm --filter @kandev/web test -- e2e/scripts/run-e2e.test.ts e2e/scripts/cursor-cloud-projects.test.ts)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project cursor-cloud --list)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project cursor-cloud-mobile --list)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project cursor-cloud tests/session/cursor-cloud.spec.ts)
(cd apps/web && pnpm e2e:run --project cursor-cloud-mobile tests/session/mobile-cursor-cloud.spec.ts)
```

### Evidence mapping

- 001.1-001.4, 004.3: `components/settings/profile-edit/cursor-cloud-config.test.ts: cloud validation; lib/cursor-cloud-capabilities.test.ts: unavailable operations`.
- 002.3, 004.2, 004.4, 006.1-006.4: `e2e/tests/session/cursor-cloud.spec.ts and mobile-cursor-cloud.spec.ts: ordinary task flow, results, geometry`.
- 002.5, 003.1-003.5, 006.2: `components/task/cursor-cloud-recovery.test.tsx: rendered recovery states only`. Failure injection and unknown-resolution E2E belong to task 08.

- 001.6-001.7: `internal/backendapp/cursor_cloud_admission_test.go: TestCursorCloudAgentDiscovery` and desktop/phone `cursor-cloud.spec.ts` discovery scenarios.

## Files likely touched

- `apps/web/components/settings/profile-edit/`.
- `apps/web/components/settings/dynamic-agent-profile-editor.tsx`.
- `apps/web/components/settings/agent-profile-page.tsx`.
- `apps/web/components/task/task-layout.tsx`.
- `apps/web/components/task/mobile/`.
- `apps/web/hooks/domains/settings/`.
- `apps/web/hooks/domains/session/`.
- `apps/web/lib/api/domains/`.
- `apps/web/lib/state/slices/session/`.
- `apps/web/src/locales/`.
- `apps/web/e2e/fixtures/` and `apps/web/e2e/helpers/` (isolated Cursor HTTP/SSE fixture).
- `apps/web/e2e/tests/session/cursor-cloud.spec.ts (new)`.
- `apps/web/e2e/tests/session/mobile-cursor-cloud.spec.ts (new)`.

- `apps/web/e2e/playwright.config.ts`.
- `apps/web/e2e/scripts/run-e2e.sh`.
- `apps/web/e2e/scripts/run-e2e.test.ts`.
- `apps/web/e2e/scripts/cursor-cloud-projects.test.ts` (new project inclusion/exclusion and worker configuration tests).
- `.github/workflows/ (E2E project and shard enumeration)`.

## Dependencies

06-observation-and-results

## Risks

A phone fallback must not overwrite desktop preferences. Hidden workspace panels must also stop their subscriptions and network requests.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed runtime ADR](../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- Source baseline and code patterns listed in the plan.

## Results

Completed 2026-09-26. Desktop and phone profile, task-start, chat, capability, and recovery surfaces are implemented. Verification passed: focused frontend tests 51/51, E2E runner tests 15/15, desktop normal-flow E2E 5/5, phone normal-flow E2E 4/4, web typecheck, lint with zero warnings, i18n checks, ratchet, and E2E build. Backend profile/discovery and managed-session tests passed. Unknown submission stays unresolved on archive; archive does not issue a blind cancellation without a known remote run ID.
