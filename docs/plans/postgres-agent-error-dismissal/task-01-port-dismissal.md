---
id: "01-port-dismissal"
title: "Port the guarded session agent-error dismissal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-AGENT-ERROR-DISMISSAL-001
acceptance_criteria:
  - AC-TASKS-AGENT-ERROR-DISMISSAL-001.1
  - AC-TASKS-AGENT-ERROR-DISMISSAL-001.2
  - AC-TASKS-AGENT-ERROR-DISMISSAL-001.3
  - AC-TASKS-AGENT-ERROR-DISMISSAL-001.4
  - AC-TASKS-AGENT-ERROR-DISMISSAL-001.5
system_design:
  - ../../specs/tasks/system-design/agent-error-dismissal.md
---

# Task 01: Port the Guarded Session Agent-Error Dismissal

## Summary

Make the existing repository acknowledgment work on PostgreSQL while retaining
SQLite's message/time guards. Add independently written real-engine regression
coverage and preserve all existing caller and presentation contracts.

## In scope

- `Repository.DismissLastAgentError` query assembly and its existing payload,
  timestamp, boolean-return, and error behavior.
- Remove only the corrected method's obsolete exact SQL-guard exemption.
- A shared semantic case matrix with SQLite and env-gated PostgreSQL entry
  tests, actual metadata writes, and repository/model readback.
- Updated delivery results and spec lifecycle only after verified execution.

## Out of scope

- UI, translations, HTTP/service fixes, ACP, launch/recovery, schema and flags.
- New stamp, first-write, idempotency, or malformed-metadata repair semantics.
- Broad porting, shared helper redesign, unrelated tests, and private ROOT
  proof-source access or harness replay.

## Acceptance

1. The PostgreSQL regression fails on the real dismissal before the fix and
   passes afterward; both engines pass the plan's complete semantic matrix
   through `LoadLastAgentError` / `IsDismissed` readback
   (AC-TASKS-AGENT-ERROR-DISMISSAL-001.1 through .3).
2. Only the existing conditional operation changes. Message-only and time-only
   replacement tests prove independent guards; payload retention, no-op
   `updated_at`, and 1ms separation prevent weakened identity. The unchanged
   authorization, API, notification and notice paths remain source-confirmed
   (AC-TASKS-AGENT-ERROR-DISMISSAL-001.4 and .5).
