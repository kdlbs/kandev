---
created: 2026-10-02
status: draft
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
legacy_specs: []
---

# Fix plan: Superseded FAILED session recovery

## Delivery priority

The user's later instruction prioritizes [managed npm startup resilience](../managed-npm-startup-resilience/plan.md).
This package is deferred secondary work. Its pending work orders are not prerequisites for startup recovery.
The earlier accepted scope remains recorded, but implementation should begin with the startup package.

## Overview

Prevent passive revival of a superseded FAILED conversation while a sibling is working.
The user selected this narrow scope for [issue 4152](https://github.com/kdlbs/kandev/issues/4152).
The task system owns the repair because it owns session activation and primary ownership.
Implement the admission guard first, then verify desktop and phone recovery.

## Evidence and root cause

Investigation used checkout `d22fbaab1fc`, 152 commits after the reported v0.96.0.
No live user instance was mutated. The issue contains no comments or image attachments.

`GetTaskSessionStatus` explicitly permits FAILED sessions with resumable tokens to recover.
`TestGetTaskSessionStatus_AutoResumesFailedSessionWithResumeToken` preserves that behavior.
`autoResumeEligibility` checks dynamic routing and queued ownership but does not inspect working siblings.
The web hook resumes an eligible selected conversation when it mounts.
The executor co-residency observer records a warning and deliberately permits concurrent execution.
Together, these paths explain how passive inspection can revive the old failure.

A temporary `TestIssue4152Diagnostic` seeded a non-primary FAILED session and a RUNNING primary sibling.
It called the real `autoResumeEligibility` and `passiveLaunchResponse` methods against the SQLite test repository.
The test failed both intended assertions: eligibility returned `true`, and passive launch returned no suppression.
Command: `(cd apps/backend && go test ./internal/orchestrator -run '^TestIssue4152Diagnostic$' -count=1)`.
Result: expected FAIL, two behavioral assertions, package runtime 0.129 seconds. The temporary file was removed.
This proves the admission gap, not the original incident's entire lifecycle or ACP failure cause.

Current web recovery sends `activation_source=session_open`; the issue reports `origin=manual` on v0.96.0.
The package tests current intent classification without treating that historical log as current behavior.

## Scope

### In scope

- Criteria AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.13 through 001.15.
- Status, passive launch, guarded admission, and deferred passive replay consistency.
- Failed inventory reads, mixed sibling states, ownership changes, and explicit recovery compatibility.
- Desktop and phone inspection with existing recovery controls and remembered selection.

### Out of scope

- General concurrent-writer locks, confirmation dialogs, or changes to explicit recovery.
- Always selecting primary, blocking all FAILED recovery, or disabling ordinary open-time recovery.
- npm prefix serialization, ACP retries, provider upgrades, or claiming issue 4152 is fully fixed.
- New persistent platform tasks, agents, migrations, feature flags, or metrics.

## Technical approach

Use the [existing design](../../specs/tasks/system-design/queued-session-ownership.md#superseded-failed-conversation-recovery)
and [accepted decision](../../decisions/2026-10-02-superseded-failed-session-recovery.md).
Extract the predicate into `apps/backend/internal/orchestrator/session_open_failed_recovery.go`.
Call it from `autoResumeEligibility`; retain current queue and dynamic-route restrictions.
Use the existing STARTING/RUNNING predicate from `sessionstate.IsWorking`.
Recheck fresh session ownership at the existing guarded resume admission boundary.
Return `failed_session_sibling_working` through existing status and suppressed launch fields.
Do not change the co-residency observer or explicit launch origin.

The browser already understands denied automatic recovery and suppressed launch dispositions.
Change browser logic only if the targeted tests expose a missing workspace or explicit-recovery path.
Any such change stays within this package's existing controls and contracts.

| Path | Expected result | Evidence |
| --- | --- | --- |
| ACP or native provider, session_open | Suppress the matching failed candidate before provider launch | Orchestrator tests |
| Explicit Resume or message-driven recovery | Existing admission, including concurrency | Orchestrator and web tests |
| session_focus without idle provenance | Existing refusal, no bypass | Launch test |
| Non-failed parked conversation | Existing automatic recovery | Existing session-open tests |
| Missing or ambiguous ownership | Suppress candidate passive recovery | Repository failure tests |

The guard is provider-neutral. These tests do not establish a provider-specific startup repair.
There is no new transport or unsupported-provider fallback.

## UI-01: Inspect a superseded failure

Entry: open the task and select its old FAILED conversation while a sibling is working.
The layout and controls already exist. The correction changes their passive execution behavior.

```text
Before (source trace): select FAILED A1 -> resume A1 beside RUNNING A2
After, desktop:
[ A1: Failed (selected) ] [ A2: Running, primary ]
[ A1 transcript and existing failure details     ]
[ Existing explicit recovery actions            ]

After, phone:
[ Task header                                   ]
[ Session picker: A1 Failed                      ]
[ A1 transcript and existing failure details     ]
[ Existing recovery actions, stacked             ]
```

The session remains selected and readable; no passive agent start occurs.
Retain existing localized controls, navigation, safe areas, and one chat scroll owner.
No new banner or label is required. The phone exemplar is the current session picker and recovery card.
The ASCII labels are explanatory, not new product copy. Criteria: 001.13 and 001.15.

## Tests

- `session_open_failed_recovery_test.go:TestSessionOpenFailedRecoveryStatusAndAdmission`: criterion 001.13; behavioral RED before implementation.
- `TestSessionOpenFailedRecoveryInventory`: criteria 001.13/14; mixed rows, read errors, missing candidate, and primary ambiguity.
- `TestSessionOpenFailedRecoveryRechecksOwnership`: criterion 001.14; deterministic status-to-admission changes and deferred replay.
- `TestSessionOpenFailedRecoveryExplicitCompatibility`: criterion 001.15; explicit launch, message recovery, primary failures, idle-only siblings, ordinary parked recovery.
- `use-session-resumption.test.ts`: no start projection or automatic launch after denied status; no fallback after suppressed launch; explicit recovery remains available.

## E2E tests

Add `tests/session/superseded-failed-session-recovery.spec.ts` for chromium.
Add `tests/session/mobile-superseded-failed-session-recovery.spec.ts` for mobile-chrome.
Seed a FAILED resumable non-primary session plus a working primary through isolated test fixtures.
Open, select, reload, and revisit the old conversation. Assert retained selection, failure, readable history, and no runtime start or prompt creation.
Then explicitly resume and assert the intended conversation starts without changing primary ownership.
Use a no-working-sibling control case. Criteria: 001.13 through 001.15.

## Work orders

- [ ] [Task 01: Guard passive failed-session recovery](task-01-guard-passive-recovery.md)
- [ ] [Task 02: Verify desktop and phone recovery](task-02-verify-conversation-recovery.md)

Execute sequentially. Task 02 depends on Task 01. No delegation is authorized.
Prior packages `queued-session-ownership`, `session-open-recovery-eligibility`, and `resume-todo-turn-boundary` retain their recorded delivery results.
This package adds an exception; it does not reopen their completed work orders or remove their compatibility tests.

## Verification results

Design checks passed on 2026-10-02:

- `python3 scripts/list-docs.py validate`: 341 decisions and 1295 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `git diff --check`: passed.
- `.github/scripts/pr-docs.cjs:validateCoverage`: both work orders covered, no errors.
  This local preflight supplied the package and its referenced documents with a projected task-operations change.
  It validates implementation traceability; it does not claim a live PR check ran.
- `git status --short`: only the requirement, design, decision, and plan package are changed.
- GitHub assignment read-back: `carlosflorencio`; issue remains OPEN.

The package is unstaged and uncommitted. The temporary diagnostic was removed.
Implementation and browser checks: pending, to run after a later implementation request.
No permanent test or production changes belong to this design turn.

## Risks and remaining ACP investigation

- A state-based admission check is not a workspace writer lock. Later authorized concurrent starts remain possible.
- WAITING_FOR_INPUT is intentionally not working for this guard. Existing explicit recovery remains capable of concurrent execution.
- Read failures must preserve inspection without accidentally permitting a fresh fallback.
- The issue's common npm prefix is confirmed; its role in the closed-pipe failure remains unproven.
- Further ACP investigation needs exact-version, isolated concurrent/sequential launch evidence and sanitized process-exit diagnostics.
  Distinguish npm failure, adapter process exit, and parent cancellation before choosing a repair.
- Keep issue 4152 open after delivering this narrow protection unless separate evidence resolves the launch failure.

## Follow-up: startup root-cause investigation

The user challenged the package's focus on 2026-10-02. This package remains secondary protection, not a fix for the original launch failure.

### Findings

The ACP SDK wraps a local `sendMessage` write error as JSON-RPC error -32603.
Thus, the displayed Internal error does not prove Claude returned an error response.
`process.Manager.waitForExit` calls `cmd.Wait`, which closes the managed stdin/stdout pipes after process exit.
The explicit stop path can also close stdin. The reported error alone cannot distinguish the earlier cause.

A standalone Go reproduction ran `sh -c 'exit 0'`, waited successfully, then accessed both pipes.
It produced exactly `write |1: file already closed` and `read |0: file already closed`.
This demonstrates that both strings can follow a normal child exit; it does not explain the incident's exit.

Both v0.96.0 and the current checkout wait 500 ms before agent initialization.
Failure observation near 500 ms can therefore reflect when Kandev first attempts the handshake.
It is not evidence of an npm-specific timeout. The agent subprocess uses `exec.Command`, not an HTTP-scoped `exec.CommandContext`.

### Exact adapter version probes

Probes used `@agentclientprotocol/claude-agent-acp@0.81.2`, Node 24.16.0, and npm 11.13.0 on Linux.
Each experiment used disposable HOME, cache, prefix, temporary directory, and workspace paths.
No user credentials or task prompts were supplied.
Processes within each wave shared both the npm prefix and npm cache.

| Experiment | Result |
| --- | --- |
| Three concurrent cold-cache acpdbg probes | 3/3 initialize and session/new succeeded |
| Three concurrent warm-cache acpdbg probes | 3/3 initialize and session/new succeeded |
| Two sequential acpdbg probes | 2/2 initialize and session/new succeeded |
| Three cold concurrent launches, initialize delayed one second | 3/3 initialize succeeded |
| Three warm concurrent launches, initialize delayed one second | 3/3 initialize succeeded |

The delayed probes held stdin open before initialization. No process exited before the request or successful response.
Some full probes emitted connection-closed diagnostics during intentional teardown after successful session creation.
Those shutdown messages are not startup failures.

Installed npm 11.13.0 stores npx packages under the package-hashed `_npx` cache and uses a concurrency lock around installation.
The issue's shared project prefix alone does not establish concurrent cache corruption.
These results do not exclude a race with another npm version, existing cache state, or full Kandev orchestration.
The reporter's Node/npm versions, exact environment, and process-exit records remain unavailable.

### Retained local evidence and next diagnostic boundary

- ACP probe frames and summary: `/tmp/kandev-4152-probe-2fqurg00/`.
- Delayed initialization frames and summary: `/tmp/kandev-4152-delayed-pznnfg22/`.
- These are temporary local artifacts, not committed portable fixtures.
- All probe runtime directories were removed after the processes exited. No production code changed.

The next evidence must come from the affected installation: Node/npm versions and agentctl logs spanning process start, stop requests, exit, and initialization.
Capture exit code or signal, stderr, and exact ordering for the three failed launches and one successful retry.
For the issue's timestamps, inspect approximately 08:28:00 through 08:32:45 in the reporter's log timezone.
Include kernel OOM records only if the exit evidence indicates a kill or unexplained disappearance.
Do not change npm locking, add generic retries, or classify the original failure as solved without that evidence.
