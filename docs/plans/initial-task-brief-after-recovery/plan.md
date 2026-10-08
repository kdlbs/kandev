---
created: 2026-10-07
status: implemented
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
legacy_specs: []
---

# Fix plan: Initial task brief after recovery

## Overview

Preserve the original brief when the first direct message reaches an ordinary
session recovered into WAITING_FOR_INPUT without prior input. Admission and
dispatch are covered by sequential backend tests, and the restart flow is proven
in desktop and phone Chat. Review remediation also preserves accepted prompt
context through fresh-runtime fallback and closes both sides of the first-message
queue handoff. Both work orders are complete. The mobile recovery test holds the
prompt-free boundary through page recovery and restores the prior auto-start
preference afterward.

The task system owns this repair because it owns the accepted prompt boundary,
stored user content, and agent dispatch. The existing transcript renderer owns
presentation and needs no new context-preservation policy.

## Evidence and confirmed root cause

Investigation used read-only task/session APIs and retained backend logs from
the user's instance on port 38429. The task is
`ad469646-9ea4-4591-851b-5f4b45bf40d9`; its only session is
`d436b5fa-605b-4a24-a3c2-e1538b302298`. Planning checkout:
`330e02a478`; live checkout at inspection: `b3f207b2e7`. The failing eligibility
predicate is present in both. No provider process was launched by the investigation.

Times below are Europe/Lisbon, UTC+01:00:

| Time | Evidence |
| --- | --- |
| Oct 6, 15:14 | Session and worktree prepared; log reports `agentctl execution created (agent not started)`. |
| Oct 6, 21:08 | Prepared execution removed during backend shutdown. |
| Oct 7, 09:42 | Recovery creates a fresh Cursor conversation with an empty resume token. Lifecycle logs `no task description and no resume context needed, marking as ready`, followed by STARTING to WAITING_FOR_INPUT. |
| Oct 7, 14:34 | Rebase instruction is the only stored user message, with `prompt_index: 1`; dispatch logs contain `prompt_length: 82`. The nonempty task description remains saved. |

The agent's response reports missing prior context. Cancellation follows
dispatch and therefore did not cause the omission. Evidence is stored history
and lifecycle/dispatch logs, not a raw ACP wire capture.

`MessageHandlers.wsAddMessage` derives `startCreatedSession` from CREATED state.
`eligibleForInitialTaskBrief` requires that flag before constructing a candidate.
Recovery had already made this never-prompted session WAITING_FOR_INPUT, so the
repository never received the combined candidate. It stored the instruction
alone, and `forwardMessageAsPrompt` sent it through `PromptTask`. Once prompt #1
existed, the synthetic task-description row disappeared under its existing rule.

The prior requirement explicitly restricted eligibility to CREATED. This is a
missing recovery condition in the same capability, not an instruction to change
recovery policy. Extend its terms and criteria `.11` and `.12`; retain `.1` to
`.10` and their existing safeguards. The paired design keeps the existing
durable sequence transaction authoritative. No new ADR is needed for this local
extension; the session-open and saved-expansion ADRs remain authoritative.

## Scope

### In scope

- Ready, never-prompted ordinary sessions retain the brief in their first direct message.
- Existing CREATED behavior, first-boundary races, fallback reservations, and rollback remain correct.
- Ordinary prompt/resume and feedback-queue dispatch receive committed content without a second agent launch.
- Saved references, attachments, plan comments, and idempotent request identities remain intact.
- Desktop and phone restart/reload evidence and a concise update to the task guide.

### Out of scope

- Automatically sending the brief on open, recovery, or agent boot.
- Replaying accepted submissions, repairing this existing conversation, or rewriting history.
- Changing workflow transitions, explicit-start behavior, capacity policy, terminal recovery, or Office/utility context.
- New API fields, database tables, migrations, runtime flags, or provider-specific branches.
- Transcript fallback changes, new controls, locale keys, or layout changes.

## Technical approach

### Admission and preparation

In `apps/backend/internal/task/handlers/message_handlers.go` and
`message_handlers_initial_task_brief.go`, distinguish brief candidacy from
`startCreatedSession`. Preserve existing CREATED/redirection handling and admit
a candidate for a resolved WAITING_FOR_INPUT ordinary session when its durable
prompt marker is absent. Keep existing blocked-state and workflow gates intact;
this repair does not admit messages solely because a runtime exists.

Expose a narrow internal service lookup over
`repository.MessageRepository.HasUserPromptHistory` in
`apps/backend/internal/task/service/service_messages.go`. For ready sessions,
skip candidate preparation when history exists; propagate read failures before
message admission. Do not use a provider token, lifecycle-only turn, visible
message count, or returned ordinal as the history decision.

