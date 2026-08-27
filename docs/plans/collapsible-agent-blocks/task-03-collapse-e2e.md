---
id: "03-collapse-e2e"
title: "Collapse behavior E2E"
status: done
wave: 1
depends_on: ["01-collapse-persistence-hook", "02-collapsible-agent-card-ui"]
plan: "plan.md"
spec: "../../specs/agents/requirements/collapsible-agent-blocks.md"
---

# Task 03: Collapse behavior E2E

Playwright coverage for the collapse flow on `/settings/agents`, desktop and
mobile. Uses `apiClient` so the count assertion is exact: if the first seeded
agent has zero profiles, create one via `apiClient.createAgentProfile`.

- **Acceptance:**
  1. Desktop spec `agent-block-collapse.spec.ts` proves: default expanded
     (profile body `agent-profiles-<name>` visible, no stored key before the
     first toggle); collapse hides the body and shows the profile-count text
     in the header; reload keeps the block collapsed with the count visible;
     expand restores the body and removes the count from the header.
  2. Mobile spec `mobile-agent-block-collapse.spec.ts` proves the same user
     value on the `mobile-chrome` project (file name `mobile-*.spec.ts` picks
     the project up automatically): collapse a block from the header control,
     count visible in the header, reload still collapsed.
  3. No existing settings E2E breaks (default remains expanded).
- **Verification:**
  ```bash
  cd apps/web && pnpm e2e:run -- tests/settings/agent-block-collapse.spec.ts
  cd apps/web && pnpm e2e:run -- --project mobile-chrome tests/settings/mobile-agent-block-collapse.spec.ts
  ```
  (`e2e:run` rebuilds backend + web and serves the production build; see
  `apps/web/e2e/README.md` if the environment needs `--host`/`--docker`.)
- **Files likely touched:**
  - `apps/web/e2e/tests/settings/agent-block-collapse.spec.ts` (new)
  - `apps/web/e2e/tests/settings/mobile-agent-block-collapse.spec.ts` (new)
- **Dependencies:** tasks 01 and 02.
- **Parallelism:** sequential (last).
- **Inputs:** Spec "Scenarios"; existing spec conventions in
  `apps/web/e2e/tests/settings/agent-profile-delete.spec.ts` (`apiClient`,
  `testPage` from `../../fixtures/test-base`); selectors
  `agent-group-<name>`, `agent-profiles-<name>`, `collapse-agent-<name>`.
- **Output contract:** summary, files changed, exact Playwright results for
  both projects, and task/plan status update in the same conversation.

## Results

- Desktop: `cd apps/web && pnpm e2e:run -- tests/settings/agent-block-collapse.spec.ts` → 2/2 passed (`--repeat-each=2`).
- Mobile: `cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/settings/mobile-agent-block-collapse.spec.ts` → 2/2 passed (`--repeat-each=2`).
- Files: `apps/web/e2e/tests/settings/agent-block-collapse.spec.ts` (new), `apps/web/e2e/tests/settings/mobile-agent-block-collapse.spec.ts` (new).
- Both specs route-inject a deterministic zero-profile agent via `**/api/v1/agents/discovery` (the fixture only registers mock-agent, which has profiles). The injected card's presence gates interaction until discovery settles — this fixes a real flake: cards render from the boot payload as orphans (key = agent id) and remount when discovery resolves (key = agent name), detaching the toggle mid-scroll.
- The mobile spec asserts no horizontal overflow (`scrollWidth <= clientWidth`) after collapsing, including for the long "No profiles yet" label on the zero-profile card.
- Host-mode E2E (no Docker available in this environment); `pnpm e2e:run` rebuilt backend + web + fixture plugin.

## Review loop

Adversarial review rounds (sub-agent `reviewer`):
- Round 1: 2 major findings (zero-profile label mobile overflow; uncaught localStorage write error) → fixed in `f98c54fc6` (wrap cluster + min-w-0 span; try/catch `handleToggleCollapsed` + component test).
- Round 2: 1 minor finding (zero-profile E2E branch unreachable under the fixture) → fixed in `a092fc4e0` (deterministic route injection in both specs + discovery-settled gating).
- Round 3: NO FINDINGS → loop ended.
