---
created: 2026-11-02
status: partial
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
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
legacy_specs: []
---

# Implementation Plan: Antigravity local subscription support

## Overview

Deliver a usable local Antigravity ACP path with the user's personal Google AI subscription.
Install the separate native server and harness, complete its own Google OAuth flow, and reuse that login across local tasks.
Preserve native questions, model selection, failure recovery, and usage information.

The package updates the existing [requirements](../../specs/agents/requirements/antigravity-acp-agent.md) and [system design](../../specs/agents/system-design/antigravity-acp-agent.md).
It contains seven work orders.
Tasks 01-06 are implemented; Task 07 remains the live native qualification gate.
A later explicit implementation request started production and permanent test work.

## Scope

### In scope

- One personal Google account using `oauth-personal` on the Kandev host.
- Explicit local native-bundle installation through current install jobs, without changing remote bootstrap scripts.
- One shared native store and native credential backend across task, probe, inference, and resume.
- Antigravity-only `zed` wire identity for live and diagnostic model discovery.
- Focused settings sign-in, phone presentation, and task links back to setup.
- Human questions preserved under auto-approve.
- Structured errors and a bounded current-turn native outcome check for hidden failures.
- Optional native token usage, context occupancy, estimated-cost clarity, and native `/usage`.
- Deterministic tests plus explicitly separate live vendor qualification.

### Out of scope

- Multi-account management, hosted OAuth callbacks, callback forwarding, and credential import from `agy`.
- Enterprise/ADC/API-key setup UI or automatic remote provisioning.
- Automatic runtime updates, a general native package manager, and overwriting external PATH installs.
- CPU affinity as a default fix.
- Google quota polling, Office allowance bars, quota-driven routing, and historical SQLite usage import.
- Delegation, commits, pushes, PR creation, and production edits during this design turn.

## Confirmed intent and evidence

The user wants a simple local subscription path and already signed into `agy`.
That login does not satisfy the standalone ACP server's separate credential store.
The user accepted using the Antigravity-specific `zed` identity after the T3 comparison.
No extra account-isolation layer is needed.

Investigation used Kandev `f648e79f9ecc7d5db0db301f20d0b23c7cb44d97`, T3 `3ddd3989e03e44c0106c6e3e44033a2f5b8b50ae`, CLI 1.4.12, and official ACP 1.4.5.
See the design's evidence table for primary links, artifact digest, and measured versus source-only observations.
The local consent attempt was cancelled before approval. Successful subscription inference remains unverified.
The raw ignored captures are private research material, not reusable committed fixtures.

The current vendor Linux build crashes with SIGILL on this machine before initialize.
CPU 0 affinity allowed diagnostic initialization but is not an approved product workaround.
Task 01 records startup qualification. Task 07 cannot pass the affected platform until the native runtime works without that workaround.
A newer vendor release requires a new reviewed digest and compatibility evidence before the pin changes.

## Technical approach

### Local bundle and installation contract

Add a small `internal/agent/antigravityruntime/` package with `Resolve` and `Install` responsibilities.
Use immutable, version/platform/digest-specific directories under the existing Kandev user cache.
Keep ordinary agent base constructors pure. Local launch orchestration resolves the selected absolute executable before profile wrapping.
Preserve manual complete PATH installs when no managed destination exists.
An invalid managed destination is an actionable error, not a silent PATH or version fallback.

Extend `controller.JobStore` to accept a server-owned Go install action alongside existing script actions.
Retain job status/output/cancel, bounded concurrency, maintenance guards, and cache invalidation.
Leave `InstallScript()` empty and all remote bootstrap behavior unchanged.
Expose additive `supports_local_install` metadata in discovery DTOs and web types.
Render Install from that capability or an existing script, without inventing a shell command for a binary download.
Update `agent-install-catalog.tsx` filtering and `install-agent-card.tsx` gating accordingly.
No SQL migration or release flag is planned; the agent already exists and setup remains an explicit action.

The artifact table is code-reviewed, exact, and platform-specific. Requests cannot choose URLs or arbitrary versions.
Validate compressed/expanded limits, SHA-256, entry whitelist, file types, and containment before atomic publication.
A corrupt existing destination needs explicit repair after its users stop; do not overwrite a live directory.

### Shared local launch and ACP policy

