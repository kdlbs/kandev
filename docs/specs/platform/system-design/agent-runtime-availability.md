---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003
---

# Replace standalone agentctl within a running backend

## Ownership and mapping

The platform owns the local runtime connection and replacement coordinator.
Executor ownership, workspace identity, and durable session reconciliation retain their existing owners.
This design extends [durable delivery](durable-agent-delivery.md), not its native harness contract.

| Requirement | Sections |
| --- | --- |
| REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001 | Availability, API and presentation |
| REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002 | Runtime owner, detection, replacement, compatibility |
| REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003 | Session recovery, consumer inventory |

## Current implementation and intended replacement

`backendapp/agentctl.go` creates or adopts a server and writes endpoint, token, and PID into startup config.
`backendapp/agents.go` and `backendapp/main.go` construct standalone and host-utility clients from those values.
`agentctl/availability.go` is currently monotonic within a boot.
`launcher.monitorExit` reports exit, but does not own all consumers needed for safe replacement.
The coordinator replaces this startup-only lifetime model. It must not call whole-application startup a second time.

## Runtime owner

Add a runtime owner in `internal/agent/runtime/agentctl`, wired by backendapp.
It exposes immutable connection leases and a single fenced publication point.
Each lease carries backend boot identity, local runtime epoch, authenticated control identity, endpoint, credential, and cancellation context.
The epoch is monotonic within a backend boot; it is neither a harness generation nor a transport stream sequence.
Credentials stay private and never enter availability snapshots, diagnostic labels, or client-visible responses.

Operations acquire a lease before using a local runtime client.
Replacement closes admission, cancels old leases, and detaches old event subscriptions before preparing successor consumers.
Consumers validate lease identity again before applying asynchronous results to SQL, caches, or session state.
Cancellation cannot undo a request already accepted remotely; such mutations remain uncertain and are never blindly retried.
Do not hold the runtime-owner mutex across HTTP, storage, process shutdown, or callbacks.

Prepare a complete successor binding while public admission remains closed.
Required consumers acknowledge readiness before one atomic publication makes the binding available.
On partial failure, close candidate clients and owned resources; never expose mixed credentials.
Keep the old binding retired. Failed preparation cannot make it available again.
After publication, future calls resolve the current lease rather than rereading mutable config fields.
`cfg.Agent.Standalone*` remains a bootstrap input, not a concurrently mutated connection registry.

## Consumer inventory

Audit direct config readers and transitive clients, not just references to the auth token.

| Owner | Required behavior |
| --- | --- |
| backendapp launcher/adoption wiring | Coordinator owns launcher, renewal, candidate cleanup, and publication |
| lifecycle standalone executor and backend host execution | Acquire current control lease; invalidate old per-instance clients and PID metadata |
| lifecycle streams, completion callbacks, run-owner persistence | Fence old epoch before state mutation; retain durable event identity |
| hostutility.Manager and profile reconciliation | Cancel old calls; rebuild instance/model caches and control clients; no utility prompt replay |
| plugins using host utility, settings/model discovery, provider probes | Follow shared manager binding; expose unavailable results without repeating external operations |
| workspace Git/file/shell APIs, previews, MCP forwarding, metrics/debug helpers | Resolve current execution/lease; invalidate old tunnels and sockets; no hidden mutation retry |
| LSP leases and browser upstreams | Close stale upstreams with existing recovery semantics; recreate lazily after publication |
| ownership renewal and adoption record store | One renewal owner for the published epoch; persist the matching credential reference and endpoint |
| boot state and gateway notifications | Publish ordered sanitized revisioned snapshots |

Do not restart unrelated remote executors. A remote request that depends on local host utility may fail explicitly until that utility recovers.
Do not duplicate plugin processes, periodic workers, subscriptions, or MCP registrations during rebinding.

## Detection and process containment

A launched server's Wait result is authoritative process-exit evidence tied to its epoch.
Intentional Stop suppresses recovery. Duplicate or old exit callbacks are harmless.
An adopted server has no child Wait handle; monitor authenticated control health under its existing ownership lease.
Use existing recovery read timeouts/retries. Network timeout alone is not process-death evidence.
A responsive foreign owner, failed authentication, or unverifiable process identity leaves replacement blocked.
An adopted endpoint can recover its connection without spawning a new process when ownership still matches.

Persist or reuse an owner-scoped process identity record for controlled child cleanup.
Identify installation, execution, runtime epoch, OS process birth identity, and process group/job where supported.
A PID or port alone cannot authorize termination. Revalidate identity at the kill boundary.
Windows keeps the existing suspended-start Job Object path.
Linux retains parent-death handling and verifies remaining owned children; do not assume SIGTERM killed every descendant.
macOS/BSD needs explicit owned-process-group containment after confirmed server death.
If evidence is absent, preserve the process and block the affected session rather than killing an unrelated process.
Never release a journal lock or erase recovery state to bypass a possibly live harness.
A confirmed dead control server can be replaced while an ambiguous prior session remains fenced against journal reuse and dispatch.

Capture bounded sanitized exit diagnostics without blocking stderr draining.
Record exit code and correlation identity in privileged logs, not raw provider payloads in the UI.
This feature does not claim to diagnose the Git symptom in #3962.

## Replacement state machine

States are available, recovering, unavailable, and internal stopping.
A single coordinator serializes signals. Shutdown wins over retry, health results, and publication.

1. Fence local runtime admission and affected sessions; publish recovering with a new snapshot revision.
2. Cancel old request leases, renewal, and dependent streams. Establish ownership and process-death evidence.
3. Contain proven owned children, preserving journals and worktrees. Classify unresolved session ownership individually.
4. Launch a candidate with fresh credentials using existing launcher health/authentication machinery.
5. Prepare required consumer bindings and persist matching adoption/renewal records before publication.
6. Publish the successor binding atomically. Release only runtime-level admission; retain per-session recovery fences.
7. Reconcile affected sessions. Session failures do not retroactively declare a healthy runtime unavailable.

