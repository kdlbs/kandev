---
id: "05-utility-profile-e2e"
title: "Prove utility profile workflows"
status: pending
wave: 4
depends_on: ["03-update-utility-consumers", "04-settings-profile-pickers"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 05: Prove utility profile workflows

## Intent

Prove persisted profile selection, custom-agent editing, stale-profile repair state, and equivalent
phone reachability against the production E2E build.

## Acceptance

- Desktop E2E saves/reloads a default profile and action override, creates/edits a custom utility
  agent with a profile, and asserts legacy agent/model controls are absent.
- A disabled saved profile remains visible as unavailable with repair guidance and is not offered as
  a runnable choice.
- Mobile E2E completes the default/action selection flow, opens custom edit, verifies touch
  reachability/viewport containment, and asserts no horizontal document overflow.

## Files likely touched

- `apps/web/e2e/tests/settings/utility-agents.spec.ts`
- `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/settings-page.ts`

## Dependencies

Tasks 03 and 04.

## Parallelism

Parallel-safe with task 06 after product behavior is complete: this task owns E2E files while task
06 owns documentation. Sequential execution remains the default.

## Inputs

- Spec: custom/default/override, stale profile, reload, and phone `Scenarios`.
- Plan: `E2E Tests` and `Mobile design contract`.
- Existing patterns: current desktop and `mobile-chrome` utility-agent specs, agent-profile API
  seeding helpers, and managed production-build E2E runner.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/settings/utility-agents.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-utility-agents.spec.ts
```

Confirm each command discovers the intended test count. The managed runner rebuilds backend/web
artifacts and tears down isolated runtime data.

## Output contract

Report scenarios, discovered/passed test counts, rendered mobile containment evidence, cleanup,
files changed, blockers, risks, and synchronized task/plan status. Do not treat API-only assertions
as E2E proof.

## Results

The focused unit/component suites pass. The managed Playwright runner was started twice but did not emit a final discovery/result summary in this workspace, so production E2E proof remains pending for follow-up.
