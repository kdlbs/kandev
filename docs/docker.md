# Docker Guide

Run Kandev in a Docker container. For Kubernetes deployment, see [k8s.md](k8s.md).

## Quick Start

```bash
docker run -p 38429:38429 -v kandev-data:/data ghcr.io/kdlbs/kandev:latest
```

Open `http://localhost:38429` in your browser.

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

### What lives on the volume

| Path | Contents |
|---|---|
| `/data/data/` | SQLite database (`kandev.db`, `-wal`, `-shm`) |
| `/data/worktrees/`, `/data/tasks/`, `/data/repos/`, `/data/sessions/`, `/data/lsp-servers/` | Per-session state |
| `/data/.npm-global/` | Agent CLIs installed via `npm install -g` (`NPM_CONFIG_PREFIX`) |
| `/data/home/` | `$HOME` for the in-container `kandev` user — `gh` CLI and agent CLI auth state |

### Persistent agent and `gh` CLI auth

The image sets `HOME=/data/home`, so every CLI that writes its auth under `$HOME` lands on the volume and survives container restarts and image upgrades:

- `gh` CLI — `~/.config/gh/hosts.yml`
- Claude Code — `~/.claude/.credentials.json`, `~/.claude.json`
- Codex — `~/.codex/auth.json`, `~/.codex/config.toml`
- Auggie — `~/.augment/session.json`
- GitHub Copilot — `~/.copilot/...`
- OpenCode, Amp — `~/.config/<tool>/...`

A one-time `docker exec -it kandev gh auth login` (or `claude login`, `codex login`, etc.) is enough; you do not need to redo it after `docker pull` and recreating the container.

> The GitHub PAT configured in **Settings → Integrations → GitHub** is stored as a secret in the database and has always persisted. The `HOME=/data/home` setup covers the separate `gh auth login` flow that the backend falls back to when no `GITHUB_TOKEN` secret is set.

## Configuration

Configuration is done via `KANDEV_`-prefixed environment variables:

```bash
docker run -p 38429:38429 \
  -v kandev-data:/data \
  -e KANDEV_LOG_LEVEL=debug \
  ghcr.io/kdlbs/kandev:latest
```

### Environment Variables

See [`configuration.md`](./configuration.md) for the full reference (including the YAML form and every knob the backend reads). The table below covers the env vars most often set in a Docker deployment.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `KANDEV_HOME_DIR` | No | `/data` | Kandev home directory - contains `data/` (DB), `tasks/`, `worktrees/`, `repos/`, `sessions/`, and `lsp-servers/` |
| `KANDEV_DATABASE_DRIVER` | No | `sqlite` | Database driver (`sqlite` or `postgres`) |
| `KANDEV_DATABASE_PATH` | No | `$KANDEV_HOME_DIR/data/kandev.db` | SQLite database file path (override) |
| `KANDEV_LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `KANDEV_LOGGING_FORMAT` | No | `text` | Log format: `text` or `json` |
| `KANDEV_LOGGING_OUTPUTPATH` | No | `stdout` | Log destination: `stdout`, `stderr`, or a file path (rotated when a file) |
| `KANDEV_LOGGING_MAXSIZEMB` | No | `100` | Rotate the log file when it exceeds this size (MB). File output only. |
| `KANDEV_LOGGING_MAXBACKUPS` | No | `5` | Max rotated files to retain (`0` = unlimited). File output only. |
| `KANDEV_LOGGING_MAXAGEDAYS` | No | `30` | Max age of rotated files in days (`0` = unlimited). File output only. |
| `KANDEV_LOGGING_COMPRESS` | No | `true` | Gzip rotated files. File output only. |
| `KANDEV_DOCKER_ENABLED` | No | `false` | Enable Docker runtime for agents (see below) |

> **File-mode note:** when `KANDEV_LOGGING_OUTPUTPATH` is a file path, the active log file is created with mode `0600` (owner read/write only). Run any log shipper or sidecar as the same user, or use `stdout`/`stderr` and let the container runtime collect logs.

> **Upgrading from a pre-`KANDEV_HOME_DIR` image?** The SQLite DB path moved from `/data/kandev.db` to `/data/data/kandev.db`. The backend auto-migrates the legacy `kandev.db` (plus any `-wal`/`-shm` files) on first boot — look for `Migrated SQLite database from pre-KANDEV_HOME_DIR location` in the logs. If you prefer to pin the old location instead, set `-e KANDEV_DATABASE_PATH=/data/kandev.db`. If you previously set `KANDEV_DATA_DIR`, replace it with `KANDEV_HOME_DIR`.

### PostgreSQL

To use PostgreSQL instead of SQLite:

```bash
docker run -p 38429:38429 \
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
| `38429` | API + WebSocket + Web UI |

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
      - "38429:38429"
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
      - "38429:38429"
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

Since Kandev serves everything on a single port, a reverse proxy only needs to forward all traffic to port 38429. No extra environment variables are needed — the frontend automatically uses `window.location.origin` to reach the API.

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
    reverse_proxy kandev:38429
}
```

## Docker-in-Docker (Agent Containers)

By default, `KANDEV_DOCKER_ENABLED=false` inside the container. To enable Docker-based agent execution, mount the Docker socket:

```bash
docker run -p 38429:38429 \
  -v kandev-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e KANDEV_DOCKER_ENABLED=true \
  ghcr.io/kdlbs/kandev:latest
```

> **Note:** Mounting the Docker socket gives the container full access to the host's Docker daemon. Only do this in trusted environments.

## Upgrading

```bash
docker pull ghcr.io/kdlbs/kandev:latest
docker compose up -d  # or: docker stop kandev && docker rm kandev && docker run ...
```

The volume at `/data` carries over the database, worktrees, npm globals, and `$HOME` for agent CLIs, so there is no manual migration step.

> **Upgrading across the `HOME=/data/home` change:** if you previously ran `docker exec` to `gh auth login` or log in to agent CLIs on a pre-`HOME=/data/home` image, that state lived in the ephemeral `/home/kandev` inside the container and is not carried over. Log in once on the new container and it will persist for all subsequent upgrades. If you want to preserve the old state, copy it onto the volume before recreating the container:
>
> ```bash
> docker exec kandev sh -c 'cp -a /home/kandev/. /data/home/ 2>/dev/null || true'
> ```

## Health Check

The backend exposes a `/health` endpoint:

```bash
curl http://localhost:38429/health
```

For Docker health checks in compose:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:38429/health"]
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
