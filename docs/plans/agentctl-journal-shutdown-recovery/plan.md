---
created: 2026-10-09
status: completed
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
  - ../../specs/platform/system-design/agent-runtime-availability.md
legacy_specs: []
---

# Implementation plan: journal shutdown and interrupted-session recovery

## Overview

Prevent one instance shutdown from crashing the shared agentctl process.
Recover retained delivery evidence after execution removal, then provide useful recovery controls on desktop and phone.
Platform owns this repair because it owns the journal lifetime and session recovery contract.
Work proceeds sequentially: journal safety, ACK/capacity repair, recovery authority, then UI and end-to-end evidence.
All four work orders are complete. Backend, frontend, and desktop/phone browser checks passed; detailed evidence and platform limits are recorded below.

## Evidence and assumptions

- Source baseline: `3fed5570cec533f468c25ed03c84967bdc972588`.
- The live incident used `d7a44e96ede93d50c6277773f4174de716272f02` on 2026-10-09.
- At 16:00:40 Lisbon time, stale-instance cleanup preceded a nil bbolt database panic in `Journal.Replay`, at `journal.go:549`.
- `Journal.Close` sets `j.db` to nil under its mutex. `Replay` takes a read lock but does not reject the closed state.
- The panic occurred inside `durableAgentStreamWriter`, outside request-handler recovery. The shared process exited with status 2.
- Logs recorded 28 disconnected session streams. This count includes idle streams, not only running prompts.
- The replacement runtime became healthy at 16:00:41.771. The backend did not need a restart.
- Task `c48225be-bc86-4af3-be43-e8e21271e2c6`, session `986672db-b0fd-47ec-8a50-6d48b5c982bb`, then lost its execution entry.
- Its two retry attempts returned `execution not found`. Earlier recovery events were ignored because the canonical submission lookup failed.
- The exact cause of that missing canonical association needs the initial-dispatch regression in Task 02. Missing evidence must remain visible meanwhile.
- `RecoveryChoiceButtons` adds `mt-3` inside `RecoveryActions`. The uncertain-delivery row centers that entire wrapper beside Stop.
- Existing browser coverage kills the child and checks the uncertainty banner. It does not complete the post-crash retry interaction.

The read-only baseline is retained in `.kandev/diagnostics/ee24ec1b52b730bbdaa3de909e6c0160.zip`.
Its manifest reports truncation. Host logs provide the crash and retry sequence.
No live restart or database repair is part of this package.

### Authorized same-session recovery probe

The user requested recovery of existing sessions with their original context, without creating new sessions.
Read-only inventory found 14 candidate sessions with saved native resume tokens and matching local conversation files.
This inventory is not proof that every candidate still needs work; batch admission must recheck each session.
The local baseline is `.kandev/diagnostics/resume-existing-baseline.json`.

At 16:23:38 Lisbon time, the supported `session.recover` action `resume` restored the linked task's original native conversation.
The Kandev session remained `986672db-b0fd-47ec-8a50-6d48b5c982bb`.
Its native identity remained `01a120e9-4d97-7553-9ed3-67047711bbdf`.
Logs confirmed native load success. A subsequent continuation prompt was rejected before acceptance:
`durable delivery requires reconciliation before a new prompt`.
State-only retry then returned success, which proves attachment but does not prove readiness for a new prompt.
A second continuation attempt after replay received the same reconciliation error. Neither continuation was accepted by the agent.

The current native resume path does not retire unresolved delivery submissions.
Only forced history continuation invokes retirement, and the existing retirement method records cancellation rather than retaining uncertainty.
Task 02 must close this gap without requiring a replacement native conversation.
At that checkpoint, the pilot remained restored but blocked. Other candidates had not been mutated.

### User-authorized operational repair