Use one local resolver in lifecycle launch/recovery and hostutility command resolution.
Derive subscription environment sanitation after profile inputs have been combined.
Carry method and interactive permission through existing instance/inference configuration, scoped to local Antigravity.
Only the dedicated operator setup instance permits interactive authentication; retained agentctl versions need capability checks.
Reject conflicting homes instead of creating a task-specific store.
Keep the native credential backend and let the server refresh tokens.
A tiny private agentctl browser helper prevents automatic browser launch without relying on Unix-only shell commands.
Test Python BROWSER quoting on the supported OS families.

Put pure identity, auth-method selection, consent URL, native question, and error parsing in `internal/agentctl/acpcompat/`.
Keep RPC calls, state, stderr ownership, and SQL operations outside pure dialect hooks.
Live and utility paths explicitly authenticate `oauth-personal` before session operations.
If consent is needed during a background operation, return `auth_required` and clean up the temporary listener/process.
Preserve initialize capabilities and auth methods on that failure path.

Registered-agent `acpdbg` uses the same Antigravity identity and runtime resolution for qualification.
Arbitrary `--exec` remains an explicit raw diagnostic escape hatch; its result is not proof of the product's policy.
Add debugger compatibility tests without removing raw frame visibility or its existing commands.
Never enable automatic answers to native `interaction_` requests in qualification.

### Explicit sign-in vertical slice

Use a dedicated hostutility setup instance and existing authenticated agentctl configure/start/initialize/authenticate methods.
Add bounded in-memory attempts and the three operator login endpoints from the design.
Expose native URL progress through a transient `agent.auth.status` response, not a journaled task event or raw stderr API.
Reuse stderr consumer/sanitizer hooks and redact auth status in tracing.
Keep the overall auth deadline at ten minutes without extending every ACP RPC timeout.
Status and cancellation must work while authentication is pending.

The server owns the loopback callback, OAuth state, PKCE, tokens, and persistence.
After consent succeeds, perform a normal fresh capability probe and refresh the model catalog.
Distinguish successful login plus failed catalog retrieval from failed consent.
A reload reattaches to the active attempt. Restart expires its transient state.
Settings uses a focused dialog on desktop and an inset sheet/focused screen on phone.
The phone copies the URL for opening on the host; this package does not redirect a phone callback to the host.

### Native questions

Add an explicit human-input classification before every process-manager auto-approval branch.
Use only the Antigravity tool-call prefix rule and existing pending permission UI.
Retain IDs, labels, cancellation, duplicate-response protection, and process generations.
Normal tool permission policies remain unchanged, including coordinator and managed-MCP restrictions.
Office must wait for the human rather than choose the first allowed option.

### Errors and current-turn outcome

Map provider statuses through existing `routingerr` and ACP error contracts.
Separate auth, entitlement, model access, quota, throttling, and unknown failures.
Only valid evidence supplies reset/retry hints.

For hidden `end_turn` errors, qualify a narrow read-only native journal reader.
Use the existing `github.com/mattn/go-sqlite3` dependency, a per-prompt high-water index, bounded payloads, path validation, and file identity checks.
Read only new records from the active native session. Do not import token history or parse assistant prose.
If required outcome evidence cannot be verified, emit an explicit compatibility failure and withhold success completion.
Keep recoverable prompt errors in the same native process and conversation.

### Usage and visible costs

Preserve existing typed prompt usage and context usage conversion.
Add fixtures proving per-turn normalization, persistence/reload, omitted fields, and no double counting.
Declare subscription default metadata without registering a Google Office quota client.
Use localized guidance in the current conversation usage details for estimated dollars.
Pass `/usage` through native task commands and render its Markdown without synthesizing token usage.

### Compatibility matrix

