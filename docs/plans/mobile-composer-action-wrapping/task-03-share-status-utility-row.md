---
id: "03-share-status-utility-row"
title: "Share mobile status and utility rows"
status: done
wave: 3
depends_on:
  - "02-place-action-above-utilities"
plan: "plan.md"
requirements:
  - REQ-UI-COMPOSER-ACTION-WRAP-001
acceptance_criteria:
  - AC-UI-COMPOSER-ACTION-WRAP-001.1
  - AC-UI-COMPOSER-ACTION-WRAP-001.2
  - AC-UI-COMPOSER-ACTION-WRAP-001.3
  - AC-UI-COMPOSER-ACTION-WRAP-001.4
  - AC-UI-COMPOSER-ACTION-WRAP-001.5
  - AC-UI-COMPOSER-ACTION-WRAP-001.6
  - AC-UI-COMPOSER-ACTION-WRAP-001.7
system_design:
  - ../../specs/ui/system-design/composer-action-wrapping.md
---

# Task 03: Share mobile status and utility rows

## Summary

Let status icons and transcript utilities occupy one phone row when they fit.
Keep the workflow action above them. Use complete-control wrapping when space
is insufficient.

## In scope

`ChatStatusBar` composition and its status, workflow, and utility wrappers.
Preserve stable keyed controls across responsive order changes.
Add focused component and browser regressions. Update the existing public
phone-controls paragraph after implementation.

## Out of scope

Workflow and provider semantics, new copy, settings, persistence, passthrough,
Threads auto-hide CI placement, icon redesign, new animations, and PR publication.
Earlier work-order publication authorization does not apply to this task.

## Acceptance

1. A fitting phone status/utility set shares one row below the eligible
   workflow action. Dense sets wrap with contained, non-overlapping 44px targets.
2. Empty groups and hidden actions reserve no space. Keyboard order matches
   visual order. Existing actions and disclosure state survive responsive changes.
3. Desktop, workflow movement, terminal banners, and existing touch disclosures
   pass their focused regressions. Rendered screenshots match UI-03.

## UI preview

