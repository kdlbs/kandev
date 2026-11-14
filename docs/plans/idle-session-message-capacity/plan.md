---
created: 2026-11-02
status: complete
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
  - REQ-AGENTS-SESSION-CEILING-002
  - REQ-AGENTS-SESSION-CEILING-003
  - REQ-TASKS-RESUME-PROMPT-QUEUE-003
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
  - ../../specs/tasks/system-design/resume-prompt-queue.md
legacy_specs: []
---

# Implementation Plan: Idle-session message capacity

## Overview

Queue a new message turn when the enabled session ceiling has no capacity.
The message must retain its recipient and payload until ordinary queue delivery
can acquire a slot. First implement the complete backend admission path.
Then update the existing help and prove the desktop and phone outcomes.

Source: [issue #4998](https://github.com/kdlbs/kandev/issues/4998).
The issue is assigned to `carlosflorencio`.
This package implements message-turn admission and its help, documentation, and rendered coverage.

## Investigation and planning basis

The investigation used repository head
`f648e79f9ecc7d5db0db301f20d0b23c7cb44d97` on 2026-11-02.
The canonical issue had no attachments or follow-up comments.

Two separate causes explain the reported behavior:

1. `PromptTask`, its context wrappers, and `PromptTaskWithDeliverySubmissionID`
   pass `launchOriginManual`. Cold resume therefore permits a capacity override.
   Existing `TestPromptTask_ManualOverCeilingIsAdmitted` explicitly expects this.
2. `ensureSessionRunningWithAttemptAndBinding` returns from
   `sessionAlreadyPromptReady` before the cold-resume capacity gate.
   A warm idle runtime therefore starts a new turn without admission, even when
   the caller uses `launchOriginAutomatic`.

The temporary `TestReproIssue4998WarmIdlePromptBypassesCeiling` used the real
orchestrator and SQLite repository with a mock provider. It seeded one running
session and a different warm `WAITING_FOR_INPUT` session, with capacity one.
It then submitted the same follow-up through manual and automatic prompt paths.

| Path | Capacity | Population before | Provider calls | Population after |
| --- | --- | --- | --- | --- |
| Public manual prompt | 1 | 1 | 1 | 2 |
| Internal automatic prompt | 1 | 1 | 1 | 2 |

The assertion expected zero provider calls while capacity was full.
Both subtests failed for that behavioral reason. The reproduction command was:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestReproIssue4998WarmIdlePromptBypassesCeiling$' -count=1 -v)
```

The command exited 1. The temporary test was removed after evidence collection.
No production or permanent test changes remain from this investigation.

The current manual-prompt behavior conforms to part of the existing launch
contract. The automatic warm-prompt bypass violates its automatic admission rule.
This package therefore combines an implementation repair with a draft contract
amendment. It follows the issue's primary expected behavior: ordinary message
turns wait, while explicit execution controls retain their overrides.
That policy is a planning choice for review, not a claim of separate user approval.

The ceiling is install-wide, despite the issue's workspace wording.
Cancelled and failed targets are currently rejected by the agent-message path.
Preserve that behavior. Preserve the supported explicitly pinned completed follow-up.

## Scope

### In scope

- Shared admission for warm and cold message turns, including automatic delivery.
- Attempt-owned capacity across startup, dispatch, rollback, and stale callbacks.
- Durable queue admission for direct UI/API and agent-message requests.
- Release-driven retry, live capacity changes, and the existing periodic backstop.
- Independent sibling queues, current task-priority ranking, and queue policy.
- Existing completed-follow-up eligibility, accepted-delivery identity, and payload retention.
- Settings help, public documentation, and desktop/mobile E2E evidence.

### Out of scope

- Per-workspace ceilings, a new option, a release toggle, or an active default ceiling.
- A hard cap on explicit Start, Resume, Send Now, or legacy Run next.
- Raw terminal input, new permission-reply policy, or interruption semantics.
- A database migration, a new durable queue, or another scheduler goroutine.
- Global fairness across automatic launches, automation runs, and prompt queues.
- Changing task-level launch ownership or resuming queued messages at backend boot.

## Technical approach

The [agent design](../../specs/agents/system-design/session-concurrency-ceiling.md#message-turn-admission)
owns capacity. Tasks continue to own queued input and session identity.

### Admission and ticket ownership

Add a typed internal capacity intent and turn ticket in new focused
`internal/orchestrator/message_turn_capacity*.go` files.
Keep user authority and `launchOrigin` separate from capacity intent.
Normal message paths use bounded intent. Explicit execution controls alone
request the current manual override.

Extend `session_ceiling.go` under its existing mutex. Count the union of
persisted counted sessions, launch reservations, and attempt-owned turn tickets.
Keep tickets across cold startup's intermediate waiting state.
Change `ceiling_release.go` only as needed to preserve separate ownership.
An older startup or prompt callback cannot free a successor's ticket.

Consult the guard before the warm return in `promptTask`.
Cover created-message launch and completed-follow-up resume before process work.
Guard orchestrated passthrough queue input before its running claim and PTY write.
Preserve synchronous publication ordering after guard release.

### Queue ownership and wake-up

Use the existing message queue for pending payloads.
Add one process-local capacity wait per queue session incarnation.
Store the accepted head identity in that registry, not another prompt copy.
Keep two sibling sessions independent of the task's single launch-deferral record.

Restore a refused queued dispatch without changing its ID, position, or payload.
Suppress its immediate tail drain. Register the restored head and request one
recheck after dispatch guards release, closing the missed-wake window.

Extend `ceilingSweepTick` after its existing launch pass.
Reuse release signals, capacity-change signals, and its 20-second backstop.
Among eligible message waits, reuse the priority rank in `orchestrator/queue`.
Use original queue time and stable identity tie breakers.
Recheck all existing automatic reservation barriers before retry.

The registry starts empty after restart. Existing authorized session activity
can activate a waiting session again. Do not scan durable pending messages at
boot to launch their agents. This preserves
`REQ-TASKS-RESUME-PROMPT-QUEUE-003`.

### Request acceptance

`MessageHandlers.wsAddMessage` must choose reserved dispatch or durable queue
admission before its successful response. Reuse
`CreateQueuedMessageIdempotent` for the capacity-wait branch.
Transfer an admitted ticket explicitly to asynchronous dispatch.
Do not use an observation-only precheck as the final gate.

Cover the context-aware prompt wrappers and delivery-submission wrapper.
Keep existing submission IDs and acceptance/uncertainty handling.
Preserve processed turn-start metadata, prompt context, references, attachments,
model, mode, and first-message ownership.

The agent-message handler reports the existing queued result and entry ID.
Its explicitly allowed completed follow-up can persist trusted pending intent
without launching. Replay revalidates that exact destination before resume.
Raw client metadata cannot grant this intent or an override.
Failed or cancelled targets keep their existing error.

### Compatibility matrix

| Path | Identity/capability | Enabled behavior | Verification | Fallback |
| --- | --- | --- | --- | --- |
| Chat and `message.add` | Selected task/session incarnation and message identity | Reserve or commit pending input before success | Handler integration and desktop/mobile E2E | Reject admission failure without clearing draft |
| `message_task_kandev` | Resolved recipient and sender metadata | Report queued at full capacity | Real-controller handler tests | Preserve missing, stale, and terminal errors |
| Automatic structured prompt | Existing queued dispatch and delivery identity | Gate warm and cold dispatch | Orchestrator race tests | Restore the exact entry after refusal |
| Orchestrated passthrough prompt | Existing queue identity and split running publication | Gate before running claim and PTY write | Provider-boundary tests | Retain current recovery for unsupported or ambiguous delivery |
| Pinned completed follow-up | Existing explicit completed-resume permission | Queue without launching, then revalidate | Agent-message integration tests | Never revive failed or cancelled targets |
| Start/Resume/Send Now/Run next | Explicit owned execution intent | Preserve override and audit | Existing override tests plus new separation cases | Existing eligibility and ownership gates |

Structured provider coverage comes from the shared dispatch boundary.
This package does not claim live-provider validation for every agent family.

## ASCII UI preview

### UI-01: Message accepted while capacity is full

Before: a follow-up to a warm idle session immediately starts another turn.
After: the existing pending-message surface shows the accepted input.

```text
Desktop task chat                    Phone task chat
+--------------------------------+   +--------------------------+
| Conversation                   |   | Task / conversation      |
|                                |   |                          |
| Queued (1)   Auto-run [ON]      |   | Queued (1) Auto-run [ON]  |
| Follow-up message              |   | Follow-up message        |
| [existing queue actions]       |   | [existing queue actions] |
|                                |   |                          |
| [composer]              [Send] |   | [composer]        [Send]  |
+--------------------------------+   +--------------------------+
```

The queue retains its existing internal scroll region and touch actions.
The task layout retains viewport and safe-area ownership.
No new overlay, queue banner, or page composition is required.
After capacity release, the queued item becomes one new response turn.
Auto-run OFF leaves the item pending. Map this view to criteria 003.3, .4, and .8.

### UI-02: Existing Session capacity help, both viewports

```text
Session capacity
Limit automatic sessions                    [ON]
Maximum sessions                            [ 1 ]
Applies to automatic starts and new message turns in all workspaces.
Start, Resume, and Send Now can exceed the limit.
Workflow WIP is a separate limit.
```

Keep the existing Settings controls and page-level Save/Reset actions.
Phone fields retain their current stacked layout and touch targets.
The help wraps within the card. Exact translated wording is illustrative.
Map this view to criterion 002.7.

## Tests

The regression tests below are implemented and passing.

| Acceptance | Planned evidence |
| --- | --- |
| `001.1-.3`, `003.1-.2` | `message_turn_capacity_test.go`: `TestMessageTurnCapacity_WarmIdleRefusesAtLimit`, cold startup, mixed population, concurrent final-slot requests |
| `001.6`, `003.2` | Same suite: launch confirmation during a pending turn, delayed callbacks, failed old attempt, current successor ticket |
| `003.3`, `.7-.8` | `message_capacity_admission_test.go` and `message_task_capacity_test.go`: real request acceptance, persistence failure, full queue, completed intent, rejected terminals, duplicate receipt |
| `003.4-.6` | `message_turn_capacity_wait_test.go`: release-before-registration, repeated refusal, priority/FIFO, two sibling queues, Auto-run OFF, restart without launch, later activation |
| `001.8-.9`, `003.8` | Disabled/failing-lister path, increase/disable wake-up, unchanged explicit override, accepted continuation |
| `002.7` | Existing settings component tests and focused desktop/mobile rendered help assertions |

Use the real controller and repository for handler admission decisions.
Use barriers for concurrency and provider call counters for negative assertions.
Do not substitute a mocked refusal for the regression proof.

## E2E tests

Task 02 owns new `session/message-turn-capacity.spec.ts` in `chromium` and
`session/mobile-message-turn-capacity.spec.ts` in `mobile-chrome`.
Use the existing session-resume and mobile queue-management tests as examples.

Prepare an idle target and a separate long-running filler with capacity one.
Send a follow-up through the actual composer. Observe one pending item, retained
payload after reload, and no second running session. Release the filler and
observe one response to that accepted input. Also cover Auto-run OFF.
Restore saved capacity and dispose of scenario data after each test.

## Work orders

- [x] [Task 01: Bound and queue message turns](task-01-bound-and-queue-message-turns.md)
- [x] [Task 02: Explain and prove message capacity](task-02-explain-and-prove-message-capacity.md)

Task 02 depends on Task 01. Execute sequentially in the primary session.
No subagents are authorized by this package.

## Related delivery records

The original [ceiling package](../session-concurrency-ceiling/plan.md),
[opt-in package](../session-ceiling-opt-in/plan.md),
[idle-focus repair](../idle-focus-resume-admission/plan.md),
[queued-session ownership](../queued-session-ownership/plan.md),
[automation start ownership](../automation-single-start-owner/plan.md), and
[resume queue](../resume-prompt-queue/plan.md) remain historical delivery records.
This package owns the changed manual-prompt expectations and new turn coverage.
Do not rewrite their earlier passing results as evidence for this change.

## Verification results

- Investigation: temporary reproduction failed at the intended provider-call assertion in both paths.
- Cleanup: temporary reproduction removed.
- Backend targeted packages: `go test -trimpath -tags fts5 ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers ./internal/backendapp` passed. The orchestrator package includes the new warm, cold, concurrency, attempt-ownership, queue-restore, and capacity-wait tests.
- Backend race coverage: `GOMAXPROCS=2 go test -trimpath -tags fts5 -race ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers -run '^Test(MessageTurnCapacity|MessageCapacityAdmission|MessageTaskCapacity|CIAutomationIdlePrompt)' -count=1` passed.
- Review follow-up: prompt_ensure replay and fresh CI auto-fix rounds remain bounded; CREATED/completed retries acquire a ticket before startup, honor Auto-run, and preserve waits across transient reads; released/expired tickets are rejected; steer fallback queues an accepted message after a capacity refusal.
- Backend lint: `bash scripts/lint-go-changed <changed-go-files>` reported 0 issues.
- Frontend unit coverage: `pnpm exec vitest run components/settings/system/session-capacity-settings.test.tsx lib/settings-discovery/catalog.test.ts` passed 28 tests.
- Frontend typecheck: `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed.
- Localization: `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed; `ja`, `ko`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, and pseudo are complete.
- Public docs: `node --test scripts/validate-public-docs.test.mjs` passed 63 tests; `node scripts/validate-public-docs.mjs` validated 54 pages.
- Specifications: catalog validation passed for 425 decisions and 1885 specifications; the specification linter passed its 36 tests and all files.
- Desktop and mobile E2E passed: 2 message-capacity scenarios and 2 Settings scenarios per project.
- Whitespace and diff checks passed during implementation.

### Local documentation coverage preflight

This offline command checks the draft against prospective backend and frontend
changes. It does not claim that those production files changed during planning.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/idle-session-message-capacity/';
const workOrders = [
  root + 'task-01-bound-and-queue-message-turns.md',
  root + 'task-02-explain-and-prove-message-capacity.md',
];
const paths = [root + 'plan.md', ...workOrders,
  'docs/specs/agents/requirements/session-concurrency-ceiling.md',
  'docs/specs/agents/system-design/session-concurrency-ceiling.md',
  'docs/specs/tasks/requirements/resume-prompt-queue.md',
  'docs/specs/tasks/system-design/resume-prompt-queue.md',
];
const fileContents = Object.fromEntries(
  paths.map(path => [path, fs.readFileSync(path, 'utf8')]),
);
const changedFiles = [
  ...workOrders.map(filename => ({ filename, status: 'added' })),
  { filename: 'apps/backend/internal/orchestrator/task_operations.go', status: 'modified' },
  { filename: 'apps/web/src/locales/en/system.json', status: 'modified' },
];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({
  ok: result.ok, status: result.status,
  workOrders: result.workOrders, errors: result.errors,
}, null, 2));
process.exitCode = result.ok ? 0 : 1;
NODE
```

## Risks

- Enabled installations lose the implicit manual-message override. Help and public docs must describe the change.
- Startup readiness can release capacity too early unless turn and launch ownership remain separate.
- A restored queue entry can loop immediately unless the capacity refusal suppresses its tail drain.
- Completed-follow-up intent must retain current permissions and exact session identity.
- Durable delivery acceptance and uncertainty must not become ordinary requeue conditions.
- A process-local wait registry requires later session activation after restart, matching the existing queue contract.
