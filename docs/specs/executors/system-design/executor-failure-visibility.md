---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-FAILURE-VISIBILITY-001
---

# Executor failure visibility design

## Boundary and current evidence

Executors owns physical-resource observations. Runtime collects them; task services
own durable episode admission, session settlement, history, and authorized readback.
Use narrow runtime interfaces, not new lifecycle imports from task consumers.

Source baseline: `ff917a370`. The supplied incident is a Failed/Evicted Pod whose
`docker-data` EmptyDir exceeded 12Gi; both containers exited 137 and the workspace
PVC stayed Bound. These are supplied observations, not a fresh cluster inspection.

Confirmed source gaps:

- `executor_kubernetes_status.go` starts with Pod reason/message, then
  `kubernetesContainerProjection` overwrites them with container termination data.
  It omits exit code, termination times, and last termination evidence.
- `executor_kubernetes_reconnect.go:reconnect` verifies identities and PVC, then
  attempts control attachment without a terminal-Pod preflight. Shared attachment
  can execute credential preparation in `executor_kubernetes_shared_control.go`.
- `manager_remote_status.go` refreshes before inspection and returns on refresh
  failure. Its regular loop visits only `executionStore`; startup records receive
  a one-time poll. Results live only in `remoteStatusBySession`.
- `manager_events.go:handleStreamDisconnectWithAttempt` immediately marks the
  execution failed and publishes a generic AgentctlError. This loses the distinction
  between transport uncertainty and resource death.
- `orchestrator/service.go:reconcileActiveSessionOnStartup` preserves conversation
  identity and settles non-surviving work to waiting, but does not retain a cause.

This extends the executor contract, not the active task-error scope contract.
Reuse [error scope/history](../../../decisions/2026-09-14-error-scope-and-history.md)
and [active recovery ownership](../../../decisions/2026-09-20-active-session-recovery-owner.md).
Their ownership decisions remain unchanged. The storage rationale below fits this
feature design; no separate architecture decision is needed.

## Kubernetes failure and recovery scenarios

Reported operational evidence motivates these portable regression scenarios. It
was not independently reproduced against production resources in this worktree.

- A worker-node reboot followed by Pod SandboxChanged can erase memory-backed
  bootstrap state while a disk-backed preparation marker survives. Missing
  bootstrap credentials can cause startup exit 2 and repeated CrashLoopBackOff
  while Pod phase remains Running and a Docker sidecar recovers. Sessions may
  retain STARTING with an empty error or a misleading duplicate-execution error.
- Stale cleanup can time out or return 401 after worker credentials change. Keep
  those secondary blockers separate from the primary executor failure.
- Authorized operator repair can restore the same Pod, PVC and workspace while
  the provider has lost its original conversation. An existing missing-rollout
  fallback may create a fresh conversation. Lost global Git configuration can
  independently prevent repository use after a worker restart.
- Legacy readiness-timeout cleanup during Resume or Continue can delete both a
  managed Pod and PVC. Rebuilding an existing Kandev session from published commits
  or recorded edits can restore availability and retain Kandev history without
  restoring original files, unpushed work or the provider conversation.
- An agent may emit end_turn without an operation identity while its session
  remains RUNNING until cancellation. This is adjacent completion-correlation
  evidence, not an executor-failure cause or a change to completion matching.

Keep these outcomes independent: coarse session state, an empty error and a
successful recovery RPC do not establish continuity. This task does not repair
credentials, change Git configuration, change legacy cleanup policy, or repeat
production repairs.

The source's `session.go:isSessionLoadFallbackErr` and
`isMissingProviderSessionErr` corroborate that explicit missing-rollout evidence
for the requested provider identity permits an existing fresh-session fallback.
Keep that policy intact; carry its actual outcome rather than inferring continuity
from the recovery RPC result or coarse session state.

## Observations and classification

Add a typed optional runtime observation capability alongside `RemoteStatusProvider`.
Keep existing RemoteStatus compatibility. The new result has a closed outcome:
`healthy`, `terminated`, `missing`, `restarted`, or `unknown`; transport availability
is separate. Include task/environment generation and exact resource identity
internally, observation time, optional source time, provider, safe reason code,
and bounded evidence. Unknown fields stay absent, never synthesized as zero.

