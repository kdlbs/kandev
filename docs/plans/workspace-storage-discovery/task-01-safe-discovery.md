---
id: "01-safe-discovery"
title: "Correct discovery and prove safe workspace measurements"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.9
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.10
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.11
system_design:
  - ../../specs/system-page/system-design/workspace-storage-discovery.md
---

# Task 01: Correct discovery and prove safe workspace measurements

## Summary

Restore workspace analysis beside an unrelated checkout with `CLAUDE.md -> AGENTS.md`.
Correct shared classification without granting cleanup eligibility to ordinary repository folders.
Prove compatibility, cleanup safety, and desktop and phone recovery through one focused implementation pass.

## In scope

- TDD regression `TestAnalyzeKeepsValidWorkspacesWithUnclassifiedCheckout` against `Provider.Analyze`.
- Shared root recognition and scratch-container symlink handling from the linked design.
- Layout, unclassified-sibling, control-symlink, orphan-cleanup, and dependency-cleanup coverage from the plan test matrix.
- Omission warnings for permission-denied unclassified paths, with recognized-root and invalid-control failures preserved.
- Real-backend Storage E2E in the two planned project-specific files.
- Accurate work-order results and plan status after the listed commands pass.

## Out of scope

- Live instance changes, automatic marker installation, directory moves, and adoption.
- New inventory fields, persistence, API shapes, flags, settings, or UI components.
- Broad local review, full E2E, commits, pushes, PR creation, and subagents.

## Acceptance

1. The new primary regression first fails with the known discovery error, then passes with exact recognized bytes and an omission warning.
2. All supported layouts remain recognized. Unclassified paths remain untouched across analysis, orphan quarantine, and dependency cleanup. Control-path and inventory guards pass.
3. Fresh real-backend Analyze shows measured workspace GB on desktop and phone with the fixture symlink intact. The asserted byte increase is relative to a fresh baseline that may include other recognized roots. Every listed command passes.

## Implementation sequence

1. Read the linked requirement, design, ADR, and existing provider tests.
2. Mark this work order `in_progress`.
3. Add the primary regression in a new test file.
4. Run the first command below and record its expected discovery-error failure.
5. Add the layout and cleanup regressions from the plan matrix.
6. Correct shared discovery with the smallest implementation that meets the design.
7. Update only the synthetic unmarked legacy fixture to canonical UUID names.
8. Add the real-backend desktop and phone tests and their owned fixture helper.
9. Run all remaining commands sequentially and record the actual results.
10. Mark the work order `done` and the plan `implemented` only after every required result passes.

The frontend production source remains unchanged. New user-facing copy is outside this work order.
Record any contract clarification in the owning requirement and design before treating implementation as complete.

## Verification

Run this complete block from the repository root.
The dependency installation is necessary only once in a fresh worktree.
The first command is the RED gate before the correction and must pass after the correction.

