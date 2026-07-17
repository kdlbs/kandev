---
title: "Kubernetes"
description: "Deploy a single Kandev control plane to Kubernetes with explicit persistence, security, and lifecycle constraints."
---

# Kubernetes

Kandev ships example Kubernetes YAML in `k8s/`. It is a single-replica, persistent deployment example—not a Helm chart, operator, or supported high-availability topology. The release workflow publishes container images but does not apply these manifests to a cluster. Review and adapt every manifest before production use.

## Architecture and limitations

The example creates:

| File | Resource | Current value |
|---|---|---|
| `k8s/configmap.yaml` | `ConfigMap/kandev-config` | `/data` home, info logs, Docker executor disabled |
| `k8s/pvc.yaml` | `PersistentVolumeClaim/kandev-data` | 10 GiB, `ReadWriteOnce`, default StorageClass |
| `k8s/deployment.yaml` | `Deployment/kandev` | one replica, `Recreate`, example resources and probes |
| `k8s/service.yaml` | `Service/kandev` | `ClusterIP`, TCP 38429 |
| `k8s/ingress.yaml` | `Ingress/kandev` | ingress-nginx-oriented example for `kandev.example.com` |

One pod serves the SPA, API, WebSocket, external MCP endpoint, and `/health` on port 38429. With SQLite, the same PVC holds the database, workspaces, CLI installs, and authentication files.

Keep `replicas: 1`. PostgreSQL and NATS are useful external dependencies, but they do not by themselves make Kandev horizontally scalable: task workspaces, local agent processes, control connections, and other runtime state remain pod/filesystem-local. A tested shared-filesystem and runtime-ownership design would also be required. No multi-replica product deployment is currently documented or validated.

The supplied `Recreate` strategy intentionally stops the old pod before starting the new one. Upgrades therefore have downtime.

## Prerequisites

- a Linux `amd64` or `arm64` cluster with `kubectl` configured;
- registry egress to `ghcr.io`, or a mirrored image;
- a default StorageClass that can provision `ReadWriteOnce`, or an explicit `storageClassName`;
- enough PVC capacity for database, repositories, worktrees, caches, and agent CLIs;
- optional ingress controller, DNS, TLS, and an external authentication gateway;
- outbound access required by selected repositories, package registries, agents, integrations, SSH hosts, or Sprites.

