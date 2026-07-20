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

## Load order and lifecycle

At backend startup, later sources override earlier ones:

1. embedded production-profile values and built-in defaults;
2. the first readable `config.yaml`; and
3. environment variables.

The public launcher has no `--config` option. It searches for a file named exactly `config.yaml` in the backend working directory and then `/etc/kandev/`. A missing file is allowed. Malformed YAML, unmarshal errors, and validated invalid values stop startup.

Configuration is read at process start; there is no file watcher. Restart Kandev after changing YAML or environment variables. The CLI, desktop shell, service manager, Docker, or Kubernetes may set environment variables on the backend, so those values can override a file unexpectedly. Use the process/service/container environment as the final source of truth.

## Environment-variable naming

Viper maps a nested YAML key by replacing `.` with `_`, adding `KANDEV_`, and uppercasing. It does **not** split camelCase words. For example:

```text
database.dbName       -> KANDEV_DATABASE_DBNAME
logging.maxSizeMb     -> KANDEV_LOGGING_MAXSIZEMB
repoClone.basePath    -> KANDEV_REPOCLONE_BASEPATH
```

Some common camelCase keys have explicit compatibility aliases. Use the documented exact names below; snake_case spellings not listed here are not equivalent.

## Complete backend reference

### Root and server

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `homeDir` | `KANDEV_HOME_DIR` | `~/.kandev` | Root for data, tasks, worktrees, cloned repositories, sessions, and logs. A leading `~/` expands. |
| `server.host` | `KANDEV_SERVER_HOST` | `0.0.0.0` | HTTP listen address. Use `127.0.0.1` for local-only access. |
| `server.port` | `KANDEV_SERVER_PORT` | `38429` | UI, HTTP API, WebSocket, and MCP port; must be `1`-`65535`. The launcher normally supplies its selected port. |
| `server.readTimeout` | `KANDEV_SERVER_READTIMEOUT` | `30` | HTTP read timeout in seconds. |
| `server.writeTimeout` | `KANDEV_SERVER_WRITETIMEOUT` | `30` | HTTP write timeout in seconds. |
| `server.webInternalUrl` | `KANDEV_WEB_INTERNAL_URL` | empty | Development reverse-proxy target for a separately running web app. Installed releases normally serve embedded assets. |

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

`database.path` is an advanced override. Persistence honors it, but the current **Settings → System → Database** and **Backups** services derive their displayed path, WAL files, backup source, and restore destination from `<home>/data/kandev.db`. Those UI operations are therefore not reliable for a custom SQLite location. Use operator-managed backup/restore for a custom path and verify the actual database path printed at startup.

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

The Docker socket is effectively root-equivalent on many hosts. Do not publish it or assume `docker.tlsVerify` secures a TCP daemon—it currently does not. Configure TLS through a supported Docker endpoint/environment and validate it independently, or keep the daemon local. See [Docker](./docker.md) and [Executors](./executors.md).

### Core agent service

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `agent.standaloneHost` | `KANDEV_AGENT_STANDALONE_HOST` | `localhost` | Host of the core `agentctl` control server. |
| `agent.standalonePort` | `AGENTCTL_PORT` or `KANDEV_AGENT_STANDALONE_PORT` | `39429` | Preferred control port. The launcher may supply a free fallback. |

The launcher starts `agentctl`, performs a one-time nonce handshake, and supplies the resulting per-launch token internally. Do not persist or proxy its bootstrap/auth state. Agent command, model, environment, permission, and MCP configuration belongs in agent profiles rather than this section.

### Authentication, Office, Plugins, voice, and feature flags

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `auth.jwtSecret` | `KANDEV_AUTH_JWTSECRET` | generated value | Accepted and validated compatibility configuration; the current main HTTP product path does not use it as an authentication boundary. |
| `auth.tokenDuration` | `KANDEV_AUTH_TOKENDURATION` | `3600` | Must be positive, but is not consumed by the current main HTTP product path. |
| `office.jwtSigningKey` | `KANDEV_OFFICE_JWTSIGNINGKEY` | random per start | HMAC key for Office agent-runtime JWTs. Set a stable secret when Office tasks must survive restarts. |
| `voice.openAIApiKey` | `KANDEV_VOICE_OPENAI_API_KEY` | empty | Server-side transcription fallback when browser speech recognition is unavailable. Empty disables the fallback and its endpoint returns unavailable. |
| `features.office` | `KANDEV_FEATURES_OFFICE` | `false` in production | Experimental Office UI, routes, services, and automation. |
| `features.plugins` | `KANDEV_FEATURES_PLUGINS` | `false` in production | Extensible plugin system: install/manage plugins, spawn plugin backends, and load native UI bundles. Loaded plugin code runs with backend privileges. |

