---
id: "01-admit-focus-resume-as-automatic"
title: "Admit focus resume as automatic"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.1
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Admit focus resume as automatic

## Summary

Admit a focus-driven idle-suspension resume against the session ceiling as an
automatic launch, so passive focus never uses a manual override.

## In scope

- Add a regression test for focus with a saturated ceiling before the change.
- Change the focus resume origin from manual to automatic.
- Keep the completed-session permission and the idle-suspension check in the
  deferred resume record, so a refused focus resume of a parked completed
  session replays once capacity frees.
- Record the focus origin and its replay in the session ceiling design.

## Out of scope

Explicit Resume, message delivery, ceiling configuration, and UI changes.

## Acceptance

- With the ceiling full, focusing an idle-suspended session launches no agent.
- A deferred focus resume of a parked completed session reaches the executor
  when the sweep replays it with free capacity.
- Existing focus, idle-suspension, and ceiling replay tests pass.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'Ceiling|Seam|Focus|IdleSuspension|IdleSession|Deferred|Replay|AutoResume|SessionOpen|Resume' -count=1)
(cd apps/backend && go vet ./internal/orchestrator)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/orchestrator/idle_session_focus.go
- apps/backend/internal/orchestrator/idle_session_focus_ceiling_test.go
- apps/backend/internal/orchestrator/ceiling_seam4.go
- apps/backend/internal/orchestrator/ceiling_replay.go
- apps/backend/internal/orchestrator/ceiling_entry.go
- apps/backend/internal/orchestrator/ceiling_dispatch_admission.go
- apps/backend/internal/orchestrator/task_operations.go
- docs/specs/agents/system-design/session-concurrency-ceiling.md

## Dependencies

None.

## Risks

A refused focus resume waits for capacity through the existing deferral path.
If the idle policy no longer parks the session when the sweep replays the
record, the record is dropped as superseded instead of retried.

## Parallelism

`sequential`

## Inputs

- Issue #4321.
- REQ-AGENTS-SESSION-CEILING-001 and its automatic and manual admission criteria.
- The linked session ceiling design.

## Historical contributor results

- RED: the new test failed on the original code with one agent launched past
  the ceiling.
- GREEN: the new test and the existing focus and idle-suspension tests pass.
- RED: the completed-session replay test launched no agent before the deferred
  record kept the focus resume permission; it passes with the change.
- The contributor baseline reported Windows dispatch-serialization failures;
  those are historical results, not the integrated candidate verification.
- `go vet` and `golangci-lint` on the changed package report no issues.

## Maintainer integration and verification

Integrated protected main, preserving immutable retry payload ownership,
route-before-lifecycle lock order, early recovery admission, and safe Resume
cleanup projection. The idle-focus queue bypass now validates the service,
held claim, exact task and session, resume kind, automatic origin and idle
provenance option. It renews the matching durable claim and checks route
admission; renewal rejects a different persisted owner as well as a replaced
or expired lease. Final provider admission remains in place.

Added package regressions for missing, foreign, released, expired, replaced,
and wrong-kind claims, route changes, absence of a manual override, and dropping
a queued completed focus resume after idle provenance is lost. The completed
replay keeps both resume permissions.

A contained stdlib harness using the production dispatch, renewal and deferral
parser/equivalence functions reproduced the task-only authorization gap and
passed all 12 cases after remediation with `go test -trimpath -race -count=1`.
Its repository CAS and route validation are explicit fake boundaries. It is
focused proof only; full real package/caller verification is delegated to the
exact-head hosted Backend, Frontend and E2E suites. Local full orchestrator and
lifecycle rebuilds are intentionally excluded by the contributor containment
scope. Full hosted results are retained in the external delivery receipt.

Desktop and phone use the same session-focus launch service and backend
admission. This change does not alter rendered composition, navigation, touch
controls or copy; existing session-resumption unit tests and mobile workflow
session-focus E2E cover those entry points. Public behavior is clarified in
`docs/public/agents-and-profiles.md` (explanation).

Lease-error regression: admission renewal errors retain their retryable identity
instead of becoming idle-provenance loss. Expired, replaced and foreign-owner
claims leave the deferred resume queued without launching; only actual loss of
idle suspension returns the superseded-provenance outcome. An extracted-source
shortcut regression failed with nonretryable provenance loss before this fix
and passes under the race detector afterward. Real replay/settlement package
regressions cover all three lease-loss variants in hosted Backend CI.

Hosted real-package verification found one fixture error in the lost-provenance
regression: `UpsertExecutorRunning` intentionally preserves a suspension marker,
so it cannot clear that marker for the test. The fixture now uses the guarded
`CompareAndSetExecutorRunningIdleSuspension` transition over the exact execution
and observed row timestamp, then asserts readback before replay. No production
budget or assertion is relaxed. Completed replay, exact claim/lease retention,
route serialization, cancellation and immutable retry regressions passed in
that run. The full corrected exact-head suites remain the final delivery gate.
The unrelated protected terminal-environment fix is included in this same
necessary fixture-remediation integration batch.
