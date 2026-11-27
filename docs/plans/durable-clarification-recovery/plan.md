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

Prevent detached clarification answers and watchdog replacements from becoming
journal-only prompt submissions. Make the existing explicit history continuation
available in the main composer recovery card for interrupted durable work. This
implements the diagnosed integration gap in PR #3598 using the existing delivery
and harness-continuity contracts.

## Contracts

- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Harness requirements](../../specs/agents/requirements/harness-session-continuity.md).
- [Delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Harness design](../../specs/agents/system-design/harness-session-continuity.md).

## Delivery

Execute [task 01](task-01-clarification-admission-and-recovery.md) in this session.
The user requested implementation and a PR following the completed investigation.

1. Reproduce missing backend registration and missing bounded recovery response.
2. Register each detached-answer and watchdog dispatch before provider admission.
3. Expose explicit history continuation for unresolved durable work and wire it
   into the primary composer recovery card after the bounded Resume response.
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
go test -trimpath -race ./internal/orchestrator ./internal/orchestrator/handlers -run 'TestDetachedClarification|TestResumeDetachedClarification|TestClarificationWatchdog|TestRestoreRequiredRecoveryResponse|TestWSLaunchSession_DurableRecovery' -count=1
golangci-lint run ./internal/orchestrator/... --new-from-rev=770ba303fd07f26edab2f7edb84ad62684111813 --timeout=5m
```

Also run `python3 scripts/list-docs.py validate` and `git diff --check` from the
repository root. From `apps/web`, run the focused card tests and managed browser
regressions:

```bash
pnpm exec vitest run components/task/chat/session-recovery-card-history.test.tsx components/task/chat/session-recovery-card.test.tsx components/task/chat/messages/action-message-recovery.test.tsx --maxWorkers=1
pnpm exec tsc --noEmit
pnpm e2e:run --host --project chromium -- tests/session/history-continuation-recovery.spec.ts --retries=0
pnpm e2e:run --host --project mobile-chrome -- tests/session/mobile-history-continuation-recovery.spec.ts --retries=0
```

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

Review follow-up: the watchdog's direct prompt call also omitted the identity,
and the primary card omitted an option already supported by the older banner.
Both defects were reproduced before their fixes, including the browser's missing
button after a bounded Resume response. Follow-up race tests passed, 46 focused
component tests passed, and one desktop plus one 320px mobile E2E passed with
retries disabled. TypeScript and changed-file ESLint passed. The browser checks
require the visible button, an unobstructed hit target, explicit user activation
and the original task/session identity; managed-clone recovery keeps precedence.
Remote CI and review disposition remain tracked in PR #4383.
