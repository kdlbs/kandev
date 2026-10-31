---
created: 2026-10-28
status: complete
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-003
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
legacy_specs: []
---

# Implementation plan: OpenCode capacity retry

## Overview

Treat the observed OpenCode admission refusal as temporary capacity pressure.
Preserve the usable ACP runtime and continue unfinished work through the existing retry owner.
Implement the backend path first. Then prove desktop and phone behavior and update public recovery guidance.

The October 28 specification amendments are implemented and accepted.
Existing accepted requirements and completed packages retain their historical status.

## Evidence and root cause

Affected identities:

- Task: `e1b5dc09-6c02-460f-991f-343774c88324`.
- Kandev session: `ab5f7703-3b1f-4ad8-9fad-3361a254c065`.
- Native session: `ses_e7b66e828ffeMm3rHKLl8Qksaw`.
- Failed execution: `0d40894c-2d5e-404b-b2ff-f52569e4aeb4`, prompt generation 1.

Read-only baseline on October 28:

- `/root/.kandev/data/kandev.db` binds those identities and records one generation.
  The session changed from `RUNNING` to `WAITING_FOR_INPUT` with a blocking provider error.
- `/root/.kandev/logs/acp/raw-acp-opencode-acp-ses_e7b66e828ffeMm3rHKLl8Qksaw.jsonl`, lines 996-1015,
  records native attempts 2-11 between `22:26:20.163Z` and `22:27:48.070Z`.
  Their error type is `provider.rate-limit`, with status `429` and the exact `ttft_admission_refused` diagnostic.
  Intervening null updates clear native retry state.
- The corresponding `normalized-acp-...jsonl` preserves these as `session_info.session_meta`.
  The capture contains 95 distinct native tool calls, all with final status `completed`.
  No duplicate call IDs, unmatched updates, or structured nonzero exit codes were observed.
  These summaries do not establish post-error liveness or account for uncaptured work.
- `/root/.kandev/logs/backend-logs-2026-10-28-000025.log`, lines 63960-64001,
  records the final prompt failure at `22:27:49Z`.
  Classification is low-confidence `agent_runtime_error`, rule `phase.poststart.unknown`, with `auto_retryable=false`.
  Retry admission reports `capacity_continuation=null` and `pre_result_safe=false`.
- The session's persisted status records the same final diagnostic with an `after 21 s` suffix.
  Raw notification logging does not contain the terminal JSON-RPC response.
  Do not describe the persisted or backend error as a captured response envelope.

The source confirms three independent gaps:

1. `routingerr` has no signature for this admission refusal.
2. `Adapter.retainableACPApplicationError` excludes the OpenCode provider.
3. `newACPDialect` has no OpenCode capacity capability, so no complete snapshot reaches the existing continuation path.

The original prompt already produced tool work. Removing the replay fence would repeat completed actions.
The repair must supply typed retention and continuation evidence instead.

## Scope

In scope: bounded signature classification, short retry hints, OpenCode runtime retention,
current-generation capacity evidence, and existing backoff admission.
Protocol/service regressions, native compatibility evidence, desktop/phone proof, and public guidance complete delivery.

Out of scope: provider or model switching, general internal-error retries, and automatic restoration after effects.
Office/dynamic/automation policy changes, new settings, release toggles, migrations, and a second retry scheduler are also excluded.
Do not change the reported task or use its conversation for reproduction.

## Technical approach

Task 01 owns the complete backend path:

- Add `opencode.capacity.admission_refused.v1` and bounded seconds parsing in `routingerr`.
- Add `newOpenCodeACPDialect` and `CapacityContinuationOpenCodeLiveSessionV1`.
  Extend only the tested capacity application-error retention path.
- Reuse the ordered tool ledger, `promptFailureDisposition`, typed stream/outcome fields,
  lifecycle `AgentTurnFailed`, and current orchestration capability checks.
- Preserve the original-prompt replay fence. After completed work, use `retryRetainedRuntimeContinuation`.
- Reuse eleven host attempts within thirty minutes, with the existing capped exponential ladder.
  For the first five capacity slots, a validated short hint supplies a minimum wait.
  Non-capacity timing stays unchanged.
- Keep native retry updates advisory. A host timer starts only after terminal settlement.

Task 02 adds a production-shaped OpenCode mock fixture and full user-flow proof.
The fixture must traverse the OpenCode dialect and classifier, rather than obtain eligibility solely through the mock capability.
Keep all test-only wiring inside existing isolated mock construction.
Reuse the current localized retry notice. No React markup or locale changes are planned.

### Compatibility

