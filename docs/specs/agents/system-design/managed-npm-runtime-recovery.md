---
status: draft
system: agents
updated: 2026-10-19
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-005
---

# Managed npm runtime recovery system design

## Purpose and boundaries

The lifecycle manager owns task-session recovery policy. The host utility
manager owns host capability-probe recovery policy. Each manager reconstructs
commands only from trusted managed-runtime metadata. The colocated `agentctl`
instance owns cache discovery and exact cache repair.

This split applies to host utility probes and to `standalone`, `docker`, and
`ssh` runtimes. The design does not add executor-specific shell commands to the
Kandev backend.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001` | [Recovery flow](#recovery-flow) |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002` | [Executor-local cache contract](#executor-local-cache-contract) |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004` | [Bounded setup retry](#bounded-setup-retry) |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-005` | [Missing bundled Codex dependency](#missing-bundled-codex-dependency) |
| `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003` | [Project-independent npm configuration](#project-independent-npm-configuration); [Release-date policy failure](#release-date-policy-failure) |

## Components and responsibilities

- `runtime/lifecycle.Manager` classifies bounded startup evidence and limits recovery to one retry.
- `agent/hostutility.Manager` classifies a structured probe failure and retries through its warm agentctl instance once without deleting shared files.
- The host utility instance retains its existing admission gates. Automatic probe recovery no longer deletes shared cache trees.
- `backendapp` starts the host utility manager once, after temporary-artifact ownership is available, and runs profile and utility reconciliation after that bootstrap.
- `runtime/agentctl.Client` calls the authenticated cache-repair endpoint on the session-scoped `agentctl` instance.
- `agentctl/server/api.Server` validates the request and coordinates the local repair operation.
- `common/npmresolution` owns the strict shared npm diagnostic matcher used by both runtime lifecycle and agentctl probe classification.
- `agentctl/server/utility.ACPInferenceExecutor` validates the exact managed package in the trusted command, classifies captured probe stderr, and returns only a stable failure code to the backend.
- `agentctl/server/process.Manager` runs `npm config get cache` with the configured agent environment.
- `agent/managedruntime` validates the package specification and removes one deterministic `_npx` tree.

## Project-independent npm configuration

Managed npm command construction adds the canonical marker
`--prefix ~/.kandev/managed-npm-runtime`. Before the command starts, the
execution host replaces that marker with a private, user-scoped directory
under its system temporary root and creates the directory. The path is
resolved where npm runs, so the same trusted command works on the backend
host, in Docker, and over SSH without embedding a backend-host path.

The project root must stay outside both the task workspace and the mounted
agent session home. Some container agents, including OpenCode, mount their
whole home from host-managed session state. Creating the project root under
that home can leave root-owned files in a host temporary directory. The
system temporary root avoids that mount while remaining local to the npm
execution host. A missing or unavailable prefix fails safely before npm
starts.
Apply the same prefix to exact-version cache preparation, capability probes,
one-shot prompts, normal task launches, and online-preferred retries. The
Kandev-owned prefix is independent of both the task workspace and an optional
`KANDEV_HOME_DIR` override for Kandev state and logs.

The child process still starts in the task workspace, and ACP receives that
workspace as its session cwd. Only npm's project configuration root changes.
User-level and global npm configuration, the selected registry, and explicit
environment overrides remain in force. The command's top-level package spec
and npm cache remain the same. Cache discovery for a failed launch must use the
same prefix and effective environment as that launch; otherwise a repository
`.npmrc` can direct repair to a different cache. A prefix preparation failure
stops startup with a sanitized runtime error before launching npm.

Keep exact-command recognition in lifecycle recovery, host utility recovery,
and agentctl probe classification aligned with the new argument shape. Their
trusted package and version checks continue to reject arbitrary commands.

## Probe failure contract

`ProbeResponse` can carry a stable managed-runtime npm-resolution failure code.
Agentctl sets it only when bounded stderr contains npm `ETARGET` and a missing
top-level package specification that exactly matches the trusted probe command.
The raw bounded stderr drives classification and does not cross the probe API.
Diagnostic logs retain a sanitized tail: likely credentials, private URL
details, local paths, and any truncated first line are removed according to
[Probe Failure Diagnostics](probe-failure-diagnostics.md).

The host utility manager accepts this code only for an agent that implements
`ManagedNPMRuntimeAgent`. It derives the package specification and
online-preferred replacement command from that agent's managed-runtime spec and
effective version. No response field supplies executable command data.

## Executor-local cache contract

The backend sends the trusted exact package specification to the authenticated
session-scoped `agentctl` API. The request does not contain a cache path,
registry URL, shell command, or package data from stderr.

The `agentctl` process resolves the cache root with its current agent
environment. This environment includes `NPM_CONFIG_CACHE`, `HOME`, npm
configuration, and profile values that also affect the failed child process.
Host utility repair requests carry the same runtime environment overrides and
strip list as the failed probe. Agentctl applies them while resolving npm's
cache, so the repair and retry target the same effective cache.

The repair operation uses `managedruntime.RemoveNpxExecutionTree`. This helper
derives the `_npx` key from the trusted package specification. Its descriptor
walk rejects symbolic links and path replacement races.

The endpoint requires the existing agentctl bearer token and instance identity.
It accepts one exact stable package specification. It returns no host path or
raw npm output.

## Recovery flow

1. Kandev starts the exact managed runtime with `--prefer-offline`.
2. The process exits before ACP initialization.
3. The lifecycle manager reads bounded sanitized stderr from `agentctl`.
4. Recovery uses the bounded setup classification below; exact-package `ETARGET` remains supported.
5. The lifecycle manager stops the failed child process.
6. Kandev preserves the shared execution tree and waits for the bounded retry delay.
7. For npm-classified failures, Kandev changes `--prefer-offline` to `--prefer-online`; unexplained early exit preserves the original preference.
8. Kandev starts the replacement child and initializes the original ACP session.

The startup generation rejects delayed callbacks from the first child. Retained
events replayed through the replacement stream also retain their originating
agentctl process generation. When it is older than the authenticated client’s
current process generation, lifecycle ignores the callback after ordinary durable
receipt and projection. Missing or malformed process evidence never proves that
an event is stale. Existing cancellation and shutdown gates remain authoritative.

For a host capability probe, the equivalent flow is:

1. Backend startup creates one host utility lifecycle and the manager runs the exact managed runtime with `--prefer-offline`.
2. Agentctl reports the stable managed-runtime npm-resolution failure code.
3. The host utility manager preserves the execution tree and the original probe environment.
4. The host utility manager rebuilds the same effective package version with `--prefer-online` and retries once.
5. A successful retry becomes the live capability or model-configuration catalogue. Retry-preparation or final probe failure becomes the published failed status.

The proposed missing-dependency extension below adds a narrow eligible
classification to task and probe recovery. Other host probe failures retain
the ETARGET-only rule.

Retain current host utility admission ordering while removing automatic cache deletion.
No backend-wide launch mutex or cache lease is introduced.
The probe retry does not run profile reconciliation between attempts. Persisted
profile model, fallback model, mode, enabled state, and active runtime version
remain unchanged on both success and failure.

## Failure behavior

If Kandev cannot stop and reap the failed process, it emits
`managed_runtime_startup` with `reason=cleanup_failed`. Replacement configure
or start failures keep their normal `agent_runtime` classification. An
exhausted exact-version resolution failure retains
`managed_runtime_npm_resolution`. Other exhausted eligible setup failures use
`managed_runtime_startup` with a bounded reason.

These errors contain bounded sanitized details. The UI keeps the existing
single **Retry runtime** action. Kandev does not change the active version.

Unsupported runtime types do not call the repair endpoint for ordinary npm
resolution failures. Kandev classifies an exact release-date policy failure
before checking repair support when the bounded diagnostic is available.
Native commands, passthrough commands, ineligible setup failures, and exhausted
retries remain on the normal terminal error path.

A host capability probe that cannot prepare or fails its online retry publishes
the final failure normally. The task retry also retains the final replacement
cause as bounded sanitized detail and keeps its attempt metadata. Authentication
refusal is classified before generic managed-runtime recovery so existing login
recovery remains available. Configure/start and session creation/restoration
failures retain their ordinary runtime-error path; they are not relabeled as
post-initialize failures or retried. Kandev does not hide a runtime that still
cannot start, change its version, or substitute a stale capability catalogue
from a different runtime generation.

## Release-date policy failure

The agentctl stderr projection recognizes npm's exact top-level `notarget`
line with the bounded `with a date before` suffix. It accepts the observed npm
11.16 date form (`M/D/YYYY, h:mm:ss AM/PM`) and retains RFC3339-shaped dates
for compatibility. Invalid calendar or time values are rejected. Agentctl
preserves a canonical date-qualified marker without copying the raw date or
any other stderr line.

For task startup, the lifecycle manager passes the trusted package spec with
the bounded diagnostic to `routingerr`. Classification checks the raw line
against that exact spec before generic diagnostic redaction can treat the
scoped package or locale-formatted date as a path or opaque token. The persisted
excerpt contains only a fixed policy message and the canonical date marker.
Ordinary exact-version `ETARGET` retains one online-preferred retry without deleting shared files. The
same distinction applies to host capability probes.

Settings update jobs classify bounded output against the trusted exact package
spec before invalidating the execution cache. Both legacy and exact-candidate
updates fail with a safe policy error when the diagnostic matches; they do not
retry or expose the raw locale-formatted date in the job error. Ordinary update
failures retain the existing cache-repair path.

The lifecycle manager publishes a stable policy failure code without retrying. The orchestrator persists one sanitized recovery entry with a localized
policy explanation and an ordinary retry action. The entry must not use the
existing stale-metadata card, whose copy says Kandev repaired the cache.
Office consumes the same code and explanation. Desktop and phone reuse the
current inline recovery presentation, collapsed details, transcript scroll
owner, and phone touch targets from the session recovery design. No new page,
dialog, or storage table is needed.

## Observability

Structured recovery logs include the recovery scope, agent ID, attempt, outcome,
and, for task sessions, the execution ID, runtime type, and startup generation.
They do not include cache paths, registry URLs, or raw stderr.

Existing failure metadata stores the stable failure code and sanitized details.
No database migration is necessary.

## Related decisions

- [Validate and persist managed runtime version selection](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
- [Run cache repair where npm runs](../../../decisions/2026-08-24-agentctl-local-managed-runtime-cache-repair.md)
- [Recover host capability probes before publishing failure](../../../decisions/2026-09-07-host-utility-managed-runtime-recovery.md)
- [Isolate managed npm runtime project configuration](../../../decisions/2026-09-24-isolate-managed-npm-project-config.md)
- [Recover missing bundled dependencies in a private runtime](../../../decisions/2026-10-19-isolated-managed-dependency-recovery.md)

## Bounded setup retry

This section implements REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004 and qualifies
the earlier cache-repair flow. See the [decision](../../../decisions/2026-10-02-bounded-managed-npm-startup-retry.md)
and [delivery package](../../../plans/managed-npm-startup-resilience/plan.md).
The extension targets task launches on standalone, local Docker, and SSH.
Host capability probes keep their strict ETARGET-only retry eligibility; their
retry also stops deleting the shared cache tree.

### Evidence and classification

Add a shared typed startup-evidence structure under `agentctl/types`.
Agentctl attaches it to `agent.initialize` error details. It contains the process
generation, exit disposition, optional exit code, canonical npm code, and
whether evidence collection completed. It also distinguishes whether npm code
diagnostics were present, whether their bounded collection was complete, and
whether a canonical npm code fell outside the recovery allowlist. Ring-buffer
eviction, oversized joined diagnostics, and a failed stderr drain make npm
diagnostics incomplete. No raw stderr, path, or URL is required.
Process generations advance on successful process start. Include the generation
in the start response, retain it in `runtime/agentctl.Client`, and send it with
initialize. Reject stale generations before touching an adapter. Legacy peers
without these fields retain existing strict ETARGET recovery; they do not gain
heuristic early-exit retries.

`process.Manager` records exit evidence for the exact child generation before
publishing its exit event. Capture intentional stop separately from process
status. Report signal termination separately from an ordinary exit. A missing
or unsettled exit observation is unknown, not proof of a dead child. Collection
may await the process-exit record for at most the existing two-second diagnostic
budget, outside manager locks, and must obey cancellation.

Wrap failures from `Client.Initialize` in a typed phase error in
`SessionManager.InitializeSessionWithSettingsPolicy`. Do not wrap errors from
`createOrLoadSession`, settings, session restoration, or prompting as pre-handshake
failures. The retry owner checks that phase before accepting evidence.

Extend `common/npmresolution` and the agentctl safe diagnostic projection with
a closed allowlist of npm-prefixed codes: ECONNRESET, ECONNREFUSED, ETIMEDOUT,
EAI_AGAIN, E502, E503, E504, EBUSY, ENOTEMPTY, and EINTEGRITY. Accept these only
for the trusted managed command and its matching process generation. Do not
classify arbitrary prose or another operation's stderr as npm setup evidence.
Known permanent conditions take priority over transient or early-exit fallback:
EACCES, EPERM, ENOSPC, EROFS, E401, E403, E404, EAUTH, ENEEDAUTH, EBADENGINE,
release-age policy, invalid command/configuration, and structured auth refusal.
Retain exact top-level-package matching for ETARGET; a transitive ETARGET must
not fall through to generic early-exit retry.

Without npm evidence, allow one retry only for an observed non-intentional
ordinary process exit during the typed initialize phase when stderr collection
is complete and no canonical npm code diagnostic was present. Unknown npm
codes receive a bounded marker and are not equivalent to empty stderr. An
oversized or otherwise incomplete diagnostic set fails closed, including when
it contains an allowlisted permanent code. A signal exit, a live process,
unknown ownership, malformed evidence, cancelled operation, or expired
deadline is ineligible. This path is `early_exit`, not a claimed npm diagnosis.

### Retry lifecycle

Reuse `beginStartupRecovery` and its existing one-replacement generation fence.
All categories, including ETARGET, consume that same budget. Never stack an
npm retry and a generic retry. Delay replacement by two seconds plus independent
jitter in the range zero to one second. Use a context-aware timer with injected
delay selection in tests. Healthy launches have no delay or global serialization.

Stop and reap the old process tree through agentctl. Unlike the current
`stopAndRepairManagedRuntime`, a stop failure cannot be logged and ignored.
Retain cancellation checks before cleanup, after delay, and immediately before
replacement. Bound recovery, including cleanup and replacement initialization,
to 90 seconds or the caller's earlier deadline. Cancellation follows the existing
owned teardown path rather than reporting a runtime failure.

Do not call `RepairManagedRuntimeCache` from automatic task or probe recovery.
ETARGET and classified transient npm failures retry the trusted exact command
with `--prefer-online`. Unexplained early exit retries the same command without
changing its offline/online preference. Preserve registry, environment, version,
workspace, executor, model, permissions, and session identity. Explicit Settings
maintenance retains the scoped repair endpoint and its existing behavior.

The startup owner retains error handling while retry is possible. Existing
uninitialized-event suppression and startup-generation fences must prevent the
first exit from publishing FAILED or releasing a waiting prompt. A successful
replacement continues the original initialization once. No recovery path may
resend a prompt after session/new, session/load, or prompt admission began.

### Presentation and diagnostics

Use existing boot progress for `Retrying agent startup (attempt 2 of 2)`.
No modal, new page, or recovery button appears while automatic recovery runs.
On exhaustion use the existing inline recovery card and one Retry runtime action.
Add the stable `managed_runtime_startup` kind through routing, orchestrator failure
persistence, and desktop/phone recovery renderers. Its reason distinguishes
`npm_transient` from `early_exit`; its attempt count distinguishes attempted retry
from blocked cleanup. Preserve exact ETARGET and policy failure kinds.
Do not change `routingerr` policy to retry post-prompt failures or enable provider
fallback merely because this new startup kind exists.

Copy must reflect the actual final failure and whether a retry occurred.
An unexplained exit must not say npm failed. A cleanup failure must not say two
attempts ran. Keep technical details sanitized and collapsed. Translate all new
copy in seven locales and generate Traditional Chinese with the existing script.
Desktop and phone retain the existing recovery card and chat scroll owner.

Structured logs record attempt, generation, code, exit disposition, backoff, and
outcome. No new metric or storage table is needed. Tests cover mixed concurrent
outcomes, delayed events, cancellation, cleanup failure, missing evidence, and
single prompt delivery after recovery. Real subprocess fixtures provide exit
and stderr evidence; API-only seeded cards are not startup-recovery proof.

## Missing bundled Codex dependency

This proposed extension implements REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-005.
The [delivery package](../../../plans/managed-codex-dependency-recovery/plan.md)
owns implementation status. It does not relax the no-shared-tree-mutation rule.

### Eligibility and evidence

The initial provider allowlist is the registered
`@agentclientprotocol/codex-acp` managed ACP command with no command prefix
and no non-empty `CODEX_PATH`. Preserve its selected exact version and CLI
arguments. Native Codex app-server, passthrough, operator-defined commands, and
other providers stay outside this extension.

Add a bounded Codex dependency diagnostic recognizer beside the existing
`common/npmresolution` matchers. Recognize the observed missing optional
platform-package diagnostic inside the failed initialize RPC error as well as
stderr. A Codex bridge can return JSON-RPC error 1001 after its provider child
exits while the bridge itself remains alive. Consequently, an ordinary exit of
the top-level npx/bridge process is not a prerequisite and must not be forged
in `ManagedStartupEvidence`.

Agentctl validates the failed command's trusted exact package spec and inspects
that execution tree using the failed subprocess environment. Resolve npm's
cache with the isolated project prefix, then derive the `_npx` key through
`managedruntime.NpxExecutionCacheKey`. Reuse the bounded manifest-inspection
approach in `utility/runtime_observation.go`; share its narrow filesystem
primitives where useful rather than duplicating traversal.

Read only bounded package manifests beneath the exact tree. Verify the bridge
name/version, its dependency on `@openai/codex`, and the installed provider's
declared platform dependency. Map the host OS/architecture to the known Codex
platform package, rather than accepting a package, executable, or path from
the error. Confirm that the expected package or native executable is absent.
Unreadable, inconsistent, malformed, escaped, or symlinked inventory is not
evidence of absence and fails closed. Do not execute a manifest-provided script
to establish eligibility.

Retain instance/process-generation evidence for task initialization and a
probe-attempt identity for one-shot discovery. Bind the verified result to
the exact package and effective environment, with no secret values or ordinary
credential hashes crossing APIs or entering logs. Older peers or ambiguous
evidence do not authorize repair. Permanent failures and cancellation continue
to take priority.

### Private preparation primitive

Add a narrow executor-local prepared-runtime owner under `agent/managedruntime`,
shared by agentctl task and utility subprocess paths. It owns a private project
directory and a private npm cache; neither is inside the failed shared tree,
task checkout, mounted provider home, or another operation's directory.

Preparation uses the trusted exact bridge package and normal npm installation
with an explicit isolated `--prefix`, isolated `--cache`, `--prefer-online`,
and `--include=optional`. It must not run `npm install -g`, mutate the shared
tree in place, copy broken `node_modules`, or install the diagnostic's package
name directly. The inclusion override is confined to this operation. Registry,
authentication, environment sanitization, release-age policy, and strip rules
remain those of the failed launch.

This is a fresh installation of the selected bridge, not a lockfile recovery
or a promise to preserve an independently unselected transitive CLI version.
Use the package's normal dependency constraints, validate the actual installed
bridge version and host platform executable, and refresh runtime observations.
Never represent a changed bundled provider version as the previous observation.

Build the replacement command from the original trusted managed command,
applying the prepared project's private prefix/cache internally at agentctl's
last command-preparation boundary. Both npm preparation and replacement remain
argument-vector operations, not shell strings. Keep task working directory,
ACP cwd, environment, model, flags, permissions, and session ID unchanged.
Bound command output, sanitize failures before persistence, and confirm that the
required bundled executable's version check succeeds before replacement.

The prepared-runtime owner retains its directory while any child can read it.
Probe cleanup releases its private tree only after reaping all probe descendants.
Task cleanup releases it only after reaping the task process tree; backend
connection closure or detach is not release. Tie ownership to the surviving
agentctl instance, not to the HTTP start request or backend context. Reuse
existing instance cleanup and owned-directory safeguards. Record owner identity
in the private root and remove only that exact verified root; no broad temporary
directory cleanup or new install-wide janitor is part of this change. Explicit
crash-orphan reclamation remains later storage maintenance.

### Wire and task retry

Extend the existing `agent.initialize` error details with an optional typed
missing-dependency classification, separate from top-level process-exit evidence.
It is emitted only after inventory verification and carries stable reason,
trusted package identity, and generation, never paths or raw diagnostics.
Advertise support through the existing agentctl capability negotiation.

Extend the existing authenticated start request with an optional typed
dependency-recovery directive containing the expected failed generation and
exact package spec. The backend sends it only through the runtime agentctl
client after the existing startup owner consumes its retry budget and confirms
cleanup. Agentctl revalidates the directive against its instance configuration
and captured failure; it never accepts a cache path or arbitrary command.
Existing empty start requests remain unchanged. A peer lacking the advertised
capability receives no directive and fails normally.

`lifecycle.Manager.prepareManagedRuntimeStartupRetry` handles this typed
initialize-phase failure independently of empty-stderr and npm-code branches.
Keep `beginStartupRecovery`, cleanup confirmation, deadline, delay, generation
fences, and cancellation ordering. Preparation runs inside the same recovery
context as replacement, with no third launch after a prior ETARGET retry.
After preparation, perform the normal initialize followed by the original
create/load/resume exactly once. No automatic repair is eligible after the
initial handshake has succeeded.

Use existing `managed_runtime_startup` failure routing with bounded dependency
recovery reasons and accurate attempts. Preparation failure must retain its
sanitized cause even when no second agent process started. Delayed events from
the first generation remain stale. Stop must also cancel an in-flight installer
and reap its descendants before private-tree release.

### Host and profile discovery

Add `managed_runtime_dependency_missing` to `utility.ProbeFailureCode` and
an optional typed recovery directive to `ProbeRequest`. The stable code comes
from the same inventory check, including initialize RPC failures; it is not
produced merely by matching generic stderr. Recheck inventory before preparing
the retry. A new request cannot repair an arbitrary package.

`hostutility.Manager.probeManagedRuntime` retains the first failed response
until one eligible retry completes. Reuse the existing instance admission and
clone the original request, preserving environment, strip policy, arguments,
runtime observations, and model/mode selections. ETARGET and missing-dependency
recovery share one probe budget. There is no recursive retry inside agentctl.
Publish only the successful replacement or the actual final failure.

Concrete profile discovery currently bypasses that retry owner and deliberately
never installs packages. Add only this narrow private-preparation exception in
both `probeProfileCapabilitiesFlight` and `resolveProfileModelConfigFlight`,
reusing a shared bounded probe helper. Preserve all profile single-flight,
secret/context hashing, runtime-generation, and late-publication checks.
Repair may not switch to the default context, save a draft, or populate the
agent-wide cache. Cancel obsolete repair work when its profile context changes;
the existing `context.WithoutCancel` lifetime is not permission to launch a
stale replacement. The
[profile discovery design](profile-capability-discovery.md) retains the
surrounding context and publication contract.

### Presentation, compatibility, and proof

Reuse existing boot retry progress and generic startup recovery copy, which
already supports sanitized reason/details and a single Retry runtime action.
Profile and agent-wide discovery retain their loading and error states. No new
UI control, layout, translation key, or recovery modal is required by this
package. Phone and desktop keep their existing compositions and touch targets.
Validate that dependency preparation failures do not claim two agent attempts
or successful shared-cache repair.

Structured logs add a bounded `missing_bundled_dependency` reason and
preparation outcome; identity belongs in logs, not metric labels. Logs and UI
details exclude npm cache paths, private registry URLs, credentials, and raw
RPC diagnostics. No database migration, runtime selection write, persistent
repaired-generation catalogue, or feature flag is introduced.

Deterministic tests use local package/CLI fixtures and no public registry or
provider credentials. Cover a bridge that stays alive after returning the
observed initialize error, a genuinely absent platform executable, mixed
healthy/recovered/exhausted siblings, unsupported shapes, hostile paths,
installer failure, cancellation, deadline, old peers, and single prompt
delivery. Browser proof must trigger the real task resume/profile refresh
pipeline, not seed a recovery card. Existing Docker and SSH fixtures prove
preparation occurs on the executor rather than the backend.
