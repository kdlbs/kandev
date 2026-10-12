---
created: 2026-10-10
status: done
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
legacy_specs: []
---

# Implementation Plan: Admit focus-driven idle resume as automatic

## Overview

Task focus resumes a session that workspace ACP idle suspension parked.
`resumeFocusedIdleSuspension` passed a manual launch origin, so a full session
ceiling admitted each focus as a manual override. Every focused session added
one session above the limit and recorded a manual-override notice without an
explicit user action. Opening a conversation already uses the automatic origin.
Issue #4321 reports the gap.

## Scope

### In scope

- Pass the automatic origin for a focus-driven idle-suspension resume.
- Cover the saturated-ceiling focus path with a regression test.
- Keep the completed-session permission and the idle-suspension check in the
  deferred resume record, so a parked completed session replays once capacity
  frees.
- Record the focus origin and its replay in the session ceiling design.

### Out of scope

- Explicit Resume and message delivery, which keep their manual admission.
- Changes to ceiling limits, other deferral kinds, or rendered UI.

## Technical approach

`idle_session_focus.go` passes `launchOriginAutomatic` to
`ResumeTaskSessionWithOptions`. A full ceiling now refuses the resume through the
existing automatic deferral path instead of granting a manual override.

A focus resume of a completed session needs the completed-session permission,
which the deferred resume record did not keep, and the sweep treated a completed
destination as superseded. The resume record now stores both resume options,
the replay restores them, and a completed destination stays valid only for a
record that carries both. The idle-suspension check on replay accepts the
session's own queued launch when the sweep's dispatch claim owns it, and a replay
that finds the session no longer parked drops the record as superseded.

## Tests

`idle_session_focus_ceiling_test.go` covers
`TestFocusTaskSessionIdleSuspensionResumeRespectsCeiling`:
AC-AGENTS-SESSION-CEILING-001.1. The ceiling is set to one and held by an
unrelated launch; focusing an idle-suspended session must not launch an agent.
`TestFocusTaskSessionDeferredCompletedResumeReplaysAfterCapacityFrees` defers a
focus resume of a parked completed session, frees the slot, and runs one sweep;
the replay must reach the executor.

## E2E tests

The service test uses the real repository, executor, and ceiling controller.
Only the agent manager is simulated. No rendered UI changes require browser coverage.

## Work orders

- [x] [Task 01: Admit focus resume as automatic](task-01-admit-focus-resume-as-automatic.md)

## Historical contributor verification results

The new test failed on the original code (one agent launched past the ceiling)
and passed after the change. The completed-session replay test launched no
agent before the deferred record kept the focus resume permission and passes
with the change. Existing focus, idle-suspension, and ceiling tests passed on that contributor
baseline. Its reported Windows dispatch-serialization failures predate the
maintainer current-main integration and are not current verification results.

## Risks

A focus resume refused by a full ceiling now waits for capacity like other
automatic starts instead of resuming immediately.

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
