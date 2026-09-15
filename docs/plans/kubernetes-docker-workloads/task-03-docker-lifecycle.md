---
id: "03-docker-lifecycle"
title: "Real Kubernetes Docker lifecycle tests"
status: completed
wave: 3
depends_on:
  - "02-full-worker-image"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-DOCKER-001
  - REQ-EXECUTORS-K8S-DOCKER-002
  - REQ-EXECUTORS-K8S-DOCKER-003
acceptance_criteria:
  - AC-EXECUTORS-K8S-DOCKER-001.3
  - AC-EXECUTORS-K8S-DOCKER-001.4
  - AC-EXECUTORS-K8S-DOCKER-002.2
  - AC-EXECUTORS-K8S-DOCKER-002.3
  - AC-EXECUTORS-K8S-DOCKER-002.4
  - AC-EXECUTORS-K8S-DOCKER-003.1
  - AC-EXECUTORS-K8S-DOCKER-003.2
  - AC-EXECUTORS-K8S-DOCKER-003.3
  - AC-EXECUTORS-K8S-DOCKER-003.4
system_design:
  - ../../specs/executors/system-design/kubernetes-docker-workloads.md
---

# Task 03: Real Kubernetes Docker lifecycle tests

## Summary

Exercise the new template and full worker through the existing Kind-backed
Kandev fixture. Verify the actual agent workspace, Docker filesystem behavior,
visible results and resource lifecycle before staging a production rollout.

## In scope

- New `kubernetes-docker-workloads.spec.ts` in the `containers` project; reuse
  existing fixture ownership, pinned Kubernetes tools and mock-agent transport.
- Load exact task 02 image inputs into the disposable Kind node. Use only a
  bounded test host selected in task 02, never the production cluster/context.
- Real API-defaulted mount grants, daemon readiness failure, actual source and
  Docker smoke, Compose bind writes visible in the Kandev terminal/diff.
- Ordinary Stop retains Pod/PVC/daemon; Resume retains result and uncommitted
  edit. Lost-Pod replacement uses the original snapshot and PVC after a profile
  edit, while Docker caches/test containers are disposable.
- Two test Pods have independent daemons; API inventory counts one Pod each.
  Record nested process cgroup ancestry/counters and bounded CPU/memory load.
- Terminal cleanup removes exact owned resources and nested compute; an
  operator-owned existing claim is untouched. Persist sanitized evidence.
- Update E2E documentation and any duration/shard catalog required by the
  existing runner, without widening worker concurrency.

## Out of scope

Production credentials, new production control plane, deployment,
full-suite execution, UI layout changes or full Docker plugin compatibility.

## Acceptance

1. Source, Chromium, Docker build/run and Compose tests execute, with actual
   workspace read/write assertions and Kandev-visible results.
2. Daemon failure is finite and visible; Stop/Resume/replacement and destructive
   cleanup obey the selected persistence model with no foreign resource deletion.
3. Recorded evidence proves daemon separation and nested resource accounting
   for the tested runtime; limitations remain explicit for untested runtimes.

## Verification

Run from the repository root. These are new tests; first add failing cases,
then make the selected flow pass. Do not run against an active production instance.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/kubernetes/kubernetes-docker-workloads.spec.ts)
(cd apps/backend && GOMAXPROCS=2 go test -p 1 ./internal/agent/kubernetes ./internal/agent/runtime/lifecycle -count=1)
git diff --check
```

Name the browser cases `full worker returns source and Docker results`,
`unavailable daemon fails preparation`, `resume and replacement retain workspace`,
and `isolated daemons clean up with owned Pods`. Keep each independently
reclaimable on assertion failure. The first case runs the new
`k8s/worker-images/full/smoke.sh --all` inside the agent container.
Attach exact fixture/runtime/image versions and command results to Results.

## Files likely touched

- `apps/web/e2e/tests/kubernetes/kubernetes-docker-workloads.spec.ts` (new)
- `apps/web/e2e/fixtures/kubernetes-worker-images.ts`
- `apps/web/e2e/fixtures/kubernetes-test-base.ts`, only needed shared fixture hooks
- `apps/web/e2e/README.md` and applicable runner catalog
- `k8s/worker-images/full/` for defects proven by these cases

## Dependencies

Tasks 01 and 02.

## Risks

Privileged nested Docker can behave differently under Kind's outer container
and other runtimes. Kind evidence does not establish production compatibility. Agent-only temp/HOME binds
are outside the shared workspace; tests must not conceal incompatible paths.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/executors/system-design/kubernetes-docker-workloads.md), Lifecycle and evidence.
- Existing `kubernetes-executor.spec.ts`, `kubernetes-worker-presets.spec.ts`,
  `kubernetes-test-base.ts` and the E2E skill/README.

## Results

All four opt-in browser cases passed on Linux/amd64 with Kind v0.32.0,
Kubernetes v1.36.1 and Docker 29.1.5. The focused run used one worker and
completed in 3.3 minutes:

- `full worker returns source and Docker results` passed in 44.4 seconds,
  including source builds/tests, Chromium, an image build/run, HTTPS transfer,
  Compose RO/RW binds and Kandev-visible workspace output.
- `unavailable daemon fails preparation` passed in 5.1 seconds with a finite,
  visible preparation failure and no agent start.
- `resume and replacement retain workspace` passed in 14.4 seconds, preserving
  the result through Stop/Resume and a lost-Pod replacement while discarding
  daemon state.
- `isolated daemons clean up with owned Pods` passed in 20.0 seconds, proving
  distinct daemon identities, nested cgroup ancestry/counter movement, one Pod
  per session and exact cleanup without deleting the existing claim fixture.

The exact implementation revision was exercised with image config digest
`sha256:101d44047de5238b7846295a2e175c162c79328b42db9a2693d568a9ac59a0f6`.
Required Kubernetes and lifecycle Go packages, changed-file ESLint and test
discovery also passed. This evidence applies to the recorded Kind/runtime
combination; other container runtimes still require their own cgroup validation.
