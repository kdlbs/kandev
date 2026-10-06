---
created: 2026-10-06
status: complete
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
  - REQ-TASKS-PROMPT-ATTACHMENTS-002
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
legacy_specs: []
---

# Implementation Plan: Fresh-start recovery

## Overview

Preserve the original submission through failed-start fresh recovery. Retire only
the recovered startup warning and preserve its history. The first three work
orders are complete: owned replay, recovery resolution, and browser evidence.
Follow-up work orders 04 and 05 address replay-policy, queue-admission,
accepted-work provenance, and metadata compare-and-set findings from review.

## Evidence and root cause

The user reported task `f96414f0-1fd9-4e83-bb71-26168dc87585`, session
`5fdf8d3e-93d3-4d2b-aad4-afba8184b8d1`. Read-only investigation found:

- Initial startup ran from 15:21:44 to 15:36:44 UTC on October 6, 2026.
  It failed with `context deadline exceeded` before agent startup completed.
- Fresh recovery started Codex at 16:02:27 UTC. Its turn completed normally
  at 16:02:53 UTC, after it asked for the missing report.
- The saved initial preview contained PNG attachment
  `63107f4a-ba30-4d9f-b5d1-94fd77b43f25`. Authorized download returned the
  original PNG, 258345 bytes. The recovery request omitted attachments.
- The same session retained bootstrap error stamp
  `ca10fbb36c5692ae2c92eed224ff7a8f` after successful recovery.

`RecoverSessionWithOptions` resets provider identity and launches `IntentResume`
without attachment descriptors. `newResumeLaunchRequest` uses only the task
text. Attachment bytes remain available, but the agent does not receive them.

`SessionManager.dispatchInitialPrompt` sends a nonempty automatic prompt without
its boot-ready branch. Recovery resolution lives in `handleAgentBootReady`.
Thus the fresh failed-start path completes a normal turn without retiring the
captured startup warning. This is a source-grounded mechanism; no raw ACP export
was required. The partial backend bundle covered the relevant timestamps.

Use disposable tests for implementation. Do not restart or mutate the reported
live task. The exact cause of the initial runtime timeout remains unknown.

## Related delivery records

[Preparation attachment previews](../preparation-attachment-previews/plan.md)
and [Composer attachment scope](../composer-attachment-scope/plan.md) own existing
submission display and upload scope.
[Session startup failure explanations](../session-startup-failure-explanations/plan.md)
and [Error scope and history](../error-scope-and-history/plan.md) own the delivered
recovery-proof and historical presentation contracts. This package adds regression
coverage without rewriting their recorded completion results or ownership.

## Scope

### In scope

- Durable original submission data for file-backed task creation.
- Explicit failed-start fresh replay, with narrow legacy compatibility.
- Claim validation, materialization, acceptance, cancellation, and restart behavior.
- Stamp-specific resolution and historical error retention.
- Desktop and phone regression evidence and public recovery guidance.

### Out of scope

- Root-cause correction for the initial runtime readiness timeout.
- Replay of all historical messages, later attachments, or sibling sessions.
- Changes to ordinary prompt content, read-only restoration, or post-conversation
  fresh-start policy. Accepted ordinary work must retire a stale initial replay
  receipt without changing ordinary resume behavior.
- Office scheduling, dynamic provider routing, new flags, and new public APIs.
- New agent sessions, persistent remediation tasks, or implementation delegation.

## Technical approach

Tasks owns submission data, claims, and transcript persistence. Agents owns
provider readiness and recovery proof. Existing task error ownership remains
unchanged. This package extends two independent contracts with linked designs;
it does not create a parallel UI specification.

1. Persist the proposed `initial_prompt_submission` record before asynchronous
   launch through `session_launch.go`, `task_operations.go`, and task-create
   handlers. Preserve the existing preview and attach no bytes to metadata.
2. Resolve pending submission descriptors through `AttachmentService` before
   fresh recovery mutates provider identity. Validate the full set.
3. Suppress the automatic description prompt only for owned failed-start fresh
   recovery. Use `resumeTaskSessionWithContinuation` to await readiness and then
   call the normal prompt path with the captured submission.
4. Hold orphan queue delivery until the replay reaches acceptance or failure.
   Enforce the session-owned hold at all shared guarded queue reservation
   boundaries, including manual and enqueue-side drains.
5. Keep boot-ready attempt identity and error stamp through resolution. Reuse
   `SessionRecoveryResolution`, stamp-CAS dismissal, and current WS projections.
6. Persist one original user message with descriptors. Preserve history and
   current composer state through replacement, reload, and reconnect.
