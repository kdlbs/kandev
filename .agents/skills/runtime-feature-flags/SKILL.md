---
name: runtime-feature-flags
description: Add, roll out, promote, graduate, or remove Kandev runtime feature flags and release toggles across the backend and frontend. Use whenever a task mentions a feature flag, release toggle, staged rollout, kill switch, or graduating a flag.
---

# Runtime Feature Flags

Use this checklist for temporary release toggles and risky features. It is
self-contained: do not depend on the agent having read an ADR or public docs.
Read `apps/backend/AGENTS.md` and `apps/web/AGENTS.md` when the change touches
those subtrees.

## Invariants

- The backend is authoritative. A disabled feature must not be reachable through
  HTTP, WebSocket, MCP, agent-tool, or background-job entry points.
- A new release toggle is off in every shipped profile when it is merged. The
  disabled path preserves the existing behavior and fails closed before deriving
  data, writing state, dispatching work, or exposing a capability.
- Effective values are ordered: explicit environment variable, SQLite override,
  then profile default. An explicit environment variable locks the admin UI.
- Use one identity across layers: `features.<camelCaseKey>`,
  `KANDEV_FEATURES_<UPPER_SNAKE_CASE>`, a Go `FeaturesConfig` field, its JSON tag,
  and the matching frontend key. Never add a parallel flag map or switch.
- Never reuse an identity listed in `retiredRuntimeFlagIdentities`.

## Add a flag

Update these layers in the same change:

1. **Profile default:** add `KANDEV_FEATURES_<NAME>` under `features:` in the
   root `profiles.yaml`; use `prod: "false"`, `dev: "false"`, and `e2e: "false"`
   for a release toggle unless the task explicitly documents a test-only
   exception.
2. **Backend config:** add a `bool` field with explicit `mapstructure` and
   `json` tags to `apps/backend/internal/common/config/config.go`.
3. **Registry binding:** add exactly one complete registration to
   `apps/backend/internal/runtimeflags/registry.go`: key, env var, kind, label,
   description, stability, risk metadata, restart/mutability metadata, typed
   `read`, and typed `apply` functions.
4. **Backend gates:** gate construction and every enabled-only entry path at
   the narrow composition boundary. Do not only hide the frontend; direct
   callers must receive the legacy behavior or a safe rejection.
5. **Frontend contract:** add the all-off key to
   `apps/web/lib/state/slices/features/types.ts`. Use `useFeature()` for client
   surfaces and `notFound()` from the relevant server layout/page when a route
   subtree must be unavailable. SSR data must remain fail-closed.
6. **Tests:** add enabled/disabled behavior tests for each changed backend path
   and frontend surface. Run the existing registry/profile/frontend contract
   tests; add focused tests for normalization, route visibility, and disabled
   side-effect prevention where applicable.

The completeness checks require exact equality between profile keys, typed
backend fields/registrations, and frontend default keys. They do not discover
semantic call sites, so trace the feature's HTTP, WebSocket, MCP, agent, worker,
and startup paths manually.

## Roll out and promote

1. Merge with all shipped profile defaults off.
2. Enable one installation through **Settings > System > Feature Toggles** or an
   explicit environment variable, restart when metadata requires it, and test
   real workflows.
3. When ready for everyone, change only the `prod` profile value to `"true"` for
   the next release. Retain the registry entry and backend/frontend gates as a
   kill switch so operators can still disable the feature.

## Graduate and remove the flag

After the feature has proven itself as the default-on behavior, make the new
behavior unconditional and remove the live flag end-to-end:

- remove the profile entry and `FeaturesConfig` field;
- remove the active registry registration;
- remove backend conditionals and legacy branches;
- remove the frontend default, `useFeature()` checks, and route gates;
- remove flag-specific tests and documentation while keeping permanent behavior
  coverage.

Before removing the registration, append its exact key and environment variable
to `retiredRuntimeFlagIdentities` in `registry.go`:

```go
{key: "features.example", envVar: "KANDEV_FEATURES_EXAMPLE"},
```

Do not delete old `runtime_flag_overrides` rows as part of graduation. Unknown
rows are intentionally inert, and the retired identity prevents stale operator
state from reactivating a future feature. Never reuse either the key or env var.

## Verification and handoff

Run the focused checks appropriate to the change:

- from `apps/backend`: `rtk go test ./internal/runtimeflags ./internal/common/config ./internal/profiles`;
- from the repository root: `rtk make -C apps/backend lint`;
- from `apps`: `rtk pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts`;
- from `apps/web`: `rtk pnpm run typecheck` and `rtk pnpm run lint`;
- run affected E2E coverage when the gated surface is user-visible;
- run `rtk git diff --check` before handoff.

Report the flag key/env identity, profile defaults, every gated entry path,
disabled/enabled test evidence, restart requirements, and whether the change is
still a kill switch or has been fully graduated.
