---
created: 2026-10-05
status: complete
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-003
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
legacy_specs: []
---

# Implementation plan: Model capacity continuation

## Overview

Recover capacity failures after completed tools through the existing retry scheduler and the same live provider conversation.
First establish immutable protocol evidence. Then admit and dispatch capacity continuation with desktop and phone proof.
The user selected continuation after completed actions, with automatic refusal for unresolved work.

## Root cause and evidence

The [reported task](https://kandev.cfl.tools/t/8b92657a-a8b2-42c7-8336-e98b4e75617c) failed at `2026-10-05T08:43:36Z`.
Its status records `failure_code=model_capacity`, `runtime_retained=true`, `recovery_disposition=refused`, and `attempts_started=0`.
Its preceding history contains completed tools and thinking output.

Capacity already classifies for short retry. The exponential delays already exist.
`handleTransientFailure` rejects post-output replay through `promptAttemptPreResultSafe`.
`continuationBindingForFailure` only accepts `CodeAgentTransportLost` with the existing read-only capability and enabled toggle.
Codex capacity therefore has no eligible recovery mode after tools.
The failure occurs before backoff reservation, rather than after retry exhaustion.

Existing `TestAgentTurnFailedSettlesDurableErrorWithoutStoppingRuntime` proves refusal with zero attempts and runtime preservation.
`TestTransientReplayUsesRetainedRuntime` proves that pre-result capacity uses the existing live-runtime scheduler.
Both baseline tests passed during design.

## Scope

### In scope

- A typed, current-generation capacity continuation snapshot for tested Codex ACP and isolated mock shapes.
- Completed-tool continuation on a retained runtime, with no original-prompt replay after effects.
- Shared retry budget, delay, cancellation, queue priority, ownership, and historical failure feedback.
- Focused native compatibility evidence, protocol/service regressions, and desktop/phone E2E evidence.
- Public recovery guidance when implementation lands.

### Out of scope

- New models, automatic provider switching, other error classes, and broader restoration support.
- Dynamic, Office, automation, utility, passthrough, and older unsupported remote shapes.
- Retrying pending tools, failed tools, ambiguous dispatch, unknown outcomes, or unaccounted background activity.
- New retry settings, runtime toggles, schedulers, persistence migrations, or task/session APIs.

## Technical approach

Task 01 owns `CapacityContinuationSnapshot` and ordered tool-outcome evidence.
It preserves the existing read-only `ContinuationSafetySnapshot` predicate.
The snapshot crosses stream, process outcome, lifecycle, and watcher boundaries without raw tool data.
Production Codex support requires a disposable native compatibility trace with unchanged process and conversation identity.

Task 02 adds a distinct capacity policy to the existing continuation binding and `transientRetryEntry`.
It reuses `handleTransientFailure`, `retryRetainedRuntimeContinuation`, and ordinary prompt admission.
Its internal instruction uses capacity wording and preserves completed actions.
Its dispatch cannot fall through to native restore when runtime continuity fails.
The capacity policy operates independently of the transport-loss toggle.
No provider-name branch belongs in orchestration.

| Provider and shape | Behavior | Evidence and fallback |
| --- | --- | --- |
| Codex ACP correlated systemError/capacity, retained runtime, known completed tools | Same-conversation continuation | Native disposable trace plus ordered protocol fixtures |
| Codex ACP capacity before work | Existing replay | Existing replay tests |
| Codex ACP pending, failed, conflicting, or unknown work | Refuse automatic continuation | Mixed-state protocol and service negatives |
| Codex ACP unaccounted collaboration/background work | Refuse automatic continuation | Start-only collaboration negative |
| Mock ACP with constructor-owned capability | Test-only equivalent | Real process traces and desktop/phone E2E |
| Cursor transport loss | Existing toggle/read-only contract | Existing continuation regressions |
| Other dialects, remote omission, unsupported ownership | Existing policy | Compatibility-negative tests |

## ASCII UI preview

UI-01: Shared task Chat region, capacity after completed tools.
This reuses the existing inline retry notice, not a new composition.
Labels illustrate localized content. Exact wording comes from the current catalogs.

```text
Before, from the reported task:
  [Completed tool calls]
  Selected model is at capacity.
  Automatic retry stopped.
  [Normal composer]

After, desktop and phone:
  [Completed tool calls]
  Model at capacity
  Continuing in 5s. Attempt 1 of 5.
  [Cancel]
  [Normal composer]

After success:
  [Completed tool calls]
  [Continued response]
  [Normal composer]

After exhaustion or refusal:
  Selected model is at capacity.
  [Actual retry result and technical details]
  [Normal composer]
```

Phone entry is the existing Chat route in `mobile/session-mobile-layout.tsx`.
The transcript retains one vertical scroll owner. The composer retains safe-area handling.
The inline Cancel control remains touch-accessible with at least a 44px phone target.
Text wraps without document overflow. Countdown updates do not steal focus.
The shared backend notice owns dispatch across reload and multiple viewers.
These structural outcomes map to `AC-PLATFORM-TURN-CONTINUITY-003.5`.

## Tests

| Criterion | Required evidence |
| --- | --- |
| `.1`, `.6` | `TestCodexCapacityContinuationEvidence` and `TestCapacityContinuationSnapshotRemoteRoundTrip` |
| `.2` | `TestCapacityContinuationAfterCompletedToolsUsesSameRuntime` |
| `.3` | `TestCapacityContinuationBudgetSurvivesProgress` and `TestCapacityContinuationSuccessResetsEpisode` |
| `.4` | `TestCapacityContinuationSupersession` and `TestCapacityContinuationRuntimeLossRefusesRestore` |
| `.5` | `TestCapacityContinuationFinalDisposition` plus desktop/phone rendered flows |

The assigned service regression names are implemented in the orchestrator and ACP protocol packages.
Keep existing replay, Cursor continuation, lifecycle retention, and diagnostic-classification regressions.

## E2E tests

Extend `tests/session/transient-turn-runtime-continuity.spec.ts` and its mobile counterpart.
Eligible completed tools must continue automatically on the same process, connection, native session, and execution.
Do not convert every existing refusal fixture into an eligible fixture.
Keep unknown-outcome coverage and add a fixture with both completed and pending tools.
Update shared helpers and mock scenarios for a capacity episode that receives a continuation instruction.
Assert that the original request appears once and that its completed side effect occurs once.
Run success, cancellation, exhaustion, reload, and second-viewer flows on desktop and phone.
Map those flows to criteria `.1` through `.5`.

## Work orders

- [x] [Task 01: Attest completed work for live capacity continuation](task-01-capacity-evidence.md) (done)
- [x] [Task 02: Dispatch capacity continuation and prove recovery](task-02-capacity-recovery.md) (done)

Task 02 depends on Task 01. Execution is sequential. No delegation is authorized.
Existing continuity and interruption packages remain completed historical evidence.
This amendment owns new delivery and does not rewrite their recorded test results.

## Verification results

Design baseline:

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^(TestAgentTurnFailedSettlesDurableErrorWithoutStoppingRuntime|TestTransientReplayUsesRetainedRuntime)$' -count=1)`: passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/decisions docs/specs docs/plans/model-capacity-continuation`: passed.
- Documentation coverage preflight: actual design-only changes are exempt, and planned source changes are covered.
- `git status --short -- docs/plans/model-capacity-continuation`: confirms all three new package files remain uncommitted.

Task 01 implementation and native compatibility evidence:

- `CapacityContinuationSnapshot` carries typed, generation-bound outcome evidence without tool content.
- Ordered ACP evidence accepts provider-confirmed completed read, execute, edit, and MCP calls; pending, failed, unknown, conflicting, duplicate, missing, unmatched, overflow, permission, and Codex collaboration evidence fails closed.
- Evidence crosses stream events, retained process outcomes, lifecycle terminal payloads, and watcher conversion. Stale-generation snapshots are dropped; retained snapshots are copied.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/instance ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'Capacity|Continuation|PromptFailureDisposition|TurnFailure' -count=1)`: passed.
- `git diff --check`: passed.

Task 02 implementation and verification:

- Capacity failures with known completed tools use a separate continuation policy on the retained conversation. It preserves the existing five-dispatch retry budget and does not enter native restore after runtime loss.
- Unsafe or missing outcome evidence, unsupported ownership, runtime loss, and inconclusive probes refuse automatic continuation. Progress keeps the retry episode; success, cancellation, supersession, and exhaustion settle through the shared retry owner.
- ACP application-error outcomes now carry the validated snapshot into the retained failure. Mock ACP scenarios model mixed completed/pending refusal, completed-work continuation without a duplicate effect, cancellation, and exhaustion.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'Capacity|Continuation|TransientReplay|Retained|TurnFailure' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./cmd/mock-agent -run 'Capacity|Continuation' -count=1)`: passed.
- `make -C apps/backend build`: passed, including the mock-agent binary used by E2E.
- Desktop continuity E2E (`chromium`): 4 passed, including refusal, retained retry, same-runtime completion across reload and second viewer, cancellation, and five-dispatch exhaustion.
- Phone continuity E2E (`mobile-chrome`): 4 passed, including the matching flows, a 44px Cancel target, and no horizontal overflow.
- `pnpm exec prettier --check` for the changed E2E helper and desktop/phone specs: passed. `gofmt -l` for changed Go files: no files reported.
- `node --test scripts/validate-public-docs.test.mjs`: 62 passed. `node scripts/validate-public-docs.mjs`: 47 pages validated.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.
- Final `git diff --check` and the documentation coverage preflight: passed.

## Documentation coverage preflight

Run this from the repository root after document edits and after implementation.

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const changed = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD']).toString().split('\0'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0'),
].filter(Boolean))];
const documents = [
  ...fs.readdirSync('docs/plans/model-capacity-continuation').map(p => `docs/plans/model-capacity-continuation/${p}`),
  'docs/specs/platform/requirements/transient-turn-runtime-continuity.md',
  'docs/specs/platform/system-design/transient-turn-runtime-continuity.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
for (const [label, paths] of [
  ['actual changes', changed],
  ['planned source coverage', [...changed, 'apps/backend/internal/orchestrator/event_handlers_transient.go']],
]) {
  const result = validateCoverage({ changedFiles: paths, fileContents });
  console.log(JSON.stringify({ label, status: result.status, errors: result.errors }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```

## Risks

- Completed ACP status does not guarantee exactly-once future model actions. The user accepted continuation from existing history.
- Codex background tools can expose start-only evidence. Those attempts must remain ineligible.
- Runtime loss after writes must never reach the existing restore fallback.
- Progress must not reset the five-attempt episode or clear unresolved evidence.
- Native compatibility evidence remains a production-support prerequisite, independent of passing mock tests.
