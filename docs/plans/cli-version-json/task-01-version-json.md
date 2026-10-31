---
id: "01-version-json"
title: "Implement native version JSON"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CLI-VERSION-JSON-001
acceptance_criteria:
  - AC-CLI-VERSION-JSON-001.1
  - AC-CLI-VERSION-JSON-001.2
  - AC-CLI-VERSION-JSON-001.3
  - AC-CLI-VERSION-JSON-001.4
  - AC-CLI-VERSION-JSON-001.5
system_design:
  - ../../specs/cli/system-design/version-json.md
---

# Task 01: Implement native version JSON

## Summary

Provide the early native launcher JSON version output and its documentation,
while preserving plain output and separate maintenance parsing. Demonstrate the
missing entrypoint behavior before production changes and record actual results.

## In scope

- Parser, JSON encoding and write-error handling confined to this version path.
- Real `Run` coverage, helper failure tests, isolated SIGPIPE control, parser
  and help coverage, and a small public CLI reference update.
- Promote the paired draft specs and complete this work order only after all
  specified implementation checks pass and the implementation matches them.

## Out of scope

Changing the npm shim, build stamps, release channels, backend metadata,
startup policy, service or maintenance behavior, or unrelated documentation.

## Acceptance

- All five linked criteria hold through the native launcher; existing plain
  version and maintenance JSON behavior remain intact.
- A real `Run` test fails causally on unsupported `--json` before production
  edits, then passes with focused tests and a stamped native binary smoke test.
- Help and the public CLI reference explain the command and its three fields;
  specified implementation checks have recorded results. Normal hooks are a
  separate delivery gate.

## Verification

Only after ROOT has read all four documents and sent a later separate execution
and global HEAVY release, use this worktree. Run commands from the repository
root with the specified toolchain; do not overlap long commands. Join every
returned native session handle and record its exit and log. Each `run-quiet`
call receives its own external directory; retain hook logs, do not bypass
hooks, and do not run a full suite or clean unrelated resources. Protect all
`/home/jcfs/a.txt` exclusions.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2
(cd apps && KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-version-install.XXXXXX)" ../scripts/run-quiet install -- pnpm install --frozen-lockfile)
```

Install once before pnpm or hooks. First add only `TestRunVersionJSON` in
`apps/backend/internal/launcher/version_json_test.go`, calling actual `Run`
with `--version --json` and asserting success and the object. Run this command
before any production edit and record the expected failure: exit 2 for unknown
`--json`, not a compile or setup failure. Then implement and expand tests.

```bash
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-version-red.XXXXXX)" GOMEMLIMIT=512MiB ../../scripts/run-quiet backend-test -- go test -race -trimpath -tags fts5 -p 1 ./internal/launcher -run '^TestRunVersionJSON$' -count=1)
```

Exercise exact object bytes and decoding with quotes, backslashes and newlines
in stamps; both orders for `--version` and `-V`; empty and nonempty values;
plain bytes; standalone JSON exit 2; and no startup or config access (use a
malformed config and protected startup seam or equivalent observable evidence).
Add writer error and short-write coverage, a Unix subprocess broken-pipe test
with a SIGPIPE control, and focused parser/help tests. Do not parallelize tests
that mutate stdout or global signal state.

```bash
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-version-green.XXXXXX)" GOMEMLIMIT=512MiB ../../scripts/run-quiet backend-test -- go test -race -trimpath -tags fts5 -p 1 ./internal/launcher -run '^(TestRunVersionJSON|TestVersionJSON|TestParseArgs|TestRunUsesDevVersionForEmptyBuild|TestRunMaintenanceJSON|TestMaintenanceJSON)' -count=1)
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-version-adjacent.XXXXXX)" GOMEMLIMIT=512MiB ../../scripts/run-quiet backend-test -- go test -race -trimpath -tags fts5 -p 1 ./internal/launcher/cli ./cmd/kandev -run '^(TestHelpDescribesPublicCommands|TestDispatch)' -count=1)
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-version-lint.XXXXXX)" GOMEMLIMIT=3GiB scripts/run-quiet go-lint -- bash scripts/lint-go-changed apps/backend/internal/launcher/launcher.go apps/backend/internal/launcher/options.go apps/backend/internal/launcher/maintenance_database.go apps/backend/internal/launcher/version_json.go apps/backend/internal/launcher/version_json_test.go apps/backend/internal/launcher/version_json_sigpipe_unix_test.go apps/backend/internal/launcher/options_test.go apps/backend/internal/launcher/cli/help.go apps/backend/internal/launcher/cli/help_test.go
```

Build the native executable in a task-owned external directory, then check the
stamped output and plain compatibility against the actual binary:

```bash
version_artifacts="$(mktemp -d /tmp/kandev-version-binary.XXXXXX)"
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-version-build.XXXXXX)" GOMEMLIMIT=512MiB ../../scripts/run-quiet backend-build -- go build -trimpath -tags fts5 -p 1 -ldflags '-X main.Version=v1.2.3 -X main.Commit=abc123 -X main.BuildTime=2026-01-02T03:04:05Z' -o "$version_artifacts/kandev" ./cmd/kandev)
python3 - "$version_artifacts/kandev" <<'PY'
import json, subprocess, sys
binary = sys.argv[1]
plain = subprocess.run([binary, '--version'], capture_output=True, check=True)
assert plain.stdout == b'v1.2.3\n' and plain.stderr == b''
result = subprocess.run([binary, '--json', '-V'], capture_output=True, check=True)
assert result.stderr == b'' and result.stdout.endswith(b'\n') and result.stdout.count(b'\n') == 1
assert json.loads(result.stdout) == {'version': 'v1.2.3', 'commit': 'abc123', 'build_time': '2026-01-02T03:04:05Z'}
PY
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Confirm all IDs and design references, run the applicable local PR documentation
coverage preflight, and record results and exact changed paths. Before commit,
check formatting of staged Go files and use normal active hooks with complete
logs. After checks, refresh the full open-PR file overlap and follow `/pr` for
one ordinary commit, push and ready PR; preserve bot/user body blocks and verify
the live body and local/remote/PR head equality with a clean tree. No merge,
queue, bypass or workspace deletion. Return truthful install, RED/GREEN, test,
lint, build, hook and native-join receipts to ROOT; later monitoring and merge
remain subject to the explicit task instructions.

