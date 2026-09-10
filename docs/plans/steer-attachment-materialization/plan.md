---
created: 2026-09-02
status: implemented
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
legacy_specs: []
---

# Implementation Plan: Steer Attachment Materialization

## Overview

A message sent while the agent is still generating is delivered through the
steer route. That route dispatches the prompt without materializing the claimed
attachment descriptors it carries.

The agent therefore receives a bounded descriptor whose bytes were never
written to `.kandev/attachments/<acp-session-id>/`. The reference resolves to
nothing and the attachment reads as empty.

The failure is silent. The steer route has no equivalent of the ordinary
prompt path's materialization error, so the user sees a delivered message and
an agent that cannot see the file.

This plan moves materialization ahead of the steer dispatch, keeps it outside
the prompt lifecycle lock, and makes a materialization failure terminal for
that message instead of silently degrading.

## Scope

### In scope

- Materialize claimed descriptors before a steer dispatch.
- Route a lost steer race through ordinary prompt admission.
- Return a durable error when materialization fails on the steer route.
- Release the steer slot and retry queued draining after a pre-dispatch failure.
- Backend regression coverage in the lifecycle package.
- One Playwright scenario for a mid-turn attachment.

### Out of scope

- Upload limits, staging, claim admission, and retention.
- Delivery-mode selection and the prompt composer.
- The launch and initial-prompt routes fixed by
  `docs/plans/session-launch-prompt-admission/`.
- Queued-message delivery, which already reaches the ordinary prompt path.

## Reproduction evidence

The defect was reproduced on a live instance. The upload succeeded and the
backend blob was written, but the session's
`.kandev/attachments/<acp-session-id>/` directory stayed empty and the agent
received a resource reference that resolved to nothing.

Two observations look like counter-evidence and are not:

- `KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING=false` appears in the backend
  process environment. `profiles.ApplyProfile` stamps profile defaults into the
  process with `os.Setenv` and records them, and `runtimeflags/config.go` treats
  an env var as explicit only when `!profiles.WasApplied(name)`. The
  `runtime_flag_overrides` row `features.claudeMidTurnSteering = 1` therefore
  wins and steering is enabled.
- The backend log shows no `sending prompt to agent` line carrying the
  attachment. That line is emitted from `preparePrompt`, which only `sendPrompt`
  calls. The steer route never reaches it, so its absence is what this defect
  predicts rather than evidence against it.

## Root cause

`SessionManager.sendPrompt` in
`apps/backend/internal/agent/runtime/lifecycle/session.go` calls
`sm.materializeAttachments` before `sm.triggerPrompt`.

`SessionManager.dispatchSteerLocked` in the same file calls
`sm.callAgentctlPrompt` with the caller's `attachments` slice untouched. No
materialization runs on that route.

`SendPromptSteerWithDispatchCallback` tries the steer first. When a turn is
live the steer succeeds and the unmaterialized descriptors reach the agent.
When no turn is live it falls back to `sendPrompt`, which materializes
correctly. That is why the defect only appears mid-turn.

## Technical approach

### Materialize before the steer

Resolve descriptors in `SendPromptSteerWithDispatchCallback`, after the status
check and before `tryDispatchSteer`.

Materialization streams up to 100 MiB over HTTP. It must not run inside
`dispatchSteerLocked`, which holds `execution.promptLifecycleMu` under
`steerDispatchTimeout`. Resolving in the caller keeps the lock bounded and
preserves the existing steer latency contract.

Return the materialization error to the caller. Do not dispatch and do not fall
back to an ordinary prompt, because the fallback would deliver the same
unresolved descriptors.

### Return lost races to ordinary admission

Check for an active generation before uploading. If the generation ends during
materialization, return a typed no-dispatch error instead of calling
`sendPrompt` directly. The orchestrator maps this result to its existing
ordinary prompt fallback, so foreground claims and prompt-cycle state stay in
one owner.

Wrap materialization failures with a separate pre-dispatch identity. The
orchestrator releases `steerInFlight` and retries the queued-message drain
after this failure, so a completion that raced with the upload cannot strand a
successor.

### Regression coverage

`newMockAgentServer` in
`apps/backend/internal/agent/runtime/lifecycle/session_test.go` already serves
an `http.ServeMux`. Add an `/api/v1/attachments/materialize` route that records
each upload, so `steerHarness` can assert materialization on the wire.

The New Agent mid-turn scenario belongs with the existing chat attachment
specs in `apps/web/e2e/tests/chat/`.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.6` | `TestSendPromptSteer_MaterializesAttachmentsBeforeDispatch` in `apps/backend/internal/agent/runtime/lifecycle/session_steer_test.go` |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.6` | `TestSendPrompt_MaterializesAttachmentsExactlyOnce` in `apps/backend/internal/agent/runtime/lifecycle/session_steer_test.go` |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.7` | `TestSendPromptSteer_MaterializationFailureDoesNotDispatch` in `apps/backend/internal/agent/runtime/lifecycle/session_steer_test.go` |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.7` | `TestSendPromptSteer_DoesNotFallbackOutsideAdmission` and `TestSteerTask_PreDispatchFailureDrainsSuccessor` cover lost-generation admission and queue recovery. |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.6` | `sends an attachment into a generating turn` in `apps/web/e2e/tests/chat/mid-turn-attachment.spec.ts` |

## E2E tests

`apps/web/e2e/tests/chat/mid-turn-attachment.spec.ts` will contain
`sends an attachment into a generating turn`.

The scenario starts a turn, uploads a file while that turn is still generating,
sends it, and asserts the agent resolves the file contents. It must fail before
the correction because the materialized file is absent from the session
directory.

## Work orders

- [x] [Task 01: Materialize steer attachments](task-01-materialize-steer-attachments.md)
- [x] [Task 02: Cover mid-turn attachment delivery](task-02-mid-turn-attachment-e2e.md)

## Verification results

- The three steer regression tests failed first for the expected reasons: zero
  materialization uploads, and no error raised on a failed materialization.
- They pass after the correction, including under the race detector.
- The full `internal/agent/runtime/lifecycle` package passes.
- `golangci-lint run ./...` reports 0 issues and `gofmt` reports no files.
- The web typecheck passes.
- `tests/chat/mid-turn-attachment.spec.ts` failed against the reverted backend
  on the materialization assertion, and passes with the correction.
- The six pre-existing `mid-turn-steering.spec.ts` scenarios still pass after
  `seedRunningGeneratingSession` moved to a shared helper.

## Risks

- `promptLifecycleMu` bounds steer latency. Materializing inside it would let a
  large upload block the lifecycle lock for the length of a network write.
- The steer route can lose its race while an upload is in progress. The
  orchestrator must own the replacement prompt and its admission checks.
- `materializeAttachments` requires a non-empty `ACPSessionID`. A steer implies
  a live turn, but the error must stay a returned error rather than a panic.
- Making a steer materialization failure terminal changes an existing silent
  path into a visible one. Queued and ordinary prompts must keep their current
  ordering behavior.
- Playwright timing for a mid-turn send depends on a turn that is still
  generating. The scenario needs a deterministic long-running mock turn.
