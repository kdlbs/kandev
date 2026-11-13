---
created: 2026-11-08
status: partial
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-001
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-002
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
  - ../../specs/tasks/system-design/prompt-attachments.md
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/tasks/system-design/workflow-completion-signal-payload-delivery.md
legacy_specs: []
---

# Implementation plan: Workflow prompt and handoff integrity

## Overview

Repair [#5178](https://github.com/kdlbs/kandev/issues/5178) by preserving an
explicit prepared-session launch brief through workflow composition and dispatch.
Keep [#5173](https://github.com/kdlbs/kandev/issues/5173) as a separate,
bounded investigation. Its normal reset control passes. its reported failure
does not yet have a confirmed cause.

Deliver Task 01 as the complete prompt-preservation slice, including native
transport evidence and public guidance. Task 02 is independent and cannot block
that fix. Both were executed sequentially in the primary session after the
explicit implementation request.

The task system owns both outcomes because it admits session input, selects
workflow recipients, and persists transition identity. Agents supplies runtime
capabilities and profiles. it does not choose which accepted brief to discard.
No UI-only specification or new system boundary is needed.

## Evidence and confidence

Investigation base and remote main:
`9aba7b2edd020f6ba0f92c2bfe318429723ea2ed`.
See [the evidence record](evidence.md) for canonical issue checks, current PR
heads, exact commands, probe setup limits, and observations.

- **#5178, high confidence:** a public `LaunchSession` call succeeds.
  A replacing template removes the explicit brief from SQLite message storage
  and the manager-bound launch prompt. Placeholder and direct-message controls
  pass. `launchStartCreated` omits the existing preserving option.
- **#5173, insufficient confidence for a fix:** normal turn-start reset clears
  the first signal and allows a later same-entry claim with the latest handoff.
  REVIEW runtime state can coexist with a Build workflow step. The issue lacks
  the event and identity evidence needed to distinguish failure from that
  valid mapping or from another admission path.

The probe did not launch a provider or exercise a live operator case.
Task 01 added the retained-plan, API raw-content and native ACP evidence.

### Assumption check

Confirmed user intent: preserve explicit constraints, retain existing authority
checks, stop at this uncommitted package, and avoid speculative companion fixes.
The source verifies separate direct, fallback, composed, and prompt-free seams.
No additional user decision blocks the confirmed fix.

The unresolved #5173 facts are diagnostic inputs. They belong to Task 02,
not an invented supersession policy. In particular, a requested structured
destination response is not equivalent to evidence that a transition occurred.
No question or approval prompt is needed for this handoff.

## Requirement and design traceability

The active
[initial-input requirement](../../specs/tasks/requirements/initial-task-brief.md)
already protects direct-message input through first-launch composition.
Reuse `REQ-TASKS-INITIAL-TASK-BRIEF-001`. clarify the missing explicit
`session.launch` surface with criteria 001.13 through 001.16.
Do not create a duplicate repair requirement.

The paired
[initial-input design](../../specs/tasks/system-design/initial-task-brief.md#explicit-prepared-session-launches)
adds the launch-origin distinction and carries the existing preserving option.
Its older direct-message admission, history, and brief-enrichment rules remain
the source for that separate seam.

| Requirement reference | Criteria used here | Design and delivery |
| --- | --- | --- |
| INITIAL-TASK-BRIEF-001 | .13-.16. existing .1, .2, .8, .10 provide direct-message controls | Initial-input design. Task 01 |
| SAVED-PROMPT-DELIVERY-001 | .6, .8, .11 | [Saved-prompt design](../../specs/tasks/system-design/saved-prompt-delivery.md). Task 01 |
| PROMPT-ATTACHMENTS-001 | .2, .3 | [Attachment design](../../specs/tasks/system-design/prompt-attachments.md). Task 01 |
| WORKFLOW-STEP-AGENT-START-OWNERSHIP-003 | .2, .8 | [Start-ownership design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md). Task 01 |
| WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001 / -003 | 001.1. 003.3, .6, .7 | [Completion design](../../specs/tasks/system-design/workflow-explicit-completion-signal.md). Task 02 |
| SIGNAL-PAYLOAD-DELIVERY-001 / -002 | 001.10, .13. 002.1 | [Payload design](../../specs/tasks/system-design/workflow-completion-signal-payload-delivery.md). Task 02 |

The table omits the common `REQ-TASKS-` prefix. Work-order frontmatter lists
the full IDs and the individual AC IDs.

The completion requirements and ADR 0015 already define duplicate/re-open
behavior. No signal requirement or design changes are proposed without the
Task 02 result. The payload and start-ownership documents retain their existing
draft statuses. this package does not promote them or rewrite their history.

### Existing delivery records and decisions

Read the linked packages as scoped implementation history:

- [Initial task brief](../initial-task-brief/plan.md) and
  [after recovery](../initial-task-brief-after-recovery/plan.md) own direct-message
  preservation. They do not cover this unified launch producer.
- [Workflow saved prompts](../workflow-entry-saved-prompts/plan.md) owns
  trusted expansion propagation, including replacement launches.
- [Fresh-start context](../fresh-start-workflow-context/plan.md) owns receipt
  recovery, not this explicit prepared launch. Keep its recorded results and
  replacement semantics intact.
- [Entry-bound delivery turns](../agent-delivery-turn-step-stamp/plan.md) owns
  current step/entry fencing. [Cross-step promotion](../cross-step-promotion-entry/plan.md)
  owns committed promotion entry IDs. Neither proves #5173.

The initial-input requirement and design link back to this new package.
No completed work order or verification result in those packages changes.

The /record assessment retains
[server-owned expansion](../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md),
[ADR 0015](../../decisions/0015-explicit-completion-signal-for-auto-advance.md),
and [entry-bound attribution](../../decisions/2026-10-22-entry-bound-delivery-turns.md).
This repair extends an existing admitted-input invariant with a local origin
flag. It creates no new security authority or persistence architecture.
The design and work order preserve enough rationale. no additional ADR is needed.
Any later same-entry supersession or destination-readback contract needs its own
decision after Task 02 establishes the actual identity boundary.

## Scope

### In scope

- Preserve explicit `start_created` launch instructions before composition,
  recording, deferral, replay, wrapping, and native dispatch.
- Distinguish explicit input from internally transported task descriptions.
- Preserve canonical saved-prompt, canvas, plan-mode, and attachment behavior.
- Reuse existing desktop and phone transcripts to show the accepted brief.
- Add the narrow public API/session explanation with implementation.
- Bound #5173 reproduction and record the next decision.

### Out of scope

- Production or permanent test edits during this design turn.
- Implementing an unproven #5173 patch or a new completion response schema.
- Disabling first-signal idempotency, restamping old turns, forced moves, or
  replacing a Review handoff after the destination already consumed it.
- Replaying historical lost prompts, new recovery receipts, migrations,
  runtime flags, provider permission changes, or a new read-only sandbox.
- UI controls, layouts, editor defaults, workflow prompt changes, profile/model
  compatibility, executor fallback, secret/actor policy, or parent checkout changes.
- Commit, push, PR, closure, comments, task transition, or autopilot.

## Technical approach

### Explicit launch origin

Add server-only `LaunchSessionRequest.PromptIsTaskDescription` with
`json:"-"`. Mark the internal HTTP task-creation producer before launch.
Audit config chat, Quick Chat, MCP creation, and any newer producer on the
implementation base. A client cannot select this classification through JSON.
An explicit prompt that equals the description remains explicit.

In `launchStartCreated`, preserve eligible explicit input using the existing
`startCreatedSessionOptions.preserveDirectPrompt`. Select it before
`admitOrDeferSeam2` can serialize the request. Keep `InitialCreatePrompt`
on its dedicated path. `NoInitialPrompt` remains prompt-free.
Do not change global step-template replacement or add every task description.

Reuse `stepPromptBodyWithOptions`, not another composer. Keep current
placeholder substitution, workflow instructions, and plan-mode ordering.
Retain the backend expansion/canvas decisions through final wrapping.
Do not extract trusted provenance from prompt-looking strings.

### Deferred and native delivery

The ceiling payload already contains `preserve_direct_prompt`.
Test its producer, repository round trip, replay consumer, and Send Now path.
Old payloads without the flag retain false; no historical inference or backfill.
A legacy queued explicit launch created before this upgrade can therefore still
be replaced by a destination step template on replay. Rejecting every unflagged
payload would also break legitimate task-description launches, so operators must
review pending launches; the public launch reference records this upgrade
exception. Entry-binding failure, changed recipient, cancellation and
supersession remain admission failures, with no prompt sent to a new owner.

The initial-submission callback still sees raw content and original attachment
identities. The prompt-origin repair must not create or overwrite that receipt.
When an explicit launch matches a pending receipt, persist a server-only
explicit-launch marker and use it during fresh-start recovery composition.
Task-description receipts do not carry the marker and keep replacement semantics.
Accepted or uncertain submissions retain the existing replay refusal. The merged
#5198 recovery path keeps its distinct task-creation semantics.

Inspect `postLaunchCreated`, the message `ToAPI` projection, the executor
launch request and native mock ACP trace. The manager boundary is necessary
but does not prove the final transport. Match native session/process/connection
identity from the same test's opt-in trace. A prepared session must have no
native prompt before the accepted explicit launch.

### Compatibility matrix

| Surface / provider shape | Input identity | Intended behavior | Evidence / unsupported-shape behavior |
| --- | --- | --- | --- |
| Structured ACP prepared session | Explicit brief | Replacing template then brief, or placeholder insertion once | Real SQLite service test and mock ACP E2E. Real vendor CLIs and remote executors are not claimed as exercised. |
| Passthrough prepared session | Explicit visible brief | Preserve visible instruction. omit hidden expansion | Backend launch controls. Unsupported runtime/profile remains an admission refusal. no new fallback. |
| Ordinary direct message | Already admitted content and trusted expansion | Existing preserving seam and brief enrichment | Existing `TestStartCreatedSession_InitialTaskBrief` and handler tests. no second enrichment. |
| HTTP task creation / empty launch | Server-owned description origin or existing fallback | Existing replacement/fallback behavior | Producer and real-launch controls. Do not infer origin from string equality. |
| Automatic workflow entry | Already-composed canonical prompt | Existing private composed seam, handoff once | Existing automatic-start and queued-handoff regressions. |
| Utility no-initial-prompt start | Server-only prompt-free request | No initial user message or prompt turn | Admission control case. The new preservation rule cannot create work. |
| Deferred launch / Send Now | Stored original request and preserving flag | Same explicit semantics through existing admission | Ceiling round trip and replay tests. stale entry/recipient is rejected. |

### Signal and handoff prerequisite

Reconstruct identity in this order:

1. Admitted user/peer operation and target session.
2. Turn ID with immutable step and entry stamps.
3. Signal claim and the exact durable winner.
4. Turn-end/subscriber event and final move CAS.
5. Committed destination entry, primary recipient, carry claim and dispatch.

Keep task runtime state, workflow step, signal acceptance, and committed
transition as separate facts. A new correction before commit can reopen the
source through normal admission. After commit, the destination owns its own
entry. No source event gains authority to overwrite it.
Task 02 decides whether any observed failure violates those existing rules.

## Tests

New test names below are proposed and must be added before the relevant
production correction. Existing names were verified against current source.
Use dedicated files rather than extending oversized test files.

| Case | Requirement / AC | Targeted test boundary and expected before/after |
| --- | --- | --- |
| Explicit brief, replacing/placeholder/empty template, retained task plan | INITIAL .13-.14 | New `session_launch_prompt_integrity_test.go:TestLaunchStartCreated_ExplicitPromptIntegrity`. Replacing case fails on base. after repair every stored and manager-bound prompt retains the brief once. |
| Known/unknown saved reference. canvas allowed/absent. plan mode. forged block | INITIAL .14. SAVED .6/.8/.11 | Same real launch matrix. Exactly one canonical server block per applicable kind. literal unknown reference survives. Mutating a saved record after preparation cannot replace an admitted snapshot. |
| Attachments and direct-message equivalence | INITIAL .14. ATTACHMENTS .2-.3 | Same launch matrix plus existing handler preserving fixture. Compare visible constraints and attachment identity, not identical Chat-specific brief enrichment. Invalid claims reach no turn/provider. |
| Explicit prompt equals description. internal description origin. empty and whitespace. invalid prompt JSON | INITIAL .16 | New `TestLaunchStartCreated_PromptOriginAndAdmission`, `TestLaunchSessionRequest_DescriptionOriginIsServerOnly`, and handler `TestTaskCreateDispatch_MarksDescriptionPrompt`. Existing empty/whitespace normalization stays intact. |
| No initial turn, already-composed automatic input, fallback after prior prompt | INITIAL .16. START .2/.8 | New admission controls. existing initial-prompt-dedup and direct-brief regressions. No extra message, wrapper, or turn. |
| Deferred capacity, replay, Send Now, stale entry, cancellation | INITIAL .15 | New `TestLaunchStartCreated_DeferredPromptIntegrity`. existing ceiling and Send Now fixtures. Require preserved flag after actual serialization, not a hand-seeded consumer-only value. |
| Pending raw receipt, failed admission, accepted/uncertain receipt controls | INITIAL .14-.16. existing recovery contract | New admission matrix uses `initialSubmissionDispatchCallbacks`. No bypass of callback CAS or new recovery mechanism. |
| Duplicate signal versus later admitted correction | COMPLETION 001.1 / 003.3. PAYLOAD 001.13 | Task 02 disposable `TestCorrectionHandoffProbe*`. Existing duplicate control retains first payload. later reset control accepts the new one. Do not define an after-patch expectation until a failing boundary exists. |
| Stale stamps, duplicate/reordered turn end, cancellation, separate Review | COMPLETION 003.6-.7. PAYLOAD 001.10 / 002.1 | Existing entry/carry tests plus Task 02 race barriers. At most one committed move. capture the actual destination prompt and preserve recipient isolation. |

Prefixes in this table abbreviate the full task-system IDs in work-order
frontmatter. Exact TDD, targeted race, document, and lint commands are in each
work order. PostgreSQL is required in any later repair of dialect-sensitive
claim logic. no such repair is selected here.

## E2E tests

Task 01 owns the complete launch outcome. Add:

- `apps/web/e2e/helpers/start-created-prompt-integrity.ts`, using
  `ApiClient.launchSession`, `listSessionMessages`, and the existing opt-in
  native trace from `helpers/transient-turn-runtime-continuity.ts`.
- `tests/session/start-created-prompt-integrity.spec.ts`, project `chromium`:
  prepare an independent CREATED session with a retained task plan.
  Submit the complete explicit read-only brief on a replacing Review template.
  Compare the first ACP prompt with its stored raw/visible message.
  Include a placeholder control and reload.
- `tests/session/mobile-start-created-prompt-integrity.spec.ts`, project
  `mobile-chrome`: the same supported API launch, then open the target
  session's existing phone Chat. The brief remains reachable once after reload,
  without horizontal overflow or duplicate transcript content.

Map both files to INITIAL .13-.14 and use backend tests for INITIAL .15-.16.
Use a trace path private to the isolated E2E backend and a dedicated mock
profile. Match native session identity, not log order across unrelated sessions.
Use only inert fixture instructions. native trace inspection proves delivery,
not that a real model obeyed a read-only request.

### Mobile-parity assessment

No rendered composition, navigation, controls, or copy changes are necessary.
The existing `SessionMobileLayout` and mobile session-selection pattern remain
the phone exemplar. Phone Chat keeps its own full-height transcript scroll
surface and existing prompt navigation. Tests use existing touch entry points.
There is no new UI to sketch. An ASCII preview introduces unrelated changes.
No new localized product text is planned. Any required product copy discovered
later must use the seven-language contract and the Traditional Chinese generator.

### Public documentation impact

The /docs-maintainer search covered public session, workflow and WebSocket
references, root README, and screenshot guidance.
The design turn changed only internal intent records. Task 01 updated
these existing public pages when implementation shipped:

- `docs/public/websocket-api.md`: reference section for explicit
  `start_created.prompt` preservation and default-input distinction.
- `docs/public/sessions-and-review.md`: session how-to guidance on supplying
  an independent brief. Delivery preservation is not a read-only permission mode.
- `docs/public/workflow-tips.md`: scope template replacement wording to
  automatic/default content where needed, retaining `{{task_prompt}}` rules.

No new page, navigation entry, screenshot, or deployment guidance is needed.
Coordinate the final workflow wording with open #3420.
#5173 guidance cannot claim transition success from `already_signaled`.

## Work orders

- [x] [Task 01: Preserve explicit prepared-session launch input](task-01-preserve-explicit-launch-input.md) (`done`)
- [ ] [Task 02: Reproduce same-entry correction and handoff ordering](task-02-reproduce-correction-handoff.md) (`in_progress`, partial bounded investigation; no patch)

Both have no blocking dependency on each other. Default order is 01, then 02.
No concurrent agents or new persistent tasks/sessions are authorized.

## Verification results

Planning evidence:

- Canonical issue and dependency-head checks completed. #5178 was assigned to
  `carlosflorencio`. #5173 remains unassigned. Neither was closed or commented.
- Corrected disposable overlay: expected #5178 replacing-template failure;
  placeholder, preserving direct-message, and normal #5173 reset controls pass.
- Focused existing controls pass in MCP handlers, SQLite repository, and
  orchestrator. [Exact commands and limits](evidence.md#existing-control-suite).
- `python3 scripts/list-docs.py validate`: passed, 439 decisions and 1946
  specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- PR-documentation `validateCoverage` preflight: `covered`, both work orders,
  no errors. This used the proposed launch source path to force coverage
  validation for the currently documentation-only diff.
- All local Markdown file targets resolve. Whitespace checks include the
  untracked package files. `git diff --check` passed.

Implementation results:

- Task 01: the launch producer now carries server-only description provenance
  and selects the existing preserving composer for explicit nonempty input.
  `TaskHandlers.dispatchTaskSession` marks ordinary creation while Quick Chat
  remains explicit. Seam 2 and ceiling replay carry the same value. Focused
  backend tests cover the replacing/placeholder/empty templates, workflow
  instructions, plan mode, canvas allowed and absent, saved-prompt expansion,
  attachment identity, and pending-receipt admission success and CAS failure.
  Desktop and phone E2E compare the complete stored raw prompt with the native
  ACP prompt and correlate its native identity. Public launch, session, and
  workflow guidance was updated.
- Task 02: partial bounded investigation only. Existing duplicate, stale-entry,
  concurrent-transition, and later-reset controls pass. No failing invariant was
  found, but the three-turn route matrix and both deterministic race barriers
  remain unrun, so the work order remains in progress and no production change
  or issue assignment was made.

Verification passed:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^TestLaunchStartCreated_' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/mcp/handlers -run '^TestHandleCreateTask_InitialPromptUsesRequestedWorkflowStepMode$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestLaunchSessionRequest_DescriptionOriginIsServerOnly|TestStartCreatedSession_InitialTaskBrief|TestWorkflowAutoStartCreatedNonEmptyPromptPreservesQueuedHandoff|TestSendQueuedNowConsumesCeilingLaunchAndPreservesWorkflowPrompt)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run '^(TestTaskCreateDispatch_(MarksDescriptionPrompt|QuickChatPromptIsNotADescription)|TestDispatchTaskSessionMarksExplicitCreationPrompt|TestStartAgentForNewTask_SetsDeferredStart)$' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator ./internal/task/handlers ./internal/prompts/service ./internal/sysprompt ./internal/task/models -count=1)
(cd apps/backend && go build -trimpath ./...)
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 golangci-lint run ./internal/orchestrator ./internal/task/handlers --timeout=20m --concurrency=2)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/start-created-prompt-integrity.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-start-created-prompt-integrity.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Limits: the native trace covers local mock ACP dispatch, not a real vendor
model's obedience or a remote executor. No PostgreSQL run was required because
the change does not alter dialect-sensitive claim or storage logic. #5173
remains open and unassigned. Changes are unstaged and uncommitted.

## Risks

- A broad preservation switch appends historical task descriptions to replacing
  templates. The internal producer flag and compatibility regressions prevent it.
- Saved definitions and canvas guidance require server provenance. prompt text
  alone cannot establish it. Recomposition can duplicate wrappers.
- The legacy replay default does not repair already-lost historical prompts.
- Future branch movement can overlap auth/profile/queue work. Resolve current
  main and relevant PR heads again before edits.
- #5173 needs event and identity evidence. Its requested structured response
  is a separate unresolved contract. No corrective patch or completion claim
  is justified by the present control.
- Real provider obedience, remote runtime behavior, and historical incident
  reconstruction remain outside the local probe's evidence.
