# Full Kubernetes worker with per-Pod Docker

This opt-in Linux/amd64 recipe extends the pinned universal image with
Go-compatible golangci-lint v2, Playwright Chromium and Docker CLI/Buildx/Compose.
The existing minimal, Node/pnpm and Python presets are unchanged.

## Build and verify

```bash
bash k8s/worker-images/full/build.sh --check
bash k8s/worker-images/full/build.sh --build --verify
```

The script requires Docker Buildx, at least 20 GiB free disk and capacity for a
2-CPU, 4-GiB disposable builder. It validates actual Docker resource controls,
uses one BuildKit step at a time and removes its exact builder on exit. It
prints the local image tag, image ID, platform and limits. It never publishes
an image. The output image remains available for testing; remove that exact
tag when finished. Source/browser verification is bounded to ten minutes and
runs as UID/GID 1000 with dropped capabilities and no host mounts.

`pins.env` records immutable base, daemon and builder manifests, plus checksums
for every added download. Rust, Go, Node, pnpm, Python/venv and native compilers
are inherited from the immutable universal image. The smoke prints the actual
versions. The browser revision matches Playwright 1.61.1 in the repository
lockfile. Only Chromium headless shell is baked, outside the managed mounts.

## Configure an executor profile

1. Distribute your verified image through your own registry and replace
   `FULL_WORKER_IMAGE_REQUIRED` using `bash k8s/worker-images/full/render-template.sh
   registry/image@sha256:<digest>`. The renderer rejects placeholders and mutable
   tags. The unrendered example is documentation only and must not be deployed.
2. Select `linux/amd64`, main container `kandev-agent`, and your workspace mode
   in an existing Kubernetes executor profile. Managed PVCs preserve results.
3. Paste the template into the profile's raw Pod template and `prepare.sh` into
   its prepare script. Preserve the placeholders: Kandev resolves repository,
   identity, authentication, setup and selected-agent installation at launch.
4. Review namespace privilege policy, image access, resources and storage before
   starting a disposable task. No cluster policy is installed by this recipe.

The main agent stays non-root. The explicitly privileged root daemon listens
only on a Pod-local Unix socket, accessible through GID 1000. Both containers
share `/workspace`; the daemon receives neither Kandev runtime nor auth mounts.
Its Docker data is a separate disposable 12-GiB `emptyDir`. Pod-wide non-root
policy is deliberately absent because it would conflict with this daemon.
No host namespaces, hostPath or service-account token are requested.

The agent can control the privileged daemon through that socket. The main
container's non-root security context does not contain commands that use Docker.
Schedule these Pods only on a worker node pool isolated from trusted workloads.

The daemon reads the Pod interface MTU and applies it to default and
user-defined bridges. It selects Docker's cgroupfs driver with a relative
`docker` parent in the default template. This requires a compatible namespace;
the isolated renderer resolves and verifies the companion's actual bounded group
before placing nested containers under its `docker` descendant. Verify both properties on the actual
CNI and container runtime before rollout.

The preparation script waits at most 60 seconds (plus a bounded client call)
for Docker before the standard clone/origin verification, repository setup and
agent installation. Kandev uploads and runs that script through its existing
restricted Pod exec channel, then releases the managed entrypoint only after
preparation succeeds. A failed script returns a bounded, sanitized diagnostic
and never starts agentctl. Kandev still adds task-branch preparation. Cache
directories are created only after repository materialization. Kandev owns
`HOME`; baked tools and browsers remain outside `/workspace` and `/run/kandev`.

## Exercise source and Docker execution

Inside the agent container:

```bash
bash /opt/full-worker/smoke.sh --all
cat /workspace/full-worker-result.txt
```

The smoke executes Go tests/lint/build, Rust tests, Node/pnpm tests, Python/venv
tests, a C compilation and a real Chromium page interaction/screenshot. Docker
acceptance additionally builds/runs a scratch image, performs a large HTTPS
transfer and runs Compose with RO input and RW output workspace binds. It
checks files and removes only its own Compose project and image.

Ordinary Stop retains the Pod, daemon and nested containers. Clean up test
Compose projects explicitly. Resume retains workspace results; lost-Pod
replacement uses the recorded template/PVC and loses disposable daemon state.
Terminal cleanup deletes only resources proven to belong to the session;
existing claims remain operator-owned.

## Compatibility limits

