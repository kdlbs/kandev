---
status: current
system: agents
created: 2026-10-16
requirements:
  - REQ-AGENTS-WRAPPERS-001
  - REQ-AGENTS-WRAPPERS-002
  - REQ-AGENTS-WRAPPERS-003
  - REQ-AGENTS-WRAPPERS-004
  - REQ-AGENTS-WRAPPERS-005
---

# Generic launch wrapper system design

## Purpose and ownership

Agents owns profile configuration and launch semantics across lifecycle, host utility, and agentctl.
Executors supply the execution environment. Plugins own extension negotiation, not the core launch decision.
This design extends [profile discovery](profile-capability-discovery.md) without changing its cache or UI ownership.

The selected design retains `command_prefix` as the public setting.
It separates the wrapper from the trusted child command until preparation on the execution host.
It requires no provider registry entry and no plugin.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-WRAPPERS-001 | Authority and admission, Configuration, Deployment guard |
| REQ-AGENTS-WRAPPERS-002 | Runtime references, Preparation, Environment |
| REQ-AGENTS-WRAPPERS-003 | Launch coverage, Transport and recovery |
| REQ-AGENTS-WRAPPERS-004 | Transport and recovery, Failure and cleanup, Compatibility |
| REQ-AGENTS-WRAPPERS-005 | Optional sandbox capability |

## Source evidence and authority

The audit baseline is PR head `e45477c896ae9326324b035d58bddf1e15b1eb5f`.
The July [launcher authority ADR](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md) predates current authentication.
Its control-plane distinction remains applicable. Its historical unauthenticated-route description is not the present implementation.

`settings/handlers/handlers.go` requires `authz.ScopeOrgConfigManage` for profile mutations and profile probes.
`requireProfileDiscoveryScope` also checks identity and that scope for model resolution with a profile or draft.
`auth/httpmw.Middleware` authenticates enabled deployments. Disabled mode injects `SyntheticIdentity`.
`authz.SubjectFromGin` treats that synthetic identity as unscoped.
These facts support customization in both modes, not a new confinement claim in disabled mode.
Full-detail read authorization and untrusted browser origin isolation remain separate authentication work.

Retain these route checks and strict draft decoding.
Authorize saved-profile visibility and secret references before creating the internal launch request.
New drafts use the same configuration scope. Reject unknown fields, incomplete snapshots, foreign profiles, and mismatched agent identities.
A task can select an eligible saved profile under existing task access rules. It cannot supply replacement wrapper argv.
Office JWTs and ordinary agent credentials do not become configuration credentials.

The backend derives wrapper provenance after authorization.
The public profile API does not accept `operator_defined`, trusted-command flags, working directories, or wrapper authority assertions.
Internal agentctl requests use the existing backend control credential and owner checks.
A provenance field classifies an authorized request. It does not authenticate a caller by itself.
Require authenticated control access for the new wrapper transport, including host utility instances.
An agentctl running with empty-token authentication cannot accept that transport.

Keep `resolveSpawnCommand` and the built-in child-command allowlist.
`OperatorDefined` remains specific to registered custom agent definitions.
Do not set it merely because a prefix exists.
Generic wrapper admission uses the separate authorized prefix contract.
Legacy unclassified utility prefixes retain the existing allowlist until their backend caller supplies the new contract.

## Configuration

Persist the existing profile `command_prefix` string. No new profile column or executor setting is needed.
Use `cliflags.ValidateCommandPrefix` and `Tokenise` once at the profile boundary.
Carry ordered tokens thereafter. Do not concatenate and reparse display text.

The first token is a literal executable. It cannot be a runtime reference, an empty string, a flag, or contain NUL.
On the execution host, resolve an absolute path, a relative path against the actual working directory, or a name through the effective PATH.
Do not search the backend host for a remote wrapper.
Name lookup excludes implicit current-directory and empty PATH entries. Explicit relative paths remain operator choices.
Resolve the inner executable using its existing trusted command admission and the same execution environment.
An authorized custom wrapper can itself be an interpreter. Kandev never adds a shell or evaluates arguments implicitly.

CLI flags belong to the child command. Wrapper arguments precede that complete command exactly once.
User configuration supplies provider-specific separators such as `--`.
Kandev does not infer separators from executable names.