3. Focused behavioral checks, scoped lint, SQL guard, and document checks pass;
   every native handle is joined and the disposable fixture is removed with
   recorded current-owned absence. Retain the one incomplete local package
   attempt. Complete owning-package validation remains mandatory in the hosted
   normal and PostgreSQL repository cohorts and their Backend parent.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/db/sqlguard/exemptions.json` (remove only this method's obsolete exemption)
- `apps/backend/internal/task/repository/sqlite/session_agent_error_dismissal_test.go`
- `apps/backend/internal/task/repository/sqlite/session_agent_error_dismissal_postgres_test.go`
- `docs/specs/tasks/requirements/agent-error-dismissal.md`
- `docs/specs/tasks/system-design/agent-error-dismissal.md`
- `docs/plans/postgres-agent-error-dismissal/plan.md`
- `docs/plans/postgres-agent-error-dismissal/task-01-port-dismissal.md`

## Dependencies

None. Read the same [owning requirement](../../specs/tasks/requirements/agent-error-dismissal.md),
[design](../../specs/tasks/system-design/agent-error-dismissal.md), and
[plan](plan.md) before execution. The current phase authorizes design only;
later implementation and resource admission arrive separately in this session.
Preserve other contributors' files and managed dependencies/caches.

## Implementation sequence

1. After release, mark this order `in_progress`. Independently write the
   planned semantic matrix in the two new test files. Reuse
   `newRepoForSessionTests` for SQLite and `testutil.OpenIsolatedPostgres` with
   `PostgresDSNFromEnv` for PostgreSQL; keep fixtures serial and uniquely scoped.
   Use actual `CreateTask` / `CreateTaskSession` and metadata writers for normal
   fixtures; direct SQL is limited to historical timestamp/null representation
   setup and raw unchanged-row assertions.
2. Run the focused command below for RED with the owned DSN. Its PostgreSQL
   matching-error case must fail on the actual SQL operation, with SQLite
   controls passing. A toolchain, fixture, timeout, or connection failure is
   not causal RED. Retain its log and actual exit.
3. Apply the design's bound JSON setter and dialect-specific occurrence
   comparison. Keep the equality checks inside the same update. Run the same
   focused command once for GREEN, then the required scoped checks. The single
   local owning-package attempt below has already exhausted its budget; do not
   repeat it. Complete that inventory through the required hosted cohorts.
4. Record results and owned-resource cleanup. Mark `done` and the plan
   `implemented` only when all listed gates pass. Promote the paired requirement
   to `active` and design to `current` only when implemented behavior conforms.

## Disposable PostgreSQL fixture

Execute only after resource admission. Use an individually owned PostgreSQL 16
container, never an existing database supplied by the shell. The synthetic
credential below belongs only to this disposable fixture. Keep the returned
container ID and evidence directory across command calls and interruptions.
Do not use anonymous/named volumes or a host data directory.

From the repository root, run fixture setup in Bash:

```bash
set -euo pipefail
dismiss_owner=53d87ce0-9cc4-47dc-9915-5f02fb3cb88e
dismiss_evidence=$(mktemp -d /tmp/kandev-agent-error-dismissal-53d87ce0.XXXXXXXX)
chmod 700 "$dismiss_evidence"
dismiss_name="kandev-dismiss-53d87ce0-${dismiss_evidence##*.}"
printf '%s\n' "$dismiss_name" > "$dismiss_evidence/container.name"
timeout --kill-after=10s 2m docker run -d \
  --name "$dismiss_name" --label "kandev.task.owner=$dismiss_owner" \
  --label "kandev.test.fixture=$dismiss_name" \
  --memory=384m --cpus=1 --pids-limit=128 \
  --tmpfs /var/lib/postgresql/data:rw,size=256m \
  -p 127.0.0.1::5432 \
  -e POSTGRES_USER=dismiss_test -e POSTGRES_PASSWORD=dismiss_test_only \
  -e POSTGRES_DB=dismiss_test postgres:16 > "$dismiss_evidence/container.id"
dismiss_cid=$(cat "$dismiss_evidence/container.id")
docker inspect "$dismiss_cid" > "$dismiss_evidence/container.inspect.json"
python3 - "$dismiss_evidence/container.inspect.json" "$dismiss_owner" <<'PY'
import json, sys
c = json.load(open(sys.argv[1]))[0]
assert c['Config']['Labels']['kandev.task.owner'] == sys.argv[2]
assert not any(m['Type'] == 'volume' for m in c['Mounts'])
assert '/var/lib/postgresql/data' in c['HostConfig']['Tmpfs']
assert all(p['HostIp'] == '127.0.0.1'
           for ports in c['NetworkSettings']['Ports'].values() if ports for p in ports)
PY
for dismiss_attempt in $(seq 1 30); do
  if timeout --kill-after=2s 3s docker exec "$dismiss_cid" \
    pg_isready -h 127.0.0.1 -U dismiss_test -d dismiss_test; then break; fi
  if [ "$dismiss_attempt" = 30 ]; then exit 1; fi
  sleep 1
done
dismiss_port=$(docker inspect --format \
  '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' "$dismiss_cid")
export KANDEV_TEST_POSTGRES_DSN="postgres://dismiss_test:dismiss_test_only@127.0.0.1:$dismiss_port/dismiss_test?sslmode=disable"
export KANDEV_DISMISS_EVIDENCE="$dismiss_evidence"
printf 'export KANDEV_TEST_POSTGRES_DSN=%q\nexport KANDEV_DISMISS_EVIDENCE=%q\n' \
  "$KANDEV_TEST_POSTGRES_DSN" "$KANDEV_DISMISS_EVIDENCE" > "$dismiss_evidence/fixture.env"
