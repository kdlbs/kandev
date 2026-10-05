---
id: "03-self-update-settings-ui"
title: "Add the self-update Settings dialog variant"
status: done
wave: 3
depends_on:
  - "02-backend-self-update-pipeline"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-003.1
  - AC-AGENTS-RUNTIME-UPDATES-003.2
  - AC-AGENTS-RUNTIME-UPDATES-003.3
  - AC-AGENTS-RUNTIME-UPDATES-003.6
  - AC-AGENTS-RUNTIME-UPDATES-003.7
  - AC-AGENTS-RUNTIME-UPDATES-003.12
  - AC-AGENTS-RUNTIME-UPDATES-003.14
  - AC-AGENTS-RUNTIME-UPDATES-003.15
  - AC-AGENTS-RUNTIME-UPDATES-003.16
system_design:
  - ../../specs/agents/system-design/harness-self-update.md
---

# Task 03: Add the self-update Settings dialog variant

## Summary

Extend the existing Agents update control to render harness-owned updates without presenting a selectable version catalogue. Keep the same trigger, responsive dialog/drawer, update job progress, streamed output, and result handling used for pinned runtimes.

## In scope

- Add typed `update_mode` handling for runtime update, status, preview, and job data.
- Hide `RuntimeVersionPicker` for `self_update` mode and display installed/current plus `stable_latest_version` as a reference, never as an update target.
- Update the API client and approval hook so `self_update` uses `update_mode` to call the approval API without a target and serializes exactly `{}`; keep target-required and default-reset requests/guards for pinned updates.
- Use structural operation state for approval availability; self-update `update` and `repair` remain actionable with no target. Show an unknown stable reference when registry metadata is unavailable. Keep preview available and classify repair only when the current ACP version is unknown or invalid.
- Explain in localized UI copy that `omp update` follows the configured OMP channel and may install a version different from the stable reference.
- Update the Agents settings approval path so any empty-ID terminal response is returned without calling `upsertAgentUpdateJob`; start runtime-update status refresh without awaiting it.
- Add component/helper tests for reference labeling, channel explanation, unknown stable metadata, no-target self-update approval with exact `{}`, structural `repair` actionability, and the pinned target-required guard. Approval coverage clicks the enabled self-update action from a targetless `update`/`repair` preview and verifies the self-update API call.
- When approval returns terminal `up_to_date` with an empty `job_id`, preserve it as dialog-local result state, render it without registering or polling a job, and clear it on reset or a new approval. This is a shared UI contract; stable metadata does not produce this result for self-update mode. Test repeated no-op responses for no shared job entry and one status refresh each; keep the result visible while refresh is pending or rejected.
- Test status-map behavior when a refresh fails and when several terminal jobs complete close together. The last good map remains visible, and a terminal job causes a status refresh.

## Out of scope

- Backend job behavior or API route changes.
- New standalone UI system, new dialog primitives, or changes to the pinned runtime version picker.

## Acceptance