The user subsequently authorized direct state repair and continuation of the existing sessions.
Scoped SQL records and 13 changed delivery journals were backed up under `.kandev/diagnostics/session-state-repair/`.
Only the linked session's blocked recovery process needed stopping to release its journal lock; the shared runtime remained running.
Under exclusive journal locks, 13 pre-crash dispatching submissions were marked cancelled and retired, with payloads, identities, and events retained.
This administrative cancellation records abandonment, not proof that previous side effects did not occur.
One additional session required native recovery but no journal mutation.
All 14 original native conversations loaded successfully and accepted one continuation instruction each.
At 16:34 Lisbon time, all 14 sessions were RUNNING, with unchanged native identities and no open recovery blocks.
The agents were instructed to inspect existing changes and surviving work before continuing, and to preserve prior scope and approval requirements.
This incident-specific repair does not implement the permanent recovery contract or fix the shared-runtime crash.

### PR 3958 recurrence: acknowledgment and capacity

The recovered session `50d12c99-a3df-4e22-916e-ef7c89f9e08c` produced new messages and tools after 16:33 Lisbon time.
At 16:34:39, agentctl logged `failed to commit durable delivery event` and stopped forwarding output.
A structurally validated journal snapshot showed 268,435,035 retained stream bytes against the 268,435,456-byte limit: only 421 bytes remained.
The journal acknowledged sequence 1193, while SQL had received and projected through 2074.
All 881 intervening events were present and projected in the backend inbox.
The retained snapshot is `.kandev/diagnostics/pr3958-recurrence/journal-snapshot.bbolt`.

At 16:36, session-open recovery classified the execution as stale and removed it; later prompts hit `unresolved_durable_work`.
The source marks the process-manager status as error when event persistence fails, without proving the harness process has exited.
Process liveness and delivery health must remain separate at the cleanup boundary.

The ACK scheduler is keyed only by stream ID and captures the first HTTP client in its send closure.
Subsequent scheduling for the same retained stream does not rebind that client. Workers are cancelled only during stream-manager shutdown.
This is a concrete source defect consistent with ACK progress stopping across runtime replacement.
The exact failing request was not retained in ACK diagnostics; Task 04 must prove the replacement sequence with a deterministic regression.
Its repair must handle projected data without a new event or prompt and keep acknowledgment available at capacity.

No larger limit or live journal mutation is part of this recurrence design update.
Task 04 is numbered after the existing files but executes between Tasks 01 and 02.

## Scope

### In scope

- Closed-journal errors and bounded instance-stream teardown, including sibling isolation.
- ACK worker replacement, durable cursor catch-up, verified pruning, bounded pressure handling, and process/delivery health separation.
- Canonical initial submission identity, retained recovery descriptors, and state-only retry without a live execution.
- Safe explicit native continuation after confirmed process termination and acknowledgment of uncertainty.
- Bulk recovery of selected interrupted sessions with the same Kandev and native conversation identities, and results for each session.
- Specific localized outcomes, aligned controls, visible Stop results, and desktop/mobile coverage.
- Recovery instructions in public documentation when implementation ships.

### Out of scope

- Automatic resend, tool replay, fake terminal outcomes, deleting journals, or blanket recovery-block removal.
- Remote executor restart policies, new feature flags, automatic conversation replacement, or full Kandev restart as the repair.
- Deployment, production publication, or changes to the user's live database. The user separately authorized commit, push, and PR creation.

## Technical approach

Task 01 adds closed-state handling to every exported journal database operation and coordinates instance stream shutdown.
Task 04 repairs ACK ownership and quiet-stream catch-up, then adds pressure recovery and liveness safeguards.
Task 02 makes the durable recovery identity authoritative after execution removal.
It reuses lifecycle reconciliation, repository submission/settlement records, runtime ownership, and native restore.
The backend returns bounded outcomes and eligible actions instead of an untyped internal error.
A separate acknowledged resume request creates a new submission from the user's new instruction.
It also advances delivery recovery state independently of native conversation identity, retaining the old uncertain outcome without blocking the authorized successor.
Batch recovery calls this same guarded operation per selected session, with bounded concurrency and independent results.
Task 03 consumes those results and fixes the action-row structure, including its pending and failure states.

