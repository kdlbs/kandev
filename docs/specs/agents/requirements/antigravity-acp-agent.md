---
status: draft
system: agents
created: 2026-09-03
owners:
  - kandev
---

# Google Antigravity ACP agent requirements

## Overview

The built-in agent uses ACP with Google's personal subscription. It selects the
local CLI and sign-in when available; otherwise it uses the standalone server.

Related: [design](../system-design/antigravity-acp-agent.md),
[standalone](../../../plans/antigravity-local-subscription/plan.md), and
[CLI](../../../plans/antigravity-cli-acp/plan.md) plans.

## Terminology

- **Standalone server:** bundled `antigravity-acp` executable.
- **Harness:** companion `localharness_external` or legacy `localharness`.
- **Native store:** Google's settings, credentials, and conversations.
- **Subscription mode:** personal Google OAuth; Google sets entitlement and allowance.
- **Local execution:** native process in the host user home, including worktrees.
- **Estimated cost:** token count times model prices.

## Draft revision and traceability

This table preserves retired IDs from the September draft and links replacements.

| Retired draft criteria | Former constraint | Replacement |
| --- | --- | --- |
| AC-AGENTS-ANTIGRAVITY-ACP-001.7 | Manual install only. | REQ-AGENTS-ANTIGRAVITY-ACP-009, REQ-AGENTS-ANTIGRAVITY-ACP-015 |
| AC-AGENTS-ANTIGRAVITY-ACP-002.1, AC-AGENTS-ANTIGRAVITY-ACP-002.2, AC-AGENTS-ANTIGRAVITY-ACP-002.4, AC-AGENTS-ANTIGRAVITY-ACP-002.5, AC-AGENTS-ANTIGRAVITY-ACP-002.6, AC-AGENTS-ANTIGRAVITY-ACP-002.7 | PATH-only lookup and unchecked harness override. | AC-AGENTS-ANTIGRAVITY-ACP-009.3 |
| AC-AGENTS-ANTIGRAVITY-ACP-003.9 | Inherit the environment unchanged. | AC-AGENTS-ANTIGRAVITY-ACP-010.2 |
| AC-AGENTS-ANTIGRAVITY-ACP-004.2, AC-AGENTS-ANTIGRAVITY-ACP-004.9 | Ignore non-default GEMINI_HOME. | AC-AGENTS-ANTIGRAVITY-ACP-010.3 |
| AC-AGENTS-ANTIGRAVITY-ACP-005.3 | Generic `-32000` auth classification only. | REQ-AGENTS-ANTIGRAVITY-ACP-013 |
| AC-AGENTS-ANTIGRAVITY-ACP-006.5 | Inherit default API-key billing. | AC-AGENTS-ANTIGRAVITY-ACP-014.1 |
| AC-AGENTS-ANTIGRAVITY-ACP-007.2, AC-AGENTS-ANTIGRAVITY-ACP-007.3 | No local install; missing-binary failure only. | REQ-AGENTS-ANTIGRAVITY-ACP-009, AC-AGENTS-ANTIGRAVITY-ACP-007.4 |

## Requirements

### REQ-AGENTS-ANTIGRAVITY-ACP-001: Built-in Antigravity agent identity

**Intent:** Preserve the existing identity and profiles.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-001.1:** The identifier shall remain `antigravity-acp`.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.2:** Display and catalog names shall remain `Antigravity` and `Antigravity ACP Agent`.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.3:** The default registry shall include the enabled agent.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.4:** Display order shall remain `22`, distinct from other built-in agents.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.5:** Both logo variants shall use the Antigravity arch. Unknown variants shall use the light logo.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.6:** The agent shall use ACP and remain non-virtual.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.8:** Profiles shall offer Kandev auto-approve and an off-by-default `--dangerously-skip-permissions` switch.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.9:** CLI passthrough shall be unavailable.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.10:** Any enabled exact duplicate shall turn the switch on. Saving shall persist one entry and preserve unrelated flags.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.11:** Include the flag only for supported CLI launches that permit blanket approval. Omit it for unsupported, standalone, non-local, coordinator, or managed-tool launches.

### REQ-AGENTS-ANTIGRAVITY-ACP-002: Discovery that fails closed on a partial install

**Intent:** Check installation completeness without spawning a process during filesystem discovery.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-002.3:** Manual installs shall require a sibling `localharness_external` or `localharness`, in that order, unless a validated override applies.
  Windows names shall have `.exe`. Candidates shall be regular files, executable on Unix. Invalid candidates shall be skipped.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.8:** Filesystem discovery shall create no files, write no configuration, and start no process.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.9:** Cancelled or expired discovery shall return the context error.