For Kubernetes, inspect the recorded Pod UID and ownership before any refresh or
control exec. Preserve Pod phase/reason/message separately from current and last
termination for workload containers, including name, exit code, signal, reason,
start/finish times, restart count and container identity. Bound to eight configured
containers and a combined 4096-byte safe diagnostic budget. Pod eviction dominates
summary selection but does not erase container evidence. Preserve the verified
EmptyDir name and capacity in the supplied example; never infer OOM from 137.
Observe init-container evidence when it prevents readiness. A failed optional
sidecar is evidence of degradation, not proof every agent execution died.
Conversely, Pod Running and a healthy Docker sidecar do not imply that the required
agent container is usable. Preserve readiness and Waiting/CrashLoopBackOff together
with LastTermination exit 2 and restart evidence. Report required-container
unavailability; infer lost execution only from matching process/container evidence,
not readiness=false alone. Repeated crash-loop restarts enrich the same unresolved
episode and update its latest restart baseline; they do not emit 126 notifications.

Correlated Pod SandboxChanged or node Rebooted evidence may enrich an episode with
its source/time when available through existing authorized diagnostics. Node-event
access is optional: do not require cluster-wide RBAC, infer reboot from SandboxChanged
alone, or postpone container-failure reporting while waiting for it. Source messages
about missing bootstrap files become a safe `runtime_bootstrap_missing` explanation;
keep paths in internal design evidence, not user-facing persisted diagnostics.
A retained preparation marker is not proof volatile credentials or global Git
configuration survived. Repair/preparation and credential re-handshake remain with
the existing lifecycle/compatibility owners, not the read-only observer.

Compare restart count/container identity only within the recorded Pod UID and
workload generation. Persisted initial inventory is the baseline; absence of a
baseline means unknown restart history, not a new restart. Expected maintenance
uses exact operation/resource identity from the existing operation owner. User
stop/shutdown intent is captured before disconnect and is separate from event locks.
Established-session inspection suppresses disconnect callbacks during intentional
stop or idle suspension. Queued classification revalidates that intent, and
reconnection shares the instance lifecycle fence with teardown. A failed stop
releases suppression so subsequent observation and explicit recovery remain
available; a successful stop cannot reconnect its deliberately closed streams.
Missing credentials, RBAC denial, timeout, identity mismatch, and missing resource
are distinct; only verified NotFound for the recorded identity means missing.
No watch implementation is required: reuse GET inspection and the existing loop.
A future watch must feed the same idempotent admission path.

## Detection and reconciliation

Inspect before refresh and after a stream loss; refresh failure must not suppress
resource inspection. Do network I/O outside prompt/session locks, using immutable
execution, prompt, startup-attempt, resource and ownership-generation snapshots.
Revalidate at admission. Never read a mutable successor identity in a callback.

Use a bounded 15-second reconnect grace for transport-only loss, with individual
inspection calls capped at five seconds. An authoritative terminal observation
bypasses grace. Healthy same-generation reattachment preserves the existing prompt;
it does not send it again. If grace expires without authoritative evidence, expose
connection/status unavailable, block new prompt dispatch while liveness is unknown,
and retry read-only inspection. Do not retire possibly live execution capacity.
If independent process evidence proves execution loss, settle as an interruption
with unknown cause. Keep existing startup cancellation/timeout ownership intact. The managed callback
currently signals prompt completion before classification; defer that signal as
well as AgentctlError for established sessions. Otherwise the prompt waiter can
still fail a successfully reconnected turn. Preserve generation fencing, transcript
buffer ownership and passthrough behavior; startup before ACP initialization keeps
its existing bounded startup owner. Never hold its startup lease across inspection.

Extend the existing one-minute runtime reconciliation loop with paged durable
inventory for unarchived task environments plus legacy session-owned records.
Deduplicate by physical resource and generation, inspect with bounded concurrency
(maximum eight) and cancellation, and reuse one observation for attached sessions.
Include retained idle environments with zero tracked executions. Exclude completed
cleanup/deleted environments; intentional parked compute is not a failure. Startup
starts the same bounded read-only pass inside the background reconciliation loop
without delaying readiness; periodic reconciliation catches
remaining rows. Fetch current connection config while preserving recorded workload
identity. No Pod/PVC, worker, or environment creation occurs in this pass.

## Persistence and admission

