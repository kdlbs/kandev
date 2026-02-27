# Docker Guide

Run Kandev in a Docker container. For Kubernetes deployment, see [k8s.md](k8s.md).

## Quick Start

```bash
docker run -p 8080:8080 -p 3000:3000 -v kandev-data:/data ghcr.io/kdlbs/kandev:latest
```

Open `http://localhost:3000` in your browser.

## Using the Pre-built Image

Kandev publishes images to GitHub Container Registry for `linux/amd64` and `linux/arm64`:

```bash
# Latest release
docker pull ghcr.io/kdlbs/kandev:latest

# Specific version
docker pull ghcr.io/kdlbs/kandev:0.9.0
```

## Building from Source

From the repository root:

```bash
# Build for your current architecture
docker build -t kandev:latest .

# Build for a specific architecture
docker build --platform linux/amd64 -t kandev:latest .
```

## Data Persistence

Kandev stores its SQLite database and git worktrees in `/data`. Mount a volume to persist data across container restarts:

```bash
# Named volume (recommended)
docker run -v kandev-data:/data ghcr.io/kdlbs/kandev:latest

# Bind mount to a host directory
docker run -v /path/on/host:/data ghcr.io/kdlbs/kandev:latest
```

Without a volume, data is lost when the container is removed.

## Configuration

Configuration is done via `KANDEV_`-prefixed environment variables:

```bash
docker run -p 8080:8080 -p 3000:3000 \
  -v kandev-data:/data \
  -e KANDEV_LOG_LEVEL=debug \
  ghcr.io/kdlbs/kandev:latest
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KANDEV_DATABASE_DRIVER` | `sqlite` | Database driver (`sqlite` or `postgres`) |
| `KANDEV_DATABASE_PATH` | `/data/kandev.db` | SQLite database file path |
| `KANDEV_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `KANDEV_LOGGING_FORMAT` | `text` | Log format: `text` or `json` |
| `KANDEV_DOCKER_ENABLED` | `false` | Enable Docker runtime for agents (see below) |
| `KANDEV_WORKTREE_BASEPATH` | `/data/worktrees` | Git worktree storage directory |

### PostgreSQL

To use PostgreSQL instead of SQLite:

```bash
docker run -p 8080:8080 -p 3000:3000 \
  -e KANDEV_DATABASE_DRIVER=postgres \
  -e KANDEV_DATABASE_HOST=host.docker.internal \
  -e KANDEV_DATABASE_PORT=5432 \
  -e KANDEV_DATABASE_USER=kandev \
  -e KANDEV_DATABASE_PASSWORD=secret \
  -e KANDEV_DATABASE_DBNAME=kandev \
  ghcr.io/kdlbs/kandev:latest
```

## Ports

| Port | Service |
|------|---------|
| `8080` | Backend API + WebSocket |
| `3000` | Web UI |

Override ports by changing the published ports and passing CLI flags:

```bash
docker run -p 9080:9080 -p 4000:4000 \
  -v kandev-data:/data \
  ghcr.io/kdlbs/kandev:latest \
  kandev start --backend-port 9080 --web-port 4000
```

## Docker Compose

Create a `docker-compose.yml`:

```yaml
services:
  kandev:
    image: ghcr.io/kdlbs/kandev:latest
    ports:
      - "8080:8080"
      - "3000:3000"
    volumes:
      - kandev-data:/data
    restart: unless-stopped

volumes:
  kandev-data:
```

```bash
docker compose up -d
```

### With PostgreSQL

```yaml
services:
  kandev:
    image: ghcr.io/kdlbs/kandev:latest
    ports:
      - "8080:8080"
      - "3000:3000"
    volumes:
      - kandev-data:/data
    environment:
      KANDEV_DATABASE_DRIVER: postgres
      KANDEV_DATABASE_HOST: postgres
      KANDEV_DATABASE_PORT: "5432"
      KANDEV_DATABASE_USER: kandev
      KANDEV_DATABASE_PASSWORD: secret
      KANDEV_DATABASE_DBNAME: kandev
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  postgres:
    image: postgres:17
    environment:
      POSTGRES_USER: kandev
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: kandev
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U kandev"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped

volumes:
  kandev-data:
  postgres-data:
```

## Docker-in-Docker (Agent Containers)

By default, `KANDEV_DOCKER_ENABLED=false` inside the container. To enable Docker-based agent execution, mount the Docker socket:

```bash
docker run -p 8080:8080 -p 3000:3000 \
  -v kandev-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e KANDEV_DOCKER_ENABLED=true \
  ghcr.io/kdlbs/kandev:latest
```

> **Note:** Mounting the Docker socket gives the container full access to the host's Docker daemon. Only do this in trusted environments.

## Health Check

The backend exposes a `/health` endpoint:

```bash
curl http://localhost:8080/health
```

For Docker health checks in compose:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 15s
```

## Troubleshooting

```bash
# View logs
docker logs kandev

# Follow logs
docker logs -f kandev

# Shell into the container
docker exec -it kandev /bin/bash

# Check data volume
docker volume inspect kandev-data
```
