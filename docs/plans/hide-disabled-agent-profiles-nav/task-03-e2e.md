---
id: "03-e2e"
title: "E2E: hide disabled agent profiles from left panel navigation"
status: done
wave: 3
depends_on: ["01-hide-setting-hook-and-toggle", "02-settings-tree-filter"]
plan: "plan.md"
spec: "../../specs/agents/requirements/hide-disabled-profiles-nav.md"
---

# Task 03: E2E — hide disabled agent profiles from left panel navigation

## Acceptance

- Playwright proves the user-facing flow on a seeded install: with the
  setting off (default) a disabled profile stays in the Settings left
  panel's Agents tree; turning the setting on hides the entry without a
  reload; re-enabling the profile reveals it again with the setting still
  on; the settings page itself keeps listing the disabled profile.

## Verification

```bash
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts` (new)

## Dependencies

Tasks 01 and 02 (the toggle and the tree filter).

## Parallelism

Sequential; it is the end-to-end proof of the other two tasks.

## Inputs

- Spec: the full `Scenarios` list.
- Plan: `Tests > E2E`.
- Existing patterns: `apps/web/e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts`
  (asserts the Settings left panel via
  `testPage.getByTestId("app-sidebar-settings-mode")` and drives the switch
  by id), `apps/web/e2e/tests/settings/agent-profile-disable.spec.ts`
  (seeded-profile flow via `apiClient.listAgents()` /
  `apiClient.updateAgentProfile(profile.id, { enabled })` and a `finally`
  restore).

## Risks

- The profile link label in the tree is `${agentLabel} • ${profile.name}`
  (with the "Disabled" badge appended when visible): assert by exact link
  name or a regex that tolerates the badge.
- Navigate to `/settings/agents` before asserting tree links — the
  route-prefix accordion opens the Agents group only on that prefix.
- The localStorage flag resets per Playwright test context; still restore
  `{ enabled: true }` on the profile in `finally` so worker-scoped seedData
  stays valid for later tests.
- E2E needs office disabled? No — this surface is kanban-flavour settings;
  no office fixtures required.

## Output contract

Report the scenario coverage, the exact command and result, blockers/risks,
then mark this task `done` and update its checkbox in `plan.md`.

## Completion report

- Spec: `apps/web/e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts`
  covers default-off visibility, hide-on disappearance, page-body listing,
  and re-enable reveal without reload; restores `enabled: true` in
  `finally`.
- Command:
  `cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts`
  — 1 passed (ran twice, also after the prettier reformat).
- Blockers: none. Required harness builds (`make -C apps/backend build-kandev
  build-mock-agent e2e-plugin-package`, `make build-web-e2e`) were produced
  from the worktree before the run.
