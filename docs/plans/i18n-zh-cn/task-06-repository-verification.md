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
spec: "../../specs/platform/requirements/i18n.md"
---

# Task 06: Repository verification

## Acceptance

- Every user-requested check is either run and recorded or, when a native-Windows
  wrapper is unavailable, its approved cross-platform equivalent is run and the
  skipped wrapper is recorded; unrelated baseline or environment failures are
  identified without concealment.
- Manual browser inspection covers Chinese text, canonical html lang, reload,
  switching back, raw keys, interpolation/Trans integrity, and obvious overflow
  on the desktop capture; the mobile scenario covers the same locale lifecycle
  at the `mobile-chrome` viewport.
- Final diff contains no unrelated formatting, generated files, dependency
  upgrades, accidental pseudo edits, or uncommitted screenshot binaries.

## Verification

```bash
(cd apps/web && pnpm exec vitest run scripts/check-i18n-keys.test.ts lib/i18n/index.test.ts lib/i18n/boot.test.ts lib/i18n/formats.test.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/backend && go test -race ./internal/system/...)
(cd apps/backend && golangci-lint run ./... --new-from-rev="origin/main" --timeout=5m)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/i18n/language-switch.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/i18n/mobile-language-switch.spec.ts)
git diff --check
```

The ledger below records the root Make targets separately from the focused
cross-platform commands actually used for this rebase fixup. The isolated
Playwright language-switch runs are the approved alternative to the `make dev`
manual browser flow.

After the checks and manual inspection pass, reconcile all task/plan results,
review the complete diff, then use the repository `commit` and `push` workflows.
PR metadata is outside this task's scope.

The managed Playwright run captures the desktop Appearance state under the
ignored `.pr-assets/` directory. Inspect that image after the run and keep it
out of the feature branch.

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

- Frontend contract suite: 4 files, 32/32 tests passed.
- `pnpm run i18n:check`: 2,790 referenced keys, 3,300 English entries, 4
  pre-existing orphans, 18 namespaces, and exact pseudo/zh-cn parity; the
  `<Trans>` index and inline-plural/module-scope checks also passed.
- `pnpm run i18n:ratchet` passed with `0 added + 8 modified file(s) clean` and
  the 248-entry guard allowlist intact. Web typecheck passed.
- The affected backend update packages passed their 500-run race regression,
  the complete `internal/system/...` race suite passed, and the changed-code
  `golangci-lint` run passed.
- Desktop language-switch E2E passed 3/3 and mobile-chrome language-switch E2E
  passed 1/1. The desktop Chinese Appearance capture at
  `apps/web/.pr-assets/language-switch--simplified-chinese-locale-desktop.png`
  was manually checked for raw keys, broken interpolation, overflow, and
  secrets; the asset remains ignored and untracked.

| Requested repository check | Status  | Evidence or approved alternative                                                                      |
| -------------------------- | ------- | ----------------------------------------------------------------------------------------------------- |
| `make fmt`                 | Not run | Focused `gofmt` and changed-file formatting checks passed.                                            |
| `make typecheck`           | Not run | `apps/web` `pnpm run typecheck` passed; no full Make wrapper was needed for the focused rebase fixup. |
| `make test`                | Not run | Focused Vitest and `go test -race ./internal/system/...` passed.                                      |
| `make lint`                | Not run | Changed-code `golangci-lint` passed; the i18n ratchet passed.                                         |
| `make test-e2e`            | Not run | Focused desktop and mobile managed Playwright runs passed.                                            |
| `git diff --check`         | Passed  | No whitespace errors.                                                                                 |
| `make dev`                 | Not run | Approved managed Playwright run plus manual screenshot inspection covered the browser flow.           |

The repository-wide Make wrappers were intentionally not rerun as part of this
focused rebase fixup; the ledger records that limitation rather than implying
full-suite coverage. No generated build output or screenshot binary is tracked.
