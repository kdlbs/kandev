---
id: "01-preserve-current-form"
title: "Preserve current Sentry instance form"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SENTRY-FORM-LIFETIME-001
acceptance_criteria:
  - AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.1
  - AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.2
  - AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.3
  - AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.4
system_design:
  - ../../specs/integrations/system-design/sentry-instance-form-lifetime.md
---

# Task 01: Preserve Current Sentry Instance Form

## Summary

Guard section closure and dirty updates by local opening ownership. Preserve
accepted writes, refresh and feedback, plus independent mounted registration.

## In scope

Sentry section/form callbacks and mounted contributor identity; real-consumer
fetch-deferred regression matrix and desktop/touch browser evidence.

## Out of scope

Jira, all other integrations, task rename, shared coordinator, Query, backend,
schema, auth and credential policies, transport cancellation and durable queues.

## Acceptance

- Actual consumer RED fails only after old ACK, followed by minimal GREEN.
- Newer fields/selection/dirty registration survive; current save, cancel,
  failure/retry and truthful old feedback work.
- Exact checks and normal hooks pass; committed clean HEAD and all owned
  processes terminal, then stop before push/PR for parent validation.

## ASCII UI preview

UI-01: Settings > Sentry, after cancelling a pending save and opening a new form.
Shared desktop/phone structure; the phone uses the existing dedicated settings
page, single content scroller, and fixed safe-area-aware Save surface.

```text
Before older ACK: [New instance: name | URL | token] [Test] [Cancel]
Current defect:   [instance cards]                  (new form disappears)
After repair:    [New instance: name | URL | token] [Test] [Cancel]
Fixed viewport:                         [Reset] [Save changes]
```

The fields, selection and dirty state survive. Spacing is illustrative; no new
layout, copy or touch geometry is proposed. Maps to AC .1 and .4 and the focused
consumer/browser checks below.

