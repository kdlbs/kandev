---
id: "03-surface-refusals-and-claim-activity"
title: "Surface refusals and claim activity payload"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SEAT-ASSURANCE-002
acceptance_criteria:
  - AC-OFFICE-SEAT-ASSURANCE-002.4
  - AC-OFFICE-SEAT-ASSURANCE-002.5
  - AC-OFFICE-SEAT-ASSURANCE-002.8
system_design:
  - ../../specs/office/system-design/participant-seat-provenance-assurance-01.md
---

# Task 03: Surface refusals and claim activity payload

Implements `AC-OFFICE-SEAT-ASSURANCE-002.4`, `-002.5` and `-002.8`, pinning
`AC-OFFICE-SEAT-PROVENANCE-005.1`, `-005.7` and `-002.9`. No production change.

Sequenced into wave 2 because it and task 04 both add files to the `dashboard`
package and both touch its shared test helpers.

## Acceptance

### Empty identifier over HTTP (`-002.4` → `-005.1`)

- Post a registration naming an empty `agent_profile_id`. Assert a client error
  (the guard at `dashboard/participants_handlers.go`, in `addParticipant`), and
  that the slate for that task and role is still empty.
- Cover both add endpoints if they are separately routed, since each names its
  own role.

### A role outside reviewer/approver (`-002.5` → `-005.7`)

- This refusal is **not reachable over HTTP**, and that is worth stating rather
  than rediscovering: the routes are role-fixed, each endpoint names its own
  role, and the service exposes only reviewer and approver entry points. There
  is no request that carries a bad role.
- The refusal that exists is the `participantFields[role]` lookup miss inside
  `addOrRemoveParticipant`, the shared body both entry points call.
- **Mechanism: a white-box case**, in the service's own package (`package
  dashboard`) rather than its external test package, calling that shared body
  with a role outside the pair and asserting it fails and writes nothing. Go
  permits both packages in one directory, so this is a **new file**, not a change
  to the existing ones.
- A structural assertion over the role map instead would pass against a body
  that ignored the map. It is the weaker choice; do not take it.
- This calls beneath the HTTP surface but **above** the permission check the
  body performs, so it does not bypass authorization — it exercises the same
  refusal an internal caller would meet.

### Claim activity asserted by content (`-002.8` → `-002.9`)

- The existing assertion in `dashboard/participants_test.go` counts rows with
  `action='task_participant_claimed'` and never inspects the `details` payload,
  so a wrong or empty payload passes.
- Replace the count-only check with a content check: decode `details` and assert
  it names the **task**, the **step**, the **role**, the **displaced agent
  profile** and the **claiming agent profile**, each with its correct value.
- Keep the count assertion as well — exactly one entry, with the right payload.
- `logParticipantClaimActivity` in `dashboard/service_tasks.go` is the writer;
  read the payload shape from it rather than guessing key names.

## Constraints

- No production code changes. A case that cannot be made to pass is evidence
  about the spec (`AC-OFFICE-SEAT-ASSURANCE-002.12`).
- `dashboard/participants_test.go` is at 766 lines against a 800-effective-line
  limit. Measure before appending; the content assertion replaces an existing
  block so it should roughly break even, but the HTTP refusal case may need a new
  file.
- The white-box case must be a new file regardless, because of its package.

## Verification

```bash
cd apps/backend && go test ./internal/office/dashboard/... -run 'Participant|Claim|Role'
cd apps/backend && go test ./internal/office/...
make -C apps/backend lint
```

Demonstrate `-002.8` is decisive: confirm the case fails if the payload is
emptied.

## Files likely touched

- `apps/backend/internal/office/dashboard/participants_test.go`
- `apps/backend/internal/office/dashboard/` — new white-box (`package dashboard`) test file
- `apps/backend/internal/office/dashboard/` — new HTTP refusal test file, if the
  existing file has no room
- this task file

## Inputs

- `docs/specs/office/requirements/participant-seat-provenance.md` — `-005.1`,
  `-005.7`, `-002.9`
- `docs/specs/office/system-design/participant-seat-provenance-assurance-01.md`
  ("The two surface refusals")
- `apps/backend/internal/office/dashboard/service_tasks.go` —
  `addOrRemoveParticipant`, `logParticipantClaimActivity`
- `/tdd`

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, risk tags, exact RED/GREEN verification commands and
results, uncertainties, and this task status set to `done`. Do not edit
`plan.md`.