| Path | Transport and identity | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Local Antigravity task | ACP, `zed` 0.0.0 | Personal OAuth, shared native store, advertised model and resume methods. | Fixtures plus native qualification. Missing consent returns auth-required; crash returns unavailable. |
| Antigravity probe/inference | ACP, same identity | Same resolved server/home/auth policy; no browser popup. | Fake executable integration. Retain initialize data on auth failure. |
| Registered Antigravity acpdbg | Raw ACP capture, same provider identity | Verify product-compatible native handshake and selected model. | Debugger tests plus manual live captures. Raw `--exec` is diagnostic only. |
| Antigravity native question | ACP permission `interaction_` ID | Human response required regardless of auto-approve. | Wire fixture, manager policy tests, desktop/phone E2E. Unknown shapes remain manual permissions. |
| Other ACP providers | Existing identities and advertised auth | Preserve canonical behavior and existing dialects. | Shared adapter/utility and permission regression tests. No Antigravity prefix or auth selection applies. |
| Legacy remote Antigravity | Existing target command and auth metadata | Preserve baseline; no managed host path or new login relay. | Resolver/locality tests. Unsupported setup fails with guidance. |
| Unknown Antigravity journal schema | ACP plus unrecognized private storage | Do not claim verified success for a version known to hide failures. | Bounded outcome-verification error. Never scan other databases or auto-replay. |

## ASCII UI preview

These are illustrative wireframes, not screenshots or pixel specifications.
Structural requirements are action order, clear states, same-machine guidance, responsive composition, and accessible controls.
All displayed copy must use localization. Referenced AC suffixes below use `AC-AGENTS-ANTIGRAVITY-ACP-`.

### UI-01: Agent settings installation

Entry: Settings > Agents > Antigravity. Applies to 009.1, 009.4, 015.1-015.3.

```text
Desktop: existing agent catalog card
+--------------------------------------------------------+
| Antigravity                         ACP not installed   |
| Uses your Google AI subscription after ACP sign-in.     |
| The agy CLI and ACP server have separate logins.        |
| [Install ACP]                                          |
+--------------------------------------------------------+

Phone: stacked card in settings scroll
+------------------------------+
| < Agents                     |
| Antigravity                  |
| ACP not installed            |
| Separate ACP setup required. |
| [        Install ACP       ] |
+------------------------------+
```

Installing replaces the action with status and Cancel through the existing job UI.
Install failure shows the specific cause and Retry. A native startup crash shows installed-but-unavailable with diagnostics.
The phone action is full width and at least 44px high; supporting text wraps.

### UI-02: Personal Google sign-in

Entry: installed card or task auth guidance. Applies to 010.4-010.6 and 015.1-015.3.

```text
Desktop: focused dialog over agent settings
+--------------------------------------------------------+
| Sign in to Antigravity                            Close|
| Use your personal Google AI subscription.              |
| Finish sign-in in a browser on this Kandev machine.    |
| Status: Waiting for Google sign-in                     |
| [Open Google sign-in] [Copy link]                      |
|                                               [Cancel]|
+--------------------------------------------------------+

Phone: focused inset sheet
+------------------------------+
| Sign in to Antigravity       |
| Use your Google subscription.|
|                              |
| Open the copied link on the  |
| machine running Kandev.      |
|                              |
| Waiting for Google sign-in   |
| [         Copy link        ] |
| [          Cancel          ] |
+------------------------------+
```

Before the attempt, show Sign in with Google as the primary action on both sizes.
While starting/checking, announce status and retain Cancel. Disable unavailable actions.
Phone does not offer a supported Open-on-this-phone callback path.
The header and actions remain reachable; only long details scroll within the sheet.
Use `mobile-picker-sheet.tsx`, `useTouchDrawer`, and existing responsive breakpoints as native-phone exemplars.

On success, show Ready and the refreshed model selector in the existing settings flow.
On denial/expiry, clear the URL and show Sign in again.
On successful authentication plus a catalog failure, show Signed in, with Retry model discovery.
Escape/Back dismisses the view with explicit cancellation behavior. Closing cancels the owned active attempt; reload reattaches while it remains active.

### UI-03: Task human question and usage details

Entry: existing task pending permission panel and usage details. Applies to 012.1-012.3 and 014.1-014.4.

```text
Task question, desktop                 Phone, existing question sheet
+--------------------------------+    +------------------------------+
| Antigravity needs your answer  |    | Antigravity needs your answer|
| Which implementation approach? |    | Which approach?              |
| [Option A] [Option B]          |    | [         Option A         ] |
| Waiting for your choice        |    | [         Option B         ] |
+--------------------------------+    +------------------------------+

Usage details on either size
+--------------------------------------------------------+
| Tokens: native values when supplied                    |
| Cost: Estimated, or unavailable without known pricing  |
| Estimate is not an additional Google subscription bill.|
| Use /usage for the native account allowance report.    |
+--------------------------------------------------------+
```

