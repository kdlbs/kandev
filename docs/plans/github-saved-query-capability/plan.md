---
created: 2026-10-27
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
system_design:
  - ../../specs/integrations/system-design/github-dashboard-mobile.md
legacy_specs: []
---

# Implementation Plan: GitHub saved-query capability

## Overview

Restore creation of GitHub saved queries when `randomUUID` is absent by reusing the existing non-security UUID helper. One sequential work order covers the rendered save flow, persistence boundary, and browser interaction.

## Scope

### In scope

- Generate existing `p_` identities with `generateUUID`, preserving secure-capability preference and the saved query's fields and persistence scope.
- Prove the affected dialog/action/hook path and focused desktop and phone browser interactions under simulated missing capability.

### Out of scope

- UUID helper semantics, security tokens, backend/API/persistence contracts, TLS or binding, other dialogs, and optimistic synchronization, save acknowledgement, stale-response or hydration behavior.

## Technical approach

`apps/web/components/github/my-github/use-saved-presets.ts` currently invokes `crypto.randomUUID()` before the optimistic publication or any settings request. Replace only this call with `generateUUID()` from `@/lib/uuid`, retaining the prefix. Existing workspace and portable settings paths and action settlement remain authoritative. The helper's existing tests in `apps/web/lib/utils.test.ts` cover preferred and fallback sources.

## Tests

For AC-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001.1/.2 and AC-INTEGRATIONS-GITHUB-MOBILE-001.1, independently author `components/github/my-github/saved-preset-capability.test.tsx`. Render the real save dialog, actions hook, saved-presets hook, toast provider, shared controls and English catalogs, mocking only the settings API boundary. Wait for loaded settings and an enabled Save; remove `randomUUID` while retaining working `getRandomValues`. Confirm a valid save causes the expected scoped request with label, kind, query and repository, publishes the result and closes the dialog. Create two views and assert distinct UUID-shaped `p_` IDs. A same-flow control with `randomUUID` available proves the ordinary outcome. Confirm the fallback case fails for the causal pre-persistence exception before the fix and passes afterward; collection or setup failures do not count as RED. Preserve and run the existing hook preference test and helper preference tests.

## E2E tests and mobile parity

The correction is shared state/data normalization with no layout, copy, navigation, scrolling, or touch-target changes. Existing desktop menu and mobile Views drawer open the same save dialog and share the hook/action state. The mobile-parity narrow exception permits component proof alone; this package also chooses focused browser proof because the capability is browser-exposed. Add separate `tests/github/github-saved-query-capability.spec.ts` (chromium) and `tests/github/mobile-github-saved-query-capability.spec.ts` (mobile-chrome), using isolated mock-provider fixtures and the actual GitHub page. Simulate the unavailable method before page load while preserving `getRandomValues`; save and assert the persisted, visible view and dialog closure, including after reload, using phone taps on the phone path. Run both tests before the fix to show a causal behavioral RED and again after the fix for GREEN; non-collection, fixture failure or missing selector is not behavioral RED. These tests simulate browser capabilities on the test origin, not actual insecure-origin networking. Do not edit the separately owned `mobile-github-results.spec.ts` or mutate real providers.

## Work orders

- [x] [Task 01: Restore saved-query creation](task-01-restore-saved-query-creation.md)

## Verification results

Implemented and verified by the linked work order. Public docs assessment: `docs/public/mobile-remote-access.md` already documents HTTP inside an encrypted VPN and the relevant Views entry; no public docs change is needed for restoring existing support.

## Risks

- A test that only calls the UUID helper would miss the real action/persistence regression; verify observable outcome and natural API boundary.
- The desktop/phone Playwright capability override is a simulation, not an insecure HTTP transport test. Keep its fixture isolated and ensure the provider is mocked.
- Overlapping work may change test ownership; reconcile current branch and exact files before edits. Do not use another task's private reproduction.
