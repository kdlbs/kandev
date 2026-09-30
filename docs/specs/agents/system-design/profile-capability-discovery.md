---
status: current
system: agents
created: 2026-09-29
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
  - REQ-AGENTS-PROFILE-DISCOVERY-003
---

# Profile capability discovery system design

## Purpose and boundaries

Profile capability discovery uses a validated host launch context for both baseline and model-dependent probes.
It extends the existing host utility boundary without changing session startup policy or managed package activation.

The current profile refresh calls `FetchDynamicModels(agentName, refresh)` and `ResolveAgentModelConfig` without profile launch settings.
Both paths reach `hostutility.buildProbeRequest`, which supplies only `agents.RuntimeEnvFor` and the managed inference command.
Session startup separately consumes `AgentProfileInfo.EnvVars`, `CLIFlagTokens`, and `CommandPrefixTokens`.

Observed regression: Codex ACP 2.0.0 starts bundled Codex 0.158.0 and omits GPT-6.1 Sol.
The same bridge with `CODEX_PATH` pointing to Codex 0.159.0 advertises that model.
These versions describe the reproduction, not a production rule or compatibility allowlist.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-PROFILE-DISCOVERY-001 | API and authority; Launch context; Probe execution |
| REQ-AGENTS-PROFILE-DISCOVERY-002 | Cache identity and refresh; Failure and observability |
| REQ-AGENTS-PROFILE-DISCOVERY-003 | Editor state; Responsive behavior |

## API and authority

Retain `GET /api/v1/agent-models/:agentName` for agent-wide discovery.
Add `POST /api/v1/agent-models/:agentName/probe` for profile discovery.
Extend the existing `POST /api/v1/agent-models/:agentName/resolve` with the same optional launch context.
Existing callers without that context retain the agent-wide behavior.

The new body contains `profile_id`, optional `launch_settings`, and `refresh`.
The resolve body also contains its existing `model`, `mode`, and `config_options` fields.
`launch_settings` is a typed snapshot containing exactly `env_vars`, `cli_flags`, and `command_prefix`.
These fields reuse profile DTO types. A supplied snapshot replaces all three launch-setting fields for this request only.
Empty arrays and an empty prefix explicitly clear inherited values. Reject incomplete snapshots and unknown fields.

- Saved profile: require an authorized concrete `profile_id`. Without a snapshot, load its current saved launch settings.
- Saved draft: authorize that profile, then validate the complete snapshot as an execution-setting edit.
- New draft: omit `profile_id`, require the complete snapshot, and enforce the existing authority to create that agent's profiles.
- Reject a profile whose agent differs from the route, a foreign profile, or a dynamic profile before spawning.
- Do not accept a package name, executable command, protocol, working directory, or raw secret value through a new special-purpose field.
- Resolve secret references on the server under existing secret-scope rules. Preserve ordinary literal environment values as profile input.

Require the same execution authority for both baseline and resolve requests with a launch context.
The legacy read endpoint must not become an alternate path for executing an unauthorized draft.
No changes to public MCP tool schemas are required. Context-free consumers retain their present behavior.

The response retains existing capability or model-option DTO fields and adds an opaque `context_revision` for profile requests.
It identifies the resolved launch context, not a database revision, and contains no raw environment or CLI values.

## Launch context

Add a small typed resolver in the agent settings domain, shared by both profile endpoints.
Keep persistence lookup and authorization outside `hostutility`.
The resolver returns validated profile inputs, effective managed command identity, and resolved environment values.

Reuse `cliflags.Resolve`, `cliflags.ValidateCommandPrefix`, and `cliflags.Tokenise`.
Reuse the existing environment definition and secret-resolution primitives used by lifecycle launch.
Do not introduce a second precedence table or import the lifecycle manager into the settings controller.
If extraction is required, move the narrow pure resolution helper into the existing agent/common environment boundary.

Merge host runtime defaults and profile environment entries with the existing agent-profile precedence.
Preserve `StripEnv` and managed npm project isolation.
Profile values must reach the child after inherited environment sanitization, exactly as supported by session launch.
Missing or inaccessible secret bindings fail before spawning. Do not copy the utility prompt path's omission of unresolved secrets.

Resolve the managed package command through `resolveInferenceCommand` on every request before cache lookup.
Append the validated CLI tokens once, and apply the validated prefix once.
The command uses the effective managed version. Profile fields cannot replace its trusted package identity.
Environment values such as `CODEX_PATH` remain provider-owned runtime inputs, without a Codex branch in Kandev.

## Probe execution

Extend the host utility probe input to accept an already-authorized launch context.
Populate `InferenceConfigDTO.Env`, `CLIFlags`, and `CommandPrefix` for both baseline and model-option resolution.
Keep the host temporary work directory and existing bounded process cleanup.
The ACP probe omits the dedicated model flag for initial discovery, then applies a requested model through existing session-model logic.
It must not strip user CLI flags that select a provider configuration or runtime.

`ACPInferenceExecutor.Probe` already calls `buildACPCommand` and `sanitizeEnvForAgent`.
Verify that command-prefix resolution selects the prefix executable consistently with prompt execution.
Preserve command allowlists and operator-defined command rules. Unsupported prefixes fail visibly instead of being ignored.

Audit secondary discovery commands, including OpenCode's `loadOpenCodeModels` refresh path.
They must receive the same applicable environment, prefix, and provider context or defer to the ACP snapshot.
Never merge a default-context secondary result into a profile snapshot.
Native protocol executors receive the same typed context only where their existing probe supports it.
An unsupported context returns a typed unsupported result without a default-context fallback.

Discovery creates no Kandev task/session and sends no model prompt.
A provider can internally allocate a temporary protocol session during the handshake, as it does today.
Cleanup terminates the owned process tree on success, timeout, cancellation, and failure.