Auto-approve never selects a question option.
Reuse current task surfaces, including phone question presentation; no new question framework or quota dashboard is added.
The usage excerpt shows field meaning, not fabricated example measurements.

## Tests

All proposed test files/methods below are implementation obligations, not files created in this planning turn.
Full AC IDs appear in each work order's frontmatter. Suffixes here use the prefix stated above.
Retired criteria are excluded. Baseline criteria are retained through existing tests and explicit regression extensions.

| AC coverage | Exact test file and method(s), relative to apps/backend unless stated | Owner |
| --- | --- | --- |
| 001.1-001.6, 001.8-001.9 | Existing `internal/agent/agents/antigravity_acp_test.go`: `TestAntigravityACP_IDAndDisplay`, `TestAntigravityACP_LogosNonEmpty`, `TestAntigravityACP_DisplayOrderUnique`, `TestAntigravityACP_NoCLIPassthrough`, `TestAntigravityACP_PermissionAndBillingDefaults`. | 01/06 |
| 002.3, 002.8-002.9, 009.1-009.4 | New `internal/agent/antigravityruntime/install_test.go`: `TestInstallAntigravityBundle`; `resolve_test.go`: `TestResolveAntigravityBundle`. Cover traversal, links, digest, cancellation, partial and corrupt destinations, manual override, platform, and filesystem-only discovery. | 01 |
| 007.1, 007.4, 009.1-009.4 | Extend `internal/agent/settings/controller/agent_install_test.go`: `TestAntigravityLocalInstallJob`; existing `TestAntigravityACP_InstallScriptEmpty`; new `internal/agent/runtime/lifecycle/antigravity_command_test.go`: `TestAntigravityCommandLocality`. | 01 |
| 003.1-003.8, 003.10-003.13, 004.1, 004.3-004.8, 004.10 | Existing `TestAntigravityACP_ArgvIsPlatformSpecific`, `TestAntigravityACP_RuntimeConfig`; new `internal/agent/runtime/lifecycle/antigravity_command_test.go`: `TestAntigravityLocalLaunchPolicy`. | 02 |
| 005.1-005.2, 005.4-005.6, 006.1-006.4, 008.1-008.3, 010.1-010.3, 011.1-011.3 | New `internal/agentctl/server/adapter/transport/acp/antigravity_auth_test.go`: `TestAntigravitySessionAuthentication`, `TestAntigravityBackgroundConsent`; new `internal/agentctl/server/utility/antigravity_probe_test.go`: `TestAntigravityProbeAndInferencePolicy`; new `internal/agent/hostutility/antigravity_launch_test.go`: `TestAntigravityHostLaunchPolicy`. | 02 |
| 005.7-005.9 | Existing `internal/agent/agents/antigravity_acp_test.go`: `TestAntigravityACP_RemoteAuth`. | 01/02 |
| 010.2, 010.6, 011.1 | New `internal/agentctl/acpcompat/antigravity_test.go`: `TestAntigravityClientInfo`, `TestAntigravityConsentURL`; new `cmd/agentctl/antigravity_browser_test.go`: `TestAntigravityBrowserHelper`; new `internal/agent/acpdbg/antigravity_test.go`: `TestAntigravityDebuggerPolicy`. | 02 |
| 010.4-010.6, 015.1-015.3 | New `internal/agent/hostutility/antigravity_login_test.go`: `TestAntigravityLoginAttempt`; new `internal/agent/capabilities/handlers/antigravity_login_test.go`: `TestAntigravityLoginAuthorization`; new `internal/agentctl/server/api/antigravity_auth_test.go`: `TestAntigravityAuthStatusWhilePending`. | 03 |
| 012.1-012.3 | New `internal/agentctl/server/process/antigravity_permission_test.go`: `TestAntigravityHumanInputNeverAutoApproves`; extend `internal/agentctl/server/acp/client_test.go`: `TestAntigravityPermissionOptionsPreserved`. | 04 |
| 012.1 | New `internal/agent/acpdbg/antigravity_test.go`: `TestAntigravityDebuggerHumanQuestion`. A raw capture must not choose the answer. | 04 |
| 013.1-013.3 | New `internal/agent/runtime/routingerr/antigravity_test.go`: `TestAntigravityErrorClassification`; new `internal/agentctl/server/adapter/transport/acp/antigravity_outcome_test.go`: `TestAntigravityCurrentTurnOutcome`, `TestAntigravityOutcomeEvidenceUnavailable`, `TestAntigravityPromptErrorKeepsSession`. | 05 |
| 014.1-014.4 | New `internal/agentctl/server/adapter/transport/acp/antigravity_usage_test.go`: `TestAntigravityUsage`, `TestAntigravityNativeUsageCommand`; extend `internal/backendapp/usage_adapter_test.go`: `TestAntigravityDoesNotRegisterQuotaClient`. | 06 |
| 009.1, 015.1-015.3 | Web `components/settings/agents/agent-install-catalog.test.ts`: local-install capability with empty script; `components/settings/antigravity-login-dialog.test.tsx`: pending/error/success and focus; `lib/api/domains/antigravity-auth-api.test.ts`: attempt lifecycle. | 01/03 |
| 014.1-014.3 | Web `components/task/chat/conversation-usage-display.test.tsx`: `Antigravity subscription cost disclosure`; preserve existing token/cost tests. | 06 |