Add `executor_failure_episodes` through the existing SQLite/PostgreSQL-compatible
repository migration path. Store episode ID, task and optional environment/session
owner, ownership generation, opaque resource key, failure fingerprint, revision,
first/last observation times, optional occurrence time, sanitized typed evidence,
resolution time and resolution kind. The episode fingerprint combines physical
identity/generation and termination/restart identity; never observation time or
message text. Unknown transport state is not a terminal episode. Permit one active
episode per resource generation; richer evidence updates its revision, not its ID.
A later failure after resolution opens a new episode. Duplicate insertion is fenced
in the database, not just in memory. Retain episodes until task deletion; no raw
provider object or credentials enter this table.

Persist an affected-session reference set using normalized rows keyed by episode
and session incarnation/execution. This supports retrying partial settlement after
a crash and multiple sessions without an unbounded JSON array. Historical detached
rows and new successor attachments are not affected. Session-only executors use
session/execution scope; task-owned Pod/container losses use environment scope even
with zero or one session. Resource authority, not failed-session count, chooses scope.

Persist episode admission, deterministic historical message IDs, and pending
settlement references in one transaction. Then apply existing conditional session
updates per affected execution/turn. Recheck liveness, stop/archive intent, resource
generation, and successor turn evidence at the write boundary. Retry pending work
idempotently after restart. Never hold a database transaction across runtime I/O.
Publish invalidation after durable writes; summary rebuild can recover a missed
publication. Do not overwrite `last_launch_error` or `last_agent_error` belonging
to a different failure. The episode table is needed because retained environments
can have no sessions and session error metadata can clear on recovery; a toast,
RemoteStatus cache, or overwritable launch-error slot cannot retain this evidence.

For confirmed lost active execution, use existing guarded abandonment semantics:
settle to recoverable WAITING_FOR_INPUT, preserve conversation/runtime inventory,
retire exactly the dead execution/capacity, and keep new admission blocked until
existing recovery authority allows it. Idle sessions stay idle with visible resource
failure; stopped/terminal sessions are not resurrected. Shared failure must not mark
unaffected live siblings failed. No `turn.completed`, workflow success, agent-error
auto-retry, synthetic completion signal, or queued-prompt consumption is permitted.
Generic stream errors for the same observation must not create a second failure.

## Projection, recovery, and compatibility

Add optional versioned `executor_failure` to TaskStatusSummary, distinct from
`task_error`, with episode ID/revision, scope, safe summary/evidence, affected-session
references, workspace/conversation facts, and server-derived actions. Carry it
through boot, HTTP/WS reads, persisted summary rebuild, equality and replacement
updates. Invalidate on episode admission/change/resolution. Missing optional fields
in old payloads are not authority to clear a newer revision. Explicit resolved
revision clears it. Unknown future reason codes show a localized generic summary.
Do not backfill invented incidents from old error strings or interrupted_at.

Shared and session-only episodes use the established composer recovery location.
Task ownership stays shared in persistence; presenting it in an attached session
must not scope the incident to that session. Session-only episodes render only
for their matching session. A task without a session renders the same card at the
bottom of the workbench. Task launch errors remain independently owned by
`TaskSharedError`. Inline expandable details contain workspace/conversation facts
and readable bounded technical rows, without an executor modal or raw JSON dump.
One cause paragraph and one guidance paragraph precede read-only recheck. Reuse
SessionRecoveryCard geometry, bounded scroll and phone/coarse-pointer targets.

Recovery history is a prominent bordered status card. A fresh or unverified
provider conversation uses warning styling; confirmed restored continuity uses
success styling. History never keeps active failure controls after resolution.
Navigation/card failure decorations retain existing permission and terminal-state
precedence. The composer and durable transcript provide the return-to-task path.

Expose an authorized read-only recheck through the task recovery controller:
require task ownership, current episode/revision, and environment generation.
Recheck inspects only, never resumes, resets or upgrades. Only advertise existing
Resume when a fresh observation and the existing recovery admission prove it valid;
its request repeats those guards. A Failed/Evicted Pod offers recheck and operator
repair guidance, not an automatic replacement or a futile exec-based retry. Existing
missing-Pod recovery remains governed by exact PVC/identity/admission rules, outside
this observer. A provider timeout retains historical cause and reports current
uncertainty. Failed recovery does not overwrite the original cause with a wrapper.