The focused lifecycle suite passed on Linux/amd64 with Kind v0.32.0,
Kubernetes v1.36.1 and Docker 29.1.5. It exercised source/browser work, Docker
build/run, Compose binds, finite readiness failure, Stop/Resume, lost-Pod
replacement, independent daemons, nested cgroup accounting and exact cleanup.
The implementation work orders record the commands, timings and accepted image
config digest.

Kind evidence applies only to the recorded architecture and runtime. Measure
nested cgroup ancestry and counters before claiming resource enforcement on a
different runtime; privileged DinD is not an adversarial isolation boundary.

Only `/workspace` is shared with the daemon. Agent-only HOME/temp bind sources,
callbacks, SSH fixtures and arbitrary Docker plugins are not promised to work.
This recipe does not establish full Kandev Docker-executor parity, provision a
control plane or update an existing service. The backend must contain the
explicit workspace-grant implementation before accepting this template.


## Isolate repository validation

Render an explicitly enabled profile after building and distributing the verified
image. Existing rendering remains unchanged:

```bash
bash k8s/worker-images/full/render-template.sh --isolated registry/image@sha256:<digest>
```

The companion resolves its own cgroup rather than assuming the mount root is
bounded, then verifies cgroup-v2 ancestry and memory charging with a bounded
64 MiB probe before preparation enables validation. Startup refuses a mismatched
3 GiB parent budget and reports its failed preflight stage. A missing/mismatched receipt,
unavailable image or incompatible runtime fails preparation; checks do not fall
back into agent memory. This requires the updated worker image containing
`/opt/full-worker/check.py` and the rendered startup command. Do not paste a
partial prepare script into an existing running task.

On an enabled worker, repository Make build/test/lint goals and both `e2e:run`
and `e2e:raw` package entry points dispatch through `scripts/worker-check` before
heavy work. All sibling sessions share one Docker name reservation. An explicit
runner command supports other workspace validation:

```bash
scripts/worker-check --kind lint -- make -C apps/backend lint
scripts/worker-check --kind build -- make -C apps/backend build
(cd apps/web && ../../scripts/worker-check --kind browser -- pnpm e2e:run --host --shards 1)
```

Each child has a 2 GiB memory/no-extra-swap limit, 2 CPUs, 512 PIDs and a 30-minute
execution deadline. Admission waits at most 60 seconds. The 3 GiB companion budget
reserves 1 GiB for the daemon and conservatively deducts other running containers'
configured memory limits; unbounded workloads block admission. A child OOM fails
its check while the agent container remains separate. The temporary HOME and
`/tmp` are disposable; workspace artifacts/cache survive. Go uses two runtime
threads and a soft heap target below the hard limit; backend lint concurrency is
two and browser validation is one worker/shard.

Policy environment keys are `FULL_WORKER_CHECK_IMAGE`, `_MEMORY_BYTES`,
`_BUDGET_BYTES`, `_RESERVE_BYTES`, `_CPUS`, `_PIDS`, `_QUEUE_SECONDS`, and
`_JOB_SECONDS` (each suffix follows `FULL_WORKER_CHECK`). Budgets must match the
measured companion hard limit and its rendered `validation_budget`. Change the
companion limit, rendered budget and main-container policy together, then verify
on a disposable task before rollout. Images require immutable digests; exact
local image IDs are accepted only when deliberately loaded for disposable tests.

Children receive workspace access and the explicit test allowlist in `check.py`:
locale/timezone, CI, mock/strict test selectors, instrumentation, port offset and
PR artifact capture. They receive neither the daemon socket nor Kandev auth,
control files, native-agent HOME, provider tokens or inherited managed Git leases.
Daemon-dependent Docker/Kind/SSH suites and agent-only callback fixtures are
unsupported inside this runner. Make options/explicit variables are passed as
arguments, with host jobserver descriptors removed. The internal
`FULL_WORKER_CHECK_INSIDE` marker prevents recursion among trusted commands.

Direct `go`, `golangci-lint`, `pnpm build`, custom provider shell commands and
raw Docker users bypass entry-point routing. Use guarded Make/package commands
or the explicit runner. This is cooperative resource protection within the task
trust boundary, not adversarial isolation. Node-wide memory pressure can still
restart Pods. Stale exited or expired never-started slots are reconciled by exact
ID and labels; running slots cannot be stolen. Publishing images and enabling
production profiles require separate operational rollout after live acceptance.
