---
id: "01-retained-evidence"
title: "Inspect retained reconstruction evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.7
system_design:
  - ../../specs/platform/system-design/durable-agent-record-recovery.md
---

# Task 01: Inspect retained reconstruction evidence

## Summary

Return bounded, authenticated evidence for a missing backend submission.
Support an existing instance and an exclusively owned retained local journal.
This work order does not write canonical records or change prompt admission.

## In scope

- Integrate the current PR #4380 dependency and reconcile its specification
  additions with this package. Record the resulting base and head.
- Add a typed runtime inspection contract with complete candidate summaries,
  exact owner identity, and optional process proof.
- Extend the existing retained control path without requiring a canonical
  submission ID before discovery. Preserve file ownership and lock checks.
- Fetch only the uniquely selected payload through a backend-only operation.
  Validate its size, hash, envelope, and descriptor association.
- Cover live-instance inspection, absent-instance inspection, fully acknowledged
  streams, completed history, conflicting candidates, and obsolete runtime leases.

## Out of scope

SQL import, browser controls, prompt dispatch, file takeover, journal migration,
remote redial, and new provider resume behavior.

## Acceptance

1. A real owner journal with one unresolved record returns complete, verified
   evidence without an instance launch, prompt, acknowledgment, or journal mutation.
2. Truncation, multiple unresolved candidates, foreign generations, changed
   payloads, locks, and obsolete leases return bounded failure results.
3. Payloads stay inside the authenticated backend/runtime boundary. Old peers
   remain blocked. Missing historical process proof remains explicitly absent.

## Verification

Run from the repository root after writing the proposed failing regressions:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'Test(RetainedReconstructionEvidence|DeliveryRecordEvidenceOwnerFence)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -count=1)
git diff --check
```

Create the named tests in each changed boundary where applicable. Verify that
the focused run executes the new tests, rather than passing with no matches.
Use deterministic barriers for lease replacement and descriptor/payload races.
Include a candidate that matches the current owner beside a conflicting
unresolved candidate. Do not rely only on all-valid or all-invalid fixtures.

## Files likely touched

- `apps/backend/internal/agentctl/journal/recovery.go`
- `apps/backend/internal/agentctl/journal/retained.go` (PR #4380)
- `apps/backend/internal/agentctl/journal/reconstruction_evidence_test.go` (new)
- `apps/backend/internal/agentctl/server/api/retained_delivery.go` (PR #4380)
- `apps/backend/internal/agentctl/server/api/retained_reconstruction_test.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/client_delivery.go`
- `apps/backend/internal/agent/runtime/agentctl/control_delivery.go` (PR #4380)
- `apps/backend/internal/agent/runtime/agentctl/control_reconstruction_test.go` (new)
- `apps/backend/internal/agent/runtime/delivery_recovery.go` (PR #4380)
- `apps/backend/internal/agent/runtime/lifecycle/delivery_record_evidence.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/delivery_record_evidence_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption.go`

Keep public runtime interfaces and facade forwarding synchronized if the new
inspection operation requires a method on the existing backend seam.

## Dependencies

PR #4380's authenticated retained read and shutdown guards. No earlier work
order in this package. Refresh the dependency before implementation.

## Risks

A missing instance is not proof of process death. The retained read must not
open a second writer or weaken an active instance's ownership.

## Parallelism

`sequential`

## Inputs

- [Design: authenticated evidence inspection](../../specs/platform/system-design/durable-agent-record-recovery.md#authenticated-evidence-inspection).
- [Design: candidate and evidence rules](../../specs/platform/system-design/durable-agent-record-recovery.md#candidate-and-evidence-rules).
- Existing `RecoveryDescriptor`, `restoreRecoveredSubmission`,
  `peerSubmissionsForRecovery`, and PR #4380's `ReadRetainedRecovery` tests.

## Results

Implemented the typed runtime evidence and selected-payload operations across the
journal, authenticated control API/client, lifecycle, and public runtime facade.
Retained inspection opens an existing journal read-only while its shared bbolt
lock excludes an active writer. Candidate summaries remain bounded at 16, include
the stream identity, and include all unresolved session generations. Payload
fetch requires the sole exact candidate, validates the 1 MiB bound, hash, strict
prompt envelope, and descriptor snapshot before and after the read. Missing
process proof remains absent, and a live successor is reported separately.

The dependency foundation from PR #4380 at
`5d50992ee303001a607832bb746a22b77c23cf2a` is included with these changes.
The branch also incorporates main through
`d92d40b67bf3ecabf74072c95325f3f86cdd5e30`, including the existing Quick Chat
Resume behavior. Published acceptance IDs from main were retained; the
unmerged dependency's conflicting IDs were moved to 006.11 through 006.14.

Verification passed:

- Focused and full race tests for journal, server API, agentctl client, lifecycle,
  and runtime facade. Full four-package race suite completed successfully.
- `git diff --check` and `git diff --cached --check`.

Regressions cover authenticated absent-instance reads, live-instance inspection,
fully acknowledged output, completed history, mixed-generation ambiguity,
truncation, stale payload/hash/envelope evidence, active journal locks, missing
files, old peers, unknown process proof, and a retired runtime lease.
