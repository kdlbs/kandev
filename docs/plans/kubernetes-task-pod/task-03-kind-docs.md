---
id: "03-kind-docs"
title: "Kind evidence and operator docs"
status: done
wave: 3
depends_on:
  - "02-shared-lifecycle"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
acceptance_criteria:
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.1
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.2
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.3
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.4
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.5
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.6
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.7
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
---

# Task 03: Kind evidence and operator docs

## Summary

Prove the user-visible second-session flow on disposable Kind and update the
operator lifecycle documentation after the implementation is verified.

## In scope

- Two sessions, one exact pod and managed PVC, shared untracked file, and distinct agent instances.
- Stop-one/sibling-live, backend restart/resume, concurrent second launch, and final archive cleanup.
- Foundation/ADR compatibility notes, public Kubernetes guide, backend engineering ownership guidance, and implementation result/status updates.

## Out of scope

Cross-task pooling, automated legacy consolidation, and UI redesign.

## Acceptance

- Kind evidence proves the shared lifecycle and unchanged workspace without operating on the user's cluster.
- Public docs explain task-owned resources, independent sessions, and the non-destructive legacy limitation.
- Each earlier work order records actual command results and the design matches the implemented boundaries.

## Verification

Use TDD for changed logic: run the named regression before and after implementation.
Commands run from repository root. Read scoped guidance and applicable TDD/E2E
skills before implementation.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/kubernetes/kubernetes-task-pod.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- New `apps/web/e2e/tests/kubernetes/kubernetes-task-pod.spec.ts`
- `apps/web/e2e/fixtures/kubernetes-test-base.ts` only if a bounded fixture extension is needed
- `docs/public/k8s.md` (reference/how-to lifecycle wording) and `docs/k8s.md` if duplicated
- `docs/specs/kubernetes-executor/spec.md`
- `docs/specs/executors/README.md` and the task-pod requirement/design pair
- `apps/backend/AGENTS.md`
- This plan and its three work orders

## Dependencies

02-shared-lifecycle.

## Risks

Requires Docker/Kind; use existing guarded host-mode runner. Do not claim skipped E2E as verified or publish draft behavior as already available.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/kubernetes-task-pod.md).
- [System design](../../specs/executors/system-design/kubernetes-task-pod.md).
- [Plan](plan.md) regression matrix.
- Existing `executor_kubernetes_fakes_test.go`, checkpoint/restart tests, and
  task environment repository tests as patterns.

## Results

Passed the disposable Kind scenario: 1 test in 6.9 minutes. Three sessions shared
one Pod UID and managed PVC; independent auth files and shared untracked data were
verified. Force-stopping the creator preserved a sibling, a new response arrived
after backend restart, status rows were healthy, and archive removed Pod/PVC.

The passing run exposed a transient cleanup retry when a resumed execution used a
new local ID. A follow-up lifecycle regression reproduced the stale connection,
and the exact-session cleanup fix passes the final Kubernetes race suite and
lint. The Kind scenario was not repeated after this narrow fix.

Eleven desktop/mobile component tests, frontend lint, 62 public-doc validator
tests, public-doc validation (47 pages), specification catalog/linter, and
`git diff --check` passed. Temporary Kind setup timeout edits were restored.
Operator docs, foundation compatibility notes, ADR, backend guidance, requirements,
and design now describe the implemented task-owned lifecycle.
