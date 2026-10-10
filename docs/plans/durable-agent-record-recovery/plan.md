---
created: 2026-10-09
status: done
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
system_design:
  - ../../specs/platform/system-design/durable-agent-record-recovery.md
legacy_specs: []
---

# Implementation plan: recover missing delivery records

## Overview

Recover a verified journal submission whose backend association is missing.
Then use the existing reconciliation and explicit continuation operations.
Implement inspection first, atomic reconstruction second, and visible recovery
with browser evidence third. All three work orders and local review remediation
are complete. Remote CI and automated review remain part of PR delivery.

The user requested implementation of this package after a read-only incident
investigation. Keep implementation sequential and update each work-order result.

## Evidence and dependency baseline

Local source baseline: `0d91baa7a899d460955e3db15f330e7c637294fd`.
Dependency: [PR #4380](https://github.com/kdlbs/kandev/pull/4380), open at
`5d50992ee303001a607832bb746a22b77c23cf2a` when inspected on 2026-10-09.
Implementation base: `02ff0578357040b0546ea17cd9a00dff9ca9ee3b`. The reviewed PR
head is stacked in the worktree without a merge commit; its dependency edits are
staged and this package's implementation remains unstaged.

The earlier read-only inspection established:

- Task `b1668267-0008-4561-9596-9404e36789fd`, session
  `83f9e1e2-0111-4b87-a642-a7618570f858`.
- Native conversation `01a1210a-9426-7322-9b5b-c13b72852138` resumed successfully.
- At 22:38:28 UTC, the new user message was saved. Admission then created an
  `unresolved_durable_work` block and returned the session to `WAITING_FOR_INPUT`.
- The retained submission `prompt:502ebcb8-95e6-44f4-9ad2-65e0ef98dbdf` remained
  `dispatching`. Its stream had high-water and acknowledged cursors of 6622.
- The session had no backend submission rows or delivery-recovery metadata.
  Its open delivery block had no submission binding.

These are observations from the captured inspection, not assertions about a
later live database. The cause of the original missing SQL record remains
unknown. This package repairs verified missing associations without assuming
why they disappeared. Do not use the user's live session as a test fixture.

PR #4380's `RetrySessionDelivery` returns `missing_canonical_submission` before
inspection when that metadata is absent. Its regression
`TestRetrySessionDeliveryMissingCanonicalSubmissionStaysVisible` preserves the
block. The PR also prevents future missing initial records, but does not import
this older non-initial submission. Native load alone leaves its journal guard
active. This is the confirmed recovery gap.

Before implementation, resolve the current dependency and main heads. Integrate
PR #4380 after merge, or explicitly stack the branch on its reviewed head.
Preserve published specification IDs. If unmerged additions collide with new
main criteria, assign the unmerged additions unused IDs and update their links.
If its implementation changes this gap, revise these work orders first.

## Scope

### In scope

- Bounded authenticated inspection of one session's retained submission.
- Unique evidence selection, immutable payload validation, and conflict refusal.
- Atomic reconstruction, provenance, block binding, and persistent presentation.
- State-only Retry followed by the existing explicit continuation flow.
- SQLite/PostgreSQL races, restart regressions, desktop/phone browser evidence,
  and recovery guidance in public documentation.

### Out of scope

- Automatic resend, tool replay, fabricated terminal outcomes, journal deletion,
  broad block clearing, or native conversation replacement.
- A journal-wide startup migration, manual production repair, deployment,
  runtime flags, remote redial, or a new background recovery scheduler.
- Proving that all historical sessions are recoverable. Missing ownership or
  process evidence remains a visible block.

## Technical approach

1. Extend the authenticated inspection seams from PR #4380. Keep candidate
   selection complete and bounded. Read the selected payload only inside the
   backend/runtime boundary. Do not weaken public descriptors.
2. Add `ReconstructAgentDeliverySubmission` as a dedicated conditional repository
   operation. Persist the submission, recovery provenance/projection, and exact
   block binding atomically. Integrate it into `RetrySessionDelivery` before the
   missing-record refusal. Keep unresolved admission closed.
3. Project an unbound delivery block into the existing recovery card. Preserve
   the rejected message's blocked status and use the committed recovery revision
   after repair. Reuse PR #4380's continuation form and idempotency contract.

The [design](../../specs/platform/system-design/durable-agent-record-recovery.md)
defines evidence, incomplete process identity, state mapping, and trust rules.
The [ADR](../../decisions/2026-10-09-retained-delivery-record-reconstruction.md)
records the limited authority to reconstruct delivery associations.

### Compatibility matrix

| Runtime or evidence | Intended behavior | Required evidence |
| --- | --- | --- |
| Compatible local live peer | Inspect through its authenticated instance client, without file takeover | Real journal/client tests and no-prompt counters |
| Local absent instance | Read its retained journal through authenticated control and exclusive ownership | Owner-marker, lock, missing-file, and replacement tests |
| Resumed idle instance with an older unresolved prompt | Separate current observation from historical prompt identity | Same-generation replacement regression |
| Missing historical process proof | Repair proven records, keep continuation blocked | Explicit incomplete-identity test |
| Remote or older peer without equivalent inspection | Typed blocked result, preserved data | Capability-negative tests |
| Multiple unresolved candidates or conflicting SQL | No import, no newest-record selection | Mixed completed/unresolved and foreign-owner tests |
| Codex or another native-resume provider | Same conversation only after existing eligibility checks | Mock native-ID evidence. Real provider verification remains separate |

## ASCII UI preview

UI-01: Existing chat recovery card, above the composer. Copy is illustrative
and must use localization. The current screenshot shows a saved message and
`0s` without an explanatory delivery result.

```text
Desktop, blocked:
  Message saved. Delivery is blocked.
  The previous instruction needs recovery.
  [Retry connection]  [Stop]

Desktop, after verified reconstruction and termination:
  Recovery records restored. The previous outcome is unknown.
  Next instruction: [                                      ]
  [ ] I understand that previous work may be incomplete.
  [Resume session]    [Stop]

Phone, blocked:                 Phone, eligible continuation:
  Message saved.                 Recovery records restored.
  Delivery is blocked.           Previous outcome unknown.
  [Retry connection]             Next instruction:
  [Stop]                         [                       ]
                                 [ ] Acknowledge uncertainty
                                 [Resume session]
                                 [Stop]
```

While Retry runs, show an accessible progress status without changing its
accessible name. If evidence is insufficient, keep the cause and safe actions
visible. Do not show Resume until the server authorizes it.

Required structure: one chat scroll owner, aligned desktop controls, stacked
phone controls, keyboard access, and phone/coarse-pointer targets of at least
44px. The phone keyboard must not hide the form's primary action.
Use the existing safe-area layout. Spacing in this preview is illustrative.
This view covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5 and 008.6.

## Tests

All names below are proposed regressions. They must exist and fail for the
specified defect before implementation can claim a passing result.

| Acceptance criteria | Regression file and test |
| --- | --- |
| 008.1, 008.2, 008.7 | `journal/reconstruction_evidence_test.go`: `TestRetainedReconstructionEvidence` |
| 008.2, 008.7 | `lifecycle/delivery_record_evidence_test.go`: `TestDeliveryRecordEvidenceOwnerFence` |
| 008.1, 008.3 | `sqlite/agent_delivery_reconstruction_test.go`: `TestReconstructAgentDeliverySubmission` |
| 008.2, 008.3 | `sqlite/agent_delivery_reconstruction_postgres_test.go`: `TestPostgresReconstructAgentDeliverySubmission` |
| 003.2, 006.4, 008.1-008.4 | `orchestrator/session_delivery_reconstruction_test.go`: `TestRetrySessionDeliveryReconstructsMissingSubmission` |
| 006.6, 008.5 | `orchestrator/session_delivery_reconstruction_test.go`: `TestMissingDeliveryRecordNoticeSurvivesReload` |
| 008.4, 008.6 | `orchestrator/reconstructed_continuation_test.go`: `TestReconstructedDeliveryContinuesOnlyExplicitly` |
| 008.5-008.7 | Existing session recovery service, hook, and presentation suites, extended for incomplete identity |

The principal RED fixture contains a real journal with one dispatching
non-initial submission, completed history, a fully acknowledged stream, and
zero canonical submission rows. SQL retains the session, generation, message,
and verified stream association. An unbound delivery block exists.
Retry currently returns `missing_canonical_submission`. The desired result
restores one association and reports uncertainty without a provider call.

Add separate fixtures for missing process proof, an idle successor instance,
two unresolved candidates, another independent block, and a newer generation.
Do not fabricate process proof in the incident-shaped fixture to claim that all
historical sessions can continue.

## E2E tests

Task 03 owns `tests/session/durable-record-recovery.spec.ts` (Chromium) and
`tests/session/mobile-durable-record-recovery.spec.ts` (mobile-chrome).
Use a disposable real SQL/journal fixture and the real recovery route.

Prove missing-record notice, blocked saved instruction, zero prompts after
Retry, one reconstructed row after duplicate Retry/restart, and one explicitly
acknowledged continuation. Preserve native ID and transcript count.
Also prove ambiguous evidence remains blocked after reload and that a late
reply cannot update a different session. Map these flows to 008.1-008.7.

The browser fixtures intentionally omit historical process proof, so these
two missing-record scenarios must remain blocked after repair. Eligible explicit
continuation is verified by the companion desktop and phone
`agent-runtime-replacement` specs, which assert one accepted new instruction
and preserve the native conversation ID.

## Work orders

- [x] [Task 01: Inspect retained reconstruction evidence](task-01-retained-evidence.md)
- [x] [Task 02: Reconstruct missing delivery associations](task-02-atomic-reconstruction.md)
- [x] [Task 03: Present and verify recovery](task-03-visible-recovery.md)

Execute 01, then 02, then 03. No delegation is authorized.

## Companion package inventory

| Package | Relationship |
| --- | --- |
| PR #4380: `agentctl-journal-shutdown-recovery` | Dependency. Its Task 02 lacks this compatibility import. Keep its existing missing-evidence negative test |
| `durable-agent-reattachment` | Existing recovery/adoption guards and UI contract. Additive follow-up link only |
| `durable-agent-session-reconciliation` | Existing stream/submission identity and projection rules. No repeated implementation |
| `durable-agent-sessions` | Original storage and continuity boundaries remain authoritative |
| `durable-agent-stream-repair` | Historical ACK/stream evidence does not validate this missing-record repair |
| `agentctl-runtime-replacement` | Runtime loss proof remains independent of record reconstruction |

Preserve prior statuses and verification counts. This package supplies new
results for its own scenarios. After dependency integration, add reciprocal
links to its plan and Task 02, without rewriting completed results.

## Verification results

Design checks passed on 2026-10-09:

- Document validation: 369 decisions and 1497 specifications validated.
- Specification lint: all files passed. Validator unit tests: 36 passed.
- Coverage preflight: all three work orders passed `validateCoverage` with a
  synthetic production trigger to exercise their complete reference chains.
- Relative Markdown file links and `git diff --check`: passed.

Task 01 implementation and verification passed on 2026-10-10. Its exact results
are recorded in [Task 01](task-01-retained-evidence.md). Task 02 implementation
and verification passed on 2026-10-10; its exact results are recorded in
[Task 02](task-02-atomic-reconstruction.md). Task 03 implementation and
verification passed on 2026-10-10; its exact results are recorded in
[Task 03](task-03-visible-recovery.md). The Task 03 desktop/phone recovery
browser checks and eligible-continuation companions passed. A rendered phone
capture was inspected after scrolling the shared chat surface to the actions.

Final review remediation passed on 2026-10-10: all eight backend packages under
race, PostgreSQL concurrent reconstruction/enrichment and continuation checks,
SQLite/PostgreSQL store conformance, SQL guard, Go lint, 132 frontend tests,
typecheck, focused ESLint, and six desktop/phone browser checks. Eight fresh
captures were inspected and compressed. Local implementation is complete;
PR #4403 is published. Review remediation adds zero-output reconstruction,
non-nested client leases, and lost-reply checkpoint recovery. Focused backend,
frontend, and all six browser checks passed again. Remote CI and review
thread disposition remain delivery work.

The conflict-fixup round preserves main's published native-resume contract and
restores its existing Resume controls for canonical interrupted work. Sixteen
focused desktop/phone browser checks, four backend race-test packages, 54 frontend
tests, typecheck, focused web lint, and documentation validators passed. Remote
CI and review remain pending for the next pushed head.

## Risks

- PR #4380 can change before implementation. Refresh its source and contracts.
- Older records can lack process or turn evidence. Never fill gaps with the
  current execution, newest turn, or generation-one defaults.
- Two databases cannot share one transaction. Revalidate journal evidence and
  runtime ownership, then condition the SQL write on the observed owner.
- Payload access crosses a sensitive boundary. Keep it bounded, authenticated,
  and absent from UI responses and logs.
- PostgreSQL needs real concurrent connections. A skipped fixture is not a pass.
- This package does not establish the original cause of the missing row.
