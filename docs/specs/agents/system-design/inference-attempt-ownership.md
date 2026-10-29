---
title: Inference child owner and settlement
system: agents
status: current
created: 2026-10-15
owners: [backend]
requirements:
  - REQ-AGENTS-INFERENCE-OWNERSHIP-001
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

Overall/grade admission, utility/review consumer wiring and shared reservations
remain the next increment. This owner seam supplies their population and release
evidence and does not claim to complete the original shared-cap requirement.