The intended behavior already forbids resend and permits an explicit next instruction after inspection.
The new criteria make missing-execution recovery, shutdown containment, and control geometry explicit.
This repair follows the existing runtime-replacement ADR; it does not introduce another process ownership model.

| Provider/transport shape | Expected behavior | Evidence / unsupported fallback |
| --- | --- | --- |
| Local durable agentctl, ACP provider | Retained replay; explicit native restore when eligible | Real journal + child-crash tests; missing native state uses existing explicit history recovery |
| Local durable native Codex transport | Same journal and ownership gates | Provider-neutral lifecycle tests; native restore only when supported |
| Surviving durable Docker/SSH/Kubernetes peer | Authenticated reattachment to the original owner | Existing reattachment regressions; no local-runtime replacement policy |
| Legacy peer or older incomplete submission records | Preserve uncertainty and identify missing evidence | No synthetic v1 identity or automatic dispatch; typed blocked result |
| Passthrough session | Existing passthrough behavior | Excluded from durable restoration |
| Office or automation | Existing admission and scheduler ownership | No chat-path launch or automatic queue release |

## ASCII UI preview

UI-01: Existing chat recovery card, uncertain delivery.
Labels are illustrative localized text. Alignment and action semantics are required.

```text
Before, desktop:
                         [Stop]
[Retry connection]

After, desktop:
Delivery was interrupted. The prompt outcome is uncertain.
[Retry connection] [Stop]
Checking the previous session...       (only while pending)

After retry, original process confirmed stopped:
The previous agent stopped. Earlier work may have run.
[Review and resume] [Retry connection] [Stop]

Phone, same recovery state:
Delivery was interrupted.
The prompt outcome is uncertain.
[       Retry connection       ]
[             Stop            ]
Checking the previous session...
```

UI-02: Explicit continuation within the card, after selecting Review and resume.

```text
Desktop:
Earlier work may have run. Inspect the changes before continuing.
Next instruction: [______________________________________]
[ ] I reviewed the interruption and want to continue.
[Resume session] [Cancel]

Phone:
Earlier work may have run.
Inspect the changes before continuing.
Next instruction:
[_____________________________]
[ ] I reviewed the interruption
    and want to continue.
[        Resume session       ]
[            Cancel           ]
```

Pending disables duplicate actions and announces progress. Stop remains reachable during state-only reconciliation.
Blocked ownership shows its specific reason and no Resume session action.
Unconfirmed Stop shows its limitation without clearing uncertainty.
Recovered output appears once, and only its matching notice clears.
The existing chat and footer allocation own scrolling. Tall phone recovery content uses the existing bounded footer scroll area. No new drawer or fixed toolbar is needed.
The nearest exemplar is the shipped recovery card and `mobile-agent-runtime-replacement.spec.ts`.
Desktop controls share one row and measure 28px at the standard font size.
Phone controls stack below 768px. Phone and coarse-pointer targets measure at least 44px.
All controls use the shared sizing helper. Status text does not affect button alignment.
The existing chat layout retains dynamic viewport and safe-area handling.
UI-01 and UI-02 cover delivery criteria 006.7-006.9 and runtime availability 003.5.

UI-03: Shared runtime recovery notice, desktop and phone.

```text
Interrupted sessions
[x] Task A: ready to resume
[x] Task B: ready to resume
[ ] Task C: native conversation unavailable
[Resume selected sessions]

Results:
Task A: continuing in the original conversation
Task B: restored, delivery reconciliation blocked
Task C: unchanged
```

Phone uses stacked rows and full-width actions with targets of at least 44px.
Selection and results remain keyboard-accessible. Refresh retains each item's status.
Retries target unfinished eligible items only. Missing context does not trigger a new session or conversation.
UI-03 covers delivery criterion 006.10.

UI-04: Delivery pressure, in the existing recovery card.

```text
Desktop:
Output delivery is paused while saved output is synchronized.
[Retry connection] [Stop]

Phone:
Output delivery is paused while
saved output is synchronized.
[       Retry connection       ]
[             Stop            ]
```

