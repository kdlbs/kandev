---
id: "06-kind-e2e-ci"
title: "Kubernetes Kind E2E and CI"
status: complete
wave: 3
depends_on: ["02-lifecycle", "03-service-api", "05-frontend-settings"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 06: Kubernetes Kind E2E and CI

- **Acceptance:** worker-owned Kind fixtures prove kubeconfig and in-cluster
  launch, exec/port-forward, restart/reconnect, storage retention/deletion,
  exact foreign-resource protection, and causal failure status.
- **Acceptance:** desktop/mobile settings E2E proves persistence, member/admin
  behavior, touch reachability, containment, and no document horizontal overflow.
- **Acceptance:** the containers project, shard planner, CI workflow, and E2E
  README include pinned Kind/Kubernetes coverage with deterministic cleanup.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-kubernetes-executor.spec.ts && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/kubernetes`
- **Files likely touched:** `apps/web/e2e/fixtures/kubernetes-test-base.ts`,
  helpers, Kubernetes/settings specs, Playwright config, shard scripts/tests,
  E2E README, and `.github/workflows/e2e-tests.yml`.
- **Dependencies:** Tasks 02, 03, and 05.
- **Parallelism:** sequential after product paths stabilize; owns E2E/CI files.
- **Inputs:** all spec scenarios; E2E fixture-state and cleanup references.

## Results

Complete on 2026-08-25. The worker-owned fixture, desktop/mobile settings
coverage, separate compatibility project, duration-aware shard planner, CI
jobs, and E2E README are implemented and verified.

### Pinned matrix

- Kind: `v0.32.0`, Linux-amd64 SHA-256
  `50030de23cf40a18505f20426f6a8506bedf13c6e509244bd1fa9463721b0f54`.
- Full lifecycle/current compatibility: Kubernetes and kubectl `v1.36.1`,
  kubectl Linux-amd64 SHA-256
  `629d3f410e09bf49b64ae7079f7f0bda1191efed311f7d37fdbab0ad5b0ec2b7`,
  node
  `kindest/node:v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5`.
- Oldest supported compatibility: Kubernetes and kubectl `v1.34.8`, kubectl
  Linux-amd64 SHA-256
  `f6249132865c13abe3c9dd5038f5da65849cb86eee1608c001831504e481aa8c`,
  node
  `kindest/node:v1.34.8@sha256:02722c2dedddcfc00febf5d27fbeb9b7b2c14294c82109ff4a85d89ac9ba3256`.
- Runtime base:
  `ubuntu:24.04@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517`.
  Its `apt-get` package resolution is not snapshot-pinned; this is a documented
  nonblocking reproducibility limitation for the fixture.

### Fresh build identity

The final run used checkout `e04428fa363ecc14b4731269630e7c5a7e579d6f`.
The aggregate SHA-256 over tracked and nonignored backend/web/workflow source
files was
`1a3910681dd99b979f0c84055a89cae332487d2788b5e064f90f7dc02ad4b181`
before the build and remained identical after every run. No
`KANDEV_E2E_SKIP_FRESHNESS` or other freshness bypass was set.

Fresh artifact SHA-256 values:

- `apps/backend/bin/kandev`:
  `63fd6daae55bc4609ba50c0cade77b736373c1b9152c4322b14c3b78dc80f08a`.
- `apps/backend/bin/agentctl-linux-amd64`:
  `051986cb48fd07b1d133a83cbc702c6d743771adb41e3c8a179ca9b583600d18`.
- `apps/backend/bin/mock-agent-linux-amd64`:
  `7bc6b792198ab184ab8b94b2d74ea3e53ca46814b6e044ffa3d9e0fae2c94838`.
- `apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz`:
  `fa024611577a123a46c8cee7380f1b4d06ae01ba5305fc8c62c401695ca6a06f`.
- `apps/web/dist/index.html`:
  `db97c3fef73f1e18d780f5281772e8f7f4084d6caa6ba5aa722acd98433cc5d7`.

### Final live verification

The strict full lifecycle command performed a fresh backend, Linux-helper,
web, and plugin-fixture build. It used one worker, explicit zero retries,
fail-on-flaky behavior, strict WebSocket accounting, and no no-build or
freshness shortcut:

```sh
cd apps/web && \
E2E_FAIL_ON_FLAKY=1 \
KANDEV_E2E_KIND_BIN=/tmp/kandev-task06-pinned-tools/kind \
KANDEV_E2E_KUBECTL_BIN=/tmp/kandev-task06-pinned-tools/kubectl \
KANDEV_E2E_CONTAINERS=1 \
pnpm e2e:run --host --project containers tests/kubernetes -- --retries=0
```

Discovery was exactly 12 tests in one file. All 12 passed on their first
attempt in 3.4 minutes:

1. Kubeconfig launch, Pod exec, loopback-only agentctl forwarding: 5.9 seconds.
2. Real in-cluster service-account launch: 8.9 seconds.
3. Same-Pod backend reconnect: 8.1 seconds.
4. Live main-container restart re-handshake: 8.0 seconds.
5. Managed PVC ordinary stop/resume preservation and terminal deletion:
   10.3 seconds.
6. Existing-claim terminal retention: 10.6 seconds.
7. Foreign same-name/different-UID Pod protection: 6.2 seconds.
8. Foreign full `kandev.ai/*` ownership-label protection: 5.6 seconds.
9. Causal RBAC failure in user-visible status: 2.2 seconds.
10. Causal scheduling failure in user-visible status: 32.3 seconds.
11. Causal image-pull failure in user-visible status: 4.0 seconds.
12. Causal PVC failure in user-visible status: 2.1 seconds.

The compatibility project discovered exactly one API-only smoke for each
server version. It reused the preceding fresh artifact identity; `--no-build`
did not disable global freshness validation.

```sh
cd apps/web && \
E2E_FAIL_ON_FLAKY=1 \
KANDEV_E2E_KUBERNETES_VERSION=v1.34.8 \
KANDEV_E2E_KIND_BIN=/tmp/kandev-task06-pinned-tools/kind \
KANDEV_E2E_KUBECTL_BIN=/tmp/kandev-task06-pinned-tools/kubectl-v1.34.8 \
KANDEV_E2E_CONTAINERS=1 \
pnpm e2e:run --host --no-build --project kubernetes-compat \
  tests/kubernetes-compat -- --retries=0
```

Result: 1/1 passed on the first attempt in 1.3 minutes (test body 6.6
seconds), proving real discovery, Pod-template admission, exec/port-forward,
task launch, agentctl round-trip, credential nonleakage, and redacted
diagnostics on Kubernetes `v1.34.8`.

```sh
cd apps/web && \
E2E_FAIL_ON_FLAKY=1 \
KANDEV_E2E_KUBERNETES_VERSION=v1.36.1 \
KANDEV_E2E_KIND_BIN=/tmp/kandev-task06-pinned-tools/kind \
KANDEV_E2E_KUBECTL_BIN=/tmp/kandev-task06-pinned-tools/kubectl-v1.36.1 \
KANDEV_E2E_CONTAINERS=1 \
pnpm e2e:run --host --no-build --project kubernetes-compat \
  tests/kubernetes-compat -- --retries=0
```

Result: 1/1 passed on the first attempt in 1.0 minute (test body 6.1
seconds) on Kubernetes `v1.36.1`.

The new active-session mobile projection was then selected as exactly one test
and run against the same fresh artifacts:

```sh
cd apps/web && \
E2E_FAIL_ON_FLAKY=1 \
pnpm e2e:run --host --no-build --project mobile-chrome \
  tests/settings/mobile-kubernetes-executor.spec.ts -- \
  --grep 'active session cards expose task and session identities without a cluster' \
  --retries=0
```

Result: 1/1 passed on the first attempt in 3.6 seconds (test body 1.7
seconds). Earlier focused settings verification remains green: desktop 2/2 in
21.7 seconds and the Pixel 5 create/test/save/reload plus member-boundary cases
2/2 in 21.9 seconds, including 44 px actions, raw-YAML containment, and no
document horizontal overflow.

Focused fixture/policy/helper/pin/global-setup/planner tests, web typecheck,
focused ESLint/Prettier, and the E2E sleep ratchet also passed during Task 06
implementation. Ordinary container discovery remains exactly the 12 accepted
browser lifecycle cases; the one-case compatibility project is opt-in and
excluded from that catalog.

### Deterministic teardown

After the full run, after each compatibility version, and after the mobile
scenario, the final exact audit reported:

- Kind clusters: none.
- `kandev-kubernetes-e2e:*` runtime images: none.
- `kandev-e2e-*` task/Kind containers: none.
- fixture ownership markers under `/tmp`: none.
- this workspace's managed E2E runner, Playwright, backend, Vite, and kubectl
  port-forward processes: none.

Unrelated Kandev workspaces and their processes were not modified.