UI-03 excerpt from [the plan](plan.md#task-03-ui-preview).
Maps to AC-UI-COMPOSER-ACTION-WRAP-001.1 through .7.

| View | Composition above the composer |
| --- | --- |
| Phone, fitting | Workflow row; shared status-left and utilities-right row |
| Phone, dense | Workflow row; complete status and utility controls wrap |
| Phone, hidden action | Shared or wrapping controls, with no action spacer |
| Phone, moving | Disabled workflow row; shared or wrapping controls |
| Desktop | Status, utilities, workflow action in existing inline order |

Terminal notices precede the rows. Phone action order and touch targets are
structural. All controls remain in the existing composer allocation.

## Verification

Run commands from repository root. In a fresh worktree, install dependencies
once with `(cd apps && pnpm install --frozen-lockfile)`.

First add the fitting-row browser regression. Run it before production changes.
Record a failure on different row geometry, not a missing selector.

```bash
KANDEV_SERVER_HOST=127.0.0.1 PATH=/usr/local/go/bin:$PATH GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/chat/mobile-chat-status-bar.spec.ts -- --grep 'shares status and transcript controls when they fit' --retries=0
```

After implementation, run these commands sequentially:

```bash
pnpm --dir apps/web exec vitest run components/task/chat/chat-status-bar.test.tsx components/task/chat/chat-input-area.test.tsx components/task/workflow-move-proceed-button.test.tsx
pnpm --dir apps/web exec eslint components/task/chat/chat-status-bar.tsx components/task/chat/chat-status-bar.test.tsx components/integrations/change-request-status-chrome.tsx e2e/tests/chat/mobile-chat-status-bar.spec.ts e2e/tests/workflow/composer-action-layout-helpers.ts
pnpm --dir apps/web exec prettier --check components/task/chat/chat-status-bar.tsx components/task/chat/chat-status-bar.test.tsx components/integrations/change-request-status-chrome.tsx e2e/tests/chat/mobile-chat-status-bar.spec.ts e2e/tests/workflow/composer-action-layout-helpers.ts
NODE_OPTIONS=--max-old-space-size=6144 pnpm --dir apps/web run typecheck
KANDEV_SERVER_HOST=127.0.0.1 PATH=/usr/local/go/bin:$PATH GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --project chromium e2e/tests/workflow/workflow-step-move-overrides.spec.ts -- --retries=0
KANDEV_SERVER_HOST=127.0.0.1 PATH=/usr/local/go/bin:$PATH GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-chat-status-bar.spec.ts e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts -- --retries=0
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Include any extracted presentation file or changed helper in ESLint and Prettier
commands. Enable `CAPTURE_PR_ASSETS=1` on final browser runs and inspect fitting,
wrapped, and desktop captures. Record any change to these commands in Results.
Run `.github/scripts/pr-docs.cjs` `validateCoverage` on the final changed paths
and linked documents before marking done.

## Files likely touched

- `apps/web/components/task/chat/chat-status-bar.tsx`
- `apps/web/components/task/chat/chat-status-bar.test.tsx`
- `apps/web/components/integrations/change-request-status-chrome.tsx`
- `apps/web/e2e/tests/chat/mobile-chat-status-bar.spec.ts`
- `apps/web/e2e/tests/workflow/composer-action-layout-helpers.ts`
- `docs/public/mobile-remote-access.md`
- Paired requirements, design, this plan, and this work order.

## Dependencies

Completed Task 02. Reuse existing workflow and open-PR fixture helpers.
The companion mobile-task-action-bar tests provide terminal-notice coverage.

## Risks

- Moving controls between parents can reset disclosure state.
- Empty wrappers and intrinsic widths can force unnecessary rows.
- Long status chips and optional utility controls can cause clipping.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/composer-action-wrapping.md)
- [Design](../../specs/ui/system-design/composer-action-wrapping.md)
- [Mobile action bar](../../specs/ui/system-design/mobile-task-action-bar.md)
- Existing component identity test and mobile status-bar fixtures.

## Results

Implementation authorized and verified on 2026-10-20. Requirements are active,
the system design is current, and Task 03 is done.

The status, workflow, and utility groups retain stable keyed instances.
Phones place the workflow action above the shared wrapping row. Empty provider
wrappers consume no space. The CI trigger measured 40px in the first browser
check. A component slot now scopes its 44px minimum to the normal composer.
Standalone Threads chips and fine-pointer desktop sizing retain their behavior.

The component red gate failed because the workflow action followed the status
control on phones. The browser red gate measured a 100px difference between
status and utility centers. After the changes, both new fit/wrap cases passed.

| Check | Result |
| --- | --- |
| Three component suites from Verification | 61 passed |
| Targeted ESLint and Prettier, including the shared chip slot | Passed |
| Typecheck with `NODE_OPTIONS=--max-old-space-size=6144` | Passed |
| Focused fit/wrap browser cases | 2 passed, retries disabled |
| Full Chromium workflow suite | 6 passed, retries disabled |
| Mobile status/workflow suites | 9 passed initially; the remaining case passed after the measurement correction below |
| Public documentation tests and validator | 63 tests passed, 48 pages validated |
| Specification catalog, validator tests, and lint | 384 decisions, 1616 specs, 36 tests, all passed |
| Complete changed-path documentation coverage | Covered, zero errors |

### Local execution adjustments

The host requires `KANDEV_SERVER_HOST=127.0.0.1`; otherwise local configuration
binds the disposable backend to a different network interface. Chromium also
requires its installed Nix libraries in `LD_LIBRARY_PATH`. The local wrapper
`/tmp/mobile-composer-env.sh` sets that library path and the bounded Go options
from Verification. It does not change repository configuration.

The checkout name contains `mobile-`. The Chromium project's existing regular
expression therefore excludes every test under this checkout. The temporary
`/tmp/composer-playwright.config.ts` imports the existing configuration, retains
its fixtures and project settings, and replaces only that Chromium exclusion
with the filename glob `**/mobile-*.spec.ts`. It resolves `testDir`, `outputDir`,
and `globalSetup` to the existing E2E directory. Listing selected all six
expected desktop cases before execution.

The fresh managed Chromium command built the final assets before it encountered
that selection issue. The successful desktop command reused those assets:

```bash
. /tmp/mobile-composer-env.sh
CAPTURE_PR_ASSETS=1 pnpm --dir apps/web e2e:run --no-build --project chromium e2e/tests/workflow/workflow-step-move-overrides.spec.ts -- --config /tmp/composer-playwright.config.ts --retries=0
CAPTURE_PR_ASSETS=1 pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-chat-status-bar.spec.ts e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts -- --retries=0
```

The default Node heap exhausted during the first typecheck. The 6144MiB run
passed. No application type error occurred. Browser projects ran sequentially
with one worker. The phone fit, wrapped-phone, and desktop screenshots were inspected against
UI-03. Public how-to documentation now describes conditional row sharing.

### Resize measurement correction

One mobile workflow case measured a detached utility as a zero-sized box during
resize. Its screenshot showed the correct layout. The helper now selects visible
buttons and measures their bounds within one synchronous browser evaluation.
The row-order, containment, reachability, and target-size assertions are unchanged.
No production code changed during this correction.

Both users of the changed helper passed with retries disabled:

```bash
. /tmp/mobile-composer-env.sh
CAPTURE_PR_ASSETS=1 pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts -- --grep 'keeps the wrapped mobile workflow' --retries=0
CAPTURE_PR_ASSETS=1 pnpm --dir apps/web e2e:run --no-build --project chromium e2e/tests/workflow/workflow-step-move-overrides.spec.ts -- --config /tmp/composer-playwright.config.ts --grep 'keeps the workflow action right-aligned' --retries=0
```

Each command passed one test. Together with the unchanged passing cases, all
six desktop and ten mobile scenarios are verified. Final lint and formatting
include the changed helper. Final specification, coverage, and diff checks pass.
Changes remain unstaged and uncommitted. No delegation or PR publication occurred.

### PR review: GitLab touch targets

PR #4633 review found that the GitLab MR trigger did not expose the shared
change-request slot. The existing composer rule therefore left its target at
18px. The MR trigger attributes now expose that slot for both drawer and popover
variants. Standalone placement and fine-pointer desktop dimensions are unchanged.

The extended mobile MR browser case failed before the fix with an 18px target
against the 44px minimum. After a fresh managed build, all four selected browser
cases passed: shared row, wrapped row, GitLab touch disclosure, and GitLab narrow
fine-pointer/desktop sizing. The GitLab checks cover 320px, 393px, 767px, and
800px coarse-pointer viewports, center-point reachability, drawer content, focus
return, and compact desktop sizing. The existing 13 MR component tests passed.
Targeted ESLint, Prettier, and typecheck with the 6144MiB heap passed.

```bash
. /tmp/mobile-composer-env.sh
CAPTURE_PR_ASSETS=1 pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/gitlab/mobile-mr-status-chip.spec.ts e2e/tests/chat/mobile-chat-status-bar.spec.ts -- --grep 'shares status and transcript controls|wraps status and transcript controls|mobile GitLab MR status chip' --retries=0
pnpm --dir apps/web exec vitest run components/gitlab/mr-status-chip.test.tsx
pnpm --dir apps/web exec eslint components/gitlab/mr-status-chip-trigger.tsx e2e/tests/gitlab/mobile-mr-status-chip.spec.ts
pnpm --dir apps/web exec prettier --check components/gitlab/mr-status-chip-trigger.tsx e2e/tests/gitlab/mobile-mr-status-chip.spec.ts
NODE_OPTIONS=--max-old-space-size=6144 pnpm --dir apps/web run typecheck
```

The screenshot fixture uses a separate capture key for each test so both GitLab
viewport captures remain in the manifest. Both GitLab browser cases passed again
after that fixture change, with retries disabled. Publication and current-head CI/review
verification remain pending until the remediation is pushed.

### PR review: Azure DevOps link targets

Claude's summary and inline review found the same missing slot on the Azure
DevOps PR link. Its browser regression measured a 28px target before the fix.
The link now exposes the shared slot through its existing Button wrapper.
The browser case checks 44px targets and center-point reachability at 320px
with touch and fine pointers, activates the link against a mocked external
destination, and verifies compact sizing at 1280px. It restores repository
metadata after the case.

The fresh frontend build and all five selected browser cases passed with
retries disabled: the two shared/wrapped composer cases, two GitLab cases, and
the Azure DevOps case. Targeted ESLint, Prettier, and typecheck with the 6144MiB
heap passed. The Azure case passed again after waiting for response completion
and tooltip dismissal before capture. The existing public
mobile documentation already describes the behavior for all status controls;
no new user-facing copy or contract is introduced.

```bash
pnpm --dir apps/web run build:e2e
. /tmp/mobile-composer-env.sh
CAPTURE_PR_ASSETS=1 pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/integrations/mobile-azure-devops.spec.ts e2e/tests/gitlab/mobile-mr-status-chip.spec.ts e2e/tests/chat/mobile-chat-status-bar.spec.ts -- --grep 'composer PR links|shares status and transcript controls|wraps status and transcript controls|mobile GitLab MR status chip' --retries=0
```

Fresh phone and desktop captures cover all three providers. Publication and
current-head CI/review verification remain pending until the fix is pushed.
