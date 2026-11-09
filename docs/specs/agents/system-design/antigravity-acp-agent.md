---
status: draft
system: agents
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-001
  - REQ-AGENTS-ANTIGRAVITY-ACP-002
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-004
  - REQ-AGENTS-ANTIGRAVITY-ACP-005
  - REQ-AGENTS-ANTIGRAVITY-ACP-006
  - REQ-AGENTS-ANTIGRAVITY-ACP-007
  - REQ-AGENTS-ANTIGRAVITY-ACP-008
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-011
  - REQ-AGENTS-ANTIGRAVITY-ACP-012
  - REQ-AGENTS-ANTIGRAVITY-ACP-013
  - REQ-AGENTS-ANTIGRAVITY-ACP-014
  - REQ-AGENTS-ANTIGRAVITY-ACP-015
---

# Google Antigravity ACP agent system design

## Purpose and boundaries

Agents owns Antigravity identity, runtime selection, readiness, local authentication, compatibility, and setup UI.
The vendor server owns Google tokens, refresh, account entitlement, the loopback listener, and conversation storage.
Tasks, costs, Office, and platform retain their existing lifecycle, accounting, routing, and process ownership.
This design supplies normalized observations to those systems. It creates no parallel routing or accounting service.

The supported new path is one personal Google account on the local Kandev machine.
Task sessions use separate server processes and one shared native store.
No per-task home, account registry, hosted callback, or Google quota client is introduced.
Existing remote-auth declarations remain compatible metadata. They do not establish remote subscription support.

The [requirements](../requirements/antigravity-acp-agent.md) explicitly retire conflicting September draft restrictions.
The [plan](../../../plans/antigravity-local-subscription/plan.md) contains implementation and qualification gates.
Implementation is present for Tasks 01-06. Both documents remain draft until live qualification evidence is recorded.

## Requirement mapping

| Requirements | Design sections |
| --- | --- |
| REQ-AGENTS-ANTIGRAVITY-ACP-001, REQ-AGENTS-ANTIGRAVITY-ACP-002 | Existing identity; local runtime resolution |
| REQ-AGENTS-ANTIGRAVITY-ACP-003, REQ-AGENTS-ANTIGRAVITY-ACP-004 | Launch policy; control flow; persistence |
| REQ-AGENTS-ANTIGRAVITY-ACP-005, REQ-AGENTS-ANTIGRAVITY-ACP-006 | Authentication; shared ACP compatibility |
| REQ-AGENTS-ANTIGRAVITY-ACP-007, REQ-AGENTS-ANTIGRAVITY-ACP-008 | Installation; concurrency and cleanup |
| REQ-AGENTS-ANTIGRAVITY-ACP-009 | Local runtime resolution; installation; qualification |
| REQ-AGENTS-ANTIGRAVITY-ACP-010 | Launch policy; authentication; security |
| REQ-AGENTS-ANTIGRAVITY-ACP-011 | Shared ACP compatibility; control flow |
| REQ-AGENTS-ANTIGRAVITY-ACP-012 | Native human questions |
| REQ-AGENTS-ANTIGRAVITY-ACP-013 | Failure and recovery |
| REQ-AGENTS-ANTIGRAVITY-ACP-014 | Usage and billing |
| REQ-AGENTS-ANTIGRAVITY-ACP-015 | Setup UI |

## Source evidence and limits

Investigation date: 2026-11-02. Kandev source baseline: `f648e79f9ecc7d5db0db301f20d0b23c7cb44d97`.
T3 Code was reviewed at commit `3ddd3989e03e44c0106c6e3e44033a2f5b8b50ae`.
Useful primary sources:

