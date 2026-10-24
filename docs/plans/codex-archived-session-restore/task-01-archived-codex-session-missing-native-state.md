---
id: "01-archived-codex-session-missing-native-state"
title: "Archived Codex session as missing native state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 01: Archived Codex session as missing native state

## Summary

Classify Codex's archived-session load failure for the requested session as
missing native state, so the restore offers context continuation instead of
blocking as unknown.

## In scope

- Add classifier and load-failure cases for the archived response before the
  change.
- Match the archived detail when it names the expected session.
- Limit the archived shape to Codex and reject word lookalikes.
- Record the Codex evidence in the harness session continuity design.

## Out of scope

Unarchiving in Codex, a dedicated restore reason, and UI changes.

## Acceptance

- An archived response for the loaded session is classified as
  `native_state_missing`, as a typed error and as JSON text.
- An archived response from a non-Codex provider, or an archived-word
  lookalike, stays unknown and preserves the native identity.
- The same response for another session, or an internal error that names the
  session without saying it is archived, stays unknown.
- Existing restore classification tests pass.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && go vet -trimpath ./internal/agent/runtime/lifecycle)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/agent/runtime/lifecycle/session.go
- apps/backend/internal/agent/runtime/lifecycle/session_missing_rollout_test.go
- apps/backend/internal/agent/runtime/lifecycle/session_load_failure_test.go
- docs/specs/agents/system-design/harness-session-continuity.md

## Dependencies

None.

## Risks

A user who would rather unarchive the session in Codex is offered context
continuation instead of a blocked restore.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001 and
  AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2.
- The linked harness session continuity design.

## Results

- RED: on the original classifier, the archived cases were not recognized and
  the load failure was classified as unknown.
- GREEN: the new cases and the existing restore classification tests pass.
- Review remediation: the coordinator regression fails for custom, Auggie,
  and unknown providers and for word lookalikes before the fix. It passes
  after the fix in a contained exact-source narrow harness, together with
  existing classifier and coordinator tests. Boundary fakes do not prove
  repository, transport, journal, caller, or race behavior. Hosted CI owns
  full real-package verification; no broad local package build was run.