Passive workspace attachment reads the active incident for the selected environment
owner and generation, or the exact legacy session. Repeat that admission check after
workspace identity is revalidated. An active incident or unavailable incident read
refuses cold and cached workspace access before runtime creation or registration.
Shared attached sessions use their environment scope; unrelated legacy sessions,
replacement generations, and resolved incidents retain ordinary admission. Files
and Terminal requests do not grant recovery authority or bypass this gate.

Session-open and focus recovery use the same current environment/generation or
legacy session scope in automatic-resume eligibility. Recheck this eligibility
at launch admission, so a failure recorded after the initial status read still
blocks passive resume before executor startup. An unavailable failure read blocks
automatic recovery with an ownership-unavailable disposition. Explicit user
recovery remains governed by its existing authorization and recovery checks.

Carry a typed primary observation plus ordered, bounded secondary operation causes
through stale cleanup and recovery. Preserve existing `errors.Is`/`errors.As`
control-flow semantics for underlying operation failures while projecting the
executor cause first. Keep the existing duplicate sentinel for verified live agents.
Confirmed loss or unverified liveness must return typed executor unavailability
before duplicate cleanup can trigger an automatic relaunch. Cleanup timeout and
cleanup 401 remain separate bounded context. Do not
flatten an error chain and select only its outermost message. A cleanup failure does
not prove liveness or authorize deleting inventory/secrets, automatic retries, or
bypassing cleanup/admission. Session STARTING with an empty legacy ErrorMessage must
still display the admitted episode. Add no auth re-handshake or Git repair mechanism
here; report those blockers safely and consume outcomes from their existing owners.

Verified Bound matching PVC means workspace retained as of that observation, not
verified file completeness. Historical volume evidence does not establish original workspace
continuity after resource deletion or replacement; reconstructed files and retained
Kandev history do not establish it either. Keep the original observation time
distinct from recovery time, and require separate evidence for current continuity.
Provider home/conversation retention is independently unknown until its owner
verifies recovery. Resolving environment availability
retires only the shared resource controls; affected sessions may still need native
conversation recovery and retain their separate recovery states. Healthy agentctl
alone cannot resolve those. Correlated successful recovery clears matching active
controls and leaves history. STARTING, focus, or a new prompt do not clear evidence.

Persist the recovery outcome on the affected session/episode reference, fenced by
attempt and generation: executor `available|unavailable|unknown`, workspace
`retained|unavailable|unknown`, and provider conversation
`restored|fresh|unavailable|unknown`. Record outcome time and safe reason code.
Use the provider load/create result boundary to distinguish restored from fresh;
never derive either from WAITING_FOR_INPUT, an empty error, unchanged Kandev session
ID, or a successful Resume response. Original/replacement provider identifiers remain
internal correlation, not public details. Legacy success without evidence is unknown.

An authorized fresh-conversation fallback resolves the environment error when healthy,
but appends/updates an idempotent session recovery-result marker: workspace retained,
original provider conversation unavailable, new conversation started. The marker is
hydrated from durable history after reload even with no active error. Surface one
non-blocking localized result notice for that attempt; do not keep stale failure
controls, disable the working composer, or repeat toasts on reload. Preserve the
original failure entry and Kandev transcript. A late attempt cannot replace a newer
outcome. Existing provider fallback policy and prompt-dispatch rules are unchanged.
Do not add a reset shortcut. Existing Reset Environment remains explicitly destructive
and must describe its Pod/managed-PVC/conversation effects through current confirmation.

## Desktop, phone, and safe copy

Reuse `task-shared-error.tsx` dialog/drawer and `session-error-details.tsx` bounded
copy support. Phone entry is the shared strip above tabs in the dedicated mobile
layout; `mobile/mobile-picker-sheet.tsx` supplies inset drawer geometry. Hierarchy:
cause, observed time, affected environment/sessions, retained workspace and separate
conversation status, then actions/details. One drawer body owns scrolling, with
100dvh bounds, safe-area clearance, focus return and at least 44px phone/coarse-pointer
targets; desktop controls stay 28px. Shared view-model and action eligibility serve
both viewports. Long diagnostics wrap without horizontal page overflow.

Persist only sanitized fields via the existing routing/error sanitizers, extended
with tests for URLs, tokens, environment assignments, paths and secret references.
Raw Kubernetes messages are untrusted. Use closed localized reason codes and safe
parameters for summaries; show only bounded redacted source text in details. Apply
defense-in-depth at readback/copy, too. Add en, pt-pt, zh-cn, zh-hk, zh-tw, ja strings,
using the zh-Hant generator and pseudo checks; no new literal UI copy.

