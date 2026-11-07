---
title: Inference child owner and settlement
system: agents
status: current
created: 2026-10-15
owners: [backend]
requirements:
  - REQ-AGENTS-INFERENCE-OWNERSHIP-001
  - REQ-AGENTS-INFERENCE-OWNERSHIP-002
---

# Inference child owner and settlement

The existing per-instance utility handler owns an attempt registry. A context
carrier joins its entry to the actual ACP/Codex spawn, prompt dispatch and common
cleanup seam. The server first prepares an opaque token; only a registered,
unconsumed token owned by that live handler may start a child. Consumption and
prepared-only cancellation are serialized by the same owner mutex. Unknown,
retired, canceled and other-owner tokens cannot create a new child.
Server configuration supplies task/session/instance identity. A separate call
UUID is correlation only. Probes have no attempt carrier.

The process manager's existing DeliveryJournal stores a bounded inference-owner
metadata value in its existing meta bucket, using the journal's lock, disk
reservation and close lifecycle. There is no additional database or ledger.
Write spawn intent before Start and PID immediately afterward. Storage failure
marks continuity unknown while preserving native execution and cleanup.

Windows already starts suspended, attaches a kill-on-close Job and resumes.
Settlement uses the existing Job's TerminateAndWait, not tasklist or HTTP status.
Linux uses Setpgid and leader-only Pdeathsig; macOS uses Setpgid without a parent
death signal. Unix restart reads retained group identities: a missing group
proves termination, a remaining group is unresolved, and an intent without a PID
is unknown. Recovery never signals a possibly reused PID/group. It only observes.
The boundary is the existing owned process group/Job, not arbitrary processes
which deliberately escape native containment.

Snapshot known describes inventory continuity. The existing journal's atomic
fresh initialization establishes a new owner's empty inventory. A reconnect to
the same live owner retains that knowledge. An agentctl owner restart, missing
prior metadata or journal failure is unknown, even when the list is empty:
another owner may have executed while storage was unavailable, leaving no intent.
Retained individual attempts can still have definite settlement. New owner
identity and retained old-owner entries
prevent a reconnecting controller from replacing unknown prior population with
zero. No background polling is added; snapshots reconcile retained unresolved
entries on demand. Retention uses the existing journal submission count and
encoded-record byte bounds. It prunes settled entries and current-owner,
unconsumed prepared tokens, preserving unresolved/active entries. A missing token
always rejects execution, so eviction never permits replay. A prepared token has
not spawned a child and must not be counted as an active inference process;
an unretained identifier is unknown, never an invented terminal result.
Existing BeginOwnedOperation admission links HTTP inference to manager teardown:
cleanup and its final storage write precede release; the journal closes after
that existing owner drain. A drain timeout remains an error, not empty population.

Host utility creates workspace-only instances without a requested ID or journal.
The native instance manager records its own UUID issuance in a server-only
configuration field, never an external creation/configuration DTO. Such an
intentionally volatile fresh instance has a known current-owner inventory,
including zero, for that live instance's lifetime. Explicit-ID recovery and a
configured-but-failed journal do not receive this exception. A new instance or
owner ID never settles another old owner's unknown attempts.

POST /api/v1/inference/attempts prepares a server token. Its preparation_id is a
client-generated correlation UUID, allowing cancellation even if the token
response is lost. POST /attempts/:id/cancel settles only an unconsumed prepared
token; POST /inference/preparations/:id/cancel also fences a late preparation.
Neither operation settles or kills a running child. POST /inference/prompt
consumes its explicit attempt_id once. Omitting it preserves legacy behavior by
preparing and consuming a new token in that request. PromptResponse.attempt
reports observation after cleanup. GET /inference/attempts and /attempts/:id
use the existing authenticated instance API group and instance guard. Identifiers
and statuses contain no execution content or process descriptors.

