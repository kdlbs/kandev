---
spec: docs/specs/workflow-step-move-overrides/spec.md
decision: docs/decisions/2026-08-13-workflow-move-overrides.md
created: 2026-08-22
status: completed
---

# Fix Plan: Preserve Profile Overrides Through Prepared Auto-Start

## Root cause

When a move with `entry_options.agent_profile_id` switches an existing task to a new target-profile session, the session is correctly prepared with the explicit profile. If that session is still `CREATED`, `processOnEnter` launches its prompt through `StartCreatedSession`. That method applies the generic workflow-step profile resolver again, replacing the one-time profile with the target step's durable profile before launch.

## Outcome

Carry the one-time profile precedence through the prepared-session auto-start path. The target session and the launch request must retain `entry_options.agent_profile_id`; ordinary starts without entry options must keep their existing workflow-profile behavior.

## Scope

- Add a focused orchestrator regression covering a profile-switched `CREATED` session and an auto-start target.
- Thread the already-loaded move options through `autoStartStepPrompt` into the prepared-session start seam.
- Preserve the existing workflow resolver for ordinary and explicitly durable workflow starts.

Out of scope: changing profile validation, model precedence, queue persistence, UI behavior, or workflow defaults.

## Acceptance criteria

- The regression fails before the fix because the prepared target session ends with the step profile.
- After the fix, both the persisted target session and the launch request use the one-time profile.
- Existing `StartCreatedSession` workflow-default tests remain green.
- Focused orchestrator tests, `go test -race` for the affected package, `gofmt`, and `git diff --check` pass.

## Verification

    cd apps/backend && GOMODCACHE=/tmp/kandev-go-modcache GOCACHE=/tmp/kandev-go-cache go test ./internal/orchestrator -run 'Test.*EntryProfile.*AutoStart|TestStartCreatedSession|TestPendingMoveWithEntryOptions' -count=1
    cd apps/backend && GOMODCACHE=/tmp/kandev-go-modcache GOCACHE=/tmp/kandev-go-cache go test -race ./internal/orchestrator -run 'Test.*EntryProfile.*AutoStart' -count=1
    cd apps/backend && GOMODCACHE=/tmp/kandev-go-modcache GOCACHE=/tmp/kandev-go-cache go test ./internal/orchestrator -count=1
    cd apps/backend && GOMODCACHE=/tmp/kandev-go-modcache GOCACHE=/tmp/kandev-go-cache make build-kandev
    git diff --check

All listed checks pass. The isolated production staging instance was rebuilt from this checkout and an end-to-end move with a disposable `gpt-5.6-luna` profile confirmed that the target session and real agent launch retain the one-time profile. Disposable staging records were deleted afterward.

## Task wave

Sequential: regression test, minimal implementation, focused verification.