- OMP's trigger and update dialog show the current version and `stable_latest_version` as a reference, clearly explain that updates follow OMP's configured channel and may install a different version, and offer no target picker or rollback/default controls.
- Approval submits neither a target version nor `use_default`; `update_mode` and structural `operation` enable targetless self-update `update`/`repair` actions, while pinned mode retains its target/default contract. Equal or newer stable metadata does not disable self-update. Registry metadata failure displays an unknown stable reference and does not block preview. Repair remains actionable when the current ACP version is unknown or invalid.
- A generic terminal `up_to_date` response with an empty `job_id` uses local dialog state without registering or polling a job. Clear this result on reset or new approval. It is not produced by comparing OMP's current version with the stable reference.
- A user click on a targetless self-update preview reaches the approval API with exact `{}`; the same empty-target action remains blocked for pinned mode.
- Empty-ID terminal `up_to_date` responses are not upserted into the shared update-job store; each starts a status refresh. The result renders while refresh is pending or rejected. Failed reads preserve the last good status.
- A terminal update job causes a status refresh. Concurrent job completions share the store-owned refresh path and do not leave old status displayed.
- Desktop dialog and phone drawer match UI-01/UI-02/UI-03 in the [plan preview](plan.md#ascii-ui-preview), use translated copy, and preserve existing job output/progress behavior.

## ASCII UI preview

See UI-01 and UI-02 plus terminal no-op state UI-03 in the [plan](plan.md#ascii-ui-preview). Required structure under AC-AGENTS-RUNTIME-UPDATES-003.1-.3, .14, and .15:

```text
+--------------------------------------+
| Update omp                     [X]   |
| Installed version: 18.3.1            |
| Stable latest (reference): 18.3.2    |
| Update follows OMP's channel.        |
| Installed version may differ.        |
| Update command: omp update           |
| Job output: streamed updater output  |
| [Update omp]                         |
+--------------------------------------+
```

### UI-03: Shared terminal no-job result

The shared update dialog can display a terminal `up_to_date` response with an
empty `job_id`. This result does not come from stable-version comparison in
self-update mode; the trusted updater decides whether its configured channel
has an update.

```text
+--------------------------------------+
| Update omp                     [X]   |
| Installed version: 18.3.2            |
| Stable latest (reference): 18.3.2    |
|                                      |
| Already up to date                   |
|                                      |
| [Close]                              |
+--------------------------------------+
```

Show this terminal result from dialog-local state. Do not create or poll a job;
refresh runtime-update status after every empty-ID no-op response.

Phone uses the same terminal result inside the existing drawer, preserving its fixed header/footer and scrollable center; no picker appears in either composition.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/agent-runtime-update.test.ts lib/api/domains/agent-update-api.test.ts components/settings/agent-runtime-update-control.test.tsx components/settings/use-agent-update-dialog-state.test.ts hooks/domains/settings/use-agent-runtime-updates.test.tsx hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx app/settings/agents/page.test.tsx
pnpm run typecheck
pnpm run i18n:check
```

## Files likely touched

- `apps/web/lib/types/http-agents.ts`
- `apps/web/lib/api/domains/agent-update-api.ts`
- `apps/web/lib/agent-runtime-update.ts`
- `apps/web/lib/agent-runtime-update.test.ts`
- `apps/web/components/settings/agent-runtime-update-control.tsx`
- `apps/web/components/settings/agent-runtime-update-control.test.tsx`
- `apps/web/components/settings/runtime-version-picker.tsx`
- `apps/web/components/settings/use-agent-update-dialog-state.ts`
- `apps/web/components/settings/use-agent-update-dialog-state.test.ts`
- `apps/web/lib/api/domains/agent-update-api.test.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-updates.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-updates.test.tsx`
- `apps/web/app/settings/agents/page.tsx`
- `apps/web/app/settings/agents/page.test.tsx`
- `apps/web/hooks/domains/settings/use-agent-runtime-update-statuses.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/agents.json`

## Dependencies

Task 02 defines the `update_mode` wire contract. The shared update dialog also
supports generic terminal no-job `up_to_date` responses.

## Risks

- Existing helper assumptions may require current/default/effective version fields even though self-update mode has no Kandev default. Keep the mode-specific UI explicit and do not invent a default selection.

## Parallelism

`sequential`

## Inputs

- Requirement AC-AGENTS-RUNTIME-UPDATES-003.1-.3, .6, .7, .12, and .14.
- [Harness self-update design](../../specs/agents/system-design/harness-self-update.md), Data and contracts and Control flow.
- Existing `AgentRuntimeUpdateControl`, `RuntimeVersionPicker`, `useAgentUpdateDialogState`, and runtime update unit tests.

## Results

Implemented mode-aware targetless approval, localized reference/channel UI, dialog-local terminal no-job state, and page-triggered advisory refresh. The shared Settings store owns status reads and coalesces refresh requests; the UI hook observes terminal jobs through that shared path.
