---
id: "01-reset-token-dialog"
title: "Reset the token creation dialog"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AUTH-AUTH-001
acceptance_criteria:
  - AC-AUTH-AUTH-001.4
  - AC-AUTH-AUTH-001.9
  - AC-AUTH-AUTH-001.10
system_design:
  - ../../specs/auth/system-design/personal-access-token-creation.md
---

# Task 01: Reset the token creation dialog

## Summary

Restore repeated creation after a completed result is closed. Use one close
transition to discard dialog-local values while preserving the existing
one-time token result and shared controls.

## In scope

- Real-rendered independent regression at the natural auth API boundary; first
  establish a collected causal RED before changing production code.
- Route explicit and standard close paths through reset; test a second named
  creation, first creation, fresh form and copied state, and close controls.
- Focused real-browser desktop and phone repeat-create flows using isolated
  auth fixtures and the existing route, not a mock-only browser journey.

## Out of scope

- Backend/API, persistence, credentials policy, in-flight races, other dialogs,
  layout or target-size changes, and new copy.

## Acceptance

1. A successful first mint displays its result once and Done closes;
   reopening gives a blank form and a second named mint displays its own result.
2. Settled Cancel, close button, Escape and outside dismissal close as before;
   reopening clears name, error, raw token and copied state. Existing list
   refresh and localized controls remain intact.
3. Desktop and phone real-browser paths show the same outcome, phone uses tap
   and has no document-level overflow. No mobile geometry change is asserted.

## ASCII UI preview

UI-01: Reopened token creation. See [full preview and annotations](plan.md#ascii-ui-preview). Desktop:

```text
+------------------------------+
| Create API token          X  |
| Description                  |
| Name                         |
| [                         ]  |
| [Cancel]      [Create token] |
+------------------------------+
```

Phone, with the existing stacked footer:

```text
+------------------------+
| Create API token    X  |
| Description            |
| Name                   |
| [                   ]  |
| [Create token]         |
| [Cancel]               |
+------------------------+
```

The blank form and controls satisfy `AC-AUTH-AUTH-001.9` and `.10`; spacing is
illustrative. Compare rendered structure on both viewports.

## Verification

After the separate implementation release and global heavy lease, use Node 24
and pnpm 9.15.9, install once in the fresh worktree, then run the independent
component test before the fix. Confirm nonzero collection, the causal stale
result failure, and the first-create/Done control. The E2E skill also requires
running each new browser test RED before the production fix: build current
pre-fix assets and record each actual failure and test count. After the fix,
run the same focused tests GREEN. No screenshots or captures are required for
this unchanged layout; retain actual runner-generated failure artifacts.

Run sequentially, retaining every native handle, actual joined exit, test count,
and log path. Each `run-quiet` invocation gets a distinct task-owned
`KANDEV_RUN_QUIET_DIR`. Managed E2E builds the backend, web assets, and fixture
plugin and uses one worker. The commands are valid against the checked-in runner:
`auth` selects non-mobile specs in `tests/auth/`; `mobile-chrome` selects
`mobile-*.spec.ts`, including in `tests/auth/`. Confirm nonzero discovery in
both runs. Do not assume a 6144 MiB TypeScript allowance; wait for ROOT's
capacity-based grant before the canonical web typecheck and record its actual
resource limit and result. If capacity is unavailable, report the pending check
rather than claim it passed.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 components/settings/account/api-tokens.test.tsx)
KANDEV_RUN_QUIET_DIR=<unique-owned-desktop-dir> scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project auth tests/auth/api-token-repeat.spec.ts
KANDEV_RUN_QUIET_DIR=<unique-owned-phone-dir> scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/auth/mobile-api-token-repeat.spec.ts
```

Replace the two placeholder paths with distinct real task-owned directories;
use fresh directories for RED and GREEN. Repeat the three focused tests after
the fix, without repeating unrelated passing suites. After the final source
changes, stage the actual owned changes, including new files, and confirm the
staged and working-tree diff match before running the following checks:

```bash
git diff --cached --name-only --diff-filter=ACMR -- apps/web
(cd apps/web && pnpm exec eslint --max-warnings 0 components/settings/account/api-tokens.tsx components/settings/account/api-tokens.test.tsx e2e/tests/auth/api-token-repeat.spec.ts e2e/tests/auth/mobile-api-token-repeat.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --cached --check
```

Include every additional actual changed TS/TSX path (including any shared E2E
helper) in the scoped ESLint invocation; omit a listed path only if the actual
file is absent and explain why. Do not run broad lint or a full test suite.
The staged i18n ratchet must see added files and must not be reported as a pass
if skipped. If any new copy becomes necessary, add seven locales and run the
applicable i18n checks. Record final results in both work order and manifest.

For each browser run record source HEAD and the relevant tracked/uncommitted
source state, runner build and joined exit, and SHA-256 fingerprints of the
actual backend binary (`apps/backend/bin/kandev`), `apps/web/dist/index.html`,
and its built JS assets after that run. Tie these to the run and rebuild after changes to production or browser-build
inputs; test-only and documentation edits do not change those assets. Record owned backend fixture temp-root and process identifiers
without secrets; verify after join that the fixture's own processes exited and
its temp root was removed. The auth fixture must use an isolated database,
restore its backend after the scenario and close its browser context, including
failure paths. Do not remove foreign files or processes. If teardown cannot be
verified, report it as unresolved rather than inventing a receipt.

Component mocks use dummy values. Browser tokens are real disposable credentials
created only in the isolated fixture backend. Compare the two values through a
boolean assertion (including nonempty and distinct) without printing either in
assertion messages, logs, or reports. Preserve actual Playwright failure
artifacts honestly and treat them as potentially sensitive fixture artifacts.

## Files likely touched

- `apps/web/components/settings/account/api-tokens.tsx`
- `apps/web/components/settings/account/api-tokens.test.tsx` (new)
- `apps/web/e2e/tests/auth/api-token-repeat.spec.ts` (new)
- `apps/web/e2e/tests/auth/mobile-api-token-repeat.spec.ts` (new)
- A shared focused test helper only if needed.

## Dependencies

None. A later explicit implementation release and the global heavy lease are
required before production changes, dependency installation, or tests.

## Risks

Do not leak real secrets into logs. A late response after closing is not part of
this settled-action repair. Existing mobile targets are not re-sized or audited.

## Parallelism

Sequential.

## Inputs

Auth requirements, the linked design, `api-tokens.tsx`, shared dialog, existing
account tests and isolated auth browser fixtures. Do not read or replay the
private qualification test.

## Results

- Frozen workspace install passed with Node 24.21.0 and pnpm 9.15.9.
- Independent `browser-locales` Vitest RED collected six tests: first-create
  control passed; repeat and Cancel failed for retained values. The simulated
  outside event did not reach Radix in the test DOM; real browser coverage owns
  that interaction. Final component run passed all seven tests.
- Managed desktop auth and mobile-chrome E2E each collected one test and failed
  on the absent fresh form before the fix. Both passed after it, including
  outside dismissal; phone used taps and checked overflow. Mobile runs reused
  the corresponding verified desktop build. Fixture roots were removed after
  each joined run; fingerprints and failure artifacts are retained in the
  task's local evidence directory.
- Scoped ESLint passed for all five changed TS/TSX files. Web typecheck passed
  with the granted 6144 MiB Node heap. The staged i18n ratchet and E2E sleep
  ratchet passed; no public docs change was needed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py
  --all`, and `git diff --check` passed for the final artifacts.