7. Rebuild first-conversation Kandev instructions from current trusted server
   state, while keeping receipt/transcript content raw and excluding stale
   hidden context.
8. Retire pending replay authority when any ordinary or interactive prompt is
   accepted. Fail closed if accepted-work provenance is ambiguous after restart.
9. Compare complete JSON member identities in SQLite metadata CAS and cover the
   dialect-sensitive behavior in the environment-gated PostgreSQL suite.

No schema migration is planned. Metadata writes preserve unrelated keys.
Acceptance uncertainty must remain explicit. Durable pending state alone cannot
prove that the provider never accepted a submission.

### Compatibility matrix

| Path | Identity and transport | Intended result | Evidence / refusal |
| --- | --- | --- | --- |
| Codex ACP initial startup recovery | Same task/session, new provider context | PNG plus file delivered through existing attachment path | Scripted ACP capture and browser fixture |
| Other attachment-capable ACP agents | Same host session, advertised delivery capabilities | Preserve native-prompt/path policy | Shared lifecycle test; no provider-specific support claim |
| Provider without native image support | Existing configured delivery capability | Existing safe path delivery or explicit refusal | Capability fixture; never drop an attachment |
| Ordinary native resume | Existing provider conversation identity | No initial replay | Negative backend regression |
| Workspace-only restore | Agent remains stopped | Preserve original failure and files | Backend and browser negative regression |
| Legacy failed-start record | Valid preview and claims, proven pre-dispatch failure | Reconstruct only the original pending submission | Legacy fixture; ambiguous acceptance refuses replay |
| Fresh recovery after accepted conversation work | Existing product policy | No new history replay | Regression against current session recovery tests |
| Passthrough / inline-only legacy submission | Existing transport contract | Preserve current behavior | No new inline persistence or forced ACP replay |

## ASCII UI preview

UI-01: Task Chat after failed initial startup. Source evidence supports the before state.

```text
Desktop, before retry
  Transcript: [Screenshot] original request
  [Startup failure record]
  [Recovery card: Start fresh session | Restore workspace]

Desktop, after successful retry
  Transcript: [Screenshot] original request (one user row)
  [Original startup failure: historical details]
  [Agent response]
  [Editable composer: existing draft and selected attachments]

Phone, after successful retry
  [Task header / Chat]
  [Screenshot and file labels]
  [Original request: one row]
  [Historical error / details]
  [Agent response]
  [Editable composer]
  [Phone navigation]
```

UI-02: Pending recovery and successor failure use the current composer region.

```text
Desktop pending: [Recovery cause | Working... | disabled alternatives]
Phone pending:
  [Recovery cause]
  [Working...]
  [Disabled alternatives, stacked]

Delivery failure after readiness:
  [New delivery error, its own details and eligible actions]
```

The transcript owns vertical message scrolling. The composer/recovery region and
phone navigation retain their current layout and safe-area clearance. Historical
errors retain readable details without recovery controls. These structural
outcomes are required; spacing and example copy are illustrative.
Phone targets remain at least 44px. No new picker, drawer, or route is introduced.
The existing image preview uses its current dismissal and focus-return behavior.
Maps to AC-TASKS-PROMPT-ATTACHMENTS-002.6/.8 and agent criteria 006.16/.17/.19/.33.

## Verification coverage

| Criteria | Implemented coverage |
| --- | --- |
| Submission 002.1-.3, 002.6 | `apps/backend/internal/orchestrator/fresh_start_submission_test.go`: new receipt, legacy preview, attachment-only replay, and one transcript row; bounded model and SQLite reopen tests |
| Authorization, refusal, and retry ownership | The same test file covers invalid claims, concurrent retry, cancelled attempts, successor errors, and cancelled boot |
| Receipt persistence and compare-and-set | `apps/backend/internal/task/handlers/task_http_handlers_initial_preview_test.go`, `apps/backend/internal/task/repository/sqlite/metadata_initial_prompt_submission_cas_test.go`, and `TestInitialTaskSubmissionReceiptSurvivesRequestCancellation` |
| Error history and client presentation | Backend recovery tests, three recovery-presentation Vitest files, and reload assertions in desktop and phone Playwright |
| Agent delivery evidence | Mock-agent ACP payload digests and browser checks of original image bytes and path-delivered file bytes |

Every backend test starts against the real persisted service path or its existing
scripted runtime seam. Assert descriptors, bytes, attempt identity, and durable
error state. A visible screenshot alone is not delivery evidence.

## E2E tests