```bash
(cd apps/backend && go test -trimpath ./internal/system/storage/workspaces -run '^TestAnalyzeKeepsValidWorkspacesWithUnclassifiedCheckout$' -count=1 -v)
(cd apps/backend && go test -trimpath -race ./internal/system/storage/workspaces -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/system/workspace-storage-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-workspace-storage-discovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use managed E2E builds and preserve the runner's shard and worker limits.
Record RED evidence, final Go results, E2E test counts, and document validation.
Run the documentation-coverage preflight against the final changed files and this package before delivery.

## Files likely touched

- `apps/backend/internal/system/storage/workspaces/provider.go`
- `apps/backend/internal/system/storage/workspaces/discovery.go`
- `apps/backend/internal/system/storage/workspaces/provider_test.go` (legacy fixture adjustment only)
- `apps/backend/internal/system/storage/workspaces/provider_discovery_test.go` (new)
- `apps/backend/internal/system/storage/workspaces/dependency_discovery_test.go` (new)
- `apps/web/e2e/helpers/workspace-storage-discovery.ts` (new)
- `apps/web/e2e/tests/system/workspace-storage-discovery.spec.ts` (new)
- `apps/web/e2e/tests/system/mobile-workspace-storage-discovery.spec.ts` (new)
- `docs/plans/workspace-storage-discovery/plan.md`
- `docs/plans/workspace-storage-discovery/task-01-safe-discovery.md`
- `docs/specs/system-page/system-design/workspace-storage-discovery.md` (lifecycle status after conformance)

## Dependencies

None. Execute sequentially in the primary session after the explicit implementation request.

## Risks

The linked [plan risks](plan.md#risks) apply.
Do not convert unsafe-path errors into warnings or follow links while trying to establish layout evidence.
Do not rename valid marked fixtures to conceal compatibility failures.

## Parallelism

`sequential`

## Inputs

- [Storage requirement](../../specs/system-page/requirements/storage-maintenance.md), criteria `001.7` and `001.9` through `001.11`.
- [Discovery design](../../specs/system-page/system-design/workspace-storage-discovery.md), all sections.
- [Recognition ADR](../../decisions/2026-10-05-workspace-storage-discovery.md).
- `apps/backend/internal/system/storage/workspaces/provider_test.go` and `dependency_cleanup_test.go` for fixture patterns.
- `apps/web/e2e/tests/system/storage-maintenance.spec.ts` for isolated disk fixtures and manual Analyze.
- `apps/web/e2e/tests/system/mobile-storage-analysis-bars.spec.ts` for the existing phone storage surface.
- `.agents/skills/tdd/SKILL.md`, `.agents/skills/e2e/SKILL.md`, and scoped engineering guidance.

## Results

Implemented shared root recognition for marked roots, legacy semantic roots, and unmarked UUID scratch
pairs. Unknown directories stay outside measurements, orphan quarantine, and dependency cleanup.
Scratch-container child links still fail after container recognition, while links under unrecognized
parents remain opaque.

- The RED command failed as expected with `symlink beneath tasks root` for the unrelated checkout's
  `CLAUDE.md` link.
- `go test -trimpath -race ./internal/system/storage/workspaces -count=1` passed.
- `pnpm install --frozen-lockfile` passed. Playwright discovery completed with zero errors.
- Desktop E2E passed: 1 test. Mobile E2E passed: 1 test. Both used managed backend and Vite builds.
- Targeted ESLint passed for the new E2E fixture helper and both specs.
- `python3 scripts/list-docs.py validate` passed: 349 decisions and 1,342 specifications.
- `python3 scripts/lint-spec-files.test.py` passed: 36 tests. `python3 scripts/lint-spec-files.py --all` passed.
- Documentation coverage preflight returned `covered` with no errors. Final `git diff --check` passed.
- After review, discovery helpers and types were extracted to `discovery.go`, and scratch-child inspection was separated from candidate classification. The workspace race suite and `make build` passed again; the targeted golangci-lint run reported 0 issues. Effective production-file counts are 721 lines in `provider.go` and 164 in `discovery.go`.
- Documentation coverage preflight passed again after the lifecycle references were updated: `covered`, no errors.

The real-backend tests measured exact recognized workspace bytes, confirmed the omission warning, and
verified the checkout and external symlink target remained unchanged. The phone test used touch and
confirmed the expanded workspace row caused no horizontal page overflow.

PR fixup RED evidence: permission-probed tests executed as `nobody` showed unreadable unclassified
directories aborting Analyze and readable recognized paths retaining their error behavior. The
semantic-marker regression confirmed the existing nested-marker rejection branch.

PR fixup verification: the permission-probed regressions passed as `nobody`, the full workspace race
suite passed, and `make build` succeeded. Desktop and mobile storage E2E each passed after the
baseline-relative byte assertions were added. The focused saved-view recovery E2E passed three
repetitions after its 44px assertion was rounded to hundredth-pixel precision.

The refreshed full normal CI shard replay completed with 248 passed, 3 skipped, and 0 failed
(251 tests total). Against `main` tip `513ea8279b0a448f20b2aa0bc6485edf7455fb74`, synthetic
merge `e80b40e42646629c9e7fac4098ee3ea71e9ef97a` was conflict-free. The workspace race suite,
backend build, and targeted golangci-lint passed (0 issues). The focused Postgres cancellation
test command exited zero, but its `cancel` subtest skipped because `KANDEV_TEST_POSTGRES_DSN` was
unset; current `main` contains the bounded connection-release assertion.