Reuse `selectInitialTaskBriefCandidate` in
`apps/backend/internal/task/repository/sqlite/message_prompt_index.go` for the
final atomic winner. An existing sequence row, including `last_seq = 0`, consumes
eligibility. Keep stale-description refresh, rollback, idempotency, and
plan/preview feedback transactions unchanged. A preflight false result is never
an ownership claim. Repository behavior should need only regression coverage.

### Delivery and queues

Use the committed `message.Content` and selected trusted expansion. A ready
recipient continues through `PromptTaskWithPromptContext` and its existing
resume retry; it must not call `StartCreatedSession` or reapply the workflow
template. Carry the acceptance-time saved-prompt context, its prepared bit, and
validated entity references into missing-runtime fallback. Verify the accepted
expansion against mutations after admission, including an accepted empty
expansion. Preserve attachments and entity references.

Audit `initialTaskBriefQueued` in `wsAddMessage` and
`MetaKeyInitialTaskBriefDispatchPending` in the queue drain. Keep contention
handling only for first-boundary contenders. The selected first-message commit
and queue insertion share per-session admission; while the selected dispatch is
active, a losing contender waits. Dispatch completion retries the queue drain,
and a contender arriving after completion uses the normal enqueue fast path.
Later ready-session messages retain normal dispatch and queue semantics. Atomic
comment-bearing queues still notify and drain the selected ready-session prompt
without assuming it owns a created-session launch.

### Compatibility

| Shape | Intended behavior | Evidence and fallback |
| --- | --- | --- |
| Cursor or other structured ready agent | Preserve first brief through ordinary prompt delivery regardless of an existing conversation ID | Shared handler capture plus mock ACP E2E; no live-provider claim |
| Structured agent missing its runtime at send | Preserve committed prompt through normal lazy resume | Real orchestrator/executor-seam regression; existing recovery errors remain visible |
| Passthrough ready session | Same visible brief/instruction, no hidden saved-reference expansion | Handler passthrough case; existing PTY dispatch remains authoritative |
| CREATED ordinary session | Existing starter and workflow-preservation behavior | Existing CREATED, redirect, and workflow-template tests |
| Office, ephemeral Quick Chat, configuration, terminal/blocked session | Existing eligibility and admission rules | Explicit negative table cases; no new provider fallback |
| Session with accepted or reserved input | Ordinary follow-up without brief | Durable-marker cases, deletion/restart, and fallback contention |

## ASCII UI preview

```text
UI-01: First message after prompt-free recovery (desktop and phone)

Observed before                 Proposed
+---------------------------+   +---------------------------+
| User #1                   |   | User #1                   |
| Rebase on the target PR.   |   | Original task brief       |
|                           |   |                           |
| Agent lacks the objective |   | Rebase on the target PR.   |
+---------------------------+   | Agent receives both       |
| Composer           [Send] |   +---------------------------+
+---------------------------+   | Composer           [Send] |
                                +---------------------------+
```

Entry: recover a prepared task after backend restart, then submit through Chat.
Before sending, keep the existing synthetic brief and no stored user prompts.
After sending, one combined stored prompt replaces that row; reload preserves it.
Criteria: `AC-TASKS-INITIAL-TASK-BRIEF-001.1`, `.2`, `.7`, `.11`, `.12`.

Desktop retains the Chat pane. Phone retains the full-height Chat destination
from `components/task/task-layout.tsx` and `SessionMobileLayout`, with one
vertical transcript scroll owner and the existing fixed composer/safe-area
behavior. This changes rendered content, not controls or composition. Long
content uses current expansion/download controls. Required structure is brief
before instruction, one prompt, and normal follow-up. Borders are illustrative.

## Tests

All suffixes below refer to `AC-TASKS-INITIAL-TASK-BRIEF-001`.
Test names marked new are implementation work, not tests executed in this design turn.

