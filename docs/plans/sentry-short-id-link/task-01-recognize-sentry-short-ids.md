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
  - AC-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001.4
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
provider canonical title and dialog success/close. The relink correction
may extend only `buildLinkedIssueTitle`'s leading-prefix recognition for
supported Sentry base32 syntax. No Jira or Linear input parsing, unrelated
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
   Sentry → real input → Save. Exercise a short-ID URL with a hyphenated host
   or organization as well as a direct base32 ID and numeric control. Seed
   unique `WEB-1A` and numeric control issues
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
- A review regression showed a hyphenated host or organization shadowing a
  short-ID URL. Extraction now prioritizes and fully validates the issue-path
  segment, including empty or invalid segments, without requiring a digit.
- Final focused Vitest run passed 13 tests; scoped ESLint passed with zero warnings.
- Final managed desktop and phone runs passed 3 tests each, with one worker,
  retries disabled, and a fresh build for each. Captures show the actual
  rendered linked title; provider data and workspaces were isolated fixtures.
- No new UI copy or public documentation change was needed. No live Sentry or
  physical-device verification is claimed.

## Review remediation: replace a base32 prefix when relinking

The existing task
external-reference requirement already requires prefix replacement. The shared
`buildLinkedIssueTitle` currently removes only decimal suffixes, so relinking
`WEB-1A: Fix login` to `WEB-1B` produces a stacked prefix and can truncate
original title text. Extend only this builder's anchored, colon-delimited
recognition to the accepted Sentry base32 suffix alphabet and decoder aliases;
preserve existing numeric Jira/Linear/Sentry cases, non-prefix titles, empty
titles and code-point-aware 60-character truncation. Do not modify unrelated
global parsers, provider input validation, API routing, query/store ownership,
dialog lifetime, or user-facing copy. A prefix matching this syntax has no
stored provider provenance, so the builder cannot distinguish a natural title
with the same complete prefix shape from an external issue prefix.

Before production changes, independently add a failing test to
`apps/web/components/task/task-external-link-utils.test.ts` for base32 prefix
replacement, including letter-only and alias suffixes, numeric Jira/Linear
controls, an unrelated title and truncation boundary. Add an actual rendered
`TaskExternalLinkDialog` regression in
`apps/web/components/task/task-sentry-short-id-link.test.tsx` proving the scoped
provider's canonical new key is passed to the real title builder and the update
uses `WEB-1B: Fix login` when the current title is `WEB-1A: Fix login`; do not
mock the builder. Confirm RED is the stacked title, not fixture setup, then
make the minimum correction and run GREEN.

Extend the existing managed desktop and phone specs with independent relink
scenarios: use the task menu and real input to link `WEB-1A`, then relink to
`WEB-1B`; seed both issues through the task-owned Sentry mock instance. Observe
both correlated scoped GETs and assert the visible and persisted title after
each save, including exactly one prefix after the second. Keep existing numeric
and URL controls independently reported. No intercepted success, global mock
dataset edit, live provider write, or shared developer instance. Capture and
inspect fresh desktop and phone outcomes for the final PR head.

After the later lease, run sequentially from the repository root, with the
pinned PATH and `GOTOOLCHAIN=local` shown above. Each quiet invocation has its
own owned directory; join each actual handle and record exit before the next.
Run the first two commands on the added tests before changing production, then
the remaining commands after the correction. No reinstall is needed.

```bash
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/relink-unit-red scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project node components/task/task-external-link-utils.test.ts --maxWorkers=1 --no-file-parallelism
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/relink-dialog-red scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser-locales components/task/task-sentry-short-id-link.test.tsx --maxWorkers=1 --no-file-parallelism
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/relink-unit-green scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project node components/task/task-external-link-utils.test.ts --maxWorkers=1 --no-file-parallelism
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/relink-dialog-green scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser-locales components/task/task-sentry-short-id-link.test.tsx --maxWorkers=1 --no-file-parallelism
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/relink-desktop scripts/run-quiet e2e --summary -- env CAPTURE_PR_ASSETS=1 GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/sentry-short-id-link.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/relink-phone scripts/run-quiet e2e --summary -- env CAPTURE_PR_ASSETS=1 GOMAXPROCS=2 GOFLAGS=-p=2 pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-sentry-short-id-link.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/tasks/accept-sentry-short_zfbey007/receipts/relink-lint scripts/run-quiet lint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/task/task-external-link-utils.ts components/task/task-external-link-utils.test.ts components/task/task-sentry-short-id-link.test.tsx e2e/tests/task/sentry-short-id-link-helpers.ts e2e/tests/task/sentry-short-id-link.spec.ts e2e/tests/task/mobile-sentry-short-id-link.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The earlier local results above remain historical evidence for the prior head;
the relink remediation was separately verified as follows:

- Builder RED reproduced stacked prefixes and lost title text; rendered-dialog
  RED produced the same stacked title through the real callback.
- Builder GREEN passed 10 tests and dialog GREEN passed 14; scoped ESLint passed.
- Managed desktop and phone each passed 4 cases with one worker, retries 0,
  and fresh builds. Both scoped mock-provider responses and rendered/persisted
  titles were asserted for the two-step relink; captures show the resulting
  single prefix. These tests do not claim live Sentry or physical-device coverage.
