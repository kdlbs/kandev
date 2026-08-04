---
id: "06-repository-verification"
title: "Repository verification"
status: done
wave: 5
depends_on:
  - "01-frontend-locale-catalogs"
  - "02-backend-locale-negotiation"
  - "03-catalog-parity-gate"
  - "04-chinese-locale-e2e"
  - "05-localization-documentation"
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 06: Repository verification

## Acceptance

- Every user-requested focused and repository-wide command is run and recorded;
  unrelated baseline or environment failures are identified without concealment.
- Manual browser inspection covers Chinese text, canonical html lang, reload,
  switching back, raw keys, interpolation/Trans integrity, and obvious overflow.
- Final diff contains no unrelated formatting, generated files, dependency
  upgrades, accidental pseudo edits, or uncommitted screenshot binaries.

## Verification

```bash
make fmt
make typecheck
make test
make lint
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm lint --max-warnings 0
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec vitest run
cd apps/backend && go test ./internal/i18n/... ./internal/webapp/... ./internal/backendapp/...
make test-e2e
git diff --check
make dev
```

After the checks and manual inspection pass, reconcile all task/plan results,
review the complete diff, then use the repository `commit`, `push`, and `pr`
workflows with title `feat(i18n): add Simplified Chinese locale`.

Keep `make dev` running only for the manual Appearance check, then stop it
cleanly after switching zh-cn → reload → English and inspecting migrated pages.

## Files likely touched

- `docs/plans/i18n-zh-cn/plan.md`
- `docs/plans/i18n-zh-cn/task-*.md`
- No new production files are expected; any required fix returns to its owning
  task and reruns that task's exact check.

## Dependencies

Tasks 01-05 must be complete.

## Parallelism

Sequential. This task reconciles all evidence before Git delivery.

## Inputs

- All completed task results, user-requested validation list, manual acceptance
  criteria, and repository delivery workflows.

## Output contract

Report the full command ledger with outcomes/counts, manual browser results,
final diff review, toolchain/environment limits, blockers/risks, synchronized
task/plan status, and readiness for commit, push, and PR creation.

## Results

- Focused frontend suite: 5 files, 28/28 tests passed.
- Frontend i18n checks: 2,646 referenced keys, 3,100 English entries, 4
  pre-existing orphans, and exact pseudo/zh-cn parity; the new-code ratchet also
  passed.
- Strict web lint, web typecheck, recursive apps TypeScript check, Vite
  production build, and changed code/catalog formatting all passed.
- Backend `internal/i18n`, `internal/webapp`, and `internal/backendapp` tests
  passed with the current Go environment (`CGO_ENABLED=1`).
- Full Vitest: 8,408 passed and four skipped.
- Affected backend packages passed. The repository-wide backend suite still
  fails in unrelated native-Windows cases that depend on POSIX commands, Unix
  file modes, symlink privileges, or Unix path semantics.
- Public docs: 58/58 validator tests passed and all 41 published pages
  validated. All 113 harness files passed their repository lint.
- Language-switch E2E: 3/3 passed against local Chrome. The final Chinese
  Appearance screenshot was manually checked for raw keys, broken text,
  overflow, and secrets, then kept only as an ignored PR asset.
- `make fmt`, `make typecheck`, `make test`, `make lint`, and `make test-e2e`
  cannot enter their underlying commands on native Windows because the root
  Makefile invokes POSIX `printf`. Direct cross-platform equivalents were run.
  `make dev` was replaced by the isolated Playwright browser run to avoid
  leaving a development process active.
- The repository-wide Prettier check reports 388 pre-existing formatting
  differences on `origin/main`; only changed code/catalog files were checked to
  avoid an unrelated rewrite. `golangci-lint` and `pre-commit` are not installed
  on this host.
- `git diff --check` passed. Final commit, push, and PR creation are intentionally
  pending user review of the English metadata and its Chinese translation.