Show paused only when flow is actually paused. Otherwise show cancellation pending or delivery interrupted from the backend's authoritative state.
Copy is illustrative and localized during implementation. Use the same accessible controls and scroll owner as UI-01.
This view covers delivery criterion 001.4.

## Tests

| Acceptance criteria | Proposed regression evidence |
| --- | --- |
| Delivery 001.1-001.3 | `journal_shutdown_test.go`: `TestJournalOperationsAfterClose`, `TestJournalCloseDuringReplay`; API `TestInstanceTeardownDoesNotCrashSiblingStream` |
| Delivery 003.1-003.2, 005.1-005.3 | `agent_delivery_submission_test.go`: initial dispatch persistence; real repository replay with duplicate terminal settlement |
| Delivery 006.1, 006.4-006.8; runtime 003.1-003.6 | Missing-execution replay, terminal, unknown, missing association, locked journal, stale epoch, explicit resume, duplicate request, mixed live/dead sessions |
| Delivery 006.7-006.9; runtime 003.5 | Component/service tests plus real crash/retry browser flows and button geometry |
| Delivery 006.10 | Batch partial failure, retry/reload idempotency, same native identity, queue ordering, and accepted continuation per eligible item |
| Delivery 001.4, 005.4-005.5 | Same-stream client replacement, quiet ACK catch-up, quota-pressure control, projected-but-unacknowledged recovery, and live-harness cleanup refusal |

## E2E tests

Task 03 extends `tests/layout/agent-runtime-replacement.spec.ts` and its mobile counterpart.
The flow kills an isolated child, waits for replacement, clicks retry, observes a typed outcome, and checks dispatch count.
It then exercises eligible explicit continuation and confirms that only the new instruction runs.
A retained terminal case proves replay without duplicate messages or workflow effects.
An unknown owner case proves visible refusal. Reload preserves the correct recovery action and uncertainty.
Desktop and phone tests measure actual control bounds, including pending and error states.
The browser recurrence fixture fills the shipped 256 MiB quota after isolated child termination, with SQL projection ahead of remote ACK. Retry must prune the retained backlog before explicit native continuation and new output. Backend regressions use reduced quotas to cover live producer pressure, control responsiveness, and quiet ACK catch-up.
At true capacity exhaustion, status and Stop stay responsive. Focusing the task must not replace a live harness merely because delivery is unhealthy.
Existing durable stream-recovery suites remain regression inputs.

## Work orders

- [x] [Task 01: Make journal shutdown safe for late consumers](task-01-journal-shutdown.md)
- [x] [Task 04: Recover acknowledgments and contain delivery pressure](task-04-acknowledgment-capacity.md)
- [x] [Task 02: Recover interrupted sessions after execution removal](task-02-session-recovery.md)
- [x] [Task 03: Complete recovery controls and browser coverage](task-03-recovery-ui.md)

## Verification results

All four work orders passed their task-defined checks on 2026-10-09. The initial design checks below are historical; implementation results follow them.
The initial repair resumed 14 sessions; the later PR 3958 recurrence establishes the additional ACK/capacity scope above.
Design-package validation on 2026-10-09:

