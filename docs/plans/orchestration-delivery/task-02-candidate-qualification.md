---
id: "02-candidate-qualification"
title: "Candidate qualification"
status: done
wave: 4
depends_on: ["01-assistant-rollout-gate"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design:
  - ../../specs/orchestration/system-design/coordinator-view.md
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 02: Candidate qualification

## Summary

Turn a clean, reviewed commit into a versioned complete runtime bundle. Establish
which behavior and database engines were tested before using private live data.

## In scope

1. Require delivery 01 and coordinator-view 01–03 to pass. For an assistant-enabled
   candidate also require assistant 03–11. Record exact SHA, release merge-base,
   toolchain versions, flags and full work-order status; fail on a dirty tree.
2. Run affected unit/integration/race checks, typecheck, locale checks, architecture
   and documentation validators. For the first candidate run the repository-wide
   test/lint targets. Classify pre-existing failures with reproduction; do not
   relabel a partial run as a full pass or change unrelated behavior to hide it.
3. Run SQL guard, fresh/replay/upgrade conformance on SQLite and disposable
   PostgreSQL, with required-store coverage for retained assistant tables even
   when the feature is off. A skipped PostgreSQL subtest is not engine evidence.
4. Run coordinator, Automation and central-view browser specs in one managed
   Chromium invocation; include the mobile Coordinator flow. Re-run only checks
   affected by any resulting fix, then freeze the candidate commit.
5. Build a new bundle directory and unique version; record SHA-256 hashes for all
   binaries plus embedded web build provenance. Run bundle launcher/self-check
   and a fresh synthetic-home smoke with the intended flag configuration.
6. Produce a redacted qualification receipt with commands, exit codes/counts,
   warnings, supported platforms/providers and artifact hashes. Do not claim a
   live-data upgrade or real-provider execution from synthetic tests.

## Out of scope

Service changes, copying private databases, publishing a stable Kandev release,
installing credentials in CI and testing unsupported platforms by inference.

## Acceptance

- The recorded clean commit passes required checks with no unexplained skipped
  database engine, and new view/media evidence matches that candidate.
- A complete immutable versioned bundle starts on synthetic data and reports its
  expected version/configuration; artifact hashes identify exactly what will run.
- The receipt is reviewable and explicitly distinguishes tested behavior from
  the private-data rehearsal and provider trials still required.

## Verification

From the root, provision the disposable PostgreSQL DSN in the local environment;
never use production or commit it. Confirm the conformance output runs PostgreSQL.
New central/mobile spec filenames below are defined by view task 03.

```bash
git status --porcelain
git merge-base HEAD upstream/release-v0.94.0
make typecheck
make test
make lint
pnpm --dir apps/web run i18n:check
make -C apps/backend sqlguard
(cd apps/backend && go test -race -tags fts5 -count=1 ./internal/persistence/requiredstores ./internal/persistence/storeconformance)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/orchestration/workspace-orchestrators.spec.ts tests/orchestration/automation-orchestrator.spec.ts tests/orchestration/coordinator-view.spec.ts tests/office/sidebar-office-gating.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/orchestration/mobile-coordinator-view.spec.ts -- --retries=0
make runtime-bundle RUNTIME_VERSION='<candidate-version>' RUNTIME_BUNDLE_DIR='<new-candidate-directory>'
git diff --check
```

Substitute real version/directory values before execution. Do not run these
resource-heavy suites simultaneously or disable managed runner guards.

## Files likely touched

Candidate receipt under `docs/review/orchestration/`; task result records. Bundles
and raw logs stay outside Git. Any source fix gets its own reviewed commit and
appropriate requirement/work-order update.

## Dependencies

Delivery 01; all three coordinator-view work orders; assistant 03–11 only for
assistant enablement. Private CI is optional and cannot replace local evidence.

## Risks

An old embedded web build or helper binary can disagree with the backend SHA.
Always use the complete runtime-bundle target, not a copied backend executable.

## Parallelism

`sequential`

## Inputs

[Dogfood runbook](dogfood-runbook.md), root/backend Makefiles, E2E skill and
existing v0.94.0 baseline validation.

## Results

Complete for Linux x86-64. Full repository
qualification found and corrected seven PostgreSQL fixture dependencies and an
Office New Task dialog compatibility regression. Fourteen PostgreSQL failure
reproductions now pass with race detection; the backend rerun passes all 286
packages and repository-wide lint passes. The full frontend run executed 1,913
files and found one incomplete onboarding fixture; the corrected navigation
files pass all 37 checks. CLI and the full script target pass after supplying
the missing toolchain prerequisites. Type checking, locale checks, SQL guard and
645 race-enabled conformance checks pass on both database engines. The combined
six-check desktop browser invocation and the phone Coordinator flow pass with
one worker and no retries. The frozen complete bundle
`0.94.0-orchestration.20260918.sha69753564d0d7` passes its launcher smoke on
synthetic data, including embedded asset hashes, disabled-assistant APIs and no
work from page visits. Matching desktop and phone capture invocations pass two
tests each. All eight screenshots and twelve silent-video source frames were
visually reviewed. The E2E-only adjacent mock helper and external web variant are
explicitly distinguished from the immutable production bundle. See the
[qualification record](../../review/orchestration/candidate-qualification.md).
The [candidate receipt](../../review/orchestration/candidate-receipt.json) records
the clean source, all six binary hashes and supported limits. Private-data
migration and rollback are delivery 03 and are not yet claimed.
