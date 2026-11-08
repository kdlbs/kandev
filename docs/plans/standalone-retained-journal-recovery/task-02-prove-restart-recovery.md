---
id: "02-prove-restart-recovery"
title: "Prove retained journal recovery across restart"
status: completed
wave: 2
depends_on:
  - "01-reconcile-standalone-journal-owner"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
system_design:
  - ../../specs/platform/system-design/durable-agent-journal-ownership.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 02: Prove retained journal recovery across restart

## Summary

Prove that a retained standalone journal remains recoverable after host tracking
is recreated. Use real instance teardown and journal ownership, then run the
existing desktop and phone restart flows against fresh builds.

## In scope

- Add `TestStandaloneRetainedJournalRestartRecovery` with real instance/control managers and SQL delivery records.
- Keep the same disposable standalone control server and journal across two backend-side lifecycle owners.
- Drive the pre-creation path with no host execution tracked and the old manager still present.
- Cover a stopped retained manager, a live initialized peer, blocked partial teardown, and a second restart.
- Preserve session/native identity, journal records, ordered replay, and uncertain submissions.
- Verify one later explicit instruction through normal admission after successful recovery.
- Keep an unrelated session usable while another session remains blocked.
- Run the existing desktop and mobile survival/restart specs with their current scenarios intact.
- Recheck public documentation and synchronize affected implementation evidence.

## Out of scope

- New fault endpoints, production-only test branches, or mutation of a developer instance.
- New UI states, labels, layout, dialogs, or phone navigation.
- Full repository verification, remote infrastructure provisioning, and provider capability expansion.
- Changing the ownership policy established in Task 01.

## Acceptance

1. Real-manager restart integration enters the formerly unconditional creation
   path. Stopped-manager replacement releases its lock and preserves retained
   records. Live adoption retains its actual execution and dispatches no prompt.
2. Repeated restart, interruption after DELETE, failed teardown, and blocked
   ownership preserve uncertainty and converge without duplicate owners or
   effects. A later distinct instruction starts exactly once after ordinary admission.
3. Desktop and phone restart scenarios preserve the conversation and usable
   controls. Results name discovered tests and actual counts. Package and
   public-documentation checks describe the final implementation accurately.

## Integration fixture

Use an isolated root, real `instance.Manager`, control API, and local HTTP
clients. Use the existing synthetic agent server or mock-agent binary for the
native protocol. Seed real SQL harness generation, cursor, and submission rows.
Keep the agentctl-side manager alive when the first backend-side owner detaches.

Create the second lifecycle owner from persisted state without a tracked host
execution. Trigger its actual `Launch` or workspace creation entry point.
Record authenticated inventory/status/DELETE/POST requests and provider actions.
This must reach Task 01's resolver. Merely replaying normal startup recovery
does not exercise the defect.

For the stopped case, preserve a committed event and an uncertain submission.
Prove process termination through fixture-owned evidence, then exercise the
existing native restore rules. Do not mark uncertainty successful or drain a queue.
For the live case, retain its process/native association, generation, stream,
active submission, and current cursor without initialize/load/resume/new/prompt.

Interrupt after a completed DELETE but before POST, recreate the backend owner,
and resume again. Also inject one DELETE error and one lost response.
Verify exact absence before retry and retain records on unknown outcomes.
Add a second unrelated session to every blocked-state scenario.
Finish by stopping only fixture-owned resources and checking journal reopen.

## Desktop and phone evidence

Use `tests/session/agent-survival-restart.spec.ts` and
`tests/session/mobile-agent-survival-restart.spec.ts`.
Keep their existing local/worktree coverage and configured mobile device.
Use the current task chat and `SessionPage` entry points.
Preserve the existing phone scroll owner, safe-area clearance, and touch targets.

Inventory the existing scenarios before adding assertions. Retain their
configuration, output replay, absent restart form, and prompt-count controls.
Add only missing assertions for actual execution identity and later-turn acceptance.
Do not replace existing flows with metadata-only recovery banners.

No rendered UI change is planned, so no ASCII preview is required.

## Verification

