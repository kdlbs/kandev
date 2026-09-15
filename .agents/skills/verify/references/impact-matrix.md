# Verify Impact Matrix

Collect `base...HEAD`, staged, unstaged, and untracked paths. Deduplicate the
union, then run every matching row. Prefer package/suite targets over
individual test names so changed dependents remain covered. For an
`apps/backend/**` path, the path match alone does not select the dialect or
event-bus rows below. Inspect the diff, then apply each specialized row whenever
its predicate matches.

When the planner supplies a last verified SHA and a `/commit` hook receipt,
read [hook-evidence.md](hook-evidence.md). Eligible hook evidence removes the
duplicate formatting/lint portions below; do not rerun them, and run every
uncovered command.

| Paths | Changed-mode commands |
| --- | --- |
| `AGENTS.md`, `CLAUDE.md`, `.agents/**`, `.claude/**`, `.codex/**`, `.cursor/**`, `.opencode/**`, ADRs/plans/specs | `git diff --check` and targeted `.github/scripts/lint-harness-files.py` inputs; parse changed TOML |
| `docs/public/**` | `node --test scripts/validate-public-docs.test.mjs` plus applicable harness lint |
| `.github/workflows/**` | `python3 .github/scripts/lint-action-pinning_test.py` plus applicable harness lint |
| `scripts/**`, `.github/scripts/**` | sibling syntax/test when obvious; otherwise `make test-scripts` |
| `apps/backend/**` | `make fmt-backend`, `make test-backend`, `make lint-backend` |
| A backend diff that changes dialect-sensitive schema or SQL, including `CREATE TABLE`, `ALTER TABLE`, `CREATE INDEX`, PostgreSQL-specific types/operators/functions, a `dialect.IsPostgres` branch, a table-rebuild/cutover migration, or SQLite-only `rowid`/JSON/date syntax | `KANDEV_TEST_POSTGRES_DSN=<dsn> make -C apps/backend test`, plus the changed package's focused PostgreSQL tests by test name with verbose output (see `apps/backend/AGENTS.md` § Schema & migrations). Confirm that the focused tests pass instead of skipping. No DSN available: record the gap explicitly instead of reporting a pass |
| A backend diff that adds or changes an event-bus subscriber, or type-asserts `event.Data` | Add a decode-path unit test that marshals the payload to JSON, decodes into `interface{}`, and invokes the subscriber with the resulting wire-shaped value (pattern: `TestGitHubPRMergedSubscriberAcceptsJSONDecodedPayload` in `internal/automation/github_pr_merged_subscriber_test.go`); a bare `event.Data.(*T)` assertion is a finding, since JSON decoding may produce a map, scalar, or array depending on the payload, not the typed pointer (see how `normalizeTaskPR` in `internal/automation/github_pr_merged_subscriber.go` accepts all three representations) |
| Eligible narrow pure helper in `apps/web/**` | changed-file Prettier/ESLint when uncovered, package-local typecheck, and the helper's colocated test file |
| `apps/web/**` | generate web metadata, `make fmt-web`, `make typecheck-web`, `make test-web`, `make lint-web` |
| `apps/cli/**` | workspace format, CLI TypeScript check/build, `make test-cli` |
| `apps/desktop/**` excluding `src-tauri` | desktop TypeScript check and the directly affected desktop smoke/test |
| `apps/desktop/src-tauri/**` | matching Rust toolchain, `cargo fmt --check`, `cargo check`, and scoped `cargo test` |
| `apps/packages/**` or shared TypeScript config | web metadata, workspace format/typecheck, web and CLI tests, web lint, desktop typecheck |

Use `mode=full` when the base/diff cannot be established; a changed path has no
safe row; the user requests it; delivery has no PR CI; or changes touch root
build/toolchain/dependency lockfiles, Makefiles, profiles, generated contracts,
release tooling, migrations/shared schemas, or unusually broad plan
implementation. Multiple known rows are not automatically ambiguous: run their
union when ownership and dependents are clear. `mode=full` does not set
`KANDEV_TEST_POSTGRES_DSN` either — a migrations/shared-schemas escalation still
needs the dialect row above.

A suite that self-skips (missing `KANDEV_TEST_POSTGRES_DSN`) is not coverage:
report it as not run, never as a pass.

## Narrow Pure Web Helper

Use the narrow row only when all of the following are proven from the diff and
source:

- The scope changes exactly one non-TSX `apps/web` helper and its colocated
  `*.test.ts`; it changes no configuration, generated artifact, shared package,
  API client, store, component, hook, or other production file.
- The test directly imports and exercises the helper. The helper and its direct
  dependencies are deterministic and do not render React, use hooks or stores,
  access browser/DOM globals, or perform network, filesystem, timer, or
  process-side effects.
- The change does not alter an acceptance flow that needs integration or E2E
  evidence. Any uncertainty, extra dependent, or indirect side effect uses the
  generic `apps/web/**` row instead.

For an eligible helper, run its test file through the web package's test command
and run `cd apps/web && pnpm run typecheck`. If hook evidence does not cover
the changed-file format/lint checks, run Prettier and ESLint against only the
two changed paths. Report this as `changed-scope PASS (narrow pure helper)`,
including the eligibility evidence and exact test path. This is a local
proportional check; PR CI remains the authoritative package matrix.

Do not run E2E unless acceptance or changed behavior requires it; targeted E2E
is separate evidence. A changed-scope pass permits commit/push, while PR CI
supplies the authoritative full matrix before readiness.