Use fabricated wire frames, URLs, credentials, native journal databases, and fake executables.
No deterministic test reads the developer's home, calls Google, or starts a real consent flow.
Test omitted usage fields, repeated context updates, resumed turns, a stale previous error, and missing reset information.
For hidden errors, cover symlinks, replaced files, absent/new files, schema mismatch, busy reads, and the current-turn boundary.

## E2E tests

| Flow and AC suffixes | Playwright file and project | Work order |
| --- | --- | --- |
| Native install action with empty script, progress, cancellation, unsupported host, failed readiness; 009.1-009.4, 015.1-015.3 | `e2e/tests/settings/antigravity-setup.spec.ts`, `chromium`; `mobile-antigravity-setup.spec.ts`, `mobile-chrome`. | 01, extended 03 |
| Google start, URL/copy, denial, expiry, cancel, reload reattach, successful model refresh, catalog failure; 010.4-010.6, 011.2, 015.1-015.3 | Same setup specs, desktop and phone. Phone asserts same-machine guidance and no unsupported local phone callback action. | 03 |
| Auto-approve enabled, no response before human selection, exactly one matching option, stop/stale response; 012.1-012.3 | `e2e/tests/chat/antigravity-question.spec.ts`, `chromium`; `mobile-antigravity-question.spec.ts`, `mobile-chrome`. | 04 |
| Typed quota with known/unknown reset, native hidden error, no success completion, persisted recovery detail; 013.1-013.3 | `e2e/tests/chat/antigravity-failure.spec.ts`, `chromium`; `mobile-antigravity-failure.spec.ts`, `mobile-chrome`. | 05 |
| Native usage, absent dollar cost, estimated-price label, `/usage`, reload and context separation; 014.1-014.4 | Extend `e2e/tests/chat/conversation-usage.spec.ts`, `chromium`; add `mobile-antigravity-usage.spec.ts`, `mobile-chrome`. | 06 |

Import repository fixtures, seed only synthetic provider responses, interact through visible UI, and assert reload/backfill contracts.
Arm a live observer before proving that no question response was sent.
Use `.tap()` in phone tests. Assert 44px actions, wrapping, keyboard/focus, and no horizontal overflow.
Keep prior existing scenarios and restore modified shared settings in cleanup.
Run desktop and mobile separately with their owning projects. The managed runner rebuilds the production frontend/backend.

## Work orders

- [x] [Task 01: Install and resolve the official local runtime](task-01-local-runtime.md)
- [x] [Task 02: Unify subscription launch and ACP readiness](task-02-subscription-policy.md)
- [ ] [Task 03: Complete local Google sign-in](task-03-google-sign-in.md)
- [ ] [Task 04: Preserve native human questions](task-04-human-questions.md)
- [ ] [Task 05: Preserve provider and current-turn failures](task-05-provider-failures.md)
- [ ] [Task 06: Report native usage honestly](task-06-native-usage.md)
- [ ] [Task 07: Qualify the native subscription path and document setup](task-07-native-qualification.md)

