---
created: 2026-10-27
status: building
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-008
  - REQ-AGENTS-BACKGROUND-WORK-009
system_design:
  - ../../specs/agents/system-design/background-work-opencode.md
  - ../../specs/agents/system-design/background-work.md
legacy_specs: []
---

# Implementation plan: OpenCode background continuity

## Overview

Make OpenCode background jobs visible through Kandev's shared viewer. Deliver
the automatic responses that follow their results. Keep an idle foreground
promptable while its jobs continue.

Implementation began on 2026-10-27. Tasks 01, 05, and 07 are complete with
tests; Task 06's public documentation is complete. Tasks 02, 03, and 04 remain
blocked on the upstream OpenCode producer, and the plan records that gate
explicitly rather than claiming continuity from consumer fixtures alone. The
work orders remain sequential.

Agents owns the provider contract because native workload identity and lifecycle
are the source of truth. Platform and Tasks retain admission, persistence,
workflow placement, and durable delivery authority.

## Confirmed scope and baseline

The user accepted the shared background-job chip, desktop detail tabs, phone
drawer, and separate foreground/background activity. OpenCode input queues while
the foreground is generating. A foreground-idle conversation accepts an ordinary
new prompt while jobs continue. This package does not add same-turn steering.

Baseline Kandev revision: 0b1017d81547f44c6dddaf52ed1471767e499c1b.
Source inspection used OpenCode 2.2.0 at upstream commit
1097e2668b1e08ad63bdf0b1c4860b9bc01e420c.
See [protocol evidence](protocol-evidence.md).

The missing producer events are a delivery dependency, not a frontend defect.
Task 01 can deliver honest launch inspection independently. Full continuity
cannot complete until Task 02 proves and packages the upstream change.

## Scope

### In scope

- Recognize OpenCode's structured background input and shell identity.
- Keep launch completion separate from job outcome.
- Negotiate supported continuous session events and explicit automatic cycles.
- Preserve journal identity, prompt ownership, queue order, and usage ownership.
- Use verified background activity with the existing parked projection.
- Reuse the shared chip, desktop tabs, phone drawer, output, and history.
- Show failure, unknown activity, unavailable output, and unsupported controls.
- Cover enabled, disabled, legacy, reconnect, and executor paths.
- Make the shared experience default-on after verification, while retaining an
  operator off switch during rollout.

### Out of scope

- A native OpenCode HTTP transport or a database/output-file side channel.
- Implementing OpenCode Stop, Interrupt, process stdin, or same-turn steering.
- Changes to Claude handoff, Codex native steering, billing, or workflow moves.
- Agent-controlled schedulers, new platform tasks, or another terminal service.
- Lossless replay across an OpenCode crash without proven upstream replay.
- Removing the shared feature flag before its default-on rollout is proven.
- Fixing the investigated PR, its poll script, or its CI failures.

## Technical approach

1. Task 01 maps confirmed launch frames to the existing workload contract.
   Preserve source call and origin turn. Once a legacy root prompt settles,
   keep unverified jobs unknown.
2. Task 02 changes the upstream ACP producer. Keep one session subscription,
   publish job outcomes and automatic foreground boundaries, negotiate the
   contract, and prove the raw wire behavior with a real packaged CLI.
3. Task 03 adds the pure OpenCode dialect parser and adapter ownership state.
   Bind identity before the notification worker can observe a successor prompt.
   Reuse existing normalized events and durable journal projection.
4. Task 04 connects fresh provider evidence to the existing parked-probe seam.
   Serialize a user-send/automatic-start race. Preserve coarse admission and
   all original job identities across new foreground turns.
5. Task 05 completes generic viewer gaps: unknown counts, history at zero active
   jobs, unavailable output, hidden unsupported actions, and phone details.
6. Task 06 proves the complete path through the browser and remote executors.
   Publish user guidance only after implementation supports the described path.
7. Task 07 promotes the existing shared feature to default-on in prod, dev, and
   e2e. Keep its operator off switch and all provider capability checks.

Reuse features.agentBackgroundWork and its existing typed registry value.
The current profile values remain false until Task 07 changes them to true.
Keep the registry entry and gates through rollout, plus explicit operator
overrides. Browser fallback defaults remain false until authoritative hydration.
Pass the effective value to agentctl and the adapter through the current
request/configuration chain. Old peers omit that additive field and remain off.
Do not read ambient environment values or create another feature registry.

