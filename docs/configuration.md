# Configuration

Kandev's backend reads configuration from three sources, in this order of precedence (later sources override earlier ones):

1. Built-in defaults (`apps/backend/internal/common/config/config.go`).
2. A YAML config file (`config.yaml`).
3. Environment variables (`KANDEV_*`).

Both the file and env vars are optional; the backend boots with sensible defaults out of the box. See [`docker.md`](./docker.md) and [`k8s.md`](./k8s.md) for deployment-specific tables; this page is the full reference.

## Config file

The backend looks for `config.yaml` in, in order:

- An explicit path passed at startup (used in tests).
- The current working directory.
- `/etc/kandev/`.

A missing file is not an error - defaults plus env vars take over.

## Environment variables

All env vars use the `KANDEV_` prefix. Nested keys map by replacing `.` with `_` and uppercasing; **camelCase becomes one uppercase run** (no underscore inserted), because viper does not synthesize a snake_case form.

| YAML key | Env var |
|---|---|
| `server.port` | `KANDEV_SERVER_PORT` |
| `server.webInternalUrl` | `KANDEV_SERVER_WEBINTERNALURL` (or alias `KANDEV_WEB_INTERNAL_URL`) |
| `database.dbName` | `KANDEV_DATABASE_DBNAME` |
| `logging.maxSizeMb` | `KANDEV_LOGGING_MAXSIZEMB` |
| `homeDir` | `KANDEV_HOME_DIR` (alias) |
| `logging.level` | `KANDEV_LOG_LEVEL` (alias) |

The aliases on the right are explicit bindings - see `LoadWithPath` in `config.go` for the full list. New keys should follow the deterministic rule (`KANDEV_<SECTION>_<KEYUPPERCASE>`) unless there is a reason to add an alias.

## Full `config.yaml` example

Every key shown here has a default - copying the whole file changes nothing. Use it as a starting point and delete what you don't need to override.

```yaml
# Kandev root directory. Empty = ~/.kandev (or KANDEV_HOME_DIR if set).
# All workspace artifacts (data, tasks, worktrees, repos, sessions) live here.
homeDir: ""

server:
  host: "0.0.0.0"
  port: 38429              # API + WebSocket + Web UI
  readTimeout: 30          # seconds
  writeTimeout: 30         # seconds
  webInternalUrl: ""       # internal URL the backend uses to call the web app

database:
  driver: "sqlite"         # "sqlite" or "postgres"
  path: ""                 # sqlite: empty = $homeDir/data/kandev.db

  # postgres-only fields below (ignored when driver=sqlite)
  host: "localhost"
  port: 5432
  user: "kandev"           # required when driver=postgres
  password: ""             # required when driver=postgres in most setups
  dbName: "kandev"         # required when driver=postgres
  sslMode: "disable"       # disable | require | verify-ca | verify-full
  maxConns: 25
  minConns: 5

nats:
  url: ""                  # empty = use in-memory event bus
  clusterId: "kandev-cluster"
  clientId: "kandev-client"
  maxReconnects: 10

events:
  namespace: ""            # empty = derive from runtime data identity

docker:
  enabled: true            # disables Docker-based executors when false
  host: ""                 # empty = platform default (unix:///var/run/docker.sock, etc.)
  apiVersion: ""           # empty = auto-negotiate
  tlsVerify: false
  defaultNetwork: "kandev-network"
  volumeBasePath: ""       # empty = /var/lib/kandev/volumes (Linux/macOS)

agent:
  standaloneHost: "localhost"
  standalonePort: 39429    # agentctl control port

auth:
  jwtSecret: ""            # empty = auto-generate a random dev secret on boot
  tokenDuration: 3600      # seconds

logging:
  level: "info"            # debug | info | warn | error
  format: "text"           # text | json (auto = json when KUBERNETES_SERVICE_HOST is set)
  outputPath: "stdout"     # stdout | stderr | /path/to/file.log

  # Rotation - only applied when outputPath is a file path.
  # Active log files are created with mode 0600 (owner-only).
  maxSizeMb: 100           # rotate at this size; 0 = lumberjack default (100MB)
  maxBackups: 5            # 0 = unlimited
  maxAgeDays: 30           # 0 = unlimited
  compress: true           # gzip rotated files

repositoryDiscovery:
  roots: []                # absolute paths to scan for local git repos
  maxDepth: 5

worktree:
  enabled: true
  defaultBranch: "main"
  cleanupOnRemove: true
  fetchTimeoutSeconds: 60
  pullTimeoutSeconds: 60

repoClone:
  basePath: ""             # empty = $homeDir/repos

debug:
  pprofEnabled: false      # enables /debug/pprof and /api/v1/debug/memory
```

## Required vs optional

Almost every field has a default and is optional. The exceptions:

| Field | When required | What happens otherwise |
|---|---|---|
| `database.user` | `database.driver=postgres` | Startup fails with `database.user is required for postgres driver` |
| `database.dbName` | `database.driver=postgres` | Startup fails with `database.dbName is required for postgres driver` |
| `database.password` | `database.driver=postgres` in most setups (some Postgres configs allow passwordless local auth) | Connection fails at runtime |
| `auth.jwtSecret` | Never strictly required, but in production set an explicit value | A random secret is generated on boot - tokens become invalid on restart |

Validated value sets (any other value is a startup error):

| Field | Allowed values |
|---|---|
| `server.port` | `1`-`65535` |
| `database.driver` | `sqlite`, `postgres` |
| `database.port` | `1`-`65535` (only validated when `driver=postgres`) |
| `database.sslMode` | `disable`, `require`, `verify-ca`, `verify-full` (only validated when `driver=postgres`) |
| `logging.level` | `debug`, `info`, `warn`, `error` |
| `logging.format` | `json`, `text` |

## Tips

- **Env vars override the file.** Useful for secrets (`KANDEV_DATABASE_PASSWORD`) and per-environment knobs (`KANDEV_LOG_LEVEL`).
- **K8s / Docker:** prefer env vars for everything; skip the YAML file entirely.
- **Local dev:** drop a `config.yaml` next to where you run the backend; viper picks it up from the current working directory.
- **Format auto-detection** (`logging.format`): set `KANDEV_ENV=production` or run inside K8s and you get JSON logs without changing the config.