## Runtime references

Add two reserved, whole-argument references to `command_prefix`:

| Token | Value |
| --- | --- |
| `{{kandev.work_dir}}` | Absolute directory used as the child process working directory |
| `{{kandev.managed_npm_dir}}` | Private managed npm project directory prepared on this execution host |

A reference is one complete token after tokenization, including when quoted.
A token containing a reserved reference plus other text is invalid. There is no substring expansion.
Unknown `{{kandev.*}}` references fail validation. References are forbidden in the executable position.
Triple braces escape a reserved token: `{{{kandev.work_dir}}}` produces the literal `{{kandev.work_dir}}`.
Validate escapes before references and expand once only.
Ordinary shell expressions, environment references, tildes, and command substitutions remain literal arguments.
The existing trusted npm command marker retains its separate internal semantics.

Example configuration shape:

```text
wrapper --directory '{{kandev.work_dir}}' --runtime '{{kandev.managed_npm_dir}}' --
```

These options are illustrative. Each wrapper defines its own policy language.
For Greywall, the historical reproduction uses explicit `--allow-path` options.
The new references supply their values without choosing or broadening those grants.
No fixed test path becomes a default.

The npm reference prepares the existing private project directory even for a native child command.
This permits one profile to cover its managed ACP adapter and native passthrough executable.
It does not install packages or grant visibility to that directory.
Without a trusted npm marker or this reference, preparation does not create the npm directory.
Do not expose credentials, token paths, home-directory contents, environment dictionaries, or the entire temporary root through references.

## Preparation

Introduce `internal/common/launchwrapper` as a narrow shared package.
Its `Spec` contains version 1, source `profile`, and prefix tokens.
Its `Context` contains the work directory and final child environment.
Its `Prepare` returns complete argv or a sanitized failure classification. The package does not load profiles, authorize users, or own processes.

Preparation runs immediately before each actual spawn:

1. Authorize the request and validate all tokens before filesystem changes.
2. Build and admit the complete inner argv, including provider flags and adapter arguments.
3. Resolve the final execution-host working directory and environment.
4. Prepare trusted managed npm paths, then resolve wrapper references.
5. Resolve executables and compose any deployment guard, profile wrapper, and child command in that order.

Keep managed npm project preparation in `internal/common/managednpm`.
The agent-owned `managedruntime` entry points delegate to this neutral package.
Both backend and standalone agentctl use the common wrapper package without an agent-owned dependency.
Windows executable resolution uses PATHEXT for extensionless names and never selects the adjacent POSIX npm shim.
Passthrough builders return the command and its wrapper snapshot together.
Request construction must not read a mutable wrapper from the execution record.
Retain its private permissions and symlink rejection.
Apply the trusted npm marker only to the inner command. Never scan arbitrary wrapper arguments for that marker.
The returned runtime directory and the inner npm `--prefix` must be identical.
Do not mutate stored templates when replacing markers or references.

## Environment

Reuse profile environment and secret resolvers. Do not copy the utility path's current omission of secret-backed entries.
Both `hostutility.ExecuteProfilePrompt` and `lifecycle.ExecuteInferenceProfilePrompt` use the scoped profile resolver, including secret-backed entries.

Precedence is inherited execution environment, managed defaults, then non-empty profile values or resolved secrets.
Preserve existing adapter and provider-gateway additions. Apply `StripEnv` last, including to explicit profile values.
Reserved backend control credentials and operator session credentials never enter the child environment.
The wrapper receives the same final environment as the child.
No separate post-confinement environment channel is introduced.
This means profile environment customization, including PATH and loader variables, is not a confinement boundary.

## Deployment guard

The utility resolver currently recognizes `/usr/local/bin/kandev-agent-guard` at its exact fixed image path.
Retain that identity rule for legacy deployment-guard requests.
Do not grant guard classification to another path with the same basename.
An authorized profile executable does not acquire deployment privileges through its name.

There is no new deployment-guard installation or policy setting in this package.
Where the deployment supplies an enforced outer guard, preparation must retain it outside the profile wrapper.
A profile cannot disable or replace it. If the transport cannot preserve that distinction, refuse the wrapped launch.
An operator-configured prefix that happens to name a guard remains profile customization, not proof of deployment enforcement.