```

Each test uses a unique schema from `OpenIsolatedPostgres`, whose cleanup drops
only that schema and closes its pool. Do not start this setup again because a
response was interrupted: inspect the retained name/ID and original handle
first. Bound Docker control calls and collect logs on readiness failure. Even
when setup or tests fail, the owning session must join its commands and clean
only the labeled container ID it created; do not proceed to tests after a
fixture assertion fails.

## Verification

All commands start from the repository root. Source the retained task-owned
`fixture.env` in each new shell before a DSN-dependent command. Do not infer
success from an unjoined session. Save each native session ID, process/group
identity when available, start/deadline, log, final join and exit in the evidence
directory. No background command is abandoned across interruptions.

Focused RED and then GREEN use this exact command, each with a fresh unique
run-quiet directory:

```bash
export KANDEV_RUN_QUIET_DIR=$(mktemp -d "$KANDEV_DISMISS_EVIDENCE/focused.XXXXXXXX")
scripts/run-quiet backend-test -- timeout --kill-after=10s 6m \
  env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB \
  bash -c 'cd apps/backend && exec /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=5m -run "^TestDismissLastAgentError" -v ./internal/task/repository/sqlite'
```

The following command was run once after GREEN with PostgreSQL gating unset.
It timed out after ten minutes and is retained as an incomplete historical
attempt, not a passing gate. Do not repeat or extend this local package run:

```bash
export KANDEV_RUN_QUIET_DIR=$(mktemp -d "$KANDEV_DISMISS_EVIDENCE/package.XXXXXXXX")
scripts/run-quiet backend-test -- timeout --kill-after=10s 11m \
  env -u KANDEV_TEST_POSTGRES_DSN GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB \
  bash -c 'cd apps/backend && exec /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=10m ./internal/task/repository/sqlite'
```

The existing `backend-tests.yml` deliberately excludes this large package from
ordinary Go-test shards. Its full native inventory is mandatory in Backend
Repository 1/2/3 and PostgreSQL repository 1/2/3 cohorts with complete-result
validation. All current cohorts and the required Backend parent must succeed
before delivery is considered ready. The local behavioral gate is the focused
actual-engine matrix above; the local timeout does not establish a package
verdict or justify changing unrelated fixtures or schema code.

Run formatting on actual changed Go files, then lint their complete owning
packages through the repository helper. The three paths below are the planned
actual change set; reconcile it with `git diff` plus untracked Go paths before
running, including any justified helper extraction and no unrelated files.

```bash
/home/jcfs/.local/share/mise/installs/go/1.26.0/bin/gofmt -w \
  apps/backend/internal/task/repository/sqlite/session.go \
  apps/backend/internal/task/repository/sqlite/session_agent_error_dismissal_test.go \
  apps/backend/internal/task/repository/sqlite/session_agent_error_dismissal_postgres_test.go
export KANDEV_RUN_QUIET_DIR=$(mktemp -d "$KANDEV_DISMISS_EVIDENCE/lint.XXXXXXXX")
scripts/run-quiet backend-lint -- timeout --kill-after=10s 6m \
  env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" \
  GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=3GiB \
  bash scripts/lint-go-changed \
  apps/backend/internal/task/repository/sqlite/session.go \
  apps/backend/internal/task/repository/sqlite/session_agent_error_dismissal_test.go \
  apps/backend/internal/task/repository/sqlite/session_agent_error_dismissal_postgres_test.go
export KANDEV_RUN_QUIET_DIR=$(mktemp -d "$KANDEV_DISMISS_EVIDENCE/sqlguard.XXXXXXXX")
scripts/run-quiet backend-sqlguard -- timeout --kill-after=10s 6m \
  env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB \
  bash -c 'cd apps/backend && exec /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go run -trimpath -tags=fts5 -p=1 ./cmd/sqlguard ./internal'
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Before commit, run the lightweight documentation-coverage preflight below.
It includes untracked files and validates the linked four documents without
running the Node test suite:

```bash
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'JS'
const fs = require('node:fs');
const {execFileSync} = require('node:child_process');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/tasks/requirements/agent-error-dismissal.md',
  'docs/specs/tasks/system-design/agent-error-dismissal.md',
  'docs/plans/postgres-agent-error-dismissal/plan.md',
  'docs/plans/postgres-agent-error-dismissal/task-01-port-dismissal.md',
];
const paths = new Set([
  ...execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], {encoding:'utf8'}).split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding:'utf8'}).split('\0'),
].filter(Boolean));
const changedFiles = [...paths].map(filename => ({filename, status:'modified'}));
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
JS
```

CI owns broad shared-store, backend, frontend, and browser checks. Existing
service tests are unchanged compatibility evidence, not an additional full
local service-package gate.

## Owned cleanup

After all fixture-consuming handles have actually joined, reload the retained
container ID, compare both owner and unique fixture labels against the retained
inspect evidence, and remove that ID with bounded `docker rm -f`. Save the
removal exit and subsequent `docker inspect` not-found result. If removal fails,
retain the identity and report the failure; never run machine-wide cleanup or
kill foreign processes. Confirm current-owned command groups are absent and
record any remaining owned resource explicitly. Keep the evidence, checkout,
dependencies, caches, and private ROOT resources intact.

## Risks

- Wrong nested JSON extraction, placeholder order, or string-valued JSON writes.
- Timestamp normalization that truncates subsecond identity or ignores offsets.
- Unscoped full-package PostgreSQL runs and unjoined handles consuming resources.
- Treating successful SQL execution as sufficient without actual model readback.

## Parallelism

`sequential`

## Inputs

- The paired requirement/design, plan coverage matrix, and backend `AGENTS.md`.
- `metadata_launch_error_cas.go`, `dialect/json.go`, and `dialect/time.go`.
- Existing session dismissal tests and PostgreSQL metadata writer tests.
- Unchanged HTTP/service/model/notice source and the five allowed ROOT receipts.

## Results

Local implementation complete; hosted delivery validation remains mandatory.
Frozen workspace dependency installation passed once.
Independently authored regressions reached a clean causal RED with PostgreSQL
SQLSTATE 42883 and all twelve SQLite cases plus two existing controls passing.
Earlier test iterations corrected nil-versus-empty modeled collections and
SQL-NULL fixture readback; they are retained separately from the clean RED.
The minimum one-method correction then passed focused GREEN: four top-level
tests and 24 new cross-dialect subtests, including real model readback.

Fixture setup initially asserted tmpfs under Docker's `Mounts`; the existing
owned container was verified using `HostConfig.Tmpfs` and reused without a
second container. The documented assertion now matches Docker's actual
inspection shape. No anonymous volumes or foreign resources were used.

The single complete owning repository package ran with the PostgreSQL DSN
unset and hit Go's ten-minute alarm (exit 1, 600.153s). Its terminal stack was
initializing a fixture in `TestArchiveTaskPurgesDurableQueueRecovery`, reported
as running for zero seconds, through `NewWithDB` / `initTaskTransferSchema`.
No dismissal assertion failure was logged. This is an incomplete package run,
not passing evidence or a demonstrated baseline failure. The original native
handle was joined and both observed process groups were confirmed absent.
No package rerun or resource increase was attempted.

Scoped changed-Go lint passed. The first full SQL guard scan rejected the now
unused exact exemption for `DismissLastAgentError`; removing only that record
made the corrected scan pass, without changing guard policy or Go behavior.

The supervising session reviewed the timeout and existing CI inventory split
and authorized complete owning-package validation in the required hosted
normal/PostgreSQL repository cohorts and Backend parent. The local timeout
remains incomplete; there is no local broad rerun or resource extension.
Final catalog validation, specification lint, actual eight-path documentation
coverage, links, sizes, shell syntax, formatting and whitespace checks passed.
The labeled PostgreSQL container was removed with exit 0; a fresh not-found
inspection confirmed absence. No owned schemas, anonymous volumes, or recorded
local command groups remained. The post-removal assertion was corrected for
the Docker client's lowercase not-found text; removal was not repeated.

Normal active commit hooks remain a delivery gate, with their actual receipt
stored outside tracked documents. Required hosted normal and PostgreSQL
repository cohorts plus the current Backend parent are still pending; this
local completion record does not claim those checks passed.
