---
created: 2026-10-26
status: complete
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Implementation plan: Quick Chat startup acceptance

## Overview

Return the accepted Quick Chat identity before repository and agent startup.
Then add keyed creation recovery and integrate the existing opening composer.
Execute the three work orders sequentially with TDD.
Implementation and task-defined verification are complete.

## Evidence and ownership

The October 26 incident created task `673a0f2c-6eba-4bcc-89f0-83e8c11d3fbf` at
20:43:49.901 UTC. Its Quick Chat POST logged HTTP 200 after 36.713 seconds.
The HTTP server's response write timeout was 30 seconds.
The browser showed 502, but the retained agent reached WAITING_FOR_INPUT.
Its opening prompt was not submitted.

A temporary real Go HTTP server and reverse proxy reproduced handler success,
upstream EOF, and browser 502 after the response deadline expired.
The exact live proxy event remains inferred because those logs were unavailable.

Tasks owns creation identity, allocation, startup, and first-message recovery.
Desktop and phone outcomes remain in that same capability.
The [acceptance ADR](../../decisions/2026-10-26-quick-chat-creation-acceptance.md)
records the contract choice and alternatives.

## Scope

### In scope

- Ordinary Quick Chat with structured or terminal-backed profiles.
- Allocation before preparation, authenticated bounded background startup, and retained failures.
- Optional creation UUIDs using shipped external-ID uniqueness and settlement.
- Exact-token read-only status lookup and authorized lifecycle/list projection.
- Pending draft ownership, same-token retry, and explicit recovery on desktop and phone.
- Explicit editable recovery after typed pre-allocation validation/reference rejection.
- Focused backend, hook, and browser regressions, localization, and public recovery guidance.

### Out of scope

- Changing the global HTTP timeout or proxy configuration.
- Redesigning configuration-chat creation, generic task creation, or provider resume semantics.
- The draft aggregate-creation protocol, a new worker queue, or database migration.
- Automatic message replay, cross-device draft synchronization, or matching chats by recency.
- Server idempotency tombstones after physical task deletion. Browser deletion retires its token.
- Feature flags, broad verification, publication, or delegated implementation.

## Technical approach

### 1. Allocate and acknowledge before startup

Change `httpStartQuickChat` from synchronous `IntentStart` to allocation through
`IntentPrepare`, with `DeferredStart: true` and no workspace materialization.
Reuse the normal task-create prepare/dispatch separation.
Dispatch exactly one `IntentStartCreated` with a bounded authenticated context.
Keep HTTP 200 and the existing identity fields for legacy callers.

Browser structured startup sets `NoInitialPrompt: true`. Its subscribed message handler
remains the sole opening sender. Terminal-backed requests retain their opening
submission and claimed file descriptors before acceptance.
Their startup owns the prompt-bearing launch. Keep initial-submission receipts
and explicit replay-blocked handling for interrupted inline data.
Legacy explicit HTTP prompts retain backend delivery, including structured profiles.

The orchestrator records background admission/preparation failures through its
existing typed conditional failure transition. A log-only failure is insufficient.
Retain allocated sessions and preserve attempt, deletion, and capacity-deferral guards.

### 2. Recover a creation by identity

Extend request/response types with optional `creation_id` and acceptance metadata.
Use a reserved server-derived external identity for keyed requests.
`CreateTaskResult` found outcomes must precede all new preparation and dispatch.
Use `Service.SettleExternalID` before background startup.
No pending aggregate-creation coordinator is a prerequisite.

Add `GET /api/v1/workspaces/:id/quick-chat/creations/:creation_id`.
It returns the exact authorized creation and optional prepared session.
It never allocates, repairs, or starts work.
An unsettled task stays incomplete. An explicit discard uses existing confirmed deletion.
Legacy rows without a token remain readable and cannot be adopted by heuristics.

### 3. Reconcile draft and conversation ownership

