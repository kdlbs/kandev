# Database Configuration

Kandev supports two database backends: **SQLite** (default) and **PostgreSQL**.

## SQLite (default)

SQLite requires no external services and works out of the box.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KANDEV_HOME_DIR` | `~/.kandev` | Kandev root directory — contains `data/` (DB), `tasks/`, `worktrees/`, `repos/`, `sessions/`, and `lsp-servers/` |
| `KANDEV_DATABASE_DRIVER` | `sqlite` | Database driver |
| `KANDEV_DATABASE_PATH` | `$KANDEV_HOME_DIR/data/kandev.db` | Path to the SQLite database file (override) |

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

| Variable | Default | Description |
|----------|---------|-------------|
| `KANDEV_DATABASE_DRIVER` | `sqlite` | Set to `postgres` |
| `KANDEV_DATABASE_HOST` | `localhost` | PostgreSQL host |
| `KANDEV_DATABASE_PORT` | `5432` | PostgreSQL port |
| `KANDEV_DATABASE_USER` | `kandev` | Database user |
| `KANDEV_DATABASE_PASSWORD` | (empty) | Database password |
| `KANDEV_DATABASE_DBNAME` | `kandev` | Database name |
| `KANDEV_DATABASE_SSLMODE` | `disable` | SSL mode (`disable`, `require`, `verify-ca`, `verify-full`) |
| `KANDEV_DATABASE_MAXCONNS` | `25` | Maximum open connections |
| `KANDEV_DATABASE_MINCONNS` | `5` | Minimum idle connections |

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
