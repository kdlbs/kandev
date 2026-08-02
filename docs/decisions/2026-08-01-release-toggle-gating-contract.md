# ADR-2026-08-01-release-toggle-gating-contract: Release Toggles Are Install-Wide Fail-Closed Gates

**Status:** proposed
**Date:** 2026-08-01
**Area:** backend, frontend, workflow

## Context

Kandev already resolves startup feature values from explicit environment,
persisted installation overrides, and `profiles.yaml`. Adding a frontend-visible
flag still duplicates its identity and config behavior across backend maps,
switches, frontend types, and defaults. More importantly, a UI preference or a
single late behavior check does not make a risky PR safe when direct callers,
agent tools, background work, shared initialization, or migrations can bypass
the gate.

## Decision

Temporary release toggles use the existing install-wide runtime-flag system;
they are not represented as portable per-user preferences. New release toggles
ship off in `prod`, `dev`, and `e2e`, and dedicated tests or selected
installations opt in explicitly.

The backend is authoritative. Every externally reachable entry path is gated at
its narrowest composition or capability boundary, and disabled requests fail
before mutation or dispatch. Frontend checks control discovery and presentation
but never substitute for backend enforcement. Schema changes, startup work, and
shared refactors remain independently safe because code executed outside the
gate is not protected by it.

The internal registration in `internal/runtimeflags/registry.go` becomes the
single backend binding for registry key, environment variable, config read, and
config apply behavior; the public `RuntimeFlagDefinition` remains metadata-only.
Generic resolution iterates registrations instead of adding each flag to
separate maps and switches. The frontend derives its feature-name type and
all-false defaults from one declaration. CI requires exact registry,
`FeaturesConfig`, profile, and frontend-default key equality and validates
registration metadata and typed binding isolation.

Rollout is staged: merge default-off, enable selected installations, ship one
default-on release with the kill switch retained, then remove the flag and
legacy behavior. Graduation moves the key and environment variable to the
append-only retired-identity set in `internal/runtimeflags/registry.go`;
retired identities are never reused. Unknown persisted overrides are ignored
rather than deleted so downgrade compatibility does not mutate operator state.

## Consequences

- Medium and large changes can merge without changing default behavior, while a
  real installation can opt in through the existing admin UI or environment.
- Direct clients and asynchronous entry paths cannot bypass an off flag.
- Flag addition and removal have fewer mechanical edit sites and generic tests
  catch incomplete plumbing.
- Each gated feature still needs deliberate rollback-safe data and migration
  design; a flag is not a substitute for compatibility.
- Retaining the kill switch for one default-on release delays complete code
  removal, but preserves rollback during the highest-exposure stage.
- Unknown override rows can accumulate, but remain inert and preserve downgrade
  behavior. The source-controlled retired-identity set prevents stale database
  or environment values from acquiring new meaning.

## Alternatives Considered

1. **Use per-user preferences as release controls.** Rejected because rollout
   ownership is installation-wide, direct callers can bypass UI preferences,
   and portable settings add persistence plumbing that must later be removed.
2. **Keep the current per-flag maps and switches.** Rejected because each flag
   multiplies addition/removal sites and makes drift likely as gated PR volume
   grows.
3. **Generate flag code from YAML.** Rejected for now because code generation
   adds build and review machinery when a definition-owned binding removes the
   duplication without generated artifacts.
4. **Use reflection in production to bind `FeaturesConfig`.** Rejected because
   explicit typed bindings are easier to navigate and fail at compile time;
   reflection remains acceptable in completeness tests.
5. **Adopt an external rollout service.** Rejected because Kandev needs
   installation opt-in and a local kill switch, not cohorts, percentage rollout,
   or a hosted control plane.
