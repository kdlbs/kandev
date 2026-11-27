---
id: "01-preserve-comment-draft"
title: "Preserve the next task comment"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMMENT-DRAFT-001
acceptance_criteria:
  - AC-TASKS-COMMENT-DRAFT-001.1
  - AC-TASKS-COMMENT-DRAFT-001.2
  - AC-TASKS-COMMENT-DRAFT-001.3
  - AC-TASKS-COMMENT-DRAFT-001.4
  - AC-TASKS-COMMENT-DRAFT-001.5
  - AC-TASKS-COMMENT-DRAFT-001.6
  - AC-TASKS-COMMENT-DRAFT-001.7
  - AC-TASKS-COMMENT-DRAFT-001.8
system_design:
  - ../../specs/tasks/system-design/comment-draft-preservation.md
---

# Task 01: Preserve the next task comment

## Summary

Apply the exact raw snapshot completion rule in the existing TaskChat composer.
Prove the reported lost-work regression through actual rendered TaskChat and
comment HTTP transport, preserving normal success, failure, and admission.

## In scope

- `ChatInput.handleSubmit` success clearing only.
- Independently authored actual rendered transport regression suite and
  targeted existing synchronization/prompt-delivery controls.
- Exact targeted validation and accurate package/task-plan lifecycle results.

## Out of scope

- Navigation/task/session ownership, unmount feedback, persistence, API,
  transport/backend/events, other composers, generic state frameworks.
- Layout, copy, controls, touch, scrolling, navigation, utility/file redesign.
- Replay/copy/import/edit/chmod/delete of ROOT proof; permanent tests or
  production edits during the design turn.
- Extra work orders, agents/tasks/tabs/sessions, browser/build/broad tests,
  moving-main rebase or synthetic merge tests.

## Acceptance

1. The completion rule uses raw snapshot equality through existing
   `setInputAndSync`; current text and ref remain synchronized. No unrelated
   production behavior or shared helpers change.
2. The rendered regression matrix below covers all linked ACs against actual
   TaskChat/providers/store/API, with only fetch transport mocked; retained text,
   request content/counts, callbacks, feedback, and admission are observed.
3. Every required affected-path check passes with original joined execution
   evidence. Update work order/plan results and only then promote the paired
   requirement to `active`, design to `current`, and plan to `implemented`.
   Delivery/merge still obey the separate ROOT gates in the manifest.

## ASCII UI preview