| Provider and shape | Intended behavior | Evidence and fallback |
| --- | --- | --- |
| OpenCode ACP exact terminal refusal, settled live runtime, completed tools | Continue in the same conversation | Captured-shape protocol tests, native compatibility trace, service and browser proof |
| Same refusal before output or effects | Existing safe replay on the retained runtime | Pre-result fence regression |
| OpenCode native retry metadata while prompt remains active | No host dispatch | Ordered metadata/terminal boundary regression |
| OpenCode with completed and pending, failed, or unknown tools | Retain healthy runtime; require manual continuation | Mixed-state protocol and service negatives |
| OpenCode auth, quota, generic internal error, or unrelated token | Existing error policy | Classifier and retention negatives |
| OpenCode child/background work without accountable completion | Refuse automatic continuation | Background and child-session negatives |
| Codex capacity | Existing continuation and limits; short hints cannot reduce nominal waits | Existing capacity suites plus hint-floor regression |
| Unknown capability, older remote omission, or broken ACP | Existing conservative recovery | Round-trip, liveness, and version-skew negatives |
| Cursor transport loss, Office, dynamic, automation, utility, passthrough | Existing owners and policy | Existing compatibility negatives |

## Existing Chat presentation

No presentation code changes are planned.
Eligible OpenCode failures enter the existing capacity state on desktop and phone.
The notice shows the condition, attempt, next retry, window deadline, and Cancel action.
Chat retains its scroll owner, the phone Cancel target, and composer safe-area behavior.
The normal composer remains available after cancellation or final failure on a healthy runtime.

## Tests

| Acceptance criteria | Required tests |
| --- | --- |
| `AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.5`, `.11` | `TestOpenCodeAdmissionCapacityClassification`, `TestOpenCodeAdmissionCapacityResetHint` |
| `AC-PLATFORM-TURN-CONTINUITY-001.1`-`.3` | `TestOpenCodeCapacityPromptFailureRetainsRuntime`, `TestOpenCodeCapacityNativeCompatibility` |
| `AC-PLATFORM-TURN-CONTINUITY-003.1`, `.6`, `.9` | `TestOpenCodeCapacityContinuationEvidence`, `TestOpenCodeCapacityNativeRetryDoesNotDispatch`, mixed-state negatives |
| `AC-PLATFORM-TURN-CONTINUITY-003.2`, `.4`, `.8` | `TestOpenCodeCapacityContinuationUsesSameRuntime`, `TestOpenCodeCapacityContinuationSupersession` |
| `AC-PLATFORM-TURN-CONTINUITY-003.3`, `.7` | `TestOpenCodeCapacityRetryHintFloor`, existing delayed-capacity deadline/restart suites |
| `AC-PLATFORM-TURN-CONTINUITY-002.1`-`.5`, `003.5`, `003.9` | Task 02 desktop/phone flows and final durable status assertions |

The assigned tests are delivered; exact commands and results are recorded under
Verification results. Existing Codex, Cursor, replay, runtime-loss, and
remote-omission regressions are retained.

## E2E tests

Create `apps/web/e2e/tests/session/opencode-capacity-retry.spec.ts` for `chromium`.
Create `mobile-opencode-capacity-retry.spec.ts` for `mobile-chrome`.
Share helpers under `apps/web/e2e/helpers/opencode-capacity-retry.ts`.

Both projects prove these outcomes:

- Native retry metadata does not start a competing host prompt.
- Terminal refusal after a completed effect shows one capacity notice and later continues on the same runtime.
- The original request and its completed effect occur once. No initialize, load, resume, or new occurs during recovery.
- Continued capacity failures preserve the original episode, including a continuation without another completed tool.
- Cancel accepts later user input and prevents the old timer from dispatching.
- Reload and another viewer retain one notice without additional dispatches.
- Mixed completed/uncertain work remains manual. Exhaustion or expiry leaves truthful feedback and a usable composer.
- Phone controls remain reachable; wrapped error details do not cause horizontal overflow.

Use the existing isolated backend retry clock. Do not accelerate provider timing through browser time alone.
Run both projects sequentially with the managed runner.

## Work orders

- [x] [Task 01: Admit OpenCode capacity continuation](task-01-opencode-capacity-continuation.md)
- [x] [Task 02: Prove recovery and update guidance](task-02-recovery-proof.md)

Task 02 depends on Task 01. Execute sequentially. No delegation is authorized.
The completed continuity, capacity-continuation, and delayed-capacity packages remain historical evidence.

## Verification results

Design evidence:

- A temporary `TestOpenCodeAdmissionCapacityRepro` ran against the pre-implementation classifier.
  It failed because the exact reported text returned `agent_runtime_error`, low confidence, and no automatic retry.
  The permanent replacement is `TestOpenCodeAdmissionCapacityClassification`.
- `python3 scripts/list-docs.py validate`: passed; 415 decisions and 1824 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: passed; 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.

Implementation evidence:

- Classifier and hint suite: `go test -trimpath -race -tags fts5 ./internal/agent/runtime/routingerr -run 'OpenCode|Signature|Classify|ResetHint' -count=1` passed.
  `TestOpenCodeAdmissionCapacityClassification` and `TestOpenCodeAdmissionCapacityResetHint` cover the exact terminal text, collision negatives, unrelated providers, precedence, and the 1-to-60 second bound.
  The adapter retention gate requires the bounded rule, so a broad capacity phrase or another OpenCode provider error keeps the existing path.
