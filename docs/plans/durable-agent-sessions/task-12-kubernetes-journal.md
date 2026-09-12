---
id: "12-kubernetes-journal"
title: "Retain the Kubernetes delivery journal"
status: complete
wave: 12
depends_on: ["11-ssh-journal"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 12: Retain the Kubernetes delivery journal

## Summary

Wire the Kubernetes executor to the retained journal contract.

## In scope

- Retain the owner-scoped journal across pod replacement within the same environment.
- Keep journal storage separate from the native harness home and deployed binary path.
- Report journal_lost after explicit storage removal. Preserve current environment-generation guards.
- Add disposable Kubernetes integration fixtures for retained storage and storage loss.

## Out of scope

Other executor providers, new storage infrastructure, and network replay.

## Acceptance

- Kubernetes replacement preserves stream identity and accepted submissions when the environment retains storage.
- Ephemeral storage prevents durable capability advertisement. Explicit storage loss leaves active work uncertain.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/agent/runtime/lifecycle -run 'TestKubernetesJournal' -count=1)
(cd apps/web && rtk env KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/kubernetes/durable-journal.spec.ts)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_journal_test.go`:

- `TestKubernetesJournalSurvivesReplacement`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1`.
- `TestKubernetesJournalLossIsExplicit`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2`.

The containers command requires Docker and the applicable repository fixtures.
A missing retained-storage fixture remains a reported blocker, not a passing result.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_bootstrap.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_journal_test.go`
- `apps/web/e2e/tests/kubernetes/durable-journal.spec.ts (new)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_journal_test.go` (new tests or extensions).

## Dependencies

[Task 11](task-11-ssh-journal.md).

## Risks

- The Kind fixture must run with retained storage. A skipped integration test does not prove durability.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented Kubernetes retained journal volume/path wiring and explicit storage-loss behavior through the shared executor contract. Local lifecycle/configuration tests pass; live Kind evidence remains environment-dependent.
