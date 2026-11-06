---
created: 2026-11-05
status: done
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
system_design:
  - ../../specs/agents/system-design/claude-background-shell-terminal-evidence.md
legacy_specs: []
---

# Implementation Plan: Claude background terminal exit

## Overview

Retire Claude background shells when the verified bridge reports their terminal
exit, even when it cannot report an exit code. One sequential work order covers
normalization, durable projection, and desktop/phone regression evidence.

## Confirmed defect and evidence

The read-only investigation examined task
`ed12acef-6faf-46a3-9d56-7d15f92b00af`, Kandev session
`eb88fa2a-493f-4fc7-85b0-427a350b5e97`, and native ACP session
`6fac977a-625b-424d-832a-bc90f1aec445`.

The live process opened `/root/.kandev/data/kandev.db`. Both rows in
`task_session_background_work` remain `running`, with revision 1 and no outcome.
The foreground session is `WAITING_FOR_INPUT`. Its initial bridge handshake
identifies official Claude bridge 0.97.0 in
`/root/.kandev/logs/backend-logs-2026-11-05-000004.log:5666`.

Evidence files under `/root/.kandev/logs/acp/`:

- `raw-acp-claude-acp-6fac977a-625b-424d-832a-bc90f1aec445.jsonl`
- `normalized-acp-claude-acp-6fac977a-625b-424d-832a-bc90f1aec445.jsonl`

| Job | Launch-complete raw line | Actual terminal-exit raw line | Exit time (UTC) | Normalized line |
| --- | --- | --- | --- | --- |
| `toolu_013N92i3bZ71EcbQTVRpRrZK` | 1191 | 1332 | 2026-11-05 12:40:53.709 | 1333 |
| `toolu_01Hkt1M5SFoBQZbdQ2Dj8EGw` | 1381 | 1457 | 2026-11-05 13:04:08.288 | 1459 |

Each exit contains the matching `terminal_id`, `exit_code:null`, and `signal:null`.
The normalizer emits no tool status and leaves background `ended=false`.
`terminalExitCode` recognizes only a numeric code. The adapter consequently
never attests termination, and the durable projection retains `running`.

Installed provider source confirms that `endBackgroundTerminal` emits these
frames after task termination. Its `withRunningTerminal` suppresses launch exit
while the command remains live. Installed bridge 0.81.2 has different launch
semantics, so unconditional interpretation would be unsafe.

The temporary `TestDiagnosticClaudeBackgroundTerminalExit` exercised the real
adapter conversion path. Its numeric-code control passed. Its captured null-code
case failed with `status=""`, `status_missing=true`, and `ended=false`.
The temporary test was removed. No live records or processes were changed.

## Scope

### In scope

- Recognize qualified Claude shell termination with an unavailable outcome.
- Preserve existing workload identity, history, accounting, and publication.
- Prove exact-job settlement with a mixed live/ended pair.
- Cover persisted and rendered desktop/phone behavior.

### Out of scope

- AIR async-task capabilities, SDK upgrades, background controls, and new turns.
- UI composition, translated copy, provider-version upgrades, and new flags.
- Live database edits, raw-log replay, historical backfill, and process restarts.

## Technical approach

Add a Claude dialect recognizer and call it from the existing locked tool-update
conversion. Emit an existing typed workload observation for an unavailable
outcome, avoiding the generic normalized projection's success assumption. Keep
verified detached-shell bindings across foreground prompt completion so a later
tool-scoped frame can settle the exact job. Use existing service persistence,
terminal monotonicity, and websocket delivery. The
[design](../../specs/agents/system-design/claude-background-shell-terminal-evidence.md)
defines the exact evidence conditions and recovery limits.

| Provider/transport | Evidence | Behavior | Verification/fallback |
| --- | --- | --- | --- |
| Official Claude ACP 0.97.0 | Matching terminal ID and valid nullable exit fields on tracked detached shell | End the matching job, retain unknown outcome | Captured sequence, fast and delayed variants |
| Claude ACP 0.81.2 or unverified version | Launch exit can use the same metadata name | Preserve existing behavior | Legacy combined launch-result fixture, process-end fallback |
| Other ACP providers | Existing numeric terminal evidence | Preserve current behavior | Existing shell-output and tool-update tests |
| Mock bridge under Claude provider identity | Test-owned official initialize tuple and captured updates | Same evidence rule | Mock tests and desktop/mobile E2E, no production bypass |
| Missing/mismatched identity or malformed metadata | No qualified end evidence | Do not settle another job | Negative adapter cases |