- Protocol, process, lifecycle, and watcher suite:
  `go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'OpenCodeCapacity|Capacity|Continuation|PromptFailure|Retained|TurnFailure' -count=1` passed.
  The full ACP package also passed (`go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -count=1`, 56.6 s).
- Orchestrator suite: `go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'OpenCodeCapacity|Capacity|Transient|AgentTurnFailed|Retained|Continuation' -count=1` passed.
  The full orchestrator package also passes under `-race` with `-timeout 25m` (820 s), above the default ten-minute package timeout; the assigned selection is the fast gate.
- The lifecycle retention allowlist now includes `opencode-acp`. `TestTransientTurnFailureRequiresCurrentEligibleRuntime` uses an unproven provider for its negative case, and `TestCapacityContinuationRetainsOpenCodeRuntime` proves the retained turn publishes `agent.turn_failed` with the OpenCode snapshot.
- Native compatibility: `KANDEV_TEST_OPENCODE_NATIVE=1 go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestOpenCodeCapacityNativeCompatibility$' -count=1` passed against `opencode v2.2.0` (85.1 s after remediation; 88.6 s on the first run).
  A local controlled provider returned the captured refusal; OpenCode settled after 12 provider requests in 82.7 s, and a continuation prompt completed in 293.8 ms on the same process, connection, and native session.
  The test skips unless opted in, and fails rather than skips once opted in when the binary or provider is unavailable.
- Browser proof: `delayed-capacity-retries.spec.ts` plus `opencode-capacity-retry.spec.ts` passed 9 tests (1.2 m) on `chromium`; `mobile-delayed-capacity-retries.spec.ts` plus `mobile-opencode-capacity-retry.spec.ts` passed 6 tests (45.9 s) on `mobile-chrome`.
  The OpenCode suites passed again after review remediation (5 desktop tests in 1.6 m; 2 phone tests in 38.9 s).
  The fixture registers the mock binary under the real `opencode-acp` identity, so the production dialect and classifier both run.
  The trace asserts one process, one connection, one native session, no load or resume, one completed effect, and seven prompts for the six-dispatch flow.
- Review remediation: the capacity ledger now fences unresolved OpenCode
  detached launches independently of the background-inspection feature and fails
  closed on missing or contradictory workload evidence; the service dispatches
  nothing for unresolved work. The relative reset suffix is read only from the
  diagnostic line that matched the capacity signature, so unrelated stderr or
  stdout cannot supply a delay. Remediation regressions:
  `TestOpenCodeCapacityDetachedLaunchBlocksContinuation`,
  `TestOpenCodeCapacityDetachedLaunchFailsClosed`,
  `TestOpenCodeCapacityOrdinaryForegroundExecuteStaysSafe`,
  `TestOpenCodeCapacityUnresolvedBackgroundDoesNotDispatch`, and
  `TestOpenCodeAdmissionCapacityHintIgnoresUnrelatedLines`.
  A terminal exit resolves a launch only after its identity was observed, and
  conflicting identity spellings fail closed even without a status.
- Changed-Go lint: `bash scripts/lint-go-changed` reported zero issues for every changed Go file.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed. `node scripts/validate-public-docs.mjs`: 54 pages passed.
- Documentation coverage preflight: passed for actual changes and planned source coverage.
- `git diff --check`: passed.

## Risks

- Native compatibility evidence covers OpenCode 2.2.0.
  A later CLI release can change the wire shape, so the bounded signature and capability value must be re-validated before support is extended to another shape.
- Completed ACP statuses do not guarantee exactly-once model actions. Continuation preserves the existing safety policy.
- Native and host attempts are different counters. Only actual host dispatches consume the host budget.
- Short hints must not flatten exponential delays or extend the fixed admission window.
- New capability values must fail closed on older components without causing recovery through native restoration.

## Documentation coverage preflight

Run from the repository root after each package update:

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const changed = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD']).toString().split('\0'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0'),
].filter(Boolean))];
const dir = 'docs/plans/opencode-capacity-retry';
const documents = [
  ...fs.readdirSync(dir).filter(p => p.endsWith('.md')).map(p => `${dir}/${p}`),
  'docs/specs/agents/requirements/provider-error-signatures.md',
  'docs/specs/agents/system-design/provider-error-signatures.md',
  'docs/specs/platform/requirements/transient-turn-runtime-continuity.md',
  'docs/specs/platform/system-design/transient-turn-runtime-continuity.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const planned = [
  'apps/backend/internal/agent/runtime/routingerr/rules.go',
  'apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_opencode.go',
  'apps/backend/internal/agentctl/types/streams/capacity_continuation.go',
  'apps/backend/internal/orchestrator/provider_capacity_retry_policy.go',
  'apps/backend/cmd/mock-agent/opencode_capacity.go',
  'apps/web/e2e/tests/session/opencode-capacity-retry.spec.ts',
  'docs/public/tasks-and-workflows.md',
];
for (const [label, paths] of [['actual', changed], ['planned', [...changed, ...planned]]]) {
  const result = validateCoverage({ changedFiles: paths, fileContents });
  console.log(JSON.stringify({ label, status: result.status, errors: result.errors }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```