Persist one immutable submitted snapshot before POST.
Keep its creation UUID, message UUID, text, and descriptor-only attachments.
Reconcile HTTP, list, and lifecycle results by that exact identity.
Move a confirmed snapshot into one conversation without clearing newer edits.
Uncertain outcomes retain their identity and expose **Check status**.
An explicit retry reuses the same token. Reload only reads status and restores recovery.
It never automatically resubmits creation or an attempted opening message.

Preserve generation guards, deletion tombstones, dismissal, and auth/workspace isolation.
Render confirmed startup through existing preparation and recovery components.
Keep configuration-chat behavior compatible through focused regressions.

### Compatibility matrix

| Path                          | Identity and transport                                 | Result                                                                               | Evidence and fallback                                                 |
| ----------------------------- | ------------------------------------------------------ | ------------------------------------------------------------------------------------ | --------------------------------------------------------------------- |
| Structured ordinary chat      | Creation UUID plus existing message UUID               | Async startup, then one subscribed opening message                                   | Handler/hook/E2E tests. Admission errors keep session recovery.       |
| Terminal-backed ordinary chat | Creation UUID plus retained initial-submission receipt | One backend prompt-bearing start                                                     | Handler/orchestrator tests. Interrupted inline payload blocks replay. |
| Legacy ordinary API caller    | No creation UUID, existing HTTP fields                 | Async startup with the same envelope. Explicit HTTP prompts retain backend delivery. | Handler compatibility tests. No guessed uncertain-response adoption.  |
| Configuration chat            | Existing endpoint and admission                        | Current separate behavior                                                            | Existing opening-payload and configuration regressions.               |
| Missing or changed identity   | Exact workspace/principal/kind/token checks            | Unknown, incomplete, or explicit conflict                                            | No other chat or automatic replacement.                               |

## ASCII UI preview

Copy is illustrative and must use locale keys. Control order and ownership are required.
These views map to `.9` through `.15` of the opening-composer requirement.

### UI-01: Desktop accepted startup

Entry: Send in ordinary Quick Chat. Preparation is still running.

```text
+-------------------------------------------------------------+
| Existing chat | New conversation (active) | +                |
+-------------------------------------------------------------+
| Preparing repository...                                    |
| Setup script: running                                      |
|                                                           |
| Opening message retained until normal admission is ready.   |
|                                                           |
| [conversation composer and existing recovery controls]      |
+-------------------------------------------------------------+
```

The setup transitions into this one retained conversation after acceptance.
Navigation remains fixed. Existing conversation content owns scrolling.
There is no second setup error reporting a confirmed startup as a failure.

### UI-02: Uncertain creation, desktop and phone

Entry: a response is lost before acceptance can be confirmed.

```text
Desktop setup region                 Phone full-height Quick Chat
+--------------------------------+   +------------------------------+
| Submitted draft and files      |   | Back  Quick Chat  Chats      |
| remain visible                 |   +------------------------------+
|                                |   | Submitted draft and files    |
| Could not confirm creation.    |   |                              |
| [Check status]                 |   | Could not confirm creation.  |
|                                |   | [Check status]               |
| No second unkeyed Send         |   |                              |
+--------------------------------+   | Internal scroll body         |
                                     +------------------------------+
                                     | Safe-area inset              |
                                     +------------------------------+
```

The phone uses the shipped Quick Chat full-height surface, not another overlay.
Its status actions have at least 44px hit targets. Desktop controls retain compact sizing.
The status action reads the same identity and never sends a message.
An incomplete task can expose the existing confirmed deletion flow below this status.

### UI-03: Phone accepted startup

```text
+------------------------------+
| Back  Quick Chat  Chats      |
+------------------------------+
| Preparing repository...      |
| Setup script: running        |
|                              |
| Conversation content         |
| (single internal scroll)     |
|                              |
+------------------------------+
| Existing composer/recovery   |
| Bottom safe-area inset       |
+------------------------------+
```

Use dynamic height and the existing scroll owner.
Keep the primary recovery action reachable with the keyboard visible.
Viewport switches preserve the same pending identity and draft.

## Tests

