---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-K8S-VALIDATION-001
---

# Kubernetes validation isolation design

## Boundary and current gap

Implements [bounded task validation](../requirements/kubernetes-validation-isolation.md).
The [validation boundary decision](../../../decisions/2026-10-07-kubernetes-validation-workload-boundary.md)
chooses the existing task-owned Docker companion. The current E2E resource guard
limits browser workers/shards, while full lint can still overlap it in the agent
container. Native providers launch their own subprocesses: intercepting agentctl
shell calls alone cannot constrain those commands.

Extend the opt-in `k8s/worker-images/full` recipe and repository check entry
points. Do not add database schema, executor UI, or a second runtime owner.
Criteria .1-.3 cover hard limits/admission; .4 covers lifecycle; .5 covers inputs;
.6 covers compatibility and rollout. This is resource isolation among mutually
trusted processes, within the existing task and privileged-daemon trust boundary.

## Runner interface and policy

Add a Python-standard-library runner `k8s/worker-images/full/check.py`, baked at
`/opt/full-worker/check.py`, and a repository shim `scripts/worker-check`:

```bash
scripts/worker-check --kind lint -- make -C apps/backend lint
scripts/worker-check --kind build -- make -C apps/backend build
scripts/worker-check --kind browser -- pnpm --dir apps/web e2e:run --host --shards 1
```

`--kind` selects a fixed allowlisted tuning policy, not shell parsing. Preserve
argv as an array and invoke the Docker CLI with subprocess argv, never string
interpolation. Canonicalize cwd and reject paths outside `/workspace`; remount
that same workspace path so cache/artifact and package paths remain valid.
Checks run as UID/GID 1000 in the verified full-worker image (immutable registry digest, or exact local image ID in a
disposable fixture) with dropped
capabilities, no privilege escalation, `--init`, bounded tmpfs, no daemon socket,
no auth/runtime volumes, and a disposable HOME. Network is ordinary Docker
bridge networking for package resolution; no host networking.

Use explicit recipe environment `FULL_WORKER_CHECK_MODE=isolated` and
`FULL_WORKER_CHECK_IMAGE=<immutable digest>` supplied by the renderer. Policy
values are finite positive integers: child memory, companion budget and reserve,
CPU count, PID limit, queue deadline and job deadline. Ship conservative example
values: 2 GiB child memory under the existing 3 GiB companion with 1 GiB reserve,
2 CPUs, 512 PIDs, 60-second queue wait and 30-minute job deadline. Operator policy
may increase these together after acceptance; never infer a safe limit from
host RAM or `docker info MemTotal`. Reject child memory above budget minus reserve.
Set Docker memory and memory-swap to the same value to disable extra swap.
Inside lint set GOMEMLIMIT at most 70% of child memory, GOMAXPROCS=2,
GOFLAGS=-p=1 and explicit linter concurrency=2 while retaining the repository's
configured lint timeout. These are tuning aids; the Docker cgroup is the hard
limit. Browser mode requires one worker and one shard and rejects unsafe worker
or shard overrides. TMPDIR stays container-local; reports/logs go to workspace.

Only pass an explicit test-environment allowlist required by repository checks,
including non-secret test selectors, browser paths and locale. Do not copy the
parent environment wholesale. Document the list next to the runner and test it.
Remove managed Git broker and agent-control fields. Checks needing credentials
must use a separately designed test binding; this package adds none.

## Shared admission and workload lifetime

One per-task Docker daemon already supplies a physical sharing boundary. Use a
fixed validation slot container name per daemon. Docker atomic create/name
reservation arbitrates across sibling sessions; an in-process semaphore or
per-E2E worker estimate is insufficient. Give each attempt a random owner ID and
record versioned runner labels, canonical workspace and immutable image identity.
Compare container IDs and full runner labels before start/stop/removal; never
remove a name alone. Name conflicts with foreign or incomplete labels fail closed.

Before create, inspect every other running container in that daemon. Deduct its
configured memory reservation; unbounded or unreadable workloads block admission.
Require sum(existing limits, candidate limit, daemon reserve) <= companion policy
budget. Concurrent supported check requests serialize through the atomic slot;
raw Docker users are outside this cooperative admission contract. The companion's
hard parent cgroup still caps those workloads independently of the agent. Do not
claim node-level OOM protection or fairness for arbitrary raw Docker users.

