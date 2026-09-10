---
id: "01-admit-launch-attachments"
title: "Admit launch attachments"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.2
  - AC-TASKS-PROMPT-ATTACHMENTS-001.3
  - AC-TASKS-PROMPT-ATTACHMENTS-001.4
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 01: Admit Launch Attachments

## Summary

Claim staged attachments at the shared `session.launch` boundary. Reject an
invalid claim before the launch intent starts runtime work.

## In scope

- Add and wire the orchestrator attachment-claimer dependency.
- Claim attachment IDs after authorization and before intent dispatch.
- Preserve same-task idempotency for earlier task-scoped claims.
- Keep cross-task and session-scoped isolation strict.
- Add backend tests and the New Agent attachment scenario.

## Out of scope

- Initial-prompt errors after a valid claim reaches the runtime.
- Upload, retention, and attachment presentation changes.

## Acceptance

- A valid staged attachment reaches the launch intent as a claimed descriptor.
- An invalid descriptor starts no agent and creates no prompt turn.
- A task-scoped claim remains reusable only for its task.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestClaimMessageAttachments_(AllowsTaskScopedClaimForSameTaskSession|IsIdempotentForSameTaskSession)' -count=1
cd apps/backend && go test ./internal/orchestrator -run 'TestLaunchSession_RejectsAttachmentClaimBeforeStart' -count=1
make -C apps/backend build
make -C apps/backend e2e-plugin-package
cd apps/web && pnpm run build:e2e
cd apps/web && pnpm e2e:run tests/session/new-session-dialog.spec.ts -- --grep 'staged attachment'
```

## Files likely touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_launch_test.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/task/repository/sqlite/attachment.go`
- `apps/backend/internal/task/repository/sqlite/attachment_test.go`
- `apps/web/e2e/tests/session/new-session-dialog.spec.ts`

## Dependencies

None.

## Risks

- An overbroad idempotency rule can let one session use a file from another session.
- A missing production dependency can make every file-backed launch fail closed.

## Parallelism

`sequential`

## Inputs

- Prompt attachment requirements and system design.
- ADR `2026-08-04-file-backed-prompt-attachments`.
- Existing direct-message and queue claim paths.
- Existing New Agent Playwright scenarios.

## Results

- Added the launch attachment claimer seam and production task-service wiring.
- Claimed file-backed descriptors after authorization and before intent dispatch.
- Allowed an existing task-scoped claim only for a later session on the same task.
- Kept session-scoped claims unavailable to a different session.
- Added focused repository and orchestrator tests plus the New Agent E2E scenario.
