---
id: "03-duplicate-e2e"
title: "Add the E2E duplicate flow"
status: done
wave: 3
depends_on: ["01-backend-duplicate-endpoint", "02-frontend-duplicate-ui"]
plan: "plan.md"
spec: "../../specs/agents/requirements/profile-duplicate.md"
---

# Task 03: E2E duplicate flow

## Acceptance

- An E2E spec (`agent-profile-duplicate.spec.ts`) proves the user-facing
  flow: create a profile with distinctive settings via `apiClient`, open
  `/settings/agents`, click its Duplicate button, and see a
  `<name> Copy` row appear without a page reload.
- The copy's profile page shows the copied settings (model / CLI flag /
  command prefix) matching the source.
- The spec cleans up both profiles in `finally`.

## Verification

- `cd apps/web && pnpm e2e:raw --grep "agent profile duplicate"` (or the
  repo's E2E runner for `tests/settings/agent-profile-duplicate.spec.ts`).
- Mobile parity: the new row button must be reachable at mobile widths; reuse
  the desktop spec unless a mobile-only interaction appears (the icon button
  sits outside the row link exactly like the existing switch, which already
  has mobile coverage patterns).

## Files likely touched

- `apps/web/e2e/tests/settings/agent-profile-duplicate.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-agent-profile-duplicate.spec.ts` (new — mobile parity: Pixel 5 tap on the 44px row control, copy row appears, no horizontal overflow)

## Dependencies

Tasks 01 and 02 (endpoint and UI must exist).

## Parallelism

Sequential by default; may run in parallel with Task 02 only with explicit
user authorization.

## Inputs

- Spec duplicate scenarios
- `agent-profile-delete.spec.ts` structure (apiClient setup, testPage.goto,
  row assertions); `apiClient.createAgentProfile` / `deleteAgentProfile`
  helpers; `cleanupTestProfiles` convention.

## Output contract

Report the passing E2E run (or the exact failure to investigate), cleanup
behaviour, changed files, and update this task plus `plan.md` status.