| Criteria | Test boundary |
| --- | --- |
| `.1`, `.2`, `.11` | New `TestWSAddMessage_InitialTaskBriefReadySession` in `message_handlers_initial_task_brief_test.go`: WAITING_FOR_INPUT, no marker, lifecycle-only boot and provider conversation metadata; persisted prompt #1 and ordinary dispatch each contain both texts once; created starter has zero calls. This is the primary red regression. |
| `.3`, `.5`, `.6`, `.12` | New `TestWSAddMessage_InitialTaskBriefReadySessionAdmission`: ordinary follow-up bypasses candidacy/forced queue; read failure prevents admission/dispatch; rollback retry, same-ID replay, concurrent sends, and fallback reservation retain their rules. |
| `.3`, `.5`, `.6`, `.11` | Extend `TestInitialTaskBriefAdmission` and `TestInitialTaskBriefAdmissionPostgres`: ready state, accepted/deleted marker, zero reservation, restart, two contenders, and stale snapshot. Assert no migration or sequence reset. |
| `.4`, `.8`, `.9`, `.10`, `.11` | Extend handler table cases for equality/empty brief, structured/passthrough, excluded task kinds, saved expansions mutated after acceptance, attachments, entity references, plan mode, and atomic feedback queue. |
| `.1`, `.8`, `.11`, `.12` | Extend `TestPromptTask_InitialTaskBriefAfterRecovery` in `prompt_launch_fallback_test.go`: a real missing-runtime resume and fresh executor launch retain the accepted saved-prompt block and validated entity snapshot once after the definition changes; a second case preserves an accepted-empty snapshot. Keep `TestSessionOpenRecoveryStatusAndLaunch` as the prompt-free recovery compatibility check. |
| `.3`, `.5`, `.11` | `TestQueueUserPrompt_T2InitialTaskBriefContenderOwnership` uses the real queue and drain for a contender arriving before owner completion and one arriving after it. `TestWSAddMessage_InitialTaskBriefConcurrentReadySessionQueuesUnselectedCandidate` covers the handler-side selection and queue metadata. |
| `.2`, `.7` | Existing `hooks/use-processed-messages-fallback.test.ts` plus rendered E2E; do not change synthetic-row eligibility. |

## E2E tests

Extend `e2e/tests/chat/initial-task-brief.spec.ts` (`chromium`),
`mobile-initial-task-brief.spec.ts` (`mobile-chrome`), and their shared helper.
Keep the existing prepared-session cases and add the restart case in each file.

1. Preserve the user's prior `prevent_auto_start_agent_on_open` preference and
   enable it for this recovery flow.
2. Create a described ordinary task with `prepare_session: true`; wait for
   workspace preparation, open `/t/<task-id>`, then start the prepared workspace
   through the production `session.launch` resume path without a prompt.
3. Wait for the same session to reach WAITING_FOR_INPUT with an agent execution;
   assert zero stored user messages. Restart the backend and reload the task.
4. Wait again for that same session to reach WAITING_FOR_INPUT and assert zero
   stored user messages before submission. This prevents accidentally passing
   through the old CREATED path and keeps the first message prompt-free.
5. Submit an instruction with the real composer. Assert prompt #1 contains both
   texts once, the saved task description is unchanged, and the session ID remains.
6. Reload and assert the combined prompt remains. Send a second message and assert
   ordinary delivery without the brief. Check no document horizontal overflow.

Use `ApiClient.listTaskSessions`, `listSessionMessages`, and the shared page
object with bounded causal waits. Restart only the worker fixture. No live-user
instance, arbitrary sleep, direct database edit, or user-data repair is allowed.
These flows cover `.1`, `.2`, `.3`, `.7`, `.11`, and `.12`.

## Work orders

- [x] [Task 01: Preserve the first brief for ready sessions](task-01-ready-session-admission.md) - done
- [x] [Task 02: Prove restart behavior in Chat](task-02-restart-chat-evidence.md) - done; depends on Task 01

## Related packages and documentation

[The completed initial-task-brief package](../initial-task-brief/plan.md) retains
its completed work orders and recorded results. This package extends its existing
test files with recovery cases; it does not relabel prior validation as recovery
evidence. Keep both existing desktop/mobile scenarios in the final test run.
The session-open recovery package and ADR retain their prompt-free policy.

Task 02 updated `docs/public/tasks-and-workflows.md` to explain that the first
Chat prompt keeps the task description after prompt-free recovery. A search of
the root README and public documentation found no conflicting wording. No new
page, screenshot, or publication build was needed for this prose correction.

## Verification results

Implementation is complete. Final code and documentation checks are recorded
below. PostgreSQL parity remains unverified because this environment has no
`KANDEV_TEST_POSTGRES_DSN`.

- Backend focused tests, initial-brief race tests, `go vet`, and `make -C
  apps/backend build` passed.
- Chromium and mobile-chrome each passed both initial-brief scenarios, including
  prompt-free backend recovery, reload, first prompt, and ordinary follow-up.
- Frontend typecheck, changed-file ESLint, the processed-message fallback tests,
  and the E2E sleep ratchet passed.
- `node --test scripts/validate-public-docs.test.mjs` passed 62 tests, and
  `node scripts/validate-public-docs.mjs` validated 47 published pages.
- `python3 scripts/list-docs.py validate` validated 360 decisions and 1427
  specifications. `python3 scripts/lint-spec-files.test.py` passed 36 tests,
  and `python3 scripts/lint-spec-files.py --all` passed.
- Changed-path `.github/scripts/pr-docs.cjs:validateCoverage` returned
  `covered` with no errors; `git diff --check` passed.
