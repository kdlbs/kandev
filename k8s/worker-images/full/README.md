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

The daemon reads the Pod interface MTU and applies it to default and
user-defined bridges. Verify nested HTTPS transfers for the actual CNI/runtime.
The preparation script waits at most 60 seconds (plus a bounded client call)
for Docker before the standard clone/origin verification, repository setup and
agent installation. Kandev still adds task-branch preparation. Cache directories
are created only after repository materialization. Kandev owns `HOME`; baked
tools and browsers remain outside `/workspace` and `/run/kandev`.

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

Real acceptance results are recorded in the implementation work orders. A
successful image smoke alone does not establish Kubernetes Docker acceptance.
Kind evidence applies only to the recorded architecture and runtime. Measure
nested cgroup ancestry and counters before claiming resource enforcement;
privileged DinD is not an adversarial isolation boundary.

Only `/workspace` is shared with the daemon. Agent-only HOME/temp bind sources,
callbacks, SSH fixtures and arbitrary Docker plugins are not promised to work.
This recipe does not establish full Kandev Docker-executor parity, provision a
control plane or update an existing service. The backend must contain the
explicit workspace-grant implementation before accepting this template.