## Tests

Create `TestClaudeBackgroundTerminalExit` in
`adapter/transport/acp/claude_background_terminal_test.go`. Include the mixed
pair, null/numeric fields, fast completion, legacy launch exit, unknown version,
foreign provider, malformed/mismatched IDs, and duplicates.
This covers AC-AGENTS-BACKGROUND-WORK-010.3 and .6 and
AC-AGENTS-BACKGROUND-WORK-002.2.

Create `TestClaudeBackgroundTerminalProjection` in
`internal/orchestrator/claude_background_terminal_projection_test.go`.
Carry the converted event into persistence/publication. Assert one ended row,
one running sibling, preserved source/origin, null code, and terminal replay
monotonicity. Existing task-service tests remain the persistence regression gate.

## E2E tests

Add desktop `claude-background-terminal-exit.spec.ts` and phone
`mobile-claude-background-terminal-exit.spec.ts` under `e2e/tests/task/`.
Share a scenario helper. Use existing process-end and viewer helpers as patterns.

The fixture emits two detached shell launches, completes both launch tools,
then emits their separate null-code terminal exits. Advertise the verified
bridge initialize tuple only within this mock scenario. Use the mock provider
registered with canonical `claude-acp` identity. Restore test-owned environment
through `backend.useEnv` cleanup. No browser-store injection can replace wire evidence.

Both projects prove count two, count one after the first exit, and chip absence
after the second. Poll persisted rows before DOM assertions. Keep a typed draft
throughout. Open History through the existing desktop or phone entry, inspect
both ended records, and reload to prove durable settlement.
This covers AC-AGENTS-BACKGROUND-WORK-005.1, .8, .9 and
AC-AGENTS-BACKGROUND-WORK-010.5 and .6. There is no UI markup change.

## Work orders

- [x] [Task 01: Settle qualified Claude terminal exits](task-01-settle-terminal-exits.md)

## Verification results

- Diagnostic adapter repro: numeric control passed, null-code case failed as expected.
- Temporary repro removed; permanent adapter and durable-projection regressions added.
- `go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -count=1`: passed.
- `go test -trimpath -tags fts5 ./internal/orchestrator -count=1`: passed.
- Adapter and orchestrator review regressions: verified/legacy cleanup scope, exact terminal accounting retirement, live sibling retention, duplicate delivery, and stale execution isolation passed.
- Signal-terminated null-code frames settle as unavailable outcomes. Mock fixture tool-call IDs are unique per invocation, and a test-owned signal file releases terminal frames only after both running jobs are observed.
- `go test -trimpath -tags fts5 ./internal/task/service -run 'Test.*BackgroundWork' -count=1`: passed.
- `go test -trimpath -tags fts5 ./cmd/mock-agent -count=1`: passed.
- Desktop and mobile Playwright scenarios: passed (`1 passed` each, about 36 s and 33 s).
- Phone coverage is Chromium phone emulation only. No physical-device validation was performed; that gate remains open.
- `bash scripts/lint-go-changed <changed-go-files>`: passed (0 issues).
- `pnpm lint` and `pnpm run typecheck` in `apps/web`: passed.
- `python3 scripts/list-docs.py validate`: passed (431 decisions, 1921 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual documentation changes
  are exempt. A prospective adapter change validates as covered, with all three
  requirement references resolved through the new design and work order.
- Public docs remain unchanged: no command, API shape, configuration, label, or
  navigation change.

## Related delivery records

The original [background-work package](../agent-background-work/plan.md) and its
[process-end settlement](../agent-background-work/task-09-settle-orphaned-workloads.md)
retain their delivered scope. The [running/history package](../background-work-running-history/plan.md)
supplies unchanged selectors and entry points. The
[liveness package](../background-work-liveness/plan.md) retains prompt/admission
ownership. This repair does not reopen their completed work or reuse their
historical verification as evidence for this correction.

## Risks

- Older bridges can report terminal metadata at launch. Recognition must retain
  the verified identity/version condition.
- A generic ended boolean currently maps to success. Use the typed observation
  to retain the unknown outcome.
- Existing stale rows need verified process-end settlement. This patch alone
  cannot recover a null-exit frame consumed before deployment.