No new database table is planned. Reuse shared background tables and the durable
agent delivery journal. Provider cycle/message identity must exist before
journaling. Implementation discovered one persistence gap: adapter capability
declarations were not retained, so the read path had to invent controls. An
additive `task_session_background_work.capabilities` column (with a boot
migration and schema-replay test) now stores the declaration; absent evidence
reads as inspect-only.

### Compatibility matrix

| Provider / shape | Intended behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- |
| OpenCode 2.2.0, background=true plus shellID/running | Inspect launch; preserve identity | Sanitized observed launch fixture | Unknown after foreground settlement |
| Negotiated OpenCode continuity v1 | Job outcomes, explicit automatic cycles, verified activity | Task 02 packaged CLI and consumer fixtures | Reject unknown contract versions |
| OpenCode v1 family or native binary | Use advertised capability only | Initialize/load negative cases | Existing ACP prompting |
| Shared feature disabled | Existing chat and admission | DTO and wire-negative tests | No new readers, observations, or controls |
| Claude ACP | Existing monitor, wakeup, launch and handoff | Existing regression tests | No OpenCode parsing |
| Codex native / other ACP | Existing provider behavior and shared UI | Viewer/conformance regressions | No invented control or steering capability |

Default-on inspection does not enable the separate native-Codex or Claude
handoff flags. A later graduation removes the shared flag and retires its key
and environment identity. This package retains them as an operator off switch.

The existing [agent-background-work package](../agent-background-work/plan.md)
owns the shared contract and its historical results. This extension does not
mark its pending native-steering Tasks 07/08 done or depend on their delivery.
The [disambiguate-waiting package](../disambiguate-waiting/task-05-parked-projection.md)
retains the parked-projection formula and legacy process-probe behavior.

## ASCII UI preview

These previews define grouping, navigation, and states. Spacing is illustrative.
Copy is localized. Reuse existing tokens and primitives.

### OC-UI-01: Desktop, foreground idle and two jobs active

Entry: existing task conversation. Maps to AC-009.1, .2, .4, and .5.

~~~text
+-----------------------------------------------------------+
| Conversation                   Working in background      |
|                                                           |
| Agent: I will report when CI and the local run finish.     |
|                                                           |
| [2 Background jobs v] [Background history]                 |
| +----------------------------------------------+          |
| | CI poller          Running           [Open]  |          |
| | CI watcher         Running           [Open]  |          |
| | Local tests        Completed         [Open]  |          |
| | [View all]                                   |          |
| +----------------------------------------------+          |
| [Message the agent...                              ]      |
|                                                [Send]     |
+-----------------------------------------------------------+
| Central tabs: [Chat] [CI poller] [Background work]          |
| Selected job: status / retained output / capability state  |
+-----------------------------------------------------------+
~~~

The chip opens the existing popover. Open selects a named central detail tab.
Tabs retain independent workload identity. Closing a tab leaves the job running.
The foreground-idle composer sends normally. An active foreground shows Queue.

### OC-UI-02: Phone, summary and focused job detail

Entry: tap the same chip. Maps to AC-009.4 and shared phone requirements.

~~~text
+-----------------------------+   +-----------------------------+
| Conversation                |   | [Back] CI poller    [Close] |
| Working in background       |   | Running                     |
|                             |   |-----------------------------|
| [2 Background jobs]         |   | Output                      |
| [Message...        ] [Send] |   | poll 1: checks pending      |
|-----------------------------|   | poll 2: checks pending      |
| Background jobs     [Close] |   |                             |
| CI poller     Running [Open]|   | One scrolling output region |
| CI watcher    Running [Open]|   |                             |
| Local tests Completed[Open] |   |                             |
| [View all]                  |   |                             |
+-----------------------------+   +-----------------------------+
~~~

The summary is an inset bottom drawer. Job detail becomes a full-height surface,
with a fixed header and one internal scroll owner. Back preserves list context.
Dismissal restores focus to the chip. Keep 44px phone targets and safe-area
clearance. Opening details must preserve the composer draft and keyboard state.

