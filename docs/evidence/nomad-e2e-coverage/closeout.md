# Nomad executor closeout verification (2026-10-28)

Code candidate: `1222b56224fd61182d7498ef479684046d2d7ef4`, branch
`feature/nomad-native-executor`. This closeout adds evidence and corrects delivery-document
metadata; it changes no production code or tests. The six frozen Nomad specification files
remain unchanged. Upstream PR `kdlbs/kandev#4835` must remain draft.

## Full backend result and exact baseline proof

The previous `/private/tmp/nomad-full-branch.txt` ended with `FAIL` and `DONE`, but had no
numeric exit receipt or sufficient command provenance. No full backend test process remained.
A fresh `make test-backend` completed with make exit **2**, underlying Go exit **1**.
The Makefile executes `CGO_ENABLED=1 go test -trimpath -tags fts5 ./...` from `apps/backend`.

- 22 failed packages, 334 passing packages, 67 packages without tests.
- 262 failed top-level tests, or 452 failed identities including subtests.
- The process package hit the default 10-minute timeout while
  `TestReadStderrDelayedGenerationCannotCloseReplacementChannel` was running. That interrupted
  test is separate from the 262 assertion failures. Tests after the interruption are not
  proven by this full run.
- None of the 263 selected top-level names contains `Nomad` or `Pool`.

The selected set is every failed root plus that interrupted root. Reran it once each on
the candidate and two clean detached scratch worktrees:

| Revision | Role | Go exit | Failed roots | Passed roots | Failed identities with subtests | Missing/skipped |
| --- | --- | --- | --- | --- | --- | --- |
| `1222b56224` | Current candidate | 1 | 262 | 1 | 452 | 0/0 |
| `73c8155caf` | Required previous-card baseline | 1 | 262 | 1 | 452 | 0/0 |
| `baa92e5dcf` | Upstream merge base | 1 | 262 | 1 | 452 | 0/0 |

All failed-name sets are identical, including subtests. Their SHA-256, over sorted
`package<TAB>test` identities joined with newlines, is
`8827a43400a9e3af3f3d6fd99278aa0f0e94f831b61f406074b2404bc5acad6e`.
The 11 passed identities including subtests match too. The interrupted stderr test passes
in all three selections. The full run's 452 assertion-failure identities exactly match
the selected rerun's failed set. Both baseline checkouts remained clean after testing.
This proves the reproduced failures predate this card; it does not make the full suite green
or establish the cause of every failure.

The complete shared argv, revision hashes, exact test names and empty set differences are in
[backend-failure-comparison.json](backend-failure-comparison.json). Its `comparison.argv`
is the actual command, run from `apps/backend` in each checkout:

```text
go test -trimpath -tags fts5 -count=1 -json -timeout=10m -run <exact anchored 263-name alternation> <22 exact package paths>
```

To repeat the saved argv on an already prepared clean checkout, run from this checkout's root:

```sh
BASELINE_CHECKOUT=/absolute/clean/baseline python3 - <<'PY'
import json, os, subprocess
from pathlib import Path
receipt = json.loads(Path('docs/evidence/nomad-e2e-coverage/backend-failure-comparison.json').read_text())
raise SystemExit(subprocess.run(receipt['comparison']['argv'], cwd=Path(os.environ['BASELINE_CHECKOUT']) / 'apps/backend').returncode)
PY
```

Environment: Darwin arm64, Go 1.27.2, inherited `GOFLAGS=-p=4`, CGO enabled, inherited macOS
temporary directory. The full run and all three comparison runs used the same login-shell
environment, whose PATH omitted Node. No tests were weakened, skipped or modified to obtain
the baseline match. Raw logs stay in `/private/tmp/ff-nomad-closeout/`; they are not committed.

## Current focused checks

Commands below run from `apps/backend`; all exit **0**. Counts are top-level Go tests,
not subtests. Full argv/count receipts are also in [focused-checks.json](focused-checks.json).

```sh
go test -trimpath -tags fts5 -count=1 -json ./internal/agent/nomad/...
go test -trimpath -tags fts5 -count=1 -json -run 'Nomad|Pool|LoadEnvZeroIdleTimeoutDisablesReaper' \
  ./internal/agent/runtime/lifecycle/ ./internal/agent/handlers/ ./internal/agent/executor/ \
  ./internal/agentctl/server/config/ ./internal/agentctl/server/instance/ ./internal/task/service/ \
  ./internal/task/repository/sqlite/ ./internal/orchestrator/executor/
go test -trimpath -tags fts5 -count=1 -json -race ./internal/agent/nomad/...
go test -trimpath -tags fts5 -count=1 -json -race -run 'Nomad|Pool' ./internal/agent/runtime/lifecycle/
go vet -trimpath -tags fts5 ./internal/agent/...
```

