---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office Enqueue Consolidation Requirements

## Overview

Four separate code paths insert an Office run row today. A gate attached to any one
of them is bypassable by the other three, so every launch-safety control depends on
there being exactly one seam to attach to. That seam does not exist yet.

This is stated as its own requirement rather than as a clause of a limit, because it
is a different kind of work with a different blast radius. The limits are new logic at
one call site. This is a boundary change: it re-wires the dashboard reactivity
adapter, the approval adapter, and the workflow engine's `queue_run` step action, and
it collapses three independent copies of the deduplication window into one. It is
therefore a **delivery prerequisite** for
[Office Unattended Launch Safety](unattended-launch-safety.md),
[Office Launch Budgets](launch-budgets.md) and
[Office Launch Backpressure](launch-backpressure.md), and it is sized and reviewed on
its own terms rather than arriving as the last bullet of a requirement about counting
depth.

Office owns this contract because Office owns the run row.

## Terminology

- **Enqueue path:** any code path that results in a new run row existing.
- **Authoritative enqueue API:** the single server-side entry point through which
  every wake reason must reach the queue.
- **Delegating caller:** an enqueue path that resolves its own arguments and then
  calls the authoritative API, holding no insert of its own.

## Requirements

### REQ-OFFICE-ENQUEUE-CONSOLIDATION-001: One authoritative enqueue seam

**Intent:** Give every launch-safety gate exactly one place to attach. Today the
deduplication window is declared three times over, which is the visible symptom of
three independent insert paths that have already drifted; a safety gate written once
against one of them would be silently absent from the others.

**User story:** As an operator relying on launch limits, I want every wake to reach
the queue through one gated entry point, so that a limit I configure cannot be
bypassed by whichever subsystem happened to queue the run.

#### Acceptance criteria

- **AC-OFFICE-ENQUEUE-CONSOLIDATION-001.1:** Exactly one server-side enqueue API
  shall be authoritative, and every wake reason shall reach the queue through it.
- **AC-OFFICE-ENQUEUE-CONSOLIDATION-001.2:** Every other enqueue path shall be a
  delegating caller. A run row inserted by any path other than the authoritative API
  is a defect.
- **AC-OFFICE-ENQUEUE-CONSOLIDATION-001.3:** The system shall have a test that fails
  when a second insert path exists, so that a new one cannot be added silently. The
  test shall assert against the run-row insert itself, not against a list of known
  callers, because a list of callers is what fell behind in the first place.
- **AC-OFFICE-ENQUEUE-CONSOLIDATION-001.4:** The deduplication window shall be
  declared exactly once. Two declarations of the same window are a defect, whatever
  their values.
- **AC-OFFICE-ENQUEUE-CONSOLIDATION-001.5:** Consolidation shall not change the
  observable behaviour of any existing wake reason: for every reason, the run row
  produced before and after shall carry the same agent profile, reason, payload,
  idempotency key, and coalescing outcome. This requirement moves where a row is
  inserted, not what is inserted.
- **AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6:** When the authoritative API is
  unavailable to a delegating caller, that caller shall fail its enqueue and surface
  the error, rather than falling back to an insert of its own. A fallback insert is
  precisely the ungated path this requirement removes.
- **AC-OFFICE-ENQUEUE-CONSOLIDATION-001.7:** The authoritative API shall accept the
  declared enqueue fields the sibling documents require, including the typed actor of
  AC-OFFICE-RUN-CAUSATION-001.15, the causing run, and the routine, so that no caller
  needs to smuggle them through a payload document. The typed actor shall be a
  **required** field, so that a caller cannot omit it and silently receive a default: a
  path that supplies no actor is a defect, not a path electing the system actor.
- **AC-OFFICE-ENQUEUE-CONSOLIDATION-001.8:** The workspace shall **not** be an accepted
  field. It is derived server-side from the woken agent's profile by
  AC-OFFICE-RUN-CAUSATION-001.20, and accepting it from a caller would let any enqueue
  path aim a workspace-scoped ceiling or budget at a workspace the woken agent does not
  belong to. The same holds for every causation value
  AC-OFFICE-RUN-CAUSATION-001.17 reserves to the server: the API shall accept the
  inputs from which those values are derived, never the values themselves.

## Out of scope

- **The gates themselves.** What is checked at the seam is owned by
  [Office Unattended Launch Safety](unattended-launch-safety.md) and
  [Office Launch Budgets](launch-budgets.md). This document delivers the seam.
- **The causation identity carried through it.** Owned by
  [Office Run Causation Chain](run-causation-chain.md).
- **Claim-side behaviour.** Nothing here changes how a queued run is selected or
  claimed.
- **Retiring the coalescing or idempotency semantics.** Consolidation preserves both
  as they are; changing them is separate work.
- **The enqueue call sites' own business logic.** Reactivity, approval flow and the
  workflow engine keep deciding *whether* to wake; they stop deciding *how* the row
  is written.
