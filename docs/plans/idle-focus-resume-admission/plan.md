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

## Verification results

The new test failed on the original code (one agent launched past the ceiling)
and passed after the change. The completed-session replay test launched no
agent before the deferred record kept the focus resume permission and passes
with the change. Existing focus, idle-suspension, and ceiling tests pass;
`TestCeilingDispatchAdmissionSerializesRouteMutation` fails on Windows with and
without this change.

## Risks

A focus resume refused by a full ceiling now waits for capacity like other
automatic starts instead of resuming immediately.
