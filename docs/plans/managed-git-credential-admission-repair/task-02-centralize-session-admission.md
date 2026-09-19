---
id: "02-centralize-session-admission"
title: "Centralize session credential admission"
status: done
wave: 2
depends_on: ["01-harden-repository-identity"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 02: Centralize Session Credential Admission

## Acceptance

- A task-policy resolver error stops admission and reaches the caller.
- Admission resolves the effective executor profile and skips broker identity validation when that
  profile configures `GITHUB_TOKEN` or `GH_TOKEN`.
- Normal and Office paths run admission before any session create, rebind, resume, or state change.
- Workflow target-profile admission runs before exit hooks and the persisted step transition.
- A failed workflow admission preserves the source step and current session and does not deliver
  the destination prompt.
- The admission operation is read-only and does not reveal an executor-profile secret.

## TDD Sequence

1. Add a policy-error case that records session storage calls and proves the current preflight
   ignores the error. Add a remote-profile `gh_cli_env` secret-reference case.
2. Add fresh-session and idle-session Office cases. Record RED results for creation, profile
   rebind, or state change before a managed identity failure.
3. Extend the workflow profile-preflight test to assert source-step persistence, current-session
   ownership, exit-hook behavior, and destination prompt routing. Record the current partial
   transition as RED.
4. Introduce one read-only admission boundary that selects policy, profile override, and persisted
   repository identity. Call it before normal and Office mutations.
5. Resolve the destination profile and effective executor profile before workflow exit processing.
   Run admission, then apply the transition and switch the session only after success.
6. Run the focused executor and orchestrator tests and record GREEN results.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor -run 'Test.*(PreflightManagedGitCredentials|PrepareSession|EnsureSessionForAgent)' -count=1
cd apps/backend && go test ./internal/orchestrator -run 'Test.*Workflow.*(Profile|Credential|Preflight|Transition)' -count=1
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_office.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_preflight_test.go`
- focused Office executor tests under `apps/backend/internal/orchestrator/executor/`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_preflight_test.go`
- this task file and `plan.md`

## Dependencies

- Task 01 must establish the common persisted repository identity contract.

## Parallelism

Sequential. The session and workflow paths must use the same admission operation and effective
profile selection.

## Inputs

- The atomic admission scenarios in the GitHub authentication specification.
- `executor.PreflightManagedGitCredentials` and executor profile environment metadata.
- `PrepareSession` and `EnsureSessionForAgentWithCreation`.
- The workflow transition and `switchSessionForStep` paths.

## Output Contract

Report each RED mutation, the final admission call sites, how profile overrides are detected
without secret access, the GREEN commands, files changed, and remaining risks. Update this task and
the parent plan with the recorded results.

## Recorded Results

- RED: a policy resolver error returned success and persisted a session.
- RED: a configured remote-profile GitHub token still failed managed repository identity
  admission.
- RED: fresh and idle Office paths created, rebound, or resumed a session before rejection.
- RED: workflow rejection occurred after the destination step was stored and changed the source
  session state.
- RED: a reusable destination session with a token profile was checked with the source session's
  executor profile.
- GREEN: the focused executor lifecycle command passed 41 tests.
- GREEN: the race-enabled workflow transition command passed 13 tests.
- Admission detects the selected profile's non-secret `gh_cli_env` binding metadata. It does not
  reveal the referenced token.
