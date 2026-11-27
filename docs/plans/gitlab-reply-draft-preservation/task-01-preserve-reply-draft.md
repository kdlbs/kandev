---
id: "01-preserve-reply-draft"
title: "Preserve the latest discussion reply draft"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.11
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.12
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.13
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.14
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.15
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
---

# Task 01: Preserve the latest discussion reply draft

## Summary

Correct successful reply settlement in the existing discussion component.
Author independent tests against the rendered section with the production
hooks/API and fetch-only mocks, proving later unsent work survives.

## In scope

- Capture raw submitted reply and clear on success only through a functional
  latest-value comparison; keep transport trimming and existing nonblank gate.
- New `mr-discussions-section.reply-draft.test.tsx` follows the complete named
  [scenario matrix](plan.md#tests), with unchanged context-helper controls.
- Update exactly the owning requirement/design and this two-file package with
  actual status/results after execution.

## Out of scope

Production hooks/API/panel changes, global frameworks, revision histories,
pending/lifecycle/source/loading policy, identity switching, reload/unmount
persistence, backend/public API/layout/copy expansion, other provider/actions,
browser/build/Go/E2E execution and broad passing-suite replay.

## Acceptance

1. Independent permanent RED reproduces B lost after actual successful POST
   and feedback refresh; unchanged success and failure controls pass with the
   production section unchanged. Only global fetch is mocked.
2. The sole production correction satisfies AC .11-.15 through exact raw
   textarea assertions, actual transport payloads, refresh/failure/retry and
   distinct-discussion/reorder cases in the plan matrix.
3. Targeted GREEN, changed-file static and documentation gates pass with actual
   original-process join/group receipts. Local completion remains separate
   from hosted review, CI and ROOT's later serial merge grant.

## TDD and fixture discipline

After ROOT's explicit later implementation INTERRUPT, mark this work order
in_progress and author the new suite independently. Do not copy/import/replay
the protected proof. Mount real `MRDiscussionsSection`, `TooltipProvider`,
`ToastProvider`, locale setup, `useMRFeedback`, `useMRActions` and
`createMRDiscussionNote`. The minimal consumer harness matches audited
`MRDetailContent` / `discussionHandlers` identity and reply binding.

Mock only global fetch, with explicit POST and feedback GET deferred responses;
route files/commits normally. Assert POST method, workspace/expected-host query
and project/IID/discussion/body. Assert textarea is enabled and Reply disabled
while POST is held. Resolve or reject POST, then independently hold or settle
refresh; assert actual textarea value, refreshed note and success/error feedback.
Use two discussion IDs and scoped selectors for isolation/reorder evidence.
Never mock hook/API/component functions or reproduce the clearing predicate in
tests. Settle every held response, unmount and clean timers without arbitrary
sleeps. Setup/selection/resource failures are not causal RED.

Nearby production precedent is GitLab settings' functional current-value
comparison; use raw snapshot rather than its trimmed-token comparison.
Routine owned causal fixture/lint repairs need no separate approval. Unknown,
resource, timeout, transport and out-of-scope failures checkpoint ROOT without
automatic retries, global setting changes, cache wiping or foreign kills.

## Mobile and documentation

Use the [pure-state and public-doc assessment](plan.md#e2e-and-mobile-assessment).
Shared local reply state changes no markup, interaction geometry, navigation,
scrolling, copy or breakpoint logic. The real rendered component regression
satisfies the mobile pure-state exception; no layout preview or Playwright
expansion is required. Public guidance remains accurate.

## Verification

Run from repo root using `/bin/bash`, login:false, existing Node24 PATH and
pinned pnpm9.15.9. Package commands below are LATER execution recipes, not
permission to run during design. One exclusive global local-heavy release from
ROOT is required before install, Vitest, lint, typecheck, i18n or hooks.
Run commands serially. Before each retain argv/log, UTC start/cutoff, native
PID/PGID; retain every original session/start chunk and actually join it,
record actual exit/join chunk and fresh group absence before the next command.

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
# One conditional install only after ROOT's explicit global-heavy release.
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 6m corepack pnpm@9.15.9 install --frozen-lockfile)
fi

# RED before the production correction: one causal regression + two controls.
(cd apps/web && timeout --signal=TERM --kill-after=10s 6m corepack pnpm@9.15.9 exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism components/gitlab/mr-discussions-section.reply-draft.test.tsx -t 'preserves continued draft after successful post and refresh|clears unchanged successful raw draft|retains unchanged draft after post failure')

# GREEN: new matrix and existing affected section context controls only.
(cd apps/web && timeout --signal=TERM --kill-after=10s 6m corepack pnpm@9.15.9 exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism components/gitlab/mr-discussions-section.reply-draft.test.tsx components/gitlab/mr-discussions-section.test.ts)
(cd apps/web && timeout --signal=TERM --kill-after=10s 6m corepack pnpm@9.15.9 exec eslint --max-warnings=0 components/gitlab/mr-discussions-section.tsx components/gitlab/mr-discussions-section.reply-draft.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 6m corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 6m corepack pnpm@9.15.9 run i18n:ratchet)

# Cheap design checks; also repeat after implementation documentation changes.
timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=10s 60s git diff --check
git status --short
```

Use the normal typecheck script including its existing pretypecheck generation;
no direct-tsc shortcut or harness/config changes. Include any additionally
changed owned test in targeted GREEN and ESLint. New tests belong to
browser-locales; wrong-project NO_TESTS is never a passing or causal result.
Do not rerun passing gates without a relevant correction or unresolved concern.

The cheap repository documentation-reference preflight runs at design and
after implementation with Node24 (60s TERM/kill10). Read the four package
files into `fileContents`, derive actual changed files from raw `git diff
--name-only -z HEAD` and `git ls-files --others --exclude-standard -z`, then
call `.github/scripts/pr-docs.cjs`'s `validateCoverage` and assert `ok: true`
and `errors: []`. During design assert exactly those four docs changed and
separately label a hypothetical planned production-section trigger; never
report planned source as actual changed code. After implementation use actual
changes. Also verify every referenced REQ/AC exists, design declares the REQ,
and work-order design is listed by the manifest. Publish final four-document
SHA256 values, full-file readback and cheap check results in the live task plan.

```bash
timeout --signal=TERM --kill-after=10s 60s /home/jcfs/.nvm/versions/node/v24.18.0/bin/node <<'NODE'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/integrations/requirements/gitlab-integration.md',
  'docs/specs/integrations/system-design/gitlab-integration-02.md',
  'docs/plans/gitlab-reply-draft-preservation/plan.md',
  'docs/plans/gitlab-reply-draft-preservation/task-01-preserve-reply-draft.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [...tracked.map(filename => ({ filename, status: 'modified' })), ...untracked.map(filename => ({ filename, status: 'added' }))];
const actual = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ scope: 'actual changed checkout', result: actual }));
assert.equal(actual.ok, true);
assert.deepEqual(actual.errors, []);
const section = 'apps/web/components/gitlab/mr-discussions-section.tsx';
if (!tracked.includes(section)) {
  assert.deepEqual([...tracked, ...untracked].sort(), paths.slice().sort());
  const planned = validateCoverage({ changedFiles: [...changedFiles, { filename: section, status: 'modified' }], fileContents });
  console.log(JSON.stringify({ scope: 'design only: planned section trigger', result: planned }));
  assert.equal(planned.ok, true);
  assert.deepEqual(planned.errors, []);
}
for (const content of Object.values(fileContents)) {
  assert(content.includes('REQ-INTEGRATIONS-GITLAB-INTEGRATION-001'));
  assert(!/[\t ]+$/m.test(content));
  assert(content.endsWith('\n'));
}
for (let i = 11; i <= 15; i++) {
  const ac = `AC-INTEGRATIONS-GITLAB-INTEGRATION-001.${i}`;
  assert(fileContents[paths[0]].includes(`**${ac}:**`));
  assert(fileContents[paths[3]].includes(ac));
}
for (const p of paths.slice(2)) assert(fileContents[p].includes('../../specs/integrations/system-design/gitlab-integration-02.md'));
NODE
```

## Files likely touched

- `apps/web/components/gitlab/mr-discussions-section.tsx` (sole production file).
- `apps/web/components/gitlab/mr-discussions-section.reply-draft.test.tsx` (new independent tests).
- Existing `mr-discussions-section.test.ts` only if a necessary owned fixture repair arises.
- Exactly this work order, sibling plan, owning requirement and part-2 design.

## Dependencies

No prior work order. Dependencies currently absent; defer the one conditional
frozen apps install until ROOT's heavy release. Design checkpoint must end
before source/permanent tests, awaiting concrete ROOT review and explicit
same-primary implementation INTERRUPT. Preserve worktree/deps/shared resources,
foreign processes and protected proof. This local work order grants no merge.

Later delivery uses normal active commits/hooks without bypass/amend; freeze
published SHA except actual corrections. All-original-joined heavy RETURN
precedes one original 90m scripts/pr-await (GNU91m kill10, cadence60); join
original before ROOT replacement. The live task plan preserves canonical
repository/PR linking with all five automation booleans false confirmed by GET,
exact-head hosted checks and substantive CodeRabbit App347564 review,
ROOT-only retry budget and separate serial MERGE grant. ROOT owns archive,
proof release and refill. Notifications are optional, never progress gates.

## Risks

Raw-versus-trimmed comparison and refresh settlement ordering are the main
implementation risks. Faithful real-consumer tests must distinguish POST
success from refresh failure and use stable discussion IDs, including reorder.

## Parallelism

`sequential`. Same primary; no delegation/tasks/sessions/tabs/model switch.

## Inputs

- [Requirement AC .11-.15](../../specs/integrations/requirements/gitlab-integration.md#discussion-reply-drafts).
- [Local state and consumer design](../../specs/integrations/system-design/gitlab-integration-02.md#discussion-reply-draft-settlement).
- [Accepted evidence](plan.md#baseline-and-accepted-evidence).
- Scoped `apps/web/AGENTS.md`, `/tdd` and `/mobile-parity` after release.

## Results

Local product implementation and targeted validation completed after ROOT's
reviewed-package implementation/delivery and exclusive global local-heavy83
release. All task-defined local documentation gates passed. Status done records
local implementation only; hosted review, CI and merge remain pending.
No merge or hosted retry is authorized.

Independent RED against unchanged production blob
`b6ebd9e75da353e9609a8fb39918f30471557b58` failed only the continued-draft
textarea assertion after actual POST and feedback refresh succeeded. Both
unchanged-success and POST-failure controls passed; ten other cases were
deselected. The original RED process was actually joined before production.
The section's sole correction captures raw submitted text and conditionally
clears the latest state through a pure functional setter.

Targeted GREEN passed all 15 tests in two files, including all 13 new cases in
the plan matrix. The real section/providers/hooks/API are exercised with only
fetch transport mocked. Two owned fixture ESLint warnings were repaired by
splitting describe groups and using a shared success-notice constant; affected
GREEN and zero-warning changed-file ESLint passed after the repair. Normal
project typecheck (including pretypecheck generation) and i18n:ratchet passed.
One final removal of extra blank lines prepares the fixture for the normal
formatting hook and does not change test behavior.

| Original command | Native session; start/join chunks | Actual exit; PID/PGID absent |
| --- | --- | --- |
| 01 frozen install | 8930; b206ef/647d89 | 0; 4095072 |
| 02 owned fixture formatting | synchronous; 89523c/89523c | 0; 4103118 |
| 03 reviewed causal RED selection | 78633; e614b7/541cc0 | 1 expected; 4103858 |
| 04 targeted GREEN | 46767; 1bbf2a/0b2406 | 0, 15 PASS; 4105196 |
| 05 initial changed-file ESLint | 43928; 0d63a1/f2228b | 1 owned fixture warnings; 4106602 |
| 08 affected GREEN after lint repair | 94111; b3ff03/5e9412 | 0, 15 PASS; 4107890 |
| 09 affected ESLint after lint repair | 40636; 319eb6/0237df | 0; 4108975 |
| 06 normal typecheck | 29446; 062f61/866de2 | 0; 4110548 |
| 07 i18n:ratchet | 40324; 338cf7/36ae38 | 0; 4115212 |
| 10 documentation gates | 73298; 9593f1/64a8bd | 0; 4119603 |

Documentation gates passed: catalog validated 364 decisions and 1,456
specifications, all 36 validator tests passed, full spec lint passed, actual
changed-file documentation coverage returned `ok: true`, `errors: []`, and
REQ/AC/design/manifest references plus whitespace passed. The actual change
inventory is exactly four documents, one production section and one new suite.

Exact argv/logs, UTC start/cutoff/completion, actual process-group scans and
native metadata are preserved in `/tmp/kandev-child83-exec-20261008` receipt
JSON and the live task plan. Commands ran serially using Node24.18.0,
pnpm9.15.9, Node4GiB and browser-locales/one worker. No protected-proof replay,
delegation, new task/session, browser/build/Go or unrelated suite ran.
