---
id: "01-fresh-start-generation"
title: "Fresh start advances the delivery generation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Fresh start advances the delivery generation

## Scope

An explicit Start fresh on a session with a committed harness generation must
launch a new native conversation on generation N+1 with its own stream.

In scope:

- The executor marks a `fresh_start` resume request with
  `ReplaceNativeConversation`. `applyDeliveryIdentity` advances the generation
  for it when a generation is committed, and otherwise clears the mark
  (`orchestrator/executor/executor_resume.go`, `executor_delivery_identity.go`).
- `backendapp/adapters.go` and the lifecycle launch path carry the mark to the
  execution (`ReplacesNativeConversation`). A retained workspace-only or
  older-generation execution is relaunched for the successor, like context
  continuation (`agent/runtime/lifecycle/manager_launch.go`, `executor_backend.go`).
- Session initialization retires unresolved predecessor submissions before the
  first prompt (`agent/runtime/lifecycle/session.go`).
- Replacement events wait for the successor commit through the existing bounded
  successor-generation wait (`durable_silent_restore_generation.go`).
- The ACP session-created payload carries the mark. On that event the
  orchestrator commits generation N+1 with a predecessor compare-and-set and
  creation reason `native_replaced`, only after the event's resume token is
  accepted for the current execution. The generation transaction revalidates
  task, incarnation, execution and native token while holding the executor
  writer's SQL locks (`agent/runtime/lifecycle/events.go`,
  `orchestrator/watcher/watcher.go`, `orchestrator/event_handlers.go`,
  `task/repository/sqlite/session_continuity.go`).

Exclusions: no change to Resume identity, block resolution, UI, schema, metrics,
or legacy (non-durable) delivery. When no generation is committed,
generation-one behavior stays unchanged. Office fresh start relaunches through
its scheduler without a recovery action, so it is not covered here.

## Acceptance

1. With committed generation 1, Start fresh launches with an empty native
   session ID on generation 2 and stream `<incarnation>:g2`. The g1 preflight
   path is never reached.
2. Predecessor submissions are retired without dispatch. Generation 2 commits
   with predecessor 1, the new native ID, and reason `native_replaced`.
   Unmarked, non-adjacent, or foreign successors leave generation 1 current and
   their events unadmitted.
3. Start fresh without a committed generation still uses generation 1.
   Process replacement and context continuation behavior are unchanged.

## Regression tests

- `orchestrator`: `TestRecoverSessionFreshStartLaunchesReplacementGeneration`
  failed on main because the launch carried generation 1, then passed.
  `TestFreshStartSessionCreatedCommitsReplacementGeneration` and
  `TestSessionCreatedWithoutReplacementKeepsCommittedGeneration` cover the commit.
  `TestFreshStartSessionCreatedFromRotatedExecutionKeepsGeneration` failed while
  a rejected resume token still committed the replacement, then passed;
  `TestFreshStartSessionCreatedCommitsReplacementForOwningExecution` covers the
  accepted path.
- `orchestrator/executor`: `TestFreshStartResumeRequestReplacesNativeConversation`
  and the `TestApplyDeliveryIdentity*` fresh-start, first-generation, and
  process-replacement cases.
- `agent/runtime/lifecycle`: `TestFreshStartReplacement*`,
  `TestReplacementExecutionsRetirePredecessorSubmissions`,
  `TestToAgentExecutionRecordsNativeConversationReplacement`, and
  `TestACPSessionCreatedPayloadCarriesNativeConversationReplacement`.
- `orchestrator/watcher` and `backendapp`: the mark survives event decoding and
  the lifecycle launch request.

## Verification

```bash
cd apps/backend
go test -trimpath -race -tags fts5 ./internal/orchestrator/executor ./internal/orchestrator/watcher ./internal/backendapp -count=1
go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'Test(RecoverSession|FreshStart|ContextContinuation|Continuation|SessionDelivery|NativeResume|SessionCreated|ResumeCallbacks|WorkspaceRecoveryBlock|RestartRecovery)' -count=1
go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(ResumedDelivery|DeliveryPreflight|RetireUnresolved|ContextContinuation|FreshStart|Replacement|SilentRestore|DurableRetention|StreamProcessor|DeliveryRetention|ToAgentExecution|ACPSessionCreated|Launch)' -count=1
```

From the repository root, run
`bash scripts/lint-go-changed $(git diff --name-only origin/main...HEAD | grep '\.go$')`;
CI owns full-backend lint. Also run `make fmt`,
`python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`,
and `git diff --check`.

## Dependencies and parallelism

None. Sequential.

## Results

RED: the new tests failed for the expected reasons. The launch carried
generation 1 and stream `:g1`, replacement events were rejected as an identity
mismatch, and the session-created event left generation 1 current.

GREEN: all listed race suites pass, and changed-file Go lint reports no issues.
The full `internal/orchestrator/...` and `internal/agent/runtime/lifecycle/...`
packages also pass without the race detector. No live session, journal, or
database was mutated.

PR review follow-up: the replacement commit now requires accepted resume-token
storage, so a session-created event from a rotated execution leaves the current
generation unchanged. The full `internal/orchestrator/...` packages pass.

## Delivery review validation

The original local RED/GREEN claims above are author-reported history, not
hosted proof. Permanent delivery regressions now cover the real token-CAS to
executor-rotation schedule, foreign task/incarnation/execution identities,
adjacent predecessor CAS, first-generation and stale native-event controls,
and predecessor block retention. The registered agentctl HTTP routes and real
journal prove retirement without provider redispatch, with new work as the
positive control. SQL-backed event admission covers a committed successor,
cancellation, a rejected commit, and a delayed predecessor event.

Hosted focused race tests, actual assertion RED/GREEN, SQLite/PostgreSQL store
coverage, and complete Backend/Frontend/E2E families gate delivery. Exact
source and hosted receipts live in the delivery session's external evidence
directory. No contributor code, build, test, dependency install or cache has
been executed on the review host.
