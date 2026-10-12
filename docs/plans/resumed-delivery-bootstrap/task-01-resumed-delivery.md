---
id: "01-resumed-delivery"
title: "Restore resumed durable event processing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 01: Restore resumed durable event processing

## Scope

Own lifecycle session initialization, processor/stream teardown, authenticated
replay admission, and focused regression tests. Preserve current-owner fences,
existing cursor validation, and native-conversation identity.

## Acceptance

1. Native resume into generation one buffers setup events until SQL authority commits.
2. A failed processor closes its own stream even if no later event arrives; the
   original failure reaches disconnect handling without affecting a replacement.
3. Already affected sessions recover complete unacknowledged retained history
   through normal receipt/projection before the recovery block can resolve.
4. Missing/foreign ownership and missing retained history remain blocked.

## Verification

```bash
cd apps/backend
go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(DurableRetention|ResumedDelivery|StreamProcessor|DeliveryRetention|DeliveryPreflight|SilentRestore)' -count=1
go test -trimpath -race ./internal/agent/runtime/agentctl -count=1
go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'Test(NativeResume|WorkspaceRecoveryBlock|RestartRecovery|SessionDelivery)' -count=1
golangci-lint run ./... --new-from-rev=a789b16d70358a159103c5cb0168017d63b8dfce --timeout=5m
```

From the repository root run documentation catalog validation, specification lint,
the PR documentation coverage preflight, and `git diff --check`.

## Dependencies and parallelism

None; sequential. Use the existing backend test fixtures and authenticated
delivery interfaces. No UI or protocol changes are planned.

## Results

RED: native-resume initialization rejected a setup event before SQL generation
commit, and processor failure left a quiet stream connected. The checkpoint
regression failed with `agent delivery checkpoint missing`. Additional negative
cases initially demonstrated acceptance of contradictory terminal/type envelopes.
Review regression tests also reproduced acceptance of unbound message/tool
content as the first setup event; bootstrap checkpoint recovery now rejects it.

GREEN: the targeted lifecycle race tests, complete agentctl race suite, targeted
orchestrator recovery tests, full backend lint, specification/catalog validation,
and PR documentation coverage preflight passed. Tests cover real SQL receipt and
projection, retained message output, corrupt/missing evidence, foreign identities,
and execution replacement while the first event is fetched.

The optional full lifecycle race suite reported 15 workspace-path, Kubernetes
preparation, and SSH-environment failures. All 15 reproduced with the exact same
test selection on unchanged base a789b16d7 in a temporary detached worktree;
there were no additional failures in the modified tree. The temporary worktree
was removed. Do not report the full local suite as passing.

Read-only inspection of the reported task found no SQL cursor or inbox events.
Its journal retained all 68 events from sequence one, with zero acknowledgments;
the first event was a nonterminal `agent_capabilities` event with no submission.
Those values meet the tested repair conditions. No running service, recovery
block, prompt, or journal was mutated. Temporary diagnostic code was removed.

Public session/recovery behavior is unchanged; internal design documentation is
updated. No UI markup or copy changed, so screenshots and browser tests are not
required for this backend repair.