## Transport and recovery

Add an optional versioned `launch_wrapper` object to configure, interactive-start, and inference DTOs.
It contains `version: 1`, `source: "profile"`, and `argv: string[]`.
Require non-empty valid argv when the object is present. Reject null, unknown versions, and conflicting legacy prefix plus new wrapper input.
The inner `agent_args`, `continue_args`, or inference `Command` remains unwrapped.
For inference, keep `CommandPrefix` only for old senders. New senders use `launch_wrapper` exclusively.

Advertise `launch-wrapper.v1` through agentctl's existing `/identity` capability list.
Expose that same build capability list at authenticated instance `/identity`.
The backend checks the instance endpoint through its bound client before dispatch.
This keeps the capability check on the same runtime binding as the launch request.
Check capability before configure and utility dispatch.
Local passthrough uses the compiled `InteractiveRunner` directly and needs no peer capability request.
SSH passthrough has a separate remote start transport without wrapper support.
Reject a wrapped request before that transport sends a start command or records a start operation.
Future support must carry the template and negotiate the capability on that transport.
Missing capability causes an explicit unsupported result for any new wrapped launch, including literal-only prefixes.
A new backend never sends an unwrapped command to an old receiver that could ignore the wrapper field.
Unwrapped profiles retain their existing compatibility path.

Store the template beside the existing protected agentctl process configuration.
Store `launch_wrapper_required` with it so a missing template cannot become an unwrapped launch.
Authenticated `GET /api/v1/agent/launch-wrapper` returns only the unresolved template and snapshot version.
Recovery reads this template from the retained instance. Child commands still come from the registry.
This is a narrow extension to the executor survival reconstruction table.
An unreadable or incomplete new snapshot refuses adoption under the existing recovery rules.
Copy it through configure replacement, one-shot configuration, restart, reset, and durable recovery serialization.
Do not place resolved secrets or filesystem paths in public task metadata.
The configuration stores unresolved references. Each new process resolves them on its own execution host.
Keep the existing rollback rule when replacement configuration fails.

Adoption of an already-running process does not reapply a wrapper or restart it.
For new-format state, require the template and supported capability before replacement.
Legacy retained configurations can continue using their already-composed literal argv when that complete configuration is authoritative.
Do not infer or remove a legacy prefix by basename or reread mutable profile settings to reconstruct missing state.
Ambiguous new-format state blocks replacement and exposes recovery-required status.
Explicit restarts that currently resolve current profile settings keep that behavior, but capture one complete authorized launch context per restart.

## Launch coverage

| Path | First increment | Integration boundary and proof |
| --- | --- | --- |
| ACP discovery and model options | Supported | `ACPInferenceExecutor.Probe`, host profile context, same wrapper and final environment |
| OpenCode secondary model listing | Supported when its existing command is compatible | `probeOpenCodeModels` uses the shared preparation. Failure can retain the same-context ACP snapshot, never an unwrapped secondary result |
| Host and executor ACP utility prompt | Supported | `ExecuteProfilePrompt` callers and `ACPInferenceExecutor.Execute`, including secret resolution |
| ACP initial and one-shot continuation | Supported | `CommandBuilder`, configure DTO, `buildFinalCommand`, adapter one-shot initial/continue argv |
| Restart, context reset, managed npm retry, replacement | Supported | Preserve templates through `manager_interaction.go`, process config, and recovery. A retry preserves the wrapper |
| Pi and other existing CLI passthrough providers | Supported when their native command accepts PTY wrapping | Initial, fresh, and resume builders carry the template to `InteractiveStartRequest` |
| Native non-ACP utility executors | Explicit refusal for wrapper contexts | Codex app-server and Claude execution must reject before process start. No ignored prefix |
| Custom registered ACP agents | Conditional support | Preserve existing operator-defined inner command admission. Wrapper uses the same generic contract |
| Docker, SSH, Kubernetes, other remote ACP executors | Conditional support | Requires `launch-wrapper.v1`, installed wrapper, and valid policy on that executor. Host success is insufficient |
| SSH native terminal passthrough | Explicit refusal for wrapper contexts | Its separate start transport cannot carry the wrapper. Reject before remote startup |
| Agent-wide discovery and runtime candidate validation | Existing default context | Do not import another profile's wrapper |
| Install/login, user shell, repository setup, MCP server launch | Excluded | No claim that these processes inherit agent wrapping |