- Nomad: **109 passed**, none skipped.
- Dependent selection: **235 passed, 2 skipped**. Skipped PostgreSQL tests:
  `TestPostgresPlanCommentAdmissionLeaseRejectsSingleConnectionPool` and
  `TestPublishRecoveredNomadEnvironmentReady_PostgresRacesCheckpointNomadExecutorInventory`.
- Race: **109 passed** in Nomad; **169 passed** in lifecycle; none skipped.
- Vet: clean.

From the repository root, `python3 -m unittest discover -s scripts -p 'test_nomad*.py'`
exits **0**, `Ran 600 tests in 27.403s`, `OK`. The initial sandboxed attempt exited 1 with
17 loopback-bind permission errors; the normal loopback-access retry passed. ResourceWarnings
were emitted by Python 3.15 and are not test failures. These are offline tests, not real Nomad E2E.

## Applicable gates and delivery metadata

All final local gate results below exit **0**:

- `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_nomad_cleanup.go apps/backend/internal/agent/runtime/lifecycle/executor_nomad_rollback_test.go`: `0 issues.`
- `gofmt -l` on those two Go files: no output; `git diff --check`: clean.
- `make lint-format`: `All matched files use Prettier code style!`
- `node apps/web/scripts/generate-release-notes.mjs`,
  `node apps/web/scripts/generate-changelog.mjs`, then `make typecheck`: pass.
- `make lint-web`: pass. `make lint-harness lint-architecture`: pass;
  `All 204 harness file(s) passed.`
- From `apps/web`, `pnpm run i18n:ratchet`: no UI source added/modified; 642-entry allowlist intact.
- `python3 scripts/lint-spec-files.py --all`: `All specification files passed.`
- `python3 scripts/list-docs.py validate`: `Validated 404 decisions and 1752 specifications.`
- `gitleaks git --log-opts='17ffa37d6c..HEAD' --redact` on the four candidate code/runner
  commits: `no leaks found`.

The first `make lint-format` attempt exited 2 because the login shell could not find pnpm.
The retry and Node-dependent gates used the installed non-login-shell PATH (Node 24.13.0,
pnpm 12.15.0). No tool or hook was bypassed.

PR documentation coverage failed on the candidate: the pool work order cites the native
executor system design, while its parent plan omitted it. Calling the existing
`.github/scripts/pr-docs.cjs` `validateCoverage` function on the branch's changed files and
linked Nomad documents reproduced exactly that error. Adding the missing parent-plan
`system_design` link changes the local verdict from exit **1** (`invalid`) to exit **0**
(`covered`, **11 work orders**, no errors). The plan's pending status and unchecked Task 03
were reconciled with the existing completed work orders and historical fixture proof;
the three remaining live-proof limits stay explicit. No validator or frozen spec was edited.

No `apps/web/` path changed against the upstream merge base, so the board's path rule does
not require Playwright. Web/CLI full suites were not repeated for these Go/runner and
documentation changes; their older receipts in the task plan remain historical. Full backend
lint remains CI-owned; this run uses the prescribed changed-package lint. PostgreSQL DSN is
unset (417 Postgres-named roots self-skipped in the full run); no PostgreSQL pass is claimed.
The latest code/runner round adds no schema, SQL dialect or event-bus subscriber changes.

## Evidence limits and handoff

- The [14 default-mode laptop results](README.md) are operator-reported; raw live JSON is
  not committed. No `--require-real` invocation ran during closeout.
- TLS/scoped-ACL fixture mode has not run live. Its policy has not been diffed against
  FleetFoundry's deployment policy; macOS OpenSSL compatibility remains unverified.
- SFTP cancellation fix 7 has Go tests only.
- Native post-restart instance reattachment has not proved an actual agent-to-Kandev tool call.
- [remaining-fixtures.md](remaining-fixtures.md) gives the supported disposable-fixture
  commands/configuration and identifies the two required harness extensions.
- No live Kandev, FleetFoundry, TeamClaude, Nomad or collector service, and no shared workflow
  profile, was changed or restarted. Actual Tank/Neo pool evaluation remains separately approved.
- Testing and Review must use an available Codex session. Their configured Sonnet routing
  cannot be changed with the exposed task tools. The external task plan and
  `/private/tmp/ff-nomad-closeout-result.md` carry the exact coordinator routing requirement.

This is a local closeout receipt, not a claim that upstream CI/review or production rollout
has completed. Current-head remote status and hook/push receipts belong in the external
task plan and coordinator result file.
