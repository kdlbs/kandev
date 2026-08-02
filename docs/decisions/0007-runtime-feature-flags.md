# 0007: profiles.yaml — runtime defaults for prod / dev / e2e

**Status:** accepted
**Date:** 2026-05-16
**Area:** backend, frontend

## Context

Kandev ships a single binary (npm/Homebrew/GitHub release) plus its Next.js web app from one repo. Two related problems were driving scope every time we cut a release:

1. **In-progress features leak into production.** Large multi-PR work (Office, ADR-0004, ADR-0005) stays in `main` for weeks before it is user-ready. Releasing in the middle exposes half-finished surfaces to users.
2. **Dev / e2e configuration is scattered.** `KANDEV_MOCK_AGENT`, `KANDEV_DEBUG_DEV_MODE`, `KANDEV_FEATURES_OFFICE`, `AGENTCTL_AUTO_APPROVE_PERMISSIONS`, `KANDEV_PLAN_COALESCE_WINDOW_MS`, etc. live as hardcoded literals in `apps/cli/src/dev.ts` and `apps/web/e2e/fixtures/backend.ts`. To answer "what does e2e mode actually set?" a contributor has to grep both files. Knobs drift between dev and e2e for no reason other than "nobody updated both places."

We needed:

1. One file that's the answer to "what's on in dev / e2e / prod?"
2. Production releases that *cannot* accidentally ship with dev/e2e knobs on, even if env vars leak.
3. Per-spec and per-host overrides that still win without special-casing.
4. Reusable plumbing — adding the next feature flag should be one registry
   binding plus the deliberately gated call sites, not a new map or switch in
   every startup path.

## Decision

A single `profiles.yaml` at the repo root, owned by the backend, declares the env-var values for three named profiles: **prod** (default), **dev** (`make dev`), and **e2e** (playwright fixtures). The backend embeds the file (`//go:embed` from `apps/backend/internal/profiles/`) and applies the active profile to its own process env at startup.

### profiles.yaml shape

```yaml
features:
  KANDEV_FEATURES_OFFICE:
    prod: "false"
    dev:  "true"
    e2e:  "true"

mocks:
  KANDEV_MOCK_AGENT:    { prod: "", dev: "true", e2e: "only" }
  KANDEV_E2E_MOCK:      { prod: "", dev: "",     e2e: "true" }
  KANDEV_MOCK_GITHUB:   { prod: "", dev: "",     e2e: "true" }
  # …

debug:
  KANDEV_DEBUG_DEV_MODE:      { prod: "", dev: "true", e2e: "" }
  KANDEV_DEBUG_PPROF_ENABLED: { prod: "", dev: "true", e2e: "" }

e2eTuning:
  AGENTCTL_AUTO_APPROVE_PERMISSIONS: { prod: "", dev: "", e2e: "true" }
  KANDEV_PLAN_COALESCE_WINDOW_MS:    { prod: "", dev: "", e2e: "2000" }
```

Top-level sections are purely for human readability (the loader walks every leaf the same way). Each leaf is keyed by the **canonical env-var name**, with a value per profile. An empty string means "leave this var unset."

### How profile selection works

`profiles.DetectEnvironment()` reads process env, in order:

| signal | profile | who sets it |
|---|---|---|
| `KANDEV_E2E_MOCK=true` | `e2e` | `apps/web/e2e/fixtures/backend.ts` |
| `KANDEV_DEBUG_DEV_MODE=true` (or `…_PPROF_ENABLED=true`) | `dev` | `apps/cli/src/dev.ts` |
| (neither) | `prod` | the default for any other launch |

`profiles.ApplyProfile()` then walks every leaf and calls `os.Setenv` **only when the var is not already set** and **only when the resolved value is non-empty**. Empty means "leave unset."

This produces the precedence chain:

```
shell env / launcher env / per-spec override   >   profiles.yaml   >   Go zero values
```

A self-hoster setting `KANDEV_FEATURES_OFFICE=true` in their k8s manifest beats the YAML's `prod: "false"`. A playwright spec setting `AGENTCTL_AUTO_APPROVE_PERMISSIONS=false` (the opposite of e2e's default) likewise wins — because the spec sets it before spawning, `ApplyProfile` sees it as already-set and skips.