Do not infer security from `auth.jwtSecret`: setting it currently does not turn the local server into an authenticated public service. Office's JWT key has a narrower, active purpose. Store both active secrets and third-party API keys in your deployment secret manager; never commit them in `config.yaml`.

The voice fallback sends audio to the configured OpenAI transcription service and incurs that provider's network, data-handling, and billing behavior. Browser-native speech recognition has its own browser/vendor behavior and does not use this server key.

### GitHub credential broker

Managed SSH, Sprites, and remote-container executors redeem task-scoped GitHub credentials through
the Kandev backend. Configure a public broker base URL independently of GitHub App registration so
PAT-only and named GitHub CLI workspaces can use those executors.

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `githubCredentialBroker.publicBaseUrl` | `KANDEV_GITHUB_CREDENTIAL_BROKER_PUBLIC_BASE_URL` | empty | Externally reachable Kandev base URL used for `/api/v1/github/credentials/resolve`. Must be absolute HTTPS, except HTTP is allowed for loopback development. Credentials, query strings, and fragments are rejected. |

When this value is empty, `githubApp.publicBaseUrl` remains a compatibility fallback. Local and
worktree execution can use the backend loopback URL; non-local executors require an externally
reachable HTTPS URL. Before clone or agent startup, managed executors send an unauthenticated
`GET` to the exact resolution route and require `204 No Content`. This readiness response contains
no connection or credential data; credential redemption remains a lease-authenticated `POST`.

### GitHub App

`githubApp` is deployment-owned configuration for workspace App installations and personal GitHub App authorizations. Leave every field empty to disable GitHub App choices while retaining workspace PAT and GitHub CLI authentication. If any field is set, the configuration is treated as enabled. Startup rejects a missing required value, an invalid key, or an unsafe public URL; an invalid App slug is rejected when an installation flow starts.

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `githubApp.appId` | `KANDEV_GITHUB_APP_APP_ID` | `0` | Positive numeric App ID from the GitHub App registration. |
| `githubApp.clientId` | `KANDEV_GITHUB_APP_CLIENT_ID` | empty | GitHub App OAuth client ID used for installation verification and personal authorization. |
| `githubApp.clientSecret` | `KANDEV_GITHUB_APP_CLIENT_SECRET` | empty | OAuth client secret. Treat as a deployment secret. |
| `githubApp.privateKey` | `KANDEV_GITHUB_APP_PRIVATE_KEY` | empty | Inline RSA private key in PEM format. Literal newlines and escaped `\n` sequences are accepted. Mutually exclusive with `privateKeyFile`. |
| `githubApp.privateKeyFile` | `KANDEV_GITHUB_APP_PRIVATE_KEY_FILE` | empty | Path to an RSA private-key PEM file readable by the Kandev service user. A leading `~/` expands. Mutually exclusive with `privateKey`. |
| `githubApp.webhookSecret` | `KANDEV_GITHUB_APP_WEBHOOK_SECRET` | empty | Shared secret configured on the GitHub App webhook, used for HMAC-SHA256 verification. |
| `githubApp.slug` | `KANDEV_GITHUB_APP_SLUG` | empty | URL slug from `https://github.com/apps/<slug>`; lowercase letters, digits, and hyphens only. |
| `githubApp.publicBaseUrl` | `KANDEV_GITHUB_APP_PUBLIC_BASE_URL` | empty | Externally reachable base URL used to build installation, OAuth, and webhook callbacks. It is also the broker URL compatibility fallback. Must be absolute HTTPS, except HTTP is allowed for loopback development. Credentials, query strings, and fragments are rejected. |

The App requires `appId`, `clientId`, `clientSecret`, `webhookSecret`, `slug`, `publicBaseUrl`, and exactly one of `privateKey` or `privateKeyFile`. Kandev accepts PKCS#1 or RSA PKCS#8 PEM keys. It validates the key and public URL at startup without returning secret values through workspace APIs.

Configure the GitHub App registration with these URLs, based on the same `publicBaseUrl`:

| GitHub setting | Path appended to `publicBaseUrl` |
|---|---|
| First user authorization callback URL | `/api/v1/github/app/install/callback` |
| Additional user authorization callback URL | `/api/v1/github/personal-connection/callback` |
| Webhook URL | `/api/v1/github/app/webhook` |

Enable **Request user authorization (OAuth) during installation**. Leave **Setup URL** empty;
GitHub makes it unavailable in this mode. The first callback receives the combined installation
and authorizer result. The additional callback is selected explicitly by Kandev's personal OAuth
flow. Keep expiring user authorization tokens enabled.

