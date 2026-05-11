# Kubernetes Deployment Guide

This guide covers building the Kandev Docker image and deploying it to a Kubernetes cluster.

## Prerequisites

- Docker (for building the image)
- A container registry (Docker Hub, GHCR, ECR, etc.)
- A Kubernetes cluster with `kubectl` configured
- A StorageClass that supports `ReadWriteOnce` PVCs (for SQLite persistence)

## Building the Image

From the repository root:

```bash
# Build for your current architecture
docker build -t kandev:latest .

# Build for a specific architecture (e.g., for amd64 clusters)
docker build --platform linux/amd64 -t kandev:latest .

# Multi-arch build (requires buildx)
docker buildx build --platform linux/amd64,linux/arm64 -t kandev:latest .
```

### Using the Pre-built Image

Kandev publishes images to GitHub Container Registry. Pull directly:

```bash
docker pull ghcr.io/kdlbs/kandev:latest
```

Or reference it in your K8s deployment:

```yaml
image: ghcr.io/kdlbs/kandev:latest
```

## Deploying to Kubernetes

### Quick Start

```bash
# Apply all manifests
kubectl apply -f k8s/

# Check status
kubectl get pods -l app=kandev
kubectl logs -l app=kandev -f
```

### What Gets Created

| Resource | File | Purpose |
|----------|------|---------|
| Deployment | `deployment.yaml` | Single-replica pod running backend + web |
| Service | `service.yaml` | ClusterIP exposing port 38429 |
| ConfigMap | `configmap.yaml` | Non-sensitive environment configuration |
| PVC | `pvc.yaml` | 10Gi persistent volume for SQLite + worktrees |
| Ingress | `ingress.yaml` | Example ingress with WebSocket support |

### Accessing the UI

**Port-forward** (quickest for testing):

```bash
kubectl port-forward svc/kandev 38429:38429
# Open http://localhost:38429
```

**Ingress**: Edit `k8s/ingress.yaml` to set your domain, then apply. The ingress routes all traffic to the backend on port 38429 (the Go backend reverse-proxies the Next.js frontend internally).

### Custom Domain / Reverse Proxy

No extra configuration is needed. The frontend automatically uses `window.location.origin` to reach the API, which works with any domain, reverse proxy, or ingress setup.

## Installing Agent CLIs

The kandev image ships with `git`, `gh` (GitHub CLI), `node`, and `npm`, but **does not bundle the coding-agent CLIs** (`claude-code`, `codex`, `auggie`, etc.) — agent choice is per-user, and bundling all of them would bloat the image significantly.

To install an agent inside the running pod, open **Settings → Agents** in the UI and click **Install** on the agent card under "Available to Install". The backend runs the agent's hard-coded install script (`npm install -g <pkg>`) and rescans on success.

The image sets `NPM_CONFIG_PREFIX=/data/.npm-global` so user-installed npm globals land on the PV and **survive pod restarts and image upgrades**. The same persistence applies if you `kubectl exec` and install manually:

```bash
kubectl exec -it deployment/kandev -- npm install -g @anthropic-ai/claude-code
```

After installing, log in with the agent's own auth (e.g. `claude login`), then click **Rescan** on the agents page.

## Configuration

Kandev reads configuration via `KANDEV_`-prefixed environment variables (Viper). Set these in `k8s/configmap.yaml` or as environment variables in the deployment.

### Core Settings