| Criteria                                              | Regression evidence                                                                                                          |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Composer `.13`, recovery `.003.1` and `.003.3`        | `TestQuickChatAcceptance_ReturnsBeforePreparation` behind a real server/proxy barrier. Async failure retains typed recovery. |
| Composer `.8`, `.9`, `.13`                            | Terminal payload/receipt tests and `NoInitialPrompt` startup tests. One delivery owner per profile kind.                     |
| Composer `.14`, `.15`                                 | `TestQuickChatCreationIdentity` concurrency, found-before-validation, settlement, authorization, and status lookup cases.    |
| Composer `.8` through `.10`, `.14`, `.15`             | Hook tests for dropped/malformed responses, WS/HTTP order, explicit retry, reload, draft generations, and deletion.          |
| Composer `.11`, `.12`, recovery `.003.2` and `.003.5` | Desktop and phone browser scenarios plus locale and type checks.                                                             |

Use a preparation channel and HTTP completion to order the backend regression.
Do not add a long timing assertion or a fake success handler that bypasses launch code.

## E2E tests

Add `quick-chat-startup-acceptance.spec.ts` and
`mobile-quick-chat-startup-acceptance.spec.ts` under `apps/web/e2e/tests/chat/`.
Use the existing opening-composer and setup-recovery helpers and disposable backend.

- Forward the real POST, capture its accepted identity, then replace its response with non-JSON 502.
- Deliver exact-token lifecycle state before and after the HTTP outcome. Assert one conversation and one opening message.
- Preserve text and uploaded descriptors through uncertainty, explicit status lookup, and same-token retry.
- Reload after allocation. Assert status-only recovery and no automatic second POST or opening-message replay.
- Navigate or dismiss during pending creation. Assert no stolen selection or reopened surface.
- Keep healthy and pending/failed tabs together. Assert foreign or similar-title chats are never adopted.
- On phones, use touch actions and assert 44px targets, safe-area reachability, and no horizontal document overflow.

The backend integration test proves bounded acceptance.
Browser response interception proves recovery, not server deadline behavior.

## Work orders

- [x] [Task 01: Accept before preparation](task-01-accept-before-preparation.md)
- [x] [Task 02: Recover keyed creation](task-02-recover-keyed-creation.md)
- [x] [Task 03: Reconcile opening drafts](task-03-reconcile-opening-drafts.md)

## Related implementation records

- [Opening composer](../quick-chat-opening-composer/plan.md) is the shipped baseline.
- [Setup recovery UX](../setup-recovery-ux/plan.md) owns retained-session failure behavior.
- This package extends those contracts. Their recorded test counts remain historical evidence.
  New work orders and new verification results belong here.

## Verification results

Implementation results are recorded in the completed work orders below.

### Design-package checks (2026-10-26)

- `python3 scripts/list-docs.py validate`: passed, including the new decision and amended specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository `pr-docs.cjs` coverage preflight: the actual documentation-only diff is exempt.
  The prospective owned implementation path is covered by all three work orders, with no reference errors.
- Package-relative file links resolve. Work-order requirements, acceptance criteria, and designs pass the repository coverage validator.
- `git diff --check -- docs/specs docs/decisions docs/plans`: passed.
- At the original design checkpoint, no product tests ran because the changes were documentation-only; all implementation work orders were pending then.

### Implementation checks (2026-10-26)

- Task 01 backend tests passed for Quick Chat acceptance, opening payload compatibility, retained failures, and launch handling.
- Task 02 identity, concurrency, route, authorization, list, boot, lifecycle DTO, and reserved external-ID tests passed across handlers, service, backendapp, and DTO packages.
- Task 03 frontend Vitest passed: 36 files and 289 tests. `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed. The catalogs are complete and the new-code ratchet reported no violations.
- Desktop Chromium E2E passed: 8 tests, including startup acceptance, opening composer, and setup recovery.
- Phone `mobile-chrome` E2E passed: 4 tests, including startup acceptance, opening composer, touch-target, and setup recovery coverage.
- Both managed E2E runs built the current production Vite assets and backend fixture.
- `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- The public Quick Chat recovery guide was updated and validated. No commit or publication was made.

