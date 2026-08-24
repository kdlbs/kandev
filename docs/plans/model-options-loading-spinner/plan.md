---
spec: docs/specs/agents/requirements/dynamic-provider-options.md
created: 2026-08-19
status: implemented
---

# Implementation Plan: Model Options Loading Spinner

## Overview

Show model-option resolution progress inside the open agent profile selector.
The selector will keep the model list visible and replace stale option controls
with the existing localized loading state. Desktop and mobile Playwright tests
will hold the resolver response so that they can prove the transient state.

The change uses the current resolver and API contract. It does not change the
backend, persistence, or the error and retry behavior below the profile field.

## Backend

No backend change is required. The existing
`POST /api/v1/agent-models/:agentName/resolve` request owns the loading period.

## Frontend

### Shared model selector

Update `ModelConfigSelectorProps` in
`apps/web/components/model-config-selector.tsx`. Add
`configOptionsLoading?: boolean` and `keepOpenOnModelChange?: boolean`. Keep the
popover open when a caller expects model-dependent option resolution.

In `ModelConfigSelectorContent`, keep the model list visible during resolution.
Show a separated status row directly below that list. The row will use
`@kandev/ui/spinner` and `agents:resolvingModelOptions`.

The loading row will replace extra option controls while a request is pending.
It will use `data-testid="model-config-options-loading"` and an accessible
status name. Existing callers will keep their current close behavior unless
they enable asynchronous option resolution.

While the request is pending, the selected model row replaces its trailing
check icon with the same spinner. The check icon returns when the request
resolves or fails.

### Agent profile wiring

Update `apps/web/components/settings/profile-model-fields.tsx` so `ModelPicker`
accepts and passes `configOptionsLoading` and `keepOpenOnModelChange`.

Update `apps/web/components/settings/profile-form-fields.tsx` to pass
`configIsLoading` and `modelConfig.supports_dynamic_models` to the start-model
picker. Keep `ModelConfigResolutionStatus` below the field for save blocking,
errors, and retry actions.

### Mobile design note

The selector composition does not change. The existing touch-usable `Popover`
in `mobile-agent-profile-config-selector.spec.ts` is the closest mobile
example. This short, searchable picker remains a popover because the task only
adds a status row to its existing option area.

The page remains the scroll owner. The popover keeps its existing viewport
limits. The shared selector state and loading logic are identical on desktop
and mobile.

## Tests

- **What:** The shared selector stays open after a model change and shows the
  loading row below the model list. The selected row shows a spinner while the
  request is pending and restores its check icon afterward.
- **File:** `apps/web/components/model-config-selector.test.tsx`.
- **How:** Use a stateful component test. Start resolution from
  `onModelChange`, make sure that stale option triggers are absent, and finish
  the request with a resolved option.

- **What:** The profile form sends its resolver state into the shared selector.
- **File:** `apps/web/components/settings/profile-form-fields.test.tsx`.
- **How:** Hold `resolveAgentModelConfig`, select another model, and inspect the
  loading and resolved states before and after the promise completes.

No locale change is required. The selector reuses
`agents:resolvingModelOptions` in all five locale catalogs.

## E2E Tests

- **Scenario:** Given an open desktop profile selector, when a model resolution
  is pending, then the selected row and the status row show spinners. Resolved
  controls replace the status row and the selected row restores its check after
  the response.
- **File:** `apps/web/e2e/tests/settings/agent-profile-acp.spec.ts`.
- **What to verify:** The popover stays open. Both loading indicators are
  visible. Stale option controls are absent. The loading indicators are
  removed, the selected row shows its check, and the new model option appears
  after the response.

- **Scenario:** Given the same flow on a phone, when a model resolution is
  pending, then both spinners are visible and contained in the selector.
- **File:**
  `apps/web/e2e/tests/settings/mobile-agent-profile-config-selector.spec.ts`.
- **What to verify:** Touch selection keeps the popover open. Both loading
  indicators are visible and are removed after resolution, with the selected
  row's check restored. The page has no horizontal overflow. The resolved
  control remains reachable by touch.

Both tests will use `injectLatency` for the resolver route. This delay is the
test stimulus that makes the transient state observable.

## Verification Results

Implementation is complete. The task-defined checks passed:

- `pnpm install --frozen-lockfile` from `apps`.
- Focused Vitest for the shared selector and profile form: 2 files, 32 tests
  passed.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
- Desktop profile E2E: 4 tests passed.
- Mobile profile E2E: 2 tests passed.
- Disposable desktop and mobile PR-asset capture tests: 1 test passed in each
  project. The screenshots were inspected, mapped in `manifest.json`, and
  compressed for publication.
- `git diff --check` and Prettier checks for all changed source and test files.

Review fixup coverage also verifies that the selector stays open whenever
options are loading, the loading row has one accessible status announcement,
and both rejected and `status: failed` resolution responses clear stale
dependent options. The targeted regression suite now passes 46 tests.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-selector-loading-state](task-01-selector-loading-state.md)

Wave 2:

- [x] [task-02-desktop-mobile-e2e](task-02-desktop-mobile-e2e.md)

The tasks are sequential because the Playwright tests require the selector
loading state from Task 01.

## Risks

- A model selection currently closes the selector when it has no extra option
  controls. The new opt-in open behavior must start before React reports the
  asynchronous loading state.
- The resolver cache can make the loading period short. Playwright must add
  latency to the selected-model request instead of using a fixed wait.
- Baseline options can belong to the prior model. The loading row must replace
  those controls until the authoritative snapshot arrives.
