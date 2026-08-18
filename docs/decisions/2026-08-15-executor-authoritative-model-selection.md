# ADR-2026-08-15: Let the Executor Own Model Selection

**Status:** accepted
**Date:** 2026-08-15
**Area:** backend, frontend, protocol, persistence

## Context

The Kandev host probes agent capabilities before a task starts.
The selected executor starts another agent process and another ACP session.

These two sessions can advertise different model catalogs.
Credentials, configuration, agent versions, account permissions, and provider state can cause the difference.

Optional configuration copying can reduce the difference.
It cannot guarantee equal catalogs for all providers or accounts.

The current strict policy fails a launch when the executor omits the profile model.
The frontend can also block the profile from host-probe data.

This policy makes a host observation authoritative for a different runtime.
It also prevents the agent from using its valid executor default.

## Decision

### The executor catalog is authoritative

The ACP session in the selected executor owns model availability at launch.
The host probe remains an editing hint.

The host and executor catalogs do not need to match.
Kandev does not block task creation because the host probe omits a saved model.

Kandev keeps the saved profile model unchanged.
The runtime decision does not rewrite the agent profile.

### Kandev does not set an unadvertised model

After ACP initialization, Kandev reads the executor model catalog and current model.
Then Kandev applies this order:

1. If the requested model is advertised, Kandev applies it.
2. If an explicit fallback is advertised, Kandev applies that fallback.
3. Otherwise, Kandev does not call the model-selection method.
4. The agent continues with its current or default model.

An empty model catalog means that the requested model is not advertised.
Kandev does not use a speculative model-selection call in this case.

If the agent does not support model selection, Kandev keeps the agent default.
Kandev also creates the same warning when the profile requested a model.

If an advertised model fails to apply, Kandev reports an explicit launch error.
This error can show a transport, provider, or protocol problem.

The existing `auto_fallback` option keeps its best-effort behavior for an apply error.
This mode continues with the agent default and creates a warning.

### Every model change is visible and durable

Kandev creates one persisted status message when it does not apply the requested model.
The message uses the warning variant.

The message metadata contains these fields:

- A stable semantic kind, `model_selection_warning`.
- The reason, such as `requested_not_advertised` or `selection_unsupported`.
- The requested model.
- The effective model when the agent reports it.
- The explicit fallback model when Kandev applies it.
- The agent provider.
- The executor type and executor profile ID.

If the effective model is unknown, the UI shows `provider default, model not reported`.

The localized message explains that Kandev did not set the requested model.
It also tells the user to inspect credentials, copied configuration, and the agent version.

The message survives browser reload and backend restart.
Kandev creates at most one message for one session-start model decision.

An event can update live model-selector state.
The persisted message remains the user audit record.

### Configuration copying remains optional

Portable configuration bundles are a parity aid.
They are not a precondition for a successful task launch.

If the user does not copy configuration, this model-selection policy still applies.
The executor default keeps the task operational and the warning explains the difference.

### Office routing remains separate

This decision controls the initial model selection inside one executor session.
It does not change Office provider routing after a provider error.

Office uses the workspace route policy from `2026-08-08-provider-neutral-agent-error-recovery.md`.
That policy can select another configured execution profile after a runtime error.

## Consequences

- A model-catalog difference no longer stops a task before the agent can start.
- Kandev never sends an unadvertised model to the executor agent.
- The user receives a durable explanation for every default or explicit fallback.
- Host capability data no longer acts as an executor launch gate.
- A profile can remain useful across executors with different provider accounts.
- An advertised model apply error remains visible and can stop the launch.
- Configuration copying can improve parity without becoming hidden authority.

## Alternatives Considered

1. **Require equal host and executor catalogs.** Rejected because Kandev cannot control provider accounts, caches, or agent versions.
2. **Fail when the requested model is absent.** Rejected because the agent default can still operate and the user requested continuation.
3. **Call the model method and ignore its error.** Rejected because this hides protocol errors and creates an unnecessary failed request.
4. **Rewrite the saved profile model.** Rejected because one executor observation must not change the user choice for all executors.
5. **Rely on configuration copying only.** Rejected because copied files cannot guarantee provider authorization or equal runtime state.
6. **Show only an ephemeral toast.** Rejected because the explanation must survive reload and remain in the task history.