Do not add support for a new native protocol to satisfy this increment.
Record unsupported behavior explicitly in tests and public documentation.
Provider support is conditional on the actual wrapper's platform and transport behavior.

## Failure and cleanup

Use stable internal failure classes: invalid configuration, unavailable wrapper, unavailable runtime context, unsupported context, and wrapped-process failure.
Map these to existing public error/status shapes, including `unsupported_profile_context` for unsupported probes.
Use generic messages that name the stage. Never echo argv, environment values, secret material, or raw stderr.
Audit `ACPInferenceExecutor.Execute`, `buildFinalCommand`, and interactive log-command handling as well as probe errors.
Wrapper-supplied text is untrusted even when executable selection was authorized.

Retain the current ACP process-group cleanup and bounded descendant termination.
Retain PTY ownership, resize, input, and cancellation through the interactive runner.
A wrapper that daemonizes outside these supported ownership semantics is unsupported.
Test wrappers that retain pipes and child processes on success, failure, timeout, and cancellation.

Temporary probe directories and passthrough MCP artifacts keep their current operation owners.
The shared managed npm directory is reusable state and must not be removed by per-launch cleanup.
Generic wrapping generates no provider policy file and starts no supervisor.
Policy denials never trigger allow-all policy, a whole-temporary-root grant, or an unwrapped retry.
Existing managed npm repair remains a distinct operation and never strips the wrapper from the subsequent agent launch.

## Compatibility and discovery state

Existing literal strings retain tokenization and ordering except for the newly reserved reference namespace.
Document the triple-brace escape for literal values in that namespace.
No profile migration, executor override hierarchy, or task policy override is introduced.
Command previews remain non-executing and show unresolved references. They must not create runtime directories.

Retain profile discovery HMAC scope, secret refresh, runtime generation, and stale-result checks.
Include the wrapper grammar version and template in the context identity.
Do not include disposable probe-directory names in cache keys.
Resolve runtime paths only on a cache miss that executes a process.
The existing explicit Refresh invalidates results after an externally changed wrapper installation or policy.
Automatic external-file watching is excluded.

The existing profile editor consumes unchanged response shapes on desktop and phone.
No rendered control, layout, or interaction change is planned.
Existing loading, error, retry, selection preservation, and authentication recovery behavior require regression coverage.

## Optional sandbox capability

Decision: defer a plugin capability from this increment.
Core plus explicit operator arguments solves admission and runtime-path references.
A plugin requirement for basic wrapping would add an availability dependency without resolving command consistency.
The current `ExecutorProviderPlugin` provisions remote environments. It is not a local wrapper hook and must not be repurposed.

A future sandbox capability could supply setup validation, version detection, typed policy configuration, denial diagnostics, or supervisor integration.
That work must define its own versioned capability through existing plugin manifest and protobuf negotiation.
It must specify execution-host placement, bounded calls, artifact ownership, cleanup, and removal while a session runs.
Core must retain authorization, trusted child argv, process ownership, cancellation, and session identity.
Plugins must never receive operator credentials or acquire authority through a UI action.
If an operator explicitly selects plugin-dependent enforcement, provider absence must fail that launch instead of reverting to generic wrapping.

Approval transport and policy learning remain deferred until real runtime evidence establishes a usable protocol.
No proposed method name, SDK interface, permission, or manifest entry is shipped or reserved here.
Generic wrapping remains usable with all plugins disabled.
This boundary and its alternatives are sufficiently captured here. No new ADR is required for the deferred extension.

## Delivery and evidence

- [Requirements](../requirements/generic-launch-wrappers.md)
- [Plan and work orders](../../../plans/generic-launch-wrappers/plan.md)
- [Historical Pi/Greywall reproduction](../../../plans/generic-launch-wrappers/reproduction.md)

The historical overlay proves neither this design nor confinement against bypass.
Implementation requires unrelated wrapper fixtures and a fresh Pi/Greywall run without the overlay.
Preserve the existing manual instance. Run new acceptance work in a separate disposable environment.
