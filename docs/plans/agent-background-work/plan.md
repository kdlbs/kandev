---
created: 2026-09-27
status: draft
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-003
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-006
  - REQ-PLATFORM-EXPLICIT-STEERING-001
  - REQ-PLATFORM-EXPLICIT-STEERING-002
  - REQ-PLATFORM-EXPLICIT-STEERING-003
system_design:
  - ../../specs/agents/system-design/background-work.md
  - ../../specs/platform/system-design/explicit-turn-steering.md
legacy_specs: []
---

# Implementation Plan: Agent Background Work

## Overview

Deliver one normalized background-work experience, first backed by native Codex
and existing ACP observations. Shared identity, messages, capabilities, routing
and UI must also accommodate a future Claude-native adapter. Implement sequentially:
contract/gate, durable observations, Codex mapping, authorized controls, shared
background UI, native same-turn delivery, composer delivery choice, and executor
fixtures/public guidance. Every work order has its own TDD evidence.

This replaces the uncommitted `codex-app-server-background-tasks` package. The
old draft's three IDs are retired in the new [requirements](../../specs/agents/requirements/background-work.md).
The user explicitly settled multi-protocol scope and later added main-conversation
same-turn steering on 2026-09-27. No native Claude
implementation or speculative ACP control extension is part of this package.

## Scope

In scope: normalized work/run records, read-only ACP observations, version-tested
Codex discovery/termination/child interruption, capability-gated shared controls,
bounded available output, existing message/usage views, durable recovery,
authorized transport, desktop/phone UX, explicit Send now to the active root
turn versus Queue for later, and executor conformance fixtures. Same-turn mode
bypasses a nonempty queue deliberately; the existing queued entries remain intact.

Out of scope: new task/session creation, scheduler changes, unrelated automatic
prompt-admission changes, host-PID control, process restart, child prompting/steering, standalone
terminal replacement, live cost accumulation or private reasoning extraction.
Codex background stdin stays unavailable; the shared input contract is exercised
through a synthetic provider until a real adapter has proven support.

## Baseline and integration points

Review baseline: PR #3916, head `b3ead0d0376317215ddcadc48b316b3743782744`,
merge base `359b5ffdbb6e25592bc3a46d88db1dbf94ff103c`. Resolve the current head
before implementation; existing checks/results do not validate this new package.
The prior [native plan](../codex-app-server/plan.md) and
[protocol follow-up](../codex-app-server-followup/plan.md) are related historical
implementation records, not proof that this UI or durable projection exists.
Their existing result blocks remain unchanged.

Existing seams: `streams.BackgroundWorkPayload`, optional adapter capabilities,
`internal/agent/runtime`, agentctl instance APIs, orchestrator background
attestation/accounting, task subagent repository, normalized tool messages,
session activity epochs and conversation-usage ledger. New symbol names in the
[design](../../specs/agents/system-design/background-work.md) are proposed.
Do not use the nonexistent useTaskBackgroundWorkState hook or assume a Codex
process is registered in Kandev ShellManager.

Test patterns: native adapter `adapter_test.go` child-after-parent and reconnect
cases; `background_work_attestation_test.go`; task repository
`subagent_context_postgres_test.go`; web `tool-subagent-message.test.tsx`;
`conversation-usage-display.tsx`; desktop/mobile `subagent.spec.ts`;
`e2e/helpers/native-codex-app-server-executor.ts`.

## Technical approach

- Agents own normalized contracts and optional capabilities. Task services own
  inspection persistence/actions; runtime transports resolve the correct executor;
  adapters own native parsing and addressing. Shared UI never parses native data.
- Add workload/run/output and action-receipt tables with exact identity/revision
  rules. Preserve invocation history, accounting, and admission as distinct owners.
- Add additive session read/action APIs and normalized websocket messages; one
  domain hook/store drives desktop and phone. New reads/mutations are gated
  server-side; old adapters/clients retain current behavior.
- The shared gate is `features.agentBackgroundWork`, independent of
  `features.codexAppServer` and Claude prompt handoff; all shipped defaults off.
  Apply `/runtime-feature-flags` during implementation and trace every entry path.
