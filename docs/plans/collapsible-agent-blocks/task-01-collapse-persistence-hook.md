---
id: "01-collapse-persistence-hook"
title: "Collapse persistence hook"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/collapsible-agent-blocks.md"
---

# Task 01: Collapse persistence hook

Per-agent collapsed/expanded preference, stored as one JSON record in
`localStorage`, exposed through a domain hook with the same
storage-event + custom-event broadcast shape as
`useLocalStorageBoolean` (see `apps/web/hooks/use-local-storage-boolean.ts`
and `apps/web/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.ts`).

- **Acceptance:**
  1. `useCollapsedAgentBlocks()` returns `collapsed(agentName): boolean` and
     `setCollapsed(agentName, collapsed): void`; unknown/missing entries and
     invalid JSON default to `false` (expanded).
  2. `setCollapsed` merges one entry into the stored record under
     `kandev:agents:collapsedBlocks:v1`, persists it, and dispatches the sync
     event so other tabs/components re-render; setting `false` writes `false`
     for that agent (explicit expanded, independent of others).
  3. Read failures degrade to the default without throwing; write failures
     throw (same contract as `useLocalStorageBoolean`).
- **Verification (TDD: write the test first, watch it fail, then implement):**
  ```bash
  cd apps && pnpm --filter @kandev/web test -- hooks/domains/settings/use-collapsed-agent-blocks.test.ts
  cd apps/web && pnpm run typecheck
  cd apps && pnpm --filter @kandev/web lint -- hooks/domains/settings/use-collapsed-agent-blocks.ts hooks/domains/settings/use-collapsed-agent-blocks.test.ts
  ```
- **Files likely touched:**
  - `apps/web/hooks/domains/settings/use-collapsed-agent-blocks.ts` (new)
  - `apps/web/hooks/domains/settings/use-collapsed-agent-blocks.test.ts` (new)
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** Spec "What" + "Data model" + "Scenarios"; existing
  `use-local-storage-boolean.ts` and its test as the mechanism template; test
  helper `apps/web/hooks/local-storage-mock.test-helpers.ts`.
- **Output contract:** summary, files changed, exact test/lint/typecheck
  results, and task/plan status update in the same conversation.

## Results

- `pnpm --filter @kandev/web test -- hooks/domains/settings/use-collapsed-agent-blocks.test.ts` → 13/13 passed (TDD: red on missing module, then green).
- `cd apps/web && pnpm run typecheck` → clean.
- `pnpm exec eslint --max-warnings 0 hooks/domains/settings/use-collapsed-agent-blocks.ts hooks/domains/settings/use-collapsed-agent-blocks.test.ts` → clean.
- Files: `apps/web/hooks/domains/settings/use-collapsed-agent-blocks.ts` (new), `apps/web/hooks/domains/settings/use-collapsed-agent-blocks.test.ts` (new). Test suite split into four describes to stay under the 100-line function limit.
