---
id: "09-e2e-qa-verification"
title: "E2E, QA, and final verification"
status: done
wave: 5
depends_on: ["08-storage-settings-ui"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 09: E2E, QA, and final verification

Prove the feature through real backend/UI flows, mobile parity, Docker isolation, and final checks.

## Acceptance

- Desktop E2E covers defaults, analyze, idle/busy behavior, quarantine, restore, persisted settings,
  and unarchive recovery; mobile E2E completes the same primary user value from the settings sheet.
- Container-project E2E proves Kandev-label scoping and dedicated-daemon gating without deleting an
  unrelated container.
- Formatting precedes typecheck/test/lint; all focused and final verification is recorded, and
  docs/spec/ADR/AGENTS references remain accurate.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/system/storage-maintenance.spec.ts
cd apps/web && pnpm e2e:run tests/system/mobile-storage-maintenance.spec.ts -- --project=mobile-chrome
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers tests/docker/storage-maintenance.spec.ts
make fmt
make typecheck test lint
```

## Files likely touched

- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- `apps/web/e2e/tests/task/unarchive-storage-recovery.spec.ts`
- `apps/web/e2e/tests/docker/storage-maintenance.spec.ts`
- E2E fixture/API/page helpers needed to seed owned orphan resources safely
- relevant `AGENTS.md`, spec, ADR, and plan status files if implementation changes their facts

## Dependencies

Task 08 and transitively all backend/provider tasks.

## Inputs

- Every spec scenario
- `/e2e`, `/mobile-parity`, `/qa`, and `/verify` workflows
- `apps/web/e2e/README.md`

## Output contract

Report exact E2E/final commands and results, artifacts or blockers, conformance gaps, residual risk,
and update this task plus `plan.md` to done only when the full feature is verified.

## Verification results

- `cd apps/web && pnpm e2e --project=chromium tests/system/storage-maintenance.spec.ts` — 3 passed,
  including the disabled-cache global-run no-op and explicit `go_cache` cleanup flow.
- `cd apps/web && pnpm e2e:run tests/task/unarchive-storage-recovery.spec.ts` — production build
  succeeded and 1 passed.
- `cd apps/web && pnpm e2e --project=mobile-chrome tests/system/mobile-storage-maintenance.spec.ts`
  — 1 passed at the Pixel 5 viewport, including explicit Go-cache cleanup and the
  horizontal-overflow assertion.
- `cd apps/web && pnpm e2e --project=containers tests/docker/storage-maintenance.spec.ts`
  — 1 skipped because no Docker daemon was reachable in the verification environment. The test is
  discovered and retains its managed writable-byte/count, Kandev-label isolation, and
  dedicated-daemon assertions.
- `cd apps/backend && go test ./internal/agent/docker ./internal/system/storage/... ./internal/backendapp`
  and the same package set with `-race` — 190 passed in 6 packages for each run.
- Focused storage web Vitest — 43 passed across the controller, API, and system-slice suites;
  focused Prettier and ESLint passed, and `pnpm run typecheck` passed.
- Final repository verification passed formatting, generated release-note/changelog checks,
  typecheck, lint, and `git diff --check`. Feature backend packages and focused race suites passed.
  The full test target also passed after resolving macOS `TMPDIR` to its physical `/private/var`
  path, avoiding the host's `/var` symlink alias in path-equality assertions.

E2E QA found and drove fixes for the manual quiet-period gate, initial-turn activity lease, cascade
archive projection, production quarantine-restorer wiring, and Go-cache analysis JSON shape. The
final host and mobile runs are green; real Docker execution remains the only environment-limited
verification.