The existing InferencePrompt call retains its native request flow. The new
InferencePromptOwned helper prepares before sending, preserves token/preparation
identifiers in InferenceAttemptError, and performs prepared-only cleanup after
cancellation or transport failure with an independent bounded context. The
NoPromptRequestSent field distinguishes failed preparation from ambiguous prompt
transport. PrepareInferenceAttempt and the two cancellation primitives are also
available to the subsequent capacity owner: it must persist the server token's
reservation association before sending an inference request. The convenience
helper is not itself a durable capacity policy. A 404 from old agentctl allows
only legacy compatibility; missing observation/settlement remains unknown and
must not authorize an all-policy admission or release another owner's slot.

The shared capacity owner joins these attempts with task launches. In finite
`all` policy the backend stores each exact binding (owner, instance, attempt,
preparation, grade and model identity) through the existing settings-backed
session-capacity store before sending the prompt. The reservation is admitted
under the same controller mutex that reads the task and inference population, so
existing ownership keeps its slot across settings changes and a missing or
unknown population refuses new admission.

A binding enters the counted population with its stable key. The deferred
settlement order is terminal observation, provider settlement, binding removal,
then retained-token acknowledgement. Worker leases keep a settled-but-not-yet
released binding counted; refresh never invents a zero from a failed snapshot.
The gated current inventory is separate from retained historical continuity. A
single unreadable execution makes the population unknown for new admission while
held bindings remain in the store and stay counted; it never erases the known
bindings or turns them into a zero.

Host utility and lifecycle inference callers route through the shared consumer.
Teardown forwards the owner's stop observation through the retained workspace
stop path, so only a proven no-start or observed owned-process termination
settles an attempt. Recovery of a still-tracked run owner uses the normal stop
path and removes its census client only after terminal persistence. Population
diagnostics use closed source/reason codes and bounded identifiers, and never
become an admission decision or a release of another owner's slot.

## Current inference population and historical continuity

Finite `all` capacity includes native inference reservations and children in the
same total and grade limits as task launches. Agentctl's attempt snapshot keeps
historical `known` separate from versioned `current` evidence. Missing historical
owner metadata alone does not make an otherwise complete current registry unknown.
Concrete unknown children, unavailable owner storage, missing bindings or failed
provider settlement still block admission; an empty list alone proves nothing.

The authenticated control inventory applies the reservation requirement under the
instance-publication lock to existing and future instances. Token consumption uses
the same owner lock as this mode change. The backend prepares a retained token,
persists its exact capacity binding, validates provider settlement authority, then
admits that token before starting the child. Admission permission is process-local.
A tokenless direct start either precedes the gate and appears in the inventory or
is rejected. Runtime leases fence replaced control servers and malformed lists
fail closed. Finite `all` mode requires the new control API and gated current
snapshots; unsupported runtimes cannot certify a free seat. Optional capacity modes
can observe the current registry without requiring this gate.

After real teardown, an owner closes only when every concrete attempt is known,
settled and acknowledged. A same-session successor still needs matching
incarnation/generation and the existing journal owner CAS. Settled, acknowledged
predecessor entries stay in history but leave the current snapshot. Historical
`known` remains false throughout. Terminal observation, provider settlement,
binding removal and retained-token ACK retain their existing order. This changes
no SQL schema and adds no background poller; backend and agentctl must be updated
together for the new gated inventory path.

Upgrade compatibility also covers the old `unproven` phase when historical
continuity is false and the retained attempt map contains no entries. The
exclusive journal, matching session/incarnation/generation and owner CAS remain
required. This transfers no attempt or reservation and leaves historical
continuity false. New process admission durably marks even legacy owners active;
active predecessors, any retained attempt entry, unknown children, identity
conflicts and storage failures do not qualify for this compatibility case.

A successor handoff is not this compatibility case: it requires matching
session/incarnation/generation and journal exclusivity, always records
historical continuity as false, and refuses a stale active claim with no
retained attempt. An active claim with retained attempts may only settle or reap
them as unproven history.