- Platform owns the additive [explicit steering contract](../../specs/platform/system-design/explicit-turn-steering.md).
  Task 07 implements exact-turn native dispatch and durable delivery receipts;
  Task 08 adds explicit composer choice. `features.sameTurnSteering` is off in
  every profile, independent of background visibility and the existing Claude
  steering gate. Native Codex also requires its existing native profile gate.
- Follow [the ADR](../../decisions/2026-09-27-provider-neutral-background-work.md).
  No raw identifiers as browser control addresses, no arbitrary signals, no
  independent billing accumulator, and no restored liveness from historical rows.

| Provider / transport | Identity and initial behavior | Verification | Unsupported fallback |
| --- | --- | --- | --- |
| Codex native / stdio | Stable thread/item/process plus child turn; list/terminate/interrupt and available output | Pinned 0.154.0 schema, fake RPCs, separately recorded live evidence | Read-only unknown/snapshot; stdin unavailable |
| ACP / existing adapter transports | Existing session/execution/call observations; stable child key only when proven | Real normalized ACP fixtures plus shared UI E2E | No controls; unavailable hierarchy/output/usage explicit |
| Claude native / future | Same shared contract required | Not implemented; no support claim | No registration or feature-specific UI |
| Synthetic adapter / conformance | Normalized keys only; all or partial capabilities | Service/runtime/agentctl round trip and rendered tests | Tests omitted interface and omitted capabilities |

Steering compatibility is separate from background-process controls:

| Capability | Main-conversation delivery | Queue policy |
| --- | --- | --- |
| Codex native same_turn | Explicit turn/steer with expectedTurnId | Send now bypasses pending next-turn entries; Queue for later appends normally |
| ACP provider_managed | Existing concurrent-prompt behavior; folding not guaranteed | Existing automatic queue-first policy unchanged |
| Unsupported / old peer / disabled | Ordinary send/queue | No native same-turn dispatch |

## ASCII UI preview

### UI-01: Compact composer pill and dedicated central tabs

Entry: the small pill sits immediately above the chat input. Click or keyboard
activation opens a Popover. Select an agent/job to open its detail directly in
its own named tab in the central group; View all opens the separate overview.

```text
   ( 2 Background Jobs )
+----------------------------------------------------------+
| Message draft...                                         |
|                                             [Send/Queue] |
+----------------------------------------------------------+

+ Background jobs ----------------------------+
| Build watcher       Running          [Open] |
| Review auth         Running          [Open] |
| [View all work]                             |
+--------------------------------------------+

Central group after opening the overview and two workloads:
+------+-----------------+-------------------+-----------------+
| Chat | Background Work | Build Watcher [x] | Review Auth [x] |
+------+-----------------+-------------------+-----------------+
| [All work]  Build watcher     Running             [Stop] |
| [Search retained output...]                              |
|                                                          |
| PASS auth.test.ts                                        |
| Watching for changes...                                  |
|                                                          |
| One scrolling output area                                |
+----------------------------------------------------------+
| [Auto-scroll: on] [Clear view] [Reset view]                |
| [Input ...] [Send input]   only if supported               |
+----------------------------------------------------------+
```

The pill is content-width, not a banner; it displays one count including running,
waiting and unknown jobs, with the breakdown in the summary. A Background work
chat action opens completed history when the active pill disappears. Initially
open each panel in the central group. One overview per session/incarnation and
one detail tab per session/incarnation/workload. Reopening a workload focuses its
existing tab in its user-chosen position. Distinct workloads keep independent
scroll/search/input draft and inspected-run state, even when titles match.
Closing any tab only closes that view; other tabs and jobs stay open/running.
Completed detail tabs remain inspectable. Returning to Chat preserves its draft.
Ordinary desktop controls remain 28px; the pill is deliberately compact.

### UI-02: Phone summary and full-height detail

Entry: the same small pill has a touch-sized hit area. Tap opens an inset summary
Drawer. Tap a job to open full-height detail directly, or View all for the list;
close the summary before transitioning. Back in detail returns to the list.

