---
id: "08-package-handoff"
title: "Built runtime and contribution handoff"
status: cancelled
wave: 8
depends_on:
  - "07-cleanup-quarantine"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMAD-001
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-003
  - REQ-EXECUTORS-NOMAD-004
  - REQ-EXECUTORS-NOMAD-005
acceptance_criteria:
  - AC-EXECUTORS-NOMAD-001.1
  - AC-EXECUTORS-NOMAD-002.1
  - AC-EXECUTORS-NOMAD-002.3
  - AC-EXECUTORS-NOMAD-003.2
  - AC-EXECUTORS-NOMAD-003.3
  - AC-EXECUTORS-NOMAD-004.3
  - AC-EXECUTORS-NOMAD-005.1
  - AC-EXECUTORS-NOMAD-005.2
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 08: Built runtime and contribution handoff

## Summary

Prove the ordinary built Kandev runtime with a fresh data directory and profile. Record actual results and prepare one cohesive built-in upstream contribution without publishing automatically.

## In scope

- Build the ordinary runtime; document native Nomad version/ACL/resources/SSH account/key prerequisites, backend/API profile setup, supported lifetime and operator recovery. Do not require a plugin package/public HTTPS gateway.
- Run the strict built-binary distribution scenario and complete real matrix, including secret/log/result inspection; no source-only hook may bypass normal registration/configuration. Capture explicit process-launch and service-ready timestamps for Nomad client and agentctl startup durations; task 04 recorded idle RSS but could not establish trustworthy startup timing.
- Update executor public docs and scoped AGENTS only where architecture changed; record actual test/results for all work orders and promote spec/plan lifecycle only if every required criterion passes.
- Run native spec/catalog/diff and PR-documentation checks; prepare problem, behavior, and evidence notes for review. Publishing is outside this work order.

## Out of scope

- Rendered UI, Docker/VM execution, plugin implementation, provider SDK/protobuf changes, model-backed workloads, and production rollout.

## Acceptance

- A normally built Kandev binary, fresh DB and normal API profile complete all real scenarios on documented native Mac topology; zero/skipped/unimplemented scenarios fail.
- Vendor code remains cohesive, shared SSH/agent protocol behavior remains tested, and install/recovery instructions reproduce the exact supported boundary without a plugin or public runtime port.
- Every work order records actual commands/results; supported behavior is not claimed while required real scenarios lack evidence. The contribution is ready for review.

## Verification

All new paths/test names are future deliverables. Run the complete block from
any shell with the named absolute root and fixture-only config set. Use TDD for
changed logic; no-tests-to-run or a skipped required real scenario fails the
work order. Do not mark done based only on existing tests.

```bash
: "${KANDEV_ROOT:?Set absolute Kandev worktree path}"
: "${KANDEV_NOMAD_TEST_CONFIG:?Set absolute fixture-only JSON config path}"
make -C "$KANDEV_ROOT/apps/backend" build
(cd "$KANDEV_ROOT/apps/backend" && go test -race ./internal/agent/nomad/... -count=1)
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e run --require-real --scenario distribution --config "$KANDEV_NOMAD_TEST_CONFIG")
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e run --require-real --scenario all --config "$KANDEV_NOMAD_TEST_CONFIG")
(cd "$KANDEV_ROOT" && python3 scripts/list-docs.py validate)
(cd "$KANDEV_ROOT" && python3 scripts/lint-spec-files.test.py)
(cd "$KANDEV_ROOT" && python3 scripts/lint-spec-files.py --all)
(cd "$KANDEV_ROOT" && node --test .github/scripts/pr-docs.test.cjs)
(cd "$KANDEV_ROOT" && git diff --check)
```

## Files likely touched

- `apps/backend/internal/agent/nomad/integration/distribution_test.go (new)`
- `scripts/nomad-native-e2e; native fixture config/example`
- `apps/backend/internal/agent/runtime/lifecycle/executor_nomad*.go and registration/configuration integration`
- `apps/backend/AGENTS.md only where architecture changes`
- `docs/public/executors.md and relevant installation/configuration docs`
- `docs/specs/executors/{requirements,system-design}/nomad-native-executor.md and this plan/work-order results`

## Dependencies

[Task 07](task-07-cleanup-quarantine.md).

## Risks

Missing actual partition and native tooling remains unverified; the handoff must report any such limitation rather than count it as a successful scenario.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-native-executor.md), using frontmatter IDs.
- [System design](../../specs/executors/system-design/nomad-native-executor.md), matching implementation boundary and current source.
- [Plan](plan.md), including strict real-worker runner/prerequisites.
- [ADR](../../decisions/2026-10-24-native-nomad-cleanup-evidence.md).
- Read applicable scoped `AGENTS.md` before editing code.

## Results

Cancelled 2026-10-25 by the owner. The executor's destination is a worker pool,
so the first handoff must be pool-capable. This scope moved into
[Nomad worker pool task 03](../nomad-worker-pool/task-03-pool-proof-handoff.md).
