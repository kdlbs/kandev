---
id: "01-durable-intake"
title: "Durable intake and ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-001
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-001.1
  - AC-ORCHESTRATION-ASSISTANT-001.2
  - AC-ORCHESTRATION-ASSISTANT-001.3
  - AC-ORCHESTRATION-ASSISTANT-001.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 01: Durable intake and ownership

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S05, S06, S19, S22, and [plan](plan.md), Backend 1. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. A user-owned default binding resumes an existing conversation; foreign owners and runtime callers cannot claim/configure it. Ownership survives switching/deleting the binding and unregistering the persona; stale runtime reads and launches are revoked.
2. Comment and outbox acceptance are atomic and idempotent by client message ID; exact source lookup survives 100 newer comments and failed enqueue.
3. Operation receipts and intent revisions reject stale authority and retain unknown outcomes across restart without repeating an uncertain write.

## Likely files

- apps/backend/internal/orchestration/models/assistant.go, operation.go (new)
- apps/backend/internal/orchestration/repository/sqlite/assistant_schema.go, intake.go, operations.go (new); comments.go
- apps/backend/internal/orchestration/runtime/handler.go, service.go, recovery.go; intake_test.go (new)
- apps/backend/internal/runs/service and repository/sqlite (only the minimal transaction/outbox integration)
- apps/backend/internal/auth/authn/identity.go (reuse identity contract, no permission broadening)

## Implementation sequence

Ownership repair (explicitly requested after the checkpoint): add a replay-safe owner column to the conversation registry and explicit legacy import columns; atomically claim it with binding selection. Cover switch/delete/reselect, migration backfill/replay and failed/concurrent claims. Gate private configuration/reimport, automation, queue admission/launch and runtime memory/history on retained ownership/current selection. Cover native task/session access through backend composition. Run the existing command below plus the focused ownership checks:

```sh
(cd apps/backend && GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp ./internal/task/service -run 'TestAssistant(Binding|Privacy)|TestConversationRunsWithoutOffice|TestScheduledDeliveryReusesConversationAndSelectedAccount|TestOrchestratorsUseProfilesAndScopeConfiguration|TestImportPreservesAssistantIdentityAndRejectsOtherWorkspaces')
```

Introduce additive owned schema and exact comment lookup first. Write real-DB failure/duplicate/concurrent-send tests before implementing outbox dispatch. Keep existing JSON fields, routes and run-scoped JWT revocation compatible. Add source/intent cursors to prompt assembly; revalidate authority server-side rather than relying on a model rereading chat.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/repository/sqlite ./internal/orchestration/runtime -run 'TestAssistant(Intake|Binding|Operation|Intent)|TestRuntimeAPIScopesWritesAndRevokesFinishedRuns|TestRestartDoesNotReplayAnInterruptedConversation')
```

## Dependencies and risks

Dependencies: none. Execute in the primary session unless the user explicitly authorizes subagents.

The queue and comments currently use separate calls. A mutex is not transactional durability. Do not widen workspace JWT scope or assume a timeout proves no dispatch.

## Output

A durable single-workspace intake and operation foundation, compatible with existing conversations.

## Results

**Ownership repair verified on 2026-09-16.** The previously failing `TestAssistantBindingSwitchKeepsPreviousConversationPrivate` now passes. Retained registry ownership, atomic selection/intake checks, backfill/replay, runtime read/launch revocation, configuration and automation restrictions, native task/session guards and legacy Office isolation close the reproduced paths. See [ownership-checkpoint.md](ownership-checkpoint.md) for the original failure and exact verification evidence.

The ownership race command above passed **23 top-level tests** (19 runtime, 2 configuration, 2 backend composition). The original task filter passed **27 runtime tests**; its repository selection remains empty, so the two repository migration tests were run separately with race detection. The task-03 context/memory/credential filter passed **10 tests**. Workspace-scoping and existing redactor tests passed with race detection. A task-service test import cycle exposed by the earlier memory work was removed by moving the unchanged redactor to a dependency-free common package while preserving the sharing API.

No production/dev data, provider or vault was accessed. No restart, deployment, commit or PR was performed. The assistant feature as a whole remains unfinished.

Implemented additive binding/intake/intent/operation storage, atomic comment acceptance, repair from the existing dispatch tick, exact source lookup and keyset history pagination. Bound assistant writes require operation identity; lost acknowledgements remain unknown. New intent or binding revisions revoke older runs. Private conversation routes reject other owners/coordinators.

Red tests observed duplicate acceptance (201 instead of 200), lost offline intake, missing original source after 105 comments, foreign/private reads, missing cursor/run receipt, stale writes, duplicate actions and a dispatched receipt surviving restart. All are green after the corresponding changes.

The exact verification command above, with `GIN_MODE=release` and `-v`, passed **21 runtime tests** (19 new assistant tests plus the two named compatibility tests). The repository package has no matching test names; its existing migration suite passed in the additional full `go test -race -tags fts5 -count=1 ./internal/orchestration/...` run. Full orchestration race run passed, including channel-coordinated simultaneous sends/actions, atomic rollback injection, migration replay and lost-acknowledgement injection. Backendapp compiled in a scoped cross-package run.

Tests used disposable SQLite fixtures only; no production/dev service or database was changed. Owned files: assistant/operation models; assistant schema, binding, intake, operations and comment repository helpers; runtime intake/intent/operation/binding handlers and tests; minimal existing dispatcher wiring. Two existing fixtures were repaired to supply canonical parent tables and the actual source comment required by exact lookup. No commits or publication performed; the shared checkout contains substantial pre-existing changes.