### OC-UI-03: Failure, unknown activity, and completed history

Maps to AC-007.4, AC-008.4-.5, and AC-009.5-.6.

~~~text
Disconnected:
  [2 Background jobs]  Activity unavailable
  CI poller: Unknown. Output updates unavailable.
  Observation-only controls: no Stop / Interrupt / stdin.

Job failure:
  CI poller: Failed (exit 2). [Open retained output]
  Agent recovery response appears in the same conversation.

All jobs terminal:
  [Background history]       [Message...             ] [Send]
  No active-job animation. Completed and failed rows remain.
~~~

Unknown is distinct from running and success. A missing terminal frame does not
complete a job. Generic count/state rendering must serve Codex and other ACP
providers without checking their names.

## Tests and acceptance traceability

All test names and new files below were proposed in the design turn. The
implementation-status section records which ones now exist and pass.

| Acceptance criteria | Work order | Evidence |
| --- | --- | --- |
| AC-007.1-.2 | 01 | TestOpenCodeBackgroundLaunchIdentity; TestOpenCodeLaunchCompleteIsNotJobComplete |
| AC-007.3, .5 | 02, 03 | Upstream wire fixture; TestOpenCodeBackgroundTerminalAndOrigin |
| AC-007.4, AC-008.5 | 01, 03 | TestOpenCodeLegacyBackgroundUnknown; TestOpenCodeContinuityNegotiation |
| AC-008.1-.2 | 02, 03 | TestOpenCodeAutomaticCycleSingleCompletion; TestOpenCodeAutomaticUsageOnce |
| AC-008.3, AC-009.2-.3 | 02, 03, 04 | Conditional producer admission; TestOpenCodeAutomaticUserPromptRace; TestOpenCodeBackgroundPromptAdmission |
| AC-008.4 | 03, 04 | TestOpenCodeBackgroundReplayAndReset; TestOpenCodeBackgroundReconnectUnknown |
| AC-009.1 | 04, 05 | TestOpenCodeProviderParkedEvidence; frontend activity fixture |
| AC-009.4-.6 | 05, 06 | Viewer unit tests and desktop/phone E2E |
| AC-001.3, .5 | 07 | Profile defaults, environment/SQLite overrides, shared viewer and disabled API checks |

Task 01 also covers negative provider identity, malformed data, feature-off
propagation, and ordinary foreground shells. Task 03 covers queued conversion,
stale terminal events, interleaved jobs, and a silent but active automatic tool.
Task 04 covers starting a new prompt without losing prior jobs.

## E2E tests

- Proposed tests/task/opencode-background-work.spec.ts, chromium:
  three identified jobs; settled root; direct new prompt; background completion;
  one automatic response; failed job; preserved original source turn.
- Proposed tests/task/mobile-opencode-background-work.spec.ts, mobile-chrome:
  the same outcomes through the chip, summary drawer, full-height detail, Back,
  safe areas, focus return, 44px targets, and preserved unsent draft.
- Both suites cover the unsupported 2.2.0 producer, feature off, unknown after
  disconnect, history after active count reaches zero, and absence of controls.
- Proposed tests/docker/opencode-background-work.spec.ts, containers:
  Docker, SSH, and Kind-backed Kubernetes fixture cases. All observations come
  through the owning executor. No host SQLite/file/process shortcut is allowed.

Use the real OpenCode adapter identity through the mock-provider registration.
The producer is deterministic; the ACP decoder, adapter, runtime, database,
WebSocket, and browser path remain real. Label these as fake-producer evidence.
Task 02 records separate real-CLI evidence and requires no paid model for its
deterministic server-backed tests.

## Work orders

- [x] [01: Observe existing OpenCode background launches](task-01-launch-observations.md)
- [ ] [02: Prove continuous events in the OpenCode producer](task-02-opencode-producer.md) (blocked: external repository and packaged artifact unavailable)
- [ ] [03: Deliver automatic foreground cycles once](task-03-automatic-delivery.md) (blocked by 02)
- [ ] [04: Preserve activity and prompt ownership](task-04-activity-and-input.md) (blocked by 03; legacy paths delivered by 01)
- [x] [05: Reuse the shared desktop and phone viewer](task-05-shared-viewer.md)
- [ ] [06: Prove executor flows and document support](task-06-executor-proof-and-docs.md) (docs delivered; executor/continuity evidence blocked by 02)
- [x] [07: Enable shared background work by default](task-07-default-on-rollout.md)

