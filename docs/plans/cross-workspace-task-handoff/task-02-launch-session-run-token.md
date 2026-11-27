---
id: "02-launch-session-run-token"
title: "Scope the Office run token to the launch session"
status: in_progress
wave: 2
depends_on:
  - "01-cross-workspace-task-handoff"
plan: "plan.md"
requirements:
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001
acceptance_criteria:
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.2
system_design:
  - ../../specs/cross-workspace-task-handoff/system-design/handoff-mechanism.md
---
# Task 02: Scope the Office run token to the launch session

## Summary

The first launch of an Office run mints its run token before the task session exists, so the token carries an empty session id.
The handoff route then refuses that run as sessionless, and the forward record cannot name a source session.
Re-mint the run token once the launch session is known, so a normally launched run carries its own session.

## Evidence and cause

- `office/service/scheduler_integration.go`: `prepareAndLaunch` mints the token before launch, and `persistLaunchedSession` stores `runs.session_id` only after a successful launch.
- `office/runtime/context_builder.go`: the run context takes its session id from `run.SessionID` or the payload, both empty on a first launch.
- The only existing re-mint ran for ceiling replay and read the post-launch `run.SessionID`.
- `office/runtime/handoff.go`: an empty source session id is refused with `errHandoffSessionlessRun`.

## In scope

- `apps/backend/internal/office/service/launch_credentials.go` replaces `ceiling_credentials.go`: `RemintLaunchCredentials` binds the token to the given launch session and falls back to `run.SessionID` when none is given.
- `apps/backend/internal/orchestrator/ceiling_seams.go` and `service.go`: the `LaunchCredentialReminter` seam takes the session id.
- `apps/backend/internal/orchestrator/task_operations.go`: `startTask` re-mints for the prepared launch session before it builds the launch options; a re-mint failure fails the launch.
- `apps/backend/internal/orchestrator/ceiling_replay.go`: replay re-mints without a session id and keeps the deferred record when the re-mint fails.
- `apps/backend/internal/backendapp/main.go`: seam wiring rename only.

## Out of scope

- Office JWT signing-key persistence across restarts.
- Changes to handoff authorization, provenance records, or the sessionless refusal itself.
- Non-Office launches, whose environment is returned unchanged.

## Acceptance

- A first-launch Office run token carries the launch session id, so a handoff from that run records it as the source session id.
- A run token with an empty session id is still refused, with no write.

## Tests

| AC suffix | Test file and test |
| --- | --- |
| 001.1 | `apps/backend/internal/office/service/launch_credentials_test.go`: `TestRemintLaunchCredentials_ScopesTokenToLaunchSession` |
| 001.2 | `apps/backend/internal/office/runtime/handoff_authorization_test.go`: `TestActionsHandoff_SessionlessRunRefused` |

## Validation

```bash
cd apps/backend
go test -trimpath ./internal/office/service -run 'TestRemintLaunchCredentials' -count=1
go test -trimpath -race ./internal/orchestrator -run 'TestStartTaskWithEnv_(Remints|RefreshFailure)|TestCeilingReplay_RefreshFailure' -count=1
```

## Review remediation and results

The review adds full mint identity assertions, missing-run and rejected-mint
coverage, concrete created-session dispatch coverage, and refresh-failure
coverage that checks owned session/turn settlement and durable credential
redaction. Deferred replay retains both early and late refresh failures.

The early failure now returns `ceilingReplayFailedResult` with the refresh
cause, matching the current main replay-result contract. Original hosted
backend, E2E build, and desktop logs demonstrated the incompatible bare-enum
return. Those build failures are compiler evidence, not behavioral test RED.

The controlled, exact production-helper proof passed eight synthetic boundary
scenarios using only the standard library inside the approved sandbox. It
does not qualify SQL, JWT signing, runtime, or lifecycle behavior. Selected
Go formatting passed after correcting owned-volume parent directory ownership.

Hosted validation of the updated tests and full suites is pending. No local
SQL or broad backend suite has run. Public documentation needs no change:
CLI syntax, handoff authorization, and signing-key storage remain unchanged.

### Hosted fixture repair

The Office helper identity/error cases and direct orchestrator dispatch/failure
cases passed in hosted backend shard 2. The replay cases failed before reaching
the reminter because their fixture used an absent-record CAS token after
clearing to a still-present empty object. The fixture now uses the repository's
observed predecessor and asserts the successful write and absence of a lost
comparison. A dispatch control shares the same fixture with early/late refusal
cases; direct refusal also observes the actual executor launch count.

This is a fixture setup repair, not evidence of a defect in production refresh
logic. The failed artifact and raw Go JSON are retained externally. Fresh hosted
validation of the repaired replay fixture remains pending.