The public URL must route those paths to the Kandev backend. For a non-local executor, configure
`githubCredentialBroker.publicBaseUrl` so the task environment can reach
`/api/v1/github/credentials/resolve` over HTTPS. Do not expose a separate unauthenticated copy of
that endpoint; its non-secret `GET` readiness check and lease-authenticated `POST` redemption must
both reach the normal Kandev service boundary.

Prefer mounting the private key as an owner-readable secret file instead of placing multiline PEM content in YAML or a general-purpose environment file. The client and webhook secrets have no `*File` configuration variants; inject their exact environment variables from the deployment secret manager. For example:

```yaml
# config.yaml
githubApp:
  appId: 123456
  clientId: "Iv1.example-client-id"
  privateKeyFile: "/run/secrets/kandev-github-app.pem"
  slug: "company-kandev"
  publicBaseUrl: "https://kandev.example.com"
```

```bash
# Values supplied by the service manager or container secret integration.
KANDEV_GITHUB_APP_CLIENT_SECRET="replace-at-deployment-time"
KANDEV_GITHUB_APP_WEBHOOK_SECRET="replace-at-deployment-time"
```

Mount `/run/secrets/kandev-github-app.pem` read-only, restrict it to the Kandev service account (for example mode `0400` or `0600`), and do not include it in the image or source repository. Follow GitHub's [private-key storage and rotation guidance](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/managing-private-keys-for-github-apps): add a new key, update the mounted file, restart Kandev, verify connections, and then delete the old key. Rotate the client and webhook secrets in GitHub and deployment configuration together. See [Integrations](./integrations.md#host-a-github-app) for required permissions, events, identity behavior, and recovery.

### Logging

| YAML key | Environment variable | Default | Current behavior |
|---|---|---|---|
| `logging.level` | `KANDEV_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. The CLI normally supplies `warn`, `info`, or `debug`. |
| `logging.format` | `KANDEV_LOGGING_FORMAT` | `text`, or `json` in production/Kubernetes | `text` or `json`; `auto` is not accepted. |
| `logging.outputPath` | `KANDEV_LOGGING_OUTPUTPATH` | `stdout` | `stdout`, `stderr`, or a file path. |
| `logging.maxSizeMb` | `KANDEV_LOGGING_MAXSIZEMB` | `100` | File-output rotation threshold. Zero uses lumberjack's 100 MiB default. |
| `logging.maxBackups` | `KANDEV_LOGGING_MAXBACKUPS` | `5` | Rotated-file count; zero means unlimited. |
| `logging.maxAgeDays` | `KANDEV_LOGGING_MAXAGEDAYS` | `30` | Rotated-file age; zero means unlimited. |
| `logging.compress` | `KANDEV_LOGGING_COMPRESS` | `true` | Gzip rotated files. |

The format default becomes JSON when `KUBERNETES_SERVICE_HOST` is non-empty or `KANDEV_ENV` is exactly `production`/`prod`; otherwise it is text. Rotation settings apply only to file output. Active log files are created owner-only (`0600`) on Unix, so a log shipper running as another user cannot read them without an explicit permission design.

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
| `debug.pprofEnabled` | `KANDEV_DEBUG_PPROF_ENABLED` | `false` | Legacy alias; also enables debug mode. |

Debug mode is high risk. It enables local diagnostic surfaces and implies `KANDEV_DEBUG_AGENT_MESSAGES=true` and `KANDEV_DEBUG_PPROF_ENABLED=true` when not explicitly locked by the environment. ACP JSONL frames include complete prompts, file content, and tool calls. Do not enable it on a shared or network-exposed backend.

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
  outputPath: "/var/log/kandev/backend.log"

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
  webInternalUrl: ""

githubCredentialBroker:
  publicBaseUrl: ""

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

auth:
  jwtSecret: ""       # compatibility-only for the main HTTP product path
  tokenDuration: 3600 # compatibility-only for the main HTTP product path

logging:
  level: "info"
  format: "text"
  outputPath: "stdout"
  maxSizeMb: 100
  maxBackups: 5
  maxAgeDays: 30
  compress: true

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

voice:
  openAIApiKey: ""

features:
  office: false
  plugins: false

githubApp:
  appId: 0
  clientId: ""
  clientSecret: ""
  privateKey: ""
  privateKeyFile: ""
  webhookSecret: ""
  slug: ""
  publicBaseUrl: ""
```

Copying this entire file is unnecessary and can freeze old defaults in a deployment. Keep only deliberate overrides. On Windows, do not copy the Unix Docker host/path literals from this example.

## Runtime feature toggles

**Settings → System → Feature Toggles** manages three startup-time flags:

| Key | Environment lock | Production default | Effect |
|---|---|---|---|
| `features.office` | `KANDEV_FEATURES_OFFICE` | off | Experimental autonomous-agent Office surfaces and automation. |
| `features.plugins` | `KANDEV_FEATURES_PLUGINS` | off | Extensible plugin system: install/manage plugins and load their backends and native UI bundles. |
| `debug.devMode` | `KANDEV_DEBUG_DEV_MODE` (also locked by explicit legacy/debug-message vars) | off | High-risk diagnostic endpoints and ACP frame logging. |

UI changes are persisted in the database and require a restart. An explicitly set environment value wins and locks the UI control. Otherwise a database override wins over the embedded profile/default. Resetting a toggle removes its database override.

The source checkout's `make dev` activates the embedded development profile, which enables Office, debug surfaces, ACP logging, and a mock agent. Installed `run`/desktop builds select the safe production profile unless the environment explicitly opts in. E2E mock variables and routes are test-only and must never be enabled on a public deployment.

## Credentials and product settings

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

## Advanced operator tuning

These startup-only variables are supported by specific runtime components but are not YAML fields. Leave them at defaults unless diagnosing a measured problem.

| Variable | Default | Parsing and effect |
|---|---:|---|
| `KANDEV_GH_MAX_CONCURRENT` | `8` | Positive integer process-wide cap for `gh` subprocesses; invalid/non-positive uses default. |
| `KANDEV_GIT_MAX_CONCURRENT` | `12` | Positive integer process-wide cap for `git` subprocesses; invalid/non-positive uses default. |
| `KANDEV_QUEUE_MAX_PER_SESSION` | `10` | Queued user messages per session. Invalid uses default; zero/negative disables the cap. |
| `KANDEV_ACP_IDLE_TIMEOUT` | `1h` | Go duration after which idle agentctl instances are reaped; `0` disables. Invalid uses default. |
| `KANDEV_ACP_IDLE_REAPER_INTERVAL` | `1m` | Go duration between idle scans. Intended mainly for testing; use the default in production. |
| `KANDEV_ACP_NOTIF_QUEUE` | `131072` | Per-connection ACP inbound notification capacity; positive values clamp to `1024`-`131072`, invalid uses default. |
| `KANDEV_PLAN_COALESCE_WINDOW_MS` | `300000` | Non-negative milliseconds for same-author plan revision coalescing; invalid/negative uses five minutes. |
| `KANDEV_OFFICE_SCHEDULER_TICK_MS` | `5000` | Positive integer safety-net interval for queued/retry run claiming. New-run signals are event-driven; invalid/non-positive uses five seconds. |
| `KANDEV_MCP_LOG_FILE` | unset | File path for per-agentctl MCP debug logs. Logs tool names, arguments, session IDs, results for tool errors, and timings; invalid paths warn and disable this sink. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Enables OTLP/HTTP tracing for backend and agentctl spans; unset uses a no-op tracer. See the transport warning below. |

Changing concurrency/queue values trades memory and process pressure against throughput. Values are captured during process/component initialization and need a restart.

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

## Validation and troubleshooting

Startup validation currently enforces:

- `server.port`: `1`-`65535`;
- `database.driver`: `sqlite` or `postgres`;
- PostgreSQL port, non-empty user/database name, and supported SSL mode;
- positive `auth.tokenDuration`;
- logging level/format; and
- positive `repositoryDiscovery.maxDepth`; and
- all-or-none GitHub App configuration, a valid RSA private key, and a safe public base URL when `githubApp` is enabled.

Other fields can pass configuration validation and still fail later—for example an unreachable NATS/PostgreSQL/Docker endpoint, unwritable log path, nonsensical timeout, or incompatible pool sizes. A field appearing in the schema does not prove its subsystem is available.

If a value appears ignored:

1. confirm the exact environment spelling, especially camelCase keys;
2. inspect the launcher/service/container environment for an overriding value;
3. confirm `config.yaml` is in the backend working directory or `/etc/kandev/`;
4. restart the backend; and
5. check whether the field is marked compatibility-only above.

Use `kandev --verbose` to surface startup errors. Do not use `--debug` merely to diagnose a YAML typo on an exposed machine; verbose logs are usually sufficient.

Variables used only to assemble/test the runtime—such as `KANDEV_WEB_DIST_DIR`, `KANDEV_DESKTOP_RUNTIME_DIR`, mock/E2E switches, supervisor socket/manifest values, and bootstrap nonces—are internal implementation contracts, not supported deployment configuration. `KANDEV_BUNDLE_DIR` is the narrow exception documented for installer/package integration in [CLI](./cli.md); end users should still let the installer set it.