```text
 ( 2 Background Jobs )
+-----------------------------------+
| Message draft...           [Send] |
+-----------------------------------+

+ Background jobs ------------------+
| Build watcher       Running   [>] |
| Review auth         Running   [>] |
| [View all work]           [Close] |
+-----------------------------------+

+ [Back] Build watcher ---- [Close] +
| Running                          |
| [Search output...]               |
|                                  |
| One scrolling content body       |
|                                  |
| [Earlier output truncated]       |
+----------------------------------+
| [Stop] [Auto-scroll]              |
| [Input...] [Send] if supported    |
+------ keyboard / safe area -------+
```

Header and conditional action/input region stay fixed; content owns vertical
scroll. Use dynamic viewport height and keyboard-aware clearance, not stacked
sheets. Phone/coarse-pointer hit targets are at least 44px even though the pill
looks small. Close returns to chat/invoker; Back returns to the selected list
row. Existing inline transcript cards remain available on both viewports.

### UI-03: Honest failure and capability states

```text
Loading:       [Loading background work...]
Empty:         [No background work observed]
Disconnected:  build  Unknown [Stop disabled: reconnecting]
Unsupported:   [Live output unavailable] [Retained output]
Uncertain:     [Input delivery unknown. Check output before sending again.]
Usage:         [Tokens: 12k estimated] [Cost unavailable]
History:       build  Ended (outcome unavailable) [Open]
```

Structure/navigation/capability gating, one scroll owner and responsive targets
are required; spacing and example text are illustrative. No Restart/Re-run/Steer
control is included. These previews cover AC-AGENTS-BACKGROUND-WORK-001.2,
002.5, 003.1-.4, 004.1-.4 and 005.1-.5 through Task 05's rendered tests.

### UI-04: Main conversation delivery choice

While a same-turn-capable root turn is active, the primary action is explicitly
Send now, including with queued work. The menu selects Queue for later. The
background-job pill remains independent of this delivery choice.

```text
Desktop
 ( 2 Background Jobs )
+----------------------------------------------------+
| Keep the public API unchanged...                   |
|                                   [Send now] [v]   |
+----------------------------------------------------+
 Queue: 2 messages for later
                         +--------------------------+
                         | Send now                 |
                         | Queue for later          |
                         +--------------------------+
```

### UI-05: Phone delivery choice and outcomes

```text
 ( 2 Background Jobs )
+-----------------------------------+
| Keep the public API unchanged...  |
|                    [Send now] [v] |
+-----------------------------------+
 [2 queued for later]
 Tap v: inset choice drawer
+-----------------------------------+
| Send now                          |
| Add to the current turn           |
| Queue for later                   |
| Run after current work            |
+-----------------------------------+

Pending:   Sending to active turn... [Send now disabled]
Accepted:  Sent to active turn
Rejected:  That turn ended. Draft retained. [Choose delivery]
Uncertain: Delivery unknown. It may already have arrived.
```

Phone targets are at least 44px, with keyboard/safe-area clearance and focus
return. Draft/attachments survive mode changes and rejection. No child-detail
composer is added. These previews cover AC-PLATFORM-EXPLICIT-STEERING-001.1-.5,
002.1-.3 and 003.1-.4; Task 08 owns the rendered checks. Example copy is localized.
See the [platform design](../../specs/platform/system-design/explicit-turn-steering.md)
for target expiry, incompatible settings and older/ACP capability fallbacks.

## Tests and acceptance traceability

All AC references below use the `AC-AGENTS-BACKGROUND-WORK-` prefix. Test names
are proposed and must be added in the named work order, not treated as existing
coverage. Exact package/file commands are in each work order.

