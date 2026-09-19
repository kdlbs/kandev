---
id: "12f-shared-review-detail"
title: "Extract the GitHub change-request detail as a host UI contract"
status: completed
wave: 3f
depends_on: ["12d-host-native-task-link-parity"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
decision: "../../decisions/2026-08-06-plugin-code-host-dashboard-parity.md"
---

# Task 12f: Extract the GitHub change-request detail as a host UI contract

## Intent

Use one host-owned review layout for GitHub and compatible plugin providers. Providers
map data and actions; they do not recreate headers, tabs, sections, buttons, or mobile
geometry.

## Owned paths

- `apps/web/components/integrations/change-request-detail.tsx` and tests
- `apps/web/components/github/pr-detail-panel.tsx` and focused regression tests
- `apps/web/lib/plugins/{types,host-api}.ts` and focused tests
- plugin API/authoring docs

## Implementation

1. Define a provider-neutral detail model for identity/state, author/timestamps,
   branches/stats, description, review summaries, checks, threaded comments, and sync
   time, plus capability-gated action descriptors/callbacks.
2. Extract the pure presentation from GitHub `PRDetailContent` into
   `ChangeRequestDetail`; retain GitHub fetching, auth, feedback caching, and mutations
   in a GitHub adapter.
3. Expose the component and its additive model through versioned `host.ui`.
4. Preserve GitHub's header, collapsible Description/Reviews/Checks/Comments sections,
   action placement, loading/error treatment, and one-scroll-owner responsive layout.

## TDD and acceptance

1. RED component/API tests require the missing host component, model, action dispatch,
   and responsive semantics.
2. GREEN GitHub adapter tests prove existing data and supported actions map without a
   visual regression.
3. Run focused Vitest, web typecheck/lint, and existing GitHub review E2E coverage.

## Mobile contract

The component fills the native mobile Review location, owns no route or drawer, retains
one vertical scroll owner, prevents horizontal overflow, and keeps action targets at
least 44px.

## Risks

- Extraction must not weaken GitHub's auth-dependent approve/merge behavior.
- Provider-neutral types cannot encode GitHub or Bitbucket REST payloads.
- Unsupported actions must be absent, not disabled forever or replaced with new layout.

## Completed verification (2026-08-06)

- GitHub and registered providers now render the same host `ChangeRequestDetail` component.
- Focused component/adapter tests, web typecheck, and lint passed.
- Live desktop and touch runs rendered Description, Reviews, CI Checks, and Comments with
  one scroll owner and no horizontal overflow.
