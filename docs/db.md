# Database Configuration

Kandev supports two database backends: **SQLite** (default) and **PostgreSQL**.

> See [`configuration.md`](./configuration.md) for the full backend configuration reference (every YAML key + env var). This page focuses on database-specific settings.

## SQLite (default)

SQLite requires no external services and works out of the box.

### Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `KANDEV_HOME_DIR` | No | `~/.kandev` | Kandev root directory - contains `data/` (DB), `tasks/`, `worktrees/`, `repos/`, `sessions/`, and `lsp-servers/` |
| `KANDEV_DATABASE_DRIVER` | No | `sqlite` | Database driver |
| `KANDEV_DATABASE_PATH` | No | `$KANDEV_HOME_DIR/data/kandev.db` | Path to the SQLite database file (override) |

### Config file (`config.yaml`)

```yaml
homeDir: ""  # empty = resolve from KANDEV_HOME_DIR or ~/.kandev
database:
  driver: sqlite
  path: ""   # empty = $homeDir/data/kandev.db
```

No additional setup is needed - the database file is created automatically on first run.

## PostgreSQL

### 1. Create the database

```sql
CREATE USER kandev WITH PASSWORD 'your-password';
CREATE DATABASE kandev OWNER kandev;
```

### 2. Configure Kandev

#### Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `KANDEV_DATABASE_DRIVER` | Yes | `sqlite` | Set to `postgres` |
| `KANDEV_DATABASE_HOST` | No | `localhost` | PostgreSQL host |
| `KANDEV_DATABASE_PORT` | No | `5432` | PostgreSQL port |
| `KANDEV_DATABASE_USER` | Yes | `kandev` | Database user |
| `KANDEV_DATABASE_PASSWORD` | Usually | (empty) | Database password - required unless your Postgres allows passwordless auth |
| `KANDEV_DATABASE_DBNAME` | Yes | `kandev` | Database name |
| `KANDEV_DATABASE_SSLMODE` | No | `disable` | SSL mode (`disable`, `require`, `verify-ca`, `verify-full`) |
| `KANDEV_DATABASE_MAXCONNS` | No | `25` | Maximum open connections |
| `KANDEV_DATABASE_MINCONNS` | No | `5` | Minimum idle connections |

#### Config file (`config.yaml`)

```yaml
database:
  driver: postgres
  host: localhost
  port: 5432
  user: kandev
  password: your-password
  dbName: kandev
  sslMode: disable
  maxConns: 25
  minConns: 5
```

### 3. Run

```bash
KANDEV_DATABASE_DRIVER=postgres \
KANDEV_DATABASE_PASSWORD=your-password \
kandev
```

Tables are created automatically on first run - no manual migrations needed.