Order: 01, 02, 03, 04, 05, 06, 07. Task 02 is an external producer dependency.
Tasks 03-06 cannot claim full continuity from fake consumer frames alone.
No implementation subagents are authorized.

## Verification strategy

Each work order contains its exact runnable checks. Fresh worktrees install
apps dependencies once before pnpm commands. Managed E2E runs build fresh
artifacts and execute sequentially with one shard.

Documentation gates, from the Kandev root:

~~~bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans
git status --short -- docs/plans/opencode-background-continuity
~~~

Run the exported validateCoverage function in .github/scripts/pr-docs.cjs
against all changed work orders and their referenced documents. Assert that
every requirement and acceptance ID exists, each design declares its work-order
requirements, and the plan lists every referenced design. Include untracked
work orders; a git diff alone does not enumerate them.

## Verification results

Design-package checks on 2026-10-27:

| Check | Result |
| --- | --- |
| python3 scripts/list-docs.py validate | Passed: 405 decisions and 1756 specifications |
| python3 scripts/lint-spec-files.test.py | Passed: 36 tests |
| python3 scripts/lint-spec-files.py --all | Passed |
| Exported PR documentation validateCoverage preflight | Covered: all seven work orders, no errors |
| Dependency and referenced-document checks | Passed: seven pending work orders and ten new Markdown documents |
| git diff --check -- docs/specs docs/plans | Passed |

The coverage preflight supplied a planned ACP runtime path to exercise the
runtime coverage rules. No runtime file was changed. The check included new,
untracked work orders and their full referenced documents.

At the design-package checkpoint, implementation was pending; the
implementation-status section below supersedes this paragraph. The upstream
wire experiment and packaged-CLI build still have not run.

## Implementation status (2026-10-27)

Tasks 01, 05, and 07 are implemented with tests. Task 06's public documentation
is implemented. Tasks 02, 03, and 04 remain blocked on the upstream producer;
no consumer-only fixture is presented as continuity evidence.

| Work order | Status | Evidence |
| --- | --- | --- |
| 01 launch observations | Done | `opencode_background_test.go`, startup-contract and profile tests, targeted Go suites |
| 02 upstream producer | Blocked | No `OPENCODE_SOURCE` checkout or packaged artifact in this workspace |
| 03 automatic delivery | Blocked | Gated on 02 |
| 04 activity and input | Blocked | Producer sample and expected-idle admission gated on 03; legacy admission covered by 01 |
| 05 shared viewer | Done | Viewer/hook/store unit tests, i18n, desktop and phone browser suites |
| 06 executor proof and docs | In progress | Public docs updated; containers and automatic-response evidence gated on 02 |
| 07 default-on rollout | Done | Profile defaults + explicit-off tests, features-contract fallback test |

Implemented behavior:

- OpenCode `background: true` launches are recognized only when the same call's
  result confirms a running shell with a nonempty native shell ID. The launch
  tool may complete freely; the workload keeps its own identity.
- Unverified legacy launches become `unknown` after foreground settlement, with
  no fabricated exit status, output, control, or verified parked activity. They
  stop fencing later prompts and remain inspectable history.
- The shared viewer keeps a history entry after the active count clears, renders
  running/waiting/unknown counts separately, hides unsupported controls, labels
  unavailable output, shows retained exit status, and gives phone detail a
  full-height surface with 44px targets. Term/unknown states clear dynamic
  action availability even when an earlier declaration reported it.
- Provider capability declarations are persisted and replayed through the
  committed projection. Native Codex subagent frames carry their verified
  interrupt declaration; observation-only OpenCode stays `output=none` with no
  actions on both the live frame and HTTP/WS hydration.
- Provider run identity is explicit: OpenCode's shell ID is run identity for
  its launches, native Codex keeps its process-ID run, and normalized updates
  never synthesize a second run from the workload ID. Launch source identity is
  preserved across the unknown settlement.
- Shipped profiles default shared background work to enabled while the runtime
  flag and explicit environment/SQLite overrides remain authoritative.

