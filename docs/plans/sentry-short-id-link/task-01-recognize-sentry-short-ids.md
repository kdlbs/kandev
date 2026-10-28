---
id: task-01
title: Recognize Sentry short IDs
status: done
wave: 1
depends_on: []
plan: ./plan.md
requirements:
  - REQ-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001
acceptance_criteria:
  - AC-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001.1
  - AC-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001.2
  - AC-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001.3
system_design:
  - ../../specs/integrations/system-design/sentry-short-id-link.md
---

# Task 01: Recognize Sentry short IDs

## Summary and scope

Repair the Sentry-specific suffix recognition and extraction in
`apps/web/components/sentry/sentry-issue-common.tsx`. In
`task-external-link-dialog.tsx`, recognize the existing numeric issue URL before
extracting short IDs, so a hyphenated organization slug is not mistaken for an
ID. Keep the existing slug
syntax, case normalization, numeric URL fallback, instance/workspace lookup,
provider canonical title and dialog success/close. No Jira, Linear, global
parser, backend production, API, query/store, layout or copy changes. Existing
`PROJ-123` placeholder remains useful. Add independently authored tests, not
copies of qualification or private discovery fixtures.

## Acceptance conditions

1. Before production changes, demonstrate a causal RED in a new rendered
   `TaskExternalLinkDialog` regression: real state, toast and i18n providers,
   only availability and external API boundaries mocked; submit `WEB-1A` and assert the expected lookup, update and close; observe
   the current validation failure and absent lookup as its RED, while numeric short-ID and
   numeric-URL controls prove scoped lookup, canonical task title, and close.
   Include a numeric URL with a hyphenated organization slug.
   No unavailable jest-dom matcher assumptions. Add focused cases for both
   exported patterns and extraction: all encoder characters and decoder aliases,
   lowercase normalization, project slug separators, numeric IDs, complete
   invalid suffix and boundary cases, including no partial prefix acceptance.
2. After the minimum correction, the independent regressions pass. The returned
   canonical short ID is used for the task title, even for a numeric URL; failed
   validation or provider resolution cannot update the task. No new copy is
   expected; if necessary, add all seven locales and run the i18n gates.
3. Independently authored managed desktop and phone Playwright tests run RED
   before the correction and GREEN after, using native task menu → Link →
   Sentry → real input → Save. Seed unique `WEB-1A` and numeric control issues
   in an isolated Sentry instance via `/api/v1/sentry/mock/issues?instanceId=...`
   (`issues` payload); add a narrow `mockSentryAddIssues` API helper if needed.
   Observe the actual correlated scoped GET and mock-provider response, then
   assert the visible task title and server-persisted title, successful close,
   and numeric control. The base32 and numeric cases must be independent tests
   so the numeric control is reported even when the base32 case fails; retain
   each result and the joined run exit. Do not edit passing source or fabricate
   a success fixture to obtain RED, or intercept and fabricate a response. Use
   a managed isolated host, one worker per shard, no retries, owned
   workspace/fixture cleanup; capture actual desktop and phone rendered behavior
   for the PR. ROOT's private discovery sources and fixtures are forbidden.

## Files and dependencies

- `apps/web/components/sentry/sentry-issue-common.tsx` (production).
- `apps/web/components/task/task-external-link-dialog.tsx` (numeric URL precedence only).
- New `apps/web/components/task/task-sentry-short-id-link.test.tsx` (component
  and pattern tests; defaults to `browser-locales`). Existing
  `task-external-link-dialog.test.tsx` is mapped to `browser`; leave its query
  wrapper and adjacent ownership code alone.
- New `apps/web/e2e/tests/task/sentry-short-id-link.spec.ts` and
  `mobile-sentry-short-id-link.spec.ts`, with a local shared helper if useful;
  `apps/web/e2e/helpers/api-client.ts` only if the scoped mock seeding helper is
  needed. Existing `external-link-ownership-helpers.ts` shows phone entry and
  cleanup patterns; do not reuse private qualification sources.
- Task action and provider contract documents linked from the plan. The host
  runner owns disposable backend and provider data. No external Sentry writes.

## Verification after separate release

Acquire the single global HEAVY lease first. Join every native process handle
and record actual exit; do not infer completion from another run. Run from the
repository root, sequentially. Each quiet command has its own owned log
folder because `run-quiet` prunes old logs there. Replace no paths: these are
specific to this task. The install is once per fresh worktree, after the lease.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/install scripts/run-quiet install -- pnpm --dir apps install --frozen-lockfile
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/vitest-red scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser-locales components/task/task-sentry-short-id-link.test.tsx --maxWorkers=1 --no-file-parallelism
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/desktop-red scripts/run-quiet e2e --summary -- env GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/sentry-short-id-link.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/phone-red scripts/run-quiet e2e --summary -- env GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-sentry-short-id-link.spec.ts -- --workers=1 --retries=0
```

Inspect the failing assertion and passing controls, and retain the actual exits;
setup errors are not causal RED. Make the minimum fix, then:

```bash
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/vitest-green scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser-locales components/task/task-sentry-short-id-link.test.tsx --maxWorkers=1 --no-file-parallelism
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/desktop-green scripts/run-quiet e2e --summary -- env GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/sentry-short-id-link.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/phone-green scripts/run-quiet e2e --summary -- env GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-sentry-short-id-link.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/eslint scripts/run-quiet lint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/sentry/sentry-issue-common.tsx components/task/task-external-link-dialog.tsx components/task/task-sentry-short-id-link.test.tsx e2e/tests/task/sentry-short-id-link.spec.ts e2e/tests/task/mobile-sentry-short-id-link.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Include any new shared E2E helper or modified API helper in scoped ESLint. Run
canonical typecheck only if the implemented change requires it. If new copy is
unavoidable, additionally run `pnpm --dir apps/web run i18n:check` and
`pnpm --dir apps/web run i18n:ratchet` with separate owned quiet directories.
Record outcomes and promote the draft spec only after implementation meets it.

The phone GREEN command above performs a fresh build. It may use `--no-build`
only when recorded fingerprints establish that the backend and web build
inputs, tools, and configuration are unchanged between the successful desktop
GREEN build and the phone run. Retain the before/after fingerprint receipts with
the desktop build and joined run outcomes; otherwise use the fresh-build command
as written. A missing or mismatched receipt never justifies asset reuse.

## Risks

Project slugs contain hyphens, so suffix boundary and greediness need real test
cases, not a partial match. Alias acceptance passes identity to the provider;
it must not invent a canonical value. Evidence must distinguish mock-provider
browser tests from live Sentry and physical-device testing.

## Results

- Independent component RED reproduced base32 rejection with numeric short-ID
  and URL controls passing. Separate REDs exposed a hyphenated organization
  slug shadowing a numeric URL and an invalid `WEB-1A-U` truncating to `WEB-1A`.
- Final focused Vitest run passed 9 tests; scoped ESLint passed with zero warnings.
- Final managed desktop and phone runs passed 2 tests each, with one worker,
  retries disabled, and a fresh build for each. Captures show the actual
  rendered linked title; provider data and workspaces were isolated fixtures.
- No new UI copy or public documentation change was needed. No live Sentry or
  physical-device verification is claimed.
