---
title: "Configuration"
description: "Configure Kandev workspaces, runtimes, settings, and environment behavior."
---

# Configuration

Kandev has three distinct configuration surfaces:

- backend startup configuration: defaults, `config.yaml`, then environment variables;
- persistent product settings edited in the web UI and stored in the database; and
- executor, agent, repository, and workflow profiles stored through their own Settings pages.

This page is the startup-configuration reference. Executor-specific fields are covered in [Executors](./executors.md), and deployment examples are in [Docker](./docker.md), [Kubernetes](./k8s.md), and [Run as a service](./run-as-a-service.md).

## Quick path

1. Use the built-in profile defaults for a first run.
2. Add `config.yaml` only for stable operator-wide settings.
3. Use environment variables for deployment-specific overrides and secrets.
4. Use the web UI for persistent product settings, agents, executors, and workflows.

## Load order and lifecycle

At backend startup, later sources override earlier ones:

1. embedded production-profile values and built-in defaults;
2. the first existing `config.yaml`; and
3. environment variables.

The public launcher has no `--config` option. It searches for exactly these
files, in order:

1. `config.yaml` in the process working directory;
2. `<KANDEV_HOME_DIR>/config.yaml`, or `~/.kandev/config.yaml` when
   `KANDEV_HOME_DIR` is unset; and
3. `/etc/kandev/config.yaml`.

Kandev uses only the first existing candidate. It does not merge files. A
missing candidate is skipped, but an unreadable, malformed, or invalid first
candidate stops startup and names that file. A home configuration file cannot
set `homeDir`, because it cannot relocate the directory that selected it.

Configuration is read at process start; there is no file watcher. Restart Kandev after changing YAML or environment variables. The CLI, desktop shell, service manager, Docker, or Kubernetes may set environment variables on the backend, so those values can override a file unexpectedly. Use the process/service/container environment as the final source of truth.

## Environment-variable naming

Viper maps a nested YAML key by replacing `.` with `_`, adding `KANDEV_`, and uppercasing. It does **not** split camelCase words. For example:

```text
database.dbName       -> KANDEV_DATABASE_DBNAME
repoClone.basePath    -> KANDEV_REPOCLONE_BASEPATH
```

Some common camelCase keys have explicit compatibility aliases. Use the documented exact names below; snake_case spellings not listed here are not equivalent.

## Complete backend reference

### Root and server

