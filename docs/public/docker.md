---
title: "Docker"
description: "Run the Kandev control plane in Docker and understand Docker-based agent execution."
---

# Docker

The published image runs the Kandev control plane: native backend, web UI, API, WebSocket endpoint, external MCP endpoint, and the host-side `agentctl`. This is different from the **Local Docker executor**, which creates a separate container for an agent.

For Kubernetes, see [Kubernetes](k8s.md). For executor profiles, see [Executors](executors.md#local-docker).

## Quick start

Bind to host loopback unless an authenticated reverse proxy protects the service:

```bash
docker volume create kandev-data
docker run -d \
  --name kandev \
  --restart unless-stopped \
  -p 127.0.0.1:38429:38429 \
  -v kandev-data:/data \
  ghcr.io/kdlbs/kandev:latest
```

Open `http://localhost:38429` and follow logs with:

```bash
docker logs -f kandev
```

Kandev currently has no built-in multi-user web login or API authorization boundary. `auth.jwtSecret` does not add one. Docker's unqualified `-p 38429:38429` publishes on every host interface, so do not use that form on an untrusted network. Use loopback, a private network/VPN, or an authenticated reverse proxy with TLS.

## Published images

The release workflow publishes multi-architecture `linux/amd64` and `linux/arm64` images to `ghcr.io/kdlbs/kandev`.

| Flavor | Moving tag | Version tags | Contents |
|---|---|---|---|
| Base | `latest` | `X.Y.Z`, `vX.Y.Z` | Kandev, Node 24/npm, Git, `gh`, Python/pipx, Apprise, Azure CLI, and the Azure DevOps extension |
| Universal | `universal` | `X.Y.Z-universal`, `vX.Y.Z-universal` | Base plus Go, Rust, pnpm, build tools, common developer CLIs, and Playwright Chromium system libraries |

The universal image does not include Playwright browser downloads, JDKs, .NET, or database servers. Its tool versions are pinned in `Dockerfile.universal` for each release. See the [image guide](https://github.com/kdlbs/kandev/blob/main/docs/images.md) for the inclusion policy and derived-image examples.

Use a version tag or digest in a persistent deployment:

```bash
docker pull ghcr.io/kdlbs/kandev:X.Y.Z
docker pull ghcr.io/kdlbs/kandev:X.Y.Z-universal
```

Replace `X.Y.Z` with a real release. `latest` moves on release. `universal` moves on release and is also rebuilt from the current `latest` base each Monday by the repository's scheduled workflow; dated `universal-weekly-YYYYMMDD` tags identify those rebuilds. Release-versioned universal tags remain unchanged.

The repository root `Dockerfile` is release packaging, not a source-build Dockerfile. The release workflow first builds a native bundle for each architecture, prepares a build context containing `bundle/` plus `docker-entrypoint.sh`, builds the amd64 and arm64 images separately, and joins their digests into the published manifest. A plain `docker build .` from a checkout fails because `bundle/` is intentionally absent.

For extra OS tools, derive from a pinned published image so the release bundle and entrypoint stay intact:

```dockerfile
FROM ghcr.io/kdlbs/kandev:X.Y.Z
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends postgresql-client \
 && rm -rf /var/lib/apt/lists/*
```

Leaving the final configured user as root is intentional for this base flavor: the inherited entrypoint repairs `/data` and drops to `kandev` before starting the command. If a derivative finishes with `USER kandev`, provision writable volume ownership in advance, as the universal flavor does.

## Image runtime behavior

The base image:

- uses Debian Bookworm and exposes only TCP 38429;
- sets `KANDEV_HOME_DIR=/data`, `HOME=/data/home`, and `NPM_CONFIG_PREFIX=/data/.npm-global`;
- disables the Docker executor with `KANDEV_DOCKER_ENABLED=false`;
- starts `kandev start --backend-port 38429 --verbose` under `tini`;
- starts as root only long enough to create `/data/home` and recursively `chown /data`, then runs Kandev as user `kandev` (UID 1000) through `gosu`.

The universal derivative sets `USER kandev`, so it skips the base entrypoint's root ownership repair. Pre-create a writable bind mount before using that flavor. Recursive ownership repair in the base image makes named volumes convenient, but can be slow on a large bind mount and can fail on root-squashed network storage. To run either image directly as UID 1000, pre-create every required path with suitable ownership and test the storage driver.

## Persistence

Always mount `/data`. Removing a container without a volume removes its database, workspaces, installed agent CLIs, and authentication state.

| Volume path | Data |
|---|---|
| `/data/data/` | SQLite `kandev.db`, WAL/SHM files, and SQLite backup snapshots |
| `/data/tasks/`, `/data/worktrees/`, `/data/repos/`, `/data/sessions/`, `/data/lsp-servers/` | Task and repository runtime state |
| `/data/agent-sessions/` | Selectively seeded per-execution agent credential/session directories for Docker executors |
| `/data/.npm-global/` | Agent CLIs installed at runtime |
| `/data/home/` | Persistent home for `gh`, agent CLI auth, Azure config, caches, and user configuration |

For a bind mount:

```bash
sudo install -d -o 1000 -g 1000 /srv/kandev
docker run -d \
  --name kandev \
  -p 127.0.0.1:38429:38429 \
  -v /srv/kandev:/data \
  ghcr.io/kdlbs/kandev:X.Y.Z
```

Confirm UID/GID policy on your host before copying this example. The image guarantees UID 1000 for the user but does not promise GID 1000 for its primary group; its root entrypoint normally corrects ownership using the image's actual group.

### CLI authentication inside the container

The base and universal flavors have different configured users. Always select `kandev` explicitly for interactive login and installs so files under `/data/home` remain usable by the service:

```bash
docker exec --user kandev -it kandev gh auth login
docker exec --user kandev -it kandev sh
```

The same rule applies to `claude login`, `codex login`, and manual `npm install -g` commands. Treat `/data/home` and the database as secret material when backing them up.

## Configuration

Pass backend settings as environment variables or mount a read-only `/etc/kandev/config.yaml`. See [Configuration](configuration.md) for the canonical field names, environment mapping, validation, and precedence.

Container-specific defaults are:

| Setting | Image value | Notes |
|---|---|---|
| `KANDEV_HOME_DIR` | `/data` | Root for database and runtime state |
| `HOME` | `/data/home` | Persistent CLI credentials and caches |
| `NPM_CONFIG_PREFIX` | `/data/.npm-global` | Runtime-installed npm CLIs |
| `KANDEV_DOCKER_ENABLED` | `false` | Overrides the backend's ordinary host default |
| `KANDEV_NO_BROWSER` | `1` | Prevents browser launch |
| Internal listener | `38429` | Fixed by the image command unless that command is replaced |
| Default log level | `info` | The image command passes `--verbose`; an explicit `KANDEV_LOG_LEVEL` wins |

Example:

```bash
docker run -d \
  --name kandev \
  -p 127.0.0.1:38429:38429 \
  -v kandev-data:/data \
  -e KANDEV_LOG_LEVEL=warn \
  ghcr.io/kdlbs/kandev:X.Y.Z
```

To expose a different host port, leave the internal command alone:

```bash
docker run -d \
  --name kandev \
  -p 127.0.0.1:9080:38429 \
  -v kandev-data:/data \
  ghcr.io/kdlbs/kandev:X.Y.Z
```

Open `http://localhost:9080`. If you replace the container command to change its internal port, publish that same port and retain `--verbose` if info logs are desired.

## Docker Compose

```yaml
services:
  kandev:
    image: ghcr.io/kdlbs/kandev:X.Y.Z
    ports:
      - "127.0.0.1:38429:38429"
    volumes:
      - kandev-data:/data
    restart: unless-stopped

volumes:
  kandev-data:
```

```bash
docker compose up -d
docker compose logs -f kandev
```

### PostgreSQL example

PostgreSQL moves database rows out of the Kandev volume, but `/data` is still required for workspaces, CLI installs, auth files, and other runtime state.

```yaml
services:
  kandev:
    image: ghcr.io/kdlbs/kandev:X.Y.Z
    ports:
      - "127.0.0.1:38429:38429"
    volumes:
      - kandev-data:/data
    environment:
      KANDEV_DATABASE_DRIVER: postgres
      KANDEV_DATABASE_HOST: postgres
      KANDEV_DATABASE_PORT: "5432"
      KANDEV_DATABASE_USER: kandev
      KANDEV_DATABASE_PASSWORD: "${KANDEV_DB_PASSWORD:?set KANDEV_DB_PASSWORD}"
      KANDEV_DATABASE_DBNAME: kandev
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: kandev
      POSTGRES_PASSWORD: "${KANDEV_DB_PASSWORD:?set KANDEV_DB_PASSWORD}"
      POSTGRES_DB: kandev
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U kandev -d kandev"]
      interval: 5s
      timeout: 3s
      retries: 10
    restart: unless-stopped

volumes:
  kandev-data:
  postgres-data:
```

Put `KANDEV_DB_PASSWORD` in a permission-restricted deployment secret, not shell history or committed YAML. Kandev's built-in System backup/restore covers SQLite only; use `pg_dump` and a tested PostgreSQL restore procedure here.

## Reverse proxy and network policy

The backend serves SPA, API, `/ws`, `/mcp`, and `/health` on one origin. Proxy the entire root path and preserve WebSocket upgrades. Caddy does this automatically:

```text
kandev.example.com {
    reverse_proxy kandev:38429
}
```

TLS alone is not authentication. Put an identity-aware/authenticated gateway in front, restrict direct access to the Kandev container, and apply CSRF/origin policy appropriate to that gateway. A subpath such as `/kandev/` is not a documented deployment base; prefer a dedicated host at `/`.

## Using Docker for agent environments

Host-installed Kandev can use **Settings > Executors > Docker** against a same-host daemon. The daemon must be reachable through global `docker.host`; the current runtime assumes bind-mount source paths exist on that daemon host. See [Local Docker executor](executors.md#local-docker) for image, credential, port, resume, and cleanup behavior.

### Containerized control plane limitation

Mounting `/var/run/docker.sock` into the Kandev service container is **not by itself a complete configuration**. Agent creation asks the daemon to bind-mount:

- the Linux `agentctl` helper path resolved inside the control-plane container;
- per-execution directories under the control plane's Kandev home;
- a local clone source in the filesystem-URL case.

The Docker daemon resolves those paths on the daemon host, not inside the Kandev container. A named `/data` volume and the image's `/app/...` helper therefore do not automatically exist at matching host paths. The current Docker executor also selects a Linux/amd64 helper unconditionally, so the agent container must be Linux/amd64-compatible (native or correctly emulated).

For reliable Local Docker execution, run the Kandev control plane on the Docker host. Alternatively, build a custom deployment that mirrors every required source at identical absolute host/container paths and test cleanup, architecture, permissions, and upgrades. This advanced layout has no supplied Compose or Kubernetes manifest. Use SSH or Sprites when the control plane itself must remain containerized.

Giving Kandev a Docker socket or daemon API grants near-root control of that host. Protect the endpoint, never expose an unauthenticated TCP daemon, and do not give untrusted Kandev users profile-build access.

Remote Docker profiles are not a workaround: that executor runtime is currently unimplemented.

## Health and observability

`GET /health` returns 503 during startup and 200 after routes are registered and the listener is accepting connections:

```bash
curl --fail http://localhost:38429/health
```

It is a startup/readiness signal, not a deep database, Git, Docker, provider, or agent check. A Compose health check can still use it:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:38429/health"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 15s
```

Container logs go to stdout/stderr by default. If configuration sends logs to a file, place that path on persistent storage and account for Kandev's file rotation settings.

## Upgrade and remove

Back up first; then pull and recreate with the same `/data` volume and configuration:

```bash
docker compose pull kandev
docker compose up -d kandev
docker compose logs -f kandev
```

SQLite schema migrations run at startup and create a pre-migration SQLite snapshot when needed. PostgreSQL does not receive that built-in snapshot. A container/image rollback does not roll back the database schema, so keep a tested backup from before the upgrade. See [Operations](operations.md) for backup and restore details.

Stopping or removing the service container leaves the named volume:

```bash
docker stop kandev
docker rm kandev
docker volume inspect kandev-data
```

Delete `kandev-data` only after verifying that its database, workspaces, and credentials are no longer needed.

## Troubleshooting

- **UI unreachable:** check `docker ps`, published address/port, host firewall, and `docker logs kandev`; then call `/health`.
- **Permission denied under `/data`:** inspect mount ownership and root-squash behavior. The runtime user is UID 1000 after entrypoint setup.
- **CLI login works only as root:** repeat it with `docker exec --user kandev`; repair ownership of the affected files before restarting.
- **Image pull fails:** authenticate to GHCR if your network policy requires it and verify the tag/platform with `docker buildx imagetools inspect`.
- **Database connection fails:** test DNS/TCP from the Kandev container, credentials, database name, and `sslMode`.
- **WebSocket disconnects behind proxy:** forward the whole origin, enable upgrade support, and increase proxy idle timeouts.
- **Docker agent container fails to mount helper/session paths:** the control plane is probably containerized or using a remote daemon whose filesystem paths do not match; use a same-host control plane or a fully mirrored custom layout.
- **Disk growth:** inspect `/data`, retained Docker agent containers, image layers/build cache, and Docker volumes before deleting anything.

Related pages: [Configuration](configuration.md), [Executors](executors.md), [Operations](operations.md), and [Run as a Service](run-as-a-service.md).