Run each command from the repository root. Browser runs are sequential.
Workspace dependencies already exist at the investigation baseline.
For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestStandaloneRetainedJournalRestartRecovery$' -count=1 -v)
mapfile -t standalone_changed_go < <(
  {
    git diff --name-only HEAD -- apps/backend
    git ls-files --others --exclude-standard -- apps/backend
  } | rg '\.go$' | sort -u
)
if ((${#standalone_changed_go[@]})); then
  bash scripts/lint-go-changed "${standalone_changed_go[@]}"
fi
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/agent-survival-restart.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-agent-survival-restart.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/standalone-retained-journal-recovery
```

If E2E helper logic changes, run its existing unit tests with exact file paths.
Record expanded lint paths, helper commands, and discovery counts in Results.
Run this repository documentation-coverage preflight from the repository root.
It includes the planned production entry point so documentation exemption cannot
hide broken work-order references. A spec-lint pass alone does not prove traceability.

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/standalone-retained-journal-recovery';
const files = fs.readdirSync(dir).filter(name => name.endsWith('.md')).map(name => `${dir}/${name}`);
files.push(
  'docs/specs/platform/requirements/durable-agent-delivery.md',
  'docs/specs/platform/system-design/durable-agent-journal-ownership.md',
  'docs/specs/platform/system-design/durable-agent-reattachment.md',
);
const fileContents = Object.fromEntries(files.map(name => [name, fs.readFileSync(name, 'utf8')]));
const changedFiles = files.map(filename => ({ filename, status: 'modified' }));
changedFiles.push({ filename: 'apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go', status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ status: result.status, workOrders: result.workOrders.length, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
JS
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/standalone_retained_journal_restart_test.go` (new)
- `apps/web/e2e/tests/session/agent-survival-restart.spec.ts` (only missing assertions)
- `apps/web/e2e/tests/session/mobile-agent-survival-restart.spec.ts` (only missing assertions)
- `docs/plans/standalone-retained-journal-recovery/plan.md`
- `docs/plans/standalone-retained-journal-recovery/task-01-reconcile-standalone-journal-owner.md`
- `docs/plans/standalone-retained-journal-recovery/task-02-prove-restart-recovery.md`

Inspect `docs/public/executors.md` and `docs/public/configuration.md` under
`/docs-maintainer`. Their current lifetime and blocked-journal explanations
already match the required behavior. Change them only if the final diff creates
an operator-visible contract. Preserve existing local survival defaults.

## Dependencies

Task 01 must pass its targeted checks before this work order starts.

## Risks

- A mocked journal error does not prove lock release. The fixture must hold a real bbolt handle.
- A normal startup test can miss the pre-creation path. Assert the resolver's recorded request sequence.
- Browser output alone cannot prove absence of provider dispatch. Keep provider-action and message counters.
- Slow retained work must use deterministic gates, not sleeps.
- Linux E2E evidence does not prove native macOS or Windows survival support.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md) and [Task 01](task-01-reconcile-standalone-journal-owner.md).
- [Journal ownership](../../specs/platform/system-design/durable-agent-journal-ownership.md#standalone-ownership-resolution).
- [Reattachment](../../specs/platform/system-design/durable-agent-reattachment.md).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md), criteria listed above.
- Existing `manager_journal_admission_test.go`, `manager_startup_adoption_test.go`, and retained-delivery recovery fixtures.
- Existing `agent-survival-restart.spec.ts`, mobile counterpart, and silent-restart helpers.
- `.agents/skills/e2e/SKILL.md` and `.agents/skills/mobile-parity/SKILL.md`.

## Results

Implemented.

Integration:
- Added `TestStandaloneRetainedJournalRestartRecovery` with a real
  `instance.Manager`, real loopback control and instance HTTP servers, a real
  bbolt journal, and backend-side lifecycle recreation. It covers:
  - stopped-owner disposal and replacement across two backend-side owners;
  - repeated restart preserving committed events and uncertain submissions;
  - a live owner adopted without CREATE, DELETE, or provider dispatch;
  - a create interruption after a completed DELETE, recovered by a later
    authenticated attempt;
  - failed and lost DELETE outcomes, with an unrelated session preserved and
    usable throughout.
- Retained records survive teardown and reopen. A dispatching submission is
  reclassified as `interrupted_unknown`, never marked successful.
- A manager-level regression proves a workspace-first restore through
  `GetOrEnsureExecution` -> `Launch` -> `StartAgentProcess` on a bound live
  peer. It captures the persisted built-in MCP binding, restores it unchanged,
  verifies with GET only, performs no fresh-helper prepare/rebind, and dispatches
  no ACP lifecycle action.
- A combined lifecycle/restart regression drives the real pre-creation resolver
  through `GetOrEnsureExecution` and `StartAgentProcess` with a real instance
  manager, control server, and journal. It asserts the retained execution
  identity, GET-only built-in verification, and no startup dispatch.
- A lost create response after the replacement journal owner was created is
  reconciled by the next attempt, which disposes that exact owner before
  reopening its journal.
- The existing mobile dynamic-unclassified-fallback E2E was reproduced during
  fixup and passes after persisted Stop repairs the runtime row instead of
  deleting its resumable evidence.
- A task-service regression proves workspace restore carries the persisted
  built-in binding from the runtime inventory into `WorkspaceInfo`.
- The E2E runs exercise the full backend with its real SQLite persistence,
  native conversation identity, output replay, and a later explicit
  instruction through the ordinary task chat.

Verification:
- `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestStandaloneRetainedJournalRestartRecovery$' -count=1 -v` passed (four subtests).
- `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestStandaloneRetainedWorkspaceRestoreKeepsBuiltinBinding$' -count=1 -v` passed.
- `go test -trimpath -tags fts5 ./internal/task/service -run '^TestGetWorkspaceInfoForSessionCarriesRetainedBuiltinBinding$' -count=1 -v` passed.
- `bash scripts/lint-go-changed` over all changed and untracked Go files: 0 issues.
- Full `./internal/agent/runtime/lifecycle` package passed (final run 170.5s).
- Full `./internal/task/service` package passed (final run 84.1s).
- `./internal/agentctl/server/process` package passed (final run 231.3s).
- Desktop: `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/agent-survival-restart.spec.ts` passed 3 tests.
- Mobile: `pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-agent-survival-restart.spec.ts` passed 2 tests.
- `python3 scripts/list-docs.py validate`: 437 decisions and 1937 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Documentation coverage preflight: `covered`, 2 work orders, no errors.
- `git diff --check`: clean.
- Public documentation rechecked. `docs/public/executors.md#agent-lifetime-and-reconnect`
  and the `features.agentSurvival` row in `docs/public/configuration.md` already
  describe the required behavior. The final diff adds no operator procedure,
  configuration key, or UI contract, so no public text changed.