- [Official ACP registry entry](https://github.com/agentclientprotocol/registry/blob/main/antigravity-acp/agent.json).
- [T3 installation](https://github.com/pingdotgg/t3code/blob/3ddd3989e03e44c0106c6e3e44033a2f5b8b50ae/apps/server/src/provider/AntigravityInstallation.ts).
- [T3 authentication](https://github.com/pingdotgg/t3code/blob/3ddd3989e03e44c0106c6e3e44033a2f5b8b50ae/apps/server/src/provider/antigravityAuthSupport.ts).
- [T3 ACP policy](https://github.com/pingdotgg/t3code/blob/3ddd3989e03e44c0106c6e3e44033a2f5b8b50ae/apps/server/src/provider/acp/AntigravityAcpSupport.ts).
- [T3 turn outcomes](https://github.com/pingdotgg/t3code/blob/3ddd3989e03e44c0106c6e3e44033a2f5b8b50ae/apps/server/src/provider/acp/AntigravityTurnOutcome.ts).

| Evidence | Observation | Confidence and limit |
| --- | --- | --- |
| Installed `agy` 1.4.12 | Google login works for its model catalog; `--acp` exits with unsupported flag. | Direct CLI execution. Does not authenticate standalone ACP. |
| Official ACP 1.4.5, Linux x64 | Archive contains server plus harness. Digest verified before extraction. | Archive and packaged source inspected. Other platforms were not executed. |
| Unmodified native launch | SIGILL before initialize on this Linux machine. | Direct execution and stack/disassembly evidence. Vendor fix not established. |
| CPU 0 diagnostic launch | Initialize succeeds; unauthenticated session creation returns a typed auth error. | Controlled workaround only. No production affinity policy follows. |
| Explicit `oauth-personal` | Native server prints a Google consent URL on stderr with PKCE and an ephemeral localhost callback. | Attempt cancelled without completing consent. Successful subscription inference is not yet verified. |
| Vendor 1.4.5 code | Prompt results can contain typed token usage; `/usage` fetches allowance and renders Markdown. | Source evidence, not a captured successful authenticated turn. No provider dollar cost observed. |
| Vendor model policy | `zed` is among clients eligible for third-party model discovery. | Source evidence. Account entitlement still determines the available catalog. |
| T3 turn handling | Native error steps can reveal failures hidden behind chat text plus `end_turn`. | Prior-art implementation. Kandev must qualify the exact journal schema with its chosen runtime. |

The Linux x64 archive was `agy-acp-server-1.4.5-linux-x86_64.zip`, with 321783181 compressed bytes.
Its SHA-256 was `923923a0d7dc8073f66cc6f78647a3107e3a7ee2b7e1a24df8b3927b5188013b`.
This is research evidence, not permission to download a mutable registry target at execution time.
Private `acp-debug/` captures contain consent state and diagnostic data. They must not become committed fixtures.

## Components and responsibilities

| Boundary | Existing location | Planned responsibility |
| --- | --- | --- |
| Agent declaration | `internal/agent/agents/antigravity_acp.go` | Preserve identity, base arguments, skills, resume, and remote metadata. Declare the subscription default. |
| Native bundle helper | `internal/agent/antigravityruntime/` | Trusted artifact records, bounded installation, filesystem validation, and local resolution. No generic native package manager. |
| Install orchestration | `internal/agent/settings/controller/agent_install.go`, `install_job.go` | Reuse job status, output, cancellation, maintenance interlock, and operator authorization. Accept a typed Go action for this local bundle. |
| Launch selection | `internal/agent/runtime/lifecycle/command.go`, `internal/agent/hostutility/manager.go` | Resolve the same absolute bundle executable on local paths. Preserve remote command behavior. |
| Wire compatibility | `internal/agentctl/acpcompat/`, `server/adapter/transport/acp/dialect.go` | Pure Antigravity client-info, auth-method, URL, question, and error translation helpers. |
| ACP/process ownership | `internal/agentctl/server/adapter/transport/acp/`, `internal/agentctl/server/process/` | Execute RPCs, observe stderr safely, maintain auth state, and manage process generations. |
| Sessionless setup | `internal/agent/hostutility/`, `agent/capabilities/handlers/` | Own one bounded local sign-in attempt using existing authenticated agentctl instances and client methods. |
| Product UI | Settings card and task auth guidance | Install/sign-in/retry state, phone presentation, and capability refresh. |
| Errors and usage | Existing routing errors, task events, and usage contracts | Consume provider observations without adding a new policy engine. |

Paths in this table are relative to `apps/backend/` except Product UI.
Keep provider-specific pure translation in the ACP compatibility layer, following ADR 0044.
Stateful process and database operations stay outside dialect functions.

## Existing identity

Keep `antigravity-acp`, display order 22, logos, non-virtual ACP, and existing profiles.
Base constructors remain pure. The local resolver replaces only the executable and supplies the selected helper path before shared profile wrapping.
MCP configuration stays in ACP session creation. No project-local MCP file or assumed transport override is added.
Native load/resume decisions remain capability-driven.

## Local runtime resolution

Use one provider-specific resolver from discovery, local task launch, recovery, probes, inference, and sign-in.
Inputs include platform, locality, effective environment, and the existing Kandev user cache location.
The result includes server path, helper path, provenance, exact managed version when known, and a bounded failure reason.
Do not infer installation from the presence of `agy`.

Resolution order:

1. Select the compiled, reviewed managed bundle when its versioned destination exists.
2. Validate its complete manifest and both regular executable files. An invalid destination is an error, not a PATH fallback.
3. If no managed destination exists, resolve the manual server through PATH and validate its sibling harness.
4. For a manual override, resolve and validate the actual `ANTIGRAVITY_HARNESS_PATH` instead of accepting any non-empty value.
5. Return unavailable when no complete runtime exists. Return context errors unchanged.

Managed bundles use a provider subdirectory under the existing Kandev user cache, keyed by version, platform, and archive digest.
Use absolute paths. Completed directories are immutable; launch never deletes or rewrites them.
An external PATH installation remains externally owned. Installation never replaces it or rewrites PATH.
No version selector, native auto-updater, SQL migration, or npm package identity is needed for this pinned first release.

Filesystem discovery performs no subprocess calls.
The existing capability probe separately establishes readiness and captures startup failures.
A file-complete install can therefore be installed but not ready.
A remote executor never receives a backend-local managed path.

## Installation

Keep `AntigravityACP.InstallScript()` empty so remote bootstrap receives no shell payload.
Extend the current install job runner with a server-owned typed action, retaining the existing script path for other agents.
Requests accept an agent identity, never a URL, shell command, extraction path, or arbitrary version.
The controller selects the trusted Antigravity artifact for the host platform.
Add `supports_local_install` to discovery DTOs and web agent types.
Catalog filters and Install controls accept that capability or an existing install script.
The binary bundle flow shows guidance rather than a fabricated shell command.

The reviewed artifact table contains OS/architecture, exact version, Google URL, SHA-256, compressed-size limit, expanded-size limit, and two expected entry names.
Cover Linux, Darwin, and Windows x64/arm64 only when the official artifact exists and the platform has qualification evidence.
Fail unsupported combinations explicitly. Do not invent an artifact URL from a naming convention.
Recheck registry checksums and the selected native version during implementation, then pin the reviewed values in code.

Download to a task-owned staging directory with request context and bounded time/size.
Verify digest before extraction. Accept only the expected server and harness regular files.
Reject duplicate names, absolute paths, traversal, links, special files, excess entry count, and excess expanded bytes.
Apply executable permissions on Unix. Publish the complete directory with atomic same-filesystem rename.
Cancellation and failure remove staging only. Existing complete bundles remain untouched.
A valid existing destination is an idempotent success.
A corrupt or conflicting destination produces repair guidance; replacement requires stopping users first and must not overwrite a live bundle.
No automatic update or garbage collection is part of this package.

Reuse install maintenance and operator-mutation guards. Office/task credentials cannot initiate global installation or authentication.
After install, invalidate filesystem discovery and capability caches. A failed readiness probe does not become an install success claim about inference.

## Launch policy

Local subscription operations share one effective policy after inherited and profile environments have been combined.
Carry a small typed auth policy in the existing instance and inference configuration: selected method plus interactive permission.
Use `acp_auth_policy: {method_id: "oauth-personal", allow_interactive: false}` on the internal wire contract.
The dedicated setup instance sets `allow_interactive: true`; profiles and task requests cannot set it.
Advertise `antigravity_personal_auth_v1` through existing agentctl capability negotiation before using this contract or transient auth status.
Only eligible local Antigravity launches receive `oauth-personal`; only the dedicated operator setup instance permits interaction.
This is an internal child-process contract, not a new persisted profile option.
Remote and other-provider launches retain their existing policy. A retained agentctl lacking this contract requires upgrade guidance.
Validate home/locality before spawn. A profile changing the user home, a non-empty `GEMINI_HOME`, or unknown locality is ineligible for this local flow.
Return actionable setup guidance instead of redirecting state or silently dropping the override.
Preserve unrelated task environment variables.

Strip these conflicting inputs only from the Antigravity child environment:

- `GEMINI_API_KEY`, `GOOGLE_API_KEY`, `GOOGLE_APPLICATION_CREDENTIALS`.
- `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`, `GOOGLE_CLOUD_QUOTA_PROJECT`, `GOOGLE_GENAI_USE_VERTEXAI`.
- `AGY_ACP_CCPA_PROJECT`, `AGY_ACP_ENABLE_OAUTH`.

Validate these names against the selected vendor release and add a regression for every recognized credential-mode input.
Use the resolved harness path. Replace inherited `BROWSER` with a Kandev-owned no-op browser helper for this child only.
The helper exits successfully without launching a browser, preventing Python from falling through to another browser.
Use the resolved absolute agentctl executable, a private helper subcommand, and tested Python BROWSER argument quoting.
It must work with spaces and Windows drive letters. Do not shell-expand URL arguments.
A small helper is preferable to a remote OAuth bridge or a new credential reader.

Keep native credential backend selection. Do not force file storage or copy `agy` credentials.
The native macOS keychain and Linux/Windows file backend remain vendor-owned.
Shared profile flags and sandbox prefixes still apply. An unsupported wrapped environment must fail explicitly rather than bypass the subscription policy.

## Shared ACP compatibility

Add a narrow Antigravity policy to the existing compatibility helpers:

- Override only wire `clientInfo` to `zed` version `0.0.0`.
- Require `oauth-personal` in advertised methods for the local subscription path.
- Parse the vendor's consent prompt and structured errors.
- Recognize native human-input requests.

The adapter and utility executor both consume the same helpers.
The policy covers task initialize, capability probes, model resolution probes, utility inference, sign-in, and restored sessions.
Registered-agent acpdbg also uses the provider identity and local runtime policy; arbitrary raw `--exec` remains diagnostic.
Do not globally rename Kandev or report `zed` as the product or task owner.
This is a compatibility assumption about the vendor's client allowlist, not a guarantee of third-party model entitlement.
The override is scoped to the Antigravity agent only; disabling that agent is the kill switch.
No separate global identity flag is introduced because the vendor allowlist is the only reason for the override.

Preserve `initialize` data before attempting authentication or `session/new`.
On an unauthenticated result, probes return advertised auth methods and `auth_required`, without an empty successful catalog.
Other providers retain the canonical identity and authentication path.

## Authentication

### Background operations

After initialize, call native `authenticate` with `oauth-personal` before new/load/resume.
Cached credentials and silent refresh remain native operations.
If the server requires interactive consent, the stderr consumer recognizes its prompt before generic buffering or logging.
Because the no-op browser helper prevents browser launch, the operation can cancel authentication and report `auth_required` promptly.
Close a temporary probe/inference process and its loopback listener through existing bounded cleanup.
Do not automatically start an interactive retry from boot, Office, task start, or recovery.

Shared adapter/process code owns RPC sequencing and cancellation.
Pure dialect helpers only describe the compatibility policy and parse observations.
Use generation checks so a late auth result cannot initialize a replacement process.
Only the explicitly interactive setup operation may retain a consent URL.

### Explicit local sign-in

Add a small in-memory attempt owner to `hostutility.Manager`.
It creates a dedicated temporary native agentctl instance, using existing runtime ownership and authenticated clients.
Reuse configure/start/initialize/`Client.Authenticate`; do not implement another JSON-RPC transport.
`Client.ConfigureAgent` takes the complete argv in `agentArgs`, including the executable at index zero.
The sign-in owner passes `command.Args()` unchanged in that field; it must not pass `args[1:]`.
The separate `command` string does not supply a missing executable when structured arguments are present.
The receiver validates structured arguments before start. Keep that validation intact.
Test the sign-in caller through the real configure HTTP handler, including Linux flags, paths with spaces, and executable-only commands.
The [sign-in launch repair](../../../plans/antigravity-sign-in-launch/plan.md) records the regression and its delivery checks.
The dedicated instance has no task, injected task MCP, or inference prompt.
It is separate from task instances and from concurrent capability probes.

Call `Client.StreamUpdates` before ACP initialization. It opens the WebSocket
synchronously and starts its reader; `nil` means setup succeeded, not that the
reader ended. Setup errors use existing cleanup. Registration holds the stream
read lock, so disconnect cleanup drains registered requests and late requests are
rejected. Do not poll stream state.
The [stream-startup repair](../../../plans/antigravity-login-stream/plan.md)
records the regression and its connection-boundary verification.

Proposed operator endpoints beside the existing capabilities handler:

- `POST /api/v1/antigravity-acp/login`: start or return the active attempt.
- `GET /api/v1/antigravity-acp/login/:attemptID`: read its current snapshot.
- `DELETE /api/v1/antigravity-acp/login/:attemptID`: cancel it idempotently.

The login group is a sibling of `/api/v1/agent-capabilities` because Gin does not
allow a static `antigravity-acp` child beside that group's existing `:type`
wildcard.

Use the existing local operator/configuration mutation guard and maintenance exclusion on start/cancel.
Reads require the same authorized operator. Do not expose global login through workspace/task MCP capabilities.
Reject arbitrary agent names or auth-method inputs on this personal-only operation.

Snapshot fields: `attempt_id`, `state`, `expires_at`, optional active `authorization_url`, and a sanitized `error_code`/message.
States: `starting`, `waiting_for_browser`, `checking`, `succeeded`, `failed`, `cancelled`, `expired`.
Use one attempt per host process, a ten-minute overall deadline, and a short terminal retention period.
The latest attempt ID is discoverable to the authorized settings client so reload can reattach without persisting the URL.
Backend restart clears attempts; the UI returns to a fresh probe rather than resurrecting old URLs.

The agentctl stderr consumer stores a recognized URL only in an active auth attempt's transient memory.
Add a narrow authenticated `agent.auth.status` stream request to read that snapshot while `authenticate` is pending.
It is a request response, not a journaled task event. Redact it in transport tracing.
Extend the existing stderr consumer/sanitizer interfaces rather than creating another stderr reader.
Use the same parser for the utility executor's bounded stderr sink.
Generic stderr buffers, logs, and error strings retain a redacted marker only.
Verify status/cancel calls remain serviceable while the long authentication RPC is pending.
Retained agentctl versions without this capability fail with upgrade guidance; do not fall back to reading unredacted stderr.

The operator opens the validated URL in a browser on the Kandev machine.
Google redirects to the ephemeral localhost listener created by the native server.
Kandev serves no OAuth callback and exchanges no token.
The server validates OAuth state/PKCE and writes its own credentials and selected auth method.
A successful native response moves the attempt to `checking`, then a fresh ordinary capability probe establishes readiness and models.
If authentication succeeds but model discovery fails, preserve that distinction and expose retryable probe guidance.
Never tell users to repeat consent solely because a catalog request failed.

Cancellation, timeout, backend shutdown, and process failure close only the owned setup instance.
Clear the URL on every terminal transition. Reject late updates by attempt ID and process generation.
Keep ordinary task processes running. Do not delete native credentials when an attempt ends.
No cross-process account lock is claimed; independent Kandev processes share vendor refresh behavior.

## Native human questions

The current process manager can auto-approve the first allowed option.
Antigravity encodes human questions as permission requests whose tool-call ID starts with `interaction_`.
Their answer choices can all have `allow_once`, making ordinary auto-approve an accidental answer selection.

Classify this request before coordinator, managed-MCP, and generic approval branches.
Carry an explicit human-input classification through normalized permission data.
Only the Antigravity compatibility helper recognizes its native prefix.
Reuse the existing pending question/permission UI with exact option IDs and labels.
No additional form builder is needed.
Explicit human answers use the existing permission response and deduplication path.
Stop, cancellation, and stale-generation checks remain authoritative.
Ordinary tools keep existing approval policies. Office waits when the user must answer.
Registered-agent acpdbg stops a recognized human-question capture without selecting an option.
The product UI owns live answer qualification; a generic diagnostic auto-approval must not invent that answer.

## Failure and recovery

### Structured failures

Add provider-specific pure mappings to existing routing-error signatures and ACP translation.
Distinguish authentication, subscription/entitlement, model access, quota exhaustion, transient throttling, and provider failure.
Use structured status/reason/data and known native error envelopes.
A `-32000` with advertised auth methods is not sufficient evidence of an auth error by itself.
Preserve valid reset timestamps or duration hints. Leave missing reset data unknown.
Never parse normal assistant prose or arbitrary tool output as a provider error.

### Errors hidden behind end_turn

For the qualified release, use a small read-only native outcome reader, separate from usage accounting.
The candidate path is `~/.gemini/antigravity-acp/conversations/<native-session-id>.db`.
Qualify the exact location and schema with the selected version before coding against them.
T3 observes table `steps`, columns `idx`, `step_type`, `step_payload`, and error step type 17.
Those are private vendor details, not a stable ACP contract.

Use the existing Go SQLite dependency. Open read-only, respect WAL, and bound busy waits and payload reads.
Validate the session identifier, root containment, every path component, regular-file type, and file identity.
Reject symlinks or replacement files. Never scan other sessions or import their history.
Before submitting a prompt, record the current high-water index or prove the file is absent.
After the result, inspect only newly appended current-turn records, capped at 16 KiB of relevant error payload.
An old error is not a new failure. A resumed session keeps the same before/after boundary rule.
Native `refusal` is evidence of quota only for the qualified version whose mapping has been verified.
Other agents retain ordinary refusal semantics.
The validated native `/usage` command uses its RPC result without requiring a model trajectory record.
Do not exempt arbitrary slash-prefixed prompts from outcome checks.

If the native version hides failures and the required outcome check is unavailable, return a sanitized outcome-verification failure.
Do not emit success completion or auto-advance the workflow from an unverified outcome.
Classify this as an unknown provider/runtime compatibility failure, not fabricated quota or auth.
Missing journal data never triggers a replay or model switch.
Once a later vendor version provides reliable structured failures, qualification may remove this reader through a separate contract update.

### Recovery ownership

Send normalized failures through current task recovery and Office routing policy.
Basic task chat benefits without Office.
No new cooldown, provider poller, account scheduler, or automatic retry loop is introduced.
Keep recoverable prompt errors within the existing native process and conversation.
Follow the accepted ACP prompt-error lifetime decision; process death and broken transport remain separate failures.
Late completion/error events must retain existing generation and turn ownership checks.

## Usage and billing

`AntigravityACP.BillingType()` supplies subscription default metadata for this supported personal path.
It is not a verified account invoice or the effective billing proof for legacy remote/API-key setups.
Do not register an Antigravity allowance client in the Office usage adapter.

The current ACP adapter already reads optional `session/prompt` usage and `usage_update` context data.
Preserve that path and add fixtures instead of building a parallel token collector.
The native fields include `inputTokens`, `outputTokens`, `cachedReadTokens`, `cachedWriteTokens`, `thoughtTokens`, and `totalTokens`.
Keep existing per-turn normalization, token deduplication, message persistence, and reload behavior.
`usage_update.used/size` is context occupancy, not cumulative billing usage.
Main-trajectory values may omit child-agent usage. Record the observed scope rather than claiming task-complete totals.

The investigated server does not supply a provider-billed dollar amount.
Keep provider cost absent. Existing price-based estimates remain estimates, including under a subscription.
Do not infer an extra charge or measured zero cost from billing metadata.
Where model pricing is unavailable, leave the estimate unavailable.
Add concise localized subscription guidance in the existing usage details surface.

Pass `/usage` through the native slash-command path.
The server performs its own allowance lookup and returns Markdown in task chat without model inference.
Its internal Google HTTP endpoint is not a public Kandev integration contract.
Do not parse that Markdown into quota bars or count the command as a new model token delta.
No fresh five-hour/weekly guarantee is implied; display whatever windows and reset information the server actually returns.

## Setup UI

Reuse the installed-agent card, install job events, capability statuses, and model refresh hooks.
Add a focused Antigravity sign-in view instead of sending users to an unstructured host shell.
Other agents retain their current PTY login dialogs.
Task auth-required guidance links to this same view and preserves the task draft and task identity.

Desktop composition:

- The card offers Install ACP when missing, Sign in with Google when needed, and Ready after capability confirmation.
- A focused dialog shows subscription guidance, browser progress, Open Google sign-in, Copy link, Cancel, and clear retry state.
- The dialog explains the separate ACP login and same-machine callback.

Phone composition:

- Follow `components/task/mobile/mobile-picker-sheet.tsx` and the existing `useTouchDrawer`/responsive breakpoint conventions.
- Use an inset sheet for the short action flow, with a focused full-screen state if text or keyboard needs more room.
- Place status and the primary action above supporting details. Keep Cancel reachable and controls at least 44px.
- Offer start, monitor, copy, cancel, and retry. Explain that the link must be opened on the Kandev machine.
- Do not make a same-phone loopback flow the primary action. No remote callback relay is added.

Use live status announcements, visible focus, Escape/Back behavior, and translated copy in all seven locales.
Generate the Traditional Chinese pair through the repository tool.
Reload reads the active attempt snapshot. Terminal states clear the URL and restore focus to the opener.
Use synthetic mock auth in browser tests; CI must not consume a real Google subscription or launch consent.

## Persistence

No application database migration is required.
Managed runtime files are separate from the native store.
Only runtime provenance/version and existing agent/profile data enter ordinary product responses.
Auth attempts and URLs remain in process memory. The vendor persists tokens and selected auth method.
Usage and normalized failures use existing durable task contracts without raw native database payloads.

## Security

Global setup actions use existing operator authority, mutation guards, and maintenance exclusion.
Do not trust a task token to authorize host credential changes.
Accept consent URLs only from the exact active native process.
Require HTTPS `accounts.google.com`, the recognized authorization path, PKCE/state fields, and an HTTP localhost/loopback redirect with a valid ephemeral port.
Reject userinfo, fragments, foreign redirects, and expired/foreign-attempt URLs.
Native code owns state comparison and token exchange; Kandev does not proxy callbacks.
No arbitrary URL opener or network fetch is exposed by the helper.

Sanitize URLs before generic stderr storage, tracing, logs, and errors.
Never put OAuth secrets or native error blobs into metrics, task messages, public fixtures, or spec examples.
Use fabricated URLs and credentials in tests.
Read the native outcome journal only for the active session and only through the bounded read-only reader.

## Observability

Reuse install/probe/auth status and structured logs for runtime provenance, version, platform, stage, and sanitized reason code.
Distinguish download failure, incomplete install, native crash, consent required, consent cancelled, catalog failure, and unverified turn outcome.
Do not add high-cardinality metrics or expose consent URLs in diagnostics.
No new metrics service is required.

## Qualification

The current Linux SIGILL prevents an unqualified success claim on the investigated host.
The CPU 0 diagnostic suggests a native per-CPU indexing issue; that root cause remains an inference.
Do not ship a universal affinity wrapper as its fix.
Before marking local subscription support complete, run the reviewed native release without affinity on each claimed platform.
Record startup, consent, authenticated model discovery, prompt, `/usage`, stop, resume, and two simultaneous tasks.
If the Linux release remains broken, report that platform as blocked and preserve actionable diagnostics.
Do not mark the live work order passed or quietly substitute `agy`.

## Related decisions

- [Host utility agentctl](../../../decisions/0002-host-utility-agentctl-for-sessionless-flows.md).
- [ACP compatibility dialects](../../../decisions/0044-acp-agent-compatibility-dialects.md).
- [Operator-owned agent settings](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md).
- [Runtime update authority](../../../decisions/2026-10-01-agent-runtime-update-authority.md).
- [ACP prompt-error runtime lifetime](../../../decisions/2026-10-29-acp-prompt-error-runtime-lifetime.md).

These existing decisions supply the ownership rules. Provider-local choices and their alternatives are recorded here.
No new generic authentication framework, account manager, or duplicate ADR is required.