## Cache identity and refresh

Separate profile observations from the existing agent-wide capability cache.
Use a bounded in-memory profile cache with the existing five-minute resolution TTL and at most 256 entries.
Evict least-recently-used entries. Cache successful snapshots and explicit unsupported results, but not transient failures.

One context identity includes authorization scope, concrete profile identity or new-draft scope, agent identity, protocol,
effective managed command/version, ordered arguments, prefix tokens, resolved environment, strip policy, and discovery generation.
Model-option keys additionally include the model, mode, and canonical option map.
Environment map order is irrelevant. Argument and prefix order is significant.

Derive the server identity with an HMAC using a process-local random key.
Resolve secret references before hashing, including their current values, but never retain those values in cache keys or results.
Do not return or log an ordinary hash of a credential. Only the opaque context identity can cross response boundaries.
The launch request holds resolved values only for its bounded execution lifetime.

Refresh bypasses matching entries and increments the generation for that complete authorized launch context.
Refresh in one profile does not change revisions or cache entries for another profile.
An older in-flight operation can finish, but cannot cache or return a result after its context generation changes.
Runtime activation advances a separate agent-wide generation and invalidates profile contexts for that agent as well as the existing agent-wide caches.
Capture that generation before resolving the managed command and recheck it after resolution so a command snapshot that spans activation is rejected.
Re-resolve runtime identity and secrets on each API call, including cache hits.
Check both generations before cache reads, cache writes, and returning probe results.
Serialize generation changes with profile-cache reads and writes so invalidation cannot race a stale cache publication.
Single-flight work shares only an identical authorized context and generation.
Reuse existing bounded host-instance operation admission; do not create an unbounded process pool.

The frontend keys profile state by profile identity, its complete launch-settings snapshot, and current model context.
Keep literal settings out of module-global cache keys and diagnostics. Use component-local draft generations and opaque server revisions.
A cache lookup for a profile must never fall back to `AvailableAgent.model_config` as authoritative profile data.
Agent-wide results can identify the agent, while the saved model label remains visible during profile discovery.

## Editor state

`useProfileFormCapabilities` passes the complete launch-settings draft to `useProfileModelCapabilities`.
Extend the profile form data boundary where environment fields currently live outside the narrowed model-selection type.
Thread profile identity from the route and distinguish a saved baseline from a new draft.

On opening a saved profile, request one baseline snapshot using saved settings.
Resolve dependent options only for the same launch context and selected model.
On a launch-setting edit, increment the local draft generation and mark results stale.
Do not spawn on each keystroke or automatically replay the previous model-option request against a partial draft.
The existing Refresh action explicitly probes the current draft, then resolves dependent options for that context.
A model change can resolve automatically after the launch context has a successful baseline.

Use these states: loading, ready, stale, and failed.
While stale or loading, retain the selected model label and draft values but disable unverified model/option choices.
Show localized text beside the existing Refresh control. Retry uses the current draft, not a captured older one.
Ignore late responses after navigation, profile switch, launch edit, or newer refresh.
Replace matching snapshots atomically; do not merge choices from two contexts.

Refresh alone never selects a model or rewrites the draft.
Preserve the existing reconciliation rules after an explicit model change with a complete matching snapshot.
Do not add a save deadlock for environment-only changes or a missing runtime.
Such edits can be saved under existing profile validation while discovery remains stale or failed.
Existing pending model-option reconciliation continues to gate a newly selected model's save.
Gateway profiles retain their current discovery bypass and provider-authentication behavior.
Workflow-only callers retain the existing resolver unless they explicitly carry a concrete profile context.

## Responsive behavior

Reuse the current profile route, selector, refresh button, and status panel.
The nearest shipped example is `mobile-agent-profile-config-selector.spec.ts` with `ModelConfigSelector`.
Keep the same model/option hierarchy and shared state on desktop and phone.
The added status wraps beneath the selector on phones and uses the page's existing scroll owner.
Touch actions keep at least 44px targets; fine-pointer controls retain their existing compact dimensions.
The existing picker owns its internal scrolling, dismissal, and focus return.
This change does not introduce a new overlay, navigation path, or fixed action region.

## Failure and observability

Validation failures identify fields without echoing values.
Secret, authorization, missing-executable, unsupported-context, provider-authentication, and timeout failures use sanitized statuses.
No failure retries through agent defaults, clears a saved selection, or activates another runtime.

The current ACP probe logs its complete command. Profile flags can contain credentials.
Remove that raw-argument logging from this path before forwarding profile settings.
Audit native and secondary probe logs too. Log provider identity, context revision, cache outcome, elapsed time, and stable failure class only.
Never log environment values, secret references with contents, raw arguments, or raw provider stderr.
Do not add profile IDs or context digests as metric labels.

## Persistence and compatibility

No database schema change and no persisted capability cache are required.
Draft discovery never writes profile data. Backend restart clears runtime caches.
Agent-wide startup probes and managed runtime candidate validation retain their trusted default context.
The live executor remains authoritative under the existing exact-model and fallback policy.
A host-only path such as `CODEX_PATH` can still be invalid on a remote executor.

## Related contracts and delivery

- [Model-aware resolution decision](../../../decisions/2026-08-07-model-aware-provider-capability-resolution.md)
- [Managed version selection](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
- [Explicit model strictness](../../../decisions/2026-09-15-explicit-profile-model-strictness.md)
- [Profile requirements](../requirements/profile-capability-discovery.md)
- [Implementation plan](../../../plans/profile-capability-discovery/plan.md)

A global `CODEX_PATH` workaround cannot describe two differently configured profiles.
Overwriting the agent-wide cache would leak one profile's catalog into another profile.
The scoped context extends existing discovery contracts; this design preserves the rationale without a separate ADR.
