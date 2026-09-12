---
created: 2026-09-12
status: implemented
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
legacy_specs: []
---

# Implementation plan: Initial task brief

## Overview

Preserve the task brief when a user starts a prepared session through Chat.
First extend atomic message admission. Then connect composition and dispatch.
Finally prove transcript visibility on desktop and phone. All work is sequential.

Source: [GitHub issue #3615](https://github.com/kdlbs/kandev/issues/3615).
Investigated revision: `8892920514250dfc9873725f90d736a5cc7e388a`.
The issue had no comments or image attachments during investigation.
The authenticated user `carlosflorencio` was assigned on 2026-09-12.

## Confirmed root cause

1. `hooks/use-message-handler.ts` submits the additional text through `message.add`.
2. `internal/task/handlers/message_handlers.go:wsAddMessage` persists the submitted content without the task description.
3. `dispatchPromptAsync` forwards the committed content to `forwardMessageAsPrompt`.
4. `StartCreatedSession` uses the supplied nonempty prompt. Its description fallback applies only to an empty prompt.
5. `hooks/use-processed-messages.ts:shouldShowTaskDescriptionFallback` removes the synthetic brief after any stored user message appears.

The transcript behavior conforms to its existing specification. The missing
contract is preservation during direct first-message admission. Do not weaken
the UI fallback predicate or change automatic workflow fallback semantics.

## Reproduction evidence

A temporary `internal/task/handlers/issue3615_repro_test.go` used the existing
`runCreatedMessageContextTest` harness. It created an ordinary task with a
nonempty description and a single CREATED session. It submitted
`rebase on latest main` through the real handler and task service.

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/handlers -run '^TestReproIssue3615$' -count=1 -v)
```

The assertion failed for the expected reason:

```text
saved brief=false, saved instruction=true, description intact=true
original task brief omitted from saved first message
FAIL TestReproIssue3615
```

This reproduction used a repository fake and no agent process. Agent omission
is supported by the dispatch source trace and issue evidence, not a live-agent run.
The temporary test was removed after recording the result. No runtime was launched.

Permanent regression: `TestWSAddMessage_InitialTaskBrief` in
`apps/backend/internal/task/handlers/message_handlers_initial_task_brief_test.go`.
Use the existing `firstTurnCaptureOrchestrator` to assert saved and dispatched
visible content, then cover the real orchestrator composition separately.

## Scope

### In scope

- Direct first-message brief composition for ordinary CREATED sessions.
- Atomic selection using the existing durable session prompt boundary.
- Idempotency, rollback, description snapshots, and concurrent admission.
- Existing structured, passthrough, saved-prompt, attachment, and queue integration.
- Persisted prompt #1 visibility on desktop and phone.

### Out of scope

- Production or permanent test changes during this design turn.
- Historical transcript backfill, new prompt ordinals, or synthetic user history.
- Changes to automatic workflow entry, Office, Quick Chat, or configuration policies.
- New public replacement controls, API fields, or database migrations.
- Provider-level delivery guarantees after crashes.

## Technical approach

Follow [the system design](../../specs/tasks/system-design/initial-task-brief.md).
A server-only initial candidate travels from the handler through the service.
The repository chooses it inside the existing prompt write transaction.
The handler dispatches the returned content with its matching trusted context.

Do not call `HasUserPromptHistory` outside a transaction and then write later.
Do not select by `PromptIndex == 1` alone: automatic fallback reserves ordinal
zero before its user row exists. Preserve the existing fallback reservation.

Prepare candidate expansions before database writes. Compare the description
snapshot during admission and reject stale selection. Keep replay identity based
on the original request. Preserve plan-comment transaction and attachment ownership.

The rendered structure stays unchanged. Stored prompt content corrects visibility.
No new locale keys are expected. Any added product copy must use existing localization rules.

## ASCII UI preview

```text
UI-01: Chat after first message (desktop and phone)

Before                         Proposed
+-------------------------+    +-------------------------+
| User #1                 |    | User #1                 |
| rebase on latest main   |    | Original task brief     |
|                         |    |                         |
| Agent response          |    | rebase on latest main   |
+-------------------------+    | Agent response          |
| Composer         [Send] |    +-------------------------+
+-------------------------+    | Composer         [Send] |
                               +-------------------------+