See [`configuration.md`](./configuration.md) for the full reference (every backend knob and its YAML form). The tables below cover what's most commonly set in K8s manifests.

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `KANDEV_SERVER_PORT` | No | `38429` | Server port (API + WebSocket + Web UI) |
| `KANDEV_HOME_DIR` | No | `/data` | Kandev home directory - contains `data/` (DB), `tasks/`, `worktrees/`, `repos/`, `sessions/`, and `lsp-servers/` |
| `KANDEV_DATABASE_DRIVER` | No | `sqlite` | Database driver (`sqlite` or `postgres`) |
| `KANDEV_DATABASE_PATH` | No | `$KANDEV_HOME_DIR/data/kandev.db` | SQLite database file path (override) |
| `KANDEV_DOCKER_ENABLED` | No | `false` | Enable Docker runtime for agents (requires DinD) |
| `KANDEV_LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `KANDEV_LOGGING_FORMAT` | No | auto | Log format: `json` (auto-detected in K8s) or `text` |
| `KANDEV_LOGGING_OUTPUTPATH` | No | `stdout` | Log destination: `stdout`, `stderr`, or a file path (rotated when a file) |
| `KANDEV_LOGGING_MAXSIZEMB` | No | `100` | Rotate the log file when it exceeds this size (MB). File output only. |
| `KANDEV_LOGGING_MAXBACKUPS` | No | `5` | Max rotated files to retain (`0` = unlimited). File output only. |
| `KANDEV_LOGGING_MAXAGEDAYS` | No | `30` | Max age of rotated files in days (`0` = unlimited). File output only. |
| `KANDEV_LOGGING_COMPRESS` | No | `true` | Gzip rotated files. File output only. |

> **Logging in K8s:** prefer the default `stdout` so kubelet collects logs. If you set `KANDEV_LOGGING_OUTPUTPATH` to a file, the active log is created with mode `0600` (owner read/write only); any sidecar reading it must run as the same user.

> **Upgrading from a pre-`KANDEV_HOME_DIR` deployment?** The SQLite DB path moved from `/data/kandev.db` to `/data/data/kandev.db`, and `KANDEV_DATA_DIR` is gone — point `KANDEV_HOME_DIR` at the same volume mount (`/data`) instead. (`KANDEV_WORKTREE_BASEPATH` still works as an explicit override if you want to keep worktrees outside the home dir.) The backend auto-migrates the legacy `kandev.db` (plus any `-wal`/`-shm` files) on first boot — look for `Migrated SQLite database from pre-KANDEV_HOME_DIR location` in the pod logs. If you'd rather pin the old path, set `KANDEV_DATABASE_PATH=/data/kandev.db` in the ConfigMap.

### PostgreSQL Settings (when `KANDEV_DATABASE_DRIVER=postgres`)

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `KANDEV_DATABASE_HOST` | No | `localhost` | PostgreSQL host |
| `KANDEV_DATABASE_PORT` | No | `5432` | PostgreSQL port |
| `KANDEV_DATABASE_USER` | Yes | `kandev` | Database user |
| `KANDEV_DATABASE_PASSWORD` | Usually | (empty) | Database password - required unless your Postgres allows passwordless auth |
| `KANDEV_DATABASE_DBNAME` | Yes | `kandev` | Database name |
| `KANDEV_DATABASE_SSLMODE` | No | `disable` | SSL mode (`disable`, `require`, `verify-ca`, `verify-full`) |

## Database: SQLite vs PostgreSQL

### SQLite (default)

- Zero-config, works out of the box
- Database stored at `/data/data/kandev.db` on the PV (derived from `KANDEV_HOME_DIR=/data`)
- **Single replica only** (SQLite is single-writer)
- Deployment strategy is `Recreate` to prevent concurrent writes
- Good for small teams / personal use

### PostgreSQL (recommended for production)

- Supports multiple replicas for horizontal scaling
- Change deployment strategy to `RollingUpdate`
- Set via environment variables:

```yaml
# In configmap.yaml or a Secret
KANDEV_DATABASE_DRIVER: postgres
KANDEV_DATABASE_HOST: postgres.default.svc.cluster.local
KANDEV_DATABASE_PORT: "5432"
KANDEV_DATABASE_USER: kandev
KANDEV_DATABASE_PASSWORD: <from-secret>
KANDEV_DATABASE_DBNAME: kandev
```

When using Postgres, the PVC is still needed for worktree storage but the database itself is external.

## Persistent Storage

The PVC at `/data` stores:

- **SQLite database** (`/data/data/kandev.db`, `/data/data/kandev.db-wal`, `/data/data/kandev.db-shm`)
- **Git worktrees** (`/data/worktrees/`), **tasks** (`/data/tasks/`), **repos** (`/data/repos/`), **sessions** (`/data/sessions/`), and **LSP servers** (`/data/lsp-servers/`)

The PVC uses `ReadWriteOnce` access mode. If your cluster requires a specific StorageClass, add it to `k8s/pvc.yaml`:

```yaml
spec:
  storageClassName: your-storage-class
```

## Health Checks

The deployment includes both probes on the `/health` endpoint:

- **Liveness probe**: Restarts the pod if the backend becomes unresponsive (30s interval, 3 failures)
- **Readiness probe**: Removes the pod from service during startup or issues (10s interval, 3 failures)

The CLI launcher also performs an internal health check — it waits for the backend to be healthy before starting the web server.

## Scaling

**Single replica (SQLite)**: The default configuration uses `replicas: 1` with `Recreate` strategy. This ensures only one instance writes to SQLite at a time.

**Multiple replicas (PostgreSQL)**: Switch to Postgres, change the deployment strategy to `RollingUpdate`, and increase replicas:

```yaml
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
```

## Upgrading

```bash
# Build and push new image
docker build -t your-registry.com/kandev:v1.1.0 .
docker push your-registry.com/kandev:v1.1.0

# Update deployment
kubectl set image deployment/kandev kandev=your-registry.com/kandev:v1.1.0

# Or edit the deployment directly
kubectl edit deployment kandev
```

SQLite migrations run automatically on startup — no manual migration step needed.

## Troubleshooting

```bash
# Check pod status
kubectl get pods -l app=kandev

# View logs
kubectl logs -l app=kandev -f

# Shell into the pod (if needed)
kubectl exec -it deployment/kandev -- /bin/bash

# Check PVC status
kubectl get pvc kandev-data

# Describe pod for events
kubectl describe pod -l app=kandev
```
