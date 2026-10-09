---
created: 2026-10-09
status: done
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Implementation plan: durable clarification recovery

## Outcome

Prevent detached clarification answers from becoming journal-only prompt
submissions, and make the existing explicit recovery available for interrupted
durable work. This implements the diagnosed integration gap in PR #3598 using
the existing delivery and harness-continuity contracts.

## Contracts

- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Harness requirements](../../specs/agents/requirements/harness-session-continuity.md).
- [Delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Harness design](../../specs/agents/system-design/harness-session-continuity.md).

## Delivery

Execute [task 01](task-01-clarification-admission-and-recovery.md) in this session.
The user requested implementation and a PR following the completed investigation.

1. Reproduce missing backend registration and missing bounded recovery response.
2. Register each detached-answer dispatch attempt before provider admission.
3. Expose explicit history continuation for unresolved durable work.
4. Run the targeted race tests and lint, then publish the PR.

## Deployment and existing sessions

The running backend must be updated to the merged build. Existing interrupted
submissions retain uncertain outcomes; this patch does not automatically resend
them, clear database blocks, or replace native conversations. An operator selects
the existing Continue from history action, which starts a new native conversation
from a bounded task/plan/chat snapshot while preserving the workspace and stored
Kandev history. The existing generation transition retires the old submissions.

## Validation

Run from `apps/backend`:

```bash
go test -trimpath -race ./internal/orchestrator ./internal/orchestrator/handlers -run 'TestDetachedClarification|TestResumeDetachedClarification|TestRestoreRequiredRecoveryResponse|TestWSLaunchSession_DurableRecovery' -count=1
golangci-lint run ./internal/orchestrator/... --new-from-rev=770ba303fd07f26edab2f7edb84ad62684111813 --timeout=5m
```

Also run `python3 scripts/list-docs.py validate` and `git diff --check` from the
repository root. Backend-only changes reuse the existing recovery controls;
no frontend layout or component changes are included.

## Risks

Unknown prompt outcomes must remain blocked until explicit recovery. A detached
answer accepted asynchronously must remain dispatching until its correlated
terminal event settles it. Legacy delivery must continue using its existing path.

## Results

The registration, unresolved-peer and launch-response regressions reproduced the
missing behavior before the production change. The targeted tests passed with
the race detector, changed-code lint reported zero issues, and documentation
coverage, catalog validation and whitespace checks passed. The implementation
keeps existing prompt claims, generation transitions and explicit recovery.
