# Docker Guide

Run Kandev in a Docker container. For Kubernetes deployment, see [k8s.md](k8s.md).

## Quick Start

```bash
docker run -p 8080:8080 -v kandev-data:/data ghcr.io/kdlbs/kandev:latest
```

Open `http://localhost:8080` in your browser.

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
docker run -p 8080:8080 \
  -v kandev-data:/data \
  -e KANDEV_LOG_LEVEL=debug \
  ghcr.io/kdlbs/kandev:latest
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KANDEV_HOME_DIR` | `/data` | Kandev home directory — contains `data/` (DB), `tasks/`, `worktrees/`, `repos/`, `sessions/`, and `lsp-servers/` |
| `KANDEV_DATABASE_DRIVER` | `sqlite` | Database driver (`sqlite` or `postgres`) |
| `KANDEV_DATABASE_PATH` | `$KANDEV_HOME_DIR/data/kandev.db` | SQLite database file path (override) |
| `KANDEV_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `KANDEV_LOGGING_FORMAT` | `text` | Log format: `text` or `json` |
| `KANDEV_DOCKER_ENABLED` | `false` | Enable Docker runtime for agents (see below) |

> **Upgrading from a pre-`KANDEV_HOME_DIR` image?** The SQLite DB path moved from `/data/kandev.db` to `/data/data/kandev.db`. The backend auto-migrates the legacy `kandev.db` (plus any `-wal`/`-shm` files) on first boot — look for `Migrated SQLite database from pre-KANDEV_HOME_DIR location` in the logs. If you prefer to pin the old location instead, set `-e KANDEV_DATABASE_PATH=/data/kandev.db`. If you previously set `KANDEV_DATA_DIR`, replace it with `KANDEV_HOME_DIR`.

### PostgreSQL

To use PostgreSQL instead of SQLite:

```bash
docker run -p 8080:8080 \
  -e KANDEV_DATABASE_DRIVER=postgres \
  -e KANDEV_DATABASE_HOST=host.docker.internal \
  -e KANDEV_DATABASE_PORT=5432 \
  -e KANDEV_DATABASE_USER=kandev \
  -e KANDEV_DATABASE_PASSWORD=secret \
  -e KANDEV_DATABASE_DBNAME=kandev \
  ghcr.io/kdlbs/kandev:latest
```

## Port

Kandev exposes a single port. The Go backend serves the API, WebSocket, and reverse-proxies the Next.js frontend — all on one port:

| Port | Service |
|------|---------|
| `8080` | API + WebSocket + Web UI |

Override the port:

```bash
docker run -p 9080:9080 \
  -v kandev-data:/data \
  ghcr.io/kdlbs/kandev:latest \
  kandev start --backend-port 9080
```

## Docker Compose

Create a `docker-compose.yml`:

```yaml
services:
  kandev:
    image: ghcr.io/kdlbs/kandev:latest
    ports:
      - "8080:8080"
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

## Reverse Proxy

Since Kandev serves everything on a single port, a reverse proxy only needs to forward all traffic to port 8080. No extra environment variables are needed — the frontend automatically uses `window.location.origin` to reach the API.

### Docker Compose with Caddy

```yaml
services:
  kandev:
    image: ghcr.io/kdlbs/kandev:latest
    volumes:
      - kandev-data:/data
    restart: unless-stopped

  caddy:
    image: caddy:2
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - caddy-data:/data
      - ./Caddyfile:/etc/caddy/Caddyfile
    restart: unless-stopped

volumes:
  kandev-data:
  caddy-data:
```

Example `Caddyfile`:

```
kandev.example.com {
    reverse_proxy kandev:8080
}
```

## Docker-in-Docker (Agent Containers)

By default, `KANDEV_DOCKER_ENABLED=false` inside the container. To enable Docker-based agent execution, mount the Docker socket:

```bash
docker run -p 8080:8080 \
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