### Backend wiring

- `apps/backend/internal/profiles/profiles.yaml` — the file (symlinked from `./profiles.yaml` at the repo root).
- `apps/backend/internal/profiles/profiles.go` — `DetectEnvironment`, `ApplyProfile`, `FeatureFlagDefaults`.
- `apps/backend/internal/common/config/config.go` — calls `profiles.ApplyProfile()` at the top of `LoadWithPath` (before Viper's `AutomaticEnv`) and then seeds Viper's `features.*` keyspace from `profiles.FeatureFlagDefaults()` so the typed `FeaturesConfig` populates correctly even in tests.
- `apps/backend/internal/runtimeflags/registry.go` — owns each flag's public metadata, key, environment variable, typed config reader, and typed config applier in one internal registration. It also owns the append-only identities of retired flags. `OptionsFromConfig`, `ValuesFromConfig`, and persisted-state application iterate active registrations.
- `FeaturesConfig` holds a typed `bool` per feature. Reflective tests require exact config/registry/profile key equality, complete metadata, isolated typed bindings, and no active collision with a retired identity. A frontend Vitest contract compares the backend JSON tags with `defaultFeatureFlags`.
- `apps/backend/internal/backendapp/main.go` and other backend entry paths gate construction, route registration, handlers, background jobs, and agent/MCP behavior at their narrow composition boundaries. When off, the relevant service stays nil or the operation is rejected before side effects; HTTP routes are simply never registered, so a guessed URL returns 404 (not 401).

### Frontend wiring (feature flags only)

- `GET /api/v1/features` returns the feature-flag map as JSON. Public, unauthenticated.
- A `features` slice in the Zustand store mirrors the response.
- The root layout (`apps/web/app/layout.tsx`) SSR-fetches the flags once per request and seeds `StateProvider` initialState, so the first paint reflects the deployment's flags. No flash of feature UI.
- `useFeature(name)` reads a single flag.
- Page-level gating uses Next.js `notFound()` from a server-side layout (e.g. `apps/web/app/office/layout.tsx`); nav entries use `useFeature` and render `null` when off.

### Launchers

- `apps/cli/src/dev.ts` sets only `KANDEV_DEBUG_DEV_MODE=true` — the *selector*. profiles.yaml's `dev:` column supplies the rest.
- `apps/web/e2e/fixtures/backend.ts` sets only `KANDEV_E2E_MOCK=true` — the selector for e2e. profiles.yaml's `e2e:` column supplies the rest. Per-host paths (DB, worktree base, Docker binaries) stay in the fixture because they're runtime-computed, not environmental.

## Consequences

- One artifact ships to everyone. Flag flips are a one-line YAML change.
- The "what's on in dev / e2e?" question is grep-friendly at the repo root.
- Per-spec / per-host overrides still work because `ApplyProfile` is idempotent and leaves already-set vars alone.
- Gated code stays compiled into the production binary and JS bundle. Acceptable trade-off vs build tags: the Office surface adds ≤ ~5% to the binary, and the operational simplicity (one artifact, runtime flips) is worth more than tree-shaking would be at our scale.
- Adding a new flag has one profile entry, one typed `FeaturesConfig` field, one registry registration, and the necessary backend/frontend gates. Completeness tests fail when a layer is omitted, added only on one side, or malformed.
- The 404-on-disabled-feature failure mode is deliberate: a flagged-off feature should look like it doesn't exist, not like a permission denial.

## Alternatives considered

1. **Go build tags (`//go:build office`).** Rejected. Build tags require a build-matrix split (release.yml would need "with office" and "without office" jobs), do not extend to the Next.js bundle, and make staging into "we built a special artifact for staging" rather than "we flipped a switch." Size savings don't justify the operational tax.
2. **Per-user permissions / RBAC.** Rejected for this purpose. The feature being hidden is about whether the deployment opted in, not about user authorization. Conflating them creates confusing "you have the permission but the feature isn't deployed" UX.
3. **Env vars sprinkled across launchers.** This was the starting state — what we replaced. The catalyst was discovering `dev.ts` and `e2e/fixtures/backend.ts` had drifted on `KANDEV_MOCK_LINEAR` settings.
4. **External flag service (LaunchDarkly, GrowthBook, etc.).** Rejected for v1. We don't yet need percentage rollouts, audience targeting, or remote kill-switches. When we do, `profiles.FeatureFlagDefaults()` is the right seam to plug a provider into.
5. **Two files (`features.yaml` + `dev-profile.yaml`).** Rejected during iteration. The user explicitly asked for one file; the mental model of "this section is feature flags, that section is mocks" is clearer when they're co-located.

## How to add a feature flag

1. Add an entry under `features:` in `profiles.yaml`:
   ```yaml
   KANDEV_FEATURES_<NAME>:
     prod: "false"
     dev:  "false"
     e2e:  "false"
   ```
2. Add the matching `bool` field to `FeaturesConfig` in
   `apps/backend/internal/common/config/config.go`, including explicit
   `mapstructure` and `json` tags. (No `v.SetDefault` is needed —
   `profiles.FeatureFlagDefaults()` seeds it automatically.)
3. Add one registration in `apps/backend/internal/runtimeflags/registry.go`
   with the key, env var, metadata, typed reader, and typed applier. Do not add
   a parallel per-flag map or switch, and do not reuse an identity in
   `retiredRuntimeFlagIdentities`. The registry completeness test must pass.
4. Gate every backend entry path at its narrow composition boundary. The
   disabled path must preserve the legacy behavior and must fail closed before
   deriving data, writing state, dispatching work, or exposing a capability.
5. Add the field to `defaultFeatureFlags` in
   `apps/web/lib/state/slices/features/types.ts`; the frontend normalizer is
   derived from that declaration. Gate client surfaces with `useFeature` and
   page subtrees with `notFound()` in the relevant server-side layout.
6. Add on/off tests for backend behavior, frontend normalization, and each
   meaningful user entry path. Shared refactors, migrations, and data reads
   must not be hidden behind a flag unless their disabled behavior is explicit.

## How to enable a feature for all users

For a temporary release toggle, merge the code with the flag off in every
profile. Enable it on a selected install through the admin Feature Toggles UI
or an explicit environment variable, restart, and observe it. Once the
feature is ready for everyone, change its `prod:` value in `profiles.yaml` to
`"true"` for the next release while retaining the registry metadata as a
kill-switch during the rollout. After the feature has been on by default for a
release or two and is permanent, remove the live flag end-to-end.

Removal includes the `profiles.yaml` entry, the `FeaturesConfig` field, the
active registry registration, backend gates, frontend declaration and checks,
tests, and docs. Move the removed key and environment variable to the
append-only `retiredRuntimeFlagIdentities` set. Unknown persisted overrides
remain inert, and neither retired identity may be reused.

## How to add a non-feature-flag knob

Same pattern — pick the section that matches the knob's purpose (`mocks`, `debug`, `e2eTuning`), add the entry, and the backend's `ApplyProfile` will apply it via `os.Setenv` at startup. Existing `os.Getenv` reads scattered across the backend keep working unchanged.

## References

- `profiles.yaml` (repo root, symlink) → `apps/backend/internal/profiles/profiles.yaml` — source of truth
- `apps/backend/internal/profiles/profiles.go` — loader (`//go:embed`, `DetectEnvironment`, `ApplyProfile`, `FeatureFlagDefaults`)
- `apps/backend/internal/common/config/config.go` — `LoadWithPath` calls `ApplyProfile` first thing
- `apps/backend/internal/backendapp/helpers.go` — `GET /api/v1/features` handler
- `apps/backend/internal/backendapp/main.go` — composition gates such as `initOfficeServices`
- `apps/backend/internal/runtimeflags/registry.go` — metadata and typed config bindings
- `apps/web/app/layout.tsx` — SSR-fetch + StateProvider seeding
- `apps/web/lib/state/slices/features/` — Zustand slice
- `apps/web/hooks/domains/features/use-feature.ts` — client hook
- `apps/web/app/office/layout.tsx` — page-level `notFound()` gating
- `apps/cli/src/dev.ts` — sets only the `KANDEV_DEBUG_DEV_MODE` selector
- `apps/web/e2e/fixtures/backend.ts` — sets only the `KANDEV_E2E_MOCK` selector + per-host paths