## Compatibility and verification

| Provider | Evidence and transport | Coverage and fallback |
| --- | --- | --- |
| Kubernetes | API GET exact Pod/PVC UID; loopback agentctl | Full Pod/container/restart evidence, shared/legacy inventory, fake client and isolated Kind |
| Docker / remote Docker | Existing daemon inspect and recorded container ID; local/SSH transport | State, exit, OOM flag and restart only when inspect proves them; transport failure stays unknown |
| Local / Worktree | Recorded controller process identity/generation and exit callback | Report proven controller loss; backend reboot alone retains interrupted/unknown cause, no guessed exit |
| SSH | Recorded host/process identity and current transport | Proven process exit only; disconnected host is unknown |
| Sprites / plugin remote | Existing supported status capability | Normalize supported terminal facts; otherwise unknown, no invented Kubernetes details or new plugin protocol |

A terminal local agent row clears its process handle without implying shared
controller loss. The same owned terminal execution may use the attached local
controller identity for inspection before explicit stale-agent cleanup. The
recorded runtime, task/execution identity, attachment timestamp and controller
availability must still match. Live or rotated rows cannot use this fallback;
unknown or terminated controller evidence continues to block relaunch.

No new runtime flag or provider retry authority. The incident observer is read-only;
all existing recovery fences remain. Recheck open PR #3849 admission changes and the
worker-compatibility sibling before implementation; planned maintenance intent must
use its eventual operation identity, not reason-string heuristics.

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-FAILURE-VISIBILITY-001 | Observations and classification; Detection and reconciliation; Persistence and admission; Projection, recovery, and compatibility; Desktop, phone, and safe copy |

Tests and exact commands are in the [plan](../../../plans/executor-failure-visibility/plan.md).
Log accepted observations and resolution with episode/resource correlation only
after sanitization. Do not add high-cardinality metric labels or raw provider payloads.

## Responsive API and unavailable worker

A responsive Kubernetes API can retain Running phase and stale container Ready
values after worker connectivity is lost. Explicit Pod Ready=False/Unknown must
prevent a healthy projection. Pod readiness reasons NodeNotReady or
NodeStatusUnknown, or a true DisruptionTarget/DeletionByTaintManager condition,
provide worker-unavailability evidence without requiring node-level RBAC. Preserve
a bounded allowlist of these conditions, their reason/status and transition times.
Readiness failure without worker evidence stays generic uncertainty.

Cached remote status becomes unknown immediately on explicit worker evidence;
the managed-disconnect path waits its existing grace before durable admission.
Stable worker-unavailability evidence creates a durable uncertain episode, including
active, idle, shared and legacy owners. It never establishes process death, settles
a turn, releases execution registration, retries or moves an environment. Recheck
may resolve only through healthy evidence for the same owned resource. Generic
API uncertainty creates no new incident and cannot erase a prior confirmed cause.
Cleanup and duplicate-launch paths preserve worker uncertainty with bounded
secondary blockers rather than claiming an agent is alive.

A matching Bound PVC proves retained volume inventory at the observation time.
It proves neither accessible data nor integrity. A volume tied to an unavailable
worker is not safely recoverable on another node merely because it is Bound. The
shared desktop dialog and phone drawer show connection uncertainty, connectivity
repair guidance and separate workspace-access uncertainty. No hardware or OOM
cause is inferred, and no node/event permission becomes mandatory.

Inspection covers typed local, Docker and Kubernetes observations. SSH, Sprites
and plugin runtimes retain their existing disconnect owner until they supply
physical evidence. A deleting Pod retains stopping semantics unless explicit
worker-unavailability evidence makes liveness unknown. Docker not-found is scoped
to the recorded container identity; other daemon errors remain unknown. Retained
remote Docker inventory can open a temporary read-only inspection connection,
without provisioning, registering an execution or resuming an agent. Each page
bounds individual reads and separately bounds persistence so a slow target cannot
starve later inventory. The composer selects its surface from the same accepted
HTTP recheck result as its card, independent of WebSocket delivery.

When disconnect ownership cannot be read, retain an explicitly unverified status and retry only the existing agent streams. Do not admit resource-loss evidence without authority, infer process death, replay a prompt, or provision an executor.
