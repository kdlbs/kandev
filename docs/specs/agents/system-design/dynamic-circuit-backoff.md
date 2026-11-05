---
status: current
system: agents
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
  - REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001
created: 2026-10-20
updated: 2026-11-01
owners:
  - kandev
---

# Dynamic circuit defaults

## Purpose and ownership

Agents owns provider resource health and the policy used by its runtime consumers.
Tasks, Office, utility invocations, and the provider-health view consume that
policy without owning separate backoff settings. This design extends
[dynamic routing](dynamic-agent-routing-01.md) and
[shared circuit admission](dynamic-circuit-health.md).

One product gate, `features.dynamicAgentRouting`, controls dynamic execution and
fixed-profile circuit protection. The registry has one backoff algorithm. Removing
the subordinate release toggle does not promote dynamic agents to default-on.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001` (`.20`, `.21`, `.24` through `.26`) | Default policy; product gate; retirement; persistence; operator surface |
| `REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001` (`.1` through `.3`) | Shared consumers; product gate; persistence |

## Default policy

`CircuitRegistry.OpenFailure` always uses the existing `circuitBackoffLadder`.
There is no `escalation` field, constructor option, `EscalationEnabled` query, or
fixed-minute alternative branch. No replacement flag or configurable timing
knob is introduced.

The existing policy helpers remain responsible for incident counting, deadline
selection, inferred reset bounds, and holds. `ReleaseProbe` always advances the
count on a failed owned probe. Its existing caller-supplied backoff remains a
minimum; the ladder can extend it. Successful probes retain the count.
`ResetFailures` remains a count-only operation after a clean completion.

The selected defaults retain the current enabled policy: 1m, 5m, 15m, 30m, 1h,
2h, then 4h. Keeping these values avoids combining flag removal with a new
provider policy. Explicit provider reset times can extend the deadline beyond
the ladder cap. An owner-release hold remains separate from timed backoff.

## Product gate and shared consumers

`ProfileExecutionResolver.Enabled()` remains the execution boundary. It is
initialized from `FeaturesConfig.DynamicAgentRouting` in
`backendapp.initDynamicRuntimeResolver`. Keep its profile resolution, recovery,
and manual-action gates. Fixed-profile suspension checks and failure charging
retain their existing enabled checks.

Remove only the secondary-policy checks from `ResetCircuitFailures` and
`ResetFixedProfileCircuitFailures`. Both reset paths must still return without
mutation when the dynamic-agent product gate is off. Existing execution,
profile, generation, turn, and captured-resource ownership checks remain.

Creating or restoring the registry is passive. Reading retained state while the
product gate is off cannot admit a dynamic profile, schedule a retry, acquire a
probe, charge a failure, or reset a count. Existing read-only circuit visibility
and its access boundary do not change.

The task queue, automation dispatcher, and Office scheduler keep their current
wait ownership, auto-run rules, suspension handling, and retry budgets. Changing
policy defaults must not introduce a second scheduler or replay path.

## Runtime-flag retirement

Remove the active `features.dynamicCircuitBackoffEscalation` registration and its
typed config field, profile entry, frontend default, startup option, and public
configuration row. Root `profiles.yaml` is a symlink to the embedded profile
file; edit that single source once.

The registration-owned config bindings already iterate `registrations`. Do not
add a parallel switch or map to `runtimeflags/config.go`.

Append this exact pair to the existing `retiredRuntimeFlagIdentities` registry:

- Key: `features.dynamicCircuitBackoffEscalation`
- Environment variable: `KANDEV_FEATURES_DYNAMIC_CIRCUIT_BACKOFF_ESCALATION`

Keep the existing collision and completeness checks. The removed identity has
no definition, feature-response field, runtime catalog entry, environment lock,
or settings card. A stale true value cannot enable dynamic agents. A stale false
value cannot select the former fixed-minute policy. Existing source precedence
and restart requirements for the remaining product flag stay intact.

`features.dynamicAgentRouting` remains off in prod, dev, and e2e. The separate
future [dynamic-routing graduation](dynamic-routing-graduation.md) is not part
of this change. Other application flags and per-candidate routing policies are
outside this retirement.

## Persistence and upgrade behavior

Keep `dynamic_resource_circuits.failure_count`, its additive migration, UTC
storage, snapshot filtering, pending-write handling, and hold-release rollback.
No schema rewrite, count backfill, circuit closure, deadline shortening, or data
delete is needed.

`Restore` always retains positive saved counts, including counts on closed
circuits. A missing or zero count remains zero. Installations whose earlier
non-escalating policy wrote zero start with that evidence; do not invent a
failure history. Existing open deadlines and probe ownership are loaded without
recalculation. The next qualifying failure uses the restored count.

Retain old `runtime_flag_overrides` rows. They are inert because the active
registry no longer recognizes the retired identity. This preserves operator
state for an older binary without allowing the retired flag to alter new code.
A downgrade can interpret those rows under its older policy; downgrade semantics
are not a reason to keep a compatibility branch in the new registry.

Disabling the product flag is an execution kill switch, not a health reset.
On a later restart and re-enable, retained failures continue the ladder until
an owned clean turn resets them. A restart alone never releases an account hold.

## Operator surface, desktop, and phone

The remaining Dynamic agent routing card describes routing, fixed-profile
admission, the default backoff, and owner-release holds. Metadata continues to
come from `runtimeflags/registry.go`. No new frontend translation keys or
per-profile controls are required for removing the old card.

`FeatureTogglesSettings` renders the backend catalog through the existing
`FeatureToggleCard`. Remove the retired flag at the source; do not hide it with
a frontend-only filter. Use the existing settings route on both desktop and
phone. Preserve its scrolling, save/reset, environment lock, restart notice,
accessibility, and touch behavior. The plan supplies the affected-row preview.

Readback remains observational. The registry can retain an expired open row;
the existing card uses the deadline to show availability without changing it.
Circuit snapshots, failure counts, prompts, and credentials acquire no new
access paths from this change.

## Verification boundaries

Use the default registry constructor in policy tests. Explicitly enabling the
removed option would hide a startup-wiring regression. Cover the full ladder,
one-incident behavior, failed and successful probes, clean completion, reset
hints, and owner holds. Existing provider-specific classifiers remain consumers
of the same qualifying-failure contract.

Retirement tests exercise true and false old environment values and retained
SQL overrides. Gate tests exercise both values of the remaining product flag.
Real-store tests prove count preservation and next-failure continuation after
restore, including SQLite and the existing optional PostgreSQL test fixture.

Desktop and phone browser cases read the real runtime catalog, retain the
Dynamic agent routing control, reject the retired control, and survive reload.
Do not use a stubbed list of flags as evidence of runtime retirement. A browser
test does not need a provider-outage clock seam; deterministic backend tests own
the timing policy and stored-state proof.

## Related decisions

- [Runtime feature flags](../../../decisions/0007-runtime-feature-flags.md)
- [Release-toggle gating contract](../../../decisions/2026-08-01-release-toggle-gating-contract.md)

The existing flag and retirement architecture is retained. This package removes
a subordinate policy switch inside the still-opt-in feature; it does not create
a new architectural boundary.
