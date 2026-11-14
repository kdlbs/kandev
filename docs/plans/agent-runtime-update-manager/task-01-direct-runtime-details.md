---
id: "01-direct-runtime-details"
title: "Open runtime version details directly"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-MANAGER-001
  - REQ-AGENTS-RUNTIME-MANAGER-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-MANAGER-001.3
  - AC-AGENTS-RUNTIME-MANAGER-001.4
  - AC-AGENTS-RUNTIME-MANAGER-001.7
  - AC-AGENTS-RUNTIME-MANAGER-002.4
system_design:
  - ../../specs/agents/system-design/runtime-update-manager.md
---

# Task 01: Open runtime version details directly

## Summary

Connect Manage versions and installed-card update controls to one page-owned runtime detail controller.
Reuse the existing desktop dialog, phone drawer, preview, and job behavior.
The bottom policy section stays in place until Task 02.

## In scope

- Extract reusable detail body/footer from `AgentRuntimeUpdateControl` and support external opening without duplicate state or portals.
- Add page-owned selected-runtime state and explicit callbacks through `InstalledAgentCard` and `AgentRuntimePolicies`.
- Replace the hash-only Manage versions link with a button that opens the correct runtime immediately.
- Retain mode-specific behavior, actual-opener focus return, loading/error handling, and localized accessible names.
- Use TDD for controlled opening and regression coverage. Preserve stable existing test IDs where their meaning remains unchanged.

## Out of scope

- Header overview, removal of the bottom section, policy relocation, target action registration, or bulk controls.
- Backend, policy persistence, and updater changes.

## Acceptance

1. Manage versions opens the selected runtime directly on desktop and phone, without a card scroll or duplicate update overlay.
2. Existing pinned, fallback, self-update, migration, compatibility, version browsing, and error behavior retain their supported actions.
3. Opening and closing through each visible trigger preserves preview identity and returns focus to that trigger.

## ASCII UI preview

Relevant excerpt from [UI-03 in the plan](plan.md#ui-03-runtime-detail-and-dirty-policy).
This work order covers 001.3, 001.4, 001.7, and 002.4.

```text
Existing runtime row
Claude  0.100.0 -> 0.103.0                  [Manage versions]
                                                    |
                                                    v
+----------------------------------------------------------+
| Claude                                               [X] |
| Current runtime   0.100.0                                 |
| Version           [0.103.0 (latest)                     v] |
| [existing status, compatibility notice, and output]       |
| [Cancel]                                 [Update Claude] |
+----------------------------------------------------------+

Phone: the same detail region uses the existing tall drawer,
with a fixed header/footer and one scrolling body.
```

No overview Back action appears until Task 02. The policy remains in its current section for this slice.
Counts and spacing are illustrative. Direct opening and one active update surface are required.

## Verification

Before implementation, run failing direct-opening tests. Then run the complete block from the repository root:

```bash
(cd apps/web && pnpm exec vitest run app/settings/agents/page.test.tsx app/settings/agents/page.agent-list-snapshot.test.tsx components/settings/agent-runtime-update-control.test.tsx components/settings/use-agent-update-dialog-state.test.ts components/settings/agent-runtime-policies.test.tsx components/settings/installed-agent-card.test.tsx components/settings/agent-runtime-update-copilot-compatibility.test.tsx hooks/domains/settings/use-agent-runtime-updates.test.tsx hooks/domains/settings/use-agent-runtime-update-manager.test.tsx)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings app/settings/agents components/settings/agent-runtime-update-control.tsx components/settings/agent-runtime-update-content.tsx components/settings/agent-runtime-update-surface.tsx components/settings/agent-runtime-policies.tsx components/settings/installed-agent-card.tsx hooks/domains/settings/use-agent-runtime-update-manager.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-notifications.spec.ts tests/settings/agent-runtime-update.spec.ts tests/settings/agent-self-update.spec.ts tests/settings/agent-runtime-update-copilot.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-notifications.spec.ts tests/settings/mobile-agent-runtime-update.spec.ts tests/settings/mobile-agent-self-update.spec.ts tests/settings/mobile-agent-runtime-update-copilot.spec.ts)
git diff --check
```

Install workspace dependencies once first if absent, as specified in the plan.
The detail content and manager hook paths are new files created by this work order.
Include any additional extracted file in changed-file lint and its matching tests in the focused Vitest invocation.
Run E2E commands sequentially with their fresh builds. Capture desktop and phone details for comparison with UI-03.

## Files likely touched

- `apps/web/app/settings/agents/page.tsx` and its page tests.
- `apps/web/components/settings/agent-runtime-policies.tsx` and its tests.
- `apps/web/components/settings/installed-agent-card.tsx` and its tests.
- `apps/web/components/settings/agent-runtime-update-control.tsx` and its tests.
- `apps/web/components/settings/agent-runtime-update-content.tsx` (new reusable detail view).
- `apps/web/components/settings/agent-runtime-update-surface.tsx`.
- `apps/web/components/settings/use-agent-update-dialog-state.ts` and its tests.
- `apps/web/hooks/domains/settings/use-agent-runtime-update-manager.ts` (new page controller).
- Existing runtime update, self-update, Copilot, notification, and fallback E2E helpers/specs where opening assertions change.
- Locale `agents.json` files only for necessary new copy.
- This work order and its plan for status and actual results.

## Dependencies

None. Read the Agents page, current detail controls, and their tests before changes.

## Risks

- External open state can accidentally reset selected versions or attach a late preview to another runtime.
- Duplicate detail controllers can produce multiple portaled overlays and ambiguous test targets.
- Native host ownership must not select the fallback's version as the host observation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/runtime-update-manager.md), REQ-AGENTS-RUNTIME-MANAGER-001 and 002.
- [Design](../../specs/agents/system-design/runtime-update-manager.md), Composition and Runtime capabilities and counts.
- Existing `agent-runtime-update-control.test.tsx`, `use-agent-update-dialog-state.test.ts`, and E2E runtime fixtures.

## Results

Completed 2026-11-10. Extracted the reusable detail body/footer, added controlled detail opening, threaded page-owned callbacks through installed cards and runtime rows, and replaced the Manage versions anchor with a direct button. Focused Vitest run: 64 tests passed across 9 files. Typecheck, i18n check, i18n ratchet, and changed-file Oxlint passed. Desktop and phone notification suites passed after their helpers were migrated to direct manager opening.