| Criteria | Work order | File and behavioral evidence |
| --- | --- | --- |
| 001.1-.4 | 01, 03, 05 | streams `background_work_contract_test.go`: TestBackgroundWorkContractCompatibility; runtime `background_work_test.go`: TestBackgroundWorkFeatureDisabled; native `background_work_test.go`: TestCodexBackgroundCapabilities; shared component provider matrix |
| 002.1-.5 | 02, 03 | orchestrator `background_work_projection_test.go`: TestBackgroundWorkProjectionReplayAndSuccessor, TestBackgroundWorkSnapshotRace; repository `background_work_test.go`: TestBackgroundWorkRepositoryRoundTrip; native tests: TestCodexChildBeforeBinding, TestCodexNestedChildAndMultipleCalls, TestCodexBackgroundPaginationAndPartialFailure |
| 003.1-.4 | 03, 04, 05 | task service `background_work_actions_test.go`: TestBackgroundWorkActionAuthorization, TestBackgroundWorkActionStaleRun, TestBackgroundWorkActionReceiptReplay, TestBackgroundWorkInputUncertain, TestBackgroundWorkChildRequestOwnership; native TestCodexChildInterruptExactTurn |
| 004.1-.4 | 02, 03, 04, 05 | service `background_work_output_test.go`: TestBackgroundWorkOutputBounds; native `background_work_output_test.go`: TestCodexOutputDeltaFinalReconciliation; service `background_work_usage_test.go`: TestBackgroundWorkUsageAttribution; shared viewer tests |
| 005.1-.5 | 05 | web background-work store/API/hook/component tests plus desktop/phone E2E; deferred HTTP, deleted incarnation, draft/admission preservation, translated states, focus and measured geometry |
| 006.1-.3 | 03, 04, 06 | pinned protocol fixtures; ACP TestACPBackgroundWorkObservationOnly; action runtime conformance; native executor fixtures and optional TestNativeCodexBackgroundWork |

Steering criteria use the `AC-PLATFORM-EXPLICIT-STEERING-` prefix:

| Criteria | Work order | File and behavioral evidence |
| --- | --- | --- |
| 001.1-.5 | 07, 08 | orchestrator `native_steer_test.go`: TestNativeSteerWithQueuedMessages, TestNativeSteerSequential, TestNativeSteerBusyAndFeatureMatrix; native `steer_test.go`: TestNativeSteerCapabilityModes; session-input-mode and composer delivery tests |
| 002.1-.5 | 07, 08, 06 | orchestrator `native_steer_test.go`: TestNativeSteerCompletionRace, TestNativeSteerUncertainReceipt, TestNativeSteerEchoDedup, TestNativeSteerSingleCompletionAndUsage, TestNativeSteerAuthorizationAndChildOwnership; repository `steering_receipt_test.go` and `steering_receipt_postgres_test.go`: TestSteeringReceiptAtomicMessage, TestPostgresSteeringReceiptRoundTrip; remote native fixtures |
| 003.1-.4 | 08 | `hooks/use-message-handler.test.ts`, `lib/api/domains/steering-delivery.test.ts`, `components/task/chat/steering-delivery-controls.test.tsx`; desktop/phone native-steering E2E: queue bypass, preserved drafts, outcomes, expired target, input/options, feature-off and ACP fallback |

Use channel/barrier ordering for stale-event and control races, not sleeps.
PostgreSQL acceptance requires an isolated DSN and actual non-skipped tests.
Follow existing feature/profile contract tests and legacy subagent/admission
regressions; do not introduce another broad QA or review phase.

## E2E tests

| Flow and criteria | File under apps/web/e2e | Project / owner |
| --- | --- | --- |
| Compact pill; central-group overview and per-workload tabs, multiple open details, repeat-click focus, independent state, restore/close; completed history, stop/interrupt, conditional input, missing capabilities, child request, usage provenance; 001, 003, 004, 005 | tests/chat/background-work.spec.ts | chromium / 05 |
| Drawer to full-height detail, Back/focus, keyboard clearance, 44px targets including narrow fine pointer, overflow, conditional input; 005 | tests/chat/mobile-background-work.spec.ts | mobile-chrome / 05 |
| Root finishes before child, refresh/late event, backend restart unknown, source ownership, no duplicate output; 002, 004 | both chat files above | chromium and mobile-chrome / 05 |
| Codex fixture and ACP observation-only rendering through backend events; 006.1-.2 | both chat files above | chromium and mobile-chrome / 05 |
| Real executor routing with fake provider; sibling/root survives targeted stop, remote IDs never host PIDs, feature/capability-off; 001.3, 003, 006.3 | tests/docker/background-work.spec.ts; tests/ssh/background-work.spec.ts; tests/kubernetes/background-work.spec.ts | containers / 06 |

