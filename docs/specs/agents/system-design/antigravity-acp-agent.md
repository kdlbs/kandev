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

Agents owns the built-in profile's runtime selection, readiness, authentication, and setup.
Google owns credentials, refresh, entitlement, consent callbacks, and conversation storage.
Tasks, Costs, Office, and Platform consume existing normalized observations.
Each native process shares the host user's store for its runtime.

See the [requirements](../requirements/antigravity-acp-agent.md) and the
[standalone](../../../plans/antigravity-local-subscription/plan.md) and
[CLI](../../../plans/antigravity-cli-acp/plan.md) delivery plans.

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

Google added the supported CLI command in 1.5.0. See the [CLI ACP documentation](https://antigravity.google/docs/cli/acp).
On November 23, CLI 1.5.15 returned 27 model and effort variants and completed a prompt on Linux amd64.
The live standalone model probe reported auth_required. A separate standalone launch exited with SIGILL.

## Components and responsibilities

| Boundary | Existing location | Planned responsibility |
| --- | --- | --- |
| Agent declaration | `internal/agent/agents/antigravity_acp.go` | Preserve identity, base arguments, skills, resume, and remote metadata. Declare the subscription default. |
| Native bundle helper | `internal/agent/antigravityruntime/` | Trusted artifact records, bounded installation, filesystem validation, and local resolution. No generic native package manager. |
| Install orchestration | `internal/agent/settings/controller/agent_install.go`, `install_job.go` | Reuse job status, output, cancellation, maintenance interlock, and operator authorization. Accept a typed Go action for this local bundle. |
| Launch selection | `internal/agent/runtime/lifecycle/manager_launch.go`, `internal/agent/hostutility/manager.go` | Consume the same selected runtime for local task, model, and inference paths. |
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
Base constructors remain pure. The local resolver supplies the entire base command before profile flags and wrappers apply.
MCP configuration stays in ACP session creation. No project-local MCP file or assumed transport override is added.
Native load/resume decisions remain capability-driven.

## Local runtime resolution

Resolve the local variant in `internal/agent/antigravityruntime`, with absolute argv,
provenance, version, and a harness path for standalone. The
[runtime selection decision](../../../decisions/2026-11-23-antigravity-cli-acp-runtime.md)
defines the priority and ownership.

1. For native restore, select the variant on the saved conversation.
2. For new host-local work, find `agy` on the host PATH or in Google's standard install location.
3. Run a bounded `agy --version` from the host, outside any task workspace.
   For version 1.5.0 or later, check bounded `agy acp --help` output for the exact worker flag.
   A qualified CLI selects `[absolute-agy, "acp", "--process-per-session"]`.
   The ACP handshake establishes readiness. [Google ACP flags](https://antigravity.google/docs/ide/extensions/acp#flags)
4. If no CLI qualifies for new local work, select the complete standalone bundle.
   Keep managed-bundle validation authoritative ahead of the manual PATH and harness checks.
5. Report version, startup, authentication, and probe failures for the chosen runtime.
   Propagate caller cancellation. Missing remote runtimes follow executor-local admission.

Filesystem discovery checks the CLI candidate and the complete standalone bundle without spawning.
Settings offers setup when qualification determines that only an older CLI is present.
Reuse the selected variant throughout one launch or probe. Bind cached capabilities to its
provenance and invalidate them on runtime change or explicit refresh. Discard work from stale probes.
Host utility and profile previews use host resolution. Non-local task launch resolves only its executor command.

### CLI worker mode and MCP

The resolver sets worker mode for each host-local CLI process, independent of the model and profile.
Compose the effective task, resume, utility, and preview arguments before the wrapper applies.
If a profile flag disables worker mode, put the required enabling flag after profile options and before any argument delimiter.
Enabled duplicates are compatible. Keep profile settings and saved runtime associations intact.

Use the HTTP capability advertised by the resulting ACP process. The common adapter delivers Core and selected packages.
The [MCP startup repair](../../../plans/antigravity-cli-mcp-default/plan.md) records the regression and launch checks.

## Installation

Keep `AntigravityACP.InstallScript()` empty so remote bootstrap receives no shell payload.
Extend the current install job runner with a server-owned typed action, retaining the existing script path for other agents.
Requests accept an agent identity, never a URL, shell command, extraction path, or arbitrary version.
The controller selects the trusted Antigravity artifact for the host platform.
Discovery projects `supports_local_install` for the pinned standalone installer.
An eligible CLI satisfies installation. Use a vendor setup link for a missing or older CLI.

The compiled artifact table pins platform, version, Google URL, digest, size limits, and both entry names.
Verify the digest in bounded staging. Extract each named regular file exactly once, with path containment.
Publish the executable pair by atomic same-filesystem rename. Cancellation removes only staging.
Reuse a valid destination. Report corrupt or conflicting directories for operator repair.

Reuse install maintenance and operator-mutation guards. Office/task credentials cannot initiate global installation or authentication.
After install, invalidate filesystem discovery and capability caches. A failed readiness probe does not become an install success claim about inference.

## Launch policy

Local operations share one effective policy after environment merge. A typed CLI variant in
instance and inference configuration selects the CLI policy. A missing variant selects the standalone policy.
Advertise native CLI support through agentctl capability negotiation. A stale helper requires upgrade guidance.

The standalone policy carries selected method and interactive permission.
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

Keep native credential backend selection. The CLI uses its vendor-owned default store under the host user home.
Apply the shared-home validation and conflicting credential-mode filtering to CLI ACP.
Use the selected worker command for CLI probes and tasks. The standalone harness path and `--uid=` belong to standalone.
The native macOS keychain and Linux/Windows file backend remain vendor-owned.
Shared profile flags and sandbox prefixes still apply. An unsupported wrapped environment must fail explicitly rather than bypass the subscription policy.

## Shared ACP compatibility

Add a narrow Antigravity policy to the existing compatibility helpers:

- For standalone, use wire `clientInfo` `zed` version `0.0.0` and the explicit `oauth-personal` policy.
- For CLI ACP, keep Kandev's regular wire identity and use the CLI's cached sign-in for `session/new` and resume.
- Parse the vendor's consent prompt and structured errors.
- Recognize native human-input requests.

The adapter and utility executor both consume the same helpers.
The utility allowlist verifies the built-in CLI variant's executable and `acp` invocation against host resolution.
The policy covers task initialize, capability probes, model resolution probes, utility inference, sign-in, and restored sessions.
Registered-agent acpdbg also uses the provider identity and local runtime policy; arbitrary raw `--exec` remains diagnostic.
Preserve `initialize` data before `session/new`. Classify auth from a typed failure or a
recognized provider auth error. The method list describes options. Use ACP model
and config options subject to the existing exact model-selection rules.

## Authentication

### Background operations

For standalone, call native `authenticate` with `oauth-personal` before new/load/resume.
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

For CLI ACP, use the operator-only agent login PTY with
`[absolute-agy, "acp", "--login"]` from the server's resolved variant. Revalidate
the variant at start. Return a refreshable error if it changed. The CLI owns consent and storage.
After exit or dismissal, run a fresh probe for readiness. Use the flow below for standalone.

Standalone recovery links select the standalone consent flow even when new sessions use the CLI.
Resolve and probe the standalone bundle directly for that attempt, with its own capability result.

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

For standalone 1.4.5, use the bounded read-only outcome reader.
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
CLI ACP uses its own wire evidence. The standalone journal and refusal heuristic apply to standalone only.
Qualify CLI failure outcomes, including any error hidden behind `end_turn`, before claiming support.

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

Reuse the installed-agent card, profile status panel, install events, and refresh hooks.
Project the resolved runtime from the backend. The list and profile use one sign-in action selection.
CLI sign-in opens the existing agent login terminal. Standalone opens the Antigravity consent dialog.
Task auth guidance links to the matching setup entry and preserves task identity and draft.

Desktop composition:

- The installation section links to CLI setup and labels the pinned standalone install. An installed runtime shows its name.
- The model status panel offers the matching sign-in action on authentication failure and enables model selection when ready.
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

Record a tagged vendor adapter version for CLI ACP in `HarnessSessionGeneration.AdapterVersion`
with its native conversation ID. Carry the same provenance in the existing ACP
association for paths that lack a generation. A saved Antigravity ID without a
tag denotes standalone. Restore reads the committed association before spawning.
If a saved variant is unavailable, report a typed restore failure and preserve the recorded identity.
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

Record qualification for each variant and platform independently.
For the CLI, record startup, authenticated models, a prompt, a failure, and resume under the same native ID.
The standalone Linux crash remains open. Its release qualification uses an unmodified launch,
then consent, models, prompt, `/usage`, stop, resume, and simultaneous tasks.

## Related decisions

- [Host utility agentctl](../../../decisions/0002-host-utility-agentctl-for-sessionless-flows.md).
- [ACP compatibility dialects](../../../decisions/0044-acp-agent-compatibility-dialects.md).
- [Operator-owned agent settings](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md).
- [Runtime update authority](../../../decisions/2026-10-01-agent-runtime-update-authority.md).
- [ACP prompt-error runtime lifetime](../../../decisions/2026-10-29-acp-prompt-error-runtime-lifetime.md).