## Files likely touched

- `apps/backend/internal/launcher/launcher.go`, `options.go`, `options_test.go`
- `apps/backend/internal/launcher/maintenance_database.go` (shared flag spelling only)
- `apps/backend/internal/launcher/version_json_test.go` and
  `version_json_sigpipe_unix_test.go` (and a narrow JSON writer file if useful)
- `apps/backend/internal/launcher/cli/help.go`, `help_test.go`
- `docs/public/cli.md` and this package's four lifecycle/status documents.

## Dependencies

None within this package. ROOT's full four-document review and later separate
execution and global HEAVY release are prerequisites for implementation.

## Risks

Signal changes are process-wide; isolate signal tests and never change the
plain path. `encoding/json` alone does not detect a nonconforming short writer.
The public CLI reference contains unrelated historical drift; leave it alone.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/cli/requirements/version-json.md)
- [System design](../../specs/cli/system-design/version-json.md)
- Existing launcher, parser, build producer, help and maintenance JSON tests.

## Results

- Frozen pnpm install completed once with pinned pnpm 9.15.9; log:
  `/tmp/kandev-version-install.NlDHOf/kandev-run.install.C7SR1JW4.log`.
- Causal RED: `TestRunVersionJSON` failed because `Run` returned 2 with
  `unknown argument "--json"`; log:
  `/tmp/kandev-version-red.oHo8Ut/kandev-run.backend-test.OuRYPUmE.log`.
- The targeted launcher race/fts5 test command passed after the final Go edit,
  including the version, parser, legacy version, maintenance JSON and Unix
  broken-pipe tests; log:
  `/tmp/kandev-version-regreen2.D19sF6/kandev-run.backend-test.GDpO7LuY.log`.
  The adjacent help and command-dispatch command passed; log:
  `/tmp/kandev-version-adjacent.WU95bx/kandev-run.backend-test.PoJsxsqg.log`.
- Changed-Go lint passed for all nine changed Go files; log:
  `/tmp/kandev-version-relint2.XqxgPo/kandev-run.go-lint.I8V5Qs3d.log`.
  Earlier lint attempts exposed and led to correction of an unchecked stderr
  write and repeated flag spelling.
- Stamped native build and the plain/JSON smoke assertions passed; build log:
  `/tmp/kandev-version-build.M3NLJ6/kandev-run.backend-build.2Fkj9eNA.log`.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py
  --all`, both public-doc validators and the local `validateCoverage` preflight
  passed. An initial public-doc attempt lacked Node on PATH; both commands
  passed with the pinned Node 24.21.0 path.
- Normal hooks, push, hosted checks and review are subsequent delivery gates.