Additional steering E2E in `tests/chat/native-steering.spec.ts` (chromium) and
`tests/chat/mobile-native-steering.spec.ts` (mobile-chrome), owned by Task 08:
Send now with queued messages, sequential steers after ack, exact-turn expiry,
uncertain delivery, unchanged queue/drain, one transcript echo/completion,
provider-managed/off-flag fallbacks, phone menu/focus/geometry and input drafts.
Task 06 extends the remote executor fixtures with exact-turn dispatch and rejection
checks, plus separately labelled optional `TestNativeCodexSameTurnSteering` live evidence.

Each background-work fixture enables the shared flag explicitly; native cases also enable the
Codex gate. Steering fixtures enable sameTurnSteering and Codex; prove steering
also works with agentBackgroundWork off. Feature-off cases assert no side effects. Use causal WS/HTTP waits
and the managed one-worker-per-shard runner. Fake fixtures prove wiring, not
real provider support. Existing subagent E2E remains part of Task 05 regression.

## Work orders

- [x] [01: Normalized contract and rollout gate](task-01-contracts-and-rollout.md)
- [x] [02: Durable observation and recovery path](task-02-observations-and-recovery.md)
- [x] [03: Codex capability mapping and reconciliation](task-03-codex-provider.md)
- [x] [04: Authorized controls and attributed usage](task-04-controls-and-usage.md)
- [x] [05: Shared desktop and phone background-work UI](task-05-shared-ui.md)
- [ ] [07: Native same-turn steering and delivery receipts](task-07-native-turn-steering.md)
- [ ] [08: Explicit composer delivery on desktop and phone](task-08-steering-composer.md)
- [ ] [06: Executor conformance fixtures and public guidance](task-06-executor-conformance-and-docs.md)

Execute sequentially in the order above: 01, 02, 03, 04, 05, 07, 08, 06.
Existing work-order IDs remain stable; Task 06 now follows the steering UI. No delegation or implementation is authorized by this plan.
Each work order updates its own Results and this manifest after its targeted checks.

## Verification results

Implementation pending. Design-package checks on 2026-09-27:

- `python3 scripts/list-docs.py validate`: passed (318 decisions, 1209 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed; untracked package files also checked for whitespace.
- New package links and acceptance mapping: passed, all 39 criteria assigned across background work and explicit steering. Two pre-existing task-summary-contention links in the platform README remain unresolved; this package adds neither link.
- `.github/scripts/pr-docs.cjs::validateCoverage`: actual documentation-only
  snapshot is exempt; an additional in-memory source-path trigger validated all
  eight work orders and their plan/requirement/design references, with no errors.

No new runtime or browser tests have run for this unimplemented package. No
production files were changed. Public-doc updates belong to Task 06 when the
behavior is implemented, not to this design-only change.

## Risks and evidence limits

- Live Codex child correlation and exact usage remain unproven by the PR's
  earlier live suite. Task 03/06 evidence must distinguish observed/unsupported/
  unobserved cases; do not infer support from successful initialization.
- Terminal-list pagination and stale snapshots can falsely retire live work;
  Task 03 must fix these before exposing controls.
- Existing invocation rows and active-subagent counts have different identities
  from provider child runs. Task 02 must not silently migrate their semantics.
- Input delivery may remain uncertain after a crash; receipts prevent automatic
  replay, not remote uncertainty. No provider identity is an authorization token.
- Future protocols may lack stable run IDs. They remain observation-only until
  exact targeted control and successor isolation can be proven.
- Output and storage bounds are explicit product limits, with visible truncation;
  logs do not promise a terminal emulator or complete provider history.

- Native steering adds a deliberate queue-order exception only for explicit
  active_turn intent. Legacy ACP steering and unlabelled/automatic messages must
  not inherit it. No ambiguous error may trigger fallback delivery.
- Existing agentctl steering acknowledges asynchronously before provider result;
  the native path must await the bounded RPC acknowledgement before reporting accepted.