```

Entry: open the task, then send a preparatory message before its first start.
AC references: `AC-TASKS-INITIAL-TASK-BRIEF-001.1`, `.2`, `.7`.
The before view is supported by the source predicate and reproduction.

Desktop uses the existing Chat pane. Phone uses full-height Chat through
`task-layout.tsx` and `SessionMobileLayout`. Both share this content order.
The transcript scrolls vertically. The composer retains its existing fixed
region and phone safe-area behavior. Long content uses existing expansion
controls and history navigation. No new drawer, button, or hover action is required.
Content order and durable visibility are required. Borders and spacing are illustrative.

## Tests

| Criteria | Test and boundary |
| --- | --- |
| `.1`, `.2`, `.4`, `.10` | `TestWSAddMessage_InitialTaskBrief`, `TestWSAddMessage_InitialTaskBriefExpandsCombinedPromptAtAdmission`, `TestWSAddMessage_InitialTaskBriefKeepsAcceptedExpansionWhenDefinitionsChange`, handler persistence and captured dispatch |
| `.3`, `.5`, `.6` | `TestInitialTaskBriefAdmission`, SQLite transaction, fallback reservation, rollback, and replay |
| `.5`, `.6` | `TestInitialTaskBriefAdmissionPostgres`, cross-connection transaction parity |
| `.8`, `.9` | `TestStartCreatedSession_InitialTaskBrief`, handler saved-reference table cases, and queued initial-brief delivery |
| `.2`, `.7` | `use-processed-messages-fallback.test.ts`, synthetic row replacement with combined prompt |

All criterion suffixes refer to `AC-TASKS-INITIAL-TASK-BRIEF-001`.
Repository tests must include two distinct first submissions, a direct/fallback
race, zero reservation, message deletion, restart, stale description, and rollback.
Handler cases must include normal launch, queued promotion, session redirection,
attachments, plan comments, saved references, equality, empty brief, and excluded modes.

## E2E tests

Add `e2e/tests/chat/initial-task-brief.spec.ts` for `chromium` and
`e2e/tests/chat/mobile-initial-task-brief.spec.ts` for `mobile-chrome`.
Seed a task without starting it. Open Chat, observe the brief, and submit the
preparatory instruction through the composer. Assert both texts in prompt #1,
reload, and assert them again. Send a later message and assert no brief replay.
These flows cover `.1`, `.2`, `.3`, and `.7`. Use existing causal WebSocket waits.

## Work orders

- [x] [Task 01: Atomic initial content selection](task-01-atomic-admission.md) — done
- [x] [Task 02: First-message composition](task-02-first-message-composition.md) — done
- [x] [Task 03: Transcript visibility evidence](task-03-transcript-evidence.md) — done

## Related packages and documentation

The completed `workflow-step-agent-start-ownership` package owns reset/start
routing. Its scope and results remain unchanged. The transcript visibility
package owns pagination. This package consumes both contracts without rewriting
their completed work orders or test results.

Task 03 added a short explanation to `docs/public/tasks-and-workflows.md` with
the implemented behavior.

## Verification results

- Permanent backend admission, handler, service, and orchestrator regressions passed, including combined saved-reference preparation, accepted-context dispatch, and queued delivery.
- SQLite admission tests passed, including rollback, stale snapshots, deletion, restart, fallback races, zero reservations, and plan-comment queues.
- PostgreSQL parity test was skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured in this environment.
- `go vet` passed for all changed backend packages.
- `make -C apps/backend lint`, `make -C apps/backend build`, and `go run ./cmd/sqlguard ./internal` passed.
- Race-enabled initial-admission tests and `go test -race ./internal/persistence/storeconformance -count=1` passed.
- `pnpm exec vitest run hooks/use-processed-messages-fallback.test.ts` passed.
- `pnpm run typecheck` passed.
- `pnpm run i18n:check` passed.
- Targeted changed-file ESLint passed with zero warnings, and `pnpm run e2e:sleep-ratchet` passed.
- Managed `chromium` and `mobile-chrome` E2E tests passed, including reload and later-message behavior.
- Both public-doc validators passed; the specification test suite passed 36 tests, all specification files passed, and `git diff --check` passed.

## Risks

- Candidate selection must pair visible content with the correct trusted expansion.
- A nonempty workflow template can replace a direct prompt during launch composition.
- A fallback reservation has no visible row yet but still owns the first boundary.
- Plan-comment admission and queued content must select the same candidate atomically.
- Existing affected conversations require manual brief recovery. This package does not backfill them.
