# ADR-2026-08-18-context-reset-preserves-runtime-configuration: Preserve ACP Runtime Configuration Across Context Reset

**Status:** accepted
**Date:** 2026-08-18
**Area:** backend, protocol, workflow

## Context

An ACP context reset creates a new provider session. The new session starts with provider defaults for its model, permission mode, and configuration options.

Kandev restores the model and mode through separate paths. These paths read mutable state after session creation, and they do not restore configuration options.

Fresh-session events can replace the persisted user selection before restoration reads it. A workflow can then continue with different runtime permissions or model configuration.

## Decision

A context reset changes conversation context only. It does not change the effective ACP runtime configuration of the task session.

Before Kandev creates a provider session, the lifecycle captures one immutable `SessionRuntimeConfig` value. This value contains the effective model, permission mode, and all selected ACP configuration options.

The capture uses profile state, live state, provider state, and explicit runtime overrides through the existing precedence rules. The capture completes before fresh-session events can publish provider defaults.

Kandev restores the captured value in this order:

1. Apply the model through the executor-owned model policy.
2. Apply the permission mode.
3. Apply each configuration option in stable option-ID order.

The same rule applies to the ACP reset path and the process-restart fallback. Fresh provider catalogs remain authoritative for supported values, but they cannot redefine restoration intent.

If one captured field cannot be restored, Kandev reports a restoration error. A workflow reset does not send its automatic prompt with a partial configuration.

Provider convergence events remain authoritative for the state that the provider accepted. Kandev does not fabricate restored state and cannot roll back the completed conversation reset.

MCP attachments remain session-construction inputs and use their existing resolution path. Authentication, credentials, CLI flags, environment values, executor selection, and profile identity are not ACP runtime configuration.

## Consequences

- Model, mode, and model-adjacent options have one reset lifecycle and one precedence model.
- A context reset cannot silently reduce permissions or change reasoning and collaboration options.
- The reset path must wait for model-aware option availability before it applies dependent options.
- A restoration error blocks an automatic workflow prompt, but the new provider conversation can already exist.
- Runtime state, explicit overrides, provider-default baselines, and original workflow snapshots keep their existing ownership rules.

## Alternatives Considered

- Restore only the model and mode: rejected because provider-defined options also change agent behavior.
- Read persisted values after session creation: rejected because fresh default events can replace those values first.
- Reapply the current agent profile: rejected because user and workflow changes can differ from the profile.
- Continue after a partial restoration: rejected because the next prompt can run with different permissions or execution settings.
- Suppress all fresh-session events during reset: rejected because Kandev still needs the fresh capability catalogs and accepted provider state.
