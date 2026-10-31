#!/usr/bin/env bash
set -euo pipefail

# Execute only after parent-owned CI/browser/build jobs quiesce and the source is frozen.
readonly REPO=/tmp/usage-history-C10-benchmark-worktree
readonly EXPECTED_HEAD=1ce8d0bfdab088f899cf0ede7a5a911d097e0cee
readonly BACKEND="$REPO/apps/backend"
readonly GO_BIN=/root/.local/share/mise/installs/go/1.26.0/bin/go
readonly EVIDENCE_ROOT="$REPO/docs/plans/usage-history/evidence/usage-history-luna-C10-full-normal-20261031T060630Z"
readonly LOG_ROOT=/tmp/usage-history-luna-C10-full-normal-20261031T060630Z
readonly PG_ENV_FILE=/tmp/usage-history-pr3660-postgres.env

readonly EVIDENCE_REL=docs/plans/usage-history/evidence/usage-history-luna-C10-full-normal-20261031T060630Z

assert_source_frame() {
  local head status_line
  head="$(git -C "$REPO" rev-parse HEAD)"
  if [[ "$head" != "$EXPECTED_HEAD" ]]; then
    printf 'Refusing run: source HEAD changed (expected %s).\n' "$EXPECTED_HEAD" >&2
    return 1
  fi
  if ! git -C "$REPO" diff --quiet || ! git -C "$REPO" diff --cached --quiet; then
    printf 'Refusing run: tracked source changed during the gate.\n' >&2
    return 1
  fi
  while IFS= read -r status_line; do
    [[ -z "$status_line" ]] && continue
    if [[ "$status_line" == "?? $EVIDENCE_REL/"* ]]; then
      continue
    fi
    printf 'Refusing run: unexpected worktree change outside benchmark evidence.\n' >&2
    return 1
  done < <(git -C "$REPO" status --porcelain)
}

assert_source_frame
actual_go="$("$GO_BIN" version)"
if [[ "$actual_go" != 'go version go1.26.0 linux/amd64' ]]; then
  printf 'Refusing run: unexpected Go toolchain: %s\n' "$actual_go" >&2
  exit 1
fi

export CGO_ENABLED=1
export CGO_CFLAGS='-O2 -g -DSQLITE_ENABLE_STMT_SCANSTATUS'
export GOMAXPROCS=2
export GOMEMLIMIT=3GiB
export GOFLAGS=''
unset KANDEV_USAGE_BENCH_TRACE_SQLITE KANDEV_USAGE_BENCH_TRACE_POSTGRES
unset KANDEV_USAGE_HISTORY_BENCH_CHILD KANDEV_TEST_POSTGRES_DSN
mkdir -p "$LOG_ROOT"

run_benchmark() {
  local engine="$1" benchmark="$2" evidence_dir="$3" log="$4"
  if (
    cd "$BACKEND"
    export KANDEV_USAGE_BENCH_EVIDENCE_DIR="$evidence_dir"
    exec "$GO_BIN" test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' \
      ./internal/analytics/repository/sqlite -run '^$' -bench "^${benchmark}$" \
      -benchtime=10x -benchmem -count=1 -timeout=0
  ) >"$log" 2>&1; then
    printf '%s gate command completed. Log: %s\n' "$engine" "$log"
    tail -n 55 "$log"
  else
    local status=$?
    printf '%s gate command failed (status %s). Tail of %s:\n' "$engine" "$status" "$log" >&2
    tail -n 140 "$log" >&2
    return "$status"
  fi
}

validate_evidence() {
  python3 - "$1" "$2" <<'PY_VALIDATE'
import json
import sys
from pathlib import Path

engine, root_text = sys.argv[1:]
root = Path(root_text)
prefix = f'usage-history-worker4-bench-{engine}-'
scenarios = [
    'recent-UTC', 'five_years-UTC',
    'recent-America-New_York', 'five_years-America-New_York',
    'recent-Asia-Kathmandu', 'five_years-Asia-Kathmandu',
    'recent-Europe-Lisbon', 'five_years-Europe-Lisbon',
    'lifetime', 'native', 'mixed_actors', 'foreign_coarse',
    'deleted_task', 'dst_new_york', 'foreign_coarse_boundary', 'empty_range',
]
expected_count = 0
for phase, native, external in (
    ('one_year', 200005, 20013),
    ('five_years', 1000005, 100013),
):
    for scenario in scenarios:
        path = root / f'{prefix}{phase}-{scenario}.json'
        if not path.is_file():
            raise SystemExit(f'missing required evidence: {path}')
        item = json.loads(path.read_text())
        expected_count += 1
        assert item['engine'] == engine and item['phase'] == phase, path
        assert len(item['latency_samples_ns']) == 10, path
        fixture = item['fixture']
        assert fixture['retained_native_observations'] == native, path
        assert fixture['retained_external_observations'] == external, path
        assert fixture['published_canonical_facts'] >= native, path
        plain = item['isolated_plain_query']
        instrumented = item['isolated_instrumented_query']
        assert plain['publication'] == instrumented['publication'], path
        if engine == 'postgres':
            for query in (plain, instrumented):
                if not query.get('postgres_version', '').startswith('18.6'):
                    raise SystemExit(f'not PostgreSQL 18.6 evidence: {path}')
            assert instrumented['statements'], path
comparison_path = root / f'{prefix}recent-before-after.json'
comparison = json.loads(comparison_path.read_text())
assert len(comparison) == 4, comparison_path
for key, pair in comparison.items():
    assert key.startswith('recent/'), (comparison_path, key)
    assert pair['one_year']['latency_sample_count'] == 10, (comparison_path, key)
    assert pair['five_years']['latency_sample_count'] == 10, (comparison_path, key)
    assert pair['one_year']['selection_sha256'] == pair['five_years']['selection_sha256'], (comparison_path, key)
    assert pair['one_year']['publication'] != pair['five_years']['publication'], (comparison_path, key)
files = list(root.glob(prefix + '*.json'))
if len(files) != 33 or expected_count != 32:
    raise SystemExit(f'{engine}: expected 33 evidence JSON files (32 scenario + comparison); found {len(files)}')
print(f'{engine}: verified 32 ten-sample scenario receipts plus 1 recent-before-after comparison; root={root}')
PY_VALIDATE
}

run_benchmark sqlite BenchmarkUsageHistoryFiveYears "$EVIDENCE_ROOT/sqlite" "$LOG_ROOT/sqlite.log"
validate_evidence sqlite "$EVIDENCE_ROOT/sqlite"
assert_source_frame

if [[ ! -r "$PG_ENV_FILE" ]]; then
  printf 'Refusing PostgreSQL gate: DSN env file is unavailable.\n' >&2
  exit 1
fi
. "$PG_ENV_FILE"
export KANDEV_TEST_POSTGRES_DSN
if [[ -z "${KANDEV_TEST_POSTGRES_DSN:-}" ]]; then
  printf 'Refusing PostgreSQL gate: KANDEV_TEST_POSTGRES_DSN is empty.\n' >&2
  exit 1
fi

run_benchmark postgres BenchmarkUsageHistoryPostgresFiveYears "$EVIDENCE_ROOT/postgres" "$LOG_ROOT/postgres.log"
validate_evidence postgres "$EVIDENCE_ROOT/postgres"
assert_source_frame
printf 'Both engine gates completed with evidence under %s\n' "$EVIDENCE_ROOT"