### REQ-AGENTS-ANTIGRAVITY-ACP-003: Consistent runtime selection

**Intent:** Preserve deterministic base commands and consistent resolution.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-003.1:** The standalone base command shall be `agy_acp_server.exe` on Windows and `agy_acp_server.par` on macOS.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.2:** The standalone Linux base command shall be `agy_acp_server.par --uid=`.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.3:** Base command platform selection shall use the Kandev host platform. Cross-platform remote execution remains excluded.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.4:** Base construction shall remain deterministic, without filesystem, environment, or PATH reads.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.5:** Session, runtime, and inference shall use equivalent base commands for the selected runtime.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.6:** Base arguments shall not vary with model, permissions, auto-approve, agent type, or resume target.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.7:** Agent-owned construction shall add neither `--debug` nor `--notices`.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.8:** Task sessions shall use their workspace as the working directory.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.10:** Declared limits shall remain 4096 MB, two CPU cores, and one hour.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.11:** The agent shall retain graceful shutdown rather than always requesting immediate kill.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.12:** One-shot inference shall remain supported.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.13:** Shared profile flags and sandbox prefixes shall retain their behavior after base construction.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.14:** A qualified `agy` shall serve local previews, model discovery, inference, and new local or worktree sessions.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.15:** If no CLI qualifies, new local operations shall select the complete standalone server.
  A failure from the selected runtime shall report that runtime's error.

### REQ-AGENTS-ANTIGRAVITY-ACP-004: Config root, sessions, and skills

**Intent:** Use the vendor store for each runtime and preserve resume.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-004.1:** The standalone session directory shall remain `{home}/.gemini/antigravity-acp`. The CLI shall use its own default store.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.3:** Native resume and recovery declarations shall remain supported.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.4:** Kandev shall add no per-launch root, injected `GEMINI_HOME`, or config-root flag.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.5:** Project skills shall use `.agents/skills`.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.6:** User skills shall use `.gemini/config/skills`.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.7:** Antigravity and Gemini directory templates shall remain distinct, without aliasing or merging.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.8:** Kandev shall not modify native state during a session. Existing explicit remote-auth copying remains the only Kandev write exception.
  Writes performed by the ACP server remain server-owned.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.10:** The container session-directory target shall remain empty, without a new bind mount.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.11:** A saved conversation shall resume with the runtime that created it.
  Older Antigravity sessions use the standalone server. If the runtime is unavailable, Kandev shall preserve the conversation and report the resume failure.

### REQ-AGENTS-ANTIGRAVITY-ACP-005: Authentication owned by the ACP server

**Intent:** Keep credential acquisition, refresh, and persistence inside the vendor server.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-005.1:** Advertised auth methods shall come from `initialize`, including when `session/new` requires authentication.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.2:** The method list shall retain server order. The subscription action shall not rewrite it.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.4:** A stderr traceback accompanying a handled JSON-RPC error shall not independently cause fatal launch failure.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.5:** Failed authentication shall expose its sanitized cause, including a missing required environment variable.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.6:** Kandev shall not mint, refresh, or rewrite native credentials.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.7:** Existing remote-auth metadata shall retain optional copies of `settings.json`, `acp_token.json`, and `acp_business_token.json` for Darwin and Linux.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.8:** The existing copy method shall tolerate absent method-specific files.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.9:** Existing remote-auth metadata shall retain explicit `GEMINI_API_KEY` and `GOOGLE_API_KEY` methods.
  These declarations do not guarantee a new remote subscription sign-in flow.

### REQ-AGENTS-ANTIGRAVITY-ACP-006: Capabilities read from the agent

**Intent:** Discover models and features from the selected server.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-006.1:** Capabilities shall come from advertised fields, not an echoed protocol version.
- **AC-AGENTS-ANTIGRAVITY-ACP-006.2:** The agent shall add no assumed HTTP or SSE MCP capability override.
- **AC-AGENTS-ANTIGRAVITY-ACP-006.3:** MCP servers shall pass through session creation without a project-local MCP strategy.
- **AC-AGENTS-ANTIGRAVITY-ACP-006.4:** The agent shall add no assumed server namespace for MCP tool names.
- **AC-AGENTS-ANTIGRAVITY-ACP-006.5:** At the next process start, all local CLI profiles and models shall receive required MCP servers without profile edits.

### REQ-AGENTS-ANTIGRAVITY-ACP-007: No automated remote provisioning

**Intent:** Keep local installation separate from executor bootstrap scripts.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-007.1:** `InstallScript()` shall remain empty.
- **AC-AGENTS-ANTIGRAVITY-ACP-007.4:** Remote bootstrap shall receive no Antigravity installer.
  A missing remote runtime shall fail without downloading, substituting a host path, or switching agents.