<details>
<summary>Complete backend startup reference</summary>

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `homeDir` | `KANDEV_HOME_DIR` | `~/.kandev` | Root for data, tasks, worktrees, cloned repositories, sessions, and logs. A leading `~/` expands. |
| `server.host` | `KANDEV_SERVER_HOST` | `0.0.0.0` | HTTP listen address. Use `127.0.0.1` for local-only access. |
| `server.port` | `KANDEV_SERVER_PORT` (`KANDEV_BACKEND_PORT`, `KANDEV_PORT` aliases) | `38429` | UI, HTTP API, WebSocket, and MCP port; must be `1`-`65535`. The launcher normally supplies its selected port. |
| `server.readTimeout` | `KANDEV_SERVER_READTIMEOUT` | `30` | HTTP read timeout in seconds. |
| `server.writeTimeout` | `KANDEV_SERVER_WRITETIMEOUT` | `30` | HTTP write timeout in seconds. |
| `server.webInternalUrl` | `KANDEV_WEB_INTERNAL_URL` | empty | Development reverse-proxy target for a separately running web app. Installed releases normally serve embedded assets. |
| `server.webTitlePrefix` | `KANDEV_WEB_TITLE_PREFIX` | empty | Prefixes the browser tab title as `<prefix> Kandev` (for example `TEST` renders `TEST Kandev`), so several instances stay distinguishable in adjacent tabs. `make dev` defaults to `Dev`; `make start-debug` keeps production defaults, enables diagnostics, and defaults to `Debug`; PR previews use `Preview`. An explicit value overrides these defaults. Empty keeps the plain `Kandev` title. |
| `server.trustedProxies` | `KANDEV_TRUSTED_PROXIES` | empty list | IP addresses or CIDR ranges for proxies whose forwarded client headers Kandev accepts. See [Trusted proxies](#trusted-proxies-for-x-forwarded-for). |

The default host exposes the server on every interface even though the CLI prints a `localhost` URL. The current local product path must not be treated as an authenticated multi-user perimeter. For remote access, bind to loopback and use a trusted authenticated tunnel/proxy, or isolate the network at the deployment layer.

### Database

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `database.driver` | `KANDEV_DATABASE_DRIVER` | `sqlite` | `sqlite` or `postgres` (case-normalized). |
| `database.path` | `KANDEV_DATABASE_PATH` | `<home>/data/kandev.db` | SQLite database path. Empty resolves to the default. |
| `database.host` | `KANDEV_DATABASE_HOST` | `localhost` | PostgreSQL only. |
| `database.port` | `KANDEV_DATABASE_PORT` | `5432` | PostgreSQL only; must be `1`-`65535`. |
| `database.user` | `KANDEV_DATABASE_USER` | `kandev` | Required and non-empty for PostgreSQL. |
| `database.password` | `KANDEV_DATABASE_PASSWORD` | empty | PostgreSQL password; requirement depends on server authentication policy. |
| `database.dbName` | `KANDEV_DATABASE_DBNAME` | `kandev` | Required and non-empty for PostgreSQL. |
| `database.sslMode` | `KANDEV_DATABASE_SSLMODE` | `disable` | `disable`, `require`, `verify-ca`, or `verify-full`. |
| `database.maxConns` | `KANDEV_DATABASE_MAXCONNS` | `25` | PostgreSQL maximum pool size. |
| `database.minConns` | `KANDEV_DATABASE_MINCONNS` | `5` | PostgreSQL minimum pool size. |

SQLite is the supported default and enables WAL mode. PostgreSQL deployments must provision the database, network policy, TLS trust, backups, and credentials before starting Kandev. Passing the password in an environment variable avoids putting it in YAML but still exposes it to processes/administrators allowed to inspect the environment; use your platform's secret injection controls.

`database.path` is an advanced SQLite file-path override. The **Settings → System → Database** and **Backups** pages use that exact file, its WAL files, and the sibling `backups/` directory. Restore stages `<configured-database-path>.new`, quiesces scheduling and active workers, validates the checkpoint result, closes the SQLite pool, and uses rollback-capable quarantine replacement for the configured file and WAL sidecars. Restart Kandev immediately after a successful restore. When the override is empty, the default path remains `<home>/data/kandev.db` and the backup directory remains `<home>/data/backups/`. Kandev does not move snapshots from another directory automatically. The System restore endpoint is SQLite-only; use PostgreSQL recovery tools for PostgreSQL.

One backend owns a Kandev home at a time. When SQLite uses a custom path outside that home, the backend also owns that database path, so separate homes alone do not permit concurrent backends against one SQLite file. Use a separate home and database for an intentional second instance. Ownership is released when the backend exits.

Database-only snapshots also omit `<home>/data/master.key`, the AES-256 key used to decrypt stored secrets. Preserve that owner-only key with an independently secured home/data backup; restoring the database without its matching key leaves encrypted credentials unreadable. See [Operations](./operations.md).

### Event bus and NATS

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `nats.url` | `KANDEV_NATS_URL` | empty | Empty uses the in-process event bus; otherwise connect to NATS. |
| `nats.clusterId` | `KANDEV_NATS_CLUSTERID` | `kandev-cluster` | Accepted compatibility field; the current NATS client does not consume it. |
| `nats.clientId` | `KANDEV_NATS_CLIENTID` | `kandev-client` | NATS connection name. |
| `nats.maxReconnects` | `KANDEV_NATS_MAXRECONNECTS` | `10` | Reconnect limit; the client uses a two-second reconnect wait and a 5 MiB reconnect buffer. |
| `events.namespace` | `KANDEV_EVENTS_NAMESPACE` | derived | Queue-group namespace. Empty derives a stable, sanitized hash from database identity. |

An external NATS URL moves event traffic across the configured network and can embed credentials/TLS parameters. Protect it as a secret where applicable, require TLS for untrusted networks, and keep namespaces distinct when deployments share one NATS server. `clusterId` does not provide isolation in the current implementation.

### Docker runtime

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `docker.enabled` | `KANDEV_DOCKER_ENABLED` | `true` | Registers the local Docker executor. The client connects lazily, so startup can succeed without a daemon. |
| `docker.host` | `KANDEV_DOCKER_HOST` | `DOCKER_HOST`, otherwise platform socket | Docker endpoint used by the client. Defaults to `unix:///var/run/docker.sock` on Unix and `npipe:////./pipe/docker_engine` on Windows. |
| `docker.apiVersion` | `KANDEV_DOCKER_APIVERSION` | empty | Empty uses Docker API negotiation. |
| `docker.tlsVerify` | `KANDEV_DOCKER_TLSVERIFY` | `false` | Accepted compatibility field; not wired into the current client. |
| `docker.defaultNetwork` | `KANDEV_DOCKER_DEFAULTNETWORK` | `kandev-network` | Accepted compatibility field; not wired into current executor networking. |
| `docker.volumeBasePath` | `KANDEV_DOCKER_VOLUMEBASEPATH` | `/var/lib/kandev/volumes` on Unix; `%LOCALAPPDATA%\kandev\volumes` on Windows | Accepted compatibility field; not wired into current executor volume placement. |

The Docker socket is effectively root-equivalent on many hosts. Do not publish it or assume `docker.tlsVerify` secures a TCP daemon; it currently does not. Configure TLS through a supported Docker endpoint/environment and validate it independently, or keep the daemon local. See [Docker](./docker.md) and [Executors](./executors.md).

### Core agent service

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `agent.standaloneHost` | `KANDEV_AGENT_STANDALONE_HOST` | `localhost` | Host of the core `agentctl` control server. |
| `agent.standalonePort` | `AGENTCTL_PORT` or `KANDEV_AGENT_STANDALONE_PORT` | `39429` | Preferred control port. The launcher may supply a free fallback. |

The launcher starts `agentctl`, performs a one-time nonce handshake, and supplies the resulting per-launch token internally. Do not persist or proxy its bootstrap/auth state. Agent command, model, environment, permission, and MCP configuration belongs in agent profiles rather than this section.

### Setup and launch timing

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `tasks.preparationTimeout` | `KANDEV_TASK_PREPARATION_TIMEOUT` | `10m` | Positive Go duration for repository setup and executor-profile prepare scripts. |

`tasks.preparationTimeout` controls how long Kandev allows repository setup and
executor-profile prepare scripts to run. The value uses Go duration syntax,
such as `90s`, `10m`, or `1h`.

Only positive durations are accepted. An unset, invalid, zero, or negative value
uses the `10m` default. Kandev reads this environment variable when the backend
starts, so restart the backend after changing it. The setting applies to Local,
Worktree, Docker, Sprites, and SSH launches.

Runtime launch phases use the configured preparation timeout plus a fixed
five-minute allowance for runtime creation and `agentctl` readiness. With the
default, each launch-phase limit is `15m`. Preparation scripts use a separate
context, so earlier work such as Sprite uploads does not reduce their full
`10m` preparation budget. The environment variable overrides YAML. This is a
startup setting, not a database or Settings value.

### Capacity and managed-process startup settings

| YAML key | Environment variable | Type and default | Current behavior |
|---|---|---|---|
| `credentials.file` | `KANDEV_CREDENTIALS_FILE` | path, empty | Optional fallback JSON credential file. The file is read lazily and must be restricted to the Kandev service account. |
| `limits.ghMaxConcurrent` | `KANDEV_GH_MAX_CONCURRENT` | positive integer, `8` | Process-wide `gh` subprocess admission cap. |
| `limits.gitMaxConcurrent` | `KANDEV_GIT_MAX_CONCURRENT` | positive integer, `12` | Process-wide `git` subprocess admission cap. |
| `limits.lspMaxConnections` | `KANDEV_LSP_MAX_CONNECTIONS` | positive integer, `8` | Active browser-to-task-host language-server connection cap. |
| `messageQueue.maxPerSession` | `KANDEV_QUEUE_MAX_PER_SESSION` | integer `>= 0`, `10` | Per-session pending-message cap. Zero means unlimited. A non-negative YAML or environment value locks capacity in Settings; a negative environment value means unlimited, and invalid environment input falls through to the lower-precedence source. |
| `agentctl.idleTimeout` | `KANDEV_ACP_IDLE_TIMEOUT` | Go duration, `1h` | Idle managed-agent reaping timeout. Zero disables reaping. |
| `agentctl.idleReaperInterval` | `KANDEV_ACP_IDLE_REAPER_INTERVAL` | Go duration, `1m` | Interval between idle-agent scans. |
| `agentctl.notificationQueueCapacity` | `KANDEV_ACP_NOTIF_QUEUE` | integer `1024`-`131072`, `131072` | ACP inbound notification queue capacity. An out-of-range YAML value fails startup; an invalid or out-of-range environment value uses the built-in default. |
| `planning.coalesceWindowMs` | `KANDEV_PLAN_COALESCE_WINDOW_MS` | integer `>= 0`, `300000` | Same-author plan revision coalescing window in milliseconds. |
| `observability.otlpEndpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | URL, empty | OTLP tracing endpoint. Treat the value and emitted spans as sensitive. |
| `office.schedulerTickMs` | `KANDEV_OFFICE_SCHEDULER_TICK_MS` | positive integer, `5000` | Office queued/retry run safety-net interval in milliseconds. |
| `launcher.webPort` | `KANDEV_WEB_PORT` | automatic, `0` | Development web-server port. It is used with `dev` and ignored by embedded-asset launches. |
| `launcher.healthTimeoutMs` | `KANDEV_HEALTH_TIMEOUT_MS` | positive integer, `45000` | Launcher startup-health timeout in milliseconds. Development and E2E profiles use a longer default. |
| `launcher.noBrowser` | `KANDEV_NO_BROWSER` | `false` | Suppresses browser opening when true or `1`, depending on launch mode. |

These settings are read at startup. Their environment aliases remain
compatible overrides, and a YAML value is not copied into a public process
environment variable. Managed agentctl children receive their resolved subset
through a private process contract.

### Authentication, Office, Plugins, and feature flags

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `auth.jwtSecret` | `KANDEV_AUTH_JWTSECRET` | generated value | Accepted and validated compatibility configuration; the current main HTTP product path does not use it as an authentication boundary. |
| `auth.tokenDuration` | `KANDEV_AUTH_TOKENDURATION` | `3600` | Must be positive, but is not consumed by the current main HTTP product path. |
| `auth.sessionTTLHours` | `KANDEV_AUTH_SESSIONTTLHOURS` | `720` | Compatibility session lifetime setting. |
| `auth.cookieName` | `KANDEV_AUTH_COOKIE_NAME` | empty | Session cookie base name. Empty (default) means the effective name is derived from the request host: `kandev_session` on a default-port host, `kandev_session_<port>` on a ported host. This isolates multiple instances on one host (see [authentication](authentication.md#multiple-instances-on-one-host)). A non-empty value is used verbatim (never port-suffixed) and disables automatic isolation, so it must be unique per cookie host. Precedence: environment over config file over default. |
| `office.jwtSigningKey` | `KANDEV_OFFICE_JWTSIGNINGKEY` | random per start | HMAC key for Office agent-runtime JWTs. Set a stable secret when Office tasks must survive restarts. |
| `githubCredentialBroker.publicBaseUrl` | `KANDEV_GITHUB_CREDENTIAL_BROKER_PUBLIC_BASE_URL` | empty | Public HTTPS base URL used by remote executors to resolve GitHub credentials. Loopback HTTP is allowed for development. |
Runtime feature flags are documented in [Runtime feature toggles](#runtime-feature-toggles); they are profile, environment, and database-backed controls, not canonical YAML settings.

Do not infer security from `auth.jwtSecret`: setting it currently does not turn the local server into an authenticated public service. Office's JWT key has a narrower, active purpose. Store both active secrets and third-party API keys in your deployment secret manager; never commit them in `config.yaml`.

`voice.openAIApiKey` / `KANDEV_VOICE_OPENAI_API_KEY` was removed. Voice Mode is now the
[Voice Mode plugin](https://github.com/kdlbs/kandev-plugin-voice), and its transcription key lives
in that plugin's own settings. Kandev ignores the old key and no longer serves `/api/v1/transcribe`.

### Trusted proxies for X-Forwarded-For

`server.trustedProxies` and its `KANDEV_TRUSTED_PROXIES` override list the
reverse proxies whose forwarded client IP Kandev trusts. YAML uses an array;
the environment form is a comma-separated list of IP addresses or CIDR ranges,
for example `10.0.0.0/8,192.168.0.0/16`. IPv6 addresses and CIDRs are
accepted. When the TCP peer of a request is in the list, the client IP is
read from `X-Forwarded-For` (then `X-Real-IP`); otherwise those headers are
ignored and the TCP peer address is used. The resolved IP feeds the login
session record (Settings > Account > Security) and the login rate-limiter
key.

For a reverse proxy, use a narrow list of proxy addresses:

```yaml
server:
  trustedProxies:
    - 10.0.0.5
    - 192.168.0.0/16
```

Default: unset, meaning no trusted proxies. Forwarded headers are ignored
entirely and the recorded client IP is always the TCP peer. This is the
secure default: gin would otherwise trust every proxy by default, which lets
a directly reachable backend accept a spoofed client IP.

Security implication: only list proxies you control that always overwrite the
forwarded headers. A backend reached directly by a caller whose address
falls inside a listed range can have `X-Forwarded-For` spoofed by that
caller, which also defeats the ClientIP-keyed login rate limiter (login
attempts are limited per IP+email). Callers outside the listed ranges still
fall back to their own peer address.

An entry that is neither a valid IP nor a valid CIDR is rejected at startup
with a warning naming the bad value, and the whole variable is ignored (fail
closed: no partial trust). A trailing or doubled comma is treated the same
way. The backend never crashes on a bad value.

The same list gates `X-Forwarded-Host`, which a proxy sends so the port-scoped
session cookie can be resolved from the browser's original host. A proxy that
sends it while missing from the list logs
`ignoring X-Forwarded-Host from untrusted peer` with the peer address and the
forwarded host. That warning is logged once per distinct peer and host, not per
request, so it names the misconfiguration without flooding the log. Add the
peer it names to `KANDEV_TRUSTED_PROXIES` if it is your reverse proxy.

The value is read once at startup and must reach the backend process. Set the
environment override in the environment of the process that launches kandev,
for example a systemd drop-in for `kandev.service` (`systemctl --user edit
kandev.service`) or the container environment. The supervisor manifest at
`~/.kandev/supervisor/launch.json` is generated by the launcher on every
launch and is not an environment configuration source.

### Logging

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `logging.level` | `KANDEV_LOG_LEVEL` | `info` | File threshold: `debug`, `info`, `warn`, or `error`. `--debug` selects `debug`; normal and `--verbose` launches select `info`. |
| `logging.format` | `KANDEV_LOGGING_FORMAT` | `text`, or `json` in production/Kubernetes | `text` or `json`; `auto` is not accepted. |

Every backend launch writes to `<home>/logs/backend-logs.log` and prints that resolved path at startup. The active file appends across same-day restarts and accepts at most 16 MiB. Before a new entry would exceed that limit, Kandev closes the file as `backend-logs-YYYY-MM-DD-NNNNNN.log` and opens a new active file.

Active and closed backend files use at most 256 MiB in total. Kandev removes the oldest closed segments when needed, so high-volume periods keep the newest evidence instead of stopping file logging. Three UTC days is the maximum file age, not a reserved allocation for each day. The segment size, total budget, and maximum age are fixed and are not configurable. Legacy `backend-logs-YYYY-MM-DD.log` files remain readable during upgrades and count toward the total budget. Files are owner-only (`0600`) on Unix.

Normal launches write info and above to the file and warn and above to stdout. `--debug` writes debug and above to the file while stdout remains warn and above. `--verbose` writes info and above to both. The format default becomes JSON when `KUBERNETES_SERVICE_HOST` is non-empty or `KANDEV_ENV` is exactly `production`/`prod`; otherwise it is text.

Debug output may contain repository paths, subprocess output, prompts, file content, and tool-call data. Treat it as sensitive.

### Repository, worktree, and clone paths

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `repositoryDiscovery.roots` | `KANDEV_REPOSITORYDISCOVERY_ROOTS` | `[]` | Roots traversed by automatic repository discovery. Explicitly selected repository paths need not be included. Prefer absolute paths. Array encoding through environment variables is Viper-dependent; YAML is clearer. |
| `repositoryDiscovery.maxDepth` | `KANDEV_REPOSITORYDISCOVERY_MAXDEPTH` | `5` | Positive directory traversal depth. |
| `worktree.enabled` | `KANDEV_WORKTREE_ENABLED` | `true` | Enables the worktree provider. |
| `worktree.defaultBranch` | `KANDEV_WORKTREE_DEFAULTBRANCH` | `main` | Accepted compatibility field; current task behavior uses each repository's stored/detected default branch instead. |
| `worktree.cleanupOnRemove` | `KANDEV_WORKTREE_CLEANUPONREMOVE` | `true` | Accepted compatibility field; current lifecycle cleanup is controlled by repository/task operations, not this value. |
| `worktree.fetchTimeoutSeconds` | `KANDEV_WORKTREE_FETCHTIMEOUTSECONDS` | `60` | Git fetch timeout during worktree preparation. |
| `worktree.pullTimeoutSeconds` | `KANDEV_WORKTREE_PULLTIMEOUTSECONDS` | `60` | Git pull timeout during worktree preparation. |
| `repoClone.basePath` | `KANDEV_REPOCLONE_BASEPATH` | `<home>/repos` | Base directory for provider-backed clones. A leading `~/` expands. |

Discovery roots bound automatic filesystem traversal, so scope them narrowly. They do not authorize explicitly selected repository paths: **Add Local Repository** validates and saves the exact accessible Git repository the user chooses without widening automatic scans. Worktrees and clones can contain credentials or generated files ignored by Git; review repository copy-file and setup/cleanup settings before remote execution. See [Git operations](./git-operations.md).

### Debug configuration

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `debug.devMode` | `KANDEV_DEBUG_DEV_MODE` | `false` | Enables diagnostic endpoints and agent-message debug logging. |
| `debug.pprofEnabled` | `KANDEV_DEBUG_PPROF_ENABLED` | `false` | Legacy diagnostics switch. It enables pprof behavior but does not select the `dev` profile. |

`KANDEV_DEBUG_DEV_MODE=true` selects the `dev` profile. `make dev` sets that
selector and defaults the browser title to `Dev Kandev`. `make start-debug`
enables pprof and debug logging without selecting the `dev` profile, and
defaults the browser title to `Debug Kandev`. Debug mode is high risk. It
enables local diagnostic surfaces and implies
`KANDEV_DEBUG_AGENT_MESSAGES=true` and `KANDEV_DEBUG_PPROF_ENABLED=true` when
not explicitly locked by the environment. ACP JSONL frames include complete
prompts, file content, and tool calls. Do not enable it on a shared or
network-exposed backend.

## Minimal examples

For a local-only CLI server with an isolated home:

```bash
KANDEV_SERVER_HOST=127.0.0.1 \
KANDEV_HOME_DIR="$PWD/.kandev-local" \
kandev --headless
```

For a file-based deployment, override only what is needed:

```yaml
homeDir: "/srv/kandev"

server:
  host: "127.0.0.1"
  port: 38429

logging:
  format: "json"

repositoryDiscovery:
  roots:
    - "/srv/repositories"
```

This example changes automatic discovery only. An explicitly selected repository may live outside
`/srv/repositories` if the Kandev process can access and validate it.

A complete shape, including compatibility fields, is:

```yaml
homeDir: ""

server:
  host: "0.0.0.0"
  port: 38429
  readTimeout: 30
  writeTimeout: 30
  trustedProxies: []
  webInternalUrl: ""
  webTitlePrefix: ""

database:
  driver: "sqlite"
  path: ""
  host: "localhost"
  port: 5432
  user: "kandev"
  password: ""
  dbName: "kandev"
  sslMode: "disable"
  maxConns: 25
  minConns: 5

nats:
  url: ""
  clusterId: "kandev-cluster" # compatibility-only today
  clientId: "kandev-client"
  maxReconnects: 10

events:
  namespace: ""

docker:
  enabled: true
  host: "unix:///var/run/docker.sock" # use the Windows named pipe on Windows
  apiVersion: ""
  tlsVerify: false                    # compatibility-only today
  defaultNetwork: "kandev-network"  # compatibility-only today
  volumeBasePath: "/var/lib/kandev/volumes" # compatibility-only today

agent:
  standaloneHost: "localhost"
  standalonePort: 39429

tasks:
  preparationTimeout: "10m"

credentials:
  file: ""

limits:
  ghMaxConcurrent: 8
  gitMaxConcurrent: 12
  lspMaxConnections: 8

messageQueue:
  maxPerSession: 10

agentctl:
  idleTimeout: "1h"
  idleReaperInterval: "1m"
  notificationQueueCapacity: 131072

planning:
  coalesceWindowMs: 300000

observability:
  otlpEndpoint: ""

launcher:
  webPort: 0
  healthTimeoutMs: 45000
  noBrowser: false

auth:
  jwtSecret: ""       # compatibility-only for the main HTTP product path
  tokenDuration: 3600 # compatibility-only for the main HTTP product path

logging:
  level: "info"
  format: "text"

repositoryDiscovery:
  roots: []                # automatic scan roots; explicit paths need not be included
  maxDepth: 5

worktree:
  enabled: true
  defaultBranch: "main"    # compatibility-only today
  cleanupOnRemove: true    # compatibility-only today
  fetchTimeoutSeconds: 60
  pullTimeoutSeconds: 60

repoClone:
  basePath: ""

debug:
  devMode: false
  pprofEnabled: false

office:
  jwtSigningKey: ""
  schedulerTickMs: 5000

```

Copying this entire file is unnecessary and can freeze old defaults in a deployment. Keep only deliberate overrides. On Windows, do not copy the Unix Docker host/path literals from this example.

</details>

## Runtime feature toggles

**Settings → System → Feature Toggles** manages startup-time flags:

| Key | Environment lock | Production default | Effect |
|---|---|---|---|
| `features.dynamicAgentRouting` | `KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING` | off | Experimental dynamic profiles with ordered provider-error fallback. |
| `debug.devMode` | `KANDEV_DEBUG_DEV_MODE` | off | High-risk diagnostic endpoints and ACP frame logging. |

The `KANDEV_FEATURES_*` values have no canonical YAML keys. They are selected
from the embedded profile and can be overridden by the environment or the
database-backed toggle controls in this section. Use **Settings → System →
Feature Toggles** for persistent product changes.

UI changes are persisted in the database and require a restart. An explicitly set environment value wins and locks the UI control. Otherwise a database override wins over the embedded profile/default. Resetting a toggle removes its database override.

For a risky release feature, keep the flag off in the shipped profiles, enable it
only on a selected install through an admin override or explicit environment,
restart, and test it there. Promote the `prod` profile default only after the
feature is ready for everyone; keep the registry entry as a kill-switch until
the rollout is complete, then remove the live flag and move its key and
environment variable to the runtime registry's append-only retired identities.
Plugins are part of the base product and are not a runtime toggle.

The source checkout's `make dev` activates the embedded development profile, which enables Office, debug surfaces, ACP logging, and a mock agent; authentication and Claude background prompt handoff remain opt-in. Installed `run`/desktop builds select the safe production profile unless the environment explicitly opts in. E2E mock variables and routes are test-only and must never be enabled on a public deployment.

## Credentials and product settings
The **Unread Messages** preference in **Settings > General > Task Actions** controls the Slack-style **New** divider in session transcripts. It defaults off for each user, persists with user settings, and takes effect immediately. Enabling it also allows that user's active transcript view to advance the session read cursor.


Most integrations, executor profiles, agent profiles, MCP servers, repository settings, and UI preferences are persistent database records edited under **Settings**. They are not fields in `config.yaml`. Secret values use an encrypted secret store backed by `<home>/data/master.key`; filesystem permissions, database backups, and key backup are part of the security boundary.

For headless injection, Kandev can also read agent credentials from the process environment by their required name (for example `ANTHROPIC_API_KEY`) or the `KANDEV_`-prefixed form. `KANDEV_CREDENTIALS_FILE` adds a fallback JSON provider:

```json
{
  "ANTHROPIC_API_KEY": "replace-at-deployment-time",
  "OPENAI_API_KEY": "replace-at-deployment-time"
}
```

The file is loaded lazily, expects a flat string-to-string object, and is cached; restart after changing it. A missing file behaves as no file credentials, while unreadable or invalid JSON produces credential-resolution errors. The database secret store and environment providers are consulted before this file. Restrict file permissions to the Kandev service account and never commit it.

Profile environment variables are eventually injected into agent subprocesses or remote executor environments. Anyone who can edit a profile, inspect a remote host/container, enable debug frame logs, or run commands as the Kandev account may be able to access them. Use least-privilege, task-scoped credentials and rotate them after exposure.

### YAML secrets and file permissions

Stable startup secrets can be stored in `config.yaml`, including
`database.password`, `nats.url`, `auth.jwtSecret`, `office.jwtSigningKey`, and
`observability.otlpEndpoint`. Kandev keeps secret values in typed
configuration, never writes those values to logs, and warns when a selected
secret-bearing file is readable by group or other users on Unix. The warning
does not block startup. Use owner-only mode `0600` and prefer the deployment's
secret injection mechanism when one is available. Do not commit secret values
to a repository.

## Advanced operator tuning

These settings are startup-only unless stated otherwise. Their YAML keys are
the canonical file form; the environment aliases remain compatibility
overrides.

| YAML key | Environment variable | Default | Parsing and effect |
|---|---|---:|---|
| `limits.ghMaxConcurrent` | `KANDEV_GH_MAX_CONCURRENT` | `8` | Positive integer process-wide cap for `gh` subprocesses; invalid/non-positive uses default. |
| `limits.gitMaxConcurrent` | `KANDEV_GIT_MAX_CONCURRENT` | `12` | Positive integer process-wide cap for `git` subprocesses; invalid/non-positive uses default. |
| `limits.lspMaxConnections` | `KANDEV_LSP_MAX_CONNECTIONS` | `8` | Positive integer cap for active browser-to-task-host language-server streams; invalid/non-positive uses default. |
| `messageQueue.maxPerSession` | `KANDEV_QUEUE_MAX_PER_SESSION` | `10` | Pending messages per session. A non-negative YAML value locks the saved UI capacity; a valid environment value overrides YAML and locks it; a negative environment value means unlimited; malformed environment input falls through to YAML, the saved setting, or default. |
| `agentctl.idleTimeout` | `KANDEV_ACP_IDLE_TIMEOUT` | `1h` | Go duration after which idle managed agentctl instances are reaped; `0` disables. Invalid uses default. |
| `agentctl.idleReaperInterval` | `KANDEV_ACP_IDLE_REAPER_INTERVAL` | `1m` | Go duration between idle scans. |
| `agentctl.notificationQueueCapacity` | `KANDEV_ACP_NOTIF_QUEUE` | `131072` | Per-connection ACP inbound notification capacity. YAML values outside `1024`-`131072` fail startup; invalid or out-of-range environment values use `131072`. |
| `planning.coalesceWindowMs` | `KANDEV_PLAN_COALESCE_WINDOW_MS` | `300000` | Non-negative milliseconds for same-author plan revision coalescing; invalid/negative uses five minutes. |
| `office.schedulerTickMs` | `KANDEV_OFFICE_SCHEDULER_TICK_MS` | `5000` | Positive integer safety-net interval for queued/retry run claiming. New-run signals are event-driven. |
| `observability.otlpEndpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Enables OTLP/HTTP tracing for backend and agentctl spans; unset uses a no-op tracer. |

Changing concurrency values trades process pressure against throughput and requires a restart. Under **Settings > Task Behavior > Message Queue**, an admin can save an install-wide capacity and independently control manual and automatic merging. The merge switches apply live and persist across restarts. `messageQueue.maxPerSession` resolves as environment, YAML, saved setting, then default; a non-negative YAML value or any valid environment value locks only capacity. A negative environment value means unlimited, while malformed environment input falls through to the lower-precedence source. `0` means unlimited. Lowering the live limit does not prune existing rows; new admissions remain blocked until the pending count drops below the limit, while retries of already accepted work remain eligible. The default-on automatic switch affects only later admissions and never bypasses capacity or sweeps existing rows.

The current OTLP exporter strips an `http://` or `https://` prefix from the configured endpoint and always uses `WithInsecure()`. Treat this as implementation-bound cleartext transport: send it only to a trusted private collector over a protected network, not directly across an untrusted network. The service name is `kandev-agentctl`, and spans can include task/session/execution IDs plus raw agent-event JSON truncated to 8192 characters. That payload can contain prompts, files, and tool data. Use collector-side access controls and retention accordingly.

### ACP debug-log controls

These apply only when `KANDEV_DEBUG_AGENT_MESSAGES=true`:

| Variable | Default |
|---|---|
| `KANDEV_DEBUG_LOG_DIR` | `<home>/logs/acp` |
| `KANDEV_DEBUG_ACP_MAX_FILES` | `200` |
| `KANDEV_DEBUG_ACP_RETENTION_HOURS` | `48` |
| `KANDEV_DEBUG_ACP_MAX_FILE_BYTES` | `8388608` (8 MiB) |

Retention values must be positive integers; invalid/non-positive values use defaults. Directories and files use owner-only `0700`/`0600` modes on Unix. Rotation, age pruning, and file-count pruning bound normal growth, but these files remain highly sensitive and can exist inside a Docker executor rather than on the host.

## Configuration exclusions

The catalog records variables that are not operator startup settings. They have
no public YAML key:

- Internal launcher and process wiring: `KANDEV_INTERNAL_CONFIG_FILE`,
  `KANDEV_INTERNAL_CONFIG_HOME_FILE`,
  `KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG`,
  `KANDEV_LAUNCHER_PARENT_PID`, `KANDEV_BACKEND_PID_FILE`,
  `KANDEV_DESKTOP_NATIVE_NOTIFICATIONS`, `KANDEV_TASK_ID`,
  `KANDEV_SESSION_ID`, and `KANDEV_WORKSPACE_ID`.
- Generated or packaging values: `KANDEV_DESKTOP_HEALTH_TOKEN`,
  `KANDEV_BUNDLE_DIR`, and `KANDEV_WEB_DIST_DIR`.
- Test and profile selectors: `KANDEV_E2E_MOCK`, `KANDEV_MOCK_AGENT`,
  `KANDEV_MOCK_GITHUB`, `KANDEV_MOCK_GITLAB`, `KANDEV_MOCK_JIRA`,
  `KANDEV_MOCK_LINEAR`, and `AGENTCTL_AUTO_APPROVE_PERMISSIONS`.
- Runtime flags and diagnostics: `KANDEV_FEATURES_OFFICE`,
  `KANDEV_FEATURES_AUTH`,
  `KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF`,
  `KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING`,
  `KANDEV_DEBUG_AGENT_MESSAGES`, `KANDEV_DEBUG_ACP_MAX_FILES`,
  `KANDEV_DEBUG_ACP_RETENTION_HOURS`, `KANDEV_DEBUG_ACP_MAX_FILE_BYTES`,
  `KANDEV_MCP_LOG_FILE`, and `KANDEV_DEBUG_LOG_DIR`.

These values are generated, injected by a supervisor or workspace, selected by
the test/profile system, or intentionally limited to diagnostics. They are
not part of the stable YAML contract.

## Validation and troubleshooting

Startup validation currently enforces:

- `server.port`: `1`-`65535`;
- `database.driver`: `sqlite` or `postgres`;
- PostgreSQL port, non-empty user/database name, and supported SSL mode;
- positive `auth.tokenDuration`;
- logging level/format; and
- positive `repositoryDiscovery.maxDepth`.

Other fields can pass configuration validation and still fail later, for example an unreachable NATS/PostgreSQL/Docker endpoint, unwritable log path, nonsensical timeout, or incompatible pool sizes. A field appearing in the schema does not prove its subsystem is available.

If a value appears ignored:

1. confirm the exact environment spelling, especially camelCase keys;
2. inspect the launcher/service/container environment for an overriding value;
3. confirm the selected `config.yaml` candidate is the first existing file in the working-directory, home, system order;
4. restart the backend; and
5. check whether the field is marked compatibility-only above.

Use `kandev --verbose` to surface startup errors. Do not use `--debug` merely to diagnose a YAML typo on an exposed machine; verbose logs are usually sufficient.

Variables used only to assemble/test the runtime, such as `KANDEV_WEB_DIST_DIR`, `KANDEV_DESKTOP_RUNTIME_DIR`, mock/E2E switches, supervisor socket/manifest values, and bootstrap nonces, are internal implementation contracts, not supported deployment configuration. `KANDEV_MCP_LOG_FILE` remains a debug-only component variable without a YAML key. `KANDEV_BUNDLE_DIR` is the narrow exception documented for installer/package integration in [CLI](./cli.md); end users should still let the installer set it.
