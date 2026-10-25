---
id: "02-collapsible-agent-card-ui"
title: "Collapsible agent card UI"
status: done
wave: 1
depends_on: ["01-collapse-persistence-hook"]
plan: "plan.md"
spec: "../../specs/agents/requirements/collapsible-agent-blocks.md"
---

# Task 02: Collapsible agent card UI

Wrap each installed-agent card's profiles body in a Radix `Collapsible` and
show the profile count in the header while collapsed, using the hook from
task 01.

- **Acceptance:**
  1. `InstalledAgentCard` renders a touch-sized ghost chevron button
     (`data-testid="collapse-agent-<name>"`, `cursor-pointer`,
     `min-h-11 min-w-11`) in the header actions cluster; default state is
     expanded and the profiles body (`children`, i.e. `AgentProfilesSubList`)
     is visible.
  2. Clicking the button collapses the body (`CollapsibleContent` hides
     `agent-profiles-<name>`) and shows the count in the header next to the
     button: `t("agents:profileCount", { count })` when profiles exist,
     `t("agents:noProfilesYet")` when zero. Expanded state does NOT duplicate
     the count in the header. Clicking again expands and removes the header
     count.
  3. A pre-seeded `kandev:agents:collapsedBlocks:v1` entry for the agent makes
     the card mount collapsed (persistence honored); toggling writes the
     record via the task-01 hook.
  4. New i18n keys `agents:collapseAgentProfiles` and
     `agents:expandAgentProfiles` (en + regenerated pseudo + pt-pt + zh-cn)
     pass `i18n:check` and `i18n:ratchet`; no hardcoded UI literals.
- **Verification (TDD: component test first, then implement):**
  ```bash
  cd apps && pnpm --filter @kandev/web test -- components/settings/installed-agent-card.test.tsx
  cd apps/web && pnpm run typecheck
  cd apps && pnpm --filter @kandev/web lint -- components/settings/installed-agent-card.tsx components/settings/installed-agent-card.test.tsx
  cd apps/web && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet
  ```
- **Files likely touched:**
  - `apps/web/components/settings/installed-agent-card.tsx`
  - `apps/web/components/settings/installed-agent-card.test.tsx` (new)
  - `apps/web/src/locales/en/agents.json` (2 keys)
  - `apps/web/src/locales/pseudo/agents.json` (regenerated)
  - `apps/web/src/locales/pt-pt/agents.json`, `apps/web/src/locales/zh-cn/agents.json`
- **Dependencies:** task 01 (hook).
- **Parallelism:** sequential.
- **Inputs:** Spec "What" + "Scenarios"; `@kandev/ui/collapsible`
  (`apps/packages/ui/src/collapsible.tsx`); existing component test pattern in
  `apps/web/components/settings/agents/agent-profiles-section.test.tsx`
  (state-provider mock, Radix mocks); `ProfileRowActions` for touch-target
  sizing precedent. Mobile contract: collapse button is the touch control
  (≥44px), count text is inline text (no hover dependency); see
  `skill://mobile-parity` if the layout changes unexpectedly.
- **Output contract:** summary, files changed, exact test/lint/i18n results,
  and task/plan status update in the same conversation.

## Results

- `pnpm --filter @kandev/web test -- components/settings/installed-agent-card.test.tsx` → 6/6 passed (TDD: red on missing feature, then green).
- `cd apps/web && pnpm run typecheck` → clean.
- `pnpm exec eslint --max-warnings 0 components/settings/installed-agent-card.tsx components/settings/installed-agent-card.test.tsx hooks/domains/settings/use-collapsed-agent-blocks.ts hooks/domains/settings/use-collapsed-agent-blocks.test.ts` → clean (extracted `AgentCollapseControl` sub-component to stay under the 100-line function limit; helper functions in the test to avoid the duplicate-string lint).
- `pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet` → all green (en + regenerated pseudo + pt-pt + zh-cn).
- Files: `apps/web/components/settings/installed-agent-card.tsx` (Collapsible body + header toggle/count), `apps/web/components/settings/installed-agent-card.test.tsx` (new), `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/agents.json` (2 new keys each).
- Note: this repo's unit tests use happy-dom without jest-dom; assertions use plain DOM properties (`hidden`, `getAttribute`), and Radix Presence unmounts closed `CollapsibleContent` (asserted hidden-or-detached).
- Post-review refinement (Test phase): header action order is now `profile count (when collapsed) | update button (conditional) | collapse button`. The collapse button matches the update trigger exactly (`variant="ghost" size="icon" h-11 w-11 sm:h-7 sm:w-7`), transparent at rest with grey only on hover — the ghost variant's `aria-expanded:bg-muted` is neutralized via `aria-expanded:bg-transparent hover:bg-muted!` because this button uses `aria-expanded` as a disclosure state, not a select visual. A unit test guards count-left-of-collapse ordering; visual bg/size verified in the live demo instance.
