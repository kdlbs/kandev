---
id: "03-desktop-mobile-e2e"
title: "Desktop and mobile runtime E2E"
status: complete
wave: 3
depends_on: ["02-frontend-runtime-disclosure"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 03: Desktop and mobile runtime E2E

- **Acceptance:** the existing real kubeconfig Kind case validates the exact
  Pod details, status, cube glyph, Refresh, and settings navigation inside the
  desktop task disclosure without increasing the 12-case lifecycle catalog.
- **Acceptance:** one isolated `mobile-chrome` case taps a named Kubernetes
  executor control and verifies Drawer parity, 44 px target size, and no
  document horizontal overflow without requiring a live cluster.
- **Acceptance:** the E2E-owned Kind cluster/image/process teardown remains
  exact; the user-requested seed lab and the main Kandev instance on port 9998
  are not stopped or modified.
- **Verification:**
  `cd apps && pnpm install --frozen-lockfile && cd web && E2E_FAIL_ON_FLAKY=1 pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-kubernetes-task-environment.spec.ts -- --retries=0 && E2E_FAIL_ON_FLAKY=1 KANDEV_E2E_KIND_BIN=/tmp/kandev-task06-pinned-tools/kind KANDEV_E2E_KUBECTL_BIN=/tmp/kandev-task06-pinned-tools/kubectl KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/kubernetes -- --grep 'launches through kubeconfig with Pod exec and a loopback-only agentctl forward' --retries=0 && cd ../.. && git diff --check`
- **Files likely touched:**
  `apps/web/e2e/tests/kubernetes/kubernetes-executor.spec.ts` and
  `apps/web/e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts`.
- **Dependencies:** Task 02 completed frontend behavior.
- **Parallelism:** sequential.
- **Inputs:** spec scenarios; plan E2E section; existing Kubernetes fixture
  ownership and mobile overflow helpers.
- **Output contract:** report exact discovery/pass counts, timings, artifact
  paths, source-freshness evidence, teardown audit, files changed,
  blockers/risks, and synchronize task/plan status.

## Results

- RED: the first isolated mobile run reached the Drawer but showed the generic
  no-environment state and Reset action because the intentionally unreachable
  cluster failed before creating a persisted task environment. The fixture was
  corrected to seed only its isolated worker database and intercept only the
  exact filtered read-only Kubernetes status request.
- GREEN: the fresh `mobile-chrome` command discovered exactly one test and
  passed 1/1 on the first attempt in 4.3 seconds overall (2.5 second test body),
  including Drawer parity, the 44 px trigger, canonical settings navigation,
  absence of Reset, and no horizontal overflow.
- GREEN: the fresh pinned Kind command extended the existing kubeconfig case
  without changing the 12-case catalog, discovered exactly one focused test,
  and passed 1/1 on the first attempt in 1.2 minutes overall (7.4 second test
  body). It verified the cube glyph, exact live Pod details, Refresh, no generic
  empty-resource text, and canonical settings navigation.
- Final teardown found only the separately requested seed Kind cluster, image,
  and container. The E2E-owned cluster, images, processes, and ownership markers
  were absent; the seed lab and main Kandev instance on port 9998 were not
  modified.
