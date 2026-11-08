---
id: "02-first-profile-browser-proof"
title: "Prove first-profile creation"
status: completed
wave: 2
depends_on:
  - "01-draft-discovery-context"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-DISCOVERY-001.2
  - AC-AGENTS-PROFILE-DISCOVERY-001.3
  - AC-AGENTS-PROFILE-DISCOVERY-001.5
  - AC-AGENTS-PROFILE-DISCOVERY-003.2
  - AC-AGENTS-PROFILE-DISCOVERY-003.3
  - AC-AGENTS-PROFILE-DISCOVERY-003.4
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
---

# Task 02: Prove first-profile creation

## Summary

Prove that a first profile can discover, select, save, and reload a model without a pre-existing parent row.
Use the real backend endpoints and mock ACP peer on desktop and phone.

## In scope

- Add a disposable registered Amp identity through the existing mock-provider fixture support.
- Remove only that identity's parent row while retaining its registry entry and available status.
- Exercise the existing create-profile page, refresh, model selection, Save, and reload.
- Assert request context, API persistence, phone action reachability, and cleanup.

## Out of scope

- Production page, hook, copy, layout, navigation, or shared control changes.
- Real provider credentials, real Devin subprocesses, general fixture redesign, or production test endpoints.

## Acceptance

1. Desktop and phone start with a registered available agent absent from saved settings and complete the first-profile save flow.
2. Probe and model-option requests reach the real backend with complete drafts and no `profile_id`, without creating the parent before Save.
3. Save persists the selected model, reload retains it, and cleanup restores the worker's prior environment and owned data.

## Scenario and fixture

Acquire `testPage` before changing worker state.
Use `backend.restart({ KANDEV_MOCK_PROVIDERS: "amp-acp" })` to enable an additional mock ACP provider.
The default registry cannot accept Devin through this routing-provider override, so use Amp for browser evidence.
Task 01 separately tests the real default Devin and Qwen descriptors at the controller boundary.
Do not expand routing-provider IDs for this test.

Capture the baseline and require Amp to be absent before setup, or preserve any existing Amp data.
After the restart, find only the new Amp parent and delete it with `apiClient.rawRequest`.
Assert a successful DELETE response and confirm that the available catalogue still contains Amp.
Assert no Amp row exists in `listAgents` before opening the editor.
Leave the normal mock-agent seed and its consumers unchanged.

Open `/settings/agents/amp-acp?mode=create`.
Observe `/api/v1/agent-models/amp-acp/probe` and `/resolve` before the page starts discovery.
Use request/response observers, not fulfilled capability responses.
Require the complete launch snapshot and omitted `profile_id` on both calls.
Refresh the current draft and require a successful ready state.
Choose an advertised model and wait for matching dependent option resolution.
Require the parent to remain absent through that point.
Set a unique profile name and save through the visible action.
Assert one persisted Amp parent and the selected model on its new profile.
Reload the page and require the same selected model label.

Use a shared test helper for setup, request evidence, and cleanup.
Keep viewport-specific interaction in the desktop and phone specs.
Assert unique controls before interacting with them.
If the UI initially selects a default model, explicitly choose a different advertised model to prove dependent resolution.

The phone exemplar is `mobile-profile-capability-discovery.spec.ts`.
Retain its direct navigation, inline capability status, model picker, and settings scroll owner.
Use touch input for refresh, model selection, and Save.
Require at least 44px refresh and save hit targets and zero horizontal page overflow.
No new rendered composition is part of this repair, so no ASCII preview is required.

In `finally` or `afterEach`, delete only the owned Amp parent/profile data and restore the baseline backend environment.
Call `backend.restart()` after cleanup, including assertion failures.
Restart alone does not undo persisted fixture changes.
The existing controller RED test establishes the defect before Task 01.
Record the browser result as acceptance evidence after the fix, rather than claiming an unrun browser RED result.

## Verification

Run from the repository root.
If this worktree has no workspace dependencies, install them once before the following commands:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run desktop and phone commands sequentially through the managed runner.
Do not add worker overrides or overlap suites.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/settings/first-profile-capability-discovery.spec.ts tests/settings/profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-first-profile-capability-discovery.spec.ts tests/settings/mobile-profile-capability-discovery.spec.ts)
(cd apps/web && pnpm exec eslint e2e/helpers/first-profile-capability-discovery.ts e2e/tests/settings/first-profile-capability-discovery.spec.ts e2e/tests/settings/mobile-first-profile-capability-discovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record discovered and passed test counts for both projects.
Keep existing discovery specs in these runs to expose fixture leakage and saved-profile regressions.

## Files likely touched

- New `apps/web/e2e/helpers/first-profile-capability-discovery.ts`
- New `apps/web/e2e/tests/settings/first-profile-capability-discovery.spec.ts`
- New `apps/web/e2e/tests/settings/mobile-first-profile-capability-discovery.spec.ts`

Read `fixtures/backend.ts`, `helpers/api-client.ts`, and existing profile discovery specs as patterns.
Existing `rawRequest`, mock-provider registration, and worker restart support make new production fixture endpoints unnecessary.

## Dependencies

[Task 01](task-01-draft-discovery-context.md) and its controller/HTTP checks must pass first.

## Risks

- A mocked browser response can conceal the missing-parent defect.
- A catalogue-only fixture cannot establish backend state. Assert both availability and saved-parent absence through the API.
- Worker restarts retain the database. Restore owned rows and environment explicitly.

## Parallelism

`sequential`

## Inputs

- [Plan browser proof](plan.md#browser-proof-without-shared-seed-damage)
- [Requirements](../../specs/agents/requirements/profile-capability-discovery.md)
- [System design](../../specs/agents/system-design/profile-capability-discovery.md), identity, editor, and responsive sections.
- Existing desktop/phone profile-discovery specs and the backend mock-provider restart pattern.

## Results

Browser coverage added 2026-11-03:

- `apps/web/e2e/helpers/first-profile-capability-discovery.ts` restarts the worker with `KANDEV_MOCK_PROVIDERS=amp-acp`, deletes only the Amp parent rows, keeps the registry entry available, records real probe/resolve traffic, saves, reloads, and restores the worker in cleanup.
- `apps/web/e2e/tests/settings/first-profile-capability-discovery.spec.ts` and `apps/web/e2e/tests/settings/mobile-first-profile-capability-discovery.spec.ts` cover desktop and phone: no persisted parent before Save, complete launch snapshot with an omitted `profile_id`, dependent option resolution, persistence, reload, phone 44px refresh target, and no horizontal overflow.
- Managed run status: `pnpm e2e:run --host --shards 1 --project chromium tests/settings/first-profile-capability-discovery.spec.ts` passed (1 test, 4.8s). `pnpm e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-first-profile-capability-discovery.spec.ts` passed (1 test, 6.4s). The specs assert real backend responses and never fulfill capability requests.
- The boot profile reconciler can reseed a default profile for a newly available agent, so the helper deletes until the row stays absent across a settled read before opening the draft.
- The worker must start without an Amp installation; `expectAmpBaselineAbsent` fails before any mutation instead of deleting pre-existing data.
- The chosen advertised model value is captured from the picker row and compared with the persisted profile model, before and after reload.
- The phone flow measures the Save target at 44px, activates Refresh and Save with touch, and asserts no horizontal overflow while the first-profile draft is rendered.
