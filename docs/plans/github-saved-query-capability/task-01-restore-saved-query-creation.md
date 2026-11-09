---
id: "01-restore-saved-query-creation"
title: "Restore saved-query creation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001.1
  - AC-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001.2
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.1
system_design:
  - ../../specs/integrations/system-design/github-dashboard-mobile.md
---

# Task 01: Restore saved-query creation

## Summary

Use the established non-security UUID helper for GitHub saved-query identity and verify that real save composition persists and publishes the expected view with and without `randomUUID`.

## In scope

- Independently write the rendered regression and secure-capability control described in the [plan](plan.md#tests); demonstrate causal RED before the minimal fix and GREEN afterward.
- Add the focused desktop and mobile browser flows in the [plan](plan.md#e2e-tests-and-mobile-parity); prove causal RED in both before the source fix and GREEN after it.
- Preserve the existing `randomUUID` preference test, the `p_` prefix, all input fields and current synchronization and error handling.

## Out of scope

Helper semantics or security-sensitive randomness, backend/API and storage shape, transport/binding, other UI or dialog behavior, and changes to `mobile-github-results.spec.ts`.

## Acceptance

- In the affected rendered flow, missing `randomUUID` with `getRandomValues` produces distinct UUID-shaped `p_` IDs, the expected settings request and published view, and successful dialog closure; the normal capability control has the same successful outcome.
- The shared helper is used at the existing identity creation point; no saved-query fields or synchronization semantics change.
- Desktop and phone browser tests complete the actual GitHub page save flow with capability simulation and isolated mock provider data.

## Verification

Run only after the separate implementation release and under one globally held HEAVY lease. Serialize install, tests, lint, browser builds/runs and hooks; retain and join each actual native handle, record its exit status and confirm cleanup of the known fixtures owned by each run. Do not audit historical or unrelated processes or incidental metadata. Use Node 24.21.0 and pnpm 9.15.9 on PATH, including hooks. Run from repository root. A fresh worktree needs one frozen install before package commands. Give every `scripts/run-quiet` call its own owned `KANDEV_RUN_QUIET_DIR`. Do not count setup/collection failure as RED. `scripts/vitest-reviewed-projects.json` places `use-saved-presets.test.ts` in `browser`; `lib/utils.test.ts` and the unlisted new suite belong to `browser-locales`. Vitest 4 takes `--maxWorkers=1`.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
node --version
pnpm --version
(cd apps && pnpm install --frozen-lockfile)
# First run the new rendered suite before the source fix and record causal RED plus the passing control.
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 components/github/my-github/saved-preset-capability.test.tsx)
# Before the source fix: require nonzero discovery for both exact projects and paths.
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium --list tests/github/github-saved-query-capability.spec.ts)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome --list tests/github/mobile-github-saved-query-capability.spec.ts)
# Run both E2E files pre-fix, with a managed desktop build and verified reuse for phone as described below.
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child021/e2e-red-desktop scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/github/github-saved-query-capability.spec.ts
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child021/e2e-red-phone scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/github/mobile-github-saved-query-capability.spec.ts
# After implementing the source fix, run the new rendered suite and only the relevant existing controls.
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 components/github/my-github/saved-preset-capability.test.tsx)
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 lib/utils.test.ts -t 'generateUUID')
(cd apps/web && pnpm exec vitest run --project browser --maxWorkers=1 components/github/my-github/use-saved-presets.test.ts -t 'uses a random UUID for new preset ids')
# Fresh managed post-fix build for desktop; verify fingerprints again before phone reuse.
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child021/e2e-green-desktop scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/github/github-saved-query-capability.spec.ts
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child021/e2e-green-phone scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/github/mobile-github-saved-query-capability.spec.ts
(cd apps/web && pnpm exec eslint --max-warnings 0 components/github/my-github/use-saved-presets.ts components/github/my-github/saved-preset-capability.test.tsx e2e/helpers/github-saved-query-capability.ts e2e/tests/github/github-saved-query-capability.spec.ts e2e/tests/github/mobile-github-saved-query-capability.spec.ts)
# Stage all actual new and changed paths before ratchets so untracked files are included.
git add docs/specs/integrations/requirements/github-dashboard-mobile.md docs/specs/integrations/system-design/github-dashboard-mobile.md docs/plans/github-saved-query-capability apps/web/components/github/my-github/use-saved-presets.ts apps/web/components/github/my-github/saved-preset-capability.test.tsx apps/web/e2e/helpers/github-saved-query-capability.ts apps/web/e2e/tests/github/github-saved-query-capability.spec.ts apps/web/e2e/tests/github/mobile-github-saved-query-capability.spec.ts
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --cached --check
git status --short
```

The Playwright configuration excludes `mobile-*.spec.ts` from chromium and matches it in mobile-chrome; the paths above follow its `testDir`. Record the actual nonzero discovery count and causal assertion failure for each RED, then actual GREEN counts and joins. A setup, collection or selector failure is not RED. If the desktop managed run fails during its product assertion, its successful build may still be reused for the immediately following phone run. Record and compare, before each phone run, the corresponding desktop build's HEAD, binary diff and status of build inputs (including content hashes for relevant untracked inputs), plus hashes of `apps/web/dist` files, `apps/backend/bin/{kandev,mock-agent,e2e-delivery-fixture}`, and `apps/backend/.build/{kandev-plugin-e2e-1.0.0.tar.gz,e2e-plugin-identity.json}`. Reuse only if every input and artifact fingerprint is unchanged and the desktop build completed; otherwise run the phone command without `--no-build`. No source or build input changes may intervene. The runner's `--no-build` is documented to reuse Vite, backend and packaged fixtures; global setup still checks freshness and plugin identity. A post-fix build can never reuse pre-fix assets. Use unique log directories, record actual native handles and joins and verify the known owned fixture cleanup.

Add every actually changed TS/TSX/helper path to the scoped ESLint and test commands and stage list; do not silently omit later additions. If the two browser specs share a new helper, include it in the stage and ESLint lists. Run canonical `(cd apps/web && pnpm run typecheck)` only after ROOT grants a resource budget based on current capacity and prior evidence; record grant, command and result, or explicit blocker. Run catalog/spec/diff checks after final document edits. Do not run a full local suite or replay passing suites without a new reason.

## Files likely touched

- `apps/web/components/github/my-github/use-saved-presets.ts`
- `apps/web/components/github/my-github/saved-preset-capability.test.tsx`
- `apps/web/e2e/helpers/github-saved-query-capability.ts`
- `apps/web/e2e/tests/github/github-saved-query-capability.spec.ts`
- `apps/web/e2e/tests/github/mobile-github-saved-query-capability.spec.ts`
- This plan and its linked integration requirement and system design, for results and lifecycle.

## Dependencies

None. Check current ownership and branch state before implementation.

## Risks

Simulated browser capability is not an actual insecure-origin network proof. Preserve real `getRandomValues` and restore patched globals after tests. Avoid asserting only the implementation call; check the persisted object and rendered outcome.

## Parallelism

Sequential.

## Inputs

The [GitHub dashboard requirement](../../specs/integrations/requirements/github-dashboard-mobile.md), [design](../../specs/integrations/system-design/github-dashboard-mobile.md), `use-saved-presets.ts`, `use-saved-preset-actions.ts`, `save-preset-dialog.tsx`, existing hook/UUID tests, and scoped web guidance.

## Results

- Frozen install passed with Node 24.21.0 and pnpm 9.15.9. The rendered pre-fix run failed the fallback case and passed its secure-capability control; after the fix, both cases passed. Focused UUID helper (5) and existing randomUUID preference (1) passed.
- Each desktop and phone browser test collected once, failed before the fix on the open-dialog outcome with save-error feedback, and passed afterward. Phone reused matching fresh pre/post desktop artifacts; input and artifact fingerprints and run logs are retained in task evidence. All four owned fixture directories were absent after their runs. These simulate browser capability and do not prove insecure-origin networking.
- Scoped ESLint passed on all five changed TS/TSX files. Staged i18n and E2E sleep ratchets, catalog validation, specification lint and diff checks passed.
- The initial 6144 MiB canonical typecheck found two test-only type errors. Both were fixed, and the affected rendered suite and scoped ESLint passed again. The corrected current inputs then passed canonical web typecheck with `NODE_OPTIONS=--max-old-space-size=6144`.