- Review remediation focused backend tests passed across handlers, service,
  SQLite, orchestrator, and executor. The queue ownership regression passed both
  arrival schedules; fallback coverage passed the accepted expansion, validated
  reference, definition-mutation, and accepted-empty cases.
- Review remediation race tests passed across handlers, service, and SQLite
  with the selector `InitialTaskBrief|ConcurrentInitialBrief|HasUserPromptHistory`.
  The selected test names include the renamed concurrent handler regression and
  `TestHasUserPromptHistoryDelegatesAndReturnsReadErrors`.
- Review remediation `go vet` passed for the five affected backend packages, and
  `make -C apps/backend build` passed. PostgreSQL parity remains unverified
  because `KANDEV_TEST_POSTGRES_DSN` is unset.
- The first post-review CI attempt exposed
  `TestPromptTask_QueuedAcceptedTurnIdentityReadFailurePreservesExecution`.
  Local full-package reproduction traced it to a direct-prompt guard that also
  matched unrelated accepted queue work. The guard now checks the dedicated
  initial-brief marker, while queue draining keeps its accepted-dispatch guard.
  The full orchestrator package and focused work-order and race suites pass
  after this correction; fresh remote checks for the resulting commit remain
  the delivery gate at the time of that review.

Design validation on 2026-10-07:

- `python3 scripts/list-docs.py validate` passed: 360 decisions and 1425 specifications.
- `python3 scripts/lint-spec-files.test.py` passed: 36 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- `.github/scripts/pr-docs.cjs:validateCoverage` passed with both new work orders,
  their linked documents, and prospective handler, service, and E2E changed paths.
  This is structural package evidence, not final implementation coverage.
- `git diff --check` passed; new work orders are unstaged and uncommitted.

### PR E2E triage (2026-10-08)

The first PR E2E run after remediation, `37688349104` on head
`8e296c1d0d0046208654700214c4fb504da63d70`, failed the mobile and desktop queue
admission cases described in Task 01. The base had advanced to
`c40f6d96d726b8ff765c038ceabacf50405ba9be`; the affected queue tests, helpers,
and backend queue implementation were unchanged by that base advance.

Both cases passed without retries in local synthetic-merge replays against the
then-current base `c40f6d96d726b8ff765c038ceabacf50405ba9be`. The mobile shard's
assigned project completed with 58 passed and one skipped. The desktop case
passed at its assigned position, but the wider replay was not clean: a newer
mobile fork-comparison test could not read the linked worktree's `.git` pointer
from the Docker container. The downloaded shard manifest no longer matched the
current test catalog, so its file assignment was replayed directly.

The base then advanced to `db0348d1623ea9dc06e28c85afbcba9e4c087c06`. No
files changed between `c40f6d96d726b8ff765c038ceabacf50405ba9be` and that base
overlap the PR's changed-file set. On synthetic merge
`d5e946c24cf5b38b7cf2b948185b817195556d63` (head
`8e296c1d0d0046208654700214c4fb504da63d70`, base `db0348d1623ea9dc06e28c85afbcba9e4c087c06`),
the desktop and mobile queue-admission cases both passed with retries disabled
(2 passed, 49.6s). Docker was unavailable to the runner, so this focused replay
used host mode.

During the subsequent full PR check run, `main` advanced to
`1257838968f5c92305a858427cdf723a87b0a882`. No files changed between
`db0348d1623ea9dc06e28c85afbcba9e4c087c06` and this base overlap the PR's
changed-file set. On synthetic merge `8ab712b5b58889b41a335d5a9977dcd30ba68475`
(head `b6b3fd7ff488abfba3529d3031de27f3017160a1`, base
`1257838968f5c92305a858427cdf723a87b0a882`), both queue-admission cases passed
with zero retries (2 passed, 45.3s). Docker was unavailable, so the replay used
host mode. Local replays do not replace exact-head PR checks, and PostgreSQL
parity remains unverified because `KANDEV_TEST_POSTGRES_DSN` is unset.

## Risks

- The selected-dispatch ownership marker must be released on every dispatch exit so a first-boundary contender cannot remain queued indefinitely.
- Conflating candidacy with process start can replace a recovered conversation or run the workflow template again.
- A read-before-write check alone can duplicate briefs under concurrency; final selection must remain transactional.
- An accepted prompt deleted from the transcript or a zero fallback reservation must still suppress insertion.
- Existing saved expansions and feedback queues need acceptance-time identity, not a second mutable lookup.
- Browser restart tests must prove WAITING_FOR_INPUT before send; a passing CREATED test does not cover this defect.
- PostgreSQL tests require a disposable configured database; a skip must be reported as missing parity evidence.
- The reported conversation already consumed prompt #1 and needs manual context recovery outside this package.