Keep the named workload until exit status and OOM state are recorded; attach
stdout/stderr without reinterpretation, then remove by exact ID. An exited owned
slot can be reclaimed, but a running or created slot cannot be stolen just
because its caller died. Queue retries use bounded waits. An in-container timeout
supervisor owns the execution deadline independently of the caller; the CLI also
handles TERM/INT and removes only its workload after bounded stop. Docker daemon
restart reconciles exited/dead state by exact labels, or returns a clear failure.
A disconnected CLI must not permit a second check while the old one runs.

Admission is conservative and serial rather than a new scheduler. All CPU and
memory consumed by the check, its disposable E2E backend, and browser descendants
must be charged under the separately limited Docker companion. Validate nested
cgroup ancestry and counters through the existing full-worker accounting fixture
on the actual runtime. Add a companion startup preflight with compatible cgroup-v2,
cgroupfs driver and a bounded accounting probe. Resolve the startup process's
own cgroup from `/proc/self/cgroup`; a privileged CRI container may expose the
host namespace, where the mount root is not the companion. Verify that group's
finite 3 GiB budget before changing it. Move the startup process into its `init`
child and enable CPU/memory/PID controllers within that bounded group. Configure
dockerd with its exact `docker` descendant as parent. Remove any previous receipt,
start dockerd, then run a temporary 64 MiB probe. From the companion,
verify the probe PID's cgroup is a descendant of the companion and its counters
charge that parent; read the companion's finite memory.max and compare it with
policy. Publish an atomic, non-secret receipt under the shared socket directory
only after proof succeeds, keyed to daemon identity and a startup generation.
Preparation and the runner require this receipt plus matching live Docker
identity. Unknown receipts, mismatched budgets or missing proof fail closed.
The receipt's probe path is companion-relative; its separate companion path
records the namespace mapping. Preflight failure logs name the failed stage.
Do not mount the companion's cgroup filesystem into the agent or validator.
Embed the startup helper in the rendered template using the existing shell
command surface; no ConfigMap or extra grant is needed. It bounds daemon
readiness, image availability and probe execution, removes the exact probe
container, and forwards shutdown signals to dockerd. Host/Kind acceptance checks
ancestry independently rather than trusting the receipt alone.

## Repository integration

Route backend Make lint/test/build entry points and root backend delegates through
one repository shim when enabled. Route both `run-e2e.sh` and `run-raw-e2e.sh`
through that shim before build or browser work begins. Once inside a validation
container, an internal marker avoids recursive wrapping; absence of the Docker
socket plus unit tests prevents silent host fallback. Marker spoofing by a
trusted user is outside security enforcement, and must be documented.

The browser runner selects host mode inside its isolated container; unsupported
Docker/Kind/SSH projects are rejected rather than mounting the socket or compiling
on the agent. `scripts/run-quiet` remains an output wrapper, not a universal
resource classifier. Agent guidance requires the guarded Make/package commands
or explicit runner for full lint, Go race compilation, builds and browser suites.
Direct `golangci-lint`, `go`, custom shell commands and full-worker image builds
are not automatically intercepted; documentation must state this limit. Builds
of the worker image retain their existing separately bounded BuildKit workflow.

## Rollout and evidence

Extend the existing immutable image build/renderer/prepare recipe, without editing
retained profiles or replacing the recovered task Pod. Existing workers default
to disabled. A newly rendered isolated profile references the same immutable
full-worker image for agent tools and validator tools. Distribution/publishing and
production profile enablement are separate explicitly authorized operational work.
A later profile edit cannot change a retained workload snapshot.

Disposable Kind acceptance starts two sessions in one task, queues concurrent
lint/browser checks, and asserts one active validation workload and configured
limits. Deliberately exceed only the child validation memory limit: child fails,
agent restart count is unchanged, control health and native conversation survive,
workspace artifact bytes persist. Restart the main container separately to test
failure recovery. Capture resource counters, UIDs, exact container labels and
sanitized receipts; remove only the fixture-owned task/resources.

## Delivery

[Session resilience plan](../../../plans/kubernetes-session-resilience/plan.md).


Existing Docker fixtures use a host-local harness tag. New acceptance must load
the exact verified validator image into the nested task daemon and assert its
image ID; a host-local tag is not automatically available there. Production
rendering continues to require a registry digest. Fixture-only image injection
must not relax production image validation.
