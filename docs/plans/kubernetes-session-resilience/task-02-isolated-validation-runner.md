---
id: "02-isolated-validation-runner"
title: "Bound validation in the task Docker companion"
status: done
wave: 2
depends_on:
  - "01-restart-safe-cleanup"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-VALIDATION-001
acceptance_criteria:
  - AC-EXECUTORS-K8S-VALIDATION-001.1
  - AC-EXECUTORS-K8S-VALIDATION-001.2
  - AC-EXECUTORS-K8S-VALIDATION-001.3
  - AC-EXECUTORS-K8S-VALIDATION-001.4
  - AC-EXECUTORS-K8S-VALIDATION-001.5
system_design:
  - ../../specs/executors/system-design/kubernetes-validation-isolation.md
---

# Isolated validation runner

## Summary

Provide one executable check runner with hard child limits, measured companion
accounting and task-wide admission, independently of repository command hooks.

## Scope and exclusions

Implement argv-safe execution, immutable image/policy checks, the atomic Docker
slot, aggregate reservations, cancellation/deadlines and exact-ID reconciliation.
Bake the runner into the full worker and add the bounded companion preflight and
receipt contract. Update renderer/build/prepare source checks. Exclude command
hooks (03), production profile edits, new companions, publishing and daemon/socket
access from inside validation children.

## Implementation acceptance

1. Tests prove atomic admission across callers, finite aggregate limits, foreign
   slot rejection, failed isolation without fallback, and preserved argv/output.
2. Caller death, TERM/INT, timeouts and daemon restart cannot start overlapping
   checks or remove an unrelated container; the child has an independent deadline.
3. The recipe requires a live, matching accounting receipt and immutable image;
   child configuration has explicit limits and workspace-only/environment grants.

## Likely files

Existing `k8s/worker-images/full/`: `Dockerfile`, `build.sh`, `build_test.py`,
`daemon_test.py`, `pod-template.yaml`, `render-template.sh`, `prepare.sh`,
`accounting.sh`. Future outputs in that directory: `check.py`, `check_test.py`,
`daemon-validation-preflight.sh`. Future repository shim: `scripts/worker-check`.
Keep source/helper injection compatible with the full build's directory context.
Extend fixture/image handling only where needed for this independent runner;
final real acceptance belongs to 04.

## Tests and verification

Use Python unittest with injected Docker subprocess responses and controllable
clock/cancellation. Assert actual argv/limits, label/ID comparison, queued races,
other unbounded workloads, receipt mismatch, child OOM/exit codes, stale/foreign
containers and no sensitive inherited environment. Existing source checks prove
startup helper injection, exact memory policy, signal forwarding and cleanup.
New output tests must exist before running this block from repository root:

```bash
python3 -m unittest discover -s k8s/worker-images/full -p '*_test.py'
bash k8s/worker-images/full/build.sh --check
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

These unit/source commands do not prove real cgroup isolation. Work order 04 owns
image build and actual descendant/parent measurements; do not mark live rollout
ready after mocked command assertions alone.

## Dependencies, risks and parallelism

Depends on 01 for sequential delivery. Shared recipe files make this unsafe to
parallelize with 03/04. Receipt is runtime proof among trusted processes, not a
credential. Sum configured limits conservatively; no unbounded child/other job
can be admitted. Local image ID support must not accept mutable production tags.

## Inputs

[Plan](plan.md), [validation design](../../specs/executors/system-design/kubernetes-validation-isolation.md),
[requirements](../../specs/executors/requirements/kubernetes-validation-isolation.md),
full-worker recipe, accounting fixture and task trust ADR.

## Results

Implemented the standard-library runner, atomic per-daemon slot, conservative
shared memory admission, independent child deadline, exact-ID cleanup and
allowlisted environment. The opt-in renderer embeds a bounded companion probe
and publishes an accounting receipt before preparation enables checks. Default
rendering remains unchanged. Validation images never pull implicitly at admission.

`python3 -m unittest discover -s k8s/worker-images/full -p '*_test.py'`
passed 19 tests. The initial runner/recipe tests failed before implementation;
new timeout and pull-policy tests failed before their fixes. Source recipe
validation, documentation catalog/spec lint and diff whitespace passed.
Mock/source evidence does not establish live cgroup accounting; 04 remains the
real Docker/Kind acceptance gate. No image was published or profile changed.
