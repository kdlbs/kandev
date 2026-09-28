---
status: current
system: platform
requirements:
  - REQ-PLATFORM-FEATURE-TOGGLES-001
---

# Frontend Feature State System Design

## Purpose and boundaries

Platform owns the startup-effective feature map and its projection into the web
application. This design records the existing API, server-rendered bootstrap,
and Zustand state boundary. It does not change toggle definitions, defaults,
runtime-flag resolution, or backend feature gates.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-FEATURE-TOGGLES-001` | Feature map, Bootstrap flow, Failure behavior |

## Components and contracts

- `apps/backend/internal/common/config.FeaturesConfig` is the JSON shape for
  feature booleans. `apps/backend/internal/backendapp/helpers.go` serves the
  startup-effective value at the public `GET /api/v1/features` endpoint.
- `apps/web/app/actions/features.ts`, `getFeatureFlagsAction`, fetches that
  endpoint without caching. It uses `defaultFeatureFlags` as the typed key set
  and accepts only boolean values for declared keys.
- `apps/web/lib/state/slices/features/types.ts` declares the frontend feature
  keys and all-false defaults. `useFeature` reads one declared key from the
  app store.
- `apps/web/lib/state/slices/features/features-slice.ts` owns the `features`
  map and the `setFeatures` action. `createAppStore` composes this slice with
  the other app state, and `StateProvider` accepts the layout's initial state.

## Bootstrap flow

1. The backend resolves feature configuration during startup and publishes its
   effective booleans through `GET /api/v1/features`.
2. The web root layout calls `getFeatureFlagsAction` and passes its result as
   `StateProvider` initial state before rendering client components.
3. `mergeInitialState` combines supplied flags with the all-false frontend
   defaults. The root store applies the merged initial state after slice
   composition, so the server values take precedence over slice defaults.
4. Client gates read the resulting value through `useFeature`.

## State updates and composition

`setFeatures` accepts a complete `FeatureFlags` map and replaces that map in one
Immer recipe. The slice creator needs only a setter for recipe updates over
`Draft<FeaturesSlice>`. It does not need `get`, `api`, the Zustand replacement
overload, or whole-state replacement. `createFeaturesSlice` therefore receives
the recipe-only setter directly; this is a local dependency type, not a generic
slice factory. The refactor preserves the action signature and state behavior.

## Failure behavior

If the endpoint is unavailable or returns a non-success response, the server
action returns all-false defaults. Missing or non-boolean declared values also
remain false, and undeclared response keys are ignored. The store's defaults
keep client gates closed when no initial feature map is supplied. The backend
remains authoritative for protected operations; this frontend state does not
replace backend enforcement.

## Related decisions

- [Runtime feature flags](../../../decisions/0007-runtime-feature-flags.md)
- [Release toggle gating contract](../../../decisions/2026-08-01-release-toggle-gating-contract.md)
- [Runtime settings overrides](../../../decisions/0018-runtime-settings-overrides.md)
- [Restart supervisor](../../../decisions/0019-restart-supervisor.md)
