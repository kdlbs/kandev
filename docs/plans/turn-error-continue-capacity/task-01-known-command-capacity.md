---
id: "01-known-command-capacity"
title: "Retry capacity after confirmed command failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-003.1
  - AC-PLATFORM-TURN-CONTINUITY-003.2
  - AC-PLATFORM-TURN-CONTINUITY-003.3
  - AC-PLATFORM-TURN-CONTINUITY-003.4
  - AC-PLATFORM-TURN-CONTINUITY-003.5
  - AC-PLATFORM-TURN-CONTINUITY-003.6
  - AC-PLATFORM-TURN-CONTINUITY-003.7
  - AC-PLATFORM-TURN-CONTINUITY-003.8
  - AC-PLATFORM-TURN-CONTINUITY-003.10
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 01: Retry capacity after confirmed command failures

## Summary

Admit the reported Codex capacity case after commands with known unsuccessful terminal outcomes.
Use the existing backend retry episode and retained conversation. Prove the changed boundary with TDD and desktop/phone browser evidence.

## In scope

- Add the versioned Codex/mock V2 support values and bounded known-failed-command count.
- Recognize matching execute-tool terminal identity, nonzero integer exit, and absent cancellation/signal through ordered prompt evidence.
- Preserve blocking evidence across duplicate, partial, contradictory, late, and overflow updates.
- Keep successful and unsuccessful results distinct. Do not fabricate successful completion.
- Extend snapshot copying and JSON tests across process, lifecycle, and watcher boundaries.
- Align orchestration support, admission, refusal, and clean-successor predicates with the versioned contract.
- Reuse the existing capacity scheduler, instruction, budget, window, cancellation, and dispatch owners.
- Extend the isolated mock fixture and existing desktop/phone continuity suites for this case.
- Update the capacity eligibility sections in both public recovery pages through the docs-maintainer skill.

## Out of scope

- Manual Dismiss and continue controls, assigned to Task 02.
- V1 semantic changes, OpenCode failed-tool support, pending-tool recovery, or another provider capability.
- New scheduler, reset policy, model fallback, runtime restoration, flag, setting, or database migration.
- Mutation of the reported task or capture of unrelated protocol data.

## Acceptance

1. Codex V2 with only successful tools and confirmed terminal command failures enters the existing capacity episode on the same runtime.
   The original request occurs once, failed results remain visible, and continuation preserves completed work.
2. All blocked shapes remain refused. V1 and unknown versions keep their existing policy.
   Cancellation, stale ownership, configuration changes, queue priority, and runtime loss prevent delayed dispatch.
3. Desktop and phone show the existing retry notice, actual attempts, deadline, Cancel action, and truthful final feedback.
   The eleven-attempt limit and thirty-minute window remain unchanged across repeated failures, progress, reload, another viewer, and restart.

## UI preview