Each outage has a sixty-second recovery window and shares a three-start budget until stable health resets it.
Use waits of one and two seconds before the second and third attempt, bounded by the remaining deadline.
A candidate has at most fifteen seconds for startup, or the shorter remaining episode deadline.
A successor crash within five minutes of availability starts a new outage window but retains the remaining start budget.
Five minutes of healthy availability resets the start budget. Successful publication ends the current outage window.
At exhaustion, publish unavailable. An authorized explicit retry starts a new bounded episode.
Duplicate retry requests join the existing episode; they do not reset its budget.
Initial backend startup uses existing startup failure semantics.

## Session recovery

Fence affected local executions by old runtime epoch under existing admission guards.
Preserve task/session, workspace, harness generation, stream/incarnation, turn, submission, and queue claim identity.
Reuse durable journal/inbox reconciliation from PR #3598. Do not equate a new runtime epoch with a new conversation.
A dead instance's retained journal can be reopened by one recovery owner after exclusive ownership is established.
Recovery may read and project evidence before launching any harness process.
A live surviving instance can be reattached only through supported authenticated ownership transfer.
Otherwise contain proven owned children and retain the session for native recovery.

| Evidence | Outcome |
| --- | --- |
| Retained terminal with preceding output | Project in order and settle once through existing effect/queue authority |
| Idle settled session, valid native state | Keep session resumable; initialize its replacement instance lazily through existing native restore |
| Active work, missing terminal, or unknown external side effects | Persist uncertain outcome; require existing user recovery action; never resend |
| Missing/corrupt/locked journal or ambiguous ownership | Persist typed blocked recovery; retain all evidence |
| True legacy session | Retain legacy uncertainty rules; never invent a v1 completion |

Infrastructure recovery does not automatically issue a model prompt, continue a conversation, execute a tool, or approve a pending permission.
Stop records user intent against the original submission and epoch.
If safe control is impossible, report that the stop could not be confirmed.
Do not turn cancellation intent into a false terminal result.
Office budget/provenance rules, automation guards, queue claims, Send Now, steer, and manual actions remain authoritative.

## Availability, API and presentation

Extend the existing boot and `system.agent_runtime.status_changed` payload with boot_id, revision, runtime_epoch, recovery_id, and retry_allowed.
Public fields contain no token, endpoint secret, OS identity, or provider stderr.
Use bounded reasons such as agentctl_exited, ownership_unverified, start_failed, and recovery_exhausted.
Within one boot, clients accept only increasing revisions; changed boot identity resets comparison through authoritative hydration.
Delayed HTTP responses cannot overwrite a newer WebSocket revision.

Route: POST /api/v1/system/agent-runtime/retry under the existing system admin middleware.
Request contains the observed boot_id, runtime_epoch, revision, and a request_id.
Return the current sanitized snapshot and recovery identifier. Return conflict for a stale boot/epoch.
The operation is asynchronous; disconnection of the HTTP caller does not cancel an accepted recovery episode.
Read capability through the existing runtime snapshot; do not reuse supervisor restart capability as child-recovery authorization.
Full Restart Kandev remains a secondary explicit fallback when supported. It does not run automatically.

Reuse `AgentRuntimeUnavailableAlert`, shared store handlers, and boot hydration.
Recovering shows an in-flow status region; unavailable shows a persistent alert and admin-only Retry agent runtime.
Session-level uncertain notices remain after the global alert clears.
Do not use RestartProgressDialog's changed-boot wait for child recovery: successful child replacement keeps boot_id unchanged.
Desktop keeps compact inline actions. Phone stacks controls with 44px targets, one route scroll owner, and safe-area clearance.
Localize all new text in English, Portuguese, Simplified Chinese, both Traditional Chinese catalogs, and Japanese.

## Persistence, compatibility and release

Extend existing installation/adoption and execution recovery owners if additional process birth identity or epoch fields are needed.
New fields must distinguish unknown legacy evidence from verified identity; zero values cannot authorize killing or dispatch.
Use additive registered migrations with SQLite fresh/reopen/upgrade and PostgreSQL conformance when SQL changes.
After a backend crash during replacement, existing startup ownership checks reconcile candidate and prior records before launching another server.
A candidate is not public before its required owner record commits. Do not log and ignore that failure on recovery paths.
Retain journals and uncertain submissions across rollback. Do not permit concurrent old/new backend owners.
No protocol-version change is required unless implementation changes the wire contract incompatibly; then negotiate explicitly.
An older browser must receive safe unavailable behavior or be required to refresh before using the new recovery action.
Do not ship partial consumer migration. No runtime toggle is introduced; keep the complete package on PR #3598 until validated.

## Observability

The coordinator owns bounded counters agent_runtime_recovery_attempts_total and agent_runtime_recovery_outcomes_total,
plus agent_runtime_recovery_duration_seconds and agent_runtime_recovery_in_progress.
Labels use fixed phase/outcome/reason enums; identities belong only in sanitized structured logs.
Existing agent_delivery_* and agent_restore_* metrics retain their owners.
Document the new families in root AGENTS.md during implementation, with no speculative runtime claims before shipping.

## Related records

- [Decision](../../../decisions/2026-09-27-agentctl-runtime-replacement.md)
- [Implementation package](../../../plans/agentctl-runtime-replacement/plan.md)
- [Existing ownership design](../../executors/system-design/agent-survival-across-restart-01.md)
