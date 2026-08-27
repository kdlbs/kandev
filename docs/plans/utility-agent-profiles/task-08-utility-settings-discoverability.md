---
id: "08-utility-settings-discoverability"
title: "Improve utility settings discoverability"
status: completed
wave: 5
depends_on: ["04-settings-profile-pickers"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 08: Improve utility settings discoverability

## Intent

Make the Utility Agents page understandable for first-time users and make profile selection fast,
consistent, and usable on desktop and phone screens.

## Acceptance

- The page renders cards in this order: Default utility agent model, Configuration Chat Agent,
  Actions, and Custom utility agents.
- The page description explains that utility agents are one-shot Kandev UI helpers for operations
  such as commit, PR, and prompt text generation. It says that they are separate from agents that
  work inside task sessions.
- The Actions card has a localized description. It says that the card overrides a profile for a
  specific Kandev UI action and does not configure task-session agents.
- The default profile, every built-in action override, the custom utility-agent dialog, and the
  Configuration Chat Agent selector use the same searchable profile-picker interaction.
- The profile-picker trigger and each option show the existing parent-agent icon and a readable
  profile label. Typing filters by profile label and parent agent name. Keyboard navigation and
  touch selection remain available.
- Unavailable saved profiles remain visible as repairable unavailable selections. Filtering does
  not make the selected stale ID disappear, and utility eligibility rules remain unchanged.
- Desktop and mobile layouts have no document horizontal overflow. The picker list scrolls inside
  its bounded popover on a phone viewport.
- New and changed user-facing text uses the English, Chinese, and generated pseudo-locale catalogs.

## Files likely touched

- `apps/web/app/settings/utility-agents/page.tsx`
- `apps/web/components/settings/utility-agents-section.tsx`
- `apps/web/components/settings/utility-sections.tsx`
- `apps/web/components/settings/utility-agent-dialog.tsx`
- `apps/web/components/settings/config-chat-agent-section.tsx`
- `apps/web/components/settings/utility-agent-profile-picker.tsx` (shared picker, if extraction is
  needed)
- `apps/web/components/settings/utility-sections.test.tsx`
- `apps/web/components/settings/utility-agent-dialog.test.tsx`
- `apps/web/components/settings/config-chat-agent-section.test.tsx` (if present or added)
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/zh-cn/settings.json`
- `apps/web/src/locales/pseudo/settings.json`
- `apps/web/e2e/tests/settings/utility-agents.spec.ts`
- `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`

## Dependencies

Task 04 profile-backed settings state and API fields.

## Implementation notes

- Reuse the existing `Combobox` and `AgentLogo` components. Do not create a second filtering
  algorithm or a second icon lookup path.
- Keep the utility profile eligibility helper as the source of truth for utility selectors. The
  Configuration Chat Agent selector keeps its current profile scope unless the shared picker needs
  a separate option list.
- Preserve the current test IDs for cards and rows. Add a stable test ID for the shared profile
  picker so desktop and mobile tests do not depend on Radix Select internals.
- Keep the Settings save coordinator and custom-agent dialog save behavior unchanged except for the
  picker control and its localized copy.

## Verification

Bootstrap once if needed:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run:

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/utility-sections.test.tsx components/settings/utility-agent-dialog.test.tsx components/settings/config-chat-agent-section.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/settings/utility-agents.spec.ts -- --retries=0
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-utility-agents.spec.ts -- --retries=0
```

The E2E checks must assert card order, explanatory descriptions, icon rendering, typed filtering,
stale-selection visibility, and mobile horizontal containment.

## Output contract

Report the final card order, copy keys, shared picker behavior, stale-profile behavior, desktop and
mobile test counts, files changed, and any remaining risks. Update this task and the parent plan with
the exact command results.

## Results

Implemented the discoverability follow-up.

- Moved Configuration Chat Agent directly below the default utility profile card.
- Added localized page and Actions descriptions that define one-shot Kandev UI helpers and
  separate them from task-session agents.
- Added the shared icon-enabled searchable profile picker to the default, built-in action, custom
  utility-agent, and Configuration Chat Agent selectors. Search matches profile labels and parent
  agent names, while stale saved IDs remain repairable.
- Added desktop and mobile coverage for card order, copy, icon rendering, typed filtering, bounded
  scrolling, touch selection, and horizontal overflow.

Verification:

- `pnpm --filter @kandev/web test -- --run components/settings/utility-agent-profile-picker.test.ts components/settings/utility-agents-section.test.ts` — 2 files, 8 tests passed.
- `pnpm run typecheck` — passed.
- Targeted ESLint for changed TS/TSX/E2E files — passed.
- `pnpm run i18n:check && pnpm run i18n:ratchet` — passed; existing catalog parity warnings remain advisory.
- Desktop utility-agent E2E — 8 tests passed; focused discoverability and wheel tests — 2 passed.
- Mobile utility-agent E2E (`mobile-chrome`) — 1 test passed.
- `git diff --check` — passed.