UI-04, excerpt from the [combined preview](plan.md#ui-preview):

| View | Waiting state | Required interaction |
| --- | --- | --- |
| Desktop | Existing capacity notice with reason, attempt, countdown, and deadline | Compact Cancel action in the existing notice |
| Phone | Same information wraps vertically inside Chat | Separate reachable Cancel row with an actual target of at least 44px |

No manual continuation control competes with automatic recovery.
The existing transcript remains the scroll owner. Composer drafts and safe areas retain their current behavior.
The existing successful and unsuccessful tool rows remain readable after recovery.
AC references: 003.2 through 003.5 and 003.10.
Validate this structure through the existing desktop and phone continuity suites after a fresh managed build.

## Verification

Run from the repository root. Read `/tdd`, backend and agentctl guidance, and mock-agent guidance before implementation.
Record the failing eligibility assertion before production edits. A missing fixture or import is not behavioral Red.

Bootstrap once if the worktree has no workspace dependencies:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'Test(CapacityContinuation|CodexCapacity|RecordTerminalOutcomeCopiesCapacity)' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'Test(CapacityContinuation|CapacityDelayedRetry)' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/session/transient-turn-runtime-continuity.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-transient-turn-runtime-continuity.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run changed-Go lint from the repository root, including newly created test files:

```bash
mapfile -t recovery_go_paths < <(git ls-files -m -o --exclude-standard -- 'apps/backend/**/*.go')
if ((${#recovery_go_paths[@]})); then
  bash scripts/lint-go-changed "${recovery_go_paths[@]}"
fi
```

Use `--no-build` only when no relevant source changed after a managed build.
Do not overlap the desktop and phone commands or override worker limits.
Record all actual commands and results, including the final changed-file list and rendered comparison with UI-04.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/capacity_continuation.go` and its tests.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_capacity_continuation.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_capacity_known_command_test.go` (new).
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex.go` and `dialect_mock.go`.
- Existing process `turn_outcome_test.go`, lifecycle `capacity_continuation_test.go`, and watcher `continuation_safety_test.go`.
- `apps/backend/internal/orchestrator/provider_interruption_continuation.go`.
- `apps/backend/internal/orchestrator/provider_interruption_known_command_test.go` (new).
- `apps/backend/internal/orchestrator/provider_interruption_capacity_delayed_test.go`.
- `apps/backend/cmd/mock-agent/capacity_continuity.go` and nearby scenario registration.
- `apps/web/e2e/helpers/transient-turn-runtime-continuity.ts`.
- `apps/web/e2e/tests/session/transient-turn-runtime-continuity.spec.ts` and its `mobile-` counterpart.
- `docs/public/tasks-and-workflows.md` and `docs/public/sessions-and-review.md`.

## Dependencies

None.

## Risks

- A terminal nonzero exit does not prove rollback. Keep original-prompt replay blocked after work.
- `terminalExitCode` alone ignores signal and terminal identity. The new proof must validate both.
- Another failed or pending tool must not disappear behind one qualifying command.
- Older hosts must reject V2. OpenCode V1 must retain its existing predicate.
- Synthetic prompt cleanup cannot supply terminal proof.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/transient-turn-runtime-continuity.md), requirement 003 and criterion 003.10.
- [Design](../../specs/platform/system-design/transient-turn-runtime-continuity.md), Capacity continuation evidence, admission, timing, and ownership.
- [Decision](../../decisions/2026-11-05-capacity-known-command-failures.md).
- Sanitized incident structure and existing protocol, retry-clock, and native-identity fixtures.
- Existing completed model-capacity and delayed-capacity packages. Their earlier counts are historical, not this task's results.

## Results

Done. Codex and mock now emit `codex_live_session_v2` / `mock_live_session_v2` with `known_failed_shell_tools`. V1 predicates, OpenCode support, and unknown support versions retain their prior conservative policy.

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'Test(CapacityContinuation|CodexCapacity|RecordTerminalOutcomeCopiesCapacity)' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'Test(CapacityContinuation|CapacityDelayedRetry)' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./cmd/mock-agent -run 'Test(ParseRetainedCapacityCmd|RetainedCapacity|HandleRetainedCapacity)' -count=1)`: passed.
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues.
- `(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/session/transient-turn-runtime-continuity.spec.ts)`: 5 passed.
- `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-transient-turn-runtime-continuity.spec.ts)`: 5 passed.
- `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.

New evidence covers `TestCapacityContinuationV2KnownFailedShellTools`, `TestCodexCapacityKnownCommandFailure`, `TestCapacityContinuationKnownCommandFailure`, and the mock `/capacity-known-failed-command` scenario. The adapter table pins raw-before-terminal, terminal-before-raw, same-frame, malformed raw, formatted return-code, string cancellation, alias conflict, and mixed pending-tool refusal, plus an agreeing split-frame positive; parsed raw exit evidence is retained per tool and compared in both arrival orders. The scenario emits matching terminal identity, integer nonzero exit, and null signal, and the continuation does not repeat the command. The desktop and phone suites assert the existing retry notice, actual attempt, deadline, Cancel action, same runtime identity, and preserved failed tool rows against UI-04.