**UI-01: Existing task comment composer** (shared desktop/phone),
[full preview](plan.md#ascii-ui-preview), AC `.1` through `.8`.

```text
Pending A, user types B          Success A, B retained
[ B editable          ]   -->  [ B editable          ]
[ existing tools/send ]        [ existing tools/send ]
```

Preserve structure and existing localized copy. The only changed outcome is
acknowledgement-time text selection. Empty/whitespace-only B stays ineligible
to send; unchanged/restored A clears; rejection keeps current text with existing
feedback. No layout comparison/browser run is required for this pure-state fix.

## Rendered regression matrix

New file: `apps/web/components/task/simple/task-chat.comment-send.test.tsx`.
Use `describe("TaskChat comment send draft preservation", ...)` with the names
below; table variants may be parameterized. AC numbers refer to
`AC-TASKS-COMMENT-DRAFT-001`.

| Named test/scenario | Required observation | AC |
| --- | --- | --- |
| `preserves changed raw text after older success` | Deferred A; replace text with B, multiline B, same trimmed A with changed outer whitespace, whitespace-only B, and empty B; success preserves each exact current value | `.1`, `.2`, `.4`, `.8` |
| `clears the unchanged acknowledged raw draft` | Leading/trailing whitespace and multiline A posts trimmed A, clears on success, refreshes once | `.2`, `.4` |
| `clears the exact submitted text restored before success` | A to B to exact raw A while pending; success clears | `.2` |
| `retains current text on rejected HTTP response` | Real non-2xx Response, unchanged A and edited B variants; exact current text, existing error feedback, no refresh, submission settles | `.3` |
| `retains current text on rejected fetch request` | Rejected fetch with unchanged A and edited B variants; same failure observations, no automatic request | `.3` |
| `admits keyboard and button sends consistently` | Both initial actions send once; while pending textarea editable, button disabled and Enter makes no additional POST; after settle admission restored | `.4`, `.5`, `.8` |
| `blocks empty and whitespace-only sends` | Both admission paths make no POST or callback | `.5` |
| `keeps Shift Enter available without submitting` | Handler does not prevent the native newline action or send; multiline request body tested through real textarea changes | `.5` |
| `sends the retained next draft on a later deliberate send` | A succeeds preserving B; next send posts trimmed B and unchanged B clears; two successes yield two callbacks total | `.6` |
| `allows an explicit retry after failure` | A fails with B retained; deliberate retry posts B and clears only on its own success; one success callback | `.3`, `.6` |
| `keeps independently mounted task composers isolated` | Two roots/task IDs with separate callbacks and deferred requests; settle in reverse order, including success/failure variants; each root retains/clears only its own current draft and pending flag | `.7` |
| `preserves asynchronously inserted file text before acknowledgement` | Real file-select handler reads a deferred text file during pending A; once actual Markdown insertion changes the textarea, success A retains it | `.1` |

Render actual `TaskChat`, `StateProvider` with `createAppStore`, `ToastProvider`,
`TooltipProvider`, and `ActiveSessionRefProvider`. Inspect existing provider
signatures and test setup before authoring. Keep the API client, synchronized
setter, textarea, button, and error feedback real. Do not reuse the heavily
stubbed timeline suite as causal evidence. Mock only fetch, explicitly routing
any auxiliary requests and rejecting unexpected calls. Verify actual
`/api/v1/office/tasks/<taskId>/comments`, `method: POST`, parsed trimmed body,
`author_type: user`, request counts, and exact callback counts. Use actual API
success/error response shapes. Scope selectors to each root; find the existing
send control through its actual tooltip/control structure without adding labels
or test IDs solely for these tests.

Use deferred promises and causal awaits inside `act`, without wall-clock sleeps.
Happy DOM does not implement native textarea newline insertion: for Shift+Enter
assert no preventDefault/no POST, and test multiline content through change
events. Do not pretend synthetic keydown proves native text insertion. File text
may be supplied by the test File's deferred `text()` method; keep processing
logic real. Settle every admitted fetch/read before cleanup, restore mocks, and
verify no unexpected request escaped the test fixture.

## Execution prerequisites and TDD

ROOT reviewed the exact four-file package after DESIGN END and sent the later
explicit implementation interrupt to session
`94cbc674-dd0b-4a9d-ac5d-2c6cee47a570`. Review evidence is recorded in
`/tmp/kandev-root-child85-design-review-20261008.json`. This work order is now
implemented; the scope and eight acceptance criteria remain unchanged.

Child84 returned GLOBAL LOCAL-HEAVY, independently qualified by ROOT. ROOT has
now granted EXCLUSIVE GLOBAL LOCAL-HEAVY85 for serial local work.
`apps/node_modules` was absent during
design. If still absent, perform exactly one frozen install with pinned
pnpm9.15.9, from repo root:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24/bin:$PATH"
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
```

Do not change lockfiles, use bypass flags, or duplicate the install after an
unknown outcome. Resource/timeout/transport/unknown failures checkpoint ROOT
before retry. Retain original handles and ACTUALLYJOIN every execution; record
actual command verdicts, terminal chunks, process groups, and absence before
explicit lease RETURN. Run commands serially, not overlapping other heavy work.

Write the new rendered regression suite independently. The accepted ROOT proof
already establishes the causal defect; do not replay it for reassurance. Run
the new suite against unchanged production for a causal RED (next raw draft is
erased) with ordinary success/failure controls. Then change only the clearing
call and run the targeted GREEN set. Routine causal fixture/lint repair stays
affected-only; checkpoint ROOT for unknown/outside-scope results before retry.

## Verification

Run from repo root after the implementation and local-heavy grants. Node 24
is required for existing i18n checks. Retain every original session handle and
join to its actual terminal result. Pin the same pnpm version; one worker and
4 GiB Node budget are mandatory.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24/bin:$PATH"
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 --project=browser-locales components/task/simple/task-chat.comment-send.test.tsx components/task/simple/task-chat.test.tsx hooks/use-prompt-result-delivery.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 components/task/simple/task-chat.tsx components/task/simple/task-chat.comment-send.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:ratchet --base c2c243b2ac1ad4068f0062c823d7c0bd2597e677)
```

For the new suite's RED run, select only
`components/task/simple/task-chat.comment-send.test.tsx` with the same project,
worker and Node flags. Run the complete targeted block after the production
correction. If causal fixture/lint repair changes any additional suite or source
path, add it explicitly to targeted tests and lint and record the actual
changed-path coverage. Test helper files need lint; do not broaden to a full suite.

Documentation-only checks (also permitted in DESIGN):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24/bin:$PATH"
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'c2c243b2ac1ad4068f0062c823d7c0bd2597e677'], { encoding: 'utf8' });
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' });
const changedFiles = [...new Set((tracked + untracked).split('\0').filter(Boolean))];
const documentPaths = [
  'docs/specs/tasks/requirements/comment-draft-preservation.md',
  'docs/specs/tasks/system-design/comment-draft-preservation.md',
  'docs/plans/preserve-next-task-comment/plan.md',
  'docs/plans/preserve-next-task-comment/task-01-preserve-comment-draft.md',
];
const fileContents = Object.fromEntries(documentPaths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ changedFiles, ...result }, null, 2));
if (!result.ok) process.exitCode = 1;
// Docs-only changes are exempt from PR coverage; still prove package links now.
const { parseFrontmatter } = require('./.github/scripts/pr-docs.cjs');
const assert = require('node:assert/strict');
const path = require('node:path');
const [requirementPath, designPath, planPath, workOrderPath] = documentPaths;
const requirement = fileContents[requirementPath];
const design = parseFrontmatter(fileContents[designPath]).data;
const plan = parseFrontmatter(fileContents[planPath]);
const order = parseFrontmatter(fileContents[workOrderPath]).data;
const resolve = (from, reference) => path.posix.normalize(path.posix.join(path.posix.dirname(from), reference));
assert.equal(resolve(workOrderPath, order.plan), planPath);
assert.ok(plan.body.includes('(task-01-preserve-comment-draft.md)'));
for (const reference of order.system_design) {
  assert.equal(resolve(workOrderPath, reference), designPath);
  assert.ok(plan.data.system_design.some(item => resolve(planPath, item) === designPath));
}
for (const id of order.requirements) {
  assert.ok(requirement.includes('### ' + id + ':'));
  assert.ok(design.requirements.includes(id));
  assert.ok(plan.data.requirements.includes(id));
}
for (const id of order.acceptance_criteria) assert.ok(requirement.includes('**' + id + ':**'));
console.log('Package requirement/AC/design/manifest/work-order references passed.');
NODE
git diff --check
git status --short --untracked-files=all
```

Before completion verify actual changed files remain within this package and
owned code/test paths; no generated-file drift or silent omissions. Public-doc
validators are not needed while public docs remain unchanged. No browser,
build, broad suite, generic QA/simplify/review, or synthetic merge test is added.

The login shell did not expose Node during DESIGN. The PATH line selects the
already-installed repository Node24 runtime without installing tools. If that
runtime is later unavailable, checkpoint ROOT before attempting an install or
changing the verification environment.

## Files likely touched

- `apps/web/components/task/simple/task-chat.tsx`
- `apps/web/components/task/simple/task-chat.comment-send.test.tsx` (new)
- The four package artifacts for lifecycle/results only.

`synchronize-input-value.ts` and `use-prompt-result-delivery.ts` are read-only
integration dependencies. Existing test files should change only for a minimal
causal fixture repair, with affected-only verification recorded.

## Dependencies

No prior work orders. Later ROOT reviewed implementation and serial local-heavy
grants are required. Delivery follows [manifest gates](plan.md#execution-and-delivery-gates).

## Risks

- Raw whitespace, latest-ref comparison, callback placement, and ref/state
  synchronization must survive the one-line correction.
- Actual tooltip controls and auxiliary fetches require faithful fixtures;
  do not solve fixture failures by replacing real composer/API modules.
- Shared flags or snapshots would break independent mounted composers.

## Parallelism

`sequential`

## Inputs

- [Requirement and ACs](../../specs/tasks/requirements/comment-draft-preservation.md)
- [Completion rule and boundaries](../../specs/tasks/system-design/comment-draft-preservation.md)
- Actual `task-chat.tsx`, `synchronize-input-value.ts`, comment API/client,
  prompt-delivery hook, actual provider signatures and existing focused tests.
- ROOT accepted read-only proof metadata in the manifest; never access its test
  as reusable implementation/test input.

## Results

Implemented the single synchronized raw equality correction in
`ChatInput.handleSubmit`. The new rendered fetch-boundary suite has 21 cases;
all eight ACs are covered through actual providers/store, textarea/buttons,
comment API, and error feedback. No production helper/API changes or test seams.

- New causal RED: expected failure on lost NEXT, five passing controls.
- Planned scoped GREEN: 3 suites, 51 tests passed.
- After own fixture formatting/duplicate-string repair: new21 passed and
  changed-source/test ESLint passed. The existing 30 controls were unchanged.
- Web typecheck passed; i18n check and base-pinned ratchet passed.
- One conditional frozen pnpm9.15.9 install passed. Every original product
  handle was ACTUALLYJOINed, with owned process groups gone; full native receipts
  and raw logs are retained in `/tmp/kandev-child85-local/`.
- Documentation/catalog/actual changed-path coverage and normal active hook
  results are recorded in the manifest and version-safe platform task plan.
- Actual six-path coverage passed with status `covered`, accepted references,
  and no errors. The normal commit hook reformatted four fixture indentation
  lines; final formatted new21 cases passed before a new normal commit attempt.

The scoped commands above are the actual executed commands. The new-suite RED
selected `preserves changed raw text after older success: next instructions`,
unchanged-success and all four request/response failure controls. No product,
install, production/permanent tests were run during DESIGN; implementation
followed ROOT's later reviewed grant. No passing product command was repeated
without an affected test/source change. No browser, build, broad suite or proof
replay. Delivery and merge remain governed by ROOT's separate grants.
