---
id: "02-release-workflow"
title: "Scheduled and manual Nightly release workflow"
status: completed
wave: 2
depends_on: ["01-version-and-publisher"]
plan: "plan.md"
spec: "../../specs/npm-nightly-channel/spec.md"
---

# Task 02: Scheduled and manual Nightly release workflow

- **Acceptance:** `release.yml` schedules exact noon UTC and skips unchanged/already-published
  commits before building.
- **Acceptance:** Maintainers can select the Nightly channel on a manual `main` dispatch; it follows
  the same metadata, build, and publication path as the schedule.
- **Acceptance:** Manual Nightly dry-run executes metadata/registry preflight but no builds or npm
  writes; desktop validation and backfill are rejected, and the Stable bump selector is ignored.
- **Acceptance:** Nightly reaches only web/runtime builds and nightly npm publication; every Stable
  GitHub/Desktop/GHCR/Homebrew mutation requires the Stable channel.
- **Acceptance:** Publication uses the exact checked-out SHA; complete Stable and Nightly workflow
  runs are serialized so a pending Stable tag cannot race npm publication.
- **Acceptance:** Preflight and publish skip when the stable Git/npm baseline disagrees or the
  observed Nightly tag moved, preventing stale or backward tag movement.
- **Acceptance:** An older scheduled rerun is skipped before building when a complete Nightly from
  a newer commit exists; an incomplete current target and ancestor-tagged older partial publishes
  are repaired, while unresolvable, divergent, or newer partial tag history fails closed.
- **Acceptance:** Workflow preflight and the npm publisher consume one shared package inventory.
- **Acceptance:** Registry verification remains fail-closed after three bounded attempts so one
  transient lookup failure does not abort the scheduled publish.
- **Verification:** `python3 .github/scripts/release-workflow-contract_test.py`
- **Verification:** `node --test scripts/release/npm-view-version.test.mjs`
- **Verification:** `cd apps && pnpm --filter kandev exec vitest run src/release-config.test.ts`
- **Verification:** `make test-scripts`
- **Files likely touched:** `.github/workflows/release.yml`,
  `.github/scripts/release-workflow-contract_test.py`, `apps/cli/src/release-config.test.ts`,
  `scripts/release/npm-packages.sh`, `scripts/release/npm-view-version.sh`,
  `scripts/release/npm-view-version.test.mjs`, `Makefile`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential because the workflow consumes Task 01's interface.
- **Inputs:** spec schedule/publication scenarios; existing `prepare`, `build-web`,
  `build-bundles`, and `publish-npm` jobs.
- **Risks:** an incomplete event gate could trigger stable side effects from cron.

## Verification results

- `python3 .github/scripts/release-workflow-contract_test.py` — passed, 22 tests.
- `node --test scripts/release/npm-view-version.test.mjs` — passed, 5 tests.
- `cd apps && pnpm --filter kandev exec vitest run src/release-config.test.ts` — passed, 13 tests.
- `make test-scripts` — passed, including both release workflow suites.
