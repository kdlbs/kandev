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
| Service | `service.yaml` | ClusterIP exposing ports 8080 (API) and 3000 (UI) |
| ConfigMap | `configmap.yaml` | Non-sensitive environment configuration |
| PVC | `pvc.yaml` | 10Gi persistent volume for SQLite + worktrees |
| Ingress | `ingress.yaml` | Example ingress with WebSocket support |

### Accessing the UI

**Port-forward** (quickest for testing):

```bash
kubectl port-forward svc/kandev 3000:3000 8080:8080
# Open http://localhost:3000
```

**Ingress**: Edit `k8s/ingress.yaml` to set your domain, then apply. The ingress routes `/api` and `/ws` to the backend (port 8080) and everything else to the web UI (port 3000).

## Configuration

Kandev reads configuration via `KANDEV_`-prefixed environment variables (Viper). Set these in `k8s/configmap.yaml` or as environment variables in the deployment.

### Core Settings

| Env Var | Default | Description |
|---------|---------|-------------|
| `KANDEV_SERVER_PORT` | `8080` | Backend API port |
| `KANDEV_DATABASE_DRIVER` | `sqlite` | Database driver (`sqlite` or `postgres`) |
| `KANDEV_DATABASE_PATH` | `/data/kandev.db` | SQLite database file path |
| `KANDEV_DOCKER_ENABLED` | `false` | Enable Docker runtime for agents (requires DinD) |
| `KANDEV_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `KANDEV_LOGGING_FORMAT` | auto | Log format: `json` (auto-detected in K8s) or `text` |
| `KANDEV_WORKTREE_BASEPATH` | `/data/worktrees` | Git worktree storage directory |

### PostgreSQL Settings (when `KANDEV_DATABASE_DRIVER=postgres`)

| Env Var | Default | Description |
|---------|---------|-------------|
| `KANDEV_DATABASE_HOST` | `localhost` | PostgreSQL host |
| `KANDEV_DATABASE_PORT` | `5432` | PostgreSQL port |
| `KANDEV_DATABASE_USER` | `kandev` | Database user |
| `KANDEV_DATABASE_PASSWORD` | (empty) | Database password |
| `KANDEV_DATABASE_DBNAME` | `kandev` | Database name |
| `KANDEV_DATABASE_SSLMODE` | `disable` | SSL mode |

## Database: SQLite vs PostgreSQL

### SQLite (default)

- Zero-config, works out of the box
- Database stored at `/data/kandev.db` on the PV
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

- **SQLite database** (`/data/kandev.db`, `/data/kandev.db-wal`, `/data/kandev.db-shm`)
- **Git worktrees** (`/data/worktrees/`) for workspace isolation

The PVC uses `ReadWriteOnce` access mode. If your cluster requires a specific StorageClass, add it to `k8s/pvc.yaml`:

```yaml
spec:
  storageClassName: your-storage-class
```

## Health Checks

The deployment includes both probes on the backend `/health` endpoint:

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