The graph is `01 -> 02 -> 03`, with 04/05/06 depending on 02, and 07 depending on all preceding work.
Execution remains sequential in the primary session unless the user later authorizes delegation.
Waves describe prerequisites; they do not authorize subagents or persistent Kandev tasks.
Tasks 03-06 have production implementations and deterministic test coverage but remain `in_progress`
until their planned browser E2E specs are added and run.
Task 03 also requires the linked sign-in launch repair described below.

## Verification results

Product implementation: Tasks 01-02 complete; Tasks 03-06 implementation complete with browser E2E still open; Task 07 remains blocked on live native qualification.
The November 4 Settings failure exposed a sign-in configure regression despite the earlier deterministic checks.
The [sign-in launch repair](../antigravity-sign-in-launch/plan.md) owns the full-argv correction and a real configure-boundary regression.
That repair is implemented and the permanent regression passes. Task 03 remains `in_progress`; Task 07 remains `blocked`.
Installer completeness was checked again without changing its contract or establishing native readiness.
Research handshakes and current-code tests do not establish live subscription inference.
After implementation, record every required command and result in its work order and reconcile the final plan status.
Do not mark Task 07 passed while platform or authenticated native evidence is missing.

### Implementation checks

Passed on 2026-11-03:

- Backend build: `go build ./...`.
- Targeted and package tests for `antigravityruntime`, `agents`, settings install jobs, `hostutility`, `acpcompat`, utility, ACP adapter, `routingerr`, process permissions, `acpdbg`, capabilities handlers, `backendapp`, lifecycle, orchestrator, and Office service.
- `bash scripts/lint-go-changed` for every changed Go file: 0 issues.
- Web: targeted Vitest suites, `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
- Browser E2E and live Google consent remain Task 07 gates; no real subscription request was made.

### Review remediation

Addressed the 2026-11-03 review findings without claiming live qualification:

- Utility admission preserves the vetted managed absolute executable and rejects an unvetted absolute path.
- Utility and process launches apply the shared subscription environment policy before spawn, including ambient home validation, credential stripping, harness pinning, and no-op BROWSER injection.
- Utility stderr retains only redacted consent markers; raw URLs and query values never reach logs or returned diagnostics.
- Credential stripping is scoped to the active local personal policy; remote and nil-policy launches retain explicit API-key and ADC inputs.
- Agentctl advertises `antigravity_personal_auth_v1`; task, probe, inference, and sign-in paths require it before using the contract.
- Login attempts fence terminal state until native cleanup completes, clear terminal URLs, and expire safely.
- Resolved server and harness paths travel together through task, probe, inference, sign-in, and registered acpdbg launches.
- Sign-in polling survives transient status failures, clears missing attempts, fences stale responses, and closes only after confirmed cancellation.
- The phone sheet renders one action group.
- Installation checks cancellation through extraction and before publication.
- Browser helper paths are always Python-quoted, including Windows drive letters and backslashes.
- Registered acpdbg resolves the managed runtime, applies the subscription environment, and authenticates before session operations; raw `--exec` remains unchanged.

### Design-package checks

Passed on 2026-11-02:

- `python3 scripts/list-docs.py validate`: 425 decisions and 1885 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` local `validateCoverage` preflight: seven work orders accepted, with no reference errors.
  The real documentation-only diff is exempt. A simulated future Antigravity source change exercised delivery-package coverage and returned `covered`.
- Local traceability check: all 74 active criteria appear in pending work orders; every dependency names an existing work order.
- Local Markdown file links and applicable document-size limits: all ten package files passed.
- `git diff --check -- docs/specs/agents docs/plans/antigravity-local-subscription`: passed.

Only documentation changed. No product implementation, permanent tests, staging, or commits occurred during planning.

## Risks

- ACP 1.4.5 SIGILL blocks this Linux host until a qualified native fix or release exists.
- Native journal schema and `zed` model eligibility are private vendor behavior. Version qualification is mandatory.
- Same-machine loopback requires a browser on the Kandev machine; phone and remote-only consent are outside this version's callback path.
- Cached Google consent does not guarantee subscription entitlement or access to every model.
- Native keychain/file behavior and Python BROWSER quoting require platform-specific checks.
- Main-trajectory token data may omit child agents. Missing dollar prices and quota windows must remain unknown.
- A native journal failure deliberately withholds success rather than letting an unverified turn auto-complete.
- Large downloads need bounded extraction and clear install progress. Do not relax existing global job limits without measured evidence.