### REQ-AGENTS-ANTIGRAVITY-ACP-008: Concurrent sessions

**Intent:** Preserve independent sessions while sharing the native store.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-008.1:** Concurrent tasks shall each launch an independent server process.
- **AC-AGENTS-ANTIGRAVITY-ACP-008.2:** Task processes shall not share a Kandev lock on the native store. Native refresh coordination remains vendor-owned.
- **AC-AGENTS-ANTIGRAVITY-ACP-008.3:** Stopping one task shall neither stop another nor remove shared native state.

### REQ-AGENTS-ANTIGRAVITY-ACP-009: Explicit local runtime installation

**Intent:** Install the official server and harness without overwriting user-managed binaries.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-009.1:** Settings shall identify the standalone install and install both executables from a reviewed release.
  A qualified CLI satisfies local installation.
- **AC-AGENTS-ANTIGRAVITY-ACP-009.2:** Installation shall verify trusted digest and bounded size, reject unsafe archive entries, and activate only a complete bundle.
  Failure or cancellation shall preserve the previous usable installation. Launch and discovery shall never download implicitly.
- **AC-AGENTS-ANTIGRAVITY-ACP-009.3:** Standalone operations shall resolve the same complete managed bundle, otherwise a complete manual PATH install.
  An invalid existing managed bundle shall fail visibly without silently switching versions. Manual harness overrides shall be validated.
- **AC-AGENTS-ANTIGRAVITY-ACP-009.4:** Installation and readiness shall remain distinct. Native crashes and unsupported platforms shall have actionable unavailable results.
  Kandev shall not silently add CPU affinity or claim live qualification from fixture tests.

### REQ-AGENTS-ANTIGRAVITY-ACP-010: Local personal Google authentication

**Intent:** Use the Google sign-in that belongs to the selected runtime.

Criteria 010.1 through 010.6 apply to the standalone server.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-010.1:** Local subscription operations shall explicitly select advertised `oauth-personal` before session creation or restore.
  Missing support or authentication failure shall stop the operation without API-key fallback.
- **AC-AGENTS-ANTIGRAVITY-ACP-010.2:** Tasks, probes, inference, and sign-in shall share one subscription launch policy.
  Conflicting API-key and provider-mode inputs shall be removed from child environments only.
  Background operations shall never open a consent browser. Required consent shall return `auth_required` promptly.
- **AC-AGENTS-ANTIGRAVITY-ACP-010.3:** Local operations shall share the native user home and default store.
  Conflicting profile HOME or non-empty `GEMINI_HOME` shall produce unsupported-configuration guidance without creating another store.
- **AC-AGENTS-ANTIGRAVITY-ACP-010.4:** Explicit Sign in with Google shall run native `authenticate` and expose its active consent URL.
  Google shall redirect to the native loopback listener on the same machine. Kandev shall need no hosted callback or token exchange.
- **AC-AGENTS-ANTIGRAVITY-ACP-010.5:** One sign-in attempt shall run per Kandev host process, with status, cancellation, expiry, and failure.
  Late results shall not replace newer state. Success shall require native authentication and refreshed capabilities, not a clicked Done button.
- **AC-AGENTS-ANTIGRAVITY-ACP-010.6:** Consent URLs shall come from the active native process and pass Google HTTPS and loopback-redirect validation.
  URLs shall remain transient and operator-only. Logs, task messages, durable events, and fixtures shall contain no secrets or consent URLs.
- **AC-AGENTS-ANTIGRAVITY-ACP-010.7:** Local CLI ACP shall use the existing CLI sign-in. When Google sign-in is required, Settings shall offer the CLI sign-in action.
  The CLI shall share the Kandev user's home. Readiness shall come from a fresh ACP probe after sign-in.

### REQ-AGENTS-ANTIGRAVITY-ACP-011: Consistent ACP compatibility

**Intent:** Discover the account's models with one provider-specific wire policy.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-011.1:** Standalone initialization shall send `clientInfo.name = zed` and `clientInfo.version = 0.0.0`.
  CLI ACP shall use Kandev's regular ACP identity. Product records and logs shall identify Kandev normally.
- **AC-AGENTS-ANTIGRAVITY-ACP-011.2:** Models shall come from authenticated ACP discovery, without copying `agy models` or inventing entitlement.
  An unavailable selected model shall fail explicitly without fallback.
- **AC-AGENTS-ANTIGRAVITY-ACP-011.3:** New sessions, advertised resume/load, probes, and utility inference shall use the same runtime, home, auth, and client policy.
  Unsupported recovery methods shall retain existing explicit recovery behavior.

### REQ-AGENTS-ANTIGRAVITY-ACP-012: Human questions survive auto-approve