### PR review fixup revalidation (2026-10-27)

- From `apps/backend`, `go test -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/executor ./internal/backendapp -run 'TestQuickChatCreationIdentity|TestCommitQuickChatStartFailureRequiresCurrentOwner|TestPostgresCommitQuickChatStartFailureIfCurrentAttempt|TestQuickChatStartFailure|TestQuickChatAcceptance|TestQuickChatFailureRetention|TestHTTPListQuickChatSessions|Test.*ReservedExternalID|Test.*QuickChat.*Authz' -count=1` passed. The environment-gated PostgreSQL case was not exercised because `KANDEV_TEST_POSTGRES_DSN` was unset.
- From `apps/backend`, `go test -race -trimpath -tags fts5 ./internal/task/handlers ./internal/orchestrator ./internal/task/repository/sqlite -run 'TestQuickChatCreationIdentity|TestQuickChatStartFailureIgnoresCompetingStartingSession|TestQuickChatStartFailureDoesNotOverwriteReplacementWaitingSession|TestCommitQuickChatStartFailureRequiresCurrentOwner|TestPostgresCommitQuickChatStartFailureIfCurrentAttempt' -count=1` passed.
- From `apps/backend`, `go test -race ./internal/task/handlers -run 'TestHTTPStartQuickChat(ForwardsAutoTitleAndKeepsProvisionalTitle|WithoutAutoTitleDoesNotMarkPendingTitle)$' -count=1` passed after the full CI shard exposed unlocked async-test reads.
- `bash scripts/lint-go-changed` passed for all changed Go files, including `task_http_handlers_test.go`, with zero issues; `make -C apps/backend build` succeeded.
- From `apps/web`, the work-order Vitest command passed 34 files and 290 tests; `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, changed-file ESLint, Prettier, and `pnpm run build` passed.
- Desktop Chromium managed E2E passed 8 tests and phone `mobile-chrome` managed E2E passed 4 tests. Each runner built the backend fixture and production web assets.
- Public-doc validation passed 63 tests and validated 54 pages. The specification catalog validated 404 decisions and 1,746 specifications; full specification lint and `git diff --check` passed.

### Current-base revalidation (2026-10-27)

- Task 01 and Task 02 backend regression commands passed for handlers,
  orchestrator, executor, service, and backendapp. The additional focused
  backend regression run passed across handlers, orchestrator, executor,
  SQLite, service, and backendapp, including deferred-start attempt ownership,
  keyed creation, typed pre-allocation rejection, authorization, and stale
  launch-failure cases.
- `bash scripts/lint-go-changed` passed for the changed Go files with zero
  issues. Both managed E2E runs built the backend fixture.
- The Task 03 frontend Vitest command passed: 35 files and 285 tests. Typecheck,
  `i18n:check`, and `i18n:ratchet` passed.
- Changed-file frontend ESLint passed with zero warnings. Prettier passed for
  changed frontend TypeScript, TSX, and locale JSON files.
- Desktop Chromium managed E2E passed: 8 tests. Phone `mobile-chrome` managed
  E2E passed: 4 tests. Both runs rebuilt the production Vite assets.
- Public-document validation passed for 54 pages. Specification validation
  passed for 404 decisions and 1,746 specifications, and all specification
  files passed lint. `git diff --check` passed.
- Desktop and phone recovery screenshots were captured, compressed, and
  inspected with synthetic data for the PR.

## Risks

- A response can still fail before acceptance. Keyed lookup must preserve an unknown or incomplete state without duplicate work.
- A crash can occur between session preparation and background dispatch. Keep the retained session and require existing explicit recovery.
- The shipped external-ID seam differs from its draft aggregate redesign. Re-read the current base before implementation.
- Found outcomes must never repeat attachment claims, session preparation, title mutation, or dispatch.
- Structured recovery must preserve the original client message identity and never replay on reload.
- Shared configuration and terminal hooks require compatibility tests despite the ordinary-chat repair scope.