Executed verification (2026-10-27):

~~~bash
(cd apps/backend && go test -trimpath -timeout 30m ./internal/agentctl/... -count=1)
(cd apps/backend && go test -trimpath -timeout 25m ./internal/agent/runtime/... -count=1)
(cd apps/backend && go test -trimpath ./internal/profiles ./internal/common/config \
  ./internal/runtimeflags ./internal/task/service ./internal/task/handlers -count=1)
(cd apps/backend && go test -trimpath -timeout 12m ./internal/orchestrator \
  -run 'Test.*(Background|Parked|Foreground|Turn|Stream|Queue|Prompt|Admission)' -count=1)
(cd apps/backend && go test -trimpath -timeout 18m ./internal/task/repository/sqlite \
  -run 'Test(Background|NewWith|.*Schema.*|.*Replay.*|.*Migrat.*)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && bash scripts/lint-go-changed <changed Go files>)
(cd apps/web && pnpm exec tsc --noEmit)   # 8 GB heap; the default heap OOMs
pnpm --dir apps/web exec vitest run lib/state/slices/session-runtime lib/background-work \
  components/task/chat/background-work
pnpm --dir apps/web exec vitest run      # full suite
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium \
  tests/task/opencode-background-work.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome \
  tests/task/mobile-opencode-background-work.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium \
  tests/task/parked-background-work.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
~~~

All passed. The existing parked-background suite still passes with the
default-on profile and the viewer changes.

The full `./internal/orchestrator` package does not terminate in this
environment (it also timed out at the default 600s before these changes); the
broad 123-second targeted subset above is the recorded evidence for that
package.

The full web suite ran before the review corrections as 3128 files and 28442
tests: 3127 files and 28437 tests passed, 4 skipped, and one unrelated file
(`lib/toast/sonner.test.ts`) timed out at its 5-second budget under host load.
That file passes in isolation (6/6 tests, 1.1s), so it is recorded as an
environment flake rather than a product failure. After the corrections, the
focused background-work, store, and capability suites (168 tests) pass.

Review corrections added regression coverage for: provider run identity (native
Codex item/process IDs with a normalized update), preserved launch association
across the unknown settlement, legacy wire shapes without the new negative
marker, explicit unverified launches, persisted capability evidence and its
schema migration, live Codex child interrupt availability, state-gated action
availability after terminal/unknown transitions, and the phone retained exit
status.

The PR fixup round additionally:

- Settled retained OpenCode launches on prompt error and user cancellation
  before the generic cancelled-tool sweep, with a cancel-path regression.
- Moved the additive capability column into the repository's migration stage
  and kept the fresh-table declaration.
- Declared explicit inspect-only capabilities for Claude ACP shell, subagent,
  and Monitor producers, with producer regressions.
- Removed the remaining acceptance-criteria reference from a production
  comment.
- Fixed two merged-base E2E test defects exposed by CI: the pseudo-locale call
  passed its URL arguments in the wrong order, and the duplicate-prompt toast
  assertion was not scoped to its error toast.
- Dispositioned every review thread: three Greptile findings fixed, one
  CodeRabbit finding fixed, and the production-default finding rejected with
  the accepted Task 07 rollout rationale.

Remaining verification gates: the Task 02 packaged-CLI wire proof, the
Docker/SSH/Kubernetes containers suite, and any claim of automatic follow-up
delivery.

## Risks

- OpenCode 2.2.0 cannot deliver the promised automatic follow-up over its current
  ACP subscription. A packaged upstream producer with proof is a release gate.
- The proposed metadata contract can change during upstream acceptance.
  Update the design and all fixtures before consumer implementation proceeds.
- Current shared design text contains proposed APIs alongside implemented seams.
  Source, targeted tests, and actual companion work-order results determine reuse.
- Automatic activity must not steal a user prompt or duplicate completion,
  billing, workflow transitions, or notifications.
- Connection loss cannot establish job termination. Initial recovery cannot
  promise native replay beyond the producer's demonstrated boundary.
- The two CI watches in the investigation are separate jobs. The viewer must
  not merge them because their commands refer to the same PR.
- The shared flag gates reads and process actions as well as UI. Default-on
  promotion therefore retains targeted capability and explicit-off tests.