**Intent:** Separate tool permission automation from answers only the user can choose.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-012.1:** Antigravity `interaction_` permission requests shall be classified as human input before automatic approval.
  Auto-approve shall never select an answer. Prefixes from other agents shall not trigger this provider-specific handling.
- **AC-AGENTS-ANTIGRAVITY-ACP-012.2:** Pending permissions shall preserve question text, labels, option IDs, and request identity.
  An explicit user response shall send the matching option once. Cancellation, stop, and stale responses shall retain existing safe behavior.
- **AC-AGENTS-ANTIGRAVITY-ACP-012.3:** Ordinary permissions shall retain manual, auto-approve, coordinator, and managed-MCP policies.
  Unattended execution shall wait for human questions rather than invent answers.

### REQ-AGENTS-ANTIGRAVITY-ACP-013: Provider failures remain failures

**Intent:** Give normal tasks and Office accurate recovery information.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-013.1:** Structured auth, entitlement, model-access, quota, throttling, and provider errors shall map to existing normalized codes.
  Generic `-32000` shall not always mean authentication failure. Unknown errors shall retain a sanitized cause.
- **AC-AGENTS-ANTIGRAVITY-ACP-013.2:** A failed current turn shall not appear successful solely because ACP returned `end_turn`.
  A bounded, read-only native error record may establish failure. Earlier errors, assistant prose, and tool output shall not establish new failures.
  If required native outcome evidence cannot be verified, Kandev shall expose that limitation and withhold success completion.
- **AC-AGENTS-ANTIGRAVITY-ACP-013.3:** Quota/reset and retry hints shall use valid provider evidence only. Missing reset information shall remain unknown.
  Failures shall use existing recovery without automatic replay, model fallback, or workflow completion.
  Recoverable prompt errors shall not by themselves destroy the native process or conversation.

### REQ-AGENTS-ANTIGRAVITY-ACP-014: Honest usage and subscription reporting

**Intent:** Show available usage without treating estimated dollars as an invoice.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-014.1:** Default billing metadata shall identify subscription usage for this personal OAuth path.
  UI copy shall distinguish estimates from additional Google charges. Metadata shall not promise entitlement or zero charges for other methods.
- **AC-AGENTS-ANTIGRAVITY-ACP-014.2:** Supplied prompt usage shall retain input, output, cache-read, cache-write, thought, and total tokens through existing contracts.
  Context occupancy shall remain separate. Missing fields shall not become invented measurements or duplicate deltas. Coverage shall state subagent inclusion.
- **AC-AGENTS-ANTIGRAVITY-ACP-014.3:** Absent provider dollar cost shall remain absent. Price-based calculations shall remain explicitly estimated.
  Missing model pricing shall not become a measured zero-dollar charge.
- **AC-AGENTS-ANTIGRAVITY-ACP-014.4:** Native `/usage` shall use regular task commands and render the server's allowance/reset response.
  It shall require no separate Google client, Office mode, fabricated quota bars, or synthetic model turn.

### REQ-AGENTS-ANTIGRAVITY-ACP-015: Clear desktop and phone setup

**Intent:** Make local setup understandable on every supported UI size.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-015.1:** Settings shall distinguish missing runtime, installing, needs sign-in, signing in, ready, and actionable failure.
  Sign-in success shall refresh models. Task auth guidance shall open setup for its runtime and preserve the task draft.
- **AC-AGENTS-ANTIGRAVITY-ACP-015.2:** Desktop shall use existing settings cards and focused dialogs.
  Phone shall use an inset sheet or focused screen, visible primary actions, 44px touch targets, and no horizontal overflow.
  For standalone consent, phone users shall be able to start, monitor, cancel, and copy the link.
  Guidance shall require consent in a browser on the Kandev machine. Opening a phone loopback callback shall not be presented as supported.
- **AC-AGENTS-ANTIGRAVITY-ACP-015.3:** New copy shall use localization, accessible status announcements, focus management, and retry/cancel actions.
  Reload shall reattach to active attempts. Closed or expired attempts shall not retain actionable stale URLs.
- **AC-AGENTS-ANTIGRAVITY-ACP-015.4:** The agent list and profile shall identify the selected runtime and offer its sign-in action.
  When no compatible runtime is installed, the agent list shall offer CLI setup and the standalone install where supported.

## Out of scope

- Multiple accounts, alternate homes, credential import, and Kandev-owned refresh.
- Hosted callbacks, remote forwarding or provisioning, and new enterprise or API-key setup.
- Automatic runtime updates and CPU affinity.
- Quota polling, allowance bars, quota-based routing, and history-database accounting.
- Static model catalogs and subscription entitlement guarantees.