Added `tests/session/fresh-start-submission-recovery.spec.ts` and
`tests/session/mobile-fresh-start-submission-recovery.spec.ts`. They run in
`chromium` and `mobile-chrome`, respectively.

Upload a real PNG plus a path-delivered resource into a disposable task.
Fail startup deterministically before provider admission. Select Start fresh
session, then inspect captured replay input/materialized bytes through the mock
agent's established fixture boundary. Assert one user row, retained historical
failure, no old active controls, and an editable composer after reload/reconnect.
The cases cover attachment-only input, unavailable attachments, and read-only
restoration. Workspace availability does not resolve the session error.
Map these cases to attachment criteria 002.1-.8 and agent 006.16/.17/.19/.33.
Use causal HTTP/WS waits and the resource-bounded managed runner.

## Work orders

- [x] [Task 01: Recover the original submission](task-01-submission-replay.md)
- [x] [Task 02: Resolve the matching startup warning](task-02-recovery-resolution.md)
- [x] [Task 03: Prove desktop and phone recovery](task-03-browser-evidence.md)
- [x] [Task 04: Close replay policy and admission gaps](task-04-replay-policy-and-admission.md)
- [x] [Task 05: Preserve metadata member identity in compare-and-set](task-05-metadata-cas-identity.md)

Order: 01 -> 02 -> 03 -> 04 and 05. The remediation work orders are independent
after task 03; task 04 owns orchestrator behavior and task 05 owns repository CAS.

## Verification results

Implementation and design validation passed on 2026-10-06:

- Managed desktop Playwright: 9 tests passed, including fresh-start and existing session recovery.
- Managed phone Playwright: 4 tests passed, including fresh-start and existing task recovery.
- Focused Go tests passed across orchestrator, task models, task handlers, SQLite repository, and mock agent. Legacy preview replay and receipt persistence after SQLite reopen passed.
- Race-enabled orchestrator tests passed for concurrent retry and cancellation paths.
- Frontend Vitest: 55 tests passed across three recovery files.
- Web typecheck and ESLint for the desktop and phone Playwright files passed.
- Both managed E2E runs built the backend and production web bundle.
- Public documentation tests and validation passed for 47 pages.
- `python3 scripts/list-docs.py validate`: 356 decisions and 1406 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check` and `gofmt -l` for changed Go source passed.

The implementation and design package remain unstaged and uncommitted. The original
runtime timeout remains unexplained and out of scope. The code blocks replay when
provider acceptance is uncertain.

## Review remediation

Work orders 04 and 05 are complete. Replay now uses current trusted first-launch
instructions, disables the unsafe missing-runtime fallback, holds every guarded
queue reservation boundary until acceptance or terminal failure, and retires
pending replay authority after later accepted prompts. SQLite metadata CAS now
compares complete JSON member identity. The original runtime timeout remains out
of scope.

Validation passed on 2026-10-06:

- `CGO_ENABLED=1 go test -trimpath -tags fts5 -p=2 ./...` passed across the backend.
- `go build -trimpath ./...` and changed-code `golangci-lint` passed.
- Focused replay, provenance, receipt-model, and SQLite CAS tests passed. The
  replay hold, runtime-disappearance, and restart-provenance tests also passed
  under the Go race detector.
- Final managed Playwright recovery specs passed: desktop 2/2 and phone 1/1.
- `python3 scripts/list-docs.py validate` validated 356 decisions and 1406
  specifications; `python3 scripts/lint-spec-files.py --all` passed.
- The PostgreSQL CAS test was invoked and skipped because
  `KANDEV_TEST_POSTGRES_DSN` is not configured.
- `gofmt -l` on touched Go sources and `git diff --check` passed.

An earlier unconstrained full run hit a timing-budget failure in an unrelated
SQLite performance test. That test passed in isolation, and the later complete
backend run passed with package concurrency limited to two.

## Risks

- Provider acceptance and durable receipt writes are not one atomic transaction.
  Refuse uncertain automatic replay rather than promise global exactly-once delivery.
- Legacy records can lack sufficient provenance. Reject ambiguous input safely.
- Queue admission must continue to consult the session-owned replay hold at each
  guarded reservation boundary as queue paths evolve.
- Full-row session writes can overwrite receipt/proof metadata. Use existing
  conditional metadata operations and cover stale snapshots.
- Shared ACP attachment code does not prove every provider supports native images.
  Keep capability fallback and refusal explicit.

## Public documentation

Public recovery guidance is updated in `docs/public/sessions-and-review.md` and
`docs/public/tasks-and-workflows.md`. It explains initial prompt replay, attachment
refusal, read-only restoration, and historical warning behavior.