[Combined preview](plan.md#ascii-ui-preview).

## Verification

Use Node 24 on PATH; pnpm commands start after `pnpm install --frozen-lockfile`
from `apps/`. Browser runs use one worker, zero retries and owned E2E fixtures.

```bash
(cd apps/web && pnpm exec vitest run components/sentry/sentry-form-lifetime.test.tsx components/sentry/sentry-settings.test.tsx components/sentry/sentry-instance-form.test.tsx)
(cd apps/web && pnpm exec eslint components/sentry/sentry-settings.tsx components/sentry/sentry-instance-form.tsx components/sentry/sentry-form-lifetime.test.tsx e2e/tests/integrations/sentry-form-lifetime-helpers.ts e2e/tests/integrations/sentry-form-lifetime.spec.ts e2e/tests/integrations/mobile-sentry-form-lifetime.spec.ts --max-warnings 0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/integrations/sentry-form-lifetime.spec.ts -- --retries=0
GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/integrations/mobile-sentry-form-lifetime.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require(process.cwd() + '/.github/scripts/pr-docs.cjs');
const base = 'f61b11093c7f75465e64b7ad8cb78a9d9910b95b';
const paths = [...new Set([...cp.execFileSync('git', ['diff', '--name-only', base], {encoding:'utf8'}).trim().split('\n'), ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], {encoding:'utf8'}).trim().split('\n')])].filter(Boolean);
const artifacts = ['docs/plans/sentry-instance-form-ownership/plan.md','docs/plans/sentry-instance-form-ownership/task-01-preserve-current-form.md','docs/specs/integrations/requirements/sentry-instance-form-lifetime.md','docs/specs/integrations/system-design/sentry-instance-form-lifetime.md'];
const result = validateCoverage({changedFiles: paths.map(filename=>({filename,status:'modified'})),fileContents:Object.fromEntries(artifacts.map(p=>[p,fs.readFileSync(p,'utf8')]))});
console.log(JSON.stringify(result,null,2));
if (!result.ok) process.exitCode=1;
NODE
git diff --check
```

## Files likely touched

- `apps/web/components/sentry/sentry-settings.tsx`
- `apps/web/components/sentry/sentry-instance-form.tsx`
- `apps/web/components/sentry/sentry-form-lifetime.test.tsx`
- `apps/web/e2e/tests/integrations/sentry-form-lifetime-helpers.ts`
- `apps/web/e2e/tests/integrations/sentry-form-lifetime.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-sentry-form-lifetime.spec.ts`
- `docs/public/integrations.md` only if public workflow clarification is useful.

## Dependencies

None. The sibling Jira task owns separate paths and artifacts.

## Risks

Same-identity and identical-text reopening; effect cleanup clearing replacement
dirty state; coordinator failure markers reused by logical instance ID.

## Parallelism

sequential

## Inputs

Paired requirement/design; shared manual-save contract and ADR 0046; existing
Sentry settings/form tests, API and desktop page object. No new ADR is warranted.

## Results

- Frozen pnpm 9.15.9 install under Node 24.18.0: passed.
- Causal initial RED: 10 intended failures, 7 passing controls. The failures
  expose replacement closure, reused failure state and identical-text ownership.
- Poll-remount RED: one intended failure after list removal, restoration and new
  field entry; the old ACK then removes the remounted form. Earlier fixture-only
  errors are separate from this behavioral evidence.
- Final focused Vitest: 3 files, 35 tests passed (23 lifetime cases and 12 existing
  form/section controls). Covers success/failure, same-instance and identical-text
  reopen, unmount/remount, polling remount, next Save/Cancel and stale test results.
- Desktop production browser: 2 passed, 1 worker, zero retries, fresh build.
  Native-tap emulated Pixel 5: 2 passed, 1 worker, zero retries, same fresh assets.
  Both use the real backend with mock Sentry, hold only an accepted write's
  response, assert backend persistence and exact newer fields/selection, and
  exercise next Save or Cancel. No real provider or operator data was used.
- Changed-file ESLint: passed with zero warnings. Web typecheck with 8 GiB heap
  and rich-output catalog check: passed. i18n check and new-code ratchet: passed.
- Documentation catalog: passed (396 decisions, 1705 specifications).
  Specification-linter tests: 36 passed. Full specification lint: passed.
  Public-doc validator tests: 63 passed. Public docs: 54 pages validated.
  PR documentation coverage: covered with the new owning requirement/design.
- Public documentation: clarified submitted-write cancellation under Sentry in
  `docs/public/integrations.md` (existing integration how-to/reference page).
- External receipts: `../loop-receipts/attempt25/` outside the repository.
  Normal commit-hook, final HEAD/diff and terminal-process evidence is recorded
  there at the parent-validation checkpoint, before any push or PR.
- Refreshed main: `5aa31bec1a8a5e9cdb679811b02aa3e7b78271c2`, one unrelated
  agent-discovery commit beyond tested base `f61b11093c7f75465e64b7ad8cb78a9d9910b95b`.
  No Sentry or settings-coordinator source changed between those heads.
  Refreshed all 69 open PRs and paginated actual changed files: only #1512 touches
  these source targets, at `a8655ad92faa050a68140a904500fc99ebc0a04b`; it retains
  unconditional closure and does not change the instance form. This is a bounded
  target-overlap finding, not a correctness review of unrelated PRs.
- Limits: phone behavior is browser device emulation; no physical device,
  OS keyboard, real provider, cross-tab or transport exactly-once claim.
  Browser screenshot capture was disabled; behavior and selection assertions
  ran on the shipped production surface. No rendered layout changed.

### Delivery validation correction

The fresh current-main worktree exposed a type-only test assertion error:
Testing Library role lookups do not accept Playwright's `exact` option. Removed
that option without changing the asserted accessible name or product behavior.
Focused unit checks passed 34/34 and the 8 GiB web typecheck passed after the
correction; final-head
CI/review and combined-result evidence remain external delivery receipts.

### Save availability during list refresh

Greptile identified that awaiting the post-write list GET held the shared Save
action busy for a replacement draft. A causal real-consumer test holds that GET
after the accepted write succeeds: RED leaves the current action in Saving;
the one-line correction starts the existing error-handled refresh independently.
The focused suites pass 35/35, including this pending-refresh control. Browser,
static, native CI and exact-head review results are recorded in the external
delivery receipts after the correction. No shared coordinator behavior changed.
