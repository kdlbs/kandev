---
created: 2026-10-11
status: done
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
legacy_specs: []
---

# Implementation Plan: Classify an archived Codex session as missing native state

## Overview

Codex answers `session/load` for a session it has archived with a `-32603
Internal error` whose details name the requested session ("session <id> is
archived. ..."). `isMissingProviderSessionErr` only recognizes Codex's missing
rollout and Auggie's not-found shapes, so the restore policy classifies the
archived response as unknown and blocks the session on every retry without
offering context continuation. AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2 asks
for a typed reason and an explicit continuation action when native restore is
unavailable.

## Scope

### In scope

- Recognize the archived detail as missing native state when it names the
  session Kandev tried to load.
- Record the Codex evidence in the harness session continuity design.

### Out of scope

- Unarchiving the session in Codex.
- A separate restore reason for archived sessions.
- Rendered UI.

## Technical approach

`matchesMissingProviderSession` accepts a `-32603 Internal error` whose details
contain the complete `session <expected id> is archived` statement, optionally
followed by a period and guidance. This shape is limited to `codex-acp`.
The session ID must match, as
for the other shapes, so an archived response for another session stays
unknown. `newRestoreRequiredError` then reports `native_state_missing`, which
allows explicit context continuation; nothing starts a replacement session on
its own.

## Tests

`session_missing_rollout_test.go` covers
`TestIsMissingProviderSessionErrRequiresMatchingStructuredEvidence` and
`session_load_failure_test.go` covers
`TestInitializeSession_LoadFailureDoesNotCreateReplacement`:
AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2. The archived response for the loaded
session is missing native state, both as a typed error and as JSON text; the
same response for another session, or an internal error that names the session
without saying it is archived, is not.

## E2E tests

The lifecycle tests drive a load through a mock agent server. No rendered UI
changes require browser coverage.

## Work orders

- [x] [Task 01: Archived Codex session as missing native state](task-01-archived-codex-session-missing-native-state.md)

## Verification results

The contributor's archived cases passed in hosted Linux lifecycle tests.
Review remediation adds provider isolation and complete-statement matching.
`TestRestoreCoordinatorArchivedEvidencePreservesIdentity` reproduces the
non-Codex and word-lookalike failures before remediation and passes afterward
in a contained narrow harness using exact classifier, policy, and coordinator
source. External ACP, model, telemetry, and routing boundaries are faked;
this proof does not establish real transport, repository, journal, or race
behavior. Full lifecycle and CI validation remains a hosted delivery gate.

## Risks

A user who would rather unarchive the session in Codex is offered context
continuation instead of a blocked restore. Continuation still requires the
user's explicit choice.
