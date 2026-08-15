# ADR-2026-08-08-utility-agent-profile-execution: Utility Agents Use Execution Profiles

**Status:** accepted (amended 2026-08-14)
**Date:** 2026-08-08
**Area:** backend, frontend, protocol

## Context

Utility agents currently persist an inference-agent identifier and model directly. The host-utility
runner consequently launches one-shot ACP subprocesses without the selected agent profile's CLI
flags, environment, command prefix, mode/config options, or permission policy. Selecting a model is
therefore insufficient to make a non-interactive utility job behave like the operator-configured
agent, and a provider may request permission after the caller can no longer respond.

## Decision

### Amendment (2026-08-14)

[ADR-2026-08-13-dynamic-agent-profile-routing](2026-08-13-dynamic-agent-profile-routing.md)
extends eligible utility selections to dynamic profiles. The utility caller
still supplies one logical `agent_profile_id`; a shared resolver either launches
the concrete profile or delegates to the dynamic conductor. Dynamic routing may
select another configured concrete candidate under its own policy. The durable
utility call records both the logical selection and concrete
`execution_profile_id`. The permission and fail-closed rules below apply to
each selected concrete attempt.

Utility-agent configuration stores an `agent_profile_id`, not an independent agent/model pair. A
built-in action may inherit the user's default utility profile; a built-in override and every custom
utility agent reference one eligible concrete or dynamic global agent profile. At invocation start, the backend resolves
that profile and uses its complete launch-affecting configuration for both host-sessionless and
task-session-bound utility execution.

`internal/agent/hostutility` remains the sessionless execution tier established by ADR 0002, but its
prompt contract becomes profile-aware. Profile resolution is backend-owned and fail-closed: missing,
deleted, disabled, CLI-passthrough-only, or non-inference-capable profiles never fall back to another
profile or to raw provider defaults. One-shot calls have no interactive permission-response surface;
they apply the profile's auto-approval/CLI policy, and reject an otherwise unresolved permission
request rather than waiting for input.

Legacy agent/model selections migrate only when they match exactly one eligible profile. An
ambiguous or unmatched selection becomes unconfigured and must be explicitly repaired in Settings.
This prevents migration from silently choosing a materially different permission policy.

The raw agent-capability probe/prompt API is not a configured utility agent and remains keyed by
agent type. Plugin configuration continues to select a utility-agent ID; the selected utility agent
now delegates to its resolved profile.

## Consequences

- Utility jobs use the same operator-managed runtime and permission configuration as normal profile
  launches, so profile changes apply to the next invocation without duplicating settings.
- The Utility Agents page becomes simpler: one default profile picker and optional profile overrides
  replace agent-family/model selectors.
- Profile deletion, disabling, or incompatibility produces an actionable configuration failure
  instead of an unrelated fallback.
- Migration can require one explicit user choice when several profiles share an agent/model pair;
  that disruption is preferable to silently choosing permissions.
- The one-shot agentctl contract must carry profile-derived command, environment, mode/config, and
  permission policy, increasing the profile-aware execution test surface.

## Alternatives Considered

1. **Keep agent/model selection and add separate utility permission toggles.** Rejected because it
   duplicates profile-owned launcher policy and lets the two configurations drift.
2. **Copy a profile's values into each utility-agent row.** Rejected because later profile edits
   would not affect existing utility agents and secret-bearing environment settings would be
   duplicated.
3. **Choose the first profile matching a legacy agent/model pair.** Rejected because profiles with
   the same model can intentionally carry different permissions, flags, wrappers, or credentials.
4. **Run utility jobs through the interactive task-agent runtime.** Rejected because sessionless
   utility actions do not own a task, executor, workspace, or durable conversation; ADR 0002's
   one-shot host tier remains the appropriate lifecycle.
