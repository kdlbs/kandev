# ADR-2026-11-20-claude-exact-model-alias-validation: Validate explicit Claude model conflicts at save

**Status:** accepted
**Date:** 2026-11-20
**Area:** backend, frontend

## Context

In issue #5183, the profile requests strict `sonnet` and sets
`ANTHROPIC_MODEL=claude-sonnet-5-5`. The executor advertises the full ID.
Kandev requires the exact requested ID and stops before sending a prompt.
The reporter verified that selecting the full ID let the same automation start.
[Issue and workaround](https://github.com/kdlbs/kandev/issues/5183#issuecomment-6720279723)

Claude uses `ANTHROPIC_MODEL` to select an initial model. Claude has separate
variables for family alias bindings. The managed bridge returns selectable IDs
and a current model ID. Those fields do not establish a binding from an absent
alias to a selectable ID. [Claude model configuration](https://code.claude.com/docs/en/model-config)
[Managed bridge 0.107.0](https://github.com/agentclientprotocol/claude-agent-acp/blob/c5c120dba9c15bd88ea5d355fa3b77d1fc341392/src/session-model.ts)

## Decision

For a strict Claude ACP profile, reject a newly saved or changed bare family
alias paired with a different literal `ANTHROPIC_MODEL` in that profile.
Ask the user to select the intended full ID or remove the environment entry.
Existing conflicting profiles keep their stored values and allow unrelated
edits. Executors continue to enforce exact model selection before inference.

## Consequences

The settings check works without a host or remote catalog probe. It also
rejects new conflicting pairs on executors that currently advertise the alias.
Users who set a separate Claude startup model and rely on a subsequent alias
switch must make that selection explicit when changing the pair. Values from
secrets, external settings, or executor environments remain subject to runtime
validation.

## Alternatives Considered

- Treat the reported current model as the requested alias. A current model can
  come from an independent environment or settings choice. Substitution would
  weaken strict selection.
- Gate saves on the host catalog. Host and executor catalogs can differ.
  That gate could reject valid remote selections.
- Add an executor-specific model probe to profile settings. A profile can
  run on several executors. A probe would require a selected executor and a
  bound launch context, and its answer would apply only to that context.

The [exact-model policy](2026-09-15-explicit-profile-model-strictness.md) and
[owning requirement](../specs/agents/requirements/no-silent-model-fallback.md)
define the retained runtime contract.
