---
status: current
system: agents
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
---

# Capacity feedback in chat

On the capacityQueue branch of message creation and the later
queuePromptIfCapacityRefused fallback, stamp the accepted queue entry with a
server-owned admission reason of `session_capacity`. Persist it with the queue
write. Project that reason on visible entries in the existing session queue
snapshot and status updates. Missing or unknown reasons produce ordinary queue
presentation. Transcript identity and launch queue status each serve other
purposes and cannot establish why a chat message entered the queue.

In the shared chat queue surface, derive one inline notice from the selected
session's visible entries. Use historical wording because the admission reason
describes the send attempt. Show “Message queued because the session limit was
reached.” With Auto-run enabled, add “It will send automatically when capacity
is available” for an otherwise eligible queue. If another delivery barrier is
known, including a paused or detached managed conversation, omit this promise. With Auto-run off, add “Turn on Auto-run to send it
automatically.” Preserve the existing queue controls and transcript identity.

Place the notice above the queue chip in the composer region. Use the shared
disk-space warning surface and row so the background, border, corner radius,
padding and icon color match. Center the clock vertically against the full
text block, including when the message wraps. While the warning is visible, read the effective settings from the
session capacity settings API. Use the enforced maximum, including environment
overrides. A disabled limit or failed read must never display a numeric limit
from saved settings or an earlier response. Wrap on phones. Announce
the change once through a polite status region and keep keyboard focus in place.
Use i18next for every locale. Reconcile from the normal queue snapshot after
reconnect, removal and dispatch. Preserve the marker on edits and restore after
a refused dispatch. A merge that retains capacity-deferred input must retain
the reason while the combined entry is pending.