The published image is documented in [Docker](docker.md#published-images). Pin a version or digest; do not use a moving tag for a controlled rollout.

## Deploy the example safely

The checked-in Deployment says `image: kandev:latest`. That is a placeholder, not the published GHCR reference; because the tag is `latest`, Kubernetes also defaults its pull policy to `Always`. Replace it. If you deliberately test a node-preloaded local image, set an appropriate `IfNotPresent` or `Never` pull policy in your own manifest. Apply a pinned published image without editing the source file:

```bash
export KANDEV_IMAGE='ghcr.io/kdlbs/kandev:X.Y.Z'

kubectl apply \
  -f k8s/configmap.yaml \
  -f k8s/pvc.yaml \
  -f k8s/service.yaml

kubectl set image \
  -f k8s/deployment.yaml \
  kandev="$KANDEV_IMAGE" \
  --local -o yaml | kubectl apply -f -

kubectl rollout status deployment/kandev
kubectl get pod -l app=kandev
```

Replace `X.Y.Z` with a real release. Add `-n <namespace>` consistently if deploying outside `default`; the supplied resources do not declare a namespace.

Do not apply `k8s/ingress.yaml` yet. It contains a placeholder host, no TLS section, no Kandev authentication, and controller-specific annotations.

For private initial access:

```bash
kubectl port-forward service/kandev 38429:38429
```

Open `http://localhost:38429`.

## Persistence and filesystem permissions

With `KANDEV_HOME_DIR=/data`, the PVC includes:

- `/data/data/kandev.db`, WAL/SHM files, and SQLite snapshots;
- `/data/tasks`, `/data/worktrees`, `/data/repos`, `/data/sessions`, and `/data/lsp-servers`;
- `/data/agent-sessions` for selectively seeded Docker-agent state;
- `/data/.npm-global` for runtime-installed npm agent CLIs;
- `/data/home` for CLI auth, Azure config, caches, and user configuration.

The base image starts as root, recursively fixes `/data` ownership, then drops to the `kandev` user at UID 1000. This may violate a restricted Pod Security policy, fail on root-squashed storage, or make a large-volume restart slow. The universal image is configured to run directly as `kandev` and therefore does not perform that ownership repair.

For a non-root pod, provision the volume for UID 1000 and test your CSI driver's `fsGroup` behavior. A common starting point is:

```yaml
spec:
  template:
    spec:
      securityContext:
        fsGroup: 1000
        fsGroupChangePolicy: OnRootMismatch
      containers:
        - name: kandev
          securityContext:
            runAsNonRoot: true
            runAsUser: 1000
```

This is storage-policy guidance, not a universally portable manifest. Some CSI drivers ignore or implement `fsGroup` differently. Verify a write to `/data/data` and `/data/home` before relying on it.

PVC retention depends on the StorageClass reclaim policy. Deleting `PersistentVolumeClaim/kandev-data` can permanently remove database, repositories, and credentials; back up and verify the target before doing so.

## Configuration and secrets

Non-sensitive values may stay in `kandev-config`. Put database passwords and deployment credentials in a Kubernetes `Secret`, then reference keys from the container:

```yaml
env:
  - name: KANDEV_DATABASE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: kandev-database
        key: password
```

Create the example secret without committing its value:

```bash
kubectl create secret generic kandev-database \
  --from-literal=password='<replace-me>'
```

Shell history and the Kubernetes API still see this literal. Prefer your cluster's normal encrypted secret-delivery workflow. Kandev secrets created in the UI live in its database, so database backups are sensitive too.

See [Configuration](configuration.md) for exact YAML and `KANDEV_` names. Important image/example values:

| Setting | Example value | Meaning |
|---|---|---|
| `KANDEV_HOME_DIR` | `/data` | Persistent Kandev root |
| `KANDEV_DOCKER_ENABLED` | `false` | No Docker daemon in the supplied pod |
| `KANDEV_LOG_LEVEL` | `info` | Backend log threshold |
| `KANDEV_DATABASE_DRIVER` | `sqlite` by default | Set `postgres` for an external database |

Kubernetes detection makes the default log format JSON. Logs remain on stdout unless `logging.outputPath` is changed.

### PostgreSQL

For PostgreSQL, configure at least:

```yaml
env:
  - name: KANDEV_DATABASE_DRIVER
    value: postgres
  - name: KANDEV_DATABASE_HOST
    value: postgres.example.internal
  - name: KANDEV_DATABASE_PORT
    value: "5432"
  - name: KANDEV_DATABASE_USER
    value: kandev
  - name: KANDEV_DATABASE_DBNAME
    value: kandev
  - name: KANDEV_DATABASE_SSLMODE
    value: verify-full
  - name: KANDEV_DATABASE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: kandev-database
        key: password
```

Use the SSL mode and trust material required by your database. PostgreSQL moves only database data; keep the `/data` PVC. Kandev's built-in backup/restore is SQLite-only, so schedule `pg_dump` and test restoration independently.

## Agent execution in a pod

Local and Worktree profiles run agents inside the Kandev pod. Install agent CLIs from **Settings > Agents**, or derive an image that contains them. Runtime npm installs persist under `/data/.npm-global`. Choose the universal image when tasks need its additional build toolchains, but account for the larger image and non-root volume requirement.

The checked-in ConfigMap disables Local Docker. Do not add only a Docker socket mount: the current runtime also needs helper, credential-session, and local-clone bind sources to exist at identical paths on the Docker daemon host, and it currently selects a Linux/amd64 helper. See [containerized control plane limitation](docker.md#containerized-control-plane-limitation). A privileged Docker-in-Docker sidecar has a separate security and persistence model and no supplied Kandev manifest.

SSH and Sprites profiles can run from Kubernetes if the pod can reach their endpoints and has the required secrets/helper bundle. SSH currently does not materialize attached repositories; review [SSH limitations](executors.md#current-repository-limitation). Remote Docker is unimplemented.

Interactive commands should run as the service user. With the base image, a Kubernetes exec starts as root, so use:

```bash
kubectl exec -it deployment/kandev -- gosu kandev gh auth login
```

The universal image already runs as `kandev`; use `kubectl exec -it deployment/kandev -- gh auth login`. Prefer Kandev secret/profile flows over ad hoc pod login where possible.

## Resources and probes

The example requests 250 millicores and 512 MiB, with limits of 2 CPU and 2 GiB. Those are placeholders, not capacity recommendations. Local/Worktree agents share the pod limit with the control plane and can exceed it during builds. Measure workload memory, CPU, ephemeral storage, PVC growth, and process counts; then set requests/limits accordingly.

Both example probes call `/health`. That endpoint returns 503 during startup and 200 once routes are wired and the TCP listener accepts connections. It is a readiness signal, not a deep check of database, repository, Docker, provider, or agent health. The supplied liveness probe therefore tests the same shallow condition.

Long migrations or slow storage may need a startup probe to prevent premature liveness restarts:

```yaml
startupProbe:
  httpGet:
    path: /health
    port: backend
  periodSeconds: 5
  failureThreshold: 60
```

Tune from observed startup time. Keep readiness on `/health`; use separate external monitoring for dependencies and real workflows.

## Ingress and exposure

Kandev has no built-in user-auth boundary. Do not expose the example Ingress publicly until an authenticated gateway and TLS are in place.

Before applying `k8s/ingress.yaml`:

1. replace `kandev.example.com`;
2. configure the real `ingressClassName` or class annotation;
3. add TLS/certificate configuration;
4. add an identity-aware authentication layer;
5. preserve WebSocket upgrades and long idle timeouts;
6. ensure clients cannot bypass the gateway through the Service or node network.

The example's `nginx.ingress.kubernetes.io/configuration-snippet` is ingress-nginx-specific and is disabled by policy in many clusters. Adapt it to your controller; modern controllers may handle WebSocket upgrades without a custom snippet. Proxy the application at `/` on a dedicated host. A subpath deployment is not a documented base-path configuration.

Apply only after review:

```bash
kubectl apply -f k8s/ingress.yaml
kubectl describe ingress kandev
```

## Backup, upgrade, and rollback

Before an upgrade, create and verify a database backup and preserve any irreplaceable task branches. See [Operations](operations.md).

```bash
kubectl set image deployment/kandev \
  kandev=ghcr.io/kdlbs/kandev:X.Y.Z
kubectl rollout status deployment/kandev
kubectl logs deployment/kandev --tail=200
```

The `Recreate` strategy stops active local agents. SQLite migrations run on startup and create a pre-migration snapshot when required. PostgreSQL migrations do not invoke `pg_dump`.

`kubectl rollout undo` changes the image, not the database schema. A binary downgrade may not understand a newer schema; restore the matching pre-upgrade database backup when required. Reapplying the checked-in `k8s/deployment.yaml` without the image transformation resets the image to `kandev:latest`, so keep your production customization in your own overlay or deployment repository.

## Remove while retaining data

Delete compute and routing resources separately from the PVC:

```bash
kubectl delete ingress kandev --ignore-not-found
kubectl delete deployment kandev
kubectl delete service kandev
kubectl delete configmap kandev-config
kubectl get pvc kandev-data
```

Do not delete the PVC until its database, workspaces, auth state, and backups have been exported or intentionally discarded.

## Troubleshooting

```bash
kubectl get pod -l app=kandev -o wide
kubectl describe pod -l app=kandev
kubectl logs deployment/kandev --tail=200
kubectl get pvc kandev-data
kubectl describe pvc kandev-data
kubectl get events --sort-by=.lastTimestamp
```

- **`ImagePullBackOff`:** the example's placeholder `kandev:latest` was not replaced, the tag is wrong, registry egress is blocked, or image-pull credentials are missing.
- **`CrashLoopBackOff` with permission errors:** check PVC ownership, Pod Security admission, root-squash, UID 1000, and universal/base image behavior.
- **Liveness kills startup:** inspect migration/storage timing and add/tune a startup probe.
- **UI works through port-forward but not ingress:** check host/DNS, TLS, auth-gateway route, WebSocket support, and controller-rejected annotations.
- **Agent CLI missing:** install it through Settings or bake it into a derived image; confirm `/data/.npm-global/bin` is on `PATH`.
- **SQLite locked or pod pending after scaling:** return to one replica and `Recreate`; do not share one SQLite database between pods.
- **PVC full:** inspect worktrees, repositories, caches, CLI installs, logs, and retained task state before expanding or deleting anything.

Related pages: [Docker](docker.md), [Configuration](configuration.md), [Executors](executors.md), and [Operations](operations.md).