- `python3 scripts/list-docs.py validate`: passed (368 decisions, 1496 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for all three work orders with a simulated production trigger.
- `git diff --check`: passed. All three untracked work orders are included in the package inventory.

The coverage preflight validates references; it does not claim that production files changed.
No product tests or browser verification ran during this design turn.

Task 01 implementation results:

- `(cd apps/backend && go test -trimpath -race ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agentctl/server/process -count=1)`: passed.
- `git diff --check` and gofmt checks: passed.

Recurrence design extension validation on 2026-10-09:

- Specification catalog and specification lint: passed.
- Work-order coverage preflight: passed for all four work orders, including Task 04 and its synthetic ACK-source trigger.
- `git diff --check`: passed.
- No production code or live session state changed during this extension. Task 04 implementation and regressions subsequently passed; see its work-order results.

### Completed implementation validation

- Task 04: the exact seven-package backend race command passed, including shutdown, replacement ACK ownership, quiet cursor catch-up, capacity, and live-owner refusal.
- Task 02: the exact five-package race command passed for SQLite and PostgreSQL. SQL guard and PostgreSQL/SQLite store conformance passed. The PostgreSQL package run used `-timeout 30m` under concurrent runner load.
- Task 03: nine frontend suites passed, with 129 tests. Type checking, lint, locale completeness, and the staged new-copy ratchet passed.
- Both required managed browser selections passed sequentially: two Chromium specs (33.9 seconds) and two mobile-chrome specs (30.2 seconds). Each full-journal flow used the shipped 256 MiB quota, verified pruning, and continued in the original conversation without repeating a prompt.
- The managed runs built the Go runtime, test fixture, plugin fixture, and E2E Vite bundle. `pnpm --dir apps/web run build:vite` also passed.
- The new fixture freshness test passed after a red regression. Makefile shell-dispatch checks and runner shell syntax checks passed.
- Go lint passed against the source baseline. Changed Go files are formatted.
- Public documentation checks passed: 63 validator tests and 47 pages. The catalog validated 368 decisions and 1496 specifications; full specification lint passed.
- Documentation coverage preflight passed for the actual staged production diff and all four work orders. The first local preflight omitted unchanged referenced documents; supplying the complete referenced package resolved that input error.
- Final diff whitespace checks passed after normalizing the work-order ending.

Linux host process proof was exercised. Darwin/Windows runtime behavior, real provider CLIs, and remote executor images were not exercised locally. Unsupported process proof fails closed. No production database or live user session was changed during implementation.

## Risks

- An execution-map miss does not prove that the old process died. Ownership checks remain mandatory.
- A prior unknown outcome cannot become a fabricated success merely to unblock the user.
- Missing initial submission association can require separate compatibility handling for existing installations.
- Journal guards must cover all database operations, including secondary files and health reads.
- Holding a lock while waiting for stream shutdown can deadlock teardown.
- Shared recovery controls affect other cards. Their targeted component suites must remain green.
- A same-task replacement session reuses files but does not automatically inherit the interrupted conversation.

## Related implementation records

- [Missing delivery record recovery](../durable-agent-record-recovery/plan.md):
  its Task 02 reconstructs verified retained submissions that lack canonical
  backend associations, using this package's authenticated inspection and
  explicit continuation flow.
- [Runtime replacement](../agentctl-runtime-replacement/plan.md).
- [Durable reattachment](../durable-agent-reattachment/plan.md).
- [Earlier streaming repair](../durable-agent-stream-repair/plan.md): prior implementation remains historical; Task 04 owns this recurrence.
- [Durable sessions](../durable-agent-sessions/plan.md).
- [Session reconciliation](../durable-agent-session-reconciliation/plan.md).

Their earlier results remain historical evidence. They do not cover this incident or the new browser interaction.

## PR review follow-up

Prepared checkpoints retry through the existing admission gates. Canonical acceptance evidence can complete failed bookkeeping without dispatching again. Browser requests and results use the full interruption identity, and pending continuation announces progress with a stable action name. Earlier runtime and reattachment plans now identify all four completed work orders.

The first-instruction persistence guard also accepts the matching canonical submission from the orchestrator launch handoff before the execution dispatches its first prompt. The desktop monitor, board Copilot, coordinator proposal, and phone Configuration Chat scenarios reproduced the original failure and passed after the correction. Focused race regressions also verify admission callbacks, launch handoffs, duplicate refusal, and foreign-owner refusal.

Final review validation passed: the exact five-package SQLite race command, SQL guard, SQLite store conformance, changed-scope Go lint, 147 frontend tests, type checking, web lint, translation checks, and documentation validation. Both recovery browser selections passed sequentially (Chromium 39.5 seconds; mobile-chrome 28.8 seconds), and all four reproduced CI scenarios passed. Four fresh screenshots document the final UI. Storage contracts did not change during this follow-up, so the earlier PostgreSQL validation remains applicable.
