---
status: current
system: agents
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
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
Raw stderr, npm log paths, cache paths, and registry URLs remain in agentctl
diagnostic logs and do not cross the probe API.

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

The startup generation rejects delayed events from the first child. The
existing cancellation and shutdown gates remain authoritative during recovery.

For a host capability probe, the equivalent flow is:

1. Backend startup creates one host utility lifecycle and the manager runs the exact managed runtime with `--prefer-offline`.
2. Agentctl reports the stable managed-runtime npm-resolution failure code.
3. The host utility manager preserves the execution tree and the original probe environment.
4. The host utility manager rebuilds the same effective package version with `--prefer-online` and retries once.
5. A successful retry becomes the live capability or model-configuration catalogue. Retry-preparation or final probe failure becomes the published failed status.

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
